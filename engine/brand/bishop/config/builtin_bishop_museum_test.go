package config

import "testing"

func TestApplyBuiltinMuseumKD(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	applyBuiltinMuseum(cfg)
	museum := cfg.Museum

	if museum.Base != "museum_base" {
		t.Fatalf("base = %q", museum.Base)
	}
	if len(museum.Modes) != 4 {
		t.Fatalf("modes = %d, want 4", len(museum.Modes))
	}
	for index, mode := range museum.Modes {
		if mode.Image != "museum_menu" || mode.Part != index+1 {
			t.Fatalf("mode%d = %+v, want museum_menu part %d", index, mode.UIButton, index+1)
		}
	}
	if museum.Return.Part != 5 || museum.Return.Frames != 2 {
		t.Fatalf("return = %+v", museum.Return)
	}

	if len(museum.CG.ThumbStrips) != 8 || museum.CG.ThumbStrips[0] != "cg0_thumb" {
		t.Fatalf("cg strips = %v", museum.CG.ThumbStrips)
	}
	if len(museum.CG.Thumbs) != 20 ||
		museum.CG.Thumbs[0] != (UIPoint{X: 84, Y: 110}) ||
		museum.CG.Thumbs[1] != (UIPoint{X: 212, Y: 110}) ||
		museum.CG.Thumbs[2] != (UIPoint{X: 340, Y: 110}) {
		t.Fatalf("cg thumbs = %v", museum.CG.Thumbs)
	}

	if len(museum.Scene.ThumbStrips) != 4 || museum.Scene.ThumbStrips[0] != "scene0_thumb" {
		t.Fatalf("scene strips = %v", museum.Scene.ThumbStrips)
	}
	if len(museum.Scene.Thumbs) != 20 ||
		museum.Scene.Thumbs[0] != (UIPoint{X: 84, Y: 110}) ||
		museum.Scene.Thumbs[1] != (UIPoint{X: 212, Y: 110}) ||
		museum.Scene.Thumbs[2] != (UIPoint{X: 340, Y: 110}) {
		t.Fatalf("scene thumbs = %v", museum.Scene.Thumbs)
	}

	// BGM 控制盒：Play/Stop 的部件号不能互换（原图左上停止、右上播放），
	// 两个图标图都是两帧，Frames=1 会把常态/悬停叠在一起。
	music := museum.Music
	if music.Play.Part != 3 || music.Stop.Part != 2 || music.Play.Frames != 2 {
		t.Fatalf("play/stop = %+v / %+v", music.Play, music.Stop)
	}
	if music.ControlBase.Part != 1 || music.Volume.Part != 8 || !music.VolumeKnob {
		t.Fatalf("control base/volume = %+v / %+v knob=%v", music.ControlBase, music.Volume, music.VolumeKnob)
	}
	if music.VolumeTrackHeight != 185 {
		t.Fatalf("volume track = %d, want 185", music.VolumeTrackHeight)
	}

	// CG 查看器：cgm_btn 是 10 帧竖排，常态 0..4、悬停 5..9。
	viewer := museum.Viewer
	if viewer.Base.Image != "cgm_base" || viewer.PanelX != 640 || viewer.PanelY != 520 {
		t.Fatalf("viewer base = %+v panel=%d,%d", viewer.Base, viewer.PanelX, viewer.PanelY)
	}
	if viewer.Prev.Start != 0 || viewer.Exit.Start != 1 || viewer.Next.Start != 2 ||
		viewer.CutIn.Start != 3 || viewer.Config.Start != 4 {
		t.Fatalf("viewer starts = %d/%d/%d/%d/%d",
			viewer.Prev.Start, viewer.Exit.Start, viewer.Next.Start, viewer.CutIn.Start, viewer.Config.Start)
	}
	if viewer.ButtonBase != 0 || viewer.HoverOffset != 5 || viewer.ToggleHoverOffset != 5 {
		t.Fatalf("viewer frames = %d/%d/%d", viewer.ButtonBase, viewer.HoverOffset, viewer.ToggleHoverOffset)
	}
}
