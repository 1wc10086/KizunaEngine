package bmp

import (
	"errors"
	"fmt"

	"kizuna/engine/core/binutil"
)

const (
	BmpFileHeaderSize    = 14
	BmpCoreHeaderSize    = 12
	BmpInfoHeaderSize    = 40
	BmpAlphaMaskHeader   = 56
	BmpMaxHeaderSize     = 124
	BmpMaxDimension      = 8192
	BmpMaxPixelCount     = 1 << 25
	bmpMaskFieldOffset   = 54
	bmpClrUsedOffset     = 46
	bmpPixelOffsetField  = 10
	bmpDibSizeField      = 14
	bmpCoreWidthField    = 18
	bmpCoreHeightField   = 20
	bmpCorePlanesField   = 22
	bmpCoreBitCountField = 24
	bmpWidthField        = 18
	bmpHeightField       = 22
	bmpPlanesField       = 26
	bmpBitCountField     = 28
	bmpRawCompressField  = 30
)

type BmpHeader struct {
	Width            int
	Height           int
	TopDown          bool
	BitCount         int
	Compression      BmpCompression
	HeaderSize       int
	PixelOffset      int
	RowBytes         int
	PaletteOffset    int
	PaletteEntrySize int
	PaletteCount     int
	RedMask          uint32
	GreenMask        uint32
	BlueMask         uint32
	AlphaMask        uint32
}

func (h *BmpHeader) IsIndexed() bool {
	return h.BitCount <= 8
}

func (h *BmpHeader) IsBitField() bool {
	return h.Compression == BmpCompressionBitFields
}

func (h *BmpHeader) HasAlphaMask() bool {
	return h.AlphaMask != 0
}

func (h *BmpHeader) PixelCount() int {
	return h.Width * h.Height
}

