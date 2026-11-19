import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:kizuna_engine/l10n/app_localizations.dart';
import 'package:m3e_core/m3e_core.dart';

import '../../../core/bridge/engine_bridge.dart';
import '../../../core/layout/shell_chrome.dart';
import '../../../core/widgets/game_root_dialog.dart';
import '../../../core/widgets/kizuna_loading.dart';
import '../../../core/widgets/snowfall_overlay.dart';
import '../../settings/viewmodel/settings_notifier.dart';
import '../viewmodel/library_notifier.dart';
import 'game_card.dart';

/// 游戏库：顶部搜索条 + 游戏卡片列表。
///
/// 这一页的交互细节（都是需求里点名要的）：
/// * 顶栏颜色恒定（见 app_theme.dart 的 AppBarTheme），滚动时不再变色；
/// * 顶栏里有降雪（SnowfallOverlay），只是在视觉上让它"活"一点；
/// * 搜索条常驻在顶栏下方，往上滑会被折叠，同时顶栏浮出搜索按钮；
///   滑回顶部时反过来展开。折叠由 SliverPersistentHeader 直接跟手，
///   不靠 setState 打时间差，所以是丝滑的；
/// * 下拉刷新换成 M3E 的 PullToRefreshIndicator（顶栏的刷新按钮已移除）；
/// * 搜索输出结果时至少展示 0.5s 的 M3EContainedLoadingIndicator，
///   出现/消失带一点弹簧。
///
/// 视图只做两件事：订阅 filteredLibraryProvider 渲染列表，把用户意图
/// 转发给 viewmodel。折叠程度、焦点、输入控制器都属于页面局部状态，留在
/// State 里——它们不是业务数据，没必要进 provider。
class LibraryScreen extends ConsumerStatefulWidget {
  const LibraryScreen({super.key});

  @override
  ConsumerState<LibraryScreen> createState() => _LibraryScreenState();
}

class _LibraryScreenState extends ConsumerState<LibraryScreen> {
  /// 搜索条占用的高度（含上下留白）。
  ///
  /// 折叠判定与 SliverPersistentHeader 的 maxExtent 必须用同一个值，
  /// 否则搜索按钮出现的时机会和搜索条收起的时机对不上。
  /// 72 = SearchBar 的 56dp 最小高度 + 上下 4/12dp 留白。
  static const double _searchHeaderExtent = 72;

  static const Duration _revealDuration = Duration(milliseconds: 320);

  final TextEditingController _searchController = TextEditingController();
  final FocusNode _searchFocus = FocusNode();
  final ScrollController _scroll = ScrollController();

  /// 折叠进度 0..1。用 ValueNotifier 是为了只重建顶栏按钮，不重建整页。
  final ValueNotifier<double> _collapse = ValueNotifier<double>(0);

  /// 结果区是否可见。
  ///
  /// 不能直接用"busy 为 false"：指示器退场还要播 220ms 的弹簧动画，
  /// 那段时间里卡片已经渲染就会出现"指示器还没消失、卡片已出现"的叠影。
  /// 所以这里等退场动画结束后再放行结果。
  bool _resultsVisible = true;
  Timer? _resultsRevealTimer;

  @override
  void initState() {
    super.initState();
    _scroll.addListener(_handleScroll);
  }

  @override
  void dispose() {
    _resultsRevealTimer?.cancel();
    _resultsRevealTimer = null;
    _scroll.dispose();
    _collapse.dispose();
    _searchFocus.dispose();
    _searchController.dispose();
    super.dispose();
  }

  /// busy 从 true 变回 false 时，等指示器退场（220ms）过完再显示结果。
  ///
  /// 只比退场动画多留 20ms 余量；再长用户就会觉得"内容迟迟不出来"。
  void _handleSearchBusyChanged(bool busy) {
    _resultsRevealTimer?.cancel();
    _resultsRevealTimer = null;
    if (busy) {
      if (_resultsVisible) {
        setState(() => _resultsVisible = false);
      }
      return;
    }
    _resultsRevealTimer = Timer(const Duration(milliseconds: 240), () {
      if (mounted) {
        setState(() => _resultsVisible = true);
      }
    });
  }

