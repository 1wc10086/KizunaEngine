package vm

import (
	"errors"
	"fmt"
)

type ScriptDialectKind int

const (
	DialectKindModern ScriptDialectKind = iota
	DialectKindLegacy
	DialectKindAncient
)

type ScriptDialect struct {
	kind        ScriptDialectKind
	translation [256]byte
}

func newIdentityDialect(kind ScriptDialectKind) ScriptDialect {
	var translation [256]byte
	for index := range translation {
		translation[index] = byte(index)
	}
	return ScriptDialect{kind: kind, translation: translation}
}

func newLegacyDialect() ScriptDialect {
	dialect := newIdentityDialect(DialectKindLegacy)
	dialect.translation[0x0A] = OpcodeResetFrame
	for opcode := 0x0B; opcode <= 0x18; opcode++ {
		dialect.translation[opcode] = byte(opcode + 1)
	}
	dialect.translation[0x19] = OpcodeNegate
	dialect.translation[0x1A] = OpcodeDisplayMessage
	for opcode := 0x1B; opcode <= 0x2B; opcode++ {
		dialect.translation[opcode] = byte(opcode + 3)
	}
	dialect.translation[0x2C] = OpcodeFetch
	dialect.translation[0x2D] = OpcodeSelectArgument
	dialect.translation[0x2E] = OpcodeSelectOption
	dialect.translation[0x2F] = OpcodeSelectCommand
	dialect.translation[0x30] = OpcodeSelectCommandTail
	dialect.translation[0x31] = OpcodeDefineLabel
	return dialect
}

var (
	DialectModern  = newIdentityDialect(DialectKindModern)
	DialectLegacy  = newLegacyDialect()
	DialectAncient = newIdentityDialect(DialectKindAncient)
)

func (d ScriptDialect) Kind() ScriptDialectKind {
	return d.kind
}

func (d ScriptDialect) IsModern() bool {
	return d.kind == DialectKindModern
}

func (d ScriptDialect) IsLegacy() bool {
	return d.kind == DialectKindLegacy
}

func (d ScriptDialect) IsAncient() bool {
	return d.kind == DialectKindAncient
}

func (d ScriptDialect) SelectTakesArgument() bool {
	return d.IsModern()
}

func (d ScriptDialect) HasMessageType() bool {
	return !d.IsAncient()
}

func (d ScriptDialect) MessageIdField() int {
	if d.IsAncient() {
		return 1
	}
	return 2
}

func (d ScriptDialect) MessageCharacterField() int {
	if d.IsAncient() {
		return -1
	}
	return 6
}

func (d ScriptDialect) MessageVoiceField() int {
	if d.IsAncient() {
		return 5
	}
	return 14
}

func (d ScriptDialect) Translate(opcode byte) byte {
	return d.translation[opcode]
}

func (d ScriptDialect) SizeOf(code []byte, position int) (int, error) {
	if position < 0 || position >= len(code) {
		return 0, fmt.Errorf("instruction position %d outside code block", position)
	}
	opcode := code[position]
	if d.IsAncient() {
		return 0, errors.New("BSScript instruction sizes are decoded by the BsScript machine")
	}
	if d.IsLegacy() {
		return legacySize(code, position, opcode)
	}
	return modernSize(code, position, opcode)
}

func modernSize(code []byte, position int, opcode byte) (int, error) {
	switch opcode {
	case OpcodeHalt, OpcodeReturnOne, OpcodeReturnTwo, OpcodeReturn, OpcodeResetFrame:
		return 1, nil
	case OpcodeGotoLabel, OpcodeJump, OpcodeJumpWhenTrue, OpcodeJumpWhenFalse,
		OpcodeDeferLabel, OpcodeDeferLabelCall, OpcodeCall:
		return OpcodeFixedArgumentSize, nil
	case OpcodeDisplayMessage:
		return messageSize(code, position)
	case OpcodeFetch:
		return OpcodeFetchSize, nil
	case OpcodeInvoke:
		count, err := readInt32(code, position+1)
		if err != nil {
			return 0, err
		}
		return OpcodeFixedArgumentSize + (4 * int(count)), nil
	case OpcodeSelectArgument, OpcodeSelectOption, OpcodeDefineLabel:
		return OpcodeFixedArgumentSize, nil
	case OpcodeSelectCommand, OpcodeSelectCommandTail, OpcodeLabelCommand,
		OpcodeAttach, OpcodeEvaluate, OpcodeTranslate:
		return 1, nil
	case OpcodeNegate, OpcodeIncrement, OpcodeDecrement, OpcodeComplement:
		return OpcodeUnarySize, nil
	default:
		return binaryOrCommandSize(opcode, position)
	}
}

func binaryOrCommandSize(opcode byte, position int) (int, error) {
	if (opcode >= OpcodeGreater && opcode <= OpcodeModulo) ||
		(opcode >= OpcodeExclusiveOr && opcode <= OpcodeShiftRight) {
		return OpcodeBinarySize, nil
	}
	if opcode >= OpcodeFirstCommand && opcode <= OpcodeLastCommand {
		return 1, nil
	}
	return 0, fmt.Errorf("unknown opcode 0x%02X at %d", opcode, position)
}

func legacySize(code []byte, position int, opcode byte) (int, error) {
	switch {
	case opcode == 0x2C:
		return OpcodeFetchSize, nil
	case opcode == 0x31:
		return OpcodeFixedArgumentSize, nil
	case opcode == 0x1A:
		return legacyMessageSize(code, position)
	case opcode >= 0x0B && opcode <= 0x18:
		return OpcodeBinarySize, nil
	case opcode == 0x19:
		return OpcodeUnarySize, nil
	case opcode >= 0x03 && opcode <= 0x09:
		return OpcodeFixedArgumentSize, nil
	default:
		return 1, nil
	}
}

func legacyMessageSize(code []byte, position int) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("truncated operand at %d", position+1)
	}
	return (4 * int(code[position+1])) + 6, nil
}

func messageSize(code []byte, position int) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("truncated operand at %d", position+1)
	}
	count := code[position+1]
	extra := 0
	if count > 1 {
		countValue, err := readInt32(code, position+10)
		if err != nil {
			return 0, err
		}
		extra = 4 * int(countValue)
	}
	return extra + (4 * int(count)) + 6, nil
}

func readInt32(code []byte, position int) (int32, error) {
	if position < 0 || position+4 > len(code) {
		return 0, fmt.Errorf("truncated operand at %d", position)
	}
	return int32(code[position]) |
		(int32(code[position+1]) << 8) |
		(int32(code[position+2]) << 16) |
		(int32(code[position+3]) << 24), nil
}
