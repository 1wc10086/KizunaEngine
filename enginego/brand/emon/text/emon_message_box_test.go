package text

import "testing"

// TestMessageBoxFractionalReveal 回归“文字速度=遅い”卡在 0 字的问题：
// 60fps 下 cps=15 每帧只有 0.25 字，必须累计余量，不能直接截断。
func TestMessageBoxFractionalReveal(t *testing.T) {
	box := NewEmonMessageBox()
	box.SetBody("abcdef")

	for i := 0; i < 4; i++ {
		box.Advance(1.0/60.0, 15)
	}
	if got := box.RevealCount(); got != 1 {
		t.Fatalf("reveal count after 4 slow frames = %d, want 1", got)
	}

	box.Advance(1.0, 15)
	if got := box.RevealCount(); got != 6 {
		t.Fatalf("reveal count = %d, want all 6 chars", got)
	}

	box.Clear()
	box.SetBody("abc")
	box.Advance(1.0/60.0, 15)
	if got := box.RevealCount(); got != 0 {
		t.Fatalf("reveal count after clear = %d, want 0", got)
	}
}
