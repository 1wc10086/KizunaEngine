package tj2

import "fmt"

const (
	OpBang  byte = 0x21
	OpHash  byte = 0x23
	OpLabel byte = 0x3B
	OpWrite byte = 0x5B
	OpLatch byte = 0x28
)

func IsBangLetter(value byte) bool {
	switch value {
	case 'a', 'b', 'c', 'e', 'g', 'i', 'j', 'm', 'n', 'p', 'r', 's', 't', 'v', 'w', 'x':
		return true
	default:
		return false
	}
}

func IsHashLetter(value byte) bool {
	switch value {
	case 'W', 'b', 'e', 'g', 'i', 'l', 'n', 'r', 's', 't', 'w', 'x':
		return true
	default:
		return false
	}
}

func InstructionLength(code []byte, position int, dialect *Tj2Dialect) (int, error) {
	if position < 0 || position >= len(code) {
		return 0, fmt.Errorf("TJ2 instruction starts outside the code area: %d.", position)
	}
	switch code[position] {
	case OpBang:
		n, err := bangOperandLength(code, position)
		if err != nil {
			return 0, err
		}
		return 2 + n, nil
	case OpHash:
		n, err := hashOperandLength(code, position, dialect)
		if err != nil {
			return 0, err
		}
		return 2 + n, nil
	case OpLabel:
		return 1, nil
	case OpWrite:
		return writeLength(code, position, dialect)
	case OpLatch:
		if err := fixed(code, position, 3); err != nil {
			return 0, err
		}
		return 3, nil
	default:
		return 1, nil
	}
}

func OpcodeExpressionLength(code []byte, position int, dialect *Tj2Dialect) (int, error) {
	return ExpressionLength(code, position, dialect)
}

func bangOperandLength(code []byte, position int) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("Truncated TJ2 bang opcode at offset %d.", position)
	}
	switch code[position+1] {
	case 'c', 'r', 'w':
		return 0, nil
	case 'b', 'g', 'j', 'n':
		if err := fixed(code, position+2, 2); err != nil {
			return 0, err
		}
		return 2, nil
	case 'e', 'v', 'x':
		if err := fixed(code, position+2, 3); err != nil {
			return 0, err
		}
		return 3, nil
	case 'a':
		if err := fixed(code, position+2, 4); err != nil {
			return 0, err
		}
		return 4, nil
	case 'i', 'm', 't':
		return cstringLength(code, position+2)
	case 's':
		n, err := cstringLength(code, position+5)
		if err != nil {
			return 0, err
		}
		return 3 + n, nil
	case 'p':
		if err := fixed(code, position+2, 2); err != nil {
			return 0, err
		}
		n, err := cstringLength(code, position+4)
		if err != nil {
			return 0, err
		}
		return 2 + n, nil
	default:
		return 0, fmt.Errorf("Unknown TJ2 bang opcode '%c' at offset %d.", code[position+1], position)
	}
}

func hashOperandLength(code []byte, position int, dialect *Tj2Dialect) (int, error) {
	if position+1 >= len(code) {
		return 0, fmt.Errorf("Truncated TJ2 hash opcode at offset %d.", position)
	}
	switch code[position+1] {
	case 'e', 'n':
		return 0, nil
	case 'W':
		return second(code, position, 1)
	case 'g':
		if err := fixed(code, position+2, 2); err != nil {
			return 0, err
		}
		return 2, nil
	case 'i':
		return ExpressionLength(code, position+2, dialect)
	case 'b':
		switch peek(code, position+2) {
		case 'd':
			return 1, nil
		case 'i', 'o':
			return 2, nil
		default:
			return second(code, position, 1)
		}
	case 's':
		return second(code, position, 1)
	case 'l':
		if peek(code, position+2) == 'i' {
			n, err := ExpressionLength(code, position+3, dialect)
			if err != nil {
				return 0, err
			}
			return 1 + n, nil
		}
		return 0, nil
	case 'r':
		if hasEset(code, position+2) {
			return 4, nil
		}
		return 0, nil
	case 't':
		switch peek(code, position+2) {
		case 'w':
			if err := fixed(code, position+3, 2); err != nil {
				return 0, err
			}
			return 3, nil
		case 't':
			if err := fixed(code, position+3, 1); err != nil {
				return 0, err
			}
			return 2, nil
		default:
			return second(code, position, 1)
		}
	case 'w':
		if peek(code, position+2) == 'b' {
			return 1, nil
		}
		return 0, nil
	case 'x':
		if peek(code, position+2) == 'l' {
			if err := fixed(code, position+3, 2); err != nil {
				return 0, err
			}
			return 3, nil
		}
		return second(code, position, 1)
	default:
		return 0, fmt.Errorf("Unknown TJ2 hash opcode '%c' at offset %d.", code[position+1], position)
	}
}

func writeLength(code []byte, position int, dialect *Tj2Dialect) (int, error) {
	cursor := position + 1
	if err := needBytes(code, cursor, 4, position, "Truncated TJ2 write statement at offset %d."); err != nil {
		return 0, err
	}
	cursor += 2
	cursor++
	cursor++
	for {
		for peek(code, cursor) == '(' {
			cursor++
		}
		if err := needBytes(code, cursor, 1, position, "Truncated TJ2 write statement at offset %d."); err != nil {
			return 0, err
		}
		value := code[cursor]
		if dialect == nil {
			switch value {
			case 'F':
				cursor += 3
			case 'V':
				cursor += 2
			default:
				cursor++
			}
		} else {
			if value == 'F' {
				cursor += 3
			} else {
				cursor += 1 + dialect.LiteralWidth()
			}
		}
		if err := needBytes(code, cursor, 1, position, "Truncated TJ2 write statement at offset %d."); err != nil {
			return 0, err
		}
		op := code[cursor]
		cursor++
		for peek(code, cursor) == ')' {
			cursor++
		}
		if op == 0 {
			return cursor - position, nil
		}
	}
}

func peek(code []byte, index int) byte {
	if index < 0 || index >= len(code) {
		return 0
	}
	return code[index]
}

func hasEset(code []byte, start int) bool {
	return start >= 0 &&
		start+4 <= len(code) &&
		code[start] == 'e' &&
		code[start+1] == 's' &&
		code[start+2] == 'e' &&
		code[start+3] == 't'
}

func second(code []byte, position, count int) (int, error) {
	return count, fixed(code, position+2, count)
}

func fixed(code []byte, start, count int) error {
	if start < 0 || start+count > len(code) {
		return fmt.Errorf("Truncated TJ2 operand at offset %d.", start)
	}
	return nil
}

func cstringLength(code []byte, start int) (int, error) {
	if start < 0 || start > len(code) {
		return 0, fmt.Errorf("Truncated TJ2 string operand at offset %d.", start)
	}
	for i := start; i < len(code); i++ {
		if code[i] == 0 {
			return (i - start) + 1, nil
		}
	}
	return 0, fmt.Errorf("Unterminated TJ2 string operand at offset %d.", start)
}
