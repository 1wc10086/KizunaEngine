package bsmap

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

type BsMapProfileSchoolHeaven struct{}

const (
	BsMapSchoolHeavenAnchorImage        = "map_sakura"
	BsMapSchoolHeavenBackgroundImage    = "map_base"
	BsMapSchoolHeavenCharacterProperty  = "#character"
	BsMapSchoolHeavenPlaceProperty      = "#place"
	BsMapSchoolHeavenMapTypeProperty    = "#map_type"
	BsMapSchoolHeavenTimeProperty       = "#time"
	BsMapSchoolHeavenSource             = "schoolheaven-rev3"
	BsMapSchoolHeavenStripFrameCount    = 3
	BsMapSchoolHeavenCharacterWidth     = 78
	BsMapSchoolHeavenCharacterHeight    = 240
	BsMapSchoolHeavenCharacterHitHeight = BsMapSchoolHeavenCharacterHeight / BsMapSchoolHeavenStripFrameCount
	BsMapSchoolHeavenButtonWidth        = 48
	BsMapSchoolHeavenButtonHeight       = 144
	BsMapSchoolHeavenButtonHitHeight    = BsMapSchoolHeavenButtonHeight / BsMapSchoolHeavenStripFrameCount
)

const (
	bsMapSchoolHeavenCharacterCount        = 8
	bsMapSchoolHeavenMaskCount             = 9
	bsMapSchoolHeavenTableStride           = 6
	bsMapSchoolHeavenTableBaseOffset       = 6
	bsMapSchoolHeavenStaticMapTypeMaximum  = 4
	bsMapSchoolHeavenDynamicTimeMinimum    = 1
	bsMapSchoolHeavenDynamicTimeMaximum    = 4
	bsMapSchoolHeavenUnresolvedPropertyKey = -1
	bsMapSchoolHeavenUnknownValue          = -1
)

var bsMapSchoolHeavenCharacters = []string{
	"map_sakura",
	"map_aoi",
	"map_erika",
	"map_lemon",
	"map_lime",
	"map_mutsumi",
	"map_kyouko",
	"map_maria",
}

var bsMapSchoolHeavenAvailabilityProperties = []string{
	"#Skr_map",
	"#Aoi_map",
	"#Erk_map",
	"#Lmn_map",
	"#Lim_map",
	"#Mtm_map",
	"#Kyk_map",
	"#Mra_map",
}

var bsMapSchoolHeavenProgressProperties = []string{
	"#Skr_progres",
	"#Aoi_progres",
	"#Erk_progres",
	"#Lmn_progres",
	"#Lim_progres",
	"#Mtm_progres",
	"#Kyk_progres",
	"#Mra_progres",
}

type bsMapSchoolHeavenSystemButton struct {
	image string
	x     int
	y     int
}

var bsMapSchoolHeavenSystemButtons = []bsMapSchoolHeavenSystemButton{
	{"map_config", 189, 499},
	{"map_log", 249, 499},
	{"map_status", 309, 499},
	{"map_qsave", 159, 539},
	{"map_qload", 219, 539},
	{"map_save", 279, 539},
	{"map_load", 339, 539},
}

type bsMapSchoolHeavenSlot struct {
	x int
	y int
}

var bsMapSchoolHeavenSlotTable = []bsMapSchoolHeavenSlot{
	{682, 466}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{680, 400}, {680, 514}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{642, 466}, {714, 400}, {713, 514}, {0, 0}, {0, 0}, {0, 0},
	{630, 390}, {714, 400}, {642, 466}, {713, 514}, {0, 0}, {0, 0},
	{630, 390}, {714, 400}, {642, 466}, {595, 523}, {713, 514}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{320, 274}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{274, 239}, {350, 284}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{284, 213}, {273, 296}, {358, 290}, {0, 0}, {0, 0}, {0, 0},
	{284, 213}, {365, 248}, {273, 296}, {356, 330}, {0, 0}, {0, 0},
	{284, 213}, {365, 248}, {200, 245}, {273, 296}, {356, 330}, {0, 0},
	{201, 204}, {284, 213}, {365, 248}, {189, 283}, {273, 296}, {356, 330},
	{530, 273}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{522, 229}, {528, 313}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{522, 229}, {447, 305}, {528, 313}, {0, 0}, {0, 0}, {0, 0},
	{447, 265}, {522, 229}, {447, 353}, {528, 313}, {0, 0}, {0, 0},
	{447, 265}, {522, 229}, {595, 185}, {487, 330}, {568, 290}, {0, 0},
	{447, 265}, {522, 229}, {595, 185}, {447, 353}, {528, 313}, {605, 281},
	{402, 153}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{362, 148}, {447, 150}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{322, 148}, {402, 153}, {487, 150}, {0, 0}, {0, 0}, {0, 0},
	{272, 141}, {352, 146}, {437, 143}, {520, 140}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{178, 104}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{198, 90}, {118, 120}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{60, 406}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{54, 345}, {60, 426}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{292, 43}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{452, 62}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{420, 62}, {500, 70}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{492, 500}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{440, 445}, {482, 520}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{440, 445}, {482, 520}, {520, 457}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
	{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0},
}

