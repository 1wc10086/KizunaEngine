package bsmap

import (
	"os"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// BsMapProfileUk 处理「裏教師」（UK / BSXScript 2.0）与 UKIE（BSXScript 3.1）
// 的地图。两代的 map.dat 完全同构：9 个地点、#time 4 个时段、
// 5 名角色 × A/B 两套时间表，共 10 张解锁表。
//
// map.dat 表项是 uint16 打包值，与 Mk2/Houkago2 的语义不同：
//   - 低字节 = 地点号 1..9（0 = 该角色不在图上），决定热区画在哪。
//   - 高字节 = 图像表下标 1..74。1..8 是 null 占位，9..74 对应
//     map_f100…map_f542 的角色头像（出现了谁就在哪个地点画谁）。
//
// 引擎写回 #result 的是表项序号 1..10（sionA, sionB, asamiA, asamiB, …），
// 不是地点号：剧本的 `map` 标签按 #result == 1..10 分派到 sion_mapA_result 等场景标签。
type BsMapProfileUk struct{}

const (
	bsMapUkBackgroundImage = "map_base"
	bsMapUkHitMaskImage    = "map_hit"
	// bsMapUkPlaceImageSample 是 UK 地图素材的指纹：map_f100 是
	// 「地点 1 的第一张角色头像」，UK 与 UKIE 都有（UK 的地点卡叫
	// plase_1..9，UKIE 改叫 Place_01..09，用头像图更稳）。其它有
	// map.dat 的作品（Kd/Mk2/Houkago2…）都没有这组 map_f* 头像。
	bsMapUkPlaceImageSample  = "map_f100"
	bsMapUkResultPropertyKey = 8 // 剧本里查不到 #result 时的回退槽位
	bsMapUkPlaceHitSize      = 64
	// bsMapUkPackedPlaceMask / bsMapUkPackedImageShift 拆 map.dat 表项的
	// 打包值：低字节 = 地点号，高字节 = 图像表下标。
	bsMapUkPackedPlaceMask  = 0xFF
	bsMapUkPackedImageShift = 8
	// bsMapUkNullImageName 是 map.dat 图像表里的空占位；名字不会出现在
	// 资源里，遇到它按"这个时段没有头像"处理。
	bsMapUkNullImageName = "null"
)

func (p *BsMapProfileUk) Name() string {
	return "Uk"
}

// Matches 用 map.dat + map_hit 掩码锁定 UK 系（裏教師 BSXScript 2.0 与
// UKIE 3.1）。UK/UKIE 是现代签名：HasDialect=false、BsDialect 返回默认值，
// 所以这里不能像 Rev 系 profile 那样检查方言——改用"素材指纹"分流：
// UK 的地图素材是 plase_1..9（其它有 map.dat 的作品用 map_ch*/sd_*/map_s1f
// 等），map.dat 的时段属性固定是 #time。Kd/Mk2/Houkago2 等由各自的 Matches
// 在更早处命中，不会被误认；反过来 UK 没有它们的特征素材。
func (p *BsMapProfileUk) Matches(context BsMapContext) bool {
	if len(context.GameRoot) == 0 {
		return false
	}
	if _, err := os.Stat(context.GameRoot + "/" + BsMapDataFileName); err != nil {
		return false
	}
	// HasDialect=true 说明是真正的 Rev/Ancient 方言——那是其它 profile
	// 的领地；UK 只服务现代签名（HasDialect=false）的 BSXScript 2.x/3.1。
	if context.HasDialect {
		return false
	}
	return context.Resources.Exists(bsMapUkBackgroundImage) &&
		context.Resources.Exists(bsMapUkHitMaskImage) &&
		context.Resources.Exists(bsMapUkPlaceImageSample)
}

func (p *BsMapProfileUk) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map uk: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapUkLayout(context)
}

type bsMapUkNode struct {
	place int // map.dat 地点序号（1..9），决定热区位置
	image string
	value int // 写回 #result 的表序号（1..10）
}

