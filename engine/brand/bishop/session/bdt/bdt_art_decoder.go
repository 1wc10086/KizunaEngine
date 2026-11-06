package bdt

import (
	"kizuna/engine/brand/bishop/gfx"
)

const (
	bdtMaskDarkLuma            = 40
	bdtMaskLightLuma           = 200
	bdtFigureLuma              = 24
	bdtMinimumFigurePercent    = 2
	bdtMinimumTonalPercent     = 90
	bdtMinimumCoverPercent     = 90
	bdtMinimumPairWidth        = 320
	bdtMinimumPairHeight       = 240
	bdtMinimumMaskLightPercent = 30
	bdtMinimumMaskDarkPercent  = 20
	bdtMaximumMaskMidPercent   = 10
)

func DecodeBdtArt(image *gfx.BsgImage) (*gfx.BsgImage, bool) {
	if image == nil {
		return nil, false
	}
	figure, ok := trySplitBdtMaskPair(image)
	if !ok {
		return image, false
	}
	return figure, true
}

func DecodeBdtArtWithVariant(image *gfx.BsgImage, variant BdtVariant) (*gfx.BsgImage, bool) {
	if image == nil {
		return nil, false
	}
	if !variant.UsesContainerArchive && !isBalancedBdtMaskPair(image) {
		return image, false
	}
	return DecodeBdtArt(image)
}

func isBalancedBdtMaskPair(image *gfx.BsgImage) bool {
	width := image.Width
	height := image.Height
	if width < bdtMinimumPairWidth || height < bdtMinimumPairHeight {
		return false
	}
	pixels := image.Pixels
	stride := image.Stride()
	if len(pixels) < stride*height {
		return false
	}
	half := width / 2
	sampled := 0
	light := 0
	dark := 0
	for y := 0; y < height; y += 2 {
		row := y * stride
		for x := half; x < width; x += 2 {
			offset := row + x*4
			luma := bdtLuma(pixels[offset], pixels[offset+1], pixels[offset+2])
			sampled++
			if luma > bdtMaskLightLuma {
				light++
			} else if luma < bdtMaskDarkLuma {
				dark++
			}
		}
	}
	if sampled == 0 {
		return false
	}
	mid := sampled - light - dark
	return light*100 >= sampled*bdtMinimumMaskLightPercent &&
		dark*100 >= sampled*bdtMinimumMaskDarkPercent &&
		mid*100 <= sampled*bdtMaximumMaskMidPercent
}

func trySplitBdtMaskPair(image *gfx.BsgImage) (*gfx.BsgImage, bool) {
	width := image.Width
	height := image.Height
	if width < 4 || width&1 != 0 || height <= 0 {
		return nil, false
	}
	pixels := image.Pixels
	stride := image.Stride()
	if len(pixels) < stride*height {
		return nil, false
	}
	half := width / 2
	sampled := 0
	tonal := 0
	subject := 0
	covered := 0
	for y := 0; y < height; y += 2 {
		row := y * stride
		for x := 0; x < half; x += 2 {
			figureOffset := row + x*4
			maskOffset := row + (x+half)*4
			maskLuma := bdtLuma(pixels[maskOffset], pixels[maskOffset+1], pixels[maskOffset+2])
			sampled++
			if maskLuma < bdtMaskDarkLuma || maskLuma > bdtMaskLightLuma {
				tonal++
			}
			if bdtLuma(pixels[figureOffset], pixels[figureOffset+1], pixels[figureOffset+2]) > bdtFigureLuma {
				subject++
				if maskLuma < bdtMaskDarkLuma {
					covered++
				}
			}
		}
	}
	if sampled == 0 || subject*100 < sampled*bdtMinimumFigurePercent {
		return nil, false
	}
	if tonal*100 < sampled*bdtMinimumTonalPercent || covered*100 < subject*bdtMinimumCoverPercent {
		return nil, false
	}
	keyed := make([]byte, half*height*4)
	for y := 0; y < height; y++ {
		row := y * stride
		targetRow := y * half * 4
		for x := 0; x < half; x++ {
			source := row + x*4
			mask := row + (x+half)*4
			target := targetRow + x*4
			keyed[target] = pixels[source]
			keyed[target+1] = pixels[source+1]
			keyed[target+2] = pixels[source+2]
			keyed[target+3] = byte(255 - bdtLuma(pixels[mask], pixels[mask+1], pixels[mask+2]))
		}
	}
	figure := gfx.NewBsgImage(half, height, image.OffsetX, image.OffsetY, keyed)
	figure.Parts = image.Parts
	figure.ContainerOffsetX = image.ContainerOffsetX
	figure.ContainerOffsetY = image.ContainerOffsetY
	figure.FrameIndex = image.FrameIndex
	figure.FrameCount = image.FrameCount
	figure.ClipLeft = image.ClipLeft
	figure.ClipTop = image.ClipTop
	figure.ClipWidth = image.ClipWidth
	figure.ClipHeight = image.ClipHeight
	figure.Alpha = image.Alpha
	figure.BlendMode = image.BlendMode
	return figure, true
}

func bdtLuma(blue, green, red byte) int {
	return (int(red)*299 + int(green)*587 + int(blue)*114) / 1000
}
