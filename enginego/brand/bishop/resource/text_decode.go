package resource

import "unicode/utf8"

func decodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}

	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			out = utf8.AppendRune(out, utf8.RuneError)
			i++
			continue
		}
		out = append(out, b[i:i+size]...)
		i += size
	}
	return string(out)
}
