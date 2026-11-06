package state

import "strings"

const (
	StageNameBlack       = "black"
	StageNameWhite       = "white"
	StageNameTransparent = "transparent"
	StageNameReserved    = "reserved"
)

const stageColorNameLength = 9

func StageNameIsEmpty(name string) bool {
	return len(name) == 0 || strings.EqualFold(name, StageNameReserved)
}

func StageNameTryParseColor(name string) (blue, green, red, alpha uint8, ok bool) {
	if len(name) != stageColorNameLength || name[0] != '*' {
		return 0, 0, 0, 0, false
	}
	var channels [4]uint8
	for index := 0; index < 4; index++ {
		high := stageHexValue(name[index*2+1])
		low := stageHexValue(name[index*2+2])
		if high < 0 || low < 0 {
			return 0, 0, 0, 0, false
		}
		channels[index] = uint8(high<<4 | low)
	}
	return channels[2], channels[1], channels[0], channels[3], true
}

func stageHexValue(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	}
	return -1
}
