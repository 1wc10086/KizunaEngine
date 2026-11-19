package stage

import (
	"testing"

	"kizuna/enginego/brand/tmrhiro/grd"
)

// newTestStage 造一个不带图片缓存的舞台（只要不取图就不会触发解码）。
func newTestStage() *Stage {
	return NewStage(grd.NewCache(func(name string) ([]byte, error) { return nil, nil }))
}

func TestSlotXByCount(t *testing.T) {
	// 这一段是“立绘站位”的回归：单人居中、双人左右、三人中左右。
	// 站位错了最直观的表现是两人同屏时都挤在中间。
	st := newTestStage()
	st.SetChar(0, "A", 0, 0, 1)
	st.SetChar(1, "B", -300, 0, 1)
	// SetChar 只存脚本给的偏移，不改写：站位公式在 vm.slotX。
	if st.Chars()[0].X != 0 || st.Chars()[1].X != -300 {
		t.Fatalf("站位 %d/%d", st.Chars()[0].X, st.Chars()[1].X)
	}
}

func TestClearChars(t *testing.T) {
	st := newTestStage()
	for i := 0; i < CharSlots; i++ {
		st.SetChar(i, "x", 0, 0, 1)
	}
	st.ClearChars()
	for i := 0; i < CharSlots; i++ {
		if st.Chars()[i].Name != "" {
			t.Fatalf("槽 %d 未清空", i)
		}
	}
}

func TestFadeClamps(t *testing.T) {
	st := newTestStage()
	st.Fade("FFFFFF", 999)
	_, alpha := st.FadeState()
	if alpha != 255 {
		t.Fatalf("alpha = %d", alpha)
	}
	st.Fade("FFFFFF", -5)
	_, alpha = st.FadeState()
	if alpha != 0 {
		t.Fatalf("alpha = %d", alpha)
	}
}

func TestParseColor(t *testing.T) {
	// 脚本里的颜色是 RGB 顺序，返回的是 BGRA 分量：FF0000 → B=0,G=0,R=255。
	b, g, r := parseColor("#FF0000")
	if b != 0 || g != 0 || r != 0xFF {
		t.Fatalf("红 = B%d G%d R%d", b, g, r)
	}
	b, g, r = parseColor("00FF00")
	if b != 0 || g != 0xFF || r != 0 {
		t.Fatalf("绿 = B%d G%d R%d", b, g, r)
	}
	b, g, r = parseColor("0000FF")
	if b != 0xFF || g != 0 || r != 0 {
		t.Fatalf("蓝 = B%d G%d R%d", b, g, r)
	}
}

func TestComposeRequiresBufferSize(t *testing.T) {
	st := newTestStage()
	if ok := st.Compose(make([]byte, 1024)); ok {
		t.Fatal("过小的缓冲不该合成成功")
	}
	buf := make([]byte, FrameWidth*FrameHeight*4)
	if !st.Compose(buf) {
		t.Fatal("标准尺寸缓冲应能合成")
	}
}
