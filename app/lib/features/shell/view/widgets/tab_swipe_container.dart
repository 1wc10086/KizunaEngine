import 'package:flutter/material.dart';

/// 带 tab 式切换动画与左右滑动切换的分支容器。
///
/// 为什么不用 PageView：分支子页是 go_router 的 Navigator，PageView 只保活
/// 屏幕附近的子页，滑远之后会把整个导航栈与滚动位置一起 dispose 掉。
/// 这里用 Stack + Offstage 复刻 IndexedStack 的"全部保活，只显示当前页"语义，
/// 切换时再让离场页与入场页按页码连续平移，于是得到 TabBarView 一样的手感，
/// 又不会丢状态。
///
/// 三种输入最终都汇到同一个连续页码 [_position]：
/// * 点底部导航 / NavigationRail → go_router 先切分支，这里在 didUpdateWidget
///   里补上从旧页到新页的滑动动画；
/// * 在内容区左右滑 → 手势直接推动 [_position]，松手后按位移与速度决定落点，
///   再请求 go_router 切分支；
/// * 手势落点与当前分支相同 → 弹回原位。
class TabSwipeContainer extends StatefulWidget {
  const TabSwipeContainer({
    super.key,
    required this.currentIndex,
    required this.onSelect,
    required this.children,
  });

  /// 当前激活分支（由 go_router 持有）。
  final int currentIndex;

  /// 请求切换分支。实现里直接转给 `StatefulNavigationShell.goBranch`。
  final ValueChanged<int> onSelect;

  final List<Widget> children;

  @override
  State<TabSwipeContainer> createState() => _TabSwipeContainerState();
}

