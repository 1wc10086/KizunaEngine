import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import 'widgets/settings_scaffold.dart';

/// 设置首页：只是一个分组入口表，全部状态都在各子页面里。
class SettingsScreen extends StatelessWidget {
  const SettingsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return SettingsScaffold(
      title: l10n.settingsTitle,
      sections: <AbstractSettingsSection>[
        SettingsSection(
          title: Text(l10n.settingsSectionAppearance),
          tiles: <AbstractSettingsTile>[
            SettingsTile.navigation(
              leading: const Icon(Icons.palette_outlined),
              title: Text(l10n.settingsAppearanceEntry),
              onPressed: (BuildContext context) {
                context.push('/appearance');
              },
            ),
          ],
        ),
        SettingsSection(
          title: Text(l10n.settingsSectionGame),
          tiles: <AbstractSettingsTile>[
            SettingsTile.navigation(
              leading: const Icon(Icons.sports_esports_outlined),
              title: Text(l10n.settingsGameEntry),
              onPressed: (BuildContext context) {
                context.push('/game-settings');
              },
            ),
          ],
        ),
        SettingsSection(
          title: Text(l10n.settingsSectionLanguage),
          tiles: <AbstractSettingsTile>[
            SettingsTile.navigation(
              leading: const Icon(Icons.translate),
              title: Text(l10n.settingsLanguageEntry),
              onPressed: (BuildContext context) {
                context.push('/language');
              },
            ),
          ],
        ),
        SettingsSection(
          title: Text(l10n.settingsSectionMore),
          tiles: <AbstractSettingsTile>[
            SettingsTile.navigation(
              leading: const Icon(Icons.info_outline),
              title: Text(l10n.settingsAboutEntry),
              onPressed: (BuildContext context) {
                context.push('/about');
              },
            ),
          ],
        ),
      ],
    );
  }
}
