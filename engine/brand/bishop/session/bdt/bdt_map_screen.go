package bdt

import (
	"errors"

	"kizuna/engine/brand/bishop/resource"
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	BdtMapHotspotCount = 9
	BdtMapScreenWidth  = 640
	BdtMapScreenHeight = 480
	BdtMapButtonSize   = 48
)

const bdtMapClauseLength = 11

const maxInt32 = int(^uint32(0) >> 1)

type BdtMapHotspot struct {
	ID     int
	X      int
	Y      int
	Width  int
	Height int
}

type bdtMapClause struct {
	Var   int
	Value int
}

type bdtMapEntry struct {
	Clauses []bdtMapClause
	Label   string
}

type BdtMapScreen struct {
	lead       []*bdtMapEntry
	groups     map[int][]*bdtMapEntry
	anchors    [][2]int
	mask       []byte
	maskWidth  int
	maskHeight int
	active     bool
}

var bdtMapFallbackAnchors = [][2]int{
	{324, 139},
	{329, 306},
	{329, 265},
	{329, 215},
	{152, 195},
	{152, 151},
	{460, 250},
	{222, 312},
	{460, 96},
}

func NewBdtMapScreen() *BdtMapScreen {
	return &BdtMapScreen{groups: map[int][]*bdtMapEntry{}}
}

func (m *BdtMapScreen) Active() bool {
	return m.active
}

func (m *BdtMapScreen) HasTable() bool {
	return len(m.lead) > 0 || len(m.groups) > 0
}

func (m *BdtMapScreen) HasMask() bool {
	return len(m.mask) > 0
}

func (m *BdtMapScreen) MaskWidth() int {
	if !m.HasMask() {
		return 0
	}
	return m.maskWidth
}

func (m *BdtMapScreen) MaskHeight() int {
	if !m.HasMask() {
		return 0
	}
	return m.maskHeight
}

func (m *BdtMapScreen) Activate(resources *resource.GameResourceIndex) bool {
	m.active = true
	m.loadTable(resources)
	m.loadMask(resources)
	return m.HasTable()
}

func (m *BdtMapScreen) Deactivate() {
	m.active = false
}

func (m *BdtMapScreen) HotspotAt(x, y int) int {
	if len(m.mask) == 0 || x < 0 || y < 0 || x >= m.maskWidth || y >= m.maskHeight {
		return 0
	}
	return int(m.mask[y*m.maskWidth+x])
}

func (m *BdtMapScreen) ResolveHotspot(hotspot int, variables []int) string {
	for _, entry := range m.lead {
		if matchBdtMapEntry(entry, variables) {
			return entry.Label
		}
	}
	if list, ok := m.groups[hotspot]; ok {
		for _, entry := range list {
			if matchBdtMapEntry(entry, variables) {
				return entry.Label
			}
		}
	}
	return ""
}

func (m *BdtMapScreen) Hotspots() []BdtMapHotspot {
	list := make([]BdtMapHotspot, 0, BdtMapHotspotCount)
	for id := 1; id <= BdtMapHotspotCount; id++ {
		anchor := bdtMapFallbackAnchors[id-1]
		if id-1 < len(m.anchors) {
			anchor = m.anchors[id-1]
		}
		x := anchor[0] - BdtMapButtonSize/2
		if x < 0 {
			x = 0
		}
		if x > BdtMapScreenWidth-BdtMapButtonSize {
			x = BdtMapScreenWidth - BdtMapButtonSize
		}
		y := anchor[1] - BdtMapButtonSize/2
		if y < 0 {
			y = 0
		}
		if y > BdtMapScreenHeight-BdtMapButtonSize {
			y = BdtMapScreenHeight - BdtMapButtonSize
		}
		list = append(list, BdtMapHotspot{ID: id, X: x, Y: y, Width: BdtMapButtonSize, Height: BdtMapButtonSize})
	}
	return list
}

func matchBdtMapEntry(entry *bdtMapEntry, variables []int) bool {
	if len(entry.Clauses) == 0 {
		return true
	}
	if variables == nil {
		return false
	}
	states := map[int]bool{}
	for _, clause := range entry.Clauses {
		hit := clause.Var >= 0 && clause.Var < len(variables) && variables[clause.Var] == clause.Value
		if taken, ok := states[clause.Var]; ok {
			states[clause.Var] = taken || hit
		} else {
			states[clause.Var] = hit
		}
	}
	for _, taken := range states {
		if !taken {
			return false
		}
	}
	return true
}

