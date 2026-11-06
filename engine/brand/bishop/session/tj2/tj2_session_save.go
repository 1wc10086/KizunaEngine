package tj2

import (
	"strings"
	"time"

	"kizuna/engine/core"
)

type Tj2SavePreview struct {
	SavedAt         time.Time
	Speaker         string
	Text            string
	Summary         string
	Thumbnail       []byte
	ThumbnailWidth  int
	ThumbnailHeight int
}

func (s *Tj2Session) ListSlots() []Tj2SaveSlotInfo {
	return s.tj2Saves.List(tj2SaveSlotCount)
}

func (s *Tj2Session) PreviewSlot(slot int) (*Tj2SavePreview, bool) {
	state := s.tj2Saves.Read(slot)
	if state == nil {
		return nil, false
	}
	return &Tj2SavePreview{
		SavedAt:         ticksToTj2Time(state.SavedAtTicks),
		Speaker:         state.Speaker,
		Text:            state.Text,
		Summary:         state.Summary,
		Thumbnail:       state.Thumbnail,
		ThumbnailWidth:  state.ThumbnailWidth,
		ThumbnailHeight: state.ThumbnailHeight,
	}, true
}

func (s *Tj2Session) SaveTo(slot int) bool {
	if s.disposed || s.machine == nil {
		return false
	}
	// 从信息框的 SAVE 按钮进入存档画面时，frame 已经被系统界面覆盖；
	// 缩略图必须在打开界面前就抓好（openClassicSave 会存进 uiSaveThumb）。
	thumbnail := s.uiSaveThumb
	if thumbnail == nil {
		thumbnail = s.frame.ScaleTo(s.profile.SaveThumbnailWidth, s.profile.SaveThumbnailHeight)
	}
	captured := s.machine.Capture()
	standees := append([]Tj2StandeeEntry(nil), s.standees.Entries()...)
	state := &Tj2SaveState{
		ScenarioNumber:    s.machine.ScenarioNumber(),
		Machine:           &captured,
		BackgroundName:    s.stage.BackgroundName(),
		BackgroundOffsetX: s.stage.BackgroundOffsetX(),
		BackgroundOffsetY: s.stage.BackgroundOffsetY(),
		BackgroundHistory: s.stage.BackgroundHistory(),
		Standees:          standees,
		BgmTrack:          s.bgmTrack,
		Speaker:           s.gameState.Speaker(),
		Text:              s.gameState.Text(),
		Summary:           s.summarize(),
		SavedAtTicks:      tj2TimeToTicks(time.Now().UTC()),
		Thumbnail:         append([]byte(nil), thumbnail.Buffer...),
		ThumbnailWidth:    thumbnail.Width,
		ThumbnailHeight:   thumbnail.Height,
		MessagePages:      append([]string(nil), s.messagePages...),
		MessagePage:       s.messagePage,
	}
	written := s.tj2Saves.Write(state, slot)
	core.Logf("tj2: save slot %d written=%v", slot, written)
	return written
}

func (s *Tj2Session) LoadFrom(slot int) bool {
	if s.disposed || s.machine == nil {
		return false
	}
	state := s.tj2Saves.Read(slot)
	if state == nil || state.Machine == nil {
		core.Logf("tj2: save slot %d missing", slot)
		return false
	}
	s.machine.Restore(*state.Machine)
	if s.machine.Faulted() {
		core.Logf("tj2: save slot %d restore faulted", slot)
		return false
	}
	s.bgmTrack = state.BgmTrack
	s.menuVisible = false
	s.tickCounter = 0
	s.timerVisible = false
	s.standees.Clear()
	for _, entry := range state.Standees {
		if len(entry.Name) > 0 {
			s.standees.Register(entry.Layer, entry.Index, entry.Name, entry.OffsetX, entry.OffsetY)
		}
	}
	s.restoreLayers(state)
	s.gameState.SetSpeaker(state.Speaker)
	s.gameState.SetText(state.Text)
	s.gameState.SetMessageWindowVisible(true)
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	s.messageHover = 0
	s.titleMode = false
	// 版本 6 起会保存 !s 的分页位置；读旧档时没有分页数据，
	// 就只显示存档里的当前页，后续点击直接继续脚本。
	if len(state.MessagePages) > 0 {
		s.messagePages = append([]string(nil), state.MessagePages...)
		page := state.MessagePage
		if page < 0 || page >= len(s.messagePages) {
			page = 0
		}
		s.messagePage = page
		s.gameState.SetText(s.messagePages[page])
	} else {
		s.messagePages = nil
		s.messagePage = 0
	}
	s.stage.Invalidate()
	if s.bgmTrack >= 0 {
		if path := s.bgm.Resolve(s.bgmTrack); path != "" {
			s.gameState.SetBgm(path)
		} else {
			s.gameState.ClearBgm()
		}
	} else {
		s.gameState.ClearBgm()
	}
	s.gameState.SetBgmLoop(true)
	s.gameState.ClearSoundEffect()
	core.Logf("tj2: save slot %d loaded scenario %d", slot, state.ScenarioNumber)
	return true
}

func (s *Tj2Session) restoreLayers(state *Tj2SaveState) {
	s.stage.ClearAll()
	s.stage.SetMessageWindow(s.loadMessageWindow())
	if len(state.BackgroundHistory) > 0 {
		s.stage.ReplayBackgroundHistory(state.BackgroundHistory)
	} else if len(state.BackgroundName) > 0 {
		background := s.images.Get(state.BackgroundName)
		if background == nil {
			core.Logf("tj2: save background '%s' missing", state.BackgroundName)
		} else {
			s.stage.SetBackground(state.BackgroundName, background, state.BackgroundOffsetX, state.BackgroundOffsetY)
		}
	}
	s.rebuildStandees()
}

func (s *Tj2Session) summarize() string {
	text := strings.TrimSpace(strings.ReplaceAll(s.gameState.Text(), "\n", " "))
	if len(s.gameState.Speaker()) > 0 {
		text = s.gameState.Speaker() + ": " + text
	}
	runes := []rune(text)
	if len(runes) <= 42 {
		return text
	}
	return string(runes[:42])
}
