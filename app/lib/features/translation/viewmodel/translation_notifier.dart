import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;

import '../../../core/bridge/kernel_bridge.dart';
import '../../../core/bridge/kernel_provider.dart';
import '../../../core/settings/settings.dart';
import '../model/translation_errors.dart';
import '../model/translation_state.dart';

const List<String> _providerKinds = <String>['openai', 'google', 'claude'];

/// 翻译流水线的 viewmodel：文件/预设的增删改、任务启动/取消/轮询。
///
/// 与内核之间是"启动一次任务 + 每 250ms 拉一次进度"的轮询模型（见
/// kernel 的 translate.start/poll），所以这里必须管住三件事：
/// 轮询定时器的生命周期、`_finishing` 的一次性收尾、以及避免慢请求
/// 把旧结果写回 state。它们都由这个 Notifier 独占，视图不感知。
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
    // Riverpod 3：provider 在 await 期间被销毁后再用 ref 会抛
    // UnmountedRefException（总览页快速进出就会触发）。这里没有后续 ref
    // 使用，但仍要避免把状态写到已销毁的 notifier 上。
    if (!ref.mounted) {
      return;
    }
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
    // 走到这里时 notifier 可能已经销毁（用户退出翻译页），此时 ref 不可用。
    final repo = _repo;
    await repo.saveTranslatorSettings(TranslatorSettings(
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
      // 输出路径被清空时保留旧值：空的输出路径会让 start 直接失败，
      // 而用户多半只是全选删掉后忘了填。
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
    final presets = state.presets.where((item) => item.id != id).toList();
    var selected = state.selectedPresetId;
    if (selected == id) {
      selected = presets.isEmpty ? '' : presets.first.id;
    }
    state = state.copyWith(presets: presets, selectedPresetId: selected);
    await _persist();
  }

  Future<void> setParameters(TranslatorParameters parameters) async {
    // 范围在这里再收一次：配置页的输入框可能被粘贴进任意数字，
    // 而内核侧对 out-of-range 的并发/分块是直接报错的。
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
    final kernel = _kernel;
    try {
      final formats = await kernel.detectFormats(inputPath);
      if (!ref.mounted) {
        return;
      }
      state = state.copyWith(formats: formats, formatsLoading: false);
    } catch (error) {
      if (!ref.mounted) {
        return;
      }
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
      state = state.copyWith(error: TranslationErrorCode.noFile);
      return false;
    }
    final preset = state.preset;
    if (preset == null) {
      state = state.copyWith(error: TranslationErrorCode.noPreset);
      return false;
    }
    if (file.formatFeatureId.trim().isEmpty) {
      state = state.copyWith(error: TranslationErrorCode.noFormat);
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
    // 先通知内核停，再本地收尾：内核可能已经在写最后一个分块，
    // 直接改 state 会让"已取消"和引擎里的实际状态不一致。
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
          logs: _append(
            state.logs,
            event.level?.isEmpty ?? true ? 'info' : event.level!,
            'raw',
            event.message ?? '',
          ),
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
        _finishDone(
          ok,
          ok ? 'translated' : 'failed',
          ok ? '${event.translated}' : (event.message ?? ''),
        );
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
    // 落盘是后台动作：任务结束的界面状态必须立刻更新，不能等文件 I/O。
    unawaited(_rememberFile());
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

  /// 把刚翻译完的文件提到列表最前面（下次打开总览默认选中它）。
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
    final time = <String>[
      now.hour.toString().padLeft(2, '0'),
      now.minute.toString().padLeft(2, '0'),
      now.second.toString().padLeft(2, '0'),
    ].join(':');
    final next = <TranslationLog>[
      ...logs,
      TranslationLog(time: time, level: level, code: code, detail: detail),
    ];
    // 日志只是诊断窗口，不设上限会让长任务把内存吃到几十 MB，也会让
    // ListView 的 index 计算越来越慢。
    while (next.length > 500) {
      next.removeAt(0);
    }
    return next;
  }
}

final translationProvider =
    NotifierProvider<TranslationNotifier, TranslationState>(
        TranslationNotifier.new);
