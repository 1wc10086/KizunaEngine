import 'dart:async';

import 'package:flutter_soloud/flutter_soloud.dart';

import '../bridge/engine_bridge.dart';
import '../bridge/protocol_consts.dart';
import 'vqf_audio.dart';

typedef AudioCompletedCallback = void Function(
    String file, bool voice, int channel);

const int _noTarget = 0xFFFFFFFF;
const int _maxEffects = 16;
const int _maxCachedSources = 24;
const Duration _pollInterval = Duration(milliseconds: 150);

final class _Slot {
  _Slot({
    required this.file,
    required this.eventVolume,
    required this.loop,
    required this.voice,
    required this.channel,
  });

  AudioSource? source;
  SoundHandle? handle;
  // TwinVQ (.vqf) 后端：与 Soloud 互斥，一个槽只有一种后端。
  // SoLoud 解不了 VQF，所以 .vqf 走 fvp（见 vqf_audio.dart），wav/ogg 仍走 SoLoud。
  VqfHandle? vqf;
  String file;
  double eventVolume;
  bool loop;
  bool voice;
  int channel;
  bool silent = false;
}

class GameAudio {
  AudioCompletedCallback? onCompleted;

  // 引擎的 volume 事件（bgm 通道）是唯一的全局音乐衰减；宿主不再叠加自己的
  // master/music/voice 增益，设置里也已经没有那三个滑块。
  double _engineMusicGain = 1;

  _Slot? _bgm;
  _Slot? _voice;
  int _bgmGen = 0;
  int _voiceGen = 0;
  int _effectsGen = 0;
  List<String> _sequence = [];
  bool _sequenceLoop = false;
  int _sequenceIndex = -1;
  double _sequenceVolume = 1;
  final List<_Slot> _effects = [];
  final Map<int, _Slot> _targets = {};
  final Map<int, double> _targetGains = {};
  final Map<String, AudioSource> _sources = {};
  // _sourceOrder 记录 _sources 的加载顺序，用于 FIFO 逐出。真机上语音/音效源是整段
  // 解码后的 PCM（每条 1~2MB 级）长驻内存，一局几百句对话下来内存一路涨，GC 压力
  // 会把 UI isolate 拖出可感知的卡顿——所以缓存必须有界。
  final List<String> _sourceOrder = [];
  Timer? _watchdog;
  bool _polling = false;
  bool _ready = false;
  bool _closed = false;

  Future<void> init() async {
    if (_ready || _closed) {
      return;
    }
    // SoLoud 初始化失败也继续：.vqf 走 fvp 不依赖 SoLoud，掐住 _ready 会让
    // 満淫電車 的人声/音效全无声；非 VQF 走 _load 时 SoLoud 本就会失败静默，
    // 行为不变（仍无声），所以这里只保证轮询和 VQF 通道照常起。
    try {
      await SoLoud.instance.init();
    } catch (_) {}
    if (_closed) {
      try {
        SoLoud.instance.deinit();
      } catch (_) {}
      return;
    }
    _ready = true;
    _watchdog ??= Timer.periodic(_pollInterval, (_) => _pollSlots());
  }

  Future<void> deinit() async {
    if (_closed) {
      return;
    }
    _closed = true;
    _watchdog?.cancel();
    _watchdog = null;
    _bgmGen++;
    _voiceGen++;
    _effectsGen++;
    _sequence = [];
    _sequenceIndex = -1;
    final bgm = _bgm;
    _bgm = null;
    await _drop(bgm);
    final voice = _voice;
    _voice = null;
    await _drop(voice);
    final effects = _effects.toList();
    _effects.clear();
    for (final slot in effects) {
      await _drop(slot);
    }
    final targets = _targets.values.toList();
    _targets.clear();
    _targetGains.clear();
    for (final slot in targets) {
      await _drop(slot);
    }
    final sources = _sources.values.toList();
    _sources.clear();
    _sourceOrder.clear();
    for (final source in sources) {
      try {
        await SoLoud.instance.disposeSource(source);
      } catch (_) {}
    }
    try {
      SoLoud.instance.deinit();
    } catch (_) {}
    _ready = false;
  }

