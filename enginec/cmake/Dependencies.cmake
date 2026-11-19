# Third-party dependencies fetched by CMake itself (no system packages, no vcpkg).
#
# Every dependency is pinned to one release tag and built statically: the Flutter
# APK ships exactly one copy of each codec, and an unchanged tag keeps the CMake
# build tree cacheable in CI. Warnings are silenced on vendored targets because a
# warning there is not something this project can fix.

include(FetchContent)

# Same policy as LiarUtil.Flutter: only zlib-ng builds through its own CMake because
# its compatibility layer is what libpng/minizip expect; the other C libraries are
# compiled from explicit source lists so host and Android builds see identical
# objects and no upstream build script runs on either platform.
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

set(ZLIB_COMPAT ON CACHE BOOL "" FORCE)
set(ZLIB_ENABLE_TESTS OFF CACHE BOOL "" FORCE)
set(ZLIBNG_ENABLE_TESTS OFF CACHE BOOL "" FORCE)
set(WITH_GTEST OFF CACHE BOOL "" FORCE)
set(WITH_BENCHMARKS OFF CACHE BOOL "" FORCE)

FetchContent_Declare(zlib-ng
    GIT_REPOSITORY https://github.com/zlib-ng/zlib-ng.git
    GIT_TAG 2.3.3
    GIT_SHALLOW TRUE)
FetchContent_MakeAvailable(zlib-ng)
enginec_third_party_target(zlib-ng)

# ---- libpng ---------------------------------------------------------------
# Only the library is built: upstream's CMake would also configure tests, tools and
# an install tree. The config header is the prebuilt copy shipped in scripts/, so no
# feature detection runs on any host.
FetchContent_Declare(libpng
    GIT_REPOSITORY https://github.com/pnggroup/libpng.git
    GIT_TAG v1.6.59
    GIT_SHALLOW TRUE
    SOURCE_SUBDIR cmake-skip-subdir)
FetchContent_MakeAvailable(libpng)

configure_file("${libpng_SOURCE_DIR}/scripts/pnglibconf.h.prebuilt"
    "${CMAKE_CURRENT_BINARY_DIR}/libpng/pnglibconf.h" COPYONLY)

# libpng enables architecture-specific filter/palette routines from pngpriv.h when
# the compiler advertises the instruction set. Those symbols live in separate files,
# so the matching tree must be compiled too or the shared library links with
# undefined references (observed as png_do_expand_palette_rgba8_neon on arm64).
set(_enginec_png_arch_sources)
if(CMAKE_SYSTEM_PROCESSOR MATCHES "^(aarch64|arm64|arm)" OR ANDROID_ABI MATCHES "^arm")
    list(APPEND _enginec_png_arch_sources
        "${libpng_SOURCE_DIR}/arm/arm_init.c"
        "${libpng_SOURCE_DIR}/arm/filter_neon_intrinsics.c"
        "${libpng_SOURCE_DIR}/arm/palette_neon_intrinsics.c")
endif()
if(CMAKE_SYSTEM_PROCESSOR MATCHES "^(x86_64|amd64|AMD64|i.86)" OR ANDROID_ABI STREQUAL "x86_64")
    list(APPEND _enginec_png_arch_sources
        "${libpng_SOURCE_DIR}/intel/intel_init.c"
        "${libpng_SOURCE_DIR}/intel/filter_sse2_intrinsics.c")
endif()

add_library(enginec_png STATIC
    "${libpng_SOURCE_DIR}/png.c"
    "${libpng_SOURCE_DIR}/pngerror.c"
    "${libpng_SOURCE_DIR}/pngget.c"
    "${libpng_SOURCE_DIR}/pngmem.c"
    "${libpng_SOURCE_DIR}/pngpread.c"
    "${libpng_SOURCE_DIR}/pngread.c"
    "${libpng_SOURCE_DIR}/pngrio.c"
    "${libpng_SOURCE_DIR}/pngrtran.c"
    "${libpng_SOURCE_DIR}/pngrutil.c"
    "${libpng_SOURCE_DIR}/pngset.c"
    "${libpng_SOURCE_DIR}/pngtrans.c"
    "${libpng_SOURCE_DIR}/pngwio.c"
    "${libpng_SOURCE_DIR}/pngwrite.c"
    "${libpng_SOURCE_DIR}/pngwtran.c"
    "${libpng_SOURCE_DIR}/pngwutil.c"
    ${_enginec_png_arch_sources})
target_include_directories(enginec_png SYSTEM PUBLIC
    "${CMAKE_CURRENT_BINARY_DIR}/libpng"
    "${libpng_SOURCE_DIR}")
target_link_libraries(enginec_png PUBLIC zlib-ng)
enginec_third_party_target(enginec_png)

