package textcodec

import (
	"unicode/utf16"
)

func DecodeUTF16LE(text []byte) string {
	var out []rune
	for index := 0; index+1 < len(text); index += 2 {
		unit := uint16(text[index]) | (uint16(text[index+1]) << 8)
		if unit >= 0xD800 && unit <= 0xDBFF {
			if index+3 < len(text) {
				low := uint16(text[index+2]) | (uint16(text[index+3]) << 8)
				if low >= 0xDC00 && low <= 0xDFFF {
					out = append(out, rune(0x10000+(uint32(unit-0xD800)<<10)+uint32(low-0xDC00)))
					index += 2
					continue
				}
			}
			out = append(out, 0xFFFD)
			continue
		}
		if unit >= 0xDC00 && unit <= 0xDFFF {
			out = append(out, 0xFFFD)
			continue
		}
		out = append(out, rune(unit))
	}
	if len(text)%2 != 0 {
		out = append(out, 0xFFFD)
	}
	return string(out)
}

func EncodeUTF16LE(s string) ([]byte, error) {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(u)*2)
	for _, v := range u {
		out = append(out, byte(v), byte(v>>8))
	}
	return out, nil
}
