package config

import "testing"

// dressScreenSampleCfg 是一份缩小的 bsxx.cfg，覆盖脱衣画面用到的各个段。
//
// 这块界面是"配置驱动"的：按钮图名、帧数、坐标、每个脱衣阶段对应哪张图
// 全部来自 bsxx.cfg。解析层把配置原样翻译成结构体，界面层才知道该画什么；
// 这里用一份手写的样本把翻译结果钉住，避免以后改解析逻辑时悄悄跑偏。
const dressScreenSampleCfg = `[bustup setting]
max strip = 7

[museum phase]
init ng level = 1

[dress screen]
enable in replay mode = 1
base = strip_base
base2 = strip_base2
mode menu type = 1
show bu = 1
enable view control = 1
show common button = 1

mode0 = strip_mode_one, 4, 77, 187
target0 = strip_ch01, 4, 28, 215
common state0 = strip_cstate0, 4, 28, 215
common state1 = strip_cstate1, 4, 77, 295
back button = strip_back, 3, 899, 5
whole body pos = 0, -20

dress00 = costume_allcos, 5, 844, 97
dress01 = costume_101, 5
dress02 = costume_102, 5
dress03 = costume_103, 5
dress pos = 844, 127
dress space = 0, 30

level1 = strip_cstate_s1000, 4, 844, 685
level2 = strip_cstate_s1001, 4, 844, 535
level3 = strip_cstate_s1010, 4, 844, 505
level4 = strip_cstate_s1012, 4, 844, 565

ngng convert table = 0, 1, 2, 3, 4, 5, 6
ch1 under wear = 5
ch2 under wear = 4
ch3 under wear = 3
ch4 under wear = 2
ch5 under wear = 1

ch01 dress01 = 1, 1, 3, 2, 4, 0, 0, 0, 1
ch01 dress02 = 2, 2, 8, 2, 0, 0, 9, 10, 1
ch01 dress03 = 3, 3, 11, 2, 0, 0, 0, 0, 1

[free-sight screen]
mode = 1
fixed position = 261+768, 615+768, 1036+768
meter region = 1003, 159, 1017, 718
knob = view_knob, 1, 1003, 159
face button = view_btn_face, 4, 886, 47
bust button = view_btn_bust, 4, 886, 83
hip button = view_btn_hip, 4, 886, 119
`

func TestParseDressScreenLayout(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)

	if screen.Empty() {
		t.Fatal("screen should not be empty")
	}
	if screen.Base != "strip_base" || screen.Base2 != "strip_base2" {
		t.Fatalf("base = %q, base2 = %q", screen.Base, screen.Base2)
	}

	// 左侧三类按钮各按自己的编号顺序收集，数量由配置决定。
	if len(screen.Modes) != 1 || len(screen.Targets) != 1 || len(screen.States) != 2 {
		t.Fatalf("modes/targets/states = %d/%d/%d", len(screen.Modes), len(screen.Targets), len(screen.States))
	}
	if screen.Modes[0].Image != "strip_mode_one" || screen.Modes[0].Frames != 4 {
		t.Fatalf("mode0 = %+v", screen.Modes[0])
	}
	if screen.Modes[0].X != 77 || screen.Modes[0].Y != 187 {
		t.Fatalf("mode0 pos = %d,%d", screen.Modes[0].X, screen.Modes[0].Y)
	}
	if screen.Targets[0].Image != "strip_ch01" {
		t.Fatalf("target0 = %q", screen.Targets[0].Image)
	}
	if screen.States[1].Image != "strip_cstate1" || screen.States[1].Y != 295 {
		t.Fatalf("common state1 = %+v", screen.States[1])
	}

	if screen.Back.Image != "strip_back" || screen.Back.Frames != 3 {
		t.Fatalf("back = %+v", screen.Back)
	}
	if screen.Back.X != 899 || screen.Back.Y != 5 {
		t.Fatalf("back pos = %d,%d", screen.Back.X, screen.Back.Y)
	}
	if screen.WholeBodyPos.X != 0 || screen.WholeBodyPos.Y != -20 {
		t.Fatalf("whole body pos = %+v", screen.WholeBodyPos)
	}

	// 服装列表的坐标不是每条各写一份，而是按 dress pos + 间距依次排开。
	if len(screen.Costumes) != 4 {
		t.Fatalf("%d costumes, want 4", len(screen.Costumes))
	}
	if screen.Costumes[0].Image != "costume_allcos" {
		t.Fatalf("costume0 = %q", screen.Costumes[0].Image)
	}
	if screen.Costumes[1].Image != "costume_101" {
		t.Fatalf("costume1 = %q", screen.Costumes[1].Image)
	}
	for index, button := range screen.Costumes {
		if button.X != 844 {
			t.Fatalf("costume %d x = %d", index, button.X)
		}
		if button.Y != 127+index*30 {
			t.Fatalf("costume %d y = %d", index, button.Y)
		}
	}
}

