package save

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

func testMachine() vm.MachineState {
	return vm.MachineState{
		ProgramCounter:      123456,
		FrameBase:           7,
		CallDepth:           3,
		Frame:               []int32{1, -2, 2147483647, -2147483648},
		Registers:           []int32{10, 20},
		Variables:           []int32{},
		Properties:          []int32{5},
		Scalars:             []float64{0.5, -3.25, 1e100},
		CallReturnAddresses: []int32{99},
		CallFrameBases:      []int32{0, 4},
		Deferred:            true,
		DeferredIsCall:      true,
		DeferredTarget:      4242,
		Halted:              true,
	}
}

func testSaveData() *SaveData {
	bgm := "bgm/track01"
	return &SaveData{
		Version:  CurrentVersion,
		GameName: "テストGame",
		SavedAt:  time.Now(),
		Machine:  testMachine(),
		Layers: []SaveLayer{
			{Slot: 1, Image: "bg01", Alpha: 1.0, Character: false, Artworks: []SaveArtwork{
				{Image: "bg01", X: 0, Y: 0, Frames: 1, AnimatedPart: -1},
			}},
			{Slot: 5, Image: "", Alpha: 0.5, Character: true, Artworks: []SaveArtwork{
				{Image: "chara/a", X: 100, Y: -20, Frames: 4, AnimatedPart: 0},
				{Image: "chara/b", X: 300, Y: 10, Frames: 1, AnimatedPart: -1},
			}},
		},
		Bgm:             &bgm,
		Speaker:         "主人公",
		Text:            "こんにちは、世界！\nSecond line.",
		Thumbnail:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
		ThumbnailWidth:  2,
		ThumbnailHeight: 1,
		Summary:         "Scene 3 — 選択肢",
	}
}

func equalSaveData(a, b *SaveData) bool {
	if a.Version != b.Version || a.GameName != b.GameName || a.Summary != b.Summary ||
		a.Speaker != b.Speaker || a.Text != b.Text ||
		a.ThumbnailWidth != b.ThumbnailWidth || a.ThumbnailHeight != b.ThumbnailHeight {
		return false
	}
	if (a.Bgm == nil) != (b.Bgm == nil) {
		return false
	}
	if a.Bgm != nil && *a.Bgm != *b.Bgm {
		return false
	}
	if !reflect.DeepEqual(a.Machine, b.Machine) {
		return false
	}
	if !reflect.DeepEqual(a.Layers, b.Layers) {
		return false
	}
	return reflect.DeepEqual(a.Thumbnail, b.Thumbnail)
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewSaveStore(dir)
	want := testSaveData()
	if !store.Write(want, 3) {
		t.Fatal("write failed")
	}
	got := store.Read(3)
	if got == nil {
		t.Fatal("read returned nil")
	}
	if !equalSaveData(want, got) {
		t.Fatalf("round trip mismatch:\nwant=%+v\ngot=%+v", want, got)
	}
	if got.SavedAt.IsZero() {
		t.Fatal("savedAt missing")
	}
	info := store.Inspect(3)
	if !info.Exists || info.Summary != want.GameName || !info.HasSavedAt {
		t.Fatalf("inspect mismatch: %+v", info)
	}
	empty := store.Inspect(7)
	if empty.Exists || empty.HasSavedAt || empty.Summary != "" {
		t.Fatalf("empty slot should not exist: %+v", empty)
	}
	if store.Read(7) != nil {
		t.Fatal("missing slot should read nil")
	}
	slots := store.List()
	if len(slots) != SlotCount {
		t.Fatalf("slot count = %d, want %d", len(slots), SlotCount)
	}
}

func TestSaveNilBgmRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewSaveStore(dir)
	want := testSaveData()
	want.Bgm = nil
	want.Thumbnail = []byte{}
	if !store.Write(want, 0) {
		t.Fatal("write failed")
	}
	got := store.Read(0)
	if got == nil {
		t.Fatal("read returned nil")
	}
	if got.Bgm != nil {
		t.Fatalf("bgm should be nil, got %q", *got.Bgm)
	}
	if !equalSaveData(want, got) {
		t.Fatal("round trip mismatch")
	}
}

func TestSaveLegacyMagicPrefix(t *testing.T) {
	dir := t.TempDir()
	store := NewSaveStore(dir)
	want := testSaveData()
	if !store.Write(want, 1) {
		t.Fatal("write failed")
	}
	path := filepath.Join(dir, "slot001.dat")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 12 || string(raw[:12]) != saveMagic {
		t.Fatalf("magic is not raw 12 bytes: %q", raw[:12])
	}
	legacy := append([]byte{byte(len(saveMagicBytes))}, raw...)
	if err := os.WriteFile(path, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	got := store.Read(1)
	if got == nil {
		t.Fatal("legacy magic prefix should be accepted")
	}
	if !equalSaveData(want, got) {
		t.Fatal("legacy round trip mismatch")
	}
}

func TestSaveCorrupt(t *testing.T) {
	dir := t.TempDir()
	store := NewSaveStore(dir)
	if err := os.WriteFile(filepath.Join(dir, "slot002.dat"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if info := store.Inspect(2); info.Exists {
		t.Fatal("corrupt slot should not exist")
	}
	if store.Read(2) != nil {
		t.Fatal("corrupt slot should read nil")
	}
}

func TestSaveCaptureApply(t *testing.T) {
	src := state.NewGameState()
	src.SetLayerAt(1, "bg01", state.StageLayerScreen, []state.LayerArtwork{
		{Image: "bg01", X: 0, Y: 0, Frames: 1, AnimatedPart: -1},
	})
	src.SetLayerAt(5, "", state.StageLayerCharacter, []state.LayerArtwork{
		{Image: "chara/a", X: 10, Y: 20, Frames: 4, AnimatedPart: 0},
	})
	src.SetAlpha(5, 0.5)
	src.SetBgm("bgm/track02")
	src.SetSpeaker("S")
	src.SetText("T")
	data := Capture(src, testMachine(), "G", "sum", []byte{9}, 1, 1)
	if len(data.Layers) != 2 {
		t.Fatalf("captured %d layers", len(data.Layers))
	}
	dst := state.NewGameState()
	data.ApplyTo(dst)
	if name, ok := dst.Bgm(); !ok || name != "bgm/track02" {
		t.Fatalf("bgm = %q, %v", name, ok)
	}
	if dst.Speaker() != "S" || dst.Text() != "T" {
		t.Fatal("speaker/text not restored")
	}
	if !dst.AwaitingInput() || !dst.MessageWindowVisible() {
		t.Fatal("awaiting flags not set")
	}
	layers := dst.Layers()
	if len(layers[1].Artworks()) != 1 || len(layers[5].Artworks()) != 1 {
		t.Fatal("layers not restored")
	}
	if layers[5].Alpha() != 0.5 || layers[5].Kind() != state.StageLayerCharacter {
		t.Fatal("layer alpha/kind not restored")
	}
}
