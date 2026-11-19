package text

import (
	"container/list"
	"math"
	"strings"
	"sync"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/core"
)

const (
	layoutCacheCapacity       = 32
	spriteLayoutCacheCapacity = 64
	textReportCapacity        = 64
)

var (
	cacheGate = &sync.Mutex{}

	layoutCache = map[layoutKey][]string{}
	layoutNodes = map[layoutKey]*list.Element{}
	layoutOrder = list.New()

	spriteWrapCache = map[spriteWrapKey][]string{}
	spriteWrapNodes = map[spriteWrapKey]*list.Element{}
	spriteWrapOrder = list.New()

	textReports = map[textReportKey]struct{}{}
)

type layoutKey struct {
	body                 string
	shown                int
	maxCharactersPerLine int
}

type spriteWrapKey struct {
	text      string
	wrapWidth int
	size      int
	family    string
	weight    int
	quality   int
}

type textReportKey struct {
	name       string
	length     int
	family     string
	boxWidth   int
	boxHeight  int
	lineHeight int
	maxLines   int
	noInk      bool
}

type textClip struct {
	left   int
	top    int
	right  int
	bottom int
	active bool
}

func DisplayWidth(c rune) int {
	if c < 0x80 || (c >= '｡' && c <= 'ﾟ') {
		return 1
	}

	return 2
}

// baselineRatio 与 core/textrender 的 baselineRatio 一致：行盒顶边到基线的
// 距离约占 0.86em。宿主按基线画字，两处比例不一致会让对白和精灵文字错开。
const baselineRatio = 0.86

func baselineOffset(size int) int {
	if size <= 0 {
		return 0
	}
	return int(math.Round(float64(size) * baselineRatio))
}

// argbWithAlpha 把 0xRRGGBB 的脚本颜色拼成 core.TextRun 要的 0xAARRGGBB。
func argbWithAlpha(rgb int, alpha int) uint32 {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 255 {
		alpha = 255
	}
	return uint32(alpha&0xFF)<<24 | uint32(rgb)&0xFFFFFF
}

// lineWidth 按半角 1 格、全角 2 格估算一行宽度，只用于裁剪判定。
func lineWidth(text string, size int) int {
	if size <= 0 {
		return 0
	}
	units := 0
	for _, c := range text {
		units += DisplayWidth(c)
	}
	return units * size / 2
}

// BaselineOffset 暴露行盒顶边到基线的距离，供其它包把 top 坐标换成 run 的 Y。
func BaselineOffset(size int) int {
	return baselineOffset(size)
}

// Argb 把 0xRRGGBB 与 alpha 拼成 core.TextRun 的颜色。
func Argb(rgb int, alpha int) uint32 {
	return argbWithAlpha(rgb, alpha)
}

func WrapText(text string, maxCharactersPerLine int) []string {
	var lines []string
	if len(text) == 0 {
		return lines
	}

	budget := maxCharactersPerLine
	if budget < 1 {
		budget = 1
	}

	budget *= 2
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "")
	for _, paragraph := range strings.Split(normalized, "\n") {
		wrapParagraph(paragraph, budget, &lines)
	}

	return lines
}

func DrawMessageBox(target *bishopgfx.BgraCanvas, box *EmonMessageBox, style *EmonTextStyle, glyphs GlyphSource, revealCount int, b byte, g byte, r byte) {
	DrawMessageBoxAt(target, box, style, glyphs, revealCount, 0, 0, b, g, r)
}

// MessageColors 是正文/名字的颜色与阴影配置，来自脚本对
// メッセージボックス / 名前 两个精灵的 SetSprTextColor 与
// 文章文字フラグ（ANTIEREA@SHADOW）。
//
// 原引擎的文字对象 sub_43AB00 把颜色和 SHADOW 标志一起挂在文本对象上，
// 每个角色在 文章表示 宏里先 SetSprTextColor(メッセージボックス,キャラ色)
// 再显示台词。BOKUONE 的三个女主角就是靠这个机制显示粉/蓝/青三种颜色；
// 只用白色画会把角色配色整个丢掉。
type MessageColors struct {
	Body          int
	Speaker       int
	Shadow        int
	ShadowEnabled bool
}

// drawColoredLine 把一行字追加到画布文字列表。SHADOW 的偏移量与精灵文本一致
// （+2,+2），原引擎 message 文本对象与精灵文本共用同一套 GDI 绘制路径。
//
// 引擎不再把字形栅格进帧：文字条目交给宿主用平台字体绘制，阴影由宿主按
// Shadow 偏移重画一份。
func drawColoredLine(target *bishopgfx.BgraCanvas, text string, x, y, size int, color int, colors MessageColors) {
	shadow := int32(0)
	if colors.ShadowEnabled {
		shadow = 2
	}
	target.Runs = append(target.Runs, core.TextRun{
		Text:    text,
		X:       int32(x),
		Y:       int32(y + baselineOffset(size)),
		Size:    int32(size),
		Color:   argbWithAlpha(color, 255),
		Shadow:  shadow,
	})
}

