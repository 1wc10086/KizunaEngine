import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/platform/platform_info.dart';
import '../../../core/update/update_service.dart';
import '../../settings/viewmodel/settings_notifier.dart';
import 'update_notifier.dart';

/// 更新流程的结果，供视图决定弹什么提示。
///
/// 为什么要把结果做成枚举而不是让 viewmodel 直接弹 SnackBar：
/// 弹窗需要 BuildContext，而 viewmodel 不该拿 Context（页面可能在
/// await 期间被销毁）。viewmodel 只报告"发生了什么"，文案与容器由视图选。
enum UpdateFlowOutcome {
  /// 静默检查且用户关掉了自动检查：什么都没做。
  skipped,

  /// 已是最新版本。
  upToDate,

  /// 检查失败（网络/限流/无资产）。
  checkFailed,

  /// 发现新版本，视图应当展示对话框。
  releaseFound,

  /// 非 Android 平台：已用系统浏览器打开 release 页面。
  browserOpened,

  /// Android：已下载并调起系统安装器。
  installStarted,

  /// 下载成功但安装失败。
  installFailed,

  /// 下载失败。
  downloadFailed,
}

class UpdateFlowResult {
  final UpdateFlowOutcome outcome;
  final ReleaseInfo? release;
  final String error;

  const UpdateFlowResult(this.outcome, {this.release, this.error = ''});
}

/// 更新流程的编排：检查 → 判断是否展示 → 下载/安装。
///
/// 对话框的"展示"留在视图（需要 Context 与 Navigator），这里只负责
/// "要不要展示"以及下载后的结果分类。参数取 WidgetRef 而不是长期持有
/// Ref：Riverpod 3 在 provider 被销毁后使用 Ref 会抛异常，而这些方法都在
/// 用户交互时调用，短生命周期地读一次最安全。
class UpdateFlowViewModel {
  const UpdateFlowViewModel();

  Future<UpdateFlowResult> check(WidgetRef ref, {bool silent = false}) async {
    if (silent) {
      // 首帧就自动检查时设置可能还没加载完；此时按"开启"处理——
      // 因为一次文件读失败而永远不提示更新，比多查一次更糟。
      // 用读快照而不是 `appSettingsProvider.future`：后者在加载失败时会
      // 抛 ProviderException（Riverpod 3 起统一包装），还得再兜一层。
      final settings = ref.read(appSettingsProvider).value;
      if (settings != null && !settings.autoUpdateCheck) {
        return const UpdateFlowResult(UpdateFlowOutcome.skipped);
      }
    }
    final release = await ref.read(updateCheckProvider.notifier).check();
    if (release == null) {
      final error = ref.read(updateCheckProvider).error;
      return UpdateFlowResult(
        error.isEmpty
            ? UpdateFlowOutcome.upToDate
            : UpdateFlowOutcome.checkFailed,
        error: error,
      );
    }
    return UpdateFlowResult(UpdateFlowOutcome.releaseFound, release: release);
  }

  Future<UpdateFlowResult> download(WidgetRef ref, ReleaseInfo release) async {
    await ref.read(updateCheckProvider.notifier).downloadAndInstall(release);
    final state = ref.read(updateCheckProvider);
    if (!isAndroidPlatform) {
      return UpdateFlowResult(
        state.error.isEmpty
            ? UpdateFlowOutcome.browserOpened
            : UpdateFlowOutcome.installFailed,
        release: release,
        error: state.error,
      );
    }
    if (state.error.isEmpty) {
      return UpdateFlowResult(
        UpdateFlowOutcome.installStarted,
        release: release,
      );
    }
    // progress == 1 说明 APK 已经下载完，失败发生在安装阶段。
    final installStage = state.progress == 1;
    return UpdateFlowResult(
      installStage
          ? UpdateFlowOutcome.installFailed
          : UpdateFlowOutcome.downloadFailed,
      release: release,
      error: state.error,
    );
  }
}

const UpdateFlowViewModel updateFlow = UpdateFlowViewModel();
