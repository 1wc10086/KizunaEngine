/// 宿主文字渲染（C++ 引擎的字形光栅化委托给 Flutter 文字栈）。
///
/// 为什么这么做：krkr 引擎自己用 FreeType 光栅化字形，字形质量与字体可用性
/// 因平台而异；把「字形 → 位图」这一步交给 Flutter（TextPainter + Impeller，
/// 走系统字体），布局与合成仍留在 C++ 引擎里。enginec 侧对应
/// `environ/PlatformTextHook.h` 与 FFI 导出 `KzcEngineSetTextRenderer`。
///
/// 两个 isolate 的角色：
///  * 光栅化（[HostTextRasterizer]）跑在 root isolate。dart:ui 的
///    `Picture.toImage` / `Image.toByteData` 只保证在有 Flutter 引擎绑定的
///    isolate 上可用；引擎 worker 是 Isolate.spawn 出来的后台 isolate，不能假设
///    dart:ui 在那儿能跑。
///  * 回调注册（[installHostText]）必须在引擎 worker isolate 里完成：C 侧是在
///    `KzcEngineInvoke` 的调用线程上同步回调的，而 `NativeCallable.isolateLocal`
///    要求"只能在创建它的那个 isolate 的线程上被调用"。
///
/// 同步回调 + 异步光栅化：C 侧回调必须当场给出位图，而 Flutter 的文字光栅化
/// 只能异步完成。所以这里维护一张字形缓存：命中就当场 memcpy 进 C 侧缓冲；未命中
/// 立刻返回 false 让引擎这一帧用 FreeType 画，同时把请求发给 root isolate 预热
/// 缓存，下次同一字形就走宿主位图。最坏情况只是持续用 FreeType（与没接 hook 时
/// 完全一样），不会中断渲染，也不会让脚本卡住。
library;

import 'dart:async';
import 'dart:ffi';
import 'dart:isolate';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:ffi/ffi.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';

// ---------------------------------------------------------------------------
// C ABI 结构（与 enginec/ffi/include/enginec/ffi/api.cppm 的
// KzcTextRun/KzcTextBitmap 逐字段对齐：int 是 32 位，float 是 32 位）。
// ---------------------------------------------------------------------------

/// 一次字形 run 请求。text 是 UTF-16、不是 NUL 结尾（常为单个字符，可能含代理对）；
/// family 是 UTF-8 的字体族名（游戏请求的字体，可能为空串）。
final class KzcTextRun extends Struct {
  external Pointer<Uint16> text;

  @Int32()
  external int length;

  external Pointer<Utf8> family;

  @Float()
  external double size;

  /// 0xRRGGBBAA（C 侧 `// RGBA` 的字面顺序，不是 Flutter 的 ARGB）。
  @Uint32()
  external int color;

  /// C 侧 TVPFontstruc.h 的 TVP_TF_BOLD (0x0200) / TVP_TF_ITALIC (0x0100)。
  /// FreeType 会用 FT_GlyphSlot_Embolden/Oblique 合成粗斜体，宿主也必须合成，
  /// 否则游戏里加粗/斜体的正文会悄悄退回常规字重。
  @Uint32()
  external int flags;
}

/// 回给引擎的位图。pixels 由本文件用 malloc 分配、引擎用 free() 释放；布局是
/// 紧凑 RGBA8（pitch == width * 4），alpha 与 Flutter 的 rawRgba 一样是预乘的
/// （引擎图集也按预乘合成，所以不做反预乘，否则半透明描边/淡出会泛白）。
final class KzcTextBitmap extends Struct {
  external Pointer<Uint8> pixels;

  @Int32()
  external int width;

  @Int32()
  external int height;

  @Int32()
  external int pitch;

  /// 整段 run 的水平步进（引擎据此推进笔位）。
  @Int32()
  external int advance;

  /// 位图顶边到 alphabetic 基线的像素距离。旧实现由 C 侧按 height*0.8 猜，
  /// 字体 ascent 比例不同就会整体偏高/偏低 1–2px；这里回传 TextPainter 的精确值。
  @Int32()
  external int baseline;
}

typedef _KzcTextRenderC = Bool Function(
    Pointer<KzcTextRun> run, Pointer<KzcTextBitmap> out);
