import 'package:flutter/material.dart';
import 'package:material_ui/material_ui.dart' as mui;

/// 为 m3e_core 组件提供 `material_ui` 主题的局部作用域。
///
/// 背景：Flutter 3.47 把 material 拆成了独立的 `material_ui` 包，m3e_core 1.x
/// 依赖的是那份实现——组件内部调用的是 `mui.Theme.of`，只认 `mui.Theme`，
/// 不认 `flutter/material` 的 Theme。应用其余部分仍跑在 flutter/material 上，
/// 所以只在真正使用 m3e 组件的局部包一层，避免整棵树迁移。
///
/// `ColorScheme.fromSeed` 要做一遍色板量化，偏重；m3e 组件在滚动、刷新时
/// 会频繁重建，因此按 (seed, brightness) 缓存最近一次结果，避免每帧重算。
class M3EThemeBridge extends StatelessWidget {
  const M3EThemeBridge({super.key, required this.child});

  final Widget child;

  static mui.ThemeData? _cachedTheme;
  static Color? _cachedSeed;
  static Brightness? _cachedBrightness;

  static mui.ThemeData _themeFor(Color seed, Brightness brightness) {
    final mui.ThemeData? cached = _cachedTheme;
    if (cached != null &&
        _cachedSeed == seed &&
        _cachedBrightness == brightness) {
      return cached;
    }
    final mui.ThemeData data = mui.ThemeData(
      useMaterial3: true,
      colorScheme: mui.ColorScheme.fromSeed(
        seedColor: seed,
        brightness: brightness,
      ),
    );
    _cachedTheme = data;
    _cachedSeed = seed;
    _cachedBrightness = brightness;
    return data;
  }

  @override
  Widget build(BuildContext context) {
    // 这里的 Theme 是 flutter/material 的：外层应用树的主题。
    final ColorScheme scheme = Theme.of(context).colorScheme;
    return mui.Theme(
      data: _themeFor(scheme.primary, scheme.brightness),
      child: child,
    );
  }
}
