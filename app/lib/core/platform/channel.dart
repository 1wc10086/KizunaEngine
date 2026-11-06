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
}
