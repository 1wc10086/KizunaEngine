// C ABI loaded by Dart (libenginec.so / enginec.dll / process on Apple).
//
// This is an interface module rather than a header: the surface is ours, it is
// small, and every consumer (the adapter, the crash reporter, the tests) is our
// own code, so there is nothing to be gained from keeping it textual.
//
// Names and semantics are identical to the Go engine's KzEngine* exports; this
// library uses the KzcEngine* prefix so both can coexist in one process on Apple.
// KzcEngineInvoke copies the response into malloc'd memory owned by the caller;
// KzcEngineFree releases it.
//
// Two rules this module must keep:
//  * The structs are a byte-level contract with Dart (app/lib/core/text/host_text.dart)
//    and with the engine side (environ/PlatformTextHook.h). The layout assertions in
//    ffi/enginec_ffi.cpp pin both ends, so a drift is a build error rather than
//    garbled glyphs.
//  * Definitions live in ffi/enginec_ffi.cpp, which is an implementation unit of
//    this module. The visibility attribute is repeated on the definitions because
//    the target builds with -fvisibility=hidden: an exported C entry point has to
//    say so where it is defined.
export module enginec.ffi.api;

// Deliberately no `import std;`: this module must stay reachable from plain
// translation units that include system headers (ffi/crashlog.cpp includes
// <filesystem>), and a module that exports the standard library cannot be mixed
// with the same standard headers in one TU. The C ABI is spelled in C types for
// exactly that reason — it is a C surface anyway, and the C++ side pins the layout
// with static_assert in ffi/enginec_ffi.cpp.

// Windows has no visibility attribute; dllexport is the equivalent and is the only
// place the two platforms differ.
#if defined(_WIN32)
#define KZC_API __declspec(dllexport)
#else
#define KZC_API __attribute__((visibility("default")))
#endif

// Layout-identical to TVPTextRun/TVPTextBitmap; `int` there is a 32-bit integer by
// contract. `text` is UTF-16 and not NUL-terminated; `flags` carries the
// TVP_TF_BOLD/ITALIC bits (see core/media/font/TVPFontstruc.h) and the host
// synthesizes them the way FreeType does.
export struct KzcTextRun
{
    const unsigned short* text;
    int length;
    const char* family; // UTF-8 font family name, may be ""
    float size;
    unsigned int color; // RGBA
    unsigned int flags;
};

export struct KzcTextBitmap
{
    unsigned char* pixels;
    int width;
    int height;
    int pitch;
    int advance;  // horizontal advance in pixels for the whole run
    int baseline; // top edge -> alphabetic baseline; 0 = not reported
};

// One rasterization callback. Named separately so the declaration below, the
// typedef and the Dart NativeCallable all speak the same function type.
using KzcTextRenderCallback = bool (*)(const KzcTextRun*, KzcTextBitmap*);

export {

extern "C" {

KZC_API void* KzcEngineCreate(void);
KZC_API void KzcEngineDestroy(void* handle);
KZC_API unsigned char* KzcEngineInvoke(void* handle, const unsigned char* request,
                                       int request_len, int* out_len);
KZC_API void KzcEngineFree(void* buffer);
KZC_API const char* KzcEngineVersion(void);

// 原生崩溃日志：把 SIGSEGV 等致命信号的报告（信号/故障地址/寄存器/回溯）
// 写进 dir（应用私有目录）。进入游戏前尽早调用；调用前发生的崩溃只有
// stderr/logcat 输出。文件名 crash-<stamp>-native.log，与 Dart 侧
// crash-*.log 同目录、同轮换策略。
KZC_API void KzcEngineSetCrashLogDirectory(const char* dir);

// 面包屑：把一行进度追加到 <dir>/engine-crumb.log（引擎初始化各阶段调用），
// 崩溃发生在哪一步一看便知。未设置目录时只进 stderr/logcat。
KZC_API void KzcEngineCrumb(const char* message);

// Host glyph rasterizer (see enginec/third_party/krkr_engine/environ/PlatformTextHook.h).
//
// The engine keeps text layout and composition; this callback only turns one run
// into pixels, so fonts and hinting come from the platform (Flutter's text stack)
// instead of shipped font files.
//
// Calling convention: the callback runs synchronously on the thread that invoked
// KzcEngineInvoke(), so a Dart NativeCallable must be created in that isolate.
// `pixels` must be malloc'd by the callee (RGBA8, pitch == width * 4, alpha
// premultiplied); the engine releases it with free(). Return false to fall back
// to the engine's FreeType rasterizer for this run.
//
// Process-global: the engine has one font seam, and the Flutter bridge holds one
// engine per process. Safe to call before KzcEngineCreate.
KZC_API void KzcEngineSetTextRenderer(KzcTextRenderCallback cb);

} // extern "C"

} // export
