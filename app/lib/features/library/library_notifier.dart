import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/bridge/engine_bridge.dart';
import '../settings/settings_notifier.dart';

class GameRootNotifier extends Notifier<String> {
  @override
  String build() {
    return ref.watch(appSettingsProvider).value?.gameRoot ?? '';
  }

  Future<void> set(String value) async {
    await ref.read(appSettingsProvider.notifier).setGameRoot(value);
  }
}

final gameRootProvider =
    NotifierProvider<GameRootNotifier, String>(GameRootNotifier.new);

final libraryEngineProvider = FutureProvider<EngineBridge>((ref) async {
  final bridge = await EngineBridge.spawn();
  ref.onDispose(() {
    bridge.dispose();
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

  Future<void> rescan() async {
    state = const AsyncLoading<List<ScanEntry>>();
    ref.invalidateSelf();
  }
}

final libraryProvider =
    AsyncNotifierProvider<LibraryNotifier, List<ScanEntry>>(LibraryNotifier.new);
