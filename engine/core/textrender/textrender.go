package textrender

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	// maxLineRunes 是一行的硬上限。日文正文没有空格可作为断点，纯按宽度排遇到
	// 坏脚本（或一个超长串）时会排出一整屏宽的行，这里兜底把它切开。
	maxLineRunes = 25
	// 渲染结果只由参数决定，但对话文本几乎不重复出现；缓存留 64 条足够覆盖
	// 自动播放时相邻的几句，同时不会把整部剧本的位图都攒在内存里。
	renderCacheCapacity = 64
	// 字号按 72 DPI 解释，这样 Size 的点数就等于像素数，调用方不用再做换算。
	renderDPI = 72.0
)

//go:embed NotoSansSC-Subset.ttf
var notoSansSCSubset []byte

// renderKey 覆盖 Render 的全部参数；任一参数不同都不能复用同一张位图。
type renderKey struct {
	text       string
	pixelSize  int
	edge       int
	shadow     int
	lineHeight int
	maxWidth   int
	maxLines   int
	color      uint32
	outline    uint32
}

type line struct {
	runes []rune
	width fixed.Int26_6
}

// Renderer 持有一份解析好的字体，并按字号与渲染参数缓存结果。
type Renderer struct {
	font *opentype.Font

	// face 的实现不保证并发安全，排版与绘制都在这把锁内完成，顺带保护两个缓存。
	gate  sync.Mutex
	faces map[int]font.Face
	cache map[renderKey]*image.RGBA
}

// New 解析内嵌字体。字体随二进制发布，安卓端不依赖系统是否安装了中文字体。
func New() (*Renderer, error) {
	parsed, err := opentype.Parse(notoSansSCSubset)
	if err != nil {
		return nil, fmt.Errorf("textrender: parse embedded font: %w", err)
	}

	return &Renderer{
		font:  parsed,
		faces: make(map[int]font.Face),
		cache: make(map[renderKey]*image.RGBA),
	}, nil
}

// Render 把一段日文排成 ARGB 位图。color 为 0xAARRGGBB；maxWidth 是文字本身
// 可用的宽度，边缘与阴影的留白由本函数另外加上，<= 0 表示不按宽度折行；
// maxLines <= 0 表示不限行，超出部分直接截掉。返回的位图由缓存持有，调用方
// 只应读取，不要修改像素。
func (r *Renderer) Render(text string, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32) *image.RGBA {
	return r.RenderOutline(text, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines, color, 0xff000000)
}

// RenderOutline 和 Render 相同，但可以指定描边/阴影颜色。
//
// 原引擎的文字没有单独配描边色：深色底用白字 + 黑描边，浅色底用黑字 +
// 白描边。固定黑描边会让浅色对话框里的黑字糊成一团。outline 是
// 0xAARRGGBB；透明度跟随正文，正文淡出时描边不会留下硬边。
func (r *Renderer) RenderOutline(text string, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32, outline uint32) *image.RGBA {
	if text == "" || pixelSize <= 0 {
		return nil
	}

	// 负值没有意义，统一归零后参与缓存键，避免同一组有效参数因写法不同而重复渲染。
	edge = max(edge, 0)
	shadow = max(shadow, 0)
	maxWidth = max(maxWidth, 0)
	maxLines = max(maxLines, 0)
	if lineHeight <= 0 {
		// 调用方没给行高时按 1em 排，至少让相邻两行的基线分开。
		lineHeight = pixelSize
	}

	key := renderKey{
		text:       text,
		pixelSize:  pixelSize,
		edge:       edge,
		shadow:     shadow,
		lineHeight: lineHeight,
		maxWidth:   maxWidth,
		maxLines:   maxLines,
		color:      color,
		outline:    outline,
	}

	r.gate.Lock()
	defer r.gate.Unlock()

	if cached, ok := r.cache[key]; ok {
		return cached
	}

	face, ok := r.faces[pixelSize]
	if !ok {
		created, err := opentype.NewFace(r.font, &opentype.FaceOptions{
			Size:    float64(pixelSize),
			DPI:     renderDPI,
			Hinting: font.HintingFull,
		})
		if err != nil {
			return nil
		}
		face = created
		r.faces[pixelSize] = face
	}

	rendered := draw(face, text, edge, shadow, lineHeight, maxWidth, maxLines, color, outline)

	if len(r.cache) >= renderCacheCapacity {
		// 满了随便丢一条：位图只由参数决定，丢掉之后还能原样重算，
		// 为此维护 LRU 链表的开销反而比偶尔重排一行更大。
		for stale := range r.cache {
			delete(r.cache, stale)
			break
		}
	}
	r.cache[key] = rendered

	return rendered
}