  Future<void> playBgm(String path,
      {double volume = 1, bool loop = false}) async {
    if (_closed || path.isEmpty) {
      return;
    }
    final current = _bgm;
    if (current != null && _sameFile(current.file, path)) {
      current.eventVolume = _clampGain(volume);
      current.loop = loop;
      final vqf = current.vqf;
      if (vqf != null) {
        setVqfLooping(vqf, loop);
      }
      _setSlotVolume(current, _mixBgm(current.eventVolume));
      return;
    }
    final gen = ++_bgmGen;
    await _drop(current);
    if (gen != _bgmGen || _closed) {
      return;
    }
    _bgm = null;
    final slot = await _start(path,
        gain: _mixBgm(volume),
        eventVolume: volume,
        loop: loop,
        voice: false,
        channel: _noTarget,
        stream: true);
    if (slot == null || gen != _bgmGen || _closed) {
      await _drop(slot);
      return;
    }
    _bgm = slot;
  }

  Future<void> stopBgm() async {
    _bgmGen++;
    final slot = _bgm;
    _bgm = null;
    await _drop(slot);
  }

  Future<void> playEffect(String path, {double volume = 1}) async {
    if (_closed || path.isEmpty) {
      return;
    }
    final gen = _effectsGen;
    if (_effects.length >= _maxEffects) {
      await _drop(_effects.removeAt(0));
    }
    final slot = await _start(path,
        gain: _mixEffect(volume),
        eventVolume: volume,
        loop: false,
        voice: false,
        channel: _noTarget);
    if (slot == null || gen != _effectsGen || _closed) {
      await _drop(slot);
      return;
    }
    _effects.add(slot);
  }

  Future<void> stopEffects() async {
    _effectsGen++;
    final slots = _effects.toList();
    _effects.clear();
    for (final slot in slots) {
      await _drop(slot);
    }
  }

  Future<void> playVoice(String path, {double volume = 1, bool loop = false}) async {
    if (_closed || path.isEmpty) {
      return;
    }
    final gen = ++_voiceGen;
    _sequence = [];
    _sequenceIndex = -1;
    await _drop(_voice);
    if (gen != _voiceGen || _closed) {
      return;
    }
    _voice = null;
    final slot = await _start(path,
        gain: _mixVoice(volume),
        eventVolume: volume,
        loop: loop,
        voice: true,
        channel: _noTarget);
    if (slot == null || gen != _voiceGen || _closed) {
      await _drop(slot);
      return;
    }
    _voice = slot;
  }

  Future<void> playVoiceSequence(List<String> names,
      {bool loop = false, double volume = 1}) async {
    if (_closed) {
      return;
    }
    final gen = ++_voiceGen;
    _sequenceVolume = _clampGain(volume);
    await _drop(_voice);
    if (gen != _voiceGen || _closed) {
      return;
    }
    _voice = null;
    _sequence = names.where((name) => name.isNotEmpty).toList();
    _sequenceLoop = loop;
    _sequenceIndex = _sequence.isEmpty ? -1 : 0;
    if (_sequenceIndex < 0) {
      _sequence = [];
      return;
    }
    final slot = await _start(_sequence[0],
        gain: _mixVoice(_sequenceVolume),
        eventVolume: _sequenceVolume,
        loop: false,
        voice: true,
        channel: _noTarget);
    if (slot == null || gen != _voiceGen || _closed) {
      await _drop(slot);
      if (gen == _voiceGen) {
        _sequence = [];
        _sequenceIndex = -1;
      }
      return;
    }
    _voice = slot;
  }

  Future<void> stopVoice() async {
    _voiceGen++;
    _sequence = [];
    _sequenceIndex = -1;
    final slot = _voice;
    _voice = null;
    await _drop(slot);
  }

