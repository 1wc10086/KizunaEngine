package textcodec

import (
	"unicode/utf16"
)

func DecodeUTF16LE(b []byte) string {
	n := len(b) / 2
	u := make([]uint16, 0, n+1)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	r := utf16.Decode(u)
	if len(b)%2 != 0 {
		r = append(r, 0xFFFD)
	}
	return string(r)
}

func EncodeUTF16LE(s string) ([]byte, error) {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(u)*2)
	for _, v := range u {
		out = append(out, byte(v), byte(v>>8))
	}
	return out, nil
}
