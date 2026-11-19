import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../viewmodel/translation_notifier.dart';
import 'translation_labels.dart';

/// 运行面板：进度条、统计行、以及可滚动的日志。
///
/// 单独拆成 widget 而不是留在 dashboard 里，是因为它订阅的是高频变化的
/// 进度字段（250ms 一次），让 dashboard 的另一半（文件/预设下拉框）
/// 跟着重建没有意义。
class TranslationSessionView extends ConsumerWidget {
  const TranslationSessionView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final state = ref.watch(translationProvider);
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        LinearProgressIndicator(
          value: state.isRunning || state.progress > 0 ? state.progress : null,
        ),
        const SizedBox(height: 8),
        Text(
          translationProgressText(l10n, state),
          style: theme.textTheme.bodyMedium,
        ),
        const SizedBox(height: 4),
        Text(
          l10n.translationStats(
            state.promptTokens,
            state.completionTokens,
            state.requests,
            state.failed,
            state.elapsedMs,
            state.unitsPerSecond.toStringAsFixed(1),
          ),
          style: theme.textTheme.bodySmall,
        ),
        const SizedBox(height: 8),
        Expanded(
          child: Container(
            decoration: BoxDecoration(
              border: Border.all(color: theme.colorScheme.outlineVariant),
              borderRadius: BorderRadius.circular(8),
            ),
            child: state.logs.isEmpty
                ? Center(child: Text(l10n.commonEmpty))
                : ListView.builder(
                    padding: const EdgeInsets.all(8),
                    itemCount: state.logs.length,
                    itemBuilder: (context, index) {
                      final log = state.logs[index];
                      return Padding(
                        padding: const EdgeInsets.symmetric(vertical: 2),
                        child: Text(
                          '${log.time} ${translationLogText(l10n, log)}',
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: log.level == 'error'
                                ? theme.colorScheme.error
                                : null,
                          ),
                        ),
                      );
                    },
                  ),
          ),
        ),
      ],
    );
  }
}
