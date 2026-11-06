import 'package:flutter/material.dart';
import 'package:kizuna_engine/core/settings/settings.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

String themeModeLabel(AppLocalizations l10n, AppThemeMode mode) => switch (mode) {
      AppThemeMode.system => l10n.themeModeSystem,
      AppThemeMode.light => l10n.themeModeLight,
      AppThemeMode.dark => l10n.themeModeDark,
    };

IconData _themeModeIcon(AppThemeMode mode) => switch (mode) {
      AppThemeMode.system => Icons.brightness_auto_rounded,
      AppThemeMode.light => Icons.light_mode_rounded,
      AppThemeMode.dark => Icons.dark_mode_rounded,
    };

Offset menuAnchorBelow(BuildContext context) {
  final renderObject = context.findRenderObject();
  if (renderObject is RenderBox && renderObject.hasSize) {
    final below = Offset(0, renderObject.size.height);
    final overlay = Overlay.of(context).context.findRenderObject();
    if (overlay is RenderBox) {
      return renderObject.localToGlobal(Offset.zero, ancestor: overlay) + below;
    }
    return renderObject.localToGlobal(Offset.zero) + below;
  }
  final size = MediaQuery.of(context).size;
  return Offset(size.width / 2, size.height / 3);
}

Future<void> showThemeModeMenu(
  BuildContext context, {
  required Offset anchor,
  required AppThemeMode current,
  required ValueChanged<AppThemeMode> onSelected,
}) async {
  final l10n = AppLocalizations.of(context);
  final colorScheme = Theme.of(context).colorScheme;
  final overlay = Overlay.of(context).context.findRenderObject();
  final overlaySize =
      overlay is RenderBox ? overlay.size : MediaQuery.of(context).size;
  final selected = await showMenu<AppThemeMode>(
    context: context,
    position: RelativeRect.fromRect(
      Rect.fromPoints(anchor, anchor),
      Offset.zero & overlaySize,
    ),
    items: <PopupMenuEntry<AppThemeMode>>[
      for (final AppThemeMode mode in AppThemeMode.values)
        PopupMenuItem<AppThemeMode>(
          value: mode,
          child: Row(
            children: <Widget>[
              Icon(
                _themeModeIcon(mode),
                color: mode == current ? colorScheme.primary : null,
              ),
              const SizedBox(width: 12),
              Text(
                themeModeLabel(l10n, mode),
                style: TextStyle(
                  color: mode == current ? colorScheme.primary : null,
                  fontWeight: mode == current ? FontWeight.w600 : null,
                ),
              ),
            ],
          ),
        ),
    ],
  );
  if (selected == null || selected == current) {
    return;
  }
  onSelected(selected);
}
