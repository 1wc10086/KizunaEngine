package config

import "testing"

func TestApplyBuiltinAncientDefaults(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	probe := BuiltinProbe{Exists: func(string) bool { return true }}
	if !ApplyBuiltinAncientDefaults(cfg, probe) {
		t.Fatal("ancient defaults were not applied")
	}
	if cfg.Title.Base != "title_base" || len(cfg.Title.Entries) != 7 {
		t.Fatalf("title = %+v", cfg.Title)
	}
	for index, entry := range cfg.Title.Entries {
		if entry.Frame != index || !entry.HoverOnly || entry.Frames != 7 {
			t.Fatalf("entry %d = %+v", index, entry)
		}
	}
	if cfg.Title.Entries[3].Key != "museum_music" {
		t.Fatalf("entry 3 = %+v", cfg.Title.Entries[3])
	}
	if cfg.Message.BaseY != 440 || cfg.Message.Unagashi != "unagashi" {
		t.Fatalf("message = %+v", cfg.Message)
	}
	if !cfg.GameMenu.Overlay || len(cfg.GameMenu.Buttons) != 12 {
		t.Fatalf("game menu = %+v", cfg.GameMenu)
	}
}

func TestApplyBuiltinAncientDefaultsRequiresAssets(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	probe := BuiltinProbe{Exists: func(name string) bool { return name != "title_menu" }}
	if ApplyBuiltinAncientDefaults(cfg, probe) {
		t.Fatal("defaults applied despite a missing asset")
	}
	if !cfg.Title.Empty() {
		t.Fatal("cfg was modified on rejection")
	}
}
