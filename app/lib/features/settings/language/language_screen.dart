import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/settings/settings.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../settings_notifier.dart';
import '../widgets/settings_scaffold.dart';

class LanguageScreen extends ConsumerWidget {
  const LanguageScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).value ?? const AppSettings();
    return SettingsScaffold(
      title: l10n.languageSettingsTitle,
      sections: <AbstractSettingsSection>[
        SettingsSection(
          title: Text(l10n.languageSettingsTitle),
          tiles: <AbstractSettingsTile>[
            SettingsTile<AppLanguage>.radioTile(
              leading: const Icon(Icons.settings_suggest_outlined),
              title: Text(l10n.languageSystem),
              radioValue: AppLanguage.system,
              groupValue: settings.language,
              onChanged: (value) {
                if (value != null) {
                  ref.read(appSettingsProvider.notifier).setLanguage(value);
                }
              },
            ),
            SettingsTile<AppLanguage>.radioTile(
              leading: const Icon(Icons.translate),
              title: Text(l10n.languageZh),
              radioValue: AppLanguage.zh,
              groupValue: settings.language,
              onChanged: (value) {
                if (value != null) {
                  ref.read(appSettingsProvider.notifier).setLanguage(value);
                }
              },
            ),
            SettingsTile<AppLanguage>.radioTile(
              leading: const Icon(Icons.language),
              title: Text(l10n.languageEn),
              radioValue: AppLanguage.en,
              groupValue: settings.language,
              onChanged: (value) {
                if (value != null) {
                  ref.read(appSettingsProvider.notifier).setLanguage(value);
                }
              },
            ),
          ],
        ),
      ],
    );
  }
}
