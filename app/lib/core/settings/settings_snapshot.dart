import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'settings_models.dart';

/// 设置快照的同步读取方式。
///
/// 设置仓库是异步打开的，但首帧不能等它：界面消费的永远是"已加载的值，
/// 加载中就用默认值"。让每个 view 都写一遍
/// `watch(appSettingsProvider).value ?? const AppSettings()` 会把同一个兜底
/// 复制十几份，任何一处漏写都会在加载窗口里拿到 null。
extension AppSettingsSnapshot on AsyncValue<AppSettings> {
  AppSettings get valueOrDefault => value ?? const AppSettings();
}
