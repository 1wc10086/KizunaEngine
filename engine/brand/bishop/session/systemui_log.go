package session

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_log.go 是引擎自绘的回想/バックログ界面。
//
// 宿主不参与排版：底图、滚动条、每句的语音图标都来自 bsxx.cfg 的 [log screen]，
// 文本用 [default font] 由引擎渲染。Flutter 只负责把帧画出来和转发点击。
// 这样回想里的行距、可点击语音、滚动手感都和原版一致，也不会出现
// "游戏是日文字体、回想是 Flutter 字体"的割裂。

type logState struct {
	entries    []core.TextUnit
	scroll     int
	voiceRects []logVoiceRect
}

type logVoiceRect struct {
	index int
	x, y  int
	w, h  int
}

func (s *Session) openLogScreen() bool {
	if s.config == nil || s.config.Log.Empty() {
		return false
	}
	if s.ui == nil {
		if !s.canOpenSystemUI() {
			return false
		}
		s.beginSystemUI(uiLog)
	} else {
		s.ui.kind = uiLog
		// 从其它系统界面切进回想时清掉上一页的拖动状态。
		s.ui.lastDragY = -1
		s.ui.logDragRemainder = 0
	}
	entries := s.TextList()
	state := &logState{entries: entries}
	if lines := s.config.Log.Lines; len(entries) > lines {
		state.scroll = len(entries) - lines
	}
	s.ui.log = state
	s.composer.Invalidate()
	core.Logf("ui: log screen opened (%d entries)", len(entries))
	return true
}

