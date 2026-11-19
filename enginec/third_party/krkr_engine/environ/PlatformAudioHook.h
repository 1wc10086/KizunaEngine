#pragma once
// Host hook for sound playback.
//
// The engine keeps the WaveSoundBuffer state machine but this build has no PCM
// output of its own: SDL runs with the dummy audio driver and the decoders that
// would feed it (wuffmpeg/wuopus/wuvorbis) are deliberately not compiled. Actual
// playback is delegated to the Flutter host (flutter_soloud, with fvp as the
// fallback for containers SoLoud cannot decode).
//
// This header is the *only* contract between the two sides. The hook reports a
// full snapshot of one buffer (storage, mixed gain, loop flag, pause flag) rather
// than "play" / "stop" edges. The host keeps one voice per channel and ignores a
// snapshot identical to the last one, so every transition below reaches it:
//
//   * Play()             -> start the sample
//   * SetVolume()/fades  -> follow KAG's fade and the volume sliders
//   * paused = true      -> stop the decoder (a paused KAG buffer must go silent)
//   * Stop()             -> stop
//   * natural end        -> stop (the host may already have finished; it is then
//                           an idempotent no-op)
//
// Reporting only on Play/Stop was the reason fades and paused voices behaved
// differently from the original engine.
//
// Containers the engine has no decoder for still produce snapshots: the wave
// decoder factory's lowest-priority fallback
// (core/media/sound/HostSilentWaveDecoder.h) satisfies Open with a silent stream
// so the script reaches Play/Stop instead of dying on an unknown wave format.
// The engine's own timeline (labels, loop points, "stop" status events) is driven
// by that silent stream, which is why the host does not have to report back
// except for one case: see TVPAudioHostCompleted.
//
// Threading: the engine calls the hook on its script thread while holding the
// buffer's critical section. The callback must never throw and must not call back
// into the engine; the adapter only copies the fields and appends to a
// mutex-protected queue.

struct TVPAudioEvent
{
    // Storage name as the script wrote it ("bgm/title.ogg"), or the host-facing
    // path when the buffer was opened through one. Empty only for a buffer that
    // was never opened.
    const char* storage;
    // Mixed gain the engine would have applied: Volume * Volume2 * GlobalVolume,
    // already 0..1. Passing the mixed value keeps the host from having to
    // re-implement KAG's three-stage volume model.
    double volume;
    bool loop;
    // Identity of the buffer for the lifetime of the process. The host keeps one
    // voice slot per channel, so two buffers never fight over one sound.
    int channel;
    // True once the buffer is no longer playing (Stop, natural end, unload).
    bool stop;
    // True while the buffer is paused. A paused buffer still owns its file, so the
    // host must stop the decoder but keep the slot.
    bool paused;
};
typedef void (*TVPAudioEventCallback)(const TVPAudioEvent& event);
extern TVPAudioEventCallback TVPAudioEventHook;

// Called by the host bridge (op_audio_done) when playback of `channel` finished.
//
// Why the engine needs this even though it runs its own timeline: for a storage
// whose length cannot be probed (most Ogg/Opus/VQF files) HostSilentWaveDecoder
// streams a long cushion of silence. Without this notification a script that
// waits for the buffer to end would wait for that cushion. The engine thread
// stops the matching buffer, which changes its status and fires the same
// onStatusChanged("stop") a decoded end-of-stream would have fired.
//
// Safe to call from any thread; unknown or already-finished channels are no-ops.
void TVPAudioHostCompleted(int channel);
