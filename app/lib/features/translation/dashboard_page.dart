import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import 'session_view.dart';
import 'translation_notifier.dart';

class TranslationDashboardPage extends ConsumerStatefulWidget {
  const TranslationDashboardPage({super.key});

  @override
  ConsumerState<TranslationDashboardPage> createState() =>
      _TranslationDashboardPageState();
}

class _TranslationDashboardPageState
    extends ConsumerState<TranslationDashboardPage> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(translationProvider.notifier).ensureLoaded();
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    ref.listen(
      translationProvider.select((value) => value.error),
      (previous, next) {
        if (next == null || next.isEmpty || next == previous) {
          return;
        }
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(_errorText(l10n, next))),
        );
        ref.read(translationProvider.notifier).clearError();
      },
    );
    final state = ref.watch(translationProvider);
    final notifier = ref.read(translationProvider.notifier);
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.translationDashboard),
        actions: <Widget>[
          IconButton(
            tooltip: l10n.translationConfig,
            icon: const Icon(Icons.folder_outlined),
            onPressed: () => context.push('/translation/config'),
          ),
          IconButton(
            tooltip: l10n.translationPresets,
            icon: const Icon(Icons.smart_toy_outlined),
            onPressed: () => context.push('/translation/presets'),
          ),
        ],
      ),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            DropdownButtonFormField<int>(
              value: state.selectedFile >= 0 &&
                      state.selectedFile < state.files.length
                  ? state.selectedFile
                  : null,
              hint: Text(l10n.translationNoFiles),
              items: <DropdownMenuItem<int>>[
                for (var i = 0; i < state.files.length; i++)
                  DropdownMenuItem<int>(
                    value: i,
                    child: Text(p.basename(state.files[i].inputPath)),
                  ),
              ],
              onChanged: state.isRunning
                  ? null
                  : (index) {
                      if (index != null) {
                        notifier.selectFile(index);
                      }
                    },
            ),
            const SizedBox(height: 12),
            DropdownButtonFormField<String>(
              value: state.presets.any((item) => item.id == state.selectedPresetId)
                  ? state.selectedPresetId
                  : null,
              hint: Text(l10n.translationNoPresets),
              items: <DropdownMenuItem<String>>[
                for (final preset in state.presets)
                  DropdownMenuItem<String>(
                    value: preset.id,
                    child: Text(preset.name),
                  ),
              ],
              onChanged: state.isRunning
                  ? null
                  : (id) {
                      if (id != null) {
                        notifier.selectPreset(id);
                      }
                    },
            ),
            const SizedBox(height: 12),
            Row(
              children: <Widget>[
                Expanded(
                  child: FilledButton(
                    onPressed: state.isRunning
                        ? null
                        : () {
                            notifier.start();
                          },
                    child: Text(l10n.translationStart),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton(
                    onPressed: state.isRunning
                        ? () {
                            notifier.cancel();
                          }
                        : null,
                    child: Text(l10n.translationStop),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            const Expanded(child: TranslationSessionView()),
          ],
        ),
      ),
    );
  }

  String _errorText(AppLocalizations l10n, String code) {
    switch (code) {
      case 'no-file':
        return l10n.translationAddFileFirst;
      case 'no-preset':
        return l10n.translationAddPresetFirst;
      case 'no-format':
        return l10n.translationSelectFormatFirst;
      default:
        return code;
    }
  }
}
