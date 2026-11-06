package gothic

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"kizuna/engine/brand/bishop/gfx"
	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

func TestGlyphIndexKnown(t *testing.T) {
	if got := GothicGlyphIndexOf('あ'); got != 283 {
		t.Fatalf("あ index = %d, want 283", got)
	}
	if got := GothicGlyphIndexOf('A'); got != -1 {
		t.Fatalf("A index = %d, want -1", got)
	}
	if got := GothicGlyphIndexOf('ｱ'); got != -1 {
		t.Fatalf("halfwidth index = %d, want -1", got)
	}
}

func TestAudioProfileTracks(t *testing.T) {
	catalog := NewGothicAudioCatalog(t.TempDir(), nil, t.TempDir())
	if got := catalog.TrackOf("平穏な一日"); got != 2 {
		t.Fatalf("track = %d, want 2", got)
	}
	if got := catalog.TrackOf("gm01"); got != 2 {
		t.Fatalf("toku1 track = %d, want 2", got)
	}
	if got := catalog.TrackOf("gm13"); got != 14 {
		t.Fatalf("toku1 track = %d, want 14", got)
	}
	if got := catalog.TrackOf("gm14"); got != 0 {
		t.Fatalf("toku1 track = %d, want 0", got)
	}
	start, end, ok := catalog.TrySegment(2)
	if !ok || start != 2.0 || end != 133.466667 {
		t.Fatalf("segment = %v %v %v", start, end, ok)
	}
	if _, _, ok := catalog.TrySegment(21); ok {
		t.Fatalf("segment 21 should fail")
	}
	if got := normalizeGothicAudioName(`"平穏な一日,xxx.wav"`); got != "平穏な一日" {
		t.Fatalf("normalize = %q", got)
	}
}

