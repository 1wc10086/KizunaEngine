package vm

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const VoiceChannel = 63

const soundEffectChannels = 7

const channelLimit = 64

const groupLimit = 32

func (m *Machine) onSound(statement *script.EmonStatement) EmonStep {
	file := m.resolveAudioFile(statement, 0)
	if file == "" || strings.EqualFold(file, "NULL") || file == "0" {
		return EmonStepContinue
	}
	token := m.ExpandToken(statement, 1)
	flags := parseSoundFlags(token)
	loop := wantsLoop(token, false)
	if flags&EmonSoundMusic != 0 {
		m.playMusicTrack(file, 255)
		return EmonStepContinue
	}
	if flags&EmonSoundGroup != 0 {
		m.playGroupSound(0, file, flags, loop)
		return EmonStepContinue
	}
	if flags&EmonSoundVoice != 0 {
		m.PlayVoiceSound(file, flags, loop)
		m.mutableAudioState().Set("voice", file)
		return EmonStepContinue
	}
	channel := m.seRoundRobin % soundEffectChannels
	m.seRoundRobin++
	m.playChannelSound(channel, file, flags, loop)
	return EmonStepContinue
}

func (m *Machine) onStopSound(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		m.stopAllAudio()
		return EmonStepContinue
	}
	token := strings.ToUpper(m.ExpandToken(statement, 0))
	if strings.Contains(token, "GROUP") {
		if statement.ArgumentCount() > 1 {
			m.stopGroupSound(m.EvalInt(statement, 1, 0))
		} else {
			groups := make([]int, 0, len(m.groupFiles))
			for group := range m.groupFiles {
				groups = append(groups, group)
			}
			for _, group := range groups {
				m.stopGroupSound(group)
			}
		}
		return EmonStepContinue
	}
	if strings.Contains(token, "VOICE") {
		m.StopVoiceSound()
		return EmonStepContinue
	}
	if strings.Contains(token, "CH") {
		channel := 0
		if statement.ArgumentCount() > 1 {
			channel = m.EvalInt(statement, 1, 0)
		}
		channel = clampChannel(channel)
		if !m.isReplaying() {
			m.context.Audio.StopChannel(channel)
		}
		m.SetChannelFile(channel, "")
		delete(m.channelLoops, channel)
		m.mutableAudioState().Delete("loopch" + strconv.Itoa(channel))
		return EmonStepContinue
	}
	if strings.Contains(token, "SE") || strings.Contains(token, "SOUND") {
		m.stopEffectAudio()
		return EmonStepContinue
	}
	m.logOnce("stop sound '" + m.raw(statement, 0) + "' is not supported")
	m.stopAllAudio()
	return EmonStepContinue
}

func (m *Machine) onSetSoundInit(statement *script.EmonStatement) EmonStep {
	m.mutableAudioState().Set("soundInit", strconv.Itoa(m.EvalInt(statement, 0, 0)))
	return EmonStepContinue
}

func (m *Machine) onSetSoundChGr(statement *script.EmonStatement) EmonStep {
	m.channelGroups[clampChannel(m.EvalInt(statement, 0, 0))] = clampGroup(m.EvalInt(statement, 1, 0))
	return EmonStepContinue
}

func (m *Machine) onSoundCh(statement *script.EmonStatement) EmonStep {
	channel := clampChannel(m.EvalInt(statement, 0, 0))
	file := m.resolveAudioFile(statement, 1)
	if file == "" || strings.EqualFold(file, "NULL") {
		return EmonStepContinue
	}
	token := m.ExpandToken(statement, 2)
	loop := wantsLoop(token, true)
	m.playChannelSound(channel, file, parseSoundFlags(token), loop)
	if loop {
		m.SetChannelFile(channel, file)
		m.channelLoops[channel] = true
		m.mutableAudioState().Set("loopch"+strconv.Itoa(channel), file)
	} else {
		m.SetChannelFile(channel, "")
		delete(m.channelLoops, channel)
		m.mutableAudioState().Delete("loopch" + strconv.Itoa(channel))
	}
	return EmonStepContinue
}

