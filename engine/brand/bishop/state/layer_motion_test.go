package state

import (
	"testing"
	"time"
)

func TestLayerMotionInterpolatesPositionAndOpacity(t *testing.T) {
	start := time.Now()
	motion := &LayerMotion{
		Start:  start,
		BaseX:  0,
		BaseY:  0,
		Width:  400,
		Height: 1000,
		Keyframes: []LayerMotionKeyframe{
			{At: 0, X: 512, Y: 100, HasPos: true, Opacity: 0.2, HasOpacity: true},
			{At: 1, X: 512, Y: 480, HasPos: true, Opacity: 1, HasOpacity: true},
		},
	}
	dx, dy, opacity, ok := motion.Transform(start.Add(500 * time.Millisecond))
	if !ok {
		t.Fatal("motion should be visible")
	}
	if dx != 312 {
		t.Fatalf("dx = %d, want 312", dx)
	}
	// 中点 y = 290，减去半高 500。
	if dy != -210 {
		t.Fatalf("dy = %d, want -210", dy)
	}
	if opacity < 0.59 || opacity > 0.61 {
		t.Fatalf("opacity = %v, want ~0.6", opacity)
	}
}

func TestLayerMotionHoldsLastValueAndCanHide(t *testing.T) {
	start := time.Now()
	motion := &LayerMotion{
		Start:  start,
		Width:  400,
		Height: 1000,
		Keyframes: []LayerMotionKeyframe{
			{At: 0, X: 512, Y: 480, HasPos: true, Opacity: 1, HasOpacity: true},
			{At: 1, Opacity: 0, HasOpacity: true},
		},
	}
	_, _, opacity, ok := motion.Transform(start.Add(5 * time.Second))
	if !ok || opacity != 0 {
		t.Fatalf("final opacity = %v ok=%v, want 0/true", opacity, ok)
	}
	if _, _, _, ok := NewHiddenLayerMotion().Transform(start); ok {
		t.Fatal("hidden motion should not draw")
	}
}
