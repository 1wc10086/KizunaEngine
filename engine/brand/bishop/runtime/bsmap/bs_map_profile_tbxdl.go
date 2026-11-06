package bsmap

import (
	"os"
	"path/filepath"

	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

type BsTbxMapTriple struct {
	Property  int
	Value     int
	Operation int
	Invert    bool
}

type BsTbxMapBranch struct {
	Conditions []BsTbxMapTriple
	Label      string
}

type BsTbxMapSection struct {
	HighLight  int
	FirstLabel string
	Branches   []BsTbxMapBranch
}

type BsMapProfileTbxdl struct{}

const (
	bsMapTbxdlButtonCount         = 10
	bsMapTbxdlButtonLeft          = 621
	bsMapTbxdlButtonTop           = 46
	bsMapTbxdlButtonStep          = 34
	bsMapTbxdlButtonWidth         = 153
	bsMapTbxdlButtonHeight        = 33
	bsMapTbxdlBackgroundImage     = "mapbase"
	bsMapTbxdlMenuImage           = "mapmenu"
	bsMapTbxdlSystemMenuImage     = "mapsysmenu"
	bsMapTbxdlMenuLeft            = 621
	bsMapTbxdlMenuTop             = 46
	bsMapTbxdlSystemMenuLeft      = 0
	bsMapTbxdlSystemMenuTop       = 0
	bsMapTbxdlMenuWidth           = 306
	bsMapTbxdlMenuHeight          = 374
	bsMapTbxdlMenuVisibleWidth    = 153
	bsMapTbxdlDecorationHitX      = -4096
	bsMapTbxdlDecorationHitY      = -4096
	BsMapTbxdlSystemParameterFile = "system.prm"
)

const (
	bsMapTbxdlMarker                  = "mapface"
	bsMapTbxdlMarkerOffset            = 8
	bsMapTbxdlMarkerByteCount         = 7
	bsMapTbxdlMinimumFileSize         = 60
	bsMapTbxdlDataPoolOffsetWord      = 13
	bsMapTbxdlDataPoolSizeWord        = 14
	bsMapTbxdlBlockSeparator          = 0x2E
	bsMapTbxdlBlockEndMarker          = 0xFF
	bsMapTbxdlJzOpcodeA               = 0x30
	bsMapTbxdlJzOpcodeB               = 0x31
	bsMapTbxdlJzOpcodeC               = 0x32
	bsMapTbxdlEqualOpcode             = 0x0A
	bsMapTbxdlPushHighLightOpcode     = 0x40
	bsMapTbxdlArithmeticEntryOpcode   = 0x02
	bsMapTbxdlLabelJumpOpcode         = 0xFB
	bsMapTbxdlMinimumGuardLength      = 14
	bsMapTbxdlGuardHighLightIndex     = 4
	bsMapTbxdlGuardEqualIndex         = 6
	bsMapTbxdlGuardJumpIndex          = 7
	bsMapTbxdlGuardEndIndex           = 12
	bsMapTbxdlPushPropertyOpcode      = 0x21
	bsMapTbxdlPushImmediateOpcode     = 0x00
	bsMapTbxdlEqualOperation          = 0x0A
	bsMapTbxdlNotEqualOperation       = 0x0B
	bsMapTbxdlGreaterOperation        = 0x0C
	bsMapTbxdlLessOperation           = 0x0D
	bsMapTbxdlGreaterOrEqualOperation = 0x0E
	bsMapTbxdlLessOrEqualOperation    = 0x0F
	bsMapTbxdlInvertJumpOpcode        = 0x31
	bsMapTbxdlTripleSize              = 11
	bsMapTbxdlDispatchSize            = 1
	bsMapTbxdlTerminatorSize          = 2
)

// bsMapTbxdlMenuRows 是 mapmenu 楼层列表的行数（1F..仕事に戻る），与 high_light 1..11 一一对应。
// bsMapTbxdlButtonCount 仍是 10 个楼层，第 11 行“仕事に戻る”单独由循环覆盖。
const bsMapTbxdlMenuRows = 11

const bsMapTbxdlCancelLabel = "mp_end"

func (p *BsMapProfileTbxdl) Name() string {
	return "Tbxdl"
}

func (p *BsMapProfileTbxdl) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptAncientCompact &&
		context.Resources.Exists(BsMapTbxdlSystemParameterFile)
}

