package config

import "testing"

// chartDatabaseSample 摘自 YKTIEDL 的 define.bsa:chart.db，只做了裁剪：
// 第一页保留四种格子和两条连线（一条直线、一条带折点），第二页留一个格子。
// 坐标原样保留，用来验证 1024 空间的 line 数据确实和原生坐标的 block 分开搬运。
const chartDatabaseSample = `{
	page: [
		{
			block: [
				{ id:0x01000000, pos:[ 800,  66], label:"L00_001", property:{ "thumb":["ev00_001_1p00","ev00_001_1f300"] } },
				{ id:0x01000300, pos:[ 800, 386], label:"L05_101", property:{ "thumb":["bg01004bb","bu01f0011a02,341","bu02f0011b10,-341"] } },
				{ id:0x01000700, pos:[ 800, 813], label:"L05_102", property:{ "thumb":"bg01004ab" } },
				{ id:0x01002c00, pos:[ 533,2547], label:"@1", property:{ "thumb":"*chart_next_block" } }
			],
			line: [
				{ id:0x02000000, source:{ id:0x01000005, pos:[ 600,  74] }, destination:{ id:0x01000301, pos:[ 600, 306] } },
				{ id:0x02004400, source:{ id:0x01004203, pos:[1261, 830] }, destination:{ id:0x01003e03, pos:[1270, 630] }, points:[[1934,1107],[1934, 840]] }
			]
		},
		{
			block: [
				{ id:0x01000000, pos:[ 667,  66], label:"L05_201", property:{ "thumb":"bg01003ab" } }
			],
			line: []
		}
	]
}`

