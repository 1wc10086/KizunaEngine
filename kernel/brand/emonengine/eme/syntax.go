package eme

import "fmt"

func isScriptTranslatable(line string) bool {
	trim := trimLeftSpaceTab(line)
	if trim == "" {
		return false
	}
	if trim[0] == '#' {
		return false
	}
	return true
}

func trimLeftSpaceTab(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

func eolBytes(name string) ([]byte, error) {
	switch name {
	case "crlf":
		return []byte{'\r', '\n'}, nil
	case "lf":
		return []byte{'\n'}, nil
	case "":
		return nil, nil
	}
	return nil, fmt.Errorf("eme: unknown eol %q", name)
}
