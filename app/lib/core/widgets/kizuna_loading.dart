import 'package:flutter/material.dart';
// m3e_core 1.1 起使用独立的 material_ui 包（Flutter 3.47 拆分）。
// 应用其余部分仍用 flutter/material；这里只给加载器包一层 material_ui.Theme，
// 让 M3EContainedLoadingIndicator 的 Theme.of 能拿到自己的主题，
// 避免为了一个加载动画把整个项目迁移到 material_ui。
import 'package:material_ui/material_ui.dart' as mui;
import 'package:m3e_core/m3e_core.dart';

/// 应用统一的加载指示器。
///
/// 使用 M3E 的 Contained 样式 + 完全圆角容器（borderRadius: null 在
/// M3EContainedLoadingIndicator 里等价于 9999px，即 pill/circle）。
/// shapes 选 4 个差异明显的几何体，避免默认 7 个形状在 88px 容器里显得碎。
class KizunaLoadingIndicator extends StatelessWidget {
  final double size;
  final String? semanticsLabel;

  const KizunaLoadingIndicator({
    super.key,
    this.size = 88,
    this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return mui.Theme(
      data: mui.ThemeData(
        useMaterial3: true,
        colorScheme: mui.ColorScheme.fromSeed(
          seedColor: scheme.primary,
          brightness: scheme.brightness,
        ),
      ),
      child: M3EContainedLoadingIndicator(
        shapes: const <Shapes>[
          Shapes.softBurst,
          Shapes.pentagon,
          Shapes.pill,
          Shapes.oval,
        ],
        width: size,
        height: size,
        padding: EdgeInsets.all(size * 0.16),
        // null = Fully rounded container，正是设计要求的胶囊/圆形外观。
        borderRadius: null,
        containerColor: scheme.primaryContainer,
        indicatorColor: scheme.onPrimaryContainer,
        semanticsLabel: semanticsLabel,
      ),
    );
  }
}
