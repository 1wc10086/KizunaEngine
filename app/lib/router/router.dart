import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/bridge/engine_bridge.dart';
import '../features/about/view/about_screen.dart';
import '../features/library/view/library_screen.dart';
import '../features/player/player_screen.dart';
import '../features/settings/view/appearance/appearance_screen.dart';
import '../features/settings/view/game/game_settings_screen.dart';
import '../features/settings/view/language/language_screen.dart';
import '../features/settings/view/settings_screen.dart';
import '../features/shell/view/home_shell.dart';
import '../features/shell/view/widgets/tab_swipe_container.dart';
import '../features/tools/view/file_process_page.dart';
import '../features/tools/view/tools_screen.dart';
import '../features/translation/view/config_page.dart';
import '../features/translation/view/dashboard_page.dart';
import '../features/translation/view/presets_page.dart';

/// 路由表。
///
/// 底部三栏用 StatefulShellRoute：切栏保留各栏的滚动位置与子路由栈
/// （游戏库里滚到一半去设置再回来，不该跳回顶部）。
///
/// 这里没有用默认的 `StatefulShellRoute.indexedStack`，而是自己提供
/// `navigatorContainerBuilder`：默认 IndexedStack 切栏没有任何动画，也不支持
/// 左右滑动。TabSwipeContainer 保留了 IndexedStack 的"全部分支保活"语义，
/// 同时在切栏时做 tab 式横向滑动，并把内容区左/右滑手势接到同一个页码上。
///
/// 其余页面一律 MaterialPage，保持"设置/关于/播放"作为压栈页面的返回语义；
/// 播放页挂在 root Navigator 上，进入后整个屏幕都是游戏画面，不会出现外壳。
final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    routes: <RouteBase>[
      StatefulShellRoute(
        builder: (context, state, navigationShell) =>
            HomeShell(navigationShell: navigationShell),
        navigatorContainerBuilder: (context, navigationShell, children) {
          return TabSwipeContainer(
            currentIndex: navigationShell.currentIndex,
            onSelect: navigationShell.goBranch,
            children: children,
          );
        },
        branches: <StatefulShellBranch>[
          StatefulShellBranch(
            routes: <RouteBase>[
              GoRoute(
                path: '/',
                builder: (context, state) => const LibraryScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
            // preload：第一次滑到这一栏时立刻就有内容。不预载的话，
            // 滑动过程中目标分支还是一个空占位，会看到一帧空白。
            preload: true,
            routes: <RouteBase>[
              GoRoute(
                path: '/tools',
                builder: (context, state) => const ToolsScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
            preload: true,
            routes: <RouteBase>[
              GoRoute(
                path: '/settings',
                builder: (context, state) => const SettingsScreen(),
              ),
            ],
          ),
        ],
      ),
      GoRoute(
        path: '/translation',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const TranslationDashboardPage(),
        ),
        routes: <RouteBase>[
          GoRoute(
            path: 'config',
            pageBuilder: (context, state) => MaterialPage<void>(
              key: state.pageKey,
              child: const TranslationConfigPage(),
            ),
          ),
          GoRoute(
            path: 'presets',
            pageBuilder: (context, state) => MaterialPage<void>(
              key: state.pageKey,
              child: const TranslationPresetsPage(),
            ),
          ),
        ],
      ),
      GoRoute(
        path: '/file-process',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const FileProcessPage(),
        ),
      ),
      GoRoute(
        path: '/appearance',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const AppearanceScreen(),
        ),
      ),
      GoRoute(
        path: '/game-settings',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const GameSettingsScreen(),
        ),
      ),
      GoRoute(
        path: '/language',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const LanguageScreen(),
        ),
      ),
      GoRoute(
        path: '/about',
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: const AboutScreen(),
        ),
      ),
      GoRoute(
        path: '/player/:dir',
        // extra 只是"省一次扫描"的快路径：深链接/冷启动恢复时没有 extra，
        // 播放页据此从目录自行探测品牌（见 PlayerScreen.entry）。
        pageBuilder: (context, state) => MaterialPage<void>(
          key: state.pageKey,
          child: PlayerScreen(
            dir: state.pathParameters['dir'] ?? '',
            entry: state.extra is ScanEntry ? state.extra as ScanEntry : null,
          ),
        ),
      ),
    ],
  );
});
