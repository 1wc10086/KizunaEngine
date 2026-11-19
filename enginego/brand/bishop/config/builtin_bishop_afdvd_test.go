package config

import "testing"

func afdvdProbe(names []string, extra ...string) BuiltinProbe {
	exists := map[string]bool{}
	for _, name := range names {
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

func TestApplyBuiltinAFDVDDefaults(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	if !ApplyBuiltinBishopDefaults(cfg, afdvdProbe(builtinAFDVDAssets)) {
		t.Fatal("AFDVD defaults were not applied")
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("screen = %dx%d, want 800x600", cfg.Width, cfg.Height)
	}
	if cfg.Title.Base != "title_base" || cfg.Title.BgmName != "bgm02" || len(cfg.Title.Entries) != 6 {
		t.Fatalf("title = %+v", cfg.Title)
	}
	if cfg.Title.Entries[1].Key != "start" || cfg.Title.Entries[1].X != 64 || cfg.Title.Entries[1].Y != 278 {
		t.Fatalf("start entry = %+v", cfg.Title.Entries[1])
	}
	if cfg.Message.Base != "message" || cfg.Message.BaseY != 448 {
		t.Fatalf("message = %+v", cfg.Message)
	}
	if cfg.Font.Size != 16 || cfg.Font.Lines != 3 {
		t.Fatalf("font = %+v", cfg.Font)
	}
	if cfg.GameMenu.Base != "gamemenu" || cfg.GameMenu.BaseY != 572 || len(cfg.GameMenu.Buttons) != 11 {
		t.Fatalf("game menu = %+v", cfg.GameMenu)
	}
	if cfg.DataScreen.SaveBase != "dm_base" || cfg.DataScreen.HitTable != "dm_hit" || len(cfg.DataScreen.Slots) != 12 {
		t.Fatalf("data screen = %+v", cfg.DataScreen)
	}
	if len(cfg.DataScreen.PageHits) != 10 || cfg.DataScreen.PageHits[0].X != 244 {
		t.Fatalf("page hits = %+v", cfg.DataScreen.PageHits)
	}
}

func TestApplyBuiltinAFDVDRequiresAssets(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	probe := afdvdProbe(builtinAFDVDAssets)
	missing := probe.Exists
	probe.Exists = func(name string) bool { return name != "dm_hit" && missing(name) }
	if ApplyBuiltinBishopDefaults(cfg, probe) {
		t.Fatal("defaults applied despite a missing asset")
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		t.Fatal("cfg was modified on rejection")
	}
}

func TestBuiltinAFDVDAndKDSignaturesAreDisjoint(t *testing.T) {
	kd := afdvdProbe(builtinBishopAssets)
	if isBuiltinAFDVD(kd) {
		t.Fatal("KD assets matched the AFDVD signature")
	}
	afdvdWithKaisou := afdvdProbe(builtinAFDVDAssets, "kaisou")
	if isBuiltinAFDVD(afdvdWithKaisou) {
		t.Fatal("AFDVD matched when KD-only kaisou is present")
	}

	kdCfg := newBsxxConfiguration(EmptyIniDocument())
	if !ApplyBuiltinBishopDefaults(kdCfg, kd) {
		t.Fatal("KD defaults were not applied")
	}
	if kdCfg.Title.BgmName != "bgm24" || kdCfg.Message.BaseY != 450 {
		t.Fatalf("KD layout was disturbed: title=%+v message=%+v", kdCfg.Title, kdCfg.Message)
	}
}
