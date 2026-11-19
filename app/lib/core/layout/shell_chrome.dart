import 'package:flutter/widgets.dart';

/// 外壳向 tab 页面暴露的布局信息。
///
/// 悬浮底部导航栏是叠在页面内容之上的，页面本身并不知道它占了多高。
/// 把"底部被遮挡的高度"通过 InheritedWidget 传下去，滚动列表据此做底部留白，
/// 最后一张卡片才能滚到玻璃栏上方，而不是永远压在它下面。
///
/// 只有 tab 页在 shell 的子树里：播放页、设置子页等压栈路由在 root Navigator 上，
/// 读不到这个 widget，`bottomInsetOf` 返回 0，不会凭空多出空白。
class ShellChrome extends InheritedWidget {
  const ShellChrome({
    super.key,
    required this.bottomInset,
    required super.child,
  });

  /// 悬浮导航栏遮挡的高度（含安全区）。大屏用 NavigationRail 时为 0。
  final double bottomInset;

  static double bottomInsetOf(BuildContext context) {
    return context
            .dependOnInheritedWidgetOfExactType<ShellChrome>()
            ?.bottomInset ??
        0.0;
  }

  @override
  bool updateShouldNotify(ShellChrome oldWidget) {
    return oldWidget.bottomInset != bottomInset;
  }
}
