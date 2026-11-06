import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;

import '../../core/bridge/kernel_bridge.dart';
import '../../core/bridge/kernel_provider.dart';
import '../../core/settings/settings.dart';
import '../settings/settings_notifier.dart';

const List<String> _providerKinds = <String>['openai', 'google', 'claude'];

enum TranslationJobState { idle, running, done, failed, canceled }

class TranslationLog {
  final String time;
  final String level;
  final String code;
  final String detail;

  const TranslationLog({
    required this.time,
    required this.level,
    required this.code,
    this.detail = '',
  });
}

class TranslationState {
  final bool loaded;
  final List<TranslatorStoredFile> files;
  final int selectedFile;
  final List<TranslatorPreset> presets;
  final String selectedPresetId;
  final TranslatorParameters parameters;
  final List<KernelTranslateFormat> formats;
  final bool formatsLoading;
  final TranslationJobState jobState;
  final bool isRunning;
  final int taskId;
  final double progress;
  final String progressText;
  final int promptTokens;
  final int completionTokens;
  final int requests;
  final int failed;
  final int elapsedMs;
  final double unitsPerSecond;
  final List<TranslationLog> logs;
  final bool? lastOk;
  final String error;

  const TranslationState({
    this.loaded = false,
    this.files = const <TranslatorStoredFile>[],
    this.selectedFile = -1,
    this.presets = const <TranslatorPreset>[],
    this.selectedPresetId = '',
    this.parameters = const TranslatorParameters(),
    this.formats = const <KernelTranslateFormat>[],
    this.formatsLoading = false,
    this.jobState = TranslationJobState.idle,
    this.isRunning = false,
    this.taskId = 0,
    this.progress = 0,
    this.progressText = '',
    this.promptTokens = 0,
    this.completionTokens = 0,
    this.requests = 0,
    this.failed = 0,
    this.elapsedMs = 0,
    this.unitsPerSecond = 0,
    this.logs = const <TranslationLog>[],
    this.lastOk,
    this.error = '',
  });

  TranslatorStoredFile? get file =>
      selectedFile >= 0 && selectedFile < files.length
          ? files[selectedFile]
          : null;

  TranslatorPreset? get preset {
    for (final item in presets) {
      if (item.id == selectedPresetId) {
        return item;
      }
    }
    return presets.isEmpty ? null : presets.first;
  }

  TranslationState copyWith({
    bool? loaded,
    List<TranslatorStoredFile>? files,
    int? selectedFile,
    List<TranslatorPreset>? presets,
    String? selectedPresetId,
    TranslatorParameters? parameters,
    List<KernelTranslateFormat>? formats,
    bool? formatsLoading,
    TranslationJobState? jobState,
    bool? isRunning,
    int? taskId,
    double? progress,
    String? progressText,
    int? promptTokens,
    int? completionTokens,
    int? requests,
    int? failed,
    int? elapsedMs,
    double? unitsPerSecond,
    List<TranslationLog>? logs,
    bool? lastOk,
    bool clearLastOk = false,
    String? error,
  }) {
    return TranslationState(
      loaded: loaded ?? this.loaded,
      files: files ?? this.files,
      selectedFile: selectedFile ?? this.selectedFile,
      presets: presets ?? this.presets,
      selectedPresetId: selectedPresetId ?? this.selectedPresetId,
      parameters: parameters ?? this.parameters,
      formats: formats ?? this.formats,
      formatsLoading: formatsLoading ?? this.formatsLoading,
      jobState: jobState ?? this.jobState,
      isRunning: isRunning ?? this.isRunning,
      taskId: taskId ?? this.taskId,
      progress: progress ?? this.progress,
      progressText: progressText ?? this.progressText,
      promptTokens: promptTokens ?? this.promptTokens,
      completionTokens: completionTokens ?? this.completionTokens,
      requests: requests ?? this.requests,
      failed: failed ?? this.failed,
      elapsedMs: elapsedMs ?? this.elapsedMs,
      unitsPerSecond: unitsPerSecond ?? this.unitsPerSecond,
      logs: logs ?? this.logs,
      lastOk: clearLastOk ? null : (lastOk ?? this.lastOk),
      error: error ?? this.error,
    );
  }
}

