package gfx

type BsgImage struct {
	Width            int
	Height           int
	OffsetX          int
	OffsetY          int
	Pixels           []byte
	Parts            []*BsgImage
	ContainerOffsetX int
	ContainerOffsetY int
	FrameIndex       int
	FrameCount       int
	ClipLeft         int
	ClipTop          int
	ClipWidth        int
	ClipHeight       int
	Alpha            int
	BlendMode        int

	content *PixelBounds
}

func NewBsgImage(width, height, offsetX, offsetY int, pixels []byte) *BsgImage {
	return &BsgImage{
		Width:      width,
		Height:     height,
		OffsetX:    offsetX,
		OffsetY:    offsetY,
		Pixels:     pixels,
		FrameCount: 1,
		Alpha:      255,
	}
}

func (i *BsgImage) FrameExtent() int {
	return i.Height
}

func (i *BsgImage) FrameHeight() int {
	if i.FrameCount > 1 {
		return max(1, i.Height/i.FrameCount)
	}

	return i.Height
}

func (i *BsgImage) ContentBounds() PixelBounds {
	if i.content == nil {
		bounds := MeasurePixelBounds(i.Pixels, i.Width, i.Height)
		i.content = &bounds
	}

	return *i.content
}

func (i *BsgImage) Stride() int {
	return i.Width * 4
}

func (i *BsgImage) ByteCount() int {
	return len(i.Pixels)
}

func (i *BsgImage) ContentCenterX() int {
	bounds := i.ContentBounds()
	return bounds.X + (bounds.Width / 2)
}

func (i *BsgImage) Relocate(offsetX, offsetY int) *BsgImage {
	return &BsgImage{
		Width:            i.Width,
		Height:           i.Height,
		OffsetX:          offsetX,
		OffsetY:          offsetY,
		Pixels:           i.Pixels,
		Parts:            i.Parts,
		ContainerOffsetX: i.ContainerOffsetX,
		ContainerOffsetY: i.ContainerOffsetY,
		FrameIndex:       i.FrameIndex,
		FrameCount:       i.FrameCount,
		ClipLeft:         i.ClipLeft,
		ClipTop:          i.ClipTop,
		ClipWidth:        i.ClipWidth,
		ClipHeight:       i.ClipHeight,
		Alpha:            i.Alpha,
		BlendMode:        i.BlendMode,
	}
}
