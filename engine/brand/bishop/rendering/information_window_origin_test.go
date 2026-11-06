package rendering

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
)

// 没有 info_base 底板、但配置了 base pos 的作品（特別授業3SLG 旧版）必须
// 以 base pos 为原点；否则信息窗会被画到 (0,0) 附近的左上角。
func TestInformationWindowOriginWithoutBaseImage(t *testing.T) {
	renderer := NewInformationWindowRenderer(nil, config.InformationWindow{
		BaseX:     100,
		BaseY:     410,
		DayStrip:  config.DigitStrip{Image: "info_dayno", Frames: 10, X: 48, Y: 27},
		DayUnit:   "info_unit",
		DayUnitX:  68,
		DayUnitY:  83,
		DayBorder: 1000,
	})
	if ox, oy := renderer.origin(); ox != 100 || oy != 410 {
		t.Fatalf("origin = %d,%d, want 100,410", ox, oy)
	}
}

// 既没有底板也没有 base pos 时保持绝对坐标。
func TestInformationWindowOriginDefault(t *testing.T) {
	renderer := NewInformationWindowRenderer(nil, config.InformationWindow{
		DayStrip: config.DigitStrip{Image: "info_dayno", Frames: 10},
	})
	if ox, oy := renderer.origin(); ox != 0 || oy != 0 {
		t.Fatalf("origin = %d,%d, want 0,0", ox, oy)
	}
}
