import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../../core/bridge/engine_bridge.dart';
import 'backlog_view.dart';
import 'choices_overlay.dart';
import 'frame_painter.dart';
import 'message_box.dart';
import 'name_dialog.dart';
import 'movie_layer.dart';
import 'player_notifier.dart';
import 'title_layer.dart';
import 'select_overlay.dart';

class PlayerStage extends ConsumerWidget {
  final PlayerNotifier notifier;
  /// 主界面只剩"退出"需要页面层处理：读档/鉴赏/设置在引擎内部完成，
  /// 播放层不需要知道宿主路由树。
  final VoidCallback onExit;

  const PlayerStage({
    super.key,
    required this.notifier,
    required this.onExit,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = ref.watch(playerProvider.select((value) => value.width));
    final height =
        ref.watch(playerProvider.select((value) => value.height));
    final backlogActive =
        ref.watch(playerProvider.select((value) => value.backlogActive));
    // 有覆盖层时，舞台层不处理点击：这些层自己会接管命中。
    // 主界面（titleMenu）和影片也在这里面——标题按钮有自己的热区，
    // 影片只有"跳过"一个按钮；让舞台层同时响应会变成一次点击两处生效
    // （标题点击既切菜单又推进脚本）。
    //
    // engineMessageWindow 为真的品牌（TMR-Hiro）把对话框、工具条、选项菜单、
    // 暂停菜单、存读档/设置/回想、鉴赏页全部画进帧位图。此时舞台层必须照常
    // 转发点击：宿主没有可以接管命中的覆盖层，拦掉点击就等于这些按钮全失效。
    final overlayActive = ref.watch(
      playerProvider.select((value) =>
          (!value.engineMessageWindow &&
              (value.selectItems.isNotEmpty ||
                  value.mapButtons.isNotEmpty ||
                  value.mapMask.has ||
                  value.choices.isNotEmpty)) ||
          value.titleMenu.isNotEmpty ||
          value.moviePath.isNotEmpty),
    );
    return LayoutBuilder(
      builder: (context, constraints) {
        final ratio = width > 0 && height > 0 ? width / height : 16 / 9;
        var pictureWidth = constraints.maxWidth;
        var pictureHeight = pictureWidth / ratio;
        if (pictureHeight > constraints.maxHeight) {
          pictureHeight = constraints.maxHeight;
          pictureWidth = pictureHeight * ratio;
        }
        final side = (constraints.maxWidth - pictureWidth) / 2;
        final top = (constraints.maxHeight - pictureHeight) / 2;
        int engineX(double dx) {
          if (width <= 0 || pictureWidth <= 0) {
            return -1;
          }
          final value = ((dx - side) * width / pictureWidth).floor();
          if (value < 0) {
            return 0;
          }
          if (value >= width) {
            return width - 1;
          }
          return value;
        }

        int engineY(double dy) {
          if (height <= 0 || pictureHeight <= 0) {
            return -1;
          }
          final value = ((dy - top) * height / pictureHeight).floor();
          if (value < 0) {
            return 0;
          }
          if (value >= height) {
            return height - 1;
          }
          return value;
        }

        void tapAt(Offset position) {
          if (overlayActive) {
            return;
          }
          final x = engineX(position.dx);
          final y = engineY(position.dy);
          if (x < 0 || y < 0) {
            return;
          }
          notifier.tapAt(x, y);
        }

        void releaseAt() {
          notifier.release();
        }

        // 拖动期间必须停止发送 hover：引擎用"上一次指针 y"计算拖动位移，
        // 同一坐标的 hover 会先把 lastDragY 更新到目标位置，拖动增量恒为 0，
        // 表现为"拖了但界面不动"。
        var dragging = false;

        void moveTo(Offset position) {
          if (overlayActive || dragging) {
            return;
          }
          final x = engineX(position.dx);
          final y = engineY(position.dy);
          if (x < 0 || y < 0) {
            return;
          }
          notifier.hover(x, y);
        }

        var clickX = -1;
        var clickY = -1;
        return Listener(
          onPointerSignal: (event) {
            if (event is PointerScrollEvent) {
              // 滚轮事件本身没有坐标，引擎靠"最后一次指针位置"判断滚哪一栏，
              // 所以这里把指针位置一起带上（引擎侧会先补一次 move）。
              final delta = event.scrollDelta.dy > 0 ? 1 : -1;
              final x = engineX(event.localPosition.dx);
              final y = engineY(event.localPosition.dy);
              if (x < 0 || y < 0) {
                notifier.wheel(delta);
              } else {
                notifier.wheel(delta, x: x, y: y);
              }
            }
          },
          onPointerMove: (event) {
            moveTo(event.localPosition);
          },
          onPointerHover: (event) {
            moveTo(event.localPosition);
          },
          child: Focus(
            autofocus: true,
            onKeyEvent: (node, event) {
              if (event is! KeyDownEvent) {
                return KeyEventResult.ignored;
              }
              if (event.logicalKey == LogicalKeyboardKey.enter ||
                  event.logicalKey == LogicalKeyboardKey.space) {
                notifier.tap();
                return KeyEventResult.handled;
              }
              if (event.logicalKey == LogicalKeyboardKey.escape) {
                notifier.rightClick();
                return KeyEventResult.handled;
              }
              if (event.logicalKey == LogicalKeyboardKey.arrowUp) {
                notifier.wheel(-1);
                return KeyEventResult.handled;
              }
              if (event.logicalKey == LogicalKeyboardKey.arrowDown) {
                notifier.wheel(1);
                return KeyEventResult.handled;
              }
              return KeyEventResult.ignored;
            },
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              // 按下不再单独发 press：引擎的 op=3 在一次调用里原子完成
              // press+release，抬起时带上坐标走这一条路径即可。
              onTapUp: (details) {
                tapAt(details.localPosition);
              },
              onTapCancel: releaseAt,
              // 触屏没有滚轮；把纵向拖动转成 pointer 子协议 6，由引擎决定
              // 滚回想/BGV，其它界面忽略。拖动被竞技场接受后 tap 识别器自动落败，
              // 所以松手不会再触发 onTapUp 推进对白。
              // 用 pan 而不是 vertical：emon 的设置界面音量滑杆是水平拖动，
              // vertical 识别器根本不会接受横向手势，滑杆就拖不动。pan 的
              // 方向由引擎按命中 sprite 决定（滑杆/回想/BGV）。
              onPanStart: (details) {
                if (overlayActive) {
                  return;
                }
                final x = engineX(details.localPosition.dx);
                final y = engineY(details.localPosition.dy);
                if (x < 0 || y < 0) {
                  return;
                }
                dragging = true;
                notifier.drag(x, y);
              },
              onPanUpdate: (details) {
                if (overlayActive) {
                  return;
                }
                final x = engineX(details.localPosition.dx);
                final y = engineY(details.localPosition.dy);
                if (x < 0 || y < 0) {
                  return;
                }
                notifier.drag(x, y);
              },
              onPanEnd: (_) {
                dragging = false;
              },
              onPanCancel: () {
                dragging = false;
              },
              onSecondaryTapDown: (details) {
                clickX = engineX(details.localPosition.dx);
                clickY = engineY(details.localPosition.dy);
              },
              onSecondaryTap: () {
                if (clickX >= 0 && clickY >= 0) {
                  notifier.rightClickAt(clickX, clickY);
                  return;
                }
                notifier.rightClick();
              },
              child: Container(
                color: const Color(0xFF000000),
                alignment: Alignment.center,
                child: AspectRatio(
                  aspectRatio: width > 0 && height > 0
                      ? width / height
                      : 16 / 9,
                  child: Stack(
                    children: <Widget>[
                      // 帧位图走 FrameBus（ValueNotifier），不走 playerProvider：
                      // 30fps 的帧更新只重建 FrameSourceView 这一个 widget，
                      // 不会像以前那样每次合成都在全局 state 上惊动所有订阅者。
                      Positioned.fill(
                        child: FrameSourceView(bus: notifier.frameBus),
                      ),
                      MapLayer(notifier: notifier),
                      SelectLayer(notifier: notifier),
                      MessageLayer(notifier: notifier),
                      ChoicesLayer(notifier: notifier),
                      if (backlogActive)
                        BacklogLayer(notifier: notifier),
                      NameLayer(notifier: notifier),
                      // 主界面与影片是"引擎停在原地等宿主"的两种状态，
                      // 必须画在最上层：底下的对话框/选择层此时都不该出现。
                      TitleLayer(
                        notifier: notifier,
                        onExit: onExit,
                      ),
                      MovieGate(notifier: notifier),
                      const EndLayer(),
                    ],
                  ),
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class MapLayer extends ConsumerWidget {
  final PlayerNotifier notifier;

  const MapLayer({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final buttons =
        ref.watch(playerProvider.select((value) => value.mapButtons));
    final mask =
        ref.watch(playerProvider.select((value) => value.mapMask));
    final width = ref.watch(playerProvider.select((value) => value.width));
    final height =
        ref.watch(playerProvider.select((value) => value.height));
    final active =
        ref.watch(playerProvider.select((value) => value.selectItems));
    if (active.isEmpty && buttons.isEmpty && !mask.has) {
      return const SizedBox.shrink();
    }
    if (width <= 0 || height <= 0) {
      return const SizedBox.shrink();
    }
    return Positioned.fill(
      child: LayoutBuilder(
        builder: (context, constraints) {
          final scale = constraints.maxWidth / width.toDouble();
          return MouseRegion(
            onHover: (event) {
              notifier.setHover(
                (event.localPosition.dx / scale).round(),
                (event.localPosition.dy / scale).round(),
              );
            },
            onExit: (_) {
              notifier.clearHover();
            },
            child: GestureDetector(
              behavior: HitTestBehavior.opaque,
              onTapDown: (details) {
                notifier.chooseMapPixel(
                  (details.localPosition.dx / scale).round(),
                  (details.localPosition.dy / scale).round(),
                );
              },
              child: Stack(
                children: <Widget>[
                  // 有图的按钮（KZK / 屈辱2 / 侵蝕 等）由引擎把卡面直接画进
                  // 帧位图，这里再描一圈半透明白框会和原版地图不一致；只有
                  // 没有图标的兜底地图（MapPrms / label scan 的行）才需要
                  // 用边框把可点区域提示出来。
                  // 有像素掩码的地图（学園2 的 map_hit）由地图底图本身
                  // 表现可点区域；再给每个地点描框会和原作画面不一致。
                  for (final button in buttons)
                    if (button.image.isEmpty && !mask.has)
                      Positioned(
                        left: button.x * scale,
                        top: button.y * scale,
                        width: (button.width <= 0 ? 1 : button.width) * scale,
                        height:
                            (button.height <= 0 ? 1 : button.height) * scale,
                        child: Container(
                          decoration: BoxDecoration(
                            border: Border.all(
                              color: const Color(0x33FFFFFF),
                            ),
                          ),
                        ),
                      ),
                ],
              ),
            ),
          );
        },
      ),
    );
  }
}

class SelectLayer extends ConsumerWidget {
  final PlayerNotifier notifier;

  const SelectLayer({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final items =
        ref.watch(playerProvider.select((value) => value.selectItems));
    // 引擎自绘界面的品牌：selectItems 只是引擎给出的“可点区域”，
    // 画面已经画进帧位图，宿主不能再叠一层自己的列表。
    final engineWindow = ref.watch(
        playerProvider.select((value) => value.engineMessageWindow));
    if (items.isEmpty || engineWindow) {
      return const SizedBox.shrink();
    }
    final frames =
        ref.watch(playerProvider.select((value) => value.selectFrames));
    final width = ref.watch(playerProvider.select((value) => value.width));
    final height =
        ref.watch(playerProvider.select((value) => value.height));
    return SelectOverlay(
      items: items,
      frames: frames,
      engineWidth: width,
      engineHeight: height,
      onPick: (x, y) => notifier.chooseMapPixel(x, y),
      onHover: (x, y) => notifier.setHover(x, y),
    );
  }
}

class MessageLayer extends ConsumerWidget {
  final PlayerNotifier notifier;

  const MessageLayer({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // 引擎会把消息窗口直接画进帧位图，此时再叠 Flutter 的 MessageBox 会
    // 出现两份对话框。用引擎上报的"这一帧已画"而不是仅看设置开关：
    // 没有 [message window] 配置的品牌仍然需要 Flutter 兜底。
    final engineWindow = ref.watch(
        playerProvider.select((value) => value.engineMessageWindow));
    // 脚本定义的界面（鉴赏/立绘/选择页）也由引擎画进帧位图；宿主的对话框叠上去
    // 会压在原版界面上，所以只要引擎在等界面输入或标题菜单开着就不画。
    // 标题菜单存在时引擎停在主界面，残留的对白不能压在标题图上。
    final onTitle = ref.watch(
        playerProvider.select((value) => value.titleMenu.isNotEmpty));
    if (engineWindow || onTitle) {
      return const SizedBox.shrink();
    }
    final speaker =
        ref.watch(playerProvider.select((value) => value.speaker));
    final text = ref.watch(playerProvider.select((value) => value.text));
    final visible = ref.watch(
        playerProvider.select((value) => value.messageWindowVisible));
    if (text.isEmpty) {
      return const SizedBox.shrink();
    }
    return MessageBox(
      speaker: speaker,
      text: text,
      visible: visible,
    );
  }
}

class ChoicesLayer extends ConsumerWidget {
  final PlayerNotifier notifier;

  const ChoicesLayer({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final choices =
        ref.watch(playerProvider.select((value) => value.choices));
    // 引擎自绘对话框的品牌（TMR-Hiro）把选项菜单也画在 sys_select 的
    // 三行图上（[SELECT] 的坐标），宿主再叠一列 Material 按钮会盖住原版
    // 菜单、位置也完全不同。此时点击仍走 Pointer，命中由引擎判定。
    final engineWindow = ref.watch(
        playerProvider.select((value) => value.engineMessageWindow));
    if (choices.isEmpty || engineWindow) {
      return const SizedBox.shrink();
    }
    return ChoicesOverlay(
      choices: choices,
      onChoose: (index) => notifier.choose(index),
    );
  }
}

class BacklogLayer extends ConsumerStatefulWidget {
  final PlayerNotifier notifier;

  const BacklogLayer({super.key, required this.notifier});

  @override
  ConsumerState<BacklogLayer> createState() => _BacklogLayerState();
}

class _BacklogLayerState extends ConsumerState<BacklogLayer> {
  late final Future<List<TextEntry>> _entries;

  @override
  void initState() {
    super.initState();
    _entries = widget.notifier.backlog();
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<List<TextEntry>>(
      future: _entries,
      builder: (context, snapshot) {
        return BacklogView(
          entries: snapshot.data ?? const <TextEntry>[],
          onClose: () {
            widget.notifier.exitBacklog();
          },
        );
      },
    );
  }
}

class NameLayer extends ConsumerWidget {
  final PlayerNotifier notifier;

  const NameLayer({super.key, required this.notifier});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final awaiting =
        ref.watch(playerProvider.select((value) => value.awaitingName));
    if (!awaiting) {
      return const SizedBox.shrink();
    }
    final prompt =
        ref.watch(playerProvider.select((value) => value.namePrompt));
    final yesNo =
        ref.watch(playerProvider.select((value) => value.promptIsYesNo));
    final enginePrompt =
        ref.watch(playerProvider.select((value) => value.enginePrompt));
    if (enginePrompt) {
      // 对话框已经画进引擎帧。是/否直接点引擎自绘按钮；文字输入只留
      // 一个透明 IME 层，文字本身由引擎画，避免 Flutter 再盖一层。
      if (yesNo) {
        return const SizedBox.shrink();
      }
      return _EnginePromptKeyboard(notifier: notifier);
    }
    return NameDialog(
      prompt: prompt,
      // emon 的 GetYesNoMessage 也复用 AwaitName：直接给文本框会让玩家
      // 必须手打“はい”，而且回答会按字符串写进变量，脚本的 ==0 判断
      // 就不再可靠。是/否问题必须走 1/0 的 SubmitYesNo。
      yesNo: yesNo,
      onSubmit: (value) => notifier.submitName(value),
      onYesNo: (yes) => notifier.submitYesNo(yes),
    );
  }
}

class EndLayer extends ConsumerWidget {
  const EndLayer({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final finished =
        ref.watch(playerProvider.select((value) => value.finished));
    final faulted =
        ref.watch(playerProvider.select((value) => value.faulted));
    if (!finished && !faulted) {
      return const SizedBox.shrink();
    }
    final l10n = AppLocalizations.of(context);
    // faulted 只是一个 step：真正的原因（emecfg 缺失 / run failed 等）只在引擎
    // LastError 里，notifier 在终端 step 时已经拉进 state.faultReason。
    // 没有原因时显示通用说明而不是空白，避免玩家只剩"出错了"三字干瞪眼。
    final faultReason =
        ref.watch(playerProvider.select((value) => value.faultReason));
    final reason = faultReason.trim();
    return Positioned.fill(
      child: Container(
        color: const Color(0x99000000),
        alignment: Alignment.center,
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Text(
                  faulted ? l10n.playerFaultedTitle : l10n.playerFinished,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    color: Color(0xFFFFFFFF),
                    fontSize: 18,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                if (faulted) ...<Widget>[
                  const SizedBox(height: 12),
                  Text(
                    reason.isEmpty
                        ? l10n.playerFaultedUnknown
                        : l10n.playerFaultedReasonWithDetail(reason),
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                      color: Color(0xFFD0D0D0),
                      fontSize: 13,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    l10n.playerFaultedRetryHint,
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                      color: Color(0xFF9E9E9E),
                      fontSize: 12,
                    ),
                  ),
                ],
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: () {
                    context.pop();
                  },
                  child: Text(l10n.playerExit),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}


/// 引擎自绘 prompt 的透明 IME 层：只负责调出系统键盘和把当前文字同步给
/// 引擎（OpPromptText）。不画输入框/按钮——那些都在引擎帧里，点击也由
/// EngineBridge 的 Pointer 命中引擎自绘热区。
class _EnginePromptKeyboard extends StatefulWidget {
  final PlayerNotifier notifier;

  const _EnginePromptKeyboard({required this.notifier});

  @override
  State<_EnginePromptKeyboard> createState() => _EnginePromptKeyboardState();
}

class _EnginePromptKeyboardState extends State<_EnginePromptKeyboard> {
  final TextEditingController _controller = TextEditingController();
  final FocusNode _focus = FocusNode();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        _focus.requestFocus();
      }
    });
  }

  @override
  void dispose() {
    _focus.dispose();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // 只占屏幕顶部一条窄带，避免挡住居中的引擎对话框按钮；opacity 接近 0
    // 让 Flutter 不显示自己的文字/边框，但 TextField 仍然能获得焦点和 IME。
    return Positioned(
      left: 0,
      right: 0,
      top: 0,
      height: 44,
      child: Opacity(
        opacity: 0.01,
        child: TextField(
          controller: _controller,
          focusNode: _focus,
          autofocus: true,
          showCursor: false,
          enableInteractiveSelection: false,
          style: const TextStyle(color: Colors.transparent),
          decoration: const InputDecoration.collapsed(hintText: ''),
          onChanged: widget.notifier.setPromptText,
        ),
      ),
    );
  }
}
