package config

import "testing"

func TestParseInformationWindow(t *testing.T) {
	document := ParseIniDocument([]byte(`[information window]

base			= info_base
base pos		=   1,   5
sync button pos	= 1

date			= #day
day sp border	= 1000
day adjustment	= 0

day no			= info_dayno, 10, 137,  10
day unit		= info_unit,   1,  47,  11
day sp			= info_sp,     1,  47,  11
day no offset	= -14

day				= #day

time			= #time
time zone range	= 1, 6
time zone		= info_time, 5,   5,  11
`))

	window := parseInformationWindow(document)
	if window.Empty() {
		t.Fatal("window should not be empty")
	}
	if window.Base != "info_base" {
		t.Fatalf("base = %q", window.Base)
	}
	if window.BaseX != 1 || window.BaseY != 5 {
		t.Fatalf("base pos = %d,%d", window.BaseX, window.BaseY)
	}
	if window.DayVariable != "#day" {
		t.Fatalf("day variable = %q", window.DayVariable)
	}
	if window.DayStrip.Image != "info_dayno" {
		t.Fatalf("day strip = %q", window.DayStrip.Image)
	}
	if window.DayStrip.Frames != 10 {
		t.Fatalf("day strip frames = %d", window.DayStrip.Frames)
	}
	if window.DayStrip.X != 137 || window.DayStrip.Y != 10 {
		t.Fatalf("day strip pos = %d,%d", window.DayStrip.X, window.DayStrip.Y)
	}
	// 下一桁偏移是独立一项，必须读出来；为 0 会让两位数叠在个位上。
	if window.DayStrip.Offset != -14 {
		t.Fatalf("day strip offset = %d", window.DayStrip.Offset)
	}
	if window.DayUnit != "info_unit" {
		t.Fatalf("day unit = %q", window.DayUnit)
	}
	if window.DayUnitX != 47 || window.DayUnitY != 11 {
		t.Fatalf("day unit pos = %d,%d", window.DayUnitX, window.DayUnitY)
	}
	// day sp 是"日期越过 day sp border 之后整块替换"的图案，不是占位符；
	// 门槛与调整量也必须读出来，否则特殊日期会显示成荒唐的天数。
	if window.DaySpecial != "info_sp" {
		t.Fatalf("day special = %q", window.DaySpecial)
	}
	if window.DayBorder != 1000 {
		t.Fatalf("day border = %d, want 1000", window.DayBorder)
	}
	if window.DayAdjustment != 0 {
		t.Fatalf("day adjustment = %d, want 0", window.DayAdjustment)
	}
	if window.TimeVariable != "#time" {
		t.Fatalf("time variable = %q", window.TimeVariable)
	}
	if window.TimeStrip.Image != "info_time" || window.TimeStrip.Frames != 5 {
		t.Fatalf("time strip = %q/%d", window.TimeStrip.Image, window.TimeStrip.Frames)
	}
	if window.TimeZoneLow != 1 || window.TimeZoneHigh != 6 {
		t.Fatalf("time zone = %d..%d", window.TimeZoneLow, window.TimeZoneHigh)
	}
}

func TestParseInformationWindowWeekStrips(t *testing.T) {
	// 黒の教室 的版式：没有 day no，只有 week no / week day；坐标同样带算式，
	// 且相对 base pos 偏移。解析必须保留帧数与真实坐标。
	document := ParseIniDocument([]byte(`[information window]
base			= info_base
base pos		= 53, 617

date			= #day
day sp border	= 1000
day adjustment	= -1

week no			= info_wno,  9,  64-53, 626-617
week day		= info_wday, 6,  94-53, 650-617
day sp			= info_daysp,1,  60-53, 631-617
`))

	window := parseInformationWindow(document)
	if window.WeekStrip.Image != "info_wno" || window.WeekStrip.Frames != 9 {
		t.Fatalf("week strip = %q/%d", window.WeekStrip.Image, window.WeekStrip.Frames)
	}
	if window.WeekStrip.X != 11 || window.WeekStrip.Y != 9 {
		t.Fatalf("week strip pos = %d,%d", window.WeekStrip.X, window.WeekStrip.Y)
	}
	if window.WeekDayStrip.Image != "info_wday" || window.WeekDayStrip.Frames != 6 {
		t.Fatalf("week day strip = %q/%d", window.WeekDayStrip.Image, window.WeekDayStrip.Frames)
	}
	if window.WeekDayStrip.X != 41 || window.WeekDayStrip.Y != 33 {
		t.Fatalf("week day pos = %d,%d", window.WeekDayStrip.X, window.WeekDayStrip.Y)
	}
	if window.Empty() {
		t.Fatal("a window with only week strips must not be empty")
	}
}

func TestParseInformationWindowWeekDaysFromStrip(t *testing.T) {
	// UK 的曜日图是 7 帧（日..土）且没有 week days 键，换算基数必须跟着图走。
	document := ParseIniDocument([]byte(`[information window]
week no		= info_wno, 10, 14, 486
week day	= info_wday, 7, 16, 512
day			= #day
`))
	window := parseInformationWindow(document)
	if window.WeekDays != 7 {
		t.Fatalf("week days = %d, want 7 from the weekday strip", window.WeekDays)
	}
}

func TestParseInformationWindowAbsoluteCoordinates(t *testing.T) {
	// UK 的 bsxx.cfg 把坐标写成了绝对屏幕值，但 base pos 仍是 448：
	// 不修正的话 y 会变成 934，落在 600 高的屏幕外。
	document := ParseIniDocument([]byte(`[information window]
base			= info_base
base pos		= 0, 448
week no			= info_wno, 10, 14, 486
week day		= info_wday, 7, 16, 512
day sp			= info_sp, 1, 26, 502
day				= #day
`))
	raw := parseInformationWindow(document)
	if raw.WeekStrip.Y != 486 {
		t.Fatalf("unnormalized week no y = %d, want 486", raw.WeekStrip.Y)
	}
	window := parseInformationWindowForScreen(document, 600)
	if window.WeekStrip.Y != 38 {
		t.Fatalf("week no y = %d, want 38", window.WeekStrip.Y)
	}
	if window.WeekDayStrip.Y != 64 {
		t.Fatalf("week day y = %d, want 64", window.WeekDayStrip.Y)
	}
	if window.DaySpecialY != 54 {
		t.Fatalf("day special y = %d, want 54", window.DaySpecialY)
	}
}

func TestParseInformationWindowRelativeCoordinatesUntouched(t *testing.T) {
	// UKIE 的坐标相对 base pos（574）书写，y=48 本身就在屏幕内，不能被当成绝对值改写。
	document := ParseIniDocument([]byte(`[information window]
base			= info_base
base pos		= 0, 574
week no			= info_wno, 9, 17, 48
day				= #day
`))
	window := parseInformationWindowForScreen(document, 768)
	if window.BaseY != 574 {
		t.Fatalf("base pos y = %d, want 574", window.BaseY)
	}
	if window.WeekStrip.Y != 48 {
		t.Fatalf("week no y = %d, want 48 untouched", window.WeekStrip.Y)
	}
}

func TestInformationWindowMissingSection(t *testing.T) {
	window := parseInformationWindow(ParseIniDocument([]byte("[main window]\nwidth = 1024\n")))
	if !window.Empty() {
		t.Fatal("a missing section should parse to an empty window")
	}
	// 时段区间没配时应当是"任何值都显示"，而不是 0..0 把时段一直藏起来。
	if window.TimeZoneLow > window.TimeZoneHigh {
		t.Fatalf("default time zone range is inverted: %d..%d",
			window.TimeZoneLow, window.TimeZoneHigh)
	}
}