typedef _KzcSetTextRendererC = Void Function(
    Pointer<NativeFunction<_KzcTextRenderC>> callback);
// lookupFunction 的第二个类型参数要纯 Dart 签名：返回 void 而不是 FFI 的
// Void。两者是不同类型，混用会报 "Expected type 'Void Function(...)' to be
// 'void Function(...)'" 的类型错误。
typedef _KzcSetTextRendererDart = void Function(
    Pointer<NativeFunction<_KzcTextRenderC>> callback);

// ---------------------------------------------------------------------------
// 数据结构与常量
// ---------------------------------------------------------------------------

/// 一次光栅化的成品，跨 isolate 传的是纯数据（width/height/advance/baseline/pixels）。
final class _Glyph {
  const _Glyph(
      this.width, this.height, this.advance, this.baseline, this.pixels);

  final int width;
  final int height;
  final int advance;

  /// 顶边到基线的距离；C 侧据此把位图贴到引擎的 ascent 线上。
  final int baseline;

  final Uint8List pixels;

  int get byteLength => pixels.length;
}

/// 缓存键：同一字形按"文字 + 字体 + 字号 + 颜色 + 粗斜体"区分。颜色必须进键——
/// 引擎会把同一字形以不同颜色（正文 / 描边 / 淡出中间态）请求多次，位图本身带着
/// 颜色；粗斜体也必须进键，否则同一字形的常规与加粗版本会互相顶掉。
final class _RunKey {
  const _RunKey(this.text, this.family, this.size, this.argb, this.bold,
      this.italic);

  final String text;
  final String family;
  final double size;
  final int argb;
  final bool bold;
  final bool italic;

  @override
  bool operator ==(Object other) =>
      other is _RunKey &&
      other.text == text &&
      other.family == family &&
      other.size == size &&
      other.argb == argb &&
      other.bold == bold &&
      other.italic == italic;

  @override
  int get hashCode => Object.hash(text, family, size, argb, bold, italic);
}

/// 缓存上限：CJK 游戏一局常用字形几千个，单字形位图约 size²×4 字节，8MiB 足够
/// 覆盖正文 + 人名 + 少量特效字，又不至于和帧缓冲抢内存。
const int _maxCacheBytes = 8 << 20;
/// 单次 run 的字符数上限：引擎按单字/短 run 调用，超长说明调用方出了问题，
/// 拒绝比让 TextPainter 排一整段（并在同步回调里白白超时）安全。
const int _maxRunLength = 64;
/// 字号上限：异常字体配置（NaN/超大字号）会把位图分配撑爆。
const double _maxFontSize = 512;

/// C 侧 `TVPTextRun::color` 注释写的是 "RGBA"，即 `0xRRGGBBAA`；Flutter 的
/// `Color` 要 `0xAARRGGBB`。
///
/// 两个实现方（C++ 的 overlay、Dart 的回调）对 "RGBA" 可能各有理解，而 krkr
/// 引擎内部惯用 `0xAARRGGBB`：低字节为 0 而高字节非 0 时按 ARGB 解释更合理
/// （RGBA 下那是全透明文字，引擎不会来请求它），这样两种写法都能上色而不是
/// 渲染成一团黑色。
int _argbOf(int rgba) {
  if ((rgba & 0xFF) == 0 && (rgba >> 24) != 0) {
    return rgba; // 已经是 0xAARRGGBB
  }
  final alpha = rgba & 0xFF;
  return (alpha << 24) |
      ((rgba >> 8) & 0xFF) << 16 |
      ((rgba >> 16) & 0xFF) << 8 |
      ((rgba >> 24) & 0xFF);
}

/// 控制字符（换行/制表/回车）留给引擎自己排：TextPainter 单行会吞掉它们，
/// 语义就与原引擎不一致了。
bool _hasControlCodeUnit(String text) =>
    text.codeUnits.any((unit) => unit < 0x20 || unit == 0x7F);

