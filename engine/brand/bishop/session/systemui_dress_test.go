package session

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
)

// systemui_dress_test.go 覆盖脱衣设定画面里那些"算出来的"逻辑：
// 服装列表怎么排、一屏能显示几条、换衣服后阶段怎么收敛、滚轮怎么滚。
//
// 这里照着 bsxx.cfg 的 [dress screen] 摆一套等价的数据：
// 服装列表从 (844,127) 起、行距 30，阶段预览从 y=505 开始。

// dressTestLevels 是阶段预览按钮（levelN）及其坐标。
var dressTestLevels = map[int]int{
	1: 685, 2: 535, 3: 505, 4: 565, 5: 595, 6: 625, 7: 655,
	8: 625, 9: 625, 10: 655, 11: 505, 12: 505, 13: 505, 14: 505,
	15: 505, 16: 505, 17: 685, 18: 505,
	19: 685, 20: 535, 21: 505, 22: 565, 23: 595, 24: 625, 25: 655,
	26: 505, 27: 505,
}

// dressTestLayout 造一份和真实配置等价的脱衣画面布局。
func dressTestLayout() config.DressScreen {
	layout := config.DressScreen{
		Base:              "strip_base",
		Base2:             "strip_base2",
		ShowBu:            true,
		ShowCommonButton:  true,
		EnableViewControl: true,
		MaxStrip:          7,
		InitNgLevel:       1,
		StripConvert:      []int{0, 1, 2, 3, 4, 5, 6},
		DressPos:          config.UIPoint{X: 844, Y: 127},
		DressSpace:        config.UIPoint{X: 0, Y: 30},
		WholeBodyPos:      config.UIPoint{X: 0, Y: -20},
	}
	for index := 0; index < 5; index++ {
		layout.Targets = append(layout.Targets, config.UIButton{Image: "strip_ch0" + string(rune('1'+index)), Frames: 4})
	}
	for index := 0; index < 7; index++ {
		layout.States = append(layout.States, config.UIButton{Image: "strip_cstate" + string(rune('0'+index)), Frames: 4})
	}
	layout.Modes = []config.UIButton{
		{Image: "strip_mode_one", Frames: 4},
		{Image: "strip_mode_all", Frames: 4},
		{Image: "strip_mode_view", Frames: 4},
	}
	layout.Back = config.UIButton{Image: "strip_back", Frames: 3}

	// 服装按钮：0 是"全部服装"，之后是各角色自己的衣服。
	layout.Costumes = append(layout.Costumes, config.UIButton{Image: "costume_allcos", Frames: 5})
	for index := 1; index <= 16; index++ {
		layout.Costumes = append(layout.Costumes, config.UIButton{Image: "costume_101", Frames: 5})
	}

	for index, y := range dressTestLevels {
		layout.Levels = append(layout.Levels, config.DressLevel{
			Index:  index,
			Button: config.UIButton{Image: "strip_cstate_s", Frames: 4, X: 844, Y: y},
		})
	}

	// 1 号角色：14 件衣服 + 一条"一括"（wear 0）。
	for wear := 1; wear <= 14; wear++ {
		stages := [7]int{3, 2, 4, 5, 6, 7, 1}
		if wear == 2 {
			stages = [7]int{8, 2, 0, 0, 9, 10, 1}
		}
		if wear == 3 {
			stages = [7]int{11, 2, 0, 0, 0, 0, 1}
		}
		layout.DressCostumes = append(layout.DressCostumes, config.DressCostume{
			Character:  1,
			Wear:       wear,
			DressImage: wear,
			Stages:     stages,
		})
	}
	layout.DressCostumes = append(layout.DressCostumes, config.DressCostume{
		Character:  1,
		Wear:       0,
		DressImage: 0,
		Stages:     [7]int{3, 2, 4, 5, 6, 7, 1},
	})
	// 2 号角色：2 件衣服 + 一条"一括"。
	layout.DressCostumes = append(layout.DressCostumes,
		config.DressCostume{Character: 2, Wear: 1, DressImage: 15, Stages: [7]int{21, 20, 22, 23, 24, 25, 19}},
		config.DressCostume{Character: 2, Wear: 2, DressImage: 16, Stages: [7]int{26, 20, 0, 0, 0, 0, 19}},
		config.DressCostume{Character: 2, Wear: 0, DressImage: 0, Stages: [7]int{21, 20, 22, 23, 24, 25, 19}},
	)

	layout.View = config.DressViewControl{
		Enabled: true,
		MeterX:  1003, MeterY: 159, MeterW: 14, MeterH: 559,
		Knob: config.UIButton{Image: "view_knob", Frames: 1, X: 1003, Y: 159},
		Face: config.UIButton{Image: "view_btn_face", Frames: 4, X: 886, Y: 47},
		Bust: config.UIButton{Image: "view_btn_bust", Frames: 4, X: 886, Y: 83},
		Hip:  config.UIButton{Image: "view_btn_hip", Frames: 4, X: 886, Y: 119},
	}
	return layout
}

