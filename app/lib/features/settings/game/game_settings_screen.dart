import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/settings/settings.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../settings_notifier.dart';
import '../widgets/settings_scaffold.dart';
import 'game_root_tile.dart';

class GameSettingsScreen extends ConsumerWidget {
  const GameSettingsScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).value ?? const AppSettings();
    return SettingsScaffold(
      title: l10n.gameSettingsTitle,
      sections: <AbstractSettingsSection>[
        SettingsSection(
          title: Text(l10n.settingsSectionGame),
          tiles: <AbstractSettingsTile>[
            GameRootTile(gameRoot: settings.gameRoot),
            SettingsTile.switchTile(
              leading: const Icon(Icons.chat_bubble_outline),
              title: Text(l10n.gameSettingsShowMessageFrame),
              initialValue: settings.showMessageFrame,
              onToggle: (value) {
                ref
                    .read(appSettingsProvider.notifier)
                    .setShowMessageFrame(
                      value ?? !settings.showMessageFrame,
                    );
              },
            ),
          ],
        ),
      ],
    );
  }
}
