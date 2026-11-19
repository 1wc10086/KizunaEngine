package core

import (
	"os"
	"path/filepath"
	"strings"
)

func HasFoldSuffix(path, suffix string) bool {
	return strings.HasSuffix(strings.ToLower(path), strings.ToLower(suffix))
}

func TrimFoldSuffix(path, suffix string) string {
	if HasFoldSuffix(path, suffix) {
		return path[:len(path)-len(suffix)]
	}
	return path
}

func LowerExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}

func EnsureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func HasFoldPrefix(path, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
}

func baseName(path string) string {
	if index := strings.LastIndexAny(path, "/\\"); index >= 0 {
		return path[index+1:]
	}
	return path
}

func nameExtension(path string) string {
	base := baseName(path)
	ext := LowerExt(base)
	if ext == "" || len(ext) >= len(base) {
		return ""
	}
	return ext
}

func Stem(path string) string {
	base := baseName(path)
	ext := nameExtension(path)
	return base[:len(base)-len(ext)]
}

func TrimExt(path string) string {
	ext := nameExtension(path)
	if ext == "" {
		return path
	}
	return path[:len(path)-len(ext)]
}

func Join(dir, name string) string {
	if dir == "" {
		return name
	}
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}
