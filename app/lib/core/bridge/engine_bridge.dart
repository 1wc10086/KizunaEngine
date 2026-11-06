import 'dart:async';
import 'dart:ffi';
import 'dart:isolate';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';

import 'native.dart';
import 'protocol_consts.dart';
import 'wire.dart';

class KzEngineException implements Exception {
  final String message;
  // 状态码随异常带出：frame() 必须区分"旧库不认识 OpFrame(37)"（StatusBadRequest）与真正的
  // 引擎错误（未 open、已 dispose…），后者若被当成"不支持"就会永久降级到旧三连 RPC。
  final int status;
  const KzEngineException(this.message, [this.status = KzStatus.error]);

  @override
  String toString() => 'KzEngineException: $message';
}

enum EngineStep {
  cont,
  awaitInput,
  awaitChoice,
  awaitSelect,
  wait,
  finished,
  faulted,
  awaitName,
  /// 停在主界面（标题菜单），等宿主回传点击。
  awaitTitle,
  /// 脚本在等影片播完，宿主播完调 [EngineBridge.movieDone]。
  awaitMovie;

  static EngineStep fromValue(int value) {
    switch (value) {
      case KzStep.awaitInput:
        return EngineStep.awaitInput;
      case KzStep.awaitChoice:
        return EngineStep.awaitChoice;
      case KzStep.awaitSelect:
        return EngineStep.awaitSelect;
      case KzStep.wait:
        return EngineStep.wait;
      case KzStep.finished:
        return EngineStep.finished;
      case KzStep.faulted:
        return EngineStep.faulted;
      case KzStep.awaitTitle:
        return EngineStep.awaitTitle;
      case KzStep.awaitMovie:
        return EngineStep.awaitMovie;
      case KzStep.awaitName:
        return EngineStep.awaitName;
      default:
        return EngineStep.cont;
    }
  }
}

typedef OpenResult = ({
  bool ok,
  int width,
  int height,
  String title,
  String version
});
typedef EngineInfo = ({
  int width,
  int height,
  String title,
  int sceneCount,
  int entryCount
});
typedef EngineText = ({String speaker, String text, int awaiting});
typedef EngineChoice = ({String name, int value});
typedef EngineScene = ({
  int id,
  String name,
  int group,
  int index,
  String title,
  List<String> images,
  bool unlocked
});
typedef EngineSlot = ({
  int index,
  String title,
  String savedAt,
  String summary,
  bool exists
});
typedef AudioEvent = ({
  int kind,
  String path,
  double volume,
  bool loop,
  int channel,
  int group,
  List<String> names,
  int frames
});
typedef EngineState = ({
  String bgm,
  int pc,
  int scene,
  bool animating,
  int waitMs,
  bool messageWindowVisible,
  // 这一帧的对话框是否已经由引擎画进帧位图；为 true 时 Flutter 不应再叠 MessageBox。
  bool engineMessageWindow,
  bool awaitingName,
  bool backlogActive,
  bool replayActive,
  int instructionStart,
  String namePrompt,
  // 引擎请求宿主执行的动作（settings/exit）；serial 单调递增用于判重。
  String hostRequest,
  int hostRequestSerial,
  // 自动推进/跳过策略：当前句是否未读 + 原版两个开关 + オート速度(0..100)。
  bool messageUnread,
  bool ceaseSkip,
  bool skipUnread,
  int autoWait
});
typedef TextEntry = ({String id, String text, String file});
typedef BrandInfo = ({String id, String name});
typedef ScanEntry = ({
  String brandId,
  String brandName,
  String name,
  String directory,
  String dataDirectory,
  String coverPath,
  List<String> extraDirs,
  String scriptPath,
  bool hasScript
});
typedef MapButton = ({
  int index,
  String image,
  int x,
  int y,
  int width,
  int height,
  int value,
  String label
});
typedef MapMask = ({bool has, int width, int height});
typedef SelectItem = ({
  int index,
  String text,
  String frame,
  int x,
  int y,
  int width,
  int height,
  int textX,
  int textY,
  int alignment,
  int defaultColor,
  int selectColor
});
typedef EngineFrame = ({int width, int height, Uint8List pixels});
// OpFrame(37) 返回的"一帧快照"：text/state/audio 用的字段号与旧 op 完全一致，
// 因此和 text()/state()/audio() 共用同一套解析函数（_parseText/_parseState/_parseAudio）。
typedef EngineFrameSnapshot = ({
  EngineStep step,
  EngineText text,
  EngineState state,
  List<AudioEvent> audio
});
typedef SlotPreview = ({
  String savedAt,
  String summary,
  String speaker,
  String text,
  int width,
  int height,
  Uint8List pixels
});
typedef CgGroup = ({String name, List<CgVariant> variants});
// 一个 CG 变体：images 是合成用的图层名，unlocked 来自全局存档的解锁位。
typedef CgVariant = ({List<String> images, bool unlocked});
// 主界面菜单项。坐标/尺寸是游戏坐标系（1024x768 那一套），
// 宿主按 FramePainter 的缩放比换算成逻辑像素。
typedef TitleMenuItem = ({
  String key,
  String label,
  int x,
  int y,
  int width,
  int height,
  bool enabled
});
// 待播影片：path 是绝对路径（宿主交给 fvp），serial 用来判重。
typedef MovieRequest = ({String path, int serial});
typedef DetectResult = ({String brandId, String brandName});

