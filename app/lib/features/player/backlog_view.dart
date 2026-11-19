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
    final l10n = AppLocalizations.of(context);
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
                      child: Padding(
                        padding: const EdgeInsets.all(24),
                        child: Text(
                          // 空回想有两种来源：还没读到任何对白、以及引擎根本不提供
                          // 文本列表（Emon 的 TextList 恒为 nil）。都按"读取中无内容"
                          // 处理已经是最中性的文案，单独加"不支持"的说明会让玩家
                          // 误以为是故障；这里保持空状态文本，不区分来源。
                          l10n.commonEmpty,
                          textAlign: TextAlign.center,
                          style: const TextStyle(
                              color: Color(0xFFFFFFFF)),
                        ),
                      ),
                    )
                  : ListView.builder(
                      itemCount: entries.length,
                      itemBuilder: (context, index) {
                        final entry = entries[index];
                        return Padding(
                          padding: const EdgeInsets.symmetric(
                              horizontal: 16, vertical: 6),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: <Widget>[
                              if (entry.id.isNotEmpty)
                                Text(
                                  entry.id,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: Theme.of(context)
                                      .textTheme
                                      .titleSmall
                                      ?.copyWith(
                                          color: const Color(0xFFFFFFFF)),
                                ),
                              Text(
                                entry.text,
                                style: Theme.of(context)
                                    .textTheme
                                    .bodyMedium
                                    ?.copyWith(
                                        color: const Color(0xFFFFFFFF)),
                              ),
                            ],
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
