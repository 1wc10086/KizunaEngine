package save

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
)

func TestContainerRoundTrip(t *testing.T) {
	bodies := [][]byte{
		{},
		{0x41},
		{0x41, 0x42},
		[]byte("hello save data"),
		makePattern(100),
		makePattern(1000),
	}
	for _, lengthPrefixed := range []bool{true, false} {
		for i, body := range bodies {
			magic := SaveFileMagic("")
			file := Pack(magic, body, lengthPrefixed)
			prefix := PrefixSize
			if lengthPrefixed {
				prefix += lengthSize
			}
			if len(file) != prefix+len(body) {
				t.Fatalf("prefixed=%v case %d: size %d want %d", lengthPrefixed, i, len(file), prefix+len(body))
			}
			payload, reason, ok := TryUnpack(file, magic, lengthPrefixed)
			if !ok {
				t.Fatalf("prefixed=%v case %d: unpack failed: %s", lengthPrefixed, i, reason)
			}
			if !bytes.Equal(payload, body) {
				t.Fatalf("prefixed=%v case %d: payload mismatch", lengthPrefixed, i)
			}
		}
	}
}

func TestContainerByteLayout(t *testing.T) {
	body := []byte{1, 2, 3, 4, 5, 6, 7}
	magic := SaveFileMagic("")
	file := Pack(magic, body, true)
	wantMagic := []byte{'E', 'M', 'O', 'N', '-', 'S', 'A', 'V', 'E', 'D', 'A', 'T', 'A', '-', '1', 0}
	if !bytes.Equal(file[0:16], wantMagic) {
		t.Fatalf("magic = %x want %x", file[0:16], wantMagic)
	}
	wantMethods := []byte{1, 4, 2, 8, 0, 0, 0, 0}
	if !bytes.Equal(file[16:24], wantMethods) {
		t.Fatalf("methods = %x want %x", file[16:24], wantMethods)
	}
	wantSubs := []uint32{0x89E73CDB, 0x11, 0x4D1483CC, 8, 0, 0, 0, 0}
	for i, want := range wantSubs {
		got := binary.LittleEndian.Uint32(file[24+i*4 : 28+i*4])
		if got != want {
			t.Fatalf("subkey %d = %#x want %#x", i, got, want)
		}
	}
	if length := binary.LittleEndian.Uint32(file[56:60]); length != 7 {
		t.Fatalf("length = %d want 7", length)
	}
	key := make([]byte, KeySize)
	copy(key, file[16:56])
	encrypted := make([]byte, 7)
	copy(encrypted, file[60:67])
	if err := eme.DecryptRange(encrypted, 0, len(encrypted), key); err != nil {
		t.Fatalf("independent decrypt: %v", err)
	}
	if !bytes.Equal(encrypted, body) {
		t.Fatalf("independent decrypt mismatch: got %x want %x", encrypted, body)
	}
	if _, _, ok := TryUnpack(file, SaveFileMagic("other"), true); ok {
		t.Fatalf("wrong magic unpacked")
	}
	truncated := file[:len(file)-1]
	if _, _, ok := TryUnpack(truncated, magic, true); ok {
		t.Fatalf("truncated file unpacked")
	}
	if _, _, ok := TryUnpack(file[:10], magic, true); ok {
		t.Fatalf("short header unpacked")
	}
}