class _TabSwipeContainerState extends State<TabSwipeContainer>
    with SingleTickerProviderStateMixin {
  static const Duration _switchDuration = Duration(milliseconds: 280);
  static const double _switchDistanceFraction = 0.25;
  /// 甩动多少"页/秒"就至少翻一页。
  static const double _switchVelocityPagesPerSecond = 0.8;

  /// 0→1 表示"从 [_animFrom] 走到 [_animTo]"。
  late final AnimationController _progress = AnimationController(
    vsync: this,
    duration: _switchDuration,
  );

  /// 连续页码：0 = 第一个分支居中，1 = 第二个分支居中……
  ///
  /// 用 ValueNotifier 而不是 setState：拖动时每帧只重建每页外面的包装层，
  /// Navigator 子树拿到的仍是同一个 widget 实例，Flutter 会跳过重建。
  final ValueNotifier<double> _position = ValueNotifier<double>(0);

  late int _index;
  double? _animFrom;
  double? _animTo;
  bool _dragging = false;

  /// 当前容器宽度，由 build 里的 LayoutBuilder 写入。
  ///
  /// 手势回调里拿 `context.size` 也能取到，但那依赖"这个 State 的 element
  /// 恰好只有一个 RenderBox 根"这一实现细节；显式记下宽度更稳，也避免
  /// 握手势时多做一次 render 树查找。
  double _width = 0;

  @override
  void initState() {
    super.initState();
    _index = widget.currentIndex;
    _position.value = _index.toDouble();
    _progress.addListener(_tick);
  }

  @override
  void didUpdateWidget(covariant TabSwipeContainer oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.currentIndex == _index) {
      return;
    }
    _index = widget.currentIndex;
    // go_router 已经把当前分支切过去了，这里只负责把视觉位置平滑地送过去。
    _animateTo(_index.toDouble());
  }

  @override
  void dispose() {
    _progress.dispose();
    _position.dispose();
    super.dispose();
  }

  void _tick() {
    final double? from = _animFrom;
    final double? to = _animTo;
    if (from == null || to == null) {
      return;
    }
    _position.value =
        from + (to - from) * Curves.easeOutCubic.transform(_progress.value);
  }

  void _animateTo(double target) {
    _animFrom = _position.value;
    _animTo = target;
    final double distance = (target - _animFrom!).abs();
    if (distance < 0.001) {
      _position.value = target;
      return;
    }
    _progress.duration = Duration(
      milliseconds: (140 + 140 * distance).clamp(140.0, 280.0).round(),
    );
    _progress.forward(from: 0);
  }

  double get _maxIndex => (widget.children.length - 1).toDouble();

  void _handleDragStart(DragStartDetails details) {
    _progress.stop();
    _animFrom = null;
    _animTo = null;
    _dragging = true;
  }

  void _handleDragUpdate(DragUpdateDetails details) {
    if (!_dragging) {
      return;
    }
    final double width = _width;
    if (width <= 0) {
      return;
    }
    // 手指左滑（delta 为负）应该走向下一页，所以这里取反。
    final double delta = -(details.primaryDelta ?? 0) / width;
    double next = _position.value + delta;
    // 越界部分按 0.3 收缩，手指能感觉到"到头了"，但不会出现大片空白。
    if (next < 0) {
      next *= 0.3;
    } else if (next > _maxIndex) {
      next = _maxIndex + (next - _maxIndex) * 0.3;
    }
    _position.value = next.clamp(-0.3, _maxIndex + 0.3);
  }

  void _handleDragEnd(DragEndDetails details) {
    if (!_dragging) {
      return;
    }
    _dragging = false;
    final double width = _width;
    // 速度换算成"页/秒"，向右滑为负。
    final double velocity =
        width > 0 ? -(details.primaryVelocity ?? 0) / width : 0;
    final double displacement = _position.value - _index;
    final int lastIndex = widget.children.length - 1;

    // 判定顺序很重要：先看甩动速度（短距离快速滑动也能翻页），
    // 再看位移是否越过阈值。位移以"相对当前分支"计算，不受 abs 页码影响。
    int target;
    if (velocity.abs() > _switchVelocityPagesPerSecond) {
      target = _index + (velocity > 0 ? 1 : -1);
    } else if (displacement.abs() > _switchDistanceFraction) {
      target = _index + (displacement > 0 ? 1 : -1);
    } else {
      target = _index;
    }
    target = target.clamp(0, lastIndex);

    if (target == _index) {
      // 落点还是当前分支：只把视觉位置弹回去（可能只是滑了一点点）。
      _animateTo(target.toDouble());
      return;
    }
    // 交给 go_router：下一次 build 会带着新的 currentIndex 回来，
    // didUpdateWidget 再接手剩下的滑动动画。
    widget.onSelect(target);
    // 兜底：如果这次切换没被路由接受（例如被 redirect 拦下），页面会停在
    // 两页之间。下一帧发现索引没变就滑回原位，不让界面卡在半路。
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted || widget.currentIndex == target) {
        return;
      }
      _animateTo(widget.currentIndex.toDouble());
    });
  }

  void _handleDragCancel() {
    if (!_dragging) {
      return;
    }
    _dragging = false;
    _animateTo(_position.value.clamp(0.0, _maxIndex));
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (BuildContext context, BoxConstraints constraints) {
        _width = constraints.maxWidth;
        return GestureDetector(
          // translucent：空白处也能起手，同时不挡住子级的点按。
          behavior: HitTestBehavior.translucent,
          onHorizontalDragStart: _handleDragStart,
          onHorizontalDragUpdate: _handleDragUpdate,
          onHorizontalDragEnd: _handleDragEnd,
          onHorizontalDragCancel: _handleDragCancel,
          child: ClipRect(
            child: Stack(
              fit: StackFit.expand,
              children: <Widget>[
                for (int i = 0; i < widget.children.length; i++)
                  _BranchSlot(
                    index: i,
                    position: _position,
                    child: widget.children[i],
                  ),
              ],
            ),
          ),
        );
      },
    );
  }
}

/// 单个分支的包装层：只负责"是否绘制"和"横向偏移"。
class _BranchSlot extends StatelessWidget {
  const _BranchSlot({
    required this.index,
    required this.position,
    required this.child,
  });

  final int index;
  final ValueNotifier<double> position;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<double>(
      valueListenable: position,
      child: child,
      builder: (BuildContext context, double page, Widget? child) {
        final double delta = index - page;
        // 只绘制当前页与相邻页：更远的分支完全 Offstage，连布局都不做。
        final bool visible = delta.abs() < 1.0;
        return Offstage(
          offstage: !visible,
          child: TickerMode(
            enabled: visible,
            child: FractionalTranslation(
              translation: Offset(delta, 0),
              child: ExcludeSemantics(
                excluding: delta.abs() > 0.001,
                child: IgnorePointer(
                  ignoring: delta.abs() > 0.001,
                  child: child,
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}
