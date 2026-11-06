package bsmap

import (
	"os"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

// BsMapProfileSansha 处理「三射面談」（SM / SMIE）的地图。
//
// 这两部作品的脚本是 BSXScript 2.0 / 3.1（现代指令集，没有 Bs 方言），
// 但地图仍然由根目录的 map.dat 驱动：
//   - 8 个地点，位置来自 map.dat 的 Places[].BaseX/Y + Chunks[]；
//   - 5 名女主角 + 3 条路线共 8 张角色表，表项低字节=地点、高字节=头像图；
//   - 另有 18 张道具表：#item_map 为 0 时把 map_item 画在地点上。
//
// 所有表的点击结果都写 #result = 表序号 1..26：脚本先把 1..8 分派给
// map_aina…map_root3，剩下的 9..26 交给 map_item 子程序。地点只决定图标
// 画在哪，不参与 #result 的取值。
type BsMapProfileSansha struct{}

const (
	bsMapSanshaBackgroundImage = "map_base"
	bsMapSanshaOverlayImage    = "map_on"
	bsMapSanshaOverlayHover    = "map_over"
	bsMapSanshaFrameImage      = "map_frame"
	bsMapSanshaHitMaskImage    = "map_hit"
	bsMapSanshaPlaceImage      = "map_ch101"
	bsMapSanshaItemImage       = "map_item"
	bsMapSanshaResultKey       = 9 // 剧本里查不到 #result 时的回退槽位
	bsMapSanshaDefaultCard     = 64
	// bsMapSanshaMaskPlaceShift 把 map_hit 的像素值换算成地点号：
	// 0=地点1 … 7=地点8，255=不可选。
	bsMapSanshaMaskPlaceShift = 1
	bsMapSanshaPackedPlace    = 0xFF
	bsMapSanshaPackedImage    = 8
)

func (p *BsMapProfileSansha) Name() string {
	return "Sansha"
}

// Matches 用素材指纹锁定三射面談。
//
// 不能用方言：SM/SMIE 是现代签名，HasDialect=false。UK 也用 map.dat + map_base +
// map_hit，但 UK 的头像是 map_f100…；这里要求 map_ch101 且没有 map_f100，
// 两代互不误吃。Houkago2/Mk2 是 Rev 系方言，早已被 HasDialect 挡掉。
func (p *BsMapProfileSansha) Matches(context BsMapContext) bool {
	if context.HasDialect {
		return false
	}
	if len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	return context.Resources.Exists(bsMapSanshaPlaceImage) &&
		context.Resources.Exists(bsMapSanshaBackgroundImage) &&
		context.Resources.Exists(bsMapSanshaHitMaskImage) &&
		!context.Resources.Exists("map_f100")
}

func (p *BsMapProfileSansha) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map sansha: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapSanshaLayout(context)
}

type bsMapSanshaNode struct {
	image    string
	property string
	key      int
	value    int
}

