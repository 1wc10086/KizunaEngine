package textcodec

import (
	"kizuna/engine/core/textcodec/tables"
)

func DecodeGBK(b []byte) string {
	var out []rune
	for index := 0; index < len(b); {
		lead := b[index]
		switch {
		case lead < 0x80:
			out = append(out, rune(lead))
			index++
		case lead == 0x80:
			out = append(out, 0x20AC)
			index++
		case lead >= 0x81 && lead <= 0xFE:
			if index+1 >= len(b) {
				out = append(out, 0xFFFD)
				index++
				continue
			}
			trail := b[index+1]
			if !IsGBKTrail(trail) {
				out = append(out, 0xFFFD)
				index++
				continue
			}
			if r, ok := tables.GbkLookup(lead, trail); ok {
				out = append(out, r)
			} else {
				out = append(out, 0xFFFD)
			}
			index += 2
		default:
			out = append(out, 0xFFFD)
			index++
		}
	}
	return string(out)
}

var gbkEncodeMap = buildGbkEncodeMap()

func buildGbkEncodeMap() map[rune]uint32 {
	m := make(map[rune]uint32, 24000)
	put := func(r rune, code uint32) {
		if _, dup := m[r]; !dup {
			m[r] = code
		}
	}
	for b := 0x80; b < 0x100; b++ {
		if r, ok := tables.GbkSingle(byte(b)); ok {
			put(r, 0x10000|uint32(b))
		}
	}
	for lead := 0x80; lead < 0x100; lead++ {
		for trail := 0x40; trail < 0x100; trail++ {
			if r, ok := tables.GbkLookup(byte(lead), byte(trail)); ok {
				put(r, 0x20000|uint32(lead<<8|trail))
			}
		}
	}
	return m
}

func EncodeGBK(s string) ([]byte, error) {
	return encodeWithMap(gbkEncodeMap, "gbk", s)
}
