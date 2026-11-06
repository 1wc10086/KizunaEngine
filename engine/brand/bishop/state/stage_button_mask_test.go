package state

import "testing"

// 地图卡掩码是“屏幕坐标系（行 0 = 顶部）”的非零像素集合：
// 命中与导出热区都必须基于同一套坐标，否则 KZK/屈辱2 的卡会整体错位。
func TestStageHitMaskHitAndBounds(t *testing.T) {
	mask := &StageHitMask{
		Width:  4,
		Height: 3,
		Values: []uint8{
			0, 0, 0, 0,
			0, 2, 2, 0,
			0, 0, 0, 0,
		},
	}
	if !mask.Hit(1, 1) || !mask.Hit(2, 1) {
		t.Fatalf("expected (1,1)/(2,1) to be clickable")
	}
	if mask.Hit(0, 0) || mask.Hit(3, 2) {
		t.Fatalf("zero pixels must not be clickable")
	}
	x, y, width, height, ok := mask.Bounds()
	if !ok || x != 1 || y != 1 || width != 2 || height != 1 {
		t.Fatalf("bounds = (%d,%d,%d,%d,%v), want (1,1,2,1,true)", x, y, width, height, ok)
	}
	if mask.ValueAt(-1, 0) != 0 || mask.ValueAt(0, 3) != 0 {
		t.Fatalf("out-of-range lookups must return 0")
	}
}

// 全零掩码必须返回 ok=false：调用方据此把按钮当成不可点，
// 不能退回整图矩形（那正是多张全屏卡重叠成同一热区的根因）。
func TestStageHitMaskEmptyBounds(t *testing.T) {
	mask := &StageHitMask{Width: 2, Height: 2, Values: make([]uint8, 4)}
	if _, _, _, _, ok := mask.Bounds(); ok {
		t.Fatalf("empty mask reported bounds")
	}
	if mask.Hit(0, 0) {
		t.Fatalf("empty mask reported a hit")
	}
}
