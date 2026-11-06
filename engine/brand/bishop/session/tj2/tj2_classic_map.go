package tj2

import (
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

const (
	Tj2ClassicRowCount = 12
	Tj2ClassicHitX     = 496
	Tj2ClassicHitWidth = 112
	// MVM01 的 12 个地点条从 y=64 开始，每格 32px；以前写 68 会让
	// 最后两行点击区偏下 4px，点在按钮上沿时偶尔选不中。
	Tj2ClassicHitTop    = 64
	Tj2ClassicHitHeight = 32

	Tj2ClassicCellX      = 496
	Tj2ClassicCellTop    = 72
	Tj2ClassicCellWidth  = 112
	Tj2ClassicCellHeight = 24

	Tj2ClassicLabelX      = 356
	Tj2ClassicLabelY      = 16
	Tj2ClassicLabelWidth  = 112
	Tj2ClassicLabelHeight = 96

	Tj2ClassicPreviewSourceX      = 20
	Tj2ClassicPreviewSourceY      = 120
	Tj2ClassicPreviewSourceWidth  = 448
	Tj2ClassicPreviewSourceHeight = 336
	Tj2ClassicPreviewX            = 20
	Tj2ClassicPreviewY            = 120
)

const (
	tj2ClassicBackgroundName = "MVB01.KG"
	tj2ClassicOverlayName    = "MVM01.KG"
	tj2ClassicCellsName      = "MVP01.KG"
	tj2ClassicNoneLabel      = "none"

	tj2ClassicKeyBlue  = 0
	tj2ClassicKeyGreen = 254
	tj2ClassicKeyRed   = 0
)

var tj2ClassicPreviewStems = []string{
	"01", "02", "03", "01", "02", "03",
	"04", "05", "06", "07", "08", "09",
}

type Tj2ClassicMap struct {
	labels           [Tj2ClassicRowCount]string
	hasLabel         [Tj2ClassicRowCount]bool
	composedRevision int
	active           bool
	selected         int
	hovered          int
	revision         int
}

func (m *Tj2ClassicMap) Active() bool {
	return m.active
}

func (m *Tj2ClassicMap) Selected() int {
	return m.selected
}

func (m *Tj2ClassicMap) Hovered() int {
	return m.hovered
}

func (m *Tj2ClassicMap) Revision() int {
	return m.revision
}

func (m *Tj2ClassicMap) SetActive(active bool) {
	if m.active == active {
		return
	}
	m.active = active
	if active {
		m.selected = 0
	} else {
		m.hovered = -1
	}
	m.revision++
}

func (m *Tj2ClassicMap) SetLabel(row int, name string) {
	if row < 0 || row >= Tj2ClassicRowCount {
		core.Logf("tj2: classic map label row %d out of range", row)
		return
	}
	var label string
	has := false
	if trimmed := strings.TrimSpace(name); len(trimmed) > 0 && !strings.EqualFold(trimmed, tj2ClassicNoneLabel) {
		label = trimmed
		has = true
	}
	if m.hasLabel[row] == has && (!has || m.labels[row] == label) {
		return
	}
	m.labels[row] = label
	m.hasLabel[row] = has
	m.revision++
}

func (m *Tj2ClassicMap) SetHovered(row int) {
	hovered := -1
	if row >= 0 && row < Tj2ClassicRowCount {
		hovered = row
	}
	if m.hovered == hovered {
		return
	}
	m.hovered = hovered
	m.revision++
}

func (m *Tj2ClassicMap) SetSelected(row int) {
	if row < 0 || row >= Tj2ClassicRowCount {
		return
	}
	if m.selected == row {
		return
	}
	m.selected = row
	m.revision++
}

func (m *Tj2ClassicMap) LabelFor(row int) string {
	if row < 0 || row >= Tj2ClassicRowCount || !m.hasLabel[row] {
		return ""
	}
	return m.labels[row]
}

func (m *Tj2ClassicMap) RowAt(x, y int) int {
	if x < Tj2ClassicHitX || x >= Tj2ClassicHitX+Tj2ClassicHitWidth || y < Tj2ClassicHitTop {
		return -1
	}
	row := (y - Tj2ClassicHitTop) / Tj2ClassicHitHeight
	if row < 0 || row >= Tj2ClassicRowCount {
		return -1
	}
	return row
}

func (m *Tj2ClassicMap) ComposeTo(frame *gfx.BgraCanvas, images *rendering.ImageCache) bool {
	if !m.active {
		return false
	}
	if m.composedRevision == m.revision {
		return false
	}
	m.composedRevision = m.revision
	background := images.Get(tj2ClassicBackgroundName)
	overlay := images.Get(tj2ClassicOverlayName)
	if background == nil && overlay == nil {
		core.Logf("tj2: classic map background missing")
		return false
	}
	frame.Clear(0, 0, 0, 255)
	if background != nil {
		frame.Draw(background, 0, 0)
	}
	if overlay != nil {
		frame.Draw(overlay, 0, 0)
	}
	cells := images.Get(tj2ClassicCellsName)
	if cells == nil {
		core.Logf("tj2: classic map cells '%s' missing", tj2ClassicCellsName)
	} else {
		for row := 0; row < Tj2ClassicRowCount; row++ {
			state := 0
			switch row {
			case m.selected:
				state = 2
			case m.hovered:
				state = 1
			}
			frame.DrawRegion(cells, Tj2ClassicCellWidth*state, Tj2ClassicCellHeight*row, Tj2ClassicCellWidth, Tj2ClassicCellHeight, Tj2ClassicCellX, Tj2ClassicCellTop+row*Tj2ClassicHitHeight)
		}
	}
	selected := m.selected
	if selected < 0 {
		selected = 0
	}
	if selected >= Tj2ClassicRowCount {
		selected = Tj2ClassicRowCount - 1
	}
	if m.hasLabel[selected] {
		labelImage := images.Get(m.labels[selected])
		if labelImage == nil {
			core.Logf("tj2: classic map label '%s' missing", m.labels[selected])
		} else {
			frame.DrawRegion(labelImage, 0, 0, Tj2ClassicLabelWidth, Tj2ClassicLabelHeight, Tj2ClassicLabelX, Tj2ClassicLabelY)
		}
	}
	previewName := "mh" + tj2ClassicPreviewStems[selected] + "a.kg"
	preview := images.Get(previewName)
	if preview == nil {
		core.Logf("tj2: classic map preview '%s' missing", previewName)
	} else {
		drawKeyedTj2Classic(frame, preview)
	}
	return true
}

func drawKeyedTj2Classic(frame *gfx.BgraCanvas, image *gfx.BsgImage) {
	target := frame.Buffer
	source := image.Pixels
	for row := 0; row < Tj2ClassicPreviewSourceHeight; row++ {
		sourceY := Tj2ClassicPreviewSourceY + row
		targetY := Tj2ClassicPreviewY + row
		if sourceY < 0 || sourceY >= image.Height || targetY < 0 || targetY >= frame.Height {
			continue
		}
		for column := 0; column < Tj2ClassicPreviewSourceWidth; column++ {
			sourceX := Tj2ClassicPreviewSourceX + column
			targetX := Tj2ClassicPreviewX + column
			if sourceX < 0 || sourceX >= image.Width || targetX < 0 || targetX >= frame.Width {
				continue
			}
			sourceOffset := sourceY*image.Width*4 + sourceX*4
			if source[sourceOffset] == tj2ClassicKeyBlue && source[sourceOffset+1] == tj2ClassicKeyGreen && source[sourceOffset+2] == tj2ClassicKeyRed {
				continue
			}
			targetOffset := targetY*frame.Width*4 + targetX*4
			target[targetOffset] = source[sourceOffset]
			target[targetOffset+1] = source[sourceOffset+1]
			target[targetOffset+2] = source[sourceOffset+2]
			target[targetOffset+3] = 255
		}
	}
}
