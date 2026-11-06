package bs

import (
	"kizuna/engine/core"
)

const (
	revImmediateZone    = 0x0100
	revImmediateAltZone = 0x0101
	revVariableZone     = 0x0102
	revPropertyZone     = 0x0103
	revFrameZone        = 0x0104
	revFlagZone         = 0x0105
)

func (m *RevScriptMachine) tryRunInternal(opcode byte) bool {
	start := m.instruction
	dialect := m.dialect
	if opcode >= 0x03 && opcode <= 0x08 && m.tryRunFlow(opcode, start) {
		return true
	}
	switch {
	case opcode == 0x08 && dialect == BsScriptRev2:
		m.returnFromCall()
		return true
	case opcode == 0x09 && dialect == BsScriptRev2:
		m.stackTop = 0
		m.pc = start + 1
		return true
	case opcode == 0x09:
		m.returnFromCall()
		return true
	case opcode == 0x0A && dialect != BsScriptRev2:
		m.stackTop = 0
		m.pc = start + 1
		return true
	}
	if RevIsNoOperation(dialect, opcode) {
		m.pc = start + 1
		return true
	}
	if RevIsCallback(dialect, opcode) {
		m.pushCallback(start)
		return true
	}
	if RevIsAccumulatorLabel(dialect, opcode) {
		m.accumulator = m.readInt32(start + 1)
		if m.watch == start {
			m.watch = -1
		}
		m.pc = start + RevStackSize
		return true
	}
	if RevIsNegate(dialect, opcode) {
		m.executeNegate(start)
		return true
	}
	if RevIsBinary(dialect, opcode) || opcode == RevAssignmentOpcode(dialect) {
		m.executeBinary(opcode, start)
		return true
	}
	return false
}

func (m *RevScriptMachine) tryRunFlow(opcode byte, start int) bool {
	if m.dialect == BsScriptRev2 {
		switch opcode {
		case 0x03:
			m.jumpToLabel(m.readInt32(start + 1))
			return true
		case 0x04:
			m.jumpToLabelOrAdvance(m.readInt32(start+1), m.flagValue() != 0, start)
			return true
		case 0x05:
			m.jumpToLabelOrAdvance(m.readInt32(start+1), m.flagValue() == 0, start)
			return true
		case 0x06:
			m.setWatch(m.readInt32(start + 1))
			m.pc = start + RevStackSize
			return true
		case 0x07:
			m.callLabel(m.readInt32(start+1), start)
			return true
		}
		return false
	}
	switch opcode {
	case 0x03:
		m.jumpToLabel(m.readInt32(start + 1))
		return true
	case 0x04:
		m.jumpAbsolute(m.readInt32(start + 1))
		return true
	case 0x05:
		m.jumpOrAdvance(m.readInt32(start+1), m.flagValue() != 0, start)
		return true
	case 0x06:
		m.jumpOrAdvance(m.readInt32(start+1), m.flagValue() == 0, start)
		return true
	case 0x07:
		m.setWatch(m.readInt32(start + 1))
		m.pc = start + RevStackSize
		if m.dialect == BsScriptRev4 && !m.watchLog {
			m.watchLog = true
			core.Logf("script: rev4 watch does not advance the pc in the original vm, advancing 5")
		}
		return true
	case 0x08:
		m.callLabel(m.readInt32(start+1), start)
		return true
	}
	return false
}

func (m *RevScriptMachine) decode(opcode byte) BsCommand {
	start := m.instruction
	if RevIsMessage(m.dialect, opcode) {
		return m.decodeMessage(start)
	}
	switch opcode {
	case 0x00:
		m.Reset()
		return BsCommand{Kind: BsCommandKindRestart}
	case 0x01:
		m.pc = start + 1
		return BsCommand{Kind: BsCommandKindQuit}
	case 0x02:
		core.Logf("script: rev opcode 0x02 needs host handling at %d", start)
		m.halted = true
		return revHaltCommand
	}
	m.pc = start + 1
	return m.project(opcode, start)
}

