package session

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
)

const (
	musicStateKey       = "bgm"
	musicVolumeStateKey = "bgmVolume"
)

func (s *Session) onAudioCommand(command vm.EmonPlaybackCommand) {
	if s.disposed {
		return
	}
	music := command.Group < 0 && command.Channel < 0
	group := command.Group >= 0 && command.Channel < 0
	switch command.Kind {
	case vm.EmonPlaybackPlay:
		s.publishPlayback(command, music)
	case vm.EmonPlaybackStop:
		if music {
			s.bgmArchiveName = ""
			s.bgm = ""
			s.pushAudioEvent(core.AudioEvent{Kind: 3, Channel: noTarget, Group: noTarget})
		} else if command.Channel == vm.VoiceChannel {
			s.pushAudioEvent(core.AudioEvent{Kind: 4, Channel: uint32(command.Channel), Group: noTarget})
		} else if group {
			s.stopGroupEffect(command.File)
		} else {
			s.pushAudioEvent(core.AudioEvent{Kind: 5, Channel: noTarget, Group: noTarget})
		}
	case vm.EmonPlaybackStopAll:
		s.bgmArchiveName = ""
		s.bgm = ""
		s.effectArchive = ""
		s.pushAudioEvent(core.AudioEvent{Kind: 9, Channel: noTarget, Group: noTarget})
	case vm.EmonPlaybackSetVolume:
		volume := toVolume(command.Volume)
		if music {
			s.bgmVolume = volume
			s.pushAudioEvent(core.AudioEvent{Kind: 7, Volume: volume, Channel: noTarget, Group: noTarget})
		} else if command.Channel >= 0 {
			s.pushAudioEvent(core.AudioEvent{Kind: 7, Volume: volume, Channel: uint32(command.Channel), Group: noTarget})
		} else if command.Group >= 0 {
			s.pushAudioEvent(core.AudioEvent{Kind: 7, Volume: volume, Channel: noTarget, Group: uint32(command.Group)})
		}
	case vm.EmonPlaybackFadeOut:
		event := core.AudioEvent{Kind: 8, Frames: uint32(command.Frames), Channel: noTarget, Group: noTarget}
		if command.Channel >= 0 {
			event.Channel = uint32(command.Channel)
		}
		if command.Group >= 0 {
			event.Group = uint32(command.Group)
		}
		s.pushAudioEvent(event)
	}
}

func (s *Session) NotifyAudioCompleted(file string, voice bool, channel uint32) {
	if s.disposed || s.audio == nil || s.machine == nil {
		return
	}
	s.audio.NotifyPlaybackCompleted(s.resolveAudioName(file), voice)
	for drained := 0; drained < maxCompletionDrain; drained++ {
		completion, ok := s.audio.DequeueCompletion()
		if !ok {
			break
		}
		s.machine.NotifySoundEndedChannel(completion.File, completion.Voice, completion.Channel)
	}
}

func (s *Session) resolveAudioName(file string) string {
	s.audioNameGate.Lock()
	defer s.audioNameGate.Unlock()
	if archive, ok := s.audioNames[strings.ToLower(file)]; ok {
		return archive
	}
	return file
}

func (s *Session) rememberAudioName(path, archive string) {
	if path == "" || path == archive {
		return
	}
	s.audioNameGate.Lock()
	defer s.audioNameGate.Unlock()
	s.audioNames[strings.ToLower(path)] = archive
	if len(s.audioNames) > audioNameMapLimit {
		s.pruneAudioNamesLocked()
	}
}

func (s *Session) publishPlayback(command vm.EmonPlaybackCommand, music bool) {
	if strings.TrimSpace(command.File) == "" {
		return
	}
	path := s.materializeAudio(command.File)
	if path == "" {
		path = command.File
	}
	volume := toVolume(command.Volume)
	if music {
		s.bgmArchiveName = command.File
		s.bgmVolume = volume
		s.bgmLoop = command.Loop
		s.bgm = path
		s.pushAudioEvent(core.AudioEvent{Kind: 0, Path: path, Volume: volume, Loop: command.Loop, Channel: noTarget, Group: noTarget})
		core.Logf("emon-audio: music '%s' -> '%s' volume=%d", command.File, path, command.Volume)
		return
	}
	s.rememberAudioName(path, command.File)
	if command.Channel == vm.VoiceChannel {
		s.pushAudioEvent(core.AudioEvent{Kind: 2, Path: path, Volume: volume, Channel: uint32(command.Channel), Group: noTarget})
		return
	}
	event := core.AudioEvent{Kind: 1, Path: path, Volume: volume, Channel: noTarget, Group: noTarget}
	if command.Channel >= 0 {
		event.Channel = uint32(command.Channel)
	}
	if command.Group >= 0 {
		event.Group = uint32(command.Group)
	}
	s.pushAudioEvent(event)
}

