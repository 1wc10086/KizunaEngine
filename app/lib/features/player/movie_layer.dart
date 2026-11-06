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
/// Bishop 用 `op.mpg` / `staffroll.mpg` 这类 MPEG-1 Program Stream 做 OP 与片尾。
/// Flutter 官方 `video_player` 在 Android 上走 MediaCodec，对 MPEG-1 支持残缺，
/// 很多机型直接黑屏；这里改用 fvp（libmdk + ffmpeg 系解码器），
/// 它能稳定解 MPEG-1，也不需要额外转码游戏资源。
///
/// 生命周期要点：影片期间引擎停在原地等 `movieDone`，所以这个 widget 的
/// dispose 必须保证一定回调 notifier，否则脚本会永久卡住（详见 _finish）。
class MovieLayer extends StatefulWidget {
  final String path;
  final PlayerNotifier notifier;

  const MovieLayer({super.key, required this.path, required this.notifier});

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

  VideoPlayerController? _controller;
  bool _finished = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    unawaited(_play());
  }

  Future<void> _play() async {
    final path = widget.path;
    try {
      debugPrint('movie: opening $path');
      final controller = VideoPlayerController.file(File(path));
      _controller = controller;
      await controller.initialize().timeout(_initializeTimeout);
      if (!mounted || _finished) {
        await controller.dispose();
        return;
      }
      final value = controller.value;
      if (value.hasError) {
        throw StateError(value.errorDescription ?? 'unknown video error');
      }
      controller.addListener(_onStateChanged);
      await controller.setLooping(false);
      await controller.play();
      debugPrint(
        'movie: playing ${value.size.width.toInt()}x${value.size.height.toInt()} '
        'duration=${value.duration}',
      );
      if (mounted) {
        setState(() {});
      }
    } catch (error) {
      debugPrint('movie: failed to play $path: $error');
      if (_finished) {
        return;
      }
      if (!mounted) {
        await _finish();
        return;
      }
      setState(() {
        _error = '$error';
      });
      await Future<void>.delayed(_errorHold);
      await _finish();
    }
  }

  void _onStateChanged() {
    final controller = _controller;
    if (controller == null) {
      return;
    }
    final value = controller.value;
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
  /// 三条路径会走到这里（正常播完 / 解码失败 / 玩家点跳过），
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
      await controller.dispose();
    }
    await widget.notifier.finishMovie();
  }

  @override
  void dispose() {
    // widget 被提前移除（玩家退出播放页）时也要把脚本推下去，
    // 否则引擎会一直等一个永远不会来的 movieDone。
    if (!_finished) {
      _finished = true;
      final controller = _controller;
      _controller = null;
      controller?.dispose();
      unawaited(widget.notifier.finishMovie());
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
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
class MovieGate extends ConsumerWidget {
  final PlayerNotifier notifier;

  const MovieGate({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final path = ref.watch(playerProvider.select((value) => value.moviePath));
    if (path.isEmpty) {
      return const SizedBox.shrink();
    }
    // key 用路径：换片时强制重建播放器，避免复用上一个影片的解码状态。
    return MovieLayer(key: ValueKey<String>(path), path: path, notifier: notifier);
  }
}
