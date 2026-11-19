import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../../core/platform/platform_info.dart';
import '../../../core/update/update_service.dart';
import '../viewmodel/update_flow.dart';
import '../viewmodel/update_notifier.dart';

/// 启动期/手动触发的更新流程入口。
///
/// [silent] 用于主页面的自动检查：不弹"已是最新"或"检查失败"，
/// 只有真的发现新版本才打断用户。
Future<void> showUpdateFlow(
  BuildContext context,
  WidgetRef ref, {
  bool silent = false,
}) async {
  final result = await updateFlow.check(ref, silent: silent);
  if (!context.mounted) {
    return;
  }
  final l10n = AppLocalizations.of(context);
  // 静默模式下除了"发现新版本"之外一律不打扰玩家。
  final hint = switch (result.outcome) {
    UpdateFlowOutcome.upToDate when !silent => l10n.aboutUpToDate,
    UpdateFlowOutcome.checkFailed when !silent => l10n.aboutCheckFailed,
    _ => null,
  };
  if (hint != null) {
    _snack(context, hint);
  }
  final release = result.release;
  if (result.outcome == UpdateFlowOutcome.releaseFound &&
      release != null &&
      context.mounted) {
    await showReleaseDialog(context, ref, release);
  }
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
      title: Text(l10n.updateDialogTitleVersion(release.version)),
      content: SingleChildScrollView(
        // release notes 常常为空（只写了 tag）；退回发布标题，避免空白对话框。
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
  // Android 的下载+安装是长流程（下载几百 MB 后调系统安装器），用不可返回的
  // 进度对话框挡住误触；其他平台只是打开 release 页面，弹进度没有意义。
  final showProgress = isAndroidPlatform;
  if (showProgress) {
    // showDialog 内部是同步 push，返回时路由已在栈上，所以稍后的
    // navigator.pop() 一定弹的是这个进度框而不是下面的页面。
    unawaited(
      showDialog<void>(
        context: context,
        barrierDismissible: false,
        builder: (_) => const PopScope(
          canPop: false,
          child: DownloadProgressDialog(),
        ),
      ),
    );
  }
  final result = await updateFlow.download(ref, release);
  if (showProgress && navigator.canPop()) {
    navigator.pop();
  }
  if (!context.mounted) {
    return;
  }
  final message = switch (result.outcome) {
    UpdateFlowOutcome.browserOpened => l10n.aboutBrowserOpened,
    UpdateFlowOutcome.installFailed => l10n.updateErrorDetail(
        l10n.aboutInstallFailed,
        result.error,
      ),
    UpdateFlowOutcome.downloadFailed => l10n.updateErrorDetail(
        l10n.aboutDownloadFailed,
        result.error,
      ),
    // installStarted 由系统安装器接管界面，再弹 SnackBar 只会盖在安装页上。
    _ => null,
  };
  if (message != null) {
    messenger.showSnackBar(SnackBar(content: Text(message)));
  }
}

/// 下载进度。订阅 progress 而不是整个 UpdateState，避免 checking/error
/// 字段变化也重建进度条。
class DownloadProgressDialog extends ConsumerWidget {
  const DownloadProgressDialog({super.key});

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

void _snack(BuildContext context, String message) {
  ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
}
