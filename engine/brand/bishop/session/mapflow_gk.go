package session

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/runtime/bsmap"
)

// mapflow_gk.go 是学園（GK）地图的动态叠层。
//
// 舞台层只放固定帧：map_base、map_submenu、date_base 和 12 个地点按钮。
// 以下内容每帧都可能变，必须在地图合成后补画：
//   - 悬停地点的第 1 帧（map_placeNN 是常态/悬停/选中三帧竖排）；
//   - 左上日期卡里的周数/曜日/AM/PM/时/分（#GKR_* 由脚本的
//     「日付指定」「時分指定」系统调用写入）；
//   - 右上角当前目标头像（#GKR_flower，1..5）。
func (s *Session) drawGkMapOverlay(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || !s.mapShown {
		return false
	}
	drawn := false
	// 悬停地点：图头坐标就是屏幕坐标，直接按 140x30 裁第 1 帧。
	if index := s.gameState.HoveredButton(); index >= 0 && index < len(s.mapItems) {
		item := s.mapItems[index]
		if image := s.composer.Load(item.Image); image != nil && image.Height >= item.HitHeight*2 {
			x, y := image.OffsetX, image.OffsetY
			if item.DrawX >= 0 && item.DrawY >= 0 {
				x, y = item.DrawX, item.DrawY
			}
			canvas.DrawRegion(image, 0, item.HitHeight, image.Width, item.HitHeight, x, y)
			drawn = true
		}
	}

	// 日期卡：数值是 0 基索引，越界时夹到最后一帧。
	drawFrame := func(name string, frame, x, y int) {
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
		drawn = true
	}
	drawFrame("date_week_no", s.sessionProperty("#GKR_week_no"), 8, 2)
	drawFrame("date_week", s.sessionProperty("#GKR_week"), 8, 38)
	drawFrame("date_AMPM", s.sessionProperty("#GKR_ampm"), 54, 38)
	drawFrame("date_hour", s.sessionProperty("#GKR_hour"), 76, 38)
	drawFrame("date_minute", s.sessionProperty("#GKR_minute"), 98, 38)

	// 目标头像：#GKR_flower 是 1 基的人物号。原版地图右上角始终显示
	// 当前目标（没有指定时回落到第 1 位），不会留一块空框。
	face := 0
	if value := s.sessionProperty("#GKR_flower"); value >= 1 && value <= 5 {
		face = value - 1
	}
	drawFrame(bsmap.BsMapGkFaceImage, face, bsmap.BsMapGkFaceX, bsmap.BsMapGkFaceY)
	return drawn
}
