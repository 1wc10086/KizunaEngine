package gothic

import (
	"os"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"kizuna/engine/brand/bishop/gfx"
)

const (
	GothicGlyphCount = 7238
	GothicGlyphSize  = 26
)

const (
	gothicMazeRecordSize = 676
	gothicMazePlaneSize  = 338
	gothicTokuRecordSize = 484
	gothicTokuPlaneSize  = 242
)

type GothicFontFile struct {
	data        []byte
	recordSize  int
	planeSize   int
	cache       map[int]*GothicGlyph
	disposed    bool
	GlyphWidth  int
	GlyphHeight int
	Advance     int
	LineHeight  int
}

func NewGothicFontFile(path string) *GothicFontFile {
	data, err := os.ReadFile(path)
	if err != nil {
		data = nil
	}
	font := &GothicFontFile{data: data, cache: map[int]*GothicGlyph{}}
	if data != nil && len(data) == GothicGlyphCount*gothicMazeRecordSize {
		font.recordSize = gothicMazeRecordSize
		font.planeSize = gothicMazePlaneSize
		// MAZE 的剧本每行最多 24 个全角字，原引擎按 17px 步进
		// （ASCII 8px）排版、25px 行高；字体平面的 26x26 只是
		// 字形画布，字间留白由步进为 17 时的重叠吃掉。
		// 早期实现误用 23/29，正文行距过宽、24 字行会挤出窗口。
		font.GlyphWidth = 26
		font.GlyphHeight = 26
		font.Advance = 17
		font.LineHeight = 25
	} else if data != nil && len(data) == GothicGlyphCount*gothicTokuRecordSize {
		font.recordSize = gothicTokuRecordSize
		font.planeSize = gothicTokuPlaneSize
		font.GlyphWidth = 22
		font.GlyphHeight = 22
		font.Advance = 19
		font.LineHeight = 22
	}
	return font
}

func (f *GothicFontFile) Get(glyphIndex int) *GothicGlyph {
	if f.disposed || f.data == nil || f.recordSize == 0 {
		return nil
	}
	if glyphIndex < 0 || glyphIndex >= GothicGlyphCount {
		return nil
	}
	offset := int64(glyphIndex)*int64(f.recordSize) + int64(f.planeSize)
	if offset+int64(f.planeSize) > int64(len(f.data)) {
		return nil
	}
	if cached, ok := f.cache[glyphIndex]; ok {
		return cached
	}
	plane := make([]byte, f.planeSize)
	copy(plane, f.data[offset:offset+int64(f.planeSize)])
	glyph := newGothicGlyph(plane, f.GlyphWidth)
	f.cache[glyphIndex] = glyph
	return glyph
}

func (f *GothicFontFile) GetForChar(value rune) *GothicGlyph {
	return f.Get(GothicGlyphIndexOf(value))
}

func (f *GothicFontFile) Dispose() {
	if f.disposed {
		return
	}
	f.disposed = true
	f.cache = map[int]*GothicGlyph{}
}

type GothicGlyph struct {
	plane    []byte
	rowBytes int
	Width    int
	Height   int
}

func newGothicGlyph(plane []byte, glyphWidth int) *GothicGlyph {
	rowBytes := glyphWidth / 2
	return &GothicGlyph{plane: plane, rowBytes: rowBytes, Width: glyphWidth, Height: len(plane) / rowBytes}
}

func (g *GothicGlyph) Nibble(x, y int) byte {
	if x < 0 || x >= g.Width || y < 0 || y >= g.Height {
		return 0
	}
	value := g.plane[(y*g.rowBytes)+(x>>1)]
	if (x & 1) == 0 {
		return value & 0xF
	}
	return (value >> 4) & 0xF
}

