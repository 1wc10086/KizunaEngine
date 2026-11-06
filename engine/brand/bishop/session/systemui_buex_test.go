package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
)

// systemui_buex_test.go 覆盖立ち絵鑑賞室里那些"算出来的"逻辑：
// 立绘图名的拼法、六组开关各自改什么、排序按钮怎么挪角色、
// 站立位置滑杆怎么换算、距离两档的顺序、预设文件的新旧格式、滚轮怎么翻页。
//
// 布局照着 define.bsa 的 museum_buex_layout 摆一套等价的数据，
// 素材一律取不到（资源索引是空的），这样绘制路径不会误伤测试。

// buexTestLayout 造一份和真实布局等价的鉴赏室布局。
func buexTestLayout() *config.BuexLayout {
	layout := &config.BuexLayout{}
	layout.Menu = []config.BuexMenuEntry{
		{Key: "setting_bgv", Button: config.ConfigButton{Name: "buex_main_bgv", MaxState: 4, X: 424, Y: 5}},
		{Key: "bg_select", Button: config.ConfigButton{Name: "buex_main_changebg", MaxState: 4, X: 304, Y: 5}},
		{Key: "setting_bu", Button: config.ConfigButton{Name: "buex_main_bureplace", MaxState: 4, X: 784, Y: 5}},
		{Key: "edit_message", Button: config.ConfigButton{Name: "buex_main_setmsg", MaxState: 4, X: 544, Y: 5}},
		{Key: "data_store", Button: config.ConfigButton{Name: "buex_main_save", MaxState: 4, X: 664, Y: 5}},
		{Key: "back_to_thumbs", Button: config.ConfigButton{Name: "buex_main_back", MaxState: 3, X: 904, Y: 5}},
	}
	layout.Bg.Base = "buex_bgbase"
	layout.Bg.Subject = config.UIPoint{X: 2, Y: -15}
	layout.Bg.PageGroups = [][]config.ConfigButton{
		{{Name: "museum_page41", MaxState: 4, X: 10, Y: 97}, {Name: "museum_page42", MaxState: 4, X: 10, Y: 124}},
		{{Name: "museum_page43", MaxState: 4, X: 10, Y: 189}},
	}
	layout.Bg.ThumbPos = []config.UIPoint{{X: 215, Y: 94}, {X: 350, Y: 94}}

	layout.Setting.Base = "buex_bubase"
	layout.Setting.Buttons = map[string]config.ConfigButton{
		"order_up":      {Name: "buex_order_up", MaxState: 3, X: 597, Y: 644},
		"order_down":    {Name: "buex_order_down", MaxState: 3, X: 970, Y: 644},
		"on_off_button": {Name: "dress_view_appearance", MaxState: 4, X: 2, Y: 63},
		"pos_pointer":   {Name: "buex_view_pointer", MaxState: 2, X: 509, Y: 743},
		"switch":        {Name: "buex_view_switch", MaxState: 3},
		"hair_mode":     {Name: "buex_view_hair", MaxState: 5, X: 33, Y: 674},
		"cutin_mode":    {Name: "buex_view_cutin", MaxState: 5, X: 33, Y: 704},
		"steam_mode":    {Name: "buex_view_steam", MaxState: 5, X: 33, Y: 644},
	}
	layout.Setting.PosRegion = [4]int{0, 743, 1019, 25}
	layout.Setting.Switches = []config.BuexSwitchGroup{
		{X: 290, Y: 647, SpaceX: 17, SpaceY: 0, Count: 2},
		{X: 195, Y: 647, SpaceX: 17, SpaceY: 0, Count: 2},
		{X: 224, Y: 707, SpaceX: 17, SpaceY: 0, Count: 7},
		{X: 195, Y: 677, SpaceX: 17, SpaceY: 0, Count: 11},
		{X: 429, Y: 677, SpaceX: 17, SpaceY: 0, Count: 5},
		// 表情组：和多排布局一样写了行距（SLIE 是 y=378/398/418 三行），
		// 颗数才按角色表情表铺开；Count=0 是新版布局的写法。
		{X: 372, Y: 647, SpaceX: 17, SpaceY: 20, Count: 0},
	}
	layout.Setting.StripConvert = []int{0, 1, 2, 3, 4, 5, 6}
	layout.Setting.Characters = []config.BuexCharacter{
		{
			Icon:      config.ConfigButton{Name: "dress_view_ch1", MaxState: 3, X: 620, Y: 644},
			Wear:      []int{1, 2, 3},
			UnderWear: 5,
			Faces:     [][]int{{1, 2, 3, 4}, {13, 14, 15, 16}},
		},
		{
			Icon:      config.ConfigButton{Name: "dress_view_ch2", MaxState: 3, X: 690, Y: 644},
			Wear:      []int{1, 2},
			UnderWear: 5,
			Faces:     [][]int{{1, 2}},
		},
	}

	layout.Edit.Base = "buex_embase"
	layout.Edit.SubMenu = []config.ConfigButton{
		{Name: "buex_sub_1", MaxState: 4, X: 784, Y: 35},
		{Name: "buex_sub_2", MaxState: 4, X: 784, Y: 62},
	}
	layout.Edit.Commands = map[string]config.ConfigButton{
		"fontsize":    {Name: "buex_em_fontsize", MaxState: 3},
		"fontcolor":   {Name: "buex_em_fontcolor", MaxState: 3},
		"typesetting": {Name: "buex_em_typesetting", MaxState: 3},
		"balloon":     {Name: "buex_em_balloon", MaxState: 3},
		"delete":      {Name: "buex_em_delete", MaxState: 3},
	}
	layout.Edit.Picker = config.BuexColorPicker{
		Base:    "color_picker_base",
		Radio:   "color_picker_radio",
		Targets: []config.UIPoint{{X: 200, Y: 50}, {X: 345, Y: 50}},
		Close:   config.ConfigButton{Name: "color_picker_close", MaxState: 2, X: 461, Y: 5},
		Bars: []config.BuexColorBar{
			{Bar: config.ConfigButton{Name: "cfgx_red_bar", X: 209, Y: 81}},
			{Bar: config.ConfigButton{Name: "cfgx_green_bar", X: 209, Y: 111}},
			{Bar: config.ConfigButton{Name: "cfgx_blue_bar", X: 209, Y: 141}},
			{Bar: config.ConfigButton{Name: "cfgx_concentration_bar", X: 209, Y: 171}},
		},
	}

	layout.Store.Base = "buex_dsbase"
	layout.Store.Commands = map[string]config.ConfigButton{
		"save":   {Name: "buex_ds_cmd_save", MaxState: 3},
		"load":   {Name: "buex_ds_cmd_load", MaxState: 3},
		"toslot": {Name: "buex_ds_cmd_toslot", MaxState: 3},
		"output": {Name: "buex_ds_output", MaxState: 3, X: 42, Y: 504},
		"clear":  {Name: "buex_ds_clear", MaxState: 3, X: 42, Y: 554},
	}
	layout.Store.ThumbPos = []config.UIPoint{{X: 215, Y: 94}, {X: 350, Y: 94}}
	layout.Store.SlotPos = []config.UIPoint{{X: 43, Y: 94}}
	return layout
}

