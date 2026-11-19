// Stubs and host bridges for subsystems this build does not ship.
//
// Movie overlays: this build has no video decoder (core/media/movie is not
// compiled), so decoding is delegated to the Flutter host (fvp). The overlays
// below never touch pixels: they materialize the movie into a file the host can
// open, report the request through environ/PlatformMovieHook.h and finish
// through the same handler the FFmpeg player reaches (EC_COMPLETE into
// tTJSNI_VideoOverlay::WndProc), which stops the overlay and fires
// onStatusChanged("stop") so the script parked on the movie continues.
#include "tjsCommHead.h"
#include "krmovie.h"
#include "Platform.h"
#include "PlatformFile.h"
#include "PlatformMediaCache.h"
#include "PlatformMovieHook.h"
#include "PlatformVideo.h"
#include "TVPApplication.h"
#include "TVPStorage.h"
#include "UtilStreams.h"

#include <atomic>
#include <chrono>
#include <cstdint>
#include <map>
#include <memory>
#include <mutex>
#include <string>
#include <utility>

// The vendored engine writes startup breadcrumbs through KzcEngineCrumb (defined
// in ffi/crashlog.cpp). enginec_core is also linked standalone by the host tests,
// where that TU is absent; this weak fallback keeps those binaries linkable. In
// the shared library the strong definition wins, so the real breadcrumb file is
// still written at runtime.
#if defined(__GNUC__) || defined(__clang__)
extern "C" __attribute__((weak)) void KzcEngineCrumb(const char* message)
{
    (void)message;
}
#endif

namespace {

// ─── outstanding host request ───────────────────────────────────────────────
// One entry per request handed to the host. TVPMovieNotifyFinished may run on
// any thread (the host bridge), but the overlay and its NativeEventQueue belong
// to the engine thread, so the completion is posted as a user message and
// dispatched there. Only `callback` is shared between the two, under `mutex`:
// overlay teardown clears it under the same lock, so a late completion finds
// nothing to call instead of reaching into a destroyed object. (Posting through
// the overlay's own event queue would leave a queued callable pointing at the
// overlay after teardown, for as long as the queue is not drained.)
struct movie_bridge
{
    movie_bridge(iTVPVideoCallback* owner, bool via_state_change)
      : callback(owner),
        state_change(via_state_change)
    {
    }

    std::mutex mutex;
    iTVPVideoCallback* callback; // the overlay (tTJSNI_VideoOverlay / layerExMovie)
    const bool state_change;     // MFEVR mode finishes via WM_STATE_CHANGE
    std::atomic<bool> delivered{false};

    // Delivers one event to the overlay's WndProc. Called from a user message,
    // i.e. on the engine thread, which is where the overlay and its TJS state
    // live. `callback` is re-read under the lock: overlay teardown clears it, so
    // a message that was already queued cannot reach a destroyed object. The
    // lock is released before the call (WndProc re-enters Stop->detach, which
    // takes it again); that is safe because both the check and the call happen on
    // the same thread as teardown, so nothing can be destroyed in between.
    void deliver(int message, std::intptr_t wparam)
    {
        iTVPVideoCallback* target = nullptr;
        {
            std::lock_guard<std::mutex> lock(mutex);
            target = callback;
        }
        if (target == nullptr)
            return;
        NativeEvent ev(message);
        ev.WParam = wparam;
        ev.LParam = 0;
        target->WndProc(ev);
    }

    // Queues a completion for the engine thread. `self` keeps the bridge (not
    // the overlay) alive until the message runs; the overlay is guarded by
    // `callback` above. Delivering through WndProc directly is the same handler
    // a queued EC_COMPLETE would reach, minus an event-queue entry that could
    // outlive teardown.
    void schedule_completion(const std::shared_ptr<movie_bridge>& self)
    {
        {
            std::lock_guard<std::mutex> lock(mutex);
            if (delivered.exchange(true, std::memory_order_acq_rel) || callback == nullptr ||
                Application == nullptr)
            {
                return;
            }
        }
        const bool via_state = state_change;
        Application->PostUserMessage(
            [self, via_state]()
            {
                self->deliver(via_state ? WM_STATE_CHANGE : WM_GRAPHNOTIFY,
                              via_state ? static_cast<std::intptr_t>(vsEnded)
                                        : static_cast<std::intptr_t>(EC_COMPLETE));
            },
            nullptr, 0);
    }

