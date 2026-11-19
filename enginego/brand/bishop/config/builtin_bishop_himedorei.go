package config

// builtin_bishop_himedorei.go 给「姫奴隷～囚われの姫君達～」（ヒメドレイ，
// BSScript Rev.6，800x600，2007）补整套内置界面。
//
// 这部作品没有 bsxx.cfg：标题、对话框、工具条、存读档页、设置、鉴赏室、
// 回想和地图的布局全部编译在 hd.exe 里。只按 bsxx.cfg 解析会得到一份
// 全空配置——进游戏没有标题、没有对话框、工具条/存读档/鉴赏室/设置全部
// 打不开，和原版差得很远。
//
// 下面的坐标不是从截图估的，而是从发行目录中的 hd.exe 反汇编得到：
//   - 标题：标题初始化函数把 title_base 画在 (0,-412)，
//     六个按钮分别在 (624,208)/(672,256)/(672,304)/(696,352)/(720,400)/(720,448)；
//   - 对话框：message 是 800x170 的合成图，原引擎贴在 (0,430)；
//     工具条两颗一列，x=602/682，y=470/492/514/536/558，CUT IN 在 (572,450)；
//   - 存读档：dm_hit 的 11 个 160x120 槽位 + 10 个 24x24 页码，
//     第一格 (32,112) 是自动存档（不在命中表里）；
//   - 设置：cfg_ex 是 754x408 的面板，放在 (24,98)；config_hit 给出
//     开关/滑杆/角色头像的精确矩形；
//   - 鉴赏室：museum_menu 的 5 个 36px 页签、museum_titlelistbase 的
//     20 首曲名、museum_bgmctrl 的播放控制盒。
//
// 命中表里的全屏索引 255 是"背景"，不参与按钮判定，因此没有写进配置。

// builtinHimedoreiAssets 是这部作品独有的素材指纹。
//
// 只看 title_base 会命中现代作品；这里要求旧式存读档框 dm_slot/dm_slotbase、
// 旧式设置 cfg_ex、五个角色头像、19 行鉴赏室列表同时存在，基本只会在
// 同代 Bishop 作品里成立。KD 有 kaisou/dm_save、MK2 有 dm_base、
// AFDVD 有 gamemenu，这些都在 Excluded 里排除。
var builtinHimedoreiAssets = []string{
	"title_base", "title_hit",
	"title_menu_op", "title_menu_begin", "title_menu_load",
	"title_menu_museum", "title_menu_setting", "title_menu_quit",
	"message", "message_hit", "unagashi",
	"dm_slotbase", "dm_slot", "dm_hit", "dm_auto", "dm_nodata",
	"dm_number", "dm_pagenum", "dm_pagebase",
	"config_base", "config_hit", "cfg_ex", "cfg_button",
	"cfg_mode_save", "cfg_mode_load", "cfg_mode_config",
	"cfg_min", "cfg_max", "cfg_meter", "cfg_mark", "cfg_ch1", "cfg_ch5",
	"museum_base", "museum_menu", "museum_titlelist",
	"museum_titlelistbase", "museum_hit", "museum_bgmctrl", "museum_ch1",
	"log_meter", "log_vp",
	"item0", "item0_head", "item0_foot",
}

var builtinHimedoreiExcluded = []string{
	"gamemenu", "gamemenu_hit", "kaisou", "dm_save", "dm_load",
	"dm_base", "msg_base", "map_base", "map_ch101",
}

