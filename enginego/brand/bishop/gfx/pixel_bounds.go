package gfx

type PixelBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

func MeasurePixelBounds(bgra []byte, width, height int) PixelBounds {
	minX := width
	minY := height
	maxX := -1
	maxY := -1

	for y := 0; y < height; y++ {
		row := y * width
		for x := 0; x < width; x++ {
			if bgra[((row+x)*4)+3] == 0 {
				continue
			}

			if x < minX {
				minX = x
			}

			if x > maxX {
				maxX = x
			}

			if y < minY {
				minY = y
			}

			if y > maxY {
				maxY = y
			}
		}
	}

	if maxX < 0 {
		return PixelBounds{X: 0, Y: 0, Width: width, Height: height}
	}

	return PixelBounds{X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}
}
