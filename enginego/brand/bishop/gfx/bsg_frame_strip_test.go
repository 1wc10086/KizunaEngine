package gfx

import "testing"

// TestFrameStripPrefersRequestedFrames 是"立绘出现多只眼睛"这个 bug 的回归测试。
//
// 现场：表情图 117x396，bsxx.cfg 写 `eye pattern = 6`（每帧 66px）。
// 旧实现在"部件数 > 1"时抢先用了 4 帧兜底，于是 396px 被切成 4 段（每帧 99px），
// 一段里塞进一只半眼睛，画面上就是好几只眼睛和眉毛叠在一起。
// 正确的帧数必须来自 cfg（requestedFrames），而不是部件数量。
func TestFrameStripPrefersRequestedFrames(t *testing.T) {
	part := &BsgImage{Width: 117, Height: 396, FrameCount: 1}
	frames, frameHeight, ok := TryResolveBsgFrameStrip(part, 2, 6)
	if !ok {
		t.Fatal("expected an animated strip")
	}
	if frames != 6 {
		t.Fatalf("frames = %d, want 6 (from the eye pattern config)", frames)
	}
	if frameHeight != 66 {
		t.Fatalf("frameHeight = %d, want 66", frameHeight)
	}
}

// TestFrameStripUsesImageFrameCount 验证 BSG 头里显式写的帧数优先级最高。
func TestFrameStripUsesImageFrameCount(t *testing.T) {
	part := &BsgImage{Width: 100, Height: 300, FrameCount: 3}
	frames, frameHeight, ok := TryResolveBsgFrameStrip(part, 2, 6)
	if !ok {
		t.Fatal("expected an animated strip")
	}
	if frames != 3 {
		t.Fatalf("frames = %d, want 3 (from the BSG header)", frames)
	}
	if frameHeight != 100 {
		t.Fatalf("frameHeight = %d, want 100", frameHeight)
	}
}

// TestFrameStripPartCountIsLastResort 验证"部件数 > 1"只是最后的兜底：
// 前两种信息都没有时才用，且不会盖掉调用方的期望帧数。
func TestFrameStripPartCountIsLastResort(t *testing.T) {
	part := &BsgImage{Width: 100, Height: 400, FrameCount: 1}
	frames, frameHeight, ok := TryResolveBsgFrameStrip(part, 2, 0)
	if !ok {
		t.Fatal("expected a strip from the part-count fallback")
	}
	if frames != BsgFrameStripFallbackFrames {
		t.Fatalf("frames = %d, want %d", frames, BsgFrameStripFallbackFrames)
	}
	if frameHeight != 100 {
		t.Fatalf("frameHeight = %d, want 100", frameHeight)
	}
}

// TestFrameStripStaticPart 验证"没有帧信息、也没有期望帧数"时按静态图处理：
// 返回 ok=false，调用方会整张画一次，而不是硬切成几段。
func TestFrameStripStaticPart(t *testing.T) {
	part := &BsgImage{Width: 139, Height: 123, FrameCount: 1}
	frames, frameHeight, ok := TryResolveBsgFrameStrip(part, 1, 1)
	if ok {
		t.Fatalf("static part reported as a strip: frames=%d", frames)
	}
	if frames != 1 || frameHeight != 123 {
		t.Fatalf("frames=%d frameHeight=%d, want 1/123", frames, frameHeight)
	}
}

func TestSourceTopBsgFrameStrip(t *testing.T) {
	part := &BsgImage{Width: 10, Height: 60, FrameCount: 1}
	// 帧号取模：动画循环回绕时不能越界。
	if got := SourceTopBsgFrameStrip(part, 3, 20, 2); got != 40 {
		t.Fatalf("frame 2 top = %d, want 40", got)
	}
	if got := SourceTopBsgFrameStrip(part, 3, 20, 3); got != 0 {
		t.Fatalf("frame 3 should wrap to 0, got %d", got)
	}
	if got := SourceTopBsgFrameStrip(part, 3, 20, -1); got != 40 {
		t.Fatalf("frame -1 should wrap to 2, got %d", got)
	}
}
