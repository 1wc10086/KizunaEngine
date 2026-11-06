package gfx

func blendPixel(source []byte, sourceOffset int, target []byte, targetOffset int, alpha int) {
	target[targetOffset] = mixPixel(source[sourceOffset], target[targetOffset], alpha)
	target[targetOffset+1] = mixPixel(source[sourceOffset+1], target[targetOffset+1], alpha)
	target[targetOffset+2] = mixPixel(source[sourceOffset+2], target[targetOffset+2], alpha)
	target[targetOffset+3] = 255
}

func copyPixel(source []byte, sourceOffset int, target []byte, targetOffset int) {
	target[targetOffset] = source[sourceOffset]
	target[targetOffset+1] = source[sourceOffset+1]
	target[targetOffset+2] = source[sourceOffset+2]
	target[targetOffset+3] = 255
}

func overlayPixel(target []byte, offset int, blue, green, red byte, alpha int) {
	target[offset] = mixPixel(blue, target[offset], alpha)
	target[offset+1] = mixPixel(green, target[offset+1], alpha)
	target[offset+2] = mixPixel(red, target[offset+2], alpha)
	target[offset+3] = 255
}

func blendRows(source []byte, sourceOffset, sourceStride int, target []byte, targetOffset, targetStride, width, height, alpha int) {
	for row := 0; row < height; row++ {
		sourceRow := sourceOffset + (row * sourceStride)
		targetRow := targetOffset + (row * targetStride)
		for column := 0; column < width; column++ {
			index := column * 4
			sourceAlpha := source[sourceRow+index+3]
			if sourceAlpha == 0 {
				continue
			}

			slot := targetRow + index
			if alpha >= 255 && sourceAlpha == 255 {
				copyPixel(source, sourceRow+index, target, slot)
				continue
			}

			effective := int(sourceAlpha)
			if alpha < 255 {
				effective = (int(sourceAlpha) * alpha) / 255
			}

			if effective <= 0 {
				continue
			}

			blendPixel(source, sourceRow+index, target, slot, effective)
		}
	}
}

func overlayRows(target []byte, offset, stride, width, height int, blue, green, red byte, alpha int) {
	for row := 0; row < height; row++ {
		rowOffset := offset + (row * stride)
		for column := 0; column < width; column++ {
			overlayPixel(target, rowOffset+(column*4), blue, green, red, alpha)
		}
	}
}

func mixPixel(source, target byte, alpha int) byte {
	return byte(((int(source) * alpha) + (int(target) * (255 - alpha)) + 127) / 255)
}
