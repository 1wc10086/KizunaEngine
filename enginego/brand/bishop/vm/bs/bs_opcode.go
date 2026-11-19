package bs

const (
	OpHalt              byte = 0x00
	OpQuit              byte = 0x01
	OpGoto              byte = 0x02
	OpCall              byte = 0x03
	OpReturn            byte = 0x04
	OpPushImmediate     byte = 0x05
	OpPushProperty      byte = 0x06
	OpPushVariable      byte = 0x07
	OpPopProperty       byte = 0x08
	OpPopVariable       byte = 0x09
	OpAdd               byte = 0x0A
	OpSubtract          byte = 0x0B
	OpMultiply          byte = 0x0C
	OpDivide            byte = 0x0D
	OpModulo            byte = 0x0E
	OpMove              byte = 0x0F
	OpLess              byte = 0x10
	OpGreater           byte = 0x11
	OpEqual             byte = 0x12
	OpNotEqual          byte = 0x13
	OpLessOrEqual       byte = 0x14
	OpGreaterOrEqual    byte = 0x15
	OpLogicalOr         byte = 0x16
	OpLogicalAnd        byte = 0x17
	OpJumpWhenFalse     byte = 0x18
	OpMessage           byte = 0x19
	OpMessageChar       byte = 0x1A
	OpMessageVoice      byte = 0x1B
	OpMessageBack       byte = 0x1C
	OpImage             byte = 0x1D
	OpImageAt           byte = 0x1E
	OpWindow            byte = 0x1F
	OpClearScreen       byte = 0x20
	OpTransition        byte = 0x21
	OpPlayBgm           byte = 0x22
	OpStopBgm           byte = 0x23
	OpPlaySound         byte = 0x24
	OpChoice            byte = 0x25
	OpSelect            byte = 0x26
	OpDialog            byte = 0x27
	OpWait              byte = 0x28
	OpSystemCall        byte = 0x29
	OpLoadAccumulator   byte = 0x2A
	OpHighest                = OpLoadAccumulator
	OpFixedArgumentSize      = 5
)

const (
	BsCompactShift byte = 0x1C
	BsUnknown      byte = OpHighest + 1
)

func BsSemantic(raw byte, dialect BsScriptDialect) byte {
	if dialect != BsScriptAncientCompact || raw < BsCompactShift {
		return raw
	}
	if raw < OpHighest {
		return raw + 1
	}
	return BsUnknown
}

func BsSize(opcode byte) int {
	switch opcode {
	case OpHalt, OpQuit, OpReturn, OpStopBgm, OpSelect, OpDialog:
		return 1
	case OpGoto, OpCall, OpPushImmediate, OpPushProperty, OpPushVariable,
		OpPopProperty, OpPopVariable, OpJumpWhenFalse, OpMessage,
		OpPlayBgm, OpSystemCall, OpLoadAccumulator:
		return OpFixedArgumentSize
	case OpMessageChar:
		return 9
	case OpMessageVoice:
		return 13
	case OpMessageBack:
		return 17
	case OpImage:
		return 6
	case OpImageAt:
		return 10
	case OpWindow:
		return 3
	case OpClearScreen:
		return 2
	case OpTransition, OpPlaySound:
		return 6
	case OpChoice:
		return 9
	case OpWait:
		return 3
	}
	if opcode >= OpAdd && opcode <= OpLogicalAnd {
		return 10
	}
	return 0
}

func BsIsKnown(opcode byte) bool {
	return opcode <= OpHighest && BsSize(opcode) > 0
}

func BsIsState(opcode byte) bool {
	switch opcode {
	case OpHalt, OpQuit:
		return true
	}
	if opcode >= OpMessage && opcode <= OpPlaySound {
		return true
	}
	if opcode >= OpSelect && opcode <= OpSystemCall {
		return true
	}
	return false
}
