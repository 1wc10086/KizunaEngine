#pragma once
// Host-delegated movie playback hook.
//
// Why: cross-platform video playback is delegated to the Flutter host (fvp =
// libmdk + ffmpeg). The engine never decodes video in this build (core/media/movie
// is not compiled); it only reports "play this movie" and waits for the host to
// report completion, exactly like the audio hook (PlatformAudioHook.h) does for
// sound. This keeps one playback pipeline on every platform instead of one
// decoder stack per OS.
//
// Flow:
//   1. the engine's VideoOverlay implementation materializes the movie out of
//      XP3 storage and fires TVPMovieEventHook;
//   2. the adapter (third_party/src/krkr.cpp) surfaces it as a movie_request to
//      the host through the wire protocol;
//   3. the host plays it (Flutter MovieLayer / fvp) and sends the completion op,
//      which lands here as TVPMovieNotifyFinished(serial);
//   4. the overlay completes and the script waiting on the movie resumes.
//
// Serial ownership: the overlay issues the serial (process-wide increasing) and
// puts it in TVPMovieRequest::serial. The adapter quotes the same number back,
// so completions are matched exactly; an unknown serial is treated as stale and
// dropped rather than resuming whichever request happens to be outstanding.
//
// Threading: TVPMovieNotifyFinished may be called from any thread, but the
// notification is marshalled onto the engine thread (the overlay, its TJS state
// and its event queue all live there). It may only be called while the engine is
// open: the adapter clears TVPMovieEventHook and stops calling it before
// SDL_AppQuit tears the application down.
//
// Completion is idempotent and safe after the fact: the first notification for
// a request wins, later or post-teardown ones are no-ops, so the host never has
// to know whether the script is still waiting.
//
// No hook installed (host-less tools, engine tests): the overlay finishes the
// request on the next engine turn instead of waiting for a completion that
// cannot arrive.

struct TVPMovieRequest
{
    const char* storage; // storage name as the engine sees it, e.g. "video/op.mpg"
    const char* local;   // host-accessible file path (materialized copy); may be ""
    int serial;          // unique per request; the completion should quote it
    // Gain from the overlay's `audioVolume` property, already normalised to 0..1.
    // KAG's `@movie volume=` maps to it; the host applies it to the movie's audio
    // track so a quiet OP does not play at full scale.
    double audio_gain;
};

typedef void (*TVPMovieEventCallback)(const TVPMovieRequest& request);

// Definition lives with the overlay implementation (plugins/kizuna_stubs.cpp).
// Inline variable so neither side owns a translation unit for the symbol.
inline TVPMovieEventCallback TVPMovieEventHook = nullptr;

// Called by the host bridge when playback of `serial` finished (or failed).
// Completes the matching overlay and resumes the script waiting on it. Safe to
// call when no matching request exists (no-op).
void TVPMovieNotifyFinished(int serial);
