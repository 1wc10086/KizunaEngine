package rendering

import (
	"image"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core/textrender"
)

// DrawText 把一行/多行文字画到 BGRA 画布上。
//
// 鉴赏室、BGV 列表、设置说明等界面都需要文字，但按原版布局逐处写字体加载
// 会重复解析 3.5MB 的内嵌字体。调用方持有 textrender.Renderer，本函数只负责
// 定位与混合；渲染结果由 Renderer 缓存。
func DrawText(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, text string, x, y, size, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32) {
	if canvas == nil || renderer == nil || text == "" {
		return
	}
	image := renderer.Render(text, size, edge, shadow, lineHeight, maxWidth, maxLines, color)
	blitRGBA(canvas, image, x, y)
}

// DrawTextImage 把已经渲染好的文字位图贴到画布上，返回绘制高度。
// 回想界面需要按每条对白的实际行数推进 Y，所以不能只调用 DrawText。
func DrawTextImage(canvas *gfx.BgraCanvas, image *image.RGBA, x, y int) int {
	if canvas == nil || image == nil {
		return 0
	}
	blitRGBA(canvas, image, x, y)
	return image.Bounds().Dy()
}
