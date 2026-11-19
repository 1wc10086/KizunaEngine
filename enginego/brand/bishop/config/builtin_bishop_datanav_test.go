package config

import "testing"

func TestApplyBuiltinDataNav(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	applyBuiltinDataNav(cfg)

	cases := []struct {
		name string
		got  UIButton
		want UIButton
	}{
		{"Config", cfg.DataScreen.Config, UIButton{Image: "cfg_mode_config", Frames: 3, X: 632, Y: 404}},
		// cfg_mode_save/load 第 2 帧是禁用叉号，固定画第 1 帧避免当前页被画成禁用。
		{"SaveMode", cfg.DataScreen.SaveMode, UIButton{Image: "cfg_mode_save", Frames: 3, X: 632, Y: 432, Start: 1, FixedFrame: true}},
		{"LoadMode", cfg.DataScreen.LoadMode, UIButton{Image: "cfg_mode_load", Frames: 3, X: 632, Y: 460, Start: 1, FixedFrame: true}},
		{"Back", cfg.DataScreen.Back, UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 488, Start: 3, FixedFrame: true}},
		{"Title", cfg.DataScreen.Title, UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 516, Start: 4, FixedFrame: true}},
		{"Exit", cfg.DataScreen.Exit, UIButton{Image: "cfg_button", Frames: 6, X: 632, Y: 544, Start: 5, FixedFrame: true}},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestApplyBuiltinDataNavCfgButtonFrames(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	applyBuiltinDataNav(cfg)
	cfgButton := UIButton{Image: "cfg_button", Frames: 6}
	if h := cfgButton.FrameHeight(144); h != 24 {
		t.Fatalf("frame height = %d, want 24", h)
	}
	for _, b := range []UIButton{cfg.DataScreen.Back, cfg.DataScreen.Title, cfg.DataScreen.Exit} {
		if b.Start < 0 || b.Start >= b.Frames {
			t.Fatalf("start %d out of range for %+v", b.Start, b)
		}
	}
}