  void _handleScroll() {
    if (!_scroll.hasClients) {
      return;
    }
    final double offset = _scroll.offset;
    final double progress = (offset / _searchHeaderExtent).clamp(0.0, 1.0);
    if ((progress - _collapse.value).abs() > 0.004) {
      _collapse.value = progress;
    }
    // 搜索条快被折没了就收起键盘：否则用户往下翻结果时输入框已经不可见，
    // 软键盘却还占着半屏。
    if (progress > 0.9 && _searchFocus.hasFocus) {
      _searchFocus.unfocus();
    }
  }

  void _onQueryChanged(String value) {
    ref.read(libraryQueryProvider.notifier).setText(value);
  }

  void _clearQuery() {
    _searchController.clear();
    ref.read(libraryQueryProvider.notifier).clear();
    _searchFocus.requestFocus();
  }

  /// 从顶栏的搜索按钮回到搜索条：先滚回顶部（让搜索条重新展开），再聚焦。
  Future<void> _revealSearch() async {
    if (_scroll.hasClients && _scroll.offset > 0) {
      await _scroll.animateTo(
        0,
        duration: _revealDuration,
        curve: Curves.easeOutCubic,
      );
    }
    if (mounted) {
      _searchFocus.requestFocus();
    }
  }

  Future<void> _refresh() async {
    // invalidate 会立刻把 provider 置为 loading 并重扫；等 future 结束，
    // 下拉指示器才会按真实耗时收回，而不是提前弹回。
    ref.invalidate(libraryProvider);
    try {
      await ref.read(libraryProvider.future);
    } catch (_) {
      // 错误由列表里的 AsyncValue 渲染，这里只负责让指示器结束。
    }
  }

  Future<void> _editRoot(String initial) async {
    final value = await showGameRootDialog(context, initial: initial);
    if (value == null || !mounted) {
      return;
    }
    // 只写设置：LibraryNotifier.build 里 watch 了 gameRootProvider，
    // 根目录一变它自己就会重扫。这里再 invalidate 一次只会多扫一遍。
    await ref.read(appSettingsProvider.notifier).setGameRoot(value);
  }

