package gfx

import (
	"errors"

	"kizuna/engine/core/binutil"
)

const (
	BsgCompositionContainerOffsetX = 0
	BsgCompositionContainerOffsetY = 0
)

type BsgComposition struct {
	Parts  []*BsgImage
	Width  int
	Height int
}

func ParseBsgComposition(data []byte) (*BsgComposition, error) {
	if !IsBsgComposition(data) {
		return nil, errors.New("missing BSS-Composition signature")
	}

	width := int(binutil.U16(data[0x16:]))
	height := int(binutil.U16(data[0x18:]))
	count := int(data[BsgPartCountOffset])

	parts := make([]*BsgImage, 0, count)
	offset := BsgCompositionHeaderSize

	for index := 0; index < count; index++ {
		if !IsBsgGraphics(data, offset) {
			break
		}

		part, err := DecodeBsgImage(data, offset)
		if err != nil {
			return nil, err
		}

		parts = append(parts, part)
		block := ReadBsgBlockSize(data, offset)
		if block > BsgHeaderSize {
			offset += block
		} else {
			offset += BsgHeaderSize
		}
	}

	if len(parts) == 0 {
		return nil, errors.New("BSS-Composition contains no decodable part")
	}

	if width <= 0 {
		width = parts[0].Width
	}

	if height <= 0 {
		height = parts[0].Height
	}

	return &BsgComposition{Parts: parts, Width: width, Height: height}, nil
}

func (c *BsgComposition) Flatten() *BsgImage {
	canvas := NewBgraCanvas(c.Width, c.Height)
	for _, part := range c.Parts {
		canvas.Draw(part, part.OffsetX, part.OffsetY)
	}

	return canvas.ToImage(BsgCompositionContainerOffsetX, BsgCompositionContainerOffsetY)
}
