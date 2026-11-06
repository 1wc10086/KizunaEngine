import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';

class ChoicesOverlay extends StatelessWidget {
  final List<EngineChoice> choices;
  final Future<void> Function(int index) onChoose;

  const ChoicesOverlay(
      {super.key, required this.choices, required this.onChoose});

  @override
  Widget build(BuildContext context) {
    if (choices.isEmpty) {
      return const SizedBox.shrink();
    }
    final l10n = AppLocalizations.of(context)!;
    return Positioned.fill(
      child: Container(
        color: const Color(0x66000000),
        alignment: Alignment.center,
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 420),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              Padding(
                padding:
                    const EdgeInsets.only(left: 24, right: 24, bottom: 8),
                child: Text(
                  l10n.playerChoices,
                  textAlign: TextAlign.center,
                  style: Theme.of(context)
                      .textTheme
                      .titleMedium
                      ?.copyWith(color: const Color(0xFFFFFFFF)),
                ),
              ),
              for (var i = 0; i < choices.length; i++)
                Padding(
                  padding: const EdgeInsets.symmetric(
                      horizontal: 24, vertical: 4),
                  child: FilledButton.tonal(
                    onPressed: () {
                      onChoose(i);
                    },
                    child: Text(
                      choices[i].name,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
