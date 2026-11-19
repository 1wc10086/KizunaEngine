package rendering

import (
	"kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/core/textrender"
)

// DrawText 把一行/多行文字排到 BGRA 画布上。
//
// 鉴赏室、BGV 列表、设置说明等界面都需要文字，但按原版布局逐处写字体加载
// 会重复创建排版器。调用方持有 textrender.Renderer，本函数只负责定位并把
// 排版结果追加到画布的文字列表；字形由宿主用平台字体绘制。
func DrawText(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, text string, x, y, size, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32) {
	if canvas == nil || renderer == nil || text == "" {
		return
	}
	// 旧位图只在 edge>0 时画描边；Render 默认会给黑描边，所以 edge 为 0
	// 时必须显式传 0，否则宿主会给本不该有描边的文字加一圈黑边。
	outline := uint32(0xff000000)
	if edge <= 0 {
		outline = 0
	}
	rendered := renderer.RenderOutline(text, size, edge, shadow, lineHeight, maxWidth, maxLines, color, outline)
	blitRGBA(canvas, rendered, x, y)
}

// DrawTextImage 把已经排好版的文字追加到画布上，返回绘制高度。
// 回想界面需要按每条对白的实际行数推进 Y，所以不能只调用 DrawText。
func DrawTextImage(canvas *gfx.BgraCanvas, rendered *textrender.Rendered, x, y int) int {
	if canvas == nil || rendered == nil {
		return 0
	}
	blitRGBA(canvas, rendered, x, y)
	return rendered.Bounds().Dy()
}

// OutlineForEdge 让 Render 系列调用保持旧位图的行为：edge<=0 时没有描边。
//
// textrender.Render 的默认描边是黑色，而旧位图只在 edge>0 时才画；直接
// 用默认值会给本不该有描边的界面文字加一圈黑边。
func OutlineForEdge(edge int) uint32 {
	if edge <= 0 {
		return 0
	}
	return 0xff000000
}
