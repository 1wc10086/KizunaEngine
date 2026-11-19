package config

import "testing"

func TestApplyBuiltinKsToolbar(t *testing.T) {
	cfg := &BsxxConfiguration{}
	probe := BuiltinProbe{Exists: func(string) bool { return true }}
	if !ApplyBuiltinKsToolbar(cfg, probe) {
		t.Fatal("ApplyBuiltinKsToolbar() = false, want true")
	}
	if len(cfg.GameMenu.Buttons) != 15 {
		t.Fatalf("buttons = %d, want 15", len(cfg.GameMenu.Buttons))
	}
	keys := map[string]bool{}
	for _, button := range cfg.GameMenu.Buttons {
		keys[button.Key] = true
	}
	for _, key := range []string{"skip", "auto", "voice", "quick save", "quick load",
		"save", "load", "config", "status", "log", "dress", "cut in",
		"view up", "view middle", "view down"} {
		if !keys[key] {
			t.Errorf("missing key %q", key)
		}
	}
	if !cfg.GameMenu.Overlay {
		t.Error("Overlay = false, want true")
	}
}

func TestApplyBuiltinKsToolbarKeepsExplicitButtons(t *testing.T) {
	cfg := &BsxxConfiguration{GameMenu: GameMenu{
		Buttons: []MenuButton{{Key: "custom"}},
	}}
	probe := BuiltinProbe{Exists: func(string) bool { return true }}
	if ApplyBuiltinKsToolbar(cfg, probe) {
		t.Fatal("ApplyBuiltinKsToolbar() = true, want false for explicit buttons")
	}
	if len(cfg.GameMenu.Buttons) != 1 || cfg.GameMenu.Buttons[0].Key != "custom" {
		t.Fatalf("buttons changed: %+v", cfg.GameMenu.Buttons)
	}
}

func TestApplyBuiltinKsToolbarRequiresPlaceBanner(t *testing.T) {
	cfg := &BsxxConfiguration{}
	probe := BuiltinProbe{Exists: func(name string) bool {
		return name != "plase_1" && name != "map_plase_marker1"
	}}
	if ApplyBuiltinKsToolbar(cfg, probe) {
		t.Fatal("ApplyBuiltinKsToolbar() = true without place banner assets")
	}
}
