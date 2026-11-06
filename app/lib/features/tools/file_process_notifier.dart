import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/bridge/kernel_bridge.dart';
import '../../core/bridge/kernel_provider.dart';

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
    try {
      final bridge = ref.read(kernelBridgeProvider);
      final detected = await bridge.detect(trimmed);
      if (gen != _detectGen) {
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
      if (gen != _detectGen) {
        return;
      }
      state = state.copyWith(errorMessage: '$error');
    }
  }

  Future<bool> start() async {
    final candidate = state.selected;
    final input = state.inputPath.trim();
    final output = state.outputPath.trim();
    if (candidate == null || input.isEmpty || output.isEmpty) {
      return false;
    }
    state = state.copyWith(isBusy: true, errorMessage: '', clearResult: true);
    try {
      final bridge = ref.read(kernelBridgeProvider);
      final result = await bridge.process(
        feature: candidate.featureId,
        action: candidate.action,
        input: input,
        output: output,
      );
      state = state.copyWith(isBusy: false, result: result);
      return true;
    } catch (error) {
      state = state.copyWith(isBusy: false, errorMessage: '$error');
      return false;
    }
  }
}

final fileProcessProvider =
    NotifierProvider.autoDispose<FileProcessNotifier, FileProcessState>(
        FileProcessNotifier.new);
