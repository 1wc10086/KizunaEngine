package config

import "fmt"

// builtin_bishop_gk.go 给「学園〜恥辱の図式〜」（GK / GKDVD / GKWindows10）补内置布局。
//
// 这三个发行版的资源完全同名（BSScript Rev.4、发行目录没有 bsxx.cfg），
// 原引擎把标题、对话框、存读档、设置、鉴赏和地图的坐标编译在 gk.exe 里。
// 配置为空时旧行为是：boot 里 `system call "title"` 之后直接落进序章
// （没有主界面）、对话框只由通用的 message 兜底（正文从 x=24 起，压到
// 左侧雕花边框外，且工具条没有热区）、地图用 MapPrms 的通用几何
// （热区从 x=116 起，实际按钮在 x=620）。
//
// 坐标来源（都能在发行素材里核对）：
//   - 标题：title_hit 的 5 个命中块 x=88..275、y=260+48i；title_menu 是
//     188x288 的 6 帧竖排图（每帧 188x48），第 0..4 帧与五个菜单项一一对应，
//     title_base 已经烘焙了常态文字，所以按钮只做悬停叠加。
//   - 对话框：message 是 800x176 的窗口框，贴屏幕底部 (0,424)；message_hit
//     给出 9 个按钮热区。正文从 x=64 起，名字和正文同一行（GK 的手册截图）。
//   - 存读档：save_base/load_base 的 6 个 320x160 槽位（dm_hit 的 56/424
//     两列、32/200/368 三行），槽位框是 dm_slot 的三帧 320x160。
//   - 设置：config_base + config_hit 的色块（10 格速度刻度、三条音量条、
//     四条文字背景条、六个角色语音页签）。
//   - 鉴赏室：museum_base 整屏、museum_menu 的 4 行三态页签、
//     museum_titlelist 的 15 首 BGM 列表、cgN_thumb/sceneN_thumb 缩略图条。
//   - 地图：MapPrms.dat 的 12 个地点标签 + map_place01..12 的图头坐标
//     (620,116+38i)；map_submenu 是顶部工具条底图，date_base/map_face 是
//     左上日期卡和右上头像。
//
// 只填空缺项，且只在"配置确实为空 + 素材签名吻合"时生效；侵蝕/淫獄/
// 学園2 等已经适配的作品不会走到这里。

// builtinGKAssets 是这一代独有的素材组合。
var builtinGKAssets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_hit",
	"save_base", "load_base", "dm_hit", "dm_slot", "dm_pagenum",
	"museum_base", "museum_menu", "museum_titlelist", "cg0_thumb",
	"config_base", "config_hit", "cfg_mark", "cfg_meter",
	"map_base", "map_place01", "map_submenu", "date_base", "map_face",
}

// builtinGKExcluded 是相邻世代独有的素材，用来避免误吃学園2/AFDVD/KD。
var builtinGKExcluded = []string{
	"title_menu_op", "title_menu_begin", "gamemenu", "dm_save", "dm_load",
	"dm_base", "kaisou", "msg_base", "date_info", "map_hit",
}

