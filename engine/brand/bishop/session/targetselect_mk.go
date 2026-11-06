package session

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// targetselect_mk.go 是牝教師～淫辱の教室～（MK，BSScript Rev.5）的
// 「ターゲット選択」画面。
//
// 剧本在放課後の段阶先发 system call "target"，等引擎把玩家选的目标写回
// #result 后，再发一个只有「ＯＫ／考え直す」的 SELECT 让玩家确认。
// 这里不能用学園2 的 uiTarget：那一屏写的是 #target，而 MK 的脚本按
// #result 分派 1..7 与 9（1..5 = 美雪/麻琴/律子/沙耶/たまき，
// 6/7 = 两人事件，9 = 止める）。
//
// 卡片和箭头都直接用发行版资源 tgt_*。布局是按 256x384 卡片 + 208x88
// 箭头图推出来的近似值：原引擎是一个可翻页的卡片浏览器，具体内边距
// 无法从资源本身看出，所以这里只保证「三张一页、左右箭头、下中決定」
// 与素材尺寸对得上，视觉上和原版不会逐像素一致。

const (
	mkTargetCardWidth  = 256
	mkTargetCardHeight = 384
	mkTargetCardLeft   = 16
	mkTargetCardTop    = 108
	mkTargetCardStep   = 256
	// 800 宽屏上每页放三张：16/272/528，第三张右缘 784，留 16px 边距。
	mkTargetCardsPerPage = 3

	mkTargetNavWidth  = 208
	mkTargetNavHeight = 88
	// prev 在左缘、next 在右缘，和卡片中部重叠是这套素材的原始摆法；
	// 命中判定里箭头优先，否则左右箭头的重叠区会全部被卡片吃掉。
	mkTargetPrevX = 0
	mkTargetNextX = 592
	mkTargetNavY  = 300
	// 決定按钮在卡片下方居中：296 + 208 = 504，水平中心正好是 400。
	mkTargetDoneX = 296
	mkTargetDoneY = 504

	// 箭头图是 208x352，四帧；原版常态用第 2 帧（带文字描边），
	// 悬停用第 1 帧（浅色发光）。
	mkTargetNavFrameNormal = 2
	mkTargetNavFrameHover  = 1
)

// 命中下标里 0..2 是当前页的卡片，100 以上给三个固定按钮，
// 这样继续复用 uiHit{kind: hitTargetCard, index} 而不用新增命中类型。
const (
	mkTargetNavPrev = 100
	mkTargetNavNext = 101
	mkTargetNavDone = 102
)

// #result 的取值：1..5 对应 tgt_ch1..tgt_ch5，6/7 是 tgt_cmb1/tgt_cmb2，
// 9 是 tgt_non。8（tgt_hrm）脚本没有分支，见 mkTargetPages 的说明。
const (
	mkTargetMiyukiValue  = 1
	mkTargetMakotoValue  = 2
	mkTargetRitsukoValue = 3
	mkTargetSayaValue    = 4
	mkTargetTamakiValue  = 5
	mkTargetComboH1Value = 6
	mkTargetComboH2Value = 7
	mkTargetStopValue    = 9
)

// beginTargetMKSelect 打开 MK 的目标选择。返回 true 表示已经进入等待输入，
// 调用方必须返回 SessionAwaitInput；false 表示这不是 MK（或素材不全），
// 调用方继续走原来的分支。
//
// 整条 MK 分支用素材探针而不是作品名判断：只有 MK 的 Graphicsa.bsa 同时带
// 这五张图，学園2（GK2）用的是自己的一整套 tgt_* 布局，必须让给 uiTarget。
func (s *Session) beginTargetMKSelect() bool {
	if s.config == nil || s.config.Museum.Gakuen2 {
		return false
	}
	if s.script == nil || s.machine == nil || s.composer == nil || s.res == nil {
		return false
	}
	// 卡片本身 + 三个按钮缺一不可：缺按钮时玩家无法翻页/确认，
	// 宁可退回原来的「直接继续」，也不要给出一个点不动的死屏。
	for _, name := range []string{"tgt_ch1", "tgt_done", "tgt_next", "tgt_prev", "tgt_non"} {
		if !s.res.Exists(name) {
			return false
		}
	}
	// 没有 #result 就写不回结果；这种作品即使碰巧有同名素材也不该接管。
	if s.script.GetPropertyIndex("#result") < 0 {
		return false
	}
	pages := s.mkTargetPages()
	s.beginSystemUI(uiTargetCard)
	if s.ui == nil {
		return false
	}
	s.ui.targetCards = &targetCardState{
		mode:     "mk",
		pages:    pages,
		page:     0,
		hover:    -1,
		hoverNav: -1,
		selected: -1,
	}
	core.Logf("ui: mk target select %d candidate(s) in %d page(s)", countMKTargetCards(pages), len(pages))
	return true
}

