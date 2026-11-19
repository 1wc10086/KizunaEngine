package stage

import (
	"testing"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
)

// TestHitTestNameSamplesAlpha 对应原引擎 GetOnSpriteName（sub_42A3F0）：
// 它除了矩形还会采样位图 alpha；而普通点击/选择命中只看矩形
// （标题菜单按钮的图形在背景图里，按钮图本身全透明）。
func TestHitTestNameSamplesAlpha(t *testing.T) {
	stg := NewEmonStage()
	sprite := stg.GetOrCreate("button", stg.Root())
	sprite.SetFlags(EmonSpriteFlagsVisible | EmonSpriteFlagsSelect)
	sprite.SetSize(2, 2)

	image := &bishopgfx.BsgImage{Width: 2, Height: 2, Pixels: make([]byte, 2*2*4)}
	image.Pixels[3] = 255 // 只有 (0,0) 不透明
	sprite.SetImage(image)

	if got := stg.HitTest(1, 0); got != sprite {
		t.Fatal("click hit test must stay rect-only")
	}
	if got := stg.HitTestName(0, 0); got != sprite {
		t.Fatal("opaque pixel must be reported by name hit test")
	}
	if got := stg.HitTestName(1, 0); got != nil {
		t.Fatal("transparent pixel must not be reported by name hit test")
	}
}