// 取请求头里的 seq：每条通道的所有 RPC 共用一个回包端口，靠 seq 配对，
// 所以 worker 的每条回包都必须带上请求里的 seq（包括 native 库加载失败的兜底应答）。
int _requestSeq(Object? message) {
  if (message is! Uint8List || message.length < 12) {
    return 0;
  }
  return ByteData.sublistView(message).getUint32(8, Endian.little);
}

// ready 消息 = [worker 的请求端口, engine handle 地址(int)]。地址为 0 表示 handle
// 没建起来（native 库缺失等）：主 isolate 据此跳过 compose worker，两个 worker 都照旧
// 对每个请求回错误响应。
void _engineWorkerMain(SendPort mainPort) {
  final incoming = ReceivePort();
  NativeEngineBindings? bindings;
  Pointer<Void>? handle;
  Object? failure;
  try {
    bindings = NativeEngineBindings.open();
    handle = bindings.create();
  } catch (error) {
    failure = error;
  }
  final created = handle;
  if (created == null) {
    mainPort.send(<Object?>[incoming.sendPort, 0]);
    incoming.listen((message) {
      final parts = message as List;
      final reply = parts[1] as SendPort;
      // shutdown（parts[0]==null）也要回 ack，否则 dispose 等不到。
      if (parts[0] == null) {
        incoming.close();
        reply.send(Uint8List(0));
        return;
      }
      reply.send(kzBuildErrorResponse(_requestSeq(parts[0]), '$failure'));
    });
    return;
  }
  final engine = bindings!;
  // 把 handle 地址交给主 isolate：它据此 spawn compose worker —— 两个 worker 共享同一个
  // 引擎实例，Go 侧按 uintptr 在 handles 表里查到同一个 *handle。
  mainPort.send(<Object?>[incoming.sendPort, created.address]);
  Pointer<Void>? owned = created;
  incoming.listen((message) {
    final parts = message as List;
    final reply = parts[1] as SendPort;
    if (parts[0] == null) {
      final h = owned;
      owned = null;
      incoming.close();
      reply.send(Uint8List(0));
      if (h != null) {
        engine.destroy(h);
      }
      return;
    }
    final request = parts[0] as Uint8List;
    try {
      reply.send(engine.invoke(owned!, request));
    } catch (error) {
      reply.send(kzBuildErrorResponse(_requestSeq(request), '$error'));
    }
  });
}

// compose 专用 worker：只跑 OpCompose，参数是 [回包/控制端口, engine handle 地址]。
//
// 它不创建也不销毁 handle —— 句柄归属权始终在主 worker 手里，这里拿到的只是一个整数地址，
// 用 Pointer.fromAddress 还原后直接交给 KzEngineInvoke。Go 侧每次调用都用 uintptr 在
// handles 表里查同一个 *handle：OpCompose 走 composeMu、其余 op 走 mu，锁顺序固定为
// composeMu → mu（见 engine/cmd/engine/main.go 的 dispatchCompose），两个线程并发调用
// 是设计允许的，因此两个 isolate 共享同一个句柄值是安全的。
//
// 生命周期：收到 shutdown（null）时只关闭自己的输入口并回 ack，绝不 destroy。
// 主 isolate 必须先等到这个 ack 才能让主 worker destroy handle —— listen 回调是串行
// 执行的，ack 只会在排在它前面的消息（含其中的同步 FFI 调用）彻底跑完之后才发出，
// 所以 ack 到手就等于"本 isolate 不会再碰这个指针"，此后 destroy 才是内存安全的。
void _composeWorkerMain(List<Object?> args) {
  final mainPort = args[0] as SendPort;
  final handle = Pointer<Void>.fromAddress(args[1] as int);
  final incoming = ReceivePort();
  mainPort.send(incoming.sendPort);
  NativeEngineBindings? bindings;
  try {
    bindings = NativeEngineBindings.open();
  } catch (_) {
    // native 库打不开就直接退出：主 isolate 的 onExit 会把这条通道标记为已死并 reject
    // 在飞请求，composeIsolate() 之后每一帧都回退到主 worker 的合成路径，画面不冻结。
    incoming.close();
    return;
  }
  final engine = bindings;
  incoming.listen((message) {
    final parts = message as List;
    final reply = parts[1] as SendPort;
    if (parts[0] == null) {
      incoming.close();
      reply.send(Uint8List(0));
      return;
    }
    final request = parts[0] as Uint8List;
    try {
      reply.send(engine.invoke(handle, request));
    } catch (error) {
      reply.send(kzBuildErrorResponse(_requestSeq(request), '$error'));
    }
  });
}

