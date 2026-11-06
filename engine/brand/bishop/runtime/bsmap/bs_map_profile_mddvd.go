package bsmap

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

type BsMapProfileMddvd struct{}

const (
	BsMapMddvdHitMaskImage       = "map_hit"
	BsMapMddvdBackgroundImage    = "map_base"
	BsMapMddvdFrameImage         = "map_frame"
	BsMapMddvdStationProperty    = "station_number"
	BsMapMddvdStationPropertyKey = 31
	// map_hit 的 0..7 是 8 个车站：原脚本按 station_number 1..8 分派
	// （0+1=1 … 7+1=8）。0 号格在 (483,148)，旧实现把它当背景跳过，
	// 结果 8 个车站只登记了 7 个，第 1 站永远点不到、后续分支错位。
	BsMapMddvdFirstMaskValue     = 0
	BsMapMddvdLastMaskValue      = 7
	BsMapMddvdTimeMaskValue      = 8
	BsMapMddvdMaxMaskValue       = 255
	BsMapMddvdNoItemIndex        = -1
	BsMapMddvdHitPadding         = 2
	BsMapMddvdDecorationHitX     = -4096
	BsMapMddvdDecorationHitY     = -4096
	BsMapMddvdStationValueOffset = 1
)

const bsMapMddvdPixelChannels = 4

func (p *BsMapProfileMddvd) Name() string {
	return "Mddvd"
}

func (p *BsMapProfileMddvd) Matches(context BsMapContext) bool {
	if !context.HasDialect {
		// 现代签名（BSXScript 2.x/3.x）没有 Bs 方言，不能当 Rev 处理。
		return false
	}
	return context.Dialect == bs.BsScriptAncientWide &&
		context.Resources.Exists(BsMapMddvdHitMaskImage) &&
		context.Resources.Exists(BsMapMddvdBackgroundImage)
}

func (p *BsMapProfileMddvd) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("script: map mddvd: build failed: %s", bsMapRecoverMessage(recovered))
			result = nil
		}
	}()
	return createBsMapMddvdLayout(context)
}

func createBsMapMddvdLayout(context BsMapContext) *BsMapLayout {
	mask := BsMapMaskLoad(context.Resources, BsMapMddvdHitMaskImage)
	if mask == nil {
		core.Logf("script: map mddvd: '%s' is not available", BsMapMddvdHitMaskImage)
		return nil
	}
	core.Logf("script: map mddvd: '%s' %dx%d indexed without a palette, so blue, green and red all hold the station id (255 = background); hit test uses the blue channel", BsMapMddvdHitMaskImage, mask.Width, mask.Height)
	items := bsMapMddvdStations(bsMapMddvdRegions(mask))
	if len(items) == 0 {
		core.Logf("script: map mddvd: '%s' has no cell %d..%d", BsMapMddvdHitMaskImage, BsMapMddvdFirstMaskValue, BsMapMddvdLastMaskValue)
		return nil
	}
	// 右下角的「時間を進める」是 map_hit 的 8 号格：脚本没有 1..8 的
	// station_number 分支时落到"什么也没发生、时间+1"的公共尾段，所以这里
	// 把它当成写 0 的普通地图项，和车站共用同一条写回路径。
	timeItem := NewBsMapItemWithProperty("", 0, 0, 0, 596, 439, 160, 40, "",
		BsMapMddvdStationProperty, BsMapMddvdStationPropertyKey, 0)
	timeIndex := len(items)
	items = append(items, timeItem)
	hitMask := bsMapMddvdMaskData(mask, items, timeIndex)
	items = addBsMapMddvdFrame(items, context)
	if hitMask == nil {
		// 没有像素掩码时仍保留矩形命中；timeItem 不在掩码里也能按矩形点到。
		core.Logf("script: map mddvd: pixel hit disabled, using rectangles")
	}
	core.Logf("script: map mddvd: %d station(s) %s", len(items), summarizeBsMapMddvd(items))
	return &BsMapLayout{
		Background: BsMapMddvdBackgroundImage,
		Items:      items,
		Source:     BsMapMddvdHitMaskImage,
		Mask:       hitMask,
		NextValue:  0,
		SystemButtons: []BsMapSystemButton{
			{Hit: [4]int{16, 392, 64, 64}, Tool: "qsave"},
			{Hit: [4]int{16, 456, 64, 64}, Tool: "qload"},
			{Hit: [4]int{16, 520, 64, 64}, Tool: "config"},
			{Hit: [4]int{80, 456, 64, 64}, Tool: "log"},
			{Hit: [4]int{80, 520, 64, 64}, Tool: "save"},
			{Hit: [4]int{144, 520, 64, 64}, Tool: "load"},
		},
	}
}