func (s *Session) drawLogScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.log == nil || s.config == nil {
		return
	}
	if s.textFont == nil {
		return
	}
	layout := s.config.Log
	state := ui.log
	// 底图缺失时不是黑屏：新版作品把 `base` 注释掉，只给 back color。
	// 回想是叠加在当前剧情画面上的半透明层，所以先恢复打开界面时抓下的
	// backdrop，再把背色叠上去；没有 backdrop（主界面直接打开）时就是
	// 纯背色，和原版在标题上打开回想的表现一致。
	if len(layout.Base) > 0 {
		s.drawImageAt(canvas, layout.Base, 0, 0)
	} else if ui.backdrop != nil {
		canvas.CopyFrom(ui.backdrop)
	}
	if layout.HasBackColor {
		color := layout.BackColor
		canvas.Overlay(byte(color), byte(color>>8), byte(color>>16), int(color>>24))
	}

	font := s.config.Font
	if layout.FontSize > 0 {
		font.Size = layout.FontSize
		font.Height = max(layout.FontSize+4, 18)
	}
	x := layout.LeftPos
	width := layout.Width
	// 回想按固定行高排版：边框和语音图标都属于"行"，不再跟着文字高度累积，
	// 否则一句长对白会把后面的行推下去，和底图、滚动条对不上。
	rowHeight := canvas.Height / max(1, layout.Lines)
	state.voiceRects = state.voiceRects[:0]
	for index := state.scroll; index < len(state.entries); index++ {
		if index-state.scroll >= layout.Lines {
			break
		}
		entry := state.entries[index]
		rowY := (index - state.scroll) * rowHeight

		// 有语音的行铺该行的边框：log_frame 是"一行高"的全宽图
		// （牝教師3 1366x187 / 特別授業3SLG 1366x129），不是整屏拼图。
		// 行高来自 lines 换算，图片比行高时只裁出本行的部分，避免压到下一行。
		if entry.File != "" && layout.VoiceFrame != "" {
			if image := s.composer.Load(layout.VoiceFrame); image != nil {
				height := min(rowHeight, image.Height)
				if height > 0 {
					canvas.DrawRegion(image, 0, 0, image.Width, height, 0, rowY)
				}
			}
		}

		nameImage := s.textFont.Render(entry.ID, font.Size, font.Edge, font.Shadow,
			font.Height, width, 1, 0xffff8822)
		textImage := s.textFont.Render(entry.Text, font.Size, font.Edge, font.Shadow,
			font.Height, width, 3, 0xffffffff)
		nameH, textH := 0, 0
		if nameImage != nil {
			nameH = nameImage.Bounds().Dy()
		}
		if textImage != nil {
			textH = textImage.Bounds().Dy()
		}
		total := nameH + 2 + textH
		y := rowY + max(0, (rowHeight-total)/2)
		rendering.DrawTextImage(canvas, nameImage, x, y)
		rendering.DrawTextImage(canvas, textImage, x, y+nameH+2)

		// 语音图标只画不透明内容：原图带透明边距，整张画会压到文字上。
		if entry.File != "" && layout.VoiceIcon != "" {
			if image := s.composer.Load(layout.VoiceIcon); image != nil {
				bounds := image.ContentBounds()
				if bounds.Width > 0 && bounds.Height > 0 {
					// 语音图标贴在正文列左侧的空白区，不压字；行边框的
					// 左侧留白在不同作品宽度不同，用"正文列 - 图标宽度 -
					// 固定间隙"定位，再夹到屏幕内。
					iconX := layout.LeftPos - bounds.Width - 12
					if iconX < 0 {
						iconX = 0
					}
					iconY := rowY + (rowHeight-bounds.Height)/2
					canvas.DrawRegion(image, bounds.X, bounds.Y, bounds.Width, bounds.Height, iconX, iconY)
					state.voiceRects = append(state.voiceRects, logVoiceRect{
						index: index, x: iconX, y: iconY, w: bounds.Width, h: bounds.Height,
					})
				}
			}
		}
	}

	// 滚动条：底图 + 按 scroll 比例画一个白色滑块。
	if !layout.Bar.Empty() {
		s.drawFrameButton(canvas, layout.Bar, 0)
		if image := s.composer.Load(layout.Bar.Image); image != nil {
			if _, _, trackH, thumbH, maxScroll, ok := s.logBarGeometry(); ok {
				fraction := float64(state.scroll) / float64(max(1, maxScroll))
				thumbY := layout.Bar.Y + int(fraction*float64(max(0, trackH-thumbH)))
				// 滚动条贴着右边缘（SLG 的 bar x=1330），滑块宽度必须
				// 夹到屏幕内，否则最右侧那几像素会被裁掉。
				thumbX := max(0, layout.Bar.X+2)
				thumbWidth := min(max(3, image.Width-4), max(1, canvas.Width-thumbX))
				canvas.Fill(thumbX, thumbY, thumbWidth, thumbH, 255, 255, 255, 190)
			}
		}
	}

	// 上下翻页箭头：2 帧（常态/悬停）。原版点一下滚一行，和滚轮同义。
	if !layout.Up.Empty() {
		frame := 0
		if ui.hover.kind == hitLogUp {
			frame = 1
		}
		s.drawFrameButton(canvas, layout.Up, frame)
	}
	if !layout.Down.Empty() {
		frame := 0
		if ui.hover.kind == hitLogDown {
			frame = 1
		}
		s.drawFrameButton(canvas, layout.Down, frame)
	}

	// 原版回想没有返回按钮，只有右键/ESC 关闭；宿主用工具栏的菜单键发 ESC。
	// 这里不再硬编码 data_back：它的尺寸随作品变化，会压住第一行正文。
}

// logBarGeometry 返回回想滚动条的几何。
//
// 只有内容超过一屏（maxScroll > 0）时才算可拖动：没有可滚的内容却给热区，
// 玩家拖了会以为界面卡住。
func (s *Session) logBarGeometry() (x, y, trackH, thumbH, maxScroll int, ok bool) {
	if s.config == nil || s.ui == nil || s.ui.log == nil {
		return 0, 0, 0, 0, 0, false
	}
	layout := s.config.Log
	if layout.Bar.Empty() || s.composer == nil {
		return 0, 0, 0, 0, 0, false
	}
	image := s.composer.Load(layout.Bar.Image)
	if image == nil {
		return 0, 0, 0, 0, 0, false
	}
	entries := len(s.ui.log.entries)
	trackH = image.Height / max(1, layout.Bar.Frames)
	maxScroll = max(0, entries-layout.Lines)
	thumbH = max(24, trackH*max(1, layout.Lines)/max(1, entries))
	return layout.Bar.X, layout.Bar.Y, trackH, thumbH, maxScroll, maxScroll > 0
}

