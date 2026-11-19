import 'dart:async';
import 'dart:typed_data';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/bridge/engine_bridge.dart' hide TextRun;
import '../../core/bridge/protocol_consts.dart';
import 'frame_channel.dart';
import 'frame_painter.dart' show clearTextLayoutCache;

// 协议层的 TextRun 是 record（core/bridge 不依赖 Flutter），FrameBus 收的是
// frame_channel 的不可变类；两个名字撞车，所以 import 时 hide 掉 bridge 的那个，
// 在发布边界做一次转换。align 不往下传：引擎已把 x 算成行的实际起点。
List<TextRun> _toFrameRuns(TextRunSnapshot runs) => <TextRun>[
      for (final run in runs)
        TextRun(
          text: run.text,
          x: run.x,
          y: run.y,
          size: run.size,
          color: run.color,
          outline: run.outline,
          shadow: run.shadow,
          width: run.width,
          bold: run.bold,
        ),
    ];

abstract class GameAudio {
  Future<void> dispatchEvent(AudioEvent event);
  Future<void> dispose();
}

class StubGameAudio implements GameAudio {
  @override
  Future<void> dispatchEvent(AudioEvent event) async {}

  @override
  Future<void> dispose() async {}
}

class PlayerState {
  final String speaker;
  final String text;
  final List<EngineChoice> choices;
  final List<SelectItem> selectItems;
  final Map<String, EngineFrame> selectFrames;
  final List<MapButton> mapButtons;
  final MapMask mapMask;
  final int width;
  final int height;
  final bool messageWindowVisible;
  /// 引擎已经把原版对话框画进帧里；宿主不再叠 Flutter 的 MessageBox。
  final bool engineMessageWindow;
  final bool awaitingName;
  /// awaitingName 是原版 GetYesNoMessage 时，宿主显示"是/否"而不是文本框。
  final bool promptIsYesNo;
  /// 引擎已经把 prompt 对话框画进帧；宿主不再叠 NameDialog。
  final bool enginePrompt;
  final String namePrompt;
  final bool replayActive;
  final bool backlogActive;
  final bool isAnimating;
  /// 主界面菜单项；非空表示引擎停在标题菜单，此时不显示对话框、不响应"推进对白"。
  final List<TitleMenuItem> titleMenu;
  /// 正在播放的影片路径；非空时铺一层全屏播放器，脚本暂停。
  final String moviePath;
  /// 与 [moviePath] 配套的引擎序号：MovieLayer 回调 movieDone 时带上它，
  /// 上一次影片的迟到回调就不会把新影片（或已经结束的影片）当成播完。
  final int movieSerial;
  /// 影片音轨增益（0..1），来自引擎的 audioVolume（KAG 的 `@movie volume=`）。
  /// 默认 1.0：原引擎的播放器初值就是满音量，缺字段不能读成静音。
  final double movieAudioGain;
  /// 引擎请求宿主执行的界面动作（settings / exit）；serial 用来判重。
  final String hostRequest;
  final int hostRequestSerial;  final bool auto;
  final bool skip;
  /// オート再生速度（引擎设置 0..100，越大越快）。宿主原来的固定
  /// 文本长度公式不读这个值，改设置不会生效，所以随状态一起同步。
  final int autoWait;
  final bool ready;
  final bool finished;
  final bool faulted;
  /// faulted/finished 时引擎侧最近一次 [EngineBridge.lastError] 快照（多行）。
  /// faulted 只是一个 step，引擎的 trace（例如 emecfg 缺失）只在 lastError 里；
  /// 不拉它，宿主就只能显示"出错了"，玩家无从判断是缺配置还是脚本坏了。
  final String faultReason;
  final String? error;

  const PlayerState({
    this.speaker = '',
    this.text = '',
    this.choices = const <EngineChoice>[],
    this.selectItems = const <SelectItem>[],
    this.selectFrames = const <String, EngineFrame>{},
    this.mapButtons = const <MapButton>[],
    this.mapMask = const (has: false, width: 0, height: 0),
    this.width = 0,
    this.height = 0,
    this.messageWindowVisible = true,
    this.engineMessageWindow = false,
    this.awaitingName = false,
    this.promptIsYesNo = false,
    this.enginePrompt = false,
    this.namePrompt = '',
    this.replayActive = false,
    this.backlogActive = false,
    this.isAnimating = false,
    this.titleMenu = const <TitleMenuItem>[],
    this.moviePath = '',
    this.movieSerial = 0,
    this.movieAudioGain = 1.0,
    this.hostRequest = '',
    this.hostRequestSerial = 0,
    this.auto = false,
    this.skip = false,
    this.autoWait = 50,
    this.ready = false,
    this.finished = false,
    this.faulted = false,
    this.faultReason = '',
    this.error,
  });

  PlayerState copyWith({
    String? speaker,
    String? text,
    List<EngineChoice>? choices,
    List<SelectItem>? selectItems,
    Map<String, EngineFrame>? selectFrames,
    List<MapButton>? mapButtons,
    MapMask? mapMask,
    int? width,
    int? height,
    bool? messageWindowVisible,
    bool? engineMessageWindow,
    bool? awaitingName,
    bool? promptIsYesNo,
    bool? enginePrompt,
    String? namePrompt,
    bool? replayActive,
    bool? backlogActive,
    bool? isAnimating,
    List<TitleMenuItem>? titleMenu,
    String? moviePath,
    int? movieSerial,
    double? movieAudioGain,
    String? hostRequest,
    int? hostRequestSerial,
    bool? auto,
    bool? skip,
    int? autoWait,
    bool? ready,
    bool? finished,
    bool? faulted,
    String? faultReason,
    String? error,
    bool clearError = false,
  }) {
    return PlayerState(
      speaker: speaker ?? this.speaker,
      text: text ?? this.text,
      choices: choices ?? this.choices,
      selectItems: selectItems ?? this.selectItems,
      selectFrames: selectFrames ?? this.selectFrames,
      mapButtons: mapButtons ?? this.mapButtons,
      mapMask: mapMask ?? this.mapMask,
      width: width ?? this.width,
      height: height ?? this.height,
      messageWindowVisible:
          messageWindowVisible ?? this.messageWindowVisible,
      engineMessageWindow:
          engineMessageWindow ?? this.engineMessageWindow,
      awaitingName: awaitingName ?? this.awaitingName,
      promptIsYesNo: promptIsYesNo ?? this.promptIsYesNo,
      enginePrompt: enginePrompt ?? this.enginePrompt,
      namePrompt: namePrompt ?? this.namePrompt,
      replayActive: replayActive ?? this.replayActive,
      backlogActive: backlogActive ?? this.backlogActive,
      isAnimating: isAnimating ?? this.isAnimating,
      titleMenu: titleMenu ?? this.titleMenu,
      moviePath: moviePath ?? this.moviePath,
      movieSerial: movieSerial ?? this.movieSerial,
      movieAudioGain: movieAudioGain ?? this.movieAudioGain,
      hostRequest: hostRequest ?? this.hostRequest,
      hostRequestSerial: hostRequestSerial ?? this.hostRequestSerial,
      auto: auto ?? this.auto,
      skip: skip ?? this.skip,
      autoWait: autoWait ?? this.autoWait,
      ready: ready ?? this.ready,
      finished: finished ?? this.finished,
      faulted: faulted ?? this.faulted,
      faultReason: faultReason ?? this.faultReason,
      error: clearError ? null : (error ?? this.error),
    );
  }
}

class PlayerSessionNotifier extends Notifier<EngineBridge?> {
  @override
  EngineBridge? build() => null;

  void set(EngineBridge? bridge) {
    state = bridge;
  }
}