// newDressTestSession 造一个打开着脱衣画面的会话。
func newDressTestSession(t *testing.T) *Session {
	t.Helper()
	s := &Session{
		config: &config.BsxxConfiguration{Dress: dressTestLayout()},
		ui:     &systemUI{kind: uiDress, dress: &dressState{}},
	}
	s.dressReset()
	return s
}

func TestDressVisibleRows(t *testing.T) {
	s := newDressTestSession(t)
	// (505 - 127) / 30 = 12：列表不能排到阶段预览的第一行。
	if got := s.dressVisibleRows(); got != 12 {
		t.Fatalf("visible rows = %d, want 12", got)
	}
}

// SLIE 的服装列表 (266,209)、行距 43，阶段预览在另一列 (x=897, y=209)。
// 列表不受预览列限制，应该一直排到画面下沿：(768-209)/43 = 13 行。
// 旧实现把预览的 Y 当底边，算出 0 行，列表只剩一条。
func TestDressVisibleRowsSeparateColumns(t *testing.T) {
	s := newDressTestSession(t)
	s.height = 768
	s.config.Dress.DressPos = config.UIPoint{X: 266, Y: 209}
	s.config.Dress.DressSpace = config.UIPoint{X: 0, Y: 43}
	for index := range s.config.Dress.Levels {
		s.config.Dress.Levels[index].Button.X = 897
		s.config.Dress.Levels[index].Button.Y = 209
	}
	if got := s.dressVisibleRows(); got != 13 {
		t.Fatalf("visible rows = %d, want 13", got)
	}
}

func TestDressCostumeListOrder(t *testing.T) {
	s := newDressTestSession(t)
	list := s.dressCostumeList()
	if len(list) != 15 {
		t.Fatalf("costume list = %d entries, want 15", len(list))
	}
	// 原版把「全服装一括」（wear 0）排在列表第一条（手册截图）。
	if list[0].Wear != 0 || list[0].DressImage != 0 {
		t.Fatalf("first entry = wear %d image %d, want 0/0", list[0].Wear, list[0].DressImage)
	}
	button, ok := s.dressCostumeButton(list[0])
	if !ok || button.Image != "costume_allcos" {
		t.Fatalf("wear-0 button = %q ok=%v", button.Image, ok)
	}
	// 第一条具体服装是 wear 1。
	if list[1].Wear != 1 {
		t.Fatalf("second wear = %d, want 1", list[1].Wear)
	}
	// 2 号角色只有自己的三条，同样是全服装一括在前。
	s.ui.dress.character = 1
	other := s.dressCostumeList()
	if len(other) != 3 {
		t.Fatalf("character 2 list = %d entries, want 3", len(other))
	}
	if other[0].Wear != 0 || other[1].Wear != 1 {
		t.Fatalf("character 2 order = %d/%d, want 0/1", other[0].Wear, other[1].Wear)
	}
}

func TestDressInitialCostumeSkipsAllCostumes(t *testing.T) {
	s := newDressTestSession(t)
	// 默认选中第一条具体服装（制服），不是「全服装一括」。
	if got := s.dressEntry(0).costume; got != 1 {
		t.Fatalf("initial costume index = %d, want 1", got)
	}
}

