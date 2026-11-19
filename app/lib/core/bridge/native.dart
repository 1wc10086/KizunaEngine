import 'dart:ffi';
import 'dart:io';
import 'dart:typed_data';

import 'package:ffi/ffi.dart';
import 'package:path/path.dart' as p;

typedef _KzEngineCreateC = Pointer<Void> Function();
typedef _KzEngineCreateDart = Pointer<Void> Function();
typedef _KzEngineDestroyC = Void Function(Pointer<Void>);
typedef _KzEngineDestroyDart = void Function(Pointer<Void>);
typedef _KzEngineInvokeC = Pointer<Uint8> Function(
    Pointer<Void>, Pointer<Uint8>, Int32, Pointer<Int32>);
typedef _KzEngineInvokeDart = Pointer<Uint8> Function(
    Pointer<Void>, Pointer<Uint8>, int, Pointer<Int32>);
typedef _KzEngineFreeC = Void Function(Pointer<Void>);
typedef _KzEngineFreeDart = void Function(Pointer<Void>);
typedef _KzEngineVersionC = Pointer<Utf8> Function();
typedef _KzEngineVersionDart = Pointer<Utf8> Function();
typedef _KzInvokeC = Pointer<Utf8> Function(Pointer<Utf8>);
typedef _KzInvokeDart = Pointer<Utf8> Function(Pointer<Utf8>);
typedef _KzFreeC = Void Function(Pointer<Void>);
typedef _KzFreeDart = void Function(Pointer<Void>);
typedef _KzVersionC = Pointer<Utf8> Function();
typedef _KzVersionDart = Pointer<Utf8> Function();

/// 按平台列出原生库的候选加载方式，顺序即优先级。
///
/// [base] 是不带前后缀的名字：`enginego` / `enginec` / `kernel`。
///
/// - iOS：引擎与内核由 `bridge/` 模块编译成一个 c-archive 静态链接进
///   Runner 可执行文件（iOS 不支持 c-shared，两个 archive 又会撞 Go 运行时
///   符号），所以符号在进程镜像里，用 [DynamicLibrary.process]。
///   此时 [base] 选不了库：符号来自链接阶段，两种引擎不能同时进同一个镜像。
/// - macOS：enginego 的发布包静态链接 bridge；保留 dylib 候选是为了兼容旧布局
///   （把 .dylib 放进 Contents/Frameworks 的调试构建）。enginec 只有 dylib。
/// - Android：.so 由 APK 的 lib/<abi>/ 提供，系统按包名搜索。
/// - Windows：DLL 放在 exe 旁边，绝对路径优先，避免 PATH 上有同名旧库。
/// - Linux：Flutter runner 的 RPATH 是 `$ORIGIN/lib`，先按 bundle 布局找，
///   再退回当前目录/系统搜索路径。
List<DynamicLibrary Function()> _nativeLoaders(String base) {
  if (Platform.isIOS) {
    return <DynamicLibrary Function()>[() => DynamicLibrary.process()];
  }
  final exeDir = p.dirname(Platform.resolvedExecutable);
  if (Platform.isAndroid) {
    return <DynamicLibrary Function()>[
      () => DynamicLibrary.open('lib$base.so'),
    ];
  }
  if (Platform.isWindows) {
    return <DynamicLibrary Function()>[
      () => DynamicLibrary.open(p.join(exeDir, '$base.dll')),
      () => DynamicLibrary.open('$base.dll'),
    ];
  }
  if (Platform.isMacOS) {
    final dylibs = <DynamicLibrary Function()>[
      () => DynamicLibrary.open(
            p.join(exeDir, '..', 'Frameworks', 'lib$base.dylib'),
          ),
      () => DynamicLibrary.open(p.join(exeDir, 'lib$base.dylib')),
      () => DynamicLibrary.open('lib$base.dylib'),
    ];
    // 进程镜像里静态链接的是 bridge（Go 引擎）的 KzEngine*：enginego 先试它，
    // 避免捡到旧的 libenginego.dylib；enginec 从不进进程镜像，对它的 process()
    // 只可能探到另一个引擎，所以干脆不列，缺 dylib 时报"打不开"而不是用错内核。
    return base == 'enginego'
        ? <DynamicLibrary Function()>[
            () => DynamicLibrary.process(),
            ...dylibs,
          ]
        : dylibs;
  }
  return <DynamicLibrary Function()>[
    () => DynamicLibrary.open(p.join(exeDir, 'lib', 'lib$base.so')),
    () => DynamicLibrary.open(p.join(exeDir, 'lib$base.so')),
    () => DynamicLibrary.open('lib$base.so'),
  ];
}

/// 打开第一个真正包含 [probe] 符号的候选。
///
/// `DynamicLibrary.process()` 永远不会加载失败，但没静态链接 bridge 时
/// 里面并没有 Kz* 符号；所以不能只看“打开成功”，必须探测符号。
/// 探测失败时继续尝试下一个候选，全部失败才抛出带最后一次错误的 StateError。
DynamicLibrary _tryOpen(String base, String probe) {
  Object? lastError;
  for (final loader in _nativeLoaders(base)) {
    try {
      final library = loader();
      if (library.providesSymbol(probe)) {
        return library;
      }
      lastError = StateError('library does not provide $probe');
    } catch (error) {
      lastError = error;
    }
  }
  throw StateError('unable to open native library "$base": $lastError');
}

