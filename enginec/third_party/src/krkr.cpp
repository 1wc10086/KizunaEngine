// C++23 module adapter over the vendored KiriKiri engine.
//
// The engine itself is the real krkrsdl3 runtime (core + TJS2 + KAG + native
// classes) compiled with SDL3's offscreen video driver and dummy audio device.
// This file owns the process-global engine instance and translates the module's
// std-only types to the engine's SDL entry points and autotest host:
//
//   SDL_AppInit      -> open (loads Config.tjs/start.tjs and runs the game)
//   SDL_AppIterate   -> run/advance/input (one real engine frame, captured here)
//   TVPAutoTest*     -> pointer/key/wheel injection and frame readback
//   TVPMovieEventHook -> host-delegated movie requests (Flutter plays them)
//   TVPTextRenderHook -> host glyph rasterization (installed from ffi/)
//
// Only one game can be open per process: SDL and TVPApplication are global in
// the original engine. Flutter opens one player at a time, and the library page
// uses enginego, so this is the same constraint the desktop build has.

module;

// SDL3 is an external C library: its headers are the one legitimate #include on
// this side of the module boundary (the standard library comes from `import std;`
// below, and the engine's own headers are macro-based upstream code that cannot be
// modularized).
#include <SDL3/SDL_init.h>
#include <SDL3/SDL_hints.h>

#include "tjsCommHead.h"
#include "tjsError.h"
#include "Platform.h"
#include "PlatformAutoTest.h"
#include "PlatformHostPaths.h"
#include "PlatformView.h"
#include "TVPCompositor.h"
#include "TVPStorage.h"
#include "TVPWindow.h"
#include "WindowManager.h"
#include "UtilStreams.h"
#include "PlatformAudioHook.h"
#include "PlatformMediaCache.h"
#include "PlatformMovieHook.h"
#include "PlatformTextHook.h"

extern "C" {
SDL_AppResult SDL_AppInit(void** appstate, int argc, char* argv[]);
SDL_AppResult SDL_AppIterate(void* appstate);
SDL_AppResult SDL_AppEvent(void* appstate, SDL_Event* event);
void SDL_AppQuit(void* appstate, SDL_AppResult result);
}

// 崩溃面包屑（enginec/ffi/crashlog.cpp）：open 的每一步都留一行，
// 进程被信号打死时 engine-crumb.log 的最后一行就是死亡位置。
extern "C" void KzcEngineCrumb(const char* message);

// TVPMovieNotifyFinished is declared by PlatformMovieHook.h and implemented by the
// movie overlay (plugins/kizuna_stubs.cpp): this adapter only reports requests and
// completes them.

module enginec.third_party.krkr;

import std;

namespace enginec::third_party::krkr {

// The host ABI is a byte-level contract: the same run crosses three definitions
// (enginec.ffi.api -> this module -> environ/PlatformTextHook.h). The ffi
// side checks its structs against these module types, so a size/alignment drift on
// either end fails the build instead of turning into garbled glyph bitmaps.
static_assert(sizeof(text_run) == sizeof(TVPTextRun));
static_assert(alignof(text_run) == alignof(TVPTextRun));
static_assert(sizeof(text_bitmap) == sizeof(TVPTextBitmap));
static_assert(alignof(text_bitmap) == alignof(TVPTextBitmap));

namespace {

struct runtime_state {
    bool open = false;
    bool finished = false;
    bool frame_valid = false;
    // Content version of the last frame handed to the host, plus the version that
    // was current when open() started. The autotest host bumps its counter only when
    // the cached pixels actually change, so comparing these two tells compose()
    // "nothing new" without touching a single pixel: the previous implementation
    // hashed the whole 1080p frame per host tick (30x/s) and then copied it twice.
    std::uint64_t last_version = 0;
    int width = 0;
    int height = 0;
    // Size the host surface was allocated with (reported by open()). compose()
    // writes with this stride, so a draw buffer whose size changes later still
    // lands in the host buffer without shearing the rows.
    int surface_width = 0;
    int surface_height = 0;
    // True once open() has captured a frame of this game; lets compose() skip the
    // stale-frame probe it would otherwise need.
    bool got_first_frame = false;
    std::string title;
    std::string error;
    std::string cache_dir;
    std::vector<audio_event> audio;
    // Last snapshot handed to the host, per engine channel. on_audio_event turns
    // absolute engine state into the relative events the wire protocol speaks
    // (start / volume / pause / resume / stop); without the previous snapshot it
    // could only report "something is playing", which is why fades and pauses used
    // to be lost.
    struct host_channel_state {
        std::string storage; // engine-side storage name this voice was started from
        std::string path;    // host-openable path handed to Flutter
        bool loop = false;
        double volume = 1.0;
        bool playing = false;
        bool paused = false;
    };
    std::unordered_map<int, host_channel_state> host_channels;
    std::mutex audio_mutex;
    // Movie handed to the host player and not finished yet. The pending flag is
    // atomic because the stepping ops and current_step() only need it and must not
    // take a lock (current_step is noexcept); the strings and the serial are only
    // touched under movie_mutex.
    std::atomic<bool> movie_pending{false};
    std::string movie_path;
    std::string movie_storage;
    // The overlay owns the serial (PlatformMovieHook.h): it is quoted on the wire so
    // the host can tell "a new request" from "the same one polled again", and handed
    // back unchanged to TVPMovieNotifyFinished. No second counter here, because two
    // numbers for one request is exactly how a completion ends up addressing a
    // request that is no longer the pending one.
    int movie_serial = 0;
    // True only while TVPMovieNotifyFinished() runs. The overlay may re-report the
    // same request while it is being completed; matched by serial against
    // movie_storage, that echo is ignored instead of turning into a request to
    // replay the just-finished movie. A different movie requested during completion
    // is still accepted.
    bool movie_completing = false;
    // Gain for the movie's audio track, copied from the overlay's request. The
    // host applies it to its player; a fixed 0 would mute every OP.
    double movie_audio_gain = 1.0;
    std::mutex movie_mutex;
    // Content version of the autotest host's cached frame recorded in open(). That
    // cache is process-global and survives close(), so while it is unchanged compose()
    // must not hand the previous game's picture to the new host buffer. Cleared as
    // soon as a frame of this game shows up.
    std::uint64_t stale_probe = 0;
    // True while SDL_AppInit has succeeded and SDL_AppQuit has not run yet. This is
    // deliberately *not* cleared by clear(): it describes the process-global
    // TVPApplication, not the protocol state, and it is what makes open()/close()
    // tear down a half-initialised or abandoned session before starting another.
    bool sdl_running = false;
    // The engine instance that currently owns the process-global session. The host
    // may destroy an old handle after a newer one has opened (two isolates can
    // overlap during a player switch); without this token that late destructor
    // would tear down the *new* game.
    const void* session_owner = nullptr;
    // Monotonic timestamp (ms) of the last frame the engine actually presented.
    // state().animating is derived from it: the host keeps composing at display
    // rate only while the picture is still changing, and drops back to its idle
    // cadence once the engine stops presenting. Atomic because state() only takes
    // a shared lock while stepping ops write it on the engine thread.
    std::atomic<std::int64_t> last_present_ms{0};

