package config

import "testing"

// houkago2Probe 返回一个只认识放課後2 素材的探针。
func houkago2Probe(extra ...string) BuiltinProbe {
	assets := map[string]bool{}
	for _, name := range builtinHoukago2Assets {
		assets[name] = true
	}
	for _, name := range extra {
		assets[name] = true
	}
	return BuiltinProbe{
		Exists:    func(name string) bool { return assets[name] },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
}

func TestApplyBuiltinHoukago2Defaults(t *testing.T) {
	cfg := &BsxxConfiguration{}
	if !ApplyBuiltinHoukago2Defaults(cfg, houkago2Probe()) {
		t.Fatal("built-in layout should apply when every fingerprint asset exists")
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("screen = %dx%d, want 800x600", cfg.Width, cfg.Height)
	}
	if len(cfg.Title.Entries) != 6 {
		t.Fatalf("title entries = %d, want 6", len(cfg.Title.Entries))
	}
	if cfg.Title.BgmName != "bgm08" || cfg.Title.Base != "title_base" {
		t.Fatalf("title bgm/base = %q/%q", cfg.Title.BgmName, cfg.Title.Base)
	}
	if cfg.Message.Base != "message" || cfg.Message.BaseY != 468 {
		t.Fatalf("message base = %q y=%d", cfg.Message.Base, cfg.Message.BaseY)
	}
	if cfg.Message.NameBase != "msg_name" || cfg.Message.FillRect[2] != 598 {
		t.Fatalf("name plate/fill not configured: %+v", cfg.Message)
	}
	if len(cfg.GameMenu.Buttons) != 8 {
		t.Fatalf("game menu buttons = %d, want 8", len(cfg.GameMenu.Buttons))
	}
	if len(cfg.DataScreen.Slots) != 12 || cfg.DataScreen.PageMax != 10 {
		t.Fatalf("data screen slots/page = %d/%d", len(cfg.DataScreen.Slots), cfg.DataScreen.PageMax)
	}
	if cfg.DataScreen.Confirm.OK.Empty() || cfg.DataScreen.Confirm.Cancel.Empty() {
		t.Fatal("overwrite confirmation buttons must exist (otherwise saving over a slot freezes)")
	}
	if cfg.LegacyConfig.Base != "cfg_ex" || cfg.LegacyConfig.BaseX != 72 {
		t.Fatalf("legacy config = %q x=%d", cfg.LegacyConfig.Base, cfg.LegacyConfig.BaseX)
	}
	if len(cfg.Museum.Modes) != 4 || len(cfg.Museum.CG.ThumbStrips) != 7 {
		t.Fatalf("museum modes/strips = %d/%d", len(cfg.Museum.Modes), len(cfg.Museum.CG.ThumbStrips))
	}
	if cfg.MapPhase.WeekDay.Start != 5 || cfg.Information.WeekDays != 6 {
		t.Fatalf("week mapping = start %d days %d", cfg.MapPhase.WeekDay.Start, cfg.Information.WeekDays)
	}
	if len(cfg.Information.Labels) != 4 {
		t.Fatalf("information labels = %d, want 4", len(cfg.Information.Labels))
	}
}

func TestApplyBuiltinHoukago2RejectsOtherGenerations(t *testing.T) {
	// title_menu_op 是 MK2/AFDVD 的分图标题，出现任意一张就不该由本作接管。
	if ApplyBuiltinHoukago2Defaults(&BsxxConfiguration{}, houkago2Probe("title_menu_op")) {
		t.Fatal("layout must not apply when a neighbouring generation asset is present")
	}
	// 已有配置的作品不覆盖。
	cfg := &BsxxConfiguration{
		Title:   TitlePhase{Base: "title_base", Entries: []TitleMenuEntry{{Key: "start"}}},
		Message: MessageWindow{Base: "message"},
	}
	if ApplyBuiltinHoukago2Defaults(cfg, houkago2Probe()) {
		t.Fatal("existing title/message config must be kept")
	}
}
