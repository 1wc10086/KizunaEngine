package textrender

import "testing"

// 日文、ASCII、半角假名三个字宽格子恰好对应合成端的半角 1 格 / 全角 2 格，
// 推进宽度必须按 cell 给。
func TestGlyphCellWidth(t *testing.T) {
	for _, character := range []rune{'あ', '漢', 'A', '0', '｡', 'ﾊ'} {
		got := glyphCellWidth(character, 24)
		want := 24
		if glyphIsHalfWidth(character) {
			want = 12
		}
		if got != want {
			t.Fatalf("glyphCellWidth(%q, 24) = %d, want %d", character, got, want)
		}
	}
}

// 空白类字符也必须是可推进的空白位图，不能是 nil，否则合成端会按缺字形处理。
func TestGlyphRasterBlank(t *testing.T) {
	clear := func(character rune, wantWidth int) {
		bitmap := GlyphRaster(character, 24)
		if bitmap == nil {
			t.Fatalf("GlyphRaster(%q) nil, want blank bitmap", character)
		}
		if bitmap.Width != wantWidth || bitmap.Height != 24 {
			t.Fatalf("GlyphRaster(%q) = %dx%d, want %dx24", character, bitmap.Width, bitmap.Height, wantWidth)
		}
		if len(bitmap.Alpha) != bitmap.Width*bitmap.Height {
			t.Fatalf("GlyphRaster(%q) alpha %d want %d", character, len(bitmap.Alpha), bitmap.Width*bitmap.Height)
		}
		for _, value := range bitmap.Alpha {
			if value != 0 {
				t.Fatalf("GlyphRaster(%q) should be blank", character)
			}
		}
	}
	// 半角空格半个 cell，全角空格一个 cell，控制字符按半角补位。
	clear(' ', 12)
	clear('\u3000', 24)
	clear('\t', 12)
}

// 字体已移除，任何字符（含日文与半角假名）都只给出没有墨的格子位图；
// 真正画字由宿主用 core.TextRun 完成。
func TestGlyphRasterHasNoInk(t *testing.T) {
	ClearGlyphCache()
	for _, character := range []rune{'あ', '漢', 'A', 'a', '｡', 'ﾊ'} {
		bitmap := GlyphRaster(character, 24)
		if bitmap == nil {
			t.Fatalf("GlyphRaster(%q) nil", character)
		}
		if bitmap.Height != 24 {
			t.Fatalf("GlyphRaster(%q) height %d want 24", character, bitmap.Height)
		}
		if len(bitmap.Alpha) != bitmap.Width*bitmap.Height {
			t.Fatalf("GlyphRaster(%q) alpha %d want %d", character, len(bitmap.Alpha), bitmap.Width*bitmap.Height)
		}
		for _, value := range bitmap.Alpha {
			if value != 0 {
				t.Fatalf("GlyphRaster(%q) should have no ink", character)
			}
		}
	}
}

// 半角宽度必须是 pixelSize/2 且全角必须是 pixelSize。
func TestGlyphRasterCellWidth(t *testing.T) {
	if got := GlyphRaster('A', 24); got == nil || got.Width != 12 {
		t.Fatalf("half-width raster = %+v, want width 12", got)
	}
	if got := GlyphRaster('あ', 24); got == nil || got.Width != 24 {
		t.Fatalf("full-width raster = %+v, want width 24", got)
	}
	// 最小 cell 不小于 1：pixelSize=1 时半角也要能推进。
	if got := GlyphRaster('A', 1); got == nil || got.Width != 1 {
		t.Fatalf("tiny half-width raster = %+v, want width 1", got)
	}
}

func TestGlyphRasterNilOnBadSize(t *testing.T) {
	if GlyphRaster('あ', 0) != nil || GlyphRaster('あ', -1) != nil {
		t.Fatal("GlyphRaster with pixelSize<=0 must be nil")
	}
}

// 清理只能丢掉缓存，重新取同字仍应有同样尺寸。
func TestClearGlyphCache(t *testing.T) {
	before := GlyphRaster('あ', 24)
	if before == nil {
		t.Fatal("before nil")
	}
	ClearGlyphCache()
	after := GlyphRaster('あ', 24)
	if after == nil || after.Width != before.Width || after.Height != before.Height || len(after.Alpha) != len(before.Alpha) {
		t.Fatalf("after clear bitmap %+v want %dx%d alpha %d", after, before.Width, before.Height, len(before.Alpha))
	}
}
