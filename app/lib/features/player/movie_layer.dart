import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
// 用官方 video_player 的控制器接口，只把"平台实现"换成 fvp：
// 这样播放控制（initialize/play/dispose）走成熟实现，
// 解码器换成 libmdk 以支持 MPEG-1。
import 'package:video_player/video_player.dart';

import 'player_notifier.dart';

/// 影片播放层。
///
/// krkr 作品用 `op.mpg` / `staffroll.mpg` 这类 MPEG-1 Program Stream 做 OP 与片尾。
/// Flutter 官方 `video_player` 在 Android 上走 MediaCodec，对 MPEG-1 支持残缺，
/// 很多机型直接黑屏；这里改用 fvp（libmdk + ffmpeg 系解码器），
/// 它能稳定解 MPEG-1，也不需要额外转码游戏资源。
///
/// 生命周期要点：影片期间引擎停在 awaitMovie 等 `movieDone`。所以这里的每一条
/// 退出路径（正常播完 / 解码失败 / 超时 / 玩家跳过 / widget 提前 dispose）都
/// 必须且只能回调一次 [PlayerNotifier.finishMovie]，否则脚本要么永久卡死，
/// 要么被重复推进两句。
class MovieLayer extends StatefulWidget {
  final String path;

  /// 引擎给的影片序号：回调 movieDone 时带回去，上一次影片的迟到回调不会
  /// 被当成当前影片播完（详见 PlayerNotifier.finishMovie）。
  final int serial;

  /// 音轨增益（0..1），引擎的 audioVolume（KAG 的 `@movie volume=`）。
  /// 只对播放器的音频轨生效，画面不受影响。
  final double audioGain;

  final PlayerNotifier notifier;

  const MovieLayer({
    super.key,
    required this.path,
    required this.serial,
    required this.audioGain,
    required this.notifier,
  });

  @override
  State<MovieLayer> createState() => _MovieLayerState();
}

class _MovieLayerState extends State<MovieLayer> {
  // 212MB 的 op.mpg 在低端机上探测容器可能要十几秒；超过这个时间就当作
  // 打不开，让脚本继续，避免玩家永远停在黑屏。正常文件通常 1s 内完成。
  static const Duration _initializeTimeout = Duration(seconds: 20);
  // 解码失败时不要立刻收场：先让错误在屏幕上停留一会儿，
  // 玩家（和我们排查问题的人）才看得到究竟为什么播不出来。
  static const Duration _errorHold = Duration(milliseconds: 2500);
  // completed 事件在个别设备/文件上会丢（最后一帧被丢弃时尤其常见），
  // 用位置兜底：离结尾 400ms 内就视为播完。
  static const Duration _completionSlack = Duration(milliseconds: 400);
  // 播放中看门狗：position 长时间不动（解码线程僵死、Surface 丢失）时也要收场，
  // 否则引擎会一直等一个不会来的 movieDone。
  static const Duration _stallTimeout = Duration(seconds: 8);

  VideoPlayerController? _controller;
  bool _finished = false;
  bool _disposed = false;
  String? _error;
  Duration _lastPosition = Duration.zero;
  DateTime _lastProgressAt = DateTime.now();

  @override
  void initState() {
    super.initState();
    unawaited(_play());
  }

