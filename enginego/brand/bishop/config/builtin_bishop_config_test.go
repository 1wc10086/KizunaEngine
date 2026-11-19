package config

import "testing"

func TestApplyBuiltinConfigScreen(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	applyBuiltinConfigScreen(cfg)

	screen := cfg.LegacyConfig
	if screen.Base != "config_base" || screen.HitTable != "config_hit" {
		t.Fatalf("base = %q hit = %q", screen.Base, screen.HitTable)
	}
	if len(screen.Speeds) != 2 {
		t.Fatalf("speeds = %d, want 2", len(screen.Speeds))
	}
	if len(screen.Colors) != 4 {
		t.Fatalf("colors = %d, want 4", len(screen.Colors))
	}
	if got := builtinConfigGaugeCount(screen.Volumes); got != 4 {
		t.Fatalf("volume gauges = %d, want 4", got)
	}
	if screen.Back.Image != "cfg_button" || screen.Back.Start != 3 {
		t.Fatalf("back = %+v, want cfg_button start 3", screen.Back)
	}
	if screen.VoiceLoopOff != (UIPoint{X: 180, Y: 2}) || screen.SkipUnreadOff != (UIPoint{X: 441, Y: 450}) {
		t.Fatalf("off points = %+v / %+v", screen.VoiceLoopOff, screen.SkipUnreadOff)
	}
	if screen.AutoSelectMarkFrame != 1 || screen.SkipModeMarkFrame != 2 {
		t.Fatalf("mark frames = %d / %d, want 1 / 2", screen.AutoSelectMarkFrame, screen.SkipModeMarkFrame)
	}
}

func builtinConfigGaugeCount(gauges []LegacyGauge) int {
	count := 0
	for _, gauge := range gauges {
		if !gauge.Knob.Empty() {
			count++
		}
	}
	return count
}
