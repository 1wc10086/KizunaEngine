package gothic

import "strings"

var gothicCommandWords = map[string]struct{}{
	"init": {}, "cls": {}, "end": {}, "logo": {}, "startmenu": {}, "staffroll": {}, "game": {}, "gosub": {},
	"goto": {}, "return": {}, "pause": {}, "music": {}, "g": {}, "t": {}, "text": {}, "if": {}, "elseif": {},
	"else": {}, "endif": {}, "int": {}, "item": {}, "itemif": {}, "menu": {}, "endmenu": {}, "break": {},
	"case": {}, "caseif": {}, "default": {}, "call": {}, "exec": {}, "macro": {}, "endm": {}, "print": {},
	"printc": {}, "scroll": {}, "page": {}, "copy": {}, "copy2": {}, "map": {}, "restart": {}, "resume": {},
	"lwait": {}, "memo": {}, "mode": {}, "w": {}, "tilt": {}, "smap": {}, "set": {}, "ani": {}, "dmap": {},
	"wave": {}, "rmap": {}, "inputname": {},
}

var gothicSubCommandWords = map[string]struct{}{
	"g": {}, "music": {}, "t": {}, "text": {}, "game": {}, "w": {}, "wave": {}, "set": {},
}

func GothicParseStatement(raw string, index int32, offset int64) *GothicStatement {
	line := raw
	view := strings.TrimRight(line, " \t\r\n")
	head := strings.TrimLeft(view, " \t")
	if len(head) == 0 {
		return NewGothicStatement(GothicStatementEmpty, index, line, "", "", []string{}, []string{}, offset)
	}
	if head[0] == '%' {
		mark := "%"
		if strings.HasPrefix(head, "%%") {
			mark = "%%"
		}
		return NewGothicStatement(GothicStatementPageBreak, index, line, mark, "", []string{}, []string{}, offset)
	}
	if head[0] != '#' {
		return NewGothicStatement(GothicStatementText, index, line, "", "", []string{}, []string{}, offset)
	}
	if len(head) >= 2 && head[1] == ':' {
		return NewGothicStatement(GothicStatementLabel, index, line, strings.TrimSpace(head[2:]), "", []string{}, []string{}, offset)
	}
	tokens := splitGothicTokens(head[1:])
	if len(tokens) == 0 {
		return NewGothicStatement(GothicStatementEmpty, index, line, "", "", []string{}, []string{}, offset)
	}
	word := strings.ToLower(tokens[0].value)
	if word == "end" {
		return NewGothicStatement(GothicStatementFinish, index, line, word, "", argValues(tokens, 1), groupValues(tokens, 1, false), offset)
	}
	if _, ok := gothicCommandWords[strings.ToLower(tokens[0].value)]; ok {
		sub := ""
		skipFirst := false
		if _, ok := gothicSubCommandWords[strings.ToLower(tokens[0].value)]; ok && len(tokens) >= 2 {
			sub = strings.ToLower(tokens[1].value)
			skipFirst = true
		}
		return NewGothicStatement(GothicStatementCommand, index, line, word, sub, argValues(tokens, 1), groupValues(tokens, 1, skipFirst), offset)
	}
	if len(tokens) >= 3 && tokens[1].value == "=" {
		return NewGothicStatement(GothicStatementAssignment, index, line, tokens[0].value, tokens[2].value, argValues(tokens, 1), groupValues(tokens, 1, false), offset)
	}
	return NewGothicStatement(GothicStatementCommand, index, line, word, "", argValues(tokens, 1), groupValues(tokens, 1, false), offset)
}

type gothicToken struct {
	value      string
	commaAfter bool
}

func splitGothicTokens(body string) []gothicToken {
	var tokens []gothicToken
	var current strings.Builder
	inQuotes := false
	for _, ch := range body {
		if ch == '"' {
			inQuotes = !inQuotes
			current.WriteRune(ch)
		} else if !inQuotes && (ch == ' ' || ch == '\t' || ch == ',') {
			if current.Len() > 0 {
				tokens = append(tokens, gothicToken{value: current.String(), commaAfter: ch == ','})
				current.Reset()
			} else if ch == ',' && len(tokens) > 0 {
				tokens[len(tokens)-1].commaAfter = true
			}
		} else {
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, gothicToken{value: current.String()})
	}
	return tokens
}

func argValues(tokens []gothicToken, start int) []string {
	args := []string{}
	for i := start; i < len(tokens); i++ {
		args = append(args, tokens[i].value)
	}
	return args
}

func groupValues(tokens []gothicToken, start int, skipFirst bool) []string {
	groups := []string{}
	var current []string
	i := start
	if skipFirst && start < len(tokens) {
		i++
	}
	for ; i < len(tokens); i++ {
		current = append(current, tokens[i].value)
		if tokens[i].commaAfter {
			groups = append(groups, strings.Join(current, " "))
			current = nil
		}
	}
	if len(current) > 0 {
		groups = append(groups, strings.Join(current, " "))
	}
	return groups
}
