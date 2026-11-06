package rendering

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
)

func TrySpecialFill(canvas *gfx.BgraCanvas, name string) bool {
	switch name {
	case state.StageNameBlack:
		canvas.Clear(0, 0, 0, 255)
		return true
	case state.StageNameWhite:
		canvas.Clear(255, 255, 255, 255)
		return true
	case state.StageNameTransparent, state.StageNameReserved:
		canvas.Clear(0, 0, 0, 0)
		return true
	}
	if blue, green, red, alpha, ok := state.StageNameTryParseColor(name); ok {
		canvas.Clear(blue, green, red, alpha)
		return true
	}
	return false
}