// mkTargetPages 按「今天在地图上的人」生成候选卡，最后一页一定带「止める」。
//
// 组合卡的开关是 #combiH1_event / #combiH2_event；脚本只处理 #result 1..7
// 与 9，因此 tgt_hrm（隐藏「?」卡，对应 8）刻意不放出来：写 8 不会落到
// 任何角色事件上，玩家会白丢一天的放課後。
func (s *Session) mkTargetPages() [][]targetCardChoice {
	cards := make([]targetCardChoice, 0, 8)
	add := func(value int, image, flag string) {
		if len(flag) > 0 && s.sessionProperty(flag) == 0 {
			return
		}
		if !s.res.Exists(image) {
			return
		}
		cards = append(cards, targetCardChoice{image: image, value: value})
	}
	add(mkTargetMiyukiValue, "tgt_ch1", "#miyuki_event")
	add(mkTargetMakotoValue, "tgt_ch2", "#makoto_event")
	add(mkTargetRitsukoValue, "tgt_ch3", "#ritsuko_event")
	add(mkTargetSayaValue, "tgt_ch4", "#saya_event")
	add(mkTargetTamakiValue, "tgt_ch5", "#tamaki_event")
	// 两人事件卡只按事件标记出现；脚本分派的是组合值本身，
	// 不再检查两位角色今天是否都在地图上（原版也是按事件旗标放卡）。
	add(mkTargetComboH1Value, "tgt_cmb1", "#combiH1_event")
	add(mkTargetComboH2Value, "tgt_cmb2", "#combiH2_event")
	// 「止める」永远可选：即使今天没有任何人，也要让玩家能结束这次选择。
	add(mkTargetStopValue, "tgt_non", "")

	// 卡片坐标在生成时就按页内格位固定下来，绘制和命中共用同一份几何，
	// 避免两处各算一次导致错位。
	for index := range cards {
		slot := index % mkTargetCardsPerPage
		cards[index].x = mkTargetCardLeft + slot*mkTargetCardStep
		cards[index].y = mkTargetCardTop
	}
	pages := make([][]targetCardChoice, 0, (len(cards)+mkTargetCardsPerPage-1)/mkTargetCardsPerPage)
	for start := 0; start < len(cards); start += mkTargetCardsPerPage {
		end := min(start+mkTargetCardsPerPage, len(cards))
		pages = append(pages, cards[start:end])
	}
	return pages
}

func countMKTargetCards(pages [][]targetCardChoice) int {
	total := 0
	for _, page := range pages {
		total += len(page)
	}
	return total
}

// drawMKTargetSelect 保留进入前的剧情帧并压暗，再画当前页卡片和三个按钮。
func (s *Session) drawMKTargetSelect(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || canvas == nil {
		return
	}
	state := ui.targetCards
	if state == nil || s.composer == nil {
		return
	}
	if ui.backdrop != nil {
		canvas.BlitCanvasRegion(ui.backdrop, 0, 0, ui.backdrop.Width, ui.backdrop.Height, 0, 0, 255)
	}
	// 和学園2 的目标选择一致：压暗底图，卡片底部的黑色名牌才读得清。
	canvas.FillBlend(0, 0, s.width, s.height, 0, 0, 0, 140)

	if state.page >= 0 && state.page < len(state.pages) {
		for slot, card := range state.pages[state.page] {
			image := s.composer.Load(card.image)
			if image == nil {
				continue
			}
			canvas.Draw(image, card.x, card.y)
			global := state.page*mkTargetCardsPerPage + slot
			// 卡片是单帧图，没有悬停/选中帧，只能用描边给反馈：
			// 金色描边表示已经点选、等待「決定」，白色表示鼠标正压在卡上。
			if global == state.selected {
				drawMKTargetOutline(canvas, card.x, card.y, mkTargetCardWidth, mkTargetCardHeight, 0, 200, 255)
			}
			if slot == state.hover {
				drawMKTargetOutline(canvas, card.x, card.y, mkTargetCardWidth, mkTargetCardHeight, 255, 255, 255)
			}
		}
	}

	s.drawMKTargetNav(canvas, "tgt_prev", mkTargetPrevX, mkTargetNavY, state.hoverNav == mkTargetNavPrev)
	s.drawMKTargetNav(canvas, "tgt_next", mkTargetNextX, mkTargetNavY, state.hoverNav == mkTargetNavNext)
	s.drawMKTargetNav(canvas, "tgt_done", mkTargetDoneX, mkTargetDoneY, state.hoverNav == mkTargetNavDone)
}

// drawMKTargetOutline 在卡片外侧画 2px 描边。放到外侧而不是内侧，
// 是为了不盖住卡片底部的角色名。
func drawMKTargetOutline(canvas *gfx.BgraCanvas, x, y, width, height int, blue, green, red byte) {
	const thickness = 2
	canvas.FillBlend(x-thickness, y-thickness, width+2*thickness, thickness, blue, green, red, 255)
	canvas.FillBlend(x-thickness, y+height, width+2*thickness, thickness, blue, green, red, 255)
	canvas.FillBlend(x-thickness, y, thickness, height, blue, green, red, 255)
	canvas.FillBlend(x+width, y, thickness, height, blue, green, red, 255)
}