func isBuiltinHimedorei(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinHimedoreiAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinHimedoreiExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// ApplyBuiltinHimedoreiDefaults 填入整套界面；只被 ApplyBuiltinBishopDefaults 调用。
func ApplyBuiltinHimedoreiDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || !isBuiltinHimedorei(probe) {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	cfg.Width = 800
	cfg.Height = 600
	cfg.ThumbnailRatio = 0.2
	// 立绘的眼睛条恰好 4 帧；默认 6 会让眨眼按 6 帧取模，节奏不匀。
	cfg.EyePattern = 4
	applyHimedoreiTitle(cfg)
	applyHimedoreiMessage(cfg)
	applyHimedoreiSelect(cfg)
	applyHimedoreiDataScreen(cfg)
	applyHimedoreiConfig(cfg)
	applyHimedoreiMuseum(cfg)
	applyHimedoreiLog(cfg)
	return true
}

func applyHimedoreiTitle(cfg *BsxxConfiguration) {
	cfg.Title = TitlePhase{
		// 反汇编里标题初始化用 bgm04 起播。
		BgmName:  "bgm04",
		Base:     "title_base",
		BaseX:    0,
		BaseY:    -412,
		HitTable: "title_hit",
		Entries: []TitleMenuEntry{
			{Key: "op", Image: "title_menu_op", Frames: 2, X: 624, Y: 208, Frame: -1},
			{Key: "start", Image: "title_menu_begin", Frames: 2, X: 672, Y: 256, Frame: -1},
			{Key: "load", Image: "title_menu_load", Frames: 2, X: 672, Y: 304, Frame: -1},
			{Key: "museum", Image: "title_menu_museum", Frames: 2, X: 696, Y: 352, Frame: -1},
			{Key: "config", Image: "title_menu_setting", Frames: 2, X: 720, Y: 400, Frame: -1},
			{Key: "quit", Image: "title_menu_quit", Frames: 2, X: 720, Y: 448, Frame: -1},
		},
	}
}

func applyHimedoreiMessage(cfg *BsxxConfiguration) {
	// message 是 800x170 的银灰渐变框，原引擎贴在 (0,430)。
	// 右侧 200px 是工具条面板，正文不能越界到面板下面。
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 430,
		Speed: 100, BgColor: 0,
		NameX: 40, NameY: 30, NameW: 480, NameH: 22, NameColor: 0xffff8822,
		TextX: 40, TextY: 58, TextW: 520, TextH: 96,
		// 银灰底图配黑字；bg color 保持 0，避免把整张底图重新上色。
		TextColor: 0xff000000, TextColorSet: true,
		Unagashi: "unagashi", UnagashiX: 560, UnagashiY: 120,
		UnagashiCellW: 32, UnagashiCellH: 32,
		UnagashiColumns: 1, UnagashiFrames: 7, UnagashiSpeed: 240,
	}
	// 16 号字、行高 24：520px 正文约 32 个全角字，四行正好填满框内。
	cfg.Font = FontSettings{
		Size: 16, Width: 16, Height: 24, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 32, Lines: 4,
	}
	// 工具条坐标来自 hd.exe 的按钮注册函数：每列 x=602/682，
	// 行距 22，从屏幕 y=470（对话框内 y=40）到 558。
	buttons := []MenuButton{
		{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 602, Y: 470}},
		{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 682, Y: 470}},
		{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 602, Y: 492}},
		{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 682, Y: 492}},
		{Key: "save", UIButton: UIButton{Image: "btn_save", Frames: 4, X: 602, Y: 514}},
		{Key: "load", UIButton: UIButton{Image: "btn_load", Frames: 4, X: 682, Y: 514}},
		{Key: "config", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 602, Y: 536}},
		{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 682, Y: 536}},
		{Key: "voice", UIButton: UIButton{Image: "btn_vplay", Frames: 4, X: 602, Y: 558}},
		{Key: "dress", UIButton: UIButton{Image: "btn_dress", Frames: 4, X: 682, Y: 558}},
		// CUT IN 是 32x30 的竖排 4 帧，位于工具面板左上方。
		{Key: "cut in", UIButton: UIButton{Image: "btn_cutin", Frames: 4, X: 572, Y: 450}},
	}
	cfg.GameMenu = GameMenu{
		Overlay:    true,
		ShowArea:   [4]int{0, 430, 800, 170},
		Persistent: false,
		Buttons:    buttons,
	}
}

