package gfx

import (
	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/core"
)

const (
	maxDimension  = 8192
	maxPixelBytes = 1 << 28
)

type emePixelKind int

const (
	emePixelUnknown emePixelKind = iota
	emePixelGrayscale
	emePixelIndexed
	emePixelBgr
	emePixelBgrx
)

func Decode(header, payload []byte, frameSize, frameInitPos int) *bishopgfx.BsgImage {
	if len(header) < 32 {
		return nil
	}
	parsed, err := ParseEmeImageHeader(header)
	if err != nil {
		return nil
	}
	return decodeParsed(parsed, payload, frameSize, frameInitPos)
}

func decodeParsed(header *EmeImageHeader, payload []byte, frameSize, frameInitPos int) *bishopgfx.BsgImage {
	width := header.Width
	height := header.Height
	stride := header.Stride
	if width <= 0 || height <= 0 || stride <= 0 || width > maxDimension || height > maxDimension {
		return nil
	}

	kind := classify(header)
	if kind == emePixelUnknown {
		return nil
	}

	bytesPerPixel := 1
	switch kind {
	case emePixelBgr:
		bytesPerPixel = 3
	case emePixelBgrx:
		bytesPerPixel = 4
	}
	if stride < width*bytesPerPixel {
		return nil
	}

	paletteBytes := header.PaletteBytes()
	if len(payload) < paletteBytes {
		return nil
	}

	pixelBytes := stride * height
	if pixelBytes > maxPixelBytes {
		return nil
	}

	var pixels []byte
	if frameSize != 0 {
		unpacked, err := eme.LzssDecompress(payload[paletteBytes:], pixelBytes, frameSize, frameInitPos)
		if err != nil {
			core.Logf("gfx: EME image decompression failed: %v", err)
			return nil
		}
		if len(unpacked) < pixelBytes {
			return nil
		}
		pixels = unpacked[:pixelBytes]
	} else {
		if len(payload)-paletteBytes < pixelBytes {
			return nil
		}
		pixels = payload[paletteBytes : paletteBytes+pixelBytes]
	}

	output := make([]byte, width*height*4)
	rowOut := width * 4
	switch kind {
	case emePixelGrayscale:
		decodeGrayscale(pixels, output, width, height, stride, rowOut)
	case emePixelIndexed:
		decodeIndexed(pixels, payload[:paletteBytes], output, width, height, stride, rowOut)
	case emePixelBgr:
		decodeBgr(pixels, output, width, height, stride, rowOut)
	default:
		decodeBgrx(pixels, output, width, height, stride, rowOut)
	}

	return bishopgfx.NewBsgImage(width, height, header.OffsetX, header.OffsetY, output)
}

func classify(header *EmeImageHeader) emePixelKind {
	if header.BitsPerPixel == 7 {
		return emePixelGrayscale
	}
	if header.BitsPerPixel == 24 {
		return emePixelBgr
	}
	if header.BitsPerPixel == 32 {
		return emePixelBgrx
	}
	if header.BitsPerPixel < 24 && header.HasPalette() {
		return emePixelIndexed
	}
	return emePixelUnknown
}

func decodeGrayscale(pixels, output []byte, width, height, stride, rowOut int) {
	for y := 0; y < height; y++ {
		source := pixels[y*stride : y*stride+width]
		target := output[y*rowOut : y*rowOut+rowOut]
		for x := 0; x < width; x++ {
			value := source[x]
			offset := x * 4
			target[offset] = value
			target[offset+1] = value
			target[offset+2] = value
			target[offset+3] = 255
		}
	}
}

func decodeIndexed(pixels, palette, output []byte, width, height, stride, rowOut int) {
	maxIndex := len(palette)/4 - 1
	for y := 0; y < height; y++ {
		source := pixels[(height-1-y)*stride : (height-1-y)*stride+width]
		target := output[y*rowOut : y*rowOut+rowOut]
		for x := 0; x < width; x++ {
			index := int(source[x])
			if index > maxIndex {
				index = maxIndex
			}
			entry := index * 4
			offset := x * 4
			blue := palette[entry]
			green := palette[entry+1]
			red := palette[entry+2]
			target[offset] = blue
			target[offset+1] = green
			target[offset+2] = red
			target[offset+3] = Alpha(IsTransparent(blue, green, red))
		}
	}
}

func decodeBgr(pixels, output []byte, width, height, stride, rowOut int) {
	for y := 0; y < height; y++ {
		source := pixels[(height-1-y)*stride:]
		target := output[y*rowOut : y*rowOut+rowOut]
		for x := 0; x < width; x++ {
			origin := x * 3
			offset := x * 4
			blue := source[origin]
			green := source[origin+1]
			red := source[origin+2]
			target[offset] = blue
			target[offset+1] = green
			target[offset+2] = red
			target[offset+3] = Alpha(IsTransparent(blue, green, red))
		}
	}
}

func decodeBgrx(pixels, output []byte, width, height, stride, rowOut int) {
	hasAlpha := false
	for y := 0; y < height; y++ {
		source := pixels[(height-1-y)*stride:]
		target := output[y*rowOut : y*rowOut+rowOut]
		for x := 0; x < width; x++ {
			origin := x * 4
			offset := x * 4
			target[offset] = source[origin]
			target[offset+1] = source[origin+1]
			target[offset+2] = source[origin+2]
			alpha := source[origin+3]
			if alpha != 0 && alpha != 255 {
				hasAlpha = true
			}
			target[offset+3] = alpha
		}
	}

	for row := 0; row < height; row++ {
		target := output[row*rowOut : row*rowOut+rowOut]
		for x := 0; x < width; x++ {
			offset := x * 4
			if IsTransparent(target[offset], target[offset+1], target[offset+2]) {
				target[offset+3] = 0
			} else if !hasAlpha {
				target[offset+3] = 255
			}
		}
	}
}
