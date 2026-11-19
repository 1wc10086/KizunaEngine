import 'package:flutter/material.dart';
import 'package:m3e_core/m3e_core.dart';

import 'm3e_theme_bridge.dart';

/// 应用统一的加载指示器。
///
/// 使用 M3E 的 Contained 样式 + 完全圆角容器（borderRadius: null 在
/// M3EContainedLoadingIndicator 里等价于 9999px，即 pill/circle）。
/// shapes 选 4 个差异明显的几何体，避免默认 7 个形状在 88px 容器里显得碎。
///
/// [progress] 为 null 时是不定长动画；传入 0..1 会变成"跟手"的确定进度，
/// 下拉刷新在手指拖动阶段就靠这个值让指示器跟着回弹。
///
/// m3e_core 读的是 material_ui 的 Theme，这里由 M3EThemeBridge 局部提供，
/// 不会影响外层 flutter/material 的配色。
class KizunaLoadingIndicator extends StatelessWidget {
  final double size;
  final double? progress;
  final String? semanticsLabel;

  const KizunaLoadingIndicator({
    super.key,
    this.size = 88,
    this.progress,
    this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    final ColorScheme scheme = Theme.of(context).colorScheme;
    return M3EThemeBridge(
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
        progress: progress,
        semanticsLabel: semanticsLabel,
      ),
    );
  }
}
