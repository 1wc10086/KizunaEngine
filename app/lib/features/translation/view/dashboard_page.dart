import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import '../viewmodel/translation_notifier.dart';
import 'session_view.dart';
import 'translation_labels.dart';

/// 翻译总览：选文件 + 选预设 + 开始/停止 + 运行面板。
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
    // 首帧之后才读文件：设置仓库的首次访问会做目录解析与文件 I/O，
    // 放在 build 之前会推迟首帧。
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) {
        return;
      }
      ref.read(translationProvider.notifier).ensureLoaded();
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    // 错误是以码的形式放在 state 里的：显示一次就清掉，否则切走再回来
    // 会重复弹同一个 SnackBar。
    ref.listen(
      translationProvider.select((value) => value.error),
      (previous, next) {
        if (next.isEmpty || next == previous) {
          return;
        }
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(translationErrorText(l10n, next))),
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
              initialValue: state.selectedFile >= 0 &&
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
              initialValue:
                  state.presets.any((item) => item.id == state.selectedPresetId)
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
                            unawaited(notifier.start());
                          },
                    child: Text(l10n.translationStart),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton(
                    onPressed: state.isRunning
                        ? () {
                            unawaited(notifier.cancel());
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
}
