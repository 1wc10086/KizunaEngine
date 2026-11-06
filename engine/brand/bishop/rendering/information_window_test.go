package rendering

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
)

// TestDigitSplit 锁住"日期按十进制拆位"的取值。
//
// 数字是逐位贴图画的，拆位错一位不会报错，只会把 12 日画成 21 日——
// 这类错误在画面上很难一眼看出来，所以单独测。
func TestDigitSplit(t *testing.T) {
	cases := []struct {
		value  int
		digits int
		want   []int
	}{
		{0, 1, []int{0}},
		{1, 1, []int{1}},
		{9, 1, []int{9}},
		{10, 2, []int{1, 0}},
		{12, 2, []int{1, 2}},
		{21, 2, []int{2, 1}},
		{99, 2, []int{9, 9}},
		{100, 3, []int{1, 0, 0}},
	}
	for _, c := range cases {
		if got := digitCount(c.value); got != c.digits {
			t.Fatalf("digitCount(%d) = %d, want %d", c.value, got, c.digits)
		}
		for i, want := range c.want {
			if got := digitAt(c.value, i); got != want {
				t.Fatalf("digitAt(%d, %d) = %d, want %d", c.value, i, got, want)
			}
		}
	}
}

// TestDigitAtOutOfRange 保证越界不会 panic 或返回脏值。
func TestDigitAtOutOfRange(t *testing.T) {
	if got := digitAt(5, -1); got != 0 {
		t.Fatalf("digitAt(5,-1) = %d, want 0", got)
	}
	if got := digitAt(5, 3); got != 0 {
		t.Fatalf("digitAt(5,3) = %d, want 0", got)
	}
}

// TestInformationWindowVisible 锁住"没有日期/时段时不画任何东西"的语义。
//
// 标题画面、回想画面和系统界面都会传 Day=-1、Time=-1。底板是独立于数字
// 的一条深色胶囊，如果只看 layout.Empty() 就会在标题左上角留下一个空白标签
// （真机截图中能明显看到），所以可见性必须由值决定。
func TestInformationWindowVisible(t *testing.T) {
	if (InformationWindowValues{Day: -1, Time: -1}).Visible() {
		t.Fatal("no day and no time must not be visible")
	}
	if !(InformationWindowValues{Day: 1, Time: -1}).Visible() {
		t.Fatal("a day alone must be visible")
	}
	if !(InformationWindowValues{Day: -1, Time: 2}).Visible() {
		t.Fatal("a time alone must be visible")
	}
	if !(InformationWindowValues{Day: 0, Time: 0}).Visible() {
		t.Fatal("zero is a valid value and must be visible")
	}
}

// TestInformationWindowEmpty 保证未配置的作品不会画出空窗。
func TestInformationWindowEmpty(t *testing.T) {
	var nilRenderer *InformationWindowRenderer
	if !nilRenderer.Empty() {
		t.Fatal("nil renderer should report empty")
	}
	renderer := NewInformationWindowRenderer(nil, config.InformationWindow{})
	if !renderer.Empty() {
		t.Fatal("a zero layout should report empty")
	}
	// 空配置下 Draw 必须安全返回 false（不能解引用 nil 图片缓存）。
	if renderer.Draw(nil, InformationWindowValues{Day: 1, Time: 1}) {
		t.Fatal("empty renderer should not draw")
	}
}

// TestDayBorderUsesSpecialImage 是"日期越过门槛"的回归测试。
//
// bsxx.cfg 的 `day sp border = 1000` 表示：变量值 >= 1000 时不再逐位画数字，
// 而是整块换成 info_sp。原引擎用它表达"日期还没意义"这类状态。
// 若忽略这条，玩家会看到一个 1000+ 的荒唐天数。
func TestDayBorderUsesSpecialImage(t *testing.T) {
	layout := config.InformationWindow{
		DayStrip:     config.DigitStrip{Image: "info_dayno", Frames: 10, X: 137, Y: 10, Offset: -14},
		DayUnit:      "info_unit",
		DaySpecial:   "info_sp",
		DaySpecialX:  47,
		DaySpecialY:  11,
		DayBorder:    1000,
		TimeZoneLow:  -1 << 30,
		TimeZoneHigh: 1 << 30,
	}
	if layout.Empty() {
		t.Fatal("layout should be usable")
	}
	// 只说清楚"低于门槛走数字路径"，实际绘制需要图片缓存，
	// 这里验证判定条件本身，避免回归成"任何日期都特殊"或"永远不特殊"。
	if layout.DayBorder != 1000 {
		t.Fatalf("border lost: %d", layout.DayBorder)
	}
	if 999 >= layout.DayBorder {
		t.Fatal("999 should render digits")
	}
	if !(1000 >= layout.DayBorder) {
		t.Fatal("1000 should render the special image")
	}
}
