package config

import "testing"

// SL 的 [custom map phase] 由引擎补画 START；这里确认坐标/帧数原样保留。
func TestParseCustomMapPhaseStart(t *testing.T) {
	document := ParseIniDocumentText(`
[custom map phase]
result = #select_value
type = #custom_map_type
start = work_start, 4, 128, 528
`)
	phase := parseCustomMapPhase(document)
	if phase.Empty() {
		t.Fatal("start button should be parsed")
	}
	if phase.Start.Image != "work_start" || phase.Start.Frames != 4 ||
		phase.Start.X != 128 || phase.Start.Y != 528 {
		t.Fatalf("start = %+v", phase.Start)
	}
}

// SLIE 的剧本自己画 START，配置里没有 start；此时引擎不应补任何按钮。
func TestParseCustomMapPhaseWithoutStart(t *testing.T) {
	document := ParseIniDocumentText(`
[custom map phase]
result = #select_value
no auto save = 1
`)
	if phase := parseCustomMapPhase(document); !phase.Empty() {
		t.Fatalf("phase = %+v, want empty", phase)
	}
}

// 工作画面的六格按钮用 `240 + 128 * (i % 2)` 排两列；坐标解析器
// 必须支持 %，否则 3 号之后的按钮会横跨到相邻角色面板上。
func TestParseCoordinateModulo(t *testing.T) {
	cases := map[string]int{
		"240 + 128 * ( 0 % 2 )": 240,
		"240 + 128 * ( 3 % 2 )": 368,
		"240 + 128 * ( 4 % 2 )": 240,
		"80 + 52 * ( 4 / 2 )":   184,
	}
	for expression, want := range cases {
		if got := parseCoordinate(expression); got != want {
			t.Fatalf("parseCoordinate(%q) = %d, want %d", expression, got, want)
		}
	}
}

// OH/OHIE 的地图卡把"按钮 ID/帧数"写在 [button object]，结果值由
// [custom map phase] result table 按 ID 查表。单列 OH 写法必须当帧数，
// 双列 OHIE 写法第一列是 ID；时间推进按钮的 ID 6 要映射回 #result=0。
func TestCustomMapResultTableAndFrames(t *testing.T) {
	document := ParseIniDocumentText(`
[custom map phase]
result = #result
result table = -1, 1, 2, 3, 4, 5, 0

[button object]
map_ch1_norm = 5
map_ch1_norm on = 4, 10000
map_nexttime = 6, 2
map_nexttime on = 1, 10000
`)
	configuration := newBsxxConfiguration(document)
	if !configuration.CustomMap.HasResultTable() {
		t.Fatal("result table should be parsed")
	}
	if value, ok := configuration.CustomMap.ResultValue(6); !ok || value != 0 {
		t.Fatalf("result value for id 6 = %d ok=%v", value, ok)
	}
	if frames := configuration.MapButtonTableFrames("map_ch1_norm"); frames != 5 {
		t.Fatalf("single-column frames = %d", frames)
	}
	if id, ok := configuration.MapButtonResultID("map_ch1_norm"); ok || id != 0 {
		t.Fatalf("single-column button should have no explicit id, got %d ok=%v", id, ok)
	}
	if hover := configuration.MapButtonHoverFrame("map_ch1_norm"); hover != 4 {
		t.Fatalf("hover frame = %d", hover)
	}
	// 双列写法：ID=6、帧数=2。
	if id, ok := configuration.MapButtonResultID("map_nexttime"); !ok || id != 6 {
		t.Fatalf("explicit id = %d ok=%v", id, ok)
	}
	if frames := configuration.MapButtonTableFrames("map_nexttime"); frames != 2 {
		t.Fatalf("double-column frames = %d", frames)
	}
	// OH 单列没有显式 ID：热卡模式只登记"今天有事件"的角色，
	// 不能按登记序号编号，必须从图名取角色槽位。
	if id, ok := configuration.MapButtonNamedID("map_ch2_hot"); !ok || id != 2 {
		t.Fatalf("named card id = %d ok=%v", id, ok)
	}
	if id, ok := configuration.MapButtonNamedID("map_nexttime"); !ok || id != 6 {
		t.Fatalf("named nexttime id = %d ok=%v", id, ok)
	}
}
