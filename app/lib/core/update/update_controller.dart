import 'dart:io';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

import '../app_info.dart';
import '../platform/channel.dart';
import '../platform/external_links.dart';
import '../platform/platform_info.dart';
import 'update_service.dart';

class UpdateState {
  final ReleaseInfo? release;
  final bool checking;
  final bool downloading;
  final double? progress;
  final String error;

  const UpdateState({
    this.release,
    this.checking = false,
    this.downloading = false,
    this.progress,
    this.error = '',
  });
}

class UpdateCheckNotifier extends Notifier<UpdateState> {
  final UpdateService _service = UpdateService();

  @override
  UpdateState build() => const UpdateState();

  Future<ReleaseInfo?> check() async {
    state = UpdateState(release: state.release, checking: true);
    try {
      final abi = await _resolveAbi();
      final release = await _service.check(
        installedVersion: kAppVersion,
        abi: abi,
      );
      state = UpdateState(release: release);
      return release;
    } catch (error) {
      state = UpdateState(error: '$error');
      return null;
    }
  }

  Future<String?> _resolveAbi() async {
    if (!isAndroidPlatform) {
      return null;
    }
    try {
      return await const InstallerChannel().androidAbi();
    } catch (_) {
      return 'arm64';
    }
  }

  Future<void> downloadAndInstall(ReleaseInfo release) async {
    // 只有 Android 有“应用内下载 + 调起安装器”的能力：APK 的下载/安装
    // 都依赖 kizuna/installer 平台通道。桌面和 iOS 没有对应的安装协议，
    // 打开 release 页面让用户自己选包，避免把 Android 的 APK 下给桌面。
    if (!isAndroidPlatform) {
      state = UpdateState(release: release);
      try {
        await openExternalUrl(release.pageUrl);
      } catch (error) {
        state = UpdateState(release: release, error: '$error');
      }
      return;
    }
    if (release.downloadUrl.isEmpty) {
      state = UpdateState(
        release: release,
        error: 'no APK asset for this Android ABI',
      );
      return;
    }
    state = UpdateState(release: release, downloading: true);
    try {
      final directory = await getTemporaryDirectory();
      final target = p.join(
        directory.path,
        'KizunaEngine-${release.version}.apk',
      );
      final file = File(target);
      if (await file.exists() && await file.length() > 0) {
        state = UpdateState(release: release, downloading: true, progress: 1);
      } else {
        _service.removeStale(target);
        // 先直连 GitHub；失败后依次套镜像前缀。每次失败都清掉 .part，
        // 否则下一次会从残缺文件继续写，最终 APK 校验不过。
        Object? lastError;
        for (final url in UpdateService.downloadCandidates(release.downloadUrl)) {
          try {
            await _service.download(
              url: url,
              targetPath: target,
              onProgress: (progress) {
                state = UpdateState(
                  release: release,
                  downloading: true,
                  progress: progress,
                );
              },
            );
            lastError = null;
            break;
          } catch (error) {
            lastError = error;
            _service.removeStale(target);
          }
        }
        if (lastError != null) {
          throw lastError;
        }
        state = UpdateState(release: release, downloading: true, progress: 1);
      }
      await const InstallerChannel().installApk(target);
      state = UpdateState(release: release, progress: 1);
    } catch (error) {
      state = UpdateState(
        release: release,
        progress: state.progress,
        error: '$error',
      );
    }
  }

  void clearRelease() {
    state = UpdateState(
      checking: state.checking,
      downloading: state.downloading,
      progress: state.progress,
      error: state.error,
    );
  }
}

final updateCheckProvider =
    NotifierProvider<UpdateCheckNotifier, UpdateState>(UpdateCheckNotifier.new);
