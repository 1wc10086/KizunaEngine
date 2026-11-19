package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGameCacheDirIsStablePerGame(t *testing.T) {
	root := "/tmp/cache"
	first := GameCacheDir(root, "/games/alpha")
	second := GameCacheDir(root, "/games/alpha")
	if first != second {
		t.Fatalf("same game produced different dirs: %q vs %q", first, second)
	}
	other := GameCacheDir(root, "/games/beta")
	if first == other {
		t.Fatalf("different games share a cache dir: %q", first)
	}
	if !strings.HasPrefix(first, root+string(filepath.Separator)) {
		t.Fatalf("cache dir %q is not under %q", first, root)
	}
}

func TestGameCacheDirNormalisesPath(t *testing.T) {
	root := "/tmp/cache"
	// 同一目录的不同写法（尾部分隔符、"."、".."）必须落到同一个 md5：
	// 否则玩家从不同入口打开同一部游戏会得到两份缓存。
	base := GameCacheDir(root, "/games/alpha")
	for _, variant := range []string{
		"/games/alpha/",
		"/games/./alpha",
		"/games/beta/../alpha",
	} {
		if got := GameCacheDir(root, variant); got != base {
			t.Fatalf("%q produced %q, want %q", variant, got, base)
		}
	}
}

func TestGameCacheDirEmptyRoot(t *testing.T) {
	// 宿主没有可用缓存根时原样返回空串，调用方据此跳过缓存，
	// 而不是在进程当前目录里造一个意外的目录。
	if got := GameCacheDir("", "/games/alpha"); got != "" {
		t.Fatalf("empty root produced %q", got)
	}
	if got := GameCacheDir("   ", "/games/alpha"); got != "" {
		t.Fatalf("blank root produced %q", got)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if !EnsureDir(dir) {
		t.Fatalf("EnsureDir(%q) failed", dir)
	}
	// 重复调用必须仍然成功（引擎每次打开都会调）。
	if !EnsureDir(dir) {
		t.Fatalf("EnsureDir(%q) failed on the second call", dir)
	}
	if EnsureDir("") {
		t.Fatal("EnsureDir(\"\") should report failure")
	}
}
