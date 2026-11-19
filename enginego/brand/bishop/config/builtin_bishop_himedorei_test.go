package config

import "testing"

func himedoreiProbe(extra ...string) BuiltinProbe {
	exists := map[string]bool{}
	for _, name := range builtinHimedoreiAssets {
		exists[name] = true
	}
	for _, name := range extra {
		exists[name] = true
	}
	return BuiltinProbe{
		Exists:    func(name string) bool { return exists[name] },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
}

func TestApplyBuiltinHimedoreiDefaults(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	if !ApplyBuiltinBishopDefaults(cfg, himedoreiProbe()) {
		t.Fatal("HIMEDOREI defaults were not applied")
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("screen = %dx%d, want 800x600", cfg.Width, cfg.Height)
	}
	if cfg.EyePattern != 4 {
		t.Fatalf("eye pattern = %d, want 4", cfg.EyePattern)
	}
	if cfg.Title.Base != "title_base" || cfg.Title.BgmName != "bgm04" || len(cfg.Title.Entries) != 6 {
		t.Fatalf("title = %+v", cfg.Title)
	}
	if cfg.Title.BaseY != -412 {
		t.Fatalf("title base y = %d, want -412", cfg.Title.BaseY)
	}
	if cfg.Title.Entries[1].Key != "start" || cfg.Title.Entries[1].X != 672 || cfg.Title.Entries[1].Y != 256 {
		t.Fatalf("start entry = %+v", cfg.Title.Entries[1])
	}
	if cfg.Message.Base != "message" || cfg.Message.BaseY != 430 || !cfg.Message.TextColorSet {
		t.Fatalf("message = %+v", cfg.Message)
	}
	if len(cfg.GameMenu.Buttons) != 11 {
		t.Fatalf("game menu = %+v", cfg.GameMenu)
	}
	if cfg.GameMenu.Buttons[2].Key != "quick save" || cfg.GameMenu.Buttons[2].X != 602 {
		t.Fatalf("toolbar = %+v", cfg.GameMenu.Buttons[2])
	}
	if len(cfg.DataScreen.Slots) != 12 || cfg.DataScreen.Slots[0].X != 32 || cfg.DataScreen.Slots[0].Y != 112 {
		t.Fatalf("data slots = %+v", cfg.DataScreen.Slots)
	}
	if cfg.DataScreen.SlotBase != "dm_slotbase" || cfg.DataScreen.AutoSlotIndex != 0 {
		t.Fatalf("data screen = %+v", cfg.DataScreen)
	}
	if len(cfg.LegacyConfig.Characters) != 5 || len(cfg.LegacyConfig.ExtraToggles) != 2 {
		t.Fatalf("legacy config = %+v", cfg.LegacyConfig)
	}
	if cfg.LegacyConfig.Base != "cfg_ex" || cfg.LegacyConfig.BaseX != 24 || cfg.LegacyConfig.BaseY != 98 {
		t.Fatalf("config base = %+v", cfg.LegacyConfig)
	}
	if !cfg.Museum.Music.BakedTracks || cfg.Museum.Music.TrackCount != 20 {
		t.Fatalf("museum music = %+v", cfg.Museum.Music)
	}
	if cfg.Museum.Base != "museum_base" || len(cfg.Museum.Modes) != 4 {
		t.Fatalf("museum = %+v", cfg.Museum)
	}
	if cfg.Museum.Modes[0].X != 32 || cfg.Museum.Modes[0].Y != 44 {
		t.Fatalf("museum first tab = %+v", cfg.Museum.Modes[0])
	}
	if len(cfg.Museum.CG.Thumbs) != 20 || cfg.Museum.CG.Thumbs[0].X != 256 ||
		cfg.Museum.CG.Thumbs[0].Y != 48 || cfg.Museum.CG.Thumbs[1].X != 388 {
		t.Fatalf("museum cg grid = %+v", cfg.Museum.CG.Thumbs[:2])
	}
	if cfg.SelectWindow.Design(0).HeaderFooterOnce != true {
		t.Fatal("select window header/footer should be drawn once")
	}
}

func TestApplyBuiltinHimedoreiRequiresAssets(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	probe := himedoreiProbe()
	missing := probe.Exists
	probe.Exists = func(name string) bool { return name != "dm_slotbase" && missing(name) }
	if ApplyBuiltinBishopDefaults(cfg, probe) {
		t.Fatal("defaults applied despite a missing asset")
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		t.Fatal("cfg was modified on rejection")
	}
}

func TestBuiltinHimedoreiSignatureIsDisjoint(t *testing.T) {
	// KD 的 kaisou、MK2 的 dm_base、AFDVD 的 gamemenu 任意一个出现，
	// 都说明不是ヒメドレイ。
	for _, extra := range []string{"kaisou", "dm_base", "gamemenu", "map_base"} {
		if isBuiltinHimedorei(himedoreiProbe(extra)) {
			t.Fatalf("HIMEDOREI matched when %s is present", extra)
		}
	}
	if isBuiltinHimedorei(afdvdProbe(builtinBishopAssets)) {
		t.Fatal("KD assets matched the HIMEDOREI signature")
	}
}
