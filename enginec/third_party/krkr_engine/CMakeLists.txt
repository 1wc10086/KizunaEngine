# Vendored KiriKiri engine (krkrsdl3 tree).
#
# This is the real engine implementation: core, TJS2, the SDL3 environment and
# the platform hooks. It is compiled with the SDL3 "offscreen" video driver and a
# dummy audio device, so the whole runtime can live inside a Flutter isolate with
# no window and no Activity. Frame capture and input injection go through the
# engine's own autotest host (environ/PlatformAutoTest.h).
#
# Everything in this directory is third_party: includes are expected and the
# warnings are not ours to fix.

set(_krkr_root "${CMAKE_CURRENT_SOURCE_DIR}")

file(GLOB_RECURSE _krkr_core_sources CONFIGURE_DEPENDS
    "${_krkr_root}/core/*.cpp" "${_krkr_root}/core/*.c")
# GPU backends need glad/Vulkan headers and a real swapchain. This build always
# uses the software backend (SDL offscreen window), so they are not compiled.
list(FILTER _krkr_core_sources EXCLUDE REGEX "/core/render/backend/(GL|Vulkan)RenderBackend\\.cpp$")
# Movie decoding is an optional FFmpeg integration; the platform video hooks in
# environ/ provide a null player, and SDL/the host can play movies later.
list(FILTER _krkr_core_sources EXCLUDE REGEX "/core/media/movie/")
file(GLOB _krkr_tjs_sources CONFIGURE_DEPENDS "${_krkr_root}/tjs2/*.cpp")

set(_krkr_env_sources
    "${_krkr_root}/shim/oniguruma.cpp"
    "${_krkr_root}/environ/Platform.cpp"
    "${_krkr_root}/environ/PlatformHostPaths.cpp"
    # Host media materialisation (audio samples, movies): one implementation for
    # both hooks, so an XP3-backed sample resolves the same way in either path.
    "${_krkr_root}/environ/PlatformMediaCache.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_app.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_audio.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_autotest.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_dialog.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_file.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_input.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_thread.cpp"
    "${_krkr_root}/environ/sdl3/sdl3_utils.cpp")

if(ANDROID)
    # android_video.cpp targets API 28 MediaExtractor/ASharedMemory and a newer
    # player class hierarchy; this build renders frames offscreen and delegates
    # movies to the host, so only the core file is needed.
    list(APPEND _krkr_env_sources "${_krkr_root}/environ/android/android_core.cpp"
                                  "${_krkr_root}/environ/unix/unix_file.cpp"
                                  "${_krkr_root}/environ/unix/unix_sysinfo.cpp")
elseif(WIN32)
    # windows_video.cpp uses a newer Media Foundation player hierarchy and a
    # missing mutex include; this build delegates movies to the host.
    list(APPEND _krkr_env_sources "${_krkr_root}/environ/windows/windows_core.cpp")
elseif(APPLE)
    # Apple's core supplies NSBundle resource lookup, OS name and video stubs.
    list(APPEND _krkr_env_sources
        "${_krkr_root}/environ/apple/apple_core.mm"
        "${_krkr_root}/environ/apple/apple_video.mm"
        # POSIX file media (TVPCreateFileMedia / TVPPreNormalizeStorageName) is
        # shared by Linux and Apple; Apple core supplies the rest.
        "${_krkr_root}/environ/unix/unix_file.cpp")
    if(CMAKE_SYSTEM_NAME STREQUAL "iOS")
        list(APPEND _krkr_env_sources "${_krkr_root}/environ/iOS/ios_core.mm")
    else()
        list(APPEND _krkr_env_sources "${_krkr_root}/environ/macos/macos_core.mm")
    endif()
else()
    # linux_core.cpp carries the platform identity; unix_file.cpp implements the
    # "file" storage media (TVPStorage.cpp constructs it, so it is required for
    # linking) and unix_sysinfo.cpp answers the memory queries from /proc.
    # Threading comes from sdl3_thread.cpp: unix_thread.cpp is the emscripten
    # variant and is not built.
    list(APPEND _krkr_env_sources "${_krkr_root}/environ/linux/linux_core.cpp"
                                  "${_krkr_root}/environ/unix/unix_file.cpp"
                                  "${_krkr_root}/environ/unix/unix_sysinfo.cpp")
