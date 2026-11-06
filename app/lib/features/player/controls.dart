import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

class PlayerControls extends StatelessWidget {
  final bool auto;
  final bool skip;
  final ValueChanged<bool> onAuto;
  final ValueChanged<bool> onSkip;
  final VoidCallback onMenu;
  final VoidCallback onExit;
  final bool landscape;
  final VoidCallback onToggleOrientation;

  const PlayerControls(
      {super.key,
      required this.auto,
      required this.skip,
      required this.onAuto,
      required this.onSkip,
      required this.onMenu,
      required this.onExit,
      required this.landscape,
      required this.onToggleOrientation});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Container(
      color: const Color(0xFF000000),
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 6),
      child: LayoutBuilder(
        builder: (context, constraints) {
          return SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            physics: const BouncingScrollPhysics(),
            child: ConstrainedBox(
              constraints: BoxConstraints(minWidth: constraints.maxWidth),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                mainAxisAlignment: MainAxisAlignment.center,
                children: <Widget>[
                  _Toggle(
                    label: l10n.playerAuto,
                    icon: Icons.play_arrow,
                    value: auto,
                    onChanged: onAuto,
                  ),
                  _Toggle(
                    label: l10n.playerSkip,
                    icon: Icons.fast_forward,
                    value: skip,
                    onChanged: onSkip,
                  ),
                  // 存档与鉴赏不放在这里：游戏本身就带这两套界面（Bishop 的
                  // [data screen] / [museum phase]），由引擎绘制并处理点击。
                  // 这里只留一个"菜单"入口，把 ESC 送给引擎展开游戏内菜单条。
                  _Action(
                    label: l10n.playerMenu,
                    icon: Icons.menu,
                    onPressed: onMenu,
                  ),
                  _Action(
                    label:
                        landscape ? l10n.playerPortrait : l10n.playerLandscape,
                    icon: landscape
                        ? Icons.screen_lock_portrait
                        : Icons.screen_lock_landscape,
                    onPressed: onToggleOrientation,
                  ),
                  _Action(
                    label: l10n.playerExit,
                    icon: Icons.exit_to_app,
                    onPressed: onExit,
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

class _Toggle extends StatelessWidget {
  final String label;
  final IconData icon;
  final bool value;
  final ValueChanged<bool> onChanged;

  const _Toggle(
      {required this.label,
      required this.icon,
      required this.value,
      required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      style: TextButton.styleFrom(
        foregroundColor: value
            ? Theme.of(context).colorScheme.primary
            : const Color(0xFFFFFFFF),
        minimumSize: const Size(72, 56),
      ),
      icon: Icon(icon),
      label: Text(label),
      onPressed: () {
        onChanged(!value);
      },
    );
  }
}

class _Action extends StatelessWidget {
  final String label;
  final IconData icon;
  final VoidCallback? onPressed;

  const _Action(
      {required this.label, required this.icon, required this.onPressed});

  @override
  Widget build(BuildContext context) {
    return TextButton.icon(
      style: TextButton.styleFrom(
        foregroundColor: const Color(0xFFFFFFFF),
        minimumSize: const Size(72, 56),
      ),
      icon: Icon(icon),
      label: Text(label),
      onPressed: onPressed,
    );
  }
}