// scrollLogBy 按行滚动回想。上下箭头一次一行，和原版按钮一致。
func (s *Session) scrollLogBy(delta int) {
	if s.ui == nil || s.ui.log == nil || s.config == nil {
		return
	}
	maxScroll := max(0, len(s.ui.log.entries)-s.config.Log.Lines)
	next := clampInt(s.ui.log.scroll+delta, 0, maxScroll)
	if next == s.ui.log.scroll {
		return
	}
	s.ui.log.scroll = next
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// scrollLogTo 把滚动条滑块中心对到指针 y 上。
//
// 轨道两端之外一律夹紧：不夹的话比例会越界，滑块被画到屏幕外。
func (s *Session) scrollLogTo(y int) {
	_, barY, trackH, thumbH, maxScroll, ok := s.logBarGeometry()
	if !ok || s.ui.log == nil {
		return
	}
	scroll := maxScroll
	if travel := trackH - thumbH; travel > 0 {
		fraction := float64(y-barY-thumbH/2) / float64(travel)
		fraction = max(0, min(1, fraction))
		scroll = int(fraction*float64(maxScroll) + 0.5)
	}
	scroll = clampInt(scroll, 0, maxScroll)
	if scroll == s.ui.log.scroll {
		return
	}
	s.ui.log.scroll = scroll
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

func (s *Session) drawLogImage(canvas *gfx.BgraCanvas, name string, x, y int) {
	if name == "" {
		return
	}
	if image := s.composer.Load(name); image != nil {
		canvas.Draw(image, x, y)
	}
}

func (s *Session) hitLogScreen(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.log == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.log
	// 上下箭头压在最上层，先判它们，避免和滚动条/语音图标抢命中。
	if s.config != nil {
		if !s.config.Log.Up.Empty() && hitButton(s, s.config.Log.Up, x, y) {
			return uiHit{kind: hitLogUp, index: -1}
		}
		if !s.config.Log.Down.Empty() && hitButton(s, s.config.Log.Down, x, y) {
			return uiHit{kind: hitLogDown, index: -1}
		}
	}
	// 滚动条在文字右侧，再判它：按在条上时不能被后面的语音图标/空区域吞掉。
	if s.config != nil && !s.config.Log.Bar.Empty() {
		if image := s.composer.Load(s.config.Log.Bar.Image); image != nil {
			if barX, barY, trackH, _, _, ok := s.logBarGeometry(); ok &&
				x >= barX && y >= barY && x < barX+image.Width && y < barY+trackH {
				return uiHit{kind: hitLogBar, index: -1}
			}
		}
	}
	for _, rect := range state.voiceRects {
		if x >= rect.x && y >= rect.y && x < rect.x+rect.w && y < rect.y+rect.h {
			return uiHit{kind: hitLogVoice, index: rect.index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateLogScreen(hit uiHit, x, y int) {
	ui := s.ui
	if ui == nil || ui.log == nil {
		return
	}
	switch hit.kind {
	case hitLogUp:
		s.scrollLogBy(-1)
	case hitLogDown:
		s.scrollLogBy(1)
	case hitLogBar:
		s.scrollLogTo(y)
	case hitLogVoice:
		if hit.index >= 0 && hit.index < len(ui.log.entries) {
			file := ui.log.entries[hit.index].File
			if file != "" {
				s.audio = append(s.audio, core.AudioEvent{
					Kind:    2,
					Path:    s.materialize(file),
					Volume:  s.volumeGain(kindVoice) * s.characterGain(file, true),
					Channel: noTarget,
					Group:   noTarget,
				})
			}
		}
	}
	s.composer.Invalidate()
}
