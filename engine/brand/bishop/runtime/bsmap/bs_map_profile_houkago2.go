package bsmap

import (
	"os"

	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

type BsMapProfileHoukago2 struct{}

const (
	bsMapHoukago2BackgroundImage    = "map_base"
	bsMapHoukago2PlaceImageSample   = "map_ch1"
	bsMapHoukago2ResultPropertyKey  = 33
	bsMapHoukago2PlaceValueBase     = 128
	bsMapHoukago2CharacterValueBase = 1
	bsMapHoukago2PlaceHitSize       = 64
	bsMapHoukago2UnresolvedProperty = -1
	bsMapHoukago2PackedEntryMask    = 0xFF
	bsMapHoukago2PackedImageShift   = 8
	bsMapHoukago2MenuBase           = "map_menu_base"
	bsMapHoukago2MenuX              = 656
	bsMapHoukago2MenuY              = 524
	bsMapHoukago2InfoBase           = "map_info_base"
	bsMapHoukago2InfoX              = 4
	bsMapHoukago2InfoY              = 4
)

func (p *BsMapProfileHoukago2) Name() string {
	return "Houkago2"
}

func (p *BsMapProfileHoukago2) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	if context.Dialect != bs.BsScriptRev5 {
		return false
	}
	if len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	return context.Resources.Exists(bsMapHoukago2BackgroundImage) &&
		context.Resources.Exists(bsMapHoukago2PlaceImageSample)
}

func (p *BsMapProfileHoukago2) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map houkago2: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapHoukago2Layout(context)
}

type bsMapHoukago2Node struct {
	image string
	value int
}

func createBsMapHoukago2Layout(context BsMapContext) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	nodes := make([][]bsMapHoukago2Node, len(mapData.Places))
	timeIndex, timeValue := resolveBsMapHoukago2TimeIndex(context, mapData)
	if timeIndex < 0 {
		core.Logf("script: map houkago2: '%s' = %d is not one of the map.dat slots, keeping %d place hotspot(s) only", mapData.TimeProperty, timeValue, len(mapData.Places))
	} else {
		addBsMapHoukago2CharacterNodes(context, mapData, timeIndex, nodes)
	}
	// 地点卡只在"当天这里没有人"时出现。有人站在同一个地点时，角色卡会
	// 用 map.dat 的 chunk 0 扇出到同一坐标；两张 64x64 卡完全重叠，而
	// hitButton 按添加顺序取第一个命中，先加地点卡会让玩家永远点不到人。
	// 原版也是"有人就找人、没人就搜地点"，所以这里用角色卡替换地点卡。
	for index := range nodes {
		if len(nodes[index]) > 0 {
			continue
		}
		image := mapData.ImageName(mapData.Places[index].ImageIndex, context.Resources)
		if len(image) == 0 {
			continue
		}
		nodes[index] = []bsMapHoukago2Node{{
			image: image,
			value: bsMapHoukago2PlaceValueBase + index,
		}}
	}
	items := make([]BsMapItem, 0, len(nodes))
	characters := 0
	resultKey := resolveBsMapHoukago2ResultKey(context, mapData.ResultProperty)
	for index := range nodes {
		items = addBsMapHoukago2EntryNodes(items, mapData, index, nodes[index], resultKey)
		characters += max(0, len(nodes[index])-1)
	}
	core.Logf("script: map houkago2: %d place hotspot(s), %d character icon(s) (#time=%d)", len(mapData.Places), characters, timeValue)
	layout := &BsMapLayout{
		Background:        bsMapHoukago2BackgroundImage,
		Items:             items,
		Source:            BsMapDataFileName,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		TimeSlot:          timeIndex,
		TimeValue:         timeValue,
		TimeSlotValid:     timeIndex >= 0,
		DayVariable:       "#day",
		// map.dat 已经给出全部地点的坐标；不要再用 [map phase] 的
		// base by time 覆盖底图（本作没有分时段底图）。
		FixedBackground: true,
		// 当天所有角色都离开时仍要打开地图：地点卡/系统菜单还能用，
		// 直接跳过会让脚本拿着上一张图的 #result 重放已经看过的场景。
		AllowEmpty: true,
		// 右下角系统菜单底板 + 左上角周数/曜日卡。两块都必须画在角色卡之上
		// （原版的地图 UI 是整屏 overlay），但都不参与命中。
		FrameImage:  bsMapHoukago2MenuBase,
		FrameX:      bsMapHoukago2MenuX,
		FrameY:      bsMapHoukago2MenuY,
		FrameImage2: bsMapHoukago2InfoBase,
		Frame2X:     bsMapHoukago2InfoX,
		Frame2Y:     bsMapHoukago2InfoY,
	}
	// 系统菜单四颗按钮的位置来自 map_hit 的命中块：
	// (668,536,52x20) QSAVE / (732,536,52x20) QLOAD /
	// (668,564,60x20) SYSTEM / (748,564,36x20) LOG。
	// map_menu_button 是 60x80 的四帧竖排图（每帧 60x20），每颗按钮固定
	// 画自己那一帧；Width/Height 覆盖默认的整图矩形，保证命中区与原版一致。
	menu := []BsMapSystemButton{
		{Image: "map_menu_button", Frames: 4, Frame: 0, X: 668, Y: 536, Width: 52, Height: 20, Tool: "qsave"},
		{Image: "map_menu_button", Frames: 4, Frame: 1, X: 732, Y: 536, Width: 52, Height: 20, Tool: "qload"},
		{Image: "map_menu_button", Frames: 4, Frame: 2, X: 668, Y: 564, Width: 60, Height: 20, Tool: "config"},
		{Image: "map_menu_button", Frames: 4, Frame: 3, X: 748, Y: 564, Width: 36, Height: 20, Tool: "log"},
	}
	for _, button := range menu {
		if context.Resources.Exists(button.Image) {
			layout.SystemButtons = append(layout.SystemButtons, button)
		}
	}
	return layout
}

