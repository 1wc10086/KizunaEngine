package bsmap

import (
	"os"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

// bs_map_profile_mk2.go 处理「牝教師2～淫悦の学舎～」（MK2，BSScript Rev.6）
// 与「牝教師2～淫従の螺旋～IE」（MK2IE，BSXScript 3.0）的行动地图。
//
// 两代发行版共用同一份 map.dat（逐字节相同），地图的实际入口不同：
//   - MK2 是 Rev.6，脚本用 system call "map" 打开地图；
//   - MK2IE 是 BSXScript 3.0，脚本用 SELECT_TAIL "map" 打开地图。
//
// map.dat 的结构决定了两件事：
//   - 每条 UnlockTable 是"一个目标"：前 4 条是四名女主角，之后 24 条是
//     道具（每人 6 个），最后 2 条是双人合作事件。Progress 属性按天推进，
//     表里每个阈值行给出当天该目标出现在哪个地点、用哪张脸。
//   - 表项是 uint16 打包值：低字节 = 地点号 1..13，高字节 = 图像表下标。
//
// #result 写回的是【表序号 + 1】（1..31），不是地点号：脚本在 map 标签之后
// 按 1、2、3… 的顺序逐条比较 #result，命中哪一条就演哪个目标的场景。
// 9999 是「時間を進める」：脚本遇到 9999 直接落到分派链末尾，把 #time 加一。
// 这些值是用真实发行版跑通后验证的，不能想当然地照抄 KS 的写法。
//
// 地图的点击热区由 map_hit 的 8 位调色板下标决定：0..12 分别对应地点 1..13，
// 255 是不可点区域。同一个地点站多人时，原版按三角排布把卡横着铺开，
// 点击整片地点区域都会命中该地点的第一张卡（和 KS 一致）。
type BsMapProfileMk2 struct{}

// BsMapProfileMk2Modern 是 MK2IE（BSXScript 3.0）的地图。
//
// 之所以要拆成两个 profile：BuildBsMapProfiles 的 context 只带"是不是 Rev
// 方言"这一个分流信号，而两代脚本的资产指纹又高度重合（同一份 map.dat、
// 同一组 map_ch*）。拆开后 MK2 仍走 Rev.6 分支，MK2IE 不再被 Rev 系
// profile 排除，也不会误吃 KS/三射面談的 map.dat。
type BsMapProfileMk2Modern struct{}

const (
	bsMapMk2BackgroundDayImage     = "map_base1"
	bsMapMk2BackgroundNightImage   = "map_base2"
	bsMapMk2PlaceImageSample       = "map_ch101"
	bsMapMk2CombImageSample        = "map_comb"
	bsMapMk2ItemImageSample        = "map_item"
	bsMapMk2HitMaskImage           = "map_hit"
	bsMapMk2MenuBaseImage          = "map_menu_base"
	bsMapMk2NextTimeButtonImage    = "map_nexttime"
	bsMapMk2NextTimeModernImage    = "map_next_time"
	bsMapMk2ResultPropertyFallback = 94
	bsMapMk2NextValue              = 9999
	bsMapMk2NightBackdropSlot      = 3
	bsMapMk2PlaceValueMask         = 0xFF
	bsMapMk2ImageShift             = 8
	bsMapMk2MaskPlaceShift         = 1
	bsMapMk2CardCellSize           = 64
	bsMapMk2CardFrameCount         = 2
	// map_hit 的下标 0..12 是 13 个地点；再大的是不可点区域。
	bsMapMk2MaskValueLimit = 16
)

// 工具条按钮的屏幕坐标来自 message_hit 的底部一行（底图 map_menu_base
// 已经把 Q.SAVE…LOG 的文字烘焙在 y=563 的条上，按钮图只是叠在文字上）。
// 坐标与 message_hit 的 800x124 窗口原点 (0,476) 相加得到。
var bsMapMk2Toolbar = []BsMapSystemButton{
	{Image: "btn_qsave", X: 28, Y: 580, Frames: 4, Tool: "qsave"},
	{Image: "btn_qload", X: 120, Y: 580, Frames: 4, Tool: "qload"},
	{Image: "btn_save", X: 224, Y: 580, Frames: 4, Tool: "save"},
	{Image: "btn_load", X: 312, Y: 580, Frames: 4, Tool: "load"},
	{Image: "btn_config", X: 396, Y: 580, Frames: 4, Tool: "config"},
	{Image: "btn_status", X: 500, Y: 580, Frames: 4, Tool: "status"},
	{Image: "btn_log", X: 596, Y: 580, Frames: 4, Tool: "log"},
}

func (p *BsMapProfileMk2) Name() string {
	return "Mk2"
}

func (p *BsMapProfileMk2) Matches(context BsMapContext) bool {
	// Rev.6 的方言语义明确，现代签名不能进来。
	if !context.HasDialect || context.Dialect != bs.BsScriptRev6 {
		return false
	}
	return bsMapMk2MatchesAssets(context)
}

func (p *BsMapProfileMk2) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map mk2: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapMk2Layout(context, false)
}