func (m *RevScriptMachine) decodeMessage(start int) BsCommand {
	count := int32(m.byteAt(start + 1))
	messageId := m.readInt32(start + 2)
	characterId := int32(-1)
	voiceId := int32(-1)
	graphicId := int32(-1)
	if count >= 1 {
		characterId = m.readInt32(start + 6)
	}
	if count >= 2 {
		voiceId = m.readInt32(start + 10)
	}
	if count >= 3 {
		graphicId = m.readInt32(start + 14)
	}
	m.pc = start + RevMessageBaseSize + RevMessageArgumentSize*int(count)
	if count > RevMessageArgumentLimit {
		core.Logf("script: rev message argument count %d at %d exceeds %d", count, start, RevMessageArgumentLimit)
	}
	return BsCommand{Kind: BsCommandKindMessage, First: messageId, Second: characterId, Third: voiceId, Fourth: graphicId}
}

func (m *RevScriptMachine) project(opcode byte, start int) BsCommand {
	var command *BsCommand
	switch m.dialect {
	case BsScriptRev2:
		command = m.projectRev2(opcode)
	case BsScriptRev3:
		command = m.projectRev3(opcode)
	case BsScriptRev4:
		command = m.projectRev4(opcode)
	case BsScriptRev5:
		command = m.projectRev5(opcode)
	case BsScriptRev6:
		command = m.projectRev6(opcode)
	}
	if command != nil {
		if command.Kind == BsCommandKindSelect {
			return m.selectCommand(opcode, start)
		}
		return *command
	}
	core.Logf("script: rev state opcode 0x%02X at %d not projected to a command", m.byteAt(start), start)
	return revNoneCommand
}

func (m *RevScriptMachine) projectRev2(opcode byte) *BsCommand {
	switch opcode {
	case 0x1A:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1B:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2), Third: m.slot(3), Fourth: m.slot(4)}
	case 0x1C:
		return &revNoneCommand
	case 0x1D:
		return &BsCommand{Kind: BsCommandKindWindow, First: m.slot(1), Second: m.slot(2)}
	case 0x1E:
		return &BsCommand{Kind: BsCommandKindClearScreen, First: m.slot(1)}
	case 0x1F:
		return &BsCommand{Kind: BsCommandKindTransition, First: m.slot(1), Second: m.slot(2)}
	case 0x20:
		return &revNoneCommand
	case 0x21:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: m.slot(1), Second: -1}
	case 0x22:
		return &BsCommand{Kind: BsCommandKindStopBgm}
	case 0x23:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revSoundChannel}
	case 0x24:
		return &BsCommand{Kind: BsCommandKindStopSound, First: revSoundChannel}
	case 0x25:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revVoiceChannel}
	case 0x27:
		return &BsCommand{Kind: BsCommandKindSelect}
	case 0x28:
		return &revNoneCommand
	case 0x29:
		return &BsCommand{Kind: BsCommandKindWait, First: m.waitMilliseconds(2), Second: m.slot(1)}
	case 0x2A:
		return &BsCommand{Kind: BsCommandKindSystemCall, First: m.slot(1)}
	}
	return nil
}

func (m *RevScriptMachine) projectRev3(opcode byte) *BsCommand {
	switch opcode {
	case 0x1B:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1C:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2), Third: m.slot(3), Fourth: m.slot(4)}
	case 0x1D:
		return &revNoneCommand
	case 0x1E:
		return &BsCommand{Kind: BsCommandKindWindow, First: m.slot(1), Second: m.slot(2)}
	case 0x1F:
		return &BsCommand{Kind: BsCommandKindClearScreen, First: m.slot(1), Second: m.waitMilliseconds(2)}
	case 0x20:
		return &BsCommand{Kind: BsCommandKindTransition, First: m.slot(1), Second: m.slot(3), Third: m.waitMilliseconds(2)}
	case 0x21:
		return &revNoneCommand
	case 0x22:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: m.slot(2), Second: m.slot(1)}
	case 0x23:
		return &BsCommand{Kind: BsCommandKindStopBgm, First: m.waitMilliseconds(1)}
	case 0x24:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revSoundChannel}
	case 0x25:
		return &BsCommand{Kind: BsCommandKindStopSound, First: revSoundChannel}
	case 0x26:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revVoiceChannel}
	case 0x28:
		return &BsCommand{Kind: BsCommandKindSelect}
	case 0x29:
		return &revNoneCommand
	case 0x2A:
		return &BsCommand{Kind: BsCommandKindWait, First: m.waitMilliseconds(2), Second: m.slot(1)}
	case 0x2B:
		return &BsCommand{Kind: BsCommandKindSystemCall, First: m.slot(1)}
	}
	return nil
}