final playerSessionProvider =
    NotifierProvider<PlayerSessionNotifier, EngineBridge?>(
        PlayerSessionNotifier.new);

class PlayerNotifier extends Notifier<PlayerState> {
  // 用户点击/按键路径不自旋：Go 侧的 advanceTap/runClick 已经在一次调用里跨过 story
  // 任务的 WAIT 链，剩下的 cont 链由 16ms tick 的 _pump 接续（_lastStep==cont 就是它的
  // 触发条件）。原先每次 press/release 各自旋最多 64 次 run（约 120ms 墙钟），一次点击
  // 两轮，连点时前一轮的自旋把后一轮点击挡在队列里，累积成 1–2 秒延迟。
  static const int _inputSpins = 0;
  // 自动 pump / 文本等待链仍按原预算跑完 cont，保持脚本自身 WAIT 节奏不变。
  static const int _pumpSpins = 64;

  EngineBridge? _bridge;
  GameAudio _audio = StubGameAudio();
  bool _dead = false;
  bool _composing = false;
  bool _pumping = false;
  bool _resolving = false;
  bool _busy = false;
  // 用户输入路径（_action→tap/press/release→_resolve）里 _pumping 恒为 false，
  // 旧自旋守卫靠 _pumping && _busy 判断"用户是否在排队"永远不成立；
  // 这里显式计数排队中的用户 action，供 _resolve 自旋让出。计数由 _finishInput 在
  // action 结束时归还，所以正在跑的那次也能看到排在它后面的点击。
  int _pendingInput = 0;
  bool _pressSent = false;
  // press 发出的时刻：release 事件可能被系统吞掉（窗口失焦、触摸被系统手势抢走），
  // _pressSent 残留为真会让 _onTick 的 due 永久为 false（定时推进停摆），用它做 800ms 兜底。
  int _pressAt = 0;
  bool _needsCompose = true;
  bool _waitPending = false;
  bool _frameWanted = false;
  bool _hoverQueued = false;
  bool _dragQueued = false;
  // 回想界面由宿主打开（引擎只发 hostRequest），这个标记要和引擎
  // State.backlogActive 取并集，否则下一次 _sync 会把它覆盖掉。
  bool _localBacklog = false;
  int _waitMs = 0;
  int _waitDueMs = 0;
  EngineStep _lastStep = EngineStep.wait;
  int _hoverX = -1;
  int _hoverY = -1;
  int _pointerX = -1;
  int _pointerY = -1;
  int _pendingHoverX = -1;
  int _pendingHoverY = -1;
  int _pendingDragX = -1;
  int _pendingDragY = -1;
  Timer? _ticker;
  Uint8List _pixels = Uint8List(0);
  /// 合成写入目标的轮转池。见 [_nextPixelBuffer]：1080p 一帧 8.3MB，按帧新分配
  /// 会制造 ~500MB/s 的垃圾，GC 直接把渲染线程拖出掉帧。
  final List<Uint8List> _pixelPool = <Uint8List>[];
  int _pixelPoolCursor = 0;
  /// 最近一次发布给 UI 的位图与配对的文字层：dirty=false 但文字变了时要复用它
  /// 再发一帧（FrameView 看到 identical(bytes) 只换文字，不重新解码图像）。
  Uint8List? _publishedPixels;
  List<TextRun> _publishedRuns = const <TextRun>[];
  /// 帧位图通道：合成好的位图从这里单独流向 FrameView，不挂在 PlayerState 上，
  /// 免得每合成一帧都让播放页所有 playerProvider 订阅者重算一遍选择器。
  final FrameBus frameBus = FrameBus();
  Future<void> _tail = Future<void>.value();
  // 音频与引擎逻辑队列分开：voice 走 SoLoud.loadFile 可能耗时数秒甚至失败，
  // 和 _tail 共用会让点击后的文本/状态更新排在音频加载后面。
  Future<void> _audioTail = Future<void>.value();
  // 逻辑 pump 仍按 16ms，但画面合成单独限流到约 30fps。
  int _lastComposeAt = 0;
  // 语音/BGM 物化已挪到 Go 后台，播放事件会晚于触发出现；引擎停在 awaitInput 空闲时
  // 不走 _sync（没有 run/pointer），晚到的事件就得等下次点击才播。
  // 故由 ticker 周期兜底轮询（约 8 次/秒的空 RPC，开销可忽略），_audioDraining 防重入。
  int _audioPollAt = 0;
  bool _audioDraining = false;
  // 最近一次 _sync 的时刻：_sync 已经 drain 过 audio，_drainAudio 的周期轮询要避开这个窗口，
  // 否则点击瞬间会往引擎队列里多塞一个空 RPC，排在真正的点击请求前面。
  int _syncAt = 0;
  // faulted/finished 已经拉过 lastError 的时刻：拉取本身是一次 RPC，终端 step 之后
  // 按钮/点击仍会调 _resolve→_sync，不加节流每次点击都会多打一次 lastError。
  int _faultFetchAt = 0;

  @override
  PlayerState build() {
    ref.onDispose(() {
      _dead = true;
      _ticker?.cancel();
      _ticker = null;
      // FrameView 监听的是这个 ValueNotifier，provider 销毁时必须一起释放，
      // 否则每开一次播放页就漏一个（连同它上面挂着的监听者）。
      frameBus.dispose();
      // 排版缓存持有 TextPainter（原生段落对象）：播放页退出时一并释放，
      // 避免每进出一部作品都多留一份已排版的日文段落。
      clearTextLayoutCache();
      final bridge = _bridge;
      _bridge = null;
      ref.read(playerSessionProvider.notifier).set(null);
      _audio.dispose();
      if (bridge != null) {
        bridge.dispose();
      }
    });
    return const PlayerState();
  }

  void attachAudio(GameAudio audio) {
    _audio = audio;
  }

  /// 取一块"可以给合成写"的整帧缓冲（三块轮转）。
  ///
  /// 为什么是三块：一块正在写（下一次 compose 的目标）、一块刚发布出去（UI 正
  /// 在把它解码成 ui.Image）、一块是上一帧（已经解完，随时可复用）。两块不够——
  /// 发布与解码之间有一个 await，两块会在"UI 还在读"时被写坏。
  ///
  /// 刚发布出去的那块（[_publishedPixels]）显式跳过：它可能还在 UI 的解码队列里
  /// （FrameBus.decodeBusy 只挡"正在解码"，挡不住"即将解码"），三块轮转正好让
  /// 它转两圈才回到写入位置。
  ///
  /// 唯一的例外是解码卡死 300ms 后的兜底路径（FrameBus 主动放弃背压）：那时 UI 已经
  /// 明显停止消费，画面优先，允许覆盖正在被解的那块——本来那张图也会被丢弃。
  ///
  /// 复用时不 memset：合成只在这块缓冲被整帧写满（dirty=true）时才发布，而
  /// dirty=false 时引擎一个字节都不写，用到的仍是它自己上一次写进去的内容。
  /// 尺寸变化（480x280 启动窗口 -> 1920x1080 正片）会重建整个池。
  Uint8List _nextPixelBuffer(int need) {
    if (_pixelPool.length != 3 || _pixelPool.first.length != need) {
      _pixelPool
        ..clear()
        ..addAll(<Uint8List>[
          Uint8List(need),
          Uint8List(need),
          Uint8List(need),
        ]);
      _pixelPoolCursor = 0;
    }
    // 最多三次一定能找到一块不是"刚发布出去"的缓冲（池里有三块）。
    for (var attempt = 0; attempt < 3; attempt++) {
      final buffer = _pixelPool[_pixelPoolCursor];
      _pixelPoolCursor = (_pixelPoolCursor + 1) % _pixelPool.length;
      if (!identical(buffer, _publishedPixels)) {
        return buffer;
      }
    }
    // 走到这里说明调用方连续两次要缓冲而中间没有发布过（例如刚 open 完）；
    // 此时上一块"已发布"的其实就是当前写入目标，交回它是安全的。
    final buffer = _pixelPool[_pixelPoolCursor];
    _pixelPoolCursor = (_pixelPoolCursor + 1) % _pixelPool.length;
    return buffer;
  }

