import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/theme/palette.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../settings_notifier.dart';
import 'palette_labels.dart';

Future<void> showPaletteDialog(
  BuildContext context,
  WidgetRef ref,
  String currentId,
) {
  return showDialog<void>(
    context: context,
    builder: (dialogContext) {
      final l10n = AppLocalizations.of(dialogContext);
      return AlertDialog(
        title: Text(l10n.appearancePalette),
        content: SingleChildScrollView(
          child: SizedBox(
            width: 320,
            child: Wrap(
              alignment: WrapAlignment.center,
              spacing: 8,
              runSpacing: 12,
              children: <Widget>[
                for (final AppPalette palette in kAppPalettes)
                  _PaletteOption(
                    palette: palette,
                    selected: palette.id == currentId,
                    onTap: () {
                      ref
                          .read(appSettingsProvider.notifier)
                          .setThemePalette(palette.id);
                      Navigator.of(dialogContext).pop();
                    },
                  ),
              ],
            ),
          ),
        ),
      );
    },
  );
}

class _PaletteOption extends StatelessWidget {
  const _PaletteOption({
    required this.palette,
    required this.selected,
    required this.onTap,
  });

  final AppPalette palette;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final scheme = ColorScheme.fromSeed(seedColor: palette.color);
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          SizedBox(
            width: 70,
            height: 70,
            child: Stack(
              children: <Widget>[
                Card(
                  elevation: 0,
                  child: Container(
                    padding: const EdgeInsets.all(10),
                    child: ClipOval(
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: <Widget>[
                          Expanded(
                            child: Container(color: scheme.primaryContainer),
                          ),
                          Expanded(
                            child: Row(
                              mainAxisAlignment: MainAxisAlignment.center,
                              children: <Widget>[
                                Expanded(
                                  child: Container(
                                    color: scheme.tertiaryContainer,
                                  ),
                                ),
                                Expanded(
                                  child: Container(
                                    color: scheme.primary,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
                if (selected)
                  Center(
                    child: Container(
                      width: 25,
                      height: 25,
                      decoration: BoxDecoration(
                        color: scheme.primary,
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        Icons.check_rounded,
                        size: 12,
                        color: Theme.of(context).colorScheme.surfaceContainerLow,
                      ),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 4),
          SizedBox(
            width: 70,
            child: Text(
              paletteLabel(l10n, palette.id),
              textAlign: TextAlign.center,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: Theme.of(context).textTheme.labelSmall,
            ),
          ),
        ],
      ),
    );
  }
}
