package vm

import (
	"strings"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/core"
)

func (m *Machine) onStart(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("start arguments are ignored")
	}
	return EmonStepContinue
}

func (m *Machine) onEnd(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("end arguments are ignored")
	}
	if len(m.frames) > 1 {
		m.popFileFrame()
		return EmonStepContinue
	}
	return EmonStepFinished
}

func (m *Machine) onScriptEnd(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("script end arguments are ignored")
	}
	return EmonStepFinished
}

func (m *Machine) onScriptReset(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("script reset arguments are ignored")
	}
	m.resetToStart()
	m.SetSystemVariable("TitleFlag", script.EmonValueFromInt(1))
	m.flushSystemState()
	return EmonStepContinue
}

func (m *Machine) onJump(statement *script.EmonStatement) EmonStep {
	target := m.labelText(statement, 0)
	if m.tryMacroReturnJump(target) {
		return EmonStepContinue
	}
	return m.doJump(target)
}

func (m *Machine) onIfJump(statement *script.EmonStatement) EmonStep {
	label := m.labelText(statement, 0)
	if label == "" {
		return EmonStepContinue
	}
	if !m.conditionsFrom(statement, 1) {
		return EmonStepContinue
	}
	if m.tryMacroReturnJump(label) {
		return EmonStepContinue
	}
	return m.doJump(label)
}

func (m *Machine) tryMacroReturnJump(target string) bool {
	destination := strings.Trim(strings.TrimSpace(target), "\"")
	if !isASCIIDigits(destination) {
		return false
	}
	if len(m.frames) == 0 {
		return false
	}
	frame := m.frames[len(m.frames)-1]
	macro := frame.Macro
	if macro == nil || frame.Program.FindLabel(destination) >= 0 {
		return false
	}
	if m.context.Scripts.Contains(destination) {
		return false
	}
	m.logOnce("jump '" + target + "' -> '" + destination + "': macro return from '" + macro.Name + "'")
	m.popFileFrame()
	return true
}

func (m *Machine) onRandomJump(statement *script.EmonStatement) EmonStep {
	destination := m.raw(statement, 0)
	if statement.ArgumentCount() < 2 {
		core.Logf("emon: random jump needs file arguments")
		return EmonStepContinue
	}
	files := []string{}
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		file := strings.Trim(strings.TrimSpace(m.labelText(statement, i)), "\"")
		if file != "" && !strings.EqualFold(file, "NULL") {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return EmonStepContinue
	}
	pick := m.nextReplayPick(destination, len(files))
	if destination != "" && !m.isReplaying() {
		m.SetVariable(destination, script.EmonValueFromInt(int32(pick)))
	}
	chosen := files[pick]
	if m.tryMacroReturnJump(chosen) {
		return EmonStepContinue
	}
	return m.doJump(chosen)
}

func (m *Machine) onSceneName(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		m.logOnce("scene name is empty")
	}
	return EmonStepContinue
}

func (m *Machine) onMacro(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("macro arguments are ignored")
	}
	return EmonStepContinue
}

func (m *Machine) onEndMacro(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("end macro arguments are ignored")
	}
	if len(m.frames) != 0 && m.frames[len(m.frames)-1].Macro != nil {
		m.frames = m.frames[:len(m.frames)-1]
	}
	return EmonStepContinue
}

func (m *Machine) onDefine(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	m.applyAssignments(m.raw(statement, 0), m.SetSymbol)
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		m.applyAssignments(m.raw(statement, i), m.SetSymbol)
	}
	return EmonStepContinue
}

func (m *Machine) onLock(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("lock arguments are ignored")
	}
	m.messageLocked = true
	m.locked = true
	return EmonStepContinue
}

func (m *Machine) onUnlock(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("unlock arguments are ignored")
	}
	m.messageLocked = false
	m.locked = false
	return EmonStepContinue
}

func (m *Machine) onLockSelect(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("lock select arguments are ignored")
	}
	m.selectLocked = true
	return EmonStepContinue
}

func (m *Machine) onUnlockSelect(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("unlock select arguments are ignored")
	}
	m.selectLocked = false
	return EmonStepContinue
}

func (m *Machine) onNoSaveOn(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("no save on arguments are ignored")
	}
	m.noSave = true
	return EmonStepContinue
}

func (m *Machine) onNoSaveOff(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("no save off arguments are ignored")
	}
	m.noSave = false
	return EmonStepContinue
}

func (m *Machine) resetCommon(keepDefinitions bool) {
	if m.backLog != nil {
		backLog := m.backLog
		m.backLog = nil
		m.backLogRevision++
		if backLog.Inner != nil {
			if text := backLog.Inner.Text(); text != nil {
				text.Clear()
			}
		}
	}
	m.frames = nil
	m.context.Variables.Clear()
	m.context.Symbols.Clear()
	m.context.SeedSymbols()
	if !keepDefinitions {
		m.runActions.Clear()
		m.triggers = nil
		m.areas = nil
	}
	m.macroPrograms.Clear()
	m.clearMotions()
	m.clearActionEphemera()
	m.context.Message.Clear()
	m.context.Message.SetVisible(false)
	m.colorSprites.Clear()
	m.context.Audio.StopAll()
	m.soundGate.Lock()
	m.soundCompletions = nil
	m.soundGate.Unlock()
	m.audioState.Clear()
	m.messageLocked = false
	m.locked = false
	m.selectLocked = false
	m.skip = false
	m.skipMode = 0
	m.auto = false
	m.noSave = false
	m.waitFrames = 0
	m.actionWaiting = false
	m.actionWaitFrameDepth = -1
	m.actionWaitFlags = 0
	m.clickWaiting = false
	m.polling = false
	m.effect = nil
	m.prompt = ""
	m.hasPrompt = false
	m.promptVariable = ""
	m.promptIsYesNo = false
	m.pendingJump = ""
	m.movie = ""
	m.hasMovie = false
	m.pendingMessage = ""
	m.hasPendingMessage = false
	m.finished = false
	m.resetRequested = false
	m.replayFrame = nil
	m.replayTarget = 0
	m.replayVariables = nil
	m.step = EmonStepContinue
}

func (m *Machine) resetToStart() {
	m.resetCommon(false)
	m.resetRequested = true
}

func isASCIIDigits(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}
	return true
}