  Future<void> _enqueue(Future<void> Function() job) {
    final next = _tail.then((_) {
      if (_dead) {
        return Future<void>.value();
      }
      return job();
    });
    _tail = next.catchError((_) {});
    return next;
  }

  // 音频事件（尤其 voice 的 SoLoud.loadFile）可能耗时数秒；放在后台串行队列，
  // 文本/状态先更新，避免点击被音频加载拖住；串行保证 stop/play 顺序。
  void _queueAudio(List<AudioEvent> events) {
    if (events.isEmpty || _dead) {
      return;
    }
    _audioTail = _audioTail.then((_) async {
      if (_dead) {
        return;
      }
      for (final event in events) {
        if (_dead) {
          return;
        }
        try {
          await _audio.dispatchEvent(event);
        } catch (_) {}
      }
    });
  }

  Future<void> _action(Future<void> Function(EngineBridge bridge) job) {
    _waitPending = false;
    _waitDueMs = 0;
    // 已有 action 在执行，说明本次要排队：记一笔，让正在自旋的 _resolve 立刻让出，
    // 否则排队中的点击要等前一次自旋跑完才被处理。
    final queued = _busy;
    if (queued) {
      _pendingInput++;
    }
    _busy = true;
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        _finishInput(queued);
        return;
      }
      try {
        await job(bridge);
      } catch (error) {
        // 用户退出播放页时，在飞的 RPC 会被 bridge 在 dispose 里 reject（不再永久挂住），
        // 这时 notifier 已经销毁，写 state 会抛"已销毁"的 StateError：判 _dead 跳过。
        if (!_dead) {
          state = state.copyWith(error: '$error');
        }
      } finally {
        _finishInput(queued);
      }
    });
  }

  // 计数在本次 action 结束时才归还：原实现在 action *开始* 时递减，减掉的是排在它
  // 后面的那次点击，于是 press 自旋看到的 _pendingInput 恒为 0、一路跑满 64 次 run，
  // 真正排队的 release 反而没有让出。_busy 也要保持到队列清空，否则 press 跑完、
  // release 还没开始时 _onTick 会插进一次自动 pump 的 advance 抢走这次点击。
  void _finishInput(bool queued) {
    if (queued && _pendingInput > 0) {
      _pendingInput--;
    }
    _busy = _pendingInput > 0;
  }

  /// [cacheDir] 传的是 Future：缓存目录准备（平台 channel 查询 + mkdir）与引擎 isolate 启动
  /// 互不依赖，调用方先发起、这里在 open 之前才收口，两边重叠执行；空值表示不用缓存目录。
  Future<void> start(
    String dir, {
    String brandId = '',
    List<String> extraDirs = const <String>[],
    Future<String>? cacheDir,
  }) {
    return _enqueue(() async {
      state = state.copyWith(clearError: true);
      try {
        // 内核按作品选：krkr 系走 C++ 内核（enginec），其余仍用 Go 引擎（enginego）。
        // 两份库说同一套协议，只有入口符号前缀不同（KzcEngine*/KzEngine*，见
        // core/bridge/native.dart），宿主其余路径完全一致。
        final nativeLibrary =
            brandId.startsWith('krkr') ? 'enginec' : 'enginego';
        // isolate 启动（加载 native 库 + 建引擎实例，几十 ms）与缓存目录准备并行：
        // 两个 Future 都先发出去再分别 await，open 需要 cacheDir，所以它最后收口。
        final bridgeFuture = EngineBridge.spawn(nativeLibrary: nativeLibrary);
        final cacheDirFuture = cacheDir ?? Future<String>.value('');
        // 顺带挂一个空的错误处理：spawn 先失败时不会再 await 它，
        // 免得那个未来没被 await 的异常冒到 zone 里（真正 await 时照样能拿到同一个错误）。
        unawaited(cacheDirFuture.then<void>((_) {}, onError: (Object error) {}));
        final bridge = await bridgeFuture;
        if (_dead) {
          await bridge.dispose();
          return;
        }
        _bridge = bridge;
        ref.read(playerSessionProvider.notifier).set(bridge);
        final opened = await bridge.open(
            dir: dir,
            brandId: brandId,
            extraDirs: extraDirs,
            cacheDir: await cacheDirFuture);
        if (!opened.ok) {
          throw KzEngineException(await bridge.lastError());
        }
        state = state.copyWith(width: opened.width, height: opened.height);
        _pixels = _nextPixelBuffer(opened.width * opened.height * 4);
        await bridge.start();
        // 存档槽位由引擎的 [data screen] 负责，宿主不需要在启动时读 280 个存档文件：
        // 只把 ready 置位，等引擎停在 AwaitTitle/AwaitInput 后按帧同步。
        state = state.copyWith(ready: true);
        _ticker?.cancel();
        // 16ms 是游戏逻辑帧节拍（WAIT FRAME=1 的恢复节奏），不是渲染帧率：
        // 33ms 会把引擎侧 10–50ms 的 pointer 响应推到下一个 tick，把 60fps 拖成 15fps。
        _ticker = Timer.periodic(const Duration(milliseconds: 16), (_) {
          _onTick();
        });
        await _resolve(await bridge.run(budget: 4096));
      } catch (error) {
        if (!_dead) {
          state = state.copyWith(error: '$error');
        }
      }
    });
  }

  /// 点击主界面菜单项。
  ///
  /// 单独一条路径而不是复用 [tap]：主界面的点击语义是"选中某个菜单项"，
  /// 由引擎按 key 分派（开始剧情 / 播 OP / 交给宿主开界面），
  /// 和"点画面推进对白"完全不同，混在 pointer 里会让坐标命中逻辑两头不讨好。
  Future<void> selectTitleItem(String key) {
    return _action((bridge) async {
      final step = await bridge.selectTitleItem(key);
      // 读档/鉴赏/设置/退出这几项引擎会停在标题，具体界面由 TitleLayer 分派；
      // 这里只把 step 同步回来（start 会返回剧情里的新 step）。
      await _resolve(step, compose: false);
    });
  }

  /// 影片播完后通知引擎继续脚本。
  ///
  /// 清掉 moviePath 是让覆盖层立刻消失：引擎侧要等这次调用返回才会推进，
  /// 如果等返回值再清，玩家会看到影片结束后黑着停一帧。
  ///
  /// [serial] 是 MovieLayer 播放的那一部影片的序号。给了就只在仍是当前影片时
  /// 生效：上一次影片的 dispose/迟到回调（换片、页面切换、解码线程收尾）会把
  /// 新一部影片提前判完，脚本于是跳过一整段剧情。省略时按"无条件完成当前影片"
  /// 处理，供宿主自己的跳过按钮/退出路径使用。
  Future<void> finishMovie({int? serial}) {
    // provider 可能已经销毁（玩家退出播放页）：此后读写 state 会抛，而
    // MovieLayer 的 dispose 仍会回调进来。
    if (_dead) {
      return Future<void>.value();
    }
    final path = state.moviePath;
    if (path.isEmpty) {
      // 没有待播影片：仍然发一次 movieDone 是危险的（会让引擎推进未等待的脚本），
      // 直接返回；MovieLayer 的幂等标记保证它不会重复调到这里。
      return Future<void>.value();
    }
    if (serial != null && serial != state.movieSerial) {
      return Future<void>.value();
    }
    state = state.copyWith(
      moviePath: '',
      movieSerial: 0,
      movieAudioGain: 1.0,
    );
    return _action((bridge) async {
      final step = await bridge.movieDone();
      await _resolve(step, compose: false);
    });
  }

  Future<void> tap() {
    // 手册：AUTO 中点击画面即停止自动播放。只有用户真实点击走这里；
    // ticker 的自动推进走 _pump，不会误关。
    if (state.auto) {
      setAuto(false);
    }
    final x = _pointerX >= 0 ? _pointerX : _centerX();
    final y = _pointerY >= 0 ? _pointerY : _centerY();
    _pointerX = x;
    _pointerY = y;
    // 按住标记继续用来挡 ticker 的定时 advance 与合成，只是存活期缩短为一次 RPC 的往返：
    // press 与 release 现在由引擎在一次 pointer 调用里原子完成。
    _pressSent = true;
    _pressAt = DateTime.now().millisecondsSinceEpoch;
    return _action((bridge) async {
      // Go 侧 op=3 把 press+release 原子执行，返回值就是 release 之后的 step，
      // 因此一次 pointer RPC + 一次 _sync 即够。原先 press/release 各走一次 _resolve，
      // 中间那次 _sync 的 frame RPC 纯属浪费（点击 4 个串行往返里有 2 个是它）。
      EngineStep step;
      try {
        step = await bridge.pointer(KzPointer.tap, x, y);
        _pressSent = false;
      } on KzEngineException {
        // 旧库不认识 sub=3（回错误状态）：退回 press→release 两连发，中间照样不发 frame，
        // 标记在 release 返回后才清，与旧的时序一致。这里只兜这一种情况：其他异常
        // （连接已关、报文错）重试也无意义，交给 _action 统一报错。
        await bridge.pointer(KzPointer.press, x, y);
        step = await bridge.pointer(KzPointer.release, x, y);
        _pressSent = false;
      }
      // 用户输入的 pointer action 不等全帧合成：只推进逻辑，渲染交回 ticker 调度。
      await _resolve(step, compose: false, maxSpins: _inputSpins);
    });
  }

  /// 带坐标的点击：舞台层在 onTapUp 把画面坐标换算成引擎坐标后走这里。
  /// 引擎 op=3 在一次调用里原子完成 press+release，按下时不再单独发包，
  /// 所以一次抬起只对应一次 pointer 调用。
  Future<void> tapAt(int x, int y) {
    _pointerX = x;
    _pointerY = y;
    return tap();
  }

  Future<void> press(int x, int y) {
    _pointerX = x;
    _pointerY = y;
    _pressSent = true;
    _pressAt = DateTime.now().millisecondsSinceEpoch;
    return _action((bridge) async {
      await _resolve(await bridge.pointer(KzPointer.press, x, y),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> release() {
    if (!_pressSent) {
      return Future<void>.value();
    }
    _pressSent = false;
    final x = _pointerX;
    final y = _pointerY;
    return _action((bridge) async {
      await _resolve(await bridge.pointer(KzPointer.release, x, y),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> hover(int x, int y) {
    _pointerX = x;
    _pointerY = y;
    _hoverX = x;
    _hoverY = y;
    _pendingHoverX = x;
    _pendingHoverY = y;
    // 连点期间（已排了 >=2 个点击）丢弃 hover/move：它们只是光标跟随，每个都要占一次引擎
    // 队列位次，会把真正的点击挤到后面。点击不丢，只丢这一批光标更新；
    // 位置上面已经记下，队列清空后的下一次 hover 会用最新值补上。
    if (_hoverQueued || _pendingInput >= 2) {
      return Future<void>.value();
    }
    _hoverQueued = true;
    return _enqueue(() async {
      _hoverQueued = false;
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      if (_dead) {
        return;
      }
      final px = _pendingHoverX;
      final py = _pendingHoverY;
      if (px < 0 || py < 0) {
        return;
      }
      try {
        await bridge.pointer(KzPointer.move, px, py);
        _frameWanted = true;
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  /// 触屏滚动：触屏没有滚轮，舞台层把纵向拖动转成 pointer 子协议 6。
  /// 引擎只把它当滚动/拖滚动条（当前打开的系统界面），不会推进对白。
  ///
  /// 一次拖动会连续触发几十次 update，和 hover 一样合并：队列里已有一发就只更新
  /// 最新坐标，等它在引擎侧跑完再带走最新值。否则每个像素排一次 isolate RPC，
  /// 滚动会被人为排队拖慢。不调 _resolve：引擎返回当前 step，合成交给 ticker。
  Future<void> drag(int x, int y) {
    _pointerX = x;
    _pointerY = y;
    _hoverX = x;
    _hoverY = y;
    _pendingDragX = x;
    _pendingDragY = y;
    if (_dragQueued) {
      return Future<void>.value();
    }
    _dragQueued = true;
    return _enqueue(() async {
      _dragQueued = false;
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      if (_dead) {
        return;
      }
      final px = _pendingDragX;
      final py = _pendingDragY;
      if (px < 0 || py < 0) {
        return;
      }
      try {
        await bridge.pointer(KzPointer.drag, px, py);
        _frameWanted = true;
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  Future<void> rightClickAt(int x, int y) {
    _pointerX = x;
    _pointerY = y;
    _hoverX = x;
    _hoverY = y;
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await _resolve(await bridge.pointer(KzPointer.rightClick, x, y),
            compose: false, maxSpins: _inputSpins);
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  Future<void> advance() {
    return _action((bridge) async {
      // 回车/空格的推进和点击同级：同样不等全帧合成，渲染交给 ticker。
      await _resolve(await bridge.advance(),
          compose: false, maxSpins: _inputSpins);
    });
  }

  /// 把桌面键盘快捷键/工具栏菜单按钮转给引擎。
  ///
  /// 游戏内菜单条、存读档画面都做成引擎模态界面后，宿主只需要送一个 ESC：
  /// 引擎自己决定是关界面还是展开菜单，避免宿主复制一套界面状态。
  Future<void> sendKey(int code) {
    return _action((bridge) async {
      await _resolve(await bridge.key(code),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> choose(int index) {
    return _action((bridge) async {
      // 选择肢也是点击：同一套节奏，合成交给 ticker。
      await _resolve(await bridge.choose(index),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> chooseMapPixel(int x, int y) {
    return _action((bridge) async {
      // 这里必须用 press，不能用 tap：op=3 在新 Go 语义下已经自带 release
      // （session.go Pointer 的 case 3 = press+release 原子执行，返回 release 后的 step），
      // 后面还有一次显式 release；用 tap 会把这次点击消费两次（双推进）。
      await _resolve(await bridge.pointer(KzPointer.press, x, y),
          compose: false, maxSpins: _inputSpins);
      await _resolve(await bridge.pointer(KzPointer.release, x, y),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> chooseButton(int index) {
    return _action((bridge) async {
      MapButton? target;
      for (final button in state.mapButtons) {
        if (button.index == index) {
          target = button;
        }
      }
      if (target == null) {
        return;
      }
      final button = target;
      final x = button.x + button.width ~/ 2;
      final y = button.y + button.height ~/ 2;
      // 同 chooseMapPixel：op=3 自带 release，这里后面还要补一次 release，
      // 所以本步只能发 press（op=1），否则这次点击被消费两次（双推进）。
      await _resolve(await bridge.pointer(KzPointer.press, x, y),
          compose: false, maxSpins: _inputSpins);
      await _resolve(await bridge.pointer(KzPointer.release, x, y),
          compose: false, maxSpins: _inputSpins);
    });
  }

  Future<void> setHover(int x, int y) async {
    final bridge = _bridge;
    if (bridge == null || _dead) {
      return;
    }
    _hoverX = x;
    _hoverY = y;
    try {
      await bridge.pointer(KzPointer.move, x, y);
      _frameWanted = true;
    } catch (_) {}
  }

  Future<void> clearHover() async {
    final bridge = _bridge;
    if (bridge == null || _dead) {
      return;
    }
    _hoverX = -1;
    _hoverY = -1;
    _frameWanted = true;
    final x = state.width;
    final y = state.height;
    if (x <= 0 || y <= 0) {
      return;
    }
    try {
      await bridge.pointer(KzPointer.move, x, y);
    } catch (_) {}
  }

  Future<void> rightClick() {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await _resolve(
            await bridge.pointer(KzPointer.rightClick, _hoverX, _hoverY),
            compose: false,
            maxSpins: _inputSpins);
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  /// 滚轮。
  ///
  /// [x]/[y] 是滚轮发生时的指针位置（引擎坐标）。滚轮事件本身不带坐标，
  /// 引擎靠"最后一次指针位置"判断要滚哪一栏（例如 BGV 鉴赏室的 5 栏），
  /// 而 hover 事件可能被节流丢弃、方向键滚动又没有坐标，所以带坐标时
  /// 先补发一次 move，再发滚轮，引擎才知道滚的是哪一栏。
  Future<void> wheel(int delta, {int x = -1, int y = -1}) {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        if (x >= 0 && y >= 0) {
          _pointerX = x;
          _pointerY = y;
          _hoverX = x;
          _hoverY = y;
          await bridge.pointer(KzPointer.move, x, y);
        }
        await _resolve(await bridge.wheel(delta >= 0 ? 1 : -1),
            compose: false, maxSpins: _inputSpins);
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  Future<void> key(int code) {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await _resolve(await bridge.key(code),
            compose: false, maxSpins: _inputSpins);
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  /// 把透明 IME 层的当前文字交给引擎，由引擎画进 prompt 输入框。
  /// 不推进脚本；确认/取消仍由引擎自绘按钮的 Pointer 命中完成。
  Future<void> setPromptText(String text) {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await bridge.setPromptText(text);
      } catch (error) {
        if (!_dead) {
          state = state.copyWith(error: '$error');
        }
      }
    });
  }

  Future<void> submitYesNo(bool yes) {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await bridge.submitYesNo(yes);
        await _resolve(await bridge.run(budget: 4096));
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  Future<void> submitName(String value) {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await bridge.submitName(value);
        await _resolve(await bridge.run(budget: 4096));
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  void setAuto(bool value) {
    state = state.copyWith(auto: value, clearError: true);
    // 把 AUTO 状态同步给引擎：emon 用它维护 SkipModeFlag 的 0x04 位，
    // 原版 AUTO 图标是脚本读该变量后自己切精灵的（不占用宿主画面）。
    final autoBridge = _bridge;
    if (autoBridge != null) {
      unawaited(autoBridge.setOption(KzOption.autoMode, value));
    }
    if (value) {
      _enqueue(() async {
        final bridge = _bridge;
        if (bridge == null) {
          return;
        }
        try {
          await _resolve(await bridge.run(budget: 4096));
        } catch (error) {
          state = state.copyWith(error: '$error');
        }
      });
    }
  }

  void setSkip(bool value) {
    state = state.copyWith(skip: value, clearError: true);
    // 把跳过状态同步给引擎，由引擎画原版的 SKIPPING 条；失败不影响跳过本身。
    final bridge = _bridge;
    if (bridge != null) {
      unawaited(bridge.setOption(KzOption.skipOverlay, value));
    }
    if (value) {
      _enqueue(() async {
        final bridge = _bridge;
        if (bridge == null) {
          return;
        }
        try {
          await _resolve(await bridge.run(budget: 4096));
        } catch (error) {
          state = state.copyWith(error: '$error');
        }
      });
    }
  }

  Future<void> openBacklog() async {
    _localBacklog = true;
    state = state.copyWith(backlogActive: true);
  }

  Future<void> exitBacklog() async {
    _localBacklog = false;
    state = state.copyWith(backlogActive: false);
  }

  Future<List<TextEntry>> backlog() async {
    final bridge = _bridge;
    if (bridge == null) {
      return const <TextEntry>[];
    }
    try {
      return await bridge.textList();
    } catch (_) {
      return const <TextEntry>[];
    }
  }

  /// 宿主兜底存档（无原版存档界面的作品走这里）。
  ///
  /// 走 [_action] 而不是直接调 bridge：存档要等前面的 pointer/compose 跑完，
  /// 否则可能存到“半次点击”的中间状态（变量已改、画面还没合成）。
  Future<bool> saveSlot(int slot, {String title = ''}) async {
    var ok = false;
    await _action((bridge) async {
      ok = await bridge.save(slot, title);
    });
    return ok;
  }

  /// 宿主兜底读档（无原版存档界面的作品走这里）。
  ///
  /// 必须经过 [_action] + [_resolve]：引擎读档后会换成存档时的台词、立绘、
  /// BGM 和画面，宿主这边如果不做一次完整同步，state 里还留着读档前的
  /// 文本/动画标记，ticker 也不知道要重新合成——表现就是“点了读档，画面
  /// 还是旧的，或者过几句话就推不动”。这里的 [_resolve] 用默认 compose=true，
  /// 让恢复后的画布立刻上屏。
  Future<void> loadSlot(int slot) {
    return _action((bridge) async {
      final step = await bridge.load(slot);
      await _resolve(step);
    });
  }

  Future<void> resync() {
    return _enqueue(() async {
      final bridge = _bridge;
      if (bridge == null) {
        return;
      }
      try {
        await _resolve(await bridge.run(budget: 4096));
      } catch (error) {
        state = state.copyWith(error: '$error');
      }
    });
  }

  // compose=false 时只推进逻辑并置 _needsCompose，渲染留给 _onTick 在下一个 tick 补，
  // 这样用户的点击 action 不必等 Go 侧全帧合成（+3.7MB 拷贝与 RGBA 解码）后才返回。
  // maxSpins 是这次调用的自旋上限：点击/按键传 _inputSpins（不再自旋），自动 pump 与文本
  // 等待链用 _pumpSpins 保持原节奏。
  Future<void> _resolve(EngineStep first,
      {bool compose = true, int maxSpins = _pumpSpins}) async {
    final bridge = _bridge;
    if (bridge == null || _dead) {
      return;
    }
    // krkr 内核的 run 是"跑一帧引擎"，cont 只表示"引擎没有结束"。这条链不能自旋：
    // 一次 16ms tick 内连跑 64 帧会让软件渲染把 CPU 吃满，成片动画反而掉到个位数 fps。
    // 一 tick 一帧才是它原本的节奏（引擎动画按真实时间推进，不需要追赶预算）。
    if (bridge.frameDriven) {
      maxSpins = 0;
    }
    _resolving = true;
    try {
      var step = first;
      var spins = 0;
      var started = DateTime.now();
      while (!_dead) {
        // 自旋只在自动 pump 路径上有意义（把一次 advance 之后的 cont 链跑完）；用户点击
        // 路径传 maxSpins=0，RPC 立刻返回，剩下的 cont 由 _onTick 的 pump 在 16ms 内接续。
        // _pendingInput 抢占：只要还有用户输入在排队就立刻退出，让 pointer 请求排在引擎
        // isolate 队列前面。!(_pumping && _busy) 保留：自动 pump 自旋期间用户 action 启动
        // 时同样让出。
        while (step == EngineStep.cont &&
            spins < maxSpins &&
            DateTime.now().difference(started).inMilliseconds < 120 &&
            _pendingInput == 0 &&
            !(_pumping && _busy) &&
            !_dead) {
          step = await bridge.run(budget: 4096);
          spins++;
        }
        await _sync(step);
        if (_dead) {
          return;
        }
            // awaitMovie 不可点击推进：影片由 MovieLayer 播，播完它自己回调 movieDone。
        // 若把这里算成 clickable，玩家点一下就会在影片播放期间推进脚本。
        final clickable =
            step == EngineStep.wait || step == EngineStep.awaitInput;
        if (!clickable || state.backlogActive) {
          _waitPending = false;
          break;
        }
        final timed = step == EngineStep.wait ||
            (step == EngineStep.awaitInput && (state.auto || state.skip));
        if (!timed) {
          _waitPending = false;
          break;
        }
        _needsCompose = true;
        final due = DateTime.now().millisecondsSinceEpoch + _delayFor(step);
        if (!_waitPending || due < _waitDueMs) {
          _waitDueMs = due;
        }
        _waitPending = true;
        break;
      }
      _needsCompose = true;
      if (compose) {
        await _compose();
      }
    } finally {
      _resolving = false;
    }
  }

  Future<void> _sync(EngineStep step) async {
    final bridge = _bridge;
    if (bridge == null || _dead) {
      return;
    }
    // 先记"刚同步过"：本次 _sync 会把 audio 事件取走，_drainAudio 的周期轮询要避开这个窗口，
    // 否则点击瞬间会往引擎队列多塞一个空 RPC（排在真正的点击请求前面）。
    _syncAt = DateTime.now().millisecondsSinceEpoch;
    // 一次 RPC 取齐 text/state/audio（旧库不认识 OpFrame(37) 时 bridge 内部自动回退三连 RPC）。
    // step 仍以调用方手里那次 advance/pointer/run 的返回值为准：frame 顺带回来的 step 是"当前"步、
    // 更新，直接用它会让 choices/select 的取数条件与旧行为不一致（本任务不改用户可见行为）。
    final snap = await bridge.frame();
    if (_dead) {
      return;
    }
    final text = snap.text;
    final st = snap.state;
    _waitMs = st.waitMs;
    // 不在关键路径上 await 音频：voice 加载可能数秒，文本/状态/输入先行。
    _queueAudio(snap.audio);
    final List<EngineChoice> choices = step == EngineStep.awaitChoice
        ? await bridge.choices()
        : const <EngineChoice>[];
    List<SelectItem> items = const <SelectItem>[];
    Map<String, EngineFrame> frames = const <String, EngineFrame>{};
    List<MapButton> buttons = const <MapButton>[];
    MapMask mask = const (has: false, width: 0, height: 0);
    if (step == EngineStep.awaitSelect) {
      items = await bridge.selectItems();
      buttons = await bridge.mapButtons();
      mask = await bridge.mapMask();
      final names = <String>{};
      for (final item in items) {
        if (item.frame.isNotEmpty) {
          names.add(item.frame);
        }
      }
      if (names.isNotEmpty) {
        final resolved = <String, EngineFrame>{};
        for (final name in names) {
          final frame = await bridge.selectFrame(name);
          if (frame.width > 0 && frame.height > 0) {
            resolved[name] = frame;
          }
        }
        frames = resolved;
      }
    }
    if (_dead) {
      return;
    }
    // 主界面：引擎停在 AwaitTitle 时把菜单项取回来；离开时清空，
    // 否则标题按钮会一直盖在剧情画面上。
    List<TitleMenuItem> titleMenu = const <TitleMenuItem>[];
    if (step == EngineStep.awaitTitle) {
      titleMenu = await bridge.titleMenu();
    }
    // 影片：引擎停在 awaitMovie（剧本影片）或 awaitTitle（标题 OP）时才会有请求。
    // 引擎在播完之前对每次轮询都返回同一个请求（同序号、同路径），所以"序号和
    // 路径都没变"就等于"这一部已经在播"，不能再触发一次；播完清空后脚本再次
    // 请求同一路径时序号会变，于是正常重新弹出。
    //
    // 这里同时把序号写进 state：MovieLayer 完成时带回来，只有它的序号还是"当前
    // 影片"才允许推进脚本。引擎不等待影片时 current_movie() 返回空，此时不覆盖
    // state.moviePath —— 覆盖会让已经播完的影片重新弹出来。
    String moviePath = state.moviePath;
    int movieSerial = state.movieSerial;
    double movieAudioGain = state.movieAudioGain;
    final request = await bridge.movie();
    if (request.path.isNotEmpty) {
      final replaying = moviePath.isNotEmpty &&
          request.serial == movieSerial &&
          _sameMoviePath(request.path, moviePath);
      if (!replaying) {
        moviePath = request.path;
        movieSerial = request.serial;
        movieAudioGain = request.audioGain;
      }
    }
    if (_dead) {
      return;
    }
    _lastStep = step;
    final wasSkip = state.skip;
    state = state.copyWith(
      autoWait: st.autoWait,
      titleMenu: titleMenu,
      moviePath: moviePath,
      movieSerial: movieSerial,
      movieAudioGain: movieAudioGain,
      hostRequest: st.hostRequest,
      hostRequestSerial: st.hostRequestSerial,
      speaker: text.speaker,
      text: text.text,
      choices: choices,
      selectItems: items,
      selectFrames: frames,
      mapButtons: buttons,
      mapMask: mask,
      messageWindowVisible: st.messageWindowVisible,
      engineMessageWindow: st.engineMessageWindow,
      awaitingName: st.awaitingName || step == EngineStep.awaitName,
      promptIsYesNo: st.promptIsYesNo,
      enginePrompt: st.enginePrompt,
      namePrompt: st.namePrompt,
      replayActive: st.replayActive,
      backlogActive: st.backlogActive || _localBacklog,
      isAnimating: st.animating,
      finished: step == EngineStep.finished,
      faulted: step == EngineStep.faulted,
      // 终端 step（finished/faulted）之后引擎的 LastError 已定型：拉回来给错误层展示。
      // emon 的 trace 是各品牌里唯一有内容的（"emecfg.ecf missing..."/"run failed"）；
      // 对没有 trace 的品牌这里拿到空串，展示层会回退到通用文案，不依赖它。
      faultReason: step == EngineStep.finished || step == EngineStep.faulted
          ? (state.faultReason.isNotEmpty
              ? state.faultReason
              : await _fetchFaultReason(bridge))
          : state.faultReason,
    );
    // 原版跳过规则（MD2 コンフィグ）：
    //  - 「選択肢でスキップを解除する」开：选择肢/地图出现时停；
    //  - 「未読文章もスキップする」关：遇到未读句子停。
    // 两条都必须在状态同步后立刻关掉 skip，否则 ticker 的下一拍又会推进。
    if (wasSkip) {
      final stopForChoice = st.ceaseSkip &&
          (step == EngineStep.awaitChoice || step == EngineStep.awaitSelect);
      final stopForUnread =
          !st.skipUnread && step == EngineStep.awaitInput && st.messageUnread;
      if (stopForChoice || stopForUnread) {
        setSkip(false);
        _waitPending = false;
      }
    }
  }

  /// 终端 step 之后拉一次 [EngineBridge.lastError] 给错误层展示。
  ///
  /// 拉取失败（isolate 已关、引擎未 open）直接回空串：展示层会回退到通用文案，
  /// 绝不能让"取错误信息"这件事本身再抛一个错把故障层盖掉。
  Future<String> _fetchFaultReason(EngineBridge bridge) async {
    final now = DateTime.now().millisecondsSinceEpoch;
    if (now - _faultFetchAt < 500) {
      return state.faultReason;
    }
    _faultFetchAt = now;
    try {
      return (await bridge.lastError()).trim();
    } catch (_) {
      return '';
    }
  }

  void _onTick() {
    if (_dead) {
      return;
    }
    final now = DateTime.now().millisecondsSinceEpoch;
    // 终端 step（finished/faulted）之后引擎不再推进：自动/跳过/interp 的 pump 若继续跑，
    // 只会反复对已结束的会话发 advance/run，还会把故障层底下的帧刷来刷去。
    // ticker 不停（帧合成/音频 drain 仍需要它），只挡逻辑推进这一路。
    if (state.finished || state.faulted) {
      _waitPending = false;
      _needsCompose = false;
      _frameWanted = false;
      return;
    }
    // 按住未抬起（_pressSent）时不跑定时 advance：press 已经写进 @147=1，此刻 advance 会
    // 先把脚本推进过这次点击，真正的 release 到了就只能再跳一句（点一下跳两句），或者
    // 被 pressSeen 判成重复点击而丢给脚本自己的 WaitMs —— 玩家看到的就是 1 秒级延迟。
    // 只挡定时 advance；_lastStep==cont 走的是 run（不消费 WAIT），按住期间照常跑。
    // release 可能永远不来（窗口失焦、触摸被系统手势抢走）——_pressSent 一旦残留为真，
    // 下面的 due 恒为 false、定时 advance 永久停摆，玩家看到的就是"点一下之后卡死"。
    // 按住超过 800ms 仍未 release 视为丢失，清掉标记让定时推进恢复。
    if (_pressSent && now - _pressAt >= 800) {
      _pressSent = false;
    }
    final due = _waitPending && !_pressSent && now >= _waitDueMs;
    final wants = state.isAnimating ||
        due ||
        _frameWanted ||
        _lastStep == EngineStep.cont;
    if (!_resolving && !_pumping && !_busy && wants) {
      _pump(due: due);
    }
    // 逻辑 tick 与渲染解耦：合成请求不能插在 pointer/advance 前面排队，
    // 否则用户输入要等整帧合成跑完才会被引擎处理（Go 侧 Compose 持锁可达几十 ms）。
    // 三个门：_busy（这次点击 action 正在跑）、_pendingInput>0（后面还排着点击）、
    // _pressSent（手还按着，release 随时会到）——任一成立都不合成，避免 release 排在合成后；
    // 另加 _composing 单飞。不能再要求 !_pumping/!_resolving：16ms tick 下自动 pump 每个 tick 都会先启动，
    // 若也挡住合成会让动画期间画面停滞；引擎 isolate 串行处理 run/advance/compose，单飞已足以防止堆积。
    // 再加 33ms 上限：逻辑 pump 仍按 16ms，画面合成最多 30fps，
    // 避免每个 tick 都触发 3.7MB 合成与后续 RGBA 解码把 UI isolate 拖住；
    // 用户 action 已通过 _resolve(compose:false) 走 ticker，不依赖这里立即合成。
    // 最后一道是 UI 解码背压：上一次合成回来的位图还在解码时，这一次触发直接跳过
    // （FrameBus 里有 300ms 兜底，解码卡死不会把画面永久冻住）。跳过不丢画面 —— 引擎
    // 每次合成都是按当前状态重算整帧，下一次触发拿到的是最新状态，只是采样点后移一拍；
    // 而 UI 解不完的帧本来也会被更新的帧顶掉，先要它只是白跑一趟合成 + 3.7MB 拷回。
    //
    // 合成间隔按内核区分：krkr 的帧由引擎自己按真实时间产生（一 tick 一帧），画面只在
    // 引擎真的送显后才变化，所以按节拍合成等于"有动画就 60fps、静止时自动变空转"；
    // 用 33ms 会把它的动画直接砍成 30fps。Go 引擎不同：合成才是绘制动作本身，且每帧
    // 要重算整幅画面 + 3.7MB 拷回，必须留在 ~30fps 以免把 UI isolate 压垮。
    //
    // krkr 的 16ms 不是"每 tick 都真的拷一帧"：引擎没送显时 `dirty=false`，C++ 侧
    // 连 memcpy 都不做，只是一次版本号比较 + 一次 RPC。真正的拷贝只发生在有新画面
    // 的那些 tick 上——这正是 60fps 动画需要的节奏。
    final composeIntervalMs = _bridge?.frameDriven == true ? 16 : 33;
    if (!_busy &&
        _pendingInput == 0 &&
        !_pressSent &&
        !_composing &&
        !frameBus.decodeBusy &&
        (state.isAnimating || _needsCompose) &&
        now - _lastComposeAt >= composeIntervalMs) {
      _lastComposeAt = now;
      // 不在这里清 _needsCompose：_compose 可能因 bridge==null/_composing/尺寸无效提前返回，
      // 提前清标记会把这个合成请求永久丢掉，表现为"点了没反应，得再点一次"。
      // 改由 _compose 真正开工（置 _composing=true）时才消费，早退时留标记由下一 tick 重试。
      _compose();
    }
    // 语音/BGM 物化已挪到 Go 后台，播放事件会晚于触发出现；引擎停在 awaitInput 空闲时
    // 没有 _sync 轮询，晚到的事件会一直等到下次点击才播。这里按约 120ms 兜底 drain 一次
    // （一次空 RPC，开销可忽略），_audioDraining 防重入。
    // 但要避开两个窗口：用户输入在途（别把空 RPC 排在点击前面）、最近 300ms 内 sync 过
    // （_sync 已经取走事件，纯属重复）。_audioPollAt 只在真正发包时更新，所以被挡住的
    // 这段时间不会攒成连续的补发。
    if (!_busy &&
        _pendingInput == 0 &&
        !_pressSent &&
        !_audioDraining &&
        now - _syncAt >= 300 &&
        now - _audioPollAt >= 120) {
      _audioPollAt = now;
      _audioDraining = true;
      _drainAudio();
    }
  }

  Future<void> _drainAudio() async {
    try {
      final bridge = _bridge;
      if (bridge == null || _dead) {
        return;
      }
      final events = await bridge.audio();
      _queueAudio(events);
    } catch (_) {
      // 轮询失败不影响逻辑，下一个周期重试。
    } finally {
      _audioDraining = false;
    }
  }

  Future<void> _pump({bool due = false}) async {
    final bridge = _bridge;
    if (bridge == null || _pumping || _dead) {
      return;
    }
    _frameWanted = false;
    _pumping = true;
    try {
      if (due) {
        _waitPending = false;
        // 自动 pump 只推进逻辑；渲染统一交给 _onTick，避免每个 16ms tick 都全帧合成。
        await _resolve(await bridge.advance(), compose: false);
      } else if (_lastStep == EngineStep.cont) {
        await _resolve(await bridge.run(budget: 4096), compose: false);
      } else {
        _needsCompose = true;
      }
    } catch (_) {
    } finally {
      _pumping = false;
    }
  }

  Future<void> _compose() async {
    final bridge = _bridge;
    if (bridge == null || _composing || _dead) {
      return;
    }
    final width = state.width;
    final height = state.height;
    if (width <= 0 || height <= 0) {
      return;
    }
    final need = width * height * 4;
    if (_pixels.length != need) {
      _pixels = _nextPixelBuffer(need);
      // 分辨率变了：上一帧发布的缓冲尺寸不对，不能再拿来配新的 runs。
      _publishedPixels = null;
      _publishedRuns = const <TextRun>[];
    }
    _composing = true;
    // 只有真正开始合成才消费请求：上面任何早退（单飞冲突/尺寸无效/引擎不在）
    // 都保留 _needsCompose，交给下一 tick 重试。
    _needsCompose = false;
    try {
      // 走 compose 专用 isolate：合成与 pointer/advance/run/frame 现在真并发，
      // 过渡动画期间 30-60ms 的合成不再把点击 RPC 堵在后面。compose isolate 起不来
      // 或中途崩了时，bridge 内部自动回退到主 worker 的 compose，画面不冻结。
      // 回包里同时带回这一帧的文字排版：原生层不再画字，Flutter 按 runs 自己画。
      // 同时带画布尺寸：krkr 的启动选项窗口是 480x280，选完分辨率后正片是
      // 1920x1080。尺寸变化时重分配缓冲并立刻再合成一帧，而不是把大画面
      // 缩进旧的小缓冲（那会让整部游戏都糊成 480x280，点击坐标也会错位）。
      final result =
          await bridge.composeIsolate(_pixels, width: width, height: height);
      if (!_dead &&
          result.width > 0 &&
          result.height > 0 &&
          (result.width != width || result.height != height)) {
        state = state.copyWith(width: result.width, height: result.height);
        _pixels = _nextPixelBuffer(result.width * result.height * 4);
        _publishedPixels = null;
        _publishedRuns = const <TextRun>[];
        _needsCompose = true;
        return;
      }
      if (result.dirty && !_dead) {
        // 免拷贝发布：原来每帧 Uint8List.fromList(_pixels) 要整块拷 3.7MB。
        // 这里把刚合成好的缓冲直接交给 UI（FrameView 解码 ui.Image 时本来就要再拷一份），
        // 同时给下一次 compose 换一块可写缓冲。
        // 不变量：交出去的位图从此只读；_pixels 永远是"下一帧可以写"的缓冲；
        // Go 侧 dirty=true 必写满整个缓冲（dirty=false 一个字节都不写），所以不会发布半帧。
        // _composing 单飞保证同一时刻只有一个 compose 在飞，await 期间没有第二处写 _pixels：
        // compose worker 只往 Go 侧的持久缓冲（native 内存）写，拷回 _pixels 的是回包后
        // 主 isolate 这一次 await 之后的同步 memcpy —— 两边没有"同一缓冲同时被写"的窗口。
        //
        // 缓冲用三块轮转而不是每帧新分配：1080p 一帧 8.3MB，按 60fps 分配就是
        // ~500MB/s 的垃圾，光 GC 就够把渲染线程拖出掉帧。三块足够让"刚发布、UI
        // 正在解码"和"下一帧正在写"互不重叠；轮转回来的那块整体被引擎写满
        // （dirty=true 的契约），不需要清零，省掉一次 8MB 的 memset。
        final published = _pixels;
        // 先登记这张图已交给 UI，再挑下一块写缓冲：_nextPixelBuffer 会跳过
        // _publishedPixels，顺序反了就会把同一块既交出去又当写目标。
        _publishedPixels = published;
        _pixels = _nextPixelBuffer(need);
        // 位图不走 state：写进 FrameBus 只惊动 FrameView 一个 widget，
        // 走 state 则会让播放页所有 playerProvider 订阅者每帧重算一遍选择器。
        // runs 跟着这一帧的字节一起发布：文字位置只对刚合成的位图有效，分开送会错帧。
        final runs = _toFrameRuns(result.runs);
        _publishedRuns = runs;
        frameBus.publish(published, width: width, height: height, runs: runs);
      } else if (!_dead) {
        // 位图没变但文字变了（逐字显示、淡入）：enginec 的 dirty 只描述位图，
        // 和 Go 侧"文字指纹变化也算 dirty"不同。这里补一次只换 runs 的发布，
        // 复用上一帧的字节缓冲——bitmapChanged=false 让它沿用 bitmap 号，
        // FrameView 只 setState 换文字，不会重新解码 ui.Image。
        final runs = _toFrameRuns(result.runs);
        if (!_sameRuns(runs, _publishedRuns)) {
          final published = _publishedPixels;
          _publishedRuns = runs;
          if (published != null) {
            // bitmapChanged: false —— 像素没换，只是文字变了：FrameView 复用已有的
            // ui.Image，只重画文字层，不再解一遍 8MB 位图。
            frameBus.publish(published,
                width: width, height: height, runs: runs, bitmapChanged: false);
          }
        }
      }
    } catch (_) {
    } finally {
      _composing = false;
    }
  }

  /// 影片路径比较：引擎可能给出绝对路径或游戏内的相对名，同一个文件不应被
  /// 当成两部影片重复触发。大小写不敏感（Windows 盘/资源包里的名字大小写不一致）。
  static bool _sameMoviePath(String left, String right) =>
      left.toLowerCase() == right.toLowerCase();

  // 位图没变但文字变了（逐字显示）时，两次 runs 是否等价。TextRun 是
  // frame_channel 的普通类而不是值类型，listEquals 会退化成引用比较，所以在
  // 这里逐字段比：只为避免"文字没变也每帧发一帧"的抖动。
  static bool _sameRuns(List<TextRun> a, List<TextRun> b) {
    if (identical(a, b)) {
      return true;
    }
    if (a.length != b.length) {
      return false;
    }
    for (var i = 0; i < a.length; i++) {
      final left = a[i];
      final right = b[i];
      if (left.text != right.text ||
          left.x != right.x ||
          left.y != right.y ||
          left.size != right.size ||
          left.color != right.color ||
          left.outline != right.outline ||
          left.shadow != right.shadow ||
          left.width != right.width ||
          left.bold != right.bold) {
        return false;
      }
    }
    return true;
  }

  int _clampWait(int value) {
    // 0/帧等待必须在下一个 tick 恢复：最小值 40ms 会跨两个 33ms tick，
    // 把 60fps 的游戏直接拖成约 15fps（WAIT FRAME=1 的文字/过渡被拉长到 1–2 秒）。
    // 规格里的"帧等待"（StepWait 且 WaitMs==0，宿主 pump 一次 = 一游戏帧）也走这条下限：16ms。
    if (value < 16) {
      return 16;
    }
    if (value > 8000) {
      return 8000;
    }
    return value;
  }

  int _delayFor(EngineStep step) {
    if (step == EngineStep.awaitInput) {
      return state.skip ? 16 : _autoDelayMs(state.text);
    }
    // StepWait：_waitMs==0 是"帧等待"，_clampWait 会归一到 16ms —— 每个 16ms tick 武装一次
    // advance，正好一个游戏帧（见 _clampWait 注释）。
    return state.skip ? 16 : _clampWait(_waitMs);
  }

  int _centerX() {
    return state.width > 0 ? state.width ~/ 2 : -1;
  }

  int _centerY() {
    return state.height > 0 ? state.height ~/ 2 : -1;
  }

  int _autoDelayMs(String text) {
    // autoWait 是引擎设置值 +1；0 表示该作品没有オート速度设置
    // （非 Bishop 引擎的 State 不填这个字段），退回旧的文本长度公式。
    if (state.autoWait <= 0) {
      var seconds = 0.9 + text.length * 0.08;
      if (seconds < 0.9) {
        seconds = 0.9;
      }
      if (seconds > 6.0) {
        seconds = 6.0;
      }
      return (seconds * 1000).round();
    }
    // 原版「オート再生速度」是 10 档；引擎设置里存 0..100，越大越快。
    // 不要再用固定公式：那样玩家调了滑块完全没效果。这里把 0..100 线性
    // 映射到"基础停顿 2600..400ms + 每字 8..2ms"，并给整句留出朗读时间。
    final speed = (state.autoWait - 1).clamp(0, 100);
    final base = 2600 - speed * 22.0;
    final perChar = 8.0 - speed * 0.06;
    final ms = (base + text.length * perChar).round();
    return ms.clamp(300, 6000).toInt();
  }

}

final playerProvider =
    NotifierProvider.autoDispose<PlayerNotifier, PlayerState>(
        PlayerNotifier.new);
