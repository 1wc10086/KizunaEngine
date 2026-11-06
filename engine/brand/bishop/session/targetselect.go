package session

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// targetselect.go 是学園2 老版周结的“目標選択”画面（system call "target"）。
//
// 脚本在 after_02/after_03 里先要求引擎显示目标选择，玩家选完后把结果写回
// #target，脚本再按 1..6/8 分支：1 春菜、2 美久、3 香織、4 泉、5 春菜&泉、
// 6 美久&香織、8 家に帰る。这里用发行版自己的 tgt_* 卡片还原：
// 每张卡片 400x112、两帧（常态/悬停），竖排居中；没有对应布局的作品直接放行。

type targetChoice struct {
	value int
	card  config.UIButton
	label string
}

const (
	targetCardWidth  = 400
	targetCardHeight = 56
	targetCardLeft   = 200
	targetCardTop    = 20
	targetCardStep   = 60
)

// beginTargetSelect 打开目标选择；没有可选目标时按“家に帰る”继续脚本。
func (s *Session) beginTargetSelect() state.SessionStep {
	// 放課後2 用 target_card00..08 的九宫格；素材指纹专属于该作，
	// 不命中时继续走下面的 MK/学園2 分支。
	if s.beginTargetHoukago2Select() {
		return state.SessionAwaitInput
	}
	// スレイブポリス（SPMM，Rev.3）走三张 300x180 的 tgt_1..3 竖排卡并写
	// #target；素材指纹专属于该作，命中时接管，不命中时返回 false，
	// 下面的 MK/学園2 分支完全不受影响。
	if s.beginTargetSpmmSelect() {
		return state.SessionAwaitInput
	}
	// 牝教師～淫辱の教室～（MK）走 #result 和 1..7/9 的分派，和学園2 的
	// #target 是两套数据。先让 MK 的素材探针尝试接管；不是 MK 时它返回 false，
	// 下面的旧逻辑（含 GK2 的 uiTarget）完全不受影响。
	if s.beginTargetMKSelect() {
		return state.SessionAwaitInput
	}
	if s.config == nil || !s.config.Museum.Gakuen2 {
		// 其它作品没有这套目标选择数据：保持原来的“记录日志后继续”。
		core.Logf("script: system call 'target' has no built-in target screen")
		return state.SessionContinue
	}
	options := s.targetChoices()
	if len(options) <= 1 {
		// 彻底没有目标时原版直接回家；素材缺失但属性有目标时不能误写 8，
		// 否则会把本该发生的角色事件跳过。
		if !s.hasTargetFlag() {
			s.writeTargetValue(targetHomeValue)
		} else {
			core.Logf("ui: target select assets missing, leaving #target untouched")
		}
		return state.SessionContinue
	}
	s.beginSystemUI(uiTarget)
	if s.ui == nil {
		return state.SessionContinue
	}
	s.ui.targetOptions = options
	s.ui.targetHover = -1
	core.Logf("ui: target select %d option(s)", len(options))
	return state.SessionAwaitInput
}

const (
	targetHarunaValue = 1
	targetMikuValue   = 2
	targetKaoriValue  = 3
	targetIzumiValue  = 4
	targetCombo1Value = 5
	targetCombo2Value = 6
	targetHaremValue  = 7
	targetHomeValue   = 8
	targetNobodyValue = 9
)

