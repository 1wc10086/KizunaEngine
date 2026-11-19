package config

import "strconv"

// builtin_bishop_ancient_datascreen.go 给 Tbxdl 世代（特別病棟 旧版 / Win10）补存档/读档画面。
//
// 这一代没有整屏底图：黑底上排 6 个槽位（2 列 x 3 行），每个槽位是一张 380x160 的框，
// 缩略图和文字由引擎叠加。坐标按素材量出，与 DataScreen 的 Slot* 字段一一对应。
// 只在存档布局为空时填入，已有 [data screen] 的作品不会进到这里。
func applyBuiltinAncientDataScreen(cfg *BsxxConfiguration) {
	if cfg == nil || !cfg.DataScreen.Empty() {
		return
	}
	screen := DataScreen{
		PageMax:      10,
		SlotsPerPage: 6,
		// 第 1 页第 1 个槽位是オートセーブ専用，框图换成 ASaveFrame。
		AutoSlotIndex:     0,
		SlotFrame:         "SaveFrame",
		SlotFrameFrames:   3,
		SlotFrameAuto:     "ASaveFrame",
		SlotFrameHover:    1,
		SlotFrameSelected: 2,
		// 命中区与框图同尺寸，避免缩略图比例（ThumbnailRatio）把热区缩到框的一半。
		SlotSize:        [2]int{380, 160},
		SlotThumb:       [4]int{205, 20, 160, 120},
		SlotTextX:       15,
		SlotTextY:       40,
		SlotTextLineGap: 30,
		SlotTextColor:   0xffffffff,
		PageSelectFrame: UIButton{Image: "PageSelectFrame", Frames: 1, X: 352, Y: 11},
		// save_load 是 206x70 的两帧图：上半是常态，下半是激活；左半 SAVE、右半 LOAD。
		SaveMode: UIButton{Image: "save_load", Frames: 2, X: 15, Y: 551, Clip: [4]int{0, 0, 103, 35}},
		LoadMode: UIButton{Image: "save_load", Frames: 2, X: 118, Y: 551, Clip: [4]int{103, 0, 103, 35}},
		Back:     UIButton{Image: "Sys_back", Frames: 3, X: 639, Y: 551},
		Title:    UIButton{Image: "Sys_title", Frames: 3, X: 685, Y: 551},
		Exit:     UIButton{Image: "Sys_quit", Frames: 3, X: 731, Y: 551},
	}
	// 槽位按行优先排列：每行左右两格，共 3 行。
	columns := [2]int{15, 405}
	rows := [3]int{55, 220, 385}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	for index := 0; index < screen.PageMax; index++ {
		screen.PageButtons = append(screen.PageButtons, UIButton{
			Image:  "Page" + strconv.Itoa(index),
			Frames: 3,
			X:      431 + 35*index,
			Y:      11,
		})
	}
	cfg.DataScreen = screen
}