  @override
  Widget build(BuildContext context) {
    final AppLocalizations l10n = AppLocalizations.of(context);
    final String root = ref.watch(gameRootProvider);
    final AsyncValue<List<ScanEntry>> games = ref.watch(filteredLibraryProvider);
    final bool isEmptyLibrary = ref.watch(
      libraryProvider.select(
        (value) => (value.value ?? const <ScanEntry>[]).isEmpty,
      ),
    );
    final bool busy = ref.watch(librarySearchBusyProvider);
    // 指示器与结果不能叠：busy 期间 + 指示器退场期间都先把结果藏起来。
    ref.listen<bool>(librarySearchBusyProvider, (previous, next) {
      _handleSearchBusyChanged(next);
    });
    final bool resultsHidden = busy || !_resultsVisible;

    return Scaffold(
      appBar: AppBar(
        // flexibleSpace 位于工具栏之下、背景之上：雪花不会挡住标题与按钮。
        //
        // 高度显式给到 kToolbarHeight + 状态栏：AppBar 的 flexibleSpace 在
        // 无界高度下会被传成 loose 约束，直接 SizedBox.expand 会炸；
        // 先量好高度，内部的 CustomPaint 才能安全撑满。
        flexibleSpace: SizedBox(
          height: kToolbarHeight + MediaQuery.paddingOf(context).top,
          child: const SnowfallOverlay(),
        ),
        title: Text(l10n.navGames),
        actions: <Widget>[
          _AnimatedSearchAction(
            collapse: _collapse,
            tooltip: l10n.librarySearch,
            onPressed: () => unawaited(_revealSearch()),
          ),
        ],
      ),
      body: LayoutBuilder(
        builder: (BuildContext context, BoxConstraints constraints) {
          // 大屏上把内容限制在 980dp 以内并居中，平板/桌面不会出现一张
          // 横跨全屏、比例失真的 16:9 卡片。
          final double horizontal = constraints.maxWidth > 980
              ? (constraints.maxWidth - 980) / 2
              : 0;

          final List<Widget> slivers = <Widget>[
            SliverPersistentHeader(
              pinned: true,
              delegate: _SearchHeaderDelegate(
                extent: _searchHeaderExtent,
                child: _buildSearchBar(l10n),
              ),
            ),
            ..._contentSlivers(
              l10n: l10n,
              games: games,
              root: root,
              isEmptyLibrary: isEmptyLibrary,
              // 搜索进行中（指示器还在播）时先不渲染卡片/“没有匹配”文案：
              // 需求是等指示器播完再出结果，两者不能叠在一起。
              resultsHidden: resultsHidden,
            ),
            SliverToBoxAdapter(
              child: SizedBox(
                height: ShellChrome.bottomInsetOf(context) + 24,
              ),
            ),
          ];

          return Stack(
            fit: StackFit.expand,
            children: <Widget>[
              Padding(
                padding: EdgeInsets.symmetric(horizontal: horizontal),
                child: M3EPullToRefreshIndicator(
                  onRefresh: _refresh,
                  // 下拉 72dp 触发；指示器沿用应用自己的 M3E 胶囊样式。
                  triggerDistance: 72,
                  indicatorBuilder: (
                    BuildContext context,
                    double progress,
                    bool isRefreshing,
                  ) {
                    return KizunaLoadingIndicator(
                      size: 34,
                      // 松手后进入不定长动画，拖动阶段跟着手指走。
                      progress: isRefreshing ? null : progress,
                      semanticsLabel: l10n.commonLoading,
                    );
                  },
                  child: CustomScrollView(
                    controller: _scroll,
                    // 内容不足一屏时也要能下拉刷新。
                    physics: const AlwaysScrollableScrollPhysics(),
                    slivers: slivers,
                  ),
                ),
              ),
              _SearchOutputIndicator(
                busy: busy,
                label: l10n.commonLoading,
              ),
            ],
          );
        },
      ),
    );
  }