func draw(face font.Face, text string, edge, shadow, lineHeight, maxWidth, maxLines int, argb uint32, outline uint32) *image.RGBA {
	lines := layout(face, text, maxWidth, maxLines)
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()

	contentWidth := 0
	for _, current := range lines {
		contentWidth = max(contentWidth, current.width.Ceil())
	}
	contentHeight := ascent + descent
	if len(lines) > 1 {
		contentHeight += (len(lines) - 1) * lineHeight
	}

	// 左边和上边要给描边留位置；右边和下边取描边与阴影中更远的那个。
	padding := edge
	extent := max(edge, shadow)
	picture := image.NewRGBA(image.Rect(0, 0, padding+contentWidth+extent, padding+contentHeight+extent))
	if picture.Rect.Empty() {
		return picture
	}

	ink := color.NRGBA{
		R: uint8(argb >> 16),
		G: uint8(argb >> 8),
		B: uint8(argb),
		A: uint8(argb >> 24),
	}
	// 描边与阴影的颜色由调用方给（浅色底用白描边），透明度跟随正文：
	// 正文淡出时它们不会留下不透明的硬边。
	dark := color.NRGBA{
		R: uint8(outline >> 16),
		G: uint8(outline >> 8),
		B: uint8(outline),
		A: ink.A,
	}
	baseline := padding + ascent

	if shadow > 0 {
		drawLines(picture, face, lines, dark, padding+shadow, baseline+shadow, lineHeight)
	}
	if edge > 0 {
		for _, dy := range []int{-edge, 0, edge} {
			for _, dx := range []int{-edge, 0, edge} {
				if dx == 0 && dy == 0 {
					continue
				}
				drawLines(picture, face, lines, dark, padding+dx, baseline+dy, lineHeight)
			}
		}
	}
	drawLines(picture, face, lines, ink, padding, baseline, lineHeight)

	return picture
}

func drawLines(dst *image.RGBA, face font.Face, lines []line, ink color.Color, x, baseline, lineHeight int) {
	source := image.NewUniform(ink)
	for index, current := range lines {
		drawer := &font.Drawer{
			Dst:  dst,
			Src:  source,
			Face: face,
			Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(baseline + index*lineHeight)},
		}
		drawer.DrawString(string(current.runes))
	}
}

// layout 先按显式换行分段，再逐字测量宽度折行；不依赖空格，因此日文长句也能断开。
func layout(face font.Face, text string, maxWidth, maxLines int) []line {
	lines := make([]line, 0, 4)
	current := line{}
	widthLimit := fixed.I(maxWidth)

	flush := func() bool {
		lines = append(lines, current)
		current = line{}
		return maxLines <= 0 || len(lines) < maxLines
	}

	for _, character := range text {
		// Windows 换行留下的 \r 不参与排版，否则会在行尾画出一个缺字方框。
		if character == '\r' {
			continue
		}
		if character == '\n' {
			if !flush() {
				return lines
			}
			continue
		}

		advance, _ := face.GlyphAdvance(character)
		tooWide := maxWidth > 0 && len(current.runes) > 0 && current.width+advance > widthLimit
		if tooWide || len(current.runes) >= maxLineRunes {
			if !flush() {
				return lines
			}
		}

		current.runes = append(current.runes, character)
		current.width += advance
	}

	// 文末没有换行时补上最后一行；显式换行结尾则不额外造一个空行。
	if len(current.runes) > 0 || len(lines) == 0 {
		if maxLines > 0 && len(lines) >= maxLines {
			return lines
		}
		lines = append(lines, current)
	}

	return lines
}
