package stage

import (
	"math"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

const (
	effectFramesPerSecond = 60.0
	blockOrderX           = uint32(73856093)
	blockOrderY           = uint32(19349663)
	blockOrderSeed        = uint32(83492791)
)

type EmonEffect struct {
	kind     EmonEffectKind
	frames   int
	param2   int
	param3   int
	progress float64
}

func NewEmonEffect(kind EmonEffectKind, frames, param2, param3 int) *EmonEffect {
	if frames < 1 {
		frames = 1
	}
	return &EmonEffect{kind: kind, frames: frames, param2: param2, param3: param3}
}

func (e *EmonEffect) Kind() EmonEffectKind {
	return e.kind
}

func (e *EmonEffect) Frames() int {
	return e.frames
}

func (e *EmonEffect) Param2() int {
	return e.param2
}

func (e *EmonEffect) Param3() int {
	return e.param3
}

func (e *EmonEffect) Progress() float64 {
	return e.progress
}

func (e *EmonEffect) Active() bool {
	return e.progress < 1
}

func (e *EmonEffect) Advance(deltaSeconds float64) {
	if e.progress >= 1 || deltaSeconds <= 0 {
		return
	}

	e.progress = math.Min(1, e.progress+deltaSeconds/(float64(e.frames)/effectFramesPerSecond))
}

func (e *EmonEffect) Apply(from, to, target *bishopgfx.BgraCanvas) {
	e.ApplyWith(from, to, target, nil, nil)
}

func (e *EmonEffect) ApplyWith(from, to, target, scratch *bishopgfx.BgraCanvas, patterns MaskPatternSource) {
	if e.progress <= 0 {
		copyEffectCanvas(from, target)
		return
	}

	if e.progress >= 1 {
		copyEffectCanvas(to, target)
		return
	}

	switch e.kind {
	case EmonEffectKindMixBlend:
		applyBlend(from, to, target, e.progress)
	case EmonEffectKindMaskFade:
		applyMaskFade(from, to, target, e.progress, e.param2, e.param3, patterns)
	case EmonEffectKindLeftRightScroll:
		applyScroll(from, to, target, e.progress, true, e.param2, scratch)
	case EmonEffectKindTopBottomScroll:
		applyScroll(from, to, target, e.progress, false, e.param2, scratch)
	default:
		copyEffectCanvas(to, target)
	}
}

func copyEffectCanvas(source, target *bishopgfx.BgraCanvas) {
	if source == target {
		return
	}

	if source.Width == target.Width && source.Height == target.Height {
		target.CopyFrom(source)
		return
	}

	width := min(source.Width, target.Width)
	height := min(source.Height, target.Height)
	sourcePixels := source.Buffer
	targetPixels := target.Buffer
	target.Clear(0, 0, 0, 255)

	for y := 0; y < height; y++ {
		copy(targetPixels[y*target.Width*4:], sourcePixels[y*source.Width*4:y*source.Width*4+width*4])
	}
}

func applyBlend(from, to, target *bishopgfx.BgraCanvas, progress float64) {
	fromPixels := from.Buffer
	toPixels := to.Buffer
	targetPixels := target.Buffer
	ahead := 1 - progress

	for y := 0; y < target.Height; y++ {
		fromRow := min(y, from.Height-1) * from.Width
		toRow := min(y, to.Height-1) * to.Width
		targetRow := y * target.Width

		for x := 0; x < target.Width; x++ {
			fromOffset := (fromRow + min(x, from.Width-1)) * 4
			toOffset := (toRow + min(x, to.Width-1)) * 4
			targetOffset := (targetRow + x) * 4
			targetPixels[targetOffset] = byte(float64(fromPixels[fromOffset])*ahead + float64(toPixels[toOffset])*progress)
			targetPixels[targetOffset+1] = byte(float64(fromPixels[fromOffset+1])*ahead + float64(toPixels[toOffset+1])*progress)
			targetPixels[targetOffset+2] = byte(float64(fromPixels[fromOffset+2])*ahead + float64(toPixels[toOffset+2])*progress)
			targetPixels[targetOffset+3] = byte(float64(fromPixels[fromOffset+3])*ahead + float64(toPixels[toOffset+3])*progress)
		}
	}
}

func applyMaskFade(from, to, target *bishopgfx.BgraCanvas, progress float64, pattern, blockSize int, patterns MaskPatternSource) {
	fromPixels := from.Buffer
	toPixels := to.Buffer
	targetPixels := target.Buffer
	if patterns != nil {
		image := patterns.Get(pattern)
		if image != nil && image.Width > 0 && image.Height > 0 {
			applyPatternFade(from, to, target, progress, image)
			return
		}
	}

	block := max(1, core.Abs(blockSize))
	threshold := uint32(progress * float64(math.MaxUint32))

	for y := 0; y < target.Height; y++ {
		blockY := uint32(y / block)
		fromRow := min(y, from.Height-1) * from.Width
		toRow := min(y, to.Height-1) * to.Width
		targetRow := y * target.Width

		for x := 0; x < target.Width; x++ {
			blockX := uint32(x / block)
			order := blockX*blockOrderX ^ blockY*blockOrderY ^ uint32(pattern)*blockOrderSeed
			takeTo := order <= threshold
			sourceRow := fromRow
			sourceWidth := from.Width
			sourcePixels := fromPixels
			if takeTo {
				sourceRow = toRow
				sourceWidth = to.Width
				sourcePixels = toPixels
			}

			sourceOffset := (sourceRow + min(x, sourceWidth-1)) * 4
			targetOffset := (targetRow + x) * 4
			targetPixels[targetOffset] = sourcePixels[sourceOffset]
			targetPixels[targetOffset+1] = sourcePixels[sourceOffset+1]
			targetPixels[targetOffset+2] = sourcePixels[sourceOffset+2]
			targetPixels[targetOffset+3] = sourcePixels[sourceOffset+3]
		}
	}
}

func applyPatternFade(from, to, target *bishopgfx.BgraCanvas, progress float64, pattern *bishopgfx.BsgImage) {
	fromPixels := from.Buffer
	toPixels := to.Buffer
	targetPixels := target.Buffer
	patternPixels := pattern.Pixels
	limit := progress * 255.0

	for y := 0; y < target.Height; y++ {
		patternY := min(y, pattern.Height-1)
		fromRow := min(y, from.Height-1) * from.Width
		toRow := min(y, to.Height-1) * to.Width
		targetRow := y * target.Width

		for x := 0; x < target.Width; x++ {
			patternX := min(x, pattern.Width-1)
			takeTo := float64(Luminance(patternPixels, (patternY*pattern.Width+patternX)*4)) <= limit
			sourceRow := fromRow
			sourceWidth := from.Width
			sourcePixels := fromPixels
			if takeTo {
				sourceRow = toRow
				sourceWidth = to.Width
				sourcePixels = toPixels
			}

			sourceOffset := (sourceRow + min(x, sourceWidth-1)) * 4
			targetOffset := (targetRow + x) * 4
			targetPixels[targetOffset] = sourcePixels[sourceOffset]
			targetPixels[targetOffset+1] = sourcePixels[sourceOffset+1]
			targetPixels[targetOffset+2] = sourcePixels[sourceOffset+2]
			targetPixels[targetOffset+3] = sourcePixels[sourceOffset+3]
		}
	}
}

func applyScroll(from, to, target *bishopgfx.BgraCanvas, progress float64, horizontal bool, direction int, scratch *bishopgfx.BgraCanvas) {
	fromPixels := resolveEffectSource(from, target, nil)
	toPixels := resolveEffectSource(to, target, scratch)
	targetPixels := target.Buffer
	reverse := direction < 0
	horizontalShift := int(math.RoundToEven(progress * float64(target.Width)))

	for y := 0; y < target.Height; y++ {
		verticalShift := int(math.RoundToEven(progress * float64(target.Height)))

		for x := 0; x < target.Width; x++ {
			takeTo := false
			position := 0
			if horizontal {
				takeTo = takeScrolled(x, target.Width, horizontalShift, reverse)
				position = scrollSource(x, target.Width, horizontalShift, reverse, takeTo)
			} else {
				takeTo = takeScrolled(y, target.Height, verticalShift, reverse)
				position = scrollSource(y, target.Height, verticalShift, reverse, takeTo)
			}

			sourcePixels := fromPixels
			sourceWidth := from.Width
			sourceHeight := from.Height
			if takeTo {
				sourcePixels = toPixels
				sourceWidth = to.Width
				sourceHeight = to.Height
			}

			sourceX := 0
			sourceY := 0
			if horizontal {
				sourceX = core.Clamp(position, 0, sourceWidth-1)
				sourceY = min(y, sourceHeight-1)
			} else {
				sourceX = min(x, sourceWidth-1)
				sourceY = core.Clamp(position, 0, sourceHeight-1)
			}

			sourceOffset := (sourceY*sourceWidth + sourceX) * 4
			targetOffset := (y*target.Width + x) * 4
			targetPixels[targetOffset] = sourcePixels[sourceOffset]
			targetPixels[targetOffset+1] = sourcePixels[sourceOffset+1]
			targetPixels[targetOffset+2] = sourcePixels[sourceOffset+2]
			targetPixels[targetOffset+3] = sourcePixels[sourceOffset+3]
		}
	}
}

func takeScrolled(position, extent, shift int, reverse bool) bool {
	if reverse {
		return position < shift
	}
	return position >= extent-shift
}

func scrollSource(position, extent, shift int, reverse, takeTo bool) int {
	if reverse {
		if takeTo {
			return position + extent - shift
		}
		return position - shift
	}

	if takeTo {
		return position - (extent - shift)
	}
	return position + shift
}

func resolveEffectSource(source, target, scratch *bishopgfx.BgraCanvas) []byte {
	if source != target {
		return source.Buffer
	}

	if scratch != nil && scratch.Width == target.Width && scratch.Height == target.Height {
		scratch.CopyFrom(source)
		return scratch.Buffer
	}

	duplicate := make([]byte, len(source.Buffer))
	copy(duplicate, source.Buffer)
	return duplicate
}