// DrawMessageBoxAt 把对白画到消息窗口原点 (originX, originY) 上。原引擎的
// 文章文字位置是相对 メッセージウィンドウ 的（ASENURE 是 0,425），不是屏幕
// 绝对坐标；直接按 style.X/Y 画会把文字画到画面顶部。
func DrawMessageBoxAt(target *bishopgfx.BgraCanvas, box *EmonMessageBox, style *EmonTextStyle, glyphs GlyphSource, revealCount, originX, originY int, b byte, g byte, r byte) {
	DrawMessageBoxColorsAt(target, box, style, glyphs, revealCount, originX, originY,
		MessageColors{
			Body:    int(r)<<16 | int(g)<<8 | int(b),
			Speaker: int(r)<<16 | int(g)<<8 | int(b),
		})
}

// DrawMessageBoxColorsAt 与 DrawMessageBoxAt 相同，但正文/名字分别取色，
// 并按 MessageColors.ShadowEnabled 画阴影。
//
// glyphs 只为兼容调用方而保留：排版不再依赖字形点阵，实际绘制交给宿主。
func DrawMessageBoxColorsAt(target *bishopgfx.BgraCanvas, box *EmonMessageBox, style *EmonTextStyle, _ GlyphSource, revealCount, originX, originY int, colors MessageColors) {
	if !box.visible {
		return
	}

	if len(box.speaker) != 0 {
		drawColoredLine(target, box.speaker, style.SpeakerX+originX, style.SpeakerY+originY, style.Size, colors.Speaker, colors)
	}

	shown := core.Clamp(revealCount, 0, runeLength(box.body))
	if shown == 0 {
		return
	}

	lines := layout(box, shown, style.MaxCharactersPerLine)
	count := len(lines)
	maxLines := style.MaxLines
	if maxLines < 0 {
		maxLines = 0
	}

	// 原引擎的对话框是滚动窗口：#Return 只把读取指针挪到下一行，正文持续
	// 累积，超出可见行数时显示最新的几行。这里用"取末尾 maxLines 行"实现，
	// 否则累加多行旁白时会一直只画最早几行，最新行进不来。
	start := 0
	if count > maxLines {
		start = count - maxLines
		count = maxLines
	}

	for i := 0; i < count; i++ {
		drawColoredLine(target, lines[start+i], style.X+originX, style.Y+originY+(i*style.LineHeight), style.Size, colors.Body, colors)
	}
}

func DrawMessageBoxFace(target *bishopgfx.BgraCanvas, box *EmonMessageBox, style *EmonTextStyle, face *EmonFontFace) {
	DrawMessageBox(target, box, style, face, box.revealCount, 255, 255, 255)
}

func ClearCache() {
	cacheGate.Lock()
	defer cacheGate.Unlock()
	layoutCache = map[layoutKey][]string{}
	layoutNodes = map[layoutKey]*list.Element{}
	layoutOrder.Init()
	spriteWrapCache = map[spriteWrapKey][]string{}
	spriteWrapNodes = map[spriteWrapKey]*list.Element{}
	spriteWrapOrder.Init()
	textReports = map[textReportKey]struct{}{}
}

func WrapSpriteText(setting *EmonSpriteTextSetting, text string, glyphs GlyphSource) []string {
	if setting == nil || glyphs == nil {
		return nil
	}

	if len(text) == 0 {
		return nil
	}

	metrics := setting.Metrics()
	key := spriteWrapKey{
		text:      text,
		wrapWidth: metrics.WrapWidth,
		size:      setting.size,
		family:    setting.family,
		weight:    metrics.Weight,
		quality:   metrics.Quality,
	}

	cacheGate.Lock()
	if cached, ok := spriteWrapCache[key]; ok {
		touchSprite(key)
		cacheGate.Unlock()
		return cached
	}
	cacheGate.Unlock()

	var lines []string
	for _, segment := range strings.Split(text, "\n") {
		wrapSegment(segment, metrics.WrapWidth, setting, glyphs, &lines)
	}

	result := lines
	if result == nil {
		result = []string{}
	}

	cacheGate.Lock()
	if existing, ok := spriteWrapCache[key]; ok {
		touchSprite(key)
		cacheGate.Unlock()
		return existing
	}

	spriteWrapCache[key] = result
	spriteWrapNodes[key] = spriteWrapOrder.PushBack(key)
	for spriteWrapOrder.Len() > spriteLayoutCacheCapacity {
		oldest := spriteWrapOrder.Front()
		if oldest == nil {
			break
		}

		spriteWrapOrder.Remove(oldest)
		oldKey := oldest.Value.(spriteWrapKey)
		delete(spriteWrapCache, oldKey)
		delete(spriteWrapNodes, oldKey)
	}
	cacheGate.Unlock()

	return result
}

