package strutil

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"kizuna/engine/core"
)

func NormalizeEOL(s string) string {
	if s == "" {
		return ""
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func TolerantAtoi(s string) (int, bool) {
	index := 0
	length := len(s)
	for index < length {
		r, size := utf8.DecodeRuneInString(s[index:])
		if !unicode.IsSpace(r) {
			break
		}
		index += size
	}
	sign := 1
	if index < length && (s[index] == '+' || s[index] == '-') {
		if s[index] == '-' {
			sign = -1
		}
		index++
	}
	value := 0
	found := false
	for index < length && s[index] >= '0' && s[index] <= '9' {
		found = true
		value = value*10 + int(s[index]-'0')
		index++
	}
	if !found {
		return 0, false
	}
	return sign * value, true
}

func SplitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func SplitTrimNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); len(part) > 0 {
			trimmed = append(trimmed, part)
		}
	}
	return trimmed
}

func TrimQuotes(s string) string {
	strippedAt := false
	if strings.HasPrefix(s, "@") {
		s = s[1:]
		strippedAt = true
	}
	if len(s) >= 2 {
		quote := s[0]
		if (quote == '"' || quote == '\'') && s[len(s)-1] == quote {
			s = s[1 : len(s)-1]
		}
	}
	if !strippedAt && strings.HasPrefix(s, "@") {
		s = s[1:]
	}
	return s
}

func HasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func HasSuffixFold(s, suffix string) bool {
	return core.HasFoldSuffix(s, suffix)
}

func HasAnySuffixFold(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if HasSuffixFold(s, suffix) {
			return true
		}
	}
	return false
}

func HasAnyExtFold(name string, exts []string) bool {
	extension := filepath.Ext(name)
	if len(extension) == 0 {
		return false
	}
	for _, candidate := range exts {
		if strings.EqualFold(extension, candidate) {
			return true
		}
	}
	return false
}

func SplitTopLevel(text string, separator rune) []string {
	runes := []rune(text)
	parts := make([]string, 0, 2)
	depth := 0
	quote := rune(0)
	start := 0
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}

			continue
		}

		if c == '"' || c == '＂' {
			quote = c
			continue
		}

		switch c {
		case '(', '（', '[', '［', '{', '｛':
			depth++
		case ')', '）', ']', '］', '}', '｝':
			if depth > 0 {
				depth--
			}
		default:
			if c == separator && depth == 0 {
				parts = append(parts, string(runes[start:i]))
				start = i + 1
			}
		}
	}

	parts = append(parts, string(runes[start:]))
	return parts
}