func GothicGlyphIndexOf(value rune) int {
	var encoded []byte
	if value < 0x80 {
		encoded = []byte{byte(value)}
	} else {
		var err error
		encoded, _, err = transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(string(value)))
		if err != nil {
			return -1
		}
	}
	if len(encoded) != 2 {
		return -1
	}
	lead := encoded[0]
	trail := encoded[1]
	var row int
	if lead >= 0x81 && lead <= 0x9F {
		row = int(lead) - 0x81
	} else if lead >= 0xE0 && lead <= 0xFC {
		row = int(lead) - 0xC1
	} else {
		return -1
	}
	var ku int
	var ten int
	if trail >= 0x40 && trail <= 0x7E {
		ku = (row * 2) + 0x21
		ten = int(trail) - 0x1F
	} else if trail >= 0x80 && trail <= 0x9E {
		ku = (row * 2) + 0x21
		ten = int(trail) - 0x20
	} else if trail >= 0x9F && trail <= 0xFC {
		ku = (row * 2) + 0x22
		ten = int(trail) - 0x7E
	} else {
		return -1
	}
	ku -= 0x20
	ten -= 0x20
	if ten < 1 || ten > 94 {
		return -1
	}
	if ku >= 1 && ku <= 8 {
		return ((ku - 1) * 94) + (ten - 1)
	}
	if ku >= 16 && ku <= 84 {
		return 752 + (((ku - 16) * 94) + (ten - 1))
	}
	return -1
}

const (
	GothicGlyphAdvance    = 23
	GothicGlyphLineHeight = 29
)

func DrawGothicGlyph(canvas *gfx.BgraCanvas, glyph *GothicGlyph, x, y, table int) {
	pixels := canvas.Buffer
	width := canvas.Width
	height := canvas.Height
	for row := 0; row < glyph.Height; row++ {
		targetY := y + row
		if targetY < 0 || targetY >= height {
			continue
		}
		for column := 0; column < glyph.Width; column++ {
			targetX := x + column
			if targetX < 0 || targetX >= width {
				continue
			}
			value := glyph.Nibble(column, row)
			if value == 0 {
				continue
			}
			var alpha int
			if table == 1 {
				alpha = (16 * (15 - int(value))) + 15
			} else {
				alpha = (16 * int(value)) - 1
			}
			inverse := 255 - alpha
			offset := ((targetY * width) + targetX) * 4
			pixels[offset] = byte(((255 * alpha) + (int(pixels[offset]) * inverse)) / 255)
			pixels[offset+1] = byte(((255 * alpha) + (int(pixels[offset+1]) * inverse)) / 255)
			pixels[offset+2] = byte(((255 * alpha) + (int(pixels[offset+2]) * inverse)) / 255)
			pixels[offset+3] = byte(((255 * alpha) + (int(pixels[offset+3]) * inverse)) / 255)
		}
	}
}

func DrawGothicText(canvas *gfx.BgraCanvas, font *GothicFontFile, text string, x, y, table int) {
	DrawGothicTextColor(canvas, font, text, x, y, table, 255, 255, 255)
}

// DrawGothicTextColor 与 DrawGothicText 相同，但可指定文字颜色。
// MAZE 的正文直接压在 CG 上，没有窗口底图；先画 1px 黑字投影、
// 再画白字，亮背景上的对白才不会糊成一片。
func DrawGothicTextColor(canvas *gfx.BgraCanvas, font *GothicFontFile, text string, x, y, table int, red, green, blue byte) {
	cursor := x
	advance := font.Advance
	for _, value := range text {
		glyph := font.GetForChar(value)
		if glyph == nil {
			cursor += advance
			continue
		}
		DrawGothicGlyphColor(canvas, glyph, cursor, y, table, red, green, blue)
		cursor += advance
	}
}

func DrawGothicGlyphColor(canvas *gfx.BgraCanvas, glyph *GothicGlyph, x, y, table int, red, green, blue byte) {
	pixels := canvas.Buffer
	width := canvas.Width
	height := canvas.Height
	for row := 0; row < glyph.Height; row++ {
		targetY := y + row
		if targetY < 0 || targetY >= height {
			continue
		}
		for column := 0; column < glyph.Width; column++ {
			targetX := x + column
			if targetX < 0 || targetX >= width {
				continue
			}
			value := glyph.Nibble(column, row)
			if value == 0 {
				continue
			}
			var alpha int
			if table == 1 {
				alpha = (16 * (15 - int(value))) + 15
			} else {
				alpha = (16 * int(value)) - 1
			}
			if alpha <= 0 {
				continue
			}
			inverse := 255 - alpha
			offset := ((targetY * width) + targetX) * 4
			pixels[offset] = byte(((int(blue) * alpha) + (int(pixels[offset]) * inverse)) / 255)
			pixels[offset+1] = byte(((int(green) * alpha) + (int(pixels[offset+1]) * inverse)) / 255)
			pixels[offset+2] = byte(((int(red) * alpha) + (int(pixels[offset+2]) * inverse)) / 255)
			pixels[offset+3] = 255
		}
	}
}
