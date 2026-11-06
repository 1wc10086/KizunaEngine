package textrender

import (
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
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
	if picture.Rect.Empty() {
		t.Fatalf("Render returned an empty image: %v", picture.Rect)
	}

	for offset := 3; offset < len(picture.Pix); offset += 4 {
		if picture.Pix[offset] != 0 {
			return
		}
	}

	t.Fatal("rendered image has no non-transparent pixels")
}

func TestLayoutWrapsByWidthRuneCapAndMaxLines(t *testing.T) {
	renderer, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	face, err := opentype.NewFace(renderer.font, &opentype.FaceOptions{
		Size:    17,
		DPI:     renderDPI,
		Hinting: font.HintingFull,
	})
	if err != nil {
		t.Fatalf("NewFace: %v", err)
	}
	defer face.Close()

	advance, _ := face.GlyphAdvance('あ')
	wrapped := layout(face, strings.Repeat("あ", 5), 2*advance.Ceil(), 0)
	if len(wrapped) != 3 {
		t.Fatalf("width wrap produced %d lines, want 3", len(wrapped))
	}
	if got := len(wrapped[0].runes); got != 2 {
		t.Fatalf("first wrapped line has %d runes, want 2", got)
	}

	capped := layout(face, strings.Repeat("あ", 26), 0, 0)
	if len(capped) != 2 {
		t.Fatalf("rune cap produced %d lines, want 2", len(capped))
	}
	if len(capped[0].runes) != maxLineRunes || len(capped[1].runes) != 1 {
		t.Fatalf("rune cap split %d/%d runes, want 25/1", len(capped[0].runes), len(capped[1].runes))
	}

	limited := layout(face, "あ\nい\nう", 0, 2)
	if len(limited) != 2 || string(limited[0].runes) != "あ" || string(limited[1].runes) != "い" {
		t.Fatalf("maxLines did not stop after two explicit lines: %d lines", len(limited))
	}
}
