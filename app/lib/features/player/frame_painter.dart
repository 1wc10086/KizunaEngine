import 'dart:async';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import 'frame_channel.dart';

/// RGBA 字节 → [ui.Image]，全在 UI isolate 上跑。
///
/// 这里有一次无法回避的拷贝：[ui.ImmutableBuffer.fromUint8List] 会把 [bytes]
/// 拷进引擎侧内存（Uint8List 走的就是这条拷贝路径，没有只借指针的 API）。
/// [ui.ImageDescriptor] 不需要 targetWidth/targetHeight —— 目标尺寸就是原尺寸，
/// 递进去只会多走一次缩放采样。
///
/// 临时对象的释放顺序：codec → descriptor → buffer（descriptor 自己持有
/// buffer 的引用，先放 buffer 会在 debug 下报"descriptor 用到了已释放的 buffer"）。
/// 返回的 image 已经解码完成，不依赖这两个对象，可以安全释放。
Future<ui.Image> decodeRgbaImage(
    Uint8List bytes, int width, int height) async {
  final buffer = await ui.ImmutableBuffer.fromUint8List(bytes);
  ui.ImageDescriptor? descriptor;
  try {
    descriptor = ui.ImageDescriptor.raw(
      buffer,
      width: width,
      height: height,
      pixelFormat: ui.PixelFormat.rgba8888,
    );
    final codec = await descriptor.instantiateCodec();
    try {
      final frame = await codec.getNextFrame();
      return frame.image;
    } finally {
      codec.dispose();
    }
  } finally {
    descriptor?.dispose();
    buffer.dispose();
  }
}

/// 排版结果缓存：一次 [TextPainter.layout] 就是一次字形整形 + 断行，日文文本
/// 尤其贵。krkr 的动画期间位图每帧都换、文字却几乎不动（对白静止、只有背景/
/// 立绘在动），不缓存就等于每帧给同样的文字重新做一次排版。
///
/// 只在 UI isolate 上访问（FramePainter.paint），所以不需要锁。键包含所有会
/// 改变排版或外观的输入：文本、字号、字重、颜色、折行宽度、描边色与线宽。
/// 颜色也计入键是刻意的——文字淡入会逐帧改 alpha，那段时间缓存会失效，但
/// 那种动画本身的时长有限，而"背景在动、文字不动"才是常态。
class _TextLayoutCache {
  // 一条对白通常只有几十个 run，128 条足以覆盖"上一句 + 当前句"并留出余量；
  // 超出后按插入顺序淘汰（对话流本身就是时序的，FIFO 比 LRU 更省一个链表）。
  static const int _capacity = 128;
  final Map<String, TextPainter> _painters = <String, TextPainter>{};
  final List<String> _order = <String>[];
  // 被淘汰但还没释放的 TextPainter。不能在淘汰现场就 dispose：同一个 paint
  // 里可能已经取出了某个 painter（例如正文），紧接着的影子/描边 layout 把它
  // 挤出了缓存，之后再用它画就会打到已释放的原生段落。这里先挂起，等本次
  // paint 结束（FramePainter.paint 的 finally）再统一释放。
  final List<TextPainter> _retired = <TextPainter>[];

  TextPainter layout(String text, TextStyle style, double maxWidth, String key) {
    final cached = _painters[key];
    if (cached != null) {
      return cached;
    }
    final painter = TextPainter(
      text: TextSpan(text: text, style: style),
      textDirection: TextDirection.ltr,
    );
    painter.layout(maxWidth: maxWidth);
    if (_order.length >= _capacity) {
      final oldest = _order.removeAt(0);
      final evicted = _painters.remove(oldest);
      if (evicted != null) {
        _retired.add(evicted);
      }
    }
    _painters[key] = painter;
    _order.add(key);
    return painter;
  }

  /// 释放本轮不再被引用的 painter。只在一次 paint 全部画完之后调用。
  void flushRetired() {
    if (_retired.isEmpty) {
      return;
    }
    for (final painter in _retired) {
      painter.dispose();
    }
    _retired.clear();
  }

  void dispose() {
    flushRetired();
    for (final painter in _painters.values) {
      painter.dispose();
    }
    _painters.clear();
    _order.clear();
  }
}

