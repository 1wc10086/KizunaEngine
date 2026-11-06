enum AppThemeMode { system, light, dark }

enum AppLanguage { system, zh, en }

const String _defaultPaletteId = 'purple';

class AppSettings {
  final String? gameRoot;
  final bool showMessageFrame;
  final bool autoUpdateCheck;
  final AppThemeMode themeMode;
  final String themePaletteId;
  final bool amoled;
  final double fontScale;
  final AppLanguage language;

  const AppSettings({
    this.gameRoot,
    this.showMessageFrame = true,
    this.autoUpdateCheck = true,
    this.themeMode = AppThemeMode.system,
    this.themePaletteId = _defaultPaletteId,
    this.amoled = false,
    this.fontScale = 1,
    this.language = AppLanguage.system,
  });

  AppSettings copyWith({
    String? gameRoot,
    bool clearGameRoot = false,
    bool? showMessageFrame,
    bool? autoUpdateCheck,
    AppThemeMode? themeMode,
    String? themePaletteId,
    bool? amoled,
    double? fontScale,
    AppLanguage? language,
  }) =>
      AppSettings(
        gameRoot: clearGameRoot ? null : (gameRoot ?? this.gameRoot),
        showMessageFrame: showMessageFrame ?? this.showMessageFrame,
        autoUpdateCheck: autoUpdateCheck ?? this.autoUpdateCheck,
        themeMode: themeMode ?? this.themeMode,
        themePaletteId: themePaletteId ?? this.themePaletteId,
        amoled: amoled ?? this.amoled,
        fontScale: _clampFontScale(fontScale ?? this.fontScale),
        language: language ?? this.language,
      );

  Map<String, Object?> toJson() => <String, Object?>{
        'gameRoot': gameRoot,
        'showMessageFrame': showMessageFrame,
        'autoUpdateCheck': autoUpdateCheck,
        'themeMode': themeMode.name,
        'themePaletteId': themePaletteId,
        'amoled': amoled,
        'fontScale': fontScale,
        'language': language.name,
      };

  factory AppSettings.fromJson(Map<String, Object?> json) {
    final root = json['gameRoot'];
    return AppSettings(
      gameRoot: root is String && root.isNotEmpty ? root : null,
      // 旧设置文件里没有这个键时也按"开启"处理：引擎渲染的消息窗口是默认形态，
      // 只有玩家显式关过才回落到 Flutter 的 MessageBox。
      showMessageFrame: _asBool(json['showMessageFrame'], true),
      autoUpdateCheck: _asBool(json['autoUpdateCheck'], true),
      themeMode: _asThemeMode(json['themeMode']),
      themePaletteId: _asPaletteId(json['themePaletteId']),
      amoled: _asBool(json['amoled'], false),
      fontScale: _clampFontScale(_asDouble(json['fontScale'], 1)),
      language: _asLanguage(json['language']),
    );
  }

  static double _asDouble(Object? value, double fallback) {
    if (value is num) {
      final converted = value.toDouble();
      return converted.isFinite ? converted : fallback;
    }
    return fallback;
  }

  static bool _asBool(Object? value, bool fallback) =>
      value is bool ? value : fallback;

  static String _asPaletteId(Object? value) =>
      value is String && value.isNotEmpty ? value : _defaultPaletteId;

  static AppThemeMode _asThemeMode(Object? value) {
    final name = value is String ? value : '';
    return AppThemeMode.values.firstWhere(
      (mode) => mode.name == name,
      orElse: () => AppThemeMode.system,
    );
  }

  static AppLanguage _asLanguage(Object? value) {
    final name = value is String ? value : '';
    return AppLanguage.values.firstWhere(
      (language) => language.name == name,
      orElse: () => AppLanguage.system,
    );
  }

  static double _clampFontScale(double value) {
    if (!value.isFinite) {
      return 1;
    }
    if (value < 0.8) {
      return 0.8;
    }
    if (value > 1.5) {
      return 1.5;
    }
    return value;
  }
}
