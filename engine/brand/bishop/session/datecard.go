package session

import "kizuna/engine/brand/bishop/gfx"

// drawDateCard 画剧情左上角的日期/时刻卡（学園 GK）。
//
// 它不是 [message window] 的子窗：原版在对话、选择肢上都把同一张卡贴在
// (0,0)。这里在地图/标题/系统界面之外每帧补画，数值直接读 #GKR_*。
func (s *Session) drawDateCard(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || s.config == nil {
		return false
	}
	card := s.config.DateCard
	if card.Empty() {
		return false
	}
	if s.titleMode || s.ui != nil || s.mapShown || s.replay ||
		s.staffrollActive() || s.eyecatchActive() || s.CurrentMovie() != "" {
		return false
	}
	s.drawImageAt(canvas, card.Base, card.X, card.Y)
	draw := func(name string, frame int, point ...int) {
		x, y := 0, 0
		if len(point) >= 2 {
			x, y = point[0], point[1]
		}
		image := s.composer.Load(name)
		if image == nil || frame < 0 {
			return
		}
		frames := max(1, image.FrameCount)
		cell := image.Height / frames
		if cell <= 0 {
			return
		}
		if frame >= frames {
			frame = frames - 1
		}
		canvas.DrawRegion(image, 0, frame*cell, image.Width, cell, x, y)
	}
	draw("date_week_no", s.sessionProperty("#GKR_week_no"), card.WeekNo.X, card.WeekNo.Y)
	draw("date_week", s.sessionProperty("#GKR_week"), card.Week.X, card.Week.Y)
	draw("date_AMPM", s.sessionProperty("#GKR_ampm"), card.AMPM.X, card.AMPM.Y)
	draw("date_hour", s.sessionProperty("#GKR_hour"), card.Hour.X, card.Hour.Y)
	draw("date_minute", s.sessionProperty("#GKR_minute"), card.Minute.X, card.Minute.Y)
	return true
}
