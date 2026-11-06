import 'dart:async';

import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;

import '../../core/settings/settings.dart';
import 'translation_notifier.dart';

class TranslationConfigPage extends ConsumerStatefulWidget {
  const TranslationConfigPage({super.key});

  @override
  ConsumerState<TranslationConfigPage> createState() =>
      _TranslationConfigPageState();
}

class _TranslationConfigPageState
    extends ConsumerState<TranslationConfigPage> {
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
    final l10n = AppLocalizations.of(context)!;
    final state = ref.watch(translationProvider);
    final notifier = ref.read(translationProvider.notifier);
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
              Expanded(child: Text(l10n.translationFiles,
                  style: Theme.of(context).textTheme.titleSmall)),
              IconButton(
                tooltip: l10n.translationAddFile,
                icon: const Icon(Icons.add),
                onPressed: () {
                  _addFileDialog(context);
                },
              ),
            ],
          ),
          if (state.files.isEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: Text(l10n.translationNoFiles),
            ),
          for (var i = 0; i < state.files.length; i++)
            Card(
              child: ListTile(
                selected: i == state.selectedFile,
                title: Text(p.basename(state.files[i].inputPath)),
                subtitle: Text(state.files[i].formatName.isEmpty
                    ? state.files[i].outputPath
                    : '${state.files[i].formatName}\n${state.files[i].outputPath}'),
                isThreeLine: state.files[i].formatName.isNotEmpty,
                onTap: state.isRunning
                    ? null
                    : () {
                        notifier.selectFile(i);
                      },
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    IconButton(
                      tooltip: l10n.translationEditFile,
                      icon: const Icon(Icons.edit),
                      onPressed: state.isRunning
                          ? null
                          : () {
                              _editFileDialog(context, i);
                            },
                    ),
                    IconButton(
                      tooltip: l10n.translationRemoveFile,
                      icon: const Icon(Icons.delete_outline),
                      onPressed: state.isRunning
                          ? null
                          : () {
                              notifier.removeFile(i);
                            },
                    ),
                  ],
                ),
              ),
            ),
          const SizedBox(height: 16),
          Text(l10n.translationParameters,
              style: Theme.of(context).textTheme.titleSmall),
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
              child: Text(_temperature.toStringAsFixed(2),
                  textAlign: TextAlign.end),
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
                targetLanguage: _targetLanguageController.text.trim().isEmpty
                    ? state.parameters.targetLanguage
                    : _targetLanguageController.text.trim(),
                temperature: _temperature,
                glossary: _glossaryController.text,
                contextLines:
                    int.tryParse(_contextLinesController.text.trim()) ??
                        state.parameters.contextLines,
              );
              notifier.setParameters(params);
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
    final l10n = AppLocalizations.of(context)!;
    final controller = TextEditingController();
    final value = await showDialog<String>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: Text(l10n.translationAddFile),
          content: TextField(
            controller: controller,
            autofocus: true,
            decoration:
                InputDecoration(hintText: l10n.translationFilePath),
          ),
          actions: <Widget>[
            TextButton(
              onPressed: () {
                Navigator.of(context).pop();
              },
              child: Text(l10n.commonCancel),
            ),
            FilledButton(
              onPressed: () {
                Navigator.of(context).pop(controller.text);
              },
              child: Text(l10n.commonConfirm),
            ),
          ],
        );
      },
    );
    controller.dispose();
    if (value == null || value.trim().isEmpty || !context.mounted) {
      return;
    }
    await ref.read(translationProvider.notifier).addFile(value);
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.translationSaved)),
      );
    }
  }

  Future<void> _editFileDialog(BuildContext context, int index) async {
    final l10n = AppLocalizations.of(context)!;
    final state = ref.read(translationProvider);
    if (index < 0 || index >= state.files.length) {
      return;
    }
    final file = state.files[index];
    final notifier = ref.read(translationProvider.notifier);
    await notifier.loadFormats(file.inputPath);
    if (!context.mounted) {
      return;
    }
    final outputController = TextEditingController(text: file.outputPath);
    String? formatId =
        file.formatFeatureId.isEmpty ? null : file.formatFeatureId;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return StatefulBuilder(
          builder: (context, setDialogState) {
            return Consumer(
              builder: (context, ref, _) {
            final formats = ref.watch(
                translationProvider.select((value) => value.formats));
            final loading = ref.watch(
                translationProvider.select((value) => value.formatsLoading));
            return AlertDialog(
              title: Text(l10n.translationEditFile),
              content: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    TextField(
                      controller: outputController,
                      decoration: InputDecoration(
                          labelText: l10n.translationOutputPath),
                    ),
                    const SizedBox(height: 12),
                    DropdownButtonFormField<String>(
                      value: formats.any((item) => item.featureId == formatId)
                          ? formatId
                          : null,
                      hint: Text(loading
                          ? l10n.commonLoading
                          : l10n.translationFormat),
                      items: <DropdownMenuItem<String>>[
                        for (final format in formats)
                          DropdownMenuItem<String>(
                            value: format.featureId,
                            child: Text(format.name),
                          ),
                      ],
                      onChanged: (value) {
                        setDialogState(() {
                          formatId = value;
                        });
                      },
                    ),
                  ],
                ),
              ),
              actions: <Widget>[
                TextButton(
                  onPressed: () {
                    Navigator.of(context).pop(false);
                  },
                  child: Text(l10n.commonCancel),
                ),
                FilledButton(
                  onPressed: () {
                    Navigator.of(context).pop(true);
                  },
                  child: Text(l10n.commonConfirm),
                ),
              ],
            );
          },
        );
          },
        );
      },
    );
    final output = outputController.text;
    outputController.dispose();
    if (confirmed != true || !context.mounted) {
      return;
    }
    String formatName = file.formatName;
    if (formatId != null) {
      final formats = ref.read(translationProvider).formats;
      for (final format in formats) {
        if (format.featureId == formatId) {
          formatName = format.name;
        }
      }
    }
    await notifier.updateFile(
      index,
      outputPath: output,
      formatFeatureId: formatId ?? file.formatFeatureId,
      formatName: formatName,
    );
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(l10n.translationSaved)),
      );
    }
  }
}
