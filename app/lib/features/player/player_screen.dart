import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/audio/game_audio.dart' as audio_core;
import '../../core/bridge/engine_bridge.dart';
import '../../core/bridge/protocol_consts.dart';
import '../../core/platform/app_paths.dart';
import '../../core/settings/settings.dart';
import '../../core/widgets/kizuna_loading.dart';
import '../settings/settings_notifier.dart';
import 'controls.dart';
import 'player_chrome.dart';
import 'player_notifier.dart';
import 'player_orientation.dart';

class PlayerScreen extends ConsumerStatefulWidget {
  final String dir;
  final ScanEntry? entry;

  const PlayerScreen({super.key, required this.dir, this.entry});

  @override
  ConsumerState<PlayerScreen> createState() => _PlayerScreenState();
}

class _PlayerScreenState extends ConsumerState<PlayerScreen> {
  static const Duration _hideDelay = Duration(seconds: 4);

  bool _landscape = false;
  bool _toolbarVisible = true;
  Orientation? _orientation;
  Timer? _hideTimer;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final audio = ref.read(globalAudioProvider);
      audio.onCompleted = (file, voice, channel) {
        ref.read(playerSessionProvider)?.audioDone(
              file: file,
              voice: voice,
              channel: channel,
            );
      };
      ref.read(playerProvider.notifier).attachAudio(_RealAudioAdapter(audio));
      final current = ref.read(playerProvider);
      if (!current.ready && current.error == null) {
        _start();
      }
    });
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final orientation = MediaQuery.of(context).orientation;
    if (orientation == _orientation) {
      return;
    }
    _orientation = orientation;
    _landscape = orientation == Orientation.landscape;
    _toolbarVisible = true;
    _scheduleHide();
  }

  @override
  void dispose() {
    _hideTimer?.cancel();
    _hideTimer = null;
    enterPortrait();
    restoreSystemUi();
    super.dispose();
  }

  Future<void> _start() async {
    if (!mounted) {
      return;
    }
    final entry = widget.entry;
    // 缓存目录准备（appCacheRoot 的 mkdir）与引擎 isolate 启动互不依赖：
    // 这里先发起但不等，notifier 在 spawn 之后再收口，两边重叠执行；
    // 失败（无平台实现/无权限）回退空串，引擎走自己的临时目录策略。
    final cacheDir = _prepareCacheDir();
    await ref.read(playerProvider.notifier).start(
          widget.dir,
          brandId: entry?.brandId ?? '',
          extraDirs: entry?.extraDirs ?? const <String>[],
          cacheDir: cacheDir,
        );
  }

  /// 准备缓存根目录：`<appSupport>/cache/kizuna`。
  ///
  /// 这里只给"根"，每个游戏自己的子目录由引擎按游戏目录的 md5 建
  /// （见 Go 侧 core.GameCacheDir），最终路径是 `cache/kizuna/<md5>/`：
  /// 同一部游戏每次游玩都落到同一个目录，不同游戏的音频物化文件、
  /// engine.log 互不覆盖，卸载游戏时也能整目录删掉。
  ///
  /// 用应用私有缓存而不是临时目录：临时目录会被系统在存储紧张时清掉，
  /// 玩家下次进游戏就得重新物化几百 MB 音频；私有目录跟着应用走，卸载时才删。
  Future<String> _prepareCacheDir() async {
    try {
      return (await appCacheRoot()).path;
    } catch (_) {
      return '';
    }
  }

  void _scheduleHide() {
    _hideTimer?.cancel();
    _hideTimer = null;
    if (!_landscape || !_toolbarVisible) {
      return;
    }
    _hideTimer = Timer(_hideDelay, () {
      if (!mounted) {
        return;
      }
      setState(() {
        _toolbarVisible = false;
      });
    });
  }

  void _toggleToolbar() {
    setState(() {
      _toolbarVisible = !_toolbarVisible;
    });
    _scheduleHide();
  }

  void _toggleOrientation() {
    final next = !_landscape;
    setState(() {
      _landscape = next;
      _toolbarVisible = true;
    });
    if (next) {
      enterLandscape();
    } else {
      enterPortrait();
    }
    _scheduleHide();
  }

  void _exit() {
    _hideTimer?.cancel();
    _hideTimer = null;
    enterPortrait();
    restoreSystemUi();
    context.pop();
  }

  Future<void> _editBuexMessage() async {
    final l10n = AppLocalizations.of(context)!;
    final controller = TextEditingController();
    final result = await showDialog<String>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text(l10n.buexEditMessage),
        content: TextField(
          controller: controller,
          autofocus: true,
          maxLines: 4,
          decoration: InputDecoration(hintText: l10n.buexEditHint),
        ),
        actions: <Widget>[
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: Text(l10n.commonCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(controller.text),
            child: Text(l10n.commonConfirm),
          ),
        ],
      ),
    );
    controller.dispose();
    if (result != null && mounted) {
      await ref.read(playerProvider.notifier).submitName(result);
    }
  }

  /// 宿主兜底存档/读档面板。
  ///
  /// 作品没有 [data screen] 布局时（例如 Ancient 的《特別病棟》），原版存档界面
  /// 无法由引擎按配置绘制。这里直接用引擎的 slots/save/load op，至少保证玩家
  /// 能完成“存档 -> 退出 -> 读档继续”的完整流程；有原版界面的作品不会走到这里。
  Future<void> _showSlots({required bool saveMode}) async {
    final session = ref.read(playerSessionProvider);
    if (session == null) {
      return;
    }
    final l10n = AppLocalizations.of(context)!;
    final slots = await session.slots();
    if (!mounted) {
      return;
    }
    final picked = await showDialog<int>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: Text(saveMode ? l10n.playerSave : l10n.playerLoad),
          content: SizedBox(
            width: 560,
            height: 420,
            child: GridView.builder(
              gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: 2,
                mainAxisExtent: 96,
                crossAxisSpacing: 8,
                mainAxisSpacing: 8,
              ),
              itemCount: slots.length,
              itemBuilder: (context, index) {
                final slot = slots[index];
                final enabled = saveMode || slot.exists;
                return Card(
                  margin: EdgeInsets.zero,
                  child: InkWell(
                    onTap: enabled
                        ? () => Navigator.of(dialogContext).pop(slot.index)
                        : null,
                    child: Padding(
                      padding: const EdgeInsets.all(10),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          Text(
                            '${slot.index + 1}. ${slot.exists ? slot.title : l10n.playerSlotEmpty}',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context).textTheme.titleSmall,
                          ),
                          const SizedBox(height: 4),
                          Text(
                            slot.exists ? slot.savedAt : '',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                          const SizedBox(height: 2),
                          Text(
                            slot.exists ? slot.summary : '',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context).textTheme.bodySmall,
                          ),
                        ],
                      ),
                    ),
                  ),
                );
              },
            ),
          ),
          actions: <Widget>[
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: Text(l10n.commonCancel),
            ),
          ],
        );
      },
    );
    if (picked == null) {
      return;
    }
    if (saveMode) {
      final saved = await session.save(picked, l10n.playerSlotSaveTitle);
      if (!saved && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.commonError)),
        );
      }
    } else {
      await session.load(picked);
    }
  }

  Widget _buildControls(PlayerNotifier notifier) {
    final auto = ref.watch(playerProvider.select((value) => value.auto));
    final skip = ref.watch(playerProvider.select((value) => value.skip));
    return PlayerControls(
      auto: auto,
      skip: skip,
      onAuto: notifier.setAuto,
      onSkip: notifier.setSkip,
      onMenu: () => notifier.sendKey(27),
      onExit: _exit,
      landscape: _landscape,
      onToggleOrientation: _toggleOrientation,
    );
  }

  Widget _portraitBody(Widget stage, Widget controls) {
    return SafeArea(
      child: Column(
        children: <Widget>[
          Expanded(child: stage),
          controls,
        ],
      ),
    );
  }

  Widget _landscapeBody(Widget stage, Widget controls) {
    final engineWidth =
        ref.watch(playerProvider.select((value) => value.width));
    final engineHeight =
        ref.watch(playerProvider.select((value) => value.height));
    return LayoutBuilder(
      builder: (context, constraints) {
        final ratio = engineWidth > 0 && engineHeight > 0
            ? engineWidth / engineHeight
            : 16 / 9;
        var pictureWidth = constraints.maxWidth;
        var pictureHeight = pictureWidth / ratio;
        if (pictureHeight > constraints.maxHeight) {
          pictureHeight = constraints.maxHeight;
          pictureWidth = pictureHeight * ratio;
        }
        final side = (constraints.maxWidth - pictureWidth) / 2;
        final double hotWidth = side > 0 ? side : 32;
        return Stack(
          children: <Widget>[
            Positioned.fill(child: stage),
            Positioned(
              left: 0,
              top: 0,
              bottom: 0,
              width: hotWidth,
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: _toggleToolbar,
              ),
            ),
            Positioned(
              right: 0,
              top: 0,
              bottom: 0,
              width: hotWidth,
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: _toggleToolbar,
              ),
            ),
            if (_toolbarVisible)
              Positioned(
                left: 0,
                right: 0,
                bottom: 0,
                child: Listener(
                  behavior: HitTestBehavior.opaque,
                  onPointerDown: (_) {
                    _scheduleHide();
                  },
                  child: controls,
                ),
              ),
          ],
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context)!;
    ref.listen(
      playerProvider.select((value) => value.error),
      (previous, next) {
        if (next != null && next != previous && context.mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text(next)),
          );
        }
      },
    );
    // 引擎要打开 Flutter 设置（标题/游戏菜单的"環境設定"）或退出播放页时，
    // 只能发一个宿主动作；路由属于页面层，在播放页收口。
    ref.listen(
      playerProvider.select((value) => value.hostRequestSerial),
      (previous, next) {
        if (next == previous) {
          return;
        }
        final request = ref.read(playerProvider).hostRequest;
        if (!context.mounted) {
          return;
        }
        switch (request) {
          case 'settings':
            context.push('/settings');
          case 'exit':
            _exit();
          case 'auto':
            {
              final current = ref.read(playerProvider).auto;
              ref.read(playerProvider.notifier).setAuto(!current);
            }
          case 'skip':
            {
              final current = ref.read(playerProvider).skip;
              ref.read(playerProvider.notifier).setSkip(!current);
            }
          case 'backlog':
            ref.read(playerProvider.notifier).openBacklog();
          case 'edit_message':
            _editBuexMessage();
          case 'save':
            _showSlots(saveMode: true);
          case 'load':
            _showSlots(saveMode: false);
        }
      },
    );
    ref.listen<EngineBridge?>(
      playerSessionProvider,
      (previous, next) {
        if (next != null && previous == null) {
          final settings =
              ref.read(appSettingsProvider).value ?? const AppSettings();
          next.setOption(
              KzOption.showMessageFrame, settings.showMessageFrame);
        }
      },
    );
    // 横屏时底部工具条是叠在画面上的（68dp 高）。标题菜单和对话框正文都在
    // 画面下沿——标题按钮的 y=660..700、正文到 706——工具条会把它们整段盖住。
    // 进入标题/播片时立即收起工具条，让引擎自己的画面优先；玩家仍可点屏幕
    // 左右边缘把它临时呼出。引擎自绘消息框（BDT/manin）同样占据 360..480，
    // 且框內已经有 SAVE/LOAD/SKIP/AUTO/LOG/SYS，这里也必须收起，否则会盖住
    // 对话框和工具按钮。
    ref.listen(
      playerProvider.select((value) =>
          value.titleMenu.isNotEmpty ||
          value.moviePath.isNotEmpty ||
          value.engineMessageWindow),
      (previous, next) {
        if (!next || previous == next || !_landscape) {
          return;
        }
        _hideTimer?.cancel();
        if (!mounted || !_toolbarVisible) {
          return;
        }
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (mounted && _toolbarVisible) {
            setState(() {
              _toolbarVisible = false;
            });
          }
        });
      },
    );
    final ready = ref.watch(playerProvider.select((value) => value.ready));
    final error = ref.watch(playerProvider.select((value) => value.error));
    final notifier = ref.read(playerProvider.notifier);
    final stage = ready
        ? PlayerStage(
            notifier: notifier,
            onExit: _exit,
          )
        : error != null
            ? _ErrorView(
                l10n: l10n,
                message: error,
                onRetry: _start,
                onBack: _exit,
              )
            : _Loading(l10n: l10n);
    final controls = _buildControls(notifier);
    return Scaffold(
      backgroundColor: const Color(0xFF000000),
      body: _landscape
          ? _landscapeBody(stage, controls)
          : _portraitBody(stage, controls),
    );
  }
}

