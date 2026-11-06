package bsmap

import (
	"strings"

	"kizuna/engine/brand/bishop/vm/bs"
)

// bs_map_profile_himedorei.go 处理「姫奴隷～囚われの姫君達～」（ヒメドレイ，
// BSScript Rev.6）的行动地图。
//
// 这部作品的 map.dat 与 MK2/放課後2 同构，但人物图标和地点卡是
// map_p01..18 / map_ch11..51 的 100x100 两帧小卡，地点位置和角色归属由
// map.dat 的 9 个 place + 18 张解锁表描述。原引擎的地图选择最终仍落到
// 脚本的 tht_map/lyz_map/est_map/mly_map 四个标签上；本 profile 因此保留
// label scan 的标签点击语义（已经验证能进入对应角色剧情），只补上原版的
// 地图底图、昼夜叠加层、底部六颗系统按钮和右上角的「時間を進める」。
//
// 不直接按 map.dat 生成 18 张角色卡的原因：脚本的 Map 例程先把选择参数
// 映射成 #result 1..18，再按结果分派；参数编码（place/character）还没有
// 逐值核对，贸然写 #result 会让角色场景错位。标签点击不依赖这套编码，
// 是当前唯一可验证安全的路径。
type BsMapProfileHimedorei struct{}

const (
	bsMapHimedoreiBase1      = "map_base1"
	bsMapHimedoreiBase2      = "map_base2"
	bsMapHimedoreiBase3      = "map_base3"
	bsMapHimedoreiOver1      = "map_over1"
	bsMapHimedoreiOver2      = "map_over2"
	bsMapHimedoreiOver3      = "map_over3"
	bsMapHimedoreiHit        = "map_hit"
	bsMapHimedoreiMenuButton = "map_menu_button"
	bsMapHimedoreiNext       = "map_next"
	bsMapHimedoreiNextValue  = 9999
)

func (p *BsMapProfileHimedorei) Name() string {
	return "Himedorei"
}

func (p *BsMapProfileHimedorei) Matches(context BsMapContext) bool {
	if !context.HasDialect || context.Dialect != bs.BsScriptRev6 {
		return false
	}
	// map_base1/map_ch11 是这一代独有的 100x200 小卡素材；MK2 的
	// map_ch101、放課後2 的 map_base 都不会同时满足。
	return context.Resources.Exists(bsMapHimedoreiBase1) &&
		context.Resources.Exists(bsMapHimedoreiBase2) &&
		context.Resources.Exists(bsMapHimedoreiBase3) &&
		context.Resources.Exists("map_ch11") &&
		context.Resources.Exists(bsMapHimedoreiHit) &&
		context.Resources.Exists(bsMapHimedoreiMenuButton)
}

