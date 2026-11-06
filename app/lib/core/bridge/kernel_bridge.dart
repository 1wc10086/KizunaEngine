import 'dart:async';
import 'dart:convert';

import 'native.dart';

class KzKernelException implements Exception {
  final String message;
  const KzKernelException(this.message);

  @override
  String toString() => 'KzKernelException: $message';
}

class KernelHandler {
  final String action;
  final String label;
  final List<String> suffixes;
  final String outputSuffix;

  const KernelHandler({
    required this.action,
    required this.label,
    required this.suffixes,
    required this.outputSuffix,
  });

  factory KernelHandler.fromJson(Map<String, Object?> json) => KernelHandler(
        action: json['action'] as String? ?? '',
        label: json['label'] as String? ?? '',
        suffixes: ((json['suffixes'] as List?) ?? const [])
            .map((entry) => '$entry')
            .toList(),
        outputSuffix: json['outputSuffix'] as String? ?? '',
      );
}

class KernelFeature {
  final String id;
  final String name;
  final String target;
  final List<KernelHandler> handlers;

  const KernelFeature({
    required this.id,
    required this.name,
    required this.target,
    required this.handlers,
  });

  factory KernelFeature.fromJson(Map<String, Object?> json) => KernelFeature(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        target: json['target'] as String? ?? '',
        handlers: ((json['handlers'] as List?) ?? const [])
            .map((entry) =>
                KernelHandler.fromJson(entry as Map<String, Object?>))
            .toList(),
      );
}

class KernelDetectCandidate {
  final String featureId;
  final String featureName;
  final String action;
  final String actionLabel;
  final String label;
  final String format;
  final String inputPath;
  final String outputPath;

  const KernelDetectCandidate({
    required this.featureId,
    required this.featureName,
    required this.action,
    required this.actionLabel,
    required this.label,
    required this.format,
    required this.inputPath,
    required this.outputPath,
  });

  factory KernelDetectCandidate.fromJson(Map<String, Object?> json) =>
      KernelDetectCandidate(
        featureId: json['featureId'] as String? ?? '',
        featureName: json['featureName'] as String? ?? '',
        action: json['action'] as String? ?? '',
        actionLabel: json['actionLabel'] as String? ?? '',
        label: json['label'] as String? ?? '',
        format: json['format'] as String? ?? '',
        inputPath: json['inputPath'] as String? ?? '',
        outputPath: json['outputPath'] as String? ?? '',
      );
}

class KernelDetectResult {
  final String inputPath;
  final String inputKind;
  final String suffix;
  final String target;
  final String outputPath;
  final List<KernelDetectCandidate> candidates;
  final String message;

  const KernelDetectResult({
    required this.inputPath,
    required this.inputKind,
    required this.suffix,
    required this.target,
    required this.outputPath,
    required this.candidates,
    required this.message,
  });

  factory KernelDetectResult.fromJson(Map<String, Object?> json) =>
      KernelDetectResult(
        inputPath: json['inputPath'] as String? ?? '',
        inputKind: json['inputKind'] as String? ?? '',
        suffix: json['suffix'] as String? ?? '',
        target: json['target'] as String? ?? '',
        outputPath: json['outputPath'] as String? ?? '',
        candidates: ((json['candidates'] as List?) ?? const [])
            .map((entry) =>
                KernelDetectCandidate.fromJson(entry as Map<String, Object?>))
            .toList(),
        message: json['message'] as String? ?? '',
      );
}

class KernelProcessResult {
  final String inputPath;
  final String outputPath;
  final int bytesIn;
  final int bytesOut;
  final int elapsedMs;
  final String summary;
  final Map<String, String> details;

  const KernelProcessResult({
    required this.inputPath,
    required this.outputPath,
    required this.bytesIn,
    required this.bytesOut,
    required this.elapsedMs,
    required this.summary,
    required this.details,
  });

  factory KernelProcessResult.fromJson(Map<String, Object?> json) =>
      KernelProcessResult(
        inputPath: json['inputPath'] as String? ?? '',
        outputPath: json['outputPath'] as String? ?? '',
        bytesIn: (json['bytesIn'] as num?)?.toInt() ?? 0,
        bytesOut: (json['bytesOut'] as num?)?.toInt() ?? 0,
        elapsedMs: (json['elapsedMs'] as num?)?.toInt() ?? 0,
        summary: json['summary'] as String? ?? '',
        details: ((json['details'] as Map?) ?? const {})
            .map((key, value) => MapEntry('$key', '$value')),
      );
}

