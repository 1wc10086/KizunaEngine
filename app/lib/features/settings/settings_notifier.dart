import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/audio/game_audio.dart' as audio_core;
import '../../core/bridge/protocol_consts.dart';
import '../../core/platform/app_paths.dart';
import '../../core/settings/settings.dart';
import '../player/player_notifier.dart';

final settingsRepositoryProvider = Provider<SettingsRepository>((ref) {
  throw UnimplementedError('settingsRepositoryProvider is not overridden');
});

final globalAudioProvider = Provider<audio_core.GameAudio>((ref) {
  final audio = audio_core.GameAudio();
  audio.init();
  return audio;
});

class SettingsNotifier extends AsyncNotifier<AppSettings> {
  SettingsRepository get _repo => ref.read(settingsRepositoryProvider);

  @override
  Future<AppSettings> build() async {
    final loaded = await _repo.loadAppSettings();
    // 首次启动（或用户清空过目录）时落到平台默认目录。这里持久化一次，
    // 避免每次启动都重新解析 path_provider，也让“恢复默认”有明确的值。
    if ((loaded.gameRoot ?? '').trim().isEmpty) {
      try {
        final root = await defaultGameRoot();
        final updated = loaded.copyWith(gameRoot: root);
        await _repo.saveAppSettings(updated);
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
    final current = state.value ?? const AppSettings();
    final trimmed = (root ?? '').trim();
    if (trimmed.isEmpty) {
      await _store(current.copyWith(clearGameRoot: true));
    } else {
      await _store(current.copyWith(gameRoot: trimmed));
    }
  }

  Future<void> setShowMessageFrame(bool value) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(showMessageFrame: value));
    final session = ref.read(playerSessionProvider);
    if (session != null) {
      await session.setOption(KzOption.showMessageFrame, value);
    }
  }

  Future<void> setAutoUpdateCheck(bool value) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(autoUpdateCheck: value));
  }

  Future<void> setThemeMode(AppThemeMode mode) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(themeMode: mode));
  }

  Future<void> setThemePalette(String paletteId) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(themePaletteId: paletteId));
  }

  Future<void> setAmoled(bool value) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(amoled: value));
  }

  Future<void> setFontScale(double value) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(fontScale: value.clamp(0.8, 1.5).toDouble()));
  }

  Future<void> setLanguage(AppLanguage value) async {
    final current = state.value ?? const AppSettings();
    await _store(current.copyWith(language: value));
  }
}

final appSettingsProvider =
    AsyncNotifierProvider<SettingsNotifier, AppSettings>(SettingsNotifier.new);