func (m *Machine) onSoundGr(statement *script.EmonStatement) EmonStep {
	group := clampGroup(m.EvalInt(statement, 0, 0))
	file := m.resolveAudioFile(statement, 1)
	if file == "" || strings.EqualFold(file, "NULL") {
		return EmonStepContinue
	}
	token := m.ExpandToken(statement, 2)
	loop := wantsLoop(token, false)
	m.playGroupSound(group, file, parseSoundFlags(token), loop)
	if loop {
		m.groupFiles[group] = file
		m.mutableAudioState().Set("loopgr"+strconv.Itoa(group), file)
	} else {
		m.mutableAudioState().Delete("loopgr" + strconv.Itoa(group))
	}
	return EmonStepContinue
}

func (m *Machine) onPlayCh(statement *script.EmonStatement) EmonStep {
	channel := clampChannel(m.EvalInt(statement, 0, 0))
	if file, exists := m.channelFiles[channel]; exists && file != "" {
		loop := false
		if value, ok := m.channelLoops[channel]; ok && value {
			loop = true
		}
		m.playChannelSound(channel, file, EmonSoundEffect, loop)
	} else {
		m.logOnce("play channel " + strconv.Itoa(channel) + " has no file")
	}
	return EmonStepContinue
}

func (m *Machine) onSetSndVolCh(statement *script.EmonStatement) EmonStep {
	if !m.isReplaying() {
		m.context.Audio.SetChannelVolume(clampChannel(m.EvalInt(statement, 0, 0)), toVolume255(m.value(statement, 1)))
	}
	return EmonStepContinue
}

func (m *Machine) onSetSndVolGr(statement *script.EmonStatement) EmonStep {
	group := clampGroup(m.EvalInt(statement, 0, 0))
	volume := toVolume255(m.value(statement, 1))
	if !m.isReplaying() {
		m.context.Audio.SetGroupVolume(group, volume)
	}
	m.mutableAudioState().Set("volgr"+strconv.Itoa(group), strconv.Itoa(volume))
	for channel, mapped := range m.channelGroups {
		if mapped == group && !m.isReplaying() {
			m.context.Audio.SetChannelVolume(channel, volume)
		}
	}
	return EmonStepContinue
}

func (m *Machine) onWaitSound(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("wait sound arguments are ignored")
	}
	return EmonStepContinue
}

func (m *Machine) onMusic(statement *script.EmonStatement) EmonStep {
	file := m.resolveAudioFile(statement, 0)
	volume := 255
	if statement.ArgumentCount() > 1 {
		volume = toVolume255(m.value(statement, 1))
	}
	if file == "" || strings.EqualFold(file, "NULL") || file == "0" {
		if !m.isReplaying() {
			m.context.Audio.StopMusic()
		}
		m.mutableAudioState().Delete("bgm")
		return EmonStepContinue
	}
	m.playMusicTrack(file, volume)
	return EmonStepContinue
}

func (m *Machine) onStopMusic(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("stop music arguments are ignored")
	}
	if !m.isReplaying() {
		m.context.Audio.StopMusic()
	}
	m.mutableAudioState().Delete("bgm")
	return EmonStepContinue
}

func (m *Machine) onFadeOutMusic(statement *script.EmonStatement) EmonStep {
	if !m.isReplaying() {
		frames := 0
		if statement.ArgumentCount() != 0 {
			frames = m.EvalInt(statement, statement.ArgumentCount()-1, 0)
		}
		m.context.Audio.FadeOutMusic(max(0, frames))
	}
	return EmonStepContinue
}

func (m *Machine) onGetPlayMusicInfo(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() < 2 {
		core.Logf("emon: get play music info needs two destinations")
		return EmonStepContinue
	}
	state := m.mutableAudioState()
	file, _ := state.Get("bgm")
	volume := 255
	if volumeText, exists := state.Get("bgmVolume"); exists {
		if parsed, ok := tryParseInt(volumeText); ok {
			volume = parsed
		}
	}
	m.SetVariable(m.raw(statement, 0), script.EmonValueFromString(file))
	m.SetVariable(m.raw(statement, 1), script.EmonValueFromInt(int32(volume)))
	return EmonStepContinue
}