class KernelTranslateFormat {
  final String featureId;
  final String format;
  final String name;

  const KernelTranslateFormat({
    required this.featureId,
    required this.format,
    required this.name,
  });

  factory KernelTranslateFormat.fromJson(Map<String, Object?> json) =>
      KernelTranslateFormat(
        featureId: json['featureId'] as String? ?? '',
        format: json['format'] as String? ?? '',
        name: json['name'] as String? ?? '',
      );
}

class KernelProvider {
  final String kind;
  final String baseUrl;
  final String model;
  final String apiKey;
  final int timeoutSeconds;
  final int maxRetries;

  const KernelProvider({
    required this.kind,
    required this.baseUrl,
    required this.model,
    required this.apiKey,
    required this.timeoutSeconds,
    required this.maxRetries,
  });

  Map<String, Object?> toJson() => <String, Object?>{
        'kind': kind,
        'baseUrl': baseUrl,
        'model': model,
        'apiKey': apiKey,
        'timeoutSeconds': timeoutSeconds,
        'maxRetries': maxRetries,
      };
}

class KernelTranslateParams {
  final int concurrency;
  final int chunkSize;
  final int japaneseRetryLimit;
  final String targetLanguage;
  final double temperature;
  final String glossary;
  final int contextLines;

  const KernelTranslateParams({
    required this.concurrency,
    required this.chunkSize,
    required this.japaneseRetryLimit,
    required this.targetLanguage,
    required this.temperature,
    required this.glossary,
    required this.contextLines,
  });

  Map<String, Object?> toJson() => <String, Object?>{
        'concurrency': concurrency,
        'chunkSize': chunkSize,
        'japaneseRetryLimit': japaneseRetryLimit,
        'targetLanguage': targetLanguage,
        'temperature': temperature,
        'glossary': glossary,
        'contextLines': contextLines,
      };
}

class KernelTranslateEvent {
  final String type;
  final String? level;
  final String? message;
  final int current;
  final int total;
  final double percent;
  final int promptTokens;
  final int completionTokens;
  final int requests;
  final int failed;
  final int elapsedMs;
  final double unitsPerSecond;
  final int count;
  final int round;
  final bool? ok;
  final String? outputPath;
  final int translated;
  final int skipped;

  const KernelTranslateEvent({
    required this.type,
    required this.level,
    required this.message,
    required this.current,
    required this.total,
    required this.percent,
    required this.promptTokens,
    required this.completionTokens,
    required this.requests,
    required this.failed,
    required this.elapsedMs,
    required this.unitsPerSecond,
    required this.count,
    required this.round,
    required this.ok,
    required this.outputPath,
    required this.translated,
    required this.skipped,
  });

  factory KernelTranslateEvent.fromJson(Map<String, Object?> json) =>
      KernelTranslateEvent(
        type: json['type'] as String? ?? '',
        level: json['level'] as String?,
        message: json['message'] as String?,
        current: (json['current'] as num?)?.toInt() ?? 0,
        total: (json['total'] as num?)?.toInt() ?? 0,
        percent: (json['percent'] as num?)?.toDouble() ?? 0.0,
        promptTokens: (json['promptTokens'] as num?)?.toInt() ?? 0,
        completionTokens: (json['completionTokens'] as num?)?.toInt() ?? 0,
        requests: (json['requests'] as num?)?.toInt() ?? 0,
        failed: (json['failed'] as num?)?.toInt() ?? 0,
        elapsedMs: (json['elapsedMs'] as num?)?.toInt() ?? 0,
        unitsPerSecond: (json['unitsPerSecond'] as num?)?.toDouble() ?? 0.0,
        count: (json['count'] as num?)?.toInt() ?? 0,
        round: (json['round'] as num?)?.toInt() ?? 0,
        ok: json['ok'] as bool?,
        outputPath: json['outputPath'] as String?,
        translated: (json['translated'] as num?)?.toInt() ?? 0,
        skipped: (json['skipped'] as num?)?.toInt() ?? 0,
      );
}

