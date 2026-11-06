package vm

import (
	"strings"
	"sync"
	"sync/atomic"

	"kizuna/engine/core"
)

const framesPerSecondAudio = 60.0

const audioChannelCount = 64

const audioGroupCount = 32

type EmonPlaybackKind int

const (
	EmonPlaybackPlay EmonPlaybackKind = iota
	EmonPlaybackStop
	EmonPlaybackStopAll
	EmonPlaybackSetVolume
	EmonPlaybackFadeOut
)

type EmonPlaybackCommand struct {
	Kind    EmonPlaybackKind
	File    string
	Group   int
	Channel int
	Loop    bool
	Volume  int
	Frames  int
}

func NewEmonPlaybackCommand(kind EmonPlaybackKind) EmonPlaybackCommand {
	return EmonPlaybackCommand{Kind: kind, Group: -1, Channel: -1, Volume: 255}
}

type EmonAudioChannel struct {
	Index                       int
	IsGroup                     bool
	File                        string
	Volume                      int
	Loop                        bool
	IsPlaying                   bool
	Voice                       bool
	PlaybackId                  int64
	PendingCompletionPlaybackId int64
}

func NewEmonAudioChannel(index int, isGroup bool) *EmonAudioChannel {
	return &EmonAudioChannel{Index: index, IsGroup: isGroup, Volume: 255}
}

func (c *EmonAudioChannel) Reset() {
	c.File = ""
	c.Volume = 255
	c.Loop = false
	c.IsPlaying = false
	c.Voice = false
	c.PlaybackId = 0
	c.PendingCompletionPlaybackId = 0
}

type EmonSoundFlags int

const (
	EmonSoundNone   EmonSoundFlags = 0
	EmonSoundEffect EmonSoundFlags = 1
	EmonSoundVoice  EmonSoundFlags = 2
	EmonSoundMusic  EmonSoundFlags = 4
	EmonSoundMust   EmonSoundFlags = 8
	EmonSoundNoLoop EmonSoundFlags = 16
	EmonSoundGroup  EmonSoundFlags = 32
)

type EmonSoundRequest struct {
	File    string
	Flags   EmonSoundFlags
	Group   int
	Channel int
}

type EmonSoundCompletion struct {
	File    string
	Channel int
	Voice   bool
}

type EmonAudioEngine struct {
	CommandIssued func(EmonPlaybackCommand)

	gate              sync.Mutex
	channels          map[int]*EmonAudioChannel
	channelOrder      []int
	groups            map[int]*EmonAudioChannel
	groupOrder        []int
	completions       []EmonSoundCompletion
	musicFile         string
	musicVolume       int
	musicPlaying      bool
	fadeTotal         int
	fadeLeft          int
	fadeFrom          int
	fadeCarry         float64
	playbackIdCounter int64
	disposed          int32
}

func NewEmonAudioEngine(dataRoot string, loader func(string) []byte) *EmonAudioEngine {
	return &EmonAudioEngine{
		channels:    make(map[int]*EmonAudioChannel),
		groups:      make(map[int]*EmonAudioChannel),
		musicVolume: 255,
	}
}

func (e *EmonAudioEngine) IsPlaying() bool {
	e.gate.Lock()
	defer e.gate.Unlock()
	if e.musicPlaying {
		return true
	}
	for _, slot := range e.channels {
		if slot.IsPlaying {
			return true
		}
	}
	for _, slot := range e.groups {
		if slot.IsPlaying {
			return true
		}
	}
	return false
}

func (e *EmonAudioEngine) PlaySound(request EmonSoundRequest) {
	file := request.File
	if file == "" {
		return
	}
	voice := request.Flags&EmonSoundVoice != 0
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	loop := request.Flags&EmonSoundNoLoop == 0
	if request.Flags&EmonSoundGroup != 0 {
		slot := e.slotForLocked(e.groups, &e.groupOrder, request.Group, true)
		if slot.IsPlaying && slot.File != "" && !strings.EqualFold(slot.File, file) {
			e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackStop, File: slot.File, Group: slot.Index, Channel: -1, Volume: slot.Volume})
		}
		e.playbackIdCounter++
		slot.PlaybackId = e.playbackIdCounter
		slot.PendingCompletionPlaybackId = 0
		slot.File = file
		slot.Loop = loop
		slot.IsPlaying = true
		slot.Voice = voice
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackPlay, File: file, Group: slot.Index, Channel: -1, Loop: loop, Volume: slot.Volume})
		return
	}
	slot := e.slotForLocked(e.channels, &e.channelOrder, request.Channel, false)
	if slot.IsPlaying && slot.File != "" && !strings.EqualFold(slot.File, file) {
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackStop, File: slot.File, Group: -1, Channel: slot.Index, Volume: slot.Volume})
	}
	e.playbackIdCounter++
	slot.PlaybackId = e.playbackIdCounter
	slot.PendingCompletionPlaybackId = 0
	slot.File = file
	slot.Loop = loop
	slot.IsPlaying = true
	slot.Voice = voice
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackPlay, File: file, Group: -1, Channel: slot.Index, Loop: loop, Volume: slot.Volume})
}