func (p *BsMapProfileTbxdl) Build(context BsMapContext) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	resources := context.Resources
	sections := func() (result map[int]BsTbxMapSection) {
		defer func() {
			if recovered := recover(); recovered != nil {
				core.Logf("script: map tbxdl: parse failed: %s", bsMapRecoverMessage(recovered))
				result = nil
			}
		}()
		return loadBsMapTbxdlSections(resources, context.GameRoot)
	}()
	if len(sections) == 0 {
		return nil
	}
	core.Logf("script: map tbxdl: %d high_light section(s), property conditions evaluated per click with first label as fallback", len(sections))
	items := make([]BsMapItem, 0, bsMapTbxdlMenuRows+2)
	items = addBsMapTbxdlDecoration(items, resources, bsMapTbxdlSystemMenuImage, bsMapTbxdlSystemMenuLeft, bsMapTbxdlSystemMenuTop)
	// mapmenu 不再当作普通装饰图整张画出：它是一张 306x374 的双列图
	// （左列常态、右列悬停）。把几何信息交给 BsMapLayout，舞台只贴左列，
	// 悬停行由 session.drawMapHover 按行补画右列。
	menuShown := resources.Exists(bsMapTbxdlMenuImage)
	if menuShown {
		core.Logf("script: map tbxdl: '%s' 双列列表图 %dx%d，只画 %dpx 常态列", bsMapTbxdlMenuImage, bsMapTbxdlMenuWidth, bsMapTbxdlMenuHeight, bsMapTbxdlMenuVisibleWidth)
	}
	buttons := 0
	cancelLabel := bsMapTbxdlCancelLabel
	// 第 11 行是「仕事に戻る」：它不是一个楼层，但仍然是 high_light 11 的
	// 有效分支，必须生成热区并作为右键取消的目标，否则玩家只能点右上角退出。
	for index := 0; index < bsMapTbxdlMenuRows; index++ {
		highLight := index + 1
		section, ok := sections[highLight]
		if !ok || len(section.FirstLabel) == 0 {
			core.Logf("script: map tbxdl: high_light %d has no candidate label in system.prm, skipping", highLight)
			continue
		}
		label := section.FirstLabel
		if context.Script.FindLabel(label) < 0 {
			core.Logf("script: map tbxdl: label '%s' missing in script, skipping high_light %d", label, highLight)
			continue
		}
		item := NewBsMapItem(
			"", 0, -1, -1,
			bsMapTbxdlButtonLeft,
			bsMapTbxdlButtonTop+(bsMapTbxdlButtonStep*index),
			bsMapTbxdlButtonWidth,
			bsMapTbxdlButtonHeight,
			label)
		item.Value = highLight
		item.Branches = section.Branches
		items = append(items, item)
		if highLight == bsMapTbxdlMenuRows {
			cancelLabel = label
		}
		buttons++
	}
	if buttons == 0 {
		return nil
	}
	background := ""
	if resources.Exists(bsMapTbxdlBackgroundImage) {
		background = bsMapTbxdlBackgroundImage
	}
	layout := &BsMapLayout{
		Background:  background,
		Items:       items,
		Source:      BsMapTbxdlSystemParameterFile,
		CancelLabel: cancelLabel,
	}
	if menuShown {
		layout.MenuImage = bsMapTbxdlMenuImage
		layout.MenuX = bsMapTbxdlMenuLeft
		layout.MenuY = bsMapTbxdlMenuTop
		layout.MenuRowHeight = bsMapTbxdlButtonStep
		layout.MenuFrameWidth = bsMapTbxdlMenuVisibleWidth
		layout.MenuFrames = bsMapTbxdlMenuRows
	}
	return layout
}