func TestParseDressScreenFlags(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)

	// 这些开关都是 0/1，非 0 即为真。
	if !screen.EnableInReplay {
		t.Fatal("enable in replay mode should be true")
	}
	if !screen.ShowBu {
		t.Fatal("show bu should be true")
	}
	if !screen.EnableViewControl {
		t.Fatal("enable view control should be true")
	}
	if !screen.ShowCommonButton {
		t.Fatal("show common button should be true")
	}
	if screen.ModeMenuType != 1 {
		t.Fatalf("mode menu type = %d", screen.ModeMenuType)
	}

	// 下着设定按角色顺序收集，下标 0 对应 1 号角色。
	if len(screen.UnderWear) != DressUnderWearCount {
		t.Fatalf("%d under wear entries, want %d", len(screen.UnderWear), DressUnderWearCount)
	}
	want := []int{5, 4, 3, 2, 1}
	for index, value := range want {
		if screen.UnderWear[index] != value {
			t.Fatalf("under wear %d = %d, want %d", index, screen.UnderWear[index], value)
		}
	}

	// 脱衣阶段的换算表原样保留。
	if len(screen.StripConvert) != 7 {
		t.Fatalf("%d strip convert entries, want 7", len(screen.StripConvert))
	}
	for index, value := range screen.StripConvert {
		if value != index {
			t.Fatalf("strip convert %d = %d", index, value)
		}
	}

	// max strip 来自 [bustup setting]，init ng level 来自 [museum phase]：
	// 它们虽然不在 [dress screen] 段里，但都属于这个画面的数据。
	if screen.MaxStrip != 7 {
		t.Fatalf("max strip = %d", screen.MaxStrip)
	}
	if screen.InitNgLevel != 1 {
		t.Fatalf("init ng level = %d", screen.InitNgLevel)
	}
}

func TestParseDressScreenCostumeTables(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)

	// 每条 chNN dressNN 表格就是"角色 × 服装"的脱衣阶段表。
	if len(screen.DressCostumes) != 3 {
		t.Fatalf("%d costume tables, want 3", len(screen.DressCostumes))
	}

	first := screen.DressCostumes[0]
	if first.Character != 1 || first.Wear != 1 || first.DressImage != 1 {
		t.Fatalf("first costume = %+v", first)
	}
	// 阶段编号原样存下来：第 i 个数字就是第 i 个脱衣阶段用的档位编号，
	// 0 表示这个阶段没有素材。
	wantStages := [DressStageCount]int{3, 2, 4, 0, 0, 0, 1}
	if first.Stages != wantStages {
		t.Fatalf("first stages = %v, want %v", first.Stages, wantStages)
	}

	second := screen.DressCostumes[1]
	if second.Wear != 2 || second.DressImage != 2 {
		t.Fatalf("second costume = %+v", second)
	}
	if second.Stages[0] != 8 || second.Stages[4] != 9 || second.Stages[5] != 10 {
		t.Fatalf("second stages = %v", second.Stages)
	}

	third := screen.DressCostumes[2]
	if third.Wear != 3 || third.DressImage != 3 {
		t.Fatalf("third costume = %+v", third)
	}
	if third.Stages[0] != 11 || third.Stages[1] != 2 {
		t.Fatalf("third stages = %v", third.Stages)
	}

	// 查表：角色与服装号都要对上。
	costume, ok := screen.CostumeFor(1, 1)
	if !ok {
		t.Fatal("costume (1,1) not found")
	}
	if costume.Stages[0] != 3 {
		t.Fatalf("costume (1,1) stages = %v", costume.Stages)
	}
	if _, ok := screen.CostumeFor(2, 1); ok {
		t.Fatal("costume (2,1) should not exist")
	}
	if _, ok := screen.CostumeFor(1, 9); ok {
		t.Fatal("costume (1,9) should not exist")
	}
}

