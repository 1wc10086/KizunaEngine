package textcodec

import (
	"strings"

	"kizuna/kernel/core/textcodec/tables"
)

func DecodeCP932(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			sb.WriteByte(c)
			i++
			continue
		}
		if r, ok := tables.Cp932Single(c); ok {
			sb.WriteRune(r)
			i++
			continue
		}
		if isCP932Lead(c) && i+1 < len(b) {
			if r, ok := tables.Cp932Lookup(c, b[i+1]); ok {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(0xFFFD)
			}
			i += 2
			continue
		}
		sb.WriteRune(0xFFFD)
		i++
	}
	return sb.String()
}

func isCP932Lead(c byte) bool {
	return (c >= 0x81 && c <= 0x9F) || (c >= 0xE0 && c <= 0xFC)
}

var cp932EncodeMap = buildEncodeMap(tables.Cp932Single, tables.Cp932Lookup)

func EncodeCP932(s string) ([]byte, error) {
	return encodeWithMap(cp932EncodeMap, "cp932", s)
}
