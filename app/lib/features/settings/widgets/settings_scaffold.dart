import 'package:card_settings_ui/card_settings_ui.dart';
import 'package:flutter/material.dart';

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
      body: SettingsList(maxWidth: 700, sections: sections),
    );
  }
}