func TestDressScreenStageLevels(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)
	// 阶段编号是 0 基的，和 common state0…6 对应。
	if level := screen.StageLevel(1, 1, 0); level != 3 {
		t.Fatalf("stage level (1,1,0) = %d", level)
	}
	if level := screen.StageLevel(1, 1, 2); level != 4 {
		t.Fatalf("stage level (1,1,2) = %d", level)
	}
	if level := screen.StageLevel(1, 1, 6); level != 1 {
		t.Fatalf("stage level (1,1,6) = %d", level)
	}
	if level := screen.StageLevel(1, 2, 4); level != 9 {
		t.Fatalf("stage level (1,2,4) = %d", level)
	}
	if level := screen.StageLevel(1, 2, 5); level != 10 {
		t.Fatalf("stage level (1,2,5) = %d", level)
	}

	// 这一件衣服在这个阶段没有素材（配置里写 0）：返回 0，界面据此跳过。
	if level := screen.StageLevel(1, 1, 3); level != 0 {
		t.Fatalf("stage level (1,1,3) = %d, want 0", level)
	}
	if screen.StageAvailable(1, 1, 2) != true {
		t.Fatal("stage (1,1,2) should be available")
	}
	if screen.StageAvailable(1, 1, 3) != false {
		t.Fatal("stage (1,1,3) should not be available")
	}

	// 越界的阶段号、别的角色的衣服都算"不可用"。
	if level := screen.StageLevel(1, 1, DressStageCount); level != 0 {
		t.Fatalf("stage level (1,1,7) = %d", level)
	}
	if level := screen.StageLevel(1, 1, -1); level != 0 {
		t.Fatalf("stage level (1,1,-1) = %d", level)
	}
	if level := screen.StageLevel(2, 1, 0); level != 0 {
		t.Fatalf("stage level (2,1,0) = %d", level)
	}
	if screen.StageAvailable(1, 1, DressStageCount) {
		t.Fatal("stage 7 should not be available")
	}
}

func TestDressScreenLevelButtons(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)

	if len(screen.Levels) != 4 {
		t.Fatalf("%d levels, want 4", len(screen.Levels))
	}
	for index, level := range screen.Levels {
		if level.Index != index+1 {
			t.Fatalf("level %d index = %d", index, level.Index)
		}
	}

	button, ok := screen.LevelButton(3)
	if !ok {
		t.Fatal("level3 not found")
	}
	if button.Image != "strip_cstate_s1010" || button.Frames != 4 {
		t.Fatalf("level3 = %+v", button)
	}
	if button.X != 844 || button.Y != 505 {
		t.Fatalf("level3 pos = %d,%d", button.X, button.Y)
	}
	if _, ok := screen.LevelButton(9); ok {
		t.Fatal("level9 should not exist")
	}
	if _, ok := screen.LevelButton(0); ok {
		t.Fatal("level0 should not exist")
	}
}

