package config

import "strconv"

// applyBuiltinMuseum 为 KD（毎日けだものっ!!，800x600）填充鉴赏室布局，
// 由 builtin_bishop_legacy.go 调用。只填空缺项，已有配置保持不动。
//
// 素材与坐标来自 KD 的 manual/imgs/museum.png、dm.png 和资源头：
//   - museum_menu 是 BSS-Composition 复合图，顶部五个页签（BGM/CG/场景/立ち絵/戻る）
//     都在同一张图的不同部件里，所以用 Part（对应 `museum_menu#N`）逐个选中，
//     而不是五张独立图片。
//   - museum_num 是 20x240、12 帧的数字条。页码按钮每个固定画第 i+1 帧，
//     用 FixedFrame 而不是随悬停切帧，否则第 3 页会显示成别的数字。
//   - cg0_thumb…cg7_thumb 与 scene0_thumb…scene3_thumb 是 120x1800、20 帧的
//     预制缩略图条，直接按帧切，不从完整 CG 合成，避免逐格合成的开销。
//
// 坐标保持 800x600 的游戏坐标，不按 1024x768 的 KDIE 比例缩放。
// BGM 页除 26 首曲目外还补了控制盒、播放/停止/上一首/下一首和音量滑块；
// CG 查看器补了右下角 cgm_base 控制盒（上一张/停止/下一张/CutIn/CFG），
// 右键收起控制盒、左键点画面切差分。立ち絵页仍由 openKDStripMuseum 提供，
// 这里不填假坐标。
func applyBuiltinMuseum(cfg *BsxxConfiguration) {
	museum := &cfg.Museum
	if museum.Base == "" {
		museum.Base = "museum_base"
	}
	if len(museum.Modes) == 0 {
		museum.Modes = []MuseumMode{
			{Key: "music", State: 1, UIButton: museumMenuPart(1, 118, 556)},
			{Key: "cg", State: 2, UIButton: museumMenuPart(2, 251, 556)},
			{Key: "scene", State: 4, UIButton: museumMenuPart(3, 384, 556)},
			{Key: "strip", State: 8, UIButton: museumMenuPart(4, 517, 556)},
		}
	}
	if museum.Return.Empty() {
		museum.Return = UIButton{Image: "museum_menu", Frames: 2, Part: 5, X: 675, Y: 556}
	}
	if len(museum.Music.Tracks) == 0 {
		// BGM 页的曲名不是文字，而是 museum_titlelist 的 26 个部件：
		// 左列 x=116、右列 x=456，从 y=82 起每行 36px。部件本身 340x108，
		// 按 3 帧切后单帧 36px，正好是一行；不切会把相邻行一起画出来。
		tracks := make([]UIButton, 0, 26)
		names := make([]string, 0, 26)
		for index := 0; index < 26; index++ {
			x, y := 116, 82+(index%13)*36
			if index >= 13 {
				x = 456
			}
			// 部件 0 是 1x1 的定位点，真正的曲名从部件 1 开始；
			// Part 是 1 基，所以第 index 首取 Parts[index]（Part=index+2）。
			tracks = append(tracks, UIButton{
				Image: "museum_titlelist", Part: index + 2,
				Frames: 3, X: x, Y: y,
			})
			names = append(names, "bgm"+twoDigits(index+1))
		}
		museum.Music.Tracks = tracks
		museum.Music.Names = names
	}
	if museum.Music.Play.Empty() {
		// museum_bgmctrl 的部件是 1 基：第 1 个是整块控制盒（四个按钮空槽 +
		// 竖向音量轨道），第 2..5 个是 37x74 的两帧按钮（常态/红色悬停），
		// 第 6/7 个是 ▼/▲，第 8 个是音量滑块。原图的排列是：
		// 左上 ■(停止)、右上 ▶(播放)、左下 ◀◀、右下 ▶▶。
		// 因此 Play 必须用第 3 个部件；用第 2 个会把停止键画在播放位置，
		// 而且 Frames=1 会把常态/悬停两帧一起画出来（一格里出现两个图标）。
		museum.Music.ControlBase = UIButton{Image: "museum_bgmctrl", Frames: 1, Part: 1, X: 5, Y: 151}
		museum.Music.Play = museumBGMControl(3, 53, 154)
		museum.Music.Stop = museumBGMControl(2, 8, 154)
		museum.Music.Prev = museumBGMControl(4, 8, 198)
		museum.Music.Next = museumBGMControl(5, 53, 198)
		museum.Music.Volume = UIButton{Image: "museum_bgmctrl", Frames: 1, Part: 8, X: 49, Y: 246}
		museum.Music.VolumeKnob = true
		// 最大音量在轨道顶端（原图滑块初始就在顶端），静音在底端；
		// 轨道 246..462，滑块高 31，所以可移动 185px。
		museum.Music.VolumeTrackHeight = 185
		museum.Music.VolumeRegion = [4]int{44, 246, 44, 216}
		// ▲ 在顶端设最大音量，▼ 在底端设静音；与原作手册一致。
		museum.Music.Max = museumBGMControl(7, 15, 262)
		museum.Music.Min = museumBGMControl(6, 15, 442)
	}

	if museum.Viewer.Prev.Empty() {
		// cgm_btn 是 24x240 的 10 帧竖排：0 上一张、1 返回、2 下一张、
		// 3 CutIn、4 CFG，5..9 是对应的红色悬停帧。底栏 cgm_base 152x68
		// 放在右下角 (640,520)，五个 24px 按钮排在它底部。
		// 手动图像里没有这一屏，坐标按面板尺寸和原作“右下控制盒”说明排布。
		museum.Viewer = CGViewer{
			Base:   UIButton{Image: "cgm_base", Frames: 1, X: 640, Y: 520},
			PanelX: 640,
			PanelY: 520,
			Prev:   museumCGButton(0, 644, 558),
			Exit:   museumCGButton(1, 674, 558),
			Next:   museumCGButton(2, 704, 558),
			CutIn:  museumCGButton(3, 734, 558),
			Config: museumCGButton(4, 764, 558),
			// 序号用引擎字体画在面板左上角（原版数字条素材没有随游戏提供）。
			ButtonBase:        0,
			HoverOffset:       5,
			ToggleHoverOffset: 5,
			TogglePanel:       true,
			ClickAdvance:      true,
		}
	}

	if len(museum.CG.PageButtons) == 0 {
		museum.CG.PageButtons = museumNumButtons(8)
	}
	if len(museum.CG.ThumbStrips) == 0 {
		museum.CG.ThumbStrips = museumThumbStrips("cg", 8)
	}
	if len(museum.CG.Thumbs) == 0 {
		museum.CG.Thumbs = museumThumbGrid()
	}
	if museum.CG.ThumbW == 0 {
		museum.CG.ThumbX, museum.CG.ThumbY = 84, 110
		museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	}

	if len(museum.Scene.PageButtons) == 0 {
		museum.Scene.PageButtons = museumNumButtons(4)
	}
	if len(museum.Scene.ThumbStrips) == 0 {
		museum.Scene.ThumbStrips = museumThumbStrips("scene", 4)
	}
	if len(museum.Scene.Thumbs) == 0 {
		museum.Scene.Thumbs = museumThumbGrid()
	}
	if museum.Scene.ThumbW == 0 {
		museum.Scene.ThumbX, museum.Scene.ThumbY = 84, 110
		museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	}
}

