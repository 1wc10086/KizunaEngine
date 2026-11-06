package config

import "testing"

func TestParseTitlePhase(t *testing.T) {
	document := ParseIniDocument([]byte(`[title phase]

bgm name		= bgm01
base			= title_base
hit table		= title_hit

op				= title_menu_opening, 2, 772, 242
start			= title_menu_start,   2, 772, 292
load			= title_menu_load,    2, 772, 342
museum			= title_menu_museum,  2, 772, 442
config			= title_menu_config,  2, 772, 392
quit			= title_menu_quit,    2, 772, 492
`))

	phase := parseTitlePhase(document)
	if phase.Empty() {
		t.Fatal("phase should not be empty")
	}
	if phase.Base != "title_base" {
		t.Fatalf("base = %q", phase.Base)
	}
	if phase.BgmName != "bgm01" {
		t.Fatalf("bgm = %q", phase.BgmName)
	}
	if phase.HitTable != "title_hit" {
		t.Fatalf("hit table = %q", phase.HitTable)
	}
	// 顺序固定为 op/start/load/museum/config/quit，而不是配置文件里的书写顺序：
	// 界面绘制顺序必须稳定，否则同一部游戏在不同机器上菜单项会换位。
	want := []string{"op", "start", "load", "museum", "config", "quit"}
	if len(phase.Entries) != len(want) {
		t.Fatalf("%d entries, want %d", len(phase.Entries), len(want))
	}
	for i, key := range want {
		if phase.Entries[i].Key != key {
			t.Fatalf("entry %d key = %q, want %q", i, phase.Entries[i].Key, key)
		}
	}
	start := phase.Entries[1]
	if start.Image != "title_menu_start" {
		t.Fatalf("start image = %q", start.Image)
	}
	if start.Frames != 2 {
		t.Fatalf("start frames = %d", start.Frames)
	}
	if start.X != 772 || start.Y != 292 {
		t.Fatalf("start pos = %d,%d", start.X, start.Y)
	}
}

func TestParseTitlePhaseArithmeticCoordinates(t *testing.T) {
	// 黒の教室 的标题按钮坐标写成 "62+150*0" 这样的算式。这里必须得到真实坐标，
	// 否则每项 X 都会退化成 0、六个按钮叠在同一个位置。
	document := ParseIniDocument([]byte(`[title phase]
op    = title_menu_opening, 2, 62+150*0, 660
start = title_menu_start,   2, 62+150*1, 660
quit  = title_menu_quit,    2, 62+150*5, 660
`))

	phase := parseTitlePhase(document)
	if len(phase.Entries) != 3 {
		t.Fatalf("%d entries, want 3", len(phase.Entries))
	}
	wantX := []int{62, 212, 812}
	for i, x := range wantX {
		if phase.Entries[i].X != x {
			t.Fatalf("entry %d x = %d, want %d", i, phase.Entries[i].X, x)
		}
		if phase.Entries[i].Y != 660 {
			t.Fatalf("entry %d y = %d, want 660", i, phase.Entries[i].Y)
		}
	}
}

func TestParseTitlePhaseIgnoresUnknownKeys(t *testing.T) {
	// 段里混着 bgm name / base / hit table 这些非菜单键，
	// 不能被当成菜单项画出来。
	document := ParseIniDocument([]byte(`[title phase]

bgm name = bgm01
base = title_base
start = title_menu_start, 2, 10, 20
`))
	phase := parseTitlePhase(document)
	if len(phase.Entries) != 1 {
		t.Fatalf("%d entries, want 1", len(phase.Entries))
	}
	if phase.Entries[0].Key != "start" {
		t.Fatalf("entry key = %q", phase.Entries[0].Key)
	}
}

func TestParseTitlePhaseSpecialMenu(t *testing.T) {
	// YK/Yakata 的隐藏菜单：special 排在 op 前面，且用变量条件 @harem == 0xf
	// 控制是否出现。解析层只保存条件原文，求值在 session 层（能读脚本变量）。
	document := ParseIniDocument([]byte(`[title phase]
special = title_menu_sp,    2, 69, 362
op      = title_menu_opening, 2, 69, 405
start   = title_menu_start,   2, 69, 448

use special mode = 1
use conditions   = @harem == 0xf
`))

	phase := parseTitlePhase(document)
	if !phase.UseSpecialMode {
		t.Fatal("use special mode was not parsed")
	}
	if phase.UseConditions != "@harem == 0xf" {
		t.Fatalf("use conditions = %q", phase.UseConditions)
	}
	if len(phase.Entries) != 3 || phase.Entries[0].Key != "special" {
		t.Fatalf("entries = %+v", phase.Entries)
	}
	if phase.Entries[0].Image != "title_menu_sp" || phase.Entries[0].X != 69 || phase.Entries[0].Y != 362 {
		t.Fatalf("special entry = %+v", phase.Entries[0])
	}
}

func TestParseTitlePhaseMissingSection(t *testing.T) {
	phase := parseTitlePhase(ParseIniDocument([]byte("[main window]\nwidth = 1024\n")))
	if !phase.Empty() {
		t.Fatalf("expected an empty phase, got %d entries", len(phase.Entries))
	}
}