// newBuexTestSession 造一个打开着鉴赏室的会话。
func newBuexTestSession(t *testing.T) *Session {
	t.Helper()
	resources := resource.Build(t.TempDir())
	composer := rendering.NewSceneComposer(resources, 1024, 768, 0, 0, nil,
		config.InformationWindow{}, config.GameMenu{}, config.MessageWindow{},
		config.FontSettings{Size: 24})
	s := &Session{
		config:   &config.BsxxConfiguration{Dress: config.DressScreen{MaxStrip: 7}},
		res:      resources,
		composer: composer,
		width:    1024,
		height:   768,
		dataDir:  t.TempDir(),
	}
	s.ui = &systemUI{kind: uiMuseumBU, hover: uiHit{kind: hitNone, index: -1}}
	s.ui.buex = s.buexNewState(buexTestLayout())
	return s
}

func TestBuexStandeeFileName(t *testing.T) {
	// bu<NN><视角><脱衣阶段><服装两位><姿势一位><变体>[表情两位]
	if got := buexStandeeFileName(1, "u", 0, 1, 1, "a", 0); got != "bu01u0011a" {
		t.Fatalf("name = %q, want bu01u0011a", got)
	}
	if got := buexStandeeFileName(1, "u", 0, 1, 1, "a", 12); got != "bu01u0011a12" {
		t.Fatalf("name = %q, want bu01u0011a12", got)
	}
	if got := buexStandeeFileName(12, "f", 3, 11, 4, "b", 0); got != "bu12f3114b" {
		t.Fatalf("name = %q, want bu12f3114b", got)
	}
}