func createBsMapUkLayout(context BsMapContext) *BsMapLayout {
	if len(context.GameRoot) == 0 {
		return nil
	}
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		return nil
	}
	// UK 的可玩时段是 #time==2/3（午前/放課後），正好落在 map.dat 的
	// 四个时段槽位里；#time==1/4（朝/夜）不进地图。找不到槽位时退化为
	// 展开每张表的全部时段（宁可多画，也不要把地图整屏吞掉）。
	timeIndex, timeValue := resolveBsMapUkTimeIndex(context, mapData)
	if timeIndex < 0 {
		core.Logf("script: map uk: '%s' = %d is not one of the map.dat slots, expanding every table", mapData.TimeProperty, timeValue)
	}
	byPlace := make([][]bsMapUkNode, len(mapData.Places)+1)
	addBsMapUkTableNodes(context, mapData, timeIndex, byPlace)
	total := 0
	for _, nodes := range byPlace {
		total += len(nodes)
	}
	if total == 0 {
		// 没有任何角色在当前时段出门：原版仍显示底图和「時間を進める」，
		// 玩家可以跳过这一格。绝不能在这里返回空布局：Session 会跳过
		// SELECT_TAIL，脚本拿着上一张图的 #result 继续分派，可能重放
		// 已经看过的角色场景。AllowEmpty 让地图照常打开。
		core.Logf("script: map uk: no unlocked table entry at #time=%d, opening the map with system buttons only", timeValue)
	}
	resultKey := resolveBsMapUkResultKey(context, mapData.ResultProperty)
	items := make([]BsMapItem, 0, total)
	// firstItemForPlace[place] 记录该地点第一条热区的下标，供掩码按地点回指。
	firstItemForPlace := make([]int, len(mapData.Places)+1)
	for i := range firstItemForPlace {
		firstItemForPlace[i] = -1
	}
	for placeIndex := 1; placeIndex < len(byPlace); placeIndex++ {
		nodes := byPlace[placeIndex]
		if len(nodes) == 0 {
			continue
		}
		place := mapData.Places[placeIndex-1]
		// 同一地点可能站着多名角色。map.dat 的 chunk 表正是按
		// 「1 人 1 格、2 人 2 格、3 人 3 格…」的三角排布烘焙的：
		// k 个人从第 k(k-1)/2 格开始依次取 k 格。只取 chunk[0]
		// 会让同地点的头像完全重叠，点谁都只命中第一个。
		offset := len(nodes) * (len(nodes) - 1) / 2
		for index := range nodes {
			chunkIndex := offset + index
			if chunkIndex >= len(place.Chunks) {
				core.Logf("script: map uk: place %d needs chunk %d, only %d available", placeIndex, chunkIndex, len(place.Chunks))
				break
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			// 头像图是两帧竖排，热区高度要按一帧取，否则整条图都会被画出来。
			width, cell := bsMapUkFaceCell(context, nodes[index].image)
			if firstItemForPlace[placeIndex] < 0 {
				firstItemForPlace[placeIndex] = len(items)
			}
			items = append(items, NewBsMapItemWithProperty(
				nodes[index].image, 0, x, y, x, y, width, cell,
				"", mapData.ResultProperty, resultKey, nodes[index].value))
		}
	}
	mask := bsMapUkMask(context)
	if mask != nil {
		reindexBsMapUkMask(mask, firstItemForPlace)
	}
	core.Logf("script: map uk: %d item(s) on %d place(s) from %d table(s) (#time=%d slot=%d) mask=%v",
		len(items), len(byPlace)-1, len(mapData.Tables), timeValue, timeIndex, mask != nil)
	return &BsMapLayout{
		Background:        bsMapUkBackgroundImage,
		Items:             items,
		Source:            BsMapDataFileName,
		Mask:              mask,
		ResultProperty:    mapData.ResultProperty,
		ResultPropertyKey: resultKey,
		// 没有角色卡时也保留地图（底图 + [map phase] 的系統按钮）。
		AllowEmpty: true,
	}
}

// resolveBsMapUkResultKey 从剧本查 #result 的属性槽位。槽位序号随剧本变化，
// 写死会把结果写进别的变量；查不到才退回旧的 8。Script 为空只出现在单元测试里。
func resolveBsMapUkResultKey(context BsMapContext, name string) int {
	if context.Script != nil {
		if index := context.Script.GetPropertyIndex(name); index >= 0 {
			return index
		}
	}
	return bsMapUkResultPropertyKey
}

