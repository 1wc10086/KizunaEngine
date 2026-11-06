package config

import "testing"

func TestApplyBuiltinInformation(t *testing.T) {
	cfg := newBsxxConfiguration(EmptyIniDocument())
	applyBuiltinInformation(cfg)

	w := cfg.Information
	if w.Empty() {
		t.Fatal("information window should not be empty")
	}
	if w.Base != "" {
		t.Fatalf("base = %q, want empty", w.Base)
	}
	if w.DayVariable != "#day" {
		t.Fatalf("day variable = %q", w.DayVariable)
	}
	if w.TimeVariable != "#time" {
		t.Fatalf("time variable = %q", w.TimeVariable)
	}
	if w.WeekStrip != (DigitStrip{Image: "msg_info_str0", Frames: 12, X: 22, Y: 572}) {
		t.Fatalf("week strip = %+v", w.WeekStrip)
	}
	if w.WeekDayStrip != (DigitStrip{Image: "msg_info_str1", Frames: 7, X: 106, Y: 575}) {
		t.Fatalf("week day strip = %+v", w.WeekDayStrip)
	}
	// msg_info_time 的图头自带 (183,123) 偏移，所以 X/Y 写对话框原点。
	if w.TimeStrip != (DigitStrip{Image: "msg_info_time", Frames: 6, X: 0, Y: 450}) {
		t.Fatalf("time strip = %+v", w.TimeStrip)
	}
	if w.TimeZoneLow != 0 || w.TimeZoneHigh != 5 {
		t.Fatalf("time zone = %d..%d, want 0..5", w.TimeZoneLow, w.TimeZoneHigh)
	}
	if w.WeekDays != 10 || w.WeekStart != 0 {
		t.Fatalf("week days/start = %d/%d, want 10/0", w.WeekDays, w.WeekStart)
	}
	if !w.DayZero || !w.TimeZero {
		t.Fatalf("zero flags = day %v time %v, want both true", w.DayZero, w.TimeZero)
	}
	if w.WeekDayFrom != 10 {
		t.Fatalf("week day from = %d, want 10", w.WeekDayFrom)
	}
}
