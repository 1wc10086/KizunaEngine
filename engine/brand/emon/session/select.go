package session

import (
	"kizuna/engine/brand/emon/stage"
	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
)

func (s *Session) MapButtons() []core.MapButton {
	if s.stage == nil || s.machine == nil {
		return nil
	}
	if !s.gate.TryLock() {
		return nil
	}
	defer s.gate.Unlock()
	visible := s.visibleSelectSprites()
	if len(visible) == 0 {
		return nil
	}
	out := make([]core.MapButton, 0, len(visible))
	for index, sprite := range visible {
		rect := sprite.HitRect()
		width := rect.Width
		height := rect.Height
		if width <= 0 || height <= 0 {
			width = selectItemMinimum
			height = selectItemMinimum
		}
		out = append(out, core.MapButton{
			Index:  uint32(index),
			Image:  sprite.Name(),
			X:      int32(sprite.AbsoluteX()),
			Y:      int32(sprite.AbsoluteY()),
			Width:  uint32(width),
			Height: uint32(height),
			Value:  int32(index + 1),
			Label:  sprite.Name(),
		})
	}
	return out
}

func (s *Session) MapMask() (bool, int, int) {
	if !s.hasSelectLocked() {
		return false, 0, 0
	}
	return true, s.width, s.height
}

func (s *Session) visibleSelectSprites() []*stage.EmonSprite {
	if s.stage == nil {
		return nil
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return nil
	}
	if s.machine != nil && s.machine.BackLogActive() {
		return nil
	}
	var visible []*stage.EmonSprite
	for _, sprite := range s.stage.SelectSprites() {
		if sprite.Visible() && isChainVisible(sprite) {
			visible = append(visible, sprite)
		}
	}
	return visible
}

func (s *Session) publishSelect() {
	visible := s.visibleSelectSprites()
	if len(visible) == 0 {
		if s.selectShown {
			s.selectNames = nil
			s.selectSig = 0
			s.selectShown = false
		}
		return
	}
	signature := selectSignature(visible)
	if s.selectShown && signature == s.selectSig {
		return
	}
	s.selectNames = s.selectNames[:0]
	for _, sprite := range visible {
		s.selectNames = append(s.selectNames, sprite.Name())
	}
	s.selectSig = signature
	s.selectShown = true
	core.Logf("emon: select window %d item(s)", len(visible))
}

func selectSignature(sprites []*stage.EmonSprite) int {
	hash := 17
	hash = hash*31 + len(sprites)
	for _, sprite := range sprites {
		for _, c := range sprite.Name() {
			hash = hash*31 + int(c)
		}
		hash = hash*31 + sprite.AbsoluteX()
		hash = hash*31 + sprite.AbsoluteY()
	}
	return hash
}

func (s *Session) resolveChoicePosition(index int) int {
	if len(s.selectNames) == 0 {
		return -1
	}
	if index >= 0 && index < len(s.selectNames) {
		return index
	}
	if index >= 1 && index <= len(s.selectNames) {
		return index - 1
	}
	return -1
}

func (s *Session) selectPosition(position int) vm.EmonStep {
	if s.machine == nil || position < 0 || position >= len(s.selectNames) {
		return s.step
	}
	name := s.selectNames[position]
	if name != "" {
		return s.machine.ChooseSelectSprite(name)
	}
	return s.machine.ChooseSelect(position + 1)
}

func (s *Session) hitButton(x, y int) int {
	visible := s.visibleSelectSprites()
	for index, sprite := range visible {
		rect := sprite.HitRect()
		width := rect.Width
		height := rect.Height
		if width <= 0 || height <= 0 {
			width = selectItemMinimum
			height = selectItemMinimum
		}
		left := sprite.AbsoluteX()
		top := sprite.AbsoluteY()
		if x >= left && y >= top && x < left+width && y < top+height {
			return index
		}
	}
	return -1
}
