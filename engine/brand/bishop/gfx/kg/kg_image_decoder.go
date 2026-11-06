package kg

import (
	"errors"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

func TryRead(data []byte) (image *gfx.BsgImage) {
	defer func() {
		if recover() != nil {
			image = nil
		}
	}()
	image, err := Read(data)
	if err != nil {
		core.Logf("gfx: KG decode failed: %v", err)
		return nil
	}
	return image
}

func Read(data []byte) (*gfx.BsgImage, error) {
	if len(data) == 0 {
		return nil, errors.New("KG data is empty")
	}
	header, err := ParseKgHeader(data)
	if err != nil {
		return nil, err
	}
	var palette []byte
	if header.BitsPerPixel == 8 {
		core.Logf("gfx: KG 8bpp image %dx%d, palette at %d", header.Width, header.Height, header.PaletteOffset)
		palette, err = readPalette(data, header.PaletteOffset)
		if err != nil {
			return nil, err
		}
	}
	pixelSize := header.PixelSize()
	stride := pixelSize * header.Width
	output := make([]byte, stride*header.Height)
	decoder := NewKgPixelDecoder(output, pixelSize, stride)
	bits := binutil.NewBitReader(data[header.DataOffset:])
	if err := decoder.UnpackChannel(bits, 0); err != nil {
		return nil, err
	}
	if pixelSize > 1 {
		if err := decoder.UnpackChannel(bits, 1); err != nil {
			return nil, err
		}
		if err := decoder.UnpackChannel(bits, 2); err != nil {
			return nil, err
		}
	}
	var alpha byte = 255
	if header.HasAlpha {
		alpha = 0
	}
	bgra, err := expandToBgra32(output, header, palette, alpha)
	if err != nil {
		return nil, err
	}
	if header.HasAlpha {
		core.Logf("gfx: KG alpha channel at %d", header.AlphaOffset)
		alphaBits := binutil.NewBitReader(data[header.AlphaOffset:])
		decoder.Output = bgra
		decoder.PixelSize = 4
		decoder.Stride = header.Width * 4
		decoder.ResetDict()
		if err := decoder.UnpackChannel(alphaBits, 3); err != nil {
			core.Logf("gfx: KG alpha decode failed, using opaque: %v", err)
			fillAlpha(bgra)
		}
	}
	pixels := flipRows(bgra, header.Width, header.Height)
	return gfx.NewBsgImage(header.Width, header.Height, 0, 0, pixels), nil
}

func readPalette(data []byte, offset int) ([]byte, error) {
	if offset < 0 || offset+1024 > len(data) {
		return nil, errors.New("KG palette is truncated")
	}
	palette := make([]byte, 256*3)
	for i := 0; i < 256; i++ {
		palette[i*3] = data[offset+i*4]
		palette[i*3+1] = data[offset+i*4+1]
		palette[i*3+2] = data[offset+i*4+2]
	}
	return palette, nil
}

func expandToBgra32(output []byte, header *KgHeader, palette []byte, alpha byte) ([]byte, error) {
	stride := header.Width * 4
	bgra := make([]byte, stride*header.Height)
	dst := 0
	if header.PixelSize() == 1 {
		if palette == nil {
			return nil, errors.New("KG palette is missing")
		}
		colors := len(palette) / 3
		for src := 0; src < len(output); src++ {
			index := int(output[src])
			if index >= colors {
				return nil, errors.New("KG palette index is out of range")
			}
			bgra[dst] = palette[index*3]
			bgra[dst+1] = palette[index*3+1]
			bgra[dst+2] = palette[index*3+2]
			bgra[dst+3] = alpha
			dst += 4
		}
	} else {
		for src := 0; src < len(output); src += header.PixelSize() {
			bgra[dst] = output[src]
			bgra[dst+1] = output[src+1]
			bgra[dst+2] = output[src+2]
			bgra[dst+3] = alpha
			dst += 4
		}
	}
	return bgra, nil
}

func fillAlpha(bgra []byte) {
	for i := 3; i < len(bgra); i += 4 {
		bgra[i] = 255
	}
}

func flipRows(bgra []byte, width, height int) []byte {
	stride := width * 4
	pixels := make([]byte, len(bgra))
	for y := 0; y < height; y++ {
		copy(pixels[(height-1-y)*stride:(height-y)*stride], bgra[y*stride:(y+1)*stride])
	}
	return pixels
}