func TestParseDressScreenAllCostumeEntry(t *testing.T) {
	// wear 为 0 的条目是"全部服装"这一共用条目：它对所有角色都成立，
	// 选中它表示"所有服装一起看"，所以按角色查也查得到。
	document := ParseIniDocument([]byte(`[dress screen]

dress00 = costume_allcos, 5, 844, 97
ch01 dress12 = 0, 0, 3, 2, 4, 5, 6, 7, 1
`))
	screen := parseDressScreen(document)

	if len(screen.DressCostumes) != 1 {
		t.Fatalf("%d costume tables, want 1", len(screen.DressCostumes))
	}
	entry := screen.DressCostumes[0]
	if entry.Wear != 0 || entry.DressImage != 0 {
		t.Fatalf("all costume entry = %+v", entry)
	}
	if entry.Stages != [DressStageCount]int{3, 2, 4, 5, 6, 7, 1} {
		t.Fatalf("all costume stages = %v", entry.Stages)
	}

	for _, character := range []int{1, 2, 5} {
		costume, ok := screen.CostumeFor(character, 0)
		if !ok {
			t.Fatalf("all costume entry not found for character %d", character)
		}
		if costume.Wear != 0 {
			t.Fatalf("costume for character %d = %+v", character, costume)
		}
	}
	if _, ok := screen.CostumeFor(1, 5); ok {
		t.Fatal("costume (1,5) should not exist")
	}
}

func TestParseDressScreenPerCharacterAllCostume(t *testing.T) {
	// 每个角色各有一条 wear 为 0 的"全部服装"条目，但阶段表各不相同：
	// 查某个角色时要拿它自己那一条，不能拿别的角色的。
	document := ParseIniDocument([]byte(`[dress screen]

ch01 dress12 = 0, 0, 3, 2, 4, 5, 6, 7, 1
ch02 dress12 = 0, 0, 21, 20, 22, 23, 24, 25, 19
`))
	screen := parseDressScreen(document)

	if len(screen.DressCostumes) != 2 {
		t.Fatalf("%d costume tables, want 2", len(screen.DressCostumes))
	}

	first, ok := screen.CostumeFor(1, 0)
	if !ok {
		t.Fatal("all costume entry for character 1 not found")
	}
	if first.Character != 1 || first.Stages[0] != 3 {
		t.Fatalf("character 1 all costume = %+v", first)
	}

	second, ok := screen.CostumeFor(2, 0)
	if !ok {
		t.Fatal("all costume entry for character 2 not found")
	}
	if second.Character != 2 || second.Stages[0] != 21 {
		t.Fatalf("character 2 all costume = %+v", second)
	}

	// 没有共用条目时退回任意一条 wear 为 0 的条目。
	fallback, ok := screen.CostumeFor(5, 0)
	if !ok {
		t.Fatal("all costume entry not found for character 5")
	}
	if fallback.Wear != 0 {
		t.Fatalf("character 5 all costume = %+v", fallback)
	}
}

func TestParseDressScreenDefaults(t *testing.T) {
	// 没有 [dress screen] 段时也要给出能用的默认值：
	// 阶段数错会让整张阶段表错位，比少几个阶段严重得多。
	screen := parseDressScreen(EmptyIniDocument())

	if !screen.Empty() {
		t.Fatal("screen should be empty")
	}
	if screen.MaxStrip != DressDefaultMaxStrip {
		t.Fatalf("max strip = %d", screen.MaxStrip)
	}
	if len(screen.StripConvert) != DressDefaultMaxStrip {
		t.Fatalf("%d strip convert entries, want %d", len(screen.StripConvert), DressDefaultMaxStrip)
	}
	for index, value := range screen.StripConvert {
		if value != index {
			t.Fatalf("strip convert %d = %d", index, value)
		}
	}
	if len(screen.UnderWear) != DressUnderWearCount {
		t.Fatalf("%d under wear entries, want %d", len(screen.UnderWear), DressUnderWearCount)
	}
	if len(screen.DressCostumes) != 0 || len(screen.Levels) != 0 {
		t.Fatal("costumes and levels should be empty")
	}
	if screen.EnableInReplay || screen.ShowBu || screen.EnableViewControl || screen.ShowCommonButton {
		t.Fatal("flags should be false")
	}
}