  Widget _buildSearchBar(AppLocalizations l10n) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 12),
      // 只监听输入框自己：清空按钮要随文字即时出现，但没必要重建整页。
      child: ValueListenableBuilder<TextEditingValue>(
        valueListenable: _searchController,
        builder: (BuildContext context, TextEditingValue value, Widget? _) {
          return SearchBar(
            controller: _searchController,
            focusNode: _searchFocus,
            hintText: l10n.librarySearchHint,
            leading: const Icon(Icons.search),
            trailing: <Widget>[
              if (value.text.isNotEmpty)
                IconButton(
                  tooltip: l10n.commonClose,
                  icon: const Icon(Icons.close),
                  onPressed: _clearQuery,
                ),
            ],
            onChanged: _onQueryChanged,
            textInputAction: TextInputAction.search,
            // 默认约束里有 minWidth 360，窄屏上没必要为它保留这个假设。
            constraints: const BoxConstraints(minHeight: 56),
          );
        },
      ),
    );
  }

  List<Widget> _contentSlivers({
    required AppLocalizations l10n,
    required AsyncValue<List<ScanEntry>> games,
    required String root,
    required bool isEmptyLibrary,
    required bool resultsHidden,
  }) {
    final List<Widget> slivers = _buildContentSlivers(
      l10n: l10n,
      games: games,
      root: root,
      isEmptyLibrary: isEmptyLibrary,
      resultsHidden: resultsHidden,
    );
    if (!resultsHidden) {
      return slivers;
    }
    // 搜索加载中（含指示器退场动画）：内容照常构建但整体透明。
    //
    // 这里没有直接不挂 sliver，而是 SliverOpacity(0) 保留布局：
    // 1) 滚动位置不会因为内容突然变短而被拉回顶部，搜索条的折叠状态不会跳；
    // 2) 手势仍落在原列表上，隐藏期间不会误触卡片（卡片自己再套了
    //    IgnorePointer），也不会出现"透明但可点"的问题。
    return <Widget>[
      for (final Widget sliver in slivers)
        SliverOpacity(opacity: 0, sliver: sliver),
    ];
  }

  List<Widget> _buildContentSlivers({
    required AppLocalizations l10n,
    required AsyncValue<List<ScanEntry>> games,
    required String root,
    required bool isEmptyLibrary,
    required bool resultsHidden,
  }) {
    if (root.isEmpty) {
      return <Widget>[
        SliverFillRemaining(
          hasScrollBody: false,
          child: _RootEmptyView(
            onSet: () {
              unawaited(_editRoot(root));
            },
          ),
        ),
      ];
    }
    return games.when(
      loading: () => <Widget>[
        SliverFillRemaining(
          hasScrollBody: false,
          child: Center(
            child: KizunaLoadingIndicator(
              semanticsLabel: l10n.commonLoading,
            ),
          ),
        ),
      ],
      error: (error, stackTrace) => <Widget>[
        SliverFillRemaining(
          hasScrollBody: false,
          child: _ErrorView(
            message: '${l10n.commonError}: $error',
            onRetry: () => ref.invalidate(libraryProvider),
          ),
        ),
      ],
      data: (entries) {
        if (isEmptyLibrary) {
          return <Widget>[
            SliverFillRemaining(
              hasScrollBody: false,
              child: Center(child: Text(l10n.libraryNoGames)),
            ),
          ];
        }
        if (entries.isEmpty) {
          return <Widget>[
            SliverFillRemaining(
              hasScrollBody: false,
              child: Center(child: Text(l10n.libraryNoMatch)),
            ),
          ];
        }
        return <Widget>[
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(12, 4, 12, 0),
            sliver: SliverList(
              delegate: SliverChildBuilderDelegate(
                (BuildContext context, int index) {
                  final ScanEntry entry = entries[index];
                  return IgnorePointer(
                    ignoring: resultsHidden,
                    child: Padding(
                      padding: const EdgeInsets.only(bottom: 12),
                      child: GameCard(
                        entry: entry,
                        onTap: () {
                          context.push(
                            '/player/${Uri.encodeComponent(entry.directory)}',
                            extra: entry,
                          );
                        },
                      ),
                    ),
                  );
                },
                childCount: entries.length,
              ),
            ),
          ),
        ];
      },
    );
  }
}

/// 顶栏右侧的搜索按钮。
///
/// 只在搜索条折叠到底之后出现（需求：滑过 SearchBar 才浮出搜索 icon）。
/// 显隐用固定 220ms 的 AnimatedOpacity/AnimatedScale，而不是把透明度直接
/// 绑到滚动位移上：折叠到位的最后几像素要是一闪而过，看起来像"图标凭空
/// 出现"；独立时长反而更顺眼。
///
/// 按钮始终占着布局位置（只是透明 + 忽略点击），搜索条展开/收起时标题不跳。
class _AnimatedSearchAction extends StatelessWidget {
  const _AnimatedSearchAction({
    required this.collapse,
    required this.tooltip,
    required this.onPressed,
  });

  final ValueNotifier<double> collapse;
  final String tooltip;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<double>(
      valueListenable: collapse,
      builder: (BuildContext context, double value, Widget? _) {
        final bool visible = value >= 0.95;
        return IgnorePointer(
          ignoring: !visible,
          child: AnimatedOpacity(
            opacity: visible ? 1 : 0,
            duration: const Duration(milliseconds: 220),
            curve: Curves.easeOutCubic,
            child: AnimatedScale(
              scale: visible ? 1 : 0.8,
              duration: const Duration(milliseconds: 220),
              curve: Curves.easeOutBack,
              child: IconButton(
                tooltip: tooltip,
                icon: const Icon(Icons.search),
                onPressed: onPressed,
              ),
            ),
          ),
        );
      },
    );
  }
}

