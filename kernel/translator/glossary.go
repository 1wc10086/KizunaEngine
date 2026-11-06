package translator

import (
	"strings"
)

type glossaryEntry struct {
	src string
	dst string
}

func parseGlossary(s string) []glossaryEntry {
	var out []glossaryEntry
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		src, dst := splitGlossaryLine(line)
		src = strings.TrimSpace(src)
		dst = strings.TrimSpace(dst)
		if src == "" || dst == "" {
			continue
		}
		out = append(out, glossaryEntry{src: src, dst: dst})
	}
	return out
}

func splitGlossaryLine(line string) (string, string) {
	for _, sep := range []string{"=", "＝", "→", "->", "：", ":"} {
		if idx := strings.Index(line, sep); idx >= 0 {
			return line[:idx], line[idx+len(sep):]
		}
	}
	return "", ""
}

func matchGlossary(entries []glossaryEntry, texts []string) []glossaryEntry {
	var out []glossaryEntry
	for _, e := range entries {
		for _, t := range texts {
			if strings.Contains(t, e.src) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

func formatGlossary(entries []glossaryEntry) string {
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, e.src+"＝"+e.dst)
	}
	return strings.Join(lines, "\n")
}
