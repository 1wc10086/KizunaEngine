package strutil

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kizuna/engine/core"
)

func TestStem(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"", ""},
		{"a", "a"},
		{"a.txt", "a"},
		{"dir/a.txt", "a"},
		{"dir/sub/a.tar.gz", "a.tar"},
		{"a.", "a"},
		{".hidden", ".hidden"},
		{"dir/.hidden", ".hidden"},
		{"a.b/c", "c"},
		{"dir/a.b/c", "c"},
		{"dir\\a.txt", "a"},
		{"dir\\sub\\a.txt", "a"},
	}
	for _, c := range cases {
		if got := Stem(c.path); got != c.want {
			t.Fatalf("Stem(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestTrimExt(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"", ""},
		{"a", "a"},
		{"a.txt", "a"},
		{"dir/a.txt", "dir/a"},
		{"dir/sub/a.tar.gz", "dir/sub/a.tar"},
		{"a.", "a"},
		{".hidden", ".hidden"},
		{"dir/.hidden", "dir/.hidden"},
		{"a.b/c", "a.b/c"},
		{"dir\\a.txt", "dir\\a"},
	}
	for _, c := range cases {
		if got := TrimExt(c.path); got != c.want {
			t.Fatalf("TrimExt(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestExtFold(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"", ""},
		{"a", ""},
		{"a.TXT", ".txt"},
		{"dir/A.Bin", ".bin"},
		{"a.tar.GZ", ".gz"},
		{"dir.x/a", ""},
		{".hidden", ""},
		{"a.", "."},
		{"dir\\a.PFT", ".pft"},
	}
	for _, c := range cases {
		if got := ExtFold(c.path); got != c.want {
			t.Fatalf("ExtFold(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestCoreClamp(t *testing.T) {
	cases := []struct {
		value int
		low   int
		high  int
		want  int
	}{
		{5, 1, 3, 3},
		{-1, 1, 3, 1},
		{2, 1, 3, 2},
		{1, 1, 3, 1},
		{3, 1, 3, 3},
		{0, 0, 0, 0},
		{-5, -3, -1, -3},
		{-9, -1, -3, -1},
	}
	for _, c := range cases {
		if got := core.Clamp(c.value, c.low, c.high); got != c.want {
			t.Fatalf("core.Clamp(%d, %d, %d) = %d, want %d", c.value, c.low, c.high, got, c.want)
		}
	}

	int32Cases := []struct {
		value int32
		low   int32
		high  int32
		want  int32
	}{
		{5, 1, 3, 3},
		{-1, 1, 3, 1},
		{2, 1, 3, 2},
		{-2147483648, -10, 10, -10},
		{2147483647, -10, 10, 10},
	}
	for _, c := range int32Cases {
		if got := core.ClampInt32(c.value, c.low, c.high); got != c.want {
			t.Fatalf("core.ClampInt32(%d, %d, %d) = %d, want %d", c.value, c.low, c.high, got, c.want)
		}
	}
}

func TestCoreAbs(t *testing.T) {
	if got := core.Abs(-7); got != 7 {
		t.Fatalf("core.Abs(-7) = %d, want 7", got)
	}
	if got := core.Abs(7); got != 7 {
		t.Fatalf("core.Abs(7) = %d, want 7", got)
	}
	if got := core.Abs(0); got != 0 {
		t.Fatalf("core.Abs(0) = %d, want 0", got)
	}
	if got := core.AbsInt32(-2147483647); got != 2147483647 {
		t.Fatalf("core.AbsInt32(-2147483647) = %d, want 2147483647", got)
	}
	if got := core.AbsInt32(2147483647); got != 2147483647 {
		t.Fatalf("core.AbsInt32(2147483647) = %d, want 2147483647", got)
	}
	if got := core.AbsInt32(0); got != 0 {
		t.Fatalf("core.AbsInt32(0) = %d, want 0", got)
	}
}

func TestCoreFileAndDirExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if !core.FileExists(file) {
		t.Fatalf("core.FileExists(%q) = false, want true", file)
	}
	if core.FileExists(dir) {
		t.Fatalf("core.FileExists(%q) = true for a directory", dir)
	}
	if core.FileExists(filepath.Join(dir, "missing.txt")) {
		t.Fatalf("core.FileExists reported a missing path")
	}
	if !core.DirExists(dir) {
		t.Fatalf("core.DirExists(%q) = false, want true", dir)
	}
	if core.DirExists(file) {
		t.Fatalf("core.DirExists(%q) = true for a file", file)
	}
	if core.DirExists(filepath.Join(dir, "missing")) {
		t.Fatalf("core.DirExists reported a missing path")
	}

	text, err := core.ReadFileString(file)
	if err != nil {
		t.Fatalf("core.ReadFileString: %v", err)
	}
	if text != "hello" {
		t.Fatalf("core.ReadFileString = %q, want %q", text, "hello")
	}
	if _, err := core.ReadFileString(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatalf("core.ReadFileString returned no error for a missing file")
	}
}

func TestCoreListFilesTop(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.txt", "a.txt", "c.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatalf("create empty temp dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write nested temp file: %v", err)
	}

	names, err := core.ListFilesTop(dir)
	if err != nil {
		t.Fatalf("core.ListFilesTop: %v", err)
	}
	want := []string{"a.txt", "b.txt", "c.bin"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("core.ListFilesTop(%q) = %#v, want %#v", dir, names, want)
	}

	empty, err := core.ListFilesTop(filepath.Join(dir, "empty"))
	if err != nil {
		t.Fatalf("core.ListFilesTop(empty): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("core.ListFilesTop(empty) = %#v, want no entries", empty)
	}

	sub, err := core.ListFilesTop(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatalf("core.ListFilesTop(sub): %v", err)
	}
	subWant := []string{"nested.txt"}
	if !reflect.DeepEqual(sub, subWant) {
		t.Fatalf("core.ListFilesTop(sub) = %#v, want %#v", sub, subWant)
	}

	if _, err := core.ListFilesTop(filepath.Join(dir, "missing")); err == nil {
		t.Fatalf("core.ListFilesTop returned no error for a missing directory")
	}
}

func TestCorePathHelpers(t *testing.T) {
	if !core.HasFoldPrefix("Scene.PFT", "scene") {
		t.Fatalf("core.HasFoldPrefix did not match ignoring case")
	}
	if core.HasFoldPrefix("cene.PFT", "scene") {
		t.Fatalf("core.HasFoldPrefix matched an unrelated prefix")
	}
	if got := core.Stem("dir/a.tar.gz"); got != "a.tar" {
		t.Fatalf("core.Stem = %q, want %q", got, "a.tar")
	}
	if got := core.Stem("dir/.hidden"); got != ".hidden" {
		t.Fatalf("core.Stem = %q, want %q", got, ".hidden")
	}
	if got := core.Stem("a.b/c"); got != "c" {
		t.Fatalf("core.Stem = %q, want %q", got, "c")
	}
	if got := core.TrimExt("dir/a.tar.gz"); got != "dir/a.tar" {
		t.Fatalf("core.TrimExt = %q, want %q", got, "dir/a.tar")
	}
	if got := core.TrimExt("a.b/c"); got != "a.b/c" {
		t.Fatalf("core.TrimExt = %q, want %q", got, "a.b/c")
	}
	if got := core.Join("dir", "a.txt"); got != "dir/a.txt" {
		t.Fatalf("core.Join = %q, want %q", got, "dir/a.txt")
	}
	if got := core.Join("dir/", "a.txt"); got != "dir/a.txt" {
		t.Fatalf("core.Join = %q, want %q", got, "dir/a.txt")
	}
	if got := core.Join("", "a.txt"); got != "a.txt" {
		t.Fatalf("core.Join = %q, want %q", got, "a.txt")
	}
}
