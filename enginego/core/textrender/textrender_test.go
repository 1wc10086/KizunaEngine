package textrender

import (
	"strings"
	"testing"
)

func TestRenderJapaneseSentence(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const sentence = "もしも、透明人間になれる能力が自分にあったら。"
	picture := renderer.Render(sentence, 17, 1, 1, 22, 400, 3, 0xFFFFFFFF)
	if picture == nil {
		t.Fatal("Render returned nil")
	}
	if picture.Bounds().Empty() {
		t.Fatalf("Render returned empty bounds: %v", picture.Bounds())
	}
	if len(picture.Lines) == 0 {
		t.Fatal("Render returned no lines")
	}

	for _, line := range picture.Lines {
		if line.Text == "" {
			t.Fatalf("empty line in %+v", picture.Lines)
		}
		if line.Size != 17 {
			t.Fatalf("line size %d, want 17", line.Size)
		}
		if line.Color != 0xFFFFFFFF {
			t.Fatalf("line color %#x, want 0xffffffff", line.Color)
		}
	}
}

func TestLayoutWrapsByWidthRuneCapAndMaxLines(t *testing.T) {
	// 全角字一格 = pixelSize，宽度上限 2 格正好放两个字。
	wrapped := layout(strings.Repeat("あ", 5), 17, 34, 0)
	if len(wrapped) != 3 {
		t.Fatalf("width wrap produced %d lines, want 3", len(wrapped))
	}
	if got := len([]rune(wrapped[0])); got != 2 {
		t.Fatalf("first wrapped line has %d runes, want 2", got)
	}

	capped := layout(strings.Repeat("あ", 26), 17, 0, 0)
	if len(capped) != 2 {
		t.Fatalf("rune cap produced %d lines, want 2", len(capped))
	}
	if len([]rune(capped[0])) != maxLineRunes || len([]rune(capped[1])) != 1 {
		t.Fatalf("rune cap split %d/%d runes, want 25/1", len([]rune(capped[0])), len([]rune(capped[1])))
	}

	limited := layout("あ\nい\nう", 17, 0, 2)
	if len(limited) != 2 || limited[0] != "あ" || limited[1] != "い" {
		t.Fatalf("maxLines did not stop after two explicit lines: %d lines", len(limited))
	}
}

func TestLayoutUsesCellWidth(t *testing.T) {
	// 半角按 pixelSize/2 推进：24px 下 'A' 占 12px，两个字正好 24px。
	if lines := layout("ABCD", 24, 24, 0); len(lines) != 2 || lines[0] != "AB" || lines[1] != "CD" {
		t.Fatalf("half-width wrap = %q, want [AB CD]", lines)
	}
	// 全角在 24px 下占 24px，一行只放得下一个。
	if lines := layout("ああ", 24, 24, 0); len(lines) != 2 || lines[0] != "あ" || lines[1] != "あ" {
		t.Fatalf("full-width wrap = %q, want [あ あ]", lines)
	}
}

func TestRenderOutlineIncludesEdgeAndShadowPadding(t *testing.T) {
	renderer, _ := New()
	// 一行 'A'（12px）+ 左 edge 1 + 右 max(edge,shadow)=3；高度 24 + 1 + 3。
	picture := renderer.RenderOutline("A", 24, 1, 3, 24, 0, 0, 0xFFFFFFFF, 0xFF000000)
	if picture == nil {
		t.Fatal("RenderOutline returned nil")
	}
	if picture.Width != 1+12+3 || picture.Height != 1+24+3 {
		t.Fatalf("bounds %dx%d, want %dx%d", picture.Width, picture.Height, 16, 28)
	}
	line := picture.Lines[0]
	if line.X != 1 || line.Y != 1 {
		t.Fatalf("line origin (%d,%d), want (1,1)", line.X, line.Y)
	}
	if want := 1 + baselineOffset(24); line.Baseline != want {
		t.Fatalf("baseline %d, want %d", line.Baseline, want)
	}
	if line.Outline != 0xFF000000 || line.Shadow != 3 {
		t.Fatalf("line outline %#x shadow %d", line.Outline, line.Shadow)
	}

	// 描边透明度跟随正文：正文半透明时描边不能是不透明的硬边。
	faded := renderer.RenderOutline("A", 24, 1, 3, 24, 0, 0, 0x80FFFFFF, 0xFF112233)
	if got := faded.Lines[0].Outline; got != 0x80112233 {
		t.Fatalf("faded outline %#x, want 0x80112233", got)
	}
}

