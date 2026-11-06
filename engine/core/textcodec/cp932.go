package textcodec

import (
	"kizuna/engine/core/textcodec/tables"
)

func DecodeCP932(b []byte) string {
	var out []rune
	for index := 0; index < len(b); {
		lead := b[index]
		switch {
		case lead < 0x80:
			out = append(out, rune(lead))
			index++
		case lead >= 0xA1 && lead <= 0xDF:
			out = append(out, 0xFF61+rune(lead-0xA1))
			index++
		case IsCP932Lead(lead):
			if index+1 >= len(b) {
				out = append(out, 0xFFFD)
				index++
				continue
			}
			trail := b[index+1]
			if !IsCP932Trail(trail) {
				out = append(out, 0xFFFD)
				index++
				continue
			}
			if r, ok := tables.Cp932Lookup(lead, trail); ok {
				out = append(out, r)
			} else {
				out = append(out, 0xFFFD)
			}
			index += 2
		default:
			if r, ok := tables.Cp932Single(lead); ok {
				out = append(out, r)
			} else {
				out = append(out, 0xFFFD)
			}
			index++
		}
	}
	return string(out)
}

var cp932EncodeMap = buildCp932EncodeMap()

func buildCp932EncodeMap() map[rune]uint32 {
	m := make(map[rune]uint32, 12000)
	put := func(r rune, code uint32) {
		if _, dup := m[r]; !dup {
			m[r] = code
		}
	}
	for b := 0x80; b < 0x100; b++ {
		if r, ok := tables.Cp932Single(byte(b)); ok {
			put(r, 0x10000|uint32(b))
		}
	}
	for lead := 0x80; lead < 0x100; lead++ {
		for trail := 0x40; trail < 0x100; trail++ {
			if r, ok := tables.Cp932Lookup(byte(lead), byte(trail)); ok {
				put(r, 0x20000|uint32(lead<<8|trail))
			}
		}
	}
	return m
}

func EncodeCP932(s string) ([]byte, error) {
	return encodeWithMap(cp932EncodeMap, "cp932", s)
}
