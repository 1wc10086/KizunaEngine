package bs

const (
	RevMaximumOpcode        = 0x31
	RevBinarySize           = 18
	RevUnarySize            = 12
	RevCallbackSize         = 13
	RevStackSize            = 5
	RevMessageBaseSize      = 6
	RevMessageArgumentSize  = 4
	RevMessageArgumentLimit = 3
)

const (
	rev2Message byte = 0x19
	rev3Message byte = 0x1A
)

var RevVersions = []BsScriptDialect{
	BsScriptRev2,
	BsScriptRev3,
	BsScriptRev4,
	BsScriptRev5,
	BsScriptRev6,
}

func RevIsRev(dialect BsScriptDialect) bool {
	switch dialect {
	case BsScriptRev2, BsScriptRev3, BsScriptRev4, BsScriptRev5, BsScriptRev6:
		return true
	}
	return false
}

func RevVersion(dialect BsScriptDialect) int {
	switch dialect {
	case BsScriptRev2:
		return 2
	case BsScriptRev3:
		return 3
	case BsScriptRev4:
		return 4
	case BsScriptRev5:
		return 5
	case BsScriptRev6:
		return 6
	}
	return 0
}

func RevForVersion(version int) (BsScriptDialect, bool) {
	switch version {
	case 2:
		return BsScriptRev2, true
	case 3:
		return BsScriptRev3, true
	case 4:
		return BsScriptRev4, true
	case 5:
		return BsScriptRev5, true
	case 6:
		return BsScriptRev6, true
	}
	return BsScriptAncientWide, false
}

func RevSignatureVersion(signature string) int {
	marker := -1
	for i := 0; i+4 <= len(signature); i++ {
		if signature[i] == 'R' && signature[i+1] == 'e' && signature[i+2] == 'v' && signature[i+3] == '.' {
			marker = i
			break
		}
	}
	if marker < 0 {
		return 0
	}
	version := 0
	digits := 0
	for i := marker + 4; i < len(signature); i++ {
		c := signature[i]
		if c < '0' || c > '9' {
			break
		}
		version = version*10 + int(c-'0')
		digits++
	}
	if digits == 0 {
		return 0
	}
	return version
}

func RevMessageOpcode(dialect BsScriptDialect) byte {
	if dialect == BsScriptRev2 {
		return rev2Message
	}
	return rev3Message
}

func RevCallbackOpcode(dialect BsScriptDialect) byte {
	switch dialect {
	case BsScriptRev2:
		return 0x26
	case BsScriptRev3:
		return 0x27
	case BsScriptRev4:
		return 0x28
	}
	return 0x2C
}

func RevNegateOpcode(dialect BsScriptDialect) byte {
	if dialect == BsScriptRev2 {
		return 0x18
	}
	return 0x19
}

func RevAssignmentOpcode(dialect BsScriptDialect) byte {
	if dialect == BsScriptRev2 {
		return 0x10
	}
	return 0x11
}

func RevAccumulatorLabelOpcode(dialect BsScriptDialect) byte {
	switch dialect {
	case BsScriptRev2:
		return 0x2B
	case BsScriptRev3:
		return 0x2C
	case BsScriptRev4:
		return 0x2D
	}
	return 0x31
}

func RevFirstBinaryOpcode(dialect BsScriptDialect) byte {
	if dialect == BsScriptRev2 {
		return 0x0A
	}
	return 0x0B
}

func RevLastBinaryOpcode(dialect BsScriptDialect) byte {
	if dialect == BsScriptRev2 {
		return 0x17
	}
	return 0x18
}

func RevIsBinary(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) &&
		opcode >= RevFirstBinaryOpcode(dialect) &&
		opcode <= RevLastBinaryOpcode(dialect) &&
		opcode != RevAssignmentOpcode(dialect)
}

func RevIsNegate(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) && opcode == RevNegateOpcode(dialect)
}

func RevIsCallback(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) && opcode == RevCallbackOpcode(dialect)
}

func RevIsAccumulatorLabel(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) && opcode == RevAccumulatorLabelOpcode(dialect)
}

func RevIsMessage(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) && opcode == RevMessageOpcode(dialect)
}