  Future<void> stopAll() async {
    await stopBgm();
    await stopVoice();
    await stopEffects();
    final slots = _targets.values.toList();
    _targets.clear();
    _targetGains.clear();
    for (final slot in slots) {
      await _drop(slot);
    }
  }

  Future<void> dispatchEvent(AudioEvent evt) async {
    if (_closed) {
      return;
    }
    switch (evt.kind) {
      case KzAudioKind.bgm:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await playBgm(evt.path, volume: evt.volume, loop: evt.loop);
        } else {
          await _playTarget(key, evt.path,
              gain: _mixTarget(evt.volume),
              eventVolume: evt.volume,
              loop: evt.loop,
              voice: false,
              channel: evt.channel,
              stream: true);
        }
      case KzAudioKind.se:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await playEffect(evt.path, volume: evt.volume);
        } else {
          await _playTarget(key, evt.path,
              gain: _mixTarget(evt.volume),
              eventVolume: evt.volume,
              loop: false,
              voice: false,
              channel: evt.channel);
        }
      case KzAudioKind.voice:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await playVoice(evt.path, volume: evt.volume, loop: evt.loop);
        } else {
          await _playTarget(key, evt.path,
              gain: _mixTarget(evt.volume),
              eventVolume: evt.volume,
              loop: evt.loop,
              voice: true,
              channel: evt.channel);
        }
      case KzAudioKind.stopBgm:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await stopBgm();
        } else {
          await _stopTarget(key);
        }
      case KzAudioKind.stopVoice:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await stopVoice();
        } else {
          await _stopTarget(key);
        }
      case KzAudioKind.stopSe:
        final key = _keyOf(evt.channel, evt.group);
        if (key == null) {
          await stopEffects();
        } else {
          await _stopTarget(key);
        }
      case KzAudioKind.voiceSequence:
        await playVoiceSequence(evt.names,
            loop: evt.loop, volume: evt.volume);
      case KzAudioKind.volume:
        _applyEventVolume(evt);
      case KzAudioKind.fadeOut:
        await _fadeEvent(evt);
      case KzAudioKind.stopAll:
        await stopAll();
    }
  }

  Future<void> _playTarget(int key, String path,
      {required double gain,
      required double eventVolume,
      required bool loop,
      required bool voice,
      required int channel,
      bool stream = false}) async {
    if (_closed || path.isEmpty) {
      return;
    }
    final current = _targets[key];
    if (current != null && _sameFile(current.file, path)) {
      current.eventVolume = _clampGain(eventVolume);
      current.loop = loop;
      final vqf = current.vqf;
      if (vqf != null) {
        setVqfLooping(vqf, loop);
      }
      _setSlotVolume(current, gain);
      return;
    }
    await _drop(current);
    if (_closed) {
      return;
    }
    _targets.remove(key);
    final slot = await _start(path,
        gain: gain,
        eventVolume: eventVolume,
        loop: loop,
        voice: voice,
        channel: channel,
        stream: stream);
    if (slot == null || _closed) {
      await _drop(slot);
      return;
    }
    _targets[key] = slot;
  }

  Future<void> _stopTarget(int key) async {
    final slot = _targets.remove(key);
    _targetGains.remove(key);
    await _drop(slot);
  }

  void _applyEventVolume(AudioEvent evt) {
    final gain = _clampGain(evt.volume);
    final key = _keyOf(evt.channel, evt.group);
    if (key == null) {
      _engineMusicGain = gain;
      final bgm = _bgm;
      if (bgm != null) {
        _setSlotVolume(bgm, _mixBgm(bgm.eventVolume));
      }
      return;
    }
    _targetGains[key] = gain;
    final slot = _targets[key];
    if (slot != null) {
      slot.eventVolume = gain;
      _setSlotVolume(slot, _mixTarget(gain));
    }
  }

  Future<void> _fadeEvent(AudioEvent evt) async {
    final key = _keyOf(evt.channel, evt.group);
    if (key != null) {
      final slot = _targets[key];
      if (slot != null) {
        await _fadeSlot(slot, evt.frames, clear: (faded) {
          if (identical(_targets[key], faded)) {
            _targets.remove(key);
          }
        });
      }
      return;
    }
    final bgm = _bgm;
    if (bgm != null) {
      await _fadeSlot(bgm, evt.frames, clear: (faded) {
        if (identical(_bgm, faded)) {
          _bgm = null;
        }
      });
    }
  }

  Future<void> _fadeSlot(_Slot slot, int frames,
      {required void Function(_Slot slot) clear}) async {
    // VQF 后端没有淡出 API：保持同样的等待时长再释放，脚本时序不漂移。
    final vqf = slot.vqf;
    if (vqf != null) {
      slot.loop = false;
      setVqfLooping(vqf, false);
      slot.silent = true;
      if (frames <= 0) {
        clear(slot);
        await _drop(slot);
        return;
      }
      final int millis =
          (frames * 1000 / 60).round().clamp(1, 60000).toInt();
      await Future.delayed(Duration(milliseconds: millis));
      clear(slot);
      await _drop(slot);
      return;
    }
    final handle = slot.handle;
    if (handle == null) {
      clear(slot);
      return;
    }
    slot.loop = false;
    slot.silent = true;
    if (frames <= 0) {
      clear(slot);
      await _drop(slot);
      return;
    }
    final int millis =
        (frames * 1000 / 60).round().clamp(1, 60000).toInt();
    try {
      SoLoud.instance.fadeVolume(handle, 0, Duration(milliseconds: millis));
    } catch (_) {
      clear(slot);
      await _drop(slot);
      return;
    }
    await Future.delayed(Duration(milliseconds: millis));
    clear(slot);
    await _drop(slot);
  }

  Future<_Slot?> _start(String path,
      {required double gain,
      required double eventVolume,
      required bool loop,
      required bool voice,
      required int channel,
      bool stream = false}) async {
    if (_closed) {
      return null;
    }
    // .vqf 分流：SoLoud 解不了 TwinVQ，必须走 fvp（见 vqf_audio.dart）；
    // 初始化失败时 startVqf 返回 null，这里同样返回 null（静默无声）。
    // fvp 通道不需要 SoLoud 就绪，所以不卡在 _ready 上。
    if (_isVqf(path)) {
      final vqf = await startVqf(path, volume: gain, loop: loop);
      if (vqf == null || _closed) {
        if (vqf != null) {
          await stopVqf(vqf);
        }
        return null;
      }
      return _Slot(
          file: path,
          eventVolume: _clampGain(eventVolume),
          loop: loop,
          voice: voice,
          channel: channel)
        ..vqf = vqf;
    }
    if (!_ready) {
      return null;
    }
    final source = await _load(path, stream: stream);
    if (source == null || _closed) {
      return null;
    }
    final slot = _Slot(
        file: path,
        eventVolume: _clampGain(eventVolume),
        loop: loop,
        voice: voice,
        channel: channel)
      ..source = source;
    try {
      slot.handle = SoLoud.instance
          .play(source, volume: _clampGain(gain), looping: loop);
    } catch (_) {
      return null;
    }
    return slot;
  }

  Future<AudioSource?> _load(String path, {bool stream = false}) async {
    // VQF 不进 SoLoud 缓存（_sources）：它走 fvp 由 startVqf 直接打开，
    // 误走到这里说明分流漏了，宁可静默返回也不让 SoLoud 去猜格式。
    if (_isVqf(path)) {
      return null;
    }
    final cached = _sources[path];
    if (cached != null) {
      return cached;
    }
    try {
      // BGM 走磁盘流式：整曲解码进内存是几十 MB 级 PCM，一局几十首全缓存不住；
      // 循环播放不需要 seek，流式的代价（按需读盘 + 多一点 CPU）正好花在刀刃上。
      // 平台解码器不支持的格式（M4A/AAC 等）或磁盘加载失败时退回整段解码——
      // 宁可多占内存也不能没声音。
      final source = await SoLoud.instance
          .loadFile(path, mode: stream ? LoadMode.disk : LoadMode.memory);
      if (_closed) {
        try {
          await SoLoud.instance.disposeSource(source);
        } catch (_) {}
        return null;
      }
      _sources[path] = source;
      _sourceOrder.add(path);
      _evictSources();
      return source;
    } catch (_) {
      if (!stream) {
        return null;
      }
      return _load(path);
    }
  }

  bool _sourceInUse(String path) {
    final bgm = _bgm;
    final voice = _voice;
    if (bgm != null && _sameFile(bgm.file, path)) {
      return true;
    }
    if (voice != null && _sameFile(voice.file, path)) {
      return true;
    }
    for (final slot in _effects) {
      if (_sameFile(slot.file, path)) {
        return true;
      }
    }
    for (final slot in _targets.values) {
      if (_sameFile(slot.file, path)) {
        return true;
      }
    }
    return false;
  }

  // _evictSources 把缓存压回 _maxCachedSources 以内：超出的最老条目逐出并
  // disposeSource。正在被任何槽播放的源不能逐（dispose 会掐断正在响的声音），
  // 留在缓存里，等它播完后下次加载再逐出。逐出只在 _load 之后触发，频率低，
  // O(n) 扫描可以接受。VQF 从不进 _sources，所以不需要在这里处理 fvp 句柄。
  void _evictSources() {
    final overflow = _sourceOrder.length - _maxCachedSources;
    if (overflow <= 0) {
      return;
    }
    for (final path in _sourceOrder.sublist(0, overflow)) {
      if (_sources.containsKey(path) && _sourceInUse(path)) {
        continue;
      }
      _sourceOrder.remove(path);
      final source = _sources.remove(path);
      if (source != null) {
        try {
          SoLoud.instance.disposeSource(source);
        } catch (_) {}
      }
    }
  }

  Future<void> _drop(_Slot? slot) async {
    if (slot == null) {
      return;
    }
    slot.silent = true;
    // VQF 后端：暂停并释放 fvp 控制器（幂等，重复 drop 安全）。
    final vqf = slot.vqf;
    slot.vqf = null;
    if (vqf != null) {
      await stopVqf(vqf);
    }
    final handle = slot.handle;
    slot.handle = null;
    if (handle == null) {
      return;
    }
    try {
      await SoLoud.instance.stop(handle);
    } catch (_) {}
  }

  void _setHandleVolume(SoundHandle handle, double volume) {
    try {
      SoLoud.instance.setVolume(handle, volume);
    } catch (_) {}
  }

  // 按槽分发音量：VQF 走 fvp，Soloud 走句柄。所有“同文件改音量 / volume 事件”
  // 路径都经这里，保证双后端行为一致。
  void _setSlotVolume(_Slot slot, double volume) {
    final vqf = slot.vqf;
    if (vqf != null) {
      setVqfVolume(vqf, volume);
      return;
    }
    final handle = slot.handle;
    if (handle != null) {
      _setHandleVolume(handle, volume);
    }
  }

  void _pollSlots() {
    if (_polling || !_ready || _closed) {
      return;
    }
    _polling = true;
    _pollAsync().then((_) {
      _polling = false;
    }, onError: (_) {
      _polling = false;
    });
  }

  Future<void> _pollAsync() async {
    if (_closed) {
      return;
    }
    final bgm = _bgm;
    if (bgm != null && await _finished(bgm)) {
      if (identical(_bgm, bgm)) {
        _bgm = null;
      }
      _emit(bgm);
    }
    if (_closed) {
      return;
    }
    final voice = _voice;
    if (voice != null && await _finished(voice)) {
      if (identical(_voice, voice)) {
        _voice = null;
        _emit(voice);
        await _advanceSequence();
      }
    }
    if (_closed) {
      return;
    }
    final effects = _effects.toList();
    for (final slot in effects) {
      if (await _finished(slot)) {
        _effects.remove(slot);
        _emit(slot);
      }
      if (_closed) {
        return;
      }
    }
    final targets = _targets.entries.toList();
    for (final entry in targets) {
      if (await _finished(entry.value)) {
        if (identical(_targets[entry.key], entry.value)) {
          _targets.remove(entry.key);
        }
        _emit(entry.value);
      }
      if (_closed) {
        return;
      }
    }
  }

  Future<bool> _finished(_Slot slot) async {
    // VQF 后端：isCompleted + position/duration 轮询；播完即 dispose，
    // 不留解码器（fvp 控制器泄漏会占住文件句柄和解码线程）。
    final vqf = slot.vqf;
    if (vqf != null) {
      if (!vqfFinished(vqf)) {
        return false;
      }
      slot.vqf = null;
      await stopVqf(vqf);
      return true;
    }
    final handle = slot.handle;
    if (handle == null) {
      return true;
    }
    if (slot.loop) {
      return false;
    }
    try {
      return !await SoLoud.instance.getIsValidVoiceHandle(handle);
    } catch (_) {
      return true;
    }
  }

  void _emit(_Slot slot) {
    if (slot.silent || slot.file.isEmpty) {
      return;
    }
    slot.silent = true;
    try {
      onCompleted?.call(slot.file, slot.voice, slot.channel);
    } catch (_) {}
  }

  Future<void> _advanceSequence() async {
    final gen = _voiceGen;
    for (var attempt = 0; attempt <= _sequence.length; attempt++) {
      if (_sequenceIndex < 0 || _sequenceIndex >= _sequence.length) {
        break;
      }
      _sequenceIndex++;
      if (_sequenceIndex >= _sequence.length) {
        if (_sequenceLoop && _sequence.isNotEmpty) {
          _sequenceIndex = 0;
        } else {
          break;
        }
      }
      final name = _sequence[_sequenceIndex];
      if (name.isEmpty) {
        continue;
      }
      final slot = await _start(name,
          gain: _mixVoice(_sequenceVolume),
          eventVolume: _sequenceVolume,
          loop: false,
          voice: true,
          channel: _noTarget);
      if (gen != _voiceGen || _closed) {
        await _drop(slot);
        return;
      }
      if (slot == null) {
        continue;
      }
      _voice = slot;
      return;
    }
    _sequence = [];
    _sequenceIndex = -1;
  }

  // 宿主不再有全局音量：每条声音只用事件自带的 volume，BGM 额外乘引擎的
  // volume 事件衰减（_engineMusicGain），最终统一 clamp 到 [0, 1]。
  double _mixBgm(double eventVolume) =>
      _clampGain(eventVolume * _engineMusicGain);

  double _mixVoice(double eventVolume) => _clampGain(eventVolume);

  double _mixEffect(double eventVolume) => _clampGain(eventVolume);

  double _mixTarget(double eventVolume) => _clampGain(eventVolume);

  static double _clampGain(double value) {
    if (!value.isFinite) {
      return 0;
    }
    if (value < 0) {
      return 0;
    }
    if (value > 1) {
      return 1;
    }
    return value;
  }

  static bool _sameFile(String left, String right) =>
      left.toLowerCase() == right.toLowerCase();

  // 大小写不敏感后缀判断（Windows 盘上 .VQF 与 .vqf 等同）。
  static bool _isVqf(String path) => path.toLowerCase().endsWith('.vqf');

  static int? _keyOf(int channel, int group) {
    if (channel != _noTarget) {
      return channel;
    }
    if (group != _noTarget) {
      return 0x10000 + group;
    }
    return null;
  }
}