# ---- libjpeg-turbo --------------------------------------------------------
# libjpeg-turbo refuses add_subdirectory() by design. Two ways out: an
# ExternalProject (which would need the Android/iOS toolchain re-injected into a
# nested configure) or an explicit source list. The explicit route is what this
# project uses for every other codec, and it keeps host/Android objects identical.
# SIMD is off (no NASM dependency, irrelevant for one-off CG decodes) and only the
# decoder/encoder libraries are built.
FetchContent_Declare(libjpeg-turbo
    GIT_REPOSITORY https://github.com/libjpeg-turbo/libjpeg-turbo.git
    GIT_TAG 3.2.1
    GIT_SHALLOW TRUE
    SOURCE_SUBDIR cmake-skip-subdir)
FetchContent_MakeAvailable(libjpeg-turbo)

# jconfig.h / jconfigint.h / jversion.h are normally produced by the upstream
# configure step. Their values are fixed for a static, non-SIMD 8-bit build.
file(WRITE "${CMAKE_CURRENT_BINARY_DIR}/libjpeg/jconfig.h" [=[
#ifndef JCONFIG_H
#define JCONFIG_H
#define JPEG_LIB_VERSION 80
#define LIBJPEG_TURBO_VERSION 3.2.1
#define LIBJPEG_TURBO_VERSION_NUMBER 3002001
#define C_ARITH_CODING_SUPPORTED 1
#define D_ARITH_CODING_SUPPORTED 1
#define MEM_SRCDST_SUPPORTED 1
/* #undef WITH_SIMD */
#ifndef BITS_IN_JSAMPLE
#define BITS_IN_JSAMPLE 8
#endif
#endif
]=])

if(MSVC)
    set(_enginec_jpeg_hidden "")
    set(_enginec_jpeg_inline "__forceinline")
    set(_enginec_jpeg_thread_local "__declspec(thread)")
    set(_enginec_jpeg_intrin 1)
    set(_enginec_jpeg_intrin_def "#define HAVE_INTRIN_H 1")
    set(_enginec_jpeg_ctzl_def "/* #undef HAVE_BUILTIN_CTZL */")
else()
    set(_enginec_jpeg_hidden "__attribute__((visibility(\"hidden\")))")
    set(_enginec_jpeg_inline "inline __attribute__((always_inline))")
    set(_enginec_jpeg_thread_local "__thread")
    set(_enginec_jpeg_intrin 0)
    set(_enginec_jpeg_intrin_def "/* #undef HAVE_INTRIN_H */")
    set(_enginec_jpeg_ctzl_def "#define HAVE_BUILTIN_CTZL 1")
endif()
math(EXPR _enginec_size_t "${CMAKE_SIZEOF_VOID_P}")