type bsMapSchoolHeavenRowEntry struct {
	threshold int
	time      int
	row       int
}

var bsMapSchoolHeavenDynamicRows = [][]bsMapSchoolHeavenRowEntry{
	{{5, 3, 2}, {10, 1, 0}, {15, 4, 8}, {20, 4, 0}, {25, 3, 3}, {35, 4, 1}, {40, 3, 2}, {45, 2, 0}, {55, 2, 3}, {60, 4, 3}, {65, 1, 1}, {75, 4, 8}},
	{{5, 2, 1}, {10, 3, 2}, {15, 3, 1}, {20, 1, 6}, {25, 2, 7}, {35, 4, 6}, {40, 3, 2}, {45, 2, 7}, {55, 4, 7}, {60, 1, 1}, {65, 2, 7}, {75, 3, 2}},
	{{5, 4, 4}, {10, 1, 8}, {15, 2, 2}, {20, 2, 4}, {25, 1, 1}, {35, 3, 7}, {40, 3, 1}, {45, 3, 0}, {55, 4, 4}, {60, 4, 8}, {65, 3, 4}, {75, 2, 0}},
	{{5, 1, 0}, {10, 4, 5}, {15, 2, 1}, {20, 1, 5}, {25, 3, 2}, {30, 3, 1}, {35, 1, 8}, {40, 3, 5}, {50, 4, 0}, {55, 1, 5}, {60, 3, 5}, {70, 3, 5}},
	{{5, 4, 0}, {10, 3, 1}, {15, 4, 1}, {20, 4, 1}, {25, 3, 2}, {30, 1, 2}, {35, 2, 3}, {40, 1, 5}, {50, 3, 5}, {55, 4, 1}, {60, 2, 2}, {70, 3, 2}},
	{{5, 3, 2}, {10, 1, 0}, {15, 2, 2}, {20, 4, 1}, {25, 2, 1}, {30, 2, 1}, {40, 4, 2}, {45, 4, 1}, {50, 3, 1}, {55, 3, 1}},
	{{5, 3, 2}, {10, 1, 2}, {15, 4, 2}, {20, 2, 3}, {30, 1, 0}},
	{{5, 2, 0}, {10, 2, 0}, {15, 4, 0}, {20, 2, 0}, {30, 1, 0}},
}

type bsMapSchoolHeavenBandEntry struct {
	threshold int
	row1      int
	row2      int
	row3      int
	row4      int
}

var bsMapSchoolHeavenDynamicBands = [][]bsMapSchoolHeavenBandEntry{
	{{20, 1, 3, 1, 8}, {30, 1, 4, 3, 1}, {55, 8, 1, 1, 8}, {70, 1, 3, 1, 8}, {-1, 8, 1, 1, 8}},
	{{20, 6, 1, 2, 6}, {30, 6, 3, 6, 6}, {55, 1, 6, 7, 6}, {70, 6, 1, 6, 1}, {-1, 1, 3, 6, 6}},
	{{20, 8, 4, 4, 1}, {30, 4, 1, 4, 8}, {55, 4, 2, 4, 1}, {70, 8, 4, 4, 2}, {-1, 4, 2, 4, 8}},
	{{30, 5, 1, 5, 5}, {45, 1, 1, 5, 5}, {70, 1, 2, 5, 1}, {-1, 5, 1, 5, 5}},
	{{30, 1, 2, 2, 1}, {45, 2, 2, 1, 1}, {70, 2, 1, 2, 2}, {-1, 2, 2, 1, 1}},
	{{10, 2, 1, 1, 1}, {20, 1, 1, 1, 2}, {40, 1, 1, 2, 2}, {55, 2, 1, 2, 1}, {-1, 2, 1, 1, 2}},
	{{0, 0, 0, 0, 0}, {10, 2, 2, 3, 2}, {30, 2, 2, 2, 2}, {-1, 2, 2, 3, 2}},
	{{10, 0, 0, 0, 0}, {30, 0, 0, 0, 0}, {-1, 0, 0, 0, 0}},
}

