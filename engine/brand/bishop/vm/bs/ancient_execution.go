package bs

import (
	"kizuna/engine/core"
)

const (
	ancientImmediateZone = 0
	ancientPropertyZone  = 1
	ancientVariableZone  = 2
	ancientRegisterZone  = 4
)

func (m *AncientScriptMachine) tryRunInternal(opcode byte) bool {
	start := m.instruction
	switch opcode {
	case OpGoto:
		m.jumpToLabel(m.readInt32(start + 1))
		return true
	case OpCall:
		if m.push(int32(start+OpFixedArgumentSize)) && m.push(m.accumulator) {
			m.jumpToLabel(m.readInt32(start + 1))
		}
		return true
	case OpReturn:
		if m.stackTop <= 0 {
			core.Logf("script: ancient return at %d with an empty stack, halting", start)
			m.halted = true
			return true
		}
		restored, okAcc := m.pop()
		resume, okAddr := m.pop()
		if okAcc && okAddr {
			m.accumulator = restored
			m.pc = int(resume)
		}
		return true
	case OpPushImmediate:
		m.push(m.readInt32(start + 1))
		m.pc = start + OpFixedArgumentSize
		return true
	case OpPushProperty:
		m.push(m.readZone(ancientPropertyZone, int(m.readInt32(start+1))))
		m.pc = start + OpFixedArgumentSize
		return true
	case OpPushVariable:
		m.push(m.readZone(ancientVariableZone, int(m.readInt32(start+1))))
		m.pc = start + OpFixedArgumentSize
		return true
	case OpPopProperty:
		if property, ok := m.pop(); ok {
			m.writeZone(ancientPropertyZone, int(m.readInt32(start+1)), property)
		}
		m.pc = start + OpFixedArgumentSize
		return true
	case OpPopVariable:
		if variable, ok := m.pop(); ok {
			m.writeZone(ancientVariableZone, int(m.readInt32(start+1)), variable)
		}
		m.pc = start + OpFixedArgumentSize
		return true
	case OpJumpWhenFalse:
		if m.flag == 0 {
			m.jumpToLabel(m.readInt32(start + 1))
		} else {
			m.pc = start + OpFixedArgumentSize
		}
		return true
	case OpChoice:
		target := m.script.GetLabelOffset(int(m.readInt32(start + 1)))
		if target < 0 {
			core.Logf("script: ancient choice at %d refers to an unknown label", start)
		}
		m.choices = append(m.choices, BsChoice{Index: len(m.choices), MessageId: m.readInt32(start + 5), LabelOffset: target})
		m.pc = start + 9
		return true
	case OpLoadAccumulator:
		m.accumulator = m.readInt32(start + 1)
		m.pc = start + OpFixedArgumentSize
		return true
	}
	if opcode >= OpAdd && opcode <= OpLogicalAnd {
		m.executeArithmetic(opcode)
		return true
	}
	return false
}

