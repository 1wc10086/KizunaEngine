package tj2

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	Tj2MapWidth  = 640
	Tj2MapHeight = 480

	Tj2MapMinCell = 1
	Tj2MapMaxCell = 10

	Tj2MapMaxFallbackChoices = 16
)

const (
	tj2MapStoreSize          = 512
	tj2MapHighlightBias      = 18
	tj2MapDestinationBias    = 217
	tj2MapDefaultBase        = 488
	tj2MapColumns            = 6
	tj2MapPageStride         = 11
	tj2MapIconAnchorX        = 534
	tj2MapIconAnchorY        = 16
	tj2MapIconTop            = 9
	tj2MapIconWidth          = 90
	tj2MapIconHeight         = 90
	tj2MapIconStride         = 92
	tj2MapIconLeftTrim       = 78
	tj2MapClockWidth         = 140
	tj2MapClockHeight        = 56
	tj2MapClockX             = 24
	tj2MapClockY             = 8
	tj2MapPaletteEntries     = 256
	tj2MapPaletteStride      = 4
	tj2MapPaletteOffsetField = 0x0C

	tj2MapSchool1Name = "SCHOOL1.KG"
	tj2MapSchool2Name = "SCHOOL2.KG"
	tj2MapMaskName    = "MMASK.KG"
	tj2MapPartsName   = "PARTS.KG"
	tj2MapClockName   = "SP_TIME.KG"
)

type Tj2MapScreen struct {
	cells            [tj2MapStoreSize]byte
	lookup           []byte
	school1          *gfx.BsgImage
	school2          *gfx.BsgImage
	parts            *gfx.BsgImage
	clock            *gfx.BsgImage
	composedRevision int
	active           bool
	fallback         bool
	hoverCell        int
	page             int
	frame            int
	clockVisible     bool
	revision         int
}

func (m *Tj2MapScreen) Active() bool {
	return m.active
}

func (m *Tj2MapScreen) Fallback() bool {
	return m.fallback
}

func (m *Tj2MapScreen) HoverCell() int {
	return m.hoverCell
}

func (m *Tj2MapScreen) Page() int {
	return m.page
}

func (m *Tj2MapScreen) Frame() int {
	return m.frame
}

func (m *Tj2MapScreen) ClockVisible() bool {
	return m.clockVisible
}

func (m *Tj2MapScreen) Revision() int {
	return m.revision
}

func (m *Tj2MapScreen) RecordDefault(cell, destination int) {
	if cell < Tj2MapMinCell || cell > Tj2MapMaxCell {
		return
	}
	m.cells[tj2MapDefaultBase+cell] = byte(destination)
}

func (m *Tj2MapScreen) RecordCells(page, cell, frame, highlight, destination int) {
	highlightOffset := 2 * (frame + tj2MapColumns*(tj2MapPageStride*page+cell+tj2MapHighlightBias))
	if highlightOffset >= 0 && highlightOffset < tj2MapStoreSize {
		m.cells[highlightOffset] = byte(highlight)
	}
	destinationOffset := 2*(frame+tj2MapColumns*(tj2MapPageStride*page+cell)) + tj2MapDestinationBias
	if destinationOffset >= 0 && destinationOffset < tj2MapStoreSize {
		m.cells[destinationOffset] = byte(destination)
	}
}

func (m *Tj2MapScreen) HighlightFor(cell, page, frame int) int {
	offset := 2 * (frame + tj2MapColumns*(tj2MapPageStride*page+cell+tj2MapHighlightBias))
	if offset < 0 || offset >= tj2MapStoreSize {
		return 0
	}
	return int(m.cells[offset])
}

func (m *Tj2MapScreen) DestinationFor(cell, page, frame int) int {
	if cell < Tj2MapMinCell || cell > Tj2MapMaxCell {
		return 0
	}
	offset := 2*(frame+tj2MapColumns*(tj2MapPageStride*page+cell)) + tj2MapDestinationBias
	var destination byte
	if offset >= 0 && offset < tj2MapStoreSize {
		destination = m.cells[offset]
	}
	if destination != 0 {
		return int(destination)
	}
	return int(m.cells[tj2MapDefaultBase+cell])
}

func (m *Tj2MapScreen) FallbackDestinations(page, frame int) []int {
	destinations := make([]int, 0, Tj2MapMaxFallbackChoices)
	for cell := Tj2MapMinCell; cell <= Tj2MapMaxCell && len(destinations) < Tj2MapMaxFallbackChoices; cell++ {
		destination := m.DestinationFor(cell, page, frame)
		if destination == 0 {
			continue
		}
		dup := false
		for _, d := range destinations {
			if d == destination {
				dup = true
				break
			}
		}
		if !dup {
			destinations = append(destinations, destination)
		}
	}
	return destinations
}

func (m *Tj2MapScreen) TryGetFallbackDestination(page, frame, index int) (int, bool) {
	destinations := m.FallbackDestinations(page, frame)
	if index < 0 || index >= len(destinations) {
		return 0, false
	}
	return destinations[index], true
}

func (m *Tj2MapScreen) SetClockVisible(visible bool) {
	if m.clockVisible == visible {
		return
	}
	m.clockVisible = visible
	if m.active && !m.fallback {
		m.revision++
	}
}