file(WRITE "${CMAKE_CURRENT_BINARY_DIR}/libjpeg/jconfigint.h" "
#ifndef JCONFIGINT_H
#define JCONFIGINT_H
#define BUILD  \"\"
#define HIDDEN  ${_enginec_jpeg_hidden}
#undef inline
#define INLINE  ${_enginec_jpeg_inline}
#define THREAD_LOCAL  ${_enginec_jpeg_thread_local}
#define PACKAGE_NAME  \"KizunaEngineC\"
#define VERSION  \"3.2.1\"
#define SIZEOF_SIZE_T  ${_enginec_size_t}
${_enginec_jpeg_ctzl_def}
${_enginec_jpeg_intrin_def}
#if defined(__has_attribute)
#if __has_attribute(fallthrough)
#define FALLTHROUGH  __attribute__((fallthrough));
#else
#define FALLTHROUGH
#endif
#else
#define FALLTHROUGH
#endif
#ifndef BITS_IN_JSAMPLE
#define BITS_IN_JSAMPLE 8
#endif
#undef C_ARITH_CODING_SUPPORTED
#undef D_ARITH_CODING_SUPPORTED
#undef WITH_SIMD
#define C_ARITH_CODING_SUPPORTED 1
#define D_ARITH_CODING_SUPPORTED 1
#define SIMD_ARCHITECTURE  NONE
#endif
")

file(WRITE "${CMAKE_CURRENT_BINARY_DIR}/libjpeg/jversion.h" [=[
#ifndef JVERSION_H
#define JVERSION_H
#define JVERSION "8d  15-Jan-2012"
#define JCOPYRIGHT "Copyright (C) 1991-2026 The libjpeg-turbo Project and many others"
#endif
]=])

set(_enginec_jpeg_src "${libjpeg-turbo_SOURCE_DIR}/src")
add_library(enginec_jpeg STATIC
    "${_enginec_jpeg_src}/jcapimin.c"
    "${_enginec_jpeg_src}/wrapper/jcapistd-8.c"
    "${_enginec_jpeg_src}/wrapper/jcapistd-12.c"
    "${_enginec_jpeg_src}/wrapper/jcapistd-16.c"
    "${_enginec_jpeg_src}/wrapper/jccoefct-8.c"
    "${_enginec_jpeg_src}/wrapper/jccoefct-12.c"
    "${_enginec_jpeg_src}/wrapper/jccolor-8.c"
    "${_enginec_jpeg_src}/wrapper/jccolor-12.c"
    "${_enginec_jpeg_src}/wrapper/jccolor-16.c"
    "${_enginec_jpeg_src}/wrapper/jcdctmgr-8.c"
    "${_enginec_jpeg_src}/wrapper/jcdctmgr-12.c"
    "${_enginec_jpeg_src}/wrapper/jcdiffct-8.c"
    "${_enginec_jpeg_src}/wrapper/jcdiffct-12.c"
    "${_enginec_jpeg_src}/wrapper/jcdiffct-16.c"
    "${_enginec_jpeg_src}/jchuff.c"
    "${_enginec_jpeg_src}/jcicc.c"
    "${_enginec_jpeg_src}/jcinit.c"
    "${_enginec_jpeg_src}/jclhuff.c"
    "${_enginec_jpeg_src}/wrapper/jclossls-8.c"
    "${_enginec_jpeg_src}/wrapper/jclossls-12.c"
    "${_enginec_jpeg_src}/wrapper/jclossls-16.c"
    "${_enginec_jpeg_src}/wrapper/jcmainct-8.c"
    "${_enginec_jpeg_src}/wrapper/jcmainct-12.c"
    "${_enginec_jpeg_src}/wrapper/jcmainct-16.c"
    "${_enginec_jpeg_src}/jcmarker.c"
    "${_enginec_jpeg_src}/jcmaster.c"
    "${_enginec_jpeg_src}/jcomapi.c"
    "${_enginec_jpeg_src}/jcparam.c"
    "${_enginec_jpeg_src}/jcphuff.c"
    "${_enginec_jpeg_src}/wrapper/jcprepct-8.c"
    "${_enginec_jpeg_src}/wrapper/jcprepct-12.c"
    "${_enginec_jpeg_src}/wrapper/jcprepct-16.c"
    "${_enginec_jpeg_src}/wrapper/jcsample-8.c"
    "${_enginec_jpeg_src}/wrapper/jcsample-12.c"
    "${_enginec_jpeg_src}/wrapper/jcsample-16.c"
    "${_enginec_jpeg_src}/jctrans.c"
    "${_enginec_jpeg_src}/jdapimin.c"
    "${_enginec_jpeg_src}/wrapper/jdapistd-8.c"
    "${_enginec_jpeg_src}/wrapper/jdapistd-12.c"
    "${_enginec_jpeg_src}/wrapper/jdapistd-16.c"
    "${_enginec_jpeg_src}/jdatadst.c"
    "${_enginec_jpeg_src}/jdatasrc.c"
    "${_enginec_jpeg_src}/wrapper/jdcoefct-8.c"
    "${_enginec_jpeg_src}/wrapper/jdcoefct-12.c"
    "${_enginec_jpeg_src}/wrapper/jdcolor-8.c"
    "${_enginec_jpeg_src}/wrapper/jdcolor-12.c"
    "${_enginec_jpeg_src}/wrapper/jdcolor-16.c"
    "${_enginec_jpeg_src}/wrapper/jddctmgr-8.c"
    "${_enginec_jpeg_src}/wrapper/jddctmgr-12.c"
    "${_enginec_jpeg_src}/wrapper/jddiffct-8.c"
    "${_enginec_jpeg_src}/wrapper/jddiffct-12.c"
    "${_enginec_jpeg_src}/wrapper/jddiffct-16.c"
    "${_enginec_jpeg_src}/jdhuff.c"
    "${_enginec_jpeg_src}/jdicc.c"
    "${_enginec_jpeg_src}/jdinput.c"
    "${_enginec_jpeg_src}/jdlhuff.c"
    "${_enginec_jpeg_src}/wrapper/jdlossls-8.c"
    "${_enginec_jpeg_src}/wrapper/jdlossls-12.c"
    "${_enginec_jpeg_src}/wrapper/jdlossls-16.c"
    "${_enginec_jpeg_src}/wrapper/jdmainct-8.c"
    "${_enginec_jpeg_src}/wrapper/jdmainct-12.c"
    "${_enginec_jpeg_src}/wrapper/jdmainct-16.c"
    "${_enginec_jpeg_src}/jdmarker.c"
    "${_enginec_jpeg_src}/jdmaster.c"
    "${_enginec_jpeg_src}/wrapper/jdmerge-8.c"
    "${_enginec_jpeg_src}/wrapper/jdmerge-12.c"
    "${_enginec_jpeg_src}/jdphuff.c"
    "${_enginec_jpeg_src}/wrapper/jdpostct-8.c"
    "${_enginec_jpeg_src}/wrapper/jdpostct-12.c"
    "${_enginec_jpeg_src}/wrapper/jdpostct-16.c"
    "${_enginec_jpeg_src}/wrapper/jdsample-8.c"
    "${_enginec_jpeg_src}/wrapper/jdsample-12.c"
    "${_enginec_jpeg_src}/wrapper/jdsample-16.c"
    "${_enginec_jpeg_src}/jdtrans.c"
    "${_enginec_jpeg_src}/jerror.c"
    "${_enginec_jpeg_src}/jfdctflt.c"
    "${_enginec_jpeg_src}/wrapper/jfdctfst-8.c"
    "${_enginec_jpeg_src}/wrapper/jfdctfst-12.c"
    "${_enginec_jpeg_src}/wrapper/jfdctint-8.c"
    "${_enginec_jpeg_src}/wrapper/jfdctint-12.c"
    "${_enginec_jpeg_src}/wrapper/jidctflt-8.c"
    "${_enginec_jpeg_src}/wrapper/jidctflt-12.c"
    "${_enginec_jpeg_src}/wrapper/jidctfst-8.c"
    "${_enginec_jpeg_src}/wrapper/jidctfst-12.c"
    "${_enginec_jpeg_src}/wrapper/jidctint-8.c"
    "${_enginec_jpeg_src}/wrapper/jidctint-12.c"
    "${_enginec_jpeg_src}/wrapper/jidctred-8.c"
    "${_enginec_jpeg_src}/wrapper/jidctred-12.c"
    "${_enginec_jpeg_src}/jmemmgr.c"
    "${_enginec_jpeg_src}/jmemnobs.c"
    "${_enginec_jpeg_src}/jpeg_nbits.c"
    "${_enginec_jpeg_src}/wrapper/jquant1-8.c"
    "${_enginec_jpeg_src}/wrapper/jquant1-12.c"
    "${_enginec_jpeg_src}/wrapper/jquant2-8.c"
    "${_enginec_jpeg_src}/wrapper/jquant2-12.c"
    "${_enginec_jpeg_src}/wrapper/jutils-8.c"
    "${_enginec_jpeg_src}/wrapper/jutils-12.c"
    "${_enginec_jpeg_src}/wrapper/jutils-16.c"
    "${_enginec_jpeg_src}/jaricom.c"
    "${_enginec_jpeg_src}/jcarith.c"
    "${_enginec_jpeg_src}/jdarith.c"
    # The KiriKiri JPEG loader uses the TurboJPEG API (tj*), not the raw libjpeg
    # API, so these four units are part of the library even though the CLI tools
    # are disabled.
    "${_enginec_jpeg_src}/turbojpeg.c"
    "${_enginec_jpeg_src}/transupp.c"
    "${_enginec_jpeg_src}/jdatadst-tj.c"
    "${_enginec_jpeg_src}/jdatasrc-tj.c"
    # turbojpeg-mp.c (included by turbojpeg.c) also implements tjLoadImage/
    # tjSaveImage, which reference the BMP/PNM/PNG adapters. They are part of the
    # library even though KiriKiri only calls the memory decompressor.
    "${_enginec_jpeg_src}/rdbmp.c"
    "${_enginec_jpeg_src}/wrbmp.c"
    "${_enginec_jpeg_src}/wrapper/rdppm-8.c"
    "${_enginec_jpeg_src}/wrapper/rdppm-12.c"
    "${_enginec_jpeg_src}/wrapper/rdppm-16.c"
    "${_enginec_jpeg_src}/wrapper/wrppm-8.c"
    "${_enginec_jpeg_src}/wrapper/wrppm-12.c"
    "${_enginec_jpeg_src}/wrapper/wrppm-16.c"
    "${_enginec_jpeg_src}/wrapper/rdpng-8.c"
    "${_enginec_jpeg_src}/wrapper/rdpng-12.c"
    "${_enginec_jpeg_src}/wrapper/rdpng-16.c"
    "${_enginec_jpeg_src}/wrapper/wrpng-8.c"
    "${_enginec_jpeg_src}/wrapper/wrpng-12.c"
    "${_enginec_jpeg_src}/wrapper/wrpng-16.c"
    "${_enginec_jpeg_src}/spng/spng.c")
target_include_directories(enginec_jpeg SYSTEM PUBLIC
    "${CMAKE_CURRENT_BINARY_DIR}/libjpeg"
    "${libjpeg-turbo_SOURCE_DIR}/src"
    "${libjpeg-turbo_SOURCE_DIR}/src/spng")
target_link_libraries(enginec_jpeg PUBLIC zlib-ng)
target_compile_features(enginec_jpeg PUBLIC c_std_99)
# turbojpeg-mp.c implements tjLoadImage/tjSaveImage unconditionally and expects the
# BMP/PNM/PNG adapters to be compiled. SPNG is the in-tree PNG reader (no libpng
# dependency); these defines are exactly what upstream's TURBOJPEG target uses.
target_compile_definitions(enginec_jpeg PRIVATE
    BMP_SUPPORTED=1 PPM_SUPPORTED=1 PNG_SUPPORTED=1 SPNG_STATIC=1)
enginec_third_party_target(enginec_jpeg)
enginec_third_party_no_strict_aliasing(enginec_jpeg)

# ---- libwebp --------------------------------------------------------------
# Only the decoder/encoder libraries; every command-line tool and the demux/mux
# helpers are off because the engine decodes complete WebP images from memory.
set(WEBP_BUILD_ANIM_UTILS OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_CWEBP OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_DWEBP OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_GIF2WEBP OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_IMG2WEBP OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_VWEBP OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_WEBPINFO OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_WEBPMUX OFF CACHE BOOL "" FORCE)
set(WEBP_BUILD_EXTRAS OFF CACHE BOOL "" FORCE)
set(WEBP_ENABLE_SIMD ON CACHE BOOL "" FORCE)

FetchContent_Declare(libwebp
    GIT_REPOSITORY https://github.com/webmproject/libwebp.git
    GIT_TAG v1.6.0
    GIT_SHALLOW TRUE)
FetchContent_MakeAvailable(libwebp)
enginec_third_party_target(webp)
enginec_third_party_no_strict_aliasing(webp)

# libwebp disables SSE2 on its portable units to keep a baseline instruction set.
# On x86_64 that makes clang reject _Float16 declarations in the macOS 26 SDK's
# math.h (and breaks sharpyuv's own SSE typedefs when SIMD is off). Appending
# -msse2 after libwebp's options restores the baseline every x86_64 Mac/PC has.
set(_enginec_webp_x86 FALSE)
if(CMAKE_SYSTEM_PROCESSOR MATCHES "^(x86_64|amd64|AMD64)")
    set(_enginec_webp_x86 TRUE)
endif()
if(CMAKE_OSX_ARCHITECTURES STREQUAL "x86_64" OR ANDROID_ABI STREQUAL "x86_64")
    set(_enginec_webp_x86 TRUE)
endif()
if(_enginec_webp_x86)
    foreach(_enginec_webp_target webp sharpyuv)
        if(TARGET ${_enginec_webp_target})
            target_compile_options(${_enginec_webp_target} PRIVATE
                $<$<C_COMPILER_ID:Clang,GNU>:-msse2>
                $<$<CXX_COMPILER_ID:Clang,GNU>:-msse2>)
        endif()
    endforeach()
endif()

# ---- PCRE2 ----------------------------------------------------------------
# TJS2's RegExp class originally used Oniguruma. PCRE2 is the replacement required
# by this port: the 8-bit library with JIT covers every pattern TJS scripts use and
# is actively maintained. The 16/32-bit libraries, grep tool and tests are off.
set(PCRE2_BUILD_PCRE2_8 ON CACHE BOOL "" FORCE)
set(PCRE2_BUILD_PCRE2_16 OFF CACHE BOOL "" FORCE)
set(PCRE2_BUILD_PCRE2_32 OFF CACHE BOOL "" FORCE)
set(PCRE2_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(PCRE2_BUILD_PCRE2GREP OFF CACHE BOOL "" FORCE)
# PCRE2's JIT uses sljit's executable-memory hooks; on iOS the Apple-specific
# allocator references W^X helpers that the iOS SDK does not expose. The interpreter
# is fast enough for TJS regexes, so JIT is disabled only for iOS.
# JIT is disabled everywhere: it mmaps RWX trampolines, which some hardened
# Android/iOS kernels reject at runtime, and TJS regexes are not hot enough to
# matter. The interpreter is the portable path.
set(PCRE2_SUPPORT_JIT OFF CACHE BOOL "" FORCE)
set(PCRE2_SUPPORT_UNICODE ON CACHE BOOL "" FORCE)

FetchContent_Declare(pcre2
    GIT_REPOSITORY https://github.com/PCRE2Project/pcre2.git
    GIT_TAG pcre2-10.49
    GIT_SHALLOW TRUE)
FetchContent_MakeAvailable(pcre2)
enginec_third_party_target(pcre2-8)

# ---- minizip-ng -----------------------------------------------------------
# ZIP containers (some KiriKiri distributions and patch archives are plain ZIP).
# Only the deflate/store path and the zlib backend are needed; encryption, bzip2,
# lzma, zstd and ppmd are disabled so no extra dependency is pulled in.
FetchContent_Declare(minizip_ng
    GIT_REPOSITORY https://github.com/zlib-ng/minizip-ng.git
    GIT_TAG 4.2.2
    GIT_SHALLOW TRUE
    SOURCE_SUBDIR cmake-skip-subdir)
FetchContent_MakeAvailable(minizip_ng)

add_library(enginec_minizip STATIC
    "${minizip_ng_SOURCE_DIR}/mz_crypt.c"
    "${minizip_ng_SOURCE_DIR}/mz_os.c"
    "${minizip_ng_SOURCE_DIR}/mz_strm.c"
    "${minizip_ng_SOURCE_DIR}/mz_strm_buf.c"
    "${minizip_ng_SOURCE_DIR}/mz_strm_mem.c"
    "${minizip_ng_SOURCE_DIR}/mz_strm_split.c"
    "${minizip_ng_SOURCE_DIR}/mz_strm_zlib.c"
    "${minizip_ng_SOURCE_DIR}/mz_zip.c"
    "${minizip_ng_SOURCE_DIR}/mz_zip_rw.c"
    "$<IF:$<BOOL:${WIN32}>,${minizip_ng_SOURCE_DIR}/mz_os_win32.c,${minizip_ng_SOURCE_DIR}/mz_os_posix.c>"
    "$<IF:$<BOOL:${WIN32}>,${minizip_ng_SOURCE_DIR}/mz_strm_os_win32.c,${minizip_ng_SOURCE_DIR}/mz_strm_os_posix.c>")

# mz_config.h is normally produced by minizip-ng's configure step. Writing it here
# keeps the dependency on one pinned tag and one header, with no upstream build
# script running. The flags match the explicit source list above.
# dirent/symlink are POSIX-only; defining them on Windows makes mz_os.h include a
# header MSVC does not ship. The remaining flags are the same on every target.
if(WIN32)
    set(_enginec_mz_dirent 0)
    set(_enginec_mz_pdir 0)
    set(_enginec_mz_symlink 0)
    set(_enginec_mz_readlink 0)
else()
    set(_enginec_mz_dirent 1)
    set(_enginec_mz_pdir 1)
    set(_enginec_mz_symlink 1)
    set(_enginec_mz_readlink 1)
endif()
file(WRITE "${CMAKE_CURRENT_BINARY_DIR}/minizip/mz_config.h" "
#ifndef MZ_CONFIG_H
#define MZ_CONFIG_H
#define HAVE_DIRENT_H ${_enginec_mz_dirent}
#define HAVE_SYS_DIRENT_H 0
#define HAVE_INTTYPES_H 1
#define HAVE_STDINT_H 1
#define HAVE_PDIR ${_enginec_mz_pdir}
#define HAVE_FSEEKO 1
#define HAVE_SYMLINK ${_enginec_mz_symlink}
#define HAVE_READLINK ${_enginec_mz_readlink}
#endif
")

target_include_directories(enginec_minizip SYSTEM PUBLIC
    "${minizip_ng_SOURCE_DIR}"
    "${CMAKE_CURRENT_BINARY_DIR}/minizip")
set(_enginec_minizip_defines
    HAVE_ZLIB=1
    ZLIB_COMPAT=1
    MZ_ZIP_NO_CRYPTO=1
    MZ_ZIP_NO_ENCRYPTION=1)
if(APPLE)
    # Apple hides BSD extensions (mkdtemp, O_NOFOLLOW) when _POSIX_C_SOURCE is
    # defined without _DARWIN_C_SOURCE.
    list(APPEND _enginec_minizip_defines _DARWIN_C_SOURCE=1)
elseif(NOT WIN32)
    list(APPEND _enginec_minizip_defines
        _POSIX_C_SOURCE=200809L _BSD_SOURCE _DEFAULT_SOURCE)
endif()
target_compile_definitions(enginec_minizip PRIVATE ${_enginec_minizip_defines})
target_compile_features(enginec_minizip PUBLIC c_std_99)
target_link_libraries(enginec_minizip PUBLIC zlib-ng)
enginec_third_party_target(enginec_minizip)

# ---- SDL3 ---------------------------------------------------------------
# The vendored KiriKiri engine's environment layer is written against SDL3. This
# build uses the offscreen video driver and the dummy audio device: the engine
# renders into an SDL texture that the autotest host reads back, so no Android
# Activity or iOS UIWindow is involved.
set(SDL_SHARED OFF CACHE BOOL "" FORCE)
set(SDL_STATIC ON CACHE BOOL "" FORCE)
set(SDL_TEST OFF CACHE BOOL "" FORCE)
set(SDL_TESTS OFF CACHE BOOL "" FORCE)
set(SDL_EXAMPLES OFF CACHE BOOL "" FORCE)
set(SDL_INSTALL OFF CACHE BOOL "" FORCE)
# The engine runs offscreen; do not require X11/Wayland to configure SDL3 on
# Linux CI or the host test machine.
set(SDL_UNIX_CONSOLE_BUILD ON CACHE BOOL "" FORCE)
# Only the offscreen driver is used. Disabling X11/Wayland removes their package
# probes (XSCRNSAVER, xkbcommon, ...) from Linux CI, where they are not installed.
set(SDL_X11 OFF CACHE BOOL "" FORCE)
set(SDL_WAYLAND OFF CACHE BOOL "" FORCE)
set(SDL_KMSDRM OFF CACHE BOOL "" FORCE)
set(SDL_OFFSCREEN ON CACHE BOOL "" FORCE)
set(SDL_DISABLE_INSTALL OFF CACHE BOOL "" FORCE)
FetchContent_Declare(sdl3
    GIT_REPOSITORY https://github.com/libsdl-org/SDL.git
    GIT_TAG release-3.4.20
    GIT_SHALLOW TRUE
    # 嵌入式补丁：无 SDLActivity 的构建里 JNI_OnLoad 不会跑、mJavaVM 恒为空，
    # SDL 的 Android 资源/包名查询会解引用空指针（实测 TVPEnumFontsProc ->
    # SDL_GetPathInfo -> PrepareAPK SIGSEGV at 0x0）。补丁在入口直接判空返回，
    # 让调用方走普通 stat/IO 路径。只读两个函数，升级 SDL 时需重核。
    # 幂等：构建树缓存会恢复"已打补丁"的源码，而 populate 的 stamp 可能缺失
    # 导致 PATCH 步骤重跑；前向应用失败时用反向校验确认已应用即算成功。
    PATCH_COMMAND bash -c "git apply --whitespace=nowarn '${CMAKE_CURRENT_SOURCE_DIR}/cmake/patches/sdl3-embedded-no-jni.patch' || git apply -R --check '${CMAKE_CURRENT_SOURCE_DIR}/cmake/patches/sdl3-embedded-no-jni.patch'")
FetchContent_MakeAvailable(sdl3)
enginec_third_party_target(SDL3-static)
# SDL3's Android hidapi source is C++ with a precompiled header. Our project-wide
# CMAKE_CXX_SCAN_FOR_MODULES makes CMake run clang-scan-deps on it, which cannot
# combine a PCH with P1689 scanning. SDL does not use modules, so opt it out.
if(TARGET SDL3-static)
    set_target_properties(SDL3-static PROPERTIES CXX_SCAN_FOR_MODULES OFF)
endif()

# ---- FreeType -----------------------------------------------------------
# KiriKiri's TVPFont/FreeTypeImp rasterizes TTF/OTF fonts that live inside the
# game archive. HarfBuzz/Brotli/PNG support are irrelevant here and would add a
# second copy of those codecs, so they are disabled.
set(FT_DISABLE_HARFBUZZ ON CACHE BOOL "" FORCE)
set(FT_DISABLE_BROTLI ON CACHE BOOL "" FORCE)
set(FT_DISABLE_PNG ON CACHE BOOL "" FORCE)
set(FT_DISABLE_ZLIB ON CACHE BOOL "" FORCE)
set(FT_DISABLE_BZIP2 ON CACHE BOOL "" FORCE)
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
FetchContent_Declare(freetype
    GIT_REPOSITORY https://github.com/freetype/freetype.git
    GIT_TAG VER-2-13-3
    GIT_SHALLOW TRUE)
FetchContent_MakeAvailable(freetype)
enginec_third_party_target(freetype)

# ---- plutovg --------------------------------------------------------------
# layerExDraw (published as layerExDraw.dll, and the "GdiPlus" namespace games
# use for vector text) draws through plutovg: paths, brushes, gradients and
# glyph outlines. KiriKiri's own krkrsdl3 build vendors exactly this library, so
# the plugin source is compiled unmodified against it.
#
# HAVE_THREADS_H is forced off on purpose: plutovg then uses a no-op glyph-cache
# mutex. The cache is only touched from the script thread (LayerExDraw's draw
# calls), and bionic's C11 <threads.h> entry points are @INTRODUCED_IN(30) while
# this APK targets API 24 — enabling them would fail the link with undefined
# references to mtx_init/mtx_lock.
set(HAVE_THREADS_H OFF CACHE BOOL "" FORCE)
FetchContent_Declare(plutovg
    GIT_REPOSITORY https://github.com/sammycage/plutovg.git
    GIT_TAG v1.3.3
    GIT_SHALLOW TRUE)
FetchContent_MakeAvailable(plutovg)
enginec_third_party_target(plutovg)

# ---- glm ------------------------------------------------------------------
# EmotePlayer (M2's E-mote) is written against glm's math types. The library is
# header-only, so only the include directory is needed; SOURCE_SUBDIR points at a
# directory that does not exist to keep glm's own CMakeLists (which installs
# compile options at directory scope) out of this build.
FetchContent_Declare(glm
    GIT_REPOSITORY https://github.com/g-truc/glm.git
    GIT_TAG 1.0.1
    GIT_SHALLOW TRUE
    SOURCE_SUBDIR cmake-skip-subdir)
FetchContent_MakeAvailable(glm)
add_library(enginec_glm INTERFACE)
target_include_directories(enginec_glm INTERFACE "${glm_SOURCE_DIR}")

# ---- libbpg ---------------------------------------------------------------
# Kirikiroid2/LoadBPG.cpp decodes .bpg images through Fabrice Bellard's libbpg.
# The decoder is a reduced, older FFmpeg HEVC implementation, so it must never
# resolve av_/ff_ symbols from another FFmpeg in the process. The upstream tree
# is downloaded once and copied into the build directory with every such
# identifier prefixed `krkr_bpg_`, headers included; only libbpg.h stays public.
# This is the same isolation the official krkrsdl3 build uses.
#
# Set FETCHCONTENT_SOURCE_DIR_KRKR_BPG_UPSTREAM to an extracted libbpg-0.9.8
# tree to build offline.
FetchContent_Declare(krkr_bpg_upstream
    URL https://bellard.org/bpg/libbpg-0.9.8.tar.gz
    URL_HASH SHA512=2d5f7a035033d8b969c2765a4f1368e1a2c8c5ace12aec5d449ca5cc4ef5eb89431a75f2d6d03fd7117b548801b5554503411f499f497da69ff33ca6e714553d
    DOWNLOAD_EXTRACT_TIMESTAMP TRUE)
FetchContent_MakeAvailable(krkr_bpg_upstream)

set(_bpg_private "${CMAKE_CURRENT_BINARY_DIR}/krkr_bpg")
file(GLOB_RECURSE _bpg_files RELATIVE "${krkr_bpg_upstream_SOURCE_DIR}"
    "${krkr_bpg_upstream_SOURCE_DIR}/libavcodec/*.c"
    "${krkr_bpg_upstream_SOURCE_DIR}/libavcodec/*.h"
    "${krkr_bpg_upstream_SOURCE_DIR}/libavutil/*.c"
    "${krkr_bpg_upstream_SOURCE_DIR}/libavutil/*.h")
list(APPEND _bpg_files config.h libbpg.c libbpg.h)
foreach(_file IN LISTS _bpg_files)
    file(READ "${krkr_bpg_upstream_SOURCE_DIR}/${_file}" _text)
    # Rename definitions, references and token-pasting macros together; a partial
    # rename would still let another FFmpeg interpose libbpg's internal helpers.
    string(REGEX REPLACE "(^|[^A-Za-z0-9_])(av_|avcodec_|avpriv_|ff_)"
        "\\1krkr_bpg_\\2" _text "${_text}")
    if(_file STREQUAL "config.h" AND CMAKE_C_BYTE_ORDER STREQUAL "BIG_ENDIAN")
        string(REPLACE "#define HAVE_BIGENDIAN 0" "#define HAVE_BIGENDIAN 1" _text "${_text}")
    elseif(_file STREQUAL "libavcodec/hevcdsp.c")
        # Upstream guards the x86 dispatcher only in the full FFmpeg build; this
        # reduced tree builds no assembly, so make the guard explicit.
        string(REPLACE "    if (ARCH_X86)\n        krkr_bpg_ff_hevc_dsp_init_x86(hevcdsp, bit_depth);"
            "#if ARCH_X86\n    krkr_bpg_ff_hevc_dsp_init_x86(hevcdsp, bit_depth);\n#endif" _text "${_text}")
    elseif(_file STREQUAL "libbpg.c")
        # LoadHeaderBPG uses bpg_decoder_get_info_from_buf, which upstream hides
        # behind #ifndef EMSCRIPTEN. The implementation itself does not depend on
        # the trimmed extension-data feature, so expose it for every platform.
        string(REPLACE "#ifndef EMSCRIPTEN\nvoid bpg_decoder_keep_extension_data"
            "#if 1\nvoid bpg_decoder_keep_extension_data" _text "${_text}")
    endif()
    get_filename_component(_dir "${_bpg_private}/${_file}" DIRECTORY)
    file(MAKE_DIRECTORY "${_dir}")
    # COPYONLY keeps the timestamp when the content did not change, so an
    # unrelated reconfigure does not rebuild the decoder.
    file(WRITE "${_bpg_private}/prepared" "${_text}")
    configure_file("${_bpg_private}/prepared" "${_bpg_private}/${_file}" COPYONLY)
endforeach()
file(REMOVE "${_bpg_private}/prepared")

set(_bpg_sources
    libavcodec/hevc_cabac.c libavcodec/hevc_filter.c libavcodec/hevc.c
    libavcodec/hevcpred.c libavcodec/hevc_refs.c libavcodec/hevcdsp.c
    libavcodec/hevc_mvs.c libavcodec/hevc_ps.c libavcodec/hevc_sei.c
    libavcodec/utils.c libavcodec/cabac.c libavcodec/golomb.c libavcodec/videodsp.c
    libavutil/mem.c libavutil/buffer.c libavutil/log2_tab.c libavutil/frame.c
    libavutil/pixdesc.c libavutil/md5.c libbpg.c)
list(TRANSFORM _bpg_sources PREPEND "${_bpg_private}/")
add_library(enginec_bpg STATIC ${_bpg_sources})
set_target_properties(enginec_bpg PROPERTIES
    C_STANDARD 99 C_STANDARD_REQUIRED ON POSITION_INDEPENDENT_CODE ON
    C_VISIBILITY_PRESET hidden)
target_compile_definitions(enginec_bpg PRIVATE
    HAVE_AV_CONFIG_H USE_VAR_BIT_DEPTH USE_PRED)
# A handful of helpers have no av_/ff_ prefix; keep those names private too. The
# replacements only affect identifiers, never #include file names.
foreach(_symbol get_bits get_bits1 get_bits_long get_se_golomb_long get_ue_golomb_long
        show_bits show_bits1 skip_bits skip_bits1 hevc_transform_init intra_pred)
    target_compile_definitions(enginec_bpg PRIVATE "${_symbol}=krkr_bpg_${_symbol}")
endforeach()
target_include_directories(enginec_bpg PRIVATE "${_bpg_private}")
file(MAKE_DIRECTORY "${_bpg_private}/include")
configure_file("${krkr_bpg_upstream_SOURCE_DIR}/libbpg.h"
    "${_bpg_private}/include/libbpg.h" COPYONLY)
target_include_directories(enginec_bpg INTERFACE "${_bpg_private}/include")
if(NOT WIN32)
    target_link_libraries(enginec_bpg PRIVATE m)
endif()
enginec_third_party_target(enginec_bpg)
