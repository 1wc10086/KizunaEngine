import 'dart:async';
import 'dart:typed_data';
import 'dart:ui' as ui;

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

class FramePainter extends CustomPainter {
  final ui.Image? image;

  const FramePainter(this.image);

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
  }

  @override
  bool shouldRepaint(FramePainter oldDelegate) =>
      oldDelegate.image != image;
}

class FrameView extends StatefulWidget {
  final Uint8List? bytes;
  final int width;
  final int height;

  /// 解码忙闲回报（可选）：合成侧据此做背压，解码没跑完就不去要新帧。
  final ValueChanged<bool>? onDecoding;

  const FrameView(
      {super.key,
      required this.bytes,
      required this.width,
      required this.height,
      this.onDecoding});

  @override
  State<FrameView> createState() => _FrameViewState();
}

class _FrameViewState extends State<FrameView> {
  ui.Image? _image;
  Uint8List? _decoded;
  Uint8List? _pending;
  int _pendingWidth = 0;
  int _pendingHeight = 0;
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
        if (identical(bytes, _decoded)) {
          _pending = null;
          return;
        }
        final width = _pendingWidth;
        final height = _pendingHeight;
        if (width <= 0 || height <= 0) {
          _decoded = bytes;
          _pending = null;
          final old = _image;
          _image = null;
          old?.dispose();
          if (mounted) {
            setState(() {});
          }
          return;
        }
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
        if (!identical(bytes, _pending)) {
          img.dispose();
          continue;
        }
        _decoded = bytes;
        _pending = null;
        final old = _image;
        _image = img;
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
      child: CustomPaint(painter: FramePainter(_image)),
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
      onDecoding: widget.bus.setDecoding,
    );
  }
}