    // Same, for the "a movie is playing" status MFEVR mode needs before it will
    // honour a completion.
    void schedule_playing(const std::shared_ptr<movie_bridge>& self)
    {
        if (Application == nullptr)
            return;
        Application->PostUserMessage(
            [self]()
            {
                self->deliver(WM_STATE_CHANGE, static_cast<std::intptr_t>(vsPlaying));
            },
            nullptr, 0);
    }
};

std::mutex g_bridge_mutex;
std::map<int, std::shared_ptr<movie_bridge>> g_bridges;
std::atomic<int> g_next_serial{1};
// The tTVPApplication instance the registered bridges belong to. The player can
// close and reopen a game (a new Application, and every overlay from the previous
// session is gone), so registrations must not outlive the session: a stale entry
// would let a completion "resume" a script that no longer exists.
tTVPApplication* g_bridge_session = nullptr;

void register_bridge(int serial, const std::shared_ptr<movie_bridge>& bridge)
{
    std::lock_guard<std::mutex> lock(g_bridge_mutex);
    if (g_bridge_session != Application)
    {
        g_bridges.clear();
        g_bridge_session = Application;
    }
    g_bridges[serial] = bridge;
}

void unregister_bridge(int serial, const std::shared_ptr<movie_bridge>& bridge)
{
    if (serial == 0)
        return;
    std::lock_guard<std::mutex> lock(g_bridge_mutex);
    const auto found = g_bridges.find(serial);
    // Erase by identity, not just by serial: a newer session may be reusing the
    // slot with its own bridge.
    if (found != g_bridges.end() && found->second == bridge)
        g_bridges.erase(found);
}

// Container FPS is unknown without a decoder; only frame<->time estimates use
// this value, and no engine path depends on it being exact.
constexpr double kNominalFps = 30.0;

// Named alias because <time.h> already puts a ::clock function in scope.
using wall_clock = std::chrono::steady_clock;

// ─── host-delegated video overlay ───────────────────────────────────────────
// One instance per engine-side VideoOverlay object. All methods run on the
// engine thread except the completion notification, which only goes through
// movie_bridge.
class HostMovieOverlay final : public iTVPVideoOverlay
{
public:
    HostMovieOverlay(iTVPVideoCallback* callback, tTJSBinaryStream* stream,
                     const tjs_char* storage, const tjs_char* type, bool layer_mode,
                     bool state_change)
      : callback_(callback),
        stream_(stream),
        storage_(storage != nullptr ? storage : TJS_N("")),
        ext_(type != nullptr ? type : TJS_N("")),
        layer_mode_(layer_mode),
        state_change_(state_change)
    {
    }

    HostMovieOverlay(const HostMovieOverlay&) = delete;
    HostMovieOverlay& operator=(const HostMovieOverlay&) = delete;

    // iTVPVideoOverlay has no virtual destructor (its lifetime is governed by
    // AddRef/Release), so this must not be marked override.
    ~HostMovieOverlay()
    {
        detach_bridge();
        delete stream_;
        stream_ = nullptr;
        // No file deletion here: the materialised copy lives in the shared host
        // cache and is reused by later requests for the same movie.
    }

    // -- reference counting -------------------------------------------------
    void AddRef() override { ref_.fetch_add(1, std::memory_order_relaxed); }
    void Release() override
    {
        if (ref_.fetch_sub(1, std::memory_order_acq_rel) == 1)
            delete this;
    }

    // -- placement: the host owns the pixels, the engine only waits ----------
    void SetWindow(tTJSNI_Window*) override {}
    void SetMessageDrainWindow(void*) override {}
    void SetRect(int, int, int, int) override {}
    void SetVisible(bool) override {}

    // -- transport ----------------------------------------------------------
    void Play() override
    {
        std::string storage;
        std::string local;
        int serial = 0;
        std::shared_ptr<movie_bridge> bridge;
        bool report = false;
        {
            std::lock_guard<std::mutex> lock(mutex_);
            if (bridge_ && !bridge_->delivered.load(std::memory_order_acquire))
            {
                // Already handed to the host: play() here means resume. Reporting
                // again would restart the decoder the host is running and break
                // the serial the host quotes back on completion.
                state_ = movie_state::playing;
                started_ = wall_clock::now();
                return;
            }
            serial = start_request_locked(storage, local, bridge);
            state_ = movie_state::playing;
            report = true;
        }
        if (report)
            report_request(serial, bridge, storage, local);
    }

