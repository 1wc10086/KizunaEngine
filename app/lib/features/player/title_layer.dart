import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';
import 'player_notifier.dart';

/// 主界面菜单项的 key，必须和 bsxx.cfg `[title phase]` 里的键名一致。
class TitleKeys {
  static const String opening = 'op';
  static const String start = 'start';
  static const String load = 'load';
  static const String museum = 'museum';
  static const String config = 'config';
  static const String quit = 'quit';
}

/// 主界面（标题菜单）的交互层。
///
/// 画面本身由引擎合成（title_base + 菜单图），这里只做两件引擎不该做的事：
///  1. 把游戏坐标系的按钮矩形换算成当前布局下的点击区域；
///  2. 把"玩家选了哪一项"回传给引擎。
///
/// 读档、鉴赏、环境设定都不再开 Flutter 路由：前两者由引擎用游戏自己的
/// [data screen] / [museum phase] 画面绘制，环境设定则由引擎发宿主动作，
/// 播放页收到后打开 Flutter 设置。这样存档/鉴赏的视觉与命中始终和原版一致。
class TitleLayer extends ConsumerWidget {
  final PlayerNotifier notifier;
  final VoidCallback onExit;

  const TitleLayer({
    super.key,
    required this.notifier,
    required this.onExit,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final menu = ref.watch(playerProvider.select((value) => value.titleMenu));
    final width = ref.watch(playerProvider.select((value) => value.width));
    final height = ref.watch(playerProvider.select((value) => value.height));
    if (menu.isEmpty || width <= 0 || height <= 0) {
      return const SizedBox.shrink();
    }
    return Positioned.fill(
      child: LayoutBuilder(
        builder: (context, constraints) {
          // 引擎坐标系 -> 逻辑像素：与画面用同一套等比缩放 + 垂直居中，
          // 保证热区和画面上看到的按钮严丝合缝。
          final scale = constraints.maxWidth / width;
          final offsetY = (constraints.maxHeight - height * scale) / 2;
          return MouseRegion(
            // 标题按钮图有 2 帧（常态/悬停），高亮由引擎按 HoveredButton 选帧。
            // 舞台层的 moveTo 在标题菜单存在时会被 overlayActive 拦掉，所以这里
            // 直接按画面坐标转发 hover，否则鼠标端永远看不到悬停帧。
            onHover: (event) {
              final x = (event.localPosition.dx / scale).floor();
              final y = ((event.localPosition.dy - offsetY) / scale).floor();
              if (x < 0 || y < 0 || x >= width || y >= height) {
                return;
              }
              notifier.hover(x, y);
            },
            child: Stack(
              children: <Widget>[
                for (final item in menu)
                  if (item.enabled)
                    Positioned(
                      left: item.x * scale,
                      top: offsetY + item.y * scale,
                      width: item.width * scale,
                      height: item.height * scale,
                      // 用 Listener 而不是 GestureDetector：播放舞台本身也有一个
                      // GestureDetector 处理点击推进，父子两个 tap recognizer 会
                      // 争抢 gesture arena。某些 Flutter 版本上父级 onTapDown 先
                      // 触发、子级 onTap 被取消，表现就是"标题按钮按了没反应"。
                      // Listener 直接拿原始指针事件，不参与 arena，保证每次点击
                      // 都一定送到引擎的标题分派逻辑；舞台层的 tapAt 在标题
                      // 菜单存在时本来就会提前 return，不会重复处理。
                      child: Listener(
                        behavior: HitTestBehavior.opaque,
                        onPointerUp: (_) => _select(context, item),
                        child: MouseRegion(
                          cursor: SystemMouseCursors.click,
                          child: const SizedBox.expand(),
                        ),
                      ),
                    ),
              ],
            ),
          );
        },
      ),
    );
  }

  Future<void> _select(BuildContext context, TitleMenuItem item) async {
    switch (item.key) {
      case TitleKeys.quit:
        await _confirmQuit(context);
        return;
      default:
        // start/op 进剧情，load/museum 由引擎打开游戏内界面，
        // config 由引擎发 "settings" 宿主动作；宿主不再自己实现这些界面。
        await notifier.selectTitleItem(item.key);
    }
  }

  Future<void> _confirmQuit(BuildContext context) async {
    final l10n = AppLocalizations.of(context)!;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text(l10n.playerQuitTitle),
        content: Text(l10n.playerQuitMessage),
        actions: <Widget>[
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: Text(l10n.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text(l10n.commonConfirm),
          ),
        ],
      ),
    );
    if (confirmed == true && context.mounted) {
      onExit();
    }
  }
}
