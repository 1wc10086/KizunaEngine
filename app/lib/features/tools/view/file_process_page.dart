import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import 'file_process_view.dart';

class FileProcessPage extends StatelessWidget {
  const FileProcessPage({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.toolsFileProcess)),
      body: const FileProcessView(),
    );
  }
}
