package textrender

import (
	"image"
	"math"
	"sync"

	"kizuna/enginego/core"
)

const (
	// maxLineRunes 是一行的硬上限。日文正文没有空格可作为断点，纯按宽度排遇到
	// 坏脚本（或一个超长串）时会排出一整屏宽的行，这里兜底把它切开。
	maxLineRunes = 25
	// 布局结果只由参数决定，但对话文本几乎不重复出现；缓存留 64 条足够覆盖
	// 自动播放时相邻的几句，同时不会把整部剧本的排版都攒在内存里。
	renderCacheCapacity = 64
	// baselineRatio 是行盒里基线相对顶边的比例（0.86em）。旧位图按内嵌字体的
	// ascent 定位基线，字体移除后保留同一比例，宿主按 run 画字时整行才不会
	// 相对旧截图上下漂移。
	baselineRatio = 0.86
)

// renderKey 覆盖 Render 的全部参数；任一参数不同都不能复用同一份排版。
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

// Renderer 按字号与排版参数缓存结果。
//
// 引擎不再持有字体：折行、基线与行距仍由 Go 决定（宿主的平台字体 ascent
// 与旧内嵌字体不同，让宿主自行排版会把原版坐标排歪），但真正画字的是
// Flutter；这里只产出 Rendered，交给 RunsAt 转成 core.TextRun。
type Renderer struct {
	gate  sync.Mutex
	cache map[renderKey]*Rendered
}

// Line 是一行的排版结果，坐标相对 Rendered 的左上角。
//
// X/Y 是行盒左上角（已含 edge 留白），Baseline 是文字基线；宿主只需要
// Baseline，Y 留给需要按行盒裁切的调用方。
type Line struct {
	Text     string
	X        int
	Y        int
	Baseline int
	Size     int
	Color    uint32
	Outline  uint32
	Shadow   int
	Bold     bool // 旧 Render 没有粗体参数，当前恒为 false。
}

// Rendered 是一段排好版但未栅格化的文字。
//
// Width/Height 与旧位图的尺寸规则完全一致：左边和上边留 edge 给描边，
// 右边和下边留 max(edge, shadow)。所以宿主把 RunsAt(x, y) 的结果画到
// (x, y) 时，文字落点与旧 blitRGBA 贴位图时重合，上层布局不用改坐标。
type Rendered struct {
	Width  int
	Height int
	Lines  []Line
}

// Bounds 返回与旧位图同尺寸的矩形，调用方用 Dx()/Dy() 量宽高。
func (r *Rendered) Bounds() image.Rectangle {
	if r == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, r.Width, r.Height)
}

// RunsAt 把每行转成交给宿主的文字条目，坐标整体平移 (x, y)。
//
// x/y 是 Rendered 左上角在引擎坐标系里的位置，与旧 blitRGBA(canvas, image, x, y)
// 的 x/y 语义相同。宿主按左对齐绘制，行的实际起点已由 X 给出，因此 Align
// 恒为 0、Width 恒为 0（不限制）。
func (r *Rendered) RunsAt(x, y int) []core.TextRun {
	if r == nil || len(r.Lines) == 0 {
		return nil
	}
	runs := make([]core.TextRun, 0, len(r.Lines))
	for _, line := range r.Lines {
		runs = append(runs, core.TextRun{
			Text:    line.Text,
			X:       int32(x + line.X),
			Y:       int32(y + line.Baseline),
			Size:    int32(line.Size),
			Color:   line.Color,
			Outline: line.Outline,
			Shadow:  int32(line.Shadow),
			Bold:    line.Bold,
		})
	}
	return runs
}

// New 返回一个排版器。
//
// 以前这里解析内嵌字体并建脸缓存；字体现在只存在于宿主侧，构造函数保留
// (renderer, error) 的签名，调用方不用改初始化代码。
func New() (*Renderer, error) {
	return &Renderer{cache: make(map[renderKey]*Rendered)}, nil
}

// Render 把一段日文排成布局结果。color 为 0xAARRGGBB；maxWidth 是文字本身
// 可用的宽度，边缘与阴影的留白由本函数另外加上，<= 0 表示不按宽度折行；
// maxLines <= 0 表示不限行，超出部分直接截掉。返回的结果由缓存持有，调用方
// 只应读取。
func (r *Renderer) Render(text string, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32) *Rendered {
	return r.RenderOutline(text, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines, color, 0xff000000)
}