    void Stop() override
    {
        std::lock_guard<std::mutex> lock(mutex_);
        base_ms_ = position_locked();
        state_ = movie_state::ended;
        // Stop means the engine no longer waits, so a later host completion for
        // this request must not fire another EC_COMPLETE.
        detach_bridge_locked();
    }

    void Pause() override
    {
        std::lock_guard<std::mutex> lock(mutex_);
        if (state_ == movie_state::playing)
        {
            base_ms_ = position_locked();
            state_ = movie_state::paused;
        }
        // The request stays registered: the host keeps playing (there is no
        // pause channel in the contract) and may still report completion.
    }

    void Rewind() override
    {
        std::lock_guard<std::mutex> lock(mutex_);
        base_ms_ = 0;
        started_ = wall_clock::now();
        if (state_ == movie_state::ended)
            state_ = movie_state::idle;
        // No seek channel: the engine only rewinds its own timeline. It follows a
        // rewind with Play() (tTJSNI_VideoOverlay::Rewind and the loop EC_COMPLETE
        // handler both do), so the delivered request is detached here — otherwise
        // Play() would see it as "the host is already playing" and a looped movie
        // would only ever play once.
        if (bridge_ && bridge_->delivered.load(std::memory_order_acquire))
            detach_bridge_locked();
    }

    // -- timeline (reported, not decoded) -----------------------------------
    void SetPosition(uint64_t tick) override
    {
        std::lock_guard<std::mutex> lock(mutex_);
        base_ms_ = tick;
        started_ = wall_clock::now();
    }

    void GetPosition(uint64_t* tick) override
    {
        if (tick == nullptr)
            return;
        std::lock_guard<std::mutex> lock(mutex_);
        *tick = position_locked();
    }

    void GetStatus(tTVPVideoStatus* status) override
    {
        if (status == nullptr)
            return;
        std::lock_guard<std::mutex> lock(mutex_);
        switch (state_)
        {
            case movie_state::playing:
                *status = vsPlaying;
                break;
            case movie_state::paused:
                *status = vsPaused;
                break;
            case movie_state::ended:
                *status = vsEnded;
                break;
            default:
                *status = vsStopped;
                break;
        }
    }

    void SetFrame(int f) override
    {
        std::lock_guard<std::mutex> lock(mutex_);
        base_ms_ = f > 0 ? static_cast<std::uint64_t>(f * 1000.0 / kNominalFps) : 0;
        started_ = wall_clock::now();
    }

    void GetFrame(int* f) override
    {
        if (f == nullptr)
            return;
        std::lock_guard<std::mutex> lock(mutex_);
        *f = static_cast<int>(static_cast<double>(position_locked()) * kNominalFps / 1000.0);
    }

    void GetFPS(double* f) override
    {
        if (f != nullptr)
            *f = kNominalFps;
    }

    void GetNumberOfFrame(int* f) override
    {
        // Duration is only known to the host's decoder.
        if (f != nullptr)
            *f = 0;
    }

    void GetTotalTime(int64_t* t) override
    {
        if (t != nullptr)
            *t = 0;
    }

    void GetVideoSize(long* width, long* height) override
    {
        // The engine never decodes, so the real size is unknown. Layer mode must
        // report a positive size because tTJSNI_VideoOverlay::Open rejects 0x0
        // before it can fall back; 1x1 keeps the (unused) layer allocation
        // negligible since the host draws the movie. Overlay/mixer report 0.
        const long size = layer_mode_ ? 1 : 0;
        if (width != nullptr)
            *width = size;
        if (height != nullptr)
            *height = size;
    }

    tTVPBaseTexture* GetFrontBuffer() override { return nullptr; }
    void SetVideoBuffer(tTVPBaseTexture*, tTVPBaseTexture*, long) override {}
    void SetMixingBitmap(tTVPBaseTexture*, float) override {}
    void ResetMixingBitmap() override {}
    void PresentVideoImage() override {}

    void SetStopFrame(int frame) override { stop_frame_ = frame; }
    void GetStopFrame(int* frame) override
    {
        if (frame != nullptr)
            *frame = stop_frame_;
    }
    void SetDefaultStopFrame() override { stop_frame_ = -1; }

    // -- playback parameters (stored, not applied: no decoder here) ----------
    void SetPlayRate(double rate) override { play_rate_ = rate > 0.0 ? rate : 1.0; }
    void GetPlayRate(double* rate) override
    {
        if (rate != nullptr)
            *rate = play_rate_;
    }