func TestSavePayloadJSONShape(t *testing.T) {
	now := time.Date(2024, 5, 6, 7, 8, 9, 0, time.FixedZone("JST", 9*3600))
	payload := &EmonSavePayload{
		Slot:           3,
		Comment:        "test",
		SavedAt:        now,
		ScriptFile:     "main.txt",
		StatementIndex: 42,
		Variables: map[string]script.EmonValue{
			"num":  script.EmonValueFromInt(7),
			"text": script.EmonValueFromString("hi"),
			"ref":  script.EmonValueFromIdentifier("SomeLabel"),
		},
		SystemVariables: map[string]script.EmonValue{},
		MacroStack: []*script.EmonMacroFrameState{
			{Name: "M", ReturnFile: "f.txt", CallerStatement: 1, ReturnStatement: 2, Arguments: map[string]script.EmonValue{}},
		},
		Frames: []*EmonFrameState{
			{Program: "p", Index: 1, Macro: "m", CallerProgram: "c", CallerStatement: 2, ReturnStatement: 3, Arguments: map[string]script.EmonValue{}},
		},
		AudioState:   map[string]string{"bgm": "song.ogg"},
		SpriteStates: []*EmonSpriteState{{Name: "sp", Visible: true, Flags: stage.EmonSpriteFlagsVisible, X: 1, Y: 2, Width: 3, Height: 4, Kind: stage.EmonSpriteKindGroup, Alpha: 200}},
		Preview:      "abc",
	}
	raw, err := MarshalSavePayload(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"Slot":3`, `"Comment":"test"`, `"ScriptFile":"main.txt"`, `"StatementIndex":42`, `"num":{"i":7}`, `"text":{"s":"hi"}`, `"ref":{"id":"SomeLabel"}`, `"MacroStack":[{`, `"Name":"M"`, `"ReturnFile":"f.txt"`, `"Program":"p"`, `"CallerProgram":"c"`, `"AudioState":{"bgm":"song.ogg"}`, `"Flags":65536`, `"Kind":32`, `"Preview":"abc"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("json missing %s in %s", want, text)
		}
	}
	if strings.Contains(text, `"F3D"`) {
		t.Fatalf("nil F3D must be omitted: %s", text)
	}
	back, err := UnmarshalSavePayload(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Slot != 3 || back.Variables["num"].AsInt() != 7 || back.Variables["text"].AsString() != "hi" || !back.Variables["ref"].IsIdentifier() {
		t.Fatalf("variable round trip failed")
	}
	if !back.SavedAt.Equal(now) {
		t.Fatalf("time round trip failed: %v vs %v", back.SavedAt, now)
	}
	if back.Frames[0].Program != "p" || back.SpriteStates[0].Flags != stage.EmonSpriteFlagsVisible {
		t.Fatalf("frame/sprite round trip failed")
	}
}

func TestSaveStoreRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), SaveDirectoryName)
	store := NewEmonSaveStore(dir)
	data := &EmonSaveData{
		Slot:           7,
		Comment:        "hello",
		SavedAt:        time.Now().Truncate(time.Second),
		ScriptFile:     "ASMainScript.txt",
		StatementIndex: 11,
		Variables:      map[string]script.EmonValue{"A": script.EmonValueFromInt(1)},
		SystemVariables: map[string]script.EmonValue{
			"B": script.EmonValueFromString("x"),
		},
		MacroStack: []*script.EmonMacroFrameState{
			{Name: "M", ReturnFile: "r", CallerStatement: 1, ReturnStatement: 2, Arguments: map[string]script.EmonValue{"P": script.EmonValueFromInt(9)}},
		},
		Frames:     []*EmonFrameState{{Program: "p", Index: 5, Arguments: map[string]script.EmonValue{}}},
		AudioState: map[string]string{"k": "v"},
		SpriteStates: []*EmonSpriteState{
			{Name: "sp", Visible: false, X: 10, Alpha: 128},
			{Name: "", Visible: true},
		},
		Preview: "preview",
	}
	if !store.Write(data) {
		t.Fatalf("write failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "Save007.dat")); err != nil {
		t.Fatalf("slot file missing: %v", err)
	}
	if !store.Exists(7) || store.Exists(8) {
		t.Fatalf("exists mismatch")
	}
	if slots := store.Slots(); len(slots) != 1 || slots[0] != 7 {
		t.Fatalf("slots = %v", slots)
	}
	back := store.Read(7)
	if back == nil {
		t.Fatalf("read returned nil")
	}
	if back.Slot != 7 || back.Comment != "hello" || back.StatementIndex != 11 || back.Preview != "preview" {
		t.Fatalf("scalar round trip failed: %+v", back)
	}
	if back.Variables["A"].AsInt() != 1 || back.SystemVariables["B"].AsString() != "x" {
		t.Fatalf("variable round trip failed")
	}
	if len(back.MacroStack) != 1 || back.MacroStack[0].Arguments["P"].AsInt() != 9 {
		t.Fatalf("macro stack round trip failed")
	}
	if len(back.SpriteStates) != 1 || back.SpriteStates[0].Name != "sp" || back.SpriteStates[0].Visible {
		t.Fatalf("sprite filter/round trip failed: %+v", back.SpriteStates)
	}
	if store.Read(8) != nil || store.Read(1000) != nil {
		t.Fatalf("absent/out-of-range read must be nil")
	}
	if !store.Delete(7) || store.Exists(7) || len(store.Slots()) != 0 {
		t.Fatalf("delete failed")
	}
}

func TestSystemStateRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), SaveDirectoryName)
	store := NewEmonSystemStateStore(dir)
	variables := map[string]script.EmonValue{
		"TitleFlag": script.EmonValueFromInt(1),
		"Name":      script.EmonValueFromString("test"),
	}
	if !store.Write(variables) {
		t.Fatalf("write failed")
	}
	file, err := os.ReadFile(store.FilePath())
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	wantMagic := []byte{'E', 'M', 'E', 'S', 'Y', 'S', '_', 'V', 'E', 'R', '1', '.', '2', '0', 0, 0}
	if !bytes.Equal(file[0:16], wantMagic) {
		t.Fatalf("system magic = %x want %x", file[0:16], wantMagic)
	}
	fresh := script.NewEmonVariableStore()
	loader := NewEmonSystemStateStore(dir)
	if !loader.Load(fresh) {
		t.Fatalf("load failed")
	}
	if fresh.Lookup("TitleFlag").AsInt() != 1 || fresh.Lookup("Name").AsString() != "test" {
		t.Fatalf("system variable round trip failed")
	}
	payload, reason, ok := TryUnpack(file, SystemMagicBytes(), false)
	if !ok {
		t.Fatalf("system unpack failed: %s", reason)
	}
	state, err := UnmarshalSystemStatePayload(payload)
	if err != nil {
		t.Fatalf("system json: %v", err)
	}
	if state.Version != FormatVersion || state.SystemVariables["TitleFlag"].AsInt() != 1 {
		t.Fatalf("system payload mismatch")
	}
}

func TestLegacyScan(t *testing.T) {
	payload := buildLegacyPayload(t)
	variables, ok := TryReadLegacySystemFile(payload)
	if !ok {
		t.Fatalf("legacy scan failed")
	}
	if variables["Score"].AsInt() != 12345 {
		t.Fatalf("int var = %d", variables["Score"].AsInt())
	}
	if got := variables["PlayerName"].AsString(); got != "test" {
		t.Fatalf("string var = %q", got)
	}
	if got := variables["Ratio"].AsString(); got != "0.5" {
		t.Fatalf("float var = %q", got)
	}
	broken := append([]byte{}, payload...)
	broken[len(broken)-1] ^= 0xFF
	if _, ok := TryReadLegacySystemFile(broken); ok {
		t.Fatalf("broken trailer must fail")
	}
	if _, ok := TryReadLegacySystemFile([]byte{1, 2, 3}); ok {
		t.Fatalf("short payload must fail")
	}
}

func TestSaveFileMagic(t *testing.T) {
	if got := SaveFileMagic(""); !bytes.Equal(got, []byte{'E', 'M', 'O', 'N', '-', 'S', 'A', 'V', 'E', 'D', 'A', 'T', 'A', '-', '1', 0}) {
		t.Fatalf("fallback magic = %x", got)
	}
	if got := SaveFileMagic("ABCDEFGHIJKLMNOPQRSTUVWXYZ"); !bytes.Equal(got, []byte("ABCDEFGHIJKLMNOP")) {
		t.Fatalf("truncated magic = %q", got)
	}
}

func makePattern(n int) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte(i*37 + 11)
	}
	return buf
}

func buildLegacyPayload(t *testing.T) []byte {
	t.Helper()
	var body []byte
	putI32 := func(v int32) {
		var tmp [4]byte
		binary.LittleEndian.PutUint32(tmp[:], uint32(v))
		body = append(body, tmp[:]...)
	}
	putName := func(name string) {
		encoded := EncodeCP932(name)
		if len(encoded) >= legacyNameSize {
			t.Fatalf("name too long")
		}
		field := make([]byte, legacyNameSize)
		copy(field, encoded)
		body = append(body, field...)
	}
	records := func() {
		putName("Score")
		putI32(0)
		putI32(12345)
		putName("PlayerName")
		putI32(0x10)
		putI32(4)
		body = append(body, EncodeCP932("test")...)
		putName("Ratio")
		putI32(4)
		var tmp [4]byte
		binary.LittleEndian.PutUint32(tmp[:], 0x3F000000)
		body = append(body, tmp[:]...)
	}
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], 3)
	body = append(body, count[:]...)
	records()
	for i := 0; i < legacyTrailerSize; i++ {
		body = append(body, legacyPadPattern[i%len(legacyPadPattern)])
	}
	return body
}