/// 用 Flutter 文字栈把一个字形 run 光栅化成 RGBA8（预乘）位图。
///
/// 必须在有 Flutter 引擎绑定的 isolate 上调用（即 root isolate）；排版参数只有
/// 字号与字体族，行高/行文由引擎掌握，这里只负责"这个字形长什么样"。
/// [bold]/[italic] 与 C 侧的 FT_GlyphSlot_Embolden/Oblique 对应：宿主不合成，
/// 游戏里加粗/斜体的正文就会悄悄退回常规字重。
Future<_Glyph?> _rasterizeGlyph(String text, String family, double size,
    int argb, bool bold, bool italic) async {
  final painter = TextPainter(
    text: TextSpan(
      text: text,
      style: TextStyle(
        // 字体族名来自游戏（常是日文字体名）；系统没有这个字体时 Flutter 会
        // 自行做字体 fallback，CJK 字形仍能出来，这正是宿主渲染的价值所在。
        fontFamily: family.isEmpty ? null : family,
        fontSize: size,
        color: ui.Color(argb),
        // 合成粗体/斜体，与引擎 FreeType 路径的选项位一一对应。
        fontWeight: bold ? FontWeight.bold : FontWeight.normal,
        fontStyle: italic ? FontStyle.italic : FontStyle.normal,
      ),
    ),
    textDirection: ui.TextDirection.ltr,
    // 宿主的系统字号缩放不能改变引擎坐标：位图严格按引擎字号一比一光栅化，
    // 否则合成后的正文会和引擎算好的行高/基线对不上。
    // TextScaler 在 painting 库（不是 dart:ui），所以不带 ui. 前缀。
    textScaler: TextScaler.noScaling,
    maxLines: 1,
  );
  ui.Image? image;
  try {
    painter.layout();
    final int advance = painter.width.round();
    // 光栅尺寸向上取整，避免最后一列/行像素被裁掉；宽高至少 1，空字形也要能
    // 交出合法缓冲（引擎按 advance 推进笔位，位图可以是 1×1 全透明）。
    //
    // 垂直方向额外留白：painter.height 是字体声明的行盒，而不少 CJK 字体
    // （思源/Noto、MS Gothic…）声明的 descent 比实际字形的墨迹浅 —— 吉、口、
    // 直 这类字底横画会探出行盒底部，picture.toImage 直接按画布裁掉，表现就是
    // "字底少一横"。顶部留白不影响落点：回传的 baseline 相对位图顶边，C 侧用
    // ascent - baseline 抵消；左侧不能留白，因为 C 侧把 ink 的 box.left 当作
    // 笔位偏移。右侧留白只防斜体/圆头字的右缘。
    final int padV = math.max(3, (size * 0.30).ceil());
    final int padRight = math.max(2, (size * 0.10).ceil());
    final int width = math.max(1, painter.width.ceil()) + padRight;
    final int height = math.max(1, painter.height.ceil()) + padV * 2;
    // 顶边到 alphabetic 基线的距离。C 侧原来按 height*0.8 猜，字体 ascent
    // 比例不同就整体偏 1–2px；这里交精确值，引擎据此贴 ascent 线。
    final double measuredBaseline =
        painter.computeDistanceToActualBaseline(TextBaseline.alphabetic);
    final int baseline =
        (measuredBaseline.isFinite ? measuredBaseline.round() : 0) + padV;
    final recorder = ui.PictureRecorder();
    final canvas = ui.Canvas(recorder);
    painter.paint(canvas, ui.Offset(0, padV.toDouble()));
    final picture = recorder.endRecording();
    try {
      image = await picture.toImage(width, height);
    } finally {
      picture.dispose();
    }
    final data = await image.toByteData(format: ui.ImageByteFormat.rawRgba);
    if (data == null) {
      return null;
    }
    return _Glyph(
      width,
      height,
      advance,
      baseline,
      Uint8List.fromList(
          data.buffer.asUint8List(data.offsetInBytes, data.lengthInBytes)),
    );
  } finally {
    image?.dispose();
    painter.dispose();
  }
}

// ---------------------------------------------------------------------------
// root isolate：光栅化服务（进程内单例）
// ---------------------------------------------------------------------------

