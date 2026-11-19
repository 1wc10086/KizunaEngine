import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../../core/bridge/kernel_bridge.dart';
import '../viewmodel/file_process_notifier.dart';

/// 文件处理视图：输入路径 → 防抖探测候选动作 → 选一个 → 执行。
///
/// 两个 TextField 用控制器是因为"输出路径"会被 viewmodel 改写（选中候选时
/// 自动填建议路径），不能用 ValueKey+初始值那种一次性写法，否则用户改了
/// 候选、输入框不会跟着变。onChanged 里用 _syncingOutput 挡住回写造成
/// 的二次 setInput。
class FileProcessView extends ConsumerStatefulWidget {
  const FileProcessView({super.key});

  @override
  ConsumerState<FileProcessView> createState() => _FileProcessViewState();
}

class _FileProcessViewState extends ConsumerState<FileProcessView> {
  late final TextEditingController _inputController;
  late final TextEditingController _outputController;
  bool _syncingOutput = false;

  @override
  void initState() {
    super.initState();
    _inputController = TextEditingController();
    _outputController = TextEditingController();
  }

  @override
  void dispose() {
    _inputController.dispose();
    _outputController.dispose();
    super.dispose();
  }

  Future<void> _run() async {
    final l10n = AppLocalizations.of(context);
    final notifier = ref.read(fileProcessProvider.notifier);
    final result = await notifier.start();
    if (!mounted) {
      return;
    }
    if (result == null) {
      final error = ref.read(fileProcessProvider).errorMessage;
      // errorMessage 非空说明内核报错，否则就是输入没填全——两种提示
      // 不一样，混成一句会让玩家以为是自己填错了。
      _snack(error.isNotEmpty ? error : l10n.toolsNeedInput);
      return;
    }
    _snack(l10n.toolsDoneElapsed(result.elapsedMs));
  }

  void _snack(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    ref.listen(
      fileProcessProvider.select((value) => value.outputPath),
      (previous, next) {
        if (next != previous && _outputController.text != next) {
          _syncingOutput = true;
          _outputController.text = next;
          _syncingOutput = false;
        }
      },
    );
    final state = ref.watch(fileProcessProvider);
    final notifier = ref.read(fileProcessProvider.notifier);
    final theme = Theme.of(context);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: <Widget>[
        TextField(
          controller: _inputController,
          decoration: InputDecoration(
            labelText: l10n.toolsInput,
            hintText: l10n.toolsInputHint,
          ),
          onChanged: notifier.setInput,
        ),
        const SizedBox(height: 12),
        if (state.detectMessage.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(state.detectMessage),
          ),
        if (state.errorMessage.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(
              state.errorMessage,
              style: TextStyle(color: theme.colorScheme.error),
            ),
          ),
        DropdownButtonFormField<int>(
          initialValue: state.selectedIndex >= 0 &&
                  state.selectedIndex < state.candidates.length
              ? state.selectedIndex
              : null,
          hint: Text(state.candidates.isEmpty
              ? l10n.toolsNoCandidates
              : l10n.toolsSelectCandidate),
          items: <DropdownMenuItem<int>>[
            for (var i = 0; i < state.candidates.length; i++)
              DropdownMenuItem<int>(
                value: i,
                child: Text(state.candidates[i].label),
              ),
          ],
          onChanged: state.isBusy
              ? null
              : (index) {
                  if (index != null) {
                    notifier.selectCandidate(index);
                  }
                },
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _outputController,
          decoration: InputDecoration(labelText: l10n.toolsOutput),
          onChanged: (value) {
            if (!_syncingOutput) {
              notifier.setOutput(value);
            }
          },
        ),
        const SizedBox(height: 16),
        FilledButton(
          onPressed: state.isBusy
              ? null
              : () {
                  _run();
                },
          child: state.isBusy
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : Text(l10n.toolsProcess),
        ),
        if (state.result != null) ...<Widget>[
          const SizedBox(height: 16),
          _ResultCard(result: state.result!),
        ],
      ],
    );
  }
}

class _ResultCard extends StatelessWidget {
  final KernelProcessResult result;

  const _ResultCard({required this.result});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            // summary 是内核给的人话（"解包完成：1234 个文件"），空的时候
            // 退回通用的“完成”，不让标题行空着。
            Text(
              result.summary.isEmpty ? l10n.toolsDone : result.summary,
              style: Theme.of(context).textTheme.titleSmall,
            ),
            const SizedBox(height: 8),
            Text('${l10n.toolsInput}: ${result.inputPath}'),
            Text('${l10n.toolsOutput}: ${result.outputPath}'),
            Text(
              '${l10n.toolsBytes}: '
              '${result.bytesIn} ${l10n.toolsArrow} ${result.bytesOut}',
            ),
            Text(
              '${l10n.toolsElapsed}: ${result.elapsedMs} ${l10n.toolsMillis}',
            ),
            // details 是内核给的附加统计（按文件类型计数等），键是数据不是
            // 界面文案，不做翻译——翻译了反而对不上日志。
            for (final entry in result.details.entries)
              Text('${entry.key}: ${entry.value}'),
          ],
        ),
      ),
    );
  }
}
