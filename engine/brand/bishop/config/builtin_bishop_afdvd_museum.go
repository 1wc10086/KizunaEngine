package config

// builtin_bishop_afdvd_museum.go 给 AFDVD（放課後～濡れた制服～ DVD 版）补鉴赏室布局。
//
// 素材是 BSS-Composition：museum_menu / museum_titlelist / museum_bgmctrl /
// museum_chs 的每个部件都自带原版屏幕坐标（OffsetX/OffsetY），所以这里不猜
// 坐标，直接按部件位置摆放。部件编号在 UIButton.Part 里是 1 基。
//
// 页面结构：
//   - 顶部五个页签 BGM / CG / シーン / 立ち絵 / 終了（museum_menu 的 5 个部件）；
//   - BGM 页：museum_titlelist 的 20 首曲名（左列 10、右列 10）+ museum_bgmctrl
//     的播放/停止/上一首/下一首/最小/最大 + museum_volume 电平条；
//   - CG 页：cg.dat 的 8 个分组，每组 20 格（5x4），格子用 museum_empty 的
//     144x115 空框；
//   - シーン 页：scene.dat 的 4 个分组，同样 5x4；
//   - 点开缩略图后用 cgm_base/cgm_btn 的查看器切换差分（按钮帧 0..4 常态、
//     5..9 悬停，和 KD 同一套约定）。

func applyBuiltinAFDVDMuseum(cfg *BsxxConfiguration) {
	cfg.Museum = Museum{
		Base: "museum_base",
		Modes: []MuseumMode{
			{Key: "music", State: 1, ActiveFrame: 1, UIButton: afdvdMuseumTab(1, 8, 8, 3)},
			{Key: "cg", State: 2, ActiveFrame: 1, UIButton: afdvdMuseumTab(2, 166, 8, 3)},
			{Key: "scene", State: 4, ActiveFrame: 1, UIButton: afdvdMuseumTab(3, 324, 8, 3)},
			{Key: "strip", State: 8, ActiveFrame: 1, UIButton: afdvdMuseumTab(4, 482, 8, 4)},
		},
		Return: afdvdMuseumTab(5, 640, 8, 2),
		// 立ち絵页：museum_chs 的 7 个整身立绘部件，坐标就是各自的 OffsetX/Y。
		// 原版这一页只做浏览，点角色不会进入换装/差分选择，所以这里保持静态。
		StripPortraits: []UIButton{
			{Image: "museum_chs", Part: 1, Frames: 1, X: 440, Y: 72},
			{Image: "museum_chs", Part: 2, Frames: 1, X: 180, Y: 72},
			{Image: "museum_chs", Part: 3, Frames: 1, X: 570, Y: 185},
			{Image: "museum_chs", Part: 4, Frames: 1, X: 32, Y: 182},
			{Image: "museum_chs", Part: 5, Frames: 1, X: 306, Y: 187},
			{Image: "museum_chs", Part: 6, Frames: 1, X: 440, Y: 338},
			{Image: "museum_chs", Part: 7, Frames: 1, X: 180, Y: 340},
		},
		Music: MuseumMusicPage{
			PageBase: UIButton{Image: "museum_titlelist", Part: 1, Frames: 1, X: 37, Y: 85},
			// 曲名部件 3 帧：常态 / 悬停 / 播放中。
			Tracks:      afdvdMusicTracks(),
			Names:       afdvdMusicNames(),
			ControlBase: UIButton{Image: "museum_bgmctrl", Part: 1, Frames: 1, X: 36, Y: 560},
			Play:        UIButton{Image: "museum_bgmctrl", Part: 3, Frames: 2, X: 214, Y: 568},
			Stop:        UIButton{Image: "museum_bgmctrl", Part: 2, Frames: 2, X: 162, Y: 568},
			Prev:        UIButton{Image: "museum_bgmctrl", Part: 4, Frames: 2, X: 110, Y: 568},
			Next:        UIButton{Image: "museum_bgmctrl", Part: 5, Frames: 2, X: 266, Y: 568},
			Min:         UIButton{Image: "museum_bgmctrl", Part: 6, Frames: 2, X: 396, Y: 568},
			Max:         UIButton{Image: "museum_bgmctrl", Part: 7, Frames: 2, X: 648, Y: 568},
			Volume:      UIButton{Image: "museum_volume", Frames: 1, X: 444, Y: 572},
			VolumeFill:  true,
			// 电平条 200x16，命中区上下各放宽 8px，手指容易点。
			VolumeRegion: [4]int{444, 564, 200, 32},
		},
		CG: MuseumCGPage{
			PageButtons: afdvdPageButtons(8),
			// 缩略图落在 museum_empty 的白框内部：外框 144x115，内边距 8px，
			// 所以换算出 128x99 的画面区，格子仍按 144x116 的步距排列。
			Thumbs:     afdvdThumbGrid(52, 84),
			ThumbFrame: UIButton{Image: "museum_empty", Frames: 1},
			ThumbX:     52,
			ThumbY:     84,
			ThumbW:     128,
			ThumbH:     99,
		},
		Scene: MuseumScenePage{
			PageButtons: afdvdPageButtons(4),
			Thumbs:      afdvdThumbGrid(52, 84),
			ThumbFrame:  UIButton{Image: "museum_empty", Frames: 1},
			ThumbX:      52,
			ThumbY:      84,
			ThumbW:      128,
			ThumbH:      99,
		},
		Viewer: CGViewer{
			Base:   UIButton{Image: "cgm_base", Frames: 1, X: 640, Y: 520},
			PanelX: 640,
			PanelY: 520,
			Prev:   afdvdViewerButton(0, 644),
			Exit:   afdvdViewerButton(1, 674),
			Next:   afdvdViewerButton(2, 704),
			CutIn:  afdvdViewerButton(3, 734),
			Config: afdvdViewerButton(4, 764),
			// cgm_btn 的 0..4 是常态、5..9 是蓝色悬停，和 KD 相同。
			ButtonBase:   0,
			HoverOffset:  5,
			TogglePanel:  true,
			ClickAdvance: true,
		},
	}
}

