package gfx

import "kizuna/engine/core/binutil"

const (
	bsgRleHeaderSize = 4
	bsgRleMaxRepeat  = 129
)

func UnpackBsgRle(source []byte, output []byte, plane, pixelSize int) int {
	if len(source) < bsgRleHeaderSize || len(output) <= plane {
		return 0
	}

	remaining := int(binutil.I32(source))
	if remaining <= 0 {
		return bsgRleHeaderSize
	}

	position := bsgRleHeaderSize
	destination := plane

	for remaining > 0 && position < len(source) {
		code := source[position]
		position++
		remaining--

		if code < 0x80 {
			count := int(code) + 1
			available := min(count, len(source)-position)
			for i := 0; i < available; i++ {
				if uint(destination) < uint(len(output)) {
					output[destination] = source[position+i]
				}

				destination += pixelSize
			}

			position += available
			remaining -= available
			if available < count {
				return position
			}

			continue
		}

		repeat := 0x101 - int(code)
		if repeat > bsgRleMaxRepeat || position >= len(source) {
			return position
		}

		value := source[position]
		position++
		remaining--

		for i := 0; i < repeat; i++ {
			if uint(destination) >= uint(len(output)) {
				break
			}

			output[destination] = value
			destination += pixelSize
		}
	}

	return position
}
