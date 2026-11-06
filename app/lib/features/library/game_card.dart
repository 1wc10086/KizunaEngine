import 'dart:io';

import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';

/// 游戏库里的一张卡片。
///
/// 需求是一行一张、上大半显示封面、下小半显示游戏名与引擎名：
/// 封面用 16:9 固定比例，窗口再宽也不会把卡片拉得过高；底部信息条高度固定，
/// 保证同一屏里每张卡片的标题基线一致。
class GameCard extends StatelessWidget {
  final ScanEntry entry;
  final VoidCallback onTap;

  const GameCard({super.key, required this.entry, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    final theme = Theme.of(context);
    return Card(
      clipBehavior: Clip.antiAlias,
      margin: EdgeInsets.zero,
      child: InkWell(
        onTap: onTap,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            AspectRatio(
              aspectRatio: 16 / 9,
              child: entry.coverPath.isEmpty
                  ? _CoverPlaceholder(
                      label: entry.name.isEmpty ? l10n.libraryCover : entry.name)
                  : Image.file(
                      File(entry.coverPath),
                      fit: BoxFit.cover,
                      errorBuilder: (context, error, stackTrace) {
                        return _CoverPlaceholder(
                          label: entry.name.isEmpty
                              ? l10n.libraryCover
                              : entry.name,
                        );
                      },
                    ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(14, 10, 14, 12),
              child: Row(
                children: <Widget>[
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Text(
                          entry.name,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: theme.textTheme.titleMedium,
                        ),
                        const SizedBox(height: 2),
                        Text(
                          entry.brandName,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: theme.colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ],
                    ),
                  ),
                  if (entry.hasScript)
                    Icon(
                      Icons.play_circle_outline,
                      color: theme.colorScheme.primary,
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _CoverPlaceholder extends StatelessWidget {
  final String label;

  const _CoverPlaceholder({required this.label});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      color: theme.colorScheme.surfaceContainerHigh,
      alignment: Alignment.center,
      child: Text(
        label.isEmpty ? '?' : label.substring(0, 1),
        style: theme.textTheme.displaySmall,
      ),
    );
  }
}
