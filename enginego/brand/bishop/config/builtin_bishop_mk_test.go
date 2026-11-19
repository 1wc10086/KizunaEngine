package config

import "testing"

// mkProbe 只声明 MK 的素材集合；excluded 里的名字用来验证相邻世代不会被误判。
func mkProbe(extra ...string) BuiltinProbe {
	assets := make(map[string]bool, len(builtinMkAssets)+len(extra))
	for _, name := range builtinMkAssets {
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

func TestBuiltinMkAppliesFullLayout(t *testing.T) {
	cfg, err := Load("/nonexistent/bsxx.cfg")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ApplyBuiltinBishopDefaults(cfg, mkProbe()) {
		t.Fatal("MK profile did not apply")
	}
	if len(cfg.Title.Entries) != 6 {
		t.Fatalf("title entries = %d, want 6", len(cfg.Title.Entries))
	}
	if cfg.Message.Base != "message" || cfg.Message.BaseY != 496 {
		t.Fatalf("message base = %q @%d, want message @496", cfg.Message.Base, cfg.Message.BaseY)
	}
	if len(cfg.GameMenu.Buttons) != 9 {
		t.Fatalf("game menu buttons = %d, want 9", len(cfg.GameMenu.Buttons))
	}
	if len(cfg.DataScreen.Slots) != 12 || cfg.DataScreen.PageMax != 10 {
		t.Fatalf("data screen = %d slots / %d pages, want 12 / 10", len(cfg.DataScreen.Slots), cfg.DataScreen.PageMax)
	}
	if len(cfg.Museum.Modes) != 4 || len(cfg.Museum.StripPortraits) != 6 {
		t.Fatalf("museum = %d modes / %d portraits, want 4 / 6", len(cfg.Museum.Modes), len(cfg.Museum.StripPortraits))
	}
	if cfg.MapPhase.Background != "map_base" || cfg.MapPhase.WeekDayShift != 1 {
		t.Fatalf("map phase bg=%q weekday shift=%d", cfg.MapPhase.Background, cfg.MapPhase.WeekDayShift)
	}
	if got := cfg.SelectWindow.Design(0).Item; got != "item0" {
		t.Fatalf("select item = %q, want item0", got)
	}
}

func TestBuiltinMkRejectsNeighbourGenerations(t *testing.T) {
	for _, name := range []string{"dm_base", "msg_base", "title_menu_op"} {
		cfg, _ := Load("/nonexistent/bsxx.cfg")
		if ApplyBuiltinBishopDefaults(cfg, mkProbe(name)) {
			t.Fatalf("MK profile accepted %q", name)
		}
	}
}