endif()

# NCB plugins. Every module below is compiled into the archive and registers a
# static constructor, so it exists even though nothing references its symbols;
# whether a game can use it still depends on the module being loaded
# (TVPLoadInternalPlugins for the modules retail games assume are present,
# Plugins.link("xxx.dll") for the rest).
#
# The list is the official krkrsdl3 build's plugin set (krkrsdl3_build's
# cmake/cmake_plugins.txt) minus the modules whose dependency this project does
# not vendor:
#   * wuffmpeg/wuopus/wuvorbis need FFmpeg/opusfile/libvorbis. Sound is delegated
#     to the host here (environ/PlatformAudioHook.h): the engine keeps the
#     WaveSoundBuffer state machine, Flutter decodes and plays the file, so
#     in-engine decoders would only feed SDL's dummy device;
#   * LoadBPG is compiled against the private libbpg build (see
#     Dependencies.cmake), so BPG images load like any other format.
# LayerExSave.cpp is empty upstream (the class never shipped), so it adds nothing.
#
# LayerExDraw and emoteplayer are the two modules that are not optional in
# practice: a KAG message window is built with Layer.clear()/Layer.drawString()
# and GdiPlus at construction time, so a game that expects layerExDraw.dll and
# does not get it dies in the middle of MainWindow's constructor (the symptom is
# "Member GdiPlus does not exist" and a title screen that ignores every click).
# plutovg and glm are fetched by the root CMakeLists; sqlite3, the ZIP/TAR/7z
# readers and the 7-Zip decoder are entirely in-tree.
set(_krkr_plugin_sources
    # plugin manager / helpers
    "${_krkr_root}/plugins/TVPPlugin.cpp"
    "${_krkr_root}/plugins/FuncStubs.cpp"
    "${_krkr_root}/plugins/kizuna_stubs.cpp"
    "${_krkr_root}/plugins/ncbind/ncbind.cpp"
    # KAG parser (the extended implementation KAG games rely on)
    "${_krkr_root}/plugins/ExtKAGParser/ExtKAG.cpp"
    "${_krkr_root}/plugins/ExtKAGParser/ExtKAGParser.cpp"
    "${_krkr_root}/plugins/ExtKAGParser/TJSAry.cpp"
    "${_krkr_root}/plugins/ExtKAGParser/TJSDic.cpp"
    # KAGParserEx: what krkrz-era games (Cuffs/Nexton, Azarashi) link as
    # "KAGParserEx.dll" in Initialize.tjs. It registers itself under the same
    # global name ("KAGParser") and saves/restores the previous class, exactly like
    # the retail plugin, so loading it replaces ExtKAGParser only for games that
    # ask for it. It is deliberately NOT preloaded: older KAG3 games must keep the
    # ExtKAGParser behaviour they were written against.
    "${_krkr_root}/plugins/KAGParserEx/KAG.cpp"
    "${_krkr_root}/plugins/KAGParserEx/KAGParserEx.cpp"
    # core game support: PNG/JPEG/WebP loading, XP3 filters, dialogs, saves
    "${_krkr_root}/plugins/Kirikiroid2/kirikiroid2.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/LoadPNG.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/LoadJPEG.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/LoadWEBP.cpp"
    # BPG (an HEVC-derived still image format used by a few 2015+ titles).
    # Decoding goes through the private libbpg build; see Dependencies.cmake.
    "${_krkr_root}/plugins/Kirikiroid2/LoadBPG.cpp"
    "${_krkr_root}/plugins/xp3filter.cpp"
    "${_krkr_root}/plugins/win32dialog.cpp"
    "${_krkr_root}/plugins/varfile.cpp"
    "${_krkr_root}/plugins/shrinkCopy.cpp"
    "${_krkr_root}/plugins/saveStruct.cpp"
    "${_krkr_root}/plugins/scriptsEx.cpp"
    "${_krkr_root}/plugins/csvParser.cpp"
    "${_krkr_root}/plugins/json.cpp"
    "${_krkr_root}/plugins/fstat.cpp"
    "${_krkr_root}/plugins/dirlist.cpp"
    "${_krkr_root}/plugins/lzfs.cpp"
    # text: the TextRender/message engine, glyph-free fallbacks and fonts
    "${_krkr_root}/plugins/textrender.cpp"
    "${_krkr_root}/plugins/addFont.cpp"
    "${_krkr_root}/plugins/getabout.cpp"
    "${_krkr_root}/plugins/windowEx.cpp"
    # layer helpers and image-set playback
    "${_krkr_root}/plugins/LayerExBase.cpp"
    "${_krkr_root}/plugins/LayerExImage.cpp"
    "${_krkr_root}/plugins/LayerExRaster.cpp"
    "${_krkr_root}/plugins/LayerExAreaAverage.cpp"
    "${_krkr_root}/plugins/LayerExBTOA.cpp"
    "${_krkr_root}/plugins/LayerExPerspective.cpp"
    "${_krkr_root}/plugins/LayerExMovie.cpp"
    "${_krkr_root}/plugins/AlphaMovie.cpp"
    "${_krkr_root}/plugins/PackinOne.cpp"
    # vector drawing (layerExDraw.dll): GdiPlus namespace + Layer drawing API.
    # Attached to the core Layer class through NCB_ATTACH_CLASS_WITH_HOOK, so it
    # must be linked even when a game never draws a path.
    "${_krkr_root}/plugins/LayerExDraw/LayerExDraw.cpp"
    # E-mote (emoteplayer.dll): Motion/EmotePlayer classes over the PSB container
    "${_krkr_root}/plugins/emoteplayer/emotefile.cpp"
    "${_krkr_root}/plugins/emoteplayer/emoteplayer.cpp"
    "${_krkr_root}/plugins/emoteplayer/emoteplayerclass.cpp"
    "${_krkr_root}/plugins/emoteplayer/emoterunner.cpp"
    # GPU draw device (drawdeviceD3D.dll). Backend-agnostic: it composites
    # through iTVPRenderBackend, which the software backend also implements.
    "${_krkr_root}/plugins/DrawDeviceD3D/DrawDeviceD3D.cpp"
    "${_krkr_root}/plugins/DrawDeviceD3D/DrawDeviceD3D_reg.cpp"
    "${_krkr_root}/plugins/DrawDeviceD3D/D3DEmotePlayer.cpp"
    # SQLite (sqlite3.dll). The amalgamation is in-tree; xp3_vfs maps the game's
    # storage namespace onto SQLite's VFS so a database inside data.xp3 is usable.
    "${_krkr_root}/plugins/sqlite3/sqlite3.c"
    "${_krkr_root}/plugins/sqlite3/sqlite3.cpp"
    "${_krkr_root}/plugins/sqlite3/extend.cpp"
    "${_krkr_root}/plugins/sqlite3/xp3_vfs.cpp"
    # archive readers registered by kirikiroid2 (TAR/ZIP/7z)
    "${_krkr_root}/plugins/Kirikiroid2/TARArchive.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/ZIPArchive.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/7zArchive.cpp"
    "${_krkr_root}/plugins/Kirikiroid2/minizip/unzip.c"
    "${_krkr_root}/plugins/Kirikiroid2/minizip/ioapi.cpp"
    # effect/graphics helpers, audio container decoders, test host
    "${_krkr_root}/plugins/gfxEffect.cpp"
    "${_krkr_root}/plugins/gfxFire.cpp"
    "${_krkr_root}/plugins/fftgraph.cpp"
    "${_krkr_root}/plugins/getSample.cpp"
    "${_krkr_root}/plugins/motionPlayer.cpp"
    "${_krkr_root}/plugins/expat.cpp"
    "${_krkr_root}/plugins/wutcwf.cpp"
    "${_krkr_root}/plugins/autotest.cpp"
    # transitions
    "${_krkr_root}/plugins/extrans/extrans.cpp"
    "${_krkr_root}/plugins/extrans/mosaic.cpp"
    "${_krkr_root}/plugins/extrans/ripple.cpp"
    "${_krkr_root}/plugins/extrans/rotatebase.cpp"
    "${_krkr_root}/plugins/extrans/rotatetrans.cpp"
    "${_krkr_root}/plugins/extrans/turn.cpp"
    "${_krkr_root}/plugins/extrans/turntrans_table.cpp"
    "${_krkr_root}/plugins/extrans/wave.cpp"
    # PSB containers (self-contained, zlib only)
    "${_krkr_root}/plugins/psbfile/PSB.cpp"
    "${_krkr_root}/plugins/psbfile/PSBData.cpp"
    "${_krkr_root}/plugins/psbfile/PSBFile.cpp"
    "${_krkr_root}/plugins/psbfile/PSBMedia.cpp"
    # PSD documents (psd.dll): layered artwork plus the psd:// storage media.
    # Reimplemented here without Boost/libpsd; see the files for the format
    # notes. The parser, the pixel decoder and the plugin are separate TUs so
    # the format logic stays testable and independent of TJS.
    "${_krkr_root}/plugins/psdfile/PSDPlugin.cpp")

