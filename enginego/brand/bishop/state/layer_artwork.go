package state

type LayerArtwork struct {
	Image        string
	X            int
	Y            int
	Frames       int
	AnimatedPart int
	Frame        int
	CellHeight   int
	// Clip 是从原图裁出的源矩形（X, Y, 宽, 高）。零宽/零高表示整张图。
	// Tbxdl 地图的 mapmenu.bsg 把"常态/悬停"两列并排放在一张 306x374 图里，
	// 必须只画左列，否则右列会溢出到屏幕外。
	ClipX      int
	ClipY      int
	ClipWidth  int
	ClipHeight int
	// ScaleX/ScaleY 把部件按比例放大/缩小后再贴到屏幕。0 或 1 表示原尺寸。
	//
	// 三射面談IE 的全身立绘有两种容器：a 组身体图是 2350x2990，b 组却是
	// 1600x2300，而表情叠加图统一是 2350x2990。原引擎会按表情容器的比例
	// 把 b 身体图放大到同一坐标系；不做这一步，b 组人物会整体左移、缩小，
	// 并且因为宽度对不上而丢表情层（眼睛/眉毛消失）。
	ScaleX float64
	ScaleY float64
}

func NewLayerArtwork(image string, x, y int) LayerArtwork {
	return LayerArtwork{Image: image, X: x, Y: y, Frames: 1, AnimatedPart: -1}
}

func LayerArtworkAt(image string) LayerArtwork {
	return LayerArtwork{Image: image, Frames: 1, AnimatedPart: -1}
}

func AnimatedLayerArtwork(image string, x, y, frames, part int) LayerArtwork {
	if frames > 1 {
		if part < 0 {
			part = 0
		}
		return LayerArtwork{Image: image, X: x, Y: y, Frames: max(frames, 1), AnimatedPart: part}
	}
	return LayerArtwork{Image: image, X: x, Y: y, Frames: max(frames, 1), AnimatedPart: -1}
}

func StripLayerArtwork(image string, x, y, part int) LayerArtwork {
	if part < 0 {
		part = 0
	}
	return LayerArtwork{Image: image, X: x, Y: y, Frames: 1, AnimatedPart: part}
}

func (a LayerArtwork) IsAnimated() bool {
	return a.AnimatedPart >= 0
}

// HasScale 判断是否需要缩放绘制。
func (a LayerArtwork) HasScale() bool {
	return (a.ScaleX > 0 && a.ScaleX != 1) || (a.ScaleY > 0 && a.ScaleY != 1)
}

func (a LayerArtwork) HasCell() bool {
	return a.CellHeight > 0
}
