package textcodec

import (
	"unicode/utf8"
)

func DetectTextEncoding(block []byte) string {
	if looksUTF16LE(block) {
		return "utf16le"
	}
	return DetectLegacy(block)
}

func DetectLegacy(block []byte) string {
	if IsUTF8Block(block) {
		return "utf8"
	}
	if scoreCP932(block) >= scoreGBK(block) {
		return "cp932"
	}
	return "gbk"
}

func looksUTF16LE(b []byte) bool {
	if len(b) < 4 || len(b)%2 != 0 {
		return false
	}
	rest := b
	if b[0] == 0xFF && b[1] == 0xFE {
		rest = b[2:]
		if len(rest) == 0 || len(rest)%2 != 0 {
			return false
		}
	}
	found := false
	for i := 0; i < len(rest); {
		lo := rest[i]
		hi := rest[i+1]
		v := uint16(lo) | uint16(hi)<<8
		switch {
		case v == 0:
			found = true
			i += 2
		case hi == 0:
			if lo < 0x20 {
				if lo != 0x09 && lo != 0x0A && lo != 0x0D {
					return false
				}
			} else if lo >= 0x7F {
				return false
			}
			found = true
			i += 2
		case v >= 0xD800 && v <= 0xDBFF:
			if i+3 >= len(rest) {
				return false
			}
			w := uint16(rest[i+2]) | uint16(rest[i+3])<<8
			if w < 0xDC00 || w > 0xDFFF {
				return false
			}
			i += 4
		case utf16Plausible(v):
			i += 2
		default:
			return false
		}
	}
	return found || allKanaPairs(rest)
}

func utf16Plausible(v uint16) bool {
	switch {
	case v >= 0x2010 && v <= 0x202F:
		return true
	case v >= 0x2460 && v <= 0x24FF:
		return true
	case v >= 0x3000 && v <= 0x303F:
		return true
	case v >= 0x3040 && v <= 0x30FF:
		return true
	case v >= 0x4E00 && v <= 0x9FFF:
		return true
	case v >= 0xFF00 && v <= 0xFFEF:
		return true
	default:
		return false
	}
}

func allKanaPairs(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	for i := 0; i < len(b); i += 2 {
		v := uint16(b[i]) | uint16(b[i+1])<<8
		if v < 0x3040 || v > 0x30FF {
			return false
		}
	}
	return true
}

func IsUTF8Block(block []byte) bool {
	pos := 0
	multi := false
	for pos < len(block) {
		end := pos
		for end < len(block) && block[end] != 0 {
			end++
		}
		text := block[pos:end]
		if len(text) > 1 && HasHighByte(text) {
			if !utf8.Valid(text) {
				return false
			}
			multi = true
		}
		pos = end + 1
	}
	return multi
}

func HasHighByte(text []byte) bool {
	for _, c := range text {
		if c >= 0x80 {
			return true
		}
	}
	return false
}

func scoreCP932(block []byte) int {
	score := 0
	for i := 0; i < len(block); i++ {
		lead := block[i]
		if lead < 0x80 {
			continue
		}
		if lead >= 0xA1 && lead <= 0xDF {
			score -= 3
			continue
		}
		if isCP932Lead(lead) && i+1 < len(block) && isCP932Trail(block[i+1]) {
			if lead == 0x82 || lead == 0x83 {
				score += 6
			} else {
				score += 2
			}
			i++
			continue
		}
		score -= 2
	}
	return score
}

func scoreGBK(block []byte) int {
	score := 0
	for i := 0; i < len(block); i++ {
		lead := block[i]
		if lead < 0x80 {
			continue
		}
		if lead <= 0xFE && i+1 < len(block) && isGBKTrail(block[i+1]) {
			if lead >= 0xB0 && lead <= 0xF7 {
				score += 3
			} else {
				score += 1
			}
			i++
			continue
		}
		score -= 2
	}
	return score
}

func isCP932Trail(c byte) bool {
	return (c >= 0x40 && c <= 0x7E) || (c >= 0x80 && c <= 0xFC)
}

func isGBKTrail(c byte) bool {
	return (c >= 0x40 && c <= 0x7E) || (c >= 0x80 && c <= 0xFE)
}
