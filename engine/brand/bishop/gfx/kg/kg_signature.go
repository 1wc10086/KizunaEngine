package kg

import "kizuna/engine/core/binutil"

func IsKg(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch signature := binutil.U32(data); signature {
	case 0x0202474B, 0x0102474B, 0x0200474B, 0x0100474B:
		return true
	}
	return false
}