func TestBuexSwitchActions(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex

	// ポーズ：a / b。
	s.buexSetSwitch(state, buexSwitchPose, 1)
	if state.chars[0].pose != "b" {
		t.Fatalf("pose = %q, want b", state.chars[0].pose)
	}
	s.buexSetSwitch(state, buexSwitchPose, 0)
	if state.chars[0].pose != "a" {
		t.Fatalf("pose = %q, want a", state.chars[0].pose)
	}

	// 距離：0 = 半身，1 = 放大半身（更近）；原版没有全身档。
	s.buexSetSwitch(state, buexSwitchView, 1)
	if state.chars[0].view != buexViewClose {
		t.Fatalf("view = %d, want close", state.chars[0].view)
	}
	s.buexSetSwitch(state, buexSwitchView, 0)
	if state.chars[0].view != buexViewBust {
		t.Fatalf("view = %d, want bust", state.chars[0].view)
	}

	// ぬぎぬぎ：查换算表。
	s.buexSetSwitch(state, buexSwitchNgng, 4)
	if state.chars[0].strip != 4 {
		t.Fatalf("strip = %d, want 4", state.chars[0].strip)
	}

	// 服装：取角色服装表里的编号。
	s.buexSetSwitch(state, buexSwitchWear, 2)
	if state.chars[0].wear != 3 {
		t.Fatalf("wear = %d, want 3", state.chars[0].wear)
	}

	// 下着：档位 2 → 脱衣阶段跟着到 min(2+2, MaxStrip-1) = 4。
	s.buexSetSwitch(state, buexSwitchUnder, 2)
	if state.chars[0].under != 2 || state.chars[0].strip != 4 {
		t.Fatalf("under/strip = %d/%d, want 2/4", state.chars[0].under, state.chars[0].strip)
	}

	// 表情：多行表情组按姿势分页。第 0 颗是"不要表情"，
	// 其余对应当前姿势那一页的表情（1 号角色的 a 页是 1..4）。
	s.buexSetSwitch(state, buexSwitchFace, 0)
	if state.chars[0].face != 0 {
		t.Fatalf("face = %d, want 0", state.chars[0].face)
	}
	s.buexSetSwitch(state, buexSwitchFace, 2)
	if state.chars[0].face != 2 || state.chars[0].pose != "a" {
		t.Fatalf("face/pose = %d/%q, want 2/a", state.chars[0].face, state.chars[0].pose)
	}
	// 换姿势后原来那张脸不在新的一页上，表情要清掉。
	s.buexSetSwitch(state, buexSwitchFace, 4)
	if state.chars[0].face != 4 {
		t.Fatalf("face = %d, want 4", state.chars[0].face)
	}
	s.buexSetSwitch(state, buexSwitchPose, 1)
	if state.chars[0].pose != "b" || state.chars[0].face != 0 {
		t.Fatalf("pose/face = %q/%d, want b/0", state.chars[0].pose, state.chars[0].face)
	}
	// b 页的表情表是 13..16，第 2 颗对应 14；选表情不改变姿势。
	s.buexSetSwitch(state, buexSwitchFace, 2)
	if state.chars[0].face != 14 || state.chars[0].pose != "b" {
		t.Fatalf("face/pose = %d/%q, want 14/b", state.chars[0].face, state.chars[0].pose)
	}
}

// TestBuexFaceFlatTable 单行表情组（侵蝕 / 淫獄の放課後2 的 space:[17,0]）
// 仍然按摊平表情表工作：一颗一颗排完 13 颗，选表情时姿势跟着表情分段变。
//
// 这两部作品的表情表和姿势不是一一对应的，改成按页取会让一半表情选不到，
// 所以分页规则只对写了行距的布局生效。
func TestBuexFaceFlatTable(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.layout.Setting.Switches[buexSwitchFace].SpaceY = 0
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 13 {
		t.Fatalf("face switch count = %d, want 13", got)
	}
	// 摊平后第 5 颗是第二页的第一个表情（13 号），姿势跟着到 b。
	s.buexSetSwitch(state, buexSwitchFace, 5)
	if state.chars[0].face != 13 || state.chars[0].pose != "b" {
		t.Fatalf("face/pose = %d/%q, want 13/b", state.chars[0].face, state.chars[0].pose)
	}
}

