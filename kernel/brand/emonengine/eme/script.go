package eme

import "kizuna/kernel/core/textcodec"

type scriptLine struct {
	body string
	eol  string
}

func splitScriptLines(text string) []scriptLine {
	out := make([]scriptLine, 0, 256)
	start := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c != '\n' && c != '\r' {
			continue
		}
		body := text[start:i]
		eol := "lf"
		if c == '\r' {
			eol = "crlf"
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
		}
		out = append(out, scriptLine{body: body, eol: eol})
		start = i + 1
	}
	if start <= len(text) {
		out = append(out, scriptLine{body: text[start:], eol: ""})
	}
	return out
}

func decodeCp932(raw []byte) string {
	return textcodec.Decode("cp932", raw)
}

func encodeCp932(s string) ([]byte, error) {
	return textcodec.Encode("cp932", s)
}
