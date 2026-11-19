// Audio orchestration for engine events.
//
// The engine owns the *timeline* (what plays when, loop points, fades, pause
// state) and reports it as a snapshot per channel. This class turns those
// snapshots into actual sound and reports back when a one-shot has played to its
// end, so a script waiting on it can continue.
//
// Two decoders, tried in order per request:
//   1. flutter_soloud — low latency, mixes many voices, available on all five
//      shipping platforms. Preferred for everything it can open.
//   2. fvp (libmdk + FFmpeg) — covers the containers SoLoud's bundled decoders
//      reject: TwinVQ (.vqf), some Opus/Vorbis variants, MPEG audio. The same
//      plugin is already in the process for video.
//
// The split is decided by *result*, never by file extension. A `.ogg` SoLoud
// cannot decode must still play, and a `.wav` that is really an MPEG stream must
// not be locked into one path by its name. A path both decoders reject is
// remembered: a script can retry a broken line dozens of times (replaying a voice,
// a looping SE on a missing file) and each attempt costs a container probe.
import 'dart:async';
import 'dart:io';

import '../bridge/engine_bridge.dart';
import '../bridge/protocol_consts.dart';
import 'audio_backend.dart';
import 'fvp_audio.dart';
import 'soloud_backend.dart';

typedef AudioCompletedCallback = void Function(
    String file, bool voice, int channel);

/// `KzField.f3` sentinel for "this event does not address a channel".
const int _noTarget = 0xFFFFFFFF;

/// Voice state is polled on this interval.
///
/// One decoder round-trip per voice per tick would be wasteful, and 120ms is fast
/// enough that a one-shot's completion reaches the script before a player notices
/// while costing almost no isolate time.
const Duration _pollInterval = Duration(milliseconds: 120);

/// Bounded set of paths both decoders rejected, so a pathological script cannot
/// grow it without limit. Once full, failing paths are retried instead.
const int _maxRejected = 2048;

/// How many un-addressed one-shot effects may ring at once.
const int _maxUnaddressedEffects = 16;

/// Which fallback slot a channel-less event addresses.
///
/// The krkr port always sets a channel, but the Go engine's older brands did not,
/// and the same Dart code serves both. Keeping the distinction here means a
/// channel-less `stopBgm` cannot silence a voice line.
enum _Slot { bgm, voice, effect }

/// One playing (or desired) voice.
final class _Voice {
  _Voice({
    required this.file,
    required this.channel,
    required this.loop,
    required this.voice,
    required this.gain,
    required this.eventVolume,
  });

  /// Host-openable path handed to the decoder.
  final String file;

  /// Engine channel, or [_noTarget] when the event did not address one. Set by
  /// [_attach] once the key is known; the completion callback quotes it back.
  int channel;

  /// True for spoken lines, which the engine may also play as a sequence.
  final bool voice;

  bool loop;
  /// Gain actually applied to the decoder.
  double gain;
  /// Volume as the engine reported it, before the host's music attenuation.
  double eventVolume;
  bool paused = false;

  /// Set once a decoder accepted the file.
  AudioVoice? playback;
  AudioBackend? backend;

  /// Guards [AudioCompletedCallback] against being fired twice for one voice.
  bool silent = false;
}

/// What a channel should be doing, built from one engine event before a decoder
/// has been asked to open it.
final class _Desired {
  const _Desired({
    required this.file,
    required this.gain,
    required this.eventVolume,
    required this.loop,
    required this.voice,
    required this.stream,
  });

  final String file;
  /// Gain the decoder starts with (volume after the host's music attenuation).
  final double gain;
  /// Volume as the engine reported it, kept so a later attenuation event can
  /// recompute the gain instead of compounding two multiplications.
  final double eventVolume;
  final bool loop;
  final bool voice;

  /// Ask the decoder to stream from disk instead of decoding everything into
  /// memory. Only for looping music: a 3-minute track is tens of megabytes of PCM
  /// and a session has dozens of them, which a phone cannot hold.
  final bool stream;
}

