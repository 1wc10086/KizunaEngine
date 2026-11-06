package session

import (
	"strings"
	"time"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/emon/save"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
)

func (s *Session) Slots() []core.Slot {
	out := make([]core.Slot, 0, saveSlotCount)
	for index := 0; index < saveSlotCount; index++ {
		slot := core.Slot{Index: uint32(index)}
		if s.disposed || s.saves == nil {
			out = append(out, slot)
			continue
		}
		stored := s.saves.Read(index)
		if stored == nil {
			out = append(out, slot)
			continue
		}
		iso := stored.SavedAt.Format(time.RFC3339)
		slot.Title = iso
		slot.SavedAt = iso
		slot.Summary = summarizeStored(stored)
		slot.Exists = true
		out = append(out, slot)
	}
	return out
}

func (s *Session) Save(slot uint32, title string) error {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return errNotOpen
	}
	if !s.gate.TryLock() {
		return errLocked
	}
	defer s.gate.Unlock()
	preview := s.summarize()
	data := s.machine.CaptureSessionState(int(slot), preview)
	if data == nil {
		core.Logf("emon: slot %d refused: no script position", slot)
		return errSaveFail
	}
	unified := &save.EmonSaveData{
		Slot:            data.Slot,
		Comment:         data.Comment,
		SavedAt:         data.SavedAt,
		ScriptFile:      data.ScriptFile,
		StatementIndex:  data.StatementIndex,
		Variables:       data.Variables,
		SystemVariables: data.SystemVariables,
		MacroStack:      data.MacroStack,
		Frames:          data.Frames,
		AudioState:      s.captureAudioState(),
		SpriteStates:    data.SpriteStates,
		Preview:         data.Preview,
	}
	if !s.saves.Write(unified) {
		return errSaveFail
	}
	core.Logf("emon: slot %d written variables=%d macros=%d frames=%d sprites=%d", slot, len(unified.Variables), len(unified.MacroStack), len(unified.Frames), len(unified.SpriteStates))
	return nil
}

func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return core.StepFaulted, errNotOpen
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	stored := s.saves.Read(int(slot))
	if stored == nil {
		return core.StepFaulted, errNoSlot
	}
	s.selectShown = false
	s.selectSig = 0
	s.selectNames = nil
	s.stageSignature = -1
	s.invalidateFrame()
	if !s.machine.RestoreState(stored) {
		return core.StepFaulted, errNoSlot
	}
	s.restoreAudioState(stored.AudioState)
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.machine.Run(composeBudget)
	})
	s.composeFrame()
	core.Logf("emon: slot %d loaded variables=%d frames=%d sprites=%d script='%s'", slot, len(stored.Variables), len(stored.Frames), len(stored.SpriteStates), stored.ScriptFile)
	return s.publish(), nil
}

func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	if s.disposed || s.saves == nil {
		return core.SlotPreview{}, false
	}
	stored := s.saves.Read(int(slot))
	if stored == nil {
		return core.SlotPreview{}, false
	}
	thumb := s.renderPreview(stored)
	pixels := append([]byte(nil), thumb.Pixels()...)
	return core.SlotPreview{
		SavedAt: stored.SavedAt.Format(time.RFC3339),
		Summary: summarizeStored(stored),
		Speaker: stored.Comment,
		Text:    stored.Preview,
		Thumb:   core.Frame{Width: previewWidth, Height: previewHeight, Pixels: pixels},
	}, true
}

func (s *Session) summarize() string {
	text := strings.TrimSpace(strings.ReplaceAll(s.text, "\n", " "))
	if s.speaker != "" {
		text = s.speaker + ": " + text
	}
	return shorten(text)
}

func summarizeStored(stored *save.EmonSaveData) string {
	if stored.Comment != "" {
		return shorten(stored.Comment)
	}
	return shorten(stored.Preview)
}

func shorten(text string) string {
	runes := []rune(text)
	if len(runes) <= summaryLimit {
		return text
	}
	return string(runes[:summaryLimit])
}

func (s *Session) renderPreview(stored *save.EmonSaveData) *bishopgfx.BgraCanvas {
	canvas := bishopgfx.NewBgraCanvas(previewWidth, previewHeight)
	canvas.Clear(24, 18, 12, 255)
	body := stored.Preview
	if body == "" {
		body = stored.Comment
	}
	if body == "" {
		return canvas
	}
	box := emontext.NewEmonMessageBox()
	box.SetVisible(true)
	box.SetBody(body)
	style := emontext.NewEmonTextStyle()
	style.Size = previewFontSize
	style.X = 8
	style.Y = 8
	style.LineHeight = previewLineHeight
	style.MaxCharactersPerLine = previewCharsPerLine
	style.MaxLines = previewLines
	emontext.DrawMessageBox(canvas, box, style, emontext.ResolveGlyphSource(), len([]rune(body)), 255, 255, 255)
	return canvas
}