class TranslationNotifier extends Notifier<TranslationState> {
  Timer? _pollTimer;
  bool _finishing = false;
  bool _loadingFormats = false;

  SettingsRepository get _repo => ref.read(settingsRepositoryProvider);

  KernelBridge get _kernel => ref.read(kernelBridgeProvider);

  @override
  TranslationState build() {
    ref.onDispose(() {
      _pollTimer?.cancel();
      _pollTimer = null;
    });
    return const TranslationState();
  }

  Future<void> ensureLoaded() async {
    if (state.loaded) {
      return;
    }
    final stored = await _repo.loadTranslatorSettings();
    final files = stored.recentFiles;
    final presets = stored.presets;
    state = state.copyWith(
      loaded: true,
      files: files,
      selectedFile: files.isEmpty ? -1 : 0,
      presets: presets,
      selectedPresetId: presets.isEmpty ? '' : presets.first.id,
      parameters: stored.parameters,
    );
  }

  Future<void> _persist() async {
    await _repo.saveTranslatorSettings(TranslatorSettings(
      presets: state.presets,
      parameters: state.parameters,
      recentFiles: state.files,
    ));
  }

  void selectFile(int index) {
    if (index < 0 || index >= state.files.length) {
      return;
    }
    state = state.copyWith(selectedFile: index);
  }

  void selectPreset(String id) {
    state = state.copyWith(selectedPresetId: id);
  }

  Future<void> addFile(String inputPath) async {
    final trimmed = inputPath.trim();
    if (trimmed.isEmpty) {
      return;
    }
    final entry = TranslatorStoredFile(
      inputPath: trimmed,
      outputPath: defaultOutputPath(trimmed),
      formatFeatureId: '',
      formatName: '',
    );
    final files = <TranslatorStoredFile>[...state.files, entry];
    state = state.copyWith(files: files, selectedFile: files.length - 1);
    await _persist();
  }

  Future<void> updateFile(
    int index, {
    String? outputPath,
    String? formatFeatureId,
    String? formatName,
  }) async {
    if (index < 0 || index >= state.files.length) {
      return;
    }
    final files = state.files.toList();
    final current = files[index];
    final output = (outputPath ?? current.outputPath).trim();
    files[index] = TranslatorStoredFile(
      inputPath: current.inputPath,
      outputPath: output.isEmpty ? current.outputPath : output,
      formatFeatureId: formatFeatureId ?? current.formatFeatureId,
      formatName: formatName ?? current.formatName,
    );
    state = state.copyWith(files: files);
    await _persist();
  }

  Future<void> removeFile(int index) async {
    if (index < 0 || index >= state.files.length) {
      return;
    }
    final files = state.files.toList()..removeAt(index);
    var selected = state.selectedFile;
    if (selected >= files.length) {
      selected = files.isEmpty ? -1 : 0;
    }
    state = state.copyWith(files: files, selectedFile: selected);
    await _persist();
  }

  Future<void> addPreset(TranslatorPreset preset) async {
    final presets = <TranslatorPreset>[...state.presets, preset];
    state = state.copyWith(
      presets: presets,
      selectedPresetId:
          state.selectedPresetId.isEmpty ? preset.id : state.selectedPresetId,
    );
    await _persist();
  }

  Future<void> updatePreset(TranslatorPreset preset) async {
    final presets = state.presets
        .map((item) => item.id == preset.id ? preset : item)
        .toList();
    state = state.copyWith(presets: presets);
    await _persist();
  }

  Future<void> removePreset(String id) async {
    final presets =
        state.presets.where((item) => item.id != id).toList();
    var selected = state.selectedPresetId;
    if (selected == id) {
      selected = presets.isEmpty ? '' : presets.first.id;
    }
    state = state.copyWith(presets: presets, selectedPresetId: selected);
    await _persist();
  }