func TestBuexSwitchCounts(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	if got := s.buexSwitchCount(state, buexSwitchPose); got != 2 {
		t.Fatalf("pose switch count = %d, want 2", got)
	}
	if got := s.buexSwitchCount(state, buexSwitchNgng); got != 7 {
		t.Fatalf("ngng switch count = %d, want 7", got)
	}
	// 服装开关跟着当前角色的服装表长度走。
	if got := s.buexSwitchCount(state, buexSwitchWear); got != 3 {
		t.Fatalf("wear switch count = %d, want 3", got)
	}
	// 表情开关跟着当前姿势那一页的表情表走：1 号角色的 a 页是 1..4，
	// 加上"不要表情"一颗，共 5 颗。
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 5 {
		t.Fatalf("face switch count = %d, want 5", got)
	}
	// 换成 b 姿势后换到第二页（13..16），颗数不变。
	state.chars[0].pose = "b"
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 5 {
		t.Fatalf("pose b face switch count = %d, want 5", got)
	}
	state.chars[0].pose = "a"
	state.selected = 1
	if got := s.buexSwitchCount(state, buexSwitchWear); got != 2 {
		t.Fatalf("character 2 wear switch count = %d, want 2", got)
	}
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 3 {
		t.Fatalf("character 2 face switch count = %d, want 3", got)
	}
	// 旧式 [bu viewer] 里 setting5/6 是独立两组时（牝教師3）按姿势分页：
	// 表情表有两页，当前页是 1..4，加上"不要表情"一颗，共 5 颗。
	state.legacy = true
	state.selected = 0
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 5 {
		t.Fatalf("legacy paged face switch count = %d, want 5", got)
	}
	// 没写行距的布局（侵蝕 / 淫獄の放課後2 的 space:[17,0]）是一整排，
	// 表情表再长也只能铺 13 颗，否则会折行盖到上一排开关上。
	state.legacy = false
	state.layout.Setting.Switches[buexSwitchFace].SpaceY = 0
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 13 {
		t.Fatalf("single-row face switch count = %d, want 13", got)
	}
}

// TestBuexUnderSwitchCount 下着开关的颗数取自布局里的 repeat_x。
//
// YK / Yakata 的下着只有 2 档（repeat_x:2），画 5 颗会多出 3 颗点到
// 不存在的档位；旧式 [bu viewer] 的 repeat_x 没有这层含义，仍然是 5 颗。
func TestBuexUnderSwitchCount(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.layout.Setting.Switches[buexSwitchUnder].Columns = 2
	if got := s.buexSwitchCount(state, buexSwitchUnder); got != 2 {
		t.Fatalf("under switch count = %d, want 2", got)
	}
	// 档位指示不能超出最后一颗：旧存档里的第 4 档落在第 1 颗上。
	state.chars[0].under = 4
	if got := s.buexSwitchActive(state, buexSwitchUnder); got != 1 {
		t.Fatalf("under active = %d, want 1", got)
	}
	// 点第 1 颗把档位真的落到 1。
	s.buexSetSwitch(state, buexSwitchUnder, 1)
	if state.chars[0].under != 1 {
		t.Fatalf("under = %d, want 1", state.chars[0].under)
	}
	// 旧式查看器不受影响。
	state.legacy = true
	if got := s.buexSwitchCount(state, buexSwitchUnder); got != 5 {
		t.Fatalf("legacy under switch count = %d, want 5", got)
	}
}

// TestBuexSwitchRowCap 多行开关组不能排到下一组控件上。
//
// 表情表一页有 21 个表情时，按颗数铺会折出五行，盖到下面的服装开关上。
// 上限按"到下一组开关的距离"算：YK 的表情组在 y=281、行距 37、每行 5 颗，
// 下面最近的服装组在 y=382，只能排 3 行 → 15 颗。
func TestBuexSwitchRowCap(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	face := &state.layout.Setting.Switches[buexSwitchFace]
	face.X, face.Y, face.SpaceX, face.SpaceY, face.Columns = 8, 281, 38, 37, 5
	state.layout.Setting.Switches[buexSwitchWear].Y = 382
	faces := make([]int, 0, 21)
	for value := 1; value <= 21; value++ {
		faces = append(faces, value)
	}
	state.layout.Setting.Characters[0].Faces = [][]int{faces}
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 15 {
		t.Fatalf("face switch count = %d, want 15", got)
	}
	// 布局没写 repeat_x 时不知道每行几颗，不能凭空收窄。
	face.Columns = 0
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 22 {
		t.Fatalf("uncapped face switch count = %d, want 22", got)
	}
	// 旧式布局不套这个上限。
	face.Columns = 5
	state.legacy = true
	if got := s.buexSwitchCount(state, buexSwitchFace); got != 13 {
		t.Fatalf("legacy face switch count = %d, want 13", got)
	}
}