/// 字形光栅化服务，跑在 root isolate（有 Flutter 引擎绑定的那个）。
///
/// 引擎 worker 每个缓存缺失的字形发一条消息过来：
/// `[id:int, text:String, family:String, size:double, argb:int, flags:int, reply:SendPort]`；
/// 成功回 `[id, width, height, advance, baseline, pixels]`，失败回全 null。
/// 失败的字形在 worker 侧被记成"不可用"，以后直接走 FreeType。
final class HostTextRasterizer {
  HostTextRasterizer._() {
    _port.listen(_onRequest);
  }

  static HostTextRasterizer? _instance;

  /// 取进程内唯一实例（懒建）。
  ///
  /// 单例是必需的：`installHostText()` 在 main.dart 里调用一次（root isolate），
  /// `EngineBridge.spawn` 每次开游戏还要再取一次端口；两者必须是同一个实例，
  /// 端口只开一个，也避免热重启时漏掉旧的 ReceivePort。
  static HostTextRasterizer ensure() => _instance ??= HostTextRasterizer._();

  /// 进程退出/热重启时释放端口。
  static void disposeInstance() {
    _instance?.dispose();
    _instance = null;
  }

  final ReceivePort _port = ReceivePort();
  bool _closed = false;

  /// 交给引擎 worker 的请求端口。
  SendPort get sendPort => _port.sendPort;

  void dispose() {
    if (_closed) {
      return;
    }
    _closed = true;
    _port.close();
  }

  Future<void> _onRequest(Object? message) async {
    if (_closed || message is! List || message.length < 7) {
      return;
    }
    final id = message[0];
    final reply = message[6];
    final flags = message[5];
    if (id is! int || reply is! SendPort || flags is! int) {
      return;
    }
    _Glyph? glyph;
    try {
      glyph = await _rasterizeGlyph(
          message[1] as String,
          message[2] as String,
          (message[3] as num).toDouble(),
          message[4] as int,
          // 与 C 侧 TVP_TF_BOLD / TVP_TF_ITALIC 相同的位置。
          (flags & 0x0200) != 0,
          (flags & 0x0100) != 0);
    } catch (error) {
      debugPrint('host_text: rasterize failed: $error');
    }
    reply.send(<Object?>[
      id,
      glyph?.width,
      glyph?.height,
      glyph?.advance,
      glyph?.baseline,
      glyph?.pixels,
    ]);
  }
}

// ---------------------------------------------------------------------------
// worker isolate：C 回调
// ---------------------------------------------------------------------------

NativeCallable<_KzcTextRenderC>? _callable;
Pointer<NativeFunction<_KzcTextRenderC>>? _installed;
DynamicLibrary? _library;
/// root isolate 的光栅化服务端口。
SendPort? _rasterizerPort;
ReceivePort? _replyPort;
final Map<_RunKey, _Glyph> _cache = <_RunKey, _Glyph>{};
/// 已发出、等回包的字形：请求 id → key（回包只带 id，用它反查）。同时充当
/// "在飞集合"：回调每帧都会为同一个未命中字形进来，不去重就会把 root isolate
/// 的排版队列灌满同一批请求。
final Map<int, _RunKey> _inflight = <int, _RunKey>{};
final Set<_RunKey> _inflightKeys = <_RunKey>{};
int _nextRequestId = 1;
int _cacheBytes = 0;
/// 光栅化整体不可用（root isolate 不可达、dart:ui 在注册侧不可用等）时置位：
/// 之后所有回调一律返回 false，引擎全程用 FreeType。
bool _broken = false;
/// 光栅化失败过的字形不再重试（缺字/字体名不合法是稳定结果，重试只是白烧 CPU）。
final Set<_RunKey> _failed = <_RunKey>{};

