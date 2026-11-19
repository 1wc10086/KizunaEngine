// 回想（backlog）的引擎自绘：原版 sq_backlog 的覆盖层由 Go 引擎直接画进帧，
// Flutter 只送点击坐标与滚轮 delta。
//
// 为什么背景不是整图：sq_backlog 先截当前画面，再叠 log_overlay 半透明遮罩，
// 所以 Compose 只叠，不清屏；清了会把已画好的 stage 一起抹掉。
package session

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/menus"
	"kizuna/enginego/core"
)

// 回想文字排版与原版 ADV 正文同口径：56 半角格/行、字号 24、行高 24。
// 每屏约 21 行（ListRect.H=520），backlogOffset 以“行”为单位增减：
// offset=0 表示最新一行贴在文字区底边，offset 增大表示向上翻（看更早）。
const (
	backlogLineHeight   = 24
	backlogFontSize     = 24
	backlogLineCapacity = 520 / backlogLineHeight
	backlogSpeakerColor = 0xFFBFE0FF
)

// backlogLine 是渲染用行：speaker 只在每条的首行非空，续行排版自然回绕。
type backlogLine struct {
	speaker string
	body    string
}

// backlogLines 把 backlog 每条拼成“说话人：正文”并按 56 半角格换行，
// 保留说话人前缀供 Compose 用浅蓝绘制；换行续行的 speaker 字段为空。
func (s *Session) backlogLines() []backlogLine {
	if len(s.backlog) == 0 {
		return nil
	}
	var lines []backlogLine
	for _, unit := range s.backlog {
		if unit.ID == "" && unit.Text == "" {
			lines = append(lines, backlogLine{})
			continue
		}
		if unit.ID != "" {
			prefix := unit.ID + "："
			wrapped := wrapMessage(prefix + unit.Text)
			if len(wrapped) == 0 {
				wrapped = []string{prefix}
			}
			first := wrapped[0]
			body := first
			if len(first) >= len(prefix) && first[:len(prefix)] == prefix {
				body = first[len(prefix):]
			}
			lines = append(lines, backlogLine{speaker: prefix, body: body})
			for _, cont := range wrapped[1:] {
				lines = append(lines, backlogLine{body: cont})
			}
			continue
		}
		wrapped := wrapMessage(unit.Text)
		if len(wrapped) == 0 {
			wrapped = []string{unit.Text}
		}
		for _, ln := range wrapped {
			lines = append(lines, backlogLine{body: ln})
		}
	}
	return lines
}

// backlogMaxOffset 是偏移上限：总行数减一屏容量，全屏以下内容不允许滚动。
func (s *Session) backlogMaxOffset() int {
	max := len(s.backlogLines()) - backlogLineCapacity
	if max < 0 {
		return 0
	}
	return max
}

// clampBacklogOffset 把偏移夹到 0..最大行数，避免越界白屏。
func (s *Session) clampBacklogOffset() {
	if s.backlogOffset < 0 {
		s.backlogOffset = 0
	}
	if max := s.backlogMaxOffset(); s.backlogOffset > max {
		s.backlogOffset = max
	}
}

