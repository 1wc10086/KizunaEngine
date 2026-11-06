import 'package:flutter/material.dart';

class AppPalette {
  final String id;
  final int argb;

  const AppPalette(this.id, this.argb);

  Color get color => Color(argb);
}

const String kDefaultPaletteId = 'purple';

const List<AppPalette> kAppPalettes = <AppPalette>[
  AppPalette('purple', 0xFF6750A4),
  AppPalette('green', 0xFF2E7D32),
  AppPalette('teal', 0xFF00796B),
  AppPalette('blue', 0xFF1976D2),
  AppPalette('indigo', 0xFF3F51B5),
  AppPalette('pink', 0xFFE91E63),
  AppPalette('yellow', 0xFFF9A825),
  AppPalette('orange', 0xFFEF6C00),
  AppPalette('deepOrange', 0xFFD84315),
];

AppPalette paletteById(String id) {
  for (final AppPalette palette in kAppPalettes) {
    if (palette.id == id) {
      return palette;
    }
  }
  return kAppPalettes.first;
}

Color paletteColor(String id) => paletteById(id).color;