# 7-Zip decoder used by Kirikiroid2/7zArchive.cpp. Upstream lists these files
# explicitly (the SDK also ships encoders, which the archive reader never calls);
# keeping the same list avoids compiling ~60 unrelated translation units.
set(_krkr_7zip_sources
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zArcIn.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zBuf.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zBuf2.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zStream.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Aes.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/AesOpt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Alloc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Bcj2.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Bcj2Enc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Blake2s.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Bra.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Bra86.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/BraIA64.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/BwtSort.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/CpuArch.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Delta.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/HuffEnc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzFind.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzFindMt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzFindOpt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzmaDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Lzma2Dec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Lzma2DecMt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Lzma2Enc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzmaEnc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/LzmaLib.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/MtCoder.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/MtDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd7.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd7aDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd7Dec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd7Enc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd8.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd8Dec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Ppmd8Enc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha1.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha1Opt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha256.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha256Opt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sort.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/SwapBytes.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Threads.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Xz.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/XzDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/XzEnc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/XzIn.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zCrc.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/7zCrcOpt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/XzCrc64.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/XzCrc64Opt.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Xxh64.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/ZstdDec.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Md5.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha3.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha512.c"
    "${_krkr_root}/plugins/Kirikiroid2/7zip/Sha512Opt.c")