func afdvdMuseumTab(part, x, y, frames int) UIButton {
	return UIButton{Image: "museum_menu", Part: part, Frames: frames, X: x, Y: y}
}

func afdvdViewerButton(start, x int) UIButton {
	return UIButton{Image: "cgm_btn", Frames: 10, Start: start, X: x, Y: 558}
}

// afdvdMusicTracks 按 museum_titlelist 的部件位置排 20 首曲名。
// 部件 0 是整张乐谱纸，曲名从部件 1 开始；左列 x=92 十行，右列 x=452 十行，
// 行距 40，单帧高 28。
func afdvdMusicTracks() []UIButton {
	tracks := make([]UIButton, 0, 20)
	for index := 0; index < 20; index++ {
		x, y := 92, 108+(index%10)*40
		if index >= 10 {
			x = 452
		}
		tracks = append(tracks, UIButton{
			Image: "museum_titlelist", Part: index + 2, Frames: 3, X: x, Y: y,
		})
	}
	return tracks
}

// afdvdMusicNames 是 20 首曲目的资源名：bgm.bsa 里正好是 bgm02..bgm21。
// bgm22 是事件用短曲，原曲名表没有把它列进鉴赏室。
func afdvdMusicNames() []string {
	names := make([]string, 0, 20)
	for track := 2; track <= 21; track++ {
		names = append(names, twoDigits(track))
	}
	return names
}

// afdvdPageButtons 生成左侧竖排页码：museum_num 是 20x240、12 帧的数字条，
// 每帧 20x20。FixedFrame 保证第 3 页永远画数字"3"，不会被悬停帧改成别的数字。
func afdvdPageButtons(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		buttons = append(buttons, UIButton{
			Image:      "museum_num",
			Frames:     12,
			X:          8,
			Y:          64 + index*22,
			Start:      index + 1,
			FixedFrame: true,
		})
	}
	return buttons
}

// afdvdThumbGrid 返回 5 列 x 4 行共 20 个格子的左上角，按行优先排列。
func afdvdThumbGrid(x0, y0 int) []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 4; row++ {
		for col := 0; col < 5; col++ {
			points = append(points, UIPoint{X: x0 + 144*col, Y: y0 + 116*row})
		}
	}
	return points
}