func (m *AncientScriptMachine) decode(opcode byte) BsCommand {
	start := m.instruction
	switch opcode {
	case OpHalt:
		m.Reset()
		return BsCommand{Kind: BsCommandKindRestart}
	case OpQuit:
		m.pc = start + 1
		return BsCommand{Kind: BsCommandKindQuit}
	case OpMessage:
		m.pc = start + 5
		return BsCommand{Kind: BsCommandKindMessage, First: m.readInt32(start + 1), Second: -1, Third: -1, Fourth: -1}
	case OpMessageChar:
		m.pc = start + 9
		return BsCommand{Kind: BsCommandKindMessage, First: m.readInt32(start + 1), Second: m.readInt32(start + 5), Third: -1, Fourth: -1}
	case OpMessageVoice:
		m.pc = start + 13
		return BsCommand{Kind: BsCommandKindMessage, First: m.readInt32(start + 1), Second: m.readInt32(start + 5), Third: m.readInt32(start + 9), Fourth: -1}
	case OpMessageBack:
		m.pc = start + 17
		return BsCommand{Kind: BsCommandKindMessage, First: m.readInt32(start + 1), Second: m.readInt32(start + 5), Third: m.readInt32(start + 9), Fourth: m.readInt32(start + 13)}
	case OpImage:
		imageName := m.readInt32(start + 1)
		imageSubcode := m.byteAt(start + 5)
		m.pc = start + 6
		return m.projectImage(imageName, imageSubcode, 0, 0)
	case OpImageAt:
		placedName := m.readInt32(start + 1)
		placedSubcode := m.byteAt(start + 5)
		placedX := m.readInt16(start + 6)
		placedY := m.readInt16(start + 8)
		m.pc = start + 10
		return m.projectImage(placedName, placedSubcode, placedX, placedY)
	case OpWindow:
		m.pc = start + 3
		return BsCommand{Kind: BsCommandKindWindow, First: int32(m.byteAt(start + 1)), Second: int32(m.byteAt(start + 2))}
	case OpClearScreen:
		m.pc = start + 2
		return BsCommand{Kind: BsCommandKindClearScreen, First: int32(m.byteAt(start + 1))}
	case OpTransition:
		m.pc = start + 6
		return BsCommand{Kind: BsCommandKindTransition, First: int32(m.byteAt(start + 1)), Second: m.readInt32(start + 2)}
	case OpPlayBgm:
		m.pc = start + 5
		return BsCommand{Kind: BsCommandKindPlayBgm, First: m.readInt32(start + 1)}
	case OpStopBgm:
		m.pc = start + 1
		return BsCommand{Kind: BsCommandKindStopBgm}
	case OpPlaySound:
		m.pc = start + 6
		return BsCommand{Kind: BsCommandKindPlaySound, First: m.readInt32(start + 2), Second: int32(m.byteAt(start + 1))}
	case OpSelect:
		m.pc = start + 1
		if len(m.choices) == 0 {
			core.Logf("script: ancient select at %d with no registered choices", start)
		}
		return BsCommand{Kind: BsCommandKindSelect}
	case OpDialog:
		m.pc = start + 1
		return BsCommand{Kind: BsCommandKindDialog}
	case OpWait:
		m.pc = start + 3
		return BsCommand{Kind: BsCommandKindWait, First: int32(100 * int(m.byteAt(start+2))), Second: int32(m.byteAt(start + 1))}
	case OpSystemCall:
		m.pc = start + 5
		return BsCommand{Kind: BsCommandKindSystemCall, First: m.readInt32(start + 1)}
	}
	return ancientNoneCommand
}

func (m *AncientScriptMachine) projectImage(names int32, subcode byte, x, y int32) BsCommand {
	switch subcode {
	case 0, 1, 2, 3, 0x81:
		return BsCommand{Kind: BsCommandKindImage, First: names, Second: int32(subcode), Third: x, Fourth: y}
	}
	m.faultf("ancient image subcode 0x%02X at %d is invalid", subcode, m.instruction)
	return ancientHaltCommand
}

func (m *AncientScriptMachine) jumpToLabel(labelId int32) {
	target := m.script.GetLabelOffset(int(labelId))
	if target < 0 {
		m.faultf("ancient label %d at %d is out of range", labelId, m.instruction)
		return
	}
	m.pc = target
}

func (m *AncientScriptMachine) push(value int32) bool {
	if m.stackTop < 0 || m.stackTop >= len(m.stack) {
		m.faultf("ancient stack overflow at %d", m.instruction)
		return false
	}
	m.stack[m.stackTop] = value
	m.stackTop++
	return true
}

func (m *AncientScriptMachine) pop() (int32, bool) {
	if m.stackTop <= 0 {
		m.faultf("ancient stack underflow at %d", m.instruction)
		return 0, false
	}
	m.stackTop--
	return m.stack[m.stackTop], true
}

func (m *AncientScriptMachine) faultf(format string, args ...any) {
	core.Logf("script: "+format, args...)
	m.faulted = true
	m.halted = true
}