func (p *BsMapProfileSchoolHeaven) Name() string {
	return "SchoolHeaven"
}

func (p *BsMapProfileSchoolHeaven) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptRev3 &&
		context.Resources.Exists(BsMapSchoolHeavenAnchorImage) &&
		context.Resources.Exists(BsMapSchoolHeavenBackgroundImage)
}

func (p *BsMapProfileSchoolHeaven) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map schoolheaven: 构建失败：%s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapSchoolHeavenLayout(context)
}

func createBsMapSchoolHeavenLayout(context BsMapContext) *BsMapLayout {
	if !context.Resources.Exists(BsMapSchoolHeavenBackgroundImage) {
		return nil
	}
	rawMapType := context.Properties.ByName(BsMapSchoolHeavenMapTypeProperty)
	mapType := rawMapType
	if rawMapType == bsMapSchoolHeavenUnknownValue {
		mapType = 0
	}
	if rawMapType == bsMapSchoolHeavenUnknownValue {
		core.Logf("script: map schoolheaven: 读不到 %s，按 0 处理", BsMapSchoolHeavenMapTypeProperty)
	}
	masks := buildBsMapSchoolHeavenMasks(context, mapType)
	items := make([]BsMapItem, 0, bsMapSchoolHeavenCharacterCount)
	placed := make([]string, 0, bsMapSchoolHeavenCharacterCount)
	skipped := make([]string, 0, bsMapSchoolHeavenCharacterCount)
	for character := 0; character < bsMapSchoolHeavenCharacterCount; character++ {
		image := bsMapSchoolHeavenCharacters[character]
		characterValue := character + 1
		if !context.Resources.Exists(image) {
			core.Logf("script: map schoolheaven: 角色图 '%s' 缺失，%s = %d 无法放置", image, BsMapSchoolHeavenCharacterProperty, characterValue)
			continue
		}
		place := bsMapSchoolHeavenPlaceOf(masks, character)
		var slot *bsMapSchoolHeavenSlot
		if place >= 0 {
			if found, ok := bsMapSchoolHeavenSlotAt(place, bsMapSchoolHeavenCountBits(masks[place]), bsMapSchoolHeavenRank(masks[place], character)); ok {
				slot = &found
			}
		}
		if slot == nil {
			skipped = append(skipped, image+"("+BsMapSchoolHeavenCharacterProperty+" "+strconv.Itoa(characterValue)+")")
			continue
		}
		items = append(items, NewBsMapItemFull(
			image, 0, slot.x, slot.y, slot.x, slot.y,
			BsMapSchoolHeavenCharacterWidth, BsMapSchoolHeavenCharacterHitHeight,
			"", BsMapSchoolHeavenCharacterProperty, bsMapSchoolHeavenUnresolvedPropertyKey, characterValue,
			BsMapSchoolHeavenPlaceProperty, bsMapSchoolHeavenUnresolvedPropertyKey, place, nil))
		placed = append(placed, image+"("+BsMapSchoolHeavenCharacterProperty+" "+strconv.Itoa(characterValue)+", "+BsMapSchoolHeavenPlaceProperty+" "+strconv.Itoa(place)+", "+strconv.Itoa(slot.x)+","+strconv.Itoa(slot.y)+")")
	}
	if len(items) == 0 {
		core.Logf("script: map schoolheaven: %s = %d 时原引擎不放置任何角色卡（图元留在 (800, 600) 屏外），返回空布局", BsMapSchoolHeavenMapTypeProperty, mapType)
		return nil
	}
	logBsMapSchoolHeavenUnavailable(context, skipped)
	systemButtons := bsMapSchoolHeavenSystemButtonList(context)
	core.Logf("script: map schoolheaven: %s = %d%s -> %d 张角色卡，%s = 图元下标 - 1（map_sakura..map_maria = 1..8，与引擎的命中下标同序）：%s", BsMapSchoolHeavenMapTypeProperty, mapType, bsMapSchoolHeavenTimeSuffix(context, mapType), len(items), BsMapSchoolHeavenCharacterProperty, strings.Join(placed, ", "))
	return &BsMapLayout{Background: BsMapSchoolHeavenBackgroundImage, Items: items, SystemButtons: systemButtons, Source: BsMapSchoolHeavenSource}
}

