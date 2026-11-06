package session

import (
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// targetselect_houkago2.go 是「放課後2～白濁のレッスン～」（HOUKAGO2，Rev.5）
// 的「ターゲット選択」画面。
//
// 剧本在 Select_target 里先 `system call "target"`，随后是只有
// 「ＯＫ／選び直す」两个 callback 的 SELECT。真正决定今天找谁的是前一步：
// 原引擎在这里铺 8 张 target_card01..08，点卡片把值写回 #result，再回到
// 脚本的 OK/選び直す 让玩家确认一次（選び直す 会重新进这张画面）。
//
// 布局取自 af2.exe 的 target card 布局函数（0x43b100）：
//   - 第一排 5 张：x=64+136i、y=40；
//   - 第二排最多 3 张：y=200，x 从左到右是 200/336/472（8 人时）；
//   - 「午後へ」按钮 target_exit 在 (304,412)，点击写 #result=9。
//
// target_card00 是卡背，用于翻牌/未解锁的占位，不参与选择；把它留在素材
// 指纹里只是为了保证没有误判到别的世代。
const (
	houkago2TargetCardWidth  = 128
	houkago2TargetCardHeight = 200
	houkago2TargetCardStep   = 136
	houkago2TargetRowTop     = 40
	houkago2TargetRowBottom  = 200
	houkago2TargetRow2X      = 200
	// 点击“決定”等价于脚本里的 9（午後へ / 今天不找人）。
	houkago2TargetExitIndex = 100
	houkago2TargetExitX     = 304
	houkago2TargetExitY     = 412
	houkago2TargetExitW     = 192
	houkago2TargetExitH     = 48
)

// houkago2TargetCards 是八张可选卡及其写回 #result 的值。
//
// 01..08 依次是 碧/紗由理/圭/花梨/薫子/三人组合 H1/H2/H3；原版的
// #result 就是卡片序号（1..8），9 留给 target_exit。
var houkago2TargetCards = []struct {
	image string
	value int
}{
	{"target_card01", 1},
	{"target_card02", 2},
	{"target_card03", 3},
	{"target_card04", 4},
	{"target_card05", 5},
	{"target_card06", 6},
	{"target_card07", 7},
	{"target_card08", 8},
}

// beginTargetHoukago2Select 在识别到放課後2 的 target_card 素材时接管目标选择。
// 返回 true 表示已进入等待输入。
func (s *Session) beginTargetHoukago2Select() bool {
	if s.script == nil || s.machine == nil || s.composer == nil || s.res == nil {
		return false
	}
	// 八张角色卡缺任意一张都不能给出完整选择；缺卡时退回原来的“继续”，
	// 宁可少一屏 UI 也不要让玩家点到空位。
	for _, card := range houkago2TargetCards {
		if !s.res.Exists(card.image) {
			core.Logf("ui: houkago2 target asset '%s' is missing, falling back", card.image)
			return false
		}
	}
	// 选中框/決定按钮/卡背也必须存在，否则这不是放課後2 的一套目标卡。
	if !s.res.Exists("target_card_cur") || !s.res.Exists("target_exit") ||
		!s.res.Exists("target_card00") {
		core.Logf("ui: houkago2 target highlight/exit/back asset is missing, falling back")
		return false
	}
	if s.script.GetPropertyIndex("#result") < 0 {
		return false
	}
	// 玩家点「選び直す」时，上一帧还画着 OK/選び直す 的选择窗；直接拿它当
	// 背景会把选择条带进目标画面。先按当前 GameState 重合成一帧（不含系统
	// 覆盖层），拿到干净的地图/剧情背景再抓 backdrop。
	if s.composer != nil && s.gameState != nil {
		s.composer.Invalidate()
		s.composer.Compose(s.gameState)
	}
	s.beginSystemUI(uiTargetCard)
	if s.ui == nil {
		return false
	}
	cards := make([]targetCardChoice, 0, len(houkago2TargetCards))
	for index, card := range houkago2TargetCards {
		x, y := 64+houkago2TargetCardStep*index, houkago2TargetRowTop
		if index >= 5 {
			// 第二排按原版只放 3 张，从左到右 200/336/472。
			x = houkago2TargetRow2X + houkago2TargetCardStep*(index-5)
			y = houkago2TargetRowBottom
		}
		cards = append(cards, targetCardChoice{image: card.image, value: card.value, x: x, y: y})
	}
	s.ui.targetCards = &targetCardState{
		mode:  "houkago2",
		cards: cards,
		hover: -1,
	}
	// 原版点完卡还要在「ＯＫ／選び直す」里确认一次；这里绝不能自动确认，
	// 否则「選び直す」分支会被吞掉，和原引擎的手感不一致。
	s.targetAutoConfirm = false
	core.Logf("ui: houkago2 target select %d card(s)", len(cards))
	return true
}

// drawHoukago2TargetSelect 保留进入前的剧情帧，再铺八张卡和決定按钮。
//
// 原版没有压暗整屏（target 画面直接叠在地图/对话上），所以这里只回放背景，
// 不再补 FillBlend 的黑色蒙版。
func (s *Session) drawHoukago2TargetSelect(canvas *gfx.BgraCanvas) {
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
		canvas.DrawRegion(image, 0, 0, image.Width, image.Height, card.x, card.y)
		if index == state.hover {
			// target_card_cur 是带高亮边框的透明叠层，正好盖住整张卡。
			if overlay := s.composer.Load("target_card_cur"); overlay != nil {
				canvas.DrawRegion(overlay, 0, 0, overlay.Width, overlay.Height, card.x, card.y)
			}
		}
	}
	exit := "target_exit"
	if state.hover == houkago2TargetExitIndex {
		exit = "target_exit_cur"
	}
	if image := s.composer.Load(exit); image != nil {
		canvas.DrawRegion(image, 0, 0, image.Width, image.Height,
			houkago2TargetExitX, houkago2TargetExitY)
	}
}

