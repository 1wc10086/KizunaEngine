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
    final overlayActive = ref.watch(
      playerProvider.select((value) =>
          value.selectItems.isNotEmpty ||
          value.mapButtons.isNotEmpty ||
          value.mapMask.has ||
          value.choices.isNotEmpty ||
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
              onVerticalDragStart: (details) {
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
              onVerticalDragUpdate: (details) {
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
              onVerticalDragEnd: (_) {
                dragging = false;
              },
              onVerticalDragCancel: () {
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
    if (items.isEmpty) {
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
    if (choices.isEmpty) {
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
    return NameDialog(
      prompt: prompt,
      onSubmit: (value) => notifier.submitName(value),
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
    final l10n = AppLocalizations.of(context)!;
    return Positioned.fill(
      child: Container(
        color: const Color(0x99000000),
        alignment: Alignment.center,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Text(
              faulted ? l10n.commonError : l10n.playerFinished,
              style: const TextStyle(color: Color(0xFFFFFFFF)),
            ),
            const SizedBox(height: 12),
            FilledButton(
              onPressed: () {
                context.pop();
              },
              child: Text(l10n.playerExit),
            ),
          ],
        ),
      ),
    );
  }
}