// targetChoices 从 #X_map 标记和素材生成候选卡片。
//
// 原版还可能有ハーレム/誰も選ばない等组合卡；脚本目前只检查 1..6/8，
// 因此这里只给出有确定分支的选项，避免把 #target 写进脚本没有处理的值。
func (s *Session) targetChoices() []targetChoice {
	hrn := s.sessionProperty("#hrn_map") == 1
	mik := s.sessionProperty("#mik_map") == 1
	kwr := s.sessionProperty("#kwr_map") == 1
	izm := s.sessionProperty("#izm_map") == 1
	options := make([]targetChoice, 0, 7)
	add := func(value int, images []string, label string) {
		if s.res == nil {
			return
		}
		for _, image := range images {
			if !s.res.Exists(image) {
				continue
			}
			options = append(options, targetChoice{
				value: value,
				card:  config.UIButton{Image: image, Frames: 2},
				label: label,
			})
			return
		}
	}
	if hrn {
		add(targetHarunaValue, []string{"tgt_hrn"}, "春菜")
	}
	if mik {
		add(targetMikuValue, []string{"tgt_mk"}, "美久")
	}
	if kwr {
		add(targetKaoriValue, []string{"tgt_kor"}, "香織")
	}
	if izm {
		add(targetIzumiValue, []string{"tgt_izm"}, "泉")
	}
	if hrn && izm {
		add(targetCombo1Value, []string{"tgt_cmb1"}, "春菜＆泉")
	}
	if mik && kwr {
		add(targetCombo2Value, []string{"tgt_cmb2"}, "美久＆香織")
	}
	if hrn && mik && kwr && izm {
		// 原版脚本没有显式比较 7；after_03 的 fallthrough 会进入 after_hrn，
		// 由该段再按 #cmbH1/#cmbH2 判定ハーレム，和卡片顺序一致。
		add(targetHaremValue, []string{"tgt_hrm"}, "全員")
	}
	// 原版 EXE 里写的是 tgt_quit，实际素材名是 tgt_exit；两个都试。
	add(targetHomeValue, []string{"tgt_quit", "tgt_exit"}, "家に帰る")
	// 「誰も選ばない」是 EXE 卡片顺序里的第 9 项（hrn..quit,non）。
	// 脚本对这个值走 after_hrn 的默认分支，和原版一致。
	add(targetNobodyValue, []string{"tgt_non"}, "誰も選ばない")
	return options
}

func (s *Session) hasTargetFlag() bool {
	for _, name := range []string{"#hrn_map", "#mik_map", "#kwr_map", "#izm_map"} {
		if s.sessionProperty(name) == 1 {
			return true
		}
	}
	return false
}

func (s *Session) sessionProperty(name string) int {
	if s.script == nil || s.machine == nil {
		return 0
	}
	index := s.script.GetPropertyIndex(name)
	if index < 0 {
		return 0
	}
	return int(s.machine.Property(index))
}

func (s *Session) writeTargetValue(value int) {
	if s.script == nil || s.machine == nil {
		return
	}
	index := s.script.GetPropertyIndex("#target")
	if index < 0 {
		core.Logf("script: target select has no #target property")
		return
	}
	s.machine.SetProperty(index, int32(value))
}

// drawTargetSelect 画目标选择：保留进入前的剧情帧，再把卡片竖排居中。
func (s *Session) drawTargetSelect(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || canvas == nil {
		return
	}
	if ui.backdrop != nil {
		canvas.BlitCanvasRegion(ui.backdrop, 0, 0, ui.backdrop.Width, ui.backdrop.Height, 0, 0, 255)
	}
	// 压暗剧情帧，卡片文字才读得清；原版也是半透明黑底。
	canvas.FillBlend(0, 0, s.width, s.height, 0, 0, 0, 140)
	for index, option := range ui.targetOptions {
		button := option.card
		button.X = targetCardLeft
		button.Y = targetCardTop + index*targetCardStep
		frame := 0
		if ui.targetHover == index {
			frame = 1
		}
		s.drawFrameButton(canvas, button, frame)
	}
}

// hitTargetSelect 返回鼠标下的卡片下标；-1 表示没有命中。
func (s *Session) hitTargetSelect(x, y int) int {
	ui := s.ui
	if ui == nil {
		return -1
	}
	for index := range ui.targetOptions {
		top := targetCardTop + index*targetCardStep
		if x >= targetCardLeft && x < targetCardLeft+targetCardWidth && y >= top && y < top+targetCardHeight {
			return index
		}
	}
	return -1
}

// activateTargetSelect 写回 #target、关闭界面并让脚本从 SYSCALL 的下一条继续。
func (s *Session) activateTargetSelect(index int) {
	ui := s.ui
	if ui == nil || index < 0 || index >= len(ui.targetOptions) {
		return
	}
	value := ui.targetOptions[index].value
	s.writeTargetValue(value)
	core.Logf("ui: target select %d -> #target=%d", index, value)
	s.closeSystemUI()
	s.lastStep = s.finishRun(s.run(commandBudget))
}