  Future<void> _play() async {
    final path = widget.path;
    VideoPlayerController? controller;
    try {
      debugPrint('movie: opening $path (serial ${widget.serial})');
      controller = VideoPlayerController.file(
        File(path),
        // 影片独占音频焦点是期望行为（OP 期间不需要别的 BGM），所以不设 mixWithOthers。
        videoPlayerOptions: VideoPlayerOptions(mixWithOthers: false),
      );
      _controller = controller;
      await controller.initialize().timeout(_initializeTimeout);
      if (_disposed || _finished) {
        // dispose()（或 _finish）可能已经把控制器摘走并释放了：只有还挂在
        // _controller 上时才由这里释放，避免二次 dispose。
        if (identical(_controller, controller)) {
          _controller = null;
          await controller.dispose();
        }
        return;
      }
      final value = controller.value;
      if (value.hasError) {
        throw StateError(value.errorDescription ?? 'unknown video error');
      }
      controller.addListener(_onStateChanged);
      // 音轨增益必须在 play 之前设：先出声再改音量会有一个满音量的起手，
      // 而片子往往一开场就是音乐。fvp 之后改 setVolume 也生效，
      // 但这里一次到位更稳。
      await controller.setVolume(widget.audioGain.clamp(0.0, 1.0).toDouble());
      await controller.setLooping(false);
      await controller.play();
      _lastProgressAt = DateTime.now();
      debugPrint(
        'movie: playing ${value.size.width.toInt()}x${value.size.height.toInt()} '
        'duration=${value.duration}',
      );
      if (mounted) {
        setState(() {});
      }
      _startWatchdog();
    } catch (error) {
      debugPrint('movie: failed to play $path: $error');
      if (_finished) {
        return;
      }
      // 初始化失败的控制器可能已半初始化：显式释放，避免解码器/文件句柄泄漏。
      // 同时把 _controller 摘掉，否则后面的 _finish 会再 dispose 同一个控制器。
      if (identical(_controller, controller)) {
        _controller = null;
      }
      if (controller != null) {
        try {
          await controller.dispose();
        } catch (_) {}
      }
      if (_disposed) {
        await _finish();
        return;
      }
      if (mounted) {
        setState(() {
          _error = '$error';
        });
      }
      await Future<void>.delayed(_errorHold);
      await _finish();
    }
  }

  /// 位置长时间不前进就收场。正常播放时 position 每帧都在变；僵死时它停在
  /// 某一帧不动，脚本会永远等下去。
  void _startWatchdog() {
    Timer.periodic(const Duration(seconds: 2), (timer) {
      if (_finished || _disposed) {
        timer.cancel();
        return;
      }
      final controller = _controller;
      if (controller == null) {
        timer.cancel();
        return;
      }
      final position = controller.value.position;
      if (position != _lastPosition) {
        _lastPosition = position;
        _lastProgressAt = DateTime.now();
        return;
      }
      if (DateTime.now().difference(_lastProgressAt) > _stallTimeout) {
        debugPrint('movie: playback stalled at $position, finishing');
        timer.cancel();
        unawaited(_finish());
      }
    });
  }

  void _onStateChanged() {
    final controller = _controller;
    if (controller == null || _finished) {
      return;
    }
    final value = controller.value;
    if (value.hasError) {
      debugPrint('movie: playback error ${value.errorDescription}');
      unawaited(_finish());
      return;
    }
    if (value.isCompleted) {
      unawaited(_finish());
      return;
    }
    if (value.duration > Duration.zero &&
        value.position >= value.duration - _completionSlack) {
      unawaited(_finish());
    }
  }

  /// 结束播放并只回调一次。
  ///
  /// 四条路径会走到这里（正常播完 / 解码失败 / 播放僵死 / 玩家点跳过），
  /// 而引擎的 `movieDone` 会推进脚本，重复调用会让剧情跳两句，
  /// 所以用 _finished 做幂等保护。
  Future<void> _finish() async {
    if (_finished) {
      return;
    }
    _finished = true;
    final controller = _controller;
    _controller = null;
    if (controller != null) {
      controller.removeListener(_onStateChanged);
      try {
        await controller.pause();
      } catch (_) {}
      try {
        await controller.dispose();
      } catch (_) {}
    }
    // 带序号：如果这一部影片已经不是当前影片（换片/退出），notifier 会忽略它。
    try {
      await widget.notifier.finishMovie(serial: widget.serial);
    } catch (error) {
      // movieDone 的 RPC 失败（引擎已关）不能把异常抛回 ticker/监听器：
      // 这时覆盖层已经拆掉、脚本也不可能再推进，报错只会污染日志。
      debugPrint('movie: movieDone failed: $error');
    }
  }

