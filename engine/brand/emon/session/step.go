package session

import (
	"time"

	"kizuna/engine/brand/emon/stage"
	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
)

func (s *Session) Run(budget uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	steps := int(budget)
	if steps <= 0 {
		steps = commandBudget
	}
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.machine.Run(steps)
	})
	return s.publish(), nil
}

func (s *Session) Advance() (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	if s.machine.MessageLocked() && s.step == vm.EmonStepAwaitInput {
		s.pump()
		return s.publish(), nil
	}
	s.awaitingInput = false
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.machine.Advance()
	})
	return s.publish(), nil
}

func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil || s.ctx == nil {
		return core.StepFaulted, nil
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return core.StepContinue, nil
	}
	if s.machine.SelectLocked() {
		return s.mapped, nil
	}
	position := s.resolveChoicePosition(int(index))
	if position < 0 {
		core.Logf("emon: select index %d rejected", index)
		return s.mapped, nil
	}
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	s.awaitingInput = false
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	s.step = s.guard(func() vm.EmonStep {
		return s.selectPosition(position)
	})
	return s.publish(), nil
}

func (s *Session) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	switch op {
	case 5:
		return core.StepContinue, nil
	case 0:
		if !s.gate.TryLock() {
			return s.mapped, nil
		}
		s.machine.SetPointer(int(x), int(y))
		s.invalidateFrame()
		mapped := s.mapped
		s.gate.Unlock()
		return mapped, nil
	case 1:
		return s.dispatchInput(func() vm.EmonStep {
			if position := s.hitButton(int(x), int(y)); position >= 0 {
				return s.selectPosition(position)
			}
			s.machine.PointerPress(int(x), int(y))
			return s.machine.Step()
		})
	case 2:
		return s.dispatchInput(func() vm.EmonStep {
			s.machine.PointerRelease(int(x), int(y))
			return s.machine.Step()
		})
	case 3:
		return s.dispatchInput(func() vm.EmonStep {
			if position := s.hitButton(int(x), int(y)); position >= 0 {
				return s.selectPosition(position)
			}
			return s.machine.Tap(int(x), int(y), false)
		})
	case 4:
		return s.dispatchInput(func() vm.EmonStep {
			s.machine.RightClick()
			return s.machine.Step()
		})
	default:
		return s.mapped, nil
	}
}

func (s *Session) PressKey(code uint32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	return s.dispatchInput(func() vm.EmonStep {
		s.machine.PressKey(int(code))
		return s.machine.Step()
	})
}

func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.machine == nil || s.ctx == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	return s.dispatchInput(func() vm.EmonStep {
		s.machine.Wheel(int(delta))
		return s.machine.Step()
	})
}

func (s *Session) dispatchInput(input func() vm.EmonStep) (core.Step, error) {
	if !s.gate.TryLock() {
		return s.mapped, nil
	}
	defer s.gate.Unlock()
	s.pump()
	s.instruction = s.machine.CurrentStatement()
	backLog := s.machine.BackLogActive()
	var next vm.EmonStep
	faulted := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				core.Logf("emon: input failed: %v", recovered)
				faulted = true
			}
		}()
		next = input()
	}()
	if faulted {
		return core.StepFaulted, nil
	}
	if backLog && !s.machine.BackLogActive() {
		s.awaitingInput = false
		s.step = s.guard(func() vm.EmonStep {
			return s.machine.Advance()
		})
	} else {
		s.step = next
	}
	return s.publish(), nil
}

func (s *Session) guard(op func() vm.EmonStep) (step vm.EmonStep) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("emon: run failed: %v", recovered)
			s.record("run failed")
			step = vm.EmonStepFaulted
		}
	}()
	return op()
}

func (s *Session) pump() {
	if s.ctx == nil || s.message == nil || s.stage == nil || s.composer == nil || s.audio == nil || s.machine == nil {
		return
	}
	delta := s.takeDelta()
	s.ctx.FrameDelta = delta
	s.composer.SetFrameDelta(delta)
	s.audio.Update(delta)
	s.message.Advance(delta, messageCharsPerSec)
	s.machine.PumpFrame()
	s.stage.Advance(delta)
}

func (s *Session) publish() core.Step {
	s.composeFrame()
	s.mapped = mapEmonStep(s.step, s.machine.BackLogActive(), s.hasSelectLocked())
	return s.mapped
}

func (s *Session) refresh() {
	if s.message == nil {
		return
	}
	s.speaker = s.message.Speaker()
	body := s.message.Body()
	if body == "" {
		s.text = ""
	} else {
		s.text = s.message.RevealedText()
	}
	s.awaitingInput = s.step == vm.EmonStepAwaitInput || s.step == vm.EmonStepAwaitAction || s.step == vm.EmonStepAwaitName
	s.publishSelect()
}