// 一条"主 isolate ↔ 某个 worker isolate"的 RPC 通道：一个请求端口、一个共享回包端口、
// 一张 seq→completer 配对表、独立的 seq 计数器，外加错误/退出端口。
// 主 worker 与 compose worker 各持一条：端口不同、配对表与 seq 空间互不相干，
// 两条通道同时有请求在飞也不会串号。
class _WorkerChannel {
  _WorkerChannel(this._port, this._inbox, this._errors, this._exits) {
    _inbox.listen(_onResponse);
    _errors.listen(_onError);
    _exits.listen(_onExit);
  }

  final SendPort _port;
  final ReceivePort _inbox;
  final ReceivePort _errors;
  final ReceivePort _exits;
  final Map<int, Completer<Uint8List>> _pending = <int, Completer<Uint8List>>{};
  // isolate 自己死掉（native 崩溃等）时用它叫醒 shutdown：否则等 ack 会永久挂住。
  final Completer<void> _exited = Completer<void>();
  int _seq = 0;
  bool _closed = false;

  /// isolate 已经退出（或崩溃）：在飞调用已被 reject，新的调用没有意义。
  bool get dead => _exited.isCompleted;

  // 跳过 0 与仍在飞的号：32 位回绕时才可能撞上。
  int _nextSeq() {
    var seq = _seq;
    do {
      seq = (seq + 1) & 0xFFFFFFFF;
    } while (seq == 0 || _pending.containsKey(seq));
    _seq = seq;
    return seq;
  }

  Future<KzReader> invoke(int op, Uint8List payload) async {
    if (_closed) {
      throw const KzEngineException('engine bridge is closed');
    }
    final seq = _nextSeq();
    final completer = Completer<Uint8List>();
    _pending[seq] = completer;
    try {
      _port.send(<Object?>[kzBuildRequest(op, seq, payload), _inbox.sendPort]);
      final raw = await completer.future;
      final response = kzParseResponse(raw);
      if (response.status != KzStatus.ok) {
        throw KzEngineException(
            response.reader.str(KzField.f42), response.status);
      }
      return response.reader;
    } finally {
      // 正常回包、被 reject、发起方提前放弃，都要把占位清掉，否则回绕时会误判 seq 占用。
      _pending.remove(seq);
    }
  }

  // 用响应头里的 seq 找回发起这次调用的 completer；迟到/未知 seq 直接丢弃。
  void _onResponse(Object? message) {
    if (message is! Uint8List || message.length < 16) {
      // 这个端口上只会有响应报文：出现短包说明协议已经错乱，
      // 不能让在飞请求静默挂死，统一报错。
      rejectPending('malformed engine response');
      return;
    }
    final seq = ByteData.sublistView(message).getUint32(8, Endian.little);
    final completer = _pending.remove(seq);
    if (completer == null || completer.isCompleted) {
      return;
    }
    completer.complete(message);
  }

  void _onError(Object? error) {
    rejectPending('engine isolate error: $error');
  }

  void _onExit(Object? _) {
    if (!_exited.isCompleted) {
      _exited.complete();
    }
    rejectPending('engine isolate exited');
  }

  // isolate 出错/退出后回包永远不会来：在飞调用必须以异常结束，而不是永久挂住。
  void rejectPending(String message) {
    if (_pending.isEmpty) {
      return;
    }
    final pending = _pending.values.toList(growable: false);
    _pending.clear();
    for (final completer in pending) {
      if (!completer.isCompleted) {
        completer.completeError(KzEngineException(message));
      }
    }
  }

  // 请 worker 关掉自己的输入口并回 ack。返回即代表该 isolate 不会再发起新的 FFI 调用
  // （回调串行，ack 排在它前面所有消息之后）；isolate 已经死过时由 dead 立刻叫醒，不空等。
  Future<void> shutdown() async {
    if (_closed) {
      return;
    }
    _closed = true;
    final ack = ReceivePort();
    _port.send(<Object?>[null, ack.sendPort]);
    await Future.any<void>(<Future<void>>[
      ack.first.then<void>((_) {}),
      _exited.future,
    ]);
    ack.close();
  }

  void close() {
    _inbox.close();
    _errors.close();
    _exits.close();
  }
}

class EngineBridge {
  // 逻辑通道：pointer/advance/run/frame/audio/save/scenes… 全部走它。
  final _WorkerChannel _main;
  final Isolate _isolate;
  // compose 专用通道：只跑 OpCompose，从而与逻辑 RPC 真正并发。
  // null 表示第二个 worker 没能起来（handle 没建起来或 spawn 失败）：此时
  // composeIsolate() 直接回退到 _main 的合成路径，行为与只有一个 worker 时一致。
  final _WorkerChannel? _compose;
  final Isolate? _composeIsolate;
  bool _closed = false;
  // 合成缓冲持久复用：原先每帧 calloc+free 3.69MB 会造成持续 GC 压力，
  // 分辨率不变时同一块内存可以一直用。两条通道共用同一块缓冲也不会并发写：
  // 调用方单飞，同一时刻最多只有一个 compose 在飞（Go 侧另有 composeMu 兜底）。
  Pointer<Uint8>? _composeBuf;
  int _composeBufSize = 0;
  // 记录在飞的 compose：dispose 时必须等它写完再释放缓冲，否则会踩已释放内存。
  Future<KzReader>? _composeFlight;
  // 旧库不认识 OpFrame(37) 时会回 StatusBadRequest；撞到一次即置 false，
  // 之后调用方直接走 text+state+audio 三连 RPC（等价旧行为），不再白跑一个失败往返。
  bool _frameSupported = true;

