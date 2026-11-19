// flutter_soloud backend.
//
// SoLoud (via miniaudio) is the default decoder: it mixes many voices natively,
// has low start latency, and is available on all five shipping platforms. What it
// does *not* do is decode everything a Kiseki-era krkr title ships — TwinVQ (.vqf)
// and a few MPEG audio variants are the usual suspects — so a failed load here is
// expected and must stay cheap: the orchestrator falls through to the fvp backend
// on the same request.
import 'dart:async';

import 'package:flutter_soloud/flutter_soloud.dart';

import 'audio_backend.dart';

/// Voice backed by one SoLoud `play()` handle.
///
/// The handle is what makes a voice observable: SoLoud has no "did this specific
/// instance finish" callback for the way games use it, so a periodic poll reads
/// [SoLoud.getIsValidVoiceHandle]. That is the same mechanism the original engine
/// uses (it polls its own buffer state), and it keeps completion tied to the
/// decoder rather than to a wall-clock estimate.
final class _SoloudVoice implements AudioVoice {
  _SoloudVoice({
    required this.path,
    required this.source,
    required this.handle,
    required this.loop,
    required this.volume,
    required this.paused,
  });

  @override
  final String path;

  final AudioSource source;
  SoundHandle? handle;

  @override
  bool loop;

  @override
  double volume;

  @override
  bool paused;

  @override
  bool disposed = false;
}

class SoloudBackend implements AudioBackend {
  SoloudBackend({
    this.maxCachedSources = 24,
  });

  /// How many decoded sources stay resident.
  ///
  /// A voice's source must stay alive while it plays, but a session touches
  /// hundreds of samples: every voice line is a file of its own, and decoded PCM
  /// is 1–2MB per line. Unbounded caching is the fastest way to an OOM kill on a
  /// phone, so the least recently used *idle* entries are evicted.
  final int maxCachedSources;

  final Map<String, AudioSource> _sources = <String, AudioSource>{};
  final List<String> _order = <String>[];
  final Map<String, int> _active = <String, int>{};

  bool _ready = false;
  bool _closed = false;

  @override
  String get name => 'soloud';

  @override
  bool get ready => _ready;

  @override
  Future<void> init() async {
    if (_ready || _closed) {
      return;
    }
    try {
      await SoLoud.instance.init();
      // SoLoud defaults to 16 concurrent voices and silently drops the oldest
      // beyond that. A KAG scene can exceed it (BGM + voice + several SE), and the
      // dropped one would be the voice line the player is reading, so raise the
      // cap; the engine's per-channel identity means nothing legitimately needs
      // hundreds.
      try {
        SoLoud.instance.setMaxActiveVoiceCount(64);
      } catch (_) {}
      _ready = true;
    } catch (_) {
      // A device that refuses to open (no audio route, emulator without audio)
      // must not take the game down; the fvp backend still works.
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
    await disposeAll();
    try {
      SoLoud.instance.deinit();
    } catch (_) {}
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
    final source = await _load(path, stream: stream);
    if (source == null || _closed) {
      return null;
    }
    try {
      final handle = SoLoud.instance.play(
        source,
        volume: _clamp(volume),
        looping: loop,
        paused: paused,
      );
      // SoLoud returns a handle even when it dropped the voice (voice cap).
      // Treating that as success would make the orchestrator believe the sample
      // is playing, so verify it before committing.
      if (!SoLoud.instance.getIsValidVoiceHandle(handle)) {
        return null;
      }
      _active[path] = (_active[path] ?? 0) + 1;
      return _SoloudVoice(
        path: path,
        source: source,
        handle: handle,
        loop: loop,
        volume: _clamp(volume),
        paused: paused,
      );
    } catch (_) {
      return null;
    }
  }

  @override
  Future<void> stop(AudioVoice voice) async {
    if (voice is! _SoloudVoice || voice.disposed) {
      return;
    }
    voice.disposed = true;
    final handle = voice.handle;
    voice.handle = null;
    final count = (_active[voice.path] ?? 1) - 1;
    if (count > 0) {
      _active[voice.path] = count;
    } else {
      _active.remove(voice.path);
    }
    if (handle == null) {
      return;
    }
    try {
      await SoLoud.instance.stop(handle);
    } catch (_) {}
  }

  @override
  void setVolume(AudioVoice voice, double volume) {
    if (voice is! _SoloudVoice || voice.disposed) {
      return;
    }
    voice.volume = _clamp(volume);
    final handle = voice.handle;
    if (handle == null) {
      return;
    }
    try {
      SoLoud.instance.setVolume(handle, voice.volume);
    } catch (_) {}
  }

  @override
  void setPaused(AudioVoice voice, bool paused) {
    if (voice is! _SoloudVoice || voice.disposed) {
      return;
    }
    voice.paused = paused;
    final handle = voice.handle;
    if (handle == null) {
      return;
    }
    try {
      SoLoud.instance.setPause(handle, paused);
    } catch (_) {}
  }

  @override
  Future<bool> finished(AudioVoice voice) async {
    if (voice is! _SoloudVoice) {
      return true;
    }
    // A looping voice never completes, even if the decoder stopped: KAG expects the
    // buffer to keep its slot until the script stops it.
    if (voice.loop) {
      return false;
    }
    if (voice.disposed) {
      return true;
    }
    final handle = voice.handle;
    if (handle == null) {
      return true;
    }
    if (voice.paused) {
      return false;
    }
    try {
      return !SoLoud.instance.getIsValidVoiceHandle(handle);
    } catch (_) {
      return true;
    }
  }

  @override
  Future<void> disposeAll() async {
    for (final source in _sources.values) {
      try {
        await SoLoud.instance.disposeSource(source);
      } catch (_) {}
    }
    _sources.clear();
    _order.clear();
    _active.clear();
  }

  Future<AudioSource?> _load(String path, {required bool stream}) async {
    final cached = _sources[path];
    if (cached != null) {
      _touch(path);
      return cached;
    }
    try {
      final source = await SoLoud.instance
          .loadFile(path, mode: stream ? LoadMode.disk : LoadMode.memory);
      if (_closed) {
        try {
          await SoLoud.instance.disposeSource(source);
        } catch (_) {}
        return null;
      }
      _sources[path] = source;
      _order.add(path);
      _evict();
      return source;
    } catch (_) {
      // Disk mode can fail where memory mode succeeds (files inside an archive
      // copy that the engine rewrote under us, incomplete writes). Retry once in
      // memory before declaring the format unsupported, so a BGM does not drop a
      // whole step down to the fallback backend for a transient reason.
      if (!stream) {
        return null;
      }
      return _load(path, stream: false);
    }
  }

  void _touch(String path) {
    _order.remove(path);
    _order.add(path);
  }

  bool _inUse(String path) => (_active[path] ?? 0) > 0;

  /// Evicts idle sources in LRU order. Never evicts a source a live voice is
  /// playing — `disposeSource` stops every handle of that source, which would cut
  /// a BGM off mid-phrase.
  void _evict() {
    final overflow = _order.length - maxCachedSources;
    if (overflow <= 0) {
      return;
    }
    for (final path in _order.sublist(0, overflow)) {
      if (_inUse(path)) {
        continue;
      }
      _order.remove(path);
      final source = _sources.remove(path);
      if (source != null) {
        try {
          SoLoud.instance.disposeSource(source);
        } catch (_) {}
      }
    }
  }

  static double _clamp(double value) {
    if (!value.isFinite) {
      return 0;
    }
    return value.clamp(0, 1).toDouble();
  }
}