// 进程级单例：FramePainter 每帧都会重建（CustomPainter 的正常用法），缓存必须
// 活得比它久；播放页销毁时由 [clearTextLayoutCache] 收尾。
final _TextLayoutCache _textLayoutCache = _TextLayoutCache();

/// 释放排版缓存里的 TextPainter（原生段落对象）。播放页退出时调用。
void clearTextLayoutCache() => _textLayoutCache.dispose();

class FramePainter extends CustomPainter {
  final ui.Image? image;

  /// 与 [image] 配套的文字覆盖层（引擎坐标系）。
  final List<TextRun> runs;

  const FramePainter(this.image, {this.runs = const <TextRun>[]});

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(
        Offset.zero & size, Paint()..color = const Color(0xFF000000));
    final img = image;
    if (img == null) {
      return;
    }
    paintImage(
      canvas: canvas,
      rect: Offset.zero & size,
      image: img,
      fit: BoxFit.contain,
      filterQuality: FilterQuality.medium,
    );
    if (runs.isEmpty || img.width <= 0 || img.height <= 0) {
      return;
    }
    // 文字坐标是引擎坐标系（与位图同尺寸），必须按 paintImage 同一套 contain
    // 计算落到屏幕上的矩形，否则留边或裁切时会和画面错位。
    final fitted = applyBoxFit(
      BoxFit.contain,
      Size(img.width.toDouble(), img.height.toDouble()),
      size,
    );
    final rect =
        Alignment.center.inscribe(fitted.destination, Offset.zero & size);
    final scale = rect.width / img.width;
    if (scale <= 0) {
      return;
    }
    canvas.save();
    canvas.clipRect(rect);
    try {
      for (final run in runs) {
        _paintRun(canvas, run, rect, scale);
      }
    } finally {
      canvas.restore();
      // 本次绘制全部结束，本轮被淘汰的 TextPainter 现在才真的没有引用。
      _textLayoutCache.flushRetired();
    }
  }

  void _paintRun(Canvas canvas, TextRun run, Rect rect, double scale) {
    final fontSize = run.size * scale;
    if (run.text.isEmpty || fontSize <= 0) {
      return;
    }
    final weight = run.bold ? FontWeight.bold : FontWeight.normal;
    final maxWidth = run.width > 0 ? run.width * scale : double.infinity;
    final left = rect.left + run.x * scale;
    final baseline = rect.top + run.y * scale;
    // 缓存键带上字号（scale 已折进去）与折行宽度：缩放/旋转屏幕后同一句文本
    // 必须重新排版，键里的数值就是这两者的最终像素值。
    final baseKey = '${run.text}\u0000$fontSize\u0000${run.bold}\u0000$maxWidth';

    // 阴影/描边只是同一段文字换画笔，字号与折行宽度都一样，所以共用正文的
    // ascent 定位顶边，不必各测一次基线。
    final fill = _textLayoutCache.layout(
      run.text,
      TextStyle(
        fontSize: fontSize,
        fontWeight: weight,
        color: Color(run.color),
      ),
      maxWidth,
      '$baseKey\u0000fill\u0000${run.color}',
    );
    final ascent =
        fill.computeDistanceToActualBaseline(TextBaseline.alphabetic);

    // 顺序照原引擎：阴影 → 描边 → 正文。阴影/描边先画，正文才不会被盖住。
    if (run.shadow != 0) {
      // 阴影沿用正文的 alpha（约六成浓度）：正文淡出时阴影一起变淡，不会留下硬边。
      final alpha = (((run.color >> 24) & 0xFF) * 0.6).round();
      final offset = run.shadow * scale;
      final painter = _textLayoutCache.layout(
        run.text,
        TextStyle(
          fontSize: fontSize,
          fontWeight: weight,
          color: Color.fromARGB(alpha, 0, 0, 0),
        ),
        maxWidth,
        '$baseKey\u0000shadow\u0000$alpha',
      );
      painter.paint(canvas, Offset(left + offset, baseline + offset - ascent));
    }
    if (((run.outline >> 24) & 0xFF) != 0) {
      // 描边宽度按缩放比走：引擎的 edge 是 1 引擎像素，2 * scale 在当前画面
      // 缩放下能盖住抗锯齿的半透明边；再窄会出现灰边，所以下限取 1 逻辑像素。
      final strokeWidth = math.max(1.0, 2.0 * scale);
      final painter = _textLayoutCache.layout(
        run.text,
        TextStyle(
          fontSize: fontSize,
          fontWeight: weight,
          foreground: Paint()
            ..style = PaintingStyle.stroke
            ..strokeWidth = strokeWidth
            ..color = Color(run.outline),
        ),
        maxWidth,
        '$baseKey\u0000outline\u0000${run.outline}\u0000$strokeWidth',
      );
      painter.paint(canvas, Offset(left, baseline - ascent));
    }
    fill.paint(canvas, Offset(left, baseline - ascent));
  }

  @override
  bool shouldRepaint(FramePainter oldDelegate) =>
      oldDelegate.image != image || !listEquals(oldDelegate.runs, runs);
}

