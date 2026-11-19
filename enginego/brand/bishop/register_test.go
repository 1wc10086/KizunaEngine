package bishop

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDetectFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBrandIdentity(t *testing.T) {
	var b brand
	if b.ID() != "bishop" || b.Name() != "Bishop" {
		t.Fatalf("id=%q name=%q", b.ID(), b.Name())
	}
	if b.New() == nil {
		t.Fatal("New returned nil")
	}
}

func TestDetect(t *testing.T) {
	modern := t.TempDir()
	writeDetectFile(t, filepath.Join(modern, "bsxx.dat"), []byte("x"))
	if !detect(modern) {
		t.Fatal("modern bsxx.dat not detected")
	}
	nested := t.TempDir()
	writeDetectFile(t, filepath.Join(nested, "game", "data", "BSS.DAT"), []byte("x"))
	if !detect(nested) {
		t.Fatal("nested BSS.DAT not detected")
	}
	legacy := t.TempDir()
	writeDetectFile(t, filepath.Join(legacy, "SCNFILE.PFT"), []byte("x"))
	writeDetectFile(t, filepath.Join(legacy, "bsxx.dat"), []byte("x"))
	if detect(legacy) {
		t.Fatal("legacy dir must not detect as bishop")
	}
	tj2 := t.TempDir()
	writeDetectFile(t, filepath.Join(tj2, "SCENE.PFT"), []byte("x"))
	writeDetectFile(t, filepath.Join(tj2, "bsx.dat"), []byte("x"))
	if detect(tj2) {
		t.Fatal("tj2 dir must not detect as bishop")
	}
	bdt := t.TempDir()
	writeDetectFile(t, filepath.Join(bdt, "scene00.bdt"), []byte("x"))
	writeDetectFile(t, filepath.Join(bdt, "bsx.dat"), []byte("x"))
	if detect(bdt) {
		t.Fatal("bdt dir must not detect as bishop")
	}
	emon := t.TempDir()
	writeDetectFile(t, filepath.Join(emon, "emecfg.ecf"), []byte("x"))
	writeDetectFile(t, filepath.Join(emon, "bsxx.dat"), []byte("x"))
	if detect(emon) {
		t.Fatal("emon dir must not detect as bishop")
	}
	if detect(t.TempDir()) {
		t.Fatal("empty dir must not detect")
	}
	if detect("") {
		t.Fatal("empty path must not detect")
	}
}
