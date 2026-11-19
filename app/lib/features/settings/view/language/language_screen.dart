import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/settings/settings_models.dart';
import 'package:kizuna_engine/core/settings/settings_snapshot.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../viewmodel/settings_notifier.dart';
import '../widgets/settings_scaffold.dart';

class LanguageScreen extends ConsumerWidget {
  const LanguageScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).valueOrDefault;

    SettingsTile<AppLanguage> radio(
      IconData icon,
      String title,
      AppLanguage value,
    ) {
      return SettingsTile<AppLanguage>.radioTile(
        leading: Icon(icon),
        title: Text(title),
        radioValue: value,
        groupValue: settings.language,
        onChanged: (next) {
          if (next != null) {
            ref.read(appSettingsProvider.notifier).setLanguage(next);
          }
        },
      );
    }

    return SettingsScaffold(
      title: l10n.languageSettingsTitle,
      sections: <AbstractSettingsSection>[
        SettingsSection(
          title: Text(l10n.languageSettingsTitle),
          tiles: <AbstractSettingsTile>[
            radio(Icons.settings_suggest_outlined, l10n.languageSystem,
                AppLanguage.system),
            radio(Icons.translate, l10n.languageZh, AppLanguage.zh),
            radio(Icons.language, l10n.languageEn, AppLanguage.en),
          ],
        ),
      ],
    );
  }
}