func (s *Session) hitHoukago2TargetSelect(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.targetCards == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.targetCards
	state.hover = -1
	if x >= houkago2TargetExitX && x < houkago2TargetExitX+houkago2TargetExitW &&
		y >= houkago2TargetExitY && y < houkago2TargetExitY+houkago2TargetExitH {
		state.hover = houkago2TargetExitIndex
		return uiHit{kind: hitTargetCard, index: houkago2TargetExitIndex}
	}
	// 两排卡片在 y=200..240 重叠 40px，第二排画在上层（绘制顺序 0→7），
	// 所以命中必须倒序，否则点第二排顶部会误选到同列的第一排。
	for index := len(state.cards) - 1; index >= 0; index-- {
		card := state.cards[index]
		if x >= card.x && x < card.x+houkago2TargetCardWidth &&
			y >= card.y && y < card.y+houkago2TargetCardHeight {
			state.hover = index
			return uiHit{kind: hitTargetCard, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateHoukago2TargetCard 写回 #result 后继续跑脚本。
//
// 原版点卡片只完成“选谁”，接下来脚本会弹出「ＯＫ／選び直す」；
// 这里不设置 targetAutoConfirm，让 AwaitChoice 正常显示那次确认，
// 选「選び直す」时脚本会重新进入本画面。
func (s *Session) activateHoukago2TargetCard(index int) {
	ui := s.ui
	if ui == nil || ui.targetCards == nil || ui.targetCards.mode != "houkago2" {
		return
	}
	state := ui.targetCards
	// 決定/背面カード = 午後へ（脚本里的 #result=9）。
	value := 9
	if index != houkago2TargetExitIndex {
		if index < 0 || index >= len(state.cards) {
			return
		}
		value = state.cards[index].value
	}
	s.writeOne("#result", -1, value)
	core.Logf("ui: houkago2 target -> #result=%d", value)
	// 宿主可能把一次点按拆成 press→release；这里紧接着继续脚本，
	// 抢在 release 前记下时间，250ms 内的 release 只当抬手。
	s.choicePickedAt = time.Now()
	s.closeSystemUI()
	s.lastStep = s.finishRun(s.run(commandBudget))
}
