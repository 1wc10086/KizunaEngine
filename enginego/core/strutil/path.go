package strutil

import "strings"

func baseName(path string) string {
	if index := strings.LastIndexAny(path, "/\\"); index >= 0 {
		return path[index+1:]
	}
	return path
}

func extensionOf(base string) string {
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 {
		return ""
	}
	return base[dot:]
}

func Stem(path string) string {
	base := baseName(path)
	ext := extensionOf(base)
	return base[:len(base)-len(ext)]
}

func TrimExt(path string) string {
	ext := extensionOf(baseName(path))
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}

func ExtFold(path string) string {
	return strings.ToLower(extensionOf(baseName(path)))
}
