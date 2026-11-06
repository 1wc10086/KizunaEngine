import 'dart:async';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:fvp/fvp.dart' as fvp;
import 'package:path/path.dart' as p;

import 'app.dart';
import 'core/app_info.dart';
import 'core/diagnostics/crash.dart';
import 'core/platform/app_paths.dart';
import 'core/platform/platform_info.dart';
import 'core/settings/settings.dart';
import 'features/settings/settings_notifier.dart';

void main() {
  runZonedGuarded(() async {
    WidgetsFlutterBinding.ensureInitialized();
    // 首帧只等这一项（平台 channel 调用，毫秒级）。原先支持目录解析、打开设置仓库、
    // 启动运行日志（含 pruneRunLogs 的目录遍历）都在 runApp 之前串行 await。
    // 桌面没有屏幕方向，只有移动端需要锁竖屏；播放页进入游戏时会再切横屏。
    if (isMobilePlatform) {
      await SystemChrome.setPreferredOrientations(
        const <DeviceOrientation>[DeviceOrientation.portraitUp],
      );
    }
    // 影片解码器注册必须早于任何 VideoPlayerController 的创建，所以放在启动阶段。
    // Bishop 的 OP / 片尾是 MPEG-1：Android 的 MediaCodec 没有 mpeg1video 解码器，
    // 所以让 fvp（libmdk + FFmpeg）接管 VideoPlayerPlatform。
    //
    // 这里刻意不传 video.decoders：手动给候选列表会把可选解码器限制在列表内，
    // MPEG-1 落到 FFmpeg 的 mpeg1video 才是正确路径，交给 mdk 自己按格式挑更稳。
    // 注册本身只设静态字段、不加载库，开销可忽略（解码器在首次播放时才初始化）。
    fvp.registerWith();
    // 支持目录只解析一次，设置仓库与崩溃目录共用这个 Future。这里不 await：
    // 目录创建与文件读取都延后到首次访问（后台完成），首帧不被文件 I/O 挡住；
    // 设置还没就绪时消费方用默认值（app.dart 本来就是 AsyncValue + 默认 AppSettings）。
    final support = appPrivateDirectory();
    final repository = SettingsRepository.lazy(
      () async => (await support).path,
    );
    FlutterError.onError = (details) {
      final exception = details.exception;
      final stack = details.stack ?? StackTrace.empty;
      // 崩溃目录同样要等支持目录解析；错误路径不在首帧关键路径上，异步记录即可。
      unawaited(_logCrash(support, exception, stack));
      Zone.current.handleUncaughtError(exception, stack);
    };
    runApp(ProviderScope(
      overrides: [
        settingsRepositoryProvider.overrideWithValue(repository),
      ],
      child: const KizunaApp(),
    ));
    // 非关键初始化放到 runApp 之后：运行日志（含 pruneRunLogs 的目录遍历）不参与 UI 启动。
    unawaited(_startRunLog(support));
  }, (error, stack) {
    debugPrint('uncaught zone error: $error');
  });
}

Future<void> _startRunLog(Future<Directory> support) async {
  try {
    final directory = await support;
    final runLog = await startRunLog(
      logDirectory: p.join(directory.path, 'logs'),
    );
    runLog?.log('KizunaEngine $kAppVersion starting');
  } catch (_) {
    // 运行日志只是诊断用途，失败不影响游戏。
  }
}

Future<void> _logCrash(
  Future<Directory> support,
  Object error,
  StackTrace stack,
) async {
  try {
    final directory = await support;
    await logCrash(
      error,
      stack,
      crashDirectory: p.join(directory.path, 'crashes'),
    );
  } catch (_) {
    // logCrash 内部已经吞掉写盘失败，这里只兜"支持目录解析失败"。
  }
}