func (p *BsMapProfileMk2Modern) Name() string {
	return "Mk2IE"
}

func (p *BsMapProfileMk2Modern) Matches(context BsMapContext) bool {
	// MK2IE 是现代签名（HasDialect=false）。这里只拒绝 Rev/Ancient，
	// 避免和 Mk2/Ks 互相抢图。
	if context.HasDialect {
		return false
	}
	if !context.Resources.Exists(bsMapMk2CombImageSample) ||
		!context.Resources.Exists(bsMapMk2ItemImageSample) {
		return false
	}
	return bsMapMk2MatchesAssets(context)
}

func (p *BsMapProfileMk2Modern) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map mk2ie: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapMk2Layout(context, true)
}

// bsMapMk2MatchesAssets 是两代共用的素材指纹。
//
// map_ch101 + map_hit + map.dat 已经是这一代独有的组合（KS 是 map_ch11、
// 三射面談是 map_ch101+地点圆点但没有 map_comb，裏教師是 map_f100）。
// 再加 map_comb 与 map_item 之后，只有牝教師2 的两代发行版会命中。
func bsMapMk2MatchesAssets(context BsMapContext) bool {
	if len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	return context.Resources.Exists(bsMapMk2BackgroundDayImage) &&
		context.Resources.Exists(bsMapMk2PlaceImageSample) &&
		context.Resources.Exists(bsMapMk2HitMaskImage)
}

type bsMapMk2Card struct {
	image string
	value int
	place int
}

