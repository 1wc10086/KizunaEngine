package library

import (
	"os"
	"path/filepath"
	"testing"
)

func writeKrkrFile(t *testing.T, dir, name string, size int) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// KiriKiri 的游戏数据目录只有 data.xp3（脚本在归档里），没有散装脚本；
// 扫描必须认出来，并把 DataDirectory 指到真正持有归档的目录。
func TestDescribeKrkrDataArchive(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "この大空に、翼をひろげて")
	writeKrkrFile(t, game, "DATA.XP3", 4096)

	entry, ok := Describe(game)
	if !ok {
		t.Fatal("Describe 未识别 data.xp3 目录")
	}
	if entry.BrandID != "krkr" || entry.BrandName != "KiriKiri" {
		t.Fatalf("brand = %q/%q", entry.BrandID, entry.BrandName)
	}
	if entry.DataDirectory != game {
		t.Fatalf("data dir = %q, want %q", entry.DataDirectory, game)
	}
	if !entry.HasScript || entry.ScriptPath != "start.ks" {
		t.Fatalf("script = %q has=%v", entry.ScriptPath, entry.HasScript)
	}
	// 有 xp3 就算完整安装：权重必须高于门槛，哪怕测试用的归档很小。
	if entry.Weight <= minimumWeight {
		t.Fatalf("weight = %d, want > %d", entry.Weight, minimumWeight)
	}
}

// 父目录里只有补丁/语音归档时，数据目录要落到内含 data.xp3 的子目录。
func TestDescribeKrkrPrefersNestedDataArchive(t *testing.T) {
	root := t.TempDir()
	writeKrkrFile(t, root, "patch.xp3", 4096)
	game := filepath.Join(root, "game")
	writeKrkrFile(t, game, "data.xp3", 4096)

	entry, ok := Describe(root)
	if !ok {
		t.Fatal("Describe 未识别嵌套的 KiriKiri 目录")
	}
	if entry.Kind != KindKrkr || entry.DataDirectory != game {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestDescribeKrkrLooseScripts(t *testing.T) {
	cases := []struct{ name, file string }{
		{"start.tjs", "start.tjs"},
		{"first.ks", "first.ks"},
		{"scenario", filepath.Join("scenario", "prologue.ks")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeKrkrFile(t, root, tc.file, 32)
			entry, ok := Describe(root)
			if !ok {
				t.Fatalf("Describe 未识别只有 %s 的目录", tc.file)
			}
			if entry.Kind != KindKrkr || !entry.HasScript {
				t.Fatalf("entry = %+v", entry)
			}
		})
	}
}

// 普通目录不能被 .ks/.xp3 之外的噪声误判成 KiriKiri。
func TestDescribeRejectsPlainDirectory(t *testing.T) {
	root := t.TempDir()
	writeKrkrFile(t, root, "readme.txt", 32)
	if entry, ok := Describe(root); ok {
		t.Fatalf("普通目录被判成 %+v", entry)
	}
}

// DetectDataDirectory 是宿主可见的入口：四个锚点各自都要能命中，
// 且 data.xp3 必须大小写不敏感。
func TestDetectDataDirectoryKrkr(t *testing.T) {
	cases := []struct{ name, file string }{
		{"data.xp3", "Data.XP3"},
		{"any xp3", "patch.xp3"},
		{"start.tjs", "start.tjs"},
		{"first.ks", "first.ks"},
		{"scenario ks", filepath.Join("scenario", "prologue.ks")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeKrkrFile(t, root, tc.file, 32)
			dir, ok := DetectDataDirectory(root)
			if !ok || dir != root {
				t.Fatalf("DetectDataDirectory = %q, %v", dir, ok)
			}
		})
	}
	if _, ok := DetectDataDirectory(t.TempDir()); ok {
		t.Fatal("空目录被识别成数据目录")
	}
}