func buildBsMapSchoolHeavenMasks(context BsMapContext, mapType int) []int {
	masks := make([]int, bsMapSchoolHeavenMaskCount)
	switch mapType {
	case 0, 2:
		setBsMapSchoolHeavenBit(masks, 1, 0, isBsMapSchoolHeavenAvailable(context, 0))
		setBsMapSchoolHeavenBit(masks, 1, 1, isBsMapSchoolHeavenAvailable(context, 1))
		setBsMapSchoolHeavenBit(masks, 2, 5, isBsMapSchoolHeavenAvailable(context, 5))
	case 1:
		setBsMapSchoolHeavenBit(masks, 1, 0, isBsMapSchoolHeavenAvailable(context, 0))
		setBsMapSchoolHeavenBit(masks, 1, 1, isBsMapSchoolHeavenAvailable(context, 1))
		setBsMapSchoolHeavenBit(masks, 2, 5, isBsMapSchoolHeavenAvailable(context, 5))
		setBsMapSchoolHeavenBit(masks, 4, 2, isBsMapSchoolHeavenAvailable(context, 2))
	case 3:
		setBsMapSchoolHeavenBit(masks, 1, 0, isBsMapSchoolHeavenAvailable(context, 0))
		setBsMapSchoolHeavenBit(masks, 1, 1, isBsMapSchoolHeavenAvailable(context, 1))
		setBsMapSchoolHeavenBit(masks, 1, 5, isBsMapSchoolHeavenAvailable(context, 5))
		setBsMapSchoolHeavenBit(masks, 4, 2, isBsMapSchoolHeavenAvailable(context, 2))
	case 4:
		setBsMapSchoolHeavenBit(masks, 1, 5, isBsMapSchoolHeavenAvailable(context, 5))
	default:
		buildBsMapSchoolHeavenDynamicMasks(context, masks, mapType)
	}
	return masks
}

func buildBsMapSchoolHeavenDynamicMasks(context BsMapContext, masks []int, mapType int) {
	time := bsMapSchoolHeavenValue(context, BsMapSchoolHeavenTimeProperty, bsMapSchoolHeavenUnknownValue)
	if time < bsMapSchoolHeavenDynamicTimeMinimum || time > bsMapSchoolHeavenDynamicTimeMaximum {
		core.Logf("script: map schoolheaven: %s = %d 走动态位置表，但 %s = %d 不在 %d..%d，原引擎同样不放置角色卡", BsMapSchoolHeavenMapTypeProperty, mapType, BsMapSchoolHeavenTimeProperty, time, bsMapSchoolHeavenDynamicTimeMinimum, bsMapSchoolHeavenDynamicTimeMaximum)
		return
	}
	for character := 0; character < bsMapSchoolHeavenCharacterCount; character++ {
		if !isBsMapSchoolHeavenAvailable(context, character) {
			continue
		}
		row := bsMapSchoolHeavenDynamicRow(character, bsMapSchoolHeavenValue(context, bsMapSchoolHeavenProgressProperties[character], 0), time)
		if row < 0 || row >= bsMapSchoolHeavenMaskCount {
			core.Logf("script: map schoolheaven: '%s' 的动态行号 %d 越界，跳过", bsMapSchoolHeavenCharacters[character], row)
			continue
		}
		masks[row] |= 1 << character
	}
}

func bsMapSchoolHeavenDynamicRow(character, progress, time int) int {
	rows := bsMapSchoolHeavenDynamicRows[character]
	position := 0
	for position < len(rows) && progress >= rows[position].threshold {
		position++
	}
	if position < len(rows) && rows[position].time == time {
		return rows[position].row
	}
	bands := bsMapSchoolHeavenDynamicBands[character]
	if len(bands) == 0 {
		return 0
	}
	last := len(bands) - 1
	band := 0
	for band < last && progress >= bands[band].threshold {
		band++
	}
	return bsMapSchoolHeavenBandRow(bands[band], time)
}

func bsMapSchoolHeavenBandRow(band bsMapSchoolHeavenBandEntry, time int) int {
	switch time {
	case 1:
		return band.row1
	case 2:
		return band.row2
	case 3:
		return band.row3
	case 4:
		return band.row4
	default:
		return band.row1
	}
}

