package textcodec

import (
	"unicode/utf8"
)

func DecodeUTF8(b []byte) string {
	return string(b)
}

func DecodeLooseUtf8(text []byte) string {
	out := make([]byte, 0, len(text))
	index := 0
	for index < len(text) {
		c := text[index]
		if c < 0x80 {
			out = append(out, c)
			index++
			continue
		}
		r, size := utf8.DecodeRune(text[index:])
		if r == utf8.RuneError && size <= 1 {
			out = append(out, 0xEF, 0xBF, 0xBD)
			index++
			continue
		}
		out = append(out, text[index:index+size]...)
		index += size
	}
	return string(out)
}
