package bsmap

import (
	"os"
	"strconv"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

// BsMapProfileKs 处理「教育指導」（KS / BSXScript 1.0）与「教育指導 IE」
// （KSIE / BSXScript 3.1）的地图。
//
// 两代共用同一份 map.dat（逐字节相同），脚本也共用同一套流程：
// afternoon_01 先 PlayBgm，再 `frame[1] = "map"` 发 SELECT_TAIL；引擎打开
// map.dat 驱动的地图，玩家点角色/道具后把表序号写回 #result，脚本再按
// 1..5 调 map_kiyoka…map_eriko、6..19 调 map_item。
//
// map.dat 的表项是 uint16 打包值，与本作以外的 map.dat 作品都不冲突：
//   - 低字节 = 地点号 1..9（决定热区画在哪个 map.dat 地点上）。
//   - 高字节 = 图像表下标；9..38 是 map_ch11…map_ch55（每名角色 6 张差分），
//     39 是 map_item（宝箱图标）。
//   - 表 0..4 是 5 名角色，state 属性是 #X_map：脚本在当天的地图事件结束后
//     把它置 1，表示这名角色已经从地图上消失；main 每天开头清零。
//   - 表 5..18 是 14 个道具，state 共用 #item_map，进度属性是 #X_itemN。
//
// 时间轴是 map.dat 的 TimeValues（#time 4..7 对应 0..3 槽），与三射面談
// 相同；但本作的卡片是 64x128（常态/悬停两帧竖排），道具也是同一尺寸，
// 而不是三射面談的 64x64/80x80 单帧。
type BsMapProfileKs struct{}

const (
	bsMapKsBackgroundImage = "map_base"
	bsMapKsOverlayImage    = "map_on"
	bsMapKsOverlayHover    = "map_over"
	bsMapKsFrameImage      = "map_frame"
	bsMapKsHitMaskImage    = "map_hit"
	bsMapKsPlaceImage      = "map_ch11"
	bsMapKsItemImage       = "map_item"
	bsMapKsResultKey       = 4 // 剧本里查不到 #result 时的回退槽位
	bsMapKsDefaultCard     = 64
	// bsMapKsMaskPlaceShift 把 map_hit 的像素值换算成地点号：
	// 0=地点1 … 8=地点9，255=不可选。注意 0 是有效地点，不能当空白。
	bsMapKsMaskPlaceShift = 1
	bsMapKsPackedPlace    = 0xFF
	bsMapKsPackedImage    = 8
	// bsMapKsPlaceBannerGap 是地点名条与角色卡底边的间距。原版把
	// 「学生棟N階」的 262x50 条贴在卡片正下方，横向以卡片组中心对齐。
	bsMapKsPlaceBannerGap = 2
)

func (p *BsMapProfileKs) Name() string {
	return "Ks"
}

// Matches 用素材指纹锁定 KS/KSIE。
//
// 两代都是现代签名（HasDialect=false），不能用方言分流；map_base/map_hit
// 也是三射面談的素材名，所以再要求本作独有的 map_ch11 + map_item，并排除
// 三射面談的 map_ch101 与裏教師的 map_f100。地图上的地点名条
// （plase_1 / map_plase_marker1）是第二道保险：其它 map.dat 作品没有这组图。
func (p *BsMapProfileKs) Matches(context BsMapContext) bool {
	if context.HasDialect || len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	if !context.Resources.Exists(bsMapKsPlaceImage) ||
		!context.Resources.Exists(bsMapKsItemImage) ||
		!context.Resources.Exists(bsMapKsBackgroundImage) ||
		!context.Resources.Exists(bsMapKsHitMaskImage) {
		return false
	}
	if context.Resources.Exists("map_ch101") || context.Resources.Exists("map_f100") {
		return false
	}
	return context.Resources.Exists("plase_1") || context.Resources.Exists("map_plase_marker1")
}

func (p *BsMapProfileKs) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map ks: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapKsLayout(context)
}

type bsMapKsNode struct {
	image string
	value int
	item  bool
}

// bsMapKsPlaceCards 记录一个地点上角色卡的包围盒，用来把地点名条
// 居中贴在卡片组正下方。只在 hasCharacter 时画名条：只有宝箱的地点
// 原版不显示地名条。
type bsMapKsPlaceCards struct {
	minX, maxX int
	bottomY    int
	cardW      int
	cardH      int
	hasChar    bool
}

