import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:path/path.dart' as p;

import '../platform/app_paths.dart';
import '../platform/folder_picker.dart';

/// 编辑游戏根目录的对话框，返回用户确认后的路径（null 表示取消）。
///
/// 库页面和设置页都要这个对话框，而且都要处理 SAF 的两条分支
/// （可直接读的 path / 只能镜像的 content uri）。抄两份的问题是提示文案
/// 与"失败时不要覆盖当前值"的处理很容易改漏一处，所以收到这里共用。
Future<String?> showGameRootDialog(
  BuildContext context, {
  required String initial,
  String? mirrorName,
}) {
  return showDialog<String>(
    context: context,
    builder: (dialogContext) => _GameRootDialog(
      initial: initial,
      // 镜像目录的命名：设置页用当前目录名，库页面没有"当前"就用默认名的
      // 本地化文案（不写死字符串，英文界面下不会出现中文目录名）。
      mirrorNameOverride: mirrorName,
    ),
  );
}

class _GameRootDialog extends StatefulWidget {
  final String initial;
  final String? mirrorNameOverride;

  const _GameRootDialog({required this.initial, this.mirrorNameOverride});

  @override
  State<_GameRootDialog> createState() => _GameRootDialogState();
}

class _GameRootDialogState extends State<_GameRootDialog> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.initial);
  bool _picking = false;
  bool _preparing = false;
  // _preparing 被"选 SAF 目录"和"恢复默认目录"共用，只有前者需要
  // 提示"正在通过 SAF 复制"，所以单独记一个来源。
  bool _mirroring = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _pick() async {
    setState(() {
      _picking = true;
    });
    final picked = await FolderPicker.pick();
    if (!mounted) {
      return;
    }
    if (picked == null) {
      setState(() {
        _picking = false;
      });
      return;
    }
    var path = picked.path;
    if ((path == null || path.isEmpty) && picked.uri != null) {
      setState(() {
        _picking = false;
        _preparing = true;
        _mirroring = true;
      });
      // SAF 目录没有可读路径时只能整目录镜像到应用私有目录；这一步可能
      // 持续几十秒，所以先把按钮切成"正在准备"再 await。
      //
      // 目标子目录名优先沿用当前目录名（同一部游戏重复选择不会生成
      // kizuna、kizuna-2…），没有名字时用本地化的“默认目录”。
      final fallbackName = p.basename(widget.initial);
      path = await FolderPicker.mirror(
        picked.uri!,
        widget.mirrorNameOverride ??
            (fallbackName.isEmpty
                ? AppLocalizations.of(context).settingsDefaultRoot
                : fallbackName),
      );
      if (!mounted) {
        return;
      }
    }
    setState(() {
      _picking = false;
      _preparing = false;
      _mirroring = false;
      if (path != null && path.isNotEmpty) {
        _controller.text = path;
      }
    });
    if (path == null || path.isEmpty) {
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(AppLocalizations.of(context).settingsFolderUnavailable),
        ),
      );
    }
  }

  Future<void> _useDefault() async {
    setState(() {
      _preparing = true;
    });
    final root = await defaultGameRoot();
    if (!mounted) {
      return;
    }
    setState(() {
      _preparing = false;
      _controller.text = root;
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final busy = _picking || _preparing;
    return AlertDialog(
      title: Text(l10n.gameSettingsRootTitle),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          TextFormField(
            controller: _controller,
            autofocus: true,
            decoration: InputDecoration(hintText: l10n.settingsGameRootHint),
            onFieldSubmitted: (value) {
              Navigator.of(context).pop(value);
            },
          ),
          const SizedBox(height: 8),
          if (_mirroring)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                l10n.settingsSafMirroring,
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ),
          if (FolderPicker.supported)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton.icon(
                icon: busy
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.folder_open),
                label: Text(
                  _preparing
                      ? l10n.settingsFolderPrepare
                      : l10n.settingsPickFolder,
                ),
                onPressed: busy ? null : _pick,
              ),
            ),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(
              icon: const Icon(Icons.restart_alt),
              label: Text(l10n.settingsUseDefaultFolder),
              onPressed: _preparing ? null : _useDefault,
            ),
          ),
        ],
      ),
      actions: <Widget>[
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(l10n.commonCancel),
        ),
        FilledButton(
          // 以输入框为准：用户可能手打路径而不经过"选择"按钮。
          onPressed: () => Navigator.of(context).pop(_controller.text.trim()),
          child: Text(l10n.commonConfirm),
        ),
      ],
    );
  }
}