  EngineBridge._(
      this._main, this._isolate, this._compose, this._composeIsolate);

  static Future<EngineBridge> spawn() async {
    final ready = ReceivePort();
    final inbox = ReceivePort();
    final errors = ReceivePort();
    final exits = ReceivePort();
    // errorsAreFatal: false —— worker 里的未捕获异常经 errors 端口回报，在飞请求全部 reject；
    // 真死了还有 exits 端口叫醒等待方（原实现只有一个 ready 端口，请求会永久挂住）。
    try {
      final isolate = await Isolate.spawn(
        _engineWorkerMain,
        ready.sendPort,
        onError: errors.sendPort,
        onExit: exits.sendPort,
        errorsAreFatal: false,
      );
      final hello = await ready.first as List;
      ready.close();
      // handle 由主 worker 创建（ready 消息里带回地址），compose worker 只拿这个地址用。
      final handleAddress = hello[1] as int;
      _WorkerChannel? compose;
      Isolate? composeIsolate;
      if (handleAddress != 0) {
        // compose worker 用自己的一套端口：回包/错误/退出三对都与主 worker 独立，
        // 一边死掉不影响另一边，配对表也不会串号。
        final composeReady = ReceivePort();
        final composeInbox = ReceivePort();
        final composeErrors = ReceivePort();
        final composeExits = ReceivePort();
        try {
          composeIsolate = await Isolate.spawn<List<Object?>>(
            _composeWorkerMain,
            <Object?>[composeReady.sendPort, handleAddress],
            onError: composeErrors.sendPort,
            onExit: composeExits.sendPort,
            errorsAreFatal: false,
          );
          final composePort = await composeReady.first as SendPort;
          composeReady.close();
          compose = _WorkerChannel(
              composePort, composeInbox, composeErrors, composeExits);
        } catch (_) {
          // 第二个 isolate 起不来不该拖垮引擎：回收它的端口，
          // composeIsolate() 会走主 worker 的合成路径，画面照出，只是与逻辑 RPC 串行。
          composeIsolate?.kill();
          composeReady.close();
          composeInbox.close();
          composeErrors.close();
          composeExits.close();
          compose = null;
          composeIsolate = null;
        }
      }
      return EngineBridge._(
        _WorkerChannel(hello[0] as SendPort, inbox, errors, exits),
        isolate,
        compose,
        composeIsolate,
      );
    } catch (_) {
      // 起不来时（native 库缺失等）端口全部回收：留着就是四个收不到消息的 ReceivePort。
      ready.close();
      inbox.close();
      errors.close();
      exits.close();
      rethrow;
    }
  }

  static String nativeVersion() =>
      NativeEngineBindings.open().versionString();

  // 逻辑 RPC 全部经主通道；_closed 由 dispose 置位，之后一律立刻失败（与旧行为一致）。
  Future<KzReader> _invoke(int op, Uint8List payload) async {
    if (_closed) {
      throw const KzEngineException('engine bridge is closed');
    }
    return _main.invoke(op, payload);
  }

  Future<void> dispose() async {
    if (_closed) {
      return;
    }
    _closed = true;
    // 1) 先等在飞的 compose 收尾：这次合成可能已把 _composeBuf.address 交给 Go 侧写入，
    //    此刻 free 会让 Go 写进已释放的内存。_closed 已置位，新的 compose 不会开始。
    final flight = _composeFlight;
    if (flight != null) {
      try {
        await flight;
      } catch (_) {}
    }
    // 2) 关 compose worker 的输入口并等 ack。必须早于第 4 步：主 worker 的 shutdown 会
    //    destroy handle，此时若 compose worker 还能调用 KzEngineInvoke 就是 use-after-free。
    //    ack 的含义：它已经处理完排在 shutdown 之前的所有消息（回调串行），
    //    所以这一步之后这条通道不可能再有任何 FFI 在跑或在排队。
    final compose = _compose;
    if (compose != null) {
      await compose.shutdown();
    }
    // 3) 到这一步合成通道已彻底停用（第 1 步保证了没有在飞的合成，第 2 步的 ack 保证了
    //    没有排在后面的合成），可以安全释放持久合成缓冲。
    final buffer = _composeBuf;
    _composeBuf = null;
    _composeBufSize = 0;
    if (buffer != null) {
      calloc.free(buffer);
    }
    // 4) 再关主 worker：它的 shutdown 分支里 destroy(handle) 并回 ack。
    //    两个 worker 都可能已经死过：channel 内部让 ack 与 exit 竞速，不会无谓地等。
    await _main.shutdown();
    // 5) 两个 isolate 都确认不再执行 FFI 之后才 kill；此后回包不会再来，在飞请求全部 reject。
    _composeIsolate?.kill();
    _isolate.kill();
    _main.rejectPending('engine bridge is closed');
    compose?.rejectPending('engine bridge is closed');
    _main.close();
    compose?.close();
  }

