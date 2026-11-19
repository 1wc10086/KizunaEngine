package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataSaveDirPrefersExistingGlobalSave(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "Game", "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "save"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 发行包预建了空的 data/save；原引擎的 common.dat 在游戏根 Save/save。
	rootSave := filepath.Join(root, "Save", "save")
	if err := os.MkdirAll(rootSave, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootSave, "common.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dataSaveDir(dataDir); got != rootSave {
		t.Fatalf("dataSaveDir=%q want %q", got, rootSave)
	}
	// 已有 data/save/common.dat 的作品必须继续用 data/save。
	if err := os.WriteFile(filepath.Join(dataDir, "save", "common.dat"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dataSaveDir(dataDir); got != filepath.Join(dataDir, "save") {
		t.Fatalf("dataSaveDir=%q want data/save", got)
	}
}
