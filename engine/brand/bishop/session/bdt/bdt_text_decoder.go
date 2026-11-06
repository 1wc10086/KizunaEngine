package bdt

import (
	"strings"

	"kizuna/engine/core/strutil"
)

type BdtChoiceLabel struct {
	Label string
	Text  string
}

func CleanBdtText(raw string) string {
	if len(raw) == 0 {
		return ""
	}
	text := strutil.NormalizeEOL(raw)
	text = strings.Trim(text, " \t\r\n")
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		text = text[1 : len(text)-1]
	}
	if strings.HasPrefix(text, "@") {
		text = text[1:]
	}
	return text
}

func BdtAtoi(value string) int {
	parsed, _ := strutil.TolerantAtoi(value)
	return parsed
}

func SplitBdtChoices(raw string) []BdtChoiceLabel {
	options := []BdtChoiceLabel{}
	if len(raw) == 0 {
		return options
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}
		comma := strings.IndexByte(line, ',')
		if comma < 0 {
			only := CleanBdtText(line)
			if len(only) > 0 {
				options = append(options, BdtChoiceLabel{Text: only})
			}
			continue
		}
		label := strings.Trim(line[:comma], " \t")
		text := CleanBdtText(line[comma+1:])
		if len(label) == 0 && len(text) == 0 {
			continue
		}
		options = append(options, BdtChoiceLabel{Label: label, Text: text})
	}
	return options
}
