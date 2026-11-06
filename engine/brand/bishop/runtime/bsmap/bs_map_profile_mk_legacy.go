package bsmap

import (
	"os"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

// bs_map_profile_mk_legacy.go 处理「牝教師～淫辱の教室～」（MK，BSScript Rev.5）
// 与其 2014 年重制版「牝教師～淫辱の教室～IE」（MKIE，BSXScript 3.0）的行动地图。
//
// 两代发行版共用同一份 map.dat（md5 652718e014caaa8b94f751fa3efcecac），
// 地图入口与 MK2 一样分两条路：MK 用 `system call "map"`，MKIE 用
// `SELECT_TAIL "map"`。此前没有 profile 认领，这两作的地图被整块跳过，
// 玩家停在 SELECT_TAIL 上、脚本拿着上一份 #character 继续跑。
//
// map.dat 里 10 个地点各有一组 chunk；20 张解锁表（前 6 条角色、后 14 条
// 道具）的表项是 uint16 打包值：
//   - 低字节 = 地点号 1..10；
//   - 高字节 = 图像表下标（1 基）。
//
// 写回变量是 "#character"，值是【表序号 + 1】（1..20），脚本在 map 标签之后
// 按 1、2、3… 逐条分派；9999 是「時間を進める」，脚本直接落到分派链末尾
// 把 #time 加一。这些语义和 MK2 一致，所以这里沿用 MK2 的骨架，只在素材
// 指纹、卡片尺寸和按钮坐标上分开。
//
// 与 MK2 的关键差异：
//   - 卡是 80x160 的两帧竖排图（80x80 常态/事件），不是 64x128；
//   - map_base/map_hit 是 800x600 整图，没有 map_base1/map_base2 的昼夜
//     切换，也没有 map_ch101/map_comb；
//   - MK 没有 bsxx.cfg：底条与系统按钮坐标编译在 mk.exe 里，只能按 map_hit
//     的命中块还原；MKIE 的 bsxx.cfg [map phase] 则把边框、时段条、系统
//     按钮全部配好了，profile 不能再补一遍，否则同一颗按钮会被画两次、
//     点一次触发两个分支。
type BsMapProfileMkLegacy struct{}

// BsMapProfileMkLegacyModern 是 MKIE（BSXScript 3.0）的地图。
//
// 拆成两个 profile 的原因与 MK2 相同：BuildBsMapProfiles 只给 context 一个
// "是否检测到 Rev 方言"的信号，而两代共用同一份 map.dat 与同一组 800x600
// 素材。按 HasDialect 分流之后，MK 不会吃掉 MKIE，MKIE 也不会被 Rev 系
// profile（Houkago2）误认。
type BsMapProfileMkLegacyModern struct{}

const (
	bsMapMkLegacyBackgroundImage     = "map_base"
	bsMapMkLegacyHitMaskImage        = "map_hit"
	bsMapMkLegacyItemImage           = "map_item"
	bsMapMkLegacyCharacterSample     = "miyu1_norm"
	bsMapMkLegacyNextTimeButton      = "map_nexttime"
	bsMapMkLegacyResultFallback      = 94 // 剧本里查不到结果变量时的回退槽位
	bsMapMkLegacyNextValue           = 9999
	bsMapMkLegacyPlaceValueMask      = 0xFF
	bsMapMkLegacyImageShift          = 8
	bsMapMkLegacyMaskPlaceShift      = 1
	bsMapMkLegacyMaskPlaceCount      = 10
	bsMapMkLegacyCardFrameCount      = 2
	bsMapMkLegacyDefaultCardWidth    = 80
	bsMapMkLegacyDefaultCardHeight   = 80
	bsMapMkLegacyNextTimeButtonX     = 448
	bsMapMkLegacyNextTimeButtonY     = 528
	bsMapMkLegacyNextTimeButtonFrame = 4
)

// 排除用的素材指纹。MK/MKIE 的发行目录里都没有这三张，反过来只要出现
// 任何一张，就说明当前是牝教師2 或别的同代作品，不能由本 profile 接管：
// map_ch101 是牝教師2/三射面談的角色卡，map_comb 与 map_base1 是牝教師2
// 独有（合作图标、白天底图）。
var bsMapMkLegacyModernExcluded = []string{
	"map_ch101",
	"map_comb",
	"map_base1",
}

// MK 没有 bsxx.cfg，底部四颗系统按钮的位置来自 map_hit 的命中块。
// 原版只给 Q.SAVE / Q.LOAD / CONFIG / LOG 四颗（没有 save/load/status），
// 与 MKIE 的 [map phase] 六颗不同；多出来的按钮在这里点不到，不能凭空补。
var bsMapMkLegacyToolbar = []BsMapSystemButton{
	{Image: "btn_qsave", X: 718, Y: 500, Frames: 4, Tool: "qsave"},
	{Image: "btn_qload", X: 718, Y: 524, Frames: 4, Tool: "qload"},
	{Image: "btn_config", X: 718, Y: 548, Frames: 4, Tool: "config"},
	{Image: "btn_log", X: 718, Y: 572, Frames: 4, Tool: "log"},
}

func (p *BsMapProfileMkLegacy) Name() string {
	return "MkLegacy"
}

func (p *BsMapProfileMkLegacy) Matches(context BsMapContext) bool {
	// MK 是 Rev.5。Rev.6（MK2/KD）和现代签名都必须被挡在外面：
	// 三个世代的 map.dat 结构相近，方言是唯一可靠的代际标尺。
	if !context.HasDialect || context.Dialect != bs.BsScriptRev5 {
		return false
	}
	return bsMapMkLegacyMatchesAssets(context)
}

func (p *BsMapProfileMkLegacy) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map mk: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapMkLegacyLayout(context, true)
}

