import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

String _gamesLabel(AppLocalizations l10n) => l10n.navGames;

String _toolsLabel(AppLocalizations l10n) => l10n.navTools;

String _settingsLabel(AppLocalizations l10n) => l10n.navSettings;

class NavDestination {
  final IconData icon;
  final IconData selectedIcon;
  final String route;
  final String Function(AppLocalizations l10n) label;

  const NavDestination({
    required this.icon,
    required this.selectedIcon,
    required this.route,
    required this.label,
  });
}

class NavDestinations {
  static const List<NavDestination> items = <NavDestination>[
    NavDestination(
      icon: Icons.sports_esports_outlined,
      selectedIcon: Icons.sports_esports,
      route: '/',
      label: _gamesLabel,
    ),
    NavDestination(
      icon: Icons.build_outlined,
      selectedIcon: Icons.build,
      route: '/tools',
      label: _toolsLabel,
    ),
    NavDestination(
      icon: Icons.settings_outlined,
      selectedIcon: Icons.settings,
      route: '/settings',
      label: _settingsLabel,
    ),
  ];
}