func (e *EmonAudioEngine) NotifyPlaybackCompleted(file string, voice bool) {
	if file == "" {
		return
	}
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	match := findCompletionSlot(e.channels, e.channelOrder, file, voice)
	if match == nil {
		match = findCompletionSlot(e.groups, e.groupOrder, file, voice)
	}
	if match == nil {
		return
	}
	match.PendingCompletionPlaybackId = match.PlaybackId
	e.completions = append(e.completions, EmonSoundCompletion{File: file, Channel: match.Index, Voice: voice})
}

func (e *EmonAudioEngine) HasCompletedSounds() bool {
	e.gate.Lock()
	defer e.gate.Unlock()
	return len(e.completions) > 0
}

func (e *EmonAudioEngine) DequeueCompletion() (EmonSoundCompletion, bool) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if len(e.completions) == 0 {
		return EmonSoundCompletion{}, false
	}
	completion := e.completions[0]
	e.completions = e.completions[1:]
	return completion, true
}

func (e *EmonAudioEngine) PlayMusic(file string, volume int) {
	if file == "" {
		return
	}
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	normalized := core.Clamp(volume, 0, 255)
	if e.musicPlaying && strings.EqualFold(e.musicFile, file) {
		if normalized == e.musicVolume {
			return
		}
		e.musicVolume = normalized
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackSetVolume, File: e.musicFile, Group: -1, Channel: -1, Loop: true, Volume: normalized})
		return
	}
	e.musicFile = file
	e.musicPlaying = true
	e.musicVolume = normalized
	e.fadeTotal = 0
	e.fadeLeft = 0
	e.fadeCarry = 0
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackPlay, File: file, Group: -1, Channel: -1, Loop: true, Volume: normalized})
}

func (e *EmonAudioEngine) StopMusic() {
	e.gate.Lock()
	defer e.gate.Unlock()
	e.stopMusicLocked()
}

func (e *EmonAudioEngine) stopMusicLocked() {
	if atomic.LoadInt32(&e.disposed) != 0 || !e.musicPlaying {
		return
	}
	file := e.musicFile
	e.musicFile = ""
	e.musicPlaying = false
	e.fadeTotal = 0
	e.fadeLeft = 0
	e.fadeCarry = 0
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackStop, File: file, Group: -1, Channel: -1})
}

func (e *EmonAudioEngine) FadeOutMusic(frames int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 || !e.musicPlaying {
		return
	}
	if frames <= 0 {
		e.stopMusicLocked()
		return
	}
	e.fadeTotal = frames
	e.fadeLeft = frames
	e.fadeFrom = e.musicVolume
	e.fadeCarry = 0
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackFadeOut, File: e.musicFile, Group: -1, Channel: -1, Loop: true, Volume: e.musicVolume, Frames: frames})
}

func (e *EmonAudioEngine) StopGroup(group int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	slot, exists := e.groups[clampEngineGroup(group)]
	if !exists || !slot.IsPlaying {
		return
	}
	file := slot.File
	slot.File = ""
	slot.Loop = false
	slot.IsPlaying = false
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackStop, File: file, Group: slot.Index, Channel: -1, Volume: slot.Volume})
}

func (e *EmonAudioEngine) StopChannel(channel int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	slot, exists := e.channels[clampEngineChannel(channel)]
	if !exists || !slot.IsPlaying {
		return
	}
	file := slot.File
	slot.File = ""
	slot.Loop = false
	slot.IsPlaying = false
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackStop, File: file, Group: -1, Channel: slot.Index, Volume: slot.Volume})
}

func (e *EmonAudioEngine) SetGroupVolume(group, volume int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	slot := e.slotForLocked(e.groups, &e.groupOrder, group, true)
	normalized := core.Clamp(volume, 0, 255)
	if slot.Volume == normalized {
		return
	}
	slot.Volume = normalized
	if slot.IsPlaying {
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackSetVolume, File: slot.File, Group: slot.Index, Channel: -1, Loop: slot.Loop, Volume: slot.Volume})
	}
}

