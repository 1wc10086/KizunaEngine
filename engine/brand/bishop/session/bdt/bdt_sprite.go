package bdt

import (
	"kizuna/engine/brand/bishop/gfx"
)

const BdtSpriteMirrorMode = 1

type BdtSprite struct {
	slot  int
	name  string
	x     int
	y     int
	mode  int
	image *gfx.BsgImage
}

func NewBdtSprite(slot int) *BdtSprite {
	return &BdtSprite{slot: slot}
}

func (s *BdtSprite) Slot() int {
	return s.slot
}

func (s *BdtSprite) Name() string {
	return s.name
}

func (s *BdtSprite) X() int {
	return s.x
}

func (s *BdtSprite) Y() int {
	return s.y
}

func (s *BdtSprite) Mode() int {
	return s.mode
}

func (s *BdtSprite) Image() *gfx.BsgImage {
	return s.image
}

func (s *BdtSprite) Visible() bool {
	return s.image != nil && len(s.name) > 0
}

func (s *BdtSprite) Set(name string, x, y, mode int, image *gfx.BsgImage) {
	s.name = name
	s.x = x
	s.y = y
	s.mode = mode
	s.image = prepareBdtSprite(image, mode, true)
}

func (s *BdtSprite) SetOpaque(name string, x, y, mode int, image *gfx.BsgImage) {
	s.name = name
	s.x = x
	s.y = y
	s.mode = mode
	s.image = prepareBdtSprite(image, mode, false)
}

func (s *BdtSprite) Clear() {
	s.name = ""
	s.x = 0
	s.y = 0
	s.mode = 0
	s.image = nil
}

func prepareBdtSprite(image *gfx.BsgImage, mode int, keyTransparency bool) *gfx.BsgImage {
	if image == nil {
		return nil
	}
	keyed := image
	if keyTransparency {
		keyed = withBdtColorkey(image)
	}
	if mode == BdtSpriteMirrorMode {
		return mirrorBdtImage(keyed)
	}
	return keyed
}

func withBdtColorkey(image *gfx.BsgImage) *gfx.BsgImage {
	pixels := image.Pixels
	if len(pixels) < 4 {
		return image
	}
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 255 {
			return image
		}
	}
	keyBlue := pixels[0]
	keyGreen := pixels[1]
	keyRed := pixels[2]
	copyPixels := make([]byte, len(pixels))
	copy(copyPixels, pixels)
	for offset := 0; offset+4 <= len(copyPixels); offset += 4 {
		if copyPixels[offset] == keyBlue && copyPixels[offset+1] == keyGreen && copyPixels[offset+2] == keyRed {
			copyPixels[offset+3] = 0
		}
	}
	keyed := gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, copyPixels)
	keyed.Parts = image.Parts
	keyed.ContainerOffsetX = image.ContainerOffsetX
	keyed.ContainerOffsetY = image.ContainerOffsetY
	keyed.FrameIndex = image.FrameIndex
	keyed.FrameCount = image.FrameCount
	keyed.ClipLeft = image.ClipLeft
	keyed.ClipTop = image.ClipTop
	keyed.ClipWidth = image.ClipWidth
	keyed.ClipHeight = image.ClipHeight
	keyed.Alpha = image.Alpha
	keyed.BlendMode = image.BlendMode
	return keyed
}

func mirrorBdtImage(image *gfx.BsgImage) *gfx.BsgImage {
	pixels := image.Pixels
	if image.Width <= 0 || image.Height <= 0 || len(pixels) < image.Width*image.Height*4 {
		return image
	}
	flipped := make([]byte, len(pixels))
	stride := image.Stride()
	for row := 0; row < image.Height; row++ {
		base := row * stride
		for column := 0; column < image.Width; column++ {
			source := base + column*4
			target := base + (image.Width-1-column)*4
			flipped[target] = pixels[source]
			flipped[target+1] = pixels[source+1]
			flipped[target+2] = pixels[source+2]
			flipped[target+3] = pixels[source+3]
		}
	}
	mirrored := gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, flipped)
	mirrored.Parts = image.Parts
	mirrored.ContainerOffsetX = image.ContainerOffsetX
	mirrored.ContainerOffsetY = image.ContainerOffsetY
	mirrored.FrameIndex = image.FrameIndex
	mirrored.FrameCount = image.FrameCount
	mirrored.ClipLeft = image.ClipLeft
	mirrored.ClipTop = image.ClipTop
	mirrored.ClipWidth = image.ClipWidth
	mirrored.ClipHeight = image.ClipHeight
	mirrored.Alpha = image.Alpha
	mirrored.BlendMode = image.BlendMode
	return mirrored
}