func (p *BsMapProfileMkLegacyModern) Name() string {
	return "MKIE"
}

func (p *BsMapProfileMkLegacyModern) Matches(context BsMapContext) bool {
	// MKIE 是现代签名（BSXScript 3.0，HasDialect=false）。Rev 系作品由
	// MkLegacy/Houkago2 处理，这里直接拒绝，避免两边互抢。
	if context.HasDialect {
		return false
	}
	return bsMapMkLegacyMatchesAssets(context)
}

func (p *BsMapProfileMkLegacyModern) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map mkie: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapMkLegacyLayout(context, false)
}

// bsMapMkLegacyMatchesAssets 是两代共用的素材指纹。
//
// map_base/map_hit/map_item 这三个名字在多部作品里重复（KS、三射面談、
// 牝教師2 都用 map_base），单靠它们会误吃别人的地图。miyu1_norm 是
// 「牝教師」初代独有的角色图标（美優），其它世代都是 map_ch*；再用
// map_ch101/map_comb/map_base1 把 MK2 排除掉，命中范围就只剩 MK/MKIE。
// map.dat 必须真实存在：profile 的 Build 依赖它，缺文件时提前返回，
// 让后面的 profile 还有机会。
func bsMapMkLegacyMatchesAssets(context BsMapContext) bool {
	if len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	if !context.Resources.Exists(bsMapMkLegacyBackgroundImage) ||
		!context.Resources.Exists(bsMapMkLegacyHitMaskImage) ||
		!context.Resources.Exists(bsMapMkLegacyItemImage) ||
		!context.Resources.Exists(bsMapMkLegacyCharacterSample) {
		return false
	}
	for _, name := range bsMapMkLegacyModernExcluded {
		if context.Resources.Exists(name) {
			return false
		}
	}
	return true
}

type bsMapMkLegacyCard struct {
	image string
	value int
	place int
}