func RevIsState(dialect BsScriptDialect, opcode byte) bool {
	switch dialect {
	case BsScriptRev2:
		return (opcode >= 0x1A && opcode <= 0x25) || (opcode >= 0x27 && opcode <= 0x2A)
	case BsScriptRev3:
		return (opcode >= 0x1B && opcode <= 0x26) || (opcode >= 0x28 && opcode <= 0x2B)
	case BsScriptRev4:
		return (opcode >= 0x1B && opcode <= 0x27) || (opcode >= 0x29 && opcode <= 0x2C)
	case BsScriptRev5:
		return (opcode >= 0x1B && opcode <= 0x2B) || (opcode >= 0x2D && opcode <= 0x30)
	case BsScriptRev6:
		return (opcode >= 0x1B && opcode <= 0x2B) || opcode == 0x2D || opcode == 0x2F || opcode == 0x30
	}
	return false
}

func RevIsNoOperation(dialect BsScriptDialect, opcode byte) bool {
	return RevIsRev(dialect) && dialect == BsScriptRev6 && opcode == 0x2E
}

func RevSize(dialect BsScriptDialect, code []byte, position int) int {
	if !RevIsRev(dialect) || position < 0 || position >= len(code) {
		return -1
	}
	opcode := code[position]
	if opcode == RevMessageOpcode(dialect) {
		if position+1 >= len(code) {
			return -1
		}
		count := int(code[position+1])
		if count > RevMessageArgumentLimit {
			count = RevMessageArgumentLimit
		}
		return RevMessageBaseSize + RevMessageArgumentSize*count
	}
	return RevFixedSize(dialect, opcode)
}

func RevFixedSize(dialect BsScriptDialect, opcode byte) int {
	switch dialect {
	case BsScriptRev2:
		return rev2Size(opcode)
	case BsScriptRev3:
		return rev3Size(opcode)
	case BsScriptRev4:
		return rev4Size(opcode)
	case BsScriptRev5:
		return rev5Size(opcode)
	case BsScriptRev6:
		return rev6Size(opcode)
	}
	return -1
}

func rev2Size(opcode byte) int {
	switch {
	case opcode <= 0x02:
		return 1
	case opcode <= 0x07:
		return RevStackSize
	case opcode <= 0x09:
		return 1
	case opcode <= 0x17:
		return RevBinarySize
	case opcode == 0x18:
		return RevUnarySize
	case opcode <= 0x25:
		return 1
	case opcode == 0x26:
		return RevCallbackSize
	case opcode <= 0x2A:
		return 1
	case opcode == 0x2B:
		return RevStackSize
	}
	return -1
}

func rev3Size(opcode byte) int {
	switch {
	case opcode <= 0x02:
		return 1
	case opcode <= 0x08:
		return RevStackSize
	case opcode <= 0x0A:
		return 1
	case opcode <= 0x18:
		return RevBinarySize
	case opcode == 0x19:
		return RevUnarySize
	case opcode <= 0x26:
		return 1
	case opcode == 0x27:
		return RevCallbackSize
	case opcode <= 0x2B:
		return 1
	case opcode == 0x2C:
		return RevStackSize
	}
	return -1
}

func rev4Size(opcode byte) int {
	switch {
	case opcode <= 0x02:
		return 1
	case opcode <= 0x08:
		return RevStackSize
	case opcode <= 0x0A:
		return 1
	case opcode <= 0x18:
		return RevBinarySize
	case opcode == 0x19:
		return RevUnarySize
	case opcode <= 0x27:
		return 1
	case opcode == 0x28:
		return RevCallbackSize
	case opcode <= 0x2C:
		return 1
	case opcode == 0x2D:
		return RevStackSize
	}
	return -1
}

func rev5Size(opcode byte) int {
	switch {
	case opcode <= 0x02:
		return 1
	case opcode <= 0x08:
		return RevStackSize
	case opcode <= 0x0A:
		return 1
	case opcode <= 0x18:
		return RevBinarySize
	case opcode == 0x19:
		return RevUnarySize
	case opcode <= 0x2B:
		return 1
	case opcode == 0x2C:
		return RevCallbackSize
	case opcode <= 0x30:
		return 1
	case opcode == 0x31:
		return RevStackSize
	}
	return -1
}

func rev6Size(opcode byte) int {
	switch {
	case opcode <= 0x02:
		return 1
	case opcode <= 0x08:
		return RevStackSize
	case opcode <= 0x0A:
		return 1
	case opcode <= 0x18:
		return RevBinarySize
	case opcode == 0x19:
		return RevUnarySize
	case opcode <= 0x2B:
		return 1
	case opcode == 0x2C:
		return RevCallbackSize
	case opcode == 0x2D:
		return RevStackSize
	case opcode <= 0x30:
		return 1
	case opcode == 0x31:
		return RevStackSize
	}
	return -1
}