func TestRenderLineHeightAndCaching(t *testing.T) {
	renderer, _ := New()
	// lineHeight <= 0 按字号：两行高度 = size + size，没有额外留白。
	flush := renderer.Render("あ\nい", 20, 0, 0, 0, 0, 0, 0xFFFFFFFF)
	if len(flush.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(flush.Lines))
	}
	if flush.Height != 2*20 {
		t.Fatalf("height %d, want 40", flush.Height)
	}
	if flush.Lines[1].Baseline-flush.Lines[0].Baseline != 20 {
		t.Fatalf("baseline gap %d, want 20", flush.Lines[1].Baseline-flush.Lines[0].Baseline)
	}

	first := renderer.Render("あ\nい", 20, 0, 0, 0, 0, 0, 0xFFFFFFFF)
	if first != flush {
		t.Fatal("identical parameters must reuse the cached layout")
	}
}

func TestRenderEmptyAndBadSize(t *testing.T) {
	renderer, _ := New()
	if renderer.Render("", 20, 0, 0, 0, 0, 0, 0xFFFFFFFF) != nil {
		t.Fatal("empty text must return nil")
	}
	if renderer.Render("あ", 0, 0, 0, 0, 0, 0, 0xFFFFFFFF) != nil {
		t.Fatal("pixelSize <= 0 must return nil")
	}
	var nilRenderer *Renderer
	if nilRenderer.Render("あ", 20, 0, 0, 0, 0, 0, 0xFFFFFFFF) != nil {
		t.Fatal("nil renderer must return nil")
	}
}

func TestRunsAt(t *testing.T) {
	renderer, _ := New()
	picture := renderer.RenderOutline("あい", 24, 1, 1, 28, 0, 0, 0xFFFFFFFF, 0xFF000000)
	if picture == nil {
		t.Fatal("RenderOutline returned nil")
	}

	runs := picture.RunsAt(100, 200)
	if len(runs) != len(picture.Lines) {
		t.Fatalf("got %d runs, want %d", len(runs), len(picture.Lines))
	}
	for index, run := range runs {
		line := picture.Lines[index]
		if run.Text != line.Text {
			t.Fatalf("run %d text %q, want %q", index, run.Text, line.Text)
		}
		if run.X != int32(100+line.X) || run.Y != int32(200+line.Baseline) {
			t.Fatalf("run %d at (%d,%d), want (%d,%d)", index, run.X, run.Y, 100+line.X, 200+line.Baseline)
		}
		if run.Size != 24 || run.Color != 0xFFFFFFFF || run.Outline != 0xFF000000 {
			t.Fatalf("run %d style %+v", index, run)
		}
		if run.Align != 0 || run.Width != 0 {
			t.Fatalf("run %d should be left aligned and unclipped: %+v", index, run)
		}
	}

	var none *Rendered
	if none.RunsAt(0, 0) != nil {
		t.Fatal("nil Rendered must produce no runs")
	}
}

func TestNewRendersWithoutFont(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if renderer == nil {
		t.Fatal("New returned nil renderer")
	}
	if picture := renderer.Render("あ", 24, 0, 0, 0, 0, 1, 0xFFFFFFFF); picture == nil {
		t.Fatal("Render must not depend on a font")
	}
}
