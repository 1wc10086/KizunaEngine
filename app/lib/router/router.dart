import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/bridge/engine_bridge.dart';
import '../features/about/about_screen.dart';
import '../features/library/library_screen.dart';
import '../features/player/player_screen.dart';
import '../features/settings/appearance/appearance_screen.dart';
import '../features/settings/game/game_settings_screen.dart';
import '../features/settings/language/language_screen.dart';
import '../features/settings/settings_screen.dart';
import '../features/shell/home_shell.dart';
import '../features/tools/file_process_page.dart';
import '../features/tools/tools_screen.dart';
import '../features/translation/config_page.dart';
import '../features/translation/dashboard_page.dart';
import '../features/translation/presets_page.dart';

final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    routes: <RouteBase>[
      StatefulShellRoute.indexedStack(
        builder: (context, state, navigationShell) =>
            HomeShell(navigationShell: navigationShell),
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
            routes: <RouteBase>[
              GoRoute(
                path: '/tools',
                builder: (context, state) => const ToolsScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
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