class GameAudio {
  GameAudio({
    AudioBackend? primary,
    AudioBackend? fallback,
  }) : _backends = <AudioBackend>[
          primary ?? SoloudBackend(),
          fallback ?? FvpAudioBackend(),
        ];

  /// Order matters: the first backend that opens a file serves it. A platform
  /// missing one of the native libraries falls through to the other without a
  /// separate code path.
  final List<AudioBackend> _backends;

  AudioCompletedCallback? onCompleted;

  /// Engine channel -> voice.
  ///
  /// KAG addresses sound by buffer, not by kind: BGM uses two buffers for
  /// cross-fades, SE several at once, and each buffer has its own channel. Mirroring
  /// that identity is what makes a fade of one buffer leave the other alone.
  final Map<int, _Voice> _channels = <int, _Voice>{};

  /// Events without a channel fall back to one slot per kind. The krkr port always
  /// sets one, but the Go engine's brands historically did not, and the same Dart
  /// code serves both.
  _Voice? _bgm;
  _Voice? _voice;
  final List<_Voice> _effects = <_Voice>[];

  /// `KzAudioKind.voiceSequence`: a list played in order, optionally looping,
  /// which KAG uses for lip-sync and chant lines.
  List<String> _sequence = const <String>[];
  bool _sequenceLoop = false;
  int _sequenceIndex = -1;
  double _sequenceGain = 1;
  int _sequenceChannel = _noTarget;

  final Set<String> _rejected = <String>{};
  Timer? _watchdog;
  bool _polling = false;
  bool _initialized = false;
  bool _closed = false;

  /// Multiplicative music attenuation reported by the Go engine's brands
  /// (`KzAudioKind.volume` without a channel). Those brands send the factor once
  /// and expect every later BGM event to be scaled by it; the krkr core instead
  /// sends fully mixed gains per channel, so this stays at 1.0 there.
  double _engineMusicGain = 1;

  /// Serialises event application. Two events for the same channel in quick
  /// succession (play then stop, or a cross-fade) must not interleave their
  /// asynchronous decoder setup: opening a file can take hundreds of milliseconds,
  /// and "stop" overtaking "play" leaves a voice nothing can stop.
  Future<void> _tail = Future<void>.value();

  Future<void> init() async {
    if (_initialized || _closed) {
      return;
    }
    _initialized = true;
    for (final backend in _backends) {
      // Deinit can run while a cold start is still awaiting the decoder: stop
      // initialising rather than leaving a backend open after teardown.
      if (_closed) {
        return;
      }
      await backend.init();
    }
    if (_closed) {
      return;
    }
    _watchdog ??= Timer.periodic(_pollInterval, (_) => _poll());
  }

  Future<void> deinit() async {
    if (_closed) {
      return;
    }
    _closed = true;
    _watchdog?.cancel();
    _watchdog = null;
    for (final voice in _allVoices()) {
      await _release(voice);
    }
    _channels.clear();
    _bgm = null;
    _voice = null;
    _effects.clear();
    _sequence = const <String>[];
    _sequenceIndex = -1;
    for (final backend in _backends) {
      await backend.deinit();
    }
  }

  /// Stops every voice but keeps the decoders initialised.
  ///
  /// Called when the player page is left: the global audio instance outlives the
  /// page (SoLoud init and decode caches are expensive), but the previous game's
  /// BGM must not keep ringing over the library screen.
  Future<void> stopAll() => _stopAll();

  /// Applies one engine event, in engine order.
  Future<void> dispatchEvent(AudioEvent event) {
    final next = _tail.then((_) async {
      if (!_closed) {
        await _apply(event);
      }
    });
    _tail = next.catchError((_) {});
    return next;
  }