func isBuiltinGK(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinGKAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinGKExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// applyBuiltinGK 填入整套界面。返回 false 表示没有命中这一代。
func applyBuiltinGK(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !isBuiltinGK(probe) {
		return false
	}
	cfg.Width = 800
	cfg.Height = 600
	// 存档缩略图与 dm_hit 的槽位一致：320x160 的卡片里贴 140x120。
	cfg.ThumbnailRatio = 0.2
	applyGKTitle(cfg)
	applyGKDateCard(cfg)
	applyGKMessage(cfg)
	applyGKFont(cfg)
	applyGKGameMenu(cfg)
	applyGKSelect(cfg)
	applyGKDataScreen(cfg)
	applyGKLog(cfg)
	applyGKMuseum(cfg)
	applyGKConfig(cfg)
	return true
}

func applyGKTitle(cfg *BsxxConfiguration) {
	// 五个菜单项共用 title_menu 的第 0..4 帧；title_base 已经画好常态文字，
	// 平时只显示底图，悬停时叠上对应的高亮帧。
	const menuX = 88
	entries := []TitleMenuEntry{
		{Key: "start", Image: "title_menu", Frames: 6, Frame: 0, X: menuX, Y: 260, HoverOnly: true},
		{Key: "load", Image: "title_menu", Frames: 6, Frame: 1, X: menuX, Y: 308, HoverOnly: true},
		{Key: "museum", Image: "title_menu", Frames: 6, Frame: 2, X: menuX, Y: 356, HoverOnly: true},
		{Key: "config", Image: "title_menu", Frames: 6, Frame: 3, X: menuX, Y: 404, HoverOnly: true},
		{Key: "quit", Image: "title_menu", Frames: 6, Frame: 4, X: menuX, Y: 452, HoverOnly: true},
	}
	cfg.Title = TitlePhase{
		Base:     "title_base",
		HitTable: "title_hit",
		Entries:  entries,
		// boot 在 system call "title" 之后紧接序章初始化，START 恢复快照继续。
		ResumeAfterTitleCall: true,
	}
}

func applyGKDateCard(cfg *BsxxConfiguration) {
	// 对话/选择肢左上角的日期卡：date_base 132x68，里面的周数/曜日/
	// AM-PM/时/分都是独立帧图。坐标取自手册截图与 date_base 的留白。
	cfg.DateCard = DateCard{
		Base:   "date_base",
		X:      0,
		Y:      0,
		WeekNo: UIPoint{X: 8, Y: 2},
		Week:   UIPoint{X: 8, Y: 38},
		AMPM:   UIPoint{X: 54, Y: 38},
		Hour:   UIPoint{X: 76, Y: 38},
		Minute: UIPoint{X: 98, Y: 38},
	}
}

func applyGKMessage(cfg *BsxxConfiguration) {
	// message.bsg 是 800x176 的整窗框，底部锚定 (0,424)。窗口内部是透明的，
	// 原引擎用玩家设置的文字背景色填满（默认近黑的深绿褐）；FillBase 先铺色
	// 再叠框，雕花边框的像素会正常盖回去。
	cfg.Message = MessageWindow{
		Base:            "message",
		BasePosSet:      true,
		BaseX:           0,
		BaseY:           424,
		Speed:           100,
		BgColor:         0xC814120E,
		FillBase:        true,
		NameX:           64,
		NameY:           20,
		NameW:           240,
		NameH:           30,
		NameColor:       0xffffe8c0,
		TextX:           64,
		TextY:           20,
		TextW:           664,
		TextH:           108,
		SpeakerBrackets: true,
		SpeakerInline:   true,
		Unagashi:        "unagashi",
		UnagashiX:       700,
		UnagashiY:       120,
		UnagashiCellW:   24,
		UnagashiCellH:   24,
		UnagashiColumns: 1,
		UnagashiFrames:  7,
		UnagashiSpeed:   120,
	}
}

func applyGKFont(cfg *BsxxConfiguration) {
	// 原版一行约 25 个全角字（手册截图「いつの間にか…教室の入り口」正好 25 字），
	// 24 号字、行高 30、正文区 108px 正好 3 行。
	cfg.Font = FontSettings{
		Size:       24,
		Width:      24,
		Height:     30,
		Weight:     1000,
		Edge:       1,
		Shadow:     1,
		LineLength: 25,
		Lines:      3,
	}
}

func applyGKGameMenu(cfg *BsxxConfiguration) {
	// 按钮标签已经烘焙在 message.bsg 上；btn_* 是 4 帧（常态/悬停/按下/不可用），
	// 贴在同一个位置，悬停时画第 1 帧就得到原版的蓝色高亮。
	button := func(key, image string, frames, x, y int) MenuButton {
		return MenuButton{Key: key, UIButton: UIButton{Image: image, Frames: frames, X: x, Y: y}}
	}
	cfg.GameMenu = GameMenu{
		Base:     "",
		BaseX:    0,
		BaseY:    424,
		ShowArea: [4]int{0, 424, 800, 176},
		Buttons: []MenuButton{
			button("voice", "btn_voice", 4, 508, 570),
			button("save", "btn_save", 4, 220, 570),
			button("load", "btn_load", 4, 292, 570),
			button("config", "btn_config", 4, 364, 570),
			button("log", "btn_log", 4, 436, 570),
			button("quick save", "btn_qsave", 4, 743, 457),
			button("quick load", "btn_qload", 4, 743, 485),
			button("skip", "btn_skip", 4, 743, 513),
			button("auto ex", "btn_auto", 4, 743, 541),
			button("window hide", "btn_hide", 4, 769, 428),
		},
	}
}

func applyGKSelect(cfg *BsxxConfiguration) {
	// item0 是 520x48 的单帧选项条，item0_head/foot 是上下各 8px 的装饰。
	// 文字画在条的中线上，选中的颜色与标题高亮一致。
	cfg.SelectWindow = NewSelectWindowConfiguration(300, 1, []SelectDesign{
		{
			Item:         "item0",
			Interval:     2,
			TextX:        260,
			TextY:        8,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0x88ffffff,
			SelectColor:  0xffffcf60,
			HeaderFooter: true,
			Frames:       1,
		},
	})
}

func applyGKDataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase: "save_base",
		LoadBase: "load_base",
		HitTable: "dm_hit",
		NoBegin:  0,
		PageMax:  10,
		// 每页只有 5 个槽位：右下角是 Page No./Shortcut 面板，不是第 6 格。
		SlotsPerPage: 5,
		NoData:       "dm_nodata",
		AutoOnly:     "dm_auto",
		// 第 0 页左上角是自动存档格（只读），显示内部 AutoSlot。
		AutoSlotIndex:     0,
		AutoSlotGrid:      true,
		SlotFrame:         "dm_slot",
		SlotFrameFrames:   3,
		SlotFrameGeneric:  true,
		SlotFrameOpaque:   true,
		SlotFrameHover:    1,
		SlotFrameSelected: 1,
		SlotEmptyFrame:    2,
		SlotSize:          [2]int{320, 160},
		SlotThumb:         [4]int{8, 8, 160, 120},
		SlotTextX:         180,
		SlotTextY:         8,
		SlotTextLineGap:   22,
		SlotTextColor:     0xffffffff,
		SlotNumberPrefix:  "No. ",
		SlotNumberWidth:   3,
		// 页码是"两位数字 + 上下箭头"，不是逐页按钮。
		PageNumber:    UIButton{Image: "dm_pagenum", Frames: 10, X: 470, Y: 404},
		PageNumberX:   470,
		PageNumberY:   404,
		PageNumberGap: 44,
		PagePrevPoint: UIPoint{X: 452, Y: 370},
		PageNextPoint: UIPoint{X: 452, Y: 474},
		// 右下 Shortcut Button：Auto / Quick。
		ShortcutAutoPoint:  UIPoint{X: 584, Y: 401},
		ShortcutQuickPoint: UIPoint{X: 584, Y: 448},
		// 底部三个按钮的图是 cfg_button 的 0..2 帧（戻る/タイトルに戻る/終了）。
		Back:  UIButton{Image: "cfg_button", Frames: 6, X: 15, Y: 546, Start: 0, FixedFrame: true, Hit: [4]int{100, 548, 52, 32}},
		Title: UIButton{Image: "cfg_button", Frames: 6, X: 149, Y: 546, Start: 1, FixedFrame: true, Hit: [4]int{176, 548, 172, 32}},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, X: 371, Y: 546, Start: 2, FixedFrame: true, Hit: [4]int{372, 548, 220, 32}},
	}
	// dm_hit 的 5 个槽位：左列 3 格 + 右列上两格。
	screen.Slots = []UIPoint{
		{X: 56, Y: 32}, {X: 424, Y: 32},
		{X: 56, Y: 200}, {X: 424, Y: 200},
		{X: 56, Y: 368},
	}
	cfg.DataScreen = screen
}