func (s *Session) stopGroupEffect(archive string) {
	if archive == "" || !strings.EqualFold(s.effectArchive, archive) {
		return
	}
	s.effectArchive = ""
	if s.machine == nil {
		return
	}
	s.pushAudioEvent(core.AudioEvent{Kind: 5, Channel: noTarget, Group: noTarget})
}

func (s *Session) pushAudioEvent(event core.AudioEvent) {
	s.audioGate.Lock()
	defer s.audioGate.Unlock()
	s.audioEvents = append(s.audioEvents, event)
}

func (s *Session) Audio() []core.AudioEvent {
	s.audioGate.Lock()
	defer s.audioGate.Unlock()
	out := s.audioEvents
	s.audioEvents = nil
	return out
}

func (s *Session) pruneAudioNamesLocked() {
	for path := range s.audioNames {
		if _, err := os.Stat(path); err != nil {
			delete(s.audioNames, path)
		}
	}
	if len(s.audioNames) <= audioNameMapLimit {
		return
	}
	for path := range s.audioNames {
		if len(s.audioNames) <= audioNameMapLimit {
			return
		}
		delete(s.audioNames, path)
	}
}

func (s *Session) materializeAudio(file string) string {
	name := strings.TrimSpace(file)
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name
		}
		return ""
	}
	if s.archives == nil {
		return ""
	}
	content := s.archives.Read(name)
	if len(content) == 0 {
		core.Logf("emon-audio: '%s' is not in the archives", name)
		return ""
	}
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		extension = ".ogg"
	}
	stem := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	sum := sha256.Sum256(content)
	digest := strings.ToUpper(hex.EncodeToString(sum[:]))[:12]
	target := filepath.Join(s.audioCacheDir, sanitizeName(stem)+"-"+digest+extension)
	s.audioGate.Lock()
	defer s.audioGate.Unlock()
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		touchAudioCacheFile(target)
		return target
	}
	s.pruneAudioCacheBeforeWrite(int64(len(content)))
	if err := os.MkdirAll(s.audioCacheDir, 0o755); err != nil {
		core.Logf("emon-audio: '%s' could not be materialized: %s", name, err.Error())
		return ""
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		core.Logf("emon-audio: '%s' could not be materialized: %s", name, err.Error())
		return ""
	}
	touchAudioCacheFile(target)
	return target
}

func (s *Session) pruneAudioCacheBeforeWrite(incoming int64) {
	s.pruneAudioNames()
	if _, err := os.Stat(s.audioCacheDir); err != nil {
		return
	}
	type cacheEntry struct {
		path   string
		length int64
		access time.Time
	}
	var entries []cacheEntry
	var total int64
	children, err := os.ReadDir(s.audioCacheDir)
	if err != nil {
		return
	}
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		full := filepath.Join(s.audioCacheDir, child.Name())
		if strings.HasSuffix(strings.ToLower(child.Name()), ".tmp") {
			_ = os.Remove(full)
			continue
		}
		info, err := child.Info()
		if err != nil {
			continue
		}
		entries = append(entries, cacheEntry{path: full, length: info.Size(), access: info.ModTime()})
		total += info.Size()
	}
	budget := int64(audioCacheBudgetMB) * 1024 * 1024
	if len(entries)+1 <= audioCacheBudgetFile && total+incoming <= budget {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].access.Before(entries[j].access)
	})
	count := len(entries) + 1
	for _, entry := range entries {
		if count <= audioCacheBudgetFile && total+incoming <= budget {
			return
		}
		if err := os.Remove(entry.path); err == nil {
			total -= entry.length
			count--
		}
	}
}

func (s *Session) pruneAudioNames() {
	s.audioNameGate.Lock()
	defer s.audioNameGate.Unlock()
	s.pruneAudioNamesLocked()
}

func touchAudioCacheFile(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

func (s *Session) captureAudioState() map[string]string {
	state := map[string]string{}
	if s.bgmArchiveName != "" {
		state[musicStateKey] = s.bgmArchiveName
	}
	state[musicVolumeStateKey] = strconv.Itoa(roundToInt(s.bgmVolume * 255))
	return state
}

func (s *Session) restoreAudioState(audioState map[string]string) {
	music := audioState[musicStateKey]
	if music == "" {
		return
	}
	volume := 255
	if text, ok := audioState[musicVolumeStateKey]; ok {
		if parsed, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
			volume = core.Clamp(parsed, 0, 255)
		}
	}
	s.bgmArchiveName = music
	s.bgmVolume = float64(volume) / 255
	s.bgmLoop = true
	if path := s.materializeAudio(music); path != "" {
		s.bgm = path
	} else {
		s.bgm = music
	}
}

func toVolume(volume int) float64 {
	return float64(core.Clamp(volume, 0, 255)) / 255
}

func roundToInt(value float64) int {
	if value >= 0 {
		return int(value + 0.5)
	}
	return int(value - 0.5)
}