func (m *BdtMapScreen) loadTable(resources *resource.GameResourceIndex) {
	if m.HasTable() {
		return
	}
	if resources == nil {
		core.Logf("bdt: %s", "map table missing: system.prm not found")
		return
	}
	location, ok := resources.TryGet("system.prm")
	if !ok {
		location, ok = resources.TryGet("system")
	}
	if !ok {
		core.Logf("bdt: %s", "map table missing: system.prm not found")
		return
	}
	data := location.Read()
	if err := m.parseTable(data); err != nil {
		core.Logf("bdt: map table parse failed: %s", err.Error())
		m.lead = nil
		m.groups = map[int][]*bdtMapEntry{}
		m.anchors = nil
	}
}

func (m *BdtMapScreen) parseTable(data []byte) error {
	blobOffset := 22870
	blobLength := 3984
	if len(data) >= 132 {
		count0, err := readBdtU32(data, 0)
		if err != nil {
			return err
		}
		count1, err := readBdtU32(data, 4)
		if err != nil {
			return err
		}
		named := len(data) >= 16 && data[8] == 'm' && data[9] == 'a' && data[10] == 'p'
		fileBlobOffset, err := readBdtU32(data, 52)
		if err != nil {
			return err
		}
		fileBlobLength, err := readBdtU32(data, 56)
		if err != nil {
			return err
		}
		if count0 == 9 && count1 == 21 && named && fileBlobOffset > 0 && fileBlobLength > 0 &&
			fileBlobLength <= len(data) && fileBlobOffset <= len(data)-fileBlobLength {
			blobOffset = fileBlobOffset
			blobLength = fileBlobLength
		}
		m.anchors = nil
		anchorsValid := true
		for i := 0; i < BdtMapHotspotCount; i++ {
			x, err := readBdtU32(data, 60+i*8)
			if err != nil {
				return err
			}
			y, err := readBdtU32(data, 64+i*8)
			if err != nil {
				return err
			}
			if x < 0 || x >= BdtMapScreenWidth || y < 0 || y >= BdtMapScreenHeight {
				anchorsValid = false
			}
			m.anchors = append(m.anchors, [2]int{x, y})
		}
		if !anchorsValid {
			m.anchors = nil
			core.Logf("bdt: %s", "map anchors out of range, using fallback")
		}
	}
	if blobOffset+blobLength > len(data) {
		return errors.New("map table blob out of range")
	}
	blob := make([]byte, blobLength)
	copy(blob, data[blobOffset:blobOffset+blobLength])
	records := splitBdtMapRecords(blob)
	currentGroup := 0
	var pending []bdtMapClause
	for _, record := range records {
		if len(record) < 2 || record[0] != 0x2E {
			return errors.New("map table record sync lost")
		}
		if record[1] == 0x02 {
			body := record[2:]
			if len(body) == bdtMapClauseLength && body[0] == 0x40 {
				hotspot := int(body[3])
				if hotspot < 1 || hotspot > BdtMapHotspotCount {
					core.Logf("bdt: map group hotspot out of range: %d", hotspot)
					currentGroup = 0
				} else {
					if _, ok := m.groups[hotspot]; !ok {
						m.groups[hotspot] = []*bdtMapEntry{}
					}
					currentGroup = hotspot
				}
				pending = nil
			} else if clauses, ok := tryParseBdtMapCondition(body); ok {
				pending = clauses
			} else {
				pending = nil
			}
		} else if record[1] == 0xFB {
			label := parseBdtMapLabel(record)
			if len(label) > 0 {
				entry := &bdtMapEntry{Clauses: pendingOrEmpty(pending), Label: label}
				if currentGroup == 0 {
					m.lead = append(m.lead, entry)
				} else {
					m.groups[currentGroup] = append(m.groups[currentGroup], entry)
				}
			}
			pending = nil
		} else {
			pending = nil
		}
	}
	core.Logf("bdt: map table groups=%d lead=%d", len(m.groups), len(m.lead))
	return nil
}

func pendingOrEmpty(pending []bdtMapClause) []bdtMapClause {
	if pending == nil {
		return []bdtMapClause{}
	}
	return pending
}

func tryParseBdtMapCondition(body []byte) ([]bdtMapClause, bool) {
	clauses := []bdtMapClause{}
	if len(body) == 0 || len(body)%bdtMapClauseLength != 0 {
		return clauses, false
	}
	for i := 0; i < len(body); i += bdtMapClauseLength {
		if body[i] != 0x21 || body[i+8] != 0 || body[i+9] != 0 || body[i+10] != 0 {
			return []bdtMapClause{}, false
		}
		variable := int(body[i+1])
		if body[i+2] != 0 || variable < 0 || variable >= bdtvm.BdtXCount {
			return []bdtMapClause{}, false
		}
		clauses = append(clauses, bdtMapClause{Var: variable, Value: int(body[i+3])})
	}
	if len(clauses) == 0 {
		return []bdtMapClause{}, false
	}
	return clauses, true
}

