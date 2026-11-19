import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';
import 'package:kizuna_engine/core/layout/shell_chrome.dart';

class SettingsScaffold extends StatelessWidget {
  const SettingsScaffold({
    super.key,
    required this.title,
    required this.sections,
  });

  final String title;
  final List<AbstractSettingsSection> sections;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(title)),
      body: SettingsList(
        maxWidth: 700,
        // 底部留白 = 常规间距 + 悬浮导航栏遮挡的高度。这个页面既作为
        // shell 的"设置"tab，也被外观/游戏/语言等子页复用；后者不在 shell
        // 子树里，ShellChrome.bottomInsetOf 返回 0，不会多出空白。
        contentPadding: EdgeInsets.fromLTRB(
          0,
          20,
          0,
          20 + ShellChrome.bottomInsetOf(context),
        ),
        sections: sections,
      ),
    );
  }
}
