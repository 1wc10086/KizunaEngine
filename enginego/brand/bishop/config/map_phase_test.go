package config

import "testing"

func TestParseMapPhaseUK(t *testing.T) {
	document := ParseIniDocumentText(`
[map phase]
base = map_base
hit table = map_hit
week no = map_wno, 10, 696, 8
week day = map_wday, 7, 697, 46
time zone image = map_time
time zone image pos = 572, 60
next time = map_next, 2, 100, 500
qsave button = map_qsave, 2, 200, 500
config button = map_config, 2, 300, 500
`)
	phase := parseMapPhase(document)
	if phase.Empty() {
		t.Fatal("map phase should be parsed")
	}
	if phase.Background != "map_base" || phase.HitImage != "map_hit" {
		t.Fatalf("background/hit = %q/%q", phase.Background, phase.HitImage)
	}
	if phase.WeekNo != (DigitStrip{Image: "map_wno", Frames: 10, X: 696, Y: 8}) {
		t.Fatalf("week no = %+v", phase.WeekNo)
	}
	if phase.WeekDay != (DigitStrip{Image: "map_wday", Frames: 7, X: 697, Y: 46}) {
		t.Fatalf("week day = %+v", phase.WeekDay)
	}
	if phase.TimeZone != (DigitStrip{Image: "map_time", Frames: 4, X: 572, Y: 60}) {
		t.Fatalf("time zone = %+v", phase.TimeZone)
	}
	if phase.TimeVariable != "#time" {
		t.Fatalf("time variable = %q", phase.TimeVariable)
	}
	if len(phase.Buttons) != 3 {
		t.Fatalf("buttons = %+v", phase.Buttons)
	}
	if phase.Buttons[0] != (MapPhaseButton{Tool: "next_time", Image: "map_next", Frames: 2, X: 100, Y: 500}) {
		t.Fatalf("next time = %+v", phase.Buttons[0])
	}
	if phase.Buttons[1].Tool != "qsave" || phase.Buttons[2].Tool != "config" {
		t.Fatalf("tools = %q, %q", phase.Buttons[1].Tool, phase.Buttons[2].Tool)
	}
}

func TestParseMapPhaseUKIE(t *testing.T) {
	document := ParseIniDocumentText(`
[map phase]
time zone = map_time, 4, 60, 572
date = #day
item button = map_item, 1, 10, 20
`)
	phase := parseMapPhase(document)
	if phase.TimeZone != (DigitStrip{Image: "map_time", Frames: 4, X: 60, Y: 572}) {
		t.Fatalf("time zone = %+v", phase.TimeZone)
	}
	if phase.DayVariable != "#day" {
		t.Fatalf("day variable = %q", phase.DayVariable)
	}
	if len(phase.Buttons) != 1 || phase.Buttons[0].Tool != "status" || phase.Buttons[0].Frames != 1 {
		t.Fatalf("buttons = %+v", phase.Buttons)
	}
}

func TestParseMapPhaseMissingSection(t *testing.T) {
	phase := parseMapPhase(ParseIniDocumentText("[other]\nbase = x\n"))
	if !phase.Empty() {
		t.Fatalf("phase = %+v, want empty", phase)
	}
}