// TestBuexAppearancePosition 显示开关跟着角色图标的配置偏移走。
//
// YK / Yakata 是 [0,37]，侵蝕是 [2,63]，淫獄是 [0,73]：写死其中任意一组
// 都会让另外几部作品的开关落进图标里或掉到图标外面（看起来像选中框错位）。
func TestBuexAppearancePosition(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.layout.Setting.Characters[0].Icon = config.ConfigButton{Name: "dress_view_ch1", MaxState: 3, X: 152, Y: 104}
	state.layout.Setting.Buttons["on_off_button"] = config.ConfigButton{Name: "dress_view_appearance", MaxState: 4, X: 0, Y: 37}
	x, y, ok := buexAppearancePosition(state)
	if !ok || x != 152 || y != 141 {
		t.Fatalf("position = %d,%d,%v, want 152,141,true", x, y, ok)
	}
	// 换角色后跟着新图标再算一次，不能用上一个角色的坐标。
	state.selected = 1
	state.layout.Setting.Characters[1].Icon = config.ConfigButton{Name: "dress_view_ch2", MaxState: 3, X: 56, Y: 104}
	if x, y, ok := buexAppearancePosition(state); !ok || x != 56 || y != 141 {
		t.Fatalf("position = %d,%d,%v, want 56,141,true", x, y, ok)
	}
	// 没有角色时不给位置，调用方别把开关画到左上角。
	state.selected = 9
	if _, _, ok := buexAppearancePosition(state); ok {
		t.Fatalf("position ok for missing character")
	}
}

func TestBuexMoveOrder(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.selected = 1
	s.buexMoveOrder(state, -1)
	if state.order[0] != 1 || state.order[1] != 0 {
		t.Fatalf("order = %v, want [1 0]", state.order)
	}
	// 已经在最前面，往前挪没有效果。
	s.buexMoveOrder(state, -1)
	if state.order[0] != 1 {
		t.Fatalf("order = %v, want unchanged", state.order)
	}
	s.buexMoveOrder(state, 1)
	if state.order[1] != 1 {
		t.Fatalf("order = %v, want [0 1]", state.order)
	}
}

func TestBuexPosRegion(t *testing.T) {
	// 原文档写的是 [0, 743, 1019, 25]：左、上、右、宽。
	left, top, width := buexPosRegion([4]int{0, 743, 1019, 25})
	if left != 0 || top != 743 || width != 1019 {
		t.Fatalf("region = %d,%d,%d, want 0,743,1019", left, top, width)
	}
}

func TestBuexSetPosition(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	s.buexSetPosition(state, 0)
	if state.position != 0 {
		t.Fatalf("position = %d, want 0", state.position)
	}
	s.buexSetPosition(state, 1019)
	if state.position != 1000 {
		t.Fatalf("position = %d, want 1000", state.position)
	}
	// 超出范围要夹住。
	s.buexSetPosition(state, 5000)
	if state.position != 1000 {
		t.Fatalf("position = %d, want 1000 (clamped)", state.position)
	}
}

// TestBuexDistanceSwitchesOrder 锁住「左半身、右放大半身」。
//
// 原版鉴赏室没有全身档：左边是半身（f）图，右边把同一张图放大到更近。
// 内部 view 枚举里 full=0 只是旧存档兼容值，不能再被开关选中。
func TestBuexDistanceSwitchesOrder(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	if state.selected < 0 || state.selected >= len(state.chars) {
		t.Fatal("no selected character")
	}
	s.buexSetSwitch(state, buexSwitchView, 0)
	if state.chars[state.selected].view != buexViewBust {
		t.Fatalf("switch 0 view = %d, want bust", state.chars[state.selected].view)
	}
	if got := s.buexSwitchActive(state, buexSwitchView); got != 0 {
		t.Fatalf("active switch = %d, want 0", got)
	}
	s.buexSetSwitch(state, buexSwitchView, 1)
	if state.chars[state.selected].view != buexViewClose {
		t.Fatalf("switch 1 view = %d, want close", state.chars[state.selected].view)
	}
	if got := s.buexSwitchActive(state, buexSwitchView); got != 1 {
		t.Fatalf("active switch = %d, want 1", got)
	}
}

