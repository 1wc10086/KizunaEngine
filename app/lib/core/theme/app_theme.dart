import 'package:flutter/material.dart';

const SliderThemeData kAppSliderTheme = SliderThemeData(year2023: false);

/// AppBar 的常驻底色。
///
/// M3 的 AppBar 在内容滚到它下方时会从 `surface` 换成 `surfaceContainer`
/// （见 Flutter AppBar 的 `scrolledUnderBackground`）。本应用要求顶栏颜色
/// 始终就是这个"滚动态颜色"：滚动时不再变色，看起来是一块固定的工具栏。
/// 因此这里直接取 `ColorScheme.surfaceContainer`，和 Material 默认的
/// scrolledUnder 颜色逐值一致，只是把它变成常驻的。
Color appBarSurfaceColor(ColorScheme scheme) => scheme.surfaceContainer;

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
    // 固定顶栏底色：backgroundColor 已经是滚动态颜色，如果不禁用
    // scrolledUnder 的 tint / elevation，滚动时会再叠一层色差，颜色就变了。
    // 用 AppBarThemeData 而不是 AppBarTheme：后者在 3.4x 起是 InheritedTheme，
    // 传进 ThemeData 时会被拆出 .data，写错成员不会报错但会静默失效。
    appBarTheme: AppBarThemeData(
      backgroundColor: appBarSurfaceColor(scheme),
      foregroundColor: scheme.onSurface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
    ),
    // SearchBar 默认带 elevation 阴影，和常驻工具栏叠在一起会显脏；
    // 这里压平并统一胶囊圆角。
    searchBarTheme: SearchBarThemeData(
      elevation: const WidgetStatePropertyAll<double>(0),
      backgroundColor:
          WidgetStatePropertyAll<Color?>(scheme.surfaceContainerHigh),
      shape: const WidgetStatePropertyAll<OutlinedBorder>(StadiumBorder()),
    ),
    sliderTheme: kAppSliderTheme,
  );
}
