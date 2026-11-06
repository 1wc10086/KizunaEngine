package text

import (
	"container/list"
	"strings"
	"sync"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
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

var blockOffsets = [...]int{
	-1, -1,
	0, -1,
	1, -1,
	-1, 0,
	1, 0,
	-1, 1,
	0, 1,
	1, 1,
}

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
	if !box.visible {
		return
	}

	if len(box.speaker) != 0 {
		drawLine(target, box.speaker, style.SpeakerX, style.SpeakerY, style.Size, glyphs, b, g, r)
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

	if count > maxLines {
		count = maxLines
	}

	for i := 0; i < count; i++ {
		drawLine(target, lines[i], style.X, style.Y+(i*style.LineHeight), style.Size, glyphs, b, g, r)
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
			ink, missing = drawSpriteTextLines(target, setting, metrics, lines, drawn, glyphs, originX, originY, alpha, clip)
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

func drawSpriteTextLines(target *bishopgfx.BgraCanvas, setting *EmonSpriteTextSetting, metrics *EmonSpriteTextMetrics, lines []string, lineCount int, glyphs GlyphSource, originX int, originY int, alpha int, clip textClip) (int, int) {
	ink := 0
	flags := setting.flags
	if (flags & EmonSpriteBlock) != 0 {
		for index := 0; index < len(blockOffsets); index += 2 {
			outlineInk, _ := drawSpriteTextPass(target, setting, metrics, lines, lineCount, glyphs, originX+blockOffsets[index], originY+blockOffsets[index+1], alpha, setting.blockColor, clip)
			ink += outlineInk
		}
	}

	if (flags & EmonSpriteShadow) != 0 {
		castInk, _ := drawSpriteTextPass(target, setting, metrics, lines, lineCount, glyphs, originX+2, originY+2, alpha, setting.shadowColor, clip)
		ink += castInk
	}

	bodyInk, missing := drawSpriteTextPass(target, setting, metrics, lines, lineCount, glyphs, originX, originY, alpha, setting.color, clip)
	return ink + bodyInk, missing
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

func drawSpriteTextPass(target *bishopgfx.BgraCanvas, setting *EmonSpriteTextSetting, metrics *EmonSpriteTextMetrics, lines []string, lineCount int, glyphs GlyphSource, originX int, originY int, alpha int, rgb int, clip textClip) (int, int) {
	blue := byte(rgb & 0xFF)
	green := byte((rgb >> 8) & 0xFF)
	red := byte((rgb >> 16) & 0xFF)
	size := setting.size
	ink := 0
	missing := 0
	for index := 0; index < lineCount; index++ {
		y := originY + (index * metrics.LineHeight)
		cursor := originX
		for _, c := range lines[index] {
			glyph := EmonGlyphRequestRender(glyphs, c, size, setting.family, metrics.Weight, metrics.Quality)
			if glyph == nil {
				missing++
				cursor += (DisplayWidth(c) * size) / 2
				continue
			}

			ink += blendGlyph(target.Buffer, target.Width, glyph, cursor, y, blue, green, red, alpha, clip)
			cursor += glyph.Width
		}
	}

	return ink, missing
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

	i := start
	for i < len(runes) {
		width := 0
		j := i
		lastBreak := -1
		for j < len(runes) {
			c := runes[j]
			w := DisplayWidth(c)
			if width+w > budget {
				break
			}

			width += w
			if c == '、' || c == '。' || c == ' ' || c == '\u3000' {
				lastBreak = j + 1
			}

			j++
		}

		if j >= len(runes) {
			*lines = append(*lines, string(runes[i:]))
			break
		}

		if j == i {
			j++
		}

		cut := j
		if lastBreak > i {
			cut = lastBreak
		}

		*lines = append(*lines, string(runes[i:cut]))
		i = cut
		for i < len(runes) && (runes[i] == ' ' || runes[i] == '\u3000') {
			i++
		}
	}
}

func drawLine(target *bishopgfx.BgraCanvas, line string, x int, y int, pixelSize int, glyphs GlyphSource, b byte, g byte, r byte) {
	size := pixelSize
	if size < 1 {
		size = 1
	}

	cursor := x
	clip := textClip{left: 0, top: 0, right: target.Width, bottom: target.Height, active: false}
	for _, c := range line {
		glyph := glyphs.Render(c, size)
		if glyph == nil {
			cursor += (DisplayWidth(c) * size) / 2
			continue
		}

		blendGlyph(target.Buffer, target.Width, glyph, cursor, y, b, g, r, 255, clip)
		cursor += glyph.Width
	}
}

func blendGlyph(buffer []byte, canvasWidth int, glyph *EmonGlyphBitmap, x int, y int, b byte, g byte, r byte, opacity int, clip textClip) int {
	alpha := glyph.Alpha
	painted := 0
	for row := 0; row < glyph.Height; row++ {
		targetY := y + row
		if targetY < clip.top || targetY >= clip.bottom {
			continue
		}

		targetRow := targetY * canvasWidth * 4
		glyphRow := row * glyph.Width
		for column := 0; column < glyph.Width; column++ {
			targetX := x + column
			if targetX < clip.left || targetX >= clip.right {
				continue
			}

			a := int(alpha[glyphRow+column])
			if a == 0 {
				continue
			}

			if opacity < 255 {
				a = a * opacity / 255
				if a == 0 {
					continue
				}
			}

			offset := targetRow + (targetX * 4)
			if a >= 255 {
				buffer[offset] = b
				buffer[offset+1] = g
				buffer[offset+2] = r
				buffer[offset+3] = 255
				painted++
				continue
			}

			keep := 255 - a
			buffer[offset] = byte(((int(b) * a) + (int(buffer[offset]) * keep) + 127) / 255)
			buffer[offset+1] = byte(((int(g) * a) + (int(buffer[offset+1]) * keep) + 127) / 255)
			buffer[offset+2] = byte(((int(r) * a) + (int(buffer[offset+2]) * keep) + 127) / 255)
			buffer[offset+3] = 255
			painted++
		}
	}

	return painted
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
