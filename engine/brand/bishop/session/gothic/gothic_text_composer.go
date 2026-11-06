package gothic

import (
	"kizuna/engine/brand/bishop/gfx"
)

const (
	// MAZE 的正文没有窗口底图：原引擎把 24 字一行的文本居中画在画面顶部，
	// 每行之间留 25px。26 列是给旧 Gothic 作品的兜底，MAZE 用 40 列
	// 避免把剧本已经排好的 24 字行再次折行。
	gothicComposerTextOriginX = 21
	gothicComposerTextOriginY = 10
	gothicComposerTextColumns = 40
	gothicComposerTextRows    = 16
	gothicComposerTextTable   = 0
	gothicComposerMenuOriginY = 120
	gothicComposerMenuCenterX = 320
)

type gothicComposerLayout struct {
	originX     int
	originY     int
	columns     int
	rows        int
	menuOriginY int
	menuCenterX int
	// centered 表示正文逐行水平居中。MAZE 的剧本行本身就是 24 字定宽，
	// 原引擎按行宽居中；Toku1 等有固定文本框的作品必须保持左对齐。
	centered bool
	// shadow 表示正文需要 1px 黑投影。MAZE 没有文字窗口底图，必须描边。
	shadow bool
}

func gothicComposerLayoutFor(font *GothicFontFile) gothicComposerLayout {
	if font.GlyphWidth == 22 {
		return gothicComposerLayout{originX: GothicToku1TextOriginX, originY: GothicToku1TextOriginY, columns: GothicToku1TextColumns, rows: GothicToku1TextRows, menuOriginY: GothicToku1TextOriginY, menuCenterX: 320}
	}
	return gothicComposerLayout{
		originX:     gothicComposerTextOriginX,
		originY:     gothicComposerTextOriginY,
		columns:     gothicComposerTextColumns,
		rows:        gothicComposerTextRows,
		menuOriginY: gothicComposerMenuOriginY,
		menuCenterX: gothicComposerMenuCenterX,
		centered:    true,
		shadow:      true,
	}
}

func DrawGothicTextComposer(canvas *gfx.BgraCanvas, font *GothicFontFile, lines []string, messageVisible bool, menuItems []string, revealLimit int) {
	if canvas == nil || font == nil {
		return
	}
	layout := gothicComposerLayoutFor(font)
	advance := font.Advance
	lineHeight := font.LineHeight
	if revealLimit >= 0 {
		lines = gothicVisibleLines(lines, revealLimit)
	}
	if messageVisible && lines != nil {
		rows := len(lines)
		if rows > layout.rows {
			rows = layout.rows
		}
		for row := 0; row < rows; row++ {
			line := lines[row]
			if len(line) == 0 {
				continue
			}
			clipped := line
			if len([]rune(line)) > layout.columns {
				clipped = string([]rune(line)[:layout.columns])
			}
			x := layout.originX
			if layout.centered {
				x = (canvas.Width - advance*len([]rune(clipped))) / 2
			}
			if x < 0 {
				x = 0
			}
			if layout.shadow {
				DrawGothicTextColor(canvas, font, clipped, x+1, layout.originY+(row*lineHeight)+1, gothicComposerTextTable, 0, 0, 0)
			}
			DrawGothicText(canvas, font, clipped, x, layout.originY+(row*lineHeight), gothicComposerTextTable)
		}
	}
	if menuItems == nil || len(menuItems) == 0 {
		return
	}
	for index := 0; index < len(menuItems); index++ {
		item := menuItems[index]
		if len(item) == 0 {
			continue
		}
		clipped := item
		if len([]rune(item)) > layout.columns {
			clipped = string([]rune(item)[:layout.columns])
		}
		x := layout.menuCenterX - ((advance * len([]rune(clipped))) / 2)
		if layout.shadow {
			DrawGothicTextColor(canvas, font, clipped, x+1, layout.menuOriginY+(index*lineHeight)+1, gothicComposerTextTable, 0, 0, 0)
		}
		DrawGothicText(canvas, font, clipped, x, layout.menuOriginY+(index*lineHeight), gothicComposerTextTable)
	}
}

// gothicVisibleLines 按累计字数截断整页文本，用于逐字显示。
// 按行截断而不是逐行独立计数，换行处才不会多显示或少显示一个字。
func gothicVisibleLines(lines []string, limit int) []string {
	if limit < 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	remaining := limit
	for _, line := range lines {
		runes := []rune(line)
		if remaining <= 0 {
			break
		}
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		out = append(out, string(runes))
		remaining -= len(runes)
	}
	return out
}