func TestMapStateZones(t *testing.T) {
	m := NewGothicMapState()
	m.AddZone(1, 2, 3, 4, "labelA")
	m.AddTarget(2, "fallback")
	if got := m.LookupKind(1, 3, 2); got != 4 {
		t.Fatalf("kind = %d", got)
	}
	if got := m.LookupLabel(1, 3, 2); got != "labelA" {
		t.Fatalf("label = %q", got)
	}
	if got := m.FallbackLabel(2); got != "fallback" {
		t.Fatalf("fallback = %q", got)
	}
	mask := gfx.NewBsgImage(2, 1, 0, 0, []byte{0, 0, 255, 255, 0, 0, 0, 255})
	table := BuildGothicMapColorTable(mask)
	if len(table) != 1 || table[0] != 255 {
		t.Fatalf("table = %v", table)
	}
	m.EnterRouteMap(1, 1, "row", mask, table)
	if got := m.RouteRowAt(0, 0); got != 1 {
		t.Fatalf("row = %d", got)
	}
	if got := m.RouteRowAt(1, 0); got != 0 {
		t.Fatalf("row = %d", got)
	}
	if got := SampleGothicMapColor(mask, 9, 9); got != -1 {
		t.Fatalf("oob = %d", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	store := NewGothicSaveStore(t.TempDir())
	thumb := make([]byte, GothicThumbnailWidth*GothicThumbnailHeight*4)
	for i := range thumb {
		thumb[i] = byte(i)
	}
	state := &GothicSaveState{
		StatementIndex:  1234,
		Variables:       []vmgothic.GothicVariableEntry{{Name: "v1", Value: 7}, {Name: "日本語", Value: -3}},
		CallStack:       []int32{1, 2, 3},
		Background:      "bg01",
		Speaker:         "話者",
		Text:            "line1\nline2",
		Summary:         "summary",
		Bgm:             "track02.wav",
		PlayerName:      "テスト",
		Thumbnail:       thumb,
		ThumbnailWidth:  GothicThumbnailWidth,
		ThumbnailHeight: GothicThumbnailHeight,
	}
	if !store.Save(3, state) {
		t.Fatalf("save failed")
	}
	loaded := store.Load(3)
	if loaded == nil {
		t.Fatalf("load failed")
	}
	if loaded.StatementIndex != 1234 || loaded.Background != "bg01" || loaded.Speaker != "話者" ||
		loaded.Text != "line1\nline2" || loaded.Summary != "summary" || loaded.Bgm != "track02.wav" ||
		loaded.PlayerName != "テスト" || loaded.ThumbnailWidth != GothicThumbnailWidth {
		t.Fatalf("mismatch: %+v", loaded)
	}
	if len(loaded.Variables) != 2 || loaded.Variables[0].Name != "v1" || loaded.Variables[0].Value != 7 {
		t.Fatalf("variables: %+v", loaded.Variables)
	}
	if len(loaded.Thumbnail) != len(thumb) || loaded.Thumbnail[7] != thumb[7] {
		t.Fatalf("thumbnail mismatch")
	}
	info := store.Inspect(3)
	if !info.Exists || info.Summary != "summary" || !info.HasSavedAt {
		t.Fatalf("inspect: %+v", info)
	}
	if preview := store.PreviewSlot(3); preview == nil || preview.Text != "line1\nline2" {
		t.Fatalf("preview failed")
	}
	if store.Load(4) != nil {
		t.Fatalf("missing slot should be nil")
	}
	path := filepath.Join(store.DirectoryPath(), "slot003.gts")
	raw, _ := os.ReadFile(path)
	raw[0] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if store.Load(3) != nil {
		t.Fatalf("corrupt magic should fail")
	}
}

func TestStageComposeSmoke(t *testing.T) {
	font := NewGothicFontFile(filepath.Join(t.TempDir(), "missing.dat"))
	if font.GlyphWidth != 0 {
		t.Fatalf("missing font should have zero width")
	}
	stage := NewGothicStage(nil, font)
	stage.LoadBase(4, "nothing")
	stage.SetPlane(4, 1, "", 10, 20)
	stage.SetPlaneRegion(4, 2, "", 0, 0, 10, 10, 5, 5)
	stage.SetMessageLines([]string{"hello"})
	stage.SetMenuItems([]string{"a", "b"})
	if !stage.Compose() {
		t.Fatalf("first compose should be dirty")
	}
	if stage.Compose() {
		t.Fatalf("second compose should be clean")
	}
	stage.BeginEffect(3, 1, 4, 4)
	if !stage.EffectActive {
		t.Fatalf("effect should be active")
	}
	stage.AdvanceEffect(10)
	if stage.EffectActive {
		t.Fatalf("effect should finish")
	}
	if !stage.Compose() {
		t.Fatalf("effect compose should be dirty")
	}
}

func TestSessionOffline(t *testing.T) {
	dir := t.TempDir()
	s := NewSession()
	engine := core.Engine(s)
	info, err := engine.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 480 {
		t.Fatalf("info = %+v", info)
	}
	step, err := engine.Run(64)
	if err != nil {
		t.Fatal(err)
	}
	if step != core.StepFaulted {
		t.Fatalf("no script should fault, got %d", step)
	}
	if !s.SubmitName("  太郎  ") || s.playerName != "太郎" {
		t.Fatalf("submit failed")
	}
	if s.SubmitName("   ") {
		t.Fatalf("empty submit should fail")
	}
	if engine.Text().Awaiting != 0 {
		t.Fatalf("awaiting should be 0")
	}
	if len(engine.Audio()) != 0 {
		t.Fatalf("audio should drain empty")
	}
	st := engine.State()
	if st.MessageWindowVisible != true || st.ReplayActive || st.AwaitingName || st.WaitMs != 0 {
		t.Fatalf("state = %+v", st)
	}
	slots := engine.Slots()
	if len(slots) != GothicSaveSlotCount || slots[0].Exists {
		t.Fatalf("slots = %d", len(slots))
	}
	if _, ok := s.PreviewSlot(0); ok {
		t.Fatalf("empty preview should fail")
	}
	if _, ok := s.ComposeImages([]string{"nope"}); ok {
		t.Fatalf("gallery should fail")
	}
	if has, w, h := s.MapMask(); has || w != 0 || h != 0 {
		t.Fatalf("mask should be empty")
	}
	buf := make([]byte, 640*480*4)
	dirty, err := engine.Compose(unsafe.Pointer(&buf[0]), len(buf))
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatalf("first compose should be dirty")
	}
	dirty, _ = engine.Compose(unsafe.Pointer(&buf[0]), len(buf))
	if dirty {
		t.Fatalf("second compose should be clean")
	}
	if err := engine.Save(0, "t"); err == nil {
		t.Fatalf("save without machine should fail")
	}
	var _ core.NamePrompter = s
	var _ core.MapExposer = s
	var _ core.GalleryComposer = s
	var _ core.SlotPreviewer = s
	var _ core.OpenerExtras = s
	var _ core.Closer = s
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDetectAnchor(t *testing.T) {
	dir := t.TempDir()
	if DetectGothicDataDir(dir) != "" {
		t.Fatalf("empty dir should not detect")
	}
	sub := filepath.Join(dir, "game", "data")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "MAINCON.SCS"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	brand := gothicBrand{}
	if !brand.Detect(dir) {
		t.Fatalf("should detect MAINCON.SCS")
	}
	if brand.ID() != "bishop-gothic" || brand.Name() != "Bishop Gothic" {
		t.Fatalf("brand = %s %s", brand.ID(), brand.Name())
	}
	if got := DetectGothicDataDir(dir); got != sub {
		t.Fatalf("datadir = %q", got)
	}
}
