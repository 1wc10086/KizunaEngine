package resource

import (
	"errors"
)

const (
	zbmTokenBits       = 8
	zbmMatchOffsetBits = 10
	zbmLiteralBits     = 8
	zbmFlagMask        = 0x80
	zbmLengthMask      = 0x7F
)

func ZbmUnpack(source []byte, capacityHint int) ([]byte, error) {
	if len(source) == 0 {
		return []byte{}, nil
	}

	var output []byte
	if capacityHint > 0 {
		output = make([]byte, 0, capacityHint)
	}

	bits := newZbmBitReader(source)

	for {
		token := bits.readBits(zbmTokenBits)
		if token < 0 {
			break
		}

		if token&zbmFlagMask != 0 {
			count := token & zbmLengthMask
			offset := bits.readBits(zbmMatchOffsetBits)
			if offset < 0 {
				return nil, errors.New("truncated ZBM match offset")
			}
			if offset <= 0 || offset > len(output) {
				return nil, errors.New("invalid ZBM match offset")
			}

			for i := 0; i < count; i++ {
				output = append(output, output[len(output)-offset])
			}

			continue
		}

		if token == 0 {
			break
		}

		for i := 0; i < token; i++ {
			value := bits.readBits(zbmLiteralBits)
			if value < 0 {
				return nil, errors.New("truncated ZBM literal")
			}

			output = append(output, byte(value))
		}
	}

	return output, nil
}

type zbmBitReader struct {
	source      []byte
	position    int
	accumulator int
	available   int
}

func newZbmBitReader(source []byte) *zbmBitReader {
	return &zbmBitReader{
		source:      source,
		position:    1,
		accumulator: int(source[0] & 0x7F),
		available:   7,
	}
}

func (b *zbmBitReader) readBits(count int) int {
	value := 0
	for i := 0; i < count; i++ {
		bit := b.readBit()
		if bit < 0 {
			return -1
		}

		value = (value << 1) | bit
	}

	return value
}

func (b *zbmBitReader) readBit() int {
	if b.available == 0 {
		if b.position >= len(b.source) {
			return -1
		}

		b.accumulator = int(b.source[b.position])
		b.position++
		b.available = 8
	}

	b.available--
	return (b.accumulator >> b.available) & 1
}
