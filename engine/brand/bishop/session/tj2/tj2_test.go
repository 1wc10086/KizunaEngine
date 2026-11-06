package tj2

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
)

func TestTj2Clean(t *testing.T) {
	cases := map[string]string{
		"abc":       "abc",
		"a_rb":      "a\nb",
		"a_nb":      "a\nb",
		"a_tb":      "a\nb",
		"a_r<>b":    "a\n<>b",
		"a_r<,x>b":  "a\n<,x>b",
		"a\r\nb\rc": "a\nb\nc",
		"x_qy":      "xy",
		"trail_":    "trail",
	}
	for in, want := range cases {
		if got := CleanTj2Text(in); got != want {
			t.Errorf("Clean(%q)=%q want %q", in, got, want)
		}
	}
	if got := tj2Atoi("  -12x"); got != -12 {
		t.Errorf("Atoi=%d want -12", got)
	}
	if got := tj2Atoi("abc"); got != 0 {
		t.Errorf("Atoi=%d want 0", got)
	}
}

func buildTj2Table(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{'E', 'X', 'S', 'C', 'H', 'E', 'D', 0})
	var scratch [4]byte
	binary.LittleEndian.PutUint32(scratch[:], 28)
	buf.Write(scratch[:])
	binary.LittleEndian.PutUint32(scratch[:], 0)
	buf.Write(scratch[:])
	binary.LittleEndian.PutUint32(scratch[:], 220)
	buf.Write(scratch[:])
	binary.LittleEndian.PutUint32(scratch[:], 1)
	buf.Write(scratch[:])
	binary.LittleEndian.PutUint32(scratch[:], 1)
	buf.Write(scratch[:])
	var counts [32]byte
	binary.LittleEndian.PutUint16(counts[0:], 3)
	binary.LittleEndian.PutUint16(counts[2:], 4)
	binary.LittleEndian.PutUint16(counts[4:], 1)
	binary.LittleEndian.PutUint16(counts[6:], 2)
	binary.LittleEndian.PutUint16(counts[8:], 1)
	binary.LittleEndian.PutUint16(counts[10:], 1)
	binary.LittleEndian.PutUint32(counts[12:], 1)
	binary.LittleEndian.PutUint16(counts[16:], 8)
	binary.LittleEndian.PutUint32(counts[20:], 4096)
	binary.LittleEndian.PutUint32(counts[28:], 9)
	buf.Write(counts[:])
	buf.WriteByte(0)
	writeName := func(s string) {
		var f [8]byte
		copy(f[:], s)
		buf.Write(f[:])
	}
	writeName("BG001")
	writeName("bgm0001")
	writeName("bgm0002")
	writeName("SE01")
	writeName("first")
	writeName("second")
	writeName("12")
	writeName("")
	var flagInit [512]byte
	for i := range flagInit {
		flagInit[i] = 0xFF
	}
	binary.LittleEndian.PutUint16(flagInit[5*2:], 9)
	binary.LittleEndian.PutUint16(flagInit[7*2:], 7)
	buf.Write(flagInit[:])
	var _ = scratch
	table := buf.Bytes()
	total := 28 + 32 + 1 + 8*1 + 8*2 + 8*1 + 16*1 + 16*1 + 512
	if len(table) != total {
		t.Fatalf("table len=%d want %d", len(table), total)
	}
	return table
}

func TestTj2TableLoad(t *testing.T) {
	table, err := LoadTj2DataTable(buildTj2Table(t))
	if err != nil {
		t.Fatal(err)
	}
	if table.SceneCount() != 3 || table.FlagCount() != 4 || table.SourceCount() != 8 {
		t.Errorf("counts %+v", table)
	}
	if table.GraphicName(0) != "BG001" {
		t.Errorf("graphic=%q", table.GraphicName(0))
	}
	if table.TrackNumbers()[1] != "bgm0002" || table.SoundName(0) != "SE01" {
		t.Errorf("tracks/sounds %+v %+v", table.TrackNumbers(), table.SoundNames())
	}
	if table.FunctionSlots()[0].Number != 12 {
		t.Errorf("slot=%+v", table.FunctionSlots()[0])
	}
	if len(table.Macros()) != 1 || table.Macros()[0].From != "first" {
		t.Errorf("macros=%+v", table.Macros())
	}
	if table.Dialect().Variant != vmtj2.Tj2VariantModern {
		t.Errorf("dialect=%+v", table.Dialect())
	}
	if len(table.FlagAliases()) != 1 || table.FlagAliases()[0] != (Tj2FlagAlias{Source: 5, Target: 9}) {
		t.Errorf("aliases=%+v", table.FlagAliases())
	}
	if _, err := LoadTj2DataTable([]byte{1, 2, 3}); err == nil {
		t.Error("short table should fail")
	}
}

