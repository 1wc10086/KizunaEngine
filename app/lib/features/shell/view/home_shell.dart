import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../../core/layout/shell_chrome.dart';
import '../../update/view/update_dialogs.dart';
import '../viewmodel/update_check_throttle.dart';
import 'nav_destinations.dart';
import 'widgets/glass_nav_bar.dart';

/// 外壳：悬浮玻璃底部导航（窄屏）/ NavigationRail（宽屏） + 自动检查更新。
///
/// 两种导航形态共用同一份分支索引，切换动画与左右滑动由 TabSwipeContainer
/// 统一处理，所以点导航、滑页面、系统手势三者的体验是一致的。
///
/// 检查更新时机有三处：进首帧、从后台回前台、点回"游戏"栏。三处都要过
/// UpdateCheckThrottle 节流，否则用户来回切栏就是连着几次 GitHub 请求。
class HomeShell extends ConsumerStatefulWidget {
  final StatefulNavigationShell navigationShell;

  const HomeShell({super.key, required this.navigationShell});

  /// 使用侧边导航栏的最小宽度。
  ///
  /// 600 是 Material 3 的 medium 窗口尺寸类下界，也是平板竖屏的典型宽度；
  /// 手机被锁在竖屏（见 main.dart），宽度远小于它，所以这里不会把手机
  /// 误判成大屏。
  static const double railBreakpoint = 600;

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _maybeCheck();
    });
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      _maybeCheck();
    }
  }

  void _maybeCheck() {
    if (!mounted) {
      return;
    }
    if (!ref.read(updateCheckThrottleProvider).take()) {
      return;
    }
    showUpdateFlow(context, ref, silent: true);
  }

  void _select(int index) {
    if (index == 0) {
      _maybeCheck();
    }
    // 再点一次当前栏 = 回到该栏的根页面，这是底部导航的惯例行为。
    widget.navigationShell.goBranch(
      index,
      initialLocation: index == widget.navigationShell.currentIndex,
    );
  }

  @override
  Widget build(BuildContext context) {
    final AppLocalizations l10n = AppLocalizations.of(context);
    final StatefulNavigationShell shell = widget.navigationShell;
    final bool wide = MediaQuery.sizeOf(context).width >= HomeShell.railBreakpoint;
    final double inset = wide ? 0 : glassNavBarInset(context);

    final Widget body = ShellChrome(
      bottomInset: inset,
      child: shell,
    );

    if (wide) {
      return Scaffold(
        body: Row(
          children: <Widget>[
            NavigationRail(
              selectedIndex: shell.currentIndex,
              onDestinationSelected: _select,
              labelType: NavigationRailLabelType.all,
              // Rail 贴边时给一条分隔线，避免和内容混在一起。
              leading: const SizedBox(height: 8),
              destinations: <NavigationRailDestination>[
                for (final NavDestination destination in NavDestinations.items)
                  NavigationRailDestination(
                    icon: Icon(destination.icon),
                    selectedIcon: Icon(destination.selectedIcon),
                    label: Text(destination.label(l10n)),
                  ),
              ],
            ),
            const VerticalDivider(width: 1, thickness: 1),
            Expanded(child: body),
          ],
        ),
      );
    }

    return Scaffold(
      // 悬浮导航栏是"浮"在内容上的：让 body 铺到屏幕底部，页面自己用
      // ShellChrome.bottomInset 给可滚动内容留出避让空间。
      extendBody: true,
      body: body,
      bottomNavigationBar: GlassBottomNavBar(
        selectedIndex: shell.currentIndex,
        onSelected: _select,
      ),
    );
  }
}
