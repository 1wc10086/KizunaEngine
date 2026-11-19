// C ABI adapter. It owns no protocol or engine logic: engine_core::invoke is the
// whole surface, so a bug here can only be pointer handling.
//
// This is an implementation unit of enginec.ffi.api, the module that declares the
// C entry points (ffi/include/enginec/ffi/api.cppm). The engine itself is reached
// through enginec, the module built from include/ + utility/ + src/.
module enginec.ffi.api;

import std;
import enginec;

// The target compiles with -fvisibility=hidden, so every exported entry point has
// to repeat the attribute on its definition (a declaration elsewhere does not make
// the symbol reachable through dlopen/dlsym).
#if defined(_WIN32)
#define KZC_API __declspec(dllexport)
#else
#define KZC_API __attribute__((visibility("default")))
#endif

static_assert(sizeof(KzcTextRun) == sizeof(enginec::third_party::krkr::text_run));
static_assert(alignof(KzcTextRun) == alignof(enginec::third_party::krkr::text_run));
static_assert(sizeof(KzcTextBitmap) == sizeof(enginec::third_party::krkr::text_bitmap));
static_assert(alignof(KzcTextBitmap) == alignof(enginec::third_party::krkr::text_bitmap));

namespace {

// One global instance per handle. Dart keeps the address for the process lifetime,
// so the object must come from the C allocator-free heap (new) and stay stable.
enginec::engine_core* as_core(void* handle) {
    return static_cast<enginec::engine_core*>(handle);
}

using text_renderer = KzcTextRenderCallback;

std::atomic<text_renderer> g_text_renderer{nullptr};

// The C ABI and the engine's hook describe the same run through different structs
// (KzcTextRun vs TVPTextRun). Copying the fields keeps the two definitions
// independent: no function-pointer punning, and a layout divergence would be a
// compile error instead of garbage pixels.
bool bridge_text_run(const enginec::third_party::krkr::text_run* run,
                     enginec::third_party::krkr::text_bitmap* out) {
    static_assert(sizeof(char16_t) == sizeof(unsigned short));
    const text_renderer callback = g_text_renderer.load(std::memory_order_acquire);
    if (callback == nullptr || run == nullptr || out == nullptr) {
        return false;
    }
    // Designated initializers so a reordered field in either definition is a
    // compile error instead of a silently swapped value.
    const KzcTextRun request{.text = reinterpret_cast<const unsigned short*>(run->text),
                             .length = run->length,
                             .family = run->family,
                             .size = run->size,
                             .color = run->color,
                             .flags = run->flags};
    KzcTextBitmap bitmap{nullptr, 0, 0, 0, 0, 0};
    if (!callback(&request, &bitmap)) {
        return false;
    }
    out->pixels = bitmap.pixels;
    out->width = bitmap.width;
    out->height = bitmap.height;
    out->pitch = bitmap.pitch;
    out->advance = bitmap.advance;
    out->baseline = bitmap.baseline;
    return true;
}

} // namespace


extern "C" {

KZC_API void* KzcEngineCreate(void) {
    try {
        return new enginec::engine_core();
    } catch (...) {
        return nullptr;
    }
}

KZC_API void KzcEngineDestroy(void* handle) {
    if (handle == nullptr) {
        return;
    }
    delete as_core(handle);
}

KZC_API unsigned char* KzcEngineInvoke(void* handle, const unsigned char* request,
                                       int request_len, int* out_len) {
    if (out_len != nullptr) {
        *out_len = 0;
    }
    std::vector<std::uint8_t> response;
    if (handle != nullptr && request != nullptr && request_len > 0) {
        try {
            response = as_core(handle)->invoke(
                std::span<const std::uint8_t>(request, static_cast<std::size_t>(request_len)));
        } catch (const std::exception& failure) {
            response = enginec::wire::build_error(0, failure.what());
        } catch (...) {
            response = enginec::wire::build_error(0, "engine panic");
        }
    } else {
        response = enginec::wire::build_error(0, "nil handle");
    }

    // Dart reads the response after this call returns, so it must live on the C
    // heap. malloc(0) is avoided: an empty payload still needs a valid pointer.
    const auto bytes = response.empty() ? std::size_t{1} : response.size();
    auto* buffer = static_cast<unsigned char*>(std::malloc(bytes));
    if (buffer == nullptr) {
        return nullptr;
    }
    if (!response.empty()) {
        std::memcpy(buffer, response.data(), response.size());
    }
    if (out_len != nullptr) {
        *out_len = static_cast<int>(response.size());
    }
    return buffer;
}

KZC_API void KzcEngineFree(void* buffer) {
    std::free(buffer);
}

KZC_API void KzcEngineSetTextRenderer(KzcTextRenderCallback cb) {
    // Order matters for teardown: Dart closes its NativeCallable right after this
    // call, so the engine-side hook must be gone before the pointer is dropped.
    if (cb == nullptr) {
        enginec::third_party::krkr::set_text_renderer(nullptr);
        g_text_renderer.store(nullptr, std::memory_order_release);
        return;
    }
    g_text_renderer.store(cb, std::memory_order_release);
    enginec::third_party::krkr::set_text_renderer(&bridge_text_run);
}

KZC_API const char* KzcEngineVersion(void) {
    // Static storage: unlike the Go implementation this is never freed by Dart, and
    // an atomically-initialized literal is thread-safe on every supported platform.
    static const char text[] = "kizuna-enginec/1.0.0";
    return text;
}

} // extern "C"
