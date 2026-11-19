import 'dart:async';

import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/core/widgets/game_root_dialog.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import '../../../library/viewmodel/library_notifier.dart';
import '../../viewmodel/settings_notifier.dart';

/// 游戏根目录这一行：显示当前目录名，点开共用对话框修改。
///
/// 对话框本身（SAF 镜像、恢复默认）在 core/widgets/game_root_dialog.dart，
/// 库页面的空状态用的是同一份实现。这里只管显示与保存。
///
/// 基类为什么是 AbstractSettingsTile：SettingsSection.tiles 只收
/// AbstractSettingsTile（纯 StatelessWidget 基类、拿不到 ref），所以
/// Riverpod 访问包在 Consumer 里，GameRootTile 本身可以直接进 tiles 列表。
class GameRootTile extends AbstractSettingsTile {
  const GameRootTile({super.key});

  Future<void> _edit(BuildContext context, WidgetRef ref, String root) async {
    final value = await showGameRootDialog(
      context,
      initial: root,
      mirrorName: p.basename(root),
    );
    if (value == null || !context.mounted) {
      return;
    }
    // 保存后不必 invalidate 库：gameRootProvider 投影自设置，
    // LibraryNotifier.build 又 watch 了它，根目录一变就会自己重扫。
    await ref.read(appSettingsProvider.notifier).setGameRoot(value.trim());
  }

  @override
  Widget build(BuildContext context) {
    return Consumer(
      builder: (context, ref, _) {
        final l10n = AppLocalizations.of(context);
        final root = ref.watch(gameRootProvider);
        return SettingsTile.navigation(
          leading: const Icon(Icons.folder_outlined),
          title: Text(l10n.gameSettingsRootTitle),
          value: Text(
            root.isEmpty ? l10n.settingsGameRootHint : p.basename(root),
          ),
          onPressed: (_) => unawaited(_edit(context, ref, root)),
        );
      },
    );
  }
}