func (m *RevScriptMachine) projectRev4(opcode byte) *BsCommand {
	switch opcode {
	case 0x1B:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1C:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2), Third: m.slot(3), Fourth: m.slot(4)}
	case 0x1D:
		return &BsCommand{Kind: BsCommandKindLayerRename, First: m.slot(1), Second: m.slot(2)}
	case 0x1E:
		return &BsCommand{Kind: BsCommandKindLayerRename, First: m.slot(1), Second: m.slot(2)}
	case 0x1F:
		return &BsCommand{Kind: BsCommandKindWindow, First: m.slot(1), Second: m.slot(2)}
	case 0x20:
		return &BsCommand{Kind: BsCommandKindClearScreen, First: m.slot(1), Second: m.waitMilliseconds(2)}
	case 0x21:
		return &BsCommand{Kind: BsCommandKindTransition, First: m.slot(1), Second: m.slot(3), Third: m.waitMilliseconds(2)}
	case 0x22:
		return &revNoneCommand
	case 0x23:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: m.slot(2), Second: m.slot(1)}
	case 0x24:
		return &BsCommand{Kind: BsCommandKindStopBgm, First: m.waitMilliseconds(1)}
	case 0x25:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revSoundChannel}
	case 0x26:
		return &BsCommand{Kind: BsCommandKindStopSound, First: revSoundChannel}
	case 0x27:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revVoiceChannel}
	case 0x29:
		return &BsCommand{Kind: BsCommandKindSelect}
	case 0x2A:
		return &revNoneCommand
	case 0x2B:
		return &BsCommand{Kind: BsCommandKindWait, First: m.waitMilliseconds(2), Second: m.slot(1)}
	case 0x2C:
		return &BsCommand{Kind: BsCommandKindSystemCall, First: m.slot(1)}
	}
	return nil
}

func (m *RevScriptMachine) projectRev5(opcode byte) *BsCommand {
	switch opcode {
	case 0x1B:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1C:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2), Third: m.slot(3), Fourth: m.slot(4)}
	case 0x1D:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerSwap), Third: m.slot(2)}
	case 0x1E:
		return &BsCommand{Kind: BsCommandKindLayerImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1F:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerVisibility), Third: m.slot(2)}
	case 0x20:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerPosition), Third: m.slot(2), Fourth: m.slot(3)}
	case 0x21:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerFilter), Third: m.slot(2), Fourth: m.slot(3)}
	case 0x22:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: 1}
	case 0x23:
		return &BsCommand{Kind: BsCommandKindWindow, First: m.slot(1), Second: m.slot(2)}
	case 0x24:
		return &BsCommand{Kind: BsCommandKindClearScreen, First: m.slot(1), Second: m.waitMilliseconds(2)}
	case 0x25:
		return &BsCommand{Kind: BsCommandKindTransition, First: m.slot(1), Second: m.slot(3), Third: m.waitMilliseconds(2)}
	case 0x26:
		return &revNoneCommand
	case 0x27:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: m.slot(2), Second: m.slot(1)}
	case 0x28:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: -1, Second: m.slot(1)}
	case 0x29:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revSoundChannel}
	case 0x2A:
		return &BsCommand{Kind: BsCommandKindStopSound, First: revSoundChannel}
	case 0x2B:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revVoiceChannel}
	case 0x2D:
		return &BsCommand{Kind: BsCommandKindSelect}
	case 0x2E:
		return &revNoneCommand
	case 0x2F:
		return &BsCommand{Kind: BsCommandKindWait, First: m.waitMilliseconds(2), Second: m.slot(1)}
	case 0x30:
		return &BsCommand{Kind: BsCommandKindSystemCall, First: m.slot(1)}
	}
	return nil
}

