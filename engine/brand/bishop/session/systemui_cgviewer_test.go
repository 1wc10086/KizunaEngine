package session

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
)

// systemui_cgviewer_test.go 覆盖 CG 查看器底栏新增控件的"纯几何"部分：
// 三个演出开关的 5 帧约定、BGV 音量条的区域/取值/滑块位置，
// 以及新按钮与前/后/退出按钮互不重叠。
//
// 这里只做几何检查，不加载素材：按钮图的真实尺寸要到运行时才知道，
// 而坐标与帧数是配置里写死的，重叠问题在配置这一层就能发现。

// cgViewerTestLayout 造一份和 bsxx.cfg 的 [cg viewer] 等价的布局。
func cgViewerTestLayout() config.CGViewer {
	return config.CGViewer{
		Base:  config.UIButton{Image: "cg_view_base", Frames: 1},
		Prev:  config.UIButton{Image: "cg_view_prev", Frames: 3, X: 482, Y: 2},
		Next:  config.UIButton{Image: "cg_view_next", Frames: 3, X: 604, Y: 2},
		Exit:  config.UIButton{Image: "cg_view_exit", Frames: 3, X: 967, Y: 2},
		CutIn: config.UIButton{Image: "cg_view_cutin", Frames: 5, X: 656, Y: 2},
		Hair:  config.UIButton{Image: "cg_view_hair", Frames: 5, X: 768, Y: 2},
		Steam: config.UIButton{Image: "cg_view_steam", Frames: 5, X: 850, Y: 2},
		// 音量条是单帧底图；详情按钮 3 帧。
		BGVVolume: config.UIButton{Image: "cg_view_volume_bar", Frames: 1, X: 162, Y: 3},
		BGVDetail: config.UIButton{Image: "cg_view_bgv_detail", Frames: 3, X: 365, Y: 2},
		// 音量条区域：x、y、宽、高。
		BGVVolumeRegion: [4]int{162, 3, 200, 23},
		NumberPatterns:  10,
		NowNoX:          548, NowNoY: 3, NowNoSpaceX: -12,
		AllThumbsX: 588, AllThumbsY: 3, AllThumbsSpaceX: -12,
	}
}

// rectsOverlap 判断两个矩形是否相交。
func rectsOverlap(a, b [4]int) bool {
	return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]
}

func TestCGViewerToggleFrames(t *testing.T) {
	// 5 帧约定：0 命中掩码、1 关、2 关的悬停、3 开、4 开的悬停。
	cases := []struct {
		on    bool
		hover bool
		want  int
	}{
		{false, false, 1},
		{false, true, 2},
		{true, false, 3},
		{true, true, 4},
	}
	for _, item := range cases {
		if got := cgViewerToggleFrame(item.on, item.hover); got != item.want {
			t.Fatalf("toggle frame(on=%v hover=%v) = %d, want %d", item.on, item.hover, got, item.want)
		}
	}
}

func TestCGViewerBGVVolumeValue(t *testing.T) {
	region := [4]int{162, 3, 200, 23}
	// 左端是 0，右端是 1000（千分比）。
	if got := cgBGVVolumeValue(region, 162); got != 0 {
		t.Fatalf("volume at left edge = %d, want 0", got)
	}
	if got := cgBGVVolumeValue(region, 262); got != 500 {
		t.Fatalf("volume at middle = %d, want 500", got)
	}
	if got := cgBGVVolumeValue(region, 362); got != 1000 {
		t.Fatalf("volume at right edge = %d, want 1000", got)
	}
	// 超出区域的两端要夹紧。
	if got := cgBGVVolumeValue(region, 0); got != 0 {
		t.Fatalf("volume before region = %d, want 0", got)
	}
	if got := cgBGVVolumeValue(region, 1024); got != 1000 {
		t.Fatalf("volume after region = %d, want 1000", got)
	}
	// 没有配置区域时不能算出音量。
	if got := cgBGVVolumeValue([4]int{}, 300); got != 0 {
		t.Fatalf("volume without region = %d, want 0", got)
	}
}

func TestCGViewerVolumeMarkerX(t *testing.T) {
	region := [4]int{162, 3, 200, 23}
	// 标记宽 3 像素：最左 162，最右 162+200-3 = 359。
	if got := cgViewerMarkerX(region, 0); got != 162 {
		t.Fatalf("marker at 0 = %d, want 162", got)
	}
	if got := cgViewerMarkerX(region, 1); got != 359 {
		t.Fatalf("marker at full = %d, want 359", got)
	}
	if got := cgViewerMarkerX(region, 0.5); got != 260 {
		t.Fatalf("marker at half = %d, want 260", got)
	}
	// 越界的音量要夹紧，不能滑到区域外面。
	if got := cgViewerMarkerX(region, 2); got != 359 {
		t.Fatalf("marker above full = %d, want 359", got)
	}
	if got := cgViewerMarkerX(region, -1); got != 162 {
		t.Fatalf("marker below zero = %d, want 162", got)
	}
}

func TestCGViewerButtonsDoNotOverlapNavigation(t *testing.T) {
	layout := cgViewerTestLayout()
	// 假设按钮图宽 40、高 100 像素（5 帧按钮的单帧高是 20）。
	const imageWidth, imageHeight = 40, 100
	nav := []struct {
		name   string
		button config.UIButton
	}{
		{"prev", layout.Prev},
		{"next", layout.Next},
		{"exit", layout.Exit},
	}
	added := []struct {
		name   string
		button config.UIButton
	}{
		{"cutin", layout.CutIn},
		{"hair", layout.Hair},
		{"steam", layout.Steam},
		{"bgv detail", layout.BGVDetail},
	}
	for _, base := range nav {
		x0, y0, x1, y1 := cgViewerButtonRect(base.button, imageWidth, imageHeight)
		baseRect := [4]int{x0, y0, x1, y1}
		for _, extra := range added {
			ex0, ey0, ex1, ey1 := cgViewerButtonRect(extra.button, imageWidth, imageHeight)
			if rectsOverlap(baseRect, [4]int{ex0, ey0, ex1, ey1}) {
				t.Fatalf("%s overlaps %s", base.name, extra.name)
			}
		}
	}
	// 音量条区域也不能压到导航按钮上。
	region := [4]int{layout.BGVVolumeRegion[0], layout.BGVVolumeRegion[1],
		layout.BGVVolumeRegion[0] + layout.BGVVolumeRegion[2],
		layout.BGVVolumeRegion[1] + layout.BGVVolumeRegion[3]}
	for _, base := range nav {
		x0, y0, x1, y1 := cgViewerButtonRect(base.button, imageWidth, imageHeight)
		if rectsOverlap([4]int{x0, y0, x1, y1}, region) {
			t.Fatalf("bgv volume region overlaps %s", base.name)
		}
	}
	// 三个演出开关彼此也不该重叠。
	for i := 0; i < len(added); i++ {
		x0, y0, x1, y1 := cgViewerButtonRect(added[i].button, imageWidth, imageHeight)
		for j := i + 1; j < len(added); j++ {
			ex0, ey0, ex1, ey1 := cgViewerButtonRect(added[j].button, imageWidth, imageHeight)
			if rectsOverlap([4]int{x0, y0, x1, y1}, [4]int{ex0, ey0, ex1, ey1}) {
				t.Fatalf("%s overlaps %s", added[i].name, added[j].name)
			}
		}
	}
}
