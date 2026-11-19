package scf

import (
	"bytes"
	"fmt"
)

const (
	ScfOpcodeBang   byte = 0x21
	ScfOpcodeHash   byte = 0x23
	ScfOpcodeLabel  byte = 0x3B
	ScfOpcodeWrite  byte = 0x5B
	ScfOpcodeLatch  byte = 0x28
	ScfOpcodeFiller byte = 0x01
)

func IsScfBangLetter(value byte) bool {
	switch value {
	case 'a', 'b', 'c', 'e', 'g', 'i', 'j', 'm', 'n', 'p', 'r', 's', 't', 'w', 'x':
		return true
	}
	return false
}

func IsScfHashLetter(value byte) bool {
	switch value {
	case 'e', 'g', 'i', 'l', 'n', 'r', 'w':
		return true
	}
	return false
}

func ScfInstructionLength(code []byte, position int) (int, error) {
	if uint(position) >= uint(len(code)) {
		return 0, fmt.Errorf("SCF instruction starts outside the code area: %d.", position)
	}
	switch code[position] {
	case ScfOpcodeBang:
		length, err := scfBangOperandLength(code, position)
		if err != nil {
			return 0, err
		}
		return 2 + length, nil
	case ScfOpcodeHash:
		length, err := scfHashOperandLength(code, position)
		if err != nil {
			return 0, err
		}
		return 2 + length, nil
	case ScfOpcodeLabel:
		return 1, nil
	case ScfOpcodeWrite:
		return scfFixed(code, position, 6)
	case ScfOpcodeLatch:
		return scfFixed(code, position, 3)
	default:
		return 1, nil
	}
}

func ScfExpressionLength(code []byte, position int) (int, error) {
	index := position
	for uint(index) < uint(len(code)) {
		if code[index] == ScfOpcodeWrite {
			index += 6
			continue
		}
		if code[index] == ScfOpcodeLabel {
			return (index + 1) - position, nil
		}
		index++
	}
	return 0, fmt.Errorf("Unterminated SCF expression at offset %d.", position)
}

func scfBangOperandLength(code []byte, position int) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("Truncated SCF bang opcode at offset %d.", position)
	}
	switch code[position+1] {
	case 'c', 'r', 'w':
		return 0, nil
	case 'b', 'e', 'g', 'j', 'n':
		return scfFixed(code, position+2, 2)
	case 'x':
		return scfFixed(code, position+2, 3)
	case 'a':
		return scfFixed(code, position+2, 4)
	case 'i', 'm', 's', 't':
		return scfCStringLength(code, position+2)
	case 'p':
		fixed, err := scfFixed(code, position+2, 2)
		if err != nil {
			return 0, err
		}
		tail, err := scfCStringLength(code, position+4)
		if err != nil {
			return 0, err
		}
		return fixed + tail, nil
	default:
		return 0, fmt.Errorf("Unknown SCF bang opcode '%c' at offset %d.", code[position+1], position)
	}
}

func scfHashOperandLength(code []byte, position int) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("Truncated SCF hash opcode at offset %d.", position)
	}
	switch code[position+1] {
	case 'e', 'l', 'n', 'r', 'w':
		return 0, nil
	case 'g':
		return scfFixed(code, position+2, 2)
	case 'i':
		return ScfExpressionLength(code, position+2)
	default:
		return 0, fmt.Errorf("Unknown SCF hash opcode '%c' at offset %d.", code[position+1], position)
	}
}

func scfFixed(code []byte, start, count int) (int, error) {
	if start < 0 || uint(start+count) > uint(len(code)) {
		return 0, fmt.Errorf("Truncated SCF operand at offset %d.", start)
	}
	return count, nil
}

func scfCStringLength(code []byte, start int) (int, error) {
	if uint(start) > uint(len(code)) {
		return 0, fmt.Errorf("Truncated SCF string operand at offset %d.", start)
	}
	end := bytes.IndexByte(code[start:], 0)
	if end < 0 {
		return 0, fmt.Errorf("Unterminated SCF string operand at offset %d.", start)
	}
	return end + 1, nil
}