    void clear()
    {
        open = false;
        finished = false;
        frame_valid = false;
        last_version = 0;
        width = 0;
        height = 0;
        surface_width = 0;
        surface_height = 0;
        got_first_frame = false;
        title.clear();
        error.clear();
        cache_dir.clear();
        audio.clear();
        host_channels.clear();
        movie_pending.store(false, std::memory_order_relaxed);
        movie_path.clear();
        movie_storage.clear();
        movie_serial = 0;
        movie_completing = false;
        movie_audio_gain = 1.0;
        stale_probe = 0;
        last_present_ms.store(0, std::memory_order_relaxed);
        // Ownership dies with the session: from here on no handle may claim it, and
        // the next open() publishes its own token.
        session_owner = nullptr;
    }
};

runtime_state& global_state()
{
    static runtime_state instance;
    return instance;
}

// Monotonic milliseconds for frame-pacing decisions. steady_clock is not affected
// by wall-clock changes (the user moving the phone's clock), which matters because
// the value is compared against a timestamp written a few milliseconds ago.
std::int64_t monotonic_ms()
{
    using namespace std::chrono;
    return duration_cast<milliseconds>(steady_clock::now().time_since_epoch()).count();
}

// Tears the process-global TVP application down. TVPApplication, the TJS
// interpreter and the SDL video/audio subsystems are singletons in the original
// engine, so exactly one game can be live per process; this is the one place that
// ends that session. Called from close(), from the destructor (the host destroys
// its handle without sending a close op) and from open() when a previous session
// is still up, so it must be idempotent.
void teardown_session()
{
    auto& s = global_state();
    if (!s.sdl_running) {
        return;
    }
    // Clear the hooks before the engine tears down: a late overlay/audio
    // notification must not write into state that is about to be reset.
    TVPAudioEventHook = nullptr;
    TVPMovieEventHook = nullptr;
    KzcEngineCrumb("close: SDL_AppQuit");
    SDL_AppQuit(nullptr, SDL_APP_SUCCESS);
    s.sdl_running = false;
    KzcEngineCrumb("close: done");
}

// Paths cross the host ABI as UTF-8 (Dart strings are UTF-8, and so are the
// engine's ttstr). On Windows std::filesystem::path(string) / path::string()
// speak the ANSI code page, so a Japanese/Chinese install directory would be
// mangled; go through the UTF-8 conversions there.
std::filesystem::path path_from_utf8(const std::string& text)
{
#if defined(_WIN32)
    return std::filesystem::path(
        std::u8string(reinterpret_cast<const char8_t*>(text.data()), text.size()));
#else
    return std::filesystem::path(text);
#endif
}

std::string path_to_utf8(const std::filesystem::path& path)
{
#if defined(_WIN32)
    const std::u8string text = path.u8string();
    return std::string(reinterpret_cast<const char*>(text.data()), text.size());
#else
    return path.string();
#endif
}

// True while the host owns the frame: every stepping op reports await_movie and
// leaves the engine untouched, so the script cannot run past the wait and no input
// is queued for a scene the player cannot see.
bool movie_waiting()
{
    return global_state().movie_pending.load(std::memory_order_acquire);
}

// Publishes the host cache directory to the shared materialiser.
//
// The engine-side helper (environ/PlatformMediaCache.h) owns the hard part:
// resolving an archive entry to a real file, copying it out atomically and keeping
// a decoder-friendly extension. This adapter only hands it the directory the
// Flutter side already owns, so extracted samples live under app control and are
// removed with the app's cache rather than polluting the game folder.
void configure_media_cache()
{
    const auto& s = global_state();
    TVPSetHostMediaCacheDir(s.cache_dir);
}

// Scripts are not consistent about case or about the storage prefix
// ("BGM/01.OGG" vs "file://./bgm/01.ogg"), and the engine compares storage names
// case-insensitively on Windows but not on POSIX. Comparing the canonical form
// keeps a KAG re-`play` of the same BGM from restarting it.
bool same_storage(std::string_view left, std::string_view right)
{
    auto normalize = [](std::string_view text) {
        std::string out(text);
        if (const auto pos = out.find("://"); pos != std::string::npos) {
            out.erase(0, pos + 3);
        }
        // ASCII-only lowering on purpose: Android's bionic declares tolower as an
        // inline in <ctype.h> *and* in namespace std, which makes std::tolower
        // ambiguous under the NDK's libc++ configuration (and a storage name is
        // ASCII before the file name anyway; the name itself is compared exactly).
        for (char& ch : out) {
            if (ch >= 'A' && ch <= 'Z') {
                ch = static_cast<char>(ch - 'A' + 'a');
            }
        }
        return out;
    };
    return normalize(left) == normalize(right);
}

std::string materialize_storage(std::string_view storage, std::string_view kind)
{
    if (storage.empty()) {
        return {};
    }
    return TVPMaterializeHostMedia(storage, kind);
}

void on_audio_event(const TVPAudioEvent& event)
{
    try {
    auto& s = global_state();
    std::lock_guard<std::mutex> lock(s.audio_mutex);
    const std::uint32_t channel = static_cast<std::uint32_t>(event.channel);
    audio_event out;
    out.channel = channel;
    out.volume = event.volume;
    out.loop = event.loop;

    // Per-channel bookkeeping: a snapshot only means something relative to the
    // last one the host received. Without it the host cannot tell "the same BGM at
    // a new volume" (update in place) from "a different file on the same buffer"
    // (restart), and a pause would look like a stop.
    auto found = s.host_channels.find(channel);

    if (event.stop) {
        // Natural end, explicit stop, or unload. Dropping the bookkeeping here is
        // what makes the next Play() on this channel start a fresh voice.
        if (found == s.host_channels.end() || !found->second.playing) {
            return; // never reported as playing: nothing to stop on the host
        }
        out.kind = found->second.loop ? 3u : 5u; // stopBgm / stopSe
        s.host_channels.erase(found);
        s.audio.push_back(std::move(out));
        return;
    }

    if (event.storage == nullptr || *event.storage == '\0') {
        return;
    }

    // A fresh voice, or the same channel switched to another file: the host needs
    // the path and must start from the beginning.
    if (found == s.host_channels.end() || !found->second.playing ||
        !same_storage(found->second.storage, event.storage) ||
        found->second.loop != event.loop) {
        out.path = materialize_storage(event.storage, "audio");
        if (out.path.empty()) {
            // The host cannot open it; reporting nothing is better than reporting a
            // path that fails to load (the script keeps running either way). Leave a
            // breadcrumb: "the game has no sound" is otherwise indistinguishable
            // from "the engine never reported a sound".
            char crumb[512];
            std::snprintf(crumb, sizeof(crumb), "audio: cannot materialize %.400s", event.storage);
            KzcEngineCrumb(crumb);
            return;
        }
        out.kind = event.loop ? 0u : 1u; // bgm / se
        // KAG sets `paused` before `play()`, so the start itself must be paused.
        // Sending play and then pause would let the decoder make a blip for as
        // long as the file takes to open.
        out.paused = event.paused;
        runtime_state::host_channel_state& state = s.host_channels[channel];
        state.storage = event.storage;
        // Copy, not move: the event below is what carries the path to the host.
        state.path = out.path;
        state.loop = event.loop;
        state.volume = event.volume;
        state.playing = true;
        state.paused = event.paused;
        s.audio.push_back(std::move(out));
        return;
    }

    runtime_state::host_channel_state& state = found->second;
    if (event.paused != state.paused) {
        state.paused = event.paused;
        out.kind = event.paused ? 10u : 11u; // pause / resume
        s.audio.push_back(std::move(out));
        // A resume also carries whatever the gain became while paused: KAG keeps
        // changing the track volume during a pause (fades, the config slider), and
        // the host must not come back at the pre-pause level.
        if (!event.paused && event.volume != state.volume) {
            state.volume = event.volume;
            audio_event volume;
            volume.channel = channel;
            volume.kind = 7u; // volume
            volume.volume = event.volume;
            volume.loop = event.loop;
            s.audio.push_back(std::move(volume));
        }
        return;
    }
    if (event.paused) {
        return; // volume while paused is applied on resume
    }
    if (event.volume != state.volume) {
        state.volume = event.volume;
        out.kind = 7u; // volume (channel-addressed absolute gain)
        out.path = state.path; // informational only
        s.audio.push_back(std::move(out));
    }
    } catch (...) {
        // Audio reporting must never break the engine's playback thread.
    }
}

// Fired by the engine's video overlay when a script asks for a movie. The overlay
// already materializes a host-accessible copy into `local`; that file is
// preferred over our own materialization, which only serves overlays that do not
// (and XP3 archives that need unpacking). The event can fire while the script is
// blocked, so it must never throw.
void on_movie_event(const TVPMovieRequest& request)
{
    try {
        const std::string storage = request.storage != nullptr ? request.storage : "";
        const std::string local = request.local != nullptr ? request.local : "";
        // Dedupe key: the engine-side name when there is one, otherwise the
        // already-materialized path (an overlay may pass only `local`).
        const std::string key = storage.empty() ? local : storage;
        auto& s = global_state();
        std::lock_guard<std::mutex> lock(s.movie_mutex);
        // A repeated report of the request we already handed over is the overlay
        // polling, not a new request: re-issuing it would make the host restart the
        // movie it is already playing. Identity is the overlay's serial, not the
        // storage name: replaying/looping the same file is a fresh request with a
        // new serial while the old completion is still outstanding, and swallowing
        // it would leave that script waiting on a serial nobody will notify.
        const bool reported =
            s.movie_pending.load(std::memory_order_relaxed) || s.movie_completing;
        if (reported && key == s.movie_storage && request.serial == s.movie_serial) {
            return;
        }
        std::string path = local;
        if (path.empty()) {
            path = materialize_storage(storage, "movie");
        }
        if (path.empty()) {
            // Nothing playable. Report the storage name anyway so the host fails
            // to open it and completes the request; an empty path would look like
            // "no movie" and leave the script waiting forever.
            char crumb[512];
            std::snprintf(crumb, sizeof(crumb), "movie: cannot materialize %.400s", storage.c_str());
            KzcEngineCrumb(crumb);
            path = storage;
        }
        if (path.empty()) {
            return;
        }
        s.movie_path = std::move(path);
        s.movie_storage = key;
        // Same number on the wire and in the completion: the host's dedupe key and
        // TVPMovieNotifyFinished's argument must name one request.
        s.movie_serial = request.serial;
        s.movie_audio_gain = request.audio_gain;
        // Publish only after the request is fully written: a reader that sees the
        // flag can rely on the strings above.
        s.movie_pending.store(true, std::memory_order_release);
    } catch (...) {
        // Movie reporting runs on the engine's script thread; a failure here must
        // not take the game down.
    }
}

// SDL string hints must outlive SDL_Init; static storage keeps them valid for the
// whole process (the engine can be opened, closed and reopened by the player).
void configure_drivers()
{
    // App identity must be set before SDL_Init: the init logging reads the app
    // name through SDL_GetAppMetadataProperty, which falls back to
    // SDL_GetExeName() when unset — and on Android that calls
    // SDL_GetAndroidPackageName(), which dereferences the JavaVM SDLActivity
    // caches in JNI_OnLoad. This build embeds the engine in Flutter (no
    // SDLActivity, library loaded via dlopen), so the fallback is a NULL-deref
    // SIGSEGV right inside SDL_InitSubSystem. Hints are checked before the
    // properties, so both are set. See crashes/crash-*-native.log evidence:
    // fault 0x0 in SDL_GetAndroidPackageName <- SDL_GetExeName <- metadata log.
    SDL_SetHint(SDL_HINT_APP_NAME, "KizunaEngine");
    SDL_SetHint(SDL_HINT_APP_ID, "com.kizuna.engine");
    SDL_SetAppMetadata("KizunaEngine", "1.0.0", "com.kizuna.engine");

    SDL_SetHint(SDL_HINT_VIDEO_DRIVER, "offscreen");
    // Force SDL's CPU renderer: the offscreen window has no GPU swapchain and the
    // Vulkan renderer is not usable in every headless environment.
    SDL_SetHint(SDL_HINT_RENDER_DRIVER, "software");
    SDL_SetHint(SDL_HINT_AUDIO_DRIVER, "dummy");
    // The offscreen driver has no window manager; do not let SDL abort on the
    // lack of a display or audio device.
    SDL_SetHint(SDL_HINT_VIDEO_MINIMIZE_ON_FOCUS_LOSS, "0");
    SDL_SetHint(SDL_HINT_AUDIO_DEVICE_STREAM_NAME, "KizunaEngine");
}

step iterate_once()
{
    auto& s = global_state();
    if (!s.open) {
        s.error = "krkr: game is not open";
        return step::faulted;
    }
    // While a movie is pending the script is parked inside the engine waiting for
    // TVPMovieNotifyFinished. Iterating would burn a frame and could let the
    // engine run past the wait, so the frame belongs to the host instead.
    if (movie_waiting()) {
        return step::await_movie;
    }
    // Arm the readback *before* the frame runs: the engine presents through
    // TVPWindow::PresentTexture while Application::Run() delivers the update, which
    // happens inside SDL_AppIterate, and the presented texture can only be read
    // back at that moment. compose() therefore never drives a frame of its own, it
    // just converts the most recent capture. Arming per engine frame (not per
    // compose) is deliberate: the host can run several steps between two renders,
    // and the frame it eventually draws must be the newest one. The readback only
    // happens when the engine actually presents, i.e. when the picture changed, so
    // a static scene costs nothing.
    krkrsdl3::TVPAutoTestArmCapture();
    // Content version before the frame: a bump means the engine actually presented
    // new pixels, which is the honest "this game is animating" signal the host uses
    // to keep composing at display rate (see engine::state()).
    const std::uint64_t presented_before = krkrsdl3::TVPAutoTestFrameVersion();
    SDL_AppResult result = SDL_APP_FAILURE;
    try {
        result = SDL_AppIterate(nullptr);
    } catch (const TJS::eTJS& failure) {
        s.error = std::string("krkr: ") + failure.GetMessage().c_str();
        return step::faulted;
    } catch (const std::exception& failure) {
        s.error = std::string("krkr: ") + failure.what();
        return step::faulted;
    }
    if (krkrsdl3::TVPAutoTestFrameVersion() != presented_before) {
        s.last_present_ms.store(monotonic_ms(), std::memory_order_relaxed);
    }
    if (result == SDL_APP_SUCCESS) {
        s.finished = true;
        return step::finished;
    }
    if (result == SDL_APP_FAILURE) {
        const char* reason = krkrsdl3::TVPAutoTestLastError();
        s.error = (reason != nullptr && *reason != '\0') ? reason : "krkr: engine iterate failed";
        return step::faulted;
    }
    // A movie can be requested by the very script run that just returned; report
    // it now so the host starts playback without waiting for another op.
    if (movie_waiting()) {
        return step::await_movie;
    }
    return s.finished ? step::finished : step::cont;
}

// Host pointer coordinates are engine-canvas (frame) coordinates: the Flutter
// stage maps a tap through the image size, which open() reports as the canvas.
// The autotest mouse entry points, however, take WINDOW pixels — KRKR_Trig_Mouse*
// subtracts the window sprite's letterbox offset and divides by its scale
// (WindowManager.cpp). With the offscreen driver the SDL window keeps its own size
// (1280x720) even when the game canvas differs, so injecting canvas coordinates
// directly made every click land at the wrong place — "buttons do nothing".
// Convert canvas -> window here; when the two sizes match this is the identity.
void canvas_to_window(int& x, int& y)
{
    if (TVPSprite* sprite = krkrsdl3::KRKR_Get_Current_Sprite()) {
        if (sprite->scale > 0.0f) {
            x = static_cast<int>(static_cast<float>(x) * sprite->scale + sprite->xPos);
            y = static_cast<int>(static_cast<float>(y) * sprite->scale + sprite->yPos);
        }
    }
}

// Inverse of the letterbox fit compose() performs: surface pixel -> canvas pixel.
// When the host surface already equals the canvas (the normal case) this is the
// identity; after a game-side resolution change it maps into the scaled picture.
void surface_to_canvas(int& x, int& y)
{
    const auto& s = global_state();
    if (s.surface_width <= 0 || s.surface_height <= 0 || s.width <= 0 || s.height <= 0) {
        return;
    }
    if (s.surface_width == s.width && s.surface_height == s.height) {
        return;
    }
    int fit_width = s.surface_width;
    int fit_height = s.surface_height;
    if (static_cast<long long>(s.width) * s.surface_height >
        static_cast<long long>(s.height) * s.surface_width) {
        fit_height = std::max(1, s.height * s.surface_width / s.width);
    } else {
        fit_width = std::max(1, s.width * s.surface_height / s.height);
    }
    const int offset_x = (s.surface_width - fit_width) / 2;
    const int offset_y = (s.surface_height - fit_height) / 2;
    // A tap on a letterbox bar has no canvas pixel; clamp it to the nearest edge so
    // it still counts as a click at the picture border instead of going negative.
    int cx = x - offset_x;
    int cy = y - offset_y;
    cx = std::clamp(cx, 0, fit_width - 1);
    cy = std::clamp(cy, 0, fit_height - 1);
    x = cx * s.width / fit_width;
    y = cy * s.height / fit_height;
    x = std::clamp(x, 0, s.width - 1);
    y = std::clamp(y, 0, s.height - 1);
}

void inject_mouse(std::uint32_t op, int32_t x, int32_t y)
{
    // Host pointer coordinates are in *surface* space (the buffer open() reported
    // and the host keeps allocating). Map them onto the current canvas first: the
    // game can switch from its 480x280 boot window to a 1920x1080 main window while
    // the host is still showing the small surface for another frame, and without
    // this inverse mapping every tap would land in the top-left corner of the game.
    surface_to_canvas(x, y);
    int wx = x;
    int wy = y;
    canvas_to_window(wx, wy);
    switch (op) {
    case 0: // move
        krkrsdl3::TVPAutoTestWarpMouse(wx, wy);
        break;
    case 1: // press
        krkrsdl3::TVPAutoTestWarpMouse(wx, wy);
        krkrsdl3::TVPAutoTestMouseButton(true, 0, wx, wy);
        break;
    case 2: // release
        krkrsdl3::TVPAutoTestMouseButton(false, 0, wx, wy);
        break;
    case 3: // tap
        krkrsdl3::TVPAutoTestWarpMouse(wx, wy);
        krkrsdl3::TVPAutoTestMouseButton(true, 0, wx, wy);
        krkrsdl3::TVPAutoTestMouseButton(false, 0, wx, wy);
        break;
    case 4: // right click
        krkrsdl3::TVPAutoTestWarpMouse(wx, wy);
        krkrsdl3::TVPAutoTestMouseButton(true, 1, wx, wy);
        krkrsdl3::TVPAutoTestMouseButton(false, 1, wx, wy);
        break;
    case 6: // drag: the engine reads hover position, a move is enough
        krkrsdl3::TVPAutoTestWarpMouse(wx, wy);
        break;
    default:
        break;
    }
}

const char* scancode_name(std::uint32_t code)
{
    switch (code) {
    case 27: return "ESCAPE";
    case 13: return "RETURN";
    case 32: return "SPACE";
    case 8: return "BACKSPACE";
    case 9: return "TAB";
    case 37: return "LEFT";
    case 38: return "UP";
    case 39: return "RIGHT";
    case 40: return "DOWN";
    case 17: return "LCTRL";
    case 16: return "LSHIFT";
    default: return nullptr;
    }
}

// Process-global host glyph rasterizer (see PlatformTextHook.h). The hook takes a
// reference and the C ABI takes a pointer, so the trampoline copies the fields
// instead of casting the function pointer: no incompatible-function call, and the
// two struct definitions stay independent of each other's layout. Atomic because
// the FFI installs/clears it from the engine-call thread while the font cache may
// ask from a render thread.
std::atomic<text_render_callback> host_text_renderer{nullptr};

bool host_text_trampoline(const TVPTextRun& run, TVPTextBitmap* out)
{
    const text_render_callback callback = host_text_renderer.load(std::memory_order_acquire);
    if (callback == nullptr || out == nullptr) {
        return false;
    }
    // Named fields on both sides: the two structs are copied, never pun'd, so a
    // reordered or retyped member must fail the build rather than shift a value.
    const text_run input{.text = run.text,
                         .length = run.length,
                         .family = run.family,
                         .size = run.size,
                         .color = run.color,
                         .flags = run.flags};
    text_bitmap result;
    if (!callback(&input, &result)) {
        return false;
    }
    *out = TVPTextBitmap{.pixels = result.pixels,
                         .width = static_cast<int>(result.width),
                         .height = static_cast<int>(result.height),
                         .pitch = static_cast<int>(result.pitch),
                         .advance = static_cast<int>(result.advance),
                         .baseline = static_cast<int>(result.baseline)};
    return true;
}

// ---------------------------------------------------------------------------
// Project layout resolution
// ---------------------------------------------------------------------------
// KiriKiri releases come in three shapes:
//   * <root>/data.xp3                              (archive at the install root)
//   * <root>/startup.tjs, <root>/scenario/first.ks (loose project root)
//   * <root>/data/system/Initialize.tjs ...        (the official data/ tree,
//                                                   e.g. the AetherKiri KAG3 demo)
// enginego's scanner reports the resolved DataDirectory, but a deep link or a
// caller that only knows the install root passes the outer folder. Resolving it
// here keeps the engine independent of how the host found the game.
bool looks_like_project_root(const std::filesystem::path& dir)
{
    std::error_code ec;
    if (!std::filesystem::is_directory(dir, ec)) {
        return false;
    }
    const auto has = [&dir](const char* name)
    {
        std::error_code ignored;
        return std::filesystem::exists(dir / name, ignored);
    };
    if (has("data.xp3") || has("startup.tjs") || has("start.tjs") || has("first.ks") ||
        has("scenario")) {
        return true;
    }
    // KAG3's System/Initialize.tjs is the entry point when startup.tjs is absent.
    if (has("system/Initialize.tjs") || has("system/MainWindow.tjs") ||
        has("System/Initialize.tjs") || has("System/MainWindow.tjs")) {
        return true;
    }
    return false;
}

std::string resolve_project_dir(const std::string& given)
{
    namespace fs = std::filesystem;
    std::error_code ec;
    const fs::path root = path_from_utf8(given);
    if (fs::is_regular_file(root, ec) || looks_like_project_root(root)) {
        return given;
    }
    // The official SDK tree, plus the "Files/" layout some releases use.
    for (const char* name : {"data", "Data", "Files", "files"}) {
        const fs::path nested = root / name;
        if (looks_like_project_root(nested)) {
            return path_to_utf8(nested);
        }
    }
    return given;
}

// KiriKiri does not mount data.xp3 for a folder argument. TVPParseArguments
// distinguishes folder and archive, and TVPBeforeSystemInit only appends the
// archive delimiter ('>') when the project argument itself exists as a storage
// (i.e. is a file). Passing the folder therefore makes the engine search loose
// files only, and a game whose startup.tjs lives in data.xp3 dies with
// "Cannot find storage startup.tjs". Hand the archive file over instead, which
// makes "…/data.xp3>" the storage root, exactly like the original launcher.
//
// A folder with a loose startup.tjs wins: that is a patched/unpacked tree where
// the loose script must take precedence over (or exist without) the archive.
// Recognised loose entry points include the data/ tree of the KAG3 SDK.
std::string resolve_project_data(const std::string& directory)
{
    namespace fs = std::filesystem;
    std::error_code ec;
    const fs::path root = path_from_utf8(directory);
    if (fs::is_regular_file(root, ec) || !fs::is_directory(root, ec)) {
        return directory; // bare .xp3 / .ks project, or missing path (open reports it)
    }
    const auto has = [&root](const char* name)
    {
        std::error_code ignored;
        return fs::exists(root / name, ignored);
    };
    if (has("startup.tjs") || has("start.tjs") || has("system/Initialize.tjs") ||
        has("System/Initialize.tjs")) {
        return directory;
    }

    // Packaged install: prefer the canonical archive, otherwise the single (or
    // largest) .xp3. Patch archives (patch.xp3, …) are mounted by the game's own
    // Initialize.tjs through System.exePath, so they are not the entry point.
    fs::path best;
    std::uintmax_t best_size = 0;
    for (const auto& entry :
         fs::directory_iterator(root, fs::directory_options::skip_permission_denied, ec))
    {
        if (ec || !entry.is_regular_file(ec)) {
            continue;
        }
        std::string name = path_to_utf8(entry.path().filename());
        // ASCII only: archive names are ASCII, and std::tolower is ambiguous on
        // bionic (the global C overload is also a candidate from `import std`).
        for (char& c : name) {
            if (c >= 'A' && c <= 'Z') {
                c = static_cast<char>(c - 'A' + 'a');
            }
        }
        if (name.size() < 4 || name.compare(name.size() - 4, 4, ".xp3") != 0) {
            continue;
        }
        if (name == "data.xp3") {
            return path_to_utf8(entry.path());
        }
        const std::uintmax_t size = entry.file_size(ec);
        if (size > best_size) {
            best = entry.path();
            best_size = size;
        }
    }
    return best.empty() ? directory : path_to_utf8(best);
}

} // namespace

struct engine::impl {
    std::unique_ptr<::enginec::third_party::krkr::runtime_state> token;
};

engine::engine() : impl_(std::make_unique<impl>()) {}

engine::~engine()
{
    // The Flutter host destroys its engine handle to leave a game; there is no
    // "close" op in the wire protocol. Without this call the process kept the
    // previous TVPApplication, TJS interpreter and SDL window alive, and the next
    // open() had to tear that abandoned session down right before initialising a
    // new one — the state that made the second launch crash inside
    // TVPInitScriptEngine. close() is idempotent and only touches a session this
    // instance owns.
    close();
}

bool engine::open(const open_options& options, info& out, std::string& error)
{
    auto& s = global_state();
    KzcEngineCrumb("open: begin");
    {
        char crumb[600];
        std::snprintf(crumb, sizeof(crumb), "open: dir=%s", options.dir.c_str());
        KzcEngineCrumb(crumb);
    }
    // A session from a previous handle (or from a failed open of this one) may still
    // be alive: SDL_AppInit/Quit are process-global, so it has to go before the new
    // application is constructed. teardown_session() is a no-op when nothing runs.
    teardown_session();
    // A previous open that failed halfway may also have left extra roots behind.
    TVPHostExtraRoots.clear();
    // Always start from a clean slate: a previous open may have failed halfway and
    // left a movie request or a stale error behind, and the host would read that as
    // belonging to the new game.
    s.clear();
    configure_drivers();
    s.cache_dir = options.cache_dir;
    // The materialiser must know the host cache before the startup script runs:
    // the very first Play() extracts into it.
    configure_media_cache();
    // Companion resource roots (multi-disc installs): publish them before
    // SDL_AppInit; sdl3_app drains the list before running the startup script.
    TVPHostExtraRoots.clear();
    for (const auto& dir : options.extra_dirs) {
        if (!dir.empty()) {
            TVPHostExtraRoots.push_back(dir);
        }
    }
    TVPAudioEventHook = &on_audio_event;
    // The engine has no video decoder in this build: the overlay reports the
    // request here and the host (Flutter/fvp) plays it.
    TVPMovieEventHook = &on_movie_event;

    // TVPParseArguments expects argv[1] to be the game path. The autotest flags
    // make the engine quiet and offscreen; TVPAutoTestSetup is called by
    // SDL_AppInit itself, so they must be on the command line.
    //
    // Path shape matters here: TVPParseArguments accepts either a folder or an
    // archive file (single-.xp3 games ship as a bare file), but it only reaches
    // the file branch when the path looks like a file. Appending a trailing
    // separator to a file makes BOTH of its existence checks fail, which surfaces
    // as a bare "SDL_AppInit failed". So: trailing separator only for folders
    // (upstream later concatenates "start.tjs"/"savedata/" onto the raw string),
    // files are passed exactly as given.
    // Resolve the install root to the actual project root when the host passed the
    // outer folder (see resolve_project_dir); a bare .xp3 file is left untouched.
    // Then pick the storage entry point: a packaged game must be opened through its
    // data.xp3 file, not the folder (see resolve_project_data).
    const std::string resolved_dir = resolve_project_dir(options.dir);
    std::string project = resolve_project_data(resolved_dir);
    if (project != options.dir) {
        char crumb[700];
        std::snprintf(crumb, sizeof(crumb), "open: resolved project=%s", project.c_str());
        KzcEngineCrumb(crumb);
    }
    {
        namespace fs = std::filesystem;
        std::error_code ec;
        const fs::path project_path = path_from_utf8(project);
        const bool is_dir = fs::is_directory(project_path, ec);
        const bool is_file = !is_dir && fs::is_regular_file(project_path, ec);
        if (!is_dir && !is_file) {
            error = "krkr: game path not found: " + project;
            s.error = error;
            return false;
        }
        if (is_dir && project.back() != '/' && project.back() != '\\') {
            project.push_back('/');
        }
    }
    // -vsync=0：离屏渲染没有显示器可同步，宿主自己控制帧节奏；个别平台的
    // vsync 实现会在隐藏窗口/软渲染器上阻塞，直接关掉（TVPSettings 默认开）。
    std::vector<std::string> args{"enginec", project, "-offscreen", "-silent", "-vsync=0"};
    std::vector<char*> argv;
    argv.reserve(args.size());
    for (auto& arg : args) {
        argv.push_back(arg.data());
    }
    void* appstate = nullptr;
    SDL_AppResult result = SDL_APP_FAILURE;
    KzcEngineCrumb("open: calling SDL_AppInit");
    // Published before the call: SDL_AppInit creates the process-global application
    // (and may fail *after* doing so). Recording the session here means a half-built
    // application is torn down by the next open()/close()/destructor instead of being
    // silently leaked into the next launch.
    s.sdl_running = true;
    // SDL_AppInit runs one frame itself before returning; the autotest setup inside
    // it resets the capture arm, so the probe below arms again rather than relying
    // on anything set before init.
    try {
        result = SDL_AppInit(&appstate, static_cast<int>(argv.size()), argv.data());
    } catch (const TJS::eTJS& failure) {
        const TJS::ttstr message = failure.GetMessage();
        error = std::string("krkr: engine startup failed: ") + message.c_str();
        s.error = error;
        teardown_session();
        return false;
    } catch (const std::exception& failure) {
        error = std::string("krkr: engine startup failed: ") + failure.what();
        s.error = error;
        teardown_session();
        return false;
    } catch (...) {
        error = "krkr: engine startup failed (TJS exception)";
        s.error = error;
        teardown_session();
        return false;
    }
    if (result == SDL_APP_FAILURE) {
        const char* reason = krkrsdl3::TVPAutoTestLastError();
        error = (reason != nullptr && *reason != '\0') ? reason : "krkr: SDL_AppInit failed";
        s.error = error;
        teardown_session();
        return false;
    }

    // Geometry must come from the engine's real presented frame.
    //
    // The host maps every tap from screen space into the coordinates it got from
    // open(): if the surface is not byte-for-byte the engine canvas, compose() may
    // still look right (it scales), but every click lands at the wrong engine
    // coordinate and buttons stop reacting. That happened when this used the SDL
    // window (offscreen driver bookkeeping) or the window paint box (can differ from
    // the draw buffer under zoom/letterboxing).
    //
    // So: present frames until one is captured and take its exact dimensions. This
    // also advances the game a couple of ticks, which is harmless during open.
    int width = 0;
    int height = 0;
    if (TVPWindow* window = TVPGetActiveWindow()) {
        width = static_cast<int>(window->GetWidth());
        height = static_cast<int>(window->GetHeight());
    }
    int window_width = 0;
    int window_height = 0;
    TVPGetWindowSize(&window_width, &window_height);

    tjs_int fw = 0;
    tjs_int fh = 0;
    tjs_int fpitch = 0;
    const std::uint8_t* first_frame = nullptr;
    // The autotest cache is process-global: on a re-open it still holds the previous
    // game's frame. Only accept a frame whose content version changed during this loop.
    const std::uint64_t stale_version = krkrsdl3::TVPAutoTestFrameVersion();
    bool have_frame = false;
    for (int attempt = 0; attempt < 24 && !have_frame; ++attempt) {
        if (krkrsdl3::TVPAutoTestFrameBudget() &&
            krkrsdl3::TVPAutoTestFrameCount() >= krkrsdl3::TVPAutoTestFrameBudget()) {
            break;
        }
        krkrsdl3::TVPAutoTestArmCapture();
        SDL_AppResult probe = SDL_APP_CONTINUE;
        try {
            probe = SDL_AppIterate(nullptr);
        } catch (...) {
            break; // a failing probe must not fail open()
        }
        have_frame = krkrsdl3::TVPAutoTestFrameVersion() != stale_version &&
                     krkrsdl3::TVPAutoTestFrameView(&first_frame, fw, fh, fpitch) &&
                     fw > 0 && fh > 0 && fpitch > 0;
        if (probe != SDL_APP_CONTINUE) {
            break;
        }
    }
    if (have_frame) {
        // No copy of `first_frame`: compose() reads the autotest cache through
        // TVPAutoTestFrameView until the engine presents a newer frame, so the
        // 3.7MB/8MB per-frame duplicate that used to live here is gone.
        s.width = fw;
        s.height = fh;
        // frame_valid stays false: the host has not received anything yet, so the
        // first compose() must deliver this frame instead of reporting "not dirty".
        s.got_first_frame = true;
    } else {
        s.width = width > 0 ? width : (window_width > 0 ? window_width : 1280);
        s.height = height > 0 ? height : (window_height > 0 ? window_height : 720);
    }
    // The host allocates its surface with exactly these dimensions (op_open f2/f3)
    // and maps pointer positions through them.
    s.surface_width = s.width;
    s.surface_height = s.height;
    {
        char crumb[320];
        std::snprintf(crumb, sizeof(crumb), "open: frame=%dx%d window=%dx%d", s.width,
                      s.height, window_width, window_height);
        KzcEngineCrumb(crumb);
    }

    // Remember the autotest host's cached frame: it is process-global and survives
    // close(), so compose() must not hand the previous game's picture to the new
    // host buffer before this game has presented one. When the probe above already
    // captured this game's frame there is nothing stale to avoid.
    s.stale_probe = s.got_first_frame ? 0 : krkrsdl3::TVPAutoTestFrameVersion();
    s.title = TVPGetWindowTitle();
    // Ownership is published together with open: from this point close()/the
    // destructor of *this* handle own the session, and a stale handle cannot end it.
    s.session_owner = this;
    s.open = true;
    // A static first screen must not report "animating" for the first 250 ms, so the
    // timestamp starts empty and only iterate_once() fills it when pixels change.
    s.last_present_ms.store(0, std::memory_order_relaxed);
    KzcEngineCrumb("open: ready");
    s.finished = result == SDL_APP_SUCCESS;
    s.error.clear();

    out.width = s.width;
    out.height = s.height;
    out.title = s.title;
    out.version = "krkrsdl3/KizunaEngine";
    out.scene_count = 1;
    out.entry_count = 1;
    return true;
}

void engine::close()
{
    auto& s = global_state();
    // Only the instance that opened the session may end it; a late close() from a
    // stale handle must not tear down the game a newer handle is playing.
    if (!s.open && !s.sdl_running) {
        return;
    }
    if (s.session_owner != nullptr && s.session_owner != this) {
        return;
    }
    KzcEngineCrumb("close: begin");
    teardown_session();
    s.session_owner = nullptr;
    s.clear();
}

bool engine::is_open() const noexcept
{
    const auto& s = global_state();
    return s.open && s.session_owner == this;
}

step engine::start(std::uint32_t entry, std::string& error)
{
    (void)entry;
    error.clear();
    auto& s = global_state();
    if (!s.open) {
        error = "krkr: game is not open";
        return step::faulted;
    }
    // SDL_AppInit already ran one frame; the title/opening scene is now live. The
    // opening scene may itself start with a movie (title OP), so report the wait.
    if (movie_waiting()) {
        return step::await_movie;
    }
    return s.finished ? step::finished : step::cont;
}

step engine::run(std::uint32_t budget, std::string& error)
{
    error.clear();
    // One engine iteration per host tick. budget is the script-command budget of
    // enginego; a frame of the real engine already runs the script to its next
    // wait, so looping would only burn battery.
    (void)budget;
    const step result = iterate_once();
    if (result == step::faulted) {
        error = global_state().error;
    }
    return result;
}

step engine::advance(std::string& error)
{
    error.clear();
    // A click must not be queued while the host owns the frame: the engine is
    // waiting inside the overlay, and the tap would land on the scene after the
    // movie once playback ends.
    if (movie_waiting()) {
        return step::await_movie;
    }
    int cx = global_state().width / 2;
    int cy = global_state().height / 2;
    // Same canvas -> window conversion as inject_mouse: the click is generated at
    // the canvas centre, the engine's trigger expects window pixels.
    canvas_to_window(cx, cy);
    krkrsdl3::TVPAutoTestWarpMouse(cx, cy);
    krkrsdl3::TVPAutoTestMouseButton(true, 0, cx, cy);
    krkrsdl3::TVPAutoTestMouseButton(false, 0, cx, cy);
    const step result = iterate_once();
    if (result == step::faulted) {
        error = global_state().error;
    }
    return result;
}

step engine::choose(std::uint32_t index, std::string& error)
{
    (void)index;
    // Choices are drawn and hit-tested by the game itself; the host clicks them.
    return advance(error);
}

step engine::pointer(std::uint32_t op, std::int32_t x, std::int32_t y, std::string& error)
{
    error.clear();
    if (movie_waiting()) {
        return step::await_movie;
    }
    inject_mouse(op, x, y);
    const step result = iterate_once();
    if (result == step::faulted) {
        error = global_state().error;
    }
    return result;
}

step engine::press_key(std::uint32_t code, std::string& error)
{
    error.clear();
    if (movie_waiting()) {
        return step::await_movie;
    }
    const char* name = scancode_name(code);
    if (name != nullptr) {
        krkrsdl3::TVPAutoTestKeyDown(name, true);
        krkrsdl3::TVPAutoTestKeyDown(name, false);
    }
    const step result = iterate_once();
    if (result == step::faulted) {
        error = global_state().error;
    }
    return result;
}

step engine::wheel(std::int32_t delta, std::string& error)
{
    error.clear();
    if (movie_waiting()) {
        return step::await_movie;
    }
    const auto& s = global_state();
    krkrsdl3::TVPAutoTestMouseWheel(s.width / 2, s.height / 2, 0.0f,
                                    static_cast<float>(delta));
    const step result = iterate_once();
    if (result == step::faulted) {
        error = global_state().error;
    }
    return result;
}

bool engine::compose(std::uint8_t* dst, std::size_t n, int surface_width, int surface_height,
                     std::string& error)
{
    error.clear();
    auto& s = global_state();
    if (!s.open || dst == nullptr) {
        return false;
    }

    // Adopt the geometry the caller's buffer actually has. The host resizes its
    // surface when canvas_size() changes, so this is where a 480x280 boot window
    // becomes the game's real 1920x1080 surface. A request that cannot be honoured
    // (zero size, or a buffer shorter than the geometry claims) keeps the previous
    // surface, which is always safe because that buffer was already allocated.
    bool surface_changed = false;
    if (surface_width > 0 && surface_height > 0 &&
        n >= static_cast<std::size_t>(surface_width) * static_cast<std::size_t>(surface_height) *
                 4u) {
        surface_changed = surface_width != s.surface_width || surface_height != s.surface_height;
        s.surface_width = surface_width;
        s.surface_height = surface_height;
    }

    // The engine frame is driven by run/advance/pointer/... (one SDL_AppIterate
    // each, see iterate_once). compose() only converts the frame captured by the
    // most recent iteration: the host calls compose from a separate render tick
    // (throttled to ~30fps) and the stepping ops from its 16ms logic tick, so an
    // SDL_AppIterate here would add an extra engine frame per render and make
    // script pacing depend on the render throttle.
    //
    // The version comes first because it is a plain integer compare: on a static
    // scene most host ticks reach here with unchanged pixels, and those ticks must
    // not touch the frame at all (they used to hash every pixel, then copy 8MB).
    const std::uint64_t version = krkrsdl3::TVPAutoTestFrameVersion();
    const std::uint8_t* frame = nullptr;
    tjs_int width = 0;
    tjs_int height = 0;
    tjs_int pitch = 0;
    const bool have_frame =
        krkrsdl3::TVPAutoTestFrameView(&frame, width, height, pitch) && width > 0 &&
        height > 0 && pitch > 0;
    if (!surface_changed && s.frame_valid && version == s.last_version &&
        (!have_frame || (width == s.width && height == s.height))) {
        // Same pixels *and* same geometry as the last composed frame: the host keeps
        // its texture and the reply says "not dirty". Comparing the cached size too
        // catches a resolution change whose pixels are identical (black boot window
        // -> black main window), which the version alone would miss.
        return false;
    }
    if (s.stale_probe != 0) {
        // The autotest cache still holds the frame from before open(): no frame of
        // this game has been presented yet, so handing it over would show the
        // previous game. Keep skipping until a new frame bumps the version.
        if (version == s.stale_probe) {
            return false;
        }
        s.stale_probe = 0;
    }

    if (!have_frame) {
        // Nothing was ever presented: there is no frame to hand over yet. The host
        // keeps the texture it already uploaded (or stays black on the very first
        // frame, which is what the engine would show too).
        return false;
    }
    s.width = width;
    s.height = height;

    // Destinations are laid out with the surface geometry the host announced in this
    // request (open()'s size for the first frames). If the game changes its draw
    // buffer (boot options -> full-size window, fullscreen toggle), scale the frame
    // to fit instead of cropping it: cropping is what made only the top-left part of
    // the picture visible. The reply tells the host the new canvas size, and the next
    // request arrives with a matching buffer, so this path runs for one frame only.
    const int dst_width = s.surface_width > 0 ? s.surface_width : width;
    const int dst_height = s.surface_height > 0 ? s.surface_height : height;
    const std::size_t dst_stride = static_cast<std::size_t>(dst_width) * 4;
    if (n < dst_stride * static_cast<std::size_t>(dst_height)) {
        return false;
    }

    s.frame_valid = true;
    s.last_version = version;

    // Engine frames are already RGBA, and the host surface reads RGBA
    // (FramePainter decodes the buffer with PixelFormat.rgba8888), so pixels are
    // copied through unchanged — the same exemption the Go core grants brands
    // declaring ComposeWritesBGRA (whose name is historical: those brands write
    // the host byte order directly; see enginego/brand/blackpackage's note).
    // Swapping R/B here turns the whole frame red-blue reversed.
    //
    // Alpha is already forced opaque by the capture (StoreCapturedFrame ORs 0xff
    // into every pixel), so the 1:1 path is a straight row copy: the previous
    // per-pixel loop wrote the same bytes, one cache line at a time, and was the
    // single largest host-side cost per frame.
    if (width == dst_width && height == dst_height) {
        if (pitch == static_cast<tjs_int>(dst_stride)) {
            std::memcpy(dst, frame, dst_stride * static_cast<std::size_t>(dst_height));
        } else {
            for (int y = 0; y < height; ++y) {
                std::memcpy(dst + static_cast<std::size_t>(y) * dst_stride,
                            frame + static_cast<std::size_t>(y) * pitch, dst_stride);
            }
        }
        return true;
    }

    // Aspect-preserving fit with black letterbox bars. Nearest-neighbour is enough
    // for a fallback that only runs after a mid-game resolution change; the normal
    // path above is a straight copy.
    int fit_width = dst_width;
    int fit_height = dst_height;
    if (static_cast<long long>(width) * dst_height >
        static_cast<long long>(height) * dst_width) {
        fit_height = std::max(1, static_cast<int>(static_cast<long long>(height) *
                                                  dst_width / width));
    } else {
        fit_width = std::max(1, static_cast<int>(static_cast<long long>(width) *
                                                 dst_height / height));
    }
    const int offset_x = (dst_width - fit_width) / 2;
    const int offset_y = (dst_height - fit_height) / 2;
    std::memset(dst, 0, dst_stride * static_cast<std::size_t>(dst_height));
    std::vector<int> xmap(static_cast<std::size_t>(fit_width));
    for (int x = 0; x < fit_width; ++x) {
        xmap[static_cast<std::size_t>(x)] =
            static_cast<int>(static_cast<long long>(x) * width / fit_width) * 4;
    }
    for (int y = 0; y < fit_height; ++y) {
        const int src_y = static_cast<int>(static_cast<long long>(y) * height / fit_height);
        const std::uint8_t* src = frame + static_cast<std::size_t>(src_y) * pitch;
        std::uint8_t* out = dst + static_cast<std::size_t>(offset_y + y) * dst_stride +
                            static_cast<std::size_t>(offset_x) * 4;
        for (int x = 0; x < fit_width; ++x) {
            std::memcpy(out + x * 4, src + xmap[static_cast<std::size_t>(x)], 4);
        }
    }
    return true;
}

engine::frame_size engine::canvas_size() const
{
    const auto& s = global_state();
    frame_size out;
    out.width = s.width;
    out.height = s.height;
    return out;
}

info engine::information() const
{
    const auto& s = global_state();
    info out;
    // The surface geometry is the only size the host can allocate; the captured
    // frame size can change if the game resizes its draw buffer.
    out.width = s.surface_width > 0 ? s.surface_width : s.width;
    out.height = s.surface_height > 0 ? s.surface_height : s.height;
    out.title = s.title;
    out.version = "krkrsdl3/KizunaEngine";
    out.scene_count = 0;
    out.entry_count = 0;
    return out;
}

std::vector<scene_info> engine::scenes() const { return {}; }
std::vector<slot_info> engine::slots() const { return {}; }

bool engine::save(std::uint32_t slot, std::string_view title, std::string& error)
{
    (void)slot;
    (void)title;
    error = "krkr: save slots are handled by the game's own data screen";
    return false;
}

step engine::load(std::uint32_t slot, std::string& error)
{
    (void)slot;
    error = "krkr: save slots are handled by the game's own data screen";
    return step::faulted;
}

// Game assets live in the engine's storage layer, not in the file system: a file
// may be loose, inside data.xp3, or behind an XP3 filter. TVPCreateStream is the
// only API that resolves all three, so both asset ops go through it exactly like
// the audio/movie materializers above.
std::size_t engine::asset_size(std::string_view path) const
{
    if (path.empty() || !global_state().open) {
        return 0;
    }
    try {
        const ttstr name(path.data(), static_cast<int>(path.size()));
        tTJSBinaryStream* stream = TVPCreateStream(name, TJS_BS_READ);
        if (stream == nullptr) {
            return 0;
        }
        const auto size = static_cast<std::size_t>(stream->GetSize());
        delete stream;
        return size;
    } catch (...) {
        // A missing/corrupt entry raises a TJS exception; the protocol answer for
        // "not available" is size 0, not a failed RPC.
        return 0;
    }
}

std::size_t engine::read_asset(std::string_view path, std::uint8_t* dst, std::size_t n) const
{
    if (path.empty() || dst == nullptr || n == 0 || !global_state().open) {
        return 0;
    }
    try {
        const ttstr name(path.data(), static_cast<int>(path.size()));
        tTJSBinaryStream* stream = TVPCreateStream(name, TJS_BS_READ);
        if (stream == nullptr) {
            return 0;
        }
        const auto size = static_cast<std::size_t>(stream->GetSize());
        const std::size_t count = size < n ? size : n;
        // Read() (not ReadBuffer) so a truncated entry returns what it has instead
        // of throwing: the host asked for "as much as fits".
        const tjs_uint read = count > 0 ? stream->Read(dst, static_cast<tjs_uint>(count)) : 0;
        delete stream;
        return static_cast<std::size_t>(read);
    } catch (...) {
        return 0;
    }
}

std::vector<audio_event> engine::take_audio()
{
    auto& s = global_state();
    std::lock_guard<std::mutex> lock(s.audio_mutex);
    std::vector<audio_event> out;
    out.swap(s.audio);
    return out;
}

text_state engine::text() const { return {}; }
std::vector<choice> engine::choices() const { return {}; }

engine_state engine::state() const
{
    engine_state out;
    const auto& s = global_state();
    out.message_window_visible = s.open;
    // The real engine paints the message window and all UI into the frame.
    out.engine_message_window = true;
    out.pc = 0;
    out.scene = 0;
    out.cease_skip = true;
    out.auto_wait = 50;
    // "Animating" means the engine is still presenting new pixels, not "the script
    // happens to be busy": a transition, a fading layer or a scrolling text box all
    // keep presenting frames, while a scene waiting for a click presents nothing.
    // The host composes at display rate only while this is true, so an idle game
    // must report false or it would decode a full frame on every tick for nothing.
    // 250 ms tolerates a slow engine frame (a heavy cross-fade can take longer than
    // one host tick) without keeping the host at full rate after the picture stops.
    const std::int64_t last = s.last_present_ms.load(std::memory_order_relaxed);
    out.animating = s.open && last != 0 && monotonic_ms() - last < 250;
    return out;
}

std::vector<text_unit> engine::text_list() const { return {}; }

std::string engine::last_error() const { return global_state().error; }

bool engine::submit_name(std::string_view value)
{
    (void)value;
    return false;
}

bool engine::submit_yes_no(bool yes)
{
    (void)yes;
    return false;
}

void engine::notify_audio_completed(std::string_view file, bool voice, std::uint32_t channel)
{
    (void)file;
    (void)voice;
    // Only the channel identifies the buffer: the host's path is a materialised copy
    // whose name the engine never sees, and two buffers can legitimately play the
    // same file. The engine resolves the channel back to its WaveSoundBuffer and
    // stops it on the engine thread, which is what makes a script waiting for a
    // one-shot to finish continue (see PlatformAudioHook.h).
    //
    // 0xFFFFFFFF is the protocol's "no channel": those events came from the Go
    // engine's brands (which do not address channels) and must not be cast to a
    // negative int here.
    if (channel == 0xFFFFFFFFu) {
        return;
    }
    TVPAudioHostCompleted(static_cast<int>(channel));
}

void engine::set_option(std::uint32_t key, bool value)
{
    (void)key;
    (void)value;
}

step engine::finish_movie(std::string& error)
{
    error.clear();
    auto& s = global_state();
    if (!s.open) {
        error = "krkr: game is not open";
        return step::faulted;
    }
    int serial = 0;
    {
        std::lock_guard<std::mutex> lock(s.movie_mutex);
        if (!s.movie_pending.load(std::memory_order_relaxed)) {
            // No wait is active: answering with a plain engine step keeps the op
            // idempotent. A late/duplicate completion from the host must not
            // advance a script that is not waiting on a movie.
            return s.finished ? step::finished : step::cont;
        }
        // movie_storage stays set on purpose: it is what recognises the overlay's
        // echo of this same movie while the completion below runs.
        serial = s.movie_serial;
        s.movie_path.clear();
        s.movie_pending.store(false, std::memory_order_release);
        s.movie_completing = true;
    }
    // Releases the overlay's wait; the script continues on the next iteration and
    // may immediately ask for the following movie (a fresh request overwrites the
    // fields below, which is why they are only cleared when still unused).
    TVPMovieNotifyFinished(serial);
    {
        std::lock_guard<std::mutex> lock(s.movie_mutex);
        s.movie_completing = false;
        if (!s.movie_pending.load(std::memory_order_relaxed)) {
            s.movie_storage.clear();
        }
    }
    return iterate_once();
}

movie_request engine::current_movie() const
{
    auto& s = global_state();
    std::lock_guard<std::mutex> lock(s.movie_mutex);
    // A pending request is returned unchanged on every poll (the host re-reads it
    // each frame). It is consumed only by finish_movie(), so the host never has
    // to reason about "already handed over" state. The serial is the overlay's own
    // id, the same number finish_movie() hands to TVPMovieNotifyFinished
    // (PlatformMovieHook.h: the overlay owns the serial).
    if (!s.movie_pending.load(std::memory_order_relaxed)) {
        return {};
    }
    movie_request out;
    out.path = s.movie_path;
    out.serial = s.movie_serial;
    out.audio_gain = s.movie_audio_gain;
    return out;
}

step engine::current_step() const noexcept
{
    auto& s = global_state();
    if (!s.open) {
        return step::faulted;
    }
    if (s.finished) {
        return step::finished;
    }
    return s.movie_pending.load(std::memory_order_acquire) ? step::await_movie : step::cont;
}

probe_result probe(std::string_view directory)
{
    probe_result out;
    namespace fs = std::filesystem;
    // Resolve first: a caller that only knows the install root still gets the
    // data/ (or Files/) project root, and single-file games keep their path.
    const fs::path root = path_from_utf8(resolve_project_dir(std::string(directory)));
    std::error_code ec;
    if (fs::is_regular_file(root, ec)) {
        // 单文件游戏（一个裸 .xp3/.ks 就是全部内容）：与 Go 侧 library.Scan
        // 的判定保持一致，data_dir 指向其父目录——引擎的存储层会自动挂载
        // 项目目录里的 xp3，启动脚本在归档内部。
        const auto ext = root.extension().string();
        if (ext == ".xp3" || ext == ".ks") {
            out.is_krkr = true;
            out.data_dir = path_to_utf8(root.parent_path());
            out.title = path_to_utf8(root.stem());
            out.width = 1280;
            out.height = 720;
        }
        return out;
    }
    if (!fs::is_directory(root, ec)) {
        return out;
    }
    const fs::path candidates[] = {root / "data.xp3", root / "startup.tjs",
                                   root / "start.tjs", root / "first.ks",
                                   root / "scenario"};
    for (const auto& candidate : candidates) {
        if (fs::exists(candidate, ec)) {
            out.is_krkr = true;
            break;
        }
    }
    if (!out.is_krkr && looks_like_project_root(root)) {
        out.is_krkr = true;
    }
    if (!out.is_krkr) {
        for (const auto& entry : fs::directory_iterator(root, ec)) {
            if (ec) {
                break;
            }
            const auto ext = entry.path().extension().string();
            if (ext == ".xp3" || ext == ".ks" || entry.path().filename() == "data.xp3") {
                out.is_krkr = true;
                break;
            }
        }
    }
    if (out.is_krkr) {
        out.data_dir = path_to_utf8(root);
        out.title = path_to_utf8(root.filename());
        out.width = 1280;
        out.height = 720;
    }
    return out;
}

bool detect(std::string_view directory) { return probe(directory).is_krkr; }

// Installs the host's glyph rasterizer. The engine keeps layout and composition;
// only "glyph -> pixels" moves to Flutter (PlatformTextHook.h). Passing nullptr
// restores the FreeType path, which is what a host without a text stack gets.
void set_text_renderer(text_render_callback callback)
{
    // Install the trampoline before publishing the callback, and clear the hook
    // before dropping the callback: a concurrent render either sees "no host
    // rasterizer" or sees both, never a half-installed pair.
    if (callback == nullptr) {
        TVPTextRenderHook = nullptr;
        host_text_renderer.store(nullptr, std::memory_order_release);
        return;
    }
    host_text_renderer.store(callback, std::memory_order_release);
    TVPTextRenderHook = &host_text_trampoline;
}

std::unique_ptr<engine> create_engine() { return std::make_unique<engine>(); }

std::string version_string() { return "kizuna-enginec/1.0.0 (krkrsdl3)"; }

} // namespace enginec::third_party::krkr