    void SetAudioBalance(long balance) override { audio_balance_ = balance; }
    void GetAudioBalance(long* balance) override
    {
        if (balance != nullptr)
            *balance = audio_balance_;
    }

    void SetAudioVolume(long volume) override { audio_volume_ = volume; }
    void GetAudioVolume(long* volume) override
    {
        if (volume != nullptr)
            *volume = audio_volume_;
    }

    // The movie's audio is part of the host playback and cannot be selected or
    // disabled from the engine side.
    void GetNumberOfAudioStream(unsigned long* count) override
    {
        if (count != nullptr)
            *count = 0;
    }
    void SelectAudioStream(unsigned long) override {}
    void GetEnableAudioStreamNum(long* num) override
    {
        if (num != nullptr)
            *num = -1;
    }
    void DisableAudioStream() override {}

    void GetNumberOfVideoStream(unsigned long* count) override
    {
        if (count != nullptr)
            *count = 1;
    }
    void SelectVideoStream(unsigned long) override {}
    void GetEnableVideoStreamNum(long* num) override
    {
        if (num != nullptr)
            *num = 0;
    }

    // Loop playback is re-requested by the engine itself (the EC_COMPLETE
    // handler rewinds and plays again), so no host-side loop control is needed.
    void SetLoopSegement(int, int) override {}

    void SetMixingMovieAlpha(float a) override { mixing_alpha_ = a; }
    void GetMixingMovieAlpha(float* a) override
    {
        if (a != nullptr)
            *a = mixing_alpha_;
    }
    void SetMixingMovieBGColor(unsigned long col) override { mixing_bg_ = col; }
    void GetMixingMovieBGColor(unsigned long* col) override
    {
        if (col != nullptr)
            *col = mixing_bg_;
    }

    void GetContrastRangeMin(float* v) override { set(v, -1.0f); }
    void GetContrastRangeMax(float* v) override { set(v, 1.0f); }
    void GetContrastDefaultValue(float* v) override { set(v, 0.0f); }
    void GetContrastStepSize(float* v) override { set(v, 0.01f); }
    void GetContrast(float* v) override { set(v, contrast_); }
    void SetContrast(float v) override { contrast_ = v; }

    void GetBrightnessRangeMin(float* v) override { set(v, -1.0f); }
    void GetBrightnessRangeMax(float* v) override { set(v, 1.0f); }
    void GetBrightnessDefaultValue(float* v) override { set(v, 0.0f); }
    void GetBrightnessStepSize(float* v) override { set(v, 0.01f); }
    void GetBrightness(float* v) override { set(v, brightness_); }
    void SetBrightness(float v) override { brightness_ = v; }

    void GetHueRangeMin(float* v) override { set(v, -180.0f); }
    void GetHueRangeMax(float* v) override { set(v, 180.0f); }
    void GetHueDefaultValue(float* v) override { set(v, 0.0f); }
    void GetHueStepSize(float* v) override { set(v, 1.0f); }
    void GetHue(float* v) override { set(v, hue_); }
    void SetHue(float v) override { hue_ = v; }

    void GetSaturationRangeMin(float* v) override { set(v, -1.0f); }
    void GetSaturationRangeMax(float* v) override { set(v, 1.0f); }
    void GetSaturationDefaultValue(float* v) override { set(v, 0.0f); }
    void GetSaturationStepSize(float* v) override { set(v, 0.01f); }
    void GetSaturation(float* v) override { set(v, saturation_); }
    void SetSaturation(float v) override { saturation_ = v; }

private:
    enum class movie_state : std::uint8_t
    {
        idle,
        playing,
        paused,
        ended,
    };

    static void set(float* out, float value)
    {
        if (out != nullptr)
            *out = value;
    }

    std::uint64_t position_locked() const
    {
        if (state_ != movie_state::playing)
            return base_ms_;
        const auto elapsed = std::chrono::duration_cast<std::chrono::milliseconds>(
            wall_clock::now() - started_);
        const double rate = play_rate_ > 0.0 ? play_rate_ : 1.0;
        return base_ms_ + static_cast<std::uint64_t>(static_cast<double>(elapsed.count()) * rate);
    }