func resolveBsMapHoukago2ResultKey(context BsMapContext, name string) int {
	if context.Script != nil && len(name) > 0 {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapHoukago2ResultPropertyKey
}

func addBsMapHoukago2EntryNodes(items []BsMapItem, mapData BsMapData, entryIndex int, entryNodes []bsMapHoukago2Node, resultKey int) []BsMapItem {
	place := mapData.Places[entryIndex]
	offset := len(entryNodes) * (len(entryNodes) - 1) / 2
	for index := range entryNodes {
		chunkIndex := offset + index
		if chunkIndex >= len(place.Chunks) {
			core.Logf("script: map houkago2: entry %d node %d needs chunk %d, only %d available", entryIndex, index, chunkIndex, len(place.Chunks))
			continue
		}
		chunk := place.Chunks[chunkIndex]
		x := place.BaseX + chunk.X
		y := place.BaseY + chunk.Y
		items = append(items, NewBsMapItemWithProperty(
			entryNodes[index].image, 0, x, y, x, y, bsMapHoukago2PlaceHitSize, bsMapHoukago2PlaceHitSize,
			"", mapData.ResultProperty, resultKey, entryNodes[index].value))
	}
	return items
}

func addBsMapHoukago2CharacterNodes(context BsMapContext, mapData BsMapData, timeIndex int, nodes [][]bsMapHoukago2Node) {
	for index := range mapData.Tables {
		table := mapData.Tables[index]
		if context.Properties.ByName(table.StateProperty) != 0 {
			continue
		}
		progress := context.Properties.ByName(table.ProgressProperty)
		if progress < 0 {
			continue
		}
		selected := table.RowFor(progress)
		if selected == nil || uint(timeIndex) >= uint(len(selected.Values)) {
			continue
		}
		packed := selected.Values[timeIndex]
		target := int(packed&bsMapHoukago2PackedEntryMask) - 1
		if uint(target) >= uint(len(nodes)) {
			continue
		}
		nodes[target] = append(nodes[target], bsMapHoukago2Node{
			image: mapData.ImageName(int(packed>>bsMapHoukago2PackedImageShift), context.Resources),
			value: bsMapHoukago2CharacterValueBase + index,
		})
	}
}

func resolveBsMapHoukago2TimeIndex(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, bsMapHoukago2UnresolvedProperty
	}
	timeValue := context.Properties.ByName(mapData.TimeProperty)
	if timeValue < 0 {
		return -1, timeValue
	}
	return mapData.TimeSlot(timeValue), timeValue
}
