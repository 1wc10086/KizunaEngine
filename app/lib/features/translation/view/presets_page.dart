import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../../core/settings/translator_settings.dart';
import '../viewmodel/translation_notifier.dart';

/// 模型预设页：一组 OpenAI 兼容 / Google / Claude 的 API 配置。
///
/// 预设表单里的 4 个 TextField 用同一个 StatefulWidget 持有控制器，
/// 这样 AlertDialog 关闭时能跟着 dispose——以前是在 showDialog 之后
/// 手动 dispose 控制器，异常路径（用户按返回键）会漏。
class TranslationPresetsPage extends ConsumerWidget {
  const TranslationPresetsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final state = ref.watch(translationProvider);
    final notifier = ref.read(translationProvider.notifier);
    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.translationPresets),
        actions: <Widget>[
          IconButton(
            tooltip: l10n.translationAddPreset,
            icon: const Icon(Icons.add),
            onPressed: () => unawaited(_editPreset(context, ref, null)),
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
                      '${apiSpecLabel(l10n, preset.apiSpec)} · ${preset.model}',
                    ),
                    onTap: state.isRunning
                        ? null
                        : () => notifier.selectPreset(preset.id),
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: <Widget>[
                        IconButton(
                          tooltip: l10n.translationEditPreset,
                          icon: const Icon(Icons.edit),
                          onPressed: state.isRunning
                              ? null
                              : () =>
                                  unawaited(_editPreset(context, ref, preset)),
                        ),
                        IconButton(
                          tooltip: l10n.translationDeletePreset,
                          icon: const Icon(Icons.delete_outline),
                          onPressed: state.isRunning
                              ? null
                              : () =>
                                  unawaited(_confirmDelete(context, ref, preset)),
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
    );
  }

  Future<void> _confirmDelete(
    BuildContext context,
    WidgetRef ref,
    TranslatorPreset preset,
  ) async {
    final l10n = AppLocalizations.of(context);
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text(l10n.translationDeletePreset),
        content: Text(l10n.translationDeletePresetConfirm),
        actions: <Widget>[
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: Text(l10n.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text(l10n.commonConfirm),
          ),
        ],
      ),
    );
    if (confirmed == true) {
      await ref.read(translationProvider.notifier).removePreset(preset.id);
    }
  }

  Future<void> _editPreset(
    BuildContext context,
    WidgetRef ref,
    TranslatorPreset? existing,
  ) async {
    final result = await showDialog<TranslatorPreset>(
      context: context,
      builder: (_) => _PresetDialog(existing: existing),
    );
    if (result == null) {
      return;
    }
    final notifier = ref.read(translationProvider.notifier);
    if (existing == null) {
      await notifier.addPreset(result);
    } else {
      await notifier.updatePreset(result);
    }
  }
}

/// API 类型 → 文案。数值来自 TranslatorPreset.apiSpec（0/1/2），
/// 与内核约定的 provider.kind 顺序一致（openai/google/claude）。
String apiSpecLabel(AppLocalizations l10n, int apiSpec) {
  switch (apiSpec) {
    case 1:
      return l10n.translationApiGoogle;
    case 2:
      return l10n.translationApiClaude;
    default:
      return l10n.translationApiOpenAI;
  }
}

class _PresetDialog extends StatefulWidget {
  final TranslatorPreset? existing;

  const _PresetDialog({required this.existing});

  @override
  State<_PresetDialog> createState() => _PresetDialogState();
}

class _PresetDialogState extends State<_PresetDialog> {
  late final TextEditingController _name =
      TextEditingController(text: widget.existing?.name ?? '');
  late final TextEditingController _baseUrl =
      TextEditingController(text: widget.existing?.baseUrl ?? '');
  late final TextEditingController _model =
      TextEditingController(text: widget.existing?.model ?? '');
  late final TextEditingController _apiKey =
      TextEditingController(text: widget.existing?.apiKey ?? '');
  late int _apiSpec = widget.existing?.apiSpec ?? 0;

  @override
  void dispose() {
    _name.dispose();
    _baseUrl.dispose();
    _model.dispose();
    _apiKey.dispose();
    super.dispose();
  }

  void _submit() {
    final name = _name.text.trim();
    // 名称是下拉框里唯一能看的内容，空名字等于一个选不中的选项。
    if (name.isEmpty) {
      return;
    }
    Navigator.of(context).pop(TranslatorPreset(
      // 新建时用时间戳当 id：预设没有服务端，本地唯一即可。
      id: widget.existing?.id ??
          DateTime.now().microsecondsSinceEpoch.toString(),
      name: name,
      apiSpec: _apiSpec,
      baseUrl: _baseUrl.text.trim(),
      model: _model.text.trim(),
      apiKey: _apiKey.text.trim(),
    ));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return AlertDialog(
      title: Text(widget.existing == null
          ? l10n.translationAddPreset
          : l10n.translationEditPreset),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            TextField(
              controller: _name,
              decoration:
                  InputDecoration(labelText: l10n.translationPresetName),
            ),
            const SizedBox(height: 12),
            DropdownButtonFormField<int>(
              initialValue: _apiSpec,
              decoration:
                  InputDecoration(labelText: l10n.translationApiSpec),
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
                    _apiSpec = value;
                  });
                }
              },
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _baseUrl,
              decoration: InputDecoration(labelText: l10n.translationBaseUrl),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _model,
              decoration: InputDecoration(labelText: l10n.translationModel),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: _apiKey,
              obscureText: true,
              decoration: InputDecoration(labelText: l10n.translationApiKey),
            ),
          ],
        ),
      ),
      actions: <Widget>[
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text(l10n.commonCancel),
        ),
        FilledButton(onPressed: _submit, child: Text(l10n.commonConfirm)),
      ],
    );
  }
}
