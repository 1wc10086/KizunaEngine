import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/settings/settings.dart';
import 'package:kizuna_engine/core/theme/palette.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../settings_notifier.dart';
import '../widgets/settings_scaffold.dart';
import '../widgets/settings_slider_tile.dart';
import 'palette_dialog.dart';
import 'theme_mode_menu.dart';

class AppearanceScreen extends ConsumerWidget {
  const AppearanceScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final settings = ref.watch(appSettingsProvider).value ?? const AppSettings();
    return SettingsScaffold(
      title: l10n.appearanceTitle,
      sections: [
        SettingsSection(
          title: Text(l10n.appearanceTitle),
          tiles: [
            SettingsTile.navigation(
              leading: const Icon(Icons.dark_mode_outlined),
              title: Text(l10n.appearanceThemeMode),
              value: Text(themeModeLabel(l10n, settings.themeMode)),
              onPressed: (tileContext) {
                showThemeModeMenu(
                  tileContext,
                  anchor: menuAnchorBelow(tileContext),
                  current: settings.themeMode,
                  onSelected: (mode) {
                    ref.read(appSettingsProvider.notifier).setThemeMode(mode);
                  },
                );
              },
            ),
            SettingsTile.navigation(
              leading: const Icon(Icons.color_lens_outlined),
              title: Text(l10n.appearancePalette),
              description: Text(l10n.appearancePaletteNote),
              trailing: _PaletteDot(
                color: paletteColor(settings.themePaletteId),
              ),
              onPressed: (tileContext) {
                showPaletteDialog(tileContext, ref, settings.themePaletteId);
              },
            ),
            SettingsTile.switchTile(
              leading: const Icon(Icons.contrast),
              title: Text(l10n.appearanceAmoled),
              description: Text(l10n.appearanceAmoledNote),
              initialValue: settings.amoled,
              onToggle: (value) {
                ref
                    .read(appSettingsProvider.notifier)
                    .setAmoled(value ?? !settings.amoled);
              },
            ),
            SettingsSliderTile(
              icon: Icons.format_size,
              title: l10n.appearanceFontScale,
              description: l10n.appearanceFontScaleNote,
              min: 0.8,
              max: 1.5,
              divisions: 14,
              value: settings.fontScale,
              labelBuilder: (value) => '${(value * 100).round()}%',
              onChangeEnd: (value) {
                ref.read(appSettingsProvider.notifier).setFontScale(value);
              },
            ),
          ],
        ),
      ],
    );
  }
}

class _PaletteDot extends StatelessWidget {
  const _PaletteDot({required this.color});

  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 20,
      height: 20,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
        border: Border.all(color: Theme.of(context).colorScheme.outlineVariant),
      ),
    );
  }
}
