import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/platform/platform_info.dart';
import '../../core/update/update_controller.dart';
import '../../core/update/update_service.dart';
import '../settings/settings_notifier.dart';

Future<void> showUpdateFlow(
  BuildContext context,
  WidgetRef ref, {
  bool silent = false,
}) async {
  final settings = await ref.read(appSettingsProvider.future);
  if (!context.mounted) {
    return;
  }
  if (silent && !settings.autoUpdateCheck) {
    return;
  }
  final release = await ref.read(updateCheckProvider.notifier).check();
  if (!context.mounted) {
    return;
  }
  final l10n = AppLocalizations.of(context);
  if (release == null) {
    if (!silent) {
      final error = ref.read(updateCheckProvider).error;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            error.isEmpty ? l10n.aboutUpToDate : l10n.aboutCheckFailed,
          ),
        ),
      );
    }
    return;
  }
  await showReleaseDialog(context, ref, release);
}

Future<void> showReleaseDialog(
  BuildContext context,
  WidgetRef ref,
  ReleaseInfo release,
) async {
  final l10n = AppLocalizations.of(context);
  final action = await showDialog<bool>(
    context: context,
    builder: (dialogContext) => AlertDialog(
      title: Text('${l10n.updateDialogTitle} ${release.version}'),
      content: SingleChildScrollView(
        child: Text(release.notes.isEmpty ? release.title : release.notes),
      ),
      actions: <Widget>[
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(false),
          child: Text(l10n.aboutLater),
        ),
        FilledButton(
          onPressed: () => Navigator.of(dialogContext).pop(true),
          child: Text(l10n.aboutDownload),
        ),
      ],
    ),
  );
  if (action != true || !context.mounted) {
    return;
  }
  final navigator = Navigator.of(context, rootNavigator: true);
  final messenger = ScaffoldMessenger.of(context);
  if (!isAndroidPlatform) {
    await ref.read(updateCheckProvider.notifier).downloadAndInstall(release);
    final error = ref.read(updateCheckProvider).error;
    messenger.showSnackBar(
      SnackBar(
        content: Text(
          error.isEmpty ? l10n.aboutBrowserOpened : l10n.aboutInstallFailed,
        ),
      ),
    );
    return;
  }
  showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => const PopScope(
      canPop: false,
      child: _DownloadProgressDialog(),
    ),
  );
  await ref.read(updateCheckProvider.notifier).downloadAndInstall(release);
  navigator.pop();
  final updateState = ref.read(updateCheckProvider);
  if (updateState.error.isNotEmpty) {
    final label = updateState.progress == 1
        ? l10n.aboutInstallFailed
        : l10n.aboutDownloadFailed;
    messenger.showSnackBar(
      SnackBar(content: Text('$label: ${updateState.error}')),
    );
  }
}

class _DownloadProgressDialog extends ConsumerWidget {
  const _DownloadProgressDialog();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final progress =
        ref.watch(updateCheckProvider.select((state) => state.progress));
    return AlertDialog(
      title: Text(l10n.updateDownloadTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          LinearProgressIndicator(value: progress),
          const SizedBox(height: 16),
          Text(l10n.updateInstallHint),
        ],
      ),
    );
  }
}
