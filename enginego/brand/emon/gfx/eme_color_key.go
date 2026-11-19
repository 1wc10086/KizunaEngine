package gfx

const ColorKeyGreen = 0x00FF00

func IsTransparent(blue, green, red uint8) bool {
	return (int(red)<<16)|(int(green)<<8)|int(blue) == ColorKeyGreen
}

func Alpha(transparent bool) uint8 {
	if transparent {
		return 0
	}
	return 255
}
