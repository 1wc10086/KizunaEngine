package config

import (
	"testing"
)

func TestApplyBuiltinBishopDefaults(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	exists := map[string]bool{}
	for _, name := range builtinBishopAssets {
		exists[name] = true
	}
	probe := BuiltinProbe{
		Exists:    func(name string) bool { return exists[name] },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
	if !ApplyBuiltinBishopDefaults(cfg, probe) {
		t.Fatal("defaults were not applied")
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Fatalf("screen = %dx%d, want 800x600", cfg.Width, cfg.Height)
	}
	if cfg.Title.Empty() || cfg.Title.Base != "title_base" || len(cfg.Title.Entries) != 6 {
		t.Fatalf("title = %+v", cfg.Title)
	}
	if cfg.Title.Entries[1].Key != "start" || cfg.Title.Entries[1].Y != 304 {
		t.Fatalf("start entry = %+v", cfg.Title.Entries[1])
	}
	if cfg.Message.Empty() || cfg.Message.Base != "message" || cfg.Message.BaseY != 450 {
		t.Fatalf("message = %+v", cfg.Message)
	}
	if cfg.Font.Size != 16 || cfg.Font.Lines != 3 {
		t.Fatalf("font = %+v", cfg.Font)
	}
	if len(cfg.GameMenu.Buttons) != 13 {
		t.Fatalf("menu buttons = %d, want 13", len(cfg.GameMenu.Buttons))
	}
	if cfg.DataScreen.SaveBase != "dm_save" || len(cfg.DataScreen.Slots) != 12 {
		t.Fatalf("data screen = %+v", cfg.DataScreen)
	}
	if len(cfg.DataScreen.PageHits) != 10 || cfg.DataScreen.PageHits[0].X != 626 {
		t.Fatalf("page hits = %+v", cfg.DataScreen.PageHits)
	}
	if cfg.Log.Base != "kaisou" || cfg.Log.Lines != 5 {
		t.Fatalf("log = %+v", cfg.Log)
	}
}

func TestApplyBuiltinBishopDefaultsRequiresAssets(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	probe := BuiltinProbe{
		Exists:    func(name string) bool { return name != "message_hit" },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
	if ApplyBuiltinBishopDefaults(cfg, probe) {
		t.Fatal("defaults applied despite a missing asset")
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		t.Fatal("cfg was modified on rejection")
	}
}

func TestApplyBuiltinBishopDefaultsKeepsExistingConfig(t *testing.T) {
	cfg := newBsxxConfiguration(ParseIniDocumentText("[message window]\nbase window = custom\n"))
	probe := BuiltinProbe{
		Exists:    func(string) bool { return true },
		RevScript: true,
		Width:     800,
		Height:    600,
	}
	if ApplyBuiltinBishopDefaults(cfg, probe) {
		t.Fatal("defaults applied over existing message config")
	}
	if cfg.Message.Base != "custom" {
		t.Fatalf("message base = %q", cfg.Message.Base)
	}
}

func TestParseDataScreenPageHits(t *testing.T) {
	cfg := newBsxxConfiguration(ParseIniDocumentText(`[data screen]
save base = s
load base = l
data slots = 1
page hit0 = 10, 20
page hit1 = 38, 20
`))
	if len(cfg.DataScreen.PageHits) != 2 || cfg.DataScreen.PageHits[1].X != 38 {
		t.Fatalf("page hits = %+v", cfg.DataScreen.PageHits)
	}
}
