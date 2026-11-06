import 'dart:async';

import 'package:flutter/foundation.dart';

import 'channel.dart';
import 'platform_info.dart';

/// 确保 Android 拥有“所有文件访问权”。
///
/// Android 11+ 的 `MANAGE_EXTERNAL_STORAGE` 是特殊权限：不能用普通
/// `requestPermissions` 弹窗申请，只能跳到系统设置页由用户手动打开。
/// Android 10 及以下没有这个权限，退化为 READ/WRITE_EXTERNAL_STORAGE
/// 运行时权限。Kotlin 侧两种情况都封装在 `requestAllFilesAccess` 里。
///
/// 返回值只在“本次调用把权限从没有变成有”时为 true。启动扫描发生在授权
/// 之前（默认游戏根目录在公共存储里，没有权限会读成空），调用方据此重新
/// 扫描游戏库；已经有权时返回 false，不会触发多余扫描。
Future<bool> requestAllFilesAccessIfNeeded() async {
  if (!isAndroidPlatform) {
    return false;
  }
  try {
    const channel = InstallerChannel();
    if (await channel.hasAllFilesAccess()) {
      return false;
    }
    // 用户可能在系统设置里停留很久，Activity 也有被回收的可能；
    // 给 Future 一个上限，避免 MethodChannel 永远等不到回包。
    return await channel
        .requestAllFilesAccess()
        .timeout(const Duration(minutes: 5), onTimeout: () => false);
  } catch (error) {
    debugPrint('requestAllFilesAccess failed: $error');
    return false;
  }
}