# PSD reader: the only subsystem in this vendored tree that is ours rather than
# upstream KiriKiri, and the only one expressed as a C++23 module. Everything else
# here has to stay header-based: TJS2/Ncbind are macro-driven (TJS_N alone appears
# 4000 times, plus the NCB registration family), and macros never cross a module
# boundary. PSD is plain std C++ plus zlib, so it is a real interface unit with
# three implementation units and no header at all.
#
# It lives in its own target on purpose: a CXX_MODULES file set makes CMake run
# clang-scan-deps over every source of the owning target, and scanning 500 vendored
# TUs to compile one module would dominate the build. The module is exported PUBLIC
# so the plugin in enginec_krkr_engine (and the PSD test) can import it.
add_library(enginec_krkr_psd STATIC)
target_sources(enginec_krkr_psd PUBLIC
    FILE_SET CXX_MODULES
        BASE_DIRS "${_krkr_root}/plugins/psdfile"
        FILES "${_krkr_root}/plugins/psdfile/psd.cppm")
target_sources(enginec_krkr_psd PRIVATE
    "${_krkr_root}/plugins/psdfile/PSDDescriptor.cpp"
    "${_krkr_root}/plugins/psdfile/PSDDocument.cpp"
    "${_krkr_root}/plugins/psdfile/PSDPixels.cpp")
target_link_libraries(enginec_krkr_psd PUBLIC enginec::std PRIVATE zlib-ng)
set_target_properties(enginec_krkr_psd PROPERTIES POSITION_INDEPENDENT_CODE ON)
enginec_third_party_target(enginec_krkr_psd)
enginec_third_party_no_strict_aliasing(enginec_krkr_psd)