func (p *BsMapProfileHimedorei) Build(context BsMapContext) (result *BsMapLayout) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
		}
	}()
	var mapData BsMapData
	if !TryLoadBsMapData(context.GameRoot, &mapData) {
		// map.dat 损坏或版本不符时退回旧的标签列表，至少保留可点路线。
		return (&BsMapProfileLabelScan{}).Build(context)
	}
	timeIndex, timeValue := himedoreiTimeSlot(context, mapData)
	layout := &BsMapLayout{
		Items: []BsMapItem{}, Decor: []BsMapItem{},
		FixedBackground: true, AllowEmpty: true,
		ResultProperty: "#result", ResultPropertyKey: -1,
		NextValue: bsMapHimedoreiNextValue, TimeValue: timeValue,
		Source: BsMapDataFileName,
	}
	base, overlay := himedoreiMapBackground(timeValue)
	if context.Resources.Exists(base) {
		layout.Background = base
	}
	if context.Resources.Exists(overlay) {
		layout.Overlay = overlay
	}
	// 每个 place 的节点顺序是：0 号地点卡，后面依次是角色/事件卡。
	// hd.exe 的 0x437390 用 count*(count-1)/2 作为该组第一块的 chunk
	// 下标，节点每增加一个整组重新排一次；这里按同样规则算坐标。
	nodes := make([][]himedoreiMapNode, len(mapData.Places))
	for index := range nodes {
		image := mapData.ImageName(mapData.Places[index].ImageIndex, context.Resources)
		nodes[index] = []himedoreiMapNode{{image: image}}
	}
	if timeIndex >= 0 {
		for tableIndex := range mapData.Tables {
			table := mapData.Tables[tableIndex]
			state := context.Properties.ByName(table.StateProperty)
			if state != 0 {
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
			place := (packed & bsMapHimedoreiPackedPlaceMask) - 1
			if place < 0 || place >= len(nodes) {
				continue
			}
			image := mapData.ImageName(packed>>bsMapHimedoreiPackedImageShift, context.Resources)
			if len(image) == 0 {
				continue
			}
			nodes[place] = append(nodes[place], himedoreiMapNode{
				image: image,
				label: himedoreiStateLabel(table.StateProperty),
				value: tableIndex + 1,
			})
		}
	}
	for placeIndex, group := range nodes {
		place := mapData.Places[placeIndex]
		total := len(group)
		baseChunk := total * (total - 1) / 2
		for nodeIndex, node := range group {
			chunkIndex := baseChunk + nodeIndex
			if chunkIndex < 0 || chunkIndex >= len(place.Chunks) {
				continue
			}
			chunk := place.Chunks[chunkIndex]
			x := place.BaseX + chunk.X
			y := place.BaseY + chunk.Y
			if len(node.image) == 0 {
				continue
			}
			if nodeIndex == 0 {
				// 地点卡只画不点：原作点地点是"移动到该处"，结果分支
				// 依赖 #place，当前只还原可见画面，避免写错位置变量。
				layout.Decor = append(layout.Decor, BsMapItem{
					Image: node.image, DrawX: x, DrawY: y,
					CellHeight: bsMapHimedoreiCardSize,
				})
				continue
			}
			if len(node.label) > 0 {
				item := NewBsMapItem(node.image, 0, x, y, x, y,
					bsMapHimedoreiCardSize, bsMapHimedoreiCardSize, node.label)
				item.CellHeight = bsMapHimedoreiCardSize
				layout.Items = append(layout.Items, item)
				continue
			}
			if node.value > 0 {
				// #ev_map 等没有独立脚本标签的事件卡：写 #result=表序号+1，
				// 脚本的 Map 例程正是用 1..18 分派这些事件。
				item := NewBsMapItemWithProperty(node.image, 0, x, y, x, y,
					bsMapHimedoreiCardSize, bsMapHimedoreiCardSize,
					"", "#result", -1, node.value)
				item.CellHeight = bsMapHimedoreiCardSize
				layout.Items = append(layout.Items, item)
			}
		}
	}
	// 底部六颗系统按钮：map_menu_button 是 80x192 的六帧图，
	// 帧高 32，命中块是 map_hit 的 80x32。
	tools := []string{"qsave", "qload", "save", "load", "config", "log"}
	for index, tool := range tools {
		layout.SystemButtons = append(layout.SystemButtons, BsMapSystemButton{
			Image: bsMapHimedoreiMenuButton, X: 40 + 128*index, Y: 560,
			Width: 80, Height: 32, Frames: 6, Tool: tool,
		})
	}
	// 右上角「時間を進める」：点击写 #result=9999。
	layout.SystemButtons = append(layout.SystemButtons, BsMapSystemButton{
		Image: bsMapHimedoreiNext, X: 588, Y: 16,
		Width: 196, Height: 32, Frames: 1, Tool: "next",
	})
	return layout
}

const (
	bsMapHimedoreiCardSize         = 100
	bsMapHimedoreiPackedPlaceMask  = 0xFF
	bsMapHimedoreiPackedImageShift = 8
)

type himedoreiMapNode struct {
	image string
	label string
	value int
}

func himedoreiTimeSlot(context BsMapContext, mapData BsMapData) (int, int) {
	if len(mapData.TimeProperty) == 0 || len(mapData.TimeValues) == 0 {
		return -1, -1
	}
	value := context.Properties.ByName(mapData.TimeProperty)
	if value < 0 {
		return -1, value
	}
	return mapData.TimeSlot(value), value
}

func himedoreiStateLabel(state string) string {
	switch strings.ToLower(state) {
	case "#tht_map":
		return "tht_map"
	case "#lyz_map":
		return "lyz_map"
	case "#est_map":
		return "est_map"
	case "#mly_map":
		return "mly_map"
	case "#cmbh1_map":
		return "cmbh1"
	case "#cmbh2_map":
		return "cmbh2"
	case "#cmbh3_map":
		return "cmbh3"
	case "#haremm_map":
		return "harem"
	}
	return ""
}

func himedoreiMapBackground(timeValue int) (string, string) {
	switch {
	case timeValue >= 6:
		return bsMapHimedoreiBase3, bsMapHimedoreiOver3
	case timeValue == 5:
		return bsMapHimedoreiBase2, bsMapHimedoreiOver2
	default:
		return bsMapHimedoreiBase1, bsMapHimedoreiOver1
	}
}

// BsMapDisplayName 把脚本标签名转换成给玩家看的名字。
//
// label scan 产出的 Label 既是按钮文字又是 SeekBsLabel 的目标；ヒメドレイ
// 的标签是 tht_map/lyz_map 这类内部名，直接显示很难看。这里只改显示，
// 不改 BsMapItem.Label，保证点击仍然跳到正确的标签。
func BsMapDisplayName(label string) string {
	switch strings.ToLower(label) {
	case "tht_map":
		return "ティータ"
	case "lyz_map":
		return "リーゼ"
	case "est_map":
		return "エステル"
	case "mly_map":
		return "ミレーユ"
	}
	return label
}
