package rendering

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

type LayerSurface struct {
	canvas   *gfx.BgraCanvas
	revision int
	frame    int
	animated bool
	// view 记录上次绘制时的视点；视点变化不改变 layer.Revision，
	// 必须单独参与重绘判断，否则点「顔/胸/腰」画面不会变。
	view int
	// generation 是 ImageCache 的清空版本，用于「アヘ顔OFF」等只换素材、
	// 不改图层结构的设置。
	generation uint64
}

func NewLayerSurface(width, height int) *LayerSurface {
	return &LayerSurface{canvas: gfx.NewBgraCanvas(width, height), revision: -1, frame: -1, view: -1}
}

func (s *LayerSurface) Canvas() *gfx.BgraCanvas {
	return s.canvas
}

func (s *LayerSurface) Refresh(layer *state.StageLayer, images *ImageCache, frame, viewMode int) {
	generation := images.Generation()
	rebuild := s.revision != layer.Revision() || s.view != viewMode || s.generation != generation
	if !rebuild && (!s.animated || s.frame == frame) {
		return
	}
	s.revision = layer.Revision()
	s.animated = isLayerAnimated(layer)
	s.frame = frame
	s.view = viewMode
	s.generation = generation
	s.canvas.Clear(0, 0, 0, 0)
	for _, artwork := range layer.Artworks() {
		s.paint(artwork, images, frame, viewMode)
	}
}

func isLayerAnimated(layer *state.StageLayer) bool {
	for _, artwork := range layer.Artworks() {
		if artwork.IsAnimated() {
			return true
		}
	}
	return false
}

func (s *LayerSurface) paint(artwork state.LayerArtwork, images *ImageCache, frame, viewMode int) {
	if len(artwork.Image) == 0 || TrySpecialFill(s.canvas, artwork.Image) {
		return
	}
	name := artwork.Image
	image := images.Get(name)
	if image == nil {
		return
	}
	offsetX, offsetY := 0, 0
	// 视点切换只做垂直位移，绝不改图名，也不额外做水平居中：
	// 脚本给立绘的 X/Y 已经是原引擎排好的位置（本作全身图是 x=-663,y=-261）。
	// 之前把 u 换成 f 再按 f 的 0,0 画，会把整张图移到屏幕外；
	// 这里保留脚本坐标，只按视点把图往上移，露出胸/腰。
	if viewMode > 0 && state.IsBustupLayerName(name) && image.Height > 0 {
		shift := image.Height / 8
		if viewMode >= 2 {
			shift = image.Height / 3
		}
		offsetY = -shift
	}
	if len(image.Parts) > 0 {
		parts := image.Parts
		for index, part := range parts {
			targetX := artwork.X + offsetX + image.ContainerOffsetX + part.OffsetX
			targetY := artwork.Y + offsetY + image.ContainerOffsetY + part.OffsetY
			// b 组全身立绘的容器比表情图小，BustupResolve 会把身体/头发标记
			// 成需要放大；容器原点不动，部件偏移和像素尺寸一起乘比例。
			if artwork.HasScale() && !isStripPart(artwork, index) {
				sx, sy := artworkScale(artwork)
				s.canvas.BlitScaled(part,
					artwork.X+offsetX+image.ContainerOffsetX+int(float64(part.OffsetX)*sx),
					artwork.Y+offsetY+image.ContainerOffsetY+int(float64(part.OffsetY)*sy),
					max(1, int(float64(part.Width)*sx)),
					max(1, int(float64(part.Height)*sy)),
					255)
				continue
			}
			if !isStripPart(artwork, index) {
				s.canvas.Draw(part, targetX, targetY)
				continue
			}
			s.paintStrip(part, len(parts), artwork, frame, targetX, targetY)
		}
		return
	}
	x := artwork.X + offsetX + image.OffsetX
	y := artwork.Y + offsetY + image.OffsetY
	if artwork.HasScale() && !artwork.IsAnimated() && !artwork.HasCell() && artwork.ClipWidth == 0 {
		sx, sy := artworkScale(artwork)
		s.canvas.BlitScaled(image,
			artwork.X+offsetX+int(float64(image.OffsetX)*sx),
			artwork.Y+offsetY+int(float64(image.OffsetY)*sy),
			max(1, int(float64(image.Width)*sx)),
			max(1, int(float64(image.Height)*sy)),
			255)
		return
	}
	// 源矩形裁剪：Tbxdl 的 mapmenu 是左右两列并排的整图，只允许画裁出的那一列。
	// 零宽/零高保持原行为（整张图），不影响其它作品。
	if artwork.ClipWidth > 0 && artwork.ClipHeight > 0 {
		s.canvas.DrawRegion(image, artwork.ClipX, artwork.ClipY, artwork.ClipWidth, artwork.ClipHeight, x, y)
		return
	}
	if artwork.HasCell() && len(image.Parts) == 0 {
		cell := min(artwork.CellHeight, image.Height)
		cellTop := core.Clamp(artwork.Frame*cell, 0, max(0, image.Height-cell))
		s.canvas.DrawRegion(image, 0, cellTop, image.Width, cell, x, y)
		return
	}
	if !artwork.IsAnimated() {
		if frames, staticHeight, ok := gfx.TryResolveBsgFrameStrip(image, 1, 1); ok {
			staticY := gfx.SourceTopBsgFrameStrip(image, frames, staticHeight, artwork.Frame)
			s.canvas.DrawRegion(image, 0, staticY, image.Width, staticHeight, x, y)
			return
		}
		s.canvas.Draw(image, x, y)
		return
	}
	frames, frameHeight, ok := gfx.TryResolveBsgFrameStrip(image, 1, artwork.Frames)
	if !ok {
		s.canvas.Draw(image, x, y)
		return
	}
	sourceY := gfx.SourceTopBsgFrameStrip(image, frames, frameHeight, frame)
	s.canvas.DrawRegion(image, 0, sourceY, image.Width, frameHeight, x, y)
}

func (s *LayerSurface) paintStrip(part *gfx.BsgImage, partCount int, artwork state.LayerArtwork, frame, x, y int) {
	frames, frameHeight, ok := gfx.TryResolveBsgFrameStrip(part, partCount, artwork.Frames)
	if !ok {
		s.canvas.Draw(part, x, y)
		return
	}
	sourceY := gfx.SourceTopBsgFrameStrip(part, frames, frameHeight, frame)
	s.canvas.DrawRegion(part, 0, sourceY, part.Width, frameHeight, x, y)
}

func isStripPart(artwork state.LayerArtwork, index int) bool {
	return artwork.IsAnimated() && index == artwork.AnimatedPart
}

// artworkScale 返回有效缩放比例，缺省轴按 1 处理。
func artworkScale(artwork state.LayerArtwork) (float64, float64) {
	sx, sy := artwork.ScaleX, artwork.ScaleY
	if sx <= 0 {
		sx = 1
	}
	if sy <= 0 {
		sy = 1
	}
	return sx, sy
}
