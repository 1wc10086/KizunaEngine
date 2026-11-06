package disc

import (
	"os"
	"path/filepath"
	"testing"
)

// Nero .nrg 的 ISO9660 数据从文件偏移 0 开始，LBA16 处的 CD001 与 .iso 相同。
// 文件末尾的 NER5 footer 不参与 extent 读取，所以这里只构造数据区即可。
func TestNrgImageRequiresISOMagic(t *testing.T) {
	dir := t.TempDir()

	withMagic := filepath.Join(dir, "disc.nrg")
	data := make([]byte, 17*2048)
	copy(data[16*2048:], "\x01CD001\x01")
	if err := os.WriteFile(withMagic, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsContainer(withMagic) {
		t.Errorf("IsContainer(%q) = false, want true", withMagic)
	}
	if !IsContainer(dir) {
		t.Errorf("IsContainer(dir containing .nrg) = false, want true")
	}

	noMagicDir := t.TempDir()
	noMagic := filepath.Join(noMagicDir, "plain.nrg")
	if err := os.WriteFile(noMagic, make([]byte, 17*2048), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsContainer(noMagic) {
		t.Errorf("IsContainer(%q) = true, want false", noMagic)
	}
}