add_library(enginec_krkr_engine STATIC
    ${_krkr_core_sources}
    ${_krkr_tjs_sources}
    ${_krkr_env_sources}
    ${_krkr_plugin_sources}
    ${_krkr_7zip_sources})


# The reference tree is flat-relative: a source in core/main includes headers
# sitting in core/media/audio, core/render, ... Add every directory that owns a
# header instead of hand-maintaining a list that drifts with the port.
file(GLOB_RECURSE _krkr_headers CONFIGURE_DEPENDS
    "${_krkr_root}/core/*.h" "${_krkr_root}/tjs2/*.h"
    "${_krkr_root}/environ/*.h" "${_krkr_root}/plugins/*.h"
    "${_krkr_root}/plugins/*.hpp")
set(_krkr_include_dirs "")
foreach(_header ${_krkr_headers})
    get_filename_component(_dir "${_header}" DIRECTORY)
    list(APPEND _krkr_include_dirs "${_dir}")
endforeach()
list(REMOVE_DUPLICATES _krkr_include_dirs)

target_include_directories(enginec_krkr_engine PUBLIC
    ${_krkr_include_dirs}
    "${_krkr_root}"
    "${_krkr_root}/core"
    "${_krkr_root}/core/main"
    "${_krkr_root}/core/base"
    "${_krkr_root}/core/common"
    "${_krkr_root}/core/archive"
    "${_krkr_root}/core/media"
    "${_krkr_root}/core/msg"
    "${_krkr_root}/core/render"
    "${_krkr_root}/core/render/backend"
    "${_krkr_root}/core/render/gl"
    "${_krkr_root}/core/script"
    "${_krkr_root}/core/utils"
    "${_krkr_root}/tjs2"
    "${_krkr_root}/environ"
    "${_krkr_root}/shim"
    "${_krkr_root}/plugins"
    "${_krkr_root}/plugins/Kirikiroid2")

target_compile_definitions(enginec_krkr_engine PUBLIC
    _KRKRSDL3_USE_SDL3=1
    _KRKRSDL3_AUTOTEST=1
    TJS_TEXT_OUT_CRLF=1
    __STDC_CONSTANT_MACROS=1
    USE_UNICODE_FSTRING=1
    PCRE2_CODE_UNIT_WIDTH=8)
if(ANDROID)
    target_compile_definitions(enginec_krkr_engine PUBLIC _KRKRSDL3_ANDROID=1)
elseif(APPLE AND CMAKE_SYSTEM_NAME STREQUAL "iOS")
    target_compile_definitions(enginec_krkr_engine PUBLIC _KRKRSDL3_IOS=1)
elseif(APPLE)
    target_compile_definitions(enginec_krkr_engine PUBLIC _KRKRSDL3_MACOS=1)
elseif(WIN32)
    target_compile_definitions(enginec_krkr_engine PUBLIC _KRKRSDL3_WINDOWS=1)
else()
    target_compile_definitions(enginec_krkr_engine PUBLIC _KRKRSDL3_LINUX=1)
endif()

target_link_libraries(enginec_krkr_engine
    PUBLIC SDL3::SDL3 freetype zlib-ng pcre2-8
    # PSD 模块（enginec.krkr.psd）用 import std，需要标准库模块的 BMI 才能编译；
    # enginec_krkr_psd 提供 psd 模块本身（PSDPlugin.cpp 只 import 它）。
    PUBLIC enginec::std
    # PRIVATE：psd 模块只被 PSDPlugin.cpp 使用，不上升到 libenginec 的公开面。
    PRIVATE enginec_krkr_psd
    # plutovg: layerExDraw's rasterizer. enginec_glm: glm headers for
    # emoteplayer/DrawDeviceD3D. enginec_bpg: the private libbpg used only by
    # LoadBPG.cpp. All stay PRIVATE: only plugin code sees them.
    PRIVATE enginec_jpeg enginec_png webp plutovg enginec_glm enginec_bpg)
enginec_third_party_target(enginec_krkr_engine)
enginec_third_party_no_strict_aliasing(enginec_krkr_engine)