  Future<void> _apply(AudioEvent event) async {
    switch (event.kind) {
      case KzAudioKind.bgm:
        await _start(event, slot: _Slot.bgm);
      case KzAudioKind.se:
        await _start(event, slot: _Slot.effect);
      case KzAudioKind.voice:
        _clearSequence();
        await _start(event, slot: _Slot.voice);
      case KzAudioKind.voiceSequence:
        await _startSequence(event);
      case KzAudioKind.stopBgm:
        await _stopSlot(_keyOf(event), _Slot.bgm);
      case KzAudioKind.stopSe:
        await _stopSlot(_keyOf(event), _Slot.effect);
      case KzAudioKind.stopVoice:
        _clearSequence();
        await _stopSlot(_keyOf(event), _Slot.voice);
      case KzAudioKind.stopAll:
        await _stopAll();
      case KzAudioKind.volume:
        _setGain(event);
      case KzAudioKind.pause:
        _setPaused(event, true);
      case KzAudioKind.resume:
        _setPaused(event, false);
      case KzAudioKind.fadeOut:
        _fadeOut(event);
    }
  }

  Future<void> _start(AudioEvent event, {required _Slot slot}) async {
    if (event.path.isEmpty) {
      return;
    }
    final voice = slot == _Slot.voice;
    final key = _keyOf(event);
    final existing = _lookup(key, slot);
    // Same file on the same channel: an update, not a restart. This happens on
    // every step of a KAG fade (the engine reports the new gain on the same
    // buffer); restarting there would rewind the track continuously.
    if (existing != null && existing.file == event.path && existing.loop == event.loop) {
      existing.eventVolume = _clamp(event.volume);
      existing.gain = _mixGain(slot, existing.eventVolume, key);
      _applyGain(existing);
      return;
    }
    // A channel-less one-shot effect must be able to overlap the previous one
    // (footsteps under a door slam); every other start replaces its slot.
    if (key != null || slot != _Slot.effect) {
      await _stopSlot(key, slot);
    }
    final eventVolume = _clamp(event.volume);
    final started = await _open(
      _Desired(
        file: event.path,
        gain: _mixGain(slot, eventVolume, key),
        eventVolume: eventVolume,
        loop: event.loop,
        voice: voice,
        stream: event.loop && !voice,
      ),
      paused: event.paused,
    );
    if (_closed) {
      await _release(started);
      return;
    }
    if (started == null && !event.loop) {
      // Neither decoder could open the file. Tell the engine the sound is over so
      // a script waiting on it continues; the engine's own silent timeline would
      // otherwise keep that wait alive for its whole cushion.
      _emitCompletedKey(key, event.path, voice);
      return;
    }
    _attach(key, started, slot: slot);
  }

  Future<void> _startSequence(AudioEvent event) async {
    final names = event.names.where((name) => name.isNotEmpty).toList();
    if (names.isEmpty) {
      return;
    }
    final key = _keyOf(event);
    await _stopSlot(key, _Slot.voice);
    _sequence = names;
    _sequenceLoop = event.loop;
    _sequenceGain = _clamp(event.volume);
    _sequenceIndex = 0;
    _sequenceChannel = key ?? _noTarget;
    final started = await _open(
      _Desired(
        file: names.first,
        gain: _sequenceGain,
        eventVolume: _sequenceGain,
        loop: false,
        voice: true,
        stream: false,
      ),
      paused: false,
    );
    if (_closed) {
      await _release(started);
      return;
    }
    _attach(key, started, slot: _Slot.voice);
  }

  Future<void> _stopAll() async {
    _clearSequence();
    final voices = _allVoices();
    _channels.clear();
    _bgm = null;
    _voice = null;
    _effects.clear();
    for (final voice in voices) {
      await _release(voice);
    }
  }

  void _setGain(AudioEvent event) {
    final key = _keyOf(event);
    final gain = _clamp(event.volume);
    if (key == null) {
      // Channel-less volume is the Go brands' "music attenuation": a factor stored
      // for later BGM events, not a gain on one voice.
      _engineMusicGain = gain;
      final bgm = _bgm;
      if (bgm != null) {
        bgm.gain = _clamp(bgm.eventVolume * _engineMusicGain);
        _applyGain(bgm);
      }
      return;
    }
    final voice = _channels[key];
    if (voice == null) {
      return;
    }
    // The krkr core sends an absolute mixed gain (KAG already multiplied the track
    // volume into it), so this replaces the value instead of compounding it.
    voice.gain = gain;
    voice.eventVolume = gain;
    _applyGain(voice);
  }

