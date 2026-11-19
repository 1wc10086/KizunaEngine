// 选择肢菜单（原版 SELECT.JPG）：sys_select 的 928x201 三联图 +
// [SELECT] 段的坐标，选项文本居中画在每一行。
//
// 为什么必须由引擎画：原版把选择肢画在场地中央（SELECT_POSY=280，每行
// 67px、行距 4px），并且这一层在绘图上盖住对话框；Flutter 的
// ChoicesOverlay 是居中的一列 Material 按钮，位置、字号、底图都不是原版。
package session

import (
	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/core"
)

// choiceRowHeight 返回单行高度（[SELECT] SELECT_HEIGHT，缺省 67）。
func (s *Session) choiceRowHeight() int {
	if s.ui == nil || s.ui.Select.Height <= 0 {
		return 67
	}
	return s.ui.Select.Height
}

// choiceOrigin 返回第一行的屏幕坐标。
func (s *Session) choiceOrigin() (int, int) {
	if s.ui == nil {
		return 0, 280
	}
	width := s.ui.Select.Width
	if width <= 0 {
		width = 928
	}
	return (frameWidth - width) / 2, s.ui.Select.PosY
}

// choiceRect 返回第 index 行的矩形。
func (s *Session) choiceRect(index int) config.Rect {
	row := s.choiceRowHeight()
	step := row + 4
	if s.ui != nil && s.ui.Select.NextY > 0 {
		step = row + s.ui.Select.NextY
	}
	x, y := s.choiceOrigin()
	return config.Rect{X: x, Y: y + index*step, W: s.choiceWidth(), H: row}
}

func (s *Session) choiceWidth() int {
	if s.ui == nil || s.ui.Select.Width <= 0 {
		return 928
	}
	return s.ui.Select.Width
}

// composeChoiceMenu 画选项菜单。
//
// sys_select 是 928x201 的整块图，里面叠了**三行**（每行 67px、行距 4px）：
// 第 1 行常态、第 2 行悬停高亮、第 3 行按下。所以每一行都贴整块图里的
// 同一行帧（frame*67），而不是按行号去找不同帧。
//
// 行高来自 [SELECT] 的 SELECT_HEIGHT（67）；SELECT_POSY=280 是第一行的 y。
// 原版每行足够宽（928），文本按 SELECT_TEXT_TYPE 居中。
func (s *Session) composeChoiceMenu() {
	if s.step != core.StepAwaitChoice || len(s.choices) == 0 {
		return
	}
	img := s.cache.Get("sys_select")
	row := s.choiceRowHeight()
	size := 24
	plusY := 0
	if s.ui != nil {
		if s.ui.Select.FontSize > 0 {
			size = s.ui.Select.FontSize
		}
		plusY = s.ui.Select.TextPlusY
	}
	for index, choice := range s.choices {
		rect := s.choiceRect(index)
		frame := 0
		if index == s.choiceHover {
			frame = 1
		}
		if img != nil {
			// 帧是"同一块图的第 N 行"：sys_select 的 201 行 = 3 × 67，
			// 第 0 行常态、第 1 行悬停、第 2 行按下。
			srcY := frame * row
			if srcY+row > img.Height {
				srcY = 0
			}
			// 帧的宽度是整块图的 928；行高按图里的 67 裁（配置的 SELECT_HEIGHT
			// 与它一致，但以图为准更稳）。
			s.stage.DrawRegion(img, 0, srcY, img.Width, row, rect.X, rect.Y)
		}
		// 文本：水平居中、垂直居中。
		textWidth := len([]rune(choice.Name)) * size
		x := rect.X + (rect.W-textWidth)/2
		if x < rect.X+8 {
			x = rect.X + 8
		}
		s.drawUIRun(choice.Name, x, rect.Y+row/2-size/2+plusY, size, 0xffffffff, rect.W-16)
	}
}

// choiceHit 返回命中的选项下标（-1 表示没命中）。
func (s *Session) choiceHit(x, y int) int {
	for index := range s.choices {
		if s.choiceRect(index).Contains(x, y) {
			return index
		}
	}
	return -1
}
