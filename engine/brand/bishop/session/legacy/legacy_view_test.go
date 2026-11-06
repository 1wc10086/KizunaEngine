package legacy

import (
	"testing"

	"kizuna/engine/brand/bishop/gfx"
)

func viewTestSolid(width, height int, blue, green, red byte) *gfx.BsgImage {
	pixels := make([]byte, width*height*4)
	for offset := 0; offset+4 <= len(pixels); offset += 4 {
		pixels[offset] = blue
		pixels[offset+1] = green
		pixels[offset+2] = red
		pixels[offset+3] = 255
	}
	return gfx.NewBsgImage(width, height, 0, 0, pixels)
}

func TestLegacyPanelHitTestCommand(t *testing.T) {
	panel := &LegacyPanel{Kind: LegacyPanelCommand, Items: []string{"a", "b"}}
	rect := legacyCommandPanelRect
	cell := legacyCommandRowRect(rect, 1)
	hit, ok := panel.HitTest(cell.X+2, cell.Y+2)
	if !ok || hit.Kind != "item" || hit.Index != 1 {
		t.Fatalf("command row hit = %+v, %v", hit, ok)
	}
	empty := legacyCommandRowRect(rect, 5)
	if _, ok := panel.HitTest(empty.X+2, empty.Y+2); ok {
		t.Fatalf("empty row should not hit")
	}
	back := rect.backButton()
	hit, ok = panel.HitTest(back.X+1, back.Y+1)
	if !ok || hit.Kind != "back" {
		t.Fatalf("back hit = %+v, %v", hit, ok)
	}
	if _, ok := panel.HitTest(0, 0); ok {
		t.Fatalf("outside panel should not hit")
	}
}

func TestLegacyPanelHitTestSystemAndSlots(t *testing.T) {
	system := &LegacyPanel{Kind: LegacyPanelSystem}
	controls := legacySystemControls(legacySystemPanelRect)
	bgm := controls[4].Rect
	hit, ok := system.HitTest(bgm.X+1, bgm.Y+1)
	if !ok || hit.Kind != "bgm" {
		t.Fatalf("bgm hit = %+v, %v", hit, ok)
	}
	save := &LegacyPanel{Kind: LegacyPanelSave}
	cell := legacySlotRect(legacySavePanelRect, 13)
	hit, ok = save.HitTest(cell.X+1, cell.Y+1)
	if !ok || hit.Kind != "save" || hit.Index != 13 {
		t.Fatalf("save slot hit = %+v, %v", hit, ok)
	}
}

func TestLegacyFrameAndTitleButtons(t *testing.T) {
	if index, ok := FrameButtonAt(150, 360); !ok || index != 0 {
		t.Fatalf("frame 0 = %d, %v", index, ok)
	}
	if index, ok := FrameButtonAt(300, 360); !ok || index != 1 {
		t.Fatalf("frame 1 = %d, %v", index, ok)
	}
	if index, ok := FrameButtonAt(450, 360); !ok || index != 2 {
		t.Fatalf("frame 2 = %d, %v", index, ok)
	}
	if _, ok := FrameButtonAt(10, 10); ok {
		t.Fatalf("frame hit outside bar")
	}
	if index, ok := TitleItemAt(200, 360); !ok || index != 0 {
		t.Fatalf("title 0 = %d, %v", index, ok)
	}
	if index, ok := TitleItemAt(200, 400); !ok || index != 1 {
		t.Fatalf("title 1 = %d, %v", index, ok)
	}
	if index, ok := TitleItemAt(200, 440); !ok || index != 2 {
		t.Fatalf("title 2 = %d, %v", index, ok)
	}
}

func TestLegacyStageTitleAndBlackKey(t *testing.T) {
	stage := NewLegacyStage()
	frame := gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	stage.ComposeTo(frame, false)
	stage.SetTitle(viewTestSolid(ScreenWidth, ScreenHeight, 10, 20, 30))
	if !stage.ComposeTo(frame, false) {
		t.Fatalf("title should be dirty")
	}
	if frame.Buffer[0] != 10 || frame.Buffer[2] != 30 {
		t.Fatalf("title not drawn: %d %d", frame.Buffer[0], frame.Buffer[2])
	}
	stage.SetTitle(nil)
	if !stage.ComposeTo(frame, false) {
		t.Fatalf("clearing title should be dirty")
	}
	keyed := WithBlackKey(gfx.NewBsgImage(2, 1, 0, 0, []byte{0, 0, 0, 255, 1, 2, 3, 255}))
	if keyed.Pixels[3] != 0 || keyed.Pixels[7] != 255 {
		t.Fatalf("black key alpha = %d, %d", keyed.Pixels[3], keyed.Pixels[7])
	}
}