func (e *EmonAudioEngine) SetChannelVolume(channel, volume int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	slot := e.slotForLocked(e.channels, &e.channelOrder, channel, false)
	normalized := core.Clamp(volume, 0, 255)
	if slot.Volume == normalized {
		return
	}
	slot.Volume = normalized
	if slot.IsPlaying {
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackSetVolume, File: slot.File, Group: -1, Channel: slot.Index, Loop: slot.Loop, Volume: slot.Volume})
	}
}

func (e *EmonAudioEngine) SetMusicVolume(volume int) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	normalized := core.Clamp(volume, 0, 255)
	if normalized == e.musicVolume {
		return
	}
	e.musicVolume = normalized
	if e.musicPlaying {
		e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackSetVolume, File: e.musicFile, Group: -1, Channel: -1, Loop: true, Volume: normalized})
	}
}

func (e *EmonAudioEngine) StopAll() {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 {
		return
	}
	for _, slot := range e.channels {
		slot.Reset()
	}
	for _, slot := range e.groups {
		slot.Reset()
	}
	e.musicFile = ""
	e.musicPlaying = false
	e.fadeTotal = 0
	e.fadeLeft = 0
	e.fadeCarry = 0
	e.raiseLocked(NewEmonPlaybackCommand(EmonPlaybackStopAll))
}

func (e *EmonAudioEngine) Update(deltaSeconds float64) {
	if isNaN(deltaSeconds) || deltaSeconds <= 0 {
		return
	}
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.LoadInt32(&e.disposed) != 0 || !e.musicPlaying || e.fadeLeft <= 0 {
		return
	}
	e.fadeCarry += deltaSeconds * framesPerSecondAudio
	steps := int(e.fadeCarry)
	if steps <= 0 {
		return
	}
	e.fadeCarry -= float64(steps)
	e.fadeLeft -= steps
	if e.fadeLeft <= 0 || e.fadeTotal <= 0 {
		e.stopMusicLocked()
		return
	}
	next := core.Clamp(int(int64(e.fadeFrom)*int64(e.fadeLeft)/int64(e.fadeTotal)), 0, 255)
	if next == e.musicVolume {
		return
	}
	e.musicVolume = next
	e.raiseLocked(EmonPlaybackCommand{Kind: EmonPlaybackSetVolume, File: e.musicFile, Group: -1, Channel: -1, Loop: true, Volume: next})
}

func (e *EmonAudioEngine) Dispose() {
	e.gate.Lock()
	defer e.gate.Unlock()
	if atomic.SwapInt32(&e.disposed, 1) != 0 {
		return
	}
	e.raiseLocked(NewEmonPlaybackCommand(EmonPlaybackStopAll))
	e.channels = make(map[int]*EmonAudioChannel)
	e.channelOrder = nil
	e.groups = make(map[int]*EmonAudioChannel)
	e.groupOrder = nil
	e.musicFile = ""
	e.musicPlaying = false
	e.fadeTotal = 0
	e.fadeLeft = 0
	e.fadeCarry = 0
}

func (e *EmonAudioEngine) slotForLocked(slots map[int]*EmonAudioChannel, order *[]int, index int, isGroup bool) *EmonAudioChannel {
	var key int
	if isGroup {
		key = clampEngineGroup(index)
	} else {
		key = clampEngineChannel(index)
	}
	if slot, exists := slots[key]; exists {
		return slot
	}
	slot := NewEmonAudioChannel(key, isGroup)
	slots[key] = slot
	*order = append(*order, key)
	return slot
}

func clampEngineChannel(index int) int {
	return core.Clamp(index, 0, audioChannelCount-1)
}

func clampEngineGroup(index int) int {
	return core.Clamp(index, 0, audioGroupCount-1)
}

func findCompletionSlot(slots map[int]*EmonAudioChannel, order []int, file string, voice bool) *EmonAudioChannel {
	for _, key := range order {
		slot, exists := slots[key]
		if !exists {
			continue
		}
		if !slot.IsPlaying || slot.Loop || slot.Voice != voice {
			continue
		}
		if !strings.EqualFold(slot.File, file) {
			continue
		}
		if slot.PlaybackId == slot.PendingCompletionPlaybackId {
			continue
		}
		return slot
	}
	return nil
}

func (e *EmonAudioEngine) raiseLocked(command EmonPlaybackCommand) {
	handler := e.CommandIssued
	if handler == nil {
		return
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				message := "unknown"
				if err, ok := recovered.(error); ok {
					message = err.Error()
				}
				core.Logf("audio: CommandIssued failed: %s", trimMessage(message))
			}
		}()
		handler(command)
	}()
}

func trimMessage(message string) string {
	if len(message) <= 256 {
		return message
	}
	return message[:256]
}