func (m *RevScriptMachine) projectRev6(opcode byte) *BsCommand {
	switch opcode {
	case 0x1B:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1C:
		return &BsCommand{Kind: BsCommandKindImage, First: m.slot(1), Second: m.slot(2), Third: m.slot(3), Fourth: m.slot(4)}
	case 0x1D:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerSwap), Third: m.slot(2)}
	case 0x1E:
		return &BsCommand{Kind: BsCommandKindLayerImage, First: m.slot(1), Second: m.slot(2)}
	case 0x1F:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerVisibility), Third: m.slot(2)}
	case 0x20:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerPosition), Third: m.slot(2), Fourth: m.slot(3)}
	case 0x21:
		return &BsCommand{Kind: BsCommandKindLayerControl, First: m.slot(1), Second: int32(BsLayerFilter), Third: m.slot(2), Fourth: m.slot(3)}
	case 0x22:
		return &revNoneCommand
	case 0x23:
		return &BsCommand{Kind: BsCommandKindWindow, First: m.slot(1), Second: m.slot(2)}
	case 0x24:
		return &BsCommand{Kind: BsCommandKindClearScreen, First: m.slot(1), Second: m.waitMilliseconds(2)}
	case 0x25:
		return &BsCommand{Kind: BsCommandKindTransition, First: m.slot(1), Second: m.slot(3), Third: m.waitMilliseconds(2)}
	case 0x26:
		return &revNoneCommand
	case 0x27:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: m.slot(2), Second: m.slot(1)}
	case 0x28:
		return &BsCommand{Kind: BsCommandKindPlayBgm, First: -1, Second: m.slot(1)}
	case 0x29:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revSoundChannel}
	case 0x2A:
		return &BsCommand{Kind: BsCommandKindStopSound, First: revSoundChannel}
	case 0x2B:
		return &BsCommand{Kind: BsCommandKindPlaySound, First: m.slot(1), Second: revVoiceChannel}
	case 0x2D:
		return &BsCommand{Kind: BsCommandKindSelect}
	case 0x2E:
		return &revNoneCommand
	case 0x2F:
		return &BsCommand{Kind: BsCommandKindWait, First: m.waitMilliseconds(2), Second: m.slot(1)}
	case 0x30:
		return &BsCommand{Kind: BsCommandKindSystemCall, First: m.slot(1)}
	}
	return nil
}

func (m *RevScriptMachine) selectCommand(opcode byte, start int) BsCommand {
	size := RevFixedSize(m.dialect, opcode)
	if size > 0 {
		m.pc = start + size
	} else {
		m.pc = start + 1
	}
	if m.dialect == BsScriptRev6 && !m.selectLog {
		m.selectLog = true
		core.Logf("script: rev select operand %d at %d ignored", m.readInt32(start+1), start)
	}
	if len(m.choices) == 0 {
		core.Logf("script: rev select at %d has no callback entries, continuing", start)
		return revNoneCommand
	}
	if m.selectHold && m.sameChoices() {
		core.Logf("script: rev select at %d was not consumed, continuing", start)
		return revNoneCommand
	}
	m.selectHold = true
	m.lastSelect = make([]BsChoice, len(m.choices))
	copy(m.lastSelect, m.choices)
	return BsCommand{Kind: BsCommandKindSelect}
}

func (m *RevScriptMachine) sameChoices() bool {
	if len(m.lastSelect) != len(m.choices) {
		return false
	}
	for i := range m.lastSelect {
		if m.lastSelect[i] != m.choices[i] {
			return false
		}
	}
	return true
}

func (m *RevScriptMachine) waitMilliseconds(slot int) int32 {
	value := 100 * m.slot(slot)
	if value < 0 {
		return 0
	}
	return value
}

