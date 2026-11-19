import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import '../../../core/bridge/kernel_bridge.dart';
import '../../../core/settings/settings.dart';
import '../viewmodel/translation_notifier.dart';

/// 翻译配置页：文件列表 + 翻译参数。
///
/// 控制器只在首次加载时同步一次（_paramsInit）：把 state 回灌到输入框
/// 会在用户打字打到一半时覆盖光标位置。之后 state 只由"保存"按钮驱动。
class TranslationConfigPage extends ConsumerStatefulWidget {
  const TranslationConfigPage({super.key});

  @override
  ConsumerState<TranslationConfigPage> createState() =>
      _TranslationConfigPageState();
}

class _TranslationConfigPageState extends ConsumerState<TranslationConfigPage> {
  late final TextEditingController _concurrencyController;
  late final TextEditingController _chunkSizeController;
  late final TextEditingController _retryController;
  late final TextEditingController _targetLanguageController;
  late final TextEditingController _glossaryController;
  late final TextEditingController _contextLinesController;
  double _temperature = 0.7;
  bool _paramsInit = false;

  @override
  void initState() {
    super.initState();
    _concurrencyController = TextEditingController();
    _chunkSizeController = TextEditingController();
    _retryController = TextEditingController();
    _targetLanguageController = TextEditingController();
    _glossaryController = TextEditingController();
    _contextLinesController = TextEditingController();
  }

  @override
  void dispose() {
    _concurrencyController.dispose();
    _chunkSizeController.dispose();
    _retryController.dispose();
    _targetLanguageController.dispose();
    _glossaryController.dispose();
    _contextLinesController.dispose();
    super.dispose();
  }

  void _syncParams(TranslatorParameters params) {
    if (_paramsInit) {
      return;
    }
    _paramsInit = true;
    _concurrencyController.text = '${params.concurrency}';
    _chunkSizeController.text = '${params.chunkSize}';
    _retryController.text = '${params.japaneseRetryLimit}';
    _targetLanguageController.text = params.targetLanguage;
    _glossaryController.text = params.glossary;
    _contextLinesController.text = '${params.contextLines}';
    _temperature = params.temperature;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final state = ref.watch(translationProvider);
    final theme = Theme.of(context);
    if (state.loaded) {
      _syncParams(state.parameters);
    }
    return Scaffold(
      appBar: AppBar(title: Text(l10n.translationConfig)),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: <Widget>[
          Row(
            children: <Widget>[
              Expanded(
                child: Text(
                  l10n.translationFiles,
                  style: theme.textTheme.titleSmall,
                ),
              ),
              IconButton(
                tooltip: l10n.translationAddFile,
                icon: const Icon(Icons.add),
                onPressed: () => unawaited(_addFileDialog(context)),
              ),
            ],
          ),
          if (state.files.isEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Text(l10n.translationNoFiles),
            ),
          for (var i = 0; i < state.files.length; i++)
            _FileTile(index: i, file: state.files[i], running: state.isRunning),
          const SizedBox(height: 16),
          Text(l10n.translationParameters, style: theme.textTheme.titleSmall),
          const SizedBox(height: 8),
          TextField(
            controller: _concurrencyController,
            keyboardType: TextInputType.number,
            decoration:
                InputDecoration(labelText: l10n.translationConcurrency),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _chunkSizeController,
            keyboardType: TextInputType.number,
            decoration: InputDecoration(labelText: l10n.translationChunkSize),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _retryController,
            keyboardType: TextInputType.number,
            decoration:
                InputDecoration(labelText: l10n.translationRetryLimit),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _targetLanguageController,
            decoration:
                InputDecoration(labelText: l10n.translationTargetLanguage),
          ),
          const SizedBox(height: 8),
          ListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(l10n.translationTemperature),
            trailing: SizedBox(
              width: 48,
              child: Text(
                _temperature.toStringAsFixed(2),
                textAlign: TextAlign.end,
              ),
            ),
            subtitle: Slider(
              value: _temperature.clamp(0.0, 2.0).toDouble(),
              max: 2,
              divisions: 40,
              onChanged: (value) {
                setState(() {
                  _temperature = value;
                });
              },
            ),
          ),
          TextField(
            controller: _glossaryController,
            maxLines: 4,
            decoration: InputDecoration(labelText: l10n.translationGlossary),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _contextLinesController,
            keyboardType: TextInputType.number,
            decoration:
                InputDecoration(labelText: l10n.translationContextLines),
          ),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: () {
              final params = TranslatorParameters(
                concurrency:
                    int.tryParse(_concurrencyController.text.trim()) ??
                        state.parameters.concurrency,
                chunkSize: int.tryParse(_chunkSizeController.text.trim()) ??
                    state.parameters.chunkSize,
                japaneseRetryLimit:
                    int.tryParse(_retryController.text.trim()) ??
                        state.parameters.japaneseRetryLimit,
                // 目标语言留空没有意义（提示词会变成"翻译成"），
                // 空输入保留旧值。
                targetLanguage: _targetLanguageController.text.trim().isEmpty
                    ? state.parameters.targetLanguage
                    : _targetLanguageController.text.trim(),
                temperature: _temperature,
                glossary: _glossaryController.text,
                contextLines:
                    int.tryParse(_contextLinesController.text.trim()) ??
                        state.parameters.contextLines,
              );
              unawaited(
                ref.read(translationProvider.notifier).setParameters(params),
              );
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text(l10n.translationSaved)),
              );
            },
            child: Text(l10n.commonConfirm),
          ),
        ],
      ),
    );
  }

  Future<void> _addFileDialog(BuildContext context) async {
    final l10n = AppLocalizations.of(context);
    final controller = TextEditingController();
    final value = await showDialog<String>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: Text(l10n.translationAddFile),
          content: TextField(
            controller: controller,
            autofocus: true,
            decoration: InputDecoration(hintText: l10n.translationFilePath),
          ),
          actions: <Widget>[
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: Text(l10n.commonCancel),
            ),
            FilledButton(
              onPressed: () =>
                  Navigator.of(dialogContext).pop(controller.text),
              child: Text(l10n.commonConfirm),
            ),
          ],
        );
      },
    );
    controller.dispose();
    if (value == null || value.trim().isEmpty || !mounted) {
      return;
    }
    await ref.read(translationProvider.notifier).addFile(value);
    if (!mounted) {
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(l10n.translationSaved)),
    );
  }
}

