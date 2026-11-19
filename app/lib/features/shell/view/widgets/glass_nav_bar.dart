import 'dart:ui' show ImageFilter;

import 'package:flutter/material.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';

import '../nav_destinations.dart';

/// 悬浮导航栏本体高度（不含外边距与系统安全区）。
const double kGlassNavBarHeight = 64;

/// 悬浮导航栏距屏幕左右、底部的留白。
const double kGlassNavBarMargin = 12;

/// 页面滚动内容需要为悬浮导航栏空出的高度。
///
/// 导航栏是浮在内容上的，页面自己不知道它有多高；列表把这段高度加进底部
/// padding，最后一项才能滚到玻璃栏上方。
double glassNavBarInset(BuildContext context) {
  return kGlassNavBarHeight +
      kGlassNavBarMargin * 2 +
      MediaQuery.paddingOf(context).bottom;
}

/// 选中项背后那块滑动指示块的高度 / 左右内缩。
///
/// 内缩不是装饰：外层的玻璃栏是半径为 32 的胶囊，如果指示块贴着栏的左右
/// 边缘，它自己的圆角会被外层的 ClipRRect 切掉。往里缩 12 之后，指示块
/// 的四个角都完整落在胶囊内部，看起来永远是圆角。
const double _kPillHeight = 46;
const double _kPillInset = 12;