func applyGKLog(cfg *BsxxConfiguration) {
	// 没有 log_base：回想叠在剧情画面上，右侧是 log_meter 滚动条，
	// 有语音的行左侧画 log_vp 图标。
	cfg.Log = LogScreen{
		HasBackColor: true,
		BackColor:    0xb4000000,
		Lines:        6,
		LeftPos:      96,
		Width:        640,
		Bar:          UIButton{Image: "log_meter", Frames: 1, X: 736, Y: 24},
		Up:           UIButton{Image: "log_uarrow", Frames: 1, X: 724, Y: 0},
		Down:         UIButton{Image: "log_darrow", Frames: 1, X: 724, Y: 576},
		VoiceIcon:    "log_vp",
		FontSize:     16,
	}
}

func applyGKMuseum(cfg *BsxxConfiguration) {
	// museum_menu 是 106x264 的 12 行图：0..3 常态、4..7 悬停、8..11 选中，
	// 四行依次是 CG鑑賞 / シーン鑑賞 / 立ち絵鑑賞 / 終了。用 FrameOffset +
	// FrameStep 让引擎按"行 + 状态*4"取帧。
	tab := func(row int) UIButton {
		return UIButton{
			Image: "museum_menu", Frames: 12,
			X: 686, Y: 16 + row*24,
			FrameOffset: row, FrameStep: 4,
		}
	}
	pageDigit := func(index int) UIButton {
		return UIButton{Image: "museum_num", Frames: 12, X: 0, Y: 0, Start: index + 1, FixedFrame: true}
	}
	// 缩略图条是 120x90 的 20 帧竖排图，5 列 x 4 行铺满右侧。
	thumbs := make([]UIPoint, 0, 20)
	for row := 0; row < 4; row++ {
		for column := 0; column < 5; column++ {
			thumbs = append(thumbs, UIPoint{X: 200 + column*120, Y: 40 + row*90})
		}
	}
	pageButtons := []UIButton{
		pageDigit(0), pageDigit(1), pageDigit(2), pageDigit(3),
	}
	// 页码按钮的图片坐标由引擎按 index 画；这里把 4 个数字排在底部。
	for index := range pageButtons {
		pageButtons[index].X = 560 + index*24
		pageButtons[index].Y = 526
	}
	cfg.Museum = Museum{
		Base:        "museum_base",
		DefaultMode: "music",
		Modes: []MuseumMode{
			{Key: "cg", UIButton: tab(0)},
			{Key: "scene", UIButton: tab(1)},
			{Key: "strip", UIButton: tab(2)},
			// BGM 没有独立页签：原版打开鉴赏室就是 BGM Player + 曲目列表，
			// CG/Scene/立绘由上方三个页签切换。空按钮只作为默认页存在。
			{Key: "music"},
		},
		Return: tab(3),
		CG: MuseumCGPage{
			PageBase:    UIButton{Image: "museum_pageline", Frames: 1, X: 48, Y: 500},
			Thumbs:      thumbs,
			ThumbStrips: []string{"cg0_thumb", "cg1_thumb", "cg2_thumb", "cg3_thumb"},
			ThumbX:      200,
			ThumbY:      40,
			ThumbW:      120,
			ThumbH:      90,
			PageButtons: pageButtons,
			// 达成率数字条：museum_num 的 0..9 帧；位置在底部左侧标签右侧。
			Percent:      UIButton{Image: "museum_num", Frames: 12, X: 132, Y: 526},
			PercentSpace: 4,
		},
		Scene: MuseumScenePage{
			PageBase:    UIButton{Image: "museum_pageline", Frames: 1, X: 48, Y: 500},
			Thumbs:      thumbs[:20],
			ThumbStrips: []string{"scene0_thumb", "scene1_thumb", "scene2_thumb"},
			ThumbX:      200,
			ThumbY:      40,
			ThumbW:      120,
			ThumbH:      90,
			PageButtons: pageButtons[:3],
			// 学園（GK）的场景鉴赏是"点缩略图直接播放"，没有
			// 即時再生/再生リスト的两种模式按钮；那两张图标属于 BGM Player。
		},
		Music: MuseumMusicPage{
			PageBase:    UIButton{Image: "museum_titlelist", Frames: 1, X: 16, Y: 16},
			BakedTracks: true,
			Tracks:      gkMusicTracks(),
			// 曲名烘焙在 museum_titlelist 里；播放需要真实的 bgm 资源名。
			Names:        gkMusicNames(),
			Play:         UIButton{Image: "museum_play", Frames: 2, X: 648, Y: 567},
			Stop:         UIButton{Image: "museum_stop", Frames: 2, X: 600, Y: 567},
			Prev:         UIButton{Image: "museum_prev", Frames: 2, X: 696, Y: 567},
			Next:         UIButton{Image: "museum_next", Frames: 2, X: 744, Y: 567},
			Volume:       UIButton{Image: "museum_volume", Frames: 1, X: 250, Y: 580},
			VolumeFill:   true,
			VolumeRegion: [4]int{250, 570, 208, 20},
		},
		Viewer: CGViewer{
			Base:         UIButton{Image: "cgm_base", Frames: 1, X: 656, Y: 540},
			PanelX:       656,
			PanelY:       540,
			Prev:         UIButton{Image: "cgm_btn", Frames: 8, X: 660, Y: 552},
			Exit:         UIButton{Image: "cgm_btn", Frames: 8, X: 684, Y: 552, Start: 1},
			Next:         UIButton{Image: "cgm_btn", Frames: 8, X: 708, Y: 552, Start: 2},
			Config:       UIButton{Image: "cgm_btn", Frames: 8, X: 732, Y: 552, Start: 3},
			ButtonBase:   0,
			HoverOffset:  4,
			TogglePanel:  true,
			ClickAdvance: true,
		},
		StripPortraits: gkStripPortraits(),
	}
}

