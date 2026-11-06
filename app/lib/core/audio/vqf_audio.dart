import 'dart:async';
import 'dart:io';

// 用 fvp 解码 TwinVQ (.vqf)：flutter_soloud 走 SoLoud/miniaudio，不含 VQF 解码器，
// 而 fvp（libmdk + FFmpeg）能解 TwinVQ，所以人声 / 音效的 .vqf 不走 SoLoud，
// 单独用 video_player 的 VideoPlayerController（平台实现已在 main.dart 注册为 fvp）播放。
import 'package:video_player/video_player.dart';

// fvp 探测容器对小语音文件通常很快；超时兜底避免坏文件把调用方永久挂住。
const Duration _initializeTimeout = Duration(seconds: 10);
// 尾帧丢失时位置会停在结尾前一小段，用 slack 兜底判完成（对齐 movie_layer）。
const Duration _completionSlack = Duration(milliseconds: 400);

/// 一条 .vqf 播放的句柄，对应 GameAudio 里 _Slot 的 vqf 后端。
///
/// voice 是单槽替换、effect 是多槽并存的调度仍由 GameAudio 的 _voice/_effects 负责，
/// 这里只管一条流的生命周期：初始化、音量、循环、播完检测、释放。
final class VqfHandle {
  VqfHandle(this.controller, {required this.volume, required this.loop});

  final VideoPlayerController controller;
  double volume;
  bool loop;
  bool _disposed = false;

  bool get isDisposed => _disposed;

  /// 标记已释放（幂等：重复 dispose 不再碰控制器，避免二次释放报错）。
  void markDisposed() {
    _disposed = true;
  }
}

double _clampVolume(double value) {
  if (!value.isFinite) {
    return 0;
  }
  return value.clamp(0, 1).toDouble();
}

/// 打开并播放一个 .vqf 文件。
///
/// initialize() 失败（格式不支持、文件不存在、解码器缺失）直接静默返回 null，
/// 不向上抛异常——缺声音比崩溃好（与 GameAudio 对 SoLoud 加载失败的处理一致）。
Future<VqfHandle?> startVqf(
  String path, {
  required double volume,
  required bool loop,
}) async {
  final clamped = _clampVolume(volume);
  VideoPlayerController? controller;
  try {
    controller = VideoPlayerController.file(
      File(path),
      // galgame 的语音/BGM 要同时响；不加 mixWithOthers 时 Android 的
      // 音频焦点会把 BGM 压低或暂停，听感与原作不符。
      videoPlayerOptions: VideoPlayerOptions(mixWithOthers: true),
    );
    await controller.initialize().timeout(_initializeTimeout);
    final value = controller.value;
    if (value.hasError) {
      try {
        await controller.dispose();
      } catch (_) {}
      return null;
    }
    await controller.setVolume(clamped);
    await controller.setLooping(loop);
    await controller.play();
    return VqfHandle(controller, volume: clamped, loop: loop);
  } catch (_) {
    // 初始化失败静默：控制器可能已半初始化，必须释放避免解码器泄漏。
    if (controller != null) {
      try {
        await controller.dispose();
      } catch (_) {}
    }
    return null;
  }
}

/// 播完检测：优先用 isCompleted（新版 fvp 会置位），同时用 position/duration
/// 轮询兜底（个别设备/文件尾帧事件会丢）。循环播放永不算播完。
bool vqfFinished(VqfHandle handle) {
  if (handle.loop) {
    return false;
  }
  final value = handle.controller.value;
  if (!value.isInitialized) {
    return false;
  }
  // 不用 VideoPlayerValue.isCompleted：部分 fvp/video_player 版本没有该字段，
  // 位置/时长轮询对所有版本都成立；400ms slack 处理尾帧事件丢失。
  final duration = value.duration;
  if (duration <= Duration.zero) {
    return false;
  }
  final threshold = duration > _completionSlack
      ? duration - _completionSlack
      : duration;
  return value.position >= threshold;
}

/// 同步改音量（GameAudio 的 _setHandleVolume 是同步接口）。
/// 异步失败只吞掉：音量是“尽力而为”，失败不值得让游戏报错。
void setVqfVolume(VqfHandle handle, double volume) {
  final clamped = _clampVolume(volume);
  handle.volume = clamped;
  if (handle.isDisposed) {
    return;
  }
  try {
    handle.controller.setVolume(clamped).then((_) {}, onError: (_) {});
  } catch (_) {}
}

