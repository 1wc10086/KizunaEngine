package session

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// targetselect_ks.go 是教育指導（KS）的放学目标选择画面。
//
// 剧本 leaveschool_select 先用 #result 的位掩码标记今天可攻略的角色
// （1=清香、2=柚姫、4=千夏、8=楓、16=恵利子…），然后发
// SELECT_TAIL "target"。原引擎按掩码把 target0…target11 的卡片横排在
// (340,142)，点哪张就把卡片值写回 #result；脚本随后按 1..9 分派到
// leaveschool_select_* 标签。
//
// KSIE 用 [custom map phase] 自己画同一屏，所以这条分支只在 config
// 里存在 [target select]（KS）时启用；其它作品完全不受影响。

type targetCardChoice struct {
	image string
	value int
	x     int
	y     int
}

// targetCardState 保存当前目标选择的卡片、布局与悬停下标。
//
// mode 为空/"ks" 时是教育指導的放学目标卡（cards/hover 有效）；
// mode == "mk" 时是牝教師～淫辱の教室～的目标选择（pages/page/selected 有效），
// 两套数据放在同一个字段里是为了复用 uiTargetCard 的绘制/命中/激活分派。
type targetCardState struct {
	cards []targetCardChoice
	hover int

	mode     string
	pages    [][]targetCardChoice
	page     int
	hoverNav int
	selected int
}

// beginTargetCardSelect 打开 KS 的目标选择。返回 false 表示没有可用卡片，
// 调用方应让脚本继续执行（写 #result=0 后走"今天不找任何人"分支）。
func (s *Session) beginTargetCardSelect() bool {
	if s.config == nil || s.config.TargetSelect.Empty() || s.script == nil || s.machine == nil {
		return false
	}
	selectScreen := s.config.TargetSelect
	mask := s.sessionProperty("#result")
	cards := make([]targetCardChoice, 0, len(selectScreen.Cards))
	for index, card := range selectScreen.Cards {
		// 位掩码只覆盖 target0..target11；卡片值为 0 的条目是取消占位，
		// 不参与选择。
		if card.Value <= 0 || index >= 31 || mask&(1<<uint(index)) == 0 {
			continue
		}
		if y := s.composer.Load(card.Image); y == nil {
			continue
		}
		cards = append(cards, targetCardChoice{image: card.Image, value: card.Value})
	}
	if len(cards) == 0 {
		// 没有可攻略对象：按原版写 0，让剧本走"直接回家"的分支。
		s.writeOne("#result", -1, 0)
		core.Logf("ui: target select has no available card")
		return false
	}
	// 整排卡片以 selected card pos 的中心为轴居中：只有一张卡时它的
	// 左上角就是配置值（340,142）；两张卡时从 280 开始排，与手册截图里
	// 清香/柚姫的位置一致。卡片图自带透明边距，所以视觉左缘会再偏移几像素。
	step := selectScreen.CardWidth
	total := step * len(cards)
	centerX := selectScreen.SelectedX + step/2
	startX := centerX - total/2
	if startX < 8 {
		startX = 8
	}
	if startX+total > s.width-8 {
		startX = s.width - total - 8
	}
	if startX < 8 {
		startX = 8
	}
	for index := range cards {
		cards[index].x = startX + index*step
		cards[index].y = selectScreen.SelectedY
	}
	s.beginSystemUI(uiTargetCard)
	if s.ui == nil {
		return false
	}
	s.ui.targetCards = &targetCardState{cards: cards, hover: -1}
	core.Logf("ui: target select %d card(s) from #result=%d", len(cards), mask)
	return true
}

// drawTargetCardSelect 把目标卡画在进入前的剧情帧上。
//
// 卡片是 120x570 的竖排图（常态/悬停/选中三帧）。原版不压暗背景，
// 也不移动对话框，所以这里直接叠在 backdrop 上。
func (s *Session) drawTargetCardSelect(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || canvas == nil {
		return
	}
	if ui.targetCards == nil {
		return
	}
	if ui.targetCards.mode == "mk" {
		s.drawMKTargetSelect(canvas)
		return
	}
	if ui.targetCards.mode == "houkago2" {
		s.drawHoukago2TargetSelect(canvas)
		return
	}
	if ui.targetCards.mode == "spmm" {
		s.drawSpmmTargetSelect(canvas)
		return
	}
	if ui.backdrop != nil {
		canvas.BlitCanvasRegion(ui.backdrop, 0, 0, ui.backdrop.Width, ui.backdrop.Height, 0, 0, 255)
	}
	selectScreen := s.config.TargetSelect
	cellHeight := selectScreen.CardHeight
	if cellHeight <= 0 {
		return
	}
	for index, card := range ui.targetCards.cards {
		x := card.x
		if x+selectScreen.CardWidth <= 0 || x >= canvas.Width {
			continue
		}
		image := s.composer.Load(card.image)
		if image == nil {
			continue
		}
		frames := image.Height / cellHeight
		if frames <= 0 {
			continue
		}
		frame := 0
		if index == ui.targetCards.hover && frames > 1 {
			frame = 1
		}
		canvas.DrawRegion(image, 0, frame*cellHeight, image.Width, cellHeight, x, card.y)
	}
}

func (s *Session) hitTargetCardSelect(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.targetCards == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	if ui.targetCards.mode == "mk" {
		return s.hitMKTargetSelect(x, y)
	}
	if ui.targetCards.mode == "houkago2" {
		return s.hitHoukago2TargetSelect(x, y)
	}
	if ui.targetCards.mode == "spmm" {
		return s.hitSpmmTargetSelect(x, y)
	}
	selectScreen := s.config.TargetSelect
	width, height := selectScreen.CardWidth, selectScreen.CardHeight
	for index, card := range ui.targetCards.cards {
		cardX := card.x
		if x >= cardX && x < cardX+width && y >= card.y && y < card.y+height {
			ui.targetCards.hover = index
			return uiHit{kind: hitTargetCard, index: index}
		}
	}
	ui.targetCards.hover = -1
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateTargetCard(index int) {
	ui := s.ui
	if ui == nil || ui.targetCards == nil {
		return
	}
	if ui.targetCards.mode == "mk" {
		s.activateMKTargetCard(index)
		return
	}
	if ui.targetCards.mode == "houkago2" {
		s.activateHoukago2TargetCard(index)
		return
	}
	if ui.targetCards.mode == "spmm" {
		s.activateSpmmTargetCard(index)
		return
	}
	if index < 0 || index >= len(ui.targetCards.cards) {
		return
	}
	value := ui.targetCards.cards[index].value
	s.writeOne("#result", -1, value)
	core.Logf("ui: target card %d -> #result=%d", index, value)
	s.closeSystemUI()
	s.lastStep = s.finishRun(s.run(commandBudget))
}