func (m *Tj2MapScreen) Activate(images *rendering.ImageCache, resources *resource.GameResourceIndex, clockVisible bool, page, frame int) bool {
	if m.active && !m.fallback {
		return true
	}
	school1 := images.Get(tj2MapSchool1Name)
	mask := images.Get(tj2MapMaskName)
	if school1 == nil || mask == nil {
		core.Logf("tj2: map art missing school1=%v mask=%v", school1 != nil, mask != nil)
		return false
	}
	lookup := buildTj2MapLookup(resources, mask)
	if lookup == nil {
		core.Logf("tj2: map mask lookup failed")
		return false
	}
	m.school1 = school1
	m.school2 = images.Get(tj2MapSchool2Name)
	m.parts = images.Get(tj2MapPartsName)
	if clockVisible {
		m.clock = images.Get(tj2MapClockName)
	} else {
		m.clock = nil
	}
	m.lookup = lookup
	m.clockVisible = clockVisible
	m.page = page
	m.frame = frame
	m.hoverCell = 0
	m.fallback = false
	m.active = true
	m.revision++
	core.Logf("tj2: map active page=%d frame=%d clock=%v school2=%v parts=%v clockArt=%v", page, frame, clockVisible, m.school2 != nil, m.parts != nil, m.clock != nil)
	return true
}

func (m *Tj2MapScreen) BeginFallback(page, frame int) {
	m.Deactivate()
	m.page = page
	m.frame = frame
	m.fallback = true
	m.active = true
	core.Logf("tj2: map fallback page=%d frame=%d", page, frame)
}

func (m *Tj2MapScreen) Deactivate() {
	m.active = false
	m.fallback = false
	m.hoverCell = 0
	m.lookup = nil
	m.school1 = nil
	m.school2 = nil
	m.parts = nil
	m.clock = nil
}

func (m *Tj2MapScreen) CellAt(x, y int) int {
	if m.lookup == nil || x < 0 || y < 0 || x >= Tj2MapWidth || y >= Tj2MapHeight {
		return 0
	}
	return int(m.lookup[y*Tj2MapWidth+x])
}

func (m *Tj2MapScreen) Hover(x, y int) bool {
	if !m.active || m.fallback {
		return false
	}
	cell := m.CellAt(x, y)
	if cell == m.hoverCell {
		return false
	}
	m.hoverCell = cell
	m.revision++
	return true
}

func (m *Tj2MapScreen) ComposeTo(frame *gfx.BgraCanvas) bool {
	if !m.active || m.fallback || m.school1 == nil || m.lookup == nil {
		return false
	}
	if m.composedRevision == m.revision {
		return false
	}
	m.composedRevision = m.revision
	frame.Clear(0, 0, 0, 255)
	frame.Draw(m.school1, 0, 0)
	if m.hoverCell >= Tj2MapMinCell && m.hoverCell <= Tj2MapMaxCell && m.school2 != nil && m.school2.Width == Tj2MapWidth && m.school2.Height == Tj2MapHeight {
		substituteTj2Map(frame, m.school2, m.lookup, m.hoverCell)
	}
	icon := m.HighlightFor(m.hoverCell, m.page, m.frame)
	if icon != 0 && m.parts != nil {
		frame.DrawRegion(m.parts, tj2MapIconStride*icon-tj2MapIconLeftTrim, tj2MapIconTop, tj2MapIconWidth, tj2MapIconHeight, tj2MapIconAnchorX, tj2MapIconAnchorY)
	}
	if m.clockVisible && m.clock != nil {
		frame.DrawRegion(m.clock, 0, 0, tj2MapClockWidth, tj2MapClockHeight, tj2MapClockX, tj2MapClockY)
	}
	return true
}

func substituteTj2Map(frame *gfx.BgraCanvas, school2 *gfx.BsgImage, lookup []byte, cell int) {
	source := school2.Pixels
	target := frame.Buffer
	if lookup == nil || len(source) != len(target) || len(lookup)*4 != len(target) {
		return
	}
	for index := range lookup {
		if lookup[index] != byte(cell) {
			continue
		}
		offset := index * 4
		target[offset] = source[offset]
		target[offset+1] = source[offset+1]
		target[offset+2] = source[offset+2]
		target[offset+3] = source[offset+3]
	}
}

func buildTj2MapLookup(resources *resource.GameResourceIndex, mask *gfx.BsgImage) []byte {
	if mask.Width != Tj2MapWidth || mask.Height != Tj2MapHeight {
		return nil
	}
	location, ok := resources.TryGet(tj2MapMaskName)
	if !ok {
		return nil
	}
	raw := location.Read()
	if len(raw) < tj2MapPaletteOffsetField+4 || raw[0] != 'K' || raw[1] != 'G' {
		return nil
	}
	paletteOffset := int(binutil.U32(raw[tj2MapPaletteOffsetField:]))
	if paletteOffset < 0 || int64(paletteOffset)+int64(tj2MapPaletteEntries*tj2MapPaletteStride) > int64(len(raw)) {
		return nil
	}
	colors := make(map[int]byte, tj2MapPaletteEntries)
	for index := 0; index < tj2MapPaletteEntries; index++ {
		base := paletteOffset + index*tj2MapPaletteStride
		key := int(raw[base])<<16 | int(raw[base+1])<<8 | int(raw[base+2])
		if _, dup := colors[key]; !dup {
			colors[key] = byte(index)
		}
	}
	pixels := mask.Pixels
	if len(pixels) != Tj2MapWidth*Tj2MapHeight*4 {
		return nil
	}
	lookup := make([]byte, Tj2MapWidth*Tj2MapHeight)
	for index := range lookup {
		key := int(pixels[index*4])<<16 | int(pixels[index*4+1])<<8 | int(pixels[index*4+2])
		if cell, ok := colors[key]; ok && int(cell) >= Tj2MapMinCell && int(cell) <= Tj2MapMaxCell {
			lookup[index] = cell
		} else {
			lookup[index] = 0
		}
	}
	return lookup
}
