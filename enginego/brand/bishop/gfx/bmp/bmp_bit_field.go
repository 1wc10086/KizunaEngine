package bmp

import "math/bits"

type BmpBitField struct {
	Mask    uint32
	Shift   int
	Maximum int
}

func BmpBitFieldFromMask(mask uint32) BmpBitField {
	if mask == 0 {
		return BmpBitField{}
	}

	shift := int(bits.TrailingZeros32(mask))
	return BmpBitField{Mask: mask, Shift: shift, Maximum: int(mask >> shift)}
}

func (f BmpBitField) Read(value uint32) byte {
	if f.Maximum <= 0 {
		return 0
	}

	return byte((uint64((value&f.Mask)>>f.Shift)*255 + uint64(f.Maximum)/2) / uint64(f.Maximum))
}