    // Registers a fresh request. The caller holds `mutex_` and calls
    // report_request() after releasing it: the hook may do storage I/O (and may
    // even complete the request synchronously).
    int start_request_locked(std::string& storage, std::string& local,
                             std::shared_ptr<movie_bridge>& out_bridge)
    {
        detach_bridge_locked();
        if (local_.empty())
            materialize_locked();
        const int serial = g_next_serial.fetch_add(1, std::memory_order_relaxed);
        serial_ = serial;
        bridge_ = std::make_shared<movie_bridge>(callback_, state_change_);
        out_bridge = bridge_;
        register_bridge(serial, bridge_);
        base_ms_ = 0;
        started_ = wall_clock::now();
        storage = storage_;
        local = local_;
        return serial;
    }

    void report_request(int serial, const std::shared_ptr<movie_bridge>& bridge,
                        const std::string& storage, const std::string& local)
    {
        if (state_change_ && bridge && Application != nullptr)
        {
            // MFEVR mode never sets ssPlay synchronously (tTJSNI_VideoOverlay only
            // follows WM_STATE_CHANGE), and a completion that arrives while the
            // status is not "play" is ignored. The report is deferred so the queue
            // clear that Play()/Rewind() perform right after this call cannot drop
            // it; the bridge keeps the target alive.
            bridge->schedule_playing(bridge);
        }
        if (TVPMovieEventHook == nullptr)
        {
            // No host: finish the request so a waiting script continues instead of
            // parking forever. Deferred by one engine turn rather than completed
            // inline: tTJSNI_VideoOverlay::Play sets its "play" status only after
            // VideoOverlay->Play() returns, and the completion handler ignores
            // anything that arrives before that.
            if (bridge && Application != nullptr)
            {
                Application->PostUserMessage([bridge, serial]() { TVPMovieNotifyFinished(serial); },
                                             nullptr, 0);
            }
            return;
        }
        // The engine's `audioVolume` is KAG's 0..100000 scale (KAG's Movie class
        // multiplies the tag's 0..100 volume by 1000). 100000 is unity, i.e. the
        // original engine initialised the player at full volume.
        const long volume = audio_volume_ < 0 ? 0 : audio_volume_;
        const double gain = static_cast<double>(volume > 100000 ? 100000 : volume) / 100000.0;
        const TVPMovieRequest request{storage.c_str(), local.c_str(), serial, gain};
        TVPMovieEventHook(request);
    }

    // Why materialize at all: the host opens the movie with its own decoder, so it
    // needs a path, and XP3 entries have none. Loose files map straight to a local
    // path; everything else is copied out into the host's cache directory by the
    // shared materialiser (environ/PlatformMediaCache.h).
    //
    // The copy deliberately outlives this overlay: the cache is keyed by storage
    // name, so a loop rewind, an "OP again" and the next play all hit it instead of
    // re-extracting a 200MB stream. The previous implementation wrote a temp copy
    // under the project directory and deleted it on destruction, which both filled
    // the game folder with leftovers on a crash and could unlink the file while the
    // host was still reading it.
    void materialize_locked()
    {
        if (!local_.empty() || storage_.empty())
            return;
        try
        {
            local_ = TVPMaterializeHostMedia(storage_, "movie");
        }
        catch (...)
        {
            // Storage/media errors must not take the script down: with an empty
            // local path the adapter materializes the storage itself.
            local_.clear();
        }
        drop_stream_locked();
    }

    void drop_stream_locked()
    {
        delete stream_;
        stream_ = nullptr;
    }

    void detach_bridge_locked()
    {
        if (!bridge_)
            return;
        unregister_bridge(serial_, bridge_);
        {
            std::lock_guard<std::mutex> lock(bridge_->mutex);
            bridge_->callback = nullptr;
        }
        bridge_.reset();
        serial_ = 0;
    }

    void detach_bridge()
    {
        std::lock_guard<std::mutex> lock(mutex_);
        detach_bridge_locked();
    }

    std::atomic<int> ref_{1};
    iTVPVideoCallback* callback_; // engine-owned, never touched from host threads
    tTJSBinaryStream* stream_;    // owned until the movie is materialized
    std::string storage_;
    std::string ext_;
    const bool layer_mode_;
    const bool state_change_;
    std::string local_; // host-openable path ("" = let the adapter do it)

