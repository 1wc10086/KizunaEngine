package session

import (
	"strconv"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/core"
)

// audio_volume.go 把设置画面的音量映射到引擎发出的 AudioEvent。
//
// 原版的音量是分通道的（master × bgm / se / bgs / voice / bgv），并且语音播放
// 期间 BGM 还要乘一个 attenuated bgm。以前这些乘法在 Flutter 侧做，引擎只发
// "事件音量"，结果设置画面无法真正控制音量，宿主也偷偷拥有一份独立音量。
// 现在全部在引擎侧乘完，Flutter 只按事件音量播放。

const (
	kindBgm = iota
	kindSe
	kindBgs
	kindVoice
	kindBgv
)

// volumeGain 返回某个通道的最终增益（不含事件自身的音量）。
func (s *Session) volumeGain(kind int) float64 {
	if s.config == nil {
		return 1
	}
	master := s.settings.MasterVolume
	switch kind {
	case kindBgm:
		return clampVolumeSetting(master * s.settings.BgmVolume)
	case kindSe:
		return clampVolumeSetting(master * s.settings.SeVolume)
	case kindBgs:
		return clampVolumeSetting(master * s.settings.BgsVolume)
	case kindVoice:
		return clampVolumeSetting(master * s.settings.VoiceVolume)
	case kindBgv:
		return clampVolumeSetting(master * s.settings.BgvVolume)
	default:
		return clampVolumeSetting(master)
	}
}

// characterGain 按语音文件名前缀（01..06）套用"個別音声設定"。
//
// 文件名形如 01_01998_001；前缀不是角色编号时按 1.0 处理，避免把系统音效
// 误判成角色语音。
func (s *Session) characterGain(name string, voice bool) float64 {
	if len(name) < 2 {
		return 1
	}
	index, err := strconv.Atoi(name[:2])
	if err != nil || index < 1 || index > 6 {
		return 1
	}
	slot := index - 1
	if !s.settings.CharacterVoiceOn[slot] {
		return 0
	}
	if voice {
		return clampVolumeSetting(s.settings.CharacterVoice[slot])
	}
	return clampVolumeSetting(s.settings.CharacterBgv[slot])
}

// appendBgmAttenuation 发一条引擎音乐增益事件。
//
// Flutter 的 GameAudio 把 kind 7 当作"乘在 BGM 事件音量上的额外增益"，
// 所以这里只发衰减系数本身（1.0 表示恢复）。
func (s *Session) appendBgmAttenuation(active bool) {
	if s.settings.BgmAttenuation <= 0 {
		return
	}
	gain := 1.0
	if active {
		gain = s.settings.BgmAttenuation
	}
	s.audio = append(s.audio, core.AudioEvent{
		Kind:    7, // volume / engine music gain
		Volume:  gain,
		Channel: noTarget,
		Group:   noTarget,
	})
}

// refreshAudioVolumes 在设置画面修改音量后立刻生效。
//
// BGM 用"同名重发"更新：Flutter 侧遇到相同文件路径只改增益，不会从头播放。
// 语音/音效没有持久通道，下一次触发时自然使用新音量；正在播的语音不重启，
// 避免玩家在设置画面里拖一下滑块就听到语音跳回开头。
func (s *Session) refreshAudioVolumes() {
	if s.gameState == nil {
		return
	}
	if bgm, ok := s.gameState.Bgm(); ok && len(bgm) > 0 {
		s.audio = append(s.audio, core.AudioEvent{
			Kind:    0,
			Path:    s.materialize(s.bgmForMode(bgm)),
			Volume:  s.gameState.BgmVolume() * s.volumeGain(kindBgm),
			Loop:    s.gameState.BgmLoop(),
			Channel: noTarget,
			Group:   noTarget,
		})
	}
	if s.voicePlaying {
		s.appendBgmAttenuation(true)
	}
}

// ---------------------------------------------------------------- BGM 类型选择

// loadBgmSelect 读 define.bsa 里的 bgm_select 布局，得到 BGM 类型映射表。
//
// 文件形如：
//
//	set: [
//		["bgm07", "bgm13"]
//		...
//	]
//
// 左边是 type0 的曲名，右边是 type1（jazz）的曲名。
// 作品可以没有这个文件，这时设置里的 BGM 类型选择就不起作用。
func (s *Session) loadBgmSelect() {
	if s.bgmSelectLoaded {
		return
	}
	s.bgmSelectLoaded = true
	if s.res == nil {
		return
	}
	// 文件名由 bsxx.cfg 的 "bgmselect db" 决定，没有配置时用默认名。
	name := "bgm_select"
	if s.config != nil && s.config.BgmSelectDatabase != "" {
		name = s.config.BgmSelectDatabase
	}
	location, ok := s.res.TryGet(name)
	if !ok || location == nil {
		return
	}
	root, err := config.ParseLayoutDocument(location.Read())
	if err != nil {
		core.Logf("settings: bgm_select parse failed: %v", err)
		return
	}
	array, ok := config.Array(root["set"])
	if !ok {
		return
	}
	table := make(map[string]string, len(array))
	for _, item := range array {
		pair, ok := config.Array(item)
		if !ok || len(pair) < 2 {
			continue
		}
		from, ok := config.Text(pair[0])
		if !ok || from == "" {
			continue
		}
		to, ok := config.Text(pair[1])
		if !ok || to == "" {
			continue
		}
		table[from] = to
	}
	if len(table) > 0 {
		s.bgmSelectTable = table
		core.Logf("settings: bgm select table loaded=%d", len(table))
	}
}

// bgmForMode 按设置里的 BGM 类型把曲名换成对应的另一首。
//
// 脚本里写死的是 type0 的曲名，玩家在设置里选了 type1 时要听 jazz 版。
// 映射只在"发出播放事件的那一刻"做：脚本、存档和鉴赏列表里仍然是原来的
// 曲名，换类型不会让别处指向不同的资源。
//
// 音乐鉴赏室里播放的是玩家自己挑的曲子，不属于剧情 BGM，不做映射。
func (s *Session) bgmForMode(name string) string {
	if s.ui != nil && s.ui.kind == uiMuseumMusic {
		return name
	}
	s.loadBgmSelect()
	return remapBgmName(s.bgmSelectTable, name, s.settings.BgmSelect)
}

// remapBgmName 按 BGM 类型查映射表。
//
// type0 是原曲，映射表里没有的曲名保持原样（有些曲子只有一个版本）。
func remapBgmName(table map[string]string, name string, mode int) string {
	if mode == 0 || len(table) == 0 || name == "" {
		return name
	}
	if mapped, ok := table[name]; ok && mapped != "" {
		return mapped
	}
	return name
}
