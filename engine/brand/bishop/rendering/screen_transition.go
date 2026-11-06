package rendering

import (
	"math"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
)

const (
	transitionRampLength    = 255.0
	transitionProgressScale = 2.0
	transitionWhite         = 255
)

type ScreenTransition struct {
	images  *ImageCache
	columns []int
	mask    *gfx.BsgImage
	maskSet bool
	maskKey string
}

func NewScreenTransition(images *ImageCache, width int) *ScreenTransition {
	return &ScreenTransition{images: images, columns: make([]int, width)}
}

func (t *ScreenTransition) SetMask(name string) {
	if t.maskSet && strings.EqualFold(name, t.maskKey) {
		return
	}
	t.maskSet = true
	t.maskKey = name
	t.mask = t.images.Get(name)
}

func (t *ScreenTransition) Apply(target, history *gfx.BgraCanvas, transitionType int, progress float64, allowMask bool) {
	progress = clampFloat(progress, 0, 1)
	if transitionType == state.NoTransition || progress >= 1 {
		return
	}
	if progress <= 0 {
		target.CopyFrom(history)
		return
	}
	if transitionType == state.WhiteFlashTransition {
		flashTransition(target, progress)
		return
	}
	if transitionType == state.MaskTransition && allowMask && t.mask != nil {
		t.dissolve(target, history, progress)
		return
	}
	crossFade(target, history, progress)
}

func crossFade(target, history *gfx.BgraCanvas, progress float64) {
	mix := int(math.RoundToEven(progress * transitionWhite))
	if mix <= 0 {
		target.CopyFrom(history)
		return
	}
	if mix >= transitionWhite {
		return
	}
	targetPixels := target.Buffer
	historyPixels := history.Buffer
	inverse := transitionWhite - mix
	for offset := 0; offset+4 <= len(targetPixels) && offset+4 <= len(historyPixels); offset += 4 {
		targetPixels[offset] = byte((int(historyPixels[offset])*inverse + int(targetPixels[offset])*mix + 127) / transitionWhite)
		targetPixels[offset+1] = byte((int(historyPixels[offset+1])*inverse + int(targetPixels[offset+1])*mix + 127) / transitionWhite)
		targetPixels[offset+2] = byte((int(historyPixels[offset+2])*inverse + int(targetPixels[offset+2])*mix + 127) / transitionWhite)
		targetPixels[offset+3] = 255
	}
}

func flashTransition(target *gfx.BgraCanvas, progress float64) {
	alpha := int(math.RoundToEven((1.0 - progress) * transitionWhite))
	if alpha <= 0 {
		return
	}
	target.Overlay(byte(transitionWhite), byte(transitionWhite), byte(transitionWhite), alpha)
}

func (t *ScreenTransition) dissolve(target, history *gfx.BgraCanvas, progress float64) {
	mask := t.mask
	if mask == nil {
		return
	}
	threshold := progress * transitionProgressScale * transitionRampLength
	if threshold >= transitionRampLength*2.0 {
		return
	}
	width := target.Width
	height := target.Height
	targetPixels := target.Buffer
	historyPixels := history.Buffer
	maskPixels := mask.Pixels
	t.fillColumns(mask.Width, width)
	for row := 0; row < height; row++ {
		maskRow := int(int64(row)*int64(mask.Height)/int64(height)) * mask.Width
		targetRow := row * width * 4
		for column := 0; column < width; column++ {
			maskOffset := (maskRow + t.columns[column]) * 4
			value := (int(maskPixels[maskOffset])*29 + int(maskPixels[maskOffset+1])*150 + int(maskPixels[maskOffset+2])*77) >> 8
			ratio := threshold - float64(value)
			offset := targetRow + column*4
			if ratio <= 0 {
				copy(targetPixels[offset:offset+4], historyPixels[offset:offset+4])
				continue
			}
			if ratio >= transitionRampLength {
				continue
			}
			blendTransition(targetPixels, historyPixels, offset, ratio)
		}
	}
}

func (t *ScreenTransition) fillColumns(maskWidth, width int) {
	length := min(width, len(t.columns))
	for column := 0; column < length; column++ {
		t.columns[column] = int(int64(column) * int64(maskWidth) / int64(width))
	}
}

func blendTransition(target, history []byte, offset int, ratio float64) {
	mix := int(math.RoundToEven(ratio))
	if mix <= 0 {
		copy(target[offset:offset+4], history[offset:offset+4])
		return
	}
	if mix >= 255 {
		return
	}
	inverse := 255 - mix
	target[offset] = byte((int(history[offset])*inverse + int(target[offset])*mix + 127) / 255)
	target[offset+1] = byte((int(history[offset+1])*inverse + int(target[offset+1])*mix + 127) / 255)
	target[offset+2] = byte((int(history[offset+2])*inverse + int(target[offset+2])*mix + 127) / 255)
	target[offset+3] = 255
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
