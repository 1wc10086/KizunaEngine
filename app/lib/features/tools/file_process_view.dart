import 'dart:async';

import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'file_process_notifier.dart';

class FileProcessView extends ConsumerStatefulWidget {
  const FileProcessView({super.key});

  @override
  ConsumerState<FileProcessView> createState() => _FileProcessViewState();
}

class _FileProcessViewState extends ConsumerState<FileProcessView> {
  late final TextEditingController _inputController;
  late final TextEditingController _outputController;
  bool _syncingOutput = false;

  @override
  void initState() {
    super.initState();
    _inputController = TextEditingController();
    _outputController = TextEditingController();
  }

  @override
  void dispose() {
    _inputController.dispose();
    _outputController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    ref.listen(
      fileProcessProvider.select((value) => value.outputPath),
      (previous, next) {
        if (next != previous && _outputController.text != next) {
          _syncingOutput = true;
          _outputController.text = next;
          _syncingOutput = false;
        }
      },
    );
    final state = ref.watch(fileProcessProvider);
    final notifier = ref.read(fileProcessProvider.notifier);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: <Widget>[
        TextField(
          controller: _inputController,
          decoration: InputDecoration(
            labelText: l10n.toolsInput,
            hintText: l10n.toolsInputHint,
          ),
          onChanged: notifier.setInput,
        ),
        const SizedBox(height: 12),
        if (state.detectMessage.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(state.detectMessage),
          ),
        if (state.errorMessage.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(
              state.errorMessage,
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        DropdownButtonFormField<int>(
          value: state.selectedIndex >= 0 &&
                  state.selectedIndex < state.candidates.length
              ? state.selectedIndex
              : null,
          hint: Text(state.candidates.isEmpty
              ? l10n.toolsNoCandidates
              : l10n.toolsSelectCandidate),
          items: <DropdownMenuItem<int>>[
            for (var i = 0; i < state.candidates.length; i++)
              DropdownMenuItem<int>(
                value: i,
                child: Text(state.candidates[i].label),
              ),
          ],
          onChanged: state.isBusy
              ? null
              : (index) {
                  if (index != null) {
                    notifier.selectCandidate(index);
                  }
                },
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _outputController,
          decoration: InputDecoration(labelText: l10n.toolsOutput),
          onChanged: (value) {
            if (!_syncingOutput) {
              notifier.setOutput(value);
            }
          },
        ),
        const SizedBox(height: 16),
        FilledButton(
          onPressed: state.isBusy
              ? null
              : () async {
                  final ok = await notifier.start();
                  if (!context.mounted) {
                    return;
                  }
                  if (!ok) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(content: Text(l10n.toolsNeedInput)),
                    );
                    return;
                  }
                  final result =
                      ref.read(fileProcessProvider).result;
                  if (result != null) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: Text(
                          '${l10n.toolsDone}: ${result.elapsedMs} ${l10n.toolsMillis}',
                        ),
                      ),
                    );
                  }
                },
          child: state.isBusy
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(l10n.toolsProcess),
        ),
        if (state.result != null) ...<Widget>[
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Text(
                    state.result!.summary.isEmpty
                        ? l10n.toolsDone
                        : state.result!.summary,
                    style: Theme.of(context).textTheme.titleSmall,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '${l10n.toolsInput}: ${state.result!.inputPath}',
                  ),
                  Text(
                    '${l10n.toolsOutput}: ${state.result!.outputPath}',
                  ),
                  Text(
                    '${l10n.toolsBytes}: ${state.result!.bytesIn} ${l10n.toolsArrow} ${state.result!.bytesOut}',
                  ),
                  Text(
                    '${l10n.toolsElapsed}: ${state.result!.elapsedMs} ${l10n.toolsMillis}',
                  ),
                  for (final entry in state.result!.details.entries)
                    Text('${entry.key}: ${entry.value}'),
                ],
              ),
            ),
          ),
        ],
      ],
    );
  }
}
