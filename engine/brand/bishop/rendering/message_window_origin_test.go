package rendering

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/resource"
)

// newMessageBaseRenderer 造一个能取到 msg_base 的渲染器。
// 底图是 1024x213，和 侵蝕/淫獄 的 msg_base 同高。
func newMessageBaseRenderer(t *testing.T, layout config.MessageWindow) *MessageWindowRenderer {
	t.Helper()
	cache := NewImageCache(resource.Build(t.TempDir()), DefaultImageBudgetBytes, DefaultImageCapacity)
	image := gfx.NewBsgImage(1024, 213, 0, 0, make([]byte, 1024*213*4))
	cache.images["msg_base"] = image
	cache.nodes["msg_base"] = cache.recency.PushBack("msg_base")
	cache.aliases["msg_base"] = "msg_base"
	return NewMessageWindowRenderer(cache, layout, config.FontSettings{}, config.GameMenu{})
}

// 没有 base pos 的作品（SL）必须把对话框锚到底部：画布高减底图高。
// 退回 (0,0) 会把底图、名字和正文一起画到屏幕顶部，而工具条还在底部，
// 就是玩家看到的那张错位图。
func TestBaseOriginWithoutBasePos(t *testing.T) {
	renderer := newMessageBaseRenderer(t, config.MessageWindow{Base: "msg_base", NameX: 270, NameY: 6})

	// SL 的画布是 1024x576，底图高 213，所以原点在 y=363；名字与正文
	// 再偏移到 369/395，落在画面下半部而不是顶端。
	if ox, oy := renderer.baseOrigin(576); ox != 0 || oy != 363 {
		t.Fatalf("origin = %d,%d, want 0,363", ox, oy)
	}
	// 换 768 高的画布同样贴底。
	if ox, oy := renderer.baseOrigin(768); ox != 0 || oy != 555 {
		t.Fatalf("origin = %d,%d, want 0,555", ox, oy)
	}
}

// 底图取不到时退回 (0,0)：没有底图就无从知道窗口高度。
func TestBaseOriginWithoutBaseImage(t *testing.T) {
	renderer := NewMessageWindowRenderer(nil, config.MessageWindow{Base: "msg_base"}, config.FontSettings{}, config.GameMenu{})
	if ox, oy := renderer.baseOrigin(576); ox != 0 || oy != 0 {
		t.Fatalf("origin = %d,%d, want 0,0", ox, oy)
	}
}

// 配置给了 base pos 时必须照用，不受画布高度影响（侵蝕/淫獄/SLIE）。
func TestBaseOriginWithBasePos(t *testing.T) {
	renderer := newMessageBaseRenderer(t, config.MessageWindow{Base: "msg_base", BaseY: 555, BasePosSet: true})

	for _, height := range []int{576, 768} {
		if ox, oy := renderer.baseOrigin(height); ox != 0 || oy != 555 {
			t.Fatalf("height=%d origin = %d,%d, want 0,555", height, ox, oy)
		}
	}
}

// 浅色对话框用黑字，深色对话框用白字；否则 SL/SLIE 的白底白字看不见。
func TestMessageTextColorByBackground(t *testing.T) {
	light := NewMessageWindowRenderer(nil, config.MessageWindow{Base: "msg_base", BgColor: 0xc0ffffff}, config.FontSettings{}, config.GameMenu{})
	if got := light.textColor(); got != 0xff000000 {
		t.Fatalf("light text color = %#x, want black", got)
	}
	dark := NewMessageWindowRenderer(nil, config.MessageWindow{Base: "msg_base", BgColor: 0x80000000}, config.FontSettings{}, config.GameMenu{})
	if got := dark.textColor(); got != 0xffffffff {
		t.Fatalf("dark text color = %#x, want white", got)
	}
}

// 深色字配白描边、浅色字配黑描边；黑色对话框上的白字仍然是黑描边。
func TestOutlineFor(t *testing.T) {
	if got := outlineFor(0xff000000); got != 0xffffffff {
		t.Fatalf("black text outline = %#x, want white", got)
	}
	if got := outlineFor(0xffffffff); got != 0xff000000 {
		t.Fatalf("white text outline = %#x, want black", got)
	}
}
