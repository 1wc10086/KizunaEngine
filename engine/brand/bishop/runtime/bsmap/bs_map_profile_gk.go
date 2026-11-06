package bsmap

import (
	"fmt"

	"kizuna/engine/brand/bishop/vm/bs"
)

// bs_map_profile_gk.go 是「学園〜恥辱の図式〜」（GK/GKDVD/GKWindows10，
// BSScript Rev.4）的地图。MapPrms.dat 给出 12 个地点标签，但通用
// MapPrms profile 把热区放在左侧 116..760，而这一代的按钮图
// map_place01..12 自带 (620, 116+38i) 的图头坐标；工具条、日期卡和
// 目标头像也不是角色卡。
//
// 这个 profile 只做几何与系统按钮；日期/头像/悬停帧由 session 的
// drawGkMapOverlay 每帧补画（舞台层只贴固定帧）。
type BsMapProfileGk struct{}

const (
	BsMapGkSourceName    = BsMapGkSource
	BsMapGkPlaceLeft     = 620
	BsMapGkPlaceTop      = 116
	BsMapGkPlaceStep     = 38
	BsMapGkPlaceWidth    = 140
	BsMapGkPlaceHeight   = 30
	BsMapGkPlaceCount    = 12
	BsMapGkPlaceImage    = "map_place"
	BsMapGkBackground    = "map_base"
	BsMapGkSubmenu       = "map_submenu"
	BsMapGkSubmenuButton = "map_submenu_button"
	BsMapGkSubmenuX      = 160
	BsMapGkSubmenuY      = 0
	BsMapGkButtonWidth   = 116
	BsMapGkButtonHeight  = 32
	BsMapGkDateBase      = "date_base"
	BsMapGkFaceImage     = "map_face"
	BsMapGkFaceX         = 440
	BsMapGkFaceY         = 0
	BsMapGkFaceWidth     = 140
	BsMapGkFaceHeight    = 120
)

func (p *BsMapProfileGk) Name() string { return "Gk" }

func (p *BsMapProfileGk) Matches(context BsMapContext) bool {
	if !context.HasDialect || context.Dialect != bs.BsScriptRev4 {
		return false
	}
	return context.Resources.Exists(BsMapGkBackground) &&
		context.Resources.Exists(BsMapGkPlaceImage+"01") &&
		context.Resources.Exists(BsMapGkSubmenu) &&
		context.Resources.Exists(BsMapGkDateBase) &&
		context.Resources.Exists(BsMapGkFaceImage)
}

func (p *BsMapProfileGk) Build(context BsMapContext) *BsMapLayout {
	parameters := LoadBsMapParameters(context.GameRoot)
	if parameters == nil || len(parameters.Jumps) < BsMapGkPlaceCount {
		return nil
	}
	items := make([]BsMapItem, 0, BsMapGkPlaceCount)
	for index := 0; index < BsMapGkPlaceCount; index++ {
		label := parameters.PlaceLabelName(index)
		if label == "" || context.Script.FindLabel(label) < 0 {
			continue
		}
		image := fmt.Sprintf("%s%02d", BsMapGkPlaceImage, index+1)
		if !context.Resources.Exists(image) {
			continue
		}
		item := NewBsMapItem(image, 0, -1, -1,
			BsMapGkPlaceLeft, BsMapGkPlaceTop+index*BsMapGkPlaceStep,
			BsMapGkPlaceWidth, BsMapGkPlaceHeight, label)
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	layout := &BsMapLayout{
		Background: BsMapGkBackground,
		FrameImage: BsMapGkSubmenu,
		FrameX:     BsMapGkSubmenuX,
		FrameY:     BsMapGkSubmenuY,
		Items:      items,
		Source:     BsMapGkSourceName,
	}
	// 日期卡是静态底图，数字在 overlay 里逐帧画。
	layout.Decor = append(layout.Decor, NewBsMapItemDecor(BsMapGkDateBase, 0, 0, -1, -1))
	// 头像由 drawGkMapOverlay 按 #GKR_flower 每帧补画；舞台层不放它，
	// 否则没有目标时也会残留一张静帧。
	// 顶部工具条：六个按钮共用 map_submenu_button 的 6 帧，每颗固定一帧。
	tools := []struct {
		tool  string
		frame int
		x     int
		y     int
	}{
		{"qsave", 0, 162, 2},
		{"qload", 1, 282, 2},
		{"save", 2, 162, 36},
		{"load", 3, 282, 36},
		{"config", 4, 162, 70},
		{"log", 5, 282, 70},
	}
	for _, tool := range tools {
		layout.SystemButtons = append(layout.SystemButtons, BsMapSystemButton{
			Image: BsMapGkSubmenuButton, X: tool.x, Y: tool.y,
			Width: BsMapGkButtonWidth, Height: BsMapGkButtonHeight,
			Frames: 6, Frame: tool.frame, Tool: tool.tool,
		})
	}
	return layout
}
