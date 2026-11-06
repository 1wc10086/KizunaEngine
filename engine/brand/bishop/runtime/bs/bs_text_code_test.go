package bs

import "testing"

// TestLegacyMessage 验证旧式 BSScript 的爱心占位符。
//
// KD 的文本用 "$0" 表示爱心字形，直接显示会在对白里留下 "$0"；
// 现代 BSXScript 不走 LegacyMessage，因此不会把字面量误替换。
func TestLegacyMessage(t *testing.T) {
	cases := map[string]string{
		"えへ$0　魚肉ソーセージだよ！": "えへ♥　魚肉ソーセージだよ！",
		"――そして日曜日が過ぎて。":   "――そして日曜日が過ぎて。",
		`\b「おはよう$0」`:      `「おはよう♥」`,
	}
	for input, want := range cases {
		if got := LegacyMessage(input); got != want {
			t.Fatalf("LegacyMessage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMessageStripsControlCodes(t *testing.T) {
	if got := Message(`\b「こんにちは」`); got != "「こんにちは」" {
		t.Fatalf("Message = %q", got)
	}
}
