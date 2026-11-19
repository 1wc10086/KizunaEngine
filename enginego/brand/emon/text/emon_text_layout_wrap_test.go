package text

import "testing"

// TestWrapTextFillsWidth 回归“按标点提前回退”的排版偏差。
// 原引擎 sub_43A680 用 GetTextExtentExPointA 取像素宽度内的最长前缀，
// 不做禁则/单词回退；每行必须尽量填满，否则长句右侧会留白、
// 行数比原版多。这里用全角假名验证每行都是满宽切分。
func TestWrapTextFillsWidth(t *testing.T) {
	lines := WrapText("あいうえおかきくけこ", 3)
	want := []string{"あいう", "えおか", "きくけ", "こ"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

// TestWrapTextDoesNotBacktrackToPunctuation 验证标点不再让整行提前断开：
// 每行 3 字时 “あいう、えおか” 应切成 “あいう” / “、えお” / “か”，
// 而不是在 、 处回退成 “あいう” 后把余下内容挤进更短的行。
func TestWrapTextDoesNotBacktrackToPunctuation(t *testing.T) {
	lines := WrapText("あいう、えおか", 3)
	want := []string{"あいう", "、えお", "か"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

// TestWrapTextKeepsExplicitBreaks 保持脚本里的换行：显式 \n 必须原样保留，
// 不能因为重新排版把两段合成一行或丢掉空行。
func TestWrapTextKeepsExplicitBreaks(t *testing.T) {
	lines := WrapText("一行目\n二行目", 10)
	if len(lines) != 2 || lines[0] != "一行目" || lines[1] != "二行目" {
		t.Fatalf("lines = %#v", lines)
	}
}
