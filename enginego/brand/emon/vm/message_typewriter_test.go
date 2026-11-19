package vm

import (
	"testing"

	emontext "kizuna/enginego/brand/emon/text"
)

// TestPageKeepsTypewriter 回归“#Page 直接把整段正文全显”的问题。
//
// 原引擎的 #Page 只进入“本页显示完并等待点击”的状态，逐字由 MessageSpeed
// 推进；第一次点击把本页剩余的字符一次显示完，第二次点击才翻页。以前
// onPage 里调用 RevealAll，导致文字速度设置无效、长文本也读不到前半段。
func TestPageKeepsTypewriter(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "page-typewriter", "本文テストです\n#Page()\n#End()\n")
	if step != EmonStepAwaitInput {
		t.Fatalf("step = %d, want await input", int(step))
	}
	box, ok := m.Context().Message.(*emontext.EmonMessageBox)
	if !ok {
		t.Fatal("test context should use EmonMessageBox")
	}
	if box.FullyRevealed() || box.RevealCount() != 0 {
		t.Fatalf("Page revealed the body instantly: %d/%d", box.RevealCount(), len([]rune(box.Body())))
	}

	// 第一次点击：跳过打字机，不翻页。
	if got := m.Advance(); got != EmonStepAwaitInput {
		t.Fatalf("first advance = %d, want still await input", int(got))
	}
	if !box.FullyRevealed() {
		t.Fatalf("first advance did not reveal the body: %d/%d", box.RevealCount(), len([]rune(box.Body())))
	}

	// 第二次点击：清空并继续，脚本走到 #End。
	if got := m.Advance(); got != EmonStepFinished {
		t.Fatalf("second advance = %d, want finished", int(got))
	}
}
