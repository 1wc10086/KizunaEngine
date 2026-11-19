package config

import "testing"

func TestParseMessageWindowBasePos(t *testing.T) {
	// bsxx.cfg 的 `0,768-213` 必须解析成 0,555，并且标记为"配置过"。
	document := ParseIniDocument([]byte(`[message window]
base window		= msg_base
base pos		= 0, 768-213
name			= 270,  6, 512, 32
text			= 270, 32, 512, 96
`))

	window := parseMessageWindow(document)
	if !window.BasePosSet {
		t.Fatal("base pos should be marked as configured")
	}
	if window.BaseX != 0 || window.BaseY != 555 {
		t.Fatalf("base pos = %d,%d, want 0,555", window.BaseX, window.BaseY)
	}
}

func TestParseMessageWindowWithoutBasePos(t *testing.T) {
	// SL（BSXScript 2.0）的 [message window] 没有 base pos。此时 BaseX/BaseY
	// 仍是 0，但 BasePosSet 必须为 false，渲染器才能改回底部锚定；
	// 否则对话框会被画到屏幕左上角。
	document := ParseIniDocument([]byte(`[message window]
base window		= msg_base
name			= 270,  6, 512, 32
text			= 270, 32, 512, 96
`))

	window := parseMessageWindow(document)
	if window.BasePosSet {
		t.Fatal("base pos must not be reported as configured")
	}
	if window.BaseX != 0 || window.BaseY != 0 {
		t.Fatalf("base pos = %d,%d, want 0,0", window.BaseX, window.BaseY)
	}
	// 区域坐标是相对原点的，缺 base pos 时不能被顺手改掉。
	if window.NameX != 270 || window.NameY != 6 || window.TextY != 32 {
		t.Fatalf("areas = %d,%d / %d", window.NameX, window.NameY, window.TextY)
	}
}