/// 悬浮玻璃底部导航栏。
///
/// 视觉上只做两件事：**悬浮**（不贴屏幕底边、自带圆角与阴影）和**玻璃**
/// （背景模糊 + 半透明底色 + 一圈很淡的描边）。没有做液态玻璃的折射/高光。
///
/// 交互（这一版按需求调整过）：
/// * 点按某一项 → 切换；
/// * **按住后左右滑** → 手指划过哪一项就切到哪一项。这里没有使用
///   `onLongPress`：长按识别器要等 500ms 才成立，需求要求"按住即可滑动"，
///   所以直接用 `Listener` 观察原始指针事件——按下后位移超过 8dp 就进入
///   拖动态，指示块全程跟手，松手吸附到最近的一项。
/// * 没有触感反馈、没有选中边框；选中项只有一个带主题色的滑动圆角块。
class GlassBottomNavBar extends StatefulWidget {
  const GlassBottomNavBar({
    super.key,
    required this.selectedIndex,
    required this.onSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onSelected;

  @override
  State<GlassBottomNavBar> createState() => _GlassBottomNavBarState();
}

class _GlassBottomNavBarState extends State<GlassBottomNavBar> {
  /// 按下后横向移动超过这个距离就进入"滑动选栏"状态。
  static const double _dragSlop = 8;

  /// 当前栏宽，由 LayoutBuilder 写入；指针事件里据此换算下标。
  double _width = 0;
  double get _itemWidth {
    final int count = NavDestinations.items.length;
    return count == 0 ? 0 : _width / count;
  }

  Offset? _downPosition;
  bool _dragging = false;

  /// 拖动中指示块的连续位置（以"项"为单位，0.5 表示第一项正中）。
  double _dragPosition = 0;

  int _indexAt(double dx) {
    if (_itemWidth <= 0) {
      return 0;
    }
    return (dx / _itemWidth).floor().clamp(0, NavDestinations.items.length - 1);
  }

  void _handlePointerDown(PointerDownEvent event) {
    _downPosition = event.localPosition;
    _dragging = false;
  }

  void _handlePointerMove(PointerMoveEvent event) {
    final Offset? down = _downPosition;
    if (down == null || _itemWidth <= 0) {
      return;
    }
    if (!_dragging && (event.localPosition.dx - down.dx).abs() > _dragSlop) {
      _dragging = true;
    }
    if (!_dragging) {
      return;
    }
    // 指示块以手指为中心：left = 手指 x - 指示块半宽，
    // 换算成"项"坐标就是 dx/itemWidth - 0.5。
    setState(() {
      _dragPosition = (event.localPosition.dx / _itemWidth - 0.5)
          .clamp(0.0, (NavDestinations.items.length - 1).toDouble());
    });
    final int index = _indexAt(event.localPosition.dx);
    if (index != widget.selectedIndex) {
      widget.onSelected(index);
    }
  }

  void _handlePointerUp(PointerUpEvent event) {
    final Offset? down = _downPosition;
    _downPosition = null;
    if (down == null) {
      return;
    }
    if (_dragging) {
      setState(() {
        _dragging = false;
      });
      return;
    }
    // 没有拖动 = 普通点按。用抬起位置判定，和按下位置一致即可。
    widget.onSelected(_indexAt(event.localPosition.dx));
  }

  void _handlePointerCancel(PointerCancelEvent event) {
    _downPosition = null;
    if (_dragging) {
      setState(() {
        _dragging = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final AppLocalizations l10n = AppLocalizations.of(context);
    final ColorScheme scheme = Theme.of(context).colorScheme;
    final BorderRadius radius = BorderRadius.circular(kGlassNavBarHeight / 2);
    final int count = NavDestinations.items.length;

    return Padding(
      padding: EdgeInsets.fromLTRB(
        kGlassNavBarMargin,
        0,
        kGlassNavBarMargin,
        kGlassNavBarMargin + MediaQuery.paddingOf(context).bottom,
      ),
      child: DecoratedBox(
        decoration: BoxDecoration(
          borderRadius: radius,
          boxShadow: <BoxShadow>[
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.22),
              blurRadius: 26,
              spreadRadius: -4,
              offset: const Offset(0, 10),
            ),
          ],
        ),
        child: ClipRRect(
          borderRadius: radius,
          child: BackdropFilter(
            // sigma 不要太大：玻璃糊得太狠会把滚动中的封面糊成一片色块，
            // 同时明显增加 GPU 开销。
            filter: ImageFilter.blur(sigmaX: 16, sigmaY: 16),
            child: DecoratedBox(
              decoration: BoxDecoration(
                // 亮色主题下要多给一点不透明度，否则白底上的玻璃看起来像"发灰"。
                color: scheme.surfaceContainerHigh.withValues(
                  alpha: scheme.brightness == Brightness.dark ? 0.58 : 0.74,
                ),
                borderRadius: radius,
                border: Border.all(
                  color: scheme.outlineVariant.withValues(alpha: 0.35),
                ),
              ),
              child: SizedBox(
                height: kGlassNavBarHeight,
                child: LayoutBuilder(
                  builder: (BuildContext context, BoxConstraints constraints) {
                    _width = constraints.maxWidth;
                    final double itemWidth = _itemWidth;
                    // 指示块目标位置：拖动时跟手，静止时停在选中项上。
                    final double position = _dragging
                        ? _dragPosition
                        : widget.selectedIndex.toDouble();
                    return Listener(
                      behavior: HitTestBehavior.opaque,
                      onPointerDown: _handlePointerDown,
                      onPointerMove: _handlePointerMove,
                      onPointerUp: _handlePointerUp,
                      onPointerCancel: _handlePointerCancel,
                      child: Stack(
                        // 让 Row 撑满 64dp，图标/文字才会在整个高度里居中。
                        fit: StackFit.expand,
                        children: <Widget>[
                          AnimatedPositioned(
                            // 拖动时零时长 = 跟手；点按切换时 240ms = 丝滑滑过去。
                            duration: _dragging
                                ? Duration.zero
                                : const Duration(milliseconds: 240),
                            curve: Curves.easeOutCubic,
                            left: itemWidth * position + _kPillInset,
                            top: (kGlassNavBarHeight - _kPillHeight) / 2,
                            width: itemWidth - _kPillInset * 2,
                            height: _kPillHeight,
                            child: DecoratedBox(
                              decoration: BoxDecoration(
                                color: scheme.primary.withValues(alpha: 0.14),
                                // 全圆角：四个角都由这个半径决定，不再有
                                // "贴边被外框切掉"的情况。
                                borderRadius:
                                    BorderRadius.circular(_kPillHeight / 2),
                              ),
                            ),
                          ),
                          Row(
                            children: <Widget>[
                              for (int i = 0; i < count; i++)
                                Expanded(
                                  child: _GlassNavItem(
                                    destination: NavDestinations.items[i],
                                    label: NavDestinations.items[i].label(l10n),
                                    selected: i == widget.selectedIndex,
                                    onTap: () => widget.onSelected(i),
                                  ),
                                ),
                            ],
                          ),
                        ],
                      ),
                    );
                  },
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// 单个导航项。纯展示：真正的点击/滑动由外层整条导航栏统一处理，
/// 这样相邻两项之间的点击识别器不会互相抢手势。
class _GlassNavItem extends StatelessWidget {
  const _GlassNavItem({
    required this.destination,
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final NavDestination destination;
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final ThemeData theme = Theme.of(context);
    final ColorScheme scheme = theme.colorScheme;
    // 选中态使用 MD3 主题的 primary，未选中态用 onSurfaceVariant：
    // 两种状态都能保证与玻璃底色的对比度。
    final Color color = selected ? scheme.primary : scheme.onSurfaceVariant;
    final TextStyle baseStyle =
        theme.textTheme.labelSmall ?? const TextStyle(fontSize: 11);

    return Semantics(
      container: true,
      button: true,
      selected: selected,
      label: label,
      onTap: onTap,
      child: Center(
        child: FittedBox(
          fit: BoxFit.scaleDown,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              AnimatedSwitcher(
                duration: const Duration(milliseconds: 200),
                child: Icon(
                  selected ? destination.selectedIcon : destination.icon,
                  key: ValueKey<bool>(selected),
                  color: color,
                  size: 22,
                ),
              ),
              const SizedBox(height: 2),
              AnimatedDefaultTextStyle(
                duration: const Duration(milliseconds: 200),
                curve: Curves.easeOutCubic,
                style: baseStyle.copyWith(
                  color: color,
                  fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
                ),
                child: Text(label, maxLines: 1),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