func createBsMapKsLayout(context BsMapContext) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	timeIndex, timeValue := resolveBsMapKsTime(context, mapData)
	if timeIndex < 0 {
		// 映射不到时段时不要跳过地图：底图、日期卡和系统按钮仍然要能用，
		// 退到第一格并在日志里留原因，避免整块玩法被静默吃掉。
		core.Logf("script: map ks: '%s' = %d is not one of the map.dat slots, using slot 0", mapData.TimeProperty, timeValue)
		timeIndex = 0
	}
	resultKey := resolveBsMapKsResultKey(context, mapData.ResultProperty)
	nodes := make([][]bsMapKsNode, len(mapData.Places)+1)
	characters, items := 0, 0
	for index := range mapData.Tables {
		table := mapData.Tables[index]
		// state != 0 表示这张表当前不生效：角色当天的事件已经结束
		// （#X_map=1），或道具被收走。main 每天开头会把它们清零。
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
		place := packed & bsMapKsPackedPlace
		if place < 1 || place >= len(nodes) {
			continue
		}
		image := mapData.ImageName(packed>>bsMapKsPackedImage, context.Resources)
		if len(image) == 0 {
			continue
		}
		node := bsMapKsNode{image: image, value: index + 1, item: image == bsMapKsItemImage}
		if node.item {
			items++
		} else {
			characters++
		}
		nodes[place] = append(nodes[place], node)
	}

	itemsList := make([]BsMapItem, 0, characters+items)
	firstItemForPlace := make([]int, len(nodes))
	for index := range firstItemForPlace {
		firstItemForPlace[index] = -1
	}
	placeCards := make([]bsMapKsPlaceCards, len(nodes))
	cellCache := map[string][2]int{}
	for placeIndex := 1; placeIndex < len(nodes); placeIndex++ {
		group := nodes[placeIndex]
		if len(group) == 0 {
			continue
		}
		place := mapData.Places[placeIndex-1]
		// 同一地点站 k 名角色时，原引擎按三角排布的 k 个 chunk 摆卡：
		// 1 人从第 0 格、2 人从第 1 格、3 人从第 3 格……这样人物横向排开，
		// 而不是全部叠在同一个地点圆点上。
		offset := len(group) * (len(group) - 1) / 2
		for index := range group {
			chunkIndex := offset + index
			if chunkIndex >= len(place.Chunks) {
				core.Logf("script: map ks: place %d needs chunk %d, only %d available", placeIndex, chunkIndex, len(place.Chunks))
				break
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			width, height := bsMapKsCardCell(context, group[index].image, cellCache)
			if firstItemForPlace[placeIndex] < 0 {
				firstItemForPlace[placeIndex] = len(itemsList)
			}
			itemsList = append(itemsList, NewBsMapItemWithProperty(
				group[index].image, 0, x, y, x, y, width, height,
				"", mapData.ResultProperty, resultKey, group[index].value))
			if !group[index].item {
				cards := &placeCards[placeIndex]
				if !cards.hasChar {
					cards.minX, cards.maxX = x, x
					cards.bottomY = y + height
					cards.cardW, cards.cardH = width, height
					cards.hasChar = true
				} else {
					cards.minX = min(cards.minX, x)
					cards.maxX = max(cards.maxX, x)
					cards.bottomY = max(cards.bottomY, y+height)
				}
			}
		}
	}

	// 地点名条（plase_1..9 / map_plase_marker1..9）只画不点：原版把它
	// 贴在角色卡组正中下方，作为地名提示；点击仍由 map_hit 掩码决定。
	decor := make([]BsMapItem, 0, len(placeCards))
	for placeIndex := 1; placeIndex < len(placeCards); placeIndex++ {
		cards := placeCards[placeIndex]
		if !cards.hasChar {
			continue
		}
		name := bsMapKsPlaceBannerName(context, placeIndex)
		if name == "" {
			continue
		}
		bannerW, _ := bsMapKsImageSize(context, name)
		center := cards.minX + (cards.maxX+cards.cardW-cards.minX)/2
		drawX := center - bannerW/2
		drawY := cards.bottomY + bsMapKsPlaceBannerGap
		decor = append(decor, NewBsMapItemDecor(name, drawX, drawY, 0, 0))
	}

	mask := bsMapKsMask(context)
	if mask != nil {
		for value := range mask.ItemForValue {
			mask.ItemForValue[value] = -1
		}
		for placeIndex := 1; placeIndex < len(firstItemForPlace); placeIndex++ {
			value := placeIndex - bsMapKsMaskPlaceShift
			if value < 0 || value >= len(mask.ItemForValue) || firstItemForPlace[placeIndex] < 0 {
				continue
			}
			mask.ItemForValue[value] = firstItemForPlace[placeIndex]
		}
	}

	core.Logf("script: map ks: %d character(s), %d item(s), %d place banner(s), #time=%d slot=%d mask=%v",
		characters, items, len(decor), timeValue, timeIndex, mask != nil)
	return &BsMapLayout{
		Background:        bsMapKsBackgroundImage,
		Overlay:           bsMapKsOverlayImage,
		OverlayHover:      bsMapKsOverlayHover,
		FrameImage:        bsMapKsFrameImage,
		Items:             itemsList,
		Decor:             decor,
		Source:            BsMapDataFileName,
		Mask:              mask,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		TimeSlot:          timeIndex,
		TimeValue:         timeValue,
		TimeSlotValid:     true,
		DayVariable:       "#day",
		// 没有任何角色/道具可点时也打开地图：脚本会重发 SELECT_TAIL
		// "map"，底图/日期卡/「時間を進める」仍然可用，避免 #result 残留。
		AllowEmpty: true,
	}
}

