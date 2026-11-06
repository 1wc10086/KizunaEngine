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