func DrawSpriteText(target *bishopgfx.BgraCanvas, setting *EmonSpriteTextSetting, text string, originX int, originY int, alpha int, glyphs GlyphSource, clipLeft int, clipTop int, clipRight int, clipBottom int, hasClip bool) {
	if target == nil || setting == nil || glyphs == nil {
		return
	}

	metrics := setting.Metrics()
	clip := resolveClip(target, clipLeft, clipTop, clipRight, clipBottom, hasClip)
	_, styled := glyphs.(StyledGlyphSource)
	wrapped := 0
	drawn := 0
	ink := 0
	missing := 0
	status := "draw"
	if len(text) == 0 {
		status = "empty"
	} else if setting.size <= 0 {
		status = "size"
	} else if alpha <= 0 {
		status = "alpha"
	} else {
		lines := WrapSpriteText(setting, text, glyphs)
		wrapped = len(lines)
		maxLines := metrics.MaxLines
		if maxLines < 1 {
			maxLines = 1
		}

		drawn = wrapped
		if drawn > maxLines {
			drawn = maxLines
		}

		if drawn == 0 {
			status = "no-lines"
		} else if clip.right <= clip.left || clip.bottom <= clip.top {
			status = "no-clip"
		} else {
			// 字形不再栅格进帧：一行文字就是一条 run，墨量按行计数，
			// 报告里的 ink 仍然能反映"这一帧有没有画出东西"。
			ink = drawSpriteTextLines(target, setting, metrics, lines, drawn, originX, originY, clip)
		}
	}

	reportSpriteText(setting, metrics, text, alpha, wrapped, drawn, ink, missing, styled, clip, status)
}

func layout(box *EmonMessageBox, shown int, maxCharactersPerLine int) []string {
	key := layoutKey{body: box.body, shown: shown, maxCharactersPerLine: maxCharactersPerLine}
	cacheGate.Lock()
	if cached, ok := layoutCache[key]; ok {
		touch(key)
		cacheGate.Unlock()
		return cached
	}
	cacheGate.Unlock()

	lines := WrapText(prefixRunes(box.body, shown), maxCharactersPerLine)
	cacheGate.Lock()
	if existing, ok := layoutCache[key]; ok {
		touch(key)
		cacheGate.Unlock()
		return existing
	}

	layoutCache[key] = lines
	layoutNodes[key] = layoutOrder.PushBack(key)
	for layoutOrder.Len() > layoutCacheCapacity {
		oldest := layoutOrder.Front()
		if oldest == nil {
			break
		}

		layoutOrder.Remove(oldest)
		oldKey := oldest.Value.(layoutKey)
		delete(layoutCache, oldKey)
		delete(layoutNodes, oldKey)
	}
	cacheGate.Unlock()

	return lines
}

func touch(key layoutKey) {
	node, ok := layoutNodes[key]
	if !ok || node == nil {
		return
	}

	layoutOrder.Remove(node)
	layoutOrder.PushBack(key)
}

func touchSprite(key spriteWrapKey) {
	node, ok := spriteWrapNodes[key]
	if !ok || node == nil {
		return
	}

	spriteWrapOrder.Remove(node)
	spriteWrapOrder.PushBack(key)
}

func wrapSegment(segment string, wrapWidth int, setting *EmonSpriteTextSetting, glyphs GlyphSource, lines *[]string) {
	runes := []rune(segment)
	start := 0
	width := 0
	breakAt := -1
	for index := 0; index < len(runes); index++ {
		c := runes[index]
		advance := glyphAdvance(glyphs, c, setting)
		if index > start && width+advance > wrapWidth {
			cut := index
			if breakAt > start {
				cut = breakAt
			}

			*lines = append(*lines, string(runes[start:cut]))
			start = cut
			for start < len(runes) && runes[start] == ' ' {
				start++
			}

			breakAt = -1
			width = 0
			index = start - 1
			continue
		}

		width += advance
		if c == ' ' {
			breakAt = index + 1
		}
	}

	*lines = append(*lines, string(runes[start:]))
}

func glyphAdvance(glyphs GlyphSource, c rune, setting *EmonSpriteTextSetting) int {
	metrics := setting.Metrics()
	glyph := EmonGlyphRequestRender(glyphs, c, setting.size, setting.family, metrics.Weight, metrics.Quality)
	if glyph == nil {
		return (DisplayWidth(c) * setting.size) / 2
	}

	return glyph.Width
}