func (m *Machine) NotifySoundEnded(file string, voice bool) {
	if voice {
		m.soundGate.Lock()
		m.soundCompletions = append(m.soundCompletions, EmonSoundCompletion{File: file, Channel: VoiceChannel, Voice: true})
		m.soundGate.Unlock()
		return
	}
	channel := -1
	for entryChannel, entryFile := range m.channelFilesView {
		if strings.EqualFold(entryFile, file) {
			channel = entryChannel
			break
		}
	}
	m.soundGate.Lock()
	m.soundCompletions = append(m.soundCompletions, EmonSoundCompletion{File: file, Channel: channel, Voice: false})
	m.soundGate.Unlock()
}

func (m *Machine) NotifySoundEndedChannel(file string, voice bool, channel int) {
	if channel < 0 {
		m.NotifySoundEnded(file, voice)
		return
	}
	resolved := channel
	if voice {
		resolved = VoiceChannel
	}
	m.soundGate.Lock()
	m.soundCompletions = append(m.soundCompletions, EmonSoundCompletion{File: file, Channel: resolved, Voice: voice})
	m.soundGate.Unlock()
}

func (m *Machine) SetChannelFile(channel int, file string) {
	if file != "" {
		m.channelFiles[channel] = file
	} else {
		delete(m.channelFiles, channel)
	}
	view := make(map[int]string, len(m.channelFiles))
	for key, value := range m.channelFiles {
		view[key] = value
	}
	m.channelFilesView = view
}

func (m *Machine) PlayVoiceSound(file string, flags EmonSoundFlags, loop bool) {
	if !m.isReplaying() {
		m.context.Audio.StopChannel(VoiceChannel)
	}
	m.playChannelSound(VoiceChannel, file, flags|EmonSoundVoice, loop)
}

func (m *Machine) StopVoiceSound() {
	if !m.isReplaying() {
		m.context.Audio.StopChannel(VoiceChannel)
	}
	m.mutableAudioState().Delete("voice")
}

func (m *Machine) playChannelSound(channel int, file string, flags EmonSoundFlags, loop bool) {
	channel = clampChannel(channel)
	request := EmonSoundRequest{File: file, Flags: flags, Group: -1, Channel: channel}
	if loop {
		request.Flags &^= EmonSoundNoLoop
	} else {
		request.Flags |= EmonSoundNoLoop
	}
	if m.isReplaying() {
		return
	}
	m.context.Audio.PlaySound(request)
}

func (m *Machine) playGroupSound(group int, file string, flags EmonSoundFlags, loop bool) {
	group = clampGroup(group)
	request := EmonSoundRequest{File: file, Flags: flags | EmonSoundGroup, Group: group, Channel: -1}
	if loop {
		request.Flags &^= EmonSoundNoLoop
	} else {
		request.Flags |= EmonSoundNoLoop
	}
	if m.isReplaying() {
		return
	}
	m.context.Audio.PlaySound(request)
}

func (m *Machine) playMusicTrack(file string, volume int) {
	if !m.isReplaying() {
		m.context.Audio.PlayMusic(file, volume)
	}
	m.mutableAudioState().Set("bgm", file)
	m.mutableAudioState().Set("bgmVolume", strconv.Itoa(volume))
}

func (m *Machine) stopGroupSound(group int) {
	group = clampGroup(group)
	if !m.isReplaying() {
		m.context.Audio.StopGroup(group)
	}
	delete(m.groupFiles, group)
	m.mutableAudioState().Delete("loopgr" + strconv.Itoa(group))
}

func (m *Machine) stopEffectAudio() {
	if m.isReplaying() {
		return
	}
	for channel := 0; channel < VoiceChannel; channel++ {
		m.context.Audio.StopChannel(channel)
	}
	groups := make([]int, 0, len(m.groupFiles))
	for group := range m.groupFiles {
		groups = append(groups, group)
	}
	for _, group := range groups {
		m.context.Audio.StopGroup(group)
	}
}