func TestParseDressScreenMaxStripRange(t *testing.T) {
	// max strip 只接受 1～9，越界的按默认值处理。
	document := ParseIniDocument([]byte(`[bustup setting]
max strip = 20

[dress screen]
`))
	if strip := parseDressScreen(document).MaxStrip; strip != DressDefaultMaxStrip {
		t.Fatalf("max strip = %d, want %d", strip, DressDefaultMaxStrip)
	}

	document = ParseIniDocument([]byte(`[bustup setting]
max strip = 3

[dress screen]
`))
	screen := parseDressScreen(document)
	if screen.MaxStrip != 3 {
		t.Fatalf("max strip = %d", screen.MaxStrip)
	}
	// 换算表没写时用恒等映射，条数跟着 max strip 走。
	if len(screen.StripConvert) != 3 {
		t.Fatalf("%d strip convert entries, want 3", len(screen.StripConvert))
	}
}

func TestParseDressScreenViewControl(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	screen := parseDressScreen(document)

	// 视角操作控件在 [dress screen] 里没写时沿用 [free-sight screen] 的：
	// 本来就是同一套控件。
	if !screen.View.Enabled {
		t.Fatal("view control should be enabled")
	}
	if screen.View.MeterX != 1003 || screen.View.MeterY != 159 {
		t.Fatalf("meter pos = %d,%d", screen.View.MeterX, screen.View.MeterY)
	}
	// meter region 写的是"左, 上, 右, 下"，换算成尺寸后是 14 x 559。
	if screen.View.MeterW != 14 || screen.View.MeterH != 559 {
		t.Fatalf("meter size = %dx%d", screen.View.MeterW, screen.View.MeterH)
	}
	if screen.View.Knob.Image != "view_knob" || screen.View.Knob.Frames != 1 {
		t.Fatalf("knob = %+v", screen.View.Knob)
	}
	if screen.View.Knob.X != 1003 || screen.View.Knob.Y != 159 {
		t.Fatalf("knob pos = %d,%d", screen.View.Knob.X, screen.View.Knob.Y)
	}
	if screen.View.Face.Image != "view_btn_face" || screen.View.Face.Frames != 4 {
		t.Fatalf("face = %+v", screen.View.Face)
	}
	if screen.View.Face.X != 886 || screen.View.Face.Y != 47 {
		t.Fatalf("face pos = %d,%d", screen.View.Face.X, screen.View.Face.Y)
	}
	if screen.View.Bust.Image != "view_btn_bust" {
		t.Fatalf("bust = %q", screen.View.Bust.Image)
	}
	if screen.View.Hip.Image != "view_btn_hip" {
		t.Fatalf("hip = %q", screen.View.Hip.Image)
	}
}

