package vm

import (
	"sort"
	"strings"
	"time"

	"kizuna/engine/brand/emon/save"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	"kizuna/engine/core"
)

func (m *Machine) onNull(statement *script.EmonStatement) EmonStep {
	return EmonStepContinue
}

func (m *Machine) CaptureSessionState(slot int, comment string) *EmonSaveData {
	anchor := -1
	for i := len(m.frames) - 1; i >= 0; i-- {
		if m.frames[i].Macro == nil {
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return nil
	}
	stack := []*script.EmonMacroFrameState{}
	for i := anchor + 1; i < len(m.frames); i++ {
		if m.frames[i].Macro == nil {
			continue
		}
		state := m.frames[i].Macro.ToState()
		stack = append(stack, &script.EmonMacroFrameState{
			Name:            state.Name,
			ReturnFile:      state.ReturnFile,
			CallerStatement: m.frames[i].Index,
			ReturnStatement: m.frames[i].Index,
			Arguments:       copyEmonValues(state.Arguments),
		})
	}
	chain := []*save.EmonFrameState{}
	for _, frame := range m.frames {
		record := &save.EmonFrameState{Program: frame.Program.Name, Index: frame.Index}
		if frame.Macro != nil {
			state := frame.Macro.ToState()
			record.Macro = state.Name
			record.CallerProgram = state.ReturnFile
			record.CallerStatement = state.CallerStatement
			record.ReturnStatement = state.ReturnStatement
			record.Arguments = copyEmonValues(state.Arguments)
		}
		chain = append(chain, record)
	}
	return &EmonSaveData{
		Slot:           int32(slot),
		Comment:        comment,
		SavedAt:        time.Now(),
		ScriptFile:     m.frames[anchor].Program.Name,
		StatementIndex: m.frames[anchor].Index,
		Variables:      m.context.Variables.Snapshot(),
		MacroStack:     stack,
		Frames:         chain,
		AudioState:     m.mutableAudioState().Snapshot(),
		SpriteStates:   m.captureSpriteStates(),
		Preview:        m.context.Message.Body(),
	}
}

func (m *Machine) CaptureSystemState() map[string]script.EmonValue {
	return m.context.SystemVariables.Snapshot()
}

func copyEmonValues(entries map[string]script.EmonValue) map[string]script.EmonValue {
	out := make(map[string]script.EmonValue, len(entries))
	for name, value := range entries {
		out[name] = value
	}
	return out
}

func (m *Machine) captureSpriteStates() []*save.EmonSpriteState {
	list := []*save.EmonSpriteState{}
	for _, sprite := range m.context.Stage.All() {
		if sprite.Name() == "" {
			continue
		}
		state := &save.EmonSpriteState{
			Name:    sprite.Name(),
			Visible: sprite.Visible(),
			Flags:   sprite.Flags(),
			X:       int32(sprite.X()),
			Y:       int32(sprite.Y()),
			Width:   int32(sprite.Width()),
			Height:  int32(sprite.Height()),
			Kind:    sprite.Kind(),
			Alpha:   int32(sprite.Alpha()),
		}
		if f3d := sprite.F3D(); f3d != nil {
			snapshot := f3d.Snapshot()
			state.F3D = &snapshot
		}
		list = append(list, state)
	}
	return list
}

func (m *Machine) EnsureSelectOrder(sprite *stage.EmonSprite) {
	if sprite.Flags()&stage.EmonSpriteFlagsSelect != 0 && sprite.SelectOrder() == 0 {
		m.selectOrderSequence++
		sprite.SetSelectOrder(m.selectOrderSequence)
	}
}

func (m *Machine) OrderedSelectSprites() []*stage.EmonSprite {
	for _, sprite := range m.context.Stage.All() {
		if _, ok := m.spriteCreationOrder.Get(sprite.Name()); !ok {
			m.spriteCreationOrder.Set(sprite.Name(), m.spriteCreationSequence)
			m.spriteCreationSequence++
		}
		m.EnsureSelectOrder(sprite)
	}
	selected := []*stage.EmonSprite{}
	for _, sprite := range m.context.Stage.All() {
		if sprite.Flags()&stage.EmonSpriteFlagsSelect == 0 || !m.isChainVisible(sprite) {
			continue
		}
		selected = append(selected, sprite)
	}
	orders := make(map[*stage.EmonSprite]int, len(selected))
	created := make(map[*stage.EmonSprite]int, len(selected))
	for _, sprite := range selected {
		orders[sprite] = selectSortKey(sprite)
		if order, ok := m.spriteCreationOrder.Get(sprite.Name()); ok {
			created[sprite] = order
		}
	}
	sort.SliceStable(selected, func(a, b int) bool {
		if orders[selected[a]] != orders[selected[b]] {
			return orders[selected[a]] < orders[selected[b]]
		}
		if created[selected[a]] != created[selected[b]] {
			return created[selected[a]] < created[selected[b]]
		}
		return strings.ToLower(selected[a].Name()) < strings.ToLower(selected[b].Name())
	})
	return selected
}

func selectSortKey(sprite *stage.EmonSprite) int {
	if sprite.SelectOrder() == 0 {
		return 2147483647
	}
	return sprite.SelectOrder()
}

func (m *Machine) RestoreState(data *EmonSaveData) bool {
	if data.ScriptFile == "" {
		core.Logf("emon: load script is empty")
		return false
	}
	chained := len(data.Frames) > 0
	var program *script.EmonProgram
	if !chained {
		program = m.context.Scripts.Load(data.ScriptFile)
		if program == nil {
			core.Logf("emon: load script '%s' was not found", data.ScriptFile)
			return false
		}
	}
	m.resetCommon(true)
	m.context.Variables.LoadFrom(data.Variables)
	m.dropShadowedLocalVariables()
	if chained {
		if !m.pushFrameChain(data.Frames) {
			m.frames = nil
			return false
		}
		core.Logf("emon: load restored frame chain of %d frames with %d sprite states", len(m.frames), len(data.SpriteStates))
	} else {
		m.frames = append(m.frames, &EmonFrame{Program: program})
		for _, state := range data.MacroStack {
			if state == nil {
				continue
			}
			definition, ok := m.context.Macros.TryGet(state.Name)
			if !ok {
				core.Logf("emon: load macro '%s' was not found", state.Name)
				m.frames = nil
				return false
			}
			macroProgram := m.getMacroProgram(definition)
			if macroProgram == nil {
				m.frames = nil
				return false
			}
			frame := &EmonFrame{Program: macroProgram, Macro: script.EmonMacroFrameFromState(state)}
			frame.Index = core.ClampInt32(state.ReturnStatement, 0, int32(len(macroProgram.Statements)))
			m.frames = append(m.frames, frame)
		}
	}
	if len(m.frames) > maxFrames {
		core.Logf("emon: load frame stack is too deep")
		m.frames = nil
		return false
	}
	m.mutableAudioState().Clear()
	for key, value := range data.AudioState {
		m.mutableAudioState().Set(key, value)
	}
	m.applySpriteStates(data.SpriteStates)
	m.mouseFollow = nil
	m.clearMotions()
	m.effect = nil
	m.actionWaiting = false
	m.actionWaitFrameDepth = -1
	m.actionWaitFlags = 0
	m.clickWaiting = false
	m.polling = false
	m.waitFrames = 0
	m.pointerDown = false
	m.prompt = ""
	m.hasPrompt = false
	m.promptVariable = ""
	m.promptIsYesNo = false
	m.pendingJump = ""
	m.movie = ""
	m.hasMovie = false
	m.pendingMessage = ""
	m.hasPendingMessage = false
	m.context.Message.Clear()
	m.finished = false
	m.step = EmonStepContinue
	if chained {
		m.reissueAudio()
	} else if len(data.MacroStack) == 0 {
		m.replayFrame = m.frames[0]
		m.replayTarget = int(core.ClampInt32(data.StatementIndex, 0, int32(len(program.Statements))))
		m.replayVariables = copyEmonValues(data.Variables)
		core.Logf("emon: load has no frame chain for '%s', replaying to statement %d (legacy save; '#End' will end the game instead of returning to the caller)", data.ScriptFile, data.StatementIndex)
	} else {
		m.frames[0].Index = core.ClampInt32(data.StatementIndex, 0, int32(len(program.Statements)))
		m.reissueAudio()
	}
	return true
}

func (m *Machine) pushFrameChain(states []*save.EmonFrameState) bool {
	for _, state := range states {
		if state == nil {
			continue
		}
		if len(m.frames) >= maxFrames {
			core.Logf("emon: load frame stack is too deep")
			return false
		}
		if state.Macro == "" {
			program := m.context.Scripts.Load(state.Program)
			if program == nil {
				core.Logf("emon: load script '%s' was not found", state.Program)
				return false
			}
			frame := &EmonFrame{Program: program}
			frame.Index = core.ClampInt32(state.Index, 0, int32(len(program.Statements)))
			m.frames = append(m.frames, frame)
			continue
		}
		definition, ok := m.context.Macros.TryGet(state.Macro)
		if !ok {
			core.Logf("emon: load macro '%s' was not found", state.Macro)
			return false
		}
		macroProgram := m.getMacroProgram(definition)
		if macroProgram == nil {
			core.Logf("emon: load macro '%s' could not be parsed", state.Macro)
			return false
		}
		arguments := state.Arguments
		if arguments == nil {
			arguments = map[string]script.EmonValue{}
		}
		frame := &EmonFrame{Program: macroProgram, Macro: script.EmonMacroFrameFromState(&script.EmonMacroFrameState{
			Name:            state.Macro,
			ReturnFile:      state.CallerProgram,
			CallerStatement: state.CallerStatement,
			ReturnStatement: state.ReturnStatement,
			Arguments:       arguments,
		})}
		frame.Index = core.ClampInt32(state.Index, 0, int32(len(macroProgram.Statements)))
		m.frames = append(m.frames, frame)
	}
	if len(m.frames) == 0 {
		core.Logf("emon: load frame stack is empty")
		return false
	}
	return true
}

func (m *Machine) applySpriteStates(states []*save.EmonSpriteState) {
	if len(states) == 0 {
		return
	}
	const bands = stage.EmonSpriteKindBack | stage.EmonSpriteKindMiddleFront | stage.EmonSpriteKindMiddleBack
	for _, sprite := range m.context.Stage.All() {
		if sprite.Name() != "" && sprite.Visible() {
			sprite.SetVisible(false)
		}
	}
	unmatched := 0
	for _, state := range states {
		if state == nil || state.Name == "" {
			continue
		}
		sprite := m.context.Stage.Find(state.Name)
		if sprite == nil {
			unmatched++
			continue
		}
		sprite.SetFlags(state.Flags)
		sprite.SetX(int(state.X))
		sprite.SetY(int(state.Y))
		if state.Width > 0 {
			sprite.SetWidth(int(state.Width))
		}
		if state.Height > 0 {
			sprite.SetHeight(int(state.Height))
		}
		placement := sprite.Kind() & bands
		sprite.SetKind(state.Kind)
		if placement != state.Kind&bands {
			m.context.Stage.Reorder(sprite, state.Kind)
		}
		sprite.SetAlpha(int(state.Alpha))
		if state.F3D != nil {
			if sprite.F3D() == nil {
				sprite.SetF3D(stage.NewEmonF3DState())
			}
			sprite.F3D().Restore(*state.F3D)
		} else {
			sprite.SetF3D(nil)
		}
	}
	if unmatched > 0 {
		noun := "ies"
		if unmatched == 1 {
			noun = "y"
		}
		core.Logf("emon: load skipped %d sprite state entr%s not present in the stage", unmatched, noun)
	}
}