  void _applyGain(_Voice voice) {
    final playback = voice.playback;
    final backend = voice.backend;
    if (playback != null && backend != null) {
      backend.setVolume(playback, voice.gain);
    }
  }

  /// Gain for a starting voice.
  ///
  /// A channel-addressed gain is already fully mixed by the krkr core. A
  /// channel-less BGM belongs to a Go brand, whose music attenuation has to be
  /// applied here.
  double _mixGain(_Slot slot, double volume, int? key) {
    if (key != null || slot != _Slot.bgm) {
      return volume;
    }
    return _clamp(volume * _engineMusicGain);
  }

  void _setPaused(AudioEvent event, bool paused) {
    final key = _keyOf(event);
    // Pause can address a voice line too (KAG pauses the buffer it is replaying
    // from the backlog), so a channel-addressed event must find any kind.
    final voice = key != null ? _channels[key] : _lookup(key, _Slot.bgm);
    if (voice == null) {
      return;
    }
    voice.paused = paused;
    final playback = voice.playback;
    final backend = voice.backend;
    if (playback != null && backend != null) {
      backend.setPaused(playback, paused);
    }
    if (!paused) {
      // Releasing a pause must restore the gain that was computed while paused:
      // KAG keeps changing volumes during a pause, and a paused-start event also
      // carried a gain the sample never applied.
      _applyGain(voice);
    }
  }

  /// Starts a fade-out.
  ///
  /// Deliberately not awaited: a fade lasts as long as its KAG `time=` (seconds),
  /// and holding the event queue for that long would freeze every later sound —
  /// including the next BGM the script starts right after the fade — and delay
  /// the text the player is waiting to read. The fade runs on its own future and
  /// only touches the one voice it was started for; replacing that voice
  /// invalidates the loop through [voice.playback].
  void _fadeOut(AudioEvent event) {
    final key = _keyOf(event);
    final voice = _lookup(key, _Slot.bgm);
    if (voice == null) {
      return;
    }
    // 0 frames means "stop now": KAG uses it for an instant cut.
    if (event.frames <= 0) {
      unawaited(_stopSlot(key, _Slot.bgm));
      return;
    }
    final playback = voice.playback;
    final backend = voice.backend;
    if (playback == null || backend == null) {
      return;
    }
    // KAG counts in 60fps engine frames; the clamp keeps a bogus value from
    // parking a fade for minutes.
    final millis = (event.frames * 1000 / 60).round().clamp(1, 60000);
    // Neither backend exposes a native fade, so the gain is stepped in Dart. 50ms
    // steps sound smooth and keep the audible curve in step with the engine's
    // timeline; the final stop happens here rather than waiting for the engine to
    // notice, because a faded-out voice must not linger in the mix.
    final steps = (millis / 50).ceil().clamp(1, 400);
    final stepMs = (millis / steps).round().clamp(1, 1000);
    final from = voice.gain;
    unawaited(() async {
      for (var i = 1; i <= steps; i++) {
        // The voice was replaced or torn down (a new BGM started, the page was
        // left): stop stepping, the new owner owns the gain now.
        if (_closed || !identical(voice.playback, playback)) {
          return;
        }
        final value = from * (1 - i / steps);
        voice.gain = value;
        backend.setVolume(playback, value);
        await Future<void>.delayed(Duration(milliseconds: stepMs));
      }
      if (_closed || !identical(voice.playback, playback)) {
        return;
      }
      voice.gain = 0;
      await _stopSlot(key, _Slot.bgm);
    }());
  }