/// 搜索输出时的加载指示器：出现/消失带一点弹簧回弹。
///
/// 结果区在它播放期间是空的（见 _contentSlivers 的 busy 分支），所以可以
/// 放心居中：不会出现"指示器压在卡片上"的观感。
/// 用 AnimatedSwitcher 而不是自己写 AnimationController：显隐两条动画
/// 的时序由框架管理，输入变化再频繁也不会叠出多个控制器。
class _SearchOutputIndicator extends StatelessWidget {
  const _SearchOutputIndicator({
    required this.busy,
    required this.label,
  });

  final bool busy;
  final String label;

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Center(
        child: AnimatedSwitcher(
          duration: const Duration(milliseconds: 320),
          reverseDuration: const Duration(milliseconds: 220),
          transitionBuilder: (Widget child, Animation<double> animation) {
            return FadeTransition(
              opacity: CurvedAnimation(
                parent: animation,
                curve: Curves.easeOut,
                reverseCurve: Curves.easeIn,
              ),
              child: ScaleTransition(
                // easeOutBack 自带一点回弹，就是需求里说的"弹簧感"。
                scale: Tween<double>(begin: 0.7, end: 1.0).animate(
                  CurvedAnimation(
                    parent: animation,
                    curve: Curves.easeOutBack,
                    reverseCurve: Curves.easeInBack,
                  ),
                ),
                child: child,
              ),
            );
          },
          child: busy
              ? KizunaLoadingIndicator(size: 76, semanticsLabel: label)
              : const SizedBox.shrink(),
        ),
      ),
    );
  }
}

/// 搜索条的头 sliver。
///
/// `pinned: true` + `minExtent: 0` 让它的高度直接由滚动位移驱动：
/// 用户往上滑多少，搜索条就压缩多少，滑过 72dp 完全收起；滑回顶部时
/// 反向展开。整个过程由滚动管线在同一个 frame 里算完，不会出现
/// setState 触发折叠时内容跟着跳一下的问题。
class _SearchHeaderDelegate extends SliverPersistentHeaderDelegate {
  _SearchHeaderDelegate({
    required this.extent,
    required this.child,
  });

  final double extent;
  final Widget child;

  @override
  double get minExtent => 0;

  @override
  double get maxExtent => extent;

  @override
  Widget build(
    BuildContext context,
    double shrinkOffset,
    bool overlapsContent,
  ) {
    final double t = (shrinkOffset / extent).clamp(0.0, 1.0);
    // SliverPersistentHeader 会把 shrinkOffset 一并传进来，期望委托自己
    // 折成"当前高度"；这里显式给 SizedBox，避免 OverflowBox 撑出约束。
    final double height = (extent - shrinkOffset).clamp(0.0, extent);
    return SizedBox(
      height: height,
      width: double.infinity,
      child: ClipRect(
        child: OverflowBox(
          alignment: Alignment.topCenter,
          minHeight: extent,
          maxHeight: extent,
          child: IgnorePointer(
            ignoring: t > 0.5,
            child: Opacity(
              opacity: 1 - t,
              child: Transform.scale(
                alignment: Alignment.topCenter,
                scale: 1 - 0.06 * t,
                child: child,
              ),
            ),
          ),
        ),
      ),
    );
  }

  @override
  bool shouldRebuild(covariant _SearchHeaderDelegate oldDelegate) {
    return oldDelegate.extent != extent || oldDelegate.child != child;
  }
}

class _ErrorView extends StatelessWidget {
  final String message;
  final VoidCallback onRetry;

  const _ErrorView({required this.message, required this.onRetry});

  @override
  Widget build(BuildContext context) {
    final AppLocalizations l10n = AppLocalizations.of(context);
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Text(message),
          const SizedBox(height: 12),
          FilledButton(onPressed: onRetry, child: Text(l10n.commonRetry)),
        ],
      ),
    );
  }
}

class _RootEmptyView extends StatelessWidget {
  final VoidCallback onSet;

  const _RootEmptyView({required this.onSet});

  @override
  Widget build(BuildContext context) {
    final AppLocalizations l10n = AppLocalizations.of(context);
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Text(l10n.libraryNoGames),
          const SizedBox(height: 12),
          FilledButton(onPressed: onSet, child: Text(l10n.settingsGameRoot)),
        ],
      ),
    );
  }
}