func createBsMapMk2Layout(context BsMapContext, modern bool) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	timeIndex, timeValue := resolveBsMapMk2Time(context, mapData)
	if timeIndex < 0 {
		// 时段变量还没写（第一天开场）时不要吞掉地图：落到第一格，
		// 底图与系统按钮照样能用，脚本仍能拿到 #result。
		core.Logf("script: map mk2: '%s' = %d is not one of the map.dat slots, using slot 0", mapData.TimeProperty, timeValue)
		timeIndex = 0
	}
	resultKey := resolveBsMapMk2ResultKey(context, mapData.ResultProperty)
	cards := make([][]bsMapMk2Card, len(mapData.Places)+1)
	for tableIndex := range mapData.Tables {
		table := mapData.Tables[tableIndex]
		// state != 0 表示当天这个目标已经不在地图上（事件结束/道具已取）。
		// 缺失的属性返回 -1：这种表整条跳过，比把它当成 0 更强。
		if context.Properties.ByName(table.StateProperty) != 0 {
			continue
		}
		progress := context.Properties.ByName(table.ProgressProperty)
		if progress < 0 {
			continue
		}
		row := table.RowFor(progress)
		if row == nil || uint(timeIndex) >= uint(len(row.Values)) {
			continue
		}
		packed := int(row.Values[timeIndex])
		if packed == 0 {
			continue
		}
		place := packed & bsMapMk2PlaceValueMask
		if place < 1 || place >= len(cards) {
			continue
		}
		image := mapData.ImageName(packed>>bsMapMk2ImageShift, context.Resources)
		if len(image) == 0 {
			continue
		}
		cards[place] = append(cards[place], bsMapMk2Card{
			image: image,
			value: tableIndex + 1,
			place: place,
		})
	}

	items := make([]BsMapItem, 0, 32)
	firstItemForPlace := make([]int, len(cards))
	for index := range firstItemForPlace {
		firstItemForPlace[index] = -1
	}
	cardCells := map[string][2]int{}
	for placeIndex := 1; placeIndex < len(cards); placeIndex++ {
		group := cards[placeIndex]
		if len(group) == 0 {
			continue
		}
		place := mapData.Places[placeIndex-1]
		// 同一地点站 k 名角色时，原引擎按三角排布的 k 个 chunk 摆卡：
		// 1 人从第 0 格、2 人从第 1 格、3 人从第 3 格……。
		offset := len(group) * (len(group) - 1) / 2
		for index := range group {
			chunkIndex := offset + index
			if chunkIndex >= len(place.Chunks) {
				core.Logf("script: map mk2: place %d needs chunk %d, only %d available", placeIndex, chunkIndex, len(place.Chunks))
				break
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			width, height := bsMapMk2CardCell(context, group[index].image, cardCells)
			if firstItemForPlace[placeIndex] < 0 {
				firstItemForPlace[placeIndex] = len(items)
			}
			item := NewBsMapItemWithProperty(
				group[index].image, 0, x, y, x, y, width, height,
				"", mapData.ResultProperty, resultKey, group[index].value)
			// CellHeight 让渲染层从 64x128 的竖排图里只取一帧，
			// 否则会把常态/悬停两张脸一起画出来。
			item.CellHeight = height
			items = append(items, item)
		}
	}

	layout := &BsMapLayout{
		Items:             items,
		Source:            BsMapDataFileName,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		TimeSlot:          timeIndex,
		TimeValue:         timeValue,
		TimeSlotValid:     true,
		DayVariable:       "#day",
		FixedBackground:   true,
		// 没有角色/道具可点时也要打开地图：原版仍显示底图、
		// 「時間を進める」与系统按钮，脚本靠 9999 推进时段。
		AllowEmpty: true,
	}
	if mask := bsMapMk2Mask(context); mask != nil {
		for value := range mask.ItemForValue {
			mask.ItemForValue[value] = -1
		}
		for value := 0; value < len(mask.ItemForValue) && value < bsMapMk2MaskValueLimit; value++ {
			place := value + bsMapMk2MaskPlaceShift
			if place < 0 || place >= len(firstItemForPlace) {
				continue
			}
			mask.ItemForValue[value] = firstItemForPlace[place]
		}
		layout.Mask = mask
	}
	layout.Background = bsMapMk2Background(context, timeIndex)
	if modern {
		// MK2IE 的边框/系统按钮/「時間を進める」全部由 bsxx.cfg 的
		// [map phase] 提供，beginMapSelect 会按配置补画，这里不重复。
		core.Logf("script: map mk2ie: %d card(s), #time=%d slot=%d backdrop='%s'", len(items), timeValue, timeIndex, layout.Background)
		return layout
	}
	// MK2（Rev.6）没有 bsxx.cfg：底条、工具栏与「時間を進める」的坐标
	// 直接编译在 mk2.exe 里，这里按 message_hit / 图片头偏移还原。
	layout.FrameImage = bsMapMk2MenuBaseImage
	for _, button := range bsMapMk2Toolbar {
		if !context.Resources.Exists(button.Image) {
			continue
		}
		layout.SystemButtons = append(layout.SystemButtons, button)
	}
	if context.Resources.Exists(bsMapMk2NextTimeButtonImage) {
		layout.SystemButtons = append(layout.SystemButtons, BsMapSystemButton{
			Image: bsMapMk2NextTimeButtonImage, X: 688, Y: 8, Frames: 2, Tool: "next",
		})
	}
	layout.NextValue = bsMapMk2NextValue
	core.Logf("script: map mk2: %d card(s), #time=%d slot=%d backdrop='%s' mask=%v", len(items), timeValue, timeIndex, layout.Background, layout.Mask != nil)
	return layout
}