func applyHimedoreiSelect(cfg *BsxxConfiguration) {
	// item0 是 520x48（2 帧 520x24），上下各有一张 520x64 的装饰头/脚。
	// hd.exe 的 select 窗口把 head/foot 只画一次（整组顶部/底部），
	// 选项间不重复；文字颜色是默认浅灰 0xdcdcdc、选中浅蓝 0x78a0c0。
	cfg.SelectWindow = NewSelectWindowConfiguration(400, 1, []SelectDesign{
		{
			Item:         "item0",
			Interval:     2,
			TextX:        260,
			TextY:        2,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffdcdcdc,
			AlreadyColor: 0xff9a9a9a,
			SelectColor:  0xff78a0c0,
			HeaderFooter: true,
			// 头/脚是整组装饰，不是每行分隔线。
			HeaderFooterOnce: true,
		},
	})
}

func applyHimedoreiDataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase: "config_base", LoadBase: "config_base", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 12,
		SlotBase: "dm_slotbase",
		// dm_slot 是 176x136 的三态框：常态/悬停/空格。缩略图内缩 8px。
		SlotFrame: "dm_slot", SlotFrameFrames: 3,
		SlotFrameHover: 1, SlotFrameSelected: 1,
		SlotThumb:     [4]int{8, 8, 160, 120},
		AutoSlotIndex: 0,
		AutoOnly:      "dm_auto",
		NoData:        "dm_nodata",
	}
	// dm_hit 的槽位是 4 列 x 3 行：第一行只有后三格出现在命中表里，
	// 第一格 (32,112) 是自动存档，引擎仍然把 12 格全部画出来。
	columns := [4]int{32, 224, 416, 608}
	rows := [3]int{112, 248, 384}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// 页码 1..10 在 (244..532, 76)，每格 24x24；dm_pagenum 是底块，
	// dm_number 是 8x16 的数字条（10 帧，第 1 页取第 1 帧）。
	screen.PageNumbers = UIButton{Image: "dm_number", Frames: 10}
	screen.PageNumberStart = 1
	for index := 0; index < 10; index++ {
		x := 244 + 32*index
		screen.PageHits = append(screen.PageHits, UIPoint{X: x, Y: 76})
		screen.PageButtons = append(screen.PageButtons, UIButton{
			Image: "dm_pagenum", Frames: 3, X: x, Y: 76,
			Hit: [4]int{x, 76, 24, 24},
		})
		screen.PageNumberPos = append(screen.PageNumberPos, UIPoint{X: x + 8, Y: 84})
	}
	screen.PageSelectFrame = UIButton{Image: "dm_pagebase", Frames: 1, X: 232, Y: 64}
	// 底部左三颗是 BACK/TITLE/EXIT（cfg_button 0/1/2），
	// 右三颗是 SAVE/LOAD/CONFIG（cfg_mode_*），命中块都是内圈 64x64。
	screen.Back = UIButton{Image: "cfg_button", Frames: 6, Start: 0, FixedFrame: true,
		X: 9, Y: 513, Hit: [4]int{24, 528, 64, 64}}
	screen.Title = UIButton{Image: "cfg_button", Frames: 6, Start: 1, FixedFrame: true,
		X: 89, Y: 513, Hit: [4]int{104, 528, 64, 64}}
	screen.Exit = UIButton{Image: "cfg_button", Frames: 6, Start: 2, FixedFrame: true,
		X: 169, Y: 513, Hit: [4]int{184, 528, 64, 64}}
	screen.SaveMode = UIButton{Image: "cfg_mode_save", Frames: 3, X: 537, Y: 513,
		Hit: [4]int{552, 528, 64, 64}}
	screen.LoadMode = UIButton{Image: "cfg_mode_load", Frames: 3, X: 617, Y: 513,
		Hit: [4]int{632, 528, 64, 64}}
	screen.Config = UIButton{Image: "cfg_mode_config", Frames: 3, X: 697, Y: 513,
		Hit: [4]int{712, 528, 64, 64}}
	cfg.DataScreen = screen
}

