package bmp

import (
	"errors"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
)

func Decode(data []byte) (*bishopgfx.BsgImage, error) {
	header, err := ParseBmpHeader(data)
	if err != nil {
		return nil, err
	}

	palette := ReadBmpPalette(header, data)
	if header.IsIndexed() && palette == nil {
		return nil, errors.New("BMP palette is missing")
	}

	canvas := bishopgfx.NewBgraCanvas(header.Width, header.Height)
	if !DecodeBmpPixels(header, data, palette, canvas) {
		return nil, errors.New("BMP pixel data is malformed")
	}

	return canvas.ToImage(0, 0), nil
}

func TryDecode(data []byte) (*bishopgfx.BsgImage, bool) {
	image, err := Decode(data)
	if err != nil {
		return nil, false
	}

	return image, true
}
