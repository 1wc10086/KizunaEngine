import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/core/layout/shell_chrome.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import 'tool_card.dart';

class ToolsScreen extends StatelessWidget {
  const ToolsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.toolsTitle)),
      body: ListView(
        // 悬浮导航栏盖住屏幕底部，列表末尾要留出这段高度，
        // 否则最后一个工具卡片永远滚不到玻璃栏上方。
        padding: EdgeInsets.fromLTRB(
          16,
          16,
          16,
          16 + ShellChrome.bottomInsetOf(context),
        ),
        children: <Widget>[
          ToolCard(
            icon: Icons.translate,
            title: l10n.translationTitle,
            description: l10n.toolsTranslationDesc,
            onTap: () => context.push('/translation'),
          ),
          const SizedBox(height: 8),
          ToolCard(
            icon: Icons.folder_zip_outlined,
            title: l10n.toolsFileProcess,
            description: l10n.toolsFileProcessDesc,
            onTap: () => context.push('/file-process'),
          ),
        ],
      ),
    );
  }
}