/// 把 Flutter 字形光栅化器注册给 enginec。
///
/// 未注册或光栅化失败时，C++ 侧自动回退到 FreeType，不会中断渲染。
///
/// 两种调用形态：
///  * `installHostText()`：root isolate 里的入口（main.dart 启动阶段）。只确保
///    [HostTextRasterizer] 服务就绪，**不**注册 C 回调——见下。
///  * `installHostText(library, port)`：引擎 worker isolate 里的真实注册，由
///    `EngineBridge.spawn` 在创建引擎前调用。
///
/// 为什么 root isolate 不能注册：C 侧 hook 是进程级全局函数指针，引擎是在
/// `KzcEngineInvoke` 的**调用线程**上同步回调的，而 `NativeCallable.isolateLocal`
/// 只允许在创建它的那个 isolate 上被调用。在 root isolate 注册、再由 worker
/// isolate 调 invoke，等于跨 isolate 调一个 isolate-local 回调——未定义行为。
/// 所以注册权交给"真正跑 invoke 的那个 isolate"，root 侧只准备光栅化服务。
///
/// [library] 是已打开的 libenginec（worker isolate 在创建引擎前已经打开过它，
/// 直接复用，避免重复 dlopen）。[rasterizer] 是 root isolate 的
/// [HostTextRasterizer.sendPort]；省略时认为注册方自己就在 root isolate 上，
/// 就地异步光栅化（只有单 isolate 的宿主会走到这条路）。
///
/// 重复调用是幂等的：hook 是进程级全局变量，重复注册只会覆盖成同一个回调。
void installHostText([DynamicLibrary? library, SendPort? rasterizer]) {
  if (library == null) {
    // root isolate：只保证服务存在，注册留给引擎 worker。
    HostTextRasterizer.ensure();
    return;
  }
  if (_installed != null) {
    return;
  }
  try {
    _library = library;
    final setRenderer = _library!.lookupFunction<_KzcSetTextRendererC,
        _KzcSetTextRendererDart>('KzcEngineSetTextRenderer');
    if (rasterizer != null) {
      final replies = ReceivePort();
      _replyPort = replies;
      _rasterizerPort = rasterizer;
      replies.listen(_onRasterized);
    }
    // exceptionalReturn：万一有异常逃出回调（不该发生，_onTextRun 自己兜住了），
    // 让 C 侧拿到 false 走 FreeType，而不是把未定义的值当成 true。
    final callable = NativeCallable<_KzcTextRenderC>.isolateLocal(_onTextRun,
        exceptionalReturn: false);
    setRenderer(callable.nativeFunction);
    _callable = callable;
    _installed = callable.nativeFunction;
  } catch (error) {
    // 旧版 libenginec 没有这个导出，或当前平台没有 enginec：保持 FreeType。
    debugPrint('host_text: text renderer not installed: $error');
    _callable?.close();
    _callable = null;
    _installed = null;
    _replyPort?.close();
    _replyPort = null;
    _rasterizerPort = null;
  }
}

/// 注销并释放回调，恢复 FreeType。
///
/// 顺序有硬要求：先让 C 侧把 hook 置空，再 close NativeCallable。反过来时，
/// 中间窗口里引擎如果还在渲染（compose 线程），就会调到已释放的函数指针。
void uninstallHostText() {
  final installed = _installed;
  final library = _library;
  _installed = null;
  _library = null;
  if (installed != null && library != null) {
    try {
      final setRenderer = library.lookupFunction<_KzcSetTextRendererC,
          _KzcSetTextRendererDart>('KzcEngineSetTextRenderer');
      setRenderer(nullptr);
    } catch (_) {
      // 库已卸载 / 导出缺失：没有可清理的全局指针。
    }
  }
  _callable?.close();
  _callable = null;
  _replyPort?.close();
  _replyPort = null;
  _rasterizerPort = null;
  _cache.clear();
  _inflight.clear();
  _inflightKeys.clear();
  _failed.clear();
  _cacheBytes = 0;
  _broken = false;
}

