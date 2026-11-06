package session

import (
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// targetselect_spmm.go 是「スレイブポリス みちる＆美鈴」（SPMM，Rev.3）
// 的「ターゲット選択」画面。
//
// 剧本在 Action 里先 `system call "target"`（名字表 index 12），随后是
// 「決定／選び直す」两个 callback 的 SELECT。脚本随后按 #target 分派：
// 1 走 Action_sub、2 走 Mtr_select、3 走 Msz_select（verified from the
// bsx.dat dispatch directly after the syscall: slot 15 is #target）。
//
// 卡片数值就是卡片序号：tgt_1 是「みちるを探しに行く」写 1，tgt_2 是
// 「美鈴を探しに行く」写 2，tgt_3 是「別の事をする」写 3。spmm.exe 的
// 字符串池顺序是 tgt_3/tgt_2/tgt_1，正好反过来，那只是资源池的
// 排布，不是指标映射；不能按竖排菜单顺序重映射。
//
// 三张卡都是 300x60 的可用区域，但每张 BSG 内部竖排了三帧
// （常态/悬停/按下，图头 FrameCount 没写实）；引擎必须按 60px 切帧，
// 否则会把三帧一次铺成 180px 高，画面上出现三份重复选项。
// 竖排居中，悬停画第 1 帧。原版没有压暗整屏，所以这里只回放 backdrop。

const (
	spmmTargetCardWidth  = 300
	spmmTargetCardHeight = 60
	spmmTargetCardFrames = 3
)

// spmmTargetCards 是三张可选卡及其写回 #target 的值：卡号即数值。
var spmmTargetCards = []struct {
	image string
	value int
}{
	{"tgt_1", 1},
	{"tgt_2", 2},
	{"tgt_3", 3},
}

// beginTargetSpmmSelect 在识别到 SPMM 的三张目标卡时接管目标选择。
// 返回 true 表示已进入等待输入；素材指纹不命中时返回 false，
// 调用方继续走原来的分支，其它作品完全不受影响。
func (s *Session) beginTargetSpmmSelect() bool {
	if s.script == nil || s.machine == nil || s.composer == nil || s.res == nil {
		return false
	}
	for _, card := range spmmTargetCards {
		if !s.res.Exists(card.image) {
			return false
		}
	}
	// 这套 tgt_1..3 如果还混着学園2（GK2）的 tgt_hrn/tgt_quit 或
	// 放課後2 的 target_card00，就不是 SPMM 的指纹；宁可退回旧逻辑
	// 也不要抢走别的作品的目标选择。
	if s.res.Exists("tgt_hrn") || s.res.Exists("tgt_quit") || s.res.Exists("target_card00") {
		return false
	}
	if s.script.GetPropertyIndex("#target") < 0 {
		return false
	}
	s.composer.Invalidate()
	s.composer.Compose(s.gameState)
	s.beginSystemUI(uiTargetCard)
	if s.ui == nil {
		return false
	}
	cards := make([]targetCardChoice, 0, len(spmmTargetCards))
	x := (s.width - spmmTargetCardWidth) / 2
	steps := spmmTargetCardHeight * len(spmmTargetCards)
	y := (s.height - steps) / 2
	if y < 0 {
		y = 0
	}
	if x < 0 {
		x = 0
	}
	for index, card := range spmmTargetCards {
		cards = append(cards, targetCardChoice{
			image: card.image, value: card.value,
			x: x, y: y + index*spmmTargetCardHeight,
		})
	}
	s.ui.targetCards = &targetCardState{
		mode:  "spmm",
		cards: cards,
		hover: -1,
	}
	// 点卡只完成“选谁”，脚本随后还会发一次「決定／選び直す」的
	// SELECT；这里不自动确认，選び直す 会重新进本画面。
	core.Logf("ui: spmm target select %d card(s)", len(cards))
	return true
}

func (s *Session) drawSpmmTargetSelect(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || canvas == nil || ui.targetCards == nil || s.composer == nil {
		return
	}
	state := ui.targetCards
	if ui.backdrop != nil {
		canvas.BlitCanvasRegion(ui.backdrop, 0, 0, ui.backdrop.Width, ui.backdrop.Height, 0, 0, 255)
	}
	for index, card := range state.cards {
		image := s.composer.Load(card.image)
		if image == nil {
			continue
		}
		frameHeight := image.Height / spmmTargetCardFrames
		if frameHeight <= 0 {
			continue
		}
		frame := 0
		if index == state.hover {
			frame = 1
			if frame >= spmmTargetCardFrames {
				frame = 0
			}
		}
		canvas.DrawRegion(image, 0, frame*frameHeight,
			image.Width, frameHeight, card.x, card.y)
		if index == state.hover {
			// 悬停帧本身已经有高亮，再补一圈细描边保证触屏上也能看清。
			drawMKTargetOutline(canvas, card.x, card.y, spmmTargetCardWidth, spmmTargetCardHeight, 255, 255, 255)
		}
	}
}

func (s *Session) hitSpmmTargetSelect(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.targetCards == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.targetCards
	state.hover = -1
	for index, card := range state.cards {
		if x >= card.x && x < card.x+spmmTargetCardWidth &&
			y >= card.y && y < card.y+spmmTargetCardHeight {
			state.hover = index
			return uiHit{kind: hitTargetCard, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateSpmmTargetCard(index int) {
	ui := s.ui
	if ui == nil || ui.targetCards == nil || ui.targetCards.mode != "spmm" {
		return
	}
	state := ui.targetCards
	if index < 0 || index >= len(state.cards) {
		return
	}
	value := state.cards[index].value
	s.writeTargetValue(value)
	core.Logf("ui: spmm target -> #target=%d", value)
	s.choicePickedAt = time.Now()
	s.closeSystemUI()
	s.lastStep = s.finishRun(s.run(commandBudget))
}
