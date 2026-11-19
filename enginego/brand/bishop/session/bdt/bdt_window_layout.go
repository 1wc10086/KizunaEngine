package bdt

import "math"

const (
	ScreenWidth      = 640
	ScreenHeight     = 480
	ThumbnailWidth   = 160
	ThumbnailHeight  = 120
	ScenarioName     = "scene00.bdt"
	SystemScriptName = "system.bdt"
	SpriteSlots      = 8
)

func WindowRect(variant BdtVariant) (int, int, int, int) {
	return variant.WindowLeft, variant.WindowTop, variant.WindowWidth, variant.WindowHeight
}

func FaceRect(variant BdtVariant) (int, int, int) {
	return variant.FaceLeft, variant.FaceTop, variant.FaceSize
}

func TextLayout(variant BdtVariant) (int, int, int, int) {
	return variant.TextLeft, variant.TextTop, variant.TextLineHeight, variant.TextCharsPerLine
}

func VariantSystemBarImage(variant BdtVariant) string {
	return variant.SystemBarImage
}

func WaitMilliseconds(value int) int {
	if value <= 0 {
		return 0
	}
	milliseconds := int64(value) * 1000
	if milliseconds > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(milliseconds)
}
