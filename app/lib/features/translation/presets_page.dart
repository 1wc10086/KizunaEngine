import 'dart:async';

import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/settings/settings.dart';
import 'translation_notifier.dart';

class TranslationPresetsPage extends ConsumerWidget {
  const TranslationPresetsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context)!;
    final state = ref.watch(translationProvider);
    final notifier = ref.read(translationProvider.notifier);
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.translationPresets),
        actions: <Widget>[
          IconButton(
            tooltip: l10n.translationAddPreset,
            icon: const Icon(Icons.add),
            onPressed: () {
              _presetDialog(context, ref, null);
            },
          ),
        ],
      ),
      body: state.presets.isEmpty
          ? Center(child: Text(l10n.translationNoPresets))
          : ListView.builder(
              padding: const EdgeInsets.all(16),
              itemCount: state.presets.length,
              itemBuilder: (context, index) {
                final preset = state.presets[index];
                return Card(
                  child: ListTile(
                    selected: preset.id == state.selectedPresetId,
                    title: Text(preset.name),
                    subtitle: Text(
                        '${_apiSpecText(l10n, preset.apiSpec)} · ${preset.model}'),
                    onTap: state.isRunning
                        ? null
                        : () {
                            notifier.selectPreset(preset.id);
                          },
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: <Widget>[
                        IconButton(
                          tooltip: l10n.translationEditPreset,
                          icon: const Icon(Icons.edit),
                          onPressed: state.isRunning
                              ? null
                              : () {
                                  _presetDialog(context, ref, preset);
                                },
                        ),
                        IconButton(
                          tooltip: l10n.translationDeletePreset,
                          icon: const Icon(Icons.delete_outline),
                          onPressed: state.isRunning
                              ? null
                              : () async {
                                  final confirmed =
                                      await _confirmDelete(context);
                                  if (confirmed && context.mounted) {
                                    notifier.removePreset(preset.id);
                                  }
                                },
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
    );
  }

  String _apiSpecText(AppLocalizations l10n, int apiSpec) {
    switch (apiSpec) {
      case 1:
        return l10n.translationApiGoogle;
      case 2:
        return l10n.translationApiClaude;
      default:
        return l10n.translationApiOpenAI;
    }
  }

  Future<bool> _confirmDelete(BuildContext context) async {
    final l10n = AppLocalizations.of(context)!;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return AlertDialog(
          title: Text(l10n.translationDeletePreset),
          content: Text(l10n.translationDeletePresetConfirm),
          actions: <Widget>[
            TextButton(
              onPressed: () {
                Navigator.of(context).pop(false);
              },
              child: Text(l10n.commonCancel),
            ),
            FilledButton(
              onPressed: () {
                Navigator.of(context).pop(true);
              },
              child: Text(l10n.commonConfirm),
            ),
          ],
        );
      },
    );
    return confirmed ?? false;
  }

  Future<void> _presetDialog(
      BuildContext context, WidgetRef ref, TranslatorPreset? existing) async {
    final l10n = AppLocalizations.of(context)!;
    final nameController = TextEditingController(text: existing?.name ?? '');
    final baseUrlController =
        TextEditingController(text: existing?.baseUrl ?? '');
    final modelController = TextEditingController(text: existing?.model ?? '');
    final apiKeyController =
        TextEditingController(text: existing?.apiKey ?? '');
    int apiSpec = existing?.apiSpec ?? 0;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) {
        return StatefulBuilder(
          builder: (context, setState) {
            return AlertDialog(
              title: Text(existing == null
                  ? l10n.translationAddPreset
                  : l10n.translationEditPreset),
              content: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    TextField(
                      controller: nameController,
                      decoration: InputDecoration(
                          labelText: l10n.translationPresetName),
                    ),
                    const SizedBox(height: 12),
                    DropdownButtonFormField<int>(
                      value: apiSpec,
                      decoration: InputDecoration(
                          labelText: l10n.translationApiSpec),
                      items: <DropdownMenuItem<int>>[
                        DropdownMenuItem<int>(
                          value: 0,
                          child: Text(l10n.translationApiOpenAI),
                        ),
                        DropdownMenuItem<int>(
                          value: 1,
                          child: Text(l10n.translationApiGoogle),
                        ),
                        DropdownMenuItem<int>(
                          value: 2,
                          child: Text(l10n.translationApiClaude),
                        ),
                      ],
                      onChanged: (value) {
                        if (value != null) {
                          setState(() {
                            apiSpec = value;
                          });
                        }
                      },
                    ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: baseUrlController,
                      decoration: InputDecoration(
                          labelText: l10n.translationBaseUrl),
                    ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: modelController,
                      decoration: InputDecoration(
                          labelText: l10n.translationModel),
                    ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: apiKeyController,
                      obscureText: true,
                      decoration: InputDecoration(
                          labelText: l10n.translationApiKey),
                    ),
                  ],
                ),
              ),
              actions: <Widget>[
                TextButton(
                  onPressed: () {
                    Navigator.of(context).pop(false);
                  },
                  child: Text(l10n.commonCancel),
                ),
                FilledButton(
                  onPressed: () {
                    Navigator.of(context).pop(true);
                  },
                  child: Text(l10n.commonConfirm),
                ),
              ],
            );
          },
        );
      },
    );
    final name = nameController.text.trim();
    final baseUrl = baseUrlController.text.trim();
    final model = modelController.text.trim();
    final apiKey = apiKeyController.text.trim();
    nameController.dispose();
    baseUrlController.dispose();
    modelController.dispose();
    apiKeyController.dispose();
    if (confirmed != true || !context.mounted) {
      return;
    }
    if (name.isEmpty) {
      return;
    }
    final notifier = ref.read(translationProvider.notifier);
    if (existing == null) {
      await notifier.addPreset(TranslatorPreset(
        id: DateTime.now().microsecondsSinceEpoch.toString(),
        name: name,
        apiSpec: apiSpec,
        baseUrl: baseUrl,
        model: model,
        apiKey: apiKey,
      ));
    } else {
      await notifier.updatePreset(TranslatorPreset(
        id: existing.id,
        name: name,
        apiSpec: apiSpec,
        baseUrl: baseUrl,
        model: model,
        apiKey: apiKey,
      ));
    }
  }
}