func addBsMapTbxdlDecoration(items []BsMapItem, resources *resource.GameResourceIndex, image string, x, y int) []BsMapItem {
	_, out := addBsMapTbxdlDecorationShown(items, resources, image, x, y)
	return out
}

func addBsMapTbxdlDecorationShown(items []BsMapItem, resources *resource.GameResourceIndex, image string, x, y int) (bool, []BsMapItem) {
	if !resources.Exists(image) {
		core.Logf("script: map tbxdl: 装饰图 '%s' 不存在，跳过", image)
		return false, items
	}
	items = append(items, NewBsMapItemDecor(image, x, y, bsMapTbxdlDecorationHitX, bsMapTbxdlDecorationHitY))
	return true, items
}

func SelectBsMapTbxdlBranch(item BsMapItem, readProperty func(int) int) string {
	if len(item.Branches) > 0 {
		for _, branch := range item.Branches {
			taken := true
			for _, triple := range branch.Conditions {
				if !testBsMapTbxdlTriple(triple, readProperty(triple.Property)) {
					taken = false
					break
				}
			}
			if taken && len(branch.Label) > 0 {
				return branch.Label
			}
		}
	}
	return item.Label
}

func testBsMapTbxdlTriple(triple BsTbxMapTriple, actual int) bool {
	var passed bool
	switch triple.Operation {
	case bsMapTbxdlEqualOperation:
		passed = actual == triple.Value
	case bsMapTbxdlNotEqualOperation:
		passed = actual != triple.Value
	case bsMapTbxdlGreaterOperation:
		passed = actual > triple.Value
	case bsMapTbxdlLessOperation:
		passed = actual < triple.Value
	case bsMapTbxdlGreaterOrEqualOperation:
		passed = actual >= triple.Value
	case bsMapTbxdlLessOrEqualOperation:
		passed = actual <= triple.Value
	default:
		passed = false
	}
	if triple.Invert {
		return !passed
	}
	return passed
}

// loadBsMapTbxdlSections 优先从资源索引读 system.prm，其次才在 gameRoot 下找。
//
// 旧版把 system.prm 放在 Disk2，dataDir 却是 Disk1 的脚本目录；只在
// gameRoot 下拼路径会读不到文件，地图候选为空，system call "map" 被当成
// "没有可去的地方"继续执行。资源索引已经收录了所有 Disk 根，所以先走
// TryGet，再退回递归查找。
// findBsMapTbxdlParameterFile 在 gameRoot 下按层数上限查找 system.prm。
// 只用于资源索引拿不到文件的兜底路径（例如没有 .Inx/.bsa 的散装安装）。
func findBsMapTbxdlParameterFile(root string, maxDepth int) string {
	if len(root) == 0 {
		return ""
	}
	queue := []struct {
		directory string
		depth     int
	}{{root, 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		path := filepath.Join(current.directory, "system.prm")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
		if current.depth >= maxDepth {
			continue
		}
		children, err := os.ReadDir(current.directory)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() {
				queue = append(queue, struct {
					directory string
					depth     int
				}{filepath.Join(current.directory, child.Name()), current.depth + 1})
			}
		}
	}
	return ""
}