// composeBacklogOverlay 叠回想：直接画在已合成好的 stage 上，不清屏。
// log_overlay 是半透明遮罩；Compose 之前已画过 stage，清屏会把画面抹掉。
func (s *Session) composeBacklogOverlay(canvas *bishopgfx.BgraCanvas) {
	layout := menus.BacklogLayoutFor()

	// 顺序与原版一致：overlay(0,0)、slider(746,40)、up、down、return、pin、文字。
	s.drawImage(canvas, layout.Overlay, 0, 0)
	s.drawImage(canvas, layout.Slider, int(layout.SliderRect.X), int(layout.SliderRect.Y))
	s.drawImage(canvas, layout.Up, int(layout.UpRect.X), int(layout.UpRect.Y))
	s.drawImage(canvas, layout.Down, int(layout.DownRect.X), int(layout.DownRect.Y))
	s.drawImage(canvas, layout.Return, int(layout.ReturnRect.X), int(layout.ReturnRect.Y))

	lines := s.backlogLines()
	s.clampBacklogOffset()

	// log_pin：x=746，y=60+scrollRatio*471（slider 区 40..565 高 525，pin 高 14）。
	// scrollRatio 以文档位置（当前屏第一行）计：offset=0 看到的是文末最新行，
	// pin 在底（60+471=531）；offset=max 看到文首最老行，pin 到顶（60）。
	// 这样 up（看更早）时 pin 向上移，与滚动条的物理位置一致。
	var pinY int
	maxOffset := s.backlogMaxOffset()
	if maxOffset > 0 {
		pinY = 60 + (maxOffset-s.backlogOffset)*471/maxOffset
	} else {
		pinY = 60 + 471
	}
	s.drawImage(canvas, layout.Pin, 746, pinY)

	if len(lines) == 0 {
		return
	}

	lr := layout.ListRect
	visible := backlogLineCapacity
	if visible > len(lines)-s.backlogOffset {
		visible = len(lines) - s.backlogOffset
	}
	if visible <= 0 {
		return
	}
	// 从底部向上：最新一行贴 ListRect 底边，offset 大表示往回翻。
	start := len(lines) - s.backlogOffset - visible
	if start < 0 {
		start = 0
	}
	baseY := int(lr.Y) + (int(lr.H) - visible*backlogLineHeight)
	for i := 0; i < visible; i++ {
		line := lines[start+i]
		if line.speaker == "" && line.body == "" {
			continue
		}
		y := baseY + i*backlogLineHeight
		x := int(lr.X)
		if line.speaker != "" {
			s.drawText(canvas, line.speaker, x, y, backlogFontSize, backlogLineHeight, int(lr.W), 1, backlogSpeakerColor)
			spW := textWidth(line.speaker, backlogFontSize)
			rem := int(lr.W) - spW
			if rem > 0 {
				s.drawText(canvas, line.body, x+spW, y, backlogFontSize, backlogLineHeight, rem, 1, 0xFFFFFFFF)
			}
		} else {
			s.drawText(canvas, line.body, x, y, backlogFontSize, backlogLineHeight, int(lr.W), 1, 0xFFFFFFFF)
		}
	}
}

// pointerBacklogOverlay 处理回想上的点击：up 看更早、down 看最新，
// 各翻一屏（backlogLineCapacity 行）；return 与点空（遮罩区）都关层。
// 点空也关是因为回想是纯阅读层，没有其他可点区，误触不穿透到 ADV。
func (s *Session) pointerBacklogOverlay(x, y int32) (core.Step, bool) {
	layout := menus.BacklogLayoutFor()
	if hit(layout.ReturnRect, x, y) {
		s.backlogDragY = 0
		return s.closeOverlay(), true
	}
	if hit(layout.UpRect, x, y) {
		s.backlogOffset += backlogLineCapacity
		s.clampBacklogOffset()
		return s.step, true
	}
	if hit(layout.DownRect, x, y) {
		s.backlogOffset -= backlogLineCapacity
		s.clampBacklogOffset()
		return s.step, true
	}
	// 文字区点击不做任何事；回想只由 return / 右键退出。
	return s.step, true
}

// dragBacklog 把纵向拖动换算成 backlogOffset 增量。
// backlogDragY 是上一帧 y，首次调用只记起点：Pointer op==6 逐帧回调，
// 直接用全局 y 当位移会把坐标系跳变吃成整屏跳动。注意 Session.Pointer 的
// 抬手（op==2）只清 configDrag，不清 backlogDragY：靠点击（pointer 回想）
// 与关层留下的清零兜底；宿主通常在拖动前发 press 也能兜底。
// 方向：向上拖（dy<0）看更早（offset 增大）；行高 24 换算成行数，像素抖动
// 至少 1 行封底，避免不滚或滚过整屏。
func (s *Session) dragBacklog(y int32) core.Step {
	if s.backlogDragY == 0 {
		s.backlogDragY = int(y)
		return s.step
	}
	dy := int(y) - s.backlogDragY
	s.backlogDragY = int(y)
	if dy == 0 {
		return s.step
	}
	delta := -dy / backlogLineHeight
	if delta == 0 {
		if dy < 0 {
			delta = 1
		} else {
			delta = -1
		}
	}
	s.backlogOffset += delta
	s.clampBacklogOffset()
	return s.step
}

// Wheel 实现 core.WheelReceiver：滚轮向上（delta>0）看更早，delta*3 行/挡。
// 非 backlog 覆盖层直接返回当前 step，宿主滚轮不会误触其他界面。
func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.overlay != overlayBacklog {
		return s.step, nil
	}
	s.backlogOffset += int(delta) * 3
	s.clampBacklogOffset()
	return s.step, nil
}