func (s *Session) composeFrame() {
	if s.ctx == nil || s.stage == nil || s.composer == nil || s.frame == nil {
		return
	}
	s.refresh()
	signature := s.pictureSignature()
	if s.composer.Effect() == nil && signature == s.presentSig {
		return
	}
	s.presentSig = signature
	s.composer.Compose(s.stage, s.frame)
	s.frameSerial++
}

func (s *Session) invalidateFrame() {
	s.presentSig = -1
}

func (s *Session) pictureSignature() int {
	hash := 17
	hash = hash*31 + s.stageSignatureLocked()
	hash = hash*31 + int(s.step)
	if s.selectShown {
		hash = hash*31 + 1
	}
	hash = hash*31 + s.selectSig
	if s.composer != nil {
		hash = hash*31 + s.composer.Revision()
	}
	if s.machine != nil {
		hash = hash*31 + s.machine.BackLogRevision()
	}
	if s.message != nil {
		hash = hash*31 + s.message.Revision()
		hash = hash*31 + s.message.RevealCount()
		hash = hash*31 + len(s.message.Body())
		if s.composer != nil {
			if effect := s.composer.Effect(); effect != nil {
				hash = hash*31 + int(effect.Kind())
			}
		}
	}
	return hash
}

func (s *Session) stageSignatureLocked() int {
	if s.stage == nil {
		return 0
	}
	hash := 17
	for _, sprite := range s.stage.All() {
		for _, c := range sprite.Name() {
			hash = hash*31 + int(c)
		}
		hash = hash*31 + sprite.X()
		hash = hash*31 + sprite.Y()
		hash = hash*31 + sprite.Alpha()
		if sprite.Visible() {
			hash = hash*31 + 1
		}
		hash = hash*31 + sprite.TextRevision()
		if image := sprite.ResolveImage(); image != nil {
			hash = hash*31 + image.Width
			hash = hash*31 + image.Height
		} else {
			hash = hash*31 + 7
		}
	}
	s.stageSignature = hash
	return hash
}

func (s *Session) takeDelta() float64 {
	now := time.Since(s.clock).Seconds()
	delta := now - s.lastSeconds
	s.lastSeconds = now
	if delta <= 0 {
		return 0
	}
	if delta > maximumDelta {
		return maximumDelta
	}
	return delta
}

func mapEmonStep(step vm.EmonStep, backLogActive bool, hasSelect bool) core.Step {
	if backLogActive {
		return core.StepWait
	}
	if (step == vm.EmonStepAwaitChoice || step == vm.EmonStepAwaitAction) && hasSelect {
		return core.StepAwaitSelect
	}
	switch step {
	case vm.EmonStepContinue:
		return core.StepContinue
	case vm.EmonStepAwaitInput:
		return core.StepAwaitInput
	case vm.EmonStepAwaitName:
		return core.StepAwaitName
	case vm.EmonStepAwaitChoice:
		return core.StepAwaitChoice
	case vm.EmonStepAwaitAction:
		return core.StepAwaitSelect
	case vm.EmonStepWait:
		return core.StepWait
	case vm.EmonStepFinished:
		return core.StepFinished
	default:
		return core.StepFaulted
	}
}

func (s *Session) isAnimating() bool {
	if s.disposed || s.machine == nil || s.ctx == nil {
		return false
	}
	if s.step == vm.EmonStepFinished || s.step == vm.EmonStepFaulted {
		return false
	}
	if s.step == vm.EmonStepContinue || s.step == vm.EmonStepWait {
		return true
	}
	if s.composer != nil && s.composer.Effect() != nil {
		return true
	}
	if s.machine.MotionsActive() {
		return true
	}
	return s.message != nil && s.message.Visible() && !s.message.FullyRevealed()
}

func (s *Session) hasSelectLocked() bool {
	if s.disposed || s.ctx == nil || s.stage == nil {
		return false
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return false
	}
	if s.machine != nil && s.machine.BackLogActive() {
		return false
	}
	return countVisibleSelectSprites(s.stage) > 0
}

func countVisibleSelectSprites(stg *stage.EmonStage) int {
	count := 0
	for _, sprite := range stg.SelectSprites() {
		if sprite.Visible() && isChainVisible(sprite) {
			count++
		}
	}
	return count
}

func isChainVisible(sprite *stage.EmonSprite) bool {
	for current := sprite.Parent(); current != nil; current = current.Parent() {
		if !current.Visible() {
			return false
		}
	}
	return true
}
