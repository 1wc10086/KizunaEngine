package gfx

import (
	"testing"

	"kizuna/enginego/core"
)

func runAt(x, y int32) core.TextRun {
	return core.TextRun{Text: "あ", X: x, Y: y, Size: 20, Color: 0xffffffff, Outline: 0xff000000, Shadow: 1}
}

// 文字跟着像素走：整块拷贝保留全部，区域拷贝只保留基线落在源矩形内的。
func TestCanvasRunsCopyAndRegion(t *testing.T) {
	source := NewBgraCanvas(8, 8)
	source.AppendRuns([]core.TextRun{runAt(2, 3), runAt(6, 7)})
	if len(source.Runs) != 2 {
		t.Fatalf("AppendRuns kept %d runs, want 2", len(source.Runs))
	}

	target := NewBgraCanvas(8, 8)
	target.CopyFrom(source)
	if len(target.Runs) != 2 || target.Runs[0] != source.Runs[0] {
		t.Fatalf("CopyFrom runs = %+v, want %+v", target.Runs, source.Runs)
	}

	// 只搬 (4,4) 起的 4x4：只有基线 (6,7) 在源矩形内，并平移到 (6,7)。
	region := NewBgraCanvas(8, 8)
	region.BlitCanvasRegion(source, 4, 4, 4, 4, 6, 7, 255)
	if len(region.Runs) != 1 {
		t.Fatalf("BlitCanvasRegion kept %d runs, want 1", len(region.Runs))
	}
	if region.Runs[0].X != 8 || region.Runs[0].Y != 10 {
		t.Fatalf("translated run at (%d,%d), want (8,10)", region.Runs[0].X, region.Runs[0].Y)
	}

	whole := NewBgraCanvas(8, 8)
	whole.BlitCanvas(source, 255)
	if len(whole.Runs) != 2 {
		t.Fatalf("BlitCanvas kept %d runs, want 2", len(whole.Runs))
	}
}

// alpha < 255 的叠加会把整层淡出，文字与描边必须跟着淡，否则淡入淡出时
// 文字会在底图上留下硬边。
func TestCanvasRunsFadeWithAlpha(t *testing.T) {
	source := NewBgraCanvas(8, 8)
	source.AppendRuns([]core.TextRun{runAt(0, 0)})

	target := NewBgraCanvas(8, 8)
	target.BlitCanvas(source, 128)
	if len(target.Runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(target.Runs))
	}
	for name, got := range map[string]uint32{
		"color":   target.Runs[0].Color,
		"outline": target.Runs[0].Outline,
	} {
		if alpha := got >> 24; alpha != 128 {
			t.Fatalf("%s alpha %d, want 128", name, alpha)
		}
	}
	if target.Runs[0].Color&0x00ffffff != 0x00ffffff {
		t.Fatalf("fade must keep RGB: %#x", target.Runs[0].Color)
	}
	// 不透明叠加不动颜色。
	opaque := NewBgraCanvas(8, 8)
	opaque.BlitCanvas(source, 255)
	if opaque.Runs[0] != source.Runs[0] {
		t.Fatalf("opaque blit changed run: %+v", opaque.Runs[0])
	}
}

// 清屏与取走都要把文字清空，避免复用画布时把上一帧的字再交一遍。
func TestCanvasRunsClearAndTake(t *testing.T) {
	canvas := NewBgraCanvas(4, 4)
	canvas.AppendRuns([]core.TextRun{runAt(1, 2)})
	taken := canvas.TakeRuns()
	if len(taken) != 1 || taken[0].X != 1 {
		t.Fatalf("TakeRuns = %+v", taken)
	}
	if len(canvas.Runs) != 0 {
		t.Fatalf("TakeRuns left %d runs", len(canvas.Runs))
	}

	canvas.AppendRuns([]core.TextRun{runAt(1, 2)})
	canvas.Clear(0, 0, 0, 255)
	if len(canvas.Runs) != 0 {
		t.Fatalf("Clear left %d runs", len(canvas.Runs))
	}
}

// 缩放把文字坐标按同一比例换算。
func TestCanvasRunsScale(t *testing.T) {
	source := NewBgraCanvas(8, 8)
	source.AppendRuns([]core.TextRun{runAt(4, 4)})
	scaled := source.ScaleTo(4, 2)
	if scaled.Width != 4 || scaled.Height != 2 {
		t.Fatalf("scaled canvas %dx%d", scaled.Width, scaled.Height)
	}
	if len(scaled.Runs) != 1 {
		t.Fatalf("ScaleTo kept %d runs, want 1", len(scaled.Runs))
	}
	if scaled.Runs[0].X != 2 || scaled.Runs[0].Y != 1 {
		t.Fatalf("scaled run at (%d,%d), want (2,1)", scaled.Runs[0].X, scaled.Runs[0].Y)
	}
}
