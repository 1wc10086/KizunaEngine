package tj2

import "fmt"

func ExpressionLength(code []byte, position int, dialect *Tj2Dialect) (int, error) {
	d := ModernDialect()
	if dialect != nil {
		d = *dialect
	}
	index := position
	for {
		if index < 0 || index >= len(code) {
			return 0, fmt.Errorf("Unterminated TJ2 expression at offset %d.", position)
		}
		token := code[index]
		if token == OpLabel {
			return (index + 1) - position, nil
		}
		if token == OpWrite {
			next, err := skipItem(code, index, position, d)
			if err != nil {
				return 0, err
			}
			index = next
			continue
		}
		index++
	}
}

func EvaluateExpression(code []byte, position int, flags []byte, dialect *Tj2Dialect) (bool, int) {
	d := ModernDialect()
	if dialect != nil {
		d = *dialect
	}
	if position < 0 || position > len(code) {
		return false, -1
	}
	cells := make([]byte, 0, 80)
	depth := 0
	ceiling := 0
	cursor := position
	for {
		if cursor < 0 || cursor >= len(code) {
			return false, -1
		}
		token := code[cursor]
		cursor++
		if token == OpLabel {
			break
		}
		switch token {
		case '&', '|':
			cells = append(cells, token)
		case '(':
			cells = append(cells, token)
			depth++
			if ceiling < depth {
				ceiling = depth
			}
		case ')':
			cells = append(cells, token)
			depth--
		case OpWrite:
			folded, next, ok := tryFoldItem(code, cursor, flags, d)
			if !ok {
				return false, -1
			}
			cursor = next
			cells = append(cells, folded)
		default:
		}
	}
	cells = append(cells, OpLabel)
	length := cursor - position
	accumulator := int32(0)
	level := ceiling
	for {
		accumulator = 0
		mode := 2
		at := 0
		if cells[0] != OpLabel {
			for {
				current := depth
				cell := cells[at]
				if level == depth {
					cells[at] = 32
				}
				switch cell {
				case 0, 1:
					if level == depth {
						if mode == 0 {
							accumulator &= int32(cell)
						} else if mode == 1 {
							accumulator |= int32(cell)
						} else {
							accumulator = int32(cell)
						}
					}
				case '&':
					if level == depth {
						mode = 0
					}
				case '(':
					current = depth + 1
					mode = 2
					depth++
				case ')':
					if level == depth {
						cells[at] = byte(accumulator)
					}
					depth--
					current = depth
				case '|':
					if level == depth {
						mode = 1
					}
				default:
				}
				if level == current {
					cells[at] = 32
				}
				at++
				cell = cells[at]
				if cell == OpLabel {
					break
				}
			}
		}
		level--
		if level < 0 {
			break
		}
	}
	return accumulator == 1, length
}

func skipItem(code []byte, index, position int, dialect Tj2Dialect) (int, error) {
	cursor := index + 1
	if err := needBytes(code, cursor, 4, position, "Truncated TJ2 expression item at offset %d."); err != nil {
		return 0, err
	}
	cursor += 2
	cursor++
	op := code[cursor]
	cursor++
	for op == '+' || op == '-' {
		if err := needBytes(code, cursor, 4, position, "Truncated TJ2 expression item at offset %d."); err != nil {
			return 0, err
		}
		cursor += 3
		op = code[cursor]
		cursor++
	}
	tail := dialect.LiteralWidth()
	if op == 'F' {
		tail = 5
	}
	if err := needBytes(code, cursor, tail, position, "Truncated TJ2 expression item at offset %d."); err != nil {
		return 0, err
	}
	return cursor + tail, nil
}

func tryFoldItem(code []byte, cursor int, flags []byte, dialect Tj2Dialect) (byte, int, bool) {
	var folded byte
	var plus int32
	var minus int32
	subtract := false
	var op byte
	for {
		if cursor+4 > len(code) {
			return 0, 0, false
		}
		index := int(code[cursor])<<8 | int(code[cursor+1])
		current, ok := tryReadFlag(flags, dialect, index)
		if !ok {
			return 0, 0, false
		}
		if subtract {
			minus += current
		} else {
			plus += current
		}
		op = code[cursor+3]
		cursor += 4
		if op == '-' {
			subtract = true
			continue
		}
		if op != '+' {
			break
		}
		subtract = false
	}
	total := plus - minus
	if total < 0 {
		total = 0
	}
	var value int32
	if op == 'F' {
		if cursor+5 > len(code) {
			return 0, 0, false
		}
		compare := code[cursor]
		index := int(code[cursor+2])<<8 | int(code[cursor+3])
		v, ok := tryReadFlag(flags, dialect, index)
		if !ok {
			return 0, 0, false
		}
		value = v
		cursor += 5
		folded = foldCompare(compare, total, value)
		return folded, cursor, true
	}
	if cursor+dialect.LiteralWidth() > len(code) {
		return 0, 0, false
	}
	value = readLiteral(code, &cursor, dialect.LiteralWidth())
	folded = foldCompare(op, total, value)
	return folded, cursor, true
}

func foldCompare(op byte, total, value int32) byte {
	switch op {
	case '!':
		if total != value {
			return 1
		}
		return 0
	case '<':
		if total < value {
			return 1
		}
		return 0
	case '=':
		if total == value {
			return 1
		}
		return 0
	case '>':
		if total > value {
			return 1
		}
		return 0
	case '{':
		if total <= value {
			return 1
		}
		return 0
	case '}':
		if total >= value {
			return 1
		}
		return 0
	default:
		return byte(value)
	}
}

func flagCount(flags []byte, dialect Tj2Dialect) int {
	count := (len(flags) - 16) / dialect.FlagUnitWidth
	if count < 0 {
		return 0
	}
	return count
}

func tryReadFlag(flags []byte, dialect Tj2Dialect, index int) (int32, bool) {
	if index < 0 || index >= flagCount(flags, dialect) {
		return 0, false
	}
	if dialect.WideFlags() {
		offset := 2 * index
		if offset+2 > len(flags) {
			return 0, false
		}
		return int32(flags[offset])<<8 | int32(flags[offset+1]), true
	}
	return int32(flags[index]), true
}

func readLiteral(code []byte, cursor *int, width int) int32 {
	var value int32
	for i := 0; i < width; i++ {
		value = (value << 8) | int32(code[*cursor])
		*cursor++
	}
	return value
}

func needBytes(code []byte, start, count, position int, format string) error {
	if start < 0 || start+count > len(code) {
		return fmt.Errorf(format, position)
	}
	return nil
}