func TestParseFreeSightScreen(t *testing.T) {
	document := ParseIniDocument([]byte(dressScreenSampleCfg))
	configuration := newBsxxConfiguration(document)

	sight := configuration.FreeSight
	if !sight.Mode {
		t.Fatal("free sight mode should be true")
	}
	// fixed position 写的是算术表达式，这里已经算成实际坐标。
	want := []int{261 + 768, 615 + 768, 1036 + 768}
	if len(sight.FixedPositions) != len(want) {
		t.Fatalf("%d fixed positions, want %d", len(sight.FixedPositions), len(want))
	}
	for index, value := range want {
		if sight.FixedPositions[index] != value {
			t.Fatalf("fixed position %d = %d, want %d", index, sight.FixedPositions[index], value)
		}
	}
	// meter region 写的是"左, 上, 右, 下"，这里换算成左上角与尺寸。
	if sight.Meter != [4]int{1003, 159, 14, 559} {
		t.Fatalf("meter = %v", sight.Meter)
	}
	if sight.Knob.Image != "view_knob" || sight.Knob.X != 1003 || sight.Knob.Y != 159 {
		t.Fatalf("knob = %+v", sight.Knob)
	}
	if sight.Face.Image != "view_btn_face" || sight.Face.Frames != 4 {
		t.Fatalf("face = %+v", sight.Face)
	}
	if sight.Bust.Image != "view_btn_bust" || sight.Bust.X != 886 || sight.Bust.Y != 83 {
		t.Fatalf("bust = %+v", sight.Bust)
	}
	if sight.Hip.Image != "view_btn_hip" || sight.Hip.X != 886 || sight.Hip.Y != 119 {
		t.Fatalf("hip = %+v", sight.Hip)
	}

	// 老字段保留：FreeSightMode / FixedPositions 是同一份数据，
	// 已经在用它们的地方不能因为新结构而失效。
	if !configuration.FreeSightMode {
		t.Fatal("legacy free sight mode should be true")
	}
	if len(configuration.FixedPositions) != len(want) {
		t.Fatalf("%d legacy fixed positions, want %d", len(configuration.FixedPositions), len(want))
	}
	for index, value := range want {
		if configuration.FixedPositions[index] != value {
			t.Fatalf("legacy fixed position %d = %d", index, configuration.FixedPositions[index])
		}
	}
}

// SL / SLIE 的 common state 编号是 0,1,2,3,5（没有 4），服装表只写 5 个值。
// 解析时要把第 5 个值放到阶段 5 上，否则最后一级永远选不到，立绘名也会
// 拼成不存在的 buNNu4...。
func TestDressScreenStageGap(t *testing.T) {
	document := ParseIniDocument([]byte(`[bustup setting]
max strip = 7

[museum phase]
init ng level = 1

[dress screen]
base = strip_base
base2 = strip_base2
show bu = 1
show common button = 1
mode0 = strip_mode_one, 4, 12, 262
target0 = strip_ch1, 4, 12, 286
common state0 = strip_cstate0, 4, 12, 286
common state1 = strip_cstate1, 4, 12, 338
common state2 = strip_cstate2, 4, 12, 390
common state3 = strip_cstate3, 4, 12, 442
common state5 = strip_cstate5, 4, 12, 494
back button = strip_back, 3, 992, 544
dress00 = costume_allcos, 5, 200, 392
dress01 = costume_1, 5
dress pos = 200, 150
dress space = 0, 34
level1 = strip_cstate0, 4, 672, 158
level2 = strip_cstate1, 4, 672, 210
level5 = strip_cstate5, 4, 672, 262
ch01 dress01 = 1, 1, 1, 2, 8, 4, 5
`))
	screen := parseDressScreen(document)

	// 空档必须保留：States[4] 是空占位，States[5] 才是"下着"。
	if len(screen.States) != 6 {
		t.Fatalf("states = %d, want 6", len(screen.States))
	}
	if !screen.States[4].Empty() {
		t.Fatalf("state4 should be a gap: %+v", screen.States[4])
	}
	if screen.States[5].Image != "strip_cstate5" {
		t.Fatalf("state5 = %+v", screen.States[5])
	}
	costume, ok := screen.CostumeFor(1, 1)
	if !ok {
		t.Fatal("costume (1,1) not found")
	}
	want := [DressStageCount]int{1, 2, 8, 4, 0, 5, 0}
	if costume.Stages != want {
		t.Fatalf("stages = %v, want %v", costume.Stages, want)
	}
	if !screen.StageAvailable(1, 1, 5) || screen.StageLevel(1, 1, 5) != 5 {
		t.Fatalf("stage 5 should be available with level 5")
	}
}