  EngineStep _stepOf(KzReader reader) =>
      EngineStep.fromValue(reader.u32(KzField.f10));

  // 下面三个解析函数同时服务旧 op（Text/State/Audio）与 OpFrame(37)：字段号逐一对齐，
  // 共用一份实现避免新增 op 后两边解析漂移（缺字段时 KzReader 返回默认值，不会抛）。
  static List<AudioEvent> _parseAudio(KzReader reader) {
    final out = <AudioEvent>[];
    reader.each(KzField.f26, (sub) {
      out.add((
        kind: sub.u32(KzField.f27),
        path: sub.str(KzField.f1),
        volume: sub.f64(KzField.f28),
        loop: sub.boolean(KzField.f29),
        channel: sub.u32(KzField.f3),
        group: sub.u32(KzField.f69),
        names: sub.strList(KzField.f70),
        frames: sub.u32(KzField.f71),
      ));
    });
    return out;
  }

  static EngineText _parseText(KzReader reader) {
    return (
      speaker: reader.str(KzField.f30),
      text: reader.str(KzField.f31),
      awaiting: reader.u32(KzField.f32),
    );
  }

  static EngineState _parseState(KzReader reader) {
    return (
      bgm: reader.str(KzField.f35),
      pc: reader.u32(KzField.f36),
      scene: reader.u32(KzField.f37),
      animating: reader.boolean(KzField.f38),
      waitMs: reader.u32(KzField.f45),
      messageWindowVisible: reader.boolean(KzField.f46),
      engineMessageWindow: reader.boolean(KzField.f82),
      awaitingName: reader.boolean(KzField.f47),
      backlogActive: reader.boolean(KzField.f48),
      replayActive: reader.boolean(KzField.f49),
      instructionStart: reader.u32(KzField.f60),
      namePrompt: reader.str(KzField.f79),
      hostRequest: reader.str(KzField.f80),
      hostRequestSerial: reader.u32(KzField.f81),
      messageUnread: reader.boolean(KzField.f88),
      ceaseSkip: reader.boolean(KzField.f89),
      skipUnread: reader.boolean(KzField.f90),
      autoWait: reader.u32(KzField.f91),
    );
  }

  Future<OpenResult> open({
    required String dir,
    String brandId = '',
    List<String> extraDirs = const <String>[],
    String cacheDir = '',
  }) async {
    final writer = KzWriter();
    writer.str(KzField.f1, dir);
    writer.str(KzField.f44, brandId);
    writer.strList(KzField.f7, extraDirs);
    writer.str(KzField.f10, cacheDir);
    final reader = await _invoke(KzOp.open, writer.takeBytes());
    return (
      ok: reader.boolean(KzField.f1),
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
      title: reader.str(KzField.f4),
      version: reader.str(KzField.f5),
    );
  }

  Future<EngineInfo> info() async {
    final reader = await _invoke(KzOp.info, Uint8List(0));
    return (
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
      title: reader.str(KzField.f4),
      sceneCount: reader.u32(KzField.f6),
      entryCount: reader.u32(KzField.f7),
    );
  }