// bsMapUkFaceCell 取头像图的一帧高度。map_f* 是"常态/悬停"两帧竖排的方图，
// BSG 头的 FrameCount 恒为 1，不能信；按宽高比切：height 是 width 的整数倍时
// 一帧高 = width，否则退回旧的 64 方框（测试夹具/异常素材）。
func bsMapUkFaceCell(context BsMapContext, name string) (width, height int) {
	if location, ok := context.Resources.TryGet(name); ok {
		data := location.Read()
		if w, h, ok := gfx.TryReadBsgDimensions(data); ok && w > 0 && h >= w && h%w == 0 {
			return w, w
		}
	}
	return bsMapUkPlaceHitSize, bsMapUkPlaceHitSize
}

// addBsMapUkTableNodes 把每张解锁表在当前时段的一个表项登记到对应地点。
// value 用表序号（1..10），剧本按 #result == 表序号调用对应的场景标签。
func addBsMapUkTableNodes(context BsMapContext, mapData BsMapData, timeIndex int, byPlace [][]bsMapUkNode) {
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
		if selected == nil {
			continue
		}
		place, image := resolveBsMapUkSlot(context, mapData, selected.Values, timeIndex)
		if place < 1 || place >= len(byPlace) {
			continue
		}
		byPlace[place] = append(byPlace[place], bsMapUkNode{
			place: place,
			image: image,
			value: index + 1,
		})
	}
}

// resolveBsMapUkSlot 取出当前时段的 (地点, 头像图)。
//
// timeIndex < 0 时（#time 不在 map.dat 的槽位里）取该行第一个非空槽：
// 这种退化只在数据异常时发生，目的仍是让地图有东西可点。
func resolveBsMapUkSlot(context BsMapContext, mapData BsMapData, slots []uint16, timeIndex int) (int, string) {
	candidates := slots
	if timeIndex >= 0 {
		if timeIndex >= len(slots) {
			return 0, ""
		}
		candidates = slots[timeIndex : timeIndex+1]
	}
	for _, packed := range candidates {
		place := int(packed & bsMapUkPackedPlaceMask)
		if place < 1 || place > len(mapData.Places) {
			continue
		}
		imageIndex := int(packed >> bsMapUkPackedImageShift)
		image := mapData.ImageName(imageIndex, context.Resources)
		if image != "" && strings.EqualFold(image, bsMapUkNullImageName) {
			image = ""
		}
		return place, image
	}
	return 0, ""
}

func resolveBsMapUkTimeIndex(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	timeValue := context.Properties.ByName(mapData.TimeProperty)
	if timeValue < 0 {
		return -1, timeValue
	}
	return mapData.TimeSlot(timeValue), timeValue
}

// bsMapUkMask 把 map_hit 的蓝通道拆成逐像素地点号表。
func bsMapUkMask(context BsMapContext) *BsMapMaskData {
	image := BsMapMaskLoad(context.Resources, bsMapUkHitMaskImage)
	if image == nil {
		return nil
	}
	return bsMapUkMaskData(image)
}

func bsMapUkMaskData(image *gfx.BsgImage) *BsMapMaskData {
	width := image.Width
	height := image.Height
	pixels := image.Pixels
	if width <= 0 || height <= 0 || len(pixels) < width*height*4 {
		return nil
	}
	values := make([]byte, width*height)
	for index := range values {
		values[index] = pixels[index*4+BsMapMaskBlueChannel]
	}
	itemForValue := make([]int, 256)
	for i := range itemForValue {
		itemForValue[i] = -1
	}
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

// reindexBsMapUkMask 把 map_hit 的掩码值映射到该地点的第一条热区。
// map_hit 存的是"地点号 - 1"（校舎=0 … 裏庭=8），不是地点号本身；255 表示没有地点。
// 每个地点只登记第一条热区：同一地点有多名角色时，精确点选交给 Session 的矩形命中，
// 掩码只负责判断"点到了哪个地点"。
func reindexBsMapUkMask(mask *BsMapMaskData, firstItemForPlace []int) {
	for value := range mask.ItemForValue {
		mask.ItemForValue[value] = -1
	}
	for place := 1; place < len(firstItemForPlace); place++ {
		value := place - 1
		if value < 0 || value >= len(mask.ItemForValue) {
			continue
		}
		if firstItemForPlace[place] >= 0 {
			mask.ItemForValue[value] = firstItemForPlace[place]
		}
	}
}