func createBsMapSanshaLayout(context BsMapContext) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	timeIndex, timeValue := resolveBsMapSanshaTime(context, mapData)
	if timeIndex < 0 {
		// 地图只能在脚本写好 #time 后打开；映射不到时段时不要跳过地图，
		// 退到第一格并把原因写进日志，否则整块玩法会像以前一样被静默跳过。
		core.Logf("script: map sansha: '%s' = %d is not one of the map.dat slots, using slot 0", mapData.TimeProperty, timeValue)
		timeIndex = 0
	}
	resultKey := resolveBsMapSanshaResultKey(context, mapData.ResultProperty)
	nodes := make([][]bsMapSanshaNode, len(mapData.Places)+1)
	characters, items := 0, 0
	for index := range mapData.Tables {
		table := mapData.Tables[index]
		// state != 0 表示这张表当前不生效（角色不在图上 / 道具已收走）。
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
		place := packed & bsMapSanshaPackedPlace
		if place < 1 || place >= len(nodes) {
			continue
		}
		image := mapData.ImageName(packed>>bsMapSanshaPackedImage, context.Resources)
		if len(image) == 0 {
			continue
		}
		// 所有表都写 #result = 表序号 1..26：
		//   1..8  角色表，脚本直接调 map_aina…map_root3；
		//   9..26 道具表，落到 map_item 子程序按 #result 分派。
		node := bsMapSanshaNode{
			image:    image,
			property: mapData.ResultProperty,
			key:      resultKey,
			value:    index + 1,
		}
		if strings.EqualFold(image, bsMapSanshaItemImage) {
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
	cardCells := map[string][2]int{}
	for placeIndex := 1; placeIndex < len(nodes); placeIndex++ {
		group := nodes[placeIndex]
		if len(group) == 0 {
			continue
		}
		place := mapData.Places[placeIndex-1]
		// 同一地点站 k 名角色时，原引擎使用三角排布的 k 个 chunk：
		// 1 人从第 0 格、2 人从第 1 格、3 人从第 3 格……这样头像横向排开
		// 而不是全部叠在同一个圆点上。
		offset := len(group) * (len(group) - 1) / 2
		for index := range group {
			chunkIndex := offset + index
			if chunkIndex >= len(place.Chunks) {
				core.Logf("script: map sansha: place %d needs chunk %d, only %d available", placeIndex, chunkIndex, len(place.Chunks))
				break
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			width, height := bsMapSanshaCardCell(context, group[index].image, cardCells)
			if firstItemForPlace[placeIndex] < 0 {
				firstItemForPlace[placeIndex] = len(itemsList)
			}
			itemsList = append(itemsList, NewBsMapItemWithProperty(
				group[index].image, 0, x, y, x, y, width, height,
				"", group[index].property, group[index].key, group[index].value))
		}
	}

	mask := bsMapSanshaMask(context)
	if mask != nil {
		// map_hit 的值是"地点号 - 1"：0 是有效地点（地点1），不能沿用
		// 学園2/UK 的"0=空白"约定。
		for value := range mask.ItemForValue {
			mask.ItemForValue[value] = -1
		}
		for placeIndex := 1; placeIndex < len(firstItemForPlace); placeIndex++ {
			value := placeIndex - bsMapSanshaMaskPlaceShift
			if value < 0 || value >= len(mask.ItemForValue) || firstItemForPlace[placeIndex] < 0 {
				continue
			}
			mask.ItemForValue[value] = firstItemForPlace[placeIndex]
		}
	}

	core.Logf("script: map sansha: %d character(s), %d item(s), #time=%d slot=%d mask=%v",
		characters, items, timeValue, timeIndex, mask != nil)
	return &BsMapLayout{
		Background:        bsMapSanshaBackgroundImage,
		Overlay:           bsMapSanshaOverlayImage,
		OverlayHover:      bsMapSanshaOverlayHover,
		FrameImage:        bsMapSanshaFrameImage,
		Items:             itemsList,
		Source:            BsMapDataFileName,
		Mask:              mask,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		TimeSlot:          timeIndex,
		TimeValue:         timeValue,
		TimeSlotValid:     true,
		DayVariable:       "#day",
		// 没有角色/道具可点时也必须打开地图：底图、时间/日期卡和
		// 「時間を進める」按钮仍然要能用，否则脚本会拿旧 #result 继续分派。
		AllowEmpty: true,
	}
}

// bsMapSanshaCardCell 取头像卡的一帧尺寸。
//
// map_ch* 是"常态 / 悬停"两帧竖排（SM 64x128、SMIE 80x160）。只读 BSG 头
// 而不解码整张图；头里没有帧数，所以按高度折半。尺寸异常时退回 64px。
func bsMapSanshaCardCell(context BsMapContext, name string, cache map[string][2]int) (int, int) {
	if size, ok := cache[name]; ok {
		return size[0], size[1]
	}
	size := [2]int{bsMapSanshaDefaultCard, bsMapSanshaDefaultCard}
	if location, ok := context.Resources.TryGet(name); ok {
		head := location.ReadPrefix(bsgHeaderProbeSize)
		if len(head) >= bsgHeaderProbeSize {
			width := int(binutil.U16(head[0x16:]))
			height := int(binutil.U16(head[0x18:]))
			if width > 0 && height > 1 {
				frames := 1
				if height%2 == 0 {
					frames = 2
				}
				size = [2]int{width, height / frames}
			}
		}
	}
	cache[name] = size
	return size[0], size[1]
}

const bsgHeaderProbeSize = 0x40

// bsMapSanshaMask 读取 map_hit 的原始调色板下标。
//
// 这张图是 8 位索引图，调色板 alpha 全 0；按 BGRA 解码后既没有 alpha 也
// 无法可靠还原"地点号-1"，必须直接取索引。
func bsMapSanshaMask(context BsMapContext) *BsMapMaskData {
	location, ok := context.Resources.TryGet(bsMapSanshaHitMaskImage)
	if !ok {
		return nil
	}
	width, height, values, ok := gfx.ReadBsgIndexedPixels(location.Read())
	if !ok || width <= 0 || height <= 0 {
		core.Logf("script: map sansha: '%s' is not an indexed mask", bsMapSanshaHitMaskImage)
		return nil
	}
	itemForValue := make([]int, 256)
	for index := range itemForValue {
		itemForValue[index] = -1
	}
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

func resolveBsMapSanshaTime(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	value := context.Properties.ByName(mapData.TimeProperty)
	if value < 0 {
		return -1, value
	}
	return mapData.TimeSlot(value), value
}

func resolveBsMapSanshaResultKey(context BsMapContext, name string) int {
	if context.Script != nil && len(name) > 0 {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapSanshaResultKey
}
