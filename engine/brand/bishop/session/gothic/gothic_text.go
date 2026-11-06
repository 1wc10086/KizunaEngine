package gothic

import (
	"strings"
	"time"

	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

// appendText 只把一行正文放进当前页，不在这里等待玩家。
//
// MAZE 的剧本记录本身就是“一行显示文本”，真正的停顿点由后面的
// `%` / `%%` 记录给出：一个段落通常 1~5 行，段落结束时才需要点击。
// 早期实现每读一行就返回 AwaitInput，玩家必须为同一段里的每个折行
// 各点一次，节奏与原引擎完全不同。
func (s *Session) appendText(text string) core.Step {
	line := text
	if len(strings.TrimSpace(line)) == 0 || strings.TrimSpace(line) == "$" {
		return core.StepContinue
	}
	for _, row := range wrapGothicRows(line, s.textColumns()) {
		if len([]rune(row)) == 0 {
			continue
		}
		s.pageLines = append(s.pageLines, row)
	}
	limit := s.textRowsForPage()
	for len(s.pageLines) > limit {
		s.pageLines = s.pageLines[1:]
	}
	s.syncMessage()
	if s.profile != gothicProfileMaze {
		// Toku1 仍按旧节奏：每读一行就停一次。它的剧本把折行和停顿
		// 混在同一个 % 里，改成分页会让玩家一次看到过多文本。
		s.gameState.SetAwaitingInput(true)
		return core.StepAwaitInput
	}
	// MAZE：文本先显示出来，停不停由下一个 %/%% 决定。
	s.gameState.SetAwaitingInput(false)
	return core.StepContinue
}

// pageWait 是段落结束：当前页保持在屏幕上，等玩家点击后再清页继续。
// 等待与清页分开是必要的——如果先清再停，玩家永远看不到最后一段。
func (s *Session) pageWait() core.Step {
	if s.profile != gothicProfileMaze {
		s.syncMessage()
		s.gameState.SetAwaitingInput(true)
		return core.StepAwaitInput
	}
	if len(s.pageLines) == 0 && !s.messageVisible() {
		return core.StepContinue
	}
	s.syncMessage()
	s.pageBreakPending = true
	s.startReveal()
	s.gameState.SetAwaitingInput(true)
	return core.StepAwaitInput
}

// segmentBreak 与 pageWait 同义。MAZE 的 `%` 与 `%%` 都是段末停顿，
// 区别只在作者的分段习惯；把两者都当段末可以保持所有文本可见。
func (s *Session) segmentBreak() core.Step {
	if s.profile != gothicProfileMaze {
		s.syncMessage()
		s.pageLines = s.pageLines[:0]
		s.gameState.SetAwaitingInput(true)
		return core.StepAwaitInput
	}
	return s.pageWait()
}

// clearPage 在玩家点击推进后清掉上一页。字形、说话人和消息状态一起清，
// 避免下一页继承上一段的名字或残留行。
func (s *Session) clearPage() {
	s.pageLines = s.pageLines[:0]
	s.gameState.SetText("")
	s.stage.SetMessageLines(nil)
	s.stage.SetMessageReveal(-1)
	s.pageBreakPending = false
	s.revealActive = false
	s.revealLimit = -1
}

// revealSpeed 返回每秒显示的字数。0 表示整页直接显示（ページ単位）。
func (s *Session) revealSpeed() int {
	if s.global == nil || s.profile != gothicProfileMaze {
		return 0
	}
	switch s.global.TextSpeed {
	case 1:
		return 90
	case 2:
		return 45
	case 3:
		return 20
	default:
		return 0
	}
}

func (s *Session) startReveal() {
	if s.profile != gothicProfileMaze {
		return
	}
	total := 0
	for _, line := range s.pageLines {
		total += len([]rune(line))
	}
	s.revealTotal = total
	if total == 0 || s.revealSpeed() <= 0 {
		s.revealActive = false
		s.revealLimit = -1
		s.stage.SetMessageReveal(-1)
		return
	}
	s.revealStart = time.Now()
	s.revealActive = true
	s.revealLimit = 0
	s.stage.SetMessageReveal(0)
}

func (s *Session) finishReveal() {
	s.revealActive = false
	s.revealLimit = -1
	s.stage.SetMessageReveal(-1)
	s.stage.Invalidate()
}

func (s *Session) updateReveal() {
	if !s.revealActive {
		return
	}
	speed := s.revealSpeed()
	if speed <= 0 {
		s.finishReveal()
		return
	}
	elapsed := time.Since(s.revealStart).Seconds()
	limit := int(elapsed * float64(speed))
	if limit >= s.revealTotal {
		s.finishReveal()
		return
	}
	if limit != s.revealLimit {
		s.revealLimit = limit
		s.stage.SetMessageReveal(limit)
		s.stage.Invalidate()
	}
}

func (s *Session) showMenu(menu *vmgothic.GothicMenu) core.Step {
	if menu == nil || menu.Items == nil || len(menu.Items) == 0 {
		core.Logf("gothic: menu without items, continuing")
		return core.StepContinue
	}
	texts := make([]string, 0, len(menu.Items))
	for _, item := range menu.Items {
		if item == nil {
			texts = append(texts, "")
			continue
		}
		texts = append(texts, item.Text)
	}
	s.gameState.SetChoices(texts)
	s.stage.SetMenuItems(texts)
	if s.profile == gothicProfileMaze {
		s.choiceActive = true
		s.choiceItems = append([]string{}, texts...)
		return core.StepAwaitSelect
	}
	return core.StepAwaitChoice
}

func (s *Session) clearMessage() {
	s.pageLines = s.pageLines[:0]
	s.gameState.SetText("")
	s.gameState.SetSpeaker("")
	s.stage.SetMessageLines(nil)
	s.pageBreakPending = false
}

func (s *Session) messageVisible() bool {
	return s.frameMain || s.frameUpper || s.frameLower
}

func (s *Session) textColumns() int {
	if s.isToku1() {
		return GothicToku1TextColumns
	}
	// MAZE 的剧本行已经按 24 个全角字排好；给 40 列只是上限，
	// 防止 25 字行被误折成两行。宽度仍受 640px 屏幕约束。
	return 40
}

func (s *Session) syncMessage() {
	s.gameState.SetText(strings.Join(s.pageLines, "\n"))
	s.stage.SetMessageLines(s.pageLines)
}

func (s *Session) textRowsForPage() int {
	return GothicTextRowsFor(s.isToku1())
}

func wrapGothicRows(text string, columns int) []string {
	rows := []string{}
	current := &strings.Builder{}
	count := 0
	for _, r := range text {
		if r == '\n' {
			rows = append(rows, current.String())
			current.Reset()
			count = 0
			continue
		}
		if r == '\r' {
			continue
		}
		current.WriteRune(r)
		count++
		if count >= columns {
			rows = append(rows, current.String())
			current.Reset()
			count = 0
		}
	}
	if current.Len() > 0 {
		rows = append(rows, current.String())
	}
	if len(rows) == 0 {
		rows = append(rows, "")
	}
	return rows
}
