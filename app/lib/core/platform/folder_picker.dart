import 'dart:io';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/services.dart';

import 'platform_info.dart';

/// 文件夹选择结果。
///
/// Android 的 SAF 不保证能把 `content://` 树映射成文件系统路径：
/// - `path` 非空：可以直接交给 Go 引擎读写；
/// - `path` 为空、`uri` 非空：需要调用 [SafMirror] 把目录复制到应用私有目录，
///   这也是“没有所有文件访问权限”时唯一可行的只读方案。
class FolderPickResult {
  final String? path;
  final String? uri;

  const FolderPickResult({this.path, this.uri});

  bool get isUsable => path != null && path!.isNotEmpty;
}

/// 跨平台目录选择器。
///
/// Android 走自实现的 SAF channel（ACTION_OPEN_DOCUMENT_TREE），这样即使
/// 没有 MANAGE_EXTERNAL_STORAGE 也能选目录；Windows/macOS 交给 file_picker
/// 调系统原生选择器。
///
/// Linux 按要求不提供按钮，只允许手动输入路径。iOS 没有目录选择 API：
/// 游戏放在应用沙盒的 Documents/KizunaEngine，用“文件”App 拷进去即可，
/// 所以同样隐藏按钮，保留手动输入。
class FolderPicker {
  static const MethodChannel _channel = MethodChannel('kizuna/folder');

  static bool get supported =>
      Platform.isAndroid || Platform.isWindows || Platform.isMacOS;

  static Future<FolderPickResult?> pick() async {
    if (!supported) {
      return null;
    }
    if (Platform.isAndroid) {
      try {
        final result =
            await _channel.invokeMapMethod<String, Object?>('pickFolder');
        if (result != null) {
          return FolderPickResult(
            path: result['path'] as String?,
            uri: result['uri'] as String?,
          );
        }
      } on PlatformException {
        // 没有 SAF Activity（极简 ROM）时退回 file_picker。
      }
    }
    try {
      final path = await FilePicker.getDirectoryPath();
      if (path != null && path.isNotEmpty) {
        return FolderPickResult(path: path);
      }
    } catch (_) {
      // 用户取消或平台不支持目录选择。
    }
    return null;
  }

  /// 把 SAF 树复制到应用私有目录，返回真实路径。
  ///
  /// 只在 [FolderPickResult.path] 为空时调用。大目录会很慢，调用方应先提示
  /// 玩家“正在准备目录”，并允许取消（当前实现由系统 Activity 返回后执行）。
  static Future<String?> mirror(String uri, String displayName) async {
    if (!isAndroidPlatform) {
      return null;
    }
    try {
      return await _channel.invokeMethod<String>('mirrorFolder', <String, Object?>{
        'uri': uri,
        'name': displayName,
      });
    } on PlatformException {
      return null;
    }
  }
}
