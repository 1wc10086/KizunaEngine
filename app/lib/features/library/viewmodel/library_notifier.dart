import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/bridge/engine_bridge.dart';
import '../../../core/settings/settings_snapshot.dart';
import '../../settings/viewmodel/settings_notifier.dart';

/// 当前游戏根目录的只读派生值。
///
/// 以前这里是个 Notifier<String>，`build()` 里 watch 设置、`set()` 里再写回
/// 设置——两个 provider 互为源，谁先刷新取决于调用顺序，很容易出现"界面已经
/// 显示新目录、扫描还用旧值"的窗口。现在根目录只有一个来源（设置），
/// 这里只做投影，写入统一走 `SettingsNotifier.setGameRoot`。
final gameRootProvider = Provider<String>((ref) {
  final settings = ref.watch(appSettingsProvider).valueOrDefault;
  return (settings.gameRoot ?? '').trim();
});

/// 扫描用的引擎实例（Go 侧 enginego）。
///
/// 与播放页的会话分开：扫描只调 op 23，不需要 open/start，独立实例可以
/// 常驻在库页面，避免每次回到库页面都重新 spawn isolate。
final libraryEngineProvider = FutureProvider<EngineBridge>((ref) async {
  final bridge = await EngineBridge.spawn();
  ref.onDispose(() {
    // dispose 是异步的（要等 isolate 退出）；这里不需要等它，
    // 但也别让 Future 悬空——unawaited 明确表达"有意不等待"。
    unawaited(bridge.dispose());
  });
  return bridge;
});

class LibraryNotifier extends AsyncNotifier<List<ScanEntry>> {
  @override
  Future<List<ScanEntry>> build() async {
    final root = ref.watch(gameRootProvider);
    if (root.isEmpty) {
      return const <ScanEntry>[];
    }
    final bridge = await ref.watch(libraryEngineProvider.future);
    return bridge.scan(root);
  }

  /// 重新扫描。
  ///
  /// 不清空 state：invalidateSelf 保留上一次的列表直到新结果回来，
  /// 玩家看到的是"旧内容"，而不是整页闪成 loading。调用方不 await
  /// （按钮回调里 await 也没人能接住错误），错误由 AsyncValue 呈现。
  void rescan() {
    ref.invalidateSelf();
  }
}

final libraryProvider = AsyncNotifierProvider<LibraryNotifier, List<ScanEntry>>(
  LibraryNotifier.new,
);

/// 库页面的搜索关键字（不落盘）。
///
/// 搜索框现在是常驻的，所以不再需要"是否展开搜索"这种状态；这里只保留
/// 关键字本身，过滤规则由 filteredLibraryProvider 负责。
class LibraryQuery {
  const LibraryQuery({this.text = ''});

  final String text;
}

/// 用 Notifier 而不是 Riverpod 3 已归入 legacy 的 StateProvider：
/// legacy 那批要从 `flutter_riverpod/legacy.dart` 单独 import，
/// 新代码没必要一开始就踩进这个坑。
class LibraryQueryNotifier extends Notifier<LibraryQuery> {
  @override
  LibraryQuery build() => const LibraryQuery();

  void setText(String value) {
    if (value == state.text) {
      return;
    }
    state = LibraryQuery(text: value);
    // 有关键字才算是"搜索输出"：清空搜索框时会立刻显示全量列表，
    // 那不是一次检索，不该闪一下加载动画。
    if (value.trim().isNotEmpty) {
      ref.read(librarySearchBusyProvider.notifier).pulse();
    }
  }

  void clear() {
    setText('');
  }
}

final libraryQueryProvider =
    NotifierProvider<LibraryQueryNotifier, LibraryQuery>(
        LibraryQueryNotifier.new);

/// 搜索结果加载指示器的忙碌状态。
///
/// 过滤本身是同步的，如果直接跟着 AsyncValue 显示指示器只会"闪一下"，
/// 比不显示更糟。需求要求搜索结果输出时至少展示 0.5s 的加载动画，
/// 所以每次输入都把状态展开成一个"至少 500ms"的窗口，计时结束自动收起。
/// 连续输入时会不断重置计时，指示器保持可见，直到最后一次输入的 500ms 之后。
class LibrarySearchBusyNotifier extends Notifier<bool> {
  static const Duration minimumVisible = Duration(milliseconds: 500);

  Timer? _timer;
  bool _disposed = false;

  @override
  bool build() {
    ref.onDispose(() {
      _disposed = true;
      _timer?.cancel();
      _timer = null;
    });
    return false;
  }

  void pulse() {
    if (_disposed) {
      return;
    }
    _timer?.cancel();
    if (!state) {
      state = true;
    }
    _timer = Timer(minimumVisible, () {
      if (_disposed) {
        return;
      }
      state = false;
    });
  }
}

final librarySearchBusyProvider =
    NotifierProvider<LibrarySearchBusyNotifier, bool>(
        LibrarySearchBusyNotifier.new);

/// 过滤后的游戏列表：把"名字/引擎名包含关键字"的规则放在 viewmodel，
/// 让 view 只负责渲染。
final filteredLibraryProvider = Provider<AsyncValue<List<ScanEntry>>>((ref) {
  final games = ref.watch(libraryProvider);
  final query = ref.watch(libraryQueryProvider.select((q) => q.text)).trim();
  if (query.isEmpty) {
    return games;
  }
  final lower = query.toLowerCase();
  return games.whenData((entries) {
    return entries.where((entry) {
      return entry.name.toLowerCase().contains(lower) ||
          entry.brandName.toLowerCase().contains(lower);
    }).toList();
  });
});