func TestBuexEditCommands(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.message = "テスト"
	// 字号在 14/18/22/26 之间循环。
	s.buexEditCommand(state, 0)
	if buexFontSizes[state.fontSize] != 18 {
		t.Fatalf("font size = %d, want 18", buexFontSizes[state.fontSize])
	}
	// 排版 0/1/2 循环。
	s.buexEditCommand(state, 2)
	if state.align != 1 {
		t.Fatalf("align = %d, want 1", state.align)
	}
	// 气泡开关。
	s.buexEditCommand(state, 3)
	if !state.balloon {
		t.Fatalf("balloon = false, want true")
	}
	// 字色命令打开取色器。
	s.buexEditCommand(state, 1)
	if !state.colorOpen {
		t.Fatalf("color picker not open")
	}
	// 删除清空留言。
	s.buexEditCommand(state, 4)
	if state.message != "" {
		t.Fatalf("message = %q, want empty", state.message)
	}
}

func TestBuexColorChannels(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	// 目标 0 是文字色，1 是描边色。
	s.buexSetColor(state, 0, 0, false)
	s.buexSetColor(state, 0, 255, false)
	if state.colorText[0] != 255 || state.colorText[1] != 255 {
		t.Fatalf("text color = %v, want red/green maxed", state.colorText)
	}
	state.colorTarget = 1
	s.buexSetColor(state, 2, 0, false)
	if state.colorEdge[2] != 0 {
		t.Fatalf("edge color = %v, want blue 0", state.colorEdge)
	}
	if state.colorText[2] != 255 {
		t.Fatalf("text color = %v, want untouched", state.colorText)
	}
}