func loadBsMapTbxdlSections(resources *resource.GameResourceIndex, gameRoot string) map[int]BsTbxMapSection {
	var data []byte
	if resources != nil {
		if location, ok := resources.TryGet(BsMapTbxdlSystemParameterFile); ok && location != nil {
			data = location.Read()
		}
	}
	if len(data) == 0 {
		path := findBsMapTbxdlParameterFile(gameRoot, 6)
		if path == "" {
			core.Logf("script: map tbxdl: '%s' not found under '%s'", BsMapTbxdlSystemParameterFile, gameRoot)
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			core.Logf("script: map tbxdl: read '%s' failed: %s", path, err.Error())
			return nil
		}
		data = raw
	}
	if len(data) < bsMapTbxdlMinimumFileSize {
		core.Logf("script: map tbxdl: '%s' truncated (%d bytes, need %d)", BsMapTbxdlSystemParameterFile, len(data), bsMapTbxdlMinimumFileSize)
		return nil
	}
	if !hasBsMapTbxdlMarker(data) {
		core.Logf("script: map tbxdl: '%s' marker '%s' missing at offset %d", BsMapTbxdlSystemParameterFile, bsMapTbxdlMarker, bsMapTbxdlMarkerOffset)
		return nil
	}
	poolOffset := int(binutil.U32(data[bsMapTbxdlDataPoolOffsetWord*4:]))
	poolSize := int(binutil.U32(data[bsMapTbxdlDataPoolSizeWord*4:]))
	if poolOffset < 0 || poolSize <= 0 || int64(poolOffset)+int64(poolSize) > int64(len(data)) {
		core.Logf("script: map tbxdl: data pool invalid (off=%d, size=%d, file=%d)", poolOffset, poolSize, len(data))
		return nil
	}
	pool := data[poolOffset : poolOffset+poolSize]
	if len(pool) < 1 || pool[0] != bsMapTbxdlBlockSeparator {
		core.Logf("script: map tbxdl: data pool first byte 0x%02X (expected 0x%02X)", pool[0], bsMapTbxdlBlockSeparator)
		return nil
	}
	sections := scanBsMapTbxdlSections(pool)
	if len(sections) == 0 {
		core.Logf("script: map tbxdl: no high_light guards in data pool")
		return nil
	}
	table := make(map[int]BsTbxMapSection, len(sections))
	for _, section := range sections {
		if section.HighLight < 1 || section.HighLight > bsMapTbxdlButtonCount+1 {
			continue
		}
		if existing, duplicate := table[section.HighLight]; duplicate {
			core.Logf("script: map tbxdl: duplicate section for high_light %d, kept '%s'", section.HighLight, existing.FirstLabel)
			continue
		}
		table[section.HighLight] = section
	}
	return table
}

func hasBsMapTbxdlMarker(data []byte) bool {
	marker := []byte(bsMapTbxdlMarker)
	if len(data) < bsMapTbxdlMarkerOffset+len(marker) {
		return false
	}
	for i := range marker {
		if data[bsMapTbxdlMarkerOffset+i] != marker[i] {
			return false
		}
	}
	return true
}

func scanBsMapTbxdlSections(pool []byte) []BsTbxMapSection {
	sections := []BsTbxMapSection{}
	branches := []BsTbxMapBranch{}
	current := 0
	first := ""
	index := 0
	for index < len(pool) {
		if pool[index] != bsMapTbxdlBlockSeparator {
			index++
			continue
		}
		blockStart := index
		index++
		for index < len(pool) && pool[index] != bsMapTbxdlBlockSeparator {
			index++
		}
		body := pool[blockStart+1 : index]
		if len(body) == 0 {
			continue
		}
		if body[0] == bsMapTbxdlArithmeticEntryOpcode {
			highLight, ok := tryParseBsMapTbxdlGuard(body)
			if ok {
				if current > 0 && len(first) > 0 {
					sections = append(sections, BsTbxMapSection{HighLight: current, FirstLabel: first, Branches: append([]BsTbxMapBranch(nil), branches...)})
				}
				current = highLight
				first = ""
				branches = []BsTbxMapBranch{}
				continue
			}
			if current <= 0 {
				continue
			}
			if triples, ok := tryParseBsMapTbxdlConditions(body); ok {
				branches = append(branches, BsTbxMapBranch{Conditions: triples, Label: ""})
			}
			continue
		}
		if body[0] == bsMapTbxdlLabelJumpOpcode && current > 0 {
			label, ok := readBsMapTbxdlLabel(body)
			if !ok {
				continue
			}
			if len(first) == 0 {
				first = label
			}
			if len(branches) > 0 && len(branches[len(branches)-1].Label) == 0 {
				branches[len(branches)-1].Label = label
			} else {
				branches = append(branches, BsTbxMapBranch{Conditions: []BsTbxMapTriple{}, Label: label})
			}
		}
	}
	if current > 0 && len(first) > 0 {
		sections = append(sections, BsTbxMapSection{HighLight: current, FirstLabel: first, Branches: append([]BsTbxMapBranch(nil), branches...)})
	}
	return sections
}

