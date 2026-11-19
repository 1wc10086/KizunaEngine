// fvp (libmdk + FFmpeg) audio backend — the fallback path.
//
// Why a second decoder exists at all: flutter_soloud uses miniaudio's built-in
// decoders (WAV, MP3, FLAC, Vorbis and, on some platforms, the OS decoder).
// KiriKiri titles regularly ship TwinVQ (.vqf), older Opus streams and unusual
// MPEG audio variants that none of those handle. fvp already lives in this
// process for video, and its FFmpeg demuxer/decoder set covers those files, so
// the same dependency serves both media types instead of adding a third native
// library.
//
// It is used with libmdk's *player* API rather than through `video_player`:
// audio has no frames to present, and going through the video plugin would
// create a texture, a platform view and an audio-focus request per voice. The
// player API is exactly the subset this file needs: media, prepare, play/pause,
// volume, loop, position, end-of-stream.
import 'dart:async';

import 'package:fvp/mdk.dart' as mdk;

import 'audio_backend.dart';

/// One libmdk player instance.
///
/// A player is per-voice on purpose: libmdk's loop count and state belong to the
/// player, and games start/stop individual voices on their own schedule. It is
/// also the only way to track completion — the mdk state machine reports "end"
/// for this specific stream.
final class _FvpVoice implements AudioVoice {
  _FvpVoice({
    required this.path,
    required this.player,
    required this.duration,
    required this.loop,
    required this.volume,
    required this.paused,
  });

  @override
  final String path;

  final mdk.Player player;

  /// Stream length in milliseconds, captured once at start.
  ///
  /// `Player.mediaInfo` builds a fresh Dart object tree (including metadata maps)
  /// on every read, so polling it per voice would allocate far more than the
  /// completion check is worth.
  final int duration;

  @override
  bool loop;

  @override
  double volume;

  @override
  bool paused;

  @override
  bool disposed = false;
}

class FvpAudioBackend implements AudioBackend {
  FvpAudioBackend();

  /// Preparing a corrupt or unsupported container must not hang the caller: mdk
  /// probes the stream synchronously on a worker thread, and an unparsable file
  /// can leave the prepared callback pending forever.
  static const Duration _prepareTimeout = Duration(seconds: 10);

  bool _ready = false;
  bool _closed = false;
  final Set<_FvpVoice> _voices = <_FvpVoice>{};

  @override
  String get name => 'fvp';

  @override
  bool get ready => _ready;

  @override
  Future<void> init() async {
    if (_ready || _closed) {
      return;
    }
    try {
      // `mdk.version()` is a pure native query: it proves libmdk is loaded
      // without allocating a player, a callback port or a texture.
      mdk.version();
      _ready = true;
    } catch (_) {
      // No libmdk in this build (a host that stripped the plugin, or web): the
      // backend stays unusable and the orchestrator simply has less coverage.
      _ready = false;
    }
  }

  @override
  Future<void> deinit() async {
    if (_closed) {
      return;
    }
    _closed = true;
    _ready = false;
    final voices = _voices.toList();
    _voices.clear();
    for (final voice in voices) {
      await _dispose(voice);
    }
  }

  @override
  Future<AudioVoice?> start(
    String path, {
    required double volume,
    required bool loop,
    required bool stream,
    required bool paused,
  }) async {
    if (!_ready || _closed) {
      return null;
    }
    mdk.Player? player;
    try {
      player = mdk.Player();
      player.media = path;
      // A negative loop count is mdk's "infinite". The engine repeats 0 for a
      // one-shot, which is also mdk's default.
      player.loop = loop ? -1 : 0;
      player.volume = _clamp(volume);
      final result = await player.prepare().timeout(_prepareTimeout);
      // mdk's prepare() contract: 0 = prepared, -1 = already loading/loaded
      // (also fine for us), any other negative value is a failure.
      if (result != 0 && result != -1) {
        await player.disposeAsync();
        return null;
      }
      // `prepare()` returning 0 only means "the callback ran": a container with
      // no streams is invalid media that mdk still reports as prepared.
      if (player.mediaInfo.streams <= 0) {
        await player.disposeAsync();
        return null;
      }
      // Start paused when the engine asked for it: a voice that starts playing
      // and is paused a moment later makes an audible blip.
      player.state = paused ? mdk.PlaybackState.paused : mdk.PlaybackState.playing;
      final voice = _FvpVoice(
        path: path,
        player: player,
        duration: player.mediaInfo.duration,
        loop: loop,
        volume: _clamp(volume),
        paused: paused,
      );
      _voices.add(voice);
      return voice;
    } catch (_) {
      if (player != null) {
        try {
          await player.disposeAsync();
        } catch (_) {}
      }
      return null;
    }
  }

  @override
  Future<void> stop(AudioVoice voice) async {
    if (voice is! _FvpVoice || voice.disposed) {
      return;
    }
    _voices.remove(voice);
    await _dispose(voice);
  }

  @override
  void setVolume(AudioVoice voice, double volume) {
    if (voice is! _FvpVoice || voice.disposed) {
      return;
    }
    voice.volume = _clamp(volume);
    try {
      voice.player.volume = voice.volume;
    } catch (_) {}
  }

  @override
  void setPaused(AudioVoice voice, bool paused) {
    if (voice is! _FvpVoice || voice.disposed) {
      return;
    }
    voice.paused = paused;
    try {
      voice.player.state =
          paused ? mdk.PlaybackState.paused : mdk.PlaybackState.playing;
    } catch (_) {}
  }

  @override
  Future<bool> finished(AudioVoice voice) async {
    if (voice is! _FvpVoice || voice.disposed) {
      return true;
    }
    if (voice.loop || voice.paused) {
      return false;
    }
    try {
      final status = voice.player.mediaStatus;
      if (status.test(mdk.MediaStatus.end) || status.test(mdk.MediaStatus.invalid)) {
        return true;
      }
      // EOS can be missed when the last frame is dropped; fall back to the
      // stream clock. 250ms of slack covers decoder latency without cutting a
      // line early.
      if (voice.duration > 0 && voice.player.position >= voice.duration - 250) {
        return true;
      }
      return voice.player.state == mdk.PlaybackState.stopped;
    } catch (_) {
      return true;
    }
  }

  @override
  Future<void> disposeAll() async {
    final voices = _voices.toList();
    _voices.clear();
    for (final voice in voices) {
      await _dispose(voice);
    }
  }

  Future<void> _dispose(_FvpVoice voice) async {
    voice.disposed = true;
    try {
      voice.player.state = mdk.PlaybackState.stopped;
    } catch (_) {}
    try {
      await voice.player.disposeAsync();
    } catch (_) {}
  }

  static double _clamp(double value) {
    if (!value.isFinite) {
      return 0;
    }
    return value.clamp(0, 1).toDouble();
  }
}
