import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';

class BacklogView extends StatelessWidget {
  final List<TextEntry> entries;
  final VoidCallback onClose;

  const BacklogView(
      {super.key, required this.entries, required this.onClose});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Positioned.fill(
      child: Container(
        color: const Color(0xE6000000),
        child: Column(
          children: <Widget>[
            AppBar(
              backgroundColor: const Color(0x00000000),
              foregroundColor: const Color(0xFFFFFFFF),
              title: Text(l10n.playerBacklog),
              actions: <Widget>[
                IconButton(
                  tooltip: l10n.commonClose,
                  icon: const Icon(Icons.close),
                  onPressed: onClose,
                ),
              ],
            ),
            Expanded(
              child: entries.isEmpty
                  ? Center(
                      child: Text(
                        l10n.commonEmpty,
                        style: const TextStyle(
                            color: Color(0xFFFFFFFF)),
                      ),
                    )
                  : ListView.builder(
                      itemCount: entries.length,
                      itemBuilder: (context, index) {
                        final entry = entries[index];
                        return Padding(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 16, vertical: 6),
                          child: Text(
                            entry.text,
                            style: Theme.of(context)
                                .textTheme
                                .bodyMedium
                                ?.copyWith(
                                    color: const Color(0xFFFFFFFF)),
                          ),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }
}