class _FileTile extends ConsumerWidget {
  final int index;
  final TranslatorStoredFile file;
  final bool running;

  const _FileTile({
    required this.index,
    required this.file,
    required this.running,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final selected =
        ref.watch(translationProvider.select((value) => value.selectedFile)) ==
            index;
    final notifier = ref.read(translationProvider.notifier);
    return Card(
      child: ListTile(
        selected: selected,
        title: Text(p.basename(file.inputPath)),
        subtitle: Text(
          file.formatName.isEmpty
              ? file.outputPath
              : '${file.formatName}\n${file.outputPath}',
        ),
        isThreeLine: file.formatName.isNotEmpty,
        onTap: running ? null : () => notifier.selectFile(index),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            IconButton(
              tooltip: l10n.translationEditFile,
              icon: const Icon(Icons.edit),
              onPressed: running
                  ? null
                  : () => unawaited(_editFileDialog(context, ref, index)),
            ),
            IconButton(
              tooltip: l10n.translationRemoveFile,
              icon: const Icon(Icons.delete_outline),
              onPressed:
                  running ? null : () => unawaited(notifier.removeFile(index)),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _editFileDialog(
    BuildContext context,
    WidgetRef ref,
    int index,
  ) async {
    final l10n = AppLocalizations.of(context);
    final notifier = ref.read(translationProvider.notifier);
    // 先拉一次格式列表：输入文件的格式由内核按内容嗅探，不同品牌
    // 支持的格式集合不同，不能写死成常量。
    await notifier.loadFormats(file.inputPath);
    if (!context.mounted) {
      return;
    }
    final picked = await showDialog<_FileEditResult>(
      context: context,
      builder: (_) => _FileEditDialog(file: file),
    );
    if (picked == null || !context.mounted) {
      return;
    }
    await notifier.updateFile(
      index,
      outputPath: picked.outputPath,
      formatFeatureId: picked.format.featureId,
      formatName: picked.format.name,
    );
    if (!context.mounted) {
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(l10n.translationSaved)),
    );
  }
}

class _FileEditResult {
  final String outputPath;
  final KernelTranslateFormat format;

  const _FileEditResult({required this.outputPath, required this.format});
}

class _FileEditDialog extends ConsumerStatefulWidget {
  final TranslatorStoredFile file;

  const _FileEditDialog({required this.file});

  @override
  ConsumerState<_FileEditDialog> createState() => _FileEditDialogState();
}

class _FileEditDialogState extends ConsumerState<_FileEditDialog> {
  late final TextEditingController _outputController =
      TextEditingController(text: widget.file.outputPath);
  late String? _formatId = widget.file.formatFeatureId.isEmpty
      ? null
      : widget.file.formatFeatureId;

  @override
  void dispose() {
    _outputController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final formats =
        ref.watch(translationProvider.select((value) => value.formats));
    final loading =
        ref.watch(translationProvider.select((value) => value.formatsLoading));
    return AlertDialog(
      title: Text(l10n.translationEditFile),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            TextField(
              controller: _outputController,
              decoration:
                  InputDecoration(labelText: l10n.translationOutputPath),
            ),
            const SizedBox(height: 12),
            DropdownButtonFormField<String>(
              initialValue: formats.any((item) => item.featureId == _formatId)
                  ? _formatId
                  : null,
              hint: Text(
                loading ? l10n.commonLoading : l10n.translationFormat,
              ),
              items: <DropdownMenuItem<String>>[
                for (final format in formats)
                  DropdownMenuItem<String>(
                    value: format.featureId,
                    child: Text(format.name),
                  ),
              ],
              onChanged: (value) {
                setState(() {
                  _formatId = value;
                });
              },
            ),
          ],
        ),
      ),
      actions: <Widget>[
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(l10n.commonCancel),
        ),
        FilledButton(
          onPressed: () {
            // 手写查找而不是 firstOrNull：后者在 dart:collection 的扩展里，
            // 本文件没有直接 import，靠传递依赖拿到会随 SDK 调整而断。
            KernelTranslateFormat? format;
            for (final item in formats) {
              if (item.featureId == _formatId) {
                format = item;
                break;
              }
            }
            if (format == null) {
              return;
            }
            Navigator.of(context).pop(
              _FileEditResult(
                outputPath: _outputController.text,
                format: format,
              ),
            );
          },
          child: Text(l10n.commonConfirm),
        ),
      ],
    );
  }
}