func (m *RevScriptMachine) pushCallback(start int) {
	slotId := int(m.readInt32(start + 1))
	labelId := int(m.readInt32(start + 5))
	target := m.script.GetLabelOffset(labelId)
	if target < 0 {
		core.Logf("script: rev callback at %d refers to unknown label %d", start, labelId)
	}
	textId := m.readInt32(start + 9)
	wide := m.dialect == BsScriptRev6
	record := 4
	if wide {
		record = 6
	}
	if m.callbackTop+record > len(m.callbacks) {
		core.Logf("script: rev callback record buffer recycled at %d, %d dword(s) dropped", start, m.callbackTop)
		m.callbackTop = 0
	}
	m.callbacks[m.callbackTop] = -1
	m.callbackTop++
	m.callbacks[m.callbackTop] = int32(slotId)
	m.callbackTop++
	if wide {
		m.callbacks[m.callbackTop] = m.slot(1)
		m.callbackTop++
		m.callbacks[m.callbackTop] = m.slot(2)
		m.callbackTop++
	}
	m.callbacks[m.callbackTop] = int32(target)
	m.callbackTop++
	m.callbacks[m.callbackTop] = textId
	m.callbackTop++
	m.choices = append(m.choices, BsChoice{Index: slotId, MessageId: textId, LabelOffset: target})
	m.selectHold = false
	m.pc = start + RevCallbackSize
}

func (m *RevScriptMachine) jumpToLabel(labelId int32) {
	target := m.script.GetLabelOffset(int(labelId))
	if target < 0 {
		m.faultf("rev label %d at %d is out of range", labelId, m.instruction)
		return
	}
	m.pc = target
}

func (m *RevScriptMachine) jumpAbsolute(target int32) {
	if target < 0 || int(target) >= len(m.code) {
		m.faultf("rev jump %d at %d is outside the code block", target, m.instruction)
		return
	}
	m.pc = int(target)
}

func (m *RevScriptMachine) jumpOrAdvance(target int32, take bool, start int) {
	if take {
		m.jumpAbsolute(target)
		return
	}
	m.pc = start + RevStackSize
}

func (m *RevScriptMachine) jumpToLabelOrAdvance(labelId int32, take bool, start int) {
	if !take {
		m.pc = start + RevStackSize
		return
	}
	m.jumpToLabel(labelId)
}

func (m *RevScriptMachine) setWatch(labelId int32) {
	target := m.script.GetLabelOffset(int(labelId))
	if target < 0 {
		m.faultf("rev watch label %d at %d is out of range", labelId, m.instruction)
		return
	}
	m.watch = target
}

func (m *RevScriptMachine) callLabel(labelId int32, start int) {
	if !m.push(int32(start+RevStackSize)) || !m.push(m.accumulator) {
		return
	}
	m.jumpToLabel(labelId)
}

func (m *RevScriptMachine) returnFromCall() {
	if m.stackTop <= 0 {
		core.Logf("script: rev return at %d with an empty stack, halting", m.instruction)
		m.halted = true
		return
	}
	if m.stackTop < 2 {
		m.faultf("rev return at %d with an incomplete call frame", m.instruction)
		return
	}
	m.stackTop--
	m.accumulator = m.stack[m.stackTop]
	m.stackTop--
	m.pc = int(m.stack[m.stackTop])
}

func (m *RevScriptMachine) push(value int32) bool {
	if m.stackTop < 0 || m.stackTop >= len(m.stack) {
		m.faultf("rev stack overflow at %d", m.instruction)
		return false
	}
	m.stack[m.stackTop] = value
	m.stackTop++
	return true
}

func (m *RevScriptMachine) faultf(format string, args ...any) {
	core.Logf("script: "+format, args...)
	m.faulted = true
	m.halted = true
}

func (m *RevScriptMachine) seekLabel(name string) {
	offset := m.script.FindLabel(name)
	if offset < 0 {
		core.Logf("script: rev entry label '%s' is missing", name)
		return
	}
	m.pc = offset
}

func (m *RevScriptMachine) executeBinary(opcode byte, start int) {
	slot := int(m.readInt32(start + 2))
	leftRegion := int(m.readUInt16(start + 6))
	leftIndex := int(m.readInt32(start + 8))
	rightRegion := int(m.readUInt16(start + 12))
	rightIndex := int(m.readInt32(start + 14))
	m.pc = start + RevBinarySize
	if opcode == RevAssignmentOpcode(m.dialect) {
		m.executeAssignment(slot, leftRegion, leftIndex, rightRegion, rightIndex)
		return
	}
	left := m.readZone(leftRegion, leftIndex)
	right := m.readZone(rightRegion, rightIndex)
	canonical := opcode
	if m.dialect == BsScriptRev2 {
		canonical = opcode + 1
	}
	m.writeFlag(slot, revCompute(canonical, left, right))
}

