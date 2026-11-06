package gfx

import (
	"errors"
	"fmt"

	"kizuna/engine/core/binutil"
)

const (
	bsgWidthOffset        = 0x16
	bsgHeightOffset       = 0x18
	bsgOffsetXField       = 0x20
	bsgOffsetYField       = 0x22
	bsgClipLeftField      = 0x24
	bsgClipTopField       = 0x26
	bsgClipWidthField     = 0x28
	bsgClipHeightField    = 0x2A
	bsgFrameIndexField    = 0x2C
	bsgFrameCountField    = 0x2D
	bsgColorModeField     = 0x30
	bsgCompressionField   = 0x31
	bsgDataOffsetField    = 0x32
	bsgDataSizeField      = 0x36
	bsgPaletteOffsetField = 0x3A
	bsgAlphaField         = 0x3E
	bsgBlendModeField     = 0x3F

	bsgMaxDimension  = 8192
	bsgMaxPixelCount = 1 << 26
)

type BsgHeader struct {
	Width         int
	Height        int
	OffsetX       int
	OffsetY       int
	ClipLeft      int
	ClipTop       int
	ClipWidth     int
	ClipHeight    int
	FrameIndex    int
	FrameCount    int
	Alpha         int
	BlendMode     int
	ColorMode     BsgColorMode
	Compression   BsgCompression
	DataOffset    int
	DataSize      int
	BlockSize     int
	PaletteOffset int
}

func (h *BsgHeader) FrameExtent() int {
	return h.Height
}

func (h *BsgHeader) FrameHeight() int {
	if h.FrameCount > 1 {
		return max(1, h.Height/h.FrameCount)
	}

	return h.Height
}

func (h *BsgHeader) Channels() int {
	switch h.ColorMode {
	case BsgColorModeBgra32:
		return 4
	case BsgColorModeBgr24:
		return 3
	default:
		return 1
	}
}

func (h *BsgHeader) PixelStride() int {
	if h.ColorMode == BsgColorModeIndexed8 {
		return 1
	}

	return 4
}

func (h *BsgHeader) PixelCount() int {
	return h.Width * h.Height
}

func (h *BsgHeader) HasPalette() bool {
	return h.PaletteOffset > 0
}

func TryReadBsgDimensions(data []byte) (width int, height int, ok bool) {
	if !IsBsgGraphics(data, 0) {
		return 0, 0, false
	}

	candidateWidth := int(binutil.U16(data[bsgWidthOffset:]))
	candidateHeight := int(binutil.U16(data[bsgHeightOffset:]))
	if candidateWidth == 0 || candidateHeight == 0 || candidateWidth > bsgMaxDimension || candidateHeight > bsgMaxDimension {
		return 0, 0, false
	}

	return candidateWidth, candidateHeight, true
}

func ReadBsgBlockSize(data []byte, baseOffset int) int {
	if !IsBsgGraphics(data, baseOffset) {
		return 0
	}

	region := data[baseOffset:]
	return int(binutil.I32(region[bsgDataOffsetField:])) +
		int(binutil.I32(region[bsgDataSizeField:]))
}

func ParseBsgHeader(data []byte, baseOffset int) (*BsgHeader, error) {
	if !IsBsgGraphics(data, baseOffset) {
		return nil, errors.New("missing BSS-Graphics signature")
	}

	region := data[baseOffset:]
	colorMode := BsgColorMode(region[bsgColorModeField])
	if colorMode > BsgColorModeIndexed8 {
		return nil, fmt.Errorf("unsupported BSG color mode %d", region[bsgColorModeField])
	}

	compression := BsgCompression(region[bsgCompressionField])
	if compression > BsgCompressionLz {
		return nil, fmt.Errorf("unsupported BSG compression %d", int(compression))
	}

	width := int(binutil.U16(region[bsgWidthOffset:]))
	height := int(binutil.U16(region[bsgHeightOffset:]))
	if width == 0 || height == 0 || width > bsgMaxDimension || height > bsgMaxDimension {
		return nil, fmt.Errorf("invalid BSG dimensions %dx%d", width, height)
	}

	if width*height > bsgMaxPixelCount {
		return nil, fmt.Errorf("BSG image is too large: %dx%d", width, height)
	}

	relativeDataOffset := int(binutil.I32(region[bsgDataOffsetField:]))
	dataSize := int(binutil.I32(region[bsgDataSizeField:]))
	dataOffset := baseOffset + relativeDataOffset
	if relativeDataOffset < 0 || dataSize < 0 || dataOffset > len(data) {
		return nil, errors.New("BSG pixel data is out of range")
	}

	available := min(dataSize, len(data)-dataOffset)
	if available <= 0 {
		return nil, errors.New("BSG pixel data is empty")
	}

	relativePaletteOffset := int(binutil.I32(region[bsgPaletteOffsetField:]))
	paletteOffset := 0
	if relativePaletteOffset > 0 {
		paletteOffset = baseOffset + relativePaletteOffset
	}

	frameCount := max(1, int(region[bsgFrameCountField]))

	return &BsgHeader{
		Width:         width,
		Height:        height,
		OffsetX:       int(int16(binutil.U16(region[bsgOffsetXField:]))),
		OffsetY:       int(int16(binutil.U16(region[bsgOffsetYField:]))),
		ClipLeft:      int(binutil.U16(region[bsgClipLeftField:])),
		ClipTop:       int(binutil.U16(region[bsgClipTopField:])),
		ClipWidth:     int(binutil.U16(region[bsgClipWidthField:])),
		ClipHeight:    int(binutil.U16(region[bsgClipHeightField:])),
		FrameIndex:    int(region[bsgFrameIndexField]),
		FrameCount:    frameCount,
		Alpha:         int(region[bsgAlphaField]),
		BlendMode:     int(region[bsgBlendModeField]),
		ColorMode:     colorMode,
		Compression:   compression,
		DataOffset:    dataOffset,
		DataSize:      available,
		BlockSize:     ReadBsgBlockSize(data, baseOffset),
		PaletteOffset: paletteOffset,
	}, nil
}