// bsMapKsCardCell 取角色卡一帧的尺寸。
//
// map_ch* / map_item 是 64x128 的两帧竖排图（常态 / 悬停），但 BSG 头里的
// FrameCount 恒为 1，所以按高度折半。尺寸异常时退回 64x64。
func bsMapKsCardCell(context BsMapContext, name string, cache map[string][2]int) (int, int) {
	if size, ok := cache[name]; ok {
		return size[0], size[1]
	}
	size := [2]int{bsMapKsDefaultCard, bsMapKsDefaultCard}
	if width, height := bsMapKsImageSize(context, name); width > 0 && height > 0 {
		if height%2 == 0 {
			size = [2]int{width, height / 2}
		} else {
			size = [2]int{width, height}
		}
	}
	cache[name] = size
	return size[0], size[1]
}

func bsMapKsImageSize(context BsMapContext, name string) (int, int) {
	if len(name) == 0 {
		return 0, 0
	}
	location, ok := context.Resources.TryGet(name)
	if !ok {
		return 0, 0
	}
	head := location.ReadPrefix(bsgHeaderProbeSize)
	if len(head) < bsgHeaderProbeSize {
		return 0, 0
	}
	return int(binutil.U16(head[0x16:])), int(binutil.U16(head[0x18:]))
}

func bsMapKsPlaceBannerName(context BsMapContext, place int) string {
	// KSIE 的 [map phase] 写 plase_N = map_plase_markerN，KS 的发行版
	// 没有这段配置但素材就叫 plase_N；两种命名都接受，先新后旧。
	for _, name := range []string{
		"map_plase_marker" + strconv.Itoa(place),
		"plase_" + strconv.Itoa(place),
	} {
		if context.Resources.Exists(name) {
			return name
		}
	}
	return ""
}

func bsMapKsMask(context BsMapContext) *BsMapMaskData {
	location, ok := context.Resources.TryGet(bsMapKsHitMaskImage)
	if !ok {
		return nil
	}
	width, height, values, ok := gfx.ReadBsgIndexedPixels(location.Read())
	if !ok || width <= 0 || height <= 0 {
		core.Logf("script: map ks: '%s' is not an indexed mask", bsMapKsHitMaskImage)
		return nil
	}
	itemForValue := make([]int, 256)
	for index := range itemForValue {
		itemForValue[index] = -1
	}
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

func resolveBsMapKsTime(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	value := context.Properties.ByName(mapData.TimeProperty)
	if value < 0 {
		return -1, value
	}
	return mapData.TimeSlot(value), value
}

func resolveBsMapKsResultKey(context BsMapContext, name string) int {
	if context.Script != nil && len(name) > 0 {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapKsResultKey
}
