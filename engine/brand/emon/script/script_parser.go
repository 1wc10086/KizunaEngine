package script

import (
	"strings"
	"unicode"
)

func ParseBytes(name string, scriptBytes []byte) *EmonProgram {
	return Parse(name, DecodeScript(scriptBytes))
}

func Parse(name string, scriptText string) *EmonProgram {
	statements := make([]*EmonStatement, 0, 64)
	lines := SplitLines(scriptText)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		number := int32(i + 1)
		runes := []rune(line)
		start := 0
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}

		if start >= len(runes) {
			continue
		}

		if runes[start] != '#' {
			statements = append(statements, &EmonStatement{
				Raw:    line,
				Line:   number,
				IsText: true,
				Text:   strings.TrimSpace(line),
			})
			continue
		}

		for _, invocation := range splitInvocations(runes, start) {
			statements = append(statements, parseCommand(invocation, line, number))
		}
	}

	return NewEmonProgram(name, statements)
}

func splitInvocations(runes []rune, start int) []string {
	invocations := make([]string, 0, 2)
	segmentStart := start
	inQuotes := false
	for i := start; i < len(runes); i++ {
		value := runes[i]
		if isQuoteRune(value) {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if value == '(' {
				close := findClose(runes, i)
				if close >= 0 {
					i = close
				}
			} else if value == '#' && i > segmentStart {
				invocations = append(invocations, string(runes[segmentStart:i]))
				segmentStart = i
			}
		}
	}

	invocations = append(invocations, string(runes[segmentStart:]))
	return invocations
}

func parseCommand(invocation string, raw string, line int32) *EmonStatement {
	body := []rune(strings.TrimSpace(invocation))
	open := -1
	inQuotes := false
	for i := 1; i < len(body); i++ {
		if isQuoteRune(body[i]) {
			inQuotes = !inQuotes
		} else if body[i] == '(' && !inQuotes {
			open = i
			break
		}
	}

	if open < 0 {
		return &EmonStatement{
			Command: trimCommandName(body),
			Raw:     raw,
			Line:    line,
		}
	}

	close := findClose(body, open)
	var argumentsText string
	if close < 0 {
		argumentsText = string(body[open+1:])
	} else {
		argumentsText = string(body[open+1 : close])
	}

	return &EmonStatement{
		Command:   strings.TrimSpace(string(body[1:open])),
		Raw:       raw,
		Line:      line,
		Arguments: splitArguments(argumentsText),
	}
}

func trimCommandName(body []rune) string {
	if len(body) > 1 {
		return strings.TrimSpace(string(body[1:]))
	}

	return ""
}

func findClose(body []rune, open int) int {
	depth := 1
	inQuotes := false
	for i := open + 1; i < len(body); i++ {
		if isQuoteRune(body[i]) {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if body[i] == '(' {
				depth++
			} else if body[i] == ')' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}

	return -1
}

func splitArguments(argumentsText string) []EmonArgument {
	if strings.TrimSpace(argumentsText) == "" {
		return nil
	}

	runes := []rune(argumentsText)
	arguments := make([]EmonArgument, 0, 4)
	depth := 0
	inQuotes := false
	start := 0
	for i := 0; i < len(runes); i++ {
		value := runes[i]
		if value == '"' {
			inQuotes = !inQuotes
		} else if !inQuotes {
			if value == '(' || value == '[' || value == '{' {
				depth++
			} else if value == ')' || value == ']' || value == '}' {
				if depth > 0 {
					depth--
				}
			} else if value == ',' && depth == 0 {
				arguments = append(arguments, parseArgument(string(runes[start:i])))
				start = i + 1
			}
		}
	}

	arguments = append(arguments, parseArgument(string(runes[start:])))
	return arguments
}

func parseArgument(part string) EmonArgument {
	trimmed := strings.TrimSpace(part)
	runes := []rune(trimmed)
	if len(runes) >= 2 && runes[0] == '"' && runes[len(runes)-1] == '"' {
		return EmonArgument{Raw: string(runes[1 : len(runes)-1]), WasQuoted: true}
	}

	return EmonArgument{Raw: trimmed}
}
