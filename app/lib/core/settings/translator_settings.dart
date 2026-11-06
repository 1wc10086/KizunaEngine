class TranslatorParameters {
  final int concurrency;
  final int chunkSize;
  final int japaneseRetryLimit;
  final String targetLanguage;
  final double temperature;
  final String glossary;
  final int contextLines;

  const TranslatorParameters({
    this.concurrency = 4,
    this.chunkSize = 30,
    this.japaneseRetryLimit = 2,
    this.targetLanguage = '简体中文',
    this.temperature = 0.7,
    this.glossary = '',
    this.contextLines = 2,
  });

  TranslatorParameters copyWith({
    int? concurrency,
    int? chunkSize,
    int? japaneseRetryLimit,
    String? targetLanguage,
    double? temperature,
    String? glossary,
    int? contextLines,
  }) =>
      TranslatorParameters(
        concurrency: concurrency ?? this.concurrency,
        chunkSize: chunkSize ?? this.chunkSize,
        japaneseRetryLimit: japaneseRetryLimit ?? this.japaneseRetryLimit,
        targetLanguage: targetLanguage ?? this.targetLanguage,
        temperature: temperature ?? this.temperature,
        glossary: glossary ?? this.glossary,
        contextLines: contextLines ?? this.contextLines,
      );

  Map<String, Object?> toJson() => <String, Object?>{
        'concurrency': concurrency,
        'chunkSize': chunkSize,
        'japaneseRetryLimit': japaneseRetryLimit,
        'targetLanguage': targetLanguage,
        'temperature': temperature,
        'glossary': glossary,
        'contextLines': contextLines,
      };

  factory TranslatorParameters.fromJson(Map<String, Object?> json) =>
      TranslatorParameters(
        concurrency: (json['concurrency'] as num?)?.toInt() ?? 4,
        chunkSize: (json['chunkSize'] as num?)?.toInt() ?? 30,
        japaneseRetryLimit: (json['japaneseRetryLimit'] as num?)?.toInt() ?? 2,
        targetLanguage: json['targetLanguage'] as String? ?? '简体中文',
        temperature: (json['temperature'] as num?)?.toDouble() ?? 0.7,
        glossary: json['glossary'] as String? ?? '',
        contextLines: (json['contextLines'] as num?)?.toInt() ?? 2,
      );
}

class TranslatorPreset {
  final String id;
  final String name;
  final int apiSpec;
  final String baseUrl;
  final String model;
  final String apiKey;

  const TranslatorPreset({
    required this.id,
    required this.name,
    required this.apiSpec,
    required this.baseUrl,
    required this.model,
    required this.apiKey,
  });

  Map<String, Object?> toJson() => <String, Object?>{
        'id': id,
        'name': name,
        'apiSpec': apiSpec,
        'baseUrl': baseUrl,
        'model': model,
        'apiKey': apiKey,
      };

  factory TranslatorPreset.fromJson(Map<String, Object?> json) =>
      TranslatorPreset(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        apiSpec: (json['apiSpec'] as num?)?.toInt() ?? 0,
        baseUrl: json['baseUrl'] as String? ?? '',
        model: json['model'] as String? ?? '',
        apiKey: json['apiKey'] as String? ?? '',
      );
}

class TranslatorStoredFile {
  final String inputPath;
  final String outputPath;
  final String formatFeatureId;
  final String formatName;

  const TranslatorStoredFile({
    required this.inputPath,
    required this.outputPath,
    required this.formatFeatureId,
    required this.formatName,
  });

  Map<String, Object?> toJson() => <String, Object?>{
        'inputPath': inputPath,
        'outputPath': outputPath,
        'formatFeatureId': formatFeatureId,
        'formatName': formatName,
      };

  factory TranslatorStoredFile.fromJson(Map<String, Object?> json) =>
      TranslatorStoredFile(
        inputPath: json['inputPath'] as String? ?? '',
        outputPath: json['outputPath'] as String? ?? '',
        formatFeatureId: json['formatFeatureId'] as String? ?? '',
        formatName: json['formatName'] as String? ?? '',
      );
}

class TranslatorSettings {
  final List<TranslatorPreset> presets;
  final TranslatorParameters parameters;
  final List<TranslatorStoredFile> recentFiles;

  const TranslatorSettings({
    this.presets = const <TranslatorPreset>[],
    this.parameters = const TranslatorParameters(),
    this.recentFiles = const <TranslatorStoredFile>[],
  });

  TranslatorSettings copyWith({
    List<TranslatorPreset>? presets,
    TranslatorParameters? parameters,
    List<TranslatorStoredFile>? recentFiles,
  }) =>
      TranslatorSettings(
        presets: presets ?? this.presets,
        parameters: parameters ?? this.parameters,
        recentFiles: recentFiles ?? this.recentFiles,
      );

  Map<String, Object?> toJson() => <String, Object?>{
        'presets': presets.map((preset) => preset.toJson()).toList(),
        'parameters': parameters.toJson(),
        'recentFiles': recentFiles.map((file) => file.toJson()).toList(),
      };

  factory TranslatorSettings.fromJson(Map<String, Object?> json) =>
      TranslatorSettings(
        presets: _decodePresets(json['presets']),
        parameters: _decodeParameters(json['parameters']),
        recentFiles: _decodeStoredFiles(json['recentFiles']),
      );
}

Map<String, Object?>? _asJsonMap(Object? value) {
  if (value is Map<String, Object?>) {
    return value;
  }
  if (value is Map) {
    return value.map((key, item) => MapEntry('$key', item));
  }
  return null;
}

TranslatorParameters _decodeParameters(Object? value) {
  final map = _asJsonMap(value);
  if (map == null) {
    return const TranslatorParameters();
  }
  try {
    return TranslatorParameters.fromJson(map);
  } catch (_) {
    return const TranslatorParameters();
  }
}

List<TranslatorPreset> _decodePresets(Object? value) {
  if (value is! List) {
    return const <TranslatorPreset>[];
  }
  final out = <TranslatorPreset>[];
  for (final entry in value) {
    final map = _asJsonMap(entry);
    if (map != null) {
      try {
        out.add(TranslatorPreset.fromJson(map));
      } catch (_) {
        continue;
      }
    }
  }
  return out;
}

List<TranslatorStoredFile> _decodeStoredFiles(Object? value) {
  if (value is! List) {
    return const <TranslatorStoredFile>[];
  }
  final out = <TranslatorStoredFile>[];
  for (final entry in value) {
    final map = _asJsonMap(entry);
    if (map != null) {
      try {
        out.add(TranslatorStoredFile.fromJson(map));
      } catch (_) {
        continue;
      }
    }
  }
  return out;
}