func TestBuexPresetLegacyFormat(t *testing.T) {
	s := newBuexTestSession(t)
	// 旧文件是一个数组，只有四个字段。
	legacy := `[{"character":0,"background":"bg01","view":1,"message":"こんにちは"}]`
	path := filepath.Join(dataSaveDir(s.dataDir), "kizuna-buex.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	stored := loadBuexPresets(s)
	if len(stored.Presets) != buexPresetCount {
		t.Fatalf("%d presets, want %d", len(stored.Presets), buexPresetCount)
	}
	preset := stored.Presets[0]
	if preset.Background != "bg01" || preset.Message != "こんにちは" {
		t.Fatalf("preset = %+v", preset)
	}
	// 旧文件的角色下标是 0 基的，读进来要修正成 1 基。
	if preset.Character != 1 {
		t.Fatalf("character = %d, want 1", preset.Character)
	}
	// 缺的字段按默认值补齐。
	if preset.Pose != "a" || preset.Wear != 1 {
		t.Fatalf("preset = %+v, want defaults", preset)
	}
}

func TestBuexPresetRoundTrip(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	state.background = "bg01"
	state.message = "テスト"
	state.chars[0].wear = 3
	state.chars[0].strip = 2
	state.chars[0].face = 5
	state.chars[0].pose = "a"
	state.presets[0] = s.buexCurrentPreset(state)
	saveBuexPresets(s, state.presets, state.slots)

	stored := loadBuexPresets(s)
	if stored.Presets[0].Background != "bg01" || stored.Presets[0].Message != "テスト" {
		t.Fatalf("preset = %+v", stored.Presets[0])
	}

	// 新的文件格式是对象；旧的数组格式也还能读。
	raw, err := os.ReadFile(buexPresetPath(s))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := decoded["presets"]; !ok {
		t.Fatalf("preset file = %s, want object with presets", raw)
	}
}

func TestBuexApplyPreset(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	s.buexApplyPreset(state, buexPreset{
		Character:  2,
		Background: "bg02",
		View:       buexViewBust,
		Strip:      3,
		Wear:       4,
		Pose:       "b",
		Face:       13,
		Under:      2,
		Message:    "テスト",
	})
	if state.selected != 1 {
		t.Fatalf("selected = %d, want 1", state.selected)
	}
	character := state.chars[1]
	if character.view != buexViewBust || character.strip != 3 || character.wear != 4 {
		t.Fatalf("character = %+v", character)
	}
	if character.pose != "b" || character.face != 13 || character.under != 2 {
		t.Fatalf("character = %+v", character)
	}
	if state.background != "bg02" || state.message != "テスト" {
		t.Fatalf("state = %q / %q", state.background, state.message)
	}
}

func TestBuexWheel(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	// 背景页：没有场景数据时一页都没有，滚轮不动。
	state.page = buexPageBackground
	s.wheelBuex(1)
	if state.bgPage != 0 {
		t.Fatalf("bg page = %d, want 0", state.bgPage)
	}
	// 存档页：滚轮换选中的预设格。
	state.page = buexPageStore
	s.wheelBuex(1)
	if state.selectedPreset != 1 {
		t.Fatalf("selected preset = %d, want 1", state.selectedPreset)
	}
	s.wheelBuex(-5)
	if state.selectedPreset != 0 {
		t.Fatalf("selected preset = %d, want 0", state.selectedPreset)
	}
	// 其它页不响应。
	state.page = buexPageStandee
	s.wheelBuex(3)
	if state.selectedPreset != 0 {
		t.Fatalf("selected preset = %d, want 0", state.selectedPreset)
	}
}

func TestBuexMenuDispatch(t *testing.T) {
	s := newBuexTestSession(t)
	state := s.ui.buex
	s.buexMenu(state, "bg_select")
	if state.page != buexPageBackground {
		t.Fatalf("page = %d, want %d", state.page, buexPageBackground)
	}
	s.buexMenu(state, "data_store")
	if state.page != buexPageStore {
		t.Fatalf("page = %d, want %d", state.page, buexPageStore)
	}
	s.buexMenu(state, "back_to_thumbs")
	if s.ui.kind != uiMuseum || s.ui.buex != nil {
		t.Fatalf("ui = %d / %v, want museum / nil", s.ui.kind, s.ui.buex)
	}
}

func TestBuexSubmitName(t *testing.T) {
	s := newBuexTestSession(t)
	if !s.SubmitName("  こんにちは  ") {
		t.Fatalf("SubmitName refused")
	}
	if s.ui.buex.message != "こんにちは" {
		t.Fatalf("message = %q", s.ui.buex.message)
	}
	if s.ui.buex.editing {
		t.Fatalf("still editing")
	}
}

// 多角色同屏（牝教師3）：每名角色的显示开关、站位和前后层序各自独立。
func TestBuexMultiStandeeState(t *testing.T) {
	s := newBuexTestSession(t)
	layout := buexTestLayout()
	layout.MultiStandee = true
	state := s.buexNewState(layout)
	if !state.multi {
		t.Fatal("MultiStandee should turn on multi mode")
	}
	// 打开时只显示第一个角色，避免所有人叠在画面中央。
	if !state.chars[0].visible || state.chars[1].visible {
		t.Fatalf("initial visible = %v / %v", state.chars[0].visible, state.chars[1].visible)
	}
	// ON/OFF 属于被点的那张角色卡，不是当前选中角色。
	state.selected = 0
	s.activateBustupMuseum(uiHit{kind: hitBUAppearance, index: 1}, 0, 0)
	if !state.chars[1].visible {
		t.Fatal("appearance hit should toggle character 1")
	}
	s.activateBustupMuseum(uiHit{kind: hitBUAppearance, index: 1}, 0, 0)
	if state.chars[1].visible {
		t.Fatal("appearance hit should toggle character 1 off")
	}
	// 站位条只移动选中的角色。
	state.selected = 1
	s.buexSetPosition(state, 0)
	if state.chars[1].position != 0 {
		t.Fatalf("character 1 position = %d, want 0", state.chars[1].position)
	}
	if state.chars[0].position != 500 {
		t.Fatalf("character 0 position = %d, want 500", state.chars[0].position)
	}
	// 重なりボタン把选中角色提到最前：order[0] 是最上层。
	s.buexMoveOrder(state, -1)
	if state.order[0] != 1 {
		t.Fatalf("order = %v, want 1 first", state.order)
	}
	if got := s.buexStandeeSwayX(state, 1); got == 0 {
		t.Fatal("position 0 should offset the standee away from center")
	}
	if got := s.buexStandeeSwayX(state, 0); got != 0 {
		t.Fatalf("character 0 position 500 should be centered, got %d", got)
	}
}