  Future<bool> start({int entryId = 0}) async {
    final writer = KzWriter();
    writer.u32(KzField.f8, entryId);
    final reader = await _invoke(KzOp.start, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<EngineStep> run({required int budget}) async {
    final writer = KzWriter();
    writer.u32(KzField.f9, budget);
    return _stepOf(await _invoke(KzOp.run, writer.takeBytes()));
  }

  Future<EngineStep> advance() async =>
      _stepOf(await _invoke(KzOp.advance, Uint8List(0)));

  Future<EngineStep> choose(int index) async {
    final writer = KzWriter();
    writer.u32(KzField.f11, index);
    return _stepOf(await _invoke(KzOp.choose, writer.takeBytes()));
  }

  Future<EngineStep> pointer(int sub, int x, int y) async {
    final writer = KzWriter();
    writer.u32(KzField.f12, sub);
    writer.i32(KzField.f13, x);
    writer.i32(KzField.f14, y);
    return _stepOf(await _invoke(KzOp.pointer, writer.takeBytes()));
  }

  // 主 worker 上的合成：compose worker 不可用时的回退路径（也保留了改动前的调用方式）。
  Future<bool> compose(Uint8List dst) => _composeOn(_main, dst);

  // compose 专用 isolate 上的合成：把 OpCompose 从逻辑链路上摘下来，点击与取帧
  // 现在和它真并发（Go 侧 OpCompose 走 composeMu，pointer/advance/run 走 mu）。
  // 只改"从哪条通道发 OpCompose"，请求/响应字节格式与字段号一字未动。
  Future<bool> composeIsolate(Uint8List dst) async {
    final channel = _compose;
    if (channel == null || channel.dead) {
      // 第二个 worker 起来失败、或它在运行中崩了：回退主 worker，画面不冻结，
      // 代价只是这一帧的合成重新和逻辑 RPC 串行。
      return compose(dst);
    }
    try {
      return await _composeOn(channel, dst);
    } catch (_) {
      // channel.dead：这次请求在飞时 compose worker 死了（onExit 已 reject 在飞请求），
      // 本帧降级到主 worker 重试一次；此后每帧都会走上面那条回退分支。
      if (!channel.dead || _closed) {
        rethrow; // 引擎真的回了错误（未 open 等）：语义与改动前一致，交给调用方
      }
      return compose(dst);
    }
  }

  // OpCompose 的实际执行体，两条通道（主 worker / compose worker）共用。
  Future<bool> _composeOn(_WorkerChannel channel, Uint8List dst) async {
    // Go 侧契约：dirty=false 时完全不写缓冲区。因此持久缓冲不需要清零/回拷，
    // 只在分辨率变化时重新分配，消除每帧 calloc/free 3.69MB 造成的 GC 抖动。
    if (_composeBuf == null || _composeBufSize != dst.length) {
      final old = _composeBuf;
      _composeBuf = null;
      _composeBufSize = 0;
      if (old != null) {
        calloc.free(old);
      }
      _composeBuf = calloc<Uint8>(dst.length);
      _composeBufSize = dst.length;
    }
    final buffer = _composeBuf!;
    final writer = KzWriter();
    writer.u64(KzField.f15, buffer.address);
    writer.u32(KzField.f16, dst.length);
    final flight = channel.invoke(KzOp.compose, writer.takeBytes());
    // 登记在飞请求：dispose 要等它结束才能安全释放缓冲（缓冲地址已经交给 Go 侧写入）。
    _composeFlight = flight;
    try {
      final reader = await flight;
      final dirty = reader.boolean(KzField.f17);
      if (dirty) {
        // 回包到达即代表 Go 侧已经写完这块缓冲（invoke 是同步 FFI），
        // 这里在主 isolate 上一次 memcpy 把它搬进调用方的 Uint8List。
        dst.setAll(0, buffer.asTypedList(dst.length));
      }
      return dirty;
    } finally {
      _composeFlight = null;
    }
  }

  Future<List<EngineScene>> scenes() async {
    final reader = await _invoke(KzOp.scenes, Uint8List(0));
    final out = <EngineScene>[];
    reader.each(KzField.f18, (sub) {
      out.add((
        id: sub.u32(KzField.f19),
        name: sub.str(KzField.f20),
        group: sub.u32(KzField.f65),
        index: sub.u32(KzField.f66),
        title: sub.str(KzField.f80),
        images: sub.strList(KzField.f81),
        // 旧库不返回 f87 时按"已解锁"处理，避免整列表变灰（见 boolean 的 fallback）。
        unlocked: sub.boolean(KzField.f87, fallback: true),
      ));
    });
    return out;
  }

  Future<List<EngineSlot>> slots() async {
    final reader = await _invoke(KzOp.slots, Uint8List(0));
    final out = <EngineSlot>[];
    reader.each(KzField.f21, (sub) {
      out.add((
        index: sub.u32(KzField.f11),
        title: sub.str(KzField.f22),
        savedAt: sub.str(KzField.f24),
        summary: sub.str(KzField.f63),
        exists: sub.boolean(KzField.f64),
      ));
    });
    return out;
  }

  Future<bool> save(int slot, String title) async {
    final writer = KzWriter();
    writer.u32(KzField.f11, slot);
    writer.str(KzField.f24, title);
    final reader = await _invoke(KzOp.save, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<EngineStep> load(int slot) async {
    final writer = KzWriter();
    writer.u32(KzField.f11, slot);
    return _stepOf(await _invoke(KzOp.load, writer.takeBytes()));
  }

  Future<int> assetSize(String path) async {
    final writer = KzWriter();
    writer.str(KzField.f1, path);
    final reader = await _invoke(KzOp.assetSize, writer.takeBytes());
    return reader.u32(KzField.f25);
  }

  Future<int> readAsset(String path, Uint8List dst) async {
    final buffer = calloc<Uint8>(dst.length);
    try {
      final writer = KzWriter();
      writer.str(KzField.f1, path);
      writer.u64(KzField.f15, buffer.address);
      writer.u32(KzField.f16, dst.length);
      final reader = await _invoke(KzOp.readAsset, writer.takeBytes());
      final written = reader.u32(KzField.f25);
      final count = written < dst.length ? written : dst.length;
      dst.setAll(0, buffer.asTypedList(count));
      return written;
    } finally {
      calloc.free(buffer);
    }
  }

  Future<List<AudioEvent>> audio() async =>
      _parseAudio(await _invoke(KzOp.audio, Uint8List(0)));

  Future<EngineText> text() async =>
      _parseText(await _invoke(KzOp.text, Uint8List(0)));

  Future<List<EngineChoice>> choices() async {
    final reader = await _invoke(KzOp.choices, Uint8List(0));
    final out = <EngineChoice>[];
    reader.each(KzField.f33, (sub) {
      out.add((name: sub.str(KzField.f20), value: sub.i32(KzField.f34)));
    });
    return out;
  }

  Future<EngineState> state() async =>
      _parseState(await _invoke(KzOp.state, Uint8List(0)));

  // 一次 RPC 取齐 step/text/state/audio：走点击路径的 _sync 原先要 4~6 次 isolate 往返，
  // 每次都排在同一把引擎锁后面，这是"点击后文字上屏慢"的直接来源。
  // 兼容：旧库不认识 op 37 会回 StatusBadRequest，捕获一次即置 _frameSupported=false，
  // 之后直接走旧三连 RPC（与改动前的行为逐字等价，代价只是首帧多一次失败往返）。
  // 坑：回退分支拿不到 step（Text/State/Audio 都不返回它），填 cont；
  // 调用方必须用自己那次 advance/pointer/run 的返回值，不要用这里的 step。
  Future<EngineFrameSnapshot> frame() async {
    if (_frameSupported) {
      try {
        final reader = await _invoke(KzOp.frame, Uint8List(0));
        return (
          step: _stepOf(reader),
          text: _parseText(reader),
          state: _parseState(reader),
          audio: _parseAudio(reader),
        );
      } on KzEngineException catch (error) {
        if (error.status != KzStatus.badRequest) {
          rethrow;
        }
        _frameSupported = false;
      }
    }
    return (
      step: EngineStep.cont,
      text: _parseText(await _invoke(KzOp.text, Uint8List(0))),
      state: _parseState(await _invoke(KzOp.state, Uint8List(0))),
      audio: _parseAudio(await _invoke(KzOp.audio, Uint8List(0))),
    );
  }

  Future<List<TextEntry>> textList() async {
    final reader = await _invoke(KzOp.textList, Uint8List(0));
    final out = <TextEntry>[];
    reader.each(KzField.f39, (sub) {
      out.add((
        id: sub.str(KzField.f19),
        text: sub.str(KzField.f40),
        file: sub.str(KzField.f41),
      ));
    });
    return out;
  }

  Future<String> lastError() async {
    final reader = await _invoke(KzOp.lastError, Uint8List(0));
    return reader.str(KzField.f42);
  }

  Future<List<BrandInfo>> brands() async {
    final reader = await _invoke(KzOp.brands, Uint8List(0));
    final out = <BrandInfo>[];
    reader.each(KzField.f18, (sub) {
      out.add((id: sub.str(KzField.f19), name: sub.str(KzField.f20)));
    });
    return out;
  }

  Future<DetectResult> detect(String dir) async {
    final writer = KzWriter();
    writer.str(KzField.f1, dir);
    final reader = await _invoke(KzOp.detect, writer.takeBytes());
    return (
      brandId: reader.str(KzField.f19),
      brandName: reader.str(KzField.f20),
    );
  }

  Future<List<ScanEntry>> scan(String root) async {
    final writer = KzWriter();
    writer.str(KzField.f1, root);
    final reader = await _invoke(KzOp.scan, writer.takeBytes());
    final out = <ScanEntry>[];
    reader.each(KzField.f71, (sub) {
      out.add((
        brandId: sub.str(KzField.f19),
        brandName: sub.str(KzField.f20),
        name: sub.str(KzField.f72),
        directory: sub.str(KzField.f73),
        dataDirectory: sub.str(KzField.f74),
        coverPath: sub.str(KzField.f75),
        extraDirs: sub.strList(KzField.f76),
        scriptPath: sub.str(KzField.f77),
        hasScript: sub.boolean(KzField.f78),
      ));
    });
    return out;
  }

  Future<bool> submitName(String value) async {
    final writer = KzWriter();
    writer.str(KzField.f1, value);
    final reader = await _invoke(KzOp.submitName, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<bool> audioDone({
    required String file,
    required bool voice,
    required int channel,
  }) async {
    final writer = KzWriter();
    writer.str(KzField.f1, file);
    writer.boolean(KzField.f2, voice);
    writer.u32(KzField.f3, channel);
    final reader = await _invoke(KzOp.audioDone, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<List<MapButton>> mapButtons() async {
    final reader = await _invoke(KzOp.mapButtons, Uint8List(0));
    final out = <MapButton>[];
    reader.each(KzField.f51, (sub) {
      out.add((
        index: sub.u32(KzField.f11),
        image: sub.str(KzField.f1),
        x: sub.i32(KzField.f13),
        y: sub.i32(KzField.f14),
        width: sub.u32(KzField.f2),
        height: sub.u32(KzField.f3),
        value: sub.i32(KzField.f34),
        label: sub.str(KzField.f20),
      ));
    });
    return out;
  }

  Future<MapMask> mapMask() async {
    final reader = await _invoke(KzOp.mapMask, Uint8List(0));
    return (
      has: reader.boolean(KzField.f1),
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
    );
  }

  Future<List<SelectItem>> selectItems() async {
    final reader = await _invoke(KzOp.selectItems, Uint8List(0));
    final out = <SelectItem>[];
    reader.each(KzField.f52, (sub) {
      out.add((
        index: sub.u32(KzField.f11),
        text: sub.str(KzField.f31),
        frame: sub.str(KzField.f1),
        x: sub.i32(KzField.f13),
        y: sub.i32(KzField.f14),
        width: sub.u32(KzField.f2),
        height: sub.u32(KzField.f3),
        textX: sub.i32(KzField.f15),
        textY: sub.i32(KzField.f16),
        alignment: sub.u32(KzField.f83),
        defaultColor: sub.u32(KzField.f84),
        selectColor: sub.u32(KzField.f85),
      ));
    });
    return out;
  }

  Future<EngineFrame> selectFrame(String frame) async {
    final writer = KzWriter();
    writer.str(KzField.f1, frame);
    final reader = await _invoke(KzOp.selectFrame, writer.takeBytes());
    return (
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
      pixels: reader.raw(KzField.f5),
    );
  }

  Future<EngineFrame> composeImages(List<String> names) async {
    final writer = KzWriter();
    writer.strList(KzField.f53, names);
    final reader = await _invoke(KzOp.composeImages, writer.takeBytes());
    return (
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
      pixels: reader.raw(KzField.f5),
    );
  }

  Future<bool> setPreview({
    required int width,
    required int height,
    required Uint8List pixels,
  }) async {
    final writer = KzWriter();
    writer.u32(KzField.f2, width);
    writer.u32(KzField.f3, height);
    writer.raw(KzField.f5, pixels);
    final reader = await _invoke(KzOp.setPreview, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<SlotPreview> slotPreview(int slot) async {
    final writer = KzWriter();
    writer.u32(KzField.f11, slot);
    final reader = await _invoke(KzOp.slotPreview, writer.takeBytes());
    return (
      savedAt: reader.str(KzField.f24),
      summary: reader.str(KzField.f63),
      speaker: reader.str(KzField.f30),
      text: reader.str(KzField.f31),
      width: reader.u32(KzField.f2),
      height: reader.u32(KzField.f3),
      pixels: reader.raw(KzField.f5),
    );
  }

  Future<List<CgGroup>> cgCatalog() async {
    final reader = await _invoke(KzOp.cgCatalog, Uint8List(0));
    final out = <CgGroup>[];
    reader.each(KzField.f86, (group) {
      final variants = <CgVariant>[];
      group.each(KzField.f54, (variant) {
        variants.add((
          images: variant.strList(KzField.f56),
          // 旧库不返回 f87 时 reader 给默认 false；为了不让旧库下鉴赏全灰，
          // 这里把"字段缺失"当成已解锁（见 KzReader.boolean 的默认值语义）。
          unlocked: variant.boolean(KzField.f87, fallback: true),
        ));
      });
      out.add((name: group.str(KzField.f20), variants: variants));
    });
    return out;
  }

  /// 取主界面菜单项；空列表表示引擎当前不在主界面。
  Future<List<TitleMenuItem>> titleMenu() async {
    final reader = await _invoke(KzOp.titleMenu, Uint8List(0));
    final out = <TitleMenuItem>[];
    reader.each(KzField.f60, (item) {
      out.add((
        key: item.str(KzField.f61),
        label: item.str(KzField.f62),
        x: item.i32(KzField.f63),
        y: item.i32(KzField.f64),
        width: item.u32(KzField.f65),
        height: item.u32(KzField.f66),
        enabled: item.boolean(KzField.f67),
      ));
    });
    return out;
  }

  /// 回传主界面点击；key 来自 [TitleMenuItem.key]。
  Future<EngineStep> selectTitleItem(String key) async {
    final writer = KzWriter();
    writer.str(KzField.f61, key);
    return _stepOf(await _invoke(KzOp.titleSelect, writer.takeBytes()));
  }

  /// 取当前待播影片；path 为空表示没有影片。
  Future<MovieRequest> movie() async {
    final reader = await _invoke(KzOp.movie, Uint8List(0));
    return (path: reader.str(KzField.f68), serial: reader.u32(KzField.f69));
  }

  /// 影片播完（或被跳过）后通知引擎继续脚本。
  Future<EngineStep> movieDone() async =>
      _stepOf(await _invoke(KzOp.movieDone, Uint8List(0)));

  Future<bool> setOption(int key, bool value) async {
    final writer = KzWriter();
    writer.u32(KzField.f57, key);
    writer.boolean(KzField.f7, value);
    final reader = await _invoke(KzOp.setOption, writer.takeBytes());
    return reader.boolean(KzField.f1);
  }

  Future<EngineStep> key(int code) async {
    final writer = KzWriter();
    writer.u32(KzField.f58, code);
    return _stepOf(await _invoke(KzOp.key, writer.takeBytes()));
  }

  Future<EngineStep> wheel(int delta) async {
    final writer = KzWriter();
    writer.i32(KzField.f59, delta);
    return _stepOf(await _invoke(KzOp.wheel, writer.takeBytes()));
  }
}