  /// Opens [desired] on the first backend that accepts it.
  Future<_Voice?> _open(_Desired desired, {required bool paused}) async {
    if (_closed || _rejected.contains(desired.file)) {
      return null;
    }
    // A path that is not a real file can never open. Checking here keeps a retry
    // loop from paying a container probe for a file the engine already failed to
    // materialise.
    if (!File(desired.file).existsSync()) {
      return null;
    }
    var anyReady = false;
    for (final backend in _backends) {
      if (!backend.ready) {
        continue;
      }
      anyReady = true;
      final playback = await backend.start(
        desired.file,
        volume: desired.gain,
        loop: desired.loop,
        stream: desired.stream,
        paused: paused,
      );
      if (_closed) {
        if (playback != null) {
          await backend.stop(playback);
        }
        return null;
      }
      if (playback == null) {
        continue;
      }
      return _Voice(
        file: desired.file,
        channel: _noTarget,
        loop: desired.loop,
        voice: desired.voice,
        gain: desired.gain,
        eventVolume: desired.eventVolume,
      )
        ..paused = paused
        ..playback = playback
        ..backend = backend;
    }
    // Only remember the failure when a decoder actually tried. During the first
    // few hundred milliseconds of a game the native libraries may still be
    // initialising; treating that as "unplayable" would permanently silence every
    // sample the opening scene starts.
    if (anyReady && _rejected.length < _maxRejected) {
      _rejected.add(desired.file);
    }
    return null;
  }

  Future<void> _release(_Voice? voice) async {
    if (voice == null) {
      return;
    }
    final playback = voice.playback;
    final backend = voice.backend;
    voice.playback = null;
    voice.backend = null;
    if (playback == null || backend == null) {
      return;
    }
    try {
      await backend.stop(playback);
    } catch (_) {}
  }

  /// Stops whatever the event addresses.
  ///
  /// A channel-addressed stop is unambiguous. A channel-less one is not: stopBgm
  /// must not cut a voice line, so the event kind decides which fallback slot is
  /// freed.
  Future<void> _stopSlot(int? key, _Slot slot) async {
    if (key != null) {
      final voice = _channels.remove(key);
      if (voice != null) {
        await _release(voice);
      }
      return;
    }

    switch (slot) {
      case _Slot.bgm:
        final bgm = _bgm;
        _bgm = null;
        await _release(bgm);
      case _Slot.voice:
        final line = _voice;
        _voice = null;
        await _release(line);
      case _Slot.effect:
        final effects = _effects.toList();
        _effects.clear();
        for (final effect in effects) {
          await _release(effect);
        }
    }
  }

  // --- polling ------------------------------------------------------------

  void _poll() {
    if (_polling || _closed) {
      return;
    }
    _polling = true;
    _pollOnce().whenComplete(() => _polling = false);
  }

  Future<void> _pollOnce() async {
    for (final entry in _channels.entries.toList()) {
      await _pollVoice(entry.key, entry.value);
      if (_closed) {
        return;
      }
    }
    final loose = <_Voice>[
      if (_bgm != null) _bgm!,
      if (_voice != null) _voice!,
      ..._effects,
    ];
    for (final voice in loose) {
      await _pollVoice(null, voice);
      if (_closed) {
        return;
      }
    }
  }

  Future<void> _pollVoice(int? key, _Voice voice) async {
    final playback = voice.playback;
    final backend = voice.backend;
    if (playback == null || backend == null) {
      return;
    }
    final done = await backend.finished(playback);
    if (_closed || !done) {
      return;
    }
    if (key != null) {
      if (!identical(_channels[key], voice)) {
        return; // already replaced by a newer voice on this channel
      }
      _channels.remove(key);
    } else {
      if (identical(_bgm, voice)) {
        _bgm = null;
      }
      if (identical(_voice, voice)) {
        _voice = null;
      }
      _effects.remove(voice);
    }
    await _release(voice);
    _emitCompleted(voice);
    // A finished line advances the sequence it belongs to; KAG's chant/lip-sync
    // lists rely on each element playing exactly once, in order.
    if (voice.voice && _sequenceIndex >= 0) {
      await _advanceSequence();
    }
  }

