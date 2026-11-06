package lua

import (
	"testing"

	"kizuna/engine/brand/bishop/gfx"
)

func TestDIBDrawOffsetPrefersExplicitPosition(t *testing.T) {
	image := &gfx.BsgImage{OffsetX: 345, OffsetY: 53}
	// 显式传坐标（哪怕是 0,0）时不再叠加图片内嵌偏移。
	explicit := &dib{x: 7, y: 9, hasPos: true}
	if dx, dy := dibDrawOffset(explicit, image); dx != 7 || dy != 9 {
		t.Fatalf("explicit offset = %d,%d", dx, dy)
	}
	// 旧版脚本不传坐标：位置来自 BSG 头部的 offset_x/offset_y。
	implicit := &dib{}
	if dx, dy := dibDrawOffset(implicit, image); dx != 345 || dy != 53 {
		t.Fatalf("implicit offset = %d,%d", dx, dy)
	}
}