func resolveClip(target *bishopgfx.BgraCanvas, clipLeft int, clipTop int, clipRight int, clipBottom int, hasClip bool) textClip {
	if !hasClip {
		return textClip{left: 0, top: 0, right: target.Width, bottom: target.Height, active: false}
	}

	left := core.Clamp(clipLeft, 0, target.Width)
	top := core.Clamp(clipTop, 0, target.Height)
	right := core.Clamp(clipRight, left, target.Width)
	bottom := core.Clamp(clipBottom, top, target.Height)
	return textClip{left: left, top: top, right: right, bottom: bottom, active: true}
}

func drawSpriteTextLines(target *bishopgfx.BgraCanvas, setting *EmonSpriteTextSetting, metrics *EmonSpriteTextMetrics, lines []string, lineCount int, originX int, originY int, clip textClip) int {
	flags := setting.flags
	shadow := int32(0)
	if (flags & EmonSpriteShadow) != 0 {
		shadow = 2
	}
	outline := uint32(0)
	if (flags & EmonSpriteBlock) != 0 {
		outline = argbWithAlpha(setting.blockColor, 255)
	}
	bold := (flags & EmonSpriteFontBold) != 0
	color := argbWithAlpha(setting.color, 255)
	size := setting.size

	ink := 0
	for index := 0; index < lineCount && index < len(lines); index++ {
		text := lines[index]
		if text == "" {
			continue
		}
		// 行盒顶边在 originY + index*行高；宿主按基线绘制。
		top := originY + index*metrics.LineHeight
		baseline := top + baselineOffset(size)
		if baseline < clip.top || baseline >= clip.bottom {
			continue
		}
		// 整行都落在裁剪框左侧时才丢弃；行首被裁掉一部分仍要画，
		// 否则靠左的对话在窄裁剪里会整行消失。
		if originX >= clip.right || originX+lineWidth(text, size) <= clip.left {
			continue
		}
		target.Runs = append(target.Runs, core.TextRun{
			Text:    text,
			X:       int32(originX),
			Y:       int32(baseline),
			Size:    int32(size),
			Color:   color,
			Outline: outline,
			Shadow:  shadow,
			Bold:    bold,
		})
		ink++
	}

	return ink
}

func reportSpriteText(setting *EmonSpriteTextSetting, metrics *EmonSpriteTextMetrics, text string, alpha int, wrapped int, drawn int, ink int, missing int, styled bool, clip textClip, status string) {
	length := runeLength(text)
	key := textReportKey{
		name:       setting.name,
		length:     length,
		family:     setting.family,
		boxWidth:   setting.boxWidth,
		boxHeight:  setting.boxHeight,
		lineHeight: metrics.LineHeight,
		maxLines:   metrics.MaxLines,
		noInk:      ink == 0,
	}
	cacheGate.Lock()
	if _, ok := textReports[key]; ok || len(textReports) >= textReportCapacity {
		cacheGate.Unlock()
		return
	}

	textReports[key] = struct{}{}
	cacheGate.Unlock()

	core.Logf("emtext: name='%s' status=%s chars=%d family='%s' styled=%t size=%d box=%dx%d wrap=%d lh=%d max=%d wrapped=%d drawn=%d ink=%d nulls=%d alpha=%d clip=%d,%d,%d,%d active=%t",
		setting.name, status, length, setting.family, styled, setting.size, setting.boxWidth, setting.boxHeight, metrics.WrapWidth, metrics.LineHeight, metrics.MaxLines, wrapped, drawn, ink, missing, alpha, clip.left, clip.top, clip.right, clip.bottom, clip.active)
}

func wrapParagraph(paragraph string, budget int, lines *[]string) {
	runes := []rune(paragraph)
	start := 0
	for start < len(runes) && (runes[start] == ' ' || runes[start] == '\u3000') {
		start++
	}

	if start >= len(runes) {
		*lines = append(*lines, "")
		return
	}

	// 按“能放下多少字就放多少字”切行，不在标点处提前回退。
	// 原引擎 sub_43A680 用 GetTextExtentExPointA 按像素宽度取最长前缀，
	// 没有禁则/单词回退；早先“优先在 、。空格处断行”会让长句每行只剩
	// 二十八九个字、右侧大片留白，和原版排版明显不同。
	i := start
	for i < len(runes) {
		width := 0
		j := i
		for j < len(runes) {
			c := runes[j]
			w := DisplayWidth(c)
			if width+w > budget {
				break
			}
			width += w
			j++
		}

		if j >= len(runes) {
			*lines = append(*lines, string(runes[i:]))
			break
		}

		if j == i {
			j++
		}

		*lines = append(*lines, string(runes[i:j]))
		i = j
	}
}

func runeLength(s string) int {
	return len([]rune(s))
}

func prefixRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}

	runes := []rune(s)
	if n >= len(runes) {
		return s
	}

	return string(runes[:n])
}