class FrameView extends StatefulWidget {
  final Uint8List? bytes;
  final int width;
  final int height;

  /// 位图内容号：只有像素真的变了才自增（见 FrameData.bitmap）。像素缓冲会被合成侧
  /// 复用，所以不能用对象身份判断"同不同一张图"。
  final int bitmap;

  /// 与 [bytes] 同批发布的文字覆盖层；画在帧位图之上。
  final List<TextRun> runs;

  /// 解码忙闲回报（可选）：合成侧据此做背压，解码没跑完就不去要新帧。
  final ValueChanged<bool>? onDecoding;

  const FrameView(
      {super.key,
      required this.bytes,
      required this.width,
      required this.height,
      this.bitmap = 0,
      this.runs = const <TextRun>[],
      this.onDecoding});

  @override
  State<FrameView> createState() => _FrameViewState();
}

class _FrameViewState extends State<FrameView> {
  ui.Image? _image;
  int? _decodedBitmap;
  Uint8List? _pending;
  int _pendingWidth = 0;
  int _pendingHeight = 0;
  int _pendingBitmap = 0;
  // 解码是异步的：文字必须和同一帧的位图一起上屏，不能跟着 widget 立刻更新，
  // 否则新文字会先画在旧画面上错开一拍。
  List<TextRun> _pendingRuns = const <TextRun>[];
  List<TextRun> _imageRuns = const <TextRun>[];
  bool _decoding = false;
  bool _reported = false;
  // 回调缓存一份：解码 await 回来时 State 可能已经 dispose，那时不能再读 widget。
  ValueChanged<bool>? _onDecoding;

  @override
  void initState() {
    super.initState();
    _onDecoding = widget.onDecoding;
    _maybeDecode();
  }

  @override
  void didUpdateWidget(FrameView oldWidget) {
    super.didUpdateWidget(oldWidget);
    _onDecoding = widget.onDecoding;
    _maybeDecode();
  }

  @override
  void dispose() {
    // 解码还在飞时销毁：这里先把忙标记归零，_decodeLoop 的 finally 再报一次就是空操作，
    // 否则合成侧要等 _decodeStuckMs 兜底才恢复出帧。
    _report(false);
    _image?.dispose();
    _image = null;
    super.dispose();
  }

  void _report(bool value) {
    if (value == _reported) {
      return;
    }
    _reported = value;
    _onDecoding?.call(value);
  }

  void _maybeDecode() {
    _pending = widget.bytes;
    _pendingWidth = widget.width;
    _pendingHeight = widget.height;
    _pendingBitmap = widget.bitmap;
    _pendingRuns = widget.runs;
    if (_decoding) {
      return;
    }
    unawaited(_decodeLoop());
  }