func TestTj2SaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewTj2SaveStore(dir)
	machine := vmtj2.Tj2MachineState{
		ProgramCounter: 7,
		ScenarioNumber: 2,
		Flags:          []byte{1, 2, 3},
		Source:         []byte{4, 5},
		NameSlots:      []string{"a", "b"},
		CondDepth:      1,
		CondStack:      []int{9},
		LoopSlots:      []int{8, 7},
		LoopHead:       3,
		Latch:          4,
		MessageOffset:  5,
		Halted:         true,
		Faulted:        false,
		Frames: []vmtj2.Tj2CallFrameState{
			{ScenarioNumber: 1, ProgramCounter: 2, CondDepth: 0, CondStack: []int{1}, LoopSlots: []int{2}, Latch: 3, MessageOffset: 4, Source: []byte{6}},
		},
	}
	state := &Tj2SaveState{
		ScenarioNumber: 2,
		Machine:        &machine,
		BackgroundName: "BG001",
		BackgroundHistory: []Tj2BackgroundLayer{
			{Name: "BG001", OffsetX: 1, OffsetY: 2},
		},
		Standees:        []Tj2StandeeEntry{{Layer: 1, Index: 3, Name: "ST01", OffsetX: 4, OffsetY: 5}},
		BgmTrack:        2,
		Speaker:         "sp",
		Text:            "hello",
		Summary:         "sp: hello",
		SavedAtTicks:    tj2TimeToTicks(time.Now().UTC()),
		Thumbnail:       []byte{1, 2, 3, 4},
		ThumbnailWidth:  1,
		ThumbnailHeight: 1,
	}
	if !store.Write(state, 3) {
		t.Fatal("write failed")
	}
	got := store.Read(3)
	if got == nil {
		t.Fatal("read failed")
	}
	if got.ScenarioNumber != 2 || got.BackgroundName != "BG001" || got.BgmTrack != 2 || got.Speaker != "sp" {
		t.Errorf("header mismatch %+v", got)
	}
	if got.Machine == nil || got.Machine.ProgramCounter != 7 || len(got.Machine.Frames) != 1 || got.Machine.Frames[0].Latch != 3 {
		t.Errorf("machine mismatch %+v", got.Machine)
	}
	if len(got.BackgroundHistory) != 1 || got.BackgroundHistory[0].OffsetY != 2 {
		t.Errorf("history %+v", got.BackgroundHistory)
	}
	if len(got.Standees) != 1 || got.Standees[0].Name != "ST01" {
		t.Errorf("standees %+v", got.Standees)
	}
	if store.Read(7) != nil {
		t.Error("missing slot should be nil")
	}
	infos := store.List(0)
	if len(infos) != tj2SaveSlotCount || !infos[3].Exists || infos[7].Exists {
		t.Errorf("list %+v", infos[3])
	}
}

func TestTj2Detect(t *testing.T) {
	brand := tj2Brand{}
	if brand.ID() != "bishop-tj2" || brand.Name() != "Bishop Tj2" {
		t.Errorf("brand %+v", brand)
	}
	dir := t.TempDir()
	if brand.Detect(dir) {
		t.Error("empty dir should not detect")
	}
	if err := os.WriteFile(filepath.Join(dir, "scene.pft"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !brand.Detect(dir) {
		t.Error("scene.pft should detect")
	}
	if brand.Detect("") {
		t.Error("empty path should not detect")
	}
	if eng := brand.New(); eng == nil {
		t.Error("New should not be nil")
	}
}

func TestTj2EngineSmoke(t *testing.T) {
	eng := &Engine{}
	info, err := eng.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 480 {
		t.Errorf("info=%+v", info)
	}
	if step, err := eng.Run(64); err != nil || step != core.StepFaulted {
		t.Errorf("run=%d err=%v", step, err)
	}
	if eng.Audio() != nil {
		t.Error("audio should drain empty")
	}
	slots := eng.Slots()
	if len(slots) != 24 || slots[0].Exists {
		t.Errorf("slots=%d", len(slots))
	}
	buf := make([]byte, 640*480*4)
	dirty, err := eng.Compose(nil, 0)
	if err != nil || !dirty {
		t.Errorf("compose=%v err=%v", dirty, err)
	}
	if dirty2, err := eng.Compose(unsafe.Pointer(&buf[0]), len(buf)); err != nil || dirty2 {
		t.Errorf("compose2=%v err=%v", dirty2, err)
	}
	if text := eng.Text(); text.Speaker != "" || text.Awaiting != 0 {
		t.Errorf("text=%+v", text)
	}
	if len(eng.Choices()) != 0 || eng.TextList() != nil || eng.Scenes() != nil {
		t.Error("lists should be empty")
	}
	if st := eng.State(); st.Animating || st.ReplayActive || st.AwaitingName {
		t.Errorf("state=%+v", st)
	}
	if _, ok := eng.PreviewSlot(0); ok {
		t.Error("no preview expected")
	}
	if len(eng.MapButtons()) != 0 {
		t.Error("no map buttons expected")
	}
	if has, _, _ := eng.MapMask(); has {
		t.Error("no map mask expected")
	}
	if err := eng.Save(0, "t"); err == nil {
		t.Error("save should fail without machine")
	}
	if _, err := eng.Load(0); err == nil {
		t.Error("load should fail without machine")
	}
	if eng.LastError() == "" {
		t.Error("last error expected")
	}
	if err := eng.Close(); err != nil {
		t.Error(err)
	}
}