func TestDressInitialState(t *testing.T) {
	s := newDressTestSession(t)
	if s.ui.dress.character != 0 || s.ui.dress.mode != dressModeSingle {
		t.Fatalf("initial character/mode = %d/%d", s.ui.dress.character, s.ui.dress.mode)
	}
	// init ng level = 1。
	if got := s.dressCurrentStage(); got != 1 {
		t.Fatalf("initial stage = %d, want 1", got)
	}
	if got := s.dressWearNumber(); got != 1 {
		t.Fatalf("initial wear = %d, want 1", got)
	}
}

func TestDressSelectCostumeClampsStage(t *testing.T) {
	s := newDressTestSession(t)
	// 先切到 3 号阶段（私服的第 4 级），再换成没有这一级的衣服。
	s.dressSelectStage(3)
	if got := s.dressCurrentStage(); got != 3 {
		t.Fatalf("stage = %d, want 3", got)
	}
	s.dressSelectCostume(2) // wear 2：阶段 3 没有素材。
	if got := s.dressCurrentStage(); got != 4 {
		t.Fatalf("clamped stage = %d, want 4", got)
	}
	if got := s.dressWearNumber(); got != 2 {
		t.Fatalf("wear = %d, want 2", got)
	}
}

func TestDressSelectStageRejectsUnavailable(t *testing.T) {
	s := newDressTestSession(t)
	s.dressSelectCostume(2) // wear 2：阶段 2 没有素材。
	s.dressSelectStage(2)
	if got := s.dressCurrentStage(); got != 1 {
		t.Fatalf("stage = %d, want 1 (unchanged)", got)
	}
	s.dressSelectStage(4)
	if got := s.dressCurrentStage(); got != 4 {
		t.Fatalf("stage = %d, want 4", got)
	}
}

func TestDressModeAllAppliesToEveryCharacter(t *testing.T) {
	s := newDressTestSession(t)
	s.ui.dress.mode = dressModeAll
	s.dressSelectCostume(1) // wear 2
	if got := s.dressWearForCharacter(0); got != 2 {
		t.Fatalf("character 1 wear = %d, want 2", got)
	}
	if got := s.dressWearForCharacter(1); got != 2 {
		t.Fatalf("character 2 wear = %d, want 2", got)
	}
	// 阶段也一样：一次改动套用到所有角色。
	s.dressSelectStage(0)
	if got := s.dressEntry(0).strip; got != 0 {
		t.Fatalf("character 1 stage = %d, want 0", got)
	}
	if got := s.dressEntry(1).strip; got != 0 {
		t.Fatalf("character 2 stage = %d, want 0", got)
	}
}

func TestDressWheelScrollsList(t *testing.T) {
	s := newDressTestSession(t)
	// 15 条服装、一屏 12 条 → 最多能滚 3 条。
	s.wheelDress(1)
	if got := s.ui.dress.scroll; got != 3 {
		t.Fatalf("scroll = %d, want 3", got)
	}
	s.wheelDress(1)
	if got := s.ui.dress.scroll; got != 3 {
		t.Fatalf("scroll = %d, want 3 (clamped)", got)
	}
	s.wheelDress(-2)
	if got := s.ui.dress.scroll; got != 0 {
		t.Fatalf("scroll = %d, want 0", got)
	}
}

func TestDressViewPresets(t *testing.T) {
	s := newDressTestSession(t)
	s.ui.dress.mode = dressModeView
	s.activateDressScreen(uiHit{kind: hitDressView, index: 1}, 0, 0)
	if got := s.ui.dress.viewPos; got != dressViewBust {
		t.Fatalf("view pos = %d, want %d", got, dressViewBust)
	}
	s.activateDressScreen(uiHit{kind: hitDressViewBar}, 0, s.config.Dress.View.MeterY+10)
	if s.ui.dress.viewPos <= 0 {
		t.Fatalf("view pos = %d, want > 0", s.ui.dress.viewPos)
	}
}