// bsMapMk2CardCell 取角色/道具卡一帧的尺寸。
//
// map_ch* / map_comb / map_item 都是 64x128 的两帧竖排图（常态/张嘴），
// BSG 头里的 FrameCount 有时是 1、有时是 2；按高度折半比信任头字段可靠。
func bsMapMk2CardCell(context BsMapContext, name string, cache map[string][2]int) (int, int) {
	if size, ok := cache[name]; ok {
		return size[0], size[1]
	}
	size := [2]int{bsMapMk2CardCellSize, bsMapMk2CardCellSize}
	if width, height := bsMapMk2ImageSize(context, name); width > 0 && height > 0 {
		if height%bsMapMk2CardFrameCount == 0 {
			size = [2]int{width, height / bsMapMk2CardFrameCount}
		} else {
			size = [2]int{width, height}
		}
	}
	cache[name] = size
	return size[0], size[1]
}

func bsMapMk2ImageSize(context BsMapContext, name string) (int, int) {
	if len(name) == 0 {
		return 0, 0
	}
	location, ok := context.Resources.TryGet(name)
	if !ok {
		return 0, 0
	}
	head := location.ReadPrefix(0x40)
	if len(head) < 0x1A {
		return 0, 0
	}
	return int(head[0x16]) | int(head[0x17])<<8, int(head[0x18]) | int(head[0x19])<<8
}

func bsMapMk2Mask(context BsMapContext) *BsMapMaskData {
	location, ok := context.Resources.TryGet(bsMapMk2HitMaskImage)
	if !ok {
		return nil
	}
	width, height, values, ok := gfx.ReadBsgIndexedPixels(location.Read())
	if !ok || width <= 0 || height <= 0 {
		core.Logf("script: map mk2: '%s' is not an indexed mask", bsMapMk2HitMaskImage)
		return nil
	}
	itemForValue := make([]int, 256)
	for index := range itemForValue {
		itemForValue[index] = -1
	}
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

func resolveBsMapMk2Time(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	value := context.Properties.ByName(mapData.TimeProperty)
	if value < 0 {
		return -1, value
	}
	return mapData.TimeSlot(value), value
}

func resolveBsMapMk2ResultKey(context BsMapContext, name string) int {
	if context.Script != nil && len(name) > 0 {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapMk2ResultPropertyFallback
}

// bsMapMk2Background 按时段选白天/夜景底图。
//
// map.dat 的四个时段（#time 5..8）里，前三个是白天（放学后到傍晚），
// 最后一个是夜里；和 KS/UK 的约定一致。缺夜景图时退回白天图，
// 不要返回空串让地图变成黑屏。
func bsMapMk2Background(context BsMapContext, timeSlot int) string {
	day := context.Resources.Exists(bsMapMk2BackgroundDayImage)
	night := context.Resources.Exists(bsMapMk2BackgroundNightImage)
	if timeSlot < bsMapMk2NightBackdropSlot {
		if day {
			return bsMapMk2BackgroundDayImage
		}
		if night {
			return bsMapMk2BackgroundNightImage
		}
		return ""
	}
	if night {
		return bsMapMk2BackgroundNightImage
	}
	if day {
		return bsMapMk2BackgroundDayImage
	}
	return ""
}