func (m *RevScriptMachine) executeAssignment(slot, destinationRegion, destinationIndex, sourceRegion, sourceIndex int) {
	switch destinationRegion {
	case revVariableZone, revPropertyZone, revFrameZone, revFlagZone:
	default:
		core.Logf("script: rev assignment to region 0x%04X at %d is ignored", destinationRegion, m.instruction)
		return
	}
	value := m.readZone(sourceRegion, sourceIndex)
	if !m.writeZone(destinationRegion, destinationIndex, value) {
		core.Logf("script: rev assignment index %d in region 0x%04X is out of range", destinationIndex, destinationRegion)
		return
	}
	m.writeFlag(slot, value)
}

func (m *RevScriptMachine) executeNegate(start int) {
	slot := int(m.readInt32(start + 2))
	region := int(m.readUInt16(start + 6))
	index := int(m.readInt32(start + 8))
	m.pc = start + RevUnarySize
	m.writeFlag(slot, -m.readZone(region, index))
}

func (m *RevScriptMachine) slot(index int) int32 {
	position := m.stackTop + index
	if position >= 0 && position < len(m.stack) {
		return m.stack[position]
	}
	return 0
}

func (m *RevScriptMachine) readZone(region, index int) int32 {
	switch region {
	case revImmediateZone, revImmediateAltZone:
		return int32(index)
	case revVariableZone:
		return readInt32Array(m.variables, index)
	case revPropertyZone:
		return readInt32Array(m.properties, index)
	case revFrameZone:
		return m.slot(index)
	case revFlagZone:
		return readInt32Array(m.flags, index)
	}
	return 0
}

func (m *RevScriptMachine) writeZone(region, index int, value int32) bool {
	switch region {
	case revVariableZone:
		return writeInt32Array(m.variables, index, value)
	case revPropertyZone:
		return writeInt32Array(m.properties, index, value)
	case revFrameZone:
		return m.writeFrame(index, value)
	case revFlagZone:
		return writeInt32Array(m.flags, index, value)
	}
	return false
}

func (m *RevScriptMachine) writeFrame(index int, value int32) bool {
	position := m.stackTop + index
	if position < 0 || position >= len(m.stack) {
		return false
	}
	m.stack[position] = value
	return true
}

func (m *RevScriptMachine) writeFlag(slot int, value int32) {
	if slot < 0 || slot >= len(m.flags) {
		core.Logf("script: rev flag slot %d at %d is out of range", slot, m.instruction)
		return
	}
	m.flags[slot] = value
}

func revCompute(opcode byte, left, right int32) int32 {
	switch opcode {
	case 0x0B:
		if left > right {
			return 1
		}
		return 0
	case 0x0C:
		if left < right {
			return 1
		}
		return 0
	case 0x0D:
		if left >= right {
			return 1
		}
		return 0
	case 0x0E:
		if left <= right {
			return 1
		}
		return 0
	case 0x0F:
		if left != right {
			return 1
		}
		return 0
	case 0x10:
		if left == right {
			return 1
		}
		return 0
	case 0x12:
		return left | right
	case 0x13:
		return left & right
	case 0x14:
		return left + right
	case 0x15:
		return left - right
	case 0x16:
		return left * right
	case 0x17:
		if right == 0 {
			return 0
		}
		return left / right
	case 0x18:
		if right == 0 {
			return 0
		}
		return left % right
	}
	return 0
}

func (m *RevScriptMachine) flagValue() int32 {
	if len(m.flags) > 0 {
		return m.flags[0]
	}
	return 0
}

func (m *RevScriptMachine) byteAt(offset int) byte {
	if offset >= 0 && offset < len(m.code) {
		return m.code[offset]
	}
	return 0
}

func (m *RevScriptMachine) readInt32(offset int) int32 {
	if offset < 0 || offset+4 > len(m.code) {
		return 0
	}
	code := m.code
	return int32(code[offset]) | int32(code[offset+1])<<8 | int32(code[offset+2])<<16 | int32(code[offset+3])<<24
}

func (m *RevScriptMachine) readUInt16(offset int) int32 {
	if offset < 0 || offset+2 > len(m.code) {
		return 0
	}
	return int32(m.code[offset]) | int32(m.code[offset+1])<<8
}
