import 'dart:io';

import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

/// 应用私有数据根目录。
///
/// 设置、运行日志、崩溃记录、引擎缓存都放这里；卸载应用时系统会一并清理。
/// - Android/iOS/macOS：path_provider 的 Application Support（系统私有目录，
///   iOS 的“Documents”会被 iCloud 备份/文件 App 暴露，Support 更适合放配置）。
/// - Windows：用户要求放在安装目录下的 `data`，和绿色版软件的习惯一致；
///   Program Files 可能只读，创建失败时回退到 Application Support，避免
///   启动时因为一次 mkdir 失败就拿不到设置目录。
/// - Linux：Application Support（通常是 ~/.local/share/<app>）。
Future<Directory> appPrivateDirectory() async {
  if (Platform.isWindows) {
    final executable = File(Platform.resolvedExecutable).parent;
    final local = Directory(p.join(executable.path, 'data'));
    try {
      await local.create(recursive: true);
      return local;
    } catch (_) {
      // 安装目录只读时继续走系统目录。
    }
  }
  final support = await getApplicationSupportDirectory();
  final directory = Directory(p.join(support.path, 'kizuna'));
  await directory.create(recursive: true);
  return directory;
}

/// 引擎缓存根。每个游戏会在其下再按目录 md5 建子目录（见 Go 侧 GameCacheDir）。
Future<Directory> appCacheRoot() async {
  final root = Directory(p.join((await appPrivateDirectory()).path, 'cache', 'kizuna'));
  await root.create(recursive: true);
  return root;
}

/// 各平台默认游戏目录。
///
/// 默认目录只负责“第一次启动时给一个合理位置”，不要求一定存在或可写：
/// Android 的公共目录需要用户授予存储权限，桌面/iOS 的 Documents 也可能
/// 被用户删掉。调用方应在扫描失败时提示玩家去设置里改目录。
Future<String> defaultGameRoot() async {
  String path;
  if (Platform.isAndroid) {
    // Android 上没有“当前用户 Documents”的通用 API；/storage/emulated/0
    // 是公共存储根，配合 MANAGE_EXTERNAL_STORAGE 使用。
    path = p.join('/storage/emulated/0', 'KizunaEngine');
  } else if (Platform.isWindows) {
    final home = Platform.environment['USERPROFILE'] ?? '';
    path = home.isEmpty
        ? p.join(Directory.current.path, 'Documents', 'KizunaEngine')
        : p.join(home, 'Documents', 'KizunaEngine');
  } else {
    // macOS/Linux/iOS：Documents 是用户可见目录；iOS 的 Documents 属于应用
    // 沙盒，无需额外权限。
    final documents = await getApplicationDocumentsDirectory();
    path = p.join(documents.path, 'KizunaEngine');
  }
  try {
    await Directory(path).create(recursive: true);
  } catch (_) {
    // 权限不足时保留路径，让扫描结果显示为空，玩家仍可手动选择。
  }
  return path;
}