// drawMKTargetNav 画一个 208x88 的翻页/決定按钮。悬停时用浅色帧，
// 其余状态统一用带文字的常态帧（0/3 帧是纯色块，不当常态用）。
func (s *Session) drawMKTargetNav(canvas *gfx.BgraCanvas, name string, x, y int, hover bool) {
	image := s.composer.Load(name)
	if image == nil {
		return
	}
	frames := image.Height / mkTargetNavHeight
	if frames <= 0 {
		return
	}
	frame := mkTargetNavFrameNormal
	if hover {
		frame = mkTargetNavFrameHover
	}
	if frame >= frames {
		frame = frames - 1
	}
	canvas.DrawRegion(image, 0, frame*mkTargetNavHeight, image.Width, mkTargetNavHeight, x, y)
}

// hitMKTargetSelect 先判三个固定按钮，再判当前页卡片。
//
// prev/next 和左右两张卡片的中部本来就有重叠（箭头贴在屏幕左右缘），
// 所以箭头必须先命中；否则玩家点箭头时选中的是卡片。
func (s *Session) hitMKTargetSelect(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.targetCards == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.targetCards
	state.hover = -1
	state.hoverNav = -1

	if x >= mkTargetPrevX && x < mkTargetPrevX+mkTargetNavWidth &&
		y >= mkTargetNavY && y < mkTargetNavY+mkTargetNavHeight {
		state.hoverNav = mkTargetNavPrev
		return uiHit{kind: hitTargetCard, index: mkTargetNavPrev}
	}
	if x >= mkTargetNextX && x < mkTargetNextX+mkTargetNavWidth &&
		y >= mkTargetNavY && y < mkTargetNavY+mkTargetNavHeight {
		state.hoverNav = mkTargetNavNext
		return uiHit{kind: hitTargetCard, index: mkTargetNavNext}
	}
	if x >= mkTargetDoneX && x < mkTargetDoneX+mkTargetNavWidth &&
		y >= mkTargetDoneY && y < mkTargetDoneY+mkTargetNavHeight {
		state.hoverNav = mkTargetNavDone
		return uiHit{kind: hitTargetCard, index: mkTargetNavDone}
	}
	if state.page >= 0 && state.page < len(state.pages) {
		for slot, card := range state.pages[state.page] {
			if x >= card.x && x < card.x+mkTargetCardWidth &&
				y >= card.y && y < card.y+mkTargetCardHeight {
				state.hover = slot
				return uiHit{kind: hitTargetCard, index: slot}
			}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateMKTargetCard 处理 MK 的点击：翻页、选中卡片、決定或「止める」。
func (s *Session) activateMKTargetCard(index int) {
	ui := s.ui
	if ui == nil || ui.targetCards == nil || ui.targetCards.mode != "mk" {
		return
	}
	state := ui.targetCards
	switch index {
	case mkTargetNavPrev:
		if state.page > 0 {
			state.page--
			state.hover = -1
			s.composer.Invalidate()
		}
	case mkTargetNavNext:
		if state.page+1 < len(state.pages) {
			state.page++
			state.hover = -1
			s.composer.Invalidate()
		}
	case mkTargetNavDone:
		s.confirmMKTargetCard(state.selected)
	default:
		if state.page < 0 || state.page >= len(state.pages) {
			return
		}
		cards := state.pages[state.page]
		if index < 0 || index >= len(cards) {
			return
		}
		global := state.page*mkTargetCardsPerPage + index
		state.selected = global
		// 「止める」不需要再按一次決定：点它就是要结束本次选择。
		if cards[index].value == mkTargetStopValue {
			s.confirmMKTargetCard(global)
			return
		}
		core.Logf("ui: mk target card page %d slot %d selected (value %d)", state.page, index, cards[index].value)
		s.composer.Invalidate()
	}
}

// confirmMKTargetCard 把全局候选下标对应的值写进 #result，然后像
// activateTargetSelect 一样关界面、让脚本从 SYSCALL 的下一条继续。
func (s *Session) confirmMKTargetCard(global int) {
	ui := s.ui
	if ui == nil || ui.targetCards == nil || global < 0 {
		return
	}
	state := ui.targetCards
	page := global / mkTargetCardsPerPage
	slot := global % mkTargetCardsPerPage
	if page < 0 || page >= len(state.pages) || slot < 0 || slot >= len(state.pages[page]) {
		return
	}
	value := state.pages[page][slot].value
	s.writeOne("#result", -1, value)
	core.Logf("ui: mk target select -> #result=%d", value)
	s.closeSystemUI()
	s.lastStep = s.finishRun(s.run(commandBudget))
}
