# The C ABI the Flutter app opens.
#
# Shared builds produce libenginec.so / enginec.dll / libenginec.dylib, whose file
# name is the contract with Dart/native.dart. iOS has no usable shared library, so
# ENGINEC_BUILD_SHARED=OFF builds the same adapter as a static archive that Runner
# links directly; the exported symbols are still KzcEngine* so they can coexist
# with the Go bridge's KzEngine* in one process.
if(ENGINEC_BUILD_SHARED)
    add_library(enginec_ffi SHARED)
else()
    add_library(enginec_ffi STATIC)
endif()
target_sources(enginec_ffi PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/enginec_ffi.cpp")
# The C ABI itself is a module (include/enginec/ffi/api.cppm): it is ours, small,
# and every consumer is in this target, so there is no header to keep.
target_sources(enginec_ffi PRIVATE
    FILE_SET CXX_MODULES
        BASE_DIRS "${CMAKE_CURRENT_SOURCE_DIR}/include"
        FILES "${CMAKE_CURRENT_SOURCE_DIR}/include/enginec/ffi/api.cppm")
# 崩溃日志是进程级胶水（signal 处理器 + 面包屑），独立于协议/引擎逻辑，
# 保持普通 TU（只 import C ABI 声明，不 import std/std 模块初始化器），
# 静态初始化期也能运行。Windows 的 MSVC 没有
# sigaction/ucontext/dladdr/_Unwind_Backtrace，用同一 ABI 的独立实现。
if(WIN32)
    target_sources(enginec_ffi PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/crashlog_windows.cpp")
else()
    target_sources(enginec_ffi PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}/crashlog.cpp")
endif()
target_compile_definitions(enginec_ffi PRIVATE ENGINE_VERSION="${ENGINEC_VERSION}")
target_link_libraries(enginec_ffi PRIVATE enginec_core)
# The file name is what Dart opens, so it must not gain an ABI suffix:
# libenginec.so on Android/Linux.
set_target_properties(enginec_ffi PROPERTIES
    OUTPUT_NAME enginec
    CXX_VISIBILITY_PRESET hidden
    VISIBILITY_INLINES_HIDDEN ON)

enginec_apply_common_flags(enginec_ffi)

# Copies the freshly built library into the Flutter jniLibs tree so Gradle
# packages it. Keeping this as a target means CI only runs
# `cmake --build --target flutter_install` and never repeats ABI path logic.
if(ENGINEC_FLUTTER_JNILIBS AND ENGINEC_BUILD_SHARED)
    if(ANDROID)
        set(_enginec_jni_abi "${ANDROID_ABI}")
    else()
        set(_enginec_jni_abi "host")
    endif()
    add_custom_target(flutter_install
        COMMAND ${CMAKE_COMMAND} -E make_directory
            "${ENGINEC_FLUTTER_JNILIBS}/${_enginec_jni_abi}"
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
            "$<TARGET_FILE:enginec_ffi>"
            "${ENGINEC_FLUTTER_JNILIBS}/${_enginec_jni_abi}/libenginec.so"
        DEPENDS enginec_ffi
        COMMENT "Staging libenginec.so for the Flutter APK (${_enginec_jni_abi})"
        VERBATIM)
endif()
