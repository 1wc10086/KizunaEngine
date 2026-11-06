package gsa

import (
	"errors"

	"kizuna/engine/brand/bishop/gfx"
)

func IsOverlayName(name string) bool {
	if len(name) < 4 {
		return false
	}
	end := len(name)
	if name[end-4] != '.' {
		return false
	}
	kind := name[end-3]
	if kind != 'G' && kind != 'g' {
		return false
	}
	part := name[end-2]
	if part != '0' && part != '1' && part != 'S' && part != 's' {
		return false
	}
	digit := name[end-1]
	return digit >= '0' && digit <= '9'
}

func Blend(basis *gfx.BsgImage, overlays []*gfx.BsgImage) (*gfx.BsgImage, error) {
	if basis == nil {
		return nil, errors.New("GSA basis image is null")
	}
	if len(basis.Pixels) != basis.Width*basis.Height*4 {
		return nil, errors.New("GSA basis pixels do not match dimensions")
	}
	pixels := make([]byte, len(basis.Pixels))
	copy(pixels, basis.Pixels)
	if overlays != nil {
		for _, overlay := range overlays {
			if overlay == nil {
				continue
			}
			if len(overlay.Pixels) != overlay.Width*overlay.Height*4 {
				continue
			}
			blendOne(pixels, basis.Width, basis.Height, overlay)
		}
	}
	return gfx.NewBsgImage(basis.Width, basis.Height, basis.OffsetX, basis.OffsetY, pixels), nil
}

func blendOne(target []byte, baseWidth, baseHeight int, overlay *gfx.BsgImage) {
	overlayX := overlay.OffsetX
	overlayY := overlay.OffsetY
	overlayWidth := overlay.Width
	overlayHeight := overlay.Height
	if overlayX < 0 {
		overlayWidth += overlayX
		overlayX = 0
	}
	if overlayY < 0 {
		overlayHeight += overlayY
		overlayY = 0
	}
	if overlayX+overlayWidth > baseWidth {
		overlayWidth = baseWidth - overlayX
	}
	if overlayY+overlayHeight > baseHeight {
		overlayHeight = baseHeight - overlayY
	}
	if overlayWidth <= 0 || overlayHeight <= 0 {
		return
	}
	source := overlay.Pixels
	sourceStride := overlay.Width * 4
	targetStride := baseWidth * 4
	sourceTop := overlay.Height - overlayHeight
	targetTop := baseHeight - overlayY - overlayHeight
	for y := 0; y < overlayHeight; y++ {
		src := (sourceTop + y) * sourceStride
		dst := (targetTop+y)*targetStride + overlayX*4
		for x := 0; x < overlayWidth; x++ {
			alpha := int(source[src+3])
			if alpha > 0 {
				if alpha == 0xFF {
					target[dst] = source[src]
					target[dst+1] = source[src+1]
					target[dst+2] = source[src+2]
					target[dst+3] = source[src+3]
				} else {
					target[dst] = byte((int(source[src])*alpha + int(target[dst])*(0xFF-alpha)) / 0xFF)
					target[dst+1] = byte((int(source[src+1])*alpha + int(target[dst+1])*(0xFF-alpha)) / 0xFF)
					target[dst+2] = byte((int(source[src+2])*alpha + int(target[dst+2])*(0xFF-alpha)) / 0xFF)
				}
			}
			src += 4
			dst += 4
		}
	}
}