func bsMapMddvdMaskData(image *gfx.BsgImage, items []BsMapItem, timeIndex int) *BsMapMaskData {
	width := image.Width
	height := image.Height
	pixels := image.Pixels
	if width <= 0 || height <= 0 || len(pixels) < width*height*bsMapMddvdPixelChannels {
		core.Logf("script: map mddvd: '%s' declares %dx%d but holds %d bytes, pixel hit disabled, the bounding boxes stay in use", BsMapMddvdHitMaskImage, width, height, len(pixels))
		return nil
	}
	values := make([]byte, width*height)
	for index := range values {
		values[index] = pixels[(index*bsMapMddvdPixelChannels)+BsMapMaskBlueChannel]
	}
	itemForValue := make([]int, BsMapMddvdMaxMaskValue+1)
	for i := range itemForValue {
		itemForValue[i] = BsMapMddvdNoItemIndex
	}
	clickable := 0
	for index := range items {
		value := items[index].Value - BsMapMddvdStationValueOffset
		if value < BsMapMddvdFirstMaskValue || value > BsMapMddvdLastMaskValue || itemForValue[value] != BsMapMddvdNoItemIndex {
			continue
		}
		itemForValue[value] = index
		clickable++
	}
	// 8 号格是「時間を進める」，对应写 station_number=0 的 timeItem。
	if timeIndex >= 0 && timeIndex < len(items) {
		itemForValue[BsMapMddvdTimeMaskValue] = timeIndex
		clickable++
	}
	core.Logf("script: map mddvd: pixel hit mask %dx%d in screen coordinates, %d value(s) of %d..%d resolve to a station, %d is the time button", width, height, clickable, BsMapMddvdFirstMaskValue, BsMapMddvdLastMaskValue, BsMapMddvdTimeMaskValue)
	return &BsMapMaskData{Width: width, Height: height, Values: values, ItemForValue: itemForValue}
}

// bsMapMddvdRegions 在通用的 BsMapMaskRegions 之外补回 0 号车站。
//
// 通用实现把 0 当"无命中"跳过（其它地图的 0 确实是背景），但 MDDVD 的
// 0 号格是一个真正的车站（station_number=1）。这里单独找 0 值像素里
// 面积最大的连通块，作为 0 号区域；顶部 1px 的黑色边线面积很小，不会被选中。
func bsMapMddvdRegions(image *gfx.BsgImage) []BsMapMaskRegion {
	regions := BsMapMaskRegions(image, BsMapMaskBlueChannel)
	if zero, ok := bsMapMddvdLargestZeroRegion(image); ok {
		regions = append([]BsMapMaskRegion{zero}, regions...)
	}
	return regions
}

func bsMapMddvdLargestZeroRegion(image *gfx.BsgImage) (BsMapMaskRegion, bool) {
	width, height := image.Width, image.Height
	pixels := image.Pixels
	if width <= 0 || height <= 0 || len(pixels) < width*height*bsMapMddvdPixelChannels {
		return BsMapMaskRegion{}, false
	}
	visited := make([]bool, width*height)
	bestArea, best := 0, BsMapMaskRegion{}
	queue := make([]int, 0, width*height)
	for start := 0; start < width*height; start++ {
		if visited[start] {
			continue
		}
		index := start * bsMapMddvdPixelChannels
		if pixels[index+3] == 0 || pixels[index+BsMapMaskBlueChannel] != 0 {
			continue
		}
		visited[start] = true
		queue = queue[:0]
		queue = append(queue, start)
		minX, minY, maxX, maxY, area := start%width, start/width, start%width, start/width, 0
		for len(queue) > 0 {
			current := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			x, y := current%width, current/width
			area++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			for _, delta := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+delta[0], y+delta[1]
				if nx < 0 || ny < 0 || nx >= width || ny >= height {
					continue
				}
				next := ny*width + nx
				if visited[next] {
					continue
				}
				offset := next * bsMapMddvdPixelChannels
				if pixels[offset+3] == 0 || pixels[offset+BsMapMaskBlueChannel] != 0 {
					continue
				}
				visited[next] = true
				queue = append(queue, next)
			}
		}
		if area > bestArea {
			bestArea = area
			best = BsMapMaskRegion{Value: 0, X: minX, Y: minY, Width: maxX - minX + 1, Height: maxY - minY + 1}
		}
	}
	// 车站格至少几十像素宽；1px 的边框线即使面积大也不该被当成车站。
	return best, bestArea > 200 && best.Width > 10 && best.Height > 10
}

func addBsMapMddvdFrame(items []BsMapItem, context BsMapContext) []BsMapItem {
	if !context.Resources.Exists(BsMapMddvdFrameImage) {
		return items
	}
	items = append(items, NewBsMapItemDecor(BsMapMddvdFrameImage, 0, 0, BsMapMddvdDecorationHitX, BsMapMddvdDecorationHitY))
	core.Logf("script: map mddvd: frame '%s'", BsMapMddvdFrameImage)
	return items
}

func bsMapMddvdStations(regions []BsMapMaskRegion) []BsMapItem {
	items := make([]BsMapItem, 0, BsMapMddvdLastMaskValue-BsMapMddvdFirstMaskValue+1)
	for _, region := range regions {
		if region.Value < BsMapMddvdFirstMaskValue || region.Value > BsMapMddvdLastMaskValue {
			continue
		}
		items = append(items, NewBsMapItemWithProperty(
			"", 0, region.X, region.Y,
			region.X-BsMapMddvdHitPadding, region.Y-BsMapMddvdHitPadding,
			region.Width+(BsMapMddvdHitPadding*2), region.Height+(BsMapMddvdHitPadding*2),
			"", BsMapMddvdStationProperty, BsMapMddvdStationPropertyKey, region.Value+BsMapMddvdStationValueOffset))
	}
	return items
}

func summarizeBsMapMddvd(items []BsMapItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, "mask "+strconv.Itoa(item.Value-BsMapMddvdStationValueOffset)+" ("+strconv.Itoa(item.HitX)+","+strconv.Itoa(item.HitY)+","+strconv.Itoa(item.HitWidth)+","+strconv.Itoa(item.HitHeight)+")")
	}
	return strings.Join(parts, ", ")
}