    std::mutex mutex_;
    movie_state state_ = movie_state::idle;
    int serial_ = 0;
    std::shared_ptr<movie_bridge> bridge_;
    wall_clock::time_point started_{};
    std::uint64_t base_ms_ = 0;
    double play_rate_ = 1.0;
    int stop_frame_ = -1;
    long audio_balance_ = 0;
    // Unity gain in the engine's 0..100000 scale. The original player starts at
    // full volume (its sound buffer's SetVolume default), and KAG only writes this
    // when the script passes `@movie volume=`. A default of 100 here made every
    // movie play at 0.1% — effectively silent.
    long audio_volume_ = 100000;
    float mixing_alpha_ = 1.0f;
    unsigned long mixing_bg_ = 0xFF000000;
    float contrast_ = 0.0f;
    float brightness_ = 0.0f;
    float hue_ = 0.0f;
    float saturation_ = 0.0f;
};

// Every engine-side factory must return a player: tTJSNI_VideoOverlay::Open
// dereferences the result before it can report an error (layer mode even throws
// on a 0x0 size), so a nullptr here would crash instead of skipping the movie.
void create_host_overlay(iTVPVideoCallback* callbackwin, tTJSBinaryStream* stream,
                         const tjs_char* streamname, const tjs_char* type,
                         iTVPVideoOverlay** out, bool layer_mode, bool state_change)
{
    if (out == nullptr)
    {
        // Factories own the stream the engine opened for them.
        delete stream;
        return;
    }
    *out = new HostMovieOverlay(callbackwin, stream, streamname, type, layer_mode, state_change);
}

} // namespace

// The job of this TU: it is the overlay implementation's translation unit, so
// the symbol the adapter links against is defined here (see PlatformMovieHook.h).
void TVPMovieNotifyFinished(int serial)
{
    std::shared_ptr<movie_bridge> bridge;
    {
        std::lock_guard<std::mutex> lock(g_bridge_mutex);
        const auto found = g_bridges.find(serial);
        if (found != g_bridges.end())
        {
            bridge = found->second;
            g_bridges.erase(found);
        }
    }
    if (!bridge)
        return;
    bridge->schedule_completion(bridge);
}

//---------------------------------------------------------------------------
void GetVideoOverlayObject(iTVPVideoCallback* callbackwin, tTJSBinaryStream* stream,
                           const tjs_char* streamname, const tjs_char* type, uint64_t,
                           iTVPVideoOverlay** out)
{
    create_host_overlay(callbackwin, stream, streamname, type, out, /*layer_mode=*/false,
                        /*state_change=*/false);
}
void GetVideoLayerObject(iTVPVideoCallback* callbackwin, tTJSBinaryStream* stream,
                         const tjs_char* streamname, const tjs_char* type, uint64_t,
                         iTVPVideoOverlay** out)
{
    create_host_overlay(callbackwin, stream, streamname, type, out, /*layer_mode=*/true,
                        /*state_change=*/false);
}
void GetMixingVideoOverlayObject(iTVPVideoCallback* callbackwin, tTJSBinaryStream* stream,
                                 const tjs_char* streamname, const tjs_char* type, uint64_t,
                                 iTVPVideoOverlay** out)
{
    create_host_overlay(callbackwin, stream, streamname, type, out, /*layer_mode=*/false,
                        /*state_change=*/false);
}
// MFEVR mode never uses the synchronous ssPlay status: tTJSNI_VideoOverlay only
// leaves it on WM_STATE_CHANGE, so this overlay reports completion that way.
void GetMFVideoOverlayObject(iTVPVideoCallback* callbackwin, tTJSBinaryStream* stream,
                             const tjs_char* streamname, const tjs_char* type, uint64_t,
                             iTVPVideoOverlay** out)
{
    create_host_overlay(callbackwin, stream, streamname, type, out, /*layer_mode=*/false,
                        /*state_change=*/true);
}

// Archive formats are implemented by plugins/Kirikiroid2/{TAR,ZIP,7z}Archive.cpp
// in this build, so no null factories are needed here (they would collide with
// the real TVPOpen*Archive symbols).

#if defined(_KRKRSDL3_ANDROID) || defined(_KRKRSDL3_WINDOWS)
// Only the FFmpeg krmovie.cpp factory helpers call these, and that file is not
// compiled here; the engine reaches the host player through the Get*Object
// functions above instead.
LayerVideoPlayer* CreateLayerVideoPlayer(TVPVideoEventCallback, void*) { return nullptr; }
OverlayVideoPlayer* CreateOverlayVideoPlayer(TVPVideoEventCallback, void*) { return nullptr; }
#endif