func museumBGMControl(part, x, y int) UIButton {
	// 37x74 是两帧竖排（常态/红色悬停）；不切帧会把两个图标叠在一格里。
	return UIButton{Image: "museum_bgmctrl", Part: part, Frames: 2, X: x, Y: y}
}

func museumCGButton(part, x, y int) UIButton {
	// Start 是常态帧（0..4）；绘制时用相对帧 0/5 选常态/红色悬停，
	// 所以不能设 FixedFrame，否则悬停帧会被 clamp 回常态。
	return UIButton{Image: "cgm_btn", Frames: 10, Start: part, X: x, Y: y}
}

func twoDigits(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func museumMenuPart(part, x, y int) UIButton {
	return UIButton{Image: "museum_menu", Frames: 4, Part: part, X: x, Y: y}
}

func museumNumButtons(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		buttons = append(buttons, UIButton{
			Image:      "museum_num",
			Frames:     12,
			X:          20,
			Y:          96 + index*22,
			Start:      index + 1,
			FixedFrame: true,
		})
	}
	return buttons
}

func museumThumbStrips(prefix string, count int) []string {
	names := make([]string, 0, count)
	for index := 0; index < count; index++ {
		names = append(names, prefix+strconv.Itoa(index)+"_thumb")
	}
	return names
}

// museumThumbGrid 返回 5 列 x 4 行共 20 个格子的左上角，按行优先排列。
func museumThumbGrid() []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 4; row++ {
		for col := 0; col < 5; col++ {
			points = append(points, UIPoint{X: 84 + 128*col, Y: 110 + 98*row})
		}
	}
	return points
}