  Future<void> setParameters(TranslatorParameters parameters) async {
    state = state.copyWith(
      parameters: TranslatorParameters(
        concurrency: parameters.concurrency.clamp(1, 32),
        chunkSize: parameters.chunkSize.clamp(5, 200),
        japaneseRetryLimit: parameters.japaneseRetryLimit.clamp(0, 5),
        targetLanguage: parameters.targetLanguage,
        temperature: parameters.temperature,
        glossary: parameters.glossary,
        contextLines: parameters.contextLines,
      ),
    );
    await _persist();
  }

  Future<void> loadFormats(String inputPath) async {
    if (_loadingFormats) {
      return;
    }
    _loadingFormats = true;
    state = state.copyWith(formatsLoading: true);
    try {
      final formats = await _kernel.detectFormats(inputPath);
      state = state.copyWith(formats: formats, formatsLoading: false);
    } catch (error) {
      state = state.copyWith(
        formats: const <KernelTranslateFormat>[],
        formatsLoading: false,
        error: '$error',
      );
    } finally {
      _loadingFormats = false;
    }
  }

  Future<bool> start() async {
    if (state.isRunning) {
      return false;
    }
    final file = state.file;
    if (file == null) {
      state = state.copyWith(error: 'no-file');
      return false;
    }
    final preset = state.preset;
    if (preset == null) {
      state = state.copyWith(error: 'no-preset');
      return false;
    }
    if (file.formatFeatureId.trim().isEmpty) {
      state = state.copyWith(error: 'no-format');
      return false;
    }
    final params = state.parameters;
    final provider = KernelProvider(
      kind: _providerKind(preset.apiSpec),
      baseUrl: preset.baseUrl,
      model: preset.model,
      apiKey: preset.apiKey,
      timeoutSeconds: 120,
      maxRetries: 3,
    );
    final translateParams = KernelTranslateParams(
      concurrency: params.concurrency,
      chunkSize: params.chunkSize,
      japaneseRetryLimit: params.japaneseRetryLimit,
      targetLanguage: params.targetLanguage,
      temperature: params.temperature,
      glossary: params.glossary,
      contextLines: params.contextLines,
    );
    state = state.copyWith(
      isRunning: true,
      jobState: TranslationJobState.running,
      progress: 0,
      progressText: 'starting',
      promptTokens: 0,
      completionTokens: 0,
      requests: 0,
      failed: 0,
      elapsedMs: 0,
      unitsPerSecond: 0,
      logs: _append(state.logs, 'info', 'started', p.basename(file.inputPath)),
      error: '',
      clearLastOk: true,
    );
    _finishing = false;
    try {
      final taskId = await _kernel.startTranslation(
        source: file.inputPath,
        target: file.outputPath,
        feature: file.formatFeatureId,
        provider: provider,
        params: translateParams,
      );
      state = state.copyWith(taskId: taskId);
    } catch (error) {
      state = state.copyWith(
        isRunning: false,
        jobState: TranslationJobState.failed,
        error: '$error',
      );
      return false;
    }
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(milliseconds: 250), (_) {
      _poll();
    });
    return true;
  }

  Future<void> cancel() async {
    if (!state.isRunning) {
      return;
    }
    _pollTimer?.cancel();
    _pollTimer = null;
    state = state.copyWith(
      logs: _append(state.logs, 'warn', 'canceling'),
    );
    final taskId = state.taskId;
    try {
      await _kernel.cancelTranslation(taskId);
    } catch (_) {}
    _finishCanceled();
  }

  Future<void> _poll() async {
    final taskId = state.taskId;
    KernelTranslatePoll poll;
    try {
      poll = await _kernel.pollTranslation(taskId);
    } catch (error) {
      state = state.copyWith(
        logs: _append(state.logs, 'error', 'raw', '$error'),
      );
      return;
    }
    for (final event in poll.events) {
      _applyEvent(event);
    }
    if (poll.done && state.isRunning) {
      _pollTimer?.cancel();
      _pollTimer = null;
      _finishDone(true, 'finished', '');
    }
  }

  void _applyEvent(KernelTranslateEvent event) {
    switch (event.type) {
      case 'log':
        state = state.copyWith(
          logs: _append(state.logs,
              event.level?.isEmpty ?? true ? 'info' : event.level!, 'raw', event.message ?? ''),
        );
      case 'progress':
        if (event.total > 0) {
          state = state.copyWith(
            progress: event.current / event.total,
            progressText: '${event.current}/${event.total}',
          );
        } else {
          state = state.copyWith(
            progress: (event.percent / 100).clamp(0.0, 1.0).toDouble(),
            progressText: '${event.percent.toStringAsFixed(1)}%',
          );
        }
      case 'stats':
        state = state.copyWith(
          promptTokens: event.promptTokens,
          completionTokens: event.completionTokens,
          requests: event.requests,
          failed: event.failed,
          elapsedMs: event.elapsedMs,
          unitsPerSecond: event.unitsPerSecond,
        );
      case 'residual':
        state = state.copyWith(
          logs: _append(state.logs, 'info', 'residual',
              '${event.count}/${event.round}'),
        );
      case 'error':
        state = state.copyWith(
          logs: _append(state.logs, 'error', 'raw', event.message ?? ''),
        );
      case 'done':
        _pollTimer?.cancel();
        _pollTimer = null;
        final ok = event.ok == true;
        _finishDone(ok, ok ? 'translated' : 'failed',
            ok ? '${event.translated}' : (event.message ?? ''));
    }
  }

  void _finishDone(bool ok, String code, String detail) {
    if (_finishing) {
      return;
    }
    _finishing = true;
    state = state.copyWith(
      isRunning: false,
      jobState: ok ? TranslationJobState.done : TranslationJobState.failed,
      logs: _append(state.logs, ok ? 'info' : 'error', code, detail),
      lastOk: ok,
    );
    _rememberFile();
  }

  void _finishCanceled() {
    if (_finishing) {
      return;
    }
    _finishing = true;
    state = state.copyWith(
      isRunning: false,
      jobState: TranslationJobState.canceled,
      logs: _append(state.logs, 'warn', 'canceled'),
      lastOk: false,
    );
  }

  Future<void> _rememberFile() async {
    final file = state.file;
    if (file == null) {
      return;
    }
    final files = state.files
        .where((item) => item.inputPath != file.inputPath)
        .toList();
    files.insert(0, file);
    state = state.copyWith(
      files: files,
      selectedFile: 0,
    );
    await _persist();
  }

  void clearError() {
    if (state.error.isNotEmpty) {
      state = state.copyWith(error: '');
    }
  }

  static String defaultOutputPath(String inputPath) {
    final directory = p.dirname(inputPath);
    final name = p.basenameWithoutExtension(inputPath);
    final extension = p.extension(inputPath);
    return p.join(directory, '$name.zh$extension');
  }

  static String _providerKind(int apiSpec) {
    if (apiSpec < 0 || apiSpec >= _providerKinds.length) {
      return _providerKinds.first;
    }
    return _providerKinds[apiSpec];
  }

  static List<TranslationLog> _append(
      List<TranslationLog> logs, String level, String code,
      [String detail = '']) {
    final now = DateTime.now();
    final time =
        '${now.hour.toString().padLeft(2, '0')}:${now.minute.toString().padLeft(2, '0')}:${now.second.toString().padLeft(2, '0')}';
    final next = <TranslationLog>[
      ...logs,
      TranslationLog(time: time, level: level, code: code, detail: detail),
    ];
    while (next.length > 500) {
      next.removeAt(0);
    }
    return next;
  }
}

final translationProvider =
    NotifierProvider<TranslationNotifier, TranslationState>(
        TranslationNotifier.new);