class KernelTranslatePoll {
  final bool done;
  final List<KernelTranslateEvent> events;

  const KernelTranslatePoll({required this.done, required this.events});
}

class KernelBridge {
  final NativeKernelBindings _bindings;
  Future<void> _gate = Future<void>.value();

  KernelBridge(this._bindings);

  factory KernelBridge.open() =>
      KernelBridge(NativeKernelBindings.open());

  String get version => _bindings.versionString();

  Future<T> _serial<T>(Future<T> Function() run) {
    final next = _gate.then((_) => run());
    _gate = next.then((_) {}, onError: (_) {});
    return next;
  }

  Future<Map<String, Object?>> _send(Map<String, Object?> request) {
    return _serial(() async {
      final body = _bindings.invokeJson(jsonEncode(request));
      final decoded = jsonDecode(body);
      if (decoded is! Map<String, Object?>) {
        throw const KzKernelException('kernel returned a non-object response');
      }
      if (decoded['ok'] != true) {
        throw KzKernelException('${decoded['error'] ?? 'unknown kernel error'}');
      }
      return decoded;
    });
  }

  Future<String> ping() async {
    final envelope = await _send(<String, Object?>{'op': 'ping'});
    return '${envelope['version'] ?? ''}';
  }

  Future<List<KernelFeature>> features() async {
    final envelope = await _send(<String, Object?>{'op': 'features'});
    return ((envelope['features'] as List?) ?? const [])
        .map((entry) => KernelFeature.fromJson(entry as Map<String, Object?>))
        .toList();
  }

  Future<KernelDetectResult> detect(String path) async {
    final envelope = await _send(
        <String, Object?>{'op': 'detect', 'path': path});
    final detect = envelope['detect'];
    if (detect is! Map<String, Object?>) {
      throw const KzKernelException(
          'kernel detect response missed the detect payload');
    }
    return KernelDetectResult.fromJson(detect);
  }

  Future<KernelProcessResult> process({
    required String feature,
    required String action,
    required String input,
    required String output,
    String? encoding,
    Map<String, String>? extra,
  }) async {
    final request = <String, Object?>{
      'op': 'process',
      'feature': feature,
      'action': action,
      'input': input,
      'output': output,
    };
    if (encoding != null) {
      request['encoding'] = encoding;
    }
    if (extra != null) {
      request['extra'] = extra;
    }
    final envelope = await _send(request);
    final result = envelope['result'];
    if (result is! Map<String, Object?>) {
      throw const KzKernelException(
          'kernel process response missed the result payload');
    }
    return KernelProcessResult.fromJson(result);
  }

  Future<List<KernelTranslateFormat>> detectFormats(String path) async {
    final envelope = await _send(
        <String, Object?>{'op': 'translate.detect', 'path': path});
    return ((envelope['formats'] as List?) ?? const [])
        .map((entry) =>
            KernelTranslateFormat.fromJson(entry as Map<String, Object?>))
        .toList();
  }

  Future<int> startTranslation({
    required String source,
    required String target,
    required String feature,
    String sourceEncoding = 'auto',
    String targetEncoding = 'utf8',
    required KernelProvider provider,
    required KernelTranslateParams params,
  }) async {
    final envelope = await _send(<String, Object?>{
      'op': 'translate.start',
      'source': source,
      'target': target,
      'feature': feature,
      'sourceEncoding': sourceEncoding,
      'targetEncoding': targetEncoding,
      'provider': provider.toJson(),
      'params': params.toJson(),
    });
    return (envelope['taskId'] as num?)?.toInt() ?? 0;
  }

  Future<KernelTranslatePoll> pollTranslation(int taskId) async {
    final envelope = await _send(
        <String, Object?>{'op': 'translate.poll', 'taskId': taskId});
    return KernelTranslatePoll(
      done: envelope['done'] == true,
      events: ((envelope['events'] as List?) ?? const [])
          .map((entry) =>
              KernelTranslateEvent.fromJson(entry as Map<String, Object?>))
          .toList(),
    );
  }

  Future<bool> cancelTranslation(int taskId) async {
    final envelope = await _send(
        <String, Object?>{'op': 'translate.cancel', 'taskId': taskId});
    return envelope['canceled'] == true;
  }
}