// RenderOutline 和 Render 相同，但可以指定描边/阴影颜色。
//
// 原引擎的文字没有单独配描边色：深色底用白字 + 黑描边，浅色底用黑字 +
// 白描边。固定黑描边会让浅色对话框里的黑字糊成一团。outline 是
// 0xAARRGGBB；透明度跟随正文，正文淡出时描边不会留下硬边。
func (r *Renderer) RenderOutline(text string, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines int, color uint32, outline uint32) *Rendered {
	if r == nil || text == "" || pixelSize <= 0 {
		return nil
	}

	// 负值没有意义，统一归零后参与缓存键，避免同一组有效参数因写法不同而重复排版。
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

	if r.cache == nil {
		// 零值 Renderer 也能用，避免调用方漏掉 New 时写 nil map 崩溃。
		r.cache = make(map[renderKey]*Rendered)
	}
	if cached, ok := r.cache[key]; ok {
		return cached
	}

	rendered := layoutLines(text, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines, color, outline)

	if len(r.cache) >= renderCacheCapacity {
		// 满了随便丢一条：排版结果只由参数决定，丢掉之后还能原样重算，
		// 为此维护 LRU 链表的开销反而比偶尔重排一行更大。
		for stale := range r.cache {
			delete(r.cache, stale)
			break
		}
	}
	r.cache[key] = rendered

	return rendered
}

// layoutLines 做排版并填好每行的坐标。
func layoutLines(text string, pixelSize, edge, shadow, lineHeight, maxWidth, maxLines int, color, outline uint32) *Rendered {
	lines := layout(text, pixelSize, maxWidth, maxLines)

	contentWidth := 0
	for _, current := range lines {
		contentWidth = max(contentWidth, lineWidth(current, pixelSize))
	}
	// 行盒高度取 1em：旧位图高度是 ascent+descent，基线按 0.86em 定位，
	// 剩下的约 0.14em 是 descent，两者相加正好一个字号。
	contentHeight := pixelSize

	// 左边和上边要给描边留位置；右边和下边取描边与阴影中更远的那个。
	padding := edge
	extent := max(edge, shadow)
	picture := &Rendered{
		Width:  padding + contentWidth + extent,
		Height: padding + contentHeight + extent,
		Lines:  make([]Line, 0, len(lines)),
	}
	if len(lines) > 1 {
		picture.Height += (len(lines) - 1) * lineHeight
	}

	for index, current := range lines {
		top := padding + index*lineHeight
		picture.Lines = append(picture.Lines, Line{
			Text:     current,
			X:        padding,
			Y:        top,
			Baseline: top + baselineOffset(pixelSize),
			Size:     pixelSize,
			Color:    color,
			// 描边透明度跟随正文：旧位图里两者共用 ink.A，正文淡出时描边
			// 不能留下不透明的硬边。
			Outline: (outline & 0x00ffffff) | (color & 0xff000000),
			Shadow:  shadow,
		})
	}

	return picture
}

// baselineOffset 是行盒顶边到基线的距离，即旧字体 ascent 的近似值。
func baselineOffset(pixelSize int) int {
	return int(math.Round(float64(pixelSize) * baselineRatio))
}

// lineWidth 是一行文字的排版宽度，与折行用的是同一套格宽。
func lineWidth(line string, pixelSize int) int {
	width := 0
	for _, character := range line {
		width += glyphCellWidth(character, pixelSize)
	}
	return width
}

// layout 先按显式换行分段，再逐字按格宽折行；不依赖空格，因此日文长句也能断开。
func layout(text string, pixelSize, maxWidth, maxLines int) []string {
	lines := make([]string, 0, 4)
	current := make([]rune, 0, maxLineRunes)
	width := 0

	flush := func() bool {
		lines = append(lines, string(current))
		current = current[:0]
		width = 0
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

		advance := glyphCellWidth(character, pixelSize)
		tooWide := maxWidth > 0 && len(current) > 0 && width+advance > maxWidth
		if tooWide || len(current) >= maxLineRunes {
			if !flush() {
				return lines
			}
		}

		current = append(current, character)
		width += advance
	}

	// 文末没有换行时补上最后一行；显式换行结尾则不额外造一个空行。
	if len(current) > 0 || len(lines) == 0 {
		if maxLines > 0 && len(lines) >= maxLines {
			return lines
		}
		lines = append(lines, string(current))
	}

	return lines
}
