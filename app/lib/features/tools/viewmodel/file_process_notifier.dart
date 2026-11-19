import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/bridge/kernel_bridge.dart';
import '../../../core/bridge/kernel_provider.dart';

/// 文件处理页的状态。
///
/// 检测是"边输入边探测"：输入框每次变更都会走一次 400ms 防抖，然后把
/// 候选动作（解包/转码/重命名…）喂给下拉框。候选来自内核，不是本地枚举——
/// 每个品牌的可处理项都不一样。
class FileProcessState {
  final String inputPath;
  final String outputPath;
  final List<KernelDetectCandidate> candidates;
  final int selectedIndex;
  final String detectMessage;
  final String errorMessage;
  final bool isBusy;
  final KernelProcessResult? result;

  const FileProcessState({
    this.inputPath = '',
    this.outputPath = '',
    this.candidates = const <KernelDetectCandidate>[],
    this.selectedIndex = -1,
    this.detectMessage = '',
    this.errorMessage = '',
    this.isBusy = false,
    this.result,
  });

  KernelDetectCandidate? get selected =>
      selectedIndex >= 0 && selectedIndex < candidates.length
          ? candidates[selectedIndex]
          : null;

  /// 是否可以点"处理"：有候选动作、输入输出都非空、且没有正在跑。
  bool get canStart =>
      !isBusy &&
      selected != null &&
      inputPath.trim().isNotEmpty &&
      outputPath.trim().isNotEmpty;

  FileProcessState copyWith({
    String? inputPath,
    String? outputPath,
    List<KernelDetectCandidate>? candidates,
    int? selectedIndex,
    String? detectMessage,
    String? errorMessage,
    bool? isBusy,
    KernelProcessResult? result,
    bool clearResult = false,
  }) {
    return FileProcessState(
      inputPath: inputPath ?? this.inputPath,
      outputPath: outputPath ?? this.outputPath,
      candidates: candidates ?? this.candidates,
      selectedIndex: selectedIndex ?? this.selectedIndex,
      detectMessage: detectMessage ?? this.detectMessage,
      errorMessage: errorMessage ?? this.errorMessage,
      isBusy: isBusy ?? this.isBusy,
      result: clearResult ? null : (result ?? this.result),
    );
  }
}

class FileProcessNotifier extends Notifier<FileProcessState> {
  Timer? _debounce;
  // 代数：防抖只是减少请求频率，慢请求仍可能乱序返回。带上代号之后
  // 只有"最后一次输入"对应的响应会被采纳，避免候选列表跳回旧结果。
  int _detectGen = 0;

  @override
  FileProcessState build() {
    ref.onDispose(() {
      _debounce?.cancel();
      _debounce = null;
    });
    return const FileProcessState();
  }

  void setInput(String value) {
    _debounce?.cancel();
    state = state.copyWith(inputPath: value, clearResult: true);
    _debounce = Timer(const Duration(milliseconds: 400), () {
      _detect(value);
    });
  }

  void setOutput(String value) {
    state = state.copyWith(outputPath: value, clearResult: true);
  }

  void selectCandidate(int index) {
    if (index < 0 || index >= state.candidates.length) {
      return;
    }
    final candidate = state.candidates[index];
    // 换动作时把输出路径重置成该动作建议的目标，否则会把上一个动作的
    // 扩展名留在输入框里（解包 vs 转码的输出类型完全不同）。
    state = state.copyWith(
      selectedIndex: index,
      outputPath: candidate.outputPath,
      clearResult: true,
    );
  }

  Future<void> _detect(String raw) async {
    final gen = ++_detectGen;
    final trimmed = raw.trim();
    if (trimmed.isEmpty) {
      state = state.copyWith(
        candidates: const <KernelDetectCandidate>[],
        selectedIndex: -1,
        detectMessage: '',
        errorMessage: '',
      );
      return;
    }
    // autoDispose：用户离开工具页后这次探测可能还在飞。先把 bridge 取出来，
    // 回来之后再确认 provider 仍然存活（Riverpod 3 里对已销毁的 ref 操作会抛）。
    final bridge = ref.read(kernelBridgeProvider);
    try {
      final detected = await bridge.detect(trimmed);
      if (gen != _detectGen || !ref.mounted) {
        return;
      }
      final items = detected.candidates;
      state = state.copyWith(
        candidates: items,
        selectedIndex: items.isEmpty ? -1 : 0,
        outputPath: detected.outputPath,
        detectMessage: detected.message,
        errorMessage: '',
      );
    } catch (error) {
      if (gen != _detectGen || !ref.mounted) {
        return;
      }
      state = state.copyWith(errorMessage: '$error');
    }
  }

  /// 执行处理；返回 null 表示输入不完整（视图据此提示），
  /// 否则返回内核的执行结果（失败时 errorMessage 已写入 state）。
  Future<KernelProcessResult?> start() async {
    final candidate = state.selected;
    final input = state.inputPath.trim();
    final output = state.outputPath.trim();
    if (candidate == null || input.isEmpty || output.isEmpty) {
      return null;
    }
    state = state.copyWith(isBusy: true, errorMessage: '', clearResult: true);
    final bridge = ref.read(kernelBridgeProvider);
    try {
      final result = await bridge.process(
        feature: candidate.featureId,
        action: candidate.action,
        input: input,
        output: output,
      );
      if (!ref.mounted) {
        return result;
      }
      state = state.copyWith(isBusy: false, result: result);
      return result;
    } catch (error) {
      if (!ref.mounted) {
        return null;
      }
      state = state.copyWith(isBusy: false, errorMessage: '$error');
      return null;
    }
  }
}

final fileProcessProvider =
    NotifierProvider.autoDispose<FileProcessNotifier, FileProcessState>(
        FileProcessNotifier.new);
