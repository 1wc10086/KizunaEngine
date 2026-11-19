import 'package:flutter_riverpod/flutter_riverpod.dart';

/// 主界面自动检查更新的节流器。
///
/// 原实现把 `_lastCheck` 放在 widget 的 State 里：切到别的 tab 再回来、
/// 或热重载之后 State 重建，冷却就失效，会立刻再查一次 GitHub（API 每小时
/// 只有 60 次额度）。放到 provider 里让"这次会话已经查过"跟进程同寿命。
class UpdateCheckThrottle {
  static const Duration cooldown = Duration(minutes: 3);

  DateTime? _last;

  /// 返回 true 表示本次允许检查。
  bool take() {
    final now = DateTime.now();
    final last = _last;
    if (last != null && now.difference(last) < cooldown) {
      return false;
    }
    _last = now;
    return true;
  }
}

final updateCheckThrottleProvider = Provider<UpdateCheckThrottle>((ref) {
  return UpdateCheckThrottle();
});
