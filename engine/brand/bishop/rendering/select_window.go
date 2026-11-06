package rendering

import (
	"kizuna/engine/brand/bishop/state"
)

// drawSelectItems 画原版的选择肢窗口。
//
// 每项由中间条（2 帧：常态/悬停）和可选的上下装饰图组成；文字用引擎字体，
// 颜色/对齐来自 bsxx.cfg 的 [select window] design。命中由 Session 负责，
// 这里只按 SelectHover 选帧。
func (c *SceneComposer) drawSelectItems(gameState *state.GameState) bool {
	items := gameState.SelectItems()
	if len(items) == 0 || c.message == nil {
		return false
	}
	hover := gameState.SelectHover()
	lineHeight := max(1, c.message.lineHeight())
	drawn := false
	for _, item := range items {
		// 悬停且配置了 SelectedItem 时，改画 SelectedFrame 图的同帧位切片；
		// 零值保持旧行为：悬停切 Frame 的第 1 帧。
		frameName := item.Frame
		if item.Index == hover && item.SelectedFrame != "" {
			frameName = item.SelectedFrame
		}
		if image := c.Images.Get(frameName); image != nil {
			frames := item.Frames
			if frames <= 0 {
				frames = state.StageButtonFrameCount
			}
			frames = max(1, frames)
			frameHeight := image.Height / frames
			frame := 0
			if item.Index == hover {
				frame = 1
			}
			sourceY := frame * frameHeight
			if sourceY+frameHeight > image.Height {
				sourceY = 0
			}
			width := min(image.Width, max(1, item.Width))
			height := min(frameHeight, max(1, item.Height))
			c.frame.DrawRegion(image, 0, sourceY, width, height, item.X, item.Y)
			drawn = true
		}
		if item.Head != "" {
			if image := c.Images.Get(item.Head); image != nil {
				c.frame.Draw(image, item.X, item.Y-image.Height)
				drawn = true
			}
		}
		if item.Foot != "" {
			if image := c.Images.Get(item.Foot); image != nil {
				c.frame.Draw(image, item.X, item.Y+item.Height)
				drawn = true
			}
		}
		x := item.X
		if item.Alignment == selectAlignLeft {
			x += item.TextX
		}
		color := item.Color(item.Index == hover)
		c.message.drawTextAligned(c.frame, item.Text, x, item.Y+item.TextY, item.Width, 1, color, lineHeight, item.Alignment)
		drawn = true
	}
	return drawn
}