  Future<void> _advanceSequence() async {
    if (_sequence.isEmpty) {
      return;
    }
    // Bound the walk to one pass over the list: with `loop` on and every element
    // unplayable, an unbounded retry would spin here forever. One full pass without
    // a start means the whole sequence is broken, so it is dropped.
    var attempted = 0;
    while (attempted <= _sequence.length) {
      _sequenceIndex++;
      if (_sequenceIndex >= _sequence.length) {
        if (!_sequenceLoop) {
          _clearSequence();
          return;
        }
        _sequenceIndex = 0;
      }
      attempted++;
      final name = _sequence[_sequenceIndex];
      if (name.isEmpty) {
        continue;
      }
      final started = await _open(
        _Desired(
          file: name,
          gain: _sequenceGain,
          eventVolume: _sequenceGain,
          loop: false,
          voice: true,
          stream: false,
        ),
        paused: false,
      );
      if (_closed) {
        await _release(started);
        return;
      }
      if (started == null) {
        continue; // skip an unplayable element instead of stalling the sequence
      }
      final key = _sequenceChannel == _noTarget ? null : _sequenceChannel;
      _attach(key, started, slot: _Slot.voice);
      return;
    }
    _clearSequence();
  }

  /// Reports completion for a channel whose voice never started. Same contract as
  /// [_emitCompleted], for the "file is unplayable" path.
  void _emitCompletedKey(int? key, String file, bool voice) {
    final callback = onCompleted;
    if (callback == null || file.isEmpty) {
      return;
    }
    try {
      callback(file, voice, key ?? _noTarget);
    } catch (_) {}
  }

  void _emitCompleted(_Voice voice) {
    if (voice.silent || voice.file.isEmpty) {
      return;
    }
    voice.silent = true;
    final callback = onCompleted;
    if (callback == null) {
      return;
    }
    try {
      callback(voice.file, voice.voice, voice.channel);
    } catch (_) {}
  }

  // --- plumbing -----------------------------------------------------------

  void _clearSequence() {
    _sequence = const <String>[];
    _sequenceIndex = -1;
  }

  int? _keyOf(AudioEvent event) {
    if (event.channel != _noTarget) {
      return event.channel;
    }
    if (event.group != _noTarget) {
      return 0x10000 + event.group;
    }
    return null;
  }

  /// Finds the voice a non-starting event addresses.
  ///
  /// The slot kind only matters for channel-less events (the Go engine's brands):
  /// the krkr port always addresses a channel, so a BGM event can never touch a
  /// voice line there.
  _Voice? _lookup(int? key, _Slot slot) {
    if (key != null) {
      return _channels[key];
    }
    return switch (slot) {
      _Slot.bgm => _bgm,
      _Slot.voice => _voice,
      _Slot.effect => _effects.isEmpty ? null : _effects.last,
    };
  }

  void _attach(int? key, _Voice? started, {required _Slot slot}) {
    if (started == null) {
      return;
    }
    if (key != null) {
      started.channel = key;
      _channels[key] = started;
      return;
    }
    if (slot == _Slot.voice) {
      _voice = started;
      return;
    }
    if (slot == _Slot.bgm || started.loop) {
      _bgm = started;
      return;
    }
    // Un-addressed one-shot effects keep a short ring so several can overlap (a
    // door slam over footsteps) while each is still polled for completion.
    _effects.add(started);
    if (_effects.length > _maxUnaddressedEffects) {
      final oldest = _effects.removeAt(0);
      unawaited(_release(oldest));
    }
  }

  List<_Voice> _allVoices() => <_Voice>[
        ..._channels.values,
        if (_bgm != null) _bgm!,
        if (_voice != null) _voice!,
        ..._effects,
      ];

  static double _clamp(double value) {
    if (!value.isFinite) {
      return 0;
    }
    return value.clamp(0, 1).toDouble();
  }
}
