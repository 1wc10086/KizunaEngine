package bdt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/save"
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
	"kizuna/engine/core"
)

func encryptBdtScript(text string) []byte {
	raw := []byte(text)
	for i, b := range raw {
		raw[i] = ^b
	}
	return raw
}

func TestCleanBdtText(t *testing.T) {
	cases := map[string]string{
		"":           "",
		"  hello  ":  "hello",
		"\"quoted\"": "quoted",
		"@atmark":    "atmark",
		"a\r\nb\rc":  "a\nb\nc",
		"\"@both\"":  "both",
		"\"unclosed": "\"unclosed",
		"\"\"":       "",
	}
	for in, want := range cases {
		if got := CleanBdtText(in); got != want {
			t.Errorf("CleanBdtText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBdtAtoi(t *testing.T) {
	cases := map[string]int{"": 0, "42": 42, "  -7x": -7, "+12": 12, "abc": 0, "-": 0, "007": 7}
	for in, want := range cases {
		if got := BdtAtoi(in); got != want {
			t.Errorf("BdtAtoi(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSplitBdtChoices(t *testing.T) {
	opts := SplitBdtChoices("$L1,go left\r\n$L2,\"go right\"\nbare\n\n,")
	if len(opts) != 3 {
		t.Fatalf("got %d options", len(opts))
	}
	if opts[0].Label != "$L1" || opts[0].Text != "go left" {
		t.Errorf("opt0 = %+v", opts[0])
	}
	if opts[1].Label != "$L2" || opts[1].Text != "go right" {
		t.Errorf("opt1 = %+v", opts[1])
	}
	if opts[2].Label != "" || opts[2].Text != "bare" {
		t.Errorf("opt2 = %+v", opts[2])
	}
}

func TestWaitMilliseconds(t *testing.T) {
	if WaitMilliseconds(0) != 0 || WaitMilliseconds(-3) != 0 {
		t.Error("non-positive must be 0")
	}
	if WaitMilliseconds(2) != 2000 {
		t.Error("2s must be 2000ms")
	}
	if WaitMilliseconds(maxInt32) != maxInt32 {
		t.Error("overflow must clamp")
	}
}

func TestBdtTicksRoundTrip(t *testing.T) {
	for _, ticks := range []int64{0, 621355968000000000, 639000000000000000} {
		if back := bdtTimeToTicks(bdtTicksToTime(ticks)); back != ticks {
			t.Errorf("ticks %d round trip = %d", ticks, back)
		}
	}
}

func TestBdtVariantDetect(t *testing.T) {
	dir := t.TempDir()
	res := resource.Build(dir)
	if got := DetectBdtVariant(res); got != ContainerVariant {
		t.Errorf("empty dir variant = %s", got.Name)
	}
	if got := DetectBdtVariant(nil); got != ContainerVariant {
		t.Errorf("nil resources variant = %s", got.Name)
	}
	cg := filepath.Join(dir, "cg")
	if err := os.MkdirAll(cg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cg, "keikoku.bmp"), []byte("BM"), 0o644); err != nil {
		t.Fatal(err)
	}
	res2 := resource.Build(dir)
	if got := DetectBdtVariant(res2); got != ManinVariant {
		t.Errorf("sentinel variant = %s, want manin", got.Name)
	}
}

func TestBdtSaveStoreRoundTrip(t *testing.T) {
	store := NewBdtSaveStore(t.TempDir())
	if store.Read(0) != nil {
		t.Error("missing slot must be nil")
	}
	if list := store.List(); len(list) != save.SlotCount {
		t.Fatalf("list len = %d", len(list))
	} else if list[0].Exists {
		t.Error("slot 0 must not exist")
	}
	state := &BdtSaveState{
		Label:           EntryLabel,
		Machine:         &bdtvm.BdtMachineState{ProgramCounter: 7, X: []int32{1, 2}, Z: []int32{3}, Flags: []int32{4}, SystemFlags: []int32{5}, CallStack: []int{6, 7}, Stop: bdtvm.BdtStopText},
		X:               []int32{1, 2},
		Z:               []int32{3},
		BackgroundName:  "bg01",
		Sprites:         []BdtSpriteEntry{{Slot: 0, Name: "sp", X: 10, Y: 20, Mode: 1}},
		BgmTrack:        3,
		Speaker:         "spk",
		Text:            "txt",
		Summary:         "sum",
		SavedAtTicks:    639000000000000000,
		Thumbnail:       bytesRepeat(0xAB, ThumbnailWidth*ThumbnailHeight*4),
		ThumbnailWidth:  ThumbnailWidth,
		ThumbnailHeight: ThumbnailHeight,
	}
	if !store.Write(state, 2) {
		t.Fatal("write failed")
	}
	back := store.Read(2)
	if back == nil {
		t.Fatal("read failed")
	}
	if back.Label != state.Label || back.BackgroundName != "bg01" || back.BgmTrack != 3 || back.Speaker != "spk" || back.Text != "txt" || back.Summary != "sum" || back.SavedAtTicks != state.SavedAtTicks {
		t.Errorf("scalar mismatch: %+v", back)
	}
	if back.Machine == nil || back.Machine.ProgramCounter != 7 || back.Machine.Stop != bdtvm.BdtStopText || len(back.Machine.CallStack) != 2 || back.Machine.CallStack[1] != 7 {
		t.Errorf("machine mismatch: %+v", back.Machine)
	}
	if len(back.Sprites) != 1 || back.Sprites[0] != state.Sprites[0] {
		t.Errorf("sprites mismatch: %+v", back.Sprites)
	}
	if len(back.Thumbnail) != len(state.Thumbnail) || back.ThumbnailWidth != ThumbnailWidth {
		t.Error("thumbnail mismatch")
	}
	info := store.Inspect(2)
	if !info.Exists || info.Summary != "sum" || info.SavedAt != state.SavedAtTicks {
		t.Errorf("inspect = %+v", info)
	}
	if err := os.WriteFile(store.pathFor(5), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if store.Read(5) != nil {
		t.Error("corrupt slot must be nil")
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func buildBdtMaskBMP(width, height int, pixels []byte) []byte {
	stride := (width + 3) &^ 3
	data := make([]byte, 54+stride*height)
	data[0] = 'B'
	data[1] = 'M'
	data[10] = 54
	data[14] = 40
	data[18] = byte(width)
	data[22] = byte(height)
	data[26] = 1
	data[28] = 8
	for row := 0; row < height; row++ {
		copy(data[54+row*stride:], pixels[row*width:(row+1)*width])
	}
	return data
}

func TestBdtMapMask8Bit(t *testing.T) {
	m := NewBdtMapScreen()
	pixels := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0}
	if err := m.parseMask(buildBdtMaskBMP(4, 4, pixels)); err != nil {
		t.Fatalf("parseMask: %v", err)
	}
	if !m.HasMask() || m.MaskWidth() != 4 || m.MaskHeight() != 4 {
		t.Fatal("mask dims wrong")
	}
	if got := m.HotspotAt(0, 0); got != 5 {
		t.Errorf("HotspotAt(0,0) = %d", got)
	}
	if got := m.HotspotAt(1, 0); got != 0 {
		t.Errorf("HotspotAt(1,0) = %d", got)
	}
	if got := m.HotspotAt(-1, 0); got != 0 {
		t.Errorf("oob must be 0")
	}
	hotspots := m.Hotspots()
	if len(hotspots) != BdtMapHotspotCount {
		t.Fatalf("hotspots = %d", len(hotspots))
	}
	for _, h := range hotspots {
		if h.Width != BdtMapButtonSize || h.Height != BdtMapButtonSize || h.X < 0 || h.Y < 0 || h.X+h.Width > BdtMapScreenWidth || h.Y+h.Height > BdtMapScreenHeight {
			t.Errorf("hotspot out of bounds: %+v", h)
		}
	}
	if err := m.parseMask([]byte("nope")); err == nil {
		t.Error("bad mask must fail")
	}
}

func TestBdtMapEmptyTable(t *testing.T) {
	res := resource.Build(t.TempDir())
	m := NewBdtMapScreen()
	if m.Activate(res) {
		t.Error("empty resources must not activate")
	}
	if m.HasTable() {
		t.Error("no table expected")
	}
}

func TestBdtArtDecoderPassthrough(t *testing.T) {
	img := gfx.NewBsgImage(8, 8, 0, 0, make([]byte, 8*8*4))
	fig, masked := DecodeBdtArt(img)
	if masked || fig != img {
		t.Error("small image must pass through unmasked")
	}
	fig2, masked2 := DecodeBdtArtWithVariant(img, ManinVariant)
	if masked2 || fig2 != img {
		t.Error("manin small image must pass through")
	}
	if _, masked := DecodeBdtArt(nil); masked {
		t.Error("nil must not be masked")
	}
}

func newBdtTestImage(w, h int) *gfx.BsgImage {
	return gfx.NewBsgImage(w, h, 0, 0, make([]byte, w*h*4))
}

func TestSessionScriptFlow(t *testing.T) {
	dir := t.TempDir()
	script := "$999-98\n\"hello\"\nSA$L1,go left\nSA$L2,go right\nSU\n$L1\n\"went left\"\n%\n$L2\n\"went right\"\n%\n"
	if err := os.WriteFile(filepath.Join(dir, "scene00.bdt"), encryptBdtScript(script), 0o644); err != nil {
		t.Fatal(err)
	}
	wave := append([]byte("RIFF----WAVE"), make([]byte, 32)...)
	if err := os.WriteFile(filepath.Join(dir, "bgm01.wav"), wave, 0o644); err != nil {
		t.Fatal(err)
	}
	eng := NewSession()
	info, err := eng.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 640 || info.Height != 480 || info.SceneCount != 1 {
		t.Fatalf("info = %+v", info)
	}
	sess := eng.(*Session)
	if sess.variant != ContainerVariant {
		t.Fatalf("variant = %s", sess.variant.Name)
	}
	step, err := eng.Run(0)
	if err != nil || step != core.StepAwaitInput {
		t.Fatalf("run = %v, %v", step, err)
	}
	if txt := eng.Text(); txt.Speaker != "" || txt.Text != "hello" || txt.Awaiting != 1 {
		t.Fatalf("text = %+v", txt)
	}
	st := eng.State()
	if !st.MessageWindowVisible || st.WaitMs != 0 || st.ReplayActive || st.InstructionStart == 0 {
		t.Fatalf("state = %+v", st)
	}
	step, _ = eng.Advance()
	if step != core.StepAwaitChoice {
		t.Fatalf("advance = %v", step)
	}
	choices := eng.Choices()
	if len(choices) != 2 || choices[0].Name != "go left" || choices[1].Value != 1 {
		t.Fatalf("choices = %+v", choices)
	}
	step, _ = eng.Choose(0)
	if step != core.StepAwaitInput {
		t.Fatalf("choose = %v", step)
	}
	if txt := eng.Text(); txt.Text != "went left" {
		t.Fatalf("text = %+v", txt)
	}
	if err := eng.Save(3, ""); err != nil {
		t.Fatalf("save: %v", err)
	}
	slots := eng.Slots()
	if len(slots) != save.SlotCount || !slots[3].Exists || slots[3].SavedAt == "" {
		t.Fatalf("slots[3] = %+v", slots[3])
	}
	preview, ok := sess.PreviewSlot(3)
	if !ok || preview.Text != "went left" || preview.SavedAt == "" {
		t.Fatalf("preview = %+v %v", preview, ok)
	}
	if preview.Thumb.Width != ThumbnailWidth || len(preview.Thumb.Pixels) != ThumbnailWidth*ThumbnailHeight*4 {
		t.Fatalf("thumb = %dx%d %d", preview.Thumb.Width, preview.Thumb.Height, len(preview.Thumb.Pixels))
	}
	if _, ok := sess.PreviewSlot(7); ok {
		t.Fatal("missing preview must fail")
	}
	step, _ = eng.Advance()
	if step != core.StepFinished {
		t.Fatalf("advance = %v", step)
	}
	step, err = eng.Load(3)
	if err != nil || step != core.StepAwaitInput {
		t.Fatalf("load = %v, %v", step, err)
	}
	if txt := eng.Text(); txt.Text != "went left" {
		t.Fatalf("after load text = %+v", txt)
	}
	if _, err := eng.Load(9); err == nil {
		t.Fatal("missing load must error")
	}
	dst := make([]byte, 640*480*4)
	dirty, err := eng.Compose(unsafe.Pointer(&dst[0]), len(dst))
	if err != nil || !dirty {
		t.Fatalf("compose = %v, %v", dirty, err)
	}
	if dst[3] != 255 {
		t.Errorf("compose alpha = %d", dst[3])
	}
	if audio := eng.Audio(); len(audio) != 0 {
		t.Fatalf("audio = %+v", audio)
	}
	sess.SetBgm(1, 0)
	events := eng.Audio()
	if len(events) != 1 || events[0].Kind != 0 || !strings.HasSuffix(events[0].Path, "bgm01.wav") || !events[0].Loop || events[0].Volume != 0.8 {
		t.Fatalf("bgm events = %+v", events)
	}
	if audio := eng.Audio(); len(audio) != 0 {
		t.Fatalf("drain failed: %+v", audio)
	}
	sess.StopBgm()
	events = eng.Audio()
	if len(events) != 1 || events[0].Kind != 3 {
		t.Fatalf("stop events = %+v", events)
	}
	if buttons := sess.MapButtons(); len(buttons) != 0 {
		t.Fatalf("map buttons = %d", len(buttons))
	}
	if has, _, _ := sess.MapMask(); has {
		t.Fatal("no mask expected")
	}
	step, err = eng.Start(0)
	if err != nil || step != core.StepContinue {
		t.Fatalf("start = %v, %v", step, err)
	}
	step, _ = eng.Run(0)
	if step != core.StepAwaitInput || eng.Text().Text != "hello" {
		t.Fatalf("restart run = %v %+v", step, eng.Text())
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestSessionFaultWithoutScript(t *testing.T) {
	eng := NewSession()
	if _, err := eng.Open(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	step, _ := eng.Run(10)
	if step != core.StepFaulted {
		t.Fatalf("run = %v", step)
	}
	if eng.LastError() == "" {
		t.Fatal("LastError must be set")
	}
	if _, err := eng.Start(0); err == nil {
		t.Fatal("start without script must error")
	}
}

func TestBdtBrandDetect(t *testing.T) {
	b := brand{}
	if b.ID() != "bishop-bdt" || b.Name() != "Bishop Bdt" {
		t.Fatalf("brand = %s %s", b.ID(), b.Name())
	}
	dir := t.TempDir()
	if b.Detect(dir) {
		t.Fatal("empty dir must not detect")
	}
	if b.Detect("") {
		t.Fatal("empty path must not detect")
	}
	if err := os.WriteFile(filepath.Join(dir, "SCENE00.BDT"), []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if !b.Detect(dir) {
		t.Fatal("bdt file must detect")
	}
	if _, ok := core.Find("bishop-bdt"); !ok {
		t.Fatal("brand not registered")
	}
}