class _Loading extends StatelessWidget {
  final AppLocalizations l10n;

  const _Loading({required this.l10n});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          KizunaLoadingIndicator(semanticsLabel: l10n.commonLoading),
          const SizedBox(height: 12),
          Text(
            l10n.commonLoading,
            style: const TextStyle(color: Color(0xFFFFFFFF)),
          ),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  final AppLocalizations l10n;
  final String message;
  final VoidCallback onRetry;
  final VoidCallback onBack;

  const _ErrorView({
    required this.l10n,
    required this.message,
    required this.onRetry,
    required this.onBack,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Text(
              l10n.commonError,
              style: const TextStyle(
                color: Color(0xFFFFFFFF),
                fontSize: 18,
                fontWeight: FontWeight.w600,
              ),
            ),
            const SizedBox(height: 12),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: Color(0xFFB0B0B0), fontSize: 13),
            ),
            const SizedBox(height: 20),
            Row(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                TextButton(
                  onPressed: onBack,
                  child: Text(l10n.commonBack),
                ),
                const SizedBox(width: 12),
                FilledButton(
                  onPressed: onRetry,
                  child: Text(l10n.commonRetry),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _RealAudioAdapter implements GameAudio {
  final audio_core.GameAudio _inner;

  _RealAudioAdapter(this._inner);

  @override
  Future<void> dispatchEvent(AudioEvent event) => _inner.dispatchEvent(event);

  @override
  Future<void> dispose() async {}
}
