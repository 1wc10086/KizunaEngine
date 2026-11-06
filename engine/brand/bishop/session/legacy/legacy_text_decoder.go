package legacy

import (
	"strings"

	"kizuna/engine/core/strutil"
)

func CleanText(text string) string {
	return strutil.NormalizeEOL(text)
}

func AtoiText(text string) int {
	value, _ := strutil.TolerantAtoi(text)
	return value
}

func WaitMillisecondsText(text string) int {
	if strings.TrimSpace(text) == "" {
		return 1000
	}
	return 20 * AtoiText(text)
}

func CountdownSecondsText(text string) int {
	value := AtoiText(text)
	if value > 0 {
		return value
	}
	return 50
}

func WithWaveExtension(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToUpper(clean), ".WAV") {
		return clean
	}
	return clean + ".WAV"
}
