package rendering

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

const minimumSelectRowHeight = 44

const (
	selectAlignLeft   = 0
	selectAlignCenter = 1
	selectAlignRight  = 2
)

// BuildSelectWindow 计算选项条的排布。
//
// 每个选项由"中间条 + 可选的上下装饰图"组成：中间条是 2 帧竖排图
// （常态/悬停），命中区只算中间条；装饰图（item0_head/item0_foot）参与
// 整体高度，避免两行选项的装饰互相压住。
func BuildSelectWindow(design config.SelectDesign, screenWidth, screenHeight, frameWidth, frameHeight, headHeight, footHeight, datumLine int, texts []string) []state.SelectItem {
	if len(texts) == 0 {
		return []state.SelectItem{}
	}
	width := frameWidth
	if width <= 0 {
		width = screenWidth / 2
	}
	height := frameHeight
	if height <= 0 {
		// 没有选项框素材时才用最小高度兜底；有素材时按素材帧高排，
		// 否则 42px 的原版行会被撑成 44px，文字和装饰都会错位。
		height = minimumSelectRowHeight
	}
	if design.HeaderFooter {
		if headHeight <= 0 {
			headHeight = 0
		}
		if footHeight <= 0 {
			footHeight = 0
		}
	} else {
		headHeight, footHeight = 0, 0
	}
	blockHeight := headHeight + height + footHeight
	interval := max(0, design.Interval)
	total := len(texts)*blockHeight + (len(texts)-1)*interval
	if design.HeaderFooterOnce {
		// 头/脚只属于整组窗口：总高 = 头 + N*行 + (N-1)*间隔 + 脚。
		blockHeight = height
		total = headHeight + len(texts)*height + (len(texts)-1)*interval + footHeight
	}
	datum := datumLine
	if datum <= 0 {
		datum = screenHeight / 2
	}
	top := core.Clamp(datum-total/2, 0, max(0, screenHeight-total))
	left := max(0, (screenWidth-width)/2)
	textX := design.TextX
	if design.Alignment == selectAlignCenter && textX == 0 {
		textX = width / 2
	}
	items := make([]state.SelectItem, 0, len(texts))
	for index, text := range texts {
		middleY := top + index*(blockHeight+interval) + headHeight
		if design.HeaderFooterOnce {
			middleY = top + headHeight + index*(height+interval)
		}
		// SelectedFrame 透传到渲染层：悬停时用 SelectedItem 图而非 Frame。
		item := state.SelectItem{
			Index:         index,
			Text:          text,
			Frame:         design.Item,
			SelectedFrame: design.SelectedItem,
			X:             left,
			Y:             middleY,
			Width:         width,
			Height:        height,
			Frames:        design.Frames,
			TextX:         textX,
			TextY:         design.TextY,
			Alignment:     design.Alignment,
			DefaultColor:  design.DefaultColor,
			SelectColor:   design.SelectColor,
			HeadHeight:    headHeight,
			FootHeight:    footHeight,
		}
		if design.HeaderFooter {
			if !design.HeaderFooterOnce || index == 0 {
				item.Head = design.Item + "_head"
			}
			if !design.HeaderFooterOnce || index == len(texts)-1 {
				item.Foot = design.Item + "_foot"
			}
		}
		items = append(items, item)
	}
	return items
}
