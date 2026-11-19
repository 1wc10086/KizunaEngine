package save

import (
	"bytes"
	"path/filepath"
	"testing"
)

// buildGlobalSave 造一份结构合法的 common.dat 字节，
// 用来验证"解析 -> 写回"是否严格对称。
//
// 前缀/间隔/尾部的字节都故意填成非零值：全零的数据会让"写错位置"
// 这类 bug 蒙混过关（写偏了也还是零）。
func buildGlobalSave(messages, cgs, scenes int) []byte {
	header := bytes.Repeat([]byte("ab"), globalHeaderSize/2)
	prefix := bytes.Repeat([]byte{0x5A}, 272)
	gap := []byte{0x01, 0x00, 0x00, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}
	tail := bytes.Repeat([]byte{0x33}, 157)

	var out []byte
	out = append(out, header...)
	out = append(out, prefix...)
	out = appendBlock(out, bytes.Repeat([]byte{1}, messages))
	out = append(out, gap...)
	out = appendBlock(out, bytes.Repeat([]byte{1}, cgs))
	out = appendBlock(out, bytes.Repeat([]byte{1}, scenes))
	out = append(out, tail...)
	return out
}

func testExpectations(messages, cgs, scenes int) GlobalExpectations {
	return GlobalExpectations{MessageCount: messages, CgCount: cgs, SceneCount: scenes}
}

func TestParseGlobalSaveRoundTrip(t *testing.T) {
	raw := buildGlobalSave(64, 8, 4)
	want := testExpectations(64, 8, 4)

	save, err := ParseGlobalSave(raw, want)
	if err != nil {
		t.Fatalf("ParseGlobalSave: %v", err)
	}
	if got := len(save.MessageRead); got != 64 {
		t.Fatalf("message table = %d, want 64", got)
	}
	if got := len(save.CgUnlocked); got != 8 {
		t.Fatalf("cg table = %d, want 8", got)
	}
	if got := len(save.SceneUnlocked); got != 4 {
		t.Fatalf("scene table = %d, want 4", got)
	}
	// 写回必须和原字节完全一致，否则会把原引擎的数据改坏。
	if encoded := save.Encode(); !bytes.Equal(encoded, raw) {
		t.Fatalf("Encode mismatch: %d bytes vs %d", len(encoded), len(raw))
	}
}

func TestParseGlobalSaveUnlockFlags(t *testing.T) {
	raw := buildGlobalSave(4, 3, 2)
	save, err := ParseGlobalSave(raw, testExpectations(4, 3, 2))
	if err != nil {
		t.Fatalf("ParseGlobalSave: %v", err)
	}
	for i := 0; i < 3; i++ {
		if !save.CgUnlockedAt(i) {
			t.Fatalf("cg %d should be unlocked", i)
		}
	}
	// 越界索引必须安全返回 false：游戏更新后表会比目录短。
	if save.CgUnlockedAt(3) {
		t.Fatal("out-of-range cg index should be locked")
	}
	if save.CgUnlockedAt(-1) {
		t.Fatal("negative cg index should be locked")
	}
}

func TestGlobalSaveUnlockAndEncode(t *testing.T) {
	save := NewGlobalSave(testExpectations(10, 5, 3))
	if save.CgUnlockedAt(2) {
		t.Fatal("fresh save should be locked")
	}
	if !save.UnlockCg(2) {
		t.Fatal("UnlockCg should report a change")
	}
	if save.UnlockCg(2) {
		t.Fatal("UnlockCg on an unlocked entry should report no change")
	}
	// 落盘再读回来，解锁状态必须还在。
	path := filepath.Join(t.TempDir(), "common.dat")
	if err := save.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadGlobalSave(path, testExpectations(10, 5, 3))
	if err != nil {
		t.Fatalf("LoadGlobalSave: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadGlobalSave returned nil for an existing file")
	}
	if !loaded.CgUnlockedAt(2) {
		t.Fatal("cg unlock did not survive a round trip")
	}
}

func TestGlobalSaveEnsureSizeKeepsFlags(t *testing.T) {
	save := NewGlobalSave(testExpectations(4, 2, 1))
	save.UnlockCg(0)
	// 游戏更新后 cg.dat 变长：补长不能清掉已有解锁位。
	save.EnsureSize(testExpectations(4, 6, 1))
	if len(save.CgUnlocked) != 6 {
		t.Fatalf("cg table = %d, want 6", len(save.CgUnlocked))
	}
	if !save.CgUnlockedAt(0) {
		t.Fatal("existing unlock was lost while growing")
	}
	if save.CgUnlockedAt(5) {
		t.Fatal("newly added entry should stay locked")
	}
}

func TestParseGlobalSaveRejectsCorrupt(t *testing.T) {
	valid := buildGlobalSave(16, 4, 2)
	cases := map[string][]byte{
		"empty":       nil,
		"header only": valid[:globalHeaderSize],
		// 截断到场景表中间：三张核心表必须完整，缺一张就拒绝。
		"cut scene table": valid[:len(valid)-157-2-1],
		// 截断到 CG 表中间。
		"cut cg table": valid[:len(valid)-157-2-4-4-1],
	}
	for name, raw := range cases {
		if _, err := ParseGlobalSave(raw, testExpectations(16, 4, 2)); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	// 只截掉尾部（尾部是不透明数据）应当仍然可解析：三张表都完好，
	// 丢的只是我们本来就不解释的字节。这里宁可接受也不要拒绝，
	// 否则一份尾部被写坏的文件会让玩家的鉴赏解锁全部作废。
	if _, err := ParseGlobalSave(valid[:len(valid)-40], testExpectations(16, 4, 2)); err != nil {
		t.Fatalf("truncated tail should still parse: %v", err)
	}
	// 期望长度对不上（例如把别的作品的存档塞进来）时也必须拒绝，
	// 否则会拿它去覆盖玩家的数据。
	if _, err := ParseGlobalSave(valid, testExpectations(999, 4, 2)); err == nil {
		t.Fatal("mismatched message count should be rejected")
	}
}

func TestLoadGlobalSaveMissingFile(t *testing.T) {
	save, err := LoadGlobalSave(filepath.Join(t.TempDir(), "absent.dat"), testExpectations(1, 1, 1))
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if save != nil {
		t.Fatal("missing file should yield a nil save")
	}
}
