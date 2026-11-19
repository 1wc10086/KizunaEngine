import '../../../core/bridge/kernel_bridge.dart';
import '../../../core/settings/settings.dart';

/// 翻译任务的一次运行状态。
enum TranslationJobState { idle, running, done, failed, canceled }

/// 翻译流水线上报的一行日志。
///
/// `code` 是"可翻译的事件类型"（started/finished/residual…），`detail`
/// 是它带的参数。视图负责把 code 翻成当前语言的文案：把已经拼好的字符串
/// 存进 state 就再也跟不了语言切换，而且日志会随着运行时间越积越多。
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

/// 翻译页的完整状态。
///
/// 一份 state 同时服务三个页面（总览/配置/预设）：它们共享同一组
/// 文件、预设、参数与运行进度，拆成三个 provider 只会让"在配置页改了
/// 参数、切回总览没生效"这类问题反复出现。
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

  /// 错误码而不是错误文案：`no-file` / `no-preset` / `no-format` 或内核异常。
  /// 视图按 code 取本地化文案，避免中英文混排。
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