  // 解码是 UI isolate 的重活（3.7MB RGBA → ui.Image）；串行且只保留最新帧，
  // 防止角色 CG 连续变化时解码队列把 UI 堵死。忙标记的进入/退出成对出现
  // （_decoding 只在循环外沿置位，_report 在函数入口与 finally 各一次），
  // 中途 return/continue 都不会漏报。
  Future<void> _decodeLoop() async {
    _decoding = true;
    _report(true);
    try {
      while (mounted) {
        final bytes = _pending;
        if (bytes == null) {
          return;
        }
        // 位图没换（只换了文字）：不必重新解码。缓冲会被复用，所以按 bitmap 号比，
        // 不能按 Uint8List 身份比。
        if (_pendingBitmap == _decodedBitmap && _image != null) {
          if (!identical(_pendingRuns, _imageRuns)) {
            _imageRuns = _pendingRuns;
            if (mounted) {
              setState(() {});
            }
          }
          _pending = null;
          return;
        }
        final width = _pendingWidth;
        final height = _pendingHeight;
        if (width <= 0 || height <= 0) {
          _decodedBitmap = _pendingBitmap;
          _pending = null;
          _imageRuns = _pendingRuns;
          final old = _image;
          _image = null;
          old?.dispose();
          if (mounted) {
            setState(() {});
          }
          return;
        }
        // 记下这次要解的是哪一版位图：解码期间 _pendingBitmap 可能被新帧改掉，
        // 回来时按它判断"这张图是不是已经过时了"。
        final decodedBitmap = _pendingBitmap;
        final ui.Image img;
        try {
          img = await decodeRgbaImage(bytes, width, height);
        } catch (_) {
          _pending = null;
          return;
        }
        if (!mounted) {
          img.dispose();
          return;
        }
        // 解码期间又来了更新的帧：丢弃旧图，继续解码最新帧，避免排队。
        if (_pendingBitmap != decodedBitmap) {
          img.dispose();
          continue;
        }
        // 接受这张图，但不在这里清 _pending：解码期间可能又来了"只换文字"的发布
        // （逐字显示时位图不变），那一批 runs 必须跟着同一张位图上屏。
        // 回到循环顶部按 bitmap 号判断：位图没变就只更新文字，变了就再解一张。
        final old = _image;
        _image = img;
        _imageRuns = _pendingRuns;
        _decodedBitmap = decodedBitmap;
        old?.dispose();
        setState(() {});
      }
    } finally {
      _decoding = false;
      _report(false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // RepaintBoundary：位图换帧只重绘自己这一层，不连累 Stack 里的兄弟控件
    // （文本框、按钮、overlay）重新光栅化。
    return RepaintBoundary(
      child: CustomPaint(painter: FramePainter(_image, runs: _imageRuns)),
    );
  }
}

/// 播放页的帧视图：位图由 [FrameBus] 驱动，帧到达只重建这一个 widget。
///
/// 以前播放页在 playerProvider 上 select(frameBytes)，每合成一帧就通知一次全局
/// state，播放页所有订阅者都得重算一遍选择器；拆出 FrameBus 后这条路径只走
/// ValueNotifier，不经过 Riverpod。
class FrameSourceView extends StatefulWidget {
  final FrameBus bus;

  const FrameSourceView({super.key, required this.bus});

  @override
  State<FrameSourceView> createState() => _FrameSourceViewState();
}

class _FrameSourceViewState extends State<FrameSourceView> {
  FrameData? _frame;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    _frame = widget.bus.frame.value;
    _generation = _frame?.generation ?? 0;
    widget.bus.frame.addListener(_onFrame);
  }

  @override
  void didUpdateWidget(FrameSourceView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.bus == widget.bus) {
      return;
    }
    oldWidget.bus.frame.removeListener(_onFrame);
    _frame = widget.bus.frame.value;
    _generation = _frame?.generation ?? 0;
    widget.bus.frame.addListener(_onFrame);
  }

  @override
  void dispose() {
    // 监听挂在 bus 上（不是本 widget 上），必须显式摘掉，否则 bus 会一直持有
    // 已销毁 State 的回调。
    widget.bus.frame.removeListener(_onFrame);
    super.dispose();
  }

  void _onFrame() {
    final next = widget.bus.frame.value;
    if (identical(next, _frame) ||
        (next != null && next.generation == _generation)) {
      return;
    }
    _generation = next?.generation ?? 0;
    if (!mounted) {
      _frame = next;
      return;
    }
    setState(() {
      _frame = next;
    });
  }

  @override
  Widget build(BuildContext context) {
    final frame = _frame;
    return FrameView(
      bytes: frame?.bytes,
      width: frame?.width ?? 0,
      height: frame?.height ?? 0,
      bitmap: frame?.bitmap ?? 0,
      runs: frame?.runs ?? const <TextRun>[],
      onDecoding: widget.bus.setDecoding,
    );
  }
}
