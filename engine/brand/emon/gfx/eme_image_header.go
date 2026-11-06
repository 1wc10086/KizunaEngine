package gfx

import (
	"encoding/binary"
	"errors"
)

type EmeImageHeader struct {
	BitsPerPixel int
	Width        int
	Height       int
	Colors       int
	Stride       int
	OffsetX      int
	OffsetY      int
}

func (h *EmeImageHeader) HasPalette() bool {
	return h.Colors != 0 && h.BitsPerPixel != 7
}

func (h *EmeImageHeader) PaletteBytes() int {
	if !h.HasPalette() {
		return 0
	}
	colors := h.Colors
	if colors < 3 {
		colors = 3
	}
	return colors * 4
}

func ParseEmeImageHeader(header32 []byte) (*EmeImageHeader, error) {
	if len(header32) < 32 {
		return nil, errors.New("EME image header is truncated")
	}
	return &EmeImageHeader{
		BitsPerPixel: int(binary.LittleEndian.Uint16(header32[0:2]) & 0xFF),
		Width:        int(binary.LittleEndian.Uint16(header32[2:4])),
		Height:       int(binary.LittleEndian.Uint16(header32[4:6])),
		Colors:       int(binary.LittleEndian.Uint16(header32[6:8])),
		Stride:       int(int32(binary.LittleEndian.Uint32(header32[8:12]))),
		OffsetX:      int(int32(binary.LittleEndian.Uint32(header32[0xC:0x10]))),
		OffsetY:      int(int32(binary.LittleEndian.Uint32(header32[0x10:0x14]))),
	}, nil
}
