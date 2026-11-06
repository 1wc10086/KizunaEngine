package gsa

import "kizuna/engine/core/binutil"

type GsaHeader struct {
	Type    int
	Width   int
	Height  int
	OffsetX int
	OffsetY int
}

func (h *GsaHeader) Is24Bit() bool {
	return h.Type == 3 || h.Type == 0x83
}

func ParseGsaHeader(data []byte) (*GsaHeader, bool) {
	if len(data) < 0xD4 {
		return nil, false
	}
	if !IsGsa(data) {
		return nil, false
	}
	headerType := int(binutil.I32(data[0xC0:0xC4]))
	width := binutil.U32(data[0xC4:0xC8])
	height := binutil.U32(data[0xC8:0xCC])
	offsetX := int(binutil.I32(data[0xCC:0xD0]))
	offsetY := int(binutil.I32(data[0xD0:0xD4]))
	if width == 0 || height == 0 || width > 8192 || height > 8192 {
		return nil, false
	}
	return &GsaHeader{
		Type:    headerType,
		Width:   int(width),
		Height:  int(height),
		OffsetX: offsetX,
		OffsetY: offsetY,
	}, true
}