func (m *AncientScriptMachine) executeArithmetic(opcode byte) {
	start := m.instruction
	zoneType := m.byteAt(start + 1)
	destinationZone := int(zoneType >> 4)
	sourceZone := int(zoneType & 0x0F)
	destinationIndex := int(m.readInt32(start + 2))
	sourceIndex := int(m.readInt32(start + 6))
	m.pc = start + 10
	if !isAncientSourceZone(sourceZone) || !isAncientDestinationZone(destinationZone, opcode >= OpLess) {
		return
	}
	current := m.readZone(destinationZone, destinationIndex)
	source := m.readZone(sourceZone, sourceIndex)
	result := ancientCompute(opcode, current, source)
	if !m.writeZone(destinationZone, destinationIndex, result) {
		return
	}
	m.flag = result
}

func isAncientSourceZone(zone int) bool {
	switch zone {
	case ancientImmediateZone, ancientPropertyZone, ancientVariableZone, ancientRegisterZone:
		return true
	}
	return false
}

func isAncientDestinationZone(zone int, comparison bool) bool {
	if comparison {
		return zone == ancientRegisterZone
	}
	switch zone {
	case ancientPropertyZone, ancientVariableZone, ancientRegisterZone:
		return true
	}
	return false
}

func (m *AncientScriptMachine) readZone(zone, index int) int32 {
	switch zone {
	case ancientImmediateZone:
		return int32(index)
	case ancientPropertyZone:
		return readInt32Array(m.properties, index)
	case ancientVariableZone:
		return readInt32Array(m.variables, index)
	case ancientRegisterZone:
		return readInt32Array(m.registers, index)
	}
	return 0
}

func (m *AncientScriptMachine) writeZone(zone, index int, value int32) bool {
	switch zone {
	case ancientPropertyZone:
		return writeInt32Array(m.properties, index, value)
	case ancientVariableZone:
		return writeInt32Array(m.variables, index, value)
	case ancientRegisterZone:
		return writeInt32Array(m.registers, index, value)
	}
	return false
}

func readInt32Array(array []int32, index int) int32 {
	if index >= 0 && index < len(array) {
		return array[index]
	}
	return 0
}

func ancientCompute(opcode byte, current, source int32) int32 {
	switch opcode {
	case OpAdd:
		return current + source
	case OpSubtract:
		return current - source
	case OpMultiply:
		return current * source
	case OpDivide:
		if source == 0 {
			return 0
		}
		return current / source
	case OpModulo:
		if source == 0 {
			return 0
		}
		return current % source
	case OpMove:
		return source
	case OpLess:
		if current < source {
			return 1
		}
		return 0
	case OpGreater:
		if current > source {
			return 1
		}
		return 0
	case OpEqual:
		if current == source {
			return 1
		}
		return 0
	case OpNotEqual:
		if current != source {
			return 1
		}
		return 0
	case OpLessOrEqual:
		if current <= source {
			return 1
		}
		return 0
	case OpGreaterOrEqual:
		if current >= source {
			return 1
		}
		return 0
	case OpLogicalOr:
		if current != 0 || source != 0 {
			return 1
		}
		return 0
	case OpLogicalAnd:
		if current != 0 && source != 0 {
			return 1
		}
		return 0
	}
	return 0
}

func (m *AncientScriptMachine) byteAt(offset int) byte {
	if offset >= 0 && offset < len(m.code) {
		return m.code[offset]
	}
	return 0
}

func (m *AncientScriptMachine) readInt32(offset int) int32 {
	if offset < 0 || offset+4 > len(m.code) {
		return 0
	}
	code := m.code
	return int32(code[offset]) | int32(code[offset+1])<<8 | int32(code[offset+2])<<16 | int32(code[offset+3])<<24
}

func (m *AncientScriptMachine) readInt16(offset int) int32 {
	if offset < 0 || offset+2 > len(m.code) {
		return 0
	}
	return int32(int16(int(m.code[offset]) | int(m.code[offset+1])<<8))
}