func (m *Machine) stopAllAudio() {
	state := m.mutableAudioState()
	music, _ := state.Get("bgm")
	volume, _ := state.Get("bgmVolume")
	if !m.isReplaying() {
		m.context.Audio.StopAll()
	}
	m.channelFiles = make(map[int]string)
	m.channelLoops = make(map[int]bool)
	m.channelFilesView = make(map[int]string)
	m.groupFiles = make(map[int]string)
	state.Clear()
	if music != "" {
		state.Set("bgm", music)
	}
	if volume != "" {
		state.Set("bgmVolume", volume)
	}
}

func (m *Machine) reissueAudio() {
	state := m.mutableAudioState()
	if music, exists := state.Get("bgm"); exists && music != "" {
		volume := 255
		if text, ok := state.Get("bgmVolume"); ok {
			if parsed, ok := tryParseInt(text); ok {
				volume = parsed
			}
		}
		m.playMusicTrack(music, volume)
	}
	for _, key := range state.Keys() {
		if !strutil.HasPrefixFold(key, "loopch") {
			continue
		}
		value, _ := state.Get(key)
		if channel, err := strconv.Atoi(key[len("loopch"):]); err == nil {
			m.playChannelSound(channel, value, EmonSoundEffect, true)
		}
	}
	for _, key := range state.Keys() {
		if !strutil.HasPrefixFold(key, "loopgr") {
			continue
		}
		value, _ := state.Get(key)
		if group, err := strconv.Atoi(key[len("loopgr"):]); err == nil {
			m.playGroupSound(group, value, EmonSoundEffect, true)
		}
	}
}

func (m *Machine) resolveAudioFile(statement *script.EmonStatement, index int32) string {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return ""
	}
	var text string
	if arg.WasQuoted {
		text = arg.Raw
	} else {
		text = m.valueOf(arg.Raw, false).AsString()
	}
	head := text
	if comma := strings.Index(head, ","); comma >= 0 {
		head = head[:comma]
	}
	return strings.Trim(strings.TrimSpace(head), "\"")
}

func parseSoundFlags(token string) EmonSoundFlags {
	if token == "" {
		return EmonSoundEffect
	}
	upper := strings.ToUpper(token)
	flags := EmonSoundFlags(EmonSoundNone)
	if strings.Contains(upper, "VOICE") {
		flags |= EmonSoundVoice
	}
	if strings.Contains(upper, "MUSIC") || strings.Contains(upper, "BGM") {
		flags |= EmonSoundMusic
	}
	if strings.Contains(upper, "SE") {
		flags |= EmonSoundEffect
	}
	if strings.Contains(upper, "MUST") {
		flags |= EmonSoundMust
	}
	if strings.Contains(upper, "NOLOOP") {
		flags |= EmonSoundNoLoop
	}
	if strings.Contains(upper, "GROUP") {
		flags |= EmonSoundGroup
	}
	if flags == EmonSoundNone {
		return EmonSoundEffect
	}
	return flags
}

func wantsLoop(token string, fallback bool) bool {
	if token == "" {
		return fallback
	}
	upper := strings.ToUpper(token)
	if strings.Contains(upper, "NOLOOP") {
		return false
	}
	return strings.Contains(upper, "LOOP") || fallback
}

func toVolume255(value script.EmonValue) int {
	text := strings.TrimSpace(value.AsString())
	if level, err := parseFloatInvariant(text); err == nil {
		if (strings.Contains(text, ".") || strings.Contains(text, "e") || strings.Contains(text, "E")) && level >= 0 && level <= 1 {
			return core.Clamp(int(level*255), 0, 255)
		}
		return core.Clamp(int(level), 0, 255)
	}
	return core.Clamp(int(value.AsInt()), 0, 255)
}

func parseFloatInvariant(text string) (float64, error) {
	if strings.Contains(text, "_") {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(strings.TrimSpace(text), 64)
}

func clampChannel(channel int) int {
	return core.Clamp(channel, 0, channelLimit-1)
}

func clampGroup(group int) int {
	return core.Clamp(group, 0, groupLimit-1)
}
