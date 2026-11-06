import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../update/update_dialogs.dart';
import 'nav_destinations.dart';

class HomeShell extends ConsumerStatefulWidget {
  final StatefulNavigationShell navigationShell;

  const HomeShell({super.key, required this.navigationShell});

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell>
    with WidgetsBindingObserver {
  static const Duration _checkCooldown = Duration(minutes: 3);
  DateTime? _lastCheck;

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
    final now = DateTime.now();
    final last = _lastCheck;
    if (last != null && now.difference(last) < _checkCooldown) {
      return;
    }
    _lastCheck = now;
    showUpdateFlow(context, ref, silent: true);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final shell = widget.navigationShell;
    return Scaffold(
      body: shell,
      bottomNavigationBar: NavigationBar(
        selectedIndex: shell.currentIndex,
        onDestinationSelected: (index) {
          if (index == 0) {
            _maybeCheck();
          }
          shell.goBranch(
            index,
            initialLocation: index == shell.currentIndex,
          );
        },
        destinations: <Widget>[
          for (final destination in NavDestinations.items)
            NavigationDestination(
              icon: Icon(destination.icon),
              selectedIcon: Icon(destination.selectedIcon),
              label: destination.label(l10n),
            ),
        ],
      ),
    );
  }
}