// gkMusicNames 是 museum_titlelist 的 15 首曲目对应的 BGM 文件。
// 发行版 bgm.bsa 的顺序就是 bgm_01..bgm_15，与列表行一一对应。
func gkMusicNames() []string {
	names := make([]string, 0, 15)
	for index := 1; index <= 15; index++ {
		names = append(names, "bgm_"+fmt.Sprintf("%02d", index))
	}
	return names
}

// gkMusicTracks 是 museum_titlelist 的 15 行命中区：每行 176x22。
func gkMusicTracks() []UIButton {
	tracks := make([]UIButton, 0, 15)
	for index := 0; index < 15; index++ {
		y := 16 + index*22
		tracks = append(tracks, UIButton{
			X: 16, Y: y, Hit: [4]int{16, y, 176, 22},
		})
	}
	return tracks
}

// gkStripPortraits 是立ち絵鑑賞的六张角色卡（200x150），2 列 x 3 行。
func gkStripPortraits() []UIButton {
	cards := make([]UIButton, 0, 6)
	for index := 0; index < 6; index++ {
		cards = append(cards, UIButton{
			Image: "museum_ch" + string(rune('1'+index)), Frames: 1,
			X: 200 + (index%2)*200, Y: 40 + (index/2)*150,
		})
	}
	return cards
}

