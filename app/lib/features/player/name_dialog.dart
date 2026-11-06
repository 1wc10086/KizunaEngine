import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

class NameDialog extends StatefulWidget {
  final String prompt;
  final Future<void> Function(String value) onSubmit;

  const NameDialog({super.key, required this.prompt, required this.onSubmit});

  @override
  State<NameDialog> createState() => _NameDialogState();
}

class _NameDialogState extends State<NameDialog> {
  final TextEditingController _controller = TextEditingController();
  bool _busy = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  Future<void> _submit(String value) async {
    if (_busy) {
      return;
    }
    setState(() {
      _busy = true;
    });
    try {
      await widget.onSubmit(value);
    } finally {
      if (mounted) {
        setState(() {
          _busy = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    return Positioned.fill(
      child: Container(
        color: const Color(0x99000000),
        alignment: Alignment.center,
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 360),
          child: Card(
            margin: const EdgeInsets.all(24),
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  Text(
                    widget.prompt.isEmpty
                        ? l10n.playerNameHint
                        : widget.prompt,
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _controller,
                    autofocus: true,
                    enabled: !_busy,
                    decoration: InputDecoration(
                      hintText: l10n.playerNameHint,
                    ),
                    onSubmitted: (value) {
                      _submit(value.trim());
                    },
                  ),
                  const SizedBox(height: 12),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: <Widget>[
                      TextButton(
                        onPressed: _busy
                            ? null
                            : () {
                                _submit('');
                              },
                        child: Text(l10n.commonCancel),
                      ),
                      const SizedBox(width: 8),
                      FilledButton(
                        onPressed: _busy
                            ? null
                            : () {
                                _submit(_controller.text.trim());
                              },
                        child: Text(l10n.commonConfirm),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
