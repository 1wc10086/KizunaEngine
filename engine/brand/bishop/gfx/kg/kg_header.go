package kg

import (
	"errors"

	"kizuna/engine/core/binutil"
)

const KgHeaderSize = 0x30

type KgHeader struct {
	Width         int
	Height        int
	HasAlpha      bool
	BitsPerPixel  int
	PaletteOffset int
	DataOffset    int
	AlphaOffset   int
}

func (h *KgHeader) PixelSize() int {
	return h.BitsPerPixel / 8
}

func ParseKgHeader(data []byte) (*KgHeader, error) {
	if len(data) < KgHeaderSize {
		return nil, errors.New("KG header is truncated")
	}
	if !IsKg(data) {
		return nil, errors.New("not a KG image")
	}
	width := int(binutil.U16(data[4:6]))
	height := int(binutil.U16(data[6:8]))
	if width <= 0 || height <= 0 {
		return nil, errors.New("KG dimensions are invalid")
	}
	if int64(width)*int64(height) > 64*1024*1024 {
		return nil, errors.New("KG dimensions are too large")
	}
	bitsPerPixel := 8
	if data[3] == 2 {
		bitsPerPixel = 24
	}
	paletteOffset := int(binutil.I32(data[0x0C:0x10]))
	dataOffset := int(binutil.I32(data[0x10:0x14]))
	alphaOffset := 0
	if data[2] == 2 {
		alphaOffset = int(binutil.I32(data[0x2C:0x30]))
	}
	hasAlpha := data[2] == 2 && bitsPerPixel == 24 && alphaOffset > 0 && alphaOffset < len(data)
	if dataOffset < 0 || dataOffset >= len(data) {
		return nil, errors.New("KG data offset is out of range")
	}
	if bitsPerPixel == 8 && (paletteOffset < 0 || paletteOffset >= len(data)) {
		return nil, errors.New("KG palette offset is out of range")
	}
	if hasAlpha && (alphaOffset < 0 || alphaOffset >= len(data)) {
		return nil, errors.New("KG alpha offset is out of range")
	}
	return &KgHeader{
		Width:         width,
		Height:        height,
		HasAlpha:      hasAlpha,
		BitsPerPixel:  bitsPerPixel,
		PaletteOffset: paletteOffset,
		DataOffset:    dataOffset,
		AlphaOffset:   alphaOffset,
	}, nil
}
