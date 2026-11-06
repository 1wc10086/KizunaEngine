package config

import "testing"

func gkProbe(names []string, exclude ...string) BuiltinProbe {
	exists := map[string]bool{}
	for _, name := range names {
		exists[name] = true
	}
	for _, name := range exclude {
		exists[name] = true
	}
	return BuiltinProbe{
		Exists:    func(name string) bool { return exists[name] },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
}

func TestApplyBuiltinGK(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	if !applyBuiltinGK(cfg, gkProbe(builtinGKAssets)) {
		t.Fatal("GK signature should match")
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("screen = %dx%d", cfg.Width, cfg.Height)
	}
	if len(cfg.Title.Entries) != 5 || !cfg.Title.ResumeAfterTitleCall {
		t.Fatalf("title = %+v", cfg.Title)
	}
	if cfg.Message.Base != "message" || !cfg.Message.SpeakerInline {
		t.Fatalf("message = %+v", cfg.Message)
	}
	if len(cfg.GameMenu.Buttons) < 9 {
		t.Fatalf("game menu buttons = %d", len(cfg.GameMenu.Buttons))
	}
	if len(cfg.DataScreen.Slots) != 5 || !cfg.DataScreen.SlotFrameGeneric {
		t.Fatalf("data screen = %+v", cfg.DataScreen)
	}
	if cfg.LegacyConfig.Base != "config_base" || len(cfg.LegacyConfig.Characters) != 6 {
		t.Fatalf("config = %+v", cfg.LegacyConfig)
	}
	if cfg.Museum.Base != "museum_base" {
		t.Fatalf("museum = %+v", cfg.Museum)
	}
}

func TestBuiltinGKExcludesNeighbours(t *testing.T) {
	for _, name := range builtinGKExcluded {
		if isBuiltinGK(gkProbe(builtinGKAssets, name)) {
			t.Fatalf("GK signature must not match when '%s' exists", name)
		}
	}
}