func parseBdtMapLabel(record []byte) string {
	for i := 2; i < len(record); i++ {
		value := record[i]
		if value == 0 {
			if i == 2 {
				return ""
			}
			return string(record[2:i])
		}
		if value >= 0x80 || value == '$' {
			return ""
		}
	}
	return ""
}

func splitBdtMapRecords(blob []byte) [][]byte {
	records := [][]byte{}
	start := 0
	i := 0
	for i+1 < len(blob) {
		if blob[i] == 0xFF && blob[i+1] == 0xFF {
			if i > start {
				record := make([]byte, i-start)
				copy(record, blob[start:i])
				records = append(records, record)
			}
			i += 2
			start = i
		} else {
			i++
		}
	}
	if start < len(blob) {
		record := make([]byte, len(blob)-start)
		copy(record, blob[start:])
		records = append(records, record)
	}
	return records
}

func (m *BdtMapScreen) loadMask(resources *resource.GameResourceIndex) {
	if m.HasMask() {
		return
	}
	if resources == nil {
		core.Logf("bdt: %s", "map mask missing: s_map3 not found")
		return
	}
	location, ok := resources.TryGet("s_map3")
	if !ok {
		location, ok = resources.TryGet("s_map3.bmp")
	}
	if !ok {
		core.Logf("bdt: %s", "map mask missing: s_map3 not found")
		return
	}
	data := location.Read()
	if err := m.parseMask(data); err != nil {
		core.Logf("bdt: map mask parse failed: %s", err.Error())
		m.mask = nil
		m.maskWidth = 0
		m.maskHeight = 0
	}
}

func (m *BdtMapScreen) parseMask(data []byte) error {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return errors.New("map mask not a bmp")
	}
	pixelOffset, err := readBdtU32(data, 10)
	if err != nil {
		return err
	}
	width, err := readBdtI32(data, 18)
	if err != nil {
		return err
	}
	height, err := readBdtI32(data, 22)
	if err != nil {
		return err
	}
	if len(data) < 30 {
		return errors.New("map mask not a bmp")
	}
	bits := int(data[28]) | int(data[29])<<8
	compression, err := readBdtU32(data, 30)
	if err != nil {
		return err
	}
	absHeight := height
	if absHeight < 0 {
		absHeight = -absHeight
	}
	if width <= 0 || width > BdtMapScreenWidth*2 || height == 0 || absHeight > BdtMapScreenHeight*2 {
		return errors.New("map mask dimensions out of range")
	}
	if pixelOffset < 0 || pixelOffset > len(data) {
		return errors.New("map mask pixels out of range")
	}
	if compression != 0 {
		return errors.New("map mask compressed")
	}
	topDown := height < 0
	mask := make([]byte, width*absHeight)
	if bits == 8 {
		stride := (width + 3) &^ 3
		for row := 0; row < absHeight; row++ {
			sourceRow := row
			if !topDown {
				sourceRow = absHeight - 1 - row
			}
			source := pixelOffset + sourceRow*stride
			if source+width > len(data) {
				return errors.New("map mask pixels out of range")
			}
			copy(mask[row*width:], data[source:source+width])
		}
	} else if bits == 24 {
		stride := (width*3 + 3) &^ 3
		colors := map[int]byte{}
		var next byte = 1
		for row := 0; row < absHeight; row++ {
			sourceRow := row
			if !topDown {
				sourceRow = absHeight - 1 - row
			}
			source := pixelOffset + sourceRow*stride
			if source+width*3 > len(data) {
				return errors.New("map mask pixels out of range")
			}
			for x := 0; x < width; x++ {
				color := int(data[source+x*3]) | int(data[source+x*3+1])<<8 | int(data[source+x*3+2])<<16
				var id byte
				if color == 0 {
					id = 0
				} else if existing, ok := colors[color]; ok {
					id = existing
				} else {
					id = next
					colors[color] = id
					if next < 255 {
						next++
					}
				}
				mask[row*width+x] = id
			}
		}
	} else {
		return errors.New("map mask bpp unsupported")
	}
	m.mask = mask
	m.maskWidth = width
	m.maskHeight = absHeight
	core.Logf("bdt: map mask %dx%d bpp=%d", width, absHeight, bits)
	return nil
}

func readBdtU32(data []byte, offset int) (int, error) {
	if offset < 0 || offset+4 > len(data) {
		return 0, errors.New("map table read out of range")
	}
	value := int(binutil.U32(data[offset:]))
	if value < 0 {
		return maxInt32, nil
	}
	return value, nil
}

func readBdtI32(data []byte, offset int) (int, error) {
	if offset < 0 || offset+4 > len(data) {
		return 0, errors.New("map table read out of range")
	}
	return int(binutil.I32(data[offset:])), nil
}
