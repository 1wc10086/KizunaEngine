package emecfg

import (
	"testing"

	"kizuna/enginego/brand/emon/text"
)

// TestApplyTextSymbolsWindowSize 覆盖 BOKUONE/TOPPATU/NOZOMI 使用的
// 文章ウィンドウサイズ排版：原引擎 sub_43AB00 先按 字号*1.1 求行数，
// 再把行高反算成 高/行数，换行宽度是 宽-字号（sub_43A680）。
// 早期实现没读这个键，退回默认 34 字/4 行，BOKUONE 的第 4 行会压到
// Q.SAVE 工具条上，TOPPATU 则会在 725px 的框里排 34*24=816px 而溢出。
func TestApplyTextSymbolsWindowSize(t *testing.T) {
	style := text.NewEmonTextStyle()
	ApplyTextSymbols(style, map[string]string{
		"文章ウィンドウ位置": "0,551",
		"文章文字位置":    "30,69",
		"文章名前位置":    "20,27",
		"文章文字サイズ":   "27",
		"文章ウィンドウサイズ": "896,114",
	})

	if style.Size != 27 {
		t.Fatalf("size = %d, want 27", style.Size)
	}
	// 行高 27*1.1=29 → 114/29=3 行 → 114/3=38。
	if style.MaxLines != 3 || style.LineHeight != 38 {
		t.Fatalf("lines/lineHeight = %d/%d, want 3/38", style.MaxLines, style.LineHeight)
	}
	// 换行宽度 = 896-27 = 869 → 32 个全角字。
	if style.MaxCharactersPerLine != 32 {
		t.Fatalf("chars per line = %d, want 32", style.MaxCharactersPerLine)
	}
	if style.X != 30 || style.Y != 69 || style.SpeakerX != 20 || style.SpeakerY != 27 {
		t.Fatalf("text origin = (%d,%d) name=(%d,%d)", style.X, style.Y, style.SpeakerX, style.SpeakerY)
	}
}

// TestApplyTextSymbolsKeepsCGSizePath 保证 ASENURE 的旧键仍按原逻辑处理，
// 不让新键的公式影响已适配好的作品。
func TestApplyTextSymbolsKeepsCGSizePath(t *testing.T) {
	style := text.NewEmonTextStyle()
	ApplyTextSymbols(style, map[string]string{
		"文章文字位置":      "39,43",
		"文章名位置":       "39,10",
		"文章文字サイズ":     "24",
		"文章ウィンドウＣＧサイズ": "720,96",
	})

	if style.MaxLines != 4 || style.LineHeight != 24 {
		t.Fatalf("lines/lineHeight = %d/%d, want 4/24", style.MaxLines, style.LineHeight)
	}
	if style.MaxCharactersPerLine != (720-39)/24 {
		t.Fatalf("chars per line = %d, want %d", style.MaxCharactersPerLine, (720-39)/24)
	}
}
