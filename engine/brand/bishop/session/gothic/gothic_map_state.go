package gothic

import (
	"kizuna/engine/brand/bishop/gfx"
)

type GothicMapMode int

const (
	GothicMapNone GothicMapMode = iota
	GothicMapRoute
	GothicMapClickMap
)

type gothicSmapKey struct {
	page int
	col  int
	row  int
}

type gothicSmapCell struct {
	kind  int
	label string
}

type GothicMapState struct {
	cells            map[gothicSmapKey]gothicSmapCell
	fallbacks        map[int]string
	routeColors      []int
	Active           GothicMapMode
	Map1Image        string
	Map1Variable     string
	Map2Image        string
	Map2Variable     string
	ClickImage       string
	TimeVariable     string
	MapVariable      string
	ClickPixels      *gfx.BsgImage
	RoutePage        int
	RouteColumn      int
	RouteRowVariable string
	RouteMask        *gfx.BsgImage
}

func NewGothicMapState() *GothicMapState {
	return &GothicMapState{
		cells:     map[gothicSmapKey]gothicSmapCell{},
		fallbacks: map[int]string{},
	}
}

func (m *GothicMapState) RouteColors() []int {
	return m.routeColors
}

func (m *GothicMapState) RouteActive() bool {
	return m.Active == GothicMapRoute
}

func (m *GothicMapState) ClickMapActive() bool {
	return m.Active == GothicMapClickMap
}

func (m *GothicMapState) AddZone(page, row, col, kind int, label string) {
	m.cells[gothicSmapKey{page: page, col: col, row: row}] = gothicSmapCell{kind: kind, label: label}
}

func (m *GothicMapState) AddTarget(slot int, label string) {
	m.fallbacks[slot] = label
}

func (m *GothicMapState) LookupKind(page, col, row int) int {
	if cell, ok := m.cells[gothicSmapKey{page: page, col: col, row: row}]; ok {
		return cell.kind
	}
	return 0
}

func (m *GothicMapState) LookupLabel(page, col, row int) string {
	if cell, ok := m.cells[gothicSmapKey{page: page, col: col, row: row}]; ok {
		return cell.label
	}
	return ""
}

func (m *GothicMapState) FallbackLabel(row int) string {
	if label, ok := m.fallbacks[row]; ok {
		return label
	}
	return ""
}

func (m *GothicMapState) EnterRouteMap(page, col int, rowVariable string, mask *gfx.BsgImage, colors []int) {
	m.Active = GothicMapRoute
	m.RoutePage = page
	m.RouteColumn = col
	m.RouteRowVariable = rowVariable
	m.RouteMask = mask
	m.routeColors = m.routeColors[:0]
	if colors != nil {
		m.routeColors = append(m.routeColors, colors...)
	}
	m.ClickImage = ""
	m.TimeVariable = ""
	m.MapVariable = ""
	m.ClickPixels = nil
}

func (m *GothicMapState) EnterClickMap(image, timeVariable, mapVariable string, pixels *gfx.BsgImage) {
	m.Active = GothicMapClickMap
	m.ClickImage = image
	m.TimeVariable = timeVariable
	m.MapVariable = mapVariable
	m.ClickPixels = pixels
	m.RoutePage = 0
	m.RouteColumn = 0
	m.RouteRowVariable = ""
	m.RouteMask = nil
	m.routeColors = m.routeColors[:0]
}

func (m *GothicMapState) RouteRowAt(x, y int) int {
	color := SampleGothicMapColor(m.RouteMask, x, y)
	if color <= 0 || color == 0xFFFFFF {
		return 0
	}
	for index, candidate := range m.routeColors {
		if candidate == color {
			return index + 1
		}
	}
	return 0
}

func BuildGothicMapColorTable(image *gfx.BsgImage) []int {
	table := []int{}
	if image == nil || image.Pixels == nil {
		return table
	}
	for y := 0; y < image.Height; y++ {
		for x := 0; x < image.Width; x++ {
			offset := ((y * image.Width) + x) * 4
			if offset < 0 || offset+2 >= len(image.Pixels) {
				continue
			}
			color := (int(image.Pixels[offset]) << 16) | (int(image.Pixels[offset+1]) << 8) | int(image.Pixels[offset+2])
			if color == 0 || color == 0xFFFFFF {
				continue
			}
			duplicate := false
			for _, existing := range table {
				if existing == color {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			table = append(table, color)
		}
	}
	return table
}

func SampleGothicMapColor(image *gfx.BsgImage, x, y int) int {
	if image == nil || image.Pixels == nil {
		return -1
	}
	if x < 0 || y < 0 || x >= image.Width || y >= image.Height {
		return -1
	}
	offset := ((y * image.Width) + x) * 4
	if offset < 0 || offset+2 >= len(image.Pixels) {
		return -1
	}
	return (int(image.Pixels[offset]) << 16) | (int(image.Pixels[offset+1]) << 8) | int(image.Pixels[offset+2])
}

func (m *GothicMapState) Leave() {
	m.Active = GothicMapNone
	m.RoutePage = 0
	m.RouteColumn = 0
	m.RouteRowVariable = ""
	m.RouteMask = nil
	m.routeColors = m.routeColors[:0]
	m.ClickImage = ""
	m.TimeVariable = ""
	m.MapVariable = ""
	m.ClickPixels = nil
}

func (m *GothicMapState) Reset() {
	m.cells = map[gothicSmapKey]gothicSmapCell{}
	m.fallbacks = map[int]string{}
	m.Map1Image = ""
	m.Map1Variable = ""
	m.Map2Image = ""
	m.Map2Variable = ""
	m.Leave()
}