bool _onTextRun(Pointer<KzcTextRun> runPtr, Pointer<KzcTextBitmap> outPtr) {
  if (runPtr == nullptr || outPtr == nullptr || _broken) {
    return false;
  }
  try {
    final run = runPtr.ref;
    final length = run.length;
    if (length <= 0 || length > _maxRunLength || run.text == nullptr) {
      return false;
    }
    final size = run.size;
    if (!size.isFinite || size <= 0 || size > _maxFontSize) {
      return false;
    }
    final text = String.fromCharCodes(run.text.asTypedList(length));
    if (_hasControlCodeUnit(text)) {
      return false;
    }
    // C 侧 glyph run 可能按"字素簇"下发（日文浊音的组合字符、代理对）；
    // TextPainter 会把它们当一段排，位图宽度自然就是整段的 advance。
    final family = run.family == nullptr ? '' : run.family.toDartString();
    final flags = run.flags;
    final key = _RunKey(text, family, size, _argbOf(run.color),
        (flags & 0x0200) != 0, (flags & 0x0100) != 0);
    final glyph = _cache[key];
    if (glyph == null) {
      _request(key);
      return false;
    }
    // 命中：分配一块 C 侧可 free() 的内存并拷进去。分配放在所有校验之后，
    // 上面任何 return false 都不会漏掉这块内存。
    final pixels = malloc<Uint8>(glyph.byteLength);
    if (pixels == nullptr) {
      return false;
    }
    pixels.asTypedList(glyph.byteLength).setAll(0, glyph.pixels);
    final out = outPtr.ref;
    out.pixels = pixels;
    out.width = glyph.width;
    out.height = glyph.height;
    out.pitch = glyph.width * 4;
    out.advance = glyph.advance;
    out.baseline = glyph.baseline;
    return true;
  } catch (error) {
    // 回调绝不能把异常抛进 C 栈：返回 false 让引擎用 FreeType。
    debugPrint('host_text: callback failed: $error');
    return false;
  }
}

/// 把缓存缺失的字形排进光栅化队列。
///
/// 这里在引擎同步调用栈上执行，只做"发消息 / 排 timer"这种 O(1) 的活；真正的
/// TextPainter 排版在事件循环里跑，不占住调用线程。
void _request(_RunKey key) {
  if (_broken || _failed.contains(key) || !_inflightKeys.add(key)) {
    return;
  }
  final id = _nextRequestId++;
  _inflight[id] = key;
  final port = _rasterizerPort;
  final replies = _replyPort;
  if (port != null && replies != null) {
    try {
      port.send(<Object?>[
        id,
        key.text,
        key.family,
        key.size,
        key.argb,
        // 与 C 侧结构一致：bold=0x0200, italic=0x0100。
        (key.bold ? 0x0200 : 0) | (key.italic ? 0x0100 : 0),
        replies.sendPort,
      ]);
      return;
    } catch (error) {
      // 端口已关（root isolate 销毁）：不再尝试跨 isolate，直接放弃。
      _inflight.remove(id);
      _inflightKeys.remove(key);
      _broken = true;
      debugPrint('host_text: rasterization disabled: $error');
      return;
    }
  }
  // 注册方自己就在 root isolate 上：就地异步光栅化。Timer.run 把工作推到本次
  // 同步调用之后——回调里不能 await，否则引擎线程会被自己的事件循环卡住。
  Timer.run(() async {
    _Glyph? glyph;
    try {
      glyph = await _rasterizeGlyph(
          key.text, key.family, key.size, key.argb, key.bold, key.italic);
    } catch (error) {
      debugPrint('host_text: rasterization disabled: $error');
      _broken = true;
    } finally {
      _inflight.remove(id);
      _inflightKeys.remove(key);
    }
    if (glyph == null) {
      _failed.add(key);
      return;
    }
    _put(key, glyph);
  });
}

void _onRasterized(Object? message) {
  if (message is! List || message.length < 6) {
    return;
  }
  final id = message[0];
  if (id is! int) {
    return;
  }
  final key = _inflight.remove(id);
  if (key == null) {
    return; // 迟到的回包（注销/重建缓存之后）：直接丢。
  }
  _inflightKeys.remove(key);
  final width = message[1];
  final baseline = message[4];
  if (width is! int || baseline is! int) {
    _failed.add(key);
    return;
  }
  final pixels = message[5];
  if (pixels is! Uint8List) {
    _failed.add(key);
    return;
  }
  _put(key,
      _Glyph(width, message[2] as int, message[3] as int, baseline, pixels));
}

void _put(_RunKey key, _Glyph glyph) {
  _cache[key] = glyph;
  _cacheBytes += glyph.byteLength;
  // 插入顺序近似 LRU：超预算就从最老的开始丢。真正的访问时间排序要在同步
  // 热路径上做额外簿记，不值得。
  while (_cacheBytes > _maxCacheBytes && _cache.length > 1) {
    final oldest = _cache.keys.first;
    final removed = _cache.remove(oldest);
    if (removed != null) {
      _cacheBytes -= removed.byteLength;
    }
  }
}
