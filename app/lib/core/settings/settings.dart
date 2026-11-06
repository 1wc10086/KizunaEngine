import 'dart:async';
import 'dart:io';

import 'package:path/path.dart' as p;

import 'settings_models.dart';
import 'settings_store.dart';
import 'translator_settings.dart';

export 'settings_models.dart';
export 'translator_settings.dart';

class SettingsRepository {
  SettingsRepository._(this._directory);

  /// 惰性打开：构造本身不碰文件系统——支持目录解析是平台调用、mkdir 是文件 I/O，
  /// 都不该挡首帧。读写的实现本来就是 async，首次访问时再 await 打开结果即可，
  /// 语义与原先的 open() 一致（目录先建好；文件不存在就是默认值）。
  factory SettingsRepository.lazy(Future<String> Function() directory) =>
      SettingsRepository._(directory);

  final Future<String> Function() _directory;
  final Map<String, Future<JsonFileStore>> _stores =
      <String, Future<JsonFileStore>>{};

  Future<JsonFileStore> _store(String name) {
    final cached = _stores[name];
    if (cached != null) {
      return cached;
    }
    final opened = _open(name);
    _stores[name] = opened;
    // 打开失败不保留失败态：下一次读写重试（否则一次目录错误会让设置永久不可用）。
    // 这里只是补一个错误处理，真正 await 的调用方照样能拿到同一个错误。
    unawaited(opened.then<void>((_) {}, onError: (Object error) {
      _stores.remove(name);
    }));
    return opened;
  }

  Future<JsonFileStore> _open(String name) async {
    final directory = await _directory();
    await Directory(directory).create(recursive: true);
    return JsonFileStore(File(p.join(directory, name)));
  }

  Future<AppSettings> loadAppSettings() async {
    final data = await (await _store('settings.json')).read();
    if (data == null) {
      return const AppSettings();
    }
    return AppSettings.fromJson(data);
  }

  Future<void> saveAppSettings(AppSettings settings) async {
    final store = await _store('settings.json');
    await store.write(settings.toJson());
  }

  Future<TranslatorSettings> loadTranslatorSettings() async {
    final data = await (await _store('translator.json')).read();
    if (data == null) {
      return const TranslatorSettings();
    }
    return TranslatorSettings.fromJson(data);
  }

  Future<void> saveTranslatorSettings(TranslatorSettings settings) async {
    final store = await _store('translator.json');
    await store.write(settings.toJson());
  }

  Future<void> close() => Future<void>.value();
}
