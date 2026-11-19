// Audio backend contract.
//
// The player viewmodel only speaks in "voices": start a file on a channel, change
// its gain, pause it, ask whether it finished. Which library actually decodes the
// file is an implementation detail of the backend, and there are two:
//
//   * flutter_soloud — the default. Low latency, mixes many voices, native
//     android/ios/macos/windows/linux support, and it is what the engine's PCM
//     model maps onto most directly.
//   * fvp (libmdk + FFmpeg) — the fallback. SoLoud ships a fixed set of decoders
//     (wav/mp3/ogg/flac and, on some platforms, the OS decoder); krkr titles also
//     ship TwinVQ (.vqf), older Opus/Vorbis variants and MPEG containers that
//     SoLoud rejects. fvp already lives in the process for video, so it costs no
//     extra native library to use it for those samples.
//
// Keeping the interface this small is what makes the fallback safe: the
// orchestrator does not need to know which backend a voice came from.
import 'dart:async';

/// One playback instance owned by a backend.
///
/// A backend creates it in [AudioBackend.start] and the orchestrator only holds
/// the reference: it must ask the backend for everything else, because the state
/// lives next to the decoder (SoLoud handle, mdk player, video_player controller).
abstract class AudioVoice {
  String get path;
  bool get loop;
  bool get paused;
  bool get disposed;
  double get volume;
}

/// A decoder that can start, stop and observe file playback.
abstract interface class AudioBackend {
  /// Human-readable name for diagnostics ("soloud", "fvp").
  String get name;

  /// Whether the backend can be used at all (native library loaded, device open).
  bool get ready;

  /// Prepares the backend. Idempotent; never throws (an unusable backend reports
  /// `ready == false` instead, so the orchestrator can fall through to the next
  /// one).
  Future<void> init();

  /// Releases the backend. Idempotent.
  Future<void> deinit();

  /// Starts [path]. Returns null when this backend cannot decode/play the file;
  /// the orchestrator then tries its fallback.
  ///
  /// [stream] asks for disk streaming (long BGM tracks: the decoded PCM of a
  /// 3-minute track is tens of megabytes, which a phone cannot keep for every
  /// track in a session). [paused] starts the voice without producing sound,
  /// which KAG uses for `@bgm paused=true`.
  Future<AudioVoice?> start(
    String path, {
    required double volume,
    required bool loop,
    required bool stream,
    required bool paused,
  });

  /// Stops and releases the voice. Idempotent.
  Future<void> stop(AudioVoice voice);

  /// Sets the gain (0..1). Best effort: implementations swallow decoder errors.
  void setVolume(AudioVoice voice, double volume);

  /// Pauses/resumes without losing the position.
  void setPaused(AudioVoice voice, bool paused);

  /// True once a non-looping voice has played to its end (or its decoder died).
  Future<bool> finished(AudioVoice voice);

  /// Releases every cached source, for hot-restart/teardown.
  Future<void> disposeAll();
}
