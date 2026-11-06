package textcodec

import (
	"strings"

	"kizuna/kernel/core/textcodec/tables"
)

func DecodeGBK(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			sb.WriteByte(c)
			i++
			continue
		}
		if isGBKLead(c) && i+1 < len(b) {
			if r, ok := tables.GbkLookup(c, b[i+1]); ok {
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

func isGBKLead(c byte) bool {
	return c >= 0x81 && c <= 0xFE
}

var gbkEncodeMap = buildEncodeMap(tables.GbkSingle, tables.GbkLookup)

func EncodeGBK(s string) ([]byte, error) {
	return encodeWithMap(gbkEncodeMap, "gbk", s)
}
