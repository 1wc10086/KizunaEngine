import 'dart:async';

import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/app_info.dart';
import 'package:kizuna_engine/core/settings/settings_snapshot.dart';
import 'package:kizuna_engine/core/update/update_service.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../../core/platform/external_links.dart';
import '../../settings/viewmodel/settings_notifier.dart';
import '../../update/view/update_dialogs.dart';
import '../../update/viewmodel/update_notifier.dart';

/// “关于”页：版本、仓库/反馈链接、开源许可、自动更新开关。
///
/// 跳浏览器这种副作用直接走 core 的 openExternalUrl —— 它是纯函数式
/// 平台调用，没有需要单测的状态机，不值得再套一层 viewmodel。
class AboutScreen extends ConsumerWidget {
  const AboutScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).valueOrDefault;
    final checking = ref.watch(
      updateCheckProvider.select((state) => state.checking),
    );
    return Scaffold(
      appBar: AppBar(title: Text(l10n.aboutTitle)),
      body: SettingsList(
        maxWidth: 700,
        sections: <AbstractSettingsSection>[
          const CustomSettingsSection(child: _AboutHeader()),
          SettingsSection(
            tiles: <AbstractSettingsTile>[
              SettingsTile.navigation(
                leading: const Icon(Icons.info_outline),
                title: Text(l10n.aboutVersion),
                value: Text('${l10n.appName} $kAppVersion'),
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.code),
                title: Text(l10n.aboutRepository),
                onPressed: (_) =>
                    unawaited(openExternalUrl(UpdateService.repositoryUrl)),
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.bug_report_outlined),
                title: Text(l10n.aboutFeedback),
                onPressed: (_) =>
                    unawaited(openExternalUrl(UpdateService.issuesUrl)),
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.description_outlined),
                title: Text(l10n.aboutLicenses),
                onPressed: (tileContext) {
                  showLicensePage(
                    context: tileContext,
                    applicationName: l10n.appName,
                    applicationVersion: kAppVersion,
                  );
                },
              ),
            ],
          ),
          SettingsSection(
            title: Text(l10n.aboutSectionUpdate),
            tiles: <AbstractSettingsTile>[
              SettingsTile.switchTile(
                leading: const Icon(Icons.update),
                title: Text(l10n.settingsAutoUpdateCheck),
                initialValue: settings.autoUpdateCheck,
                onToggle: (bool? value) {
                  ref
                      .read(appSettingsProvider.notifier)
                      .setAutoUpdateCheck(value ?? !settings.autoUpdateCheck);
                },
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.system_update_alt),
                title: Text(l10n.aboutCheckUpdate),
                trailing: checking
                    ? const SizedBox(
                        width: 24,
                        height: 24,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.system_update),
                onPressed: (tileContext) async {
                  await showUpdateFlow(tileContext, ref);
                },
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _AboutHeader extends StatelessWidget {
  const _AboutHeader();

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Image.asset(
            'assets/images/ic_launcher.png',
            width: 96,
            height: 96,
            semanticLabel: l10n.aboutIconSemantics,
          ),
          const SizedBox(height: 12),
          Text(l10n.appName, style: Theme.of(context).textTheme.headlineSmall),
        ],
      ),
    );
  }
}
