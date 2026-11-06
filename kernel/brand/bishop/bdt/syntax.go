package bdt

import (
	"strings"
)

type splitLine struct {
	body []byte
	eol  string
}

func splitLines(raw []byte) []splitLine {
	lines := make([]splitLine, 0, 4096)
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\n' {
			continue
		}
		end := i
		eol := "lf"
		if end > start && raw[end-1] == '\r' {
			end--
			eol = "crlf"
		}
		lines = append(lines, splitLine{body: raw[start:end], eol: eol})
		start = i + 1
	}
	if start < len(raw) || len(raw) == 0 {
		var tail []byte
		if start < len(raw) {
			tail = raw[start:]
		} else {
			tail = raw[len(raw):]
		}
		lines = append(lines, splitLine{body: tail, eol: ""})
	}
	return lines
}

func isTranslatable(text string) bool {
	if len(text) == 0 {
		return false
	}
	if text[0] == '#' {
		return false
	}
	if text[0] == '$' {
		return false
	}
	if text[0] == '"' || text[0] == '@' {
		return true
	}
	if isDialogue(text) || isFaceOnly(text) {
		return true
	}
	if strings.Contains(text, "\t\"") {
		return true
	}
	return false
}

func isDialogue(line string) bool {
	pos := 0
	ok, _ := scanPrefix(line, '"', &pos)
	if !ok {
		return false
	}
	body := line[pos+1:]
	if len(body) > 0 && body[len(body)-1] == '"' && strings.IndexByte(body, '"') == len(body)-1 {
		body = body[:len(body)-1]
	}
	return len(body) > 0
}

func isFaceOnly(line string) bool {
	pos := 0
	ok, parts := scanPrefix(line, '@', &pos)
	if !ok {
		return false
	}
	if len(parts[0]) == 0 && len(parts[1]) == 0 && len(parts[2]) == 0 {
		return false
	}
	return len(line[pos+1:]) > 0
}

func scanPrefix(line string, bodyLead byte, pos *int) (bool, [3]string) {
	var parts [3]string
	for {
		for *pos < len(line) && (line[*pos] == ' ' || line[*pos] == '\t') {
			*pos++
		}
		if *pos >= len(line) {
			return false, parts
		}
		if line[*pos] == bodyLead {
			return true, parts
		}
		end := *pos
		for end < len(line) && line[end] != ' ' && line[end] != '\t' {
			end++
		}
		token := line[*pos:end]
		*pos = end
		if isVoiceToken(token) {
			parts[2] = token[1:]
		} else if len(token) > 2 && token[0] == 'F' && token[1] == 'D' {
			parts[0] = token[2:]
		} else if len(token) > 0 && token[0] == '!' {
			parts[1] = token[1:]
		} else {
			return false, parts
		}
	}
}

func isVoiceToken(token string) bool {
	if len(token) < 2 || token[0] != 'V' {
		return false
	}
	for i := 1; i < len(token); i++ {
		if token[i] < '0' || token[i] > '9' {
			return false
		}
	}
	return true
}

func stripTrailingQuote(body string) (string, string) {
	if len(body) > 0 && body[len(body)-1] == '"' && strings.IndexByte(body, '"') == len(body)-1 {
		return body[:len(body)-1], "\""
	}
	return body, ""
}

func splitAffixes(text string) (string, string, string) {
	if len(text) == 0 {
		return "", "", ""
	}
	if text[0] == '"' || text[0] == '@' {
		body := text[1:]
		if text[0] == '"' {
			inner, suffix := stripTrailingQuote(body)
			return text[:1], inner, suffix
		}
		return text[:1], body, ""
	}
	for _, lead := range []byte{'"', '@'} {
		pos := 0
		ok, _ := scanPrefix(text, lead, &pos)
		if !ok {
			continue
		}
		if lead == '@' {
			return text[:pos+1], text[pos+1:], ""
		}
		inner, suffix := stripTrailingQuote(text[pos+1:])
		return text[:pos+1], inner, suffix
	}
	if idx := strings.Index(text, "\t\""); idx >= 0 {
		inner, suffix := stripTrailingQuote(text[idx+2:])
		return text[:idx+2], inner, suffix
	}
	return "", text, ""
}
