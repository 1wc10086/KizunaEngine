package pac

import (
	"os"
	"path/filepath"
	"testing"
)

// 造一个最小的 v1 归档：2 条记录、名字长度 4、数据紧接目录之后。
func buildV1(t *testing.T, dir string, names []string, payloads [][]byte) string {
	t.Helper()
	nameLen := 4
	indexSize := indexHeaderSize + (nameLen+8)*len(names)
	hdr := []byte{byte(len(names)), 0, byte(nameLen), byte(indexSize), byte(indexSize >> 8), byte(indexSize >> 16), byte(indexSize >> 24)}
	dirBytes := make([]byte, 0, indexSize)
	dirBytes = append(dirBytes, hdr...)
	offset := 0
	for i, name := range names {
		pad := make([]byte, nameLen)
		copy(pad, name)
		dirBytes = append(dirBytes, pad...)
		dirBytes = append(dirBytes, byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))
		size := len(payloads[i])
		dirBytes = append(dirBytes, byte(size), byte(size>>8), byte(size>>16), byte(size>>24))
		offset += size
	}
	data := dirBytes
	for _, payload := range payloads {
		data = append(data, payload...)
	}
	path := filepath.Join(dir, "pack.pac")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("写归档: %v", err)
	}
	return path
}

func TestOpenV1AndLookup(t *testing.T) {
	dir := t.TempDir()
	path := buildV1(t, dir, []string{"one1", "two2"}, [][]byte{{1, 2, 3}, {4, 5}})
	a, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if a.Version() != 1 {
		t.Fatalf("版本 = %d", a.Version())
	}
	if a.Count() != 2 {
		t.Fatalf("条目数 = %d", a.Count())
	}
	data, err := a.Read("TWO2")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 2 || data[0] != 4 || data[1] != 5 {
		t.Fatalf("数据 = %v", data)
	}
}

func TestFindGameDirRequiresResourceArchive(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "________")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	// 只有 srp.pac 时不算完整安装。
	srp := filepath.Join(game, "srp.pac")
	_ = os.WriteFile(srp, []byte{1, 0, 1, 7, 0, 0, 0}, 0o644)
	if _, ok := FindGameDir(root); ok {
		t.Fatal("只有 srp.pac 也被认成游戏目录")
	}
	// 补上资源归档后才认。
	if err := os.WriteFile(filepath.Join(game, "se.pac"), []byte{0, 0, 1, 0, 0, 0, 0}, 0o644); err != nil {
		t.Fatalf("写 se.pac: %v", err)
	}
	got, ok := FindGameDir(root)
	if !ok || got != game {
		t.Fatalf("FindGameDir = %q, %v", got, ok)
	}
}
