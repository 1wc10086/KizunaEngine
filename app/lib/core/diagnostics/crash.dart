import 'dart:ffi';
import 'dart:io';

import 'package:ffi/ffi.dart';
import 'package:flutter/foundation.dart';
import 'package:path/path.dart' as p;

typedef _SetCrashLogDirC = Void Function(Pointer<Utf8>);
typedef _SetCrashLogDirDart = void Function(Pointer<Utf8>);

/// 把 enginec 的原生崩溃日志接到私有目录（与 [logCrash] 同一个 crashes/）。
///
/// 引擎被信号（SIGSEGV 等）打死时 Dart 的错误钩子根本没机会跑，报告只能由
/// C++ 侧的 signal 处理器写盘（enginec/ffi/crashlog.cpp）；这里负责把目录
/// 告诉它。查不到符号（Go 引擎/旧库）时静默跳过：诊断能力不能反过来弄崩应用。
void installNativeCrashLog(DynamicLibrary library, String directory) {
  if (directory.isEmpty) {
    return;
  }
  try {
    final setDirectory = library.lookupFunction<_SetCrashLogDirC, _SetCrashLogDirDart>(
        'KzcEngineSetCrashLogDirectory');
    final buffer = directory.toNativeUtf8();
    try {
      setDirectory(buffer);
    } finally {
      malloc.free(buffer);
    }
  } catch (error) {
    debugPrint('native crash log not installed: $error');
  }
}

const int _crashKeepCount = 12;
const int _runLogKeepCount = 8;

class RunLogSession {
  final String path;
  final IOSink _sink;
  bool _closed = false;

  RunLogSession(this.path, IOSink sink) : _sink = sink;

  void log(String line) {
    if (_closed) {
      return;
    }
    try {
      _sink.writeln(line);
    } catch (_) {}
  }

  Future<void> close() async {
    if (_closed) {
      return;
    }
    _closed = true;
    try {
      await _sink.flush();
      await _sink.close();
    } catch (_) {}
  }
}

Future<void> logCrash(
  Object error,
  StackTrace stackTrace, {
  required String crashDirectory,
  String source = 'FlutterError.onError',
}) async {
  try {
    await Directory(crashDirectory).create(recursive: true);
    final buffer = StringBuffer();
    buffer.writeln('KizunaEngine crash report');
    buffer.writeln('time     : ${_formatStamp(DateTime.now())}');
    buffer.writeln('source   : $source');
    buffer.writeln('exception: ${error.runtimeType}');
    buffer.writeln('message  : $error');
    buffer.writeln('os       : ${Platform.operatingSystem}');
    buffer.writeln();
    buffer.writeln('--- stack ---');
    buffer.writeln('$stackTrace');
    var inner = error;
    var depth = 0;
    while (depth < 5) {
      Object? next;
      try {
        next = (inner as dynamic).innerException;
      } catch (_) {
        break;
      }
      if (next == null) {
        break;
      }
      depth++;
      buffer.writeln();
      buffer.writeln('--- inner $depth ---');
      buffer.writeln('$next');
      inner = next;
    }
    final name = 'crash-${_fileStamp(DateTime.now(), millis: true)}.log';
    await File(p.join(crashDirectory, name))
        .writeAsString(buffer.toString(), flush: true);
    await pruneCrashes(crashDirectory: crashDirectory);
  } catch (_) {}
}

Future<RunLogSession?> startRunLog({required String logDirectory}) async {
  try {
    await Directory(logDirectory).create(recursive: true);
    final path =
        p.join(logDirectory, 'run-${_fileStamp(DateTime.now())}.log');
    final sink = File(path).openWrite(mode: FileMode.write);
    final session = RunLogSession(path, sink);
    session.log('run log -> $path');
    await pruneRunLogs(logDirectory: logDirectory);
    return session;
  } catch (_) {
    return null;
  }
}

Future<void> pruneCrashes(
    {required String crashDirectory, int keep = _crashKeepCount}) async {
  await _prune(crashDirectory, 'crash-', '.log', keep);
}

Future<void> pruneRunLogs(
    {required String logDirectory, int keep = _runLogKeepCount}) async {
  await _prune(logDirectory, 'run-', '.log', keep);
}

Future<void> _prune(
    String directory, String prefix, String suffix, int keep) async {
  try {
    final dir = Directory(directory);
    if (!await dir.exists()) {
      return;
    }
    final entries = <File>[];
    await for (final entity in dir.list(followLinks: false)) {
      if (entity is! File) {
        continue;
      }
      final name = p.basename(entity.path);
      if (!name.startsWith(prefix) || !name.endsWith(suffix)) {
        continue;
      }
      entries.add(entity);
    }
    entries.sort((left, right) => right.path.compareTo(left.path));
    for (var i = keep; i < entries.length; i++) {
      try {
        await entries[i].delete();
      } catch (_) {}
    }
  } catch (_) {}
}

String _two(int value) => value.toString().padLeft(2, '0');

String _three(int value) => value.toString().padLeft(3, '0');

String _fileStamp(DateTime time, {bool millis = false}) {
  final base =
      '${time.year}${_two(time.month)}${_two(time.day)}-${_two(time.hour)}${_two(time.minute)}${_two(time.second)}';
  return millis ? '$base-${_three(time.millisecond)}' : base;
}

String _formatStamp(DateTime time) =>
    '${time.year}-${_two(time.month)}-${_two(time.day)} '
    '${_two(time.hour)}:${_two(time.minute)}:${_two(time.second)}.${_three(time.millisecond)}';