func tryParseBsMapTbxdlGuard(body []byte) (int, bool) {
	if len(body) < bsMapTbxdlMinimumGuardLength {
		return 0, false
	}
	if body[0] != bsMapTbxdlArithmeticEntryOpcode || body[1] != bsMapTbxdlPushHighLightOpcode || body[2] != 0x00 {
		return 0, false
	}
	if body[3] != bsMapTbxdlPushImmediateOpcode || body[bsMapTbxdlGuardEqualIndex] != bsMapTbxdlEqualOpcode {
		return 0, false
	}
	if body[bsMapTbxdlGuardJumpIndex] != bsMapTbxdlJzOpcodeA &&
		body[bsMapTbxdlGuardJumpIndex] != bsMapTbxdlJzOpcodeB &&
		body[bsMapTbxdlGuardJumpIndex] != bsMapTbxdlJzOpcodeC {
		return 0, false
	}
	if body[bsMapTbxdlGuardEndIndex] != bsMapTbxdlBlockEndMarker || body[bsMapTbxdlGuardEndIndex+1] != bsMapTbxdlBlockEndMarker {
		return 0, false
	}
	value := int(body[bsMapTbxdlGuardHighLightIndex])
	if value < 1 || value > bsMapTbxdlButtonCount+1 {
		return 0, false
	}
	return value, true
}

func tryParseBsMapTbxdlConditions(body []byte) ([]BsTbxMapTriple, bool) {
	payload := len(body) - bsMapTbxdlDispatchSize - bsMapTbxdlTerminatorSize
	if payload <= 0 || payload%bsMapTbxdlTripleSize != 0 {
		return nil, false
	}
	if body[len(body)-2] != bsMapTbxdlBlockEndMarker || body[len(body)-1] != bsMapTbxdlBlockEndMarker {
		return nil, false
	}
	count := payload / bsMapTbxdlTripleSize
	parsed := make([]BsTbxMapTriple, count)
	for slot := 0; slot < count; slot++ {
		triple := body[bsMapTbxdlDispatchSize+(slot*bsMapTbxdlTripleSize):]
		if triple[0] != bsMapTbxdlPushPropertyOpcode || triple[2] != bsMapTbxdlPushImmediateOpcode {
			return nil, false
		}
		operation := triple[5]
		if operation != bsMapTbxdlEqualOperation && operation != bsMapTbxdlNotEqualOperation &&
			operation != bsMapTbxdlGreaterOperation && operation != bsMapTbxdlLessOperation &&
			operation != bsMapTbxdlGreaterOrEqualOperation && operation != bsMapTbxdlLessOrEqualOperation {
			return nil, false
		}
		if triple[6] != bsMapTbxdlJzOpcodeA && triple[6] != bsMapTbxdlJzOpcodeB && triple[6] != bsMapTbxdlJzOpcodeC {
			return nil, false
		}
		parsed[slot] = BsTbxMapTriple{
			Property:  int(triple[1]),
			Value:     int(triple[3]) | (int(triple[4]) << 8),
			Operation: int(triple[5]),
			Invert:    triple[6] == bsMapTbxdlInvertJumpOpcode,
		}
	}
	return parsed, true
}

func readBsMapTbxdlLabel(body []byte) (string, bool) {
	rest := body[1:]
	nul := -1
	for i, code := range rest {
		if code == 0 {
			nul = i
			break
		}
	}
	if nul <= 0 || len(body) < nul+4 {
		return "", false
	}
	raw := body[1 : 1+nul]
	for _, code := range raw {
		if code < 0x20 || code > 0x7E {
			return "", false
		}
	}
	if body[nul+2] != bsMapTbxdlBlockEndMarker || body[nul+3] != bsMapTbxdlBlockEndMarker {
		return "", false
	}
	return string(raw), true
}
