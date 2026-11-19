package scf

func EvaluateExpression(code []byte, position int, flags []byte) (bool, int) {
	if position < 0 || position > len(code) {
		return false, -1
	}
	cells := make([]byte, 0, 80)
	depth := 0
	ceiling := 0
	cursor := position
	for {
		if uint(cursor) >= uint(len(code)) {
			return false, -1
		}
		token := code[cursor]
		cursor++
		if token == 59 {
			break
		}
		switch token {
		case 38, 124:
			cells = append(cells, token)
		case 40:
			cells = append(cells, token)
			depth++
			if ceiling < depth {
				ceiling = depth
			}
		case 41:
			cells = append(cells, token)
			depth--
		case 91:
			folded, after, ok := tryFoldFlag(code, cursor, flags)
			if !ok {
				return false, -1
			}
			cells = append(cells, folded)
			cursor = after
		default:
		}
	}
	cells = append(cells, 59)
	consumed := cursor - position
	accumulator := 0
	for level := ceiling; level >= 0; level-- {
		accumulator = 0
		mode := 2
		at := 0
		for cells[at] != 59 {
			cell := cells[at]
			if level == depth {
				cells[at] = 32
			}
			switch cell {
			case 0, 1:
				if level == depth {
					if mode == 0 {
						accumulator &= int(cell)
					} else if mode == 1 {
						accumulator |= int(cell)
					} else {
						accumulator = int(cell)
					}
				}
			case 38:
				if level == depth {
					mode = 0
				}
			case 40:
				depth++
				mode = 2
			case 41:
				depth--
				cells[at] = byte(accumulator)
			case 124:
				if level == depth {
					mode = 1
				}
			default:
			}
			if level == depth {
				cells[at] = 32
			}
			at++
		}
	}
	return accumulator == 1, consumed
}

func tryFoldFlag(code []byte, cursor int, flags []byte) (byte, int, bool) {
	var folded byte
	after := cursor
	if cursor+5 > len(code) {
		return folded, after, false
	}
	index := int(code[cursor])<<8 | int(code[cursor+1])
	if flags == nil || uint(index) >= uint(len(flags)) {
		return folded, after, false
	}
	flag := flags[index]
	op := code[cursor+3]
	value := code[cursor+4]
	switch op {
	case '!':
		if value != flag {
			folded = 1
		} else {
			folded = 0
		}
	case '<':
		if value > flag {
			folded = 1
		} else {
			folded = 0
		}
	case '=':
		if value == flag {
			folded = 1
		} else {
			folded = 0
		}
	case '>':
		if value < flag {
			folded = 1
		} else {
			folded = 0
		}
	case '{':
		if value >= flag {
			folded = 1
		} else {
			folded = 0
		}
	case '}':
		if value <= flag {
			folded = 1
		} else {
			folded = 0
		}
	default:
		folded = value
	}
	after = cursor + 5
	return folded, after, true
}