func createBsMapMkLegacyLayout(context BsMapContext, legacy bool) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	timeIndex, timeValue := resolveBsMapMkLegacyTime(context, mapData)
	if timeIndex < 0 {
		// 时段变量还没写（第一天开场）或写成了区间外的值时不要跳过地图：
		// 落到第一格，底图与系统按钮照样能用，脚本仍能拿到 #character。
		core.Logf("script: map mk: '%s' = %d is not one of the map.dat slots, using slot 0", mapData.TimeProperty, timeValue)
		timeIndex = 0
	}
	resultKey := resolveBsMapMkLegacyResultKey(context, mapData.ResultProperty)

	// cards[0] 空置：表项里的地点号是 1 基，直接从地点 1 开始放，
	// 省掉每次减一的越界判断。
	cards := make([][]bsMapMkLegacyCard, len(mapData.Places)+1)
	for tableIndex := range mapData.Tables {
		table := mapData.Tables[tableIndex]
		// state != 0 表示当天这个目标已经离开地图（事件演完/道具已取）。
		// 缺失属性返回 -1，整条跳过比当成 0 更安全。
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
		place := packed & bsMapMkLegacyPlaceValueMask
		if place < 1 || place >= len(cards) {
			continue
		}
		// ImageName 已经过滤掉缺失的图：本作 map.dat 的前 10 张 map_p01..p10
		// 在两个发行版里都不存在，必须跳过而不是画一张空白热区。
		image := mapData.ImageName(packed>>bsMapMkLegacyImageShift, context.Resources)
		if len(image) == 0 {
			continue
		}
		cards[place] = append(cards[place], bsMapMkLegacyCard{
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
		// 同一地点站 k 个目标时，原引擎按三角排布的 k 个 chunk 摆卡：
		// 1 个从第 0 格、2 个从第 1 格、3 个从第 3 格……。和 MK2 完全一致。
		offset := len(group) * (len(group) - 1) / 2
		for index := range group {
			chunkIndex := offset + index
			if chunkIndex >= len(place.Chunks) {
				core.Logf("script: map mk: place %d needs chunk %d, only %d available", placeIndex, chunkIndex, len(place.Chunks))
				break
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			width, height := bsMapMkLegacyCardCell(context, group[index].image, cardCells)
			if firstItemForPlace[placeIndex] < 0 {
				firstItemForPlace[placeIndex] = len(items)
			}
			item := NewBsMapItemWithProperty(
				group[index].image, 0, x, y, x, y, width, height,
				"", mapData.ResultProperty, resultKey, group[index].value)
			// 80x160 的竖排图里只有第 0 帧是常态；CellHeight 让渲染层只切
			// 一帧，悬停第 1 帧由 drawMapCardHover 补画。
			item.CellHeight = height
			items = append(items, item)
		}
	}

	layout := &BsMapLayout{
		Items:             items,
		Source:            BsMapDataFileName,
		Background:        bsMapMkLegacyBackgroundImage,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		TimeSlot:          timeIndex,
		TimeValue:         timeValue,
		TimeSlotValid:     true,
		DayVariable:       "#day",
		// MK 没有 bsxx.cfg 的 [map phase]：底图与按钮坐标完全由本 profile
		// 决定，不能再让 Session 套 base by time 覆盖。
		FixedBackground: legacy,
		// 所有人都被拜访过时原版仍显示底图与「時間を進める」；这里不打开
		// 地图的话，SELECT_TAIL 会被跳过，脚本会复用上一张图的 #character。
		AllowEmpty: true,
		// 9999 是「時間を進める」。MKIE 的下一时段按钮来自 cfg，但同样要
		// 写 9999，所以两代都填。
		NextValue: bsMapMkLegacyNextValue,
	}
	if mask := bsMapMkLegacyMask(context); mask != nil {
		// map_hit 的下标 0..9 是 10 个地点；10 是「時間を進める」的命中块，
		// 11..14 是工具条按钮。它们由按钮自己处理，不能映射到角色卡，
		// 所以只把 0..9 换成地点 1..10，其余保持 -1（不可点）。
		for value := range mask.ItemForValue {
			mask.ItemForValue[value] = -1
		}
		for value := 0; value < len(mask.ItemForValue) && value < bsMapMkLegacyMaskPlaceCount; value++ {
			place := value + bsMapMkLegacyMaskPlaceShift
			if place < 0 || place >= len(firstItemForPlace) {
				continue
			}
			mask.ItemForValue[value] = firstItemForPlace[place]
		}
		layout.Mask = mask
	}

	if !legacy {
		// MKIE 的边框/日期条/系统按钮/下一时段全部由 bsxx.cfg 的
		// [map phase] 提供，beginMapSelect 会按配置补画。这里再补一遍会让
		// 同一颗按钮进两次按钮表，点一下触发两个分支。
		core.Logf("script: map mkie: %d card(s), #time=%d slot=%d backdrop='%s' mask=%v", len(items), timeValue, timeIndex, layout.Background, layout.Mask != nil)
		return layout
	}

	// MK 的地图底条就是剧情用的 message 窗口：右侧 80px 深色面板正好
	// 承接 Q.SAVE…LOG 四颗按钮（map_hit 的 718..797/500..595 与之逐像素重合）。
	// 它必须由 showMapStage 在按钮之前绘制，否则按钮会被半透明面板压暗。
	layout.FrameImage = "message"
	layout.FrameX, layout.FrameY = 0, 496

	// MK 的工具栏与「時間を進める」坐标编译在 mk.exe 里，按 map_hit 还原。
	for _, button := range bsMapMkLegacyToolbar {
		if !context.Resources.Exists(button.Image) {
			continue
		}
		layout.SystemButtons = append(layout.SystemButtons, button)
	}
	if context.Resources.Exists(bsMapMkLegacyNextTimeButton) {
		layout.SystemButtons = append(layout.SystemButtons, BsMapSystemButton{
			Image:  bsMapMkLegacyNextTimeButton,
			X:      bsMapMkLegacyNextTimeButtonX,
			Y:      bsMapMkLegacyNextTimeButtonY,
			Frames: bsMapMkLegacyNextTimeButtonFrame,
			Tool:   "next",
		})
	}
	core.Logf("script: map mk: %d card(s), #time=%d slot=%d backdrop='%s' mask=%v", len(items), timeValue, timeIndex, layout.Background, layout.Mask != nil)
	return layout
}

// bsMapMkLegacyCardCell 取角色/道具卡一帧的尺寸。
//
// 角色（miyu1_norm 等）与 map_item 都是 80x160 的两帧竖排图，但 BSG 头的
// FrameCount 不可靠；按高度折半比信任头字段稳。万一某张图高度是奇数，
// 就当单帧处理，宁可露出整张图也不要切出半像素的错位。
func bsMapMkLegacyCardCell(context BsMapContext, name string, cache map[string][2]int) (int, int) {
	if size, ok := cache[name]; ok {
		return size[0], size[1]
	}
	size := [2]int{bsMapMkLegacyDefaultCardWidth, bsMapMkLegacyDefaultCardHeight}
	if width, height := bsMapMkLegacyImageSize(context, name); width > 0 && height > 0 {
		if height%bsMapMkLegacyCardFrameCount == 0 {
			size = [2]int{width, height / bsMapMkLegacyCardFrameCount}
		} else {
			size = [2]int{width, height}
		}
	}
	cache[name] = size
	return size[0], size[1]
}

// bsMapMkLegacyImageSize 从 BSG 头读宽高（0x16/0x18 各两字节小端）。
// 资源索引只给压缩后的字节，不能依赖解码后的图对象，所以直接读前缀。
func bsMapMkLegacyImageSize(context BsMapContext, name string) (int, int) {
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

func bsMapMkLegacyMask(context BsMapContext) *BsMapMaskData {
	location, ok := context.Resources.TryGet(bsMapMkLegacyHitMaskImage)
	if !ok {
		return nil
	}
	width, height, values, ok := gfx.ReadBsgIndexedPixels(location.Read())
	if !ok || width <= 0 || height <= 0 {
		core.Logf("script: map mk: '%s' is not an indexed mask", bsMapMkLegacyHitMaskImage)
		return nil
	}
	// 256 项覆盖 8 位调色板下标；255（不可点）与 10..14（下一时段/工具条）
	// 默认保持 -1，由上面的映射循环只点亮 0..9。
	itemForValue := make([]int, 256)
	for index := range itemForValue {
		itemForValue[index] = -1
	}
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

func resolveBsMapMkLegacyTime(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	value := context.Properties.ByName(mapData.TimeProperty)
	if value < 0 {
		return -1, value
	}
	return mapData.TimeSlot(value), value
}

func resolveBsMapMkLegacyResultKey(context BsMapContext, name string) int {
	if context.Script != nil && len(name) > 0 {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapMkLegacyResultFallback
}