func ParseBmpHeader(data []byte) (*BmpHeader, error) {
	if !IsBmp(data) {
		return nil, errors.New("missing BM signature")
	}

	if len(data) < BmpFileHeaderSize+4 {
		return nil, errors.New("BMP header is truncated")
	}

	pixelOffset := binutil.U32(data[bmpPixelOffsetField:])
	if uint64(pixelOffset) > uint64(len(data)) {
		return nil, errors.New("BMP pixel offset is out of range")
	}

	headerSize := binutil.U32(data[bmpDibSizeField:])
	if headerSize != BmpCoreHeaderSize && (headerSize < BmpInfoHeaderSize || headerSize > BmpMaxHeaderSize) {
		return nil, fmt.Errorf("unsupported BMP header size %d", headerSize)
	}

	if len(data) < BmpFileHeaderSize+int(headerSize) {
		return nil, errors.New("BMP header is truncated")
	}

	var width int
	var rawHeight int64
	var planes int
	bitCount := 0
	compression := BmpCompressionRgb
	paletteOffset := 0
	paletteEntrySize := 0
	paletteCount := 0
	var redMask uint32
	var greenMask uint32
	var blueMask uint32
	var alphaMask uint32

	if headerSize == BmpCoreHeaderSize {
		width = int(binutil.U16(data[bmpCoreWidthField:]))
		rawHeight = int64(int16(binutil.U16(data[bmpCoreHeightField:])))
		planes = int(binutil.U16(data[bmpCorePlanesField:]))
		bitCount = int(binutil.U16(data[bmpCoreBitCountField:]))
		paletteOffset = BmpFileHeaderSize + BmpCoreHeaderSize
		paletteEntrySize = 3

		if bitCount <= 8 {
			paletteCount = 1 << bitCount
		}
	} else {
		width = int(binutil.I32(data[bmpWidthField:]))
		rawHeight = int64(binutil.I32(data[bmpHeightField:]))
		planes = int(binutil.U16(data[bmpPlanesField:]))
		bitCount = int(binutil.U16(data[bmpBitCountField:]))

		rawCompression := binutil.U32(data[bmpRawCompressField:])
		if rawCompression != 0 && rawCompression != 3 {
			return nil, fmt.Errorf("unsupported BMP compression %d", rawCompression)
		}

		if rawCompression == 3 {
			compression = BmpCompressionBitFields
		}

		if compression == BmpCompressionBitFields {
			if len(data) < bmpMaskFieldOffset+12 {
				return nil, errors.New("BMP bit fields are truncated")
			}

			redMask = binutil.U32(data[bmpMaskFieldOffset:])
			greenMask = binutil.U32(data[bmpMaskFieldOffset+4:])
			blueMask = binutil.U32(data[bmpMaskFieldOffset+8:])

			if headerSize >= BmpAlphaMaskHeader {
				alphaMask = binutil.U32(data[bmpMaskFieldOffset+12:])
			}
		}

		clrUsed := binutil.U32(data[bmpClrUsedOffset:])
		paletteEntrySize = 4
		paletteOffset = BmpFileHeaderSize + int(headerSize)
		if compression == BmpCompressionBitFields && headerSize == BmpInfoHeaderSize {
			paletteOffset += 12
		}

		if bitCount <= 8 {
			colors := uint32(1 << bitCount)
			paletteCount = int(colors)
			if clrUsed > 0 && clrUsed < colors {
				paletteCount = int(clrUsed)
			}
		}
	}

	if planes != 1 {
		return nil, fmt.Errorf("unsupported BMP plane count %d", planes)
	}

	if width <= 0 || width > BmpMaxDimension {
		return nil, fmt.Errorf("invalid BMP width %d", width)
	}

	topDown := rawHeight < 0
	height := rawHeight
	if height < 0 {
		height = -height
	}
	if height <= 0 || height > BmpMaxDimension {
		return nil, fmt.Errorf("invalid BMP height %d", rawHeight)
	}

	switch bitCount {
	case 1, 4, 8, 16, 24, 32:
	default:
		return nil, fmt.Errorf("unsupported BMP bit depth %d", bitCount)
	}

	if compression == BmpCompressionBitFields && bitCount < 16 {
		return nil, errors.New("BMP bit fields require true colour data")
	}

	if compression == BmpCompressionBitFields && (redMask == 0 || greenMask == 0 || blueMask == 0) {
		return nil, errors.New("BMP bit field masks are missing")
	}

	if compression == BmpCompressionRgb && bitCount == 16 {
		redMask = 0x7C00
		greenMask = 0x03E0
		blueMask = 0x001F
	}

	if int64(width)*height > BmpMaxPixelCount {
		return nil, fmt.Errorf("BMP image is too large: %dx%d", width, height)
	}

	dataStart := BmpFileHeaderSize + int(headerSize)
	if dataStart < paletteOffset {
		dataStart = paletteOffset
	}
	if uint64(pixelOffset) < uint64(dataStart) {
		return nil, errors.New("BMP pixel offset points inside the header")
	}

	rowBytes := int((int64(width)*int64(bitCount) + 31) / 32 * 4)
	if uint64(pixelOffset)+uint64(int64(rowBytes)*height) > uint64(len(data)) {
		return nil, errors.New("BMP pixel data is out of range")
	}

	if paletteCount > 0 && int64(paletteOffset)+int64(paletteCount)*int64(paletteEntrySize) > int64(len(data)) {
		return nil, errors.New("BMP palette is out of range")
	}

	return &BmpHeader{
		Width:            width,
		Height:           int(height),
		TopDown:          topDown,
		BitCount:         bitCount,
		Compression:      compression,
		HeaderSize:       int(headerSize),
		PixelOffset:      int(pixelOffset),
		RowBytes:         rowBytes,
		PaletteOffset:    paletteOffset,
		PaletteEntrySize: paletteEntrySize,
		PaletteCount:     paletteCount,
		RedMask:          redMask,
		GreenMask:        greenMask,
		BlueMask:         blueMask,
		AlphaMask:        alphaMask,
	}, nil
}
