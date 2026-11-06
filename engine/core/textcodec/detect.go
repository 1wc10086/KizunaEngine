package textcodec

func DetectTextEncoding(block []byte) Encoding {
	if IsUtf8Block(block) {
		return Utf8
	}
	if ScoreCP932(block) >= ScoreGBK(block) {
		return Cp932
	}
	return Gbk
}

func IsUtf8Block(block []byte) bool {
	position := 0
	multiByte := false
	for position < len(block) {
		end := FindTerminator(block, position, Utf8)
		text := block[position:end]
		if len(text) > 1 && HasHighByte(text) {
			if !IsStrictUtf8(text) {
				return false
			}
			multiByte = true
		}
		position = end + 1
	}
	return multiByte
}

func HasHighByte(text []byte) bool {
	for _, value := range text {
		if value >= 0x80 {
			return true
		}
	}
	return false
}

func IsStrictUtf8(text []byte) bool {
	index := 0
	for index < len(text) {
		c := text[index]
		var size int
		var min rune
		var max rune
		switch {
		case c < 0x80:
			index++
			continue
		case c >= 0xC2 && c <= 0xDF:
			size, min, max = 2, 0x80, 0x7FF
		case c >= 0xE0 && c <= 0xEF:
			size, min, max = 3, 0x800, 0xFFFF
		case c >= 0xF0 && c <= 0xF4:
			size, min, max = 4, 0x10000, 0x10FFFF
		default:
			return false
		}
		if index+size > len(text) {
			return false
		}
		value := rune(c & (0xFF >> (size + 1)))
		for i := 1; i < size; i++ {
			t := text[index+i]
			if t < 0x80 || t > 0xBF {
				return false
			}
			value = (value << 6) | rune(t&0x3F)
		}
		if value < min || value > max {
			return false
		}
		if value >= 0xD800 && value <= 0xDFFF {
			return false
		}
		index += size
	}
	return true
}

func ScoreCP932(block []byte) int {
	score := 0
	for index := 0; index < len(block); index++ {
		lead := block[index]
		if lead < 0x80 {
			continue
		}
		if lead >= 0xA1 && lead <= 0xDF {
			score -= 3
			continue
		}
		if IsCP932Lead(lead) && index+1 < len(block) && IsCP932Trail(block[index+1]) {
			if lead == 0x82 || lead == 0x83 {
				score += 6
			} else {
				score += 2
			}
			index++
			continue
		}
		score -= 2
	}
	return score
}

func ScoreGBK(block []byte) int {
	score := 0
	for index := 0; index < len(block); index++ {
		lead := block[index]
		if lead < 0x80 {
			continue
		}
		if lead <= 0xFE && index+1 < len(block) && IsGBKTrail(block[index+1]) {
			if lead >= 0xB0 && lead <= 0xF7 {
				score += 3
			} else {
				score += 1
			}
			index++
			continue
		}
		score -= 2
	}
	return score
}

func IsCP932Lead(value byte) bool {
	return (value >= 0x81 && value <= 0x9F) || (value >= 0xE0 && value <= 0xFC)
}

func IsCP932Trail(value byte) bool {
	return (value >= 0x40 && value <= 0x7E) || (value >= 0x80 && value <= 0xFC)
}

func IsGBKLead(value byte) bool {
	return value >= 0x81 && value <= 0xFE
}

func IsGBKTrail(value byte) bool {
	return (value >= 0x40 && value <= 0x7E) || (value >= 0x80 && value <= 0xFE)
}
