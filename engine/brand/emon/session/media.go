package session

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
)

func (s *Session) Scenes() []core.Scene {
	return nil
}

func (s *Session) Text() core.TextState {
	var awaiting uint32
	if s.awaitingInput {
		awaiting = 1
	}
	return core.TextState{Speaker: s.speaker, Text: s.text, Awaiting: awaiting}
}

func (s *Session) Choices() []core.Choice {
	if len(s.selectNames) == 0 {
		return nil
	}
	out := make([]core.Choice, 0, len(s.selectNames))
	for index, name := range s.selectNames {
		out = append(out, core.Choice{Name: name, Value: int32(index)})
	}
	return out
}

func (s *Session) State() core.State {
	if s.machine == nil {
		return core.State{}
	}
	pc := s.machine.CurrentStatement()
	if pc < 0 {
		pc = 0
	}
	start := s.instruction
	if start < 0 {
		start = 0
	}
	var prompt string
	if value, ok := s.machine.PendingPrompt(); ok {
		prompt = value
	}
	visible := false
	if s.message != nil {
		visible = s.message.Visible() || len(s.message.Body()) > 0
	}
	return core.State{
		Bgm:                  s.bgm,
		PC:                   uint32(pc),
		Scene:                uint32(start),
		Animating:            s.isAnimating(),
		WaitMs:               0,
		MessageWindowVisible: visible,
		AwaitingName:         s.awaitingName(),
		BackLogActive:        s.machine.BackLogActive(),
		ReplayActive:         s.machine.Replaying(),
		InstructionStart:     uint32(start),
		NamePrompt:           prompt,
	}
}

func (s *Session) TextList() []core.TextUnit {
	return nil
}

func (s *Session) LastError() string {
	return strings.Join(s.trace, "\n")
}

func (s *Session) AssetSize(path string) int {
	return len(s.readAssetBytes(path))
}

func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if dst == nil || n <= 0 {
		return 0
	}
	raw := s.readAssetBytes(path)
	if len(raw) == 0 {
		return 0
	}
	return copy(unsafe.Slice((*byte)(dst), n), raw)
}

func (s *Session) readAssetBytes(path string) []byte {
	name := strings.TrimSpace(path)
	if name == "" {
		return nil
	}
	if filepath.IsAbs(name) {
		content, err := os.ReadFile(name)
		if err != nil {
			return nil
		}
		return content
	}
	if s.archiveDir != "" {
		if content, err := os.ReadFile(filepath.Join(s.archiveDir, name)); err == nil {
			return content
		}
	}
	if s.archives != nil {
		return s.archives.Read(name)
	}
	return nil
}

func (s *Session) ComposeImages(names []string) (core.Frame, bool) {
	canvas := bishopgfx.NewBgraCanvas(s.width, s.height)
	canvas.Clear(0, 0, 0, 255)
	drawn := false
	for _, name := range names {
		image := s.loadImage(name)
		if image == nil {
			continue
		}
		drawn = true
		canvas.Draw(image, image.OffsetX, image.OffsetY)
	}
	if !drawn {
		return core.Frame{}, false
	}
	return core.Frame{Width: canvas.Width, Height: canvas.Height, Pixels: append([]byte(nil), canvas.Pixels()...)}, true
}

func (s *Session) SetPreview(f core.Frame) {
	if s.frame == nil || f.Width != s.width || f.Height != s.height {
		return
	}
	if len(f.Pixels) < s.width*s.height*4 {
		return
	}
	pixels := s.frame.Pixels()
	for i := 0; i < s.width*s.height; i++ {
		base := i * 4
		pixels[base] = f.Pixels[base+2]
		pixels[base+1] = f.Pixels[base+1]
		pixels[base+2] = f.Pixels[base]
		pixels[base+3] = f.Pixels[base+3]
	}
	s.invalidateFrame()
}

func (s *Session) SelectFrame(name string) (core.Frame, bool) {
	_ = name
	return core.Frame{}, false
}

func (s *Session) CgGroups() []core.CgGroup {
	return nil
}

func (s *Session) awaitingName() bool {
	if s.disposed || s.machine == nil {
		return false
	}
	if s.step == vm.EmonStepAwaitName {
		return true
	}
	_, ok := s.machine.PendingPrompt()
	return ok
}

func (s *Session) SubmitName(name string) bool {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return false
	}
	if !s.awaitingName() {
		return false
	}
	if strings.TrimSpace(name) == "" {
		name = defaultName(func(candidate string) string {
			return s.machine.Lookup(candidate).AsString()
		})
	}
	if !s.gate.TryLock() {
		return false
	}
	defer s.gate.Unlock()
	s.machine.SubmitText(name)
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	faulted := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				core.Logf("emon: submit failed: %v", recovered)
				faulted = true
			}
		}()
		s.step = s.machine.Run(commandBudget)
	}()
	if faulted {
		return false
	}
	s.publish()
	return true
}

func defaultName(lookup func(string) string) string {
	if lookup == nil {
		return "主人公"
	}
	candidates := []string{"主人公名前", "主人公苗字", "主人公初期名前", "主人公初期苗字"}
	for _, candidate := range candidates {
		var current string
		func() {
			defer func() {
				_ = recover()
			}()
			current = lookup(candidate)
		}()
		if current != "" && current != candidate {
			return current
		}
	}
	return "主人公"
}

func (s *Session) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true
	if s.audio != nil {
		s.audio.CommandIssued = nil
	}
	deadline := time.Now().Add(disposeGateWaitMs * time.Millisecond)
	for {
		if s.gate.TryLock() {
			break
		}
		if time.Now().After(deadline) {
			core.Logf("emon: dispose gate held %dms, machine and archive caches left unreleased", disposeGateWaitMs)
			if s.audio != nil {
				s.audio.Dispose()
			}
			emontext.ClearCache()
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer s.gate.Unlock()
	if s.audio != nil {
		s.audio.Dispose()
	}
	if s.machine != nil {
		s.machine.ReleaseCaches()
	}
	if s.archives != nil {
		s.archives.Dispose()
	}
	if s.masks != nil {
		s.masks.Clear()
	}
	emontext.ClearCache()
	s.opened = false
	return nil
}