func applyHimedoreiConfig(cfg *BsxxConfiguration) {
	layout := LegacyConfigScreen{
		Base:     "cfg_ex",
		Backdrop: "config_base",
		BaseX:    24,
		BaseY:    98,
		HitTable: "config_hit",
		// cfg_mark 是 16x48 的三帧：方框/圆点/菱形。
		CheckMark:       "cfg_mark",
		CheckMarkFrames: 3,
		RadioButton:     "cfg_mark",
		RadioFrames:     3,
		// 标签文字在图标右侧，点整行也要生效。
		ToggleHitWidth: 120,
		ScreenMode:     UIPoint{X: 104, Y: 156},
		VoiceLoop:      UIPoint{X: 654, Y: 132},
		SkipUnread:     UIPoint{X: 430, Y: 192},
		// スキップ処理 的行序是 通常(212) / 高速(232) / 次の選択肢まで(172)。
		SkipModes:         []UIPoint{{X: 430, Y: 212}, {X: 430, Y: 232}, {X: 430, Y: 172}},
		SkipModeMarkFrame: 1,
	}
	// 画面効果 / 中出し・外出し選択肢 / 射精位置選択肢 都是三选一。
	layout.RadioGroups = []LegacyRadioGroup{
		{Key: "screen effect", MarkFrame: 1, HitWidth: 160, Points: []UIPoint{
			{X: 104, Y: 192}, {X: 104, Y: 212}, {X: 104, Y: 232},
		}},
		{Key: "auto select pos", MarkFrame: 1, HitWidth: 160, Points: []UIPoint{
			{X: 104, Y: 312}, {X: 104, Y: 332}, {X: 104, Y: 352},
		}},
		{Key: "measure", MarkFrame: 1, HitWidth: 160, Points: []UIPoint{
			{X: 104, Y: 392}, {X: 104, Y: 412}, {X: 104, Y: 432},
		}},
	}
	// MESSAGE 面板的两行速度都是 10 个刻度点（x=446..601，16px 一格）。
	layout.Speeds = []LegacyGauge{
		{Dots: himedoreiSpeedDots(132), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
		{Dots: himedoreiSpeedDots(152), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
	}
	// BACKGROUND 四条色条：赤/緑/青/濃度，cfg_meter 的 1..4 帧。
	layout.Colors = make([]LegacyGauge, 4)
	for index := 0; index < 4; index++ {
		y := 292 + 20*index
		layout.Colors[index] = himedoreiGauge("cfg_meter", index+1, y)
	}
	// VOLUME 四条：BGM(1) / 環境音(3) / 音声(4) / 効果音(2)。
	layout.Volumes = make([]LegacyGauge, 5)
	volumeRows := []struct{ channel, y int }{{1, 416}, {3, 436}, {4, 456}, {2, 476}}
	for _, row := range volumeRows {
		layout.Volumes[row.channel] = himedoreiGauge("cfg_meter", 0, row.y)
	}
	// 右侧一列五张 64x64 角色头像。
	characterY := []int{152, 220, 288, 356, 424}
	for index := 1; index <= 5; index++ {
		layout.Characters = append(layout.Characters, LegacyCharacter{
			Button: UIButton{
				Image: "cfg_ch" + itoa(index), Frames: 1,
				X: 684, Y: characterY[index-1],
			},
		})
	}
	// cfg_ex 面板上的两个额外复选框。
	layout.ExtraToggles = []LegacyToggle{
		{Key: "anti alias", X: 104, Y: 252},
		{Key: "cutin", X: 104, Y: 272},
	}
	layout.Reset = UIButton{Image: "cfg_reset", Frames: 2, X: 28, Y: 468,
		Hit: [4]int{28, 468, 240, 32}}
	layout.Back = UIButton{Image: "cfg_button", Frames: 6, Start: 0, FixedFrame: true,
		X: 9, Y: 513, Hit: [4]int{24, 528, 64, 64}}
	layout.Title = UIButton{Image: "cfg_button", Frames: 6, Start: 1, FixedFrame: true,
		X: 89, Y: 513, Hit: [4]int{104, 528, 64, 64}}
	layout.Exit = UIButton{Image: "cfg_button", Frames: 6, Start: 2, FixedFrame: true,
		X: 169, Y: 513, Hit: [4]int{184, 528, 64, 64}}
	layout.ModeSave = UIButton{Image: "cfg_mode_save", Frames: 3, X: 537, Y: 513,
		Hit: [4]int{552, 528, 64, 64}}
	layout.ModeLoad = UIButton{Image: "cfg_mode_load", Frames: 3, X: 617, Y: 513,
		Hit: [4]int{632, 528, 64, 64}}
	layout.ModeConfig = UIButton{Image: "cfg_mode_config", Frames: 3, X: 697, Y: 513,
		Hit: [4]int{712, 528, 64, 64}}
	cfg.LegacyConfig = layout
}

func himedoreiSpeedDots(y int) []UIPoint {
	points := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		points = append(points, UIPoint{X: 446 + 16*index, Y: y})
	}
	return points
}

func himedoreiGauge(image string, frame, y int) LegacyGauge {
	// 底槽已经在 cfg_ex 里画好，引擎按值截取左侧一段填色。
	// 两端 cfg_min/cfg_max 是 32x32 的圆钮，命中块是 24x16 的内圈。
	return LegacyGauge{
		Bar: UIButton{
			Image: image, Frames: 5, X: 376, Y: y,
			Start: frame, FixedFrame: true,
		},
		Min: UIButton{Image: "cfg_min", Frames: 2, X: 348, Y: y - 8,
			Hit: [4]int{352, y, 24, 16}},
		Max: UIButton{Image: "cfg_max", Frames: 2, X: 588, Y: y - 8,
			Hit: [4]int{592, y, 24, 16}},
		Fill: true, TrackWidth: 216,
	}
}

func applyHimedoreiMuseum(cfg *BsxxConfiguration) {
	museum := &cfg.Museum
	museum.Base = "museum_base"
	// museum_menu 是 176x540 的 5 行 x 3 态图：0..4 常态、
	// 5..9 悬停、10..14 选中。页签文字已经烘焙在图上。
	modeRows := []struct {
		key   string
		state uint32
		row   int
	}{
		{"music", 1, 0},
		{"cg", 2, 1},
		{"scene", 4, 2},
		{"strip", 8, 3},
	}
	for _, item := range modeRows {
		// hd.exe 的鉴赏室初始化把四个模式页签画在 x=32、y=44+48*row；
		// 每张 museum_menu 帧高 36，行间留 12px，和原版左下花纹对齐。
		y := 44 + 48*item.row
		museum.Modes = append(museum.Modes, MuseumMode{
			Key: item.key, State: item.state,
			UIButton: UIButton{
				Image: "museum_menu", Frames: 15, X: 32, Y: y,
				FrameOffset: item.row, FrameStep: 5,
				Hit: [4]int{32, y, 176, 36},
			},
		})
	}
	museum.Return = UIButton{
		Image: "museum_menu", Frames: 15, X: 32, Y: 236,
		FrameOffset: 4, FrameStep: 5,
		Hit: [4]int{32, 236, 176, 36},
	}

	// BGM 页：曲名已经烘焙在 museum_titlelistbase 里，museum_titlelist
	// 的 20 帧是同名的紫色高亮条，只补画悬停/当前行。
	music := &museum.Music
	music.PageBase = UIButton{Image: "museum_titlelistbase", Frames: 1, X: 316, Y: 22}
	music.BakedTracks = true
	music.TrackSelect = UIButton{Image: "museum_titlelist", Frames: 20, X: 316, Y: 22}
	music.TrackCount = 20
	for index := 0; index < 20; index++ {
		y := 22 + 28*index
		music.Tracks = append(music.Tracks, UIButton{
			Image: "museum_titlelist", Frames: 20,
			X: 304, Y: y, Hit: [4]int{304, y, 240, 28},
		})
		music.Names = append(music.Names, "bgm"+twoDigits(index+1))
	}
	music.ControlBase = UIButton{Image: "museum_bgmctrl", Frames: 1, X: 14, Y: 508}
	music.Prev = UIButton{Image: "museum_prev", Frames: 3, X: 122, Y: 512}
	music.Stop = UIButton{Image: "museum_stop", Frames: 3, X: 58, Y: 512}
	music.Play = UIButton{Image: "museum_play", Frames: 3, X: 16, Y: 512}
	music.Next = UIButton{Image: "museum_next", Frames: 3, X: 184, Y: 512}
	music.Min = UIButton{Image: "museum_min", Frames: 2, X: 32, Y: 572}
	music.Max = UIButton{Image: "museum_max", Frames: 2, X: 176, Y: 572}
	music.Volume = UIButton{Image: "museum_volume", Frames: 1, X: 20, Y: 544}
	music.VolumeFill = true
	music.VolumeRegion = [4]int{20, 544, 200, 16}

	// CG / シーン页：博物馆的羊皮纸区域里 4 列 x 5 行缩略图。
	// cg0_thumb..cg6_thumb 与 scene0_thumb..scene3_thumb 是 20 帧的预制条。
	museum.CG.Thumbs = himedoreiMuseumGrid()
	museum.CG.ThumbX, museum.CG.ThumbY = 256, 48
	museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	for index := 0; index < 7; index++ {
		museum.CG.ThumbStrips = append(museum.CG.ThumbStrips, "cg"+itoa(index)+"_thumb")
		museum.CG.PageButtons = append(museum.CG.PageButtons, UIButton{
			Image: "museum_page", Frames: 3, X: 280 + 30*index, Y: 560,
		})
	}
	museum.Scene.Thumbs = himedoreiMuseumGrid()
	museum.Scene.ThumbX, museum.Scene.ThumbY = 256, 48
	museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	for index := 0; index < 4; index++ {
		museum.Scene.ThumbStrips = append(museum.Scene.ThumbStrips, "scene"+itoa(index)+"_thumb")
		museum.Scene.PageButtons = append(museum.Scene.PageButtons, UIButton{
			Image: "museum_page", Frames: 3, X: 280 + 30*index, Y: 560,
		})
	}

	// 立ち絵鉴赏：museum_ch1..5 是 216x216 的角色面板，按图头坐标摆放。
	portraits := []UIPoint{
		{X: 256, Y: 24}, {X: 552, Y: 20}, {X: 256, Y: 360},
		{X: 552, Y: 364}, {X: 409, Y: 192},
	}
	for index, point := range portraits {
		museum.StripPortraits = append(museum.StripPortraits, UIButton{
			Image: "museum_ch" + itoa(index+1), Frames: 1,
			X: point.X, Y: point.Y,
		})
	}

	// CG 查看器：cgm_btn 是 24x240 的 10 帧图（常态 0..4、红色悬停 5..9）。
	museum.Viewer = CGViewer{
		PanelX: 700, PanelY: 560,
		Prev:       UIButton{Image: "cgm_btn", Frames: 10, Start: 0, X: 700, Y: 568},
		Exit:       UIButton{Image: "cgm_btn", Frames: 10, Start: 1, X: 724, Y: 568},
		Next:       UIButton{Image: "cgm_btn", Frames: 10, Start: 2, X: 748, Y: 568},
		Config:     UIButton{Image: "cgm_btn", Frames: 10, Start: 3, X: 772, Y: 568},
		ButtonBase: 0, HoverOffset: 5,
	}
}

func himedoreiMuseumGrid() []UIPoint {
	points := make([]UIPoint, 0, 20)
	// hd.exe 的 CG/Scene 缩略图排布：x=256+132*col、y=48+102*row，
	// 4 列 x 5 行，格子 120x90（缩略图条的单帧尺寸）。
	for row := 0; row < 5; row++ {
		for column := 0; column < 4; column++ {
			points = append(points, UIPoint{X: 256 + 132*column, Y: 48 + 102*row})
		}
	}
	return points
}

func applyHimedoreiLog(cfg *BsxxConfiguration) {
	// 回想页：原版没有 kaisou 整屏底图，用半透明黑 + 右侧 log_meter。
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 10, LeftPos: 30, Width: 700,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 748, Y: 24},
		VoiceIcon: "log_vp",
	}
}
