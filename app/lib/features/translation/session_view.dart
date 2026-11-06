import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'translation_notifier.dart';

String translationLogText(AppLocalizations l10n, TranslationLog log) {
  switch (log.code) {
    case 'started':
      return l10n.translationLogStarted(log.detail);
    case 'canceling':
      return l10n.translationLogCanceling;
    case 'canceled':
      return l10n.translationLogCanceled;
    case 'finished':
      return l10n.translationLogFinished;
    case 'translated':
      return l10n.translationLogTranslated(log.detail);
    case 'failed':
      return log.detail.isEmpty
          ? l10n.translationLogFailed
          : '${l10n.translationLogFailed}: ${log.detail}';
    case 'residual':
      return l10n.translationLogResidual(log.detail);
    default:
      return log.detail;
  }
}

class TranslationSessionView extends ConsumerWidget {
  const TranslationSessionView({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context)!;
    final state = ref.watch(translationProvider);
    final progressText = state.progressText.isEmpty
        ? l10n.translationNotStarted
        : (state.progressText == 'starting'
            ? l10n.translationStarting
            : state.progressText);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        LinearProgressIndicator(
          value: state.isRunning || state.progress > 0 ? state.progress : null,
        ),
        const SizedBox(height: 8),
        Text(progressText, style: Theme.of(context).textTheme.bodyMedium),
        const SizedBox(height: 4),
        Text(
          l10n.translationStats(
              state.promptTokens,
              state.completionTokens,
              state.requests,
              state.failed,
              state.elapsedMs,
              state.unitsPerSecond.toStringAsFixed(1)),
          style: Theme.of(context).textTheme.bodySmall,
        ),
        const SizedBox(height: 8),
        Expanded(
          child: Container(
            decoration: BoxDecoration(
              border: Border.all(
                  color: Theme.of(context).colorScheme.outlineVariant),
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
                          style: Theme.of(context)
                              .textTheme
                              .bodySmall
                              ?.copyWith(
                                color: log.level == 'error'
                                    ? Theme.of(context).colorScheme.error
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
