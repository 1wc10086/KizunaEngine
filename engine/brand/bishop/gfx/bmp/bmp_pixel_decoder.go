package bmp

import (
	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core/binutil"
)

func DecodeBmpPixels(header *BmpHeader, data []byte, palette []byte, canvas *bishopgfx.BgraCanvas) bool {
	if canvas.Width != header.Width || canvas.Height != header.Height {
		return false
	}

	pixels := canvas.Buffer
	stride := canvas.Stride()
	rowBytes := header.RowBytes

	for y := 0; y < header.Height; y++ {
		row := y
		if !header.TopDown {
			row = header.Height - 1 - y
		}
		origin := int64(header.PixelOffset) + int64(row)*int64(rowBytes)
		if origin+int64(rowBytes) > int64(len(data)) {
			return false
		}

		source := data[origin : origin+int64(rowBytes)]
		target := pixels[y*stride : y*stride+stride]

		decoded := false
		switch header.BitCount {
		case 1, 4, 8:
			decoded = palette != nil && decodeBmpIndexed(header, source, target, palette)
		case 16:
			decoded = decodeBmpBitField(header, source, target)
		case 24, 32:
			if header.IsBitField() {
				decoded = decodeBmpBitField(header, source, target)
			} else {
				decoded = decodeBmpTrueColor(header, source, target)
			}
		}

		if !decoded {
			return false
		}
	}

	makeBmpOpaqueWhenTransparent(pixels)

	return true
}

func decodeBmpIndexed(header *BmpHeader, source []byte, target []byte, palette []byte) bool {
	width := header.Width
	switch header.BitCount {
	case 8:
		for x := 0; x < width; x++ {
			writeBmpPaletteEntry(palette, int(source[x]), target, x*4)
		}

		return true
	case 4:
		for x := 0; x < width; x++ {
			packed := source[x>>1]
			index := int(packed & 0x0F)
			if x&1 == 0 {
				index = int(packed >> 4)
			}
			writeBmpPaletteEntry(palette, index, target, x*4)
		}

		return true
	case 1:
		for x := 0; x < width; x++ {
			packed := source[x>>3]
			index := int((packed >> (7 - uint(x&7))) & 1)
			writeBmpPaletteEntry(palette, index, target, x*4)
		}

		return true
	default:
		return false
	}
}

func decodeBmpTrueColor(header *BmpHeader, source []byte, target []byte) bool {
	width := header.Width
	if header.BitCount == 32 {
		bytes := width * 4
		for offset := 0; offset < bytes; offset++ {
			target[offset] = source[offset]
		}

		return true
	}

	if header.BitCount == 24 {
		for x := 0; x < width; x++ {
			origin := x * 3
			destination := x * 4
			target[destination] = source[origin]
			target[destination+1] = source[origin+1]
			target[destination+2] = source[origin+2]
			target[destination+3] = 255
		}

		return true
	}

	return false
}

func decodeBmpBitField(header *BmpHeader, source []byte, target []byte) bool {
	bytes := header.BitCount / 8
	if bytes != 2 && bytes != 3 && bytes != 4 {
		return false
	}

	red := BmpBitFieldFromMask(header.RedMask)
	green := BmpBitFieldFromMask(header.GreenMask)
	blue := BmpBitFieldFromMask(header.BlueMask)
	alpha := BmpBitFieldFromMask(header.AlphaMask)
	opaque := !header.HasAlphaMask()

	for x := 0; x < header.Width; x++ {
		origin := x * bytes
		var value uint32
		switch bytes {
		case 2:
			value = uint32(binutil.U16(source[origin:]))
		case 3:
			value = uint32(source[origin]) | uint32(source[origin+1])<<8 | uint32(source[origin+2])<<16
		default:
			value = binutil.U32(source[origin:])
		}

		destination := x * 4
		target[destination] = blue.Read(value)
		target[destination+1] = green.Read(value)
		target[destination+2] = red.Read(value)
		if opaque {
			target[destination+3] = 255
		} else {
			target[destination+3] = alpha.Read(value)
		}
	}

	return true
}

func writeBmpPaletteEntry(palette []byte, index int, target []byte, offset int) {
	origin := (index & 0xFF) * BmpPaletteEntrySize
	target[offset] = palette[origin]
	target[offset+1] = palette[origin+1]
	target[offset+2] = palette[origin+2]
	target[offset+3] = palette[origin+3]
}

func makeBmpOpaqueWhenTransparent(pixels []byte) {
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 0 {
			return
		}
	}

	for offset := 3; offset < len(pixels); offset += 4 {
		pixels[offset] = 255
	}
}