/// 同步改循环标志（保持句柄状态与控制器一致）。
void setVqfLooping(VqfHandle handle, bool loop) {
  handle.loop = loop;
  if (handle.isDisposed) {
    return;
  }
  try {
    handle.controller.setLooping(loop).then((_) {}, onError: (_) {});
  } catch (_) {}
}

/// 停止并释放：先 pause 再 dispose，确保解码器线程干净退出。
/// 幂等：已释放的句柄直接返回。
Future<void> stopVqf(VqfHandle handle) async {
  if (handle.isDisposed) {
    return;
  }
  handle.markDisposed();
  try {
    await handle.controller.pause();
  } catch (_) {}
  try {
    await handle.controller.dispose();
  } catch (_) {}
}

/// 只释放（播完后调用）：与 stop 相同，播完时已不在播放，无需 pause 语义，仍走幂等释放。
Future<void> disposeVqf(VqfHandle handle) => stopVqf(handle);

/// 轻量封装：语音单槽（新句替换旧句）+ 一次性音效池（多句可并发，池满时淘汰最老）。
///
/// GameAudio 内部用 _voice/_effects 槽位自己调度（需要与 SoLoud 槽统一），
/// 但这个类把“单语音 + 多音效池”语义收敛成独立接口，供不走 GameAudio 的
/// 调用方直接使用，也能让 fvp 的控制器所有权更清楚：whoever start，谁负责 stop/dispose。
final class VqfPlayer {
  /// 音效池大小：>=2 保证快速连播的音效互不掐断；一次排满即淘汰最老的一句。
  static const int effectPoolSize = 4;

  VqfHandle? _voice;
  final List<VqfHandle> _effects = [];

  bool get hasVoice => _voice != null;

  int get effectCount => _effects.length;

  /// 播放一句语音：替换当前语音（若有，先停掉再开新的）。
  /// 失败（解码/打开问题）静默返回 false，不抛异常上层。
  Future<bool> playVoice(String path, {double volume = 1, bool loop = false}) async {
    await stopVoice();
    final handle = await startVqf(path, volume: volume, loop: loop);
    if (handle == null) {
      return false;
    }
    _voice = handle;
    return true;
  }

  Future<void> stopVoice() async {
    final voice = _voice;
    _voice = null;
    if (voice != null) {
      await stopVqf(voice);
    }
  }

  /// 播放一次性音效：与池中其他音效并发；池满时淘汰最老的一条。
  Future<bool> playEffect(String path, {double volume = 1}) async {
    if (_effects.length >= effectPoolSize) {
      final oldest = _effects.removeAt(0);
      await stopVqf(oldest);
    }
    final handle = await startVqf(path, volume: volume, loop: false);
    if (handle == null) {
      return false;
    }
    _effects.add(handle);
    return true;
  }

  Future<void> stopEffects() async {
    final effects = _effects.toList();
    _effects.clear();
    for (final handle in effects) {
      await stopVqf(handle);
    }
  }

  void setVoiceVolume(double volume) {
    final voice = _voice;
    if (voice != null) {
      setVqfVolume(voice, volume);
    }
  }

  void setVoiceLoop(bool loop) {
    final voice = _voice;
    if (voice != null) {
      setVqfLooping(voice, loop);
    }
  }

  /// 轮询并回收已播完的音效（播完即 dispose）。循环的语音永不回收，
  /// 只有 stopVoice/stopAll/dispose 会释放它。
  void pollEffects() {
    final done = _effects.where(vqfFinished).toList();
    for (final handle in done) {
      _effects.remove(handle);
      // 播完后释放（fire-and-forget：stopVqf 本身不对失败抛异常）。
      stopVqf(handle);
      handle.markDisposed();
    }
  }

  bool get voiceFinished {
    final voice = _voice;
    if (voice == null) {
      return true;
    }
    if (!vqfFinished(voice)) {
      return false;
    }
    // 播完即释放，避免解码器泄漏（fire-and-forget 同上）。
    _voice = null;
    stopVqf(voice);
    voice.markDisposed();
    return true;
  }

  Future<void> stopAll() async {
    await stopVoice();
    await stopEffects();
  }

  Future<void> dispose() => stopAll();
}
