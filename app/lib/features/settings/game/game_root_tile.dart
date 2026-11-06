import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/features/library/library_notifier.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import '../../../core/platform/app_paths.dart';
import '../../../core/platform/folder_picker.dart';
import '../settings_notifier.dart';

class GameRootTile extends AbstractSettingsTile {
  final String? gameRoot;

  const GameRootTile({super.key, required this.gameRoot});

  Future<void> _edit(
    BuildContext context,
    WidgetRef ref,
    String current,
  ) async {
    final l10n = AppLocalizations.of(context);
    final controller = TextEditingController(text: current);
    String? selected = current;
    var picking = false;
    var preparing = false;
    final result = await showDialog<String>(
      context: context,
      builder: (dialogContext) {
        return StatefulBuilder(
          builder: (context, setState) {
            Future<void> pick() async {
              setState(() {
                picking = true;
              });
              final picked = await FolderPicker.pick();
              if (picked == null) {
                setState(() {
                  picking = false;
                });
                return;
              }
              var path = picked.path;
              if ((path == null || path.isEmpty) && picked.uri != null) {
                setState(() {
                  picking = false;
                  preparing = true;
                });
                path = await FolderPicker.mirror(
                  picked.uri!,
                  p.basename(current),
                );
              }
              setState(() {
                picking = false;
                preparing = false;
                if (path != null && path.isNotEmpty) {
                  selected = path;
                  controller.text = path;
                }
              });
              if (path == null || path.isEmpty) {
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(content: Text(l10n.settingsFolderUnavailable)),
                  );
                }
              }
            }

            Future<void> useDefault() async {
              setState(() {
                preparing = true;
              });
              final root = await defaultGameRoot();
              setState(() {
                preparing = false;
                selected = root;
                controller.text = root;
              });
            }

            return AlertDialog(
              title: Text(l10n.gameSettingsRootTitle),
              content: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  TextFormField(
                    controller: controller,
                    autofocus: true,
                    decoration:
                        InputDecoration(hintText: l10n.settingsGameRootHint),
                    onFieldSubmitted: (value) {
                      Navigator.of(dialogContext).pop(value);
                    },
                  ),
                  const SizedBox(height: 8),
                  if (FolderPicker.supported)
                    Align(
                      alignment: Alignment.centerLeft,
                      child: TextButton.icon(
                        icon: picking || preparing
                            ? const SizedBox(
                                width: 18,
                                height: 18,
                                child:
                                    CircularProgressIndicator(strokeWidth: 2),
                              )
                            : const Icon(Icons.folder_open),
                        label: Text(
                          preparing
                              ? l10n.settingsFolderPrepare
                              : l10n.settingsPickFolder,
                        ),
                        onPressed: picking || preparing ? null : pick,
                      ),
                    ),
                  Align(
                    alignment: Alignment.centerLeft,
                    child: TextButton.icon(
                      icon: const Icon(Icons.restart_alt),
                      label: Text(l10n.settingsUseDefaultFolder),
                      onPressed: preparing ? null : useDefault,
                    ),
                  ),
                ],
              ),
              actions: <Widget>[
                TextButton(
                  onPressed: () {
                    Navigator.of(dialogContext).pop();
                  },
                  child: Text(l10n.commonCancel),
                ),
                FilledButton(
                  onPressed: () {
                    Navigator.of(dialogContext)
                        .pop(selected ?? controller.text.trim());
                  },
                  child: Text(l10n.commonConfirm),
                ),
              ],
            );
          },
        );
      },
    );
    controller.dispose();
    if (result == null || !context.mounted) {
      return;
    }
    await ref.read(appSettingsProvider.notifier).setGameRoot(result.trim());
    if (!context.mounted) {
      return;
    }
    ref.invalidate(gameRootProvider);
  }

  @override
  Widget build(BuildContext context) {
    return Consumer(
      builder: (context, ref, child) {
        final l10n = AppLocalizations.of(context);
        final root = gameRoot ?? '';
        return SettingsTile.navigation(
          leading: const Icon(Icons.folder_outlined),
          title: Text(l10n.gameSettingsRootTitle),
          value: Text(
            root.isEmpty ? l10n.settingsGameRootHint : p.basename(root),
          ),
          onPressed: (_) {
            _edit(context, ref, root);
          },
        );
      },
    );
  }
}
