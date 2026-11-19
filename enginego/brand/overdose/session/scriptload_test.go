package session

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/core/textcodec"
)

// 汉化剧本可以不放回 script.arc：放到 <game>/script/<name>.asb 即应优先加载。
func TestLooseScriptOverride(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "script")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := buildPlainScript(t, "ユウ", "こんにちは")
	if err := os.WriteFile(filepath.Join(scriptDir, "main.asb"), plain, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Session{dir: dir}
	s.unlockSets.ensure()
	scr, err := s.loadScriptEntry("main")
	if err != nil {
		t.Fatalf("loadScriptEntry: %v", err)
	}
	if scr.Name != "main" || len(scr.Instructions) == 0 {
		t.Fatalf("script = %+v", scr)
	}
	args := scr.Instructions[0].Args
	if len(args) < 2 || args[0].Str != "ユウ" || args[1].Str != "こんにちは" {
		t.Fatalf("args = %+v", args)
	}
}

// buildPlainScript 造一条 0x1f(speaker, body) + 0x08 return 的裸指令流。
func buildPlainScript(t *testing.T, speaker, body string) []byte {
	t.Helper()
	node := func(text string) []byte {
		payload, err := textcodec.EncodeCP932(text)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]byte, 0, len(payload)+3)
		out = append(out, byte(len(payload)+3), 7)
		out = append(out, payload...)
		out = append(out, 0)
		return out
	}
	args := append(node(speaker), node(body)...)
	ins := make([]byte, 12+len(args))
	binary.LittleEndian.PutUint16(ins[0:], uint16(len(ins)))
	binary.LittleEndian.PutUint16(ins[2:], 0x1f)
	binary.LittleEndian.PutUint16(ins[8:], 2)
	copy(ins[12:], args)
	ret := make([]byte, 12)
	binary.LittleEndian.PutUint16(ret[0:], 12)
	binary.LittleEndian.PutUint16(ret[2:], 0x08)
	return append(ins, ret...)
}