func TestParseChartDatabase(t *testing.T) {
	database, err := ParseChartDatabase([]byte(chartDatabaseSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(database.Pages) != 2 {
		t.Fatalf("pages = %d", len(database.Pages))
	}

	page := database.Pages[0]
	if len(page.Blocks) != 4 {
		t.Fatalf("blocks = %d", len(page.Blocks))
	}
	first := page.Blocks[0]
	if first.ID != 0x01000000 || first.X != 800 || first.Y != 66 || first.Label != "L00_001" {
		t.Fatalf("block 0 = %+v", first)
	}
	if len(first.Thumbs) != 2 || first.Thumbs[1].Name != "ev00_001_1f300" || first.Thumbs[1].OffsetX != 0 {
		t.Fatalf("thumbs = %+v", first.Thumbs)
	}

	layered := page.Blocks[1].Thumbs
	if len(layered) != 3 {
		t.Fatalf("layered thumbs = %+v", layered)
	}
	if layered[1].Name != "bu01f0011a02" || layered[1].OffsetX != 341 {
		t.Fatalf("right thumb = %+v", layered[1])
	}
	if layered[2].Name != "bu02f0011b10" || layered[2].OffsetX != -341 {
		t.Fatalf("left thumb = %+v", layered[2])
	}

	single := page.Blocks[2].Thumbs
	if len(single) != 1 || single[0].Name != "bg01004ab" || single[0].Special {
		t.Fatalf("single thumb = %+v", single)
	}
	special := page.Blocks[3].Thumbs
	if len(special) != 1 || special[0].Name != "chart_next_block" || !special[0].Special {
		t.Fatalf("special thumb = %+v", special)
	}

	if len(page.Lines) != 2 {
		t.Fatalf("lines = %d", len(page.Lines))
	}
	straight := page.Lines[0]
	if straight.ID != 0x02000000 || straight.Source.ID != 0x01000005 ||
		straight.Source.X != 600 || straight.Source.Y != 74 {
		t.Fatalf("line 0 source = %+v", straight)
	}
	if straight.Destination.ID != 0x01000301 || straight.Destination.X != 600 ||
		straight.Destination.Y != 306 {
		t.Fatalf("line 0 destination = %+v", straight.Destination)
	}
	if len(straight.Points) != 0 {
		t.Fatalf("line 0 points = %+v", straight.Points)
	}

	bent := page.Lines[1]
	if bent.Source.X != 1261 || bent.Source.Y != 830 ||
		bent.Destination.X != 1270 || bent.Destination.Y != 630 {
		t.Fatalf("line 1 = %+v", bent)
	}
	if len(bent.Points) != 2 || bent.Points[0].X != 1934 || bent.Points[0].Y != 1107 ||
		bent.Points[1].X != 1934 || bent.Points[1].Y != 840 {
		t.Fatalf("line 1 points = %+v", bent.Points)
	}

	second := database.Pages[1]
	if len(second.Blocks) != 1 || second.Blocks[0].Label != "L05_201" || len(second.Lines) != 0 {
		t.Fatalf("page 1 = %+v", second)
	}
}

// statusChartSample 是 YKTIEDL status_layout 里 chart 页的裁剪版：只留一个
// chart 页，用来验证"没有 slot 的页面不会被丢掉"，以及按钮/确认框/缩略图比例。
const statusChartSample = `{
	base: "status_base"

	layout: {
		chart: {
			draw_area: [213, 0, 1366, 768]
			base: "chart_base"
			loading: { name:"chart_loading", max_state:12, pos:[0,0] }
			thumb_ratio: 0.094

			scene_name: { pos:[1366 - 16, 16], align:"right", edge:0xff886622 }

			button: [
				{ name:"chart_block_a",   max_state:4, pos:[  7,  18] }
				{ name:"chart_block_b",   max_state:4, pos:[114,  61] }
				{ name:"chart_now_scene", max_state:3, pos:[  6, 657] }
			]

			confirm: {
				base:   { name:"chart_confirm_base" }
				ok:     { name:"resave_ok",     max_state:3, pos:[662, 365] }
				cancel: { name:"resave_cancel", max_state:3, pos:[833, 365] }
			}
		}
	}
}`

func TestParseStatusChart(t *testing.T) {
	layout, err := ParseStatusScreen([]byte(statusChartSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Pages) != 1 || layout.Pages[0].Key != "chart" || layout.Pages[0].Base != "chart_base" {
		t.Fatalf("pages = %+v", layout.Pages)
	}

	chart := layout.Pages[0].Chart
	if chart == nil {
		t.Fatal("chart page was dropped")
	}
	if chart.DrawArea != [4]int{213, 0, 1366, 768} {
		t.Fatalf("draw area = %v", chart.DrawArea)
	}
	if chart.Loading.Name != "chart_loading" || chart.Loading.MaxState != 12 ||
		chart.Loading.X != 0 || chart.Loading.Y != 0 {
		t.Fatalf("loading = %+v", chart.Loading)
	}
	if chart.ThumbRatio != 0.094 {
		t.Fatalf("thumb ratio = %v", chart.ThumbRatio)
	}
	if chart.SceneName.X != 1350 || chart.SceneName.Y != 16 ||
		chart.SceneName.Align != "right" || chart.SceneName.Edge != 0xff886622 {
		t.Fatalf("scene name = %+v", chart.SceneName)
	}

	if len(chart.Buttons) != 3 {
		t.Fatalf("buttons = %+v", chart.Buttons)
	}
	if button := chart.Buttons[1]; button.Name != "chart_block_b" || button.MaxState != 4 ||
		button.X != 114 || button.Y != 61 {
		t.Fatalf("button 1 = %+v", button)
	}
	// now_scene 既要在 Buttons 里（按顺序画），也要单独给一份方便跳转逻辑取用。
	if chart.NowScene.Name != "chart_now_scene" || chart.NowScene.MaxState != 3 ||
		chart.NowScene.X != 6 || chart.NowScene.Y != 657 {
		t.Fatalf("now scene = %+v", chart.NowScene)
	}
	if chart.Buttons[2] != chart.NowScene {
		t.Fatalf("now scene missing from buttons: %+v", chart.Buttons)
	}

	if chart.Confirm.Base.Name != "chart_confirm_base" || chart.Confirm.Base.X != 0 {
		t.Fatalf("confirm base = %+v", chart.Confirm.Base)
	}
	if chart.Confirm.OK.Name != "resave_ok" || chart.Confirm.OK.MaxState != 3 ||
		chart.Confirm.OK.X != 662 || chart.Confirm.OK.Y != 365 {
		t.Fatalf("confirm ok = %+v", chart.Confirm.OK)
	}
	if chart.Confirm.Cancel.Name != "resave_cancel" || chart.Confirm.Cancel.X != 833 {
		t.Fatalf("confirm cancel = %+v", chart.Confirm.Cancel)
	}
}

// TestParseStatusChartDefaults 覆盖两种书写差异：thumb_ratio 以 '.' 开头时
// 词法器返回字符串 ".094"；整页只有 base、没有 thumb_ratio 时沿用原引擎默认值。
func TestParseStatusChartDefaults(t *testing.T) {
	layout, err := ParseStatusScreen([]byte(`{
		layout: {
			chart: {
				base: "chart_base"
				thumb_ratio: .094
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	chart := layout.Pages[0].Chart
	if chart == nil {
		t.Fatal("chart page was dropped")
	}
	if chart.ThumbRatio != 0.094 {
		t.Fatalf("thumb ratio = %v", chart.ThumbRatio)
	}

	layout, err = ParseStatusScreen([]byte(`{
		layout: {
			chart: { base: "chart_base" }
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	chart = layout.Pages[0].Chart
	if chart == nil {
		t.Fatal("chart page was dropped")
	}
	if chart.ThumbRatio != defaultChartThumbRatio {
		t.Fatalf("default thumb ratio = %v", chart.ThumbRatio)
	}
	if len(chart.Buttons) != 0 || chart.NowScene.Name != "" {
		t.Fatalf("chart = %+v", chart)
	}
}
