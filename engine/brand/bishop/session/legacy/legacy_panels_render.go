package legacy

import (
	"fmt"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core/textrender"
)

const (
	legacyColorWhite = 0xffffffff
	legacyColorBlack = 0xff000000
)

// legacyFill 的参数顺序是 Fill 的 blue, green, red，与 0xAARRGGBB 的通道顺序相反。
func legacyFill(canvas *gfx.BgraCanvas, rect legacyRect, blue, green, red byte) {
	canvas.Fill(rect.X, rect.Y, rect.W, rect.H, blue, green, red, 0xff)
}

// legacyBevel 画 Win95 式 3D 边：左上亮、右下暗。
func legacyBevel(canvas *gfx.BgraCanvas, rect legacyRect) {
	canvas.Fill(rect.X, rect.Y, rect.W, 1, 0xff, 0xff, 0xff, 0xff)
	canvas.Fill(rect.X, rect.Y, 1, rect.H, 0xff, 0xff, 0xff, 0xff)
	canvas.Fill(rect.X, rect.Y+rect.H-1, rect.W, 1, 0x80, 0x80, 0x80, 0xff)
	canvas.Fill(rect.X+rect.W-1, rect.Y, 1, rect.H, 0x80, 0x80, 0x80, 0xff)
}

func drawLegacyPanel(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, panel *LegacyPanel) {
	rect := panel.rect()
	if rect.W <= 0 || rect.H <= 0 {
		return
	}
	legacyFill(canvas, rect, 0xc0, 0xc0, 0xc0)
	legacyBevel(canvas, rect)
	titleRect := rect.at(3, 3, rect.W-6, legacyPanelTitleBarHeight)
	legacyFill(canvas, titleRect, 0x80, 0x00, 0x00)
	rendering.DrawText(canvas, renderer, legacyPanelTitle(panel), titleRect.X+6, titleRect.Y+4, 14, 0, 0, 16, titleRect.W-12, 1, legacyColorWhite)
	switch panel.Kind {
	case LegacyPanelCommand:
		drawCommandRows(canvas, renderer, panel, rect)
	case LegacyPanelSystem:
		drawSystemControls(canvas, renderer, panel, rect)
	case LegacyPanelSave, LegacyPanelLoad:
		drawSaveSlots(canvas, renderer, panel, rect)
	}
	drawPanelFooter(canvas, renderer, panel, rect)
}

func legacyPanelTitle(panel *LegacyPanel) string {
	if panel.Title != "" {
		return panel.Title
	}
	switch panel.Kind {
	case LegacyPanelSystem:
		return "SYSTEM 設定"
	case LegacyPanelSave, LegacyPanelLoad:
		return "データの選択"
	}
	return ""
}

func drawCommandRows(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, panel *LegacyPanel, rect legacyRect) {
	for row := 0; row < legacyListRows; row++ {
		index := panel.Scroll + row
		if index < 0 || index >= len(panel.Items) {
			continue
		}
		cell := legacyCommandRowRect(rect, row)
		textColor := uint32(legacyColorBlack)
		if index == panel.Highlight {
			legacyFill(canvas, cell, 0x80, 0x00, 0x00)
			textColor = legacyColorWhite
		} else {
			legacyFill(canvas, cell, 0xff, 0xff, 0xff)
		}
		rendering.DrawText(canvas, renderer, panel.Items[index], cell.X+6, cell.Y+4, 16, 0, 0, 16, cell.W-12, 1, textColor)
	}
}

func drawSystemControls(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, panel *LegacyPanel, rect legacyRect) {
	for _, control := range legacySystemControls(rect) {
		if control.Kind == "save" || control.Kind == "load" {
			legacyFill(canvas, control.Rect, 0xc0, 0xc0, 0xc0)
			legacyBevel(canvas, control.Rect)
		}
		label := systemControlMark(control.Kind, panel) + control.Label
		rendering.DrawText(canvas, renderer, label, control.Rect.X+4, control.Rect.Y+3, 14, 0, 0, 16, control.Rect.W-8, 1, legacyColorBlack)
	}
}

func systemControlMark(kind string, panel *LegacyPanel) string {
	switch kind {
	case "instant":
		return radioMark(panel.InstantText)
	case "typewriter":
		return radioMark(!panel.InstantText)
	case "bgm":
		if panel.BgmEnabled {
			return "[X] "
		}
		return "[ ] "
	}
	return ""
}

func radioMark(selected bool) string {
	if selected {
		return "(*) "
	}
	return "( ) "
}

func drawSaveSlots(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, panel *LegacyPanel, rect legacyRect) {
	for slot := 0; slot < legacySlotCount; slot++ {
		cell := legacySlotRect(rect, slot)
		textColor := uint32(legacyColorBlack)
		if slot == panel.Highlight {
			legacyFill(canvas, cell, 0x80, 0x00, 0x00)
			textColor = legacyColorWhite
		} else {
			legacyFill(canvas, cell, 0xff, 0xff, 0xff)
		}
		label := fmt.Sprintf("%2d : ", slot+1)
		if slot < len(panel.Items) {
			label += panel.Items[slot]
		}
		rendering.DrawText(canvas, renderer, label, cell.X+4, cell.Y+5, 14, 0, 0, 16, cell.W-8, 1, textColor)
	}
}

// drawPanelFooter 画「戻る」按钮、滚动箭头（^ 上 / v 下，避开字体可能缺失的符号）和状态行。
func drawPanelFooter(canvas *gfx.BgraCanvas, renderer *textrender.Renderer, panel *LegacyPanel, rect legacyRect) {
	back := rect.backButton()
	legacyFill(canvas, back, 0xc0, 0xc0, 0xc0)
	legacyBevel(canvas, back)
	rendering.DrawText(canvas, renderer, "戻る", back.X+24, back.Y+4, 14, 0, 0, 16, back.W-24, 1, legacyColorBlack)
	if panel.Kind == LegacyPanelCommand {
		if panel.Scroll > 0 {
			rendering.DrawText(canvas, renderer, "^", rect.X+12, back.Y+4, 14, 0, 0, 16, 16, 1, legacyColorBlack)
		}
		if panel.Scroll+legacyListRows < len(panel.Items) {
			rendering.DrawText(canvas, renderer, "v", rect.X+34, back.Y+4, 14, 0, 0, 16, 16, 1, legacyColorBlack)
		}
	}
	if panel.Status != "" {
		statusX := back.X + back.W + 8
		statusWidth := rect.X + rect.W - 8 - statusX
		if statusWidth > 0 {
			rendering.DrawText(canvas, renderer, panel.Status, statusX, back.Y+4, 14, 0, 0, 16, statusWidth, 1, legacyColorBlack)
		}
	}
}