  @override
  void dispose() {
    _disposed = true;
    // widget 被提前移除（玩家退出播放页、换片）时也要把脚本推下去，
    // 否则引擎会一直等一个永远不会来的 movieDone。finishMovie 内部按序号判重，
    // 所以这里 fire-and-forget 是安全的。
    if (!_finished) {
      _finished = true;
      final controller = _controller;
      _controller = null;
      if (controller != null) {
        controller.removeListener(_onStateChanged);
        unawaited(controller.dispose().then<void>((_) {}, onError: (_) {}));
      }
      unawaited(widget.notifier.finishMovie(serial: widget.serial));
    }
    super.dispose();
  }

  @override
  void didUpdateWidget(MovieLayer oldWidget) {
    super.didUpdateWidget(oldWidget);
    // 引擎会在影片播放期间重发同一部影片的状态（每次 state 轮询都会带
    // audioVolume），但只改音量。序号/路径变化由 MovieGate 的 key 重建覆盖，
    // 这里只处理"同一部影片换了音量"这一种。
    if (oldWidget.audioGain == widget.audioGain) {
      return;
    }
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) {
      return;
    }
    unawaited(
      controller
          .setVolume(widget.audioGain.clamp(0.0, 1.0).toDouble())
          .then<void>((_) {}, onError: (_) {}),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final controller = _controller;
    final ready = controller != null && controller.value.isInitialized;
    return Positioned.fill(
      child: ColoredBox(
        color: const Color(0xFF000000),
        child: Stack(
          children: <Widget>[
            if (ready)
              Center(
                child: AspectRatio(
                  aspectRatio: controller.value.aspectRatio,
                  child: VideoPlayer(controller),
                ),
              )
            else if (_error != null)
              Center(
                child: Padding(
                  padding: const EdgeInsets.all(24),
                  child: Text(
                    '${l10n.commonError}\n${widget.path}\n$_error',
                    textAlign: TextAlign.center,
                    style: const TextStyle(color: Color(0xFFB0B0B0)),
                  ),
                ),
              )
            else
              const Center(child: CircularProgressIndicator()),
            // 跳过按钮：MPEG-1 影片可能很长（op.mpg 有 212MB），
            // 玩家应该有办法直接跳过，原引擎也支持点击跳过。
            Positioned(
              right: 16,
              bottom: 16,
              child: TextButton(
                onPressed: _finish,
                child: Text(l10n.playerMovieSkip),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// 影片层的挂载点。
///
/// 单独抽一层是为了让 PlayerStage 不必关心"什么时候该建播放器"：
/// 只有 moviePath 非空时才构造 MovieLayer，播完自动消失（moviePath 被清空）。
/// 同一时刻只可能有一个影片在播：widget 树里只有这一个位置，换片时 key 变化
/// 会先拆掉旧的（旧层 dispose 里的 finishMovie 按序号判重，不会误推新片）。
class MovieGate extends ConsumerWidget {
  final PlayerNotifier notifier;

  const MovieGate({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // 一次 select 取齐三个字段：分两次 watch 会在引擎每帧重发同一部影片时
    // 触发两次重建（画面会闪一下）。
    final request = ref.watch(playerProvider.select((value) => (
          path: value.moviePath,
          serial: value.movieSerial,
          gain: value.movieAudioGain,
        )));
    final path = request.path;
    if (path.isEmpty) {
      return const SizedBox.shrink();
    }
    // key 用序号 + 路径：换片时强制重建播放器，避免复用上一个影片的解码状态。
    // 音量不进 key —— 同一部影片的音量变化走 didUpdateWidget，不能重开解码器。
    return MovieLayer(
      key: ValueKey<String>('${request.serial}\u0000$path'),
      path: path,
      serial: request.serial,
      audioGain: request.gain,
      notifier: notifier,
    );
  }
}