func isBsMapSchoolHeavenAvailable(context BsMapContext, character int) bool {
	return bsMapSchoolHeavenValue(context, bsMapSchoolHeavenAvailabilityProperties[character], 0) == 0
}

func bsMapSchoolHeavenValue(context BsMapContext, name string, fallback int) int {
	value := context.Properties.ByName(name)
	if value == bsMapSchoolHeavenUnknownValue {
		return fallback
	}
	return value
}

func setBsMapSchoolHeavenBit(masks []int, row, bit int, set bool) {
	if set {
		masks[row] |= 1 << bit
	}
}

func bsMapSchoolHeavenPlaceOf(masks []int, character int) int {
	bit := 1 << character
	for row := 0; row < len(masks); row++ {
		if (masks[row] & bit) != 0 {
			return row
		}
	}
	return bsMapSchoolHeavenUnknownValue
}

func bsMapSchoolHeavenCountBits(mask int) int {
	count := 0
	for mask != 0 {
		count += mask & 1
		mask >>= 1
	}
	return count
}

func bsMapSchoolHeavenRank(mask, character int) int {
	rank := 0
	for bit := 0; bit < character; bit++ {
		if (mask & (1 << bit)) != 0 {
			rank++
		}
	}
	return rank
}

func bsMapSchoolHeavenSlotAt(row, popCount, rank int) (bsMapSchoolHeavenSlot, bool) {
	index := bsMapSchoolHeavenTableIndex(row, popCount, rank)
	if index < 0 || index >= len(bsMapSchoolHeavenSlotTable) {
		return bsMapSchoolHeavenSlot{}, false
	}
	return bsMapSchoolHeavenSlotTable[index], true
}

func bsMapSchoolHeavenTableIndex(row, popCount, rank int) int {
	return (bsMapSchoolHeavenTableStride * ((bsMapSchoolHeavenTableStride * row) + popCount)) + rank - bsMapSchoolHeavenTableBaseOffset
}

func bsMapSchoolHeavenTimeSuffix(context BsMapContext, mapType int) string {
	if mapType <= bsMapSchoolHeavenStaticMapTypeMaximum {
		return ""
	}
	return "，" + BsMapSchoolHeavenTimeProperty + " = " + strconv.Itoa(context.Properties.ByName(BsMapSchoolHeavenTimeProperty))
}

func logBsMapSchoolHeavenUnavailable(context BsMapContext, skipped []string) {
	if len(skipped) > 0 {
		core.Logf("script: map schoolheaven: 未放置（原引擎把图元留在 (800, 600) 屏外，不可见也不可点）：%s", strings.Join(skipped, ", "))
	}
	blocked := make([]string, 0, bsMapSchoolHeavenCharacterCount)
	for character := 0; character < bsMapSchoolHeavenCharacterCount; character++ {
		if isBsMapSchoolHeavenAvailable(context, character) {
			continue
		}
		blocked = append(blocked, bsMapSchoolHeavenCharacters[character]+"("+bsMapSchoolHeavenAvailabilityProperties[character]+" != 0)")
	}
	if len(blocked) > 0 {
		core.Logf("script: map schoolheaven: 今天已完成、不再出现在地图上的角色：%s", strings.Join(blocked, ", "))
	}
}

// bsMapSchoolHeavenSystemButtonList 生成地图底部的系统按钮。
//
// 原引擎这些按钮走 action 表打开各自的系统界面，不写 #character/#place；
// 因此它们不能作为 BsMapItem 放进 Items（那里的点击会写回角色属性），而是用
// BsMapSystemButton 单独承载，Tool 只保存剥掉 map_ 前缀后的功能名。
// 图片不存在时跳过该按钮，其他按钮照常显示。
func bsMapSchoolHeavenSystemButtonList(context BsMapContext) []BsMapSystemButton {
	buttons := make([]BsMapSystemButton, 0, len(bsMapSchoolHeavenSystemButtons))
	for _, button := range bsMapSchoolHeavenSystemButtons {
		if !context.Resources.Exists(button.image) {
			core.Logf("script: map schoolheaven: 系统按钮图 '%s' 缺失，跳过", button.image)
			continue
		}
		buttons = append(buttons, BsMapSystemButton{
			Image:  button.image,
			X:      button.x,
			Y:      button.y,
			Width:  BsMapSchoolHeavenButtonWidth,
			Height: BsMapSchoolHeavenButtonHitHeight,
			Tool:   strings.TrimPrefix(button.image, "map_"),
		})
	}
	return buttons
}
