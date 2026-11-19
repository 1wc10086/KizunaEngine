import 'package:kizuna_engine/l10n/app_localizations.dart';

String paletteLabel(AppLocalizations l10n, String id) => switch (id) {
      'green' => l10n.paletteGreen,
      'teal' => l10n.paletteTeal,
      'blue' => l10n.paletteBlue,
      'indigo' => l10n.paletteIndigo,
      'pink' => l10n.palettePink,
      'yellow' => l10n.paletteYellow,
      'orange' => l10n.paletteOrange,
      'deepOrange' => l10n.paletteDeepOrange,
      _ => l10n.palettePurple,
    };
