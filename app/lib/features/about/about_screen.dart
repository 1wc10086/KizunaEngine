import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/app_info.dart';
import 'package:kizuna_engine/core/platform/external_links.dart';
import 'package:kizuna_engine/core/settings/settings_models.dart';
import 'package:kizuna_engine/core/update/update_controller.dart';
import 'package:kizuna_engine/core/update/update_service.dart';
import 'package:kizuna_engine/features/settings/settings_notifier.dart';
import 'package:kizuna_engine/features/update/update_dialogs.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

class AboutScreen extends ConsumerWidget {
  const AboutScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).value ?? const AppSettings();
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
                value: Text('KizunaEngine $kAppVersion'),
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.code),
                title: Text(l10n.aboutRepository),
                onPressed: (BuildContext tileContext) async {
                  await _openInBrowser(UpdateService.repositoryUrl);
                },
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.bug_report_outlined),
                title: Text(l10n.aboutFeedback),
                onPressed: (BuildContext tileContext) async {
                  await _openInBrowser(UpdateService.issuesUrl);
                },
              ),
              SettingsTile.navigation(
                leading: const Icon(Icons.description_outlined),
                title: Text(l10n.aboutLicenses),
                onPressed: (BuildContext tileContext) {
                  showLicensePage(
                    context: tileContext,
                    applicationName: 'KizunaEngine',
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
                onPressed: (BuildContext tileContext) async {
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

Future<bool> _openInBrowser(String url) => openExternalUrl(url);
