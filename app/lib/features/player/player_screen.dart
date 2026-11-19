import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/audio/game_audio.dart' as audio_core;
import '../../core/bridge/engine_bridge.dart';
import '../../core/bridge/protocol_consts.dart';
import '../../core/platform/app_paths.dart';
import '../../core/platform/external_links.dart';
import '../../core/platform/platform_info.dart';
import '../../core/settings/settings_snapshot.dart';
import '../../core/widgets/kizuna_loading.dart';
import '../settings/viewmodel/settings_notifier.dart';
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

  /// 播放页默认横屏（需求：进游戏就是全屏横屏画面）。
  ///
  /// 初值给 true 是为了"第一帧就对"：initState 里发出横屏请求，但平台把
  /// 屏幕转过来要一两帧，这期间 MediaQuery 仍是竖屏。若按 MediaQuery 决定
  /// 布局，用户会先看到一帧竖屏工具条再被掰成横屏。
  bool _landscape = true;
  bool _toolbarVisible = true;

  /// 还在等平台真正切到横屏。为 true 时忽略"竖屏"的 MediaQuery 结果，
  /// 避免上面说的闪一帧；一旦看到横屏就清除。
  ///
  /// 不能无限等：Android 16 起大屏会忽略应用的方向锁，折叠屏/平板也可能
  /// 一直保持竖屏。[_landscapeWait] 到期就接受真实朝向，避免永远按横屏
  /// 布局渲染一个竖屏窗口。
  bool _awaitingLandscape = true;
  Timer? _landscapeWait;
  Orientation? _orientation;
  Timer? _hideTimer;

  @override
  void initState() {
    super.initState();
    // 姿态请求放在首帧前：整个播放页从出现起就是横屏沉浸式。
    if (isMobilePlatform) {
      unawaited(enterPlayerLandscape());
      // 600ms 后无论平台有没有真的转过来都接受真实方向：横屏请求在某些
      // 设备上会被系统忽略，一直按横屏排版只会得到一块竖着的黑边画面。
      _landscapeWait = Timer(const Duration(milliseconds: 600), () {
        if (!mounted || !_awaitingLandscape) {
          return;
        }
        setState(() {
          _awaitingLandscape = false;
          _landscape = MediaQuery.orientationOf(context) == Orientation.landscape;
          _toolbarVisible = true;
        });
        _scheduleHide();
      });
    } else {
      _awaitingLandscape = false;
    }
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
    if (_awaitingLandscape) {
      if (orientation != Orientation.landscape) {
        return;
      }
      _awaitingLandscape = false;
      _landscapeWait?.cancel();
      _landscapeWait = null;
    }
    _landscape = orientation == Orientation.landscape;
    _toolbarVisible = true;
    _scheduleHide();
  }

  @override
  void dispose() {
    _hideTimer?.cancel();
    _hideTimer = null;
    _landscapeWait?.cancel();
    _landscapeWait = null;
    unawaited(leavePlayer());
    unawaited(restoreSystemUi());
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
    // 只有横屏自动收起：横屏是沉浸式全屏，工具条会盖住引擎画在底部的内容；
    // 竖屏保留常驻工具条（画面本来就窄，收起后玩家找不到入口）。
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
      // 用户手动切了方向：之后以真实朝向为准，不再等"自动横屏"。
      _awaitingLandscape = false;
      _toolbarVisible = true;
    });
    if (next) {
      unawaited(enterPlayerLandscape());
    } else {
      unawaited(enterPortrait());
    }
    _scheduleHide();
  }

  void _exit() {
    _hideTimer?.cancel();
    _hideTimer = null;
    unawaited(leavePlayer());
    unawaited(restoreSystemUi());
    context.pop();
  }

  Future<void> _editBuexMessage() async {
    final l10n = AppLocalizations.of(context);
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
  /// 只在"引擎没有原版存档界面"时使用：本作（TMR-Hiro）的存读档由引擎按
  /// sys_save_base/sys_load_base 与 [SAVE]/[LOAD] 布局自绘（含缩略图、页签、
  /// 自动存档/删档按钮），宿主动作 save/load 已经不再触发这里。保留它是因为
  /// 其它品牌（如 Bishop Ancient）仍然只有这套兜底面板。
  Future<void> _showSlots({required bool saveMode}) async {
    final session = ref.read(playerSessionProvider);
    if (session == null) {
      return;
    }
    final l10n = AppLocalizations.of(context);
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
                            // 序号用列表位置而不是 slot.index：各品牌引擎的槽位号
                            // 起始值不一致（有的 0 起、有的 1 起），直接 +1 显示会
                            // 差一位；列表位置永远是从 1 开始的连续编号。
                            l10n.playerSlotLabel(
                              index + 1,
                              slot.exists ? slot.title : l10n.playerSlotEmpty,
                            ),
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
      final saved = await ref
          .read(playerProvider.notifier)
          .saveSlot(picked, title: l10n.playerSlotSaveTitle);
      if (!saved && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(l10n.commonError)),
        );
      }
    } else {
      await ref.read(playerProvider.notifier).loadSlot(picked);
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

  /// 竖屏下的布局。
  ///
  /// 竖屏不是默认游玩姿态（进游戏即横屏），这里保持"画面上、工具条下"的
  /// 常驻布局：竖屏画面窄，工具条叠上去会挡住引擎底部对话框的大半。
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
    final l10n = AppLocalizations.of(context);
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
        // 脚本 #Run 的链接请求：原引擎用 ShellExecute 开浏览器，这里在 switch
        // 之前拦截 url: 前缀的请求，交给系统浏览器打开，不要落到界面动作分支。
        if (request.startsWith('url:')) {
          unawaited(openExternalUrl(request.substring(4)));
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
            unawaited(_editBuexMessage());
          // 存档/读档界面由引擎自绘（sys_save_base + sys_save_button，与原版
          // 同一套布局与缩略图）。宿主只在"引擎明确不画对话框"的作品上才需要
          // 自己的面板：engineMessageWindow 为真表示该品牌的所有界面都由引擎
          // 绘制，此时叠一层 Flutter 面板会盖住原版界面（甚至重复存档）。
          case 'save':
            if (!ref.read(playerProvider).engineMessageWindow) {
              unawaited(_showSlots(saveMode: true));
            }
          case 'load':
            if (!ref.read(playerProvider).engineMessageWindow) {
              unawaited(_showSlots(saveMode: false));
            }
        }
      },
    );
    ref.listen<EngineBridge?>(
      playerSessionProvider,
      (previous, next) {
        if (next != null && previous == null) {
          final settings =
              ref.read(appSettingsProvider).valueOrDefault;
          next.setOption(
              KzOption.showMessageFrame, settings.showMessageFrame);
        }
      },
    );
    // 会话已存在时玩家改设置，也要把开关推给引擎（否则要退出重进才生效）。
    // 这条监听放在播放页而不是设置层：设置层 import 播放层会让两个 feature
    // 形成环，而且引擎没跑时这次调用本来就没有意义。
    ref.listen(
      appSettingsProvider.select(
        (value) => value.valueOrDefault.showMessageFrame,
      ),
      (previous, next) {
        if (previous == next) {
          return;
        }
        final session = ref.read(playerSessionProvider);
        if (session != null) {
          session.setOption(KzOption.showMessageFrame, next);
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

  // 全局实例要跨页面复用（SoLoud 初始化与解码缓存很贵），但它此刻播放的是
  // 上一局的声音：离开播放页必须停掉，否则 BGM 会一直响在游戏列表上。
  @override
  Future<void> dispose() => _inner.stopAll();
}
