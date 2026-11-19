import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/audio/game_audio.dart' as audio_core;
import '../../../core/platform/app_paths.dart';
import '../../../core/settings/settings.dart';
import '../../../core/settings/settings_snapshot.dart';

/// 主题/语言/字号这类"整页重绘"所需的派生值。
///
/// 用 select 只取需要的字段：设置里还有 gameRoot、autoUpdateCheck 这些
/// 与主题无关的键，任何一次写入都会让 [appSettingsProvider] 换新对象；
/// 不 select 的话，改游戏目录也会把整棵 MaterialApp 重建一遍。
class AppearanceSettings {
  final AppThemeMode themeMode;
  final String paletteId;
  final bool amoled;
  final double fontScale;
  final AppLanguage language;

  const AppearanceSettings({
    required this.themeMode,
    required this.paletteId,
    required this.amoled,
    required this.fontScale,
    required this.language,
  });

  @override
  bool operator ==(Object other) =>
      other is AppearanceSettings &&
      other.themeMode == themeMode &&
      other.paletteId == paletteId &&
      other.amoled == amoled &&
      other.fontScale == fontScale &&
      other.language == language;

  @override
  int get hashCode =>
      Object.hash(themeMode, paletteId, amoled, fontScale, language);
}

class SettingsNotifier extends AsyncNotifier<AppSettings> {
  SettingsRepository get _repo => ref.read(settingsRepositoryProvider);

  @override
  Future<AppSettings> build() async {
    // _repo 是同步 getter，读的是注入的 provider；build 一旦开始就先把
    // 仓库拿到手，避免 await 期间 ref 被销毁（Riverpod 3 会抛
    // UnmountedRefException）后再去读一次。
    final repo = _repo;
    final loaded = await repo.loadAppSettings();
    // 首次启动（或用户清空过目录）时落到平台默认目录。这里持久化一次，
    // 避免每次启动都重新解析 path_provider，也让“恢复默认”有明确的值。
    if ((loaded.gameRoot ?? '').trim().isEmpty) {
      try {
        final root = await defaultGameRoot();
        final updated = loaded.copyWith(gameRoot: root);
        await repo.saveAppSettings(updated);
        return updated;
      } catch (_) {
        return loaded;
      }
    }
    return loaded;
  }

  Future<void> _store(AppSettings settings) async {
    await _repo.saveAppSettings(settings);
    state = AsyncData(settings);
  }

  Future<void> setGameRoot(String? root) async {
    final current = state.valueOrDefault;
    final trimmed = (root ?? '').trim();
    if (trimmed.isEmpty) {
      await _store(current.copyWith(clearGameRoot: true));
    } else {
      await _store(current.copyWith(gameRoot: trimmed));
    }
  }

  // 消息窗开关只落盘；"把开关推给正在运行的引擎"由播放页监听设置完成。
  // 让设置层反过来 import 播放层会把两个 feature 绑成环，而且引擎没跑时
  // 这条调用本来就没有意义。
  Future<void> setShowMessageFrame(bool value) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(showMessageFrame: value));
  }

  Future<void> setAutoUpdateCheck(bool value) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(autoUpdateCheck: value));
  }

  Future<void> setThemeMode(AppThemeMode mode) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(themeMode: mode));
  }

  Future<void> setThemePalette(String paletteId) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(themePaletteId: paletteId));
  }

  Future<void> setAmoled(bool value) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(amoled: value));
  }

  Future<void> setFontScale(double value) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(fontScale: value.clamp(0.8, 1.5).toDouble()));
  }

  Future<void> setLanguage(AppLanguage value) async {
    final current = state.valueOrDefault;
    await _store(current.copyWith(language: value));
  }
}

final appSettingsProvider =
    AsyncNotifierProvider<SettingsNotifier, AppSettings>(SettingsNotifier.new);

/// 全局唯一的音频引擎实例。
///
/// 播放页把它包成 PlayerNotifier 的音频端口，离开播放页后仍然存活
/// （SoLoud 初始化与解码缓存都很贵，不能跟着页面一起销毁）。定义在这里
/// 而不是某个播放页文件里，是因为它必须比播放页活得久；播放页只读不写。
final globalAudioProvider = Provider<audio_core.GameAudio>((ref) {
  final audio = audio_core.GameAudio();
  // 初始化是异步的，这里刻意不 await：失败或未就绪时，后续播放请求由
  // GameAudio 内部自己排队/降级，不阻塞首帧。
  unawaited(audio.init());
  return audio;
});

/// 只含"影响整页外观"的字段，供 app.dart 订阅。
///
/// AppearanceSettings 实现了值相等，所以设置里改 gameRoot 之类不会触发
/// MaterialApp 重建——Riverpod 对 Provider 的刷新判断走 `==`。
final appearanceSettingsProvider = Provider<AppearanceSettings>((ref) {
  final settings = ref.watch(appSettingsProvider).valueOrDefault;
  return AppearanceSettings(
    themeMode: settings.themeMode,
    paletteId: settings.themePaletteId,
    amoled: settings.amoled,
    fontScale: settings.fontScale,
    language: settings.language,
  );
});
