import 'package:flutter/material.dart';

const SliderThemeData kAppSliderTheme = SliderThemeData(year2023: false);

ThemeData buildAppTheme({
  required Brightness brightness,
  required Color seed,
  required bool amoled,
}) {
  final ColorScheme base = ColorScheme.fromSeed(
    seedColor: seed,
    brightness: brightness,
  );
  final ColorScheme tuned = base.copyWith(
    surfaceContainerLowest: base.surfaceContainerLow,
    onPrimaryContainer: base.primary,
  );
  final bool amoledActive = amoled && brightness == Brightness.dark;
  final ColorScheme scheme = amoledActive
      ? tuned.copyWith(
          surface: Colors.black,
          onSurface: Colors.white,
          surfaceContainerLowest: Colors.black,
          surfaceContainerLow: const Color(0xFF0A0A0A),
          surfaceContainer: const Color(0xFF141414),
        )
      : tuned;
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    scaffoldBackgroundColor: amoledActive ? Colors.black : null,
    appBarTheme: amoledActive
        ? const AppBarTheme(
            backgroundColor: Colors.black,
            foregroundColor: Colors.white,
          )
        : null,
    sliderTheme: kAppSliderTheme,
  );
}
