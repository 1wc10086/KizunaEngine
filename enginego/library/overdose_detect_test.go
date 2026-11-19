package library

import (
	"os"
	"path/filepath"
	"testing"
)

// Over-Dose 的数据目录没有 Bishop/Emon 的锚点文件，识别完全依赖
// script.arc/graphic.arc/system.arc。这个测试锁住 library 扫描不会再
// 把「優衣」当成未知目录跳过。
func TestDescribeOverdoseDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"script.arc", "graphic.arc", "system.arc", "sound.arc", "music.arc", "voice.arc"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entry, ok := Describe(root)
	if !ok {
		t.Fatal("Describe 未识别 Over-Dose 目录")
	}
	if entry.BrandID != "overdose" || entry.BrandName != "Over-Dose Moon" {
		t.Fatalf("brand = %q/%q", entry.BrandID, entry.BrandName)
	}
	if entry.DataDirectory != root {
		t.Fatalf("data dir = %q, want %q", entry.DataDirectory, root)
	}
	if !entry.HasScript || entry.ScriptPath != filepath.Join(root, "script.arc") {
		t.Fatalf("script = %q has=%v", entry.ScriptPath, entry.HasScript)
	}
}

// 真数据回归：用户在设置里选 YUI 父目录时，扫描必须列出「優衣」并给出 overdose。
func TestScanRealOverdose(t *testing.T) {
	const root = "/root/code/engine/游戏/YUI"
	if _, err := os.Stat(root); err != nil {
		t.Skip("未找到 YUI 游戏目录")
	}
	// 用户可能选 YUI，也可能选它的上一级（游戏库根）。
	for _, scanRoot := range []string{root, filepath.Dir(root)} {
		entries := Scan(scanRoot)
		found := false
		for _, entry := range entries {
			if entry.BrandID == "overdose" && entry.DataDirectory != "" {
				t.Logf("Scan(%s) found %s -> %s", scanRoot, entry.Name, entry.DataDirectory)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Scan(%s) 没有识别出 overdose: %+v", scanRoot, entries)
		}
	}
}

// 用户选择的是 YUI 父目录时，数据目录应落到 <root>/優衣。
func TestDescribeOverdoseNested(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "優衣")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"script.arc", "graphic.arc", "system.arc"} {
		if err := os.WriteFile(filepath.Join(game, name), []byte("stub"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entry, ok := Describe(root)
	if !ok {
		t.Fatal("Describe 未识别嵌套的 Over-Dose 目录")
	}
	if entry.BrandID != "overdose" || entry.DataDirectory != game {
		t.Fatalf("entry = %+v", entry)
	}
}