func applyGKConfig(cfg *BsxxConfiguration) {
	// config_hit 的色块给出了全部热区：
	//   0 (188,68) 全屏显示；1..3 (188,88/108/128) 画面効果；
	//   4..13 与 14..23 是两行 10 格速度刻度 (520+20i, 68/88)；
	//   24..27 (500,106/126/146/166) 跳过速度与未读跳过；
	//   28 (72,340) 语音循环；37..48 是男汁/女汁/乳汁各四个选项；
	//   29..34 是六个角色语音页签；124..127 是底部四个按钮。
	bar := func(y int, frame int) LegacyGauge {
		return LegacyGauge{
			Fill: true,
			Bar: UIButton{
				Image: "cfg_meter", Frames: 5,
				X: 452, Y: y, Start: frame, FixedFrame: true,
			},
		}
	}
	dots := func(y int) LegacyGauge {
		points := make([]UIPoint, 0, 10)
		for index := 0; index < 10; index++ {
			points = append(points, UIPoint{X: 520 + index*20, Y: y})
		}
		// cfg_mark 的三帧是方框/圆点/菱形；速度刻度用圆点，Start=-1 让
		// drawFrameButton 的 "+2" 落回第 1 帧。
		return LegacyGauge{Dots: points, DotMark: UIButton{Image: "cfg_mark", Frames: 3, Start: -1}}
	}
	cfg.LegacyConfig = LegacyConfigScreen{
		Base:            "config_base",
		HitTable:        "config_hit",
		CheckMark:       "cfg_mark",
		RadioButton:     "cfg_mark",
		CheckMarkFrames: 3,
		RadioFrames:     3,
		ToggleHitWidth:  116,
		ScreenMode:      UIPoint{X: 188, Y: 68},
		SkipUnread:      UIPoint{X: 500, Y: 166},
		VoiceLoop:       UIPoint{X: 72, Y: 340},
		// 画面効果是单选，markFrame=1 用圆点帧。
		RadioGroups: []LegacyRadioGroup{
			{
				Key:       "screen effect",
				Points:    []UIPoint{{X: 188, Y: 88}, {X: 188, Y: 108}, {X: 188, Y: 128}},
				MarkFrame: 1,
				HitWidth:  116,
			},
			{
				Key:       "male liquid",
				Points:    []UIPoint{{X: 76, Y: 212}, {X: 76, Y: 232}, {X: 76, Y: 252}, {X: 76, Y: 272}},
				MarkFrame: 1,
				HitWidth:  72,
			},
			{
				Key:       "female liquid",
				Points:    []UIPoint{{X: 166, Y: 212}, {X: 166, Y: 232}, {X: 166, Y: 252}, {X: 166, Y: 272}},
				MarkFrame: 1,
				HitWidth:  72,
			},
			{
				Key:       "milk liquid",
				Points:    []UIPoint{{X: 256, Y: 212}, {X: 256, Y: 232}, {X: 256, Y: 252}, {X: 256, Y: 272}},
				MarkFrame: 1,
				HitWidth:  72,
			},
		},
		Speeds:            []LegacyGauge{dots(68), dots(88)},
		SkipModeMarkFrame: 1,
		SkipModes: []UIPoint{
			{X: 500, Y: 106}, {X: 500, Y: 126}, {X: 500, Y: 146},
		},
		// 顺序固定为 master/BGM/SE/BGS/音声/BGV/减衰；本作只暴露 BGM/音声/効果音。
		Volumes: []LegacyGauge{
			{},
			bar(400, 0),
			bar(460, 0),
			{},
			bar(430, 0),
			{},
			{},
		},
		// 文字背景：赤/緑/青/濃度，cfg_meter 的 1..4 帧。
		Colors: []LegacyGauge{
			bar(240, 1),
			bar(270, 2),
			bar(300, 3),
			bar(330, 4),
		},
		Characters: []LegacyCharacter{
			{Button: UIButton{Image: "cfg_ch1", Frames: 2, X: 96, Y: 368}},
			{Button: UIButton{Image: "cfg_ch2", Frames: 2, X: 168, Y: 368}},
			{Button: UIButton{Image: "cfg_ch3", Frames: 2, X: 240, Y: 368}},
			{Button: UIButton{Image: "cfg_ch4", Frames: 2, X: 96, Y: 440}},
			{Button: UIButton{Image: "cfg_ch5", Frames: 2, X: 168, Y: 440}},
			{Button: UIButton{Image: "cfg_ch6", Frames: 2, X: 240, Y: 440}},
		},
		CharacterOffFrame: 1,
		Reset:             UIButton{Image: "cfg_reset", Frames: 2, X: 358, Y: 510},
		Back:              UIButton{Image: "cfg_button", Frames: 6, X: 15, Y: 546, Start: 0, FixedFrame: true, Hit: [4]int{100, 548, 52, 32}},
		Title:             UIButton{Image: "cfg_button", Frames: 6, X: 149, Y: 546, Start: 1, FixedFrame: true, Hit: [4]int{176, 548, 172, 32}},
		Exit:              UIButton{Image: "cfg_button", Frames: 6, X: 371, Y: 546, Start: 2, FixedFrame: true, Hit: [4]int{372, 548, 220, 32}},
	}
}
