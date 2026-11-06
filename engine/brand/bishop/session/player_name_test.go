package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePlayerNameReadsStartupIni(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 无名目录：只有 Startup.ini 能提供姓名，确保测试走的是文件而不是硬编码表。
	content := "[NAME]\r\nEnable\t= 1\r\nFirst\t= Uehara\r\nLast\t= Daisuke\r\n"
	if err := os.WriteFile(filepath.Join(dataDir, startupIniName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	family, given := resolvePlayerName(filepath.Join(root, "unknown-game"), dataDir)
	if family != "Uehara" || given != "Daisuke" {
		t.Fatalf("name = %q %q, want Uehara Daisuke", family, given)
	}
}

func TestResolvePlayerNameFallsBackToBuiltin(t *testing.T) {
	root := t.TempDir()
	family, given := resolvePlayerName(filepath.Join(root, "黒の教室 モーションプラス"), filepath.Join(root, "data"))
	if family != "上原" || given != "大輔" {
		t.Fatalf("name = %q %q, want 上原 大輔", family, given)
	}
}

func TestResolvePlayerNameRejectsBrokenName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, startupIniName), []byte("[NAME]\nFirst=\nLast=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	family, given := resolvePlayerName(filepath.Join(root, "unknown-game"), root)
	if family != "" || given != "" {
		t.Fatalf("name = %q %q, want empty", family, given)
	}
}
