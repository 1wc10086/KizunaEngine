import 'package:flutter/services.dart';

class InstallerChannel {
  static const MethodChannel _channel = MethodChannel('kizuna/installer');

  const InstallerChannel();

  Future<void> installApk(String path) =>
      _channel.invokeMethod<void>('installApk', <String, Object?>{'path': path});

  Future<String> androidAbi() async {
    final abi = await _channel.invokeMethod<String>('abi');
    return abi ?? 'arm64';
  }

  /// 是否已获得“所有文件访问”能力（Android 11+ 的 MANAGE_EXTERNAL_STORAGE，
  /// 或 Android 10 及以下的读写存储权限）。
  Future<bool> hasAllFilesAccess() async =>
      (await _channel.invokeMethod<bool>('hasAllFilesAccess')) ?? false;

  /// 打开系统授权流程。Android 11+ 会跳到本应用的特殊权限设置页，
  /// 用户返回后才回包；返回值是回来时的真实授权状态。
  Future<bool> requestAllFilesAccess() async =>
      (await _channel.invokeMethod<bool>('requestAllFilesAccess')) ?? false;
}
