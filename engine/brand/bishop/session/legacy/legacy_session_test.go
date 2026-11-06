package legacy

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm/scf"
)

func TestCleanText(t *testing.T) {
	if got := CleanText("a\r\nb\rc"); got != "a\nb\nc" {
		t.Fatalf("CleanText = %q", got)
	}
	if got := CleanText(""); got != "" {
		t.Fatalf("CleanText empty = %q", got)
	}
}

func TestAtoiText(t *testing.T) {
	cases := map[string]int{"": 0, "   12": 12, "+7": 7, "-9": -9, "  -42x": -42, "abc": 0, "3.5": 3}
	for input, want := range cases {
		if got := AtoiText(input); got != want {
			t.Fatalf("AtoiText(%q) = %d, want %d", input, got, want)
		}
	}
	if got := WaitMillisecondsText(""); got != 1000 {
		t.Fatalf("WaitMillisecondsText empty = %d", got)
	}
	if got := WaitMillisecondsText("50"); got != 1000 {
		t.Fatalf("WaitMillisecondsText 50 = %d", got)
	}
	if got := CountdownSecondsText(""); got != 50 {
		t.Fatalf("CountdownSecondsText empty = %d", got)
	}
	if got := WithWaveExtension("se01"); got != "se01.WAV" {
		t.Fatalf("WithWaveExtension = %q", got)
	}
	if got := WithWaveExtension("se01.wav"); got != "se01.wav" {
		t.Fatalf("WithWaveExtension wav = %q", got)
	}
}

func TestLoadLegacyDataTable(t *testing.T) {
	data := make([]byte, 0, 128)
	header := make([]byte, 28)
	putU16(header, 0, 8)
	putU16(header, 2, 16)
	putU16(header, 4, 1)
	putU16(header, 6, 1)
	putU16(header, 8, 1)
	putU16(header, 10, 1)
	putU32(header, 12, 1)
	putU16(header, 16, 32)
	putU32(header, 20, 512)
	data = append(data, header...)
	data = append(data, 0, 0)
	data = append(data, []byte("BG01\x00\x00\x00\x00")...)
	data = append(data, []byte("02\x00\x00\x00\x00\x00\x00")...)
	data = append(data, []byte("SE01\x00\x00\x00\x00")...)
	slotA := append([]byte("SLOT-A\x00\x00"), make([]byte, 9)...)
	data = append(data, slotA...)
	function := append([]byte("01  NAME"), make([]byte, 8)...)
	data = append(data, function...)
	table, err := LoadLegacyDataTable(data)
	if err != nil {
		t.Fatalf("LoadLegacyDataTable failed: %v", err)
	}
	if table.FlagCount() != 16 || table.SourceCount() != 32 || table.ScenarioBufferSize() != 512 {
		t.Fatalf("counts = %d/%d/%d", table.FlagCount(), table.SourceCount(), table.ScenarioBufferSize())
	}
	if len(table.GraphicNames()) != 1 || table.GraphicNames()[0] != "BG01" {
		t.Fatalf("graphics = %q", table.GraphicNames())
	}
	if len(table.TrackNumbers()) != 1 || table.TrackNumbers()[0] != "02" {
		t.Fatalf("tracks = %q", table.TrackNumbers())
	}
	if len(table.SoundNames()) != 1 || table.SoundNames()[0] != "SE01" {
		t.Fatalf("sounds = %q", table.SoundNames())
	}
	if len(table.FunctionSlots()) != 1 {
		t.Fatalf("function slots = %d", len(table.FunctionSlots()))
	}
	if len(table.SlotNames()) != 2 {
		t.Fatalf("slot names = %d", len(table.SlotNames()))
	}
	if _, err := LoadLegacyDataTable([]byte{1, 2}); err == nil {
		t.Fatalf("short data should fail")
	}
	var _ scf.ScfDataTable = table
}

func putU16(data []byte, offset int, value int) {
	data[offset] = byte(value)
	data[offset+1] = byte(value >> 8)
}

func putU32(data []byte, offset int, value int) {
	data[offset] = byte(value)
	data[offset+1] = byte(value >> 8)
	data[offset+2] = byte(value >> 16)
	data[offset+3] = byte(value >> 24)
}

func TestLegacySaveStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	saves := filepath.Join(dir, "save")
	store := NewLegacySaveStore(saves)
	machine := scf.ScfMachineState{
		ProgramCounter: 12,
		ScenarioNumber: 3,
		Flags:          []byte{1, 2, 3},
		Source:         []byte{4, 5},
		NameSlots:      []string{"a", "b"},
		CallDepth:      1,
		Frames: []scf.ScfMachineState{{
			ProgramCounter: 7,
			Flags:          []byte{9},
			CondStack:      []int{1},
			LoopSlots:      []int{2},
			MessageOffset:  -1,
		}},
		Halted:        false,
		Faulted:       false,
		CondDepth:     1,
		CondStack:     []int{0, 1, 2, 3, 4, 5, 6, 7},
		LoopSlots:     []int{8, 7, 6, 5, 4, 3, 2, 1},
		Latch:         5,
		MessageOffset: -1,
	}
	saved := &LegacySaveState{
		Version:         1,
		ScenarioNumber:  3,
		Machine:         &machine,
		BackgroundName:  "BG01",
		MenuVisible:     true,
		Standees:        []LegacyStandeeEntry{{Index: 1, Name: "ST01"}, {Index: 2, Name: "ST02"}},
		FaceName:        "FACE",
		BgmTrack:        4,
		Speaker:         "s",
		Text:            "hello",
		Summary:         "s: hello",
		SavedAtTicks:    TimeToTicks(TicksToTime(638000000000000000)),
		Thumbnail:       []byte{1, 2, 3, 4},
		ThumbnailWidth:  2,
		ThumbnailHeight: 1,
	}
	if !store.Write(saved, 3) {
		t.Fatalf("Write failed")
	}
	loaded := store.Read(3)
	if loaded == nil {
		t.Fatalf("Read returned nil")
	}
	if loaded.ScenarioNumber != 3 || loaded.BackgroundName != "BG01" || !loaded.MenuVisible {
		t.Fatalf("header mismatch: %+v", loaded)
	}
	if loaded.Machine == nil || loaded.Machine.ProgramCounter != 12 || len(loaded.Machine.Frames) != 1 {
		t.Fatalf("machine mismatch: %+v", loaded.Machine)
	}
	if len(loaded.Standees) != 2 || loaded.Standees[0].Name != "ST01" {
		t.Fatalf("standees mismatch: %+v", loaded.Standees)
	}
	if loaded.BgmTrack != 4 || loaded.Speaker != "s" || loaded.Text != "hello" {
		t.Fatalf("body mismatch: %+v", loaded)
	}
	if string(loaded.Thumbnail) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("thumbnail mismatch")
	}
	if store.Read(7) != nil {
		t.Fatalf("missing slot should return nil")
	}
	infos := store.List()
	if len(infos) != 24 {
		t.Fatalf("slots = %d", len(infos))
	}
	if !infos[3].Exists || infos[7].Exists {
		t.Fatalf("exists flags wrong")
	}
	if !infos[3].HasSavedAt || infos[3].Summary != "s: hello" {
		t.Fatalf("inspect wrong: %+v", infos[3])
	}
	if got := infos[3].SavedAt.Format("2006-01-02"); got == "" {
		t.Fatalf("savedAt missing")
	}
}

func TestLegacyStageCompose(t *testing.T) {
	stage := NewLegacyStage()
	frame := gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	if !stage.ComposeTo(frame, false) {
		t.Fatalf("first compose should be dirty")
	}
	if stage.ComposeTo(frame, false) {
		t.Fatalf("second compose should be clean")
	}
	if !stage.ComposeTo(frame, true) {
		t.Fatalf("flag change should be dirty")
	}
	pixels := make([]byte, 4*4*4)
	for index := range pixels {
		pixels[index] = 200
	}
	solid := gfx.NewBsgImage(4, 4, 0, 0, pixels)
	if !IsBlankCard(solid) {
		t.Fatalf("solid should be blank card")
	}
	pixels[0] = 0
	if IsBlankCard(solid) {
		t.Fatalf("varied should not be blank card")
	}
	opaque := make([]byte, 2*2*4)
	for index := 3; index < len(opaque); index += 4 {
		opaque[index] = 255
	}
	opaque[0], opaque[1], opaque[2] = 10, 20, 30
	keyed := WithColorkey(gfx.NewBsgImage(2, 2, 0, 0, opaque))
	if keyed.Pixels[3] != 0 {
		t.Fatalf("colorkey alpha = %d", keyed.Pixels[3])
	}
	stage.SetBackground("BG", solid)
	if stage.BackgroundName() != "BG" || stage.DarkBackground() == nil {
		t.Fatalf("background not set")
	}
}

func TestLegacyDetect(t *testing.T) {
	brand := legacyBrand{}
	if brand.ID() != "bishop-legacy" || brand.Name() != "Bishop Legacy" {
		t.Fatalf("brand identity wrong")
	}
	empty := t.TempDir()
	if brand.Detect(empty) {
		t.Fatalf("empty dir should not detect")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SCNFILE.PFT"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !brand.Detect(root) {
		t.Fatalf("anchor dir should detect")
	}
	nested := t.TempDir()
	dataDir := filepath.Join(nested, "DATA")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "scnfile.pft"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !brand.Detect(nested) {
		t.Fatalf("data subdir should detect")
	}
	if got := findLegacyDataDir(nested); got != dataDir {
		t.Fatalf("data dir = %q", got)
	}
}

func TestStandeeRegistry(t *testing.T) {
	registry := NewLegacyStandeeRegistry()
	for index := 0; index < LegacyStandeeCapacity; index++ {
		if !registry.Register(index, "S") {
			t.Fatalf("register %d failed", index)
		}
	}
	if registry.Register(9, "S") {
		t.Fatalf("over capacity should fail")
	}
	if registry.Primary() == nil || registry.Secondary() == nil {
		t.Fatalf("primary/secondary missing")
	}
	if !registry.Unregister(0) || registry.Unregister(0) {
		t.Fatalf("unregister wrong")
	}
	registry.Clear()
	if registry.Count() != 0 || registry.Primary() != nil {
		t.Fatalf("clear wrong")
	}
}
