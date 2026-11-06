package tj2

import (
	"strings"

	"kizuna/engine/core/strutil"
)

func CleanTj2Text(text string) string {
	if len(text) == 0 {
		return ""
	}
	normalized := strutil.NormalizeEOL(text)
	var output strings.Builder
	output.Grow(len(normalized))
	ruby := false
	index := 0
	for index < len(normalized) {
		current := normalized[index]
		if current != '_' || index+1 >= len(normalized) {
			if ruby && (current == ',' || current == '>') {
				end := strings.IndexByte(normalized[index:], '>')
				if end < 0 {
					index = len(normalized)
				} else {
					index += end + 1
				}
				ruby = false
				continue
			}
			if current == '_' && index+1 >= len(normalized) {
				index++
				continue
			}
			output.WriteByte(current)
			index++
			continue
		}
		switch normalized[index+1] {
		case 'r', 'R', 't', 'T', 'n':
			output.WriteByte('\n')
		case '<':
			ruby = true
		}
		index += 2
	}
	return output.String()
}

func tj2Atoi(text string) int {
	value, _ := strutil.TolerantAtoi(text)
	return value
}