class NativeEngineBindings {
  final DynamicLibrary library;
  /// 打开的是哪一份实现：`enginego` / `enginec`。两份库的 C ABI 只有
  /// [versionString] 的字符串所有权不同（见 [_freesVersion]）。
  final String nativeLibrary;
  // C ABI 前缀：Go 版导出 KzEngine*，C++ 版导出 KzcEngine*。两份实现符号不同，
  // 这样 iOS/macOS 可以把两个静态库链进同一个进程而不会撞符号（协议字节完全
  // 相同，只有入口名不同）。
  String get _prefix => nativeLibrary == 'enginec' ? 'KzcEngine' : 'KzEngine';

  // enginec 的 KzcEngineVersion 返回静态字符串（见 enginec/ffi/enginec_ffi.cpp），
  // 对它调 KzcEngineFree 就是 free 一个非堆指针；Go 版返回 C.CString，必须释放。
  bool get _freesVersion => nativeLibrary != 'enginec';
  late final _KzEngineCreateDart _create =
      library.lookupFunction<_KzEngineCreateC, _KzEngineCreateDart>(
          '${_prefix}Create');
  late final _KzEngineDestroyDart _destroy =
      library.lookupFunction<_KzEngineDestroyC, _KzEngineDestroyDart>(
          '${_prefix}Destroy');
  late final _KzEngineInvokeDart _invoke =
      library.lookupFunction<_KzEngineInvokeC, _KzEngineInvokeDart>(
          '${_prefix}Invoke');
  late final _KzEngineFreeDart _free =
      library.lookupFunction<_KzEngineFreeC, _KzEngineFreeDart>(
          '${_prefix}Free');
  late final _KzEngineVersionDart _version =
      library.lookupFunction<_KzEngineVersionC, _KzEngineVersionDart>(
          '${_prefix}Version');

  NativeEngineBindings(this.library, this.nativeLibrary);

  /// 打开 [nativeLibrary] 这一份引擎实现：`enginego`（Go 版）或 `enginec`（C++ 内核）。
  ///
  /// 两份库说同一套协议，入口符号分别是 KzEngine* 与 KzcEngine*；宿主只按作品
  /// 换一个库名，其余调用路径完全不变。
  ///
  /// 第一个符号同时充当“库对不对”的探针。
  factory NativeEngineBindings.open(String nativeLibrary) {
    final prefix = nativeLibrary == 'enginec' ? 'KzcEngine' : 'KzEngine';
    return NativeEngineBindings(
        _tryOpen(nativeLibrary, '${prefix}Create'), nativeLibrary);
  }

  Pointer<Void> create() => _create();

  void destroy(Pointer<Void> handle) => _destroy(handle);

  Uint8List invoke(Pointer<Void> handle, Uint8List request) {
    final requestPtr = calloc<Uint8>(request.length);
    requestPtr.asTypedList(request.length).setAll(0, request);
    final outLen = calloc<Int32>();
    try {
      final responsePtr = _invoke(handle, requestPtr, request.length, outLen);
      if (responsePtr == nullptr) {
        throw StateError('${_prefix}Invoke returned null');
      }
      try {
        final length = outLen.value;
        return Uint8List.fromList(responsePtr.asTypedList(length));
      } finally {
        _free(responsePtr.cast());
      }
    } finally {
      calloc.free(requestPtr);
      calloc.free(outLen);
    }
  }

  String versionString() {
    final ptr = _version();
    if (ptr == nullptr) {
      throw StateError('${_prefix}Version returned null');
    }
    try {
      return ptr.toDartString();
    } finally {
      if (_freesVersion) {
        _free(ptr.cast());
      }
    }
  }
}

class NativeKernelBindings {
  final DynamicLibrary library;
  late final _KzInvokeDart _invoke =
      library.lookupFunction<_KzInvokeC, _KzInvokeDart>('KzInvoke');
  late final _KzFreeDart _free =
      library.lookupFunction<_KzFreeC, _KzFreeDart>('KzFree');
  late final _KzVersionDart _version =
      library.lookupFunction<_KzVersionC, _KzVersionDart>('KzVersion');

  NativeKernelBindings(this.library);

  factory NativeKernelBindings.open() =>
      NativeKernelBindings(_tryOpen('kernel', 'KzInvoke'));

  String invokeJson(String request) {
    final requestPtr = request.toNativeUtf8();
    try {
      final responsePtr = _invoke(requestPtr.cast());
      if (responsePtr == nullptr) {
        throw StateError('KzInvoke returned null');
      }
      try {
        return responsePtr.toDartString();
      } finally {
        _free(responsePtr.cast());
      }
    } finally {
      calloc.free(requestPtr);
    }
  }

  String versionString() {
    final ptr = _version();
    if (ptr == nullptr) {
      throw StateError('KzVersion returned null');
    }
    try {
      return ptr.toDartString();
    } finally {
      _free(ptr.cast());
    }
  }
}

class NativeMemory {
  static Pointer<Uint8> allocate(int length) => calloc<Uint8>(length);

  static void free(Pointer<Uint8> ptr) => calloc.free(ptr);

  static int address(Pointer<Uint8> ptr) => ptr.address;

  static Uint8List copyOut(Pointer<Uint8> ptr, int length) =>
      Uint8List.fromList(ptr.asTypedList(length));
}
