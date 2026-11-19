package config

// builtin_bishop_schoolheaven.go 给「すくーるヘブン らぶえろハーレム☆ももいろタイフーン
// 初回限定版」这一代（BSScript Rev.3、发行目录没有 bsxx.cfg）补一份内置布局。
//
// 为什么需要单独一代：
//   - 它和 KD / AFDVD 同为"配置为空 + Rev 脚本"，但素材签名完全不同：
//     没有 kaisou / dm_save / dm_load / title_menu_op，而是
//     title_menu（6 帧竖排）、message + message_hit、save_base/load_base + dm_hit、
//     dm_slot/dm_pagenum/dm_nodata/dm_auto、museum_base + museum_menu + cgN_thumb。
//   - 现有三套内置布局的签名都匹配不上，配置全空的结果就是：没有主界面
//     （boot 的 system call "title" 之后直接落进 opening）、没有对话框
//     （宿主只能显示 Flutter 的 MessageBox）、没有任何工具按钮。
//
// 坐标来源（可核对）：
//   - 标题：title_hit 的 6 个命中块（x=320..479，y=362+32*i），
//     title_menu 是 160x192 的 6 帧竖排图（每帧 160x32），帧号 = 菜单项下标。
//     命中块的 0..5 与 title_base 里烘焙的
//     プロローグ / 始めから / 続きから / 鑑賞室 / 設定 / 終了 一一对应。
//   - 对话框：message 是 800x144 的窗口框，原引擎贴在屏幕底部（600-144=456）。
//   - 工具按钮：message_hit 的 9 个 24x24 命中块（第一行 y=24、第二行 y=96），
//     按钮图 btn_* 是 24x96 的 4 帧竖排图。
//   - 存读档：save_base/load_base 的 6 个 328x168 槽位，每个槽位内嵌
//     160x120 缩略图；dm_hit 的 100..109 是右栏 10 个页码热区。
//   - 鉴赏室：museum_base 底图 + museum_menu 的 4 行页签（常态/高亮各 4 行）。
//
// 只填空缺项，且只在"配置确实为空 + 素材签名吻合"时生效，其它作品不受影响。

// builtinSchoolHeavenAssets 是这一代的资源签名。
//
// 必须同时满足"有独立工具按钮图 + message 是 800x144 的窗口框 + 存读档用
// save_base/load_base + 鉴赏室用 museum_menu"，基本只会在这一代发行版里成立。
var builtinSchoolHeavenAssets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_hit", "message_back",
	"save_base", "load_base", "dm_hit", "dm_slot", "dm_pagenum",
	"museum_base", "museum_menu", "museum_page",
	"config_base", "config_hit", "cfg_mark",
	"item0", "item0_head", "item0_foot",
	"btn_skip", "btn_voice", "btn_auto", "btn_log",
	"map_base", "map_menu",
}

// builtinSchoolHeavenExcluded 是其它世代独有的素材。
//
// msg_base（现代作品）、gamemenu（AFDVD）、dm_save/dm_load（KD）、
// kaisou（KD）、title_menu_op（KD/AFDVD 的分图按钮）出现任意一个就说明
// 不是这一代，避免把别的作品误判进来。
var builtinSchoolHeavenExcluded = []string{
	"msg_base", "gamemenu", "dm_save", "dm_load", "kaisou",
	"title_menu_op", "title_menu_begin",
}

func isBuiltinSchoolHeaven(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinSchoolHeavenAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinSchoolHeavenExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// applyBuiltinSchoolHeaven 填入标题、对话框、工具条、选择肢、存读档、回想、
// 鉴赏室和设置画面。屏幕固定 800x600：这一代的 title_base/museum_base/
// map_base 都是 800x600，和 [main window] 缺失时的探测结果一致。
func applyBuiltinSchoolHeaven(cfg *BsxxConfiguration) {
	cfg.Width = 800
	cfg.Height = 600
	// 存档缩略图 160x120 正好是 800x600 的 0.2，与 save_base 里烘焙的缩略图框一致。
	cfg.ThumbnailRatio = 0.2
	applyBuiltinSchoolHeavenTitle(cfg)
	applyBuiltinSchoolHeavenMessage(cfg)
	applyBuiltinSchoolHeavenFont(cfg)
	applyBuiltinSchoolHeavenGameMenu(cfg)
	applyBuiltinSchoolHeavenSelect(cfg)
	applyBuiltinSchoolHeavenDataScreen(cfg)
	applyBuiltinSchoolHeavenLog(cfg)
	applyBuiltinSchoolHeavenMuseum(cfg)
	applyBuiltinSchoolHeavenConfig(cfg)
}

func applyBuiltinSchoolHeavenTitle(cfg *BsxxConfiguration) {
	// 标题底图 title_base 已经把菜单文字烘焙进去了，title_menu 的 6 帧是
	// "高亮版本"（绿字）。原引擎平时只画底图，悬停时才把对应帧叠上去；
	// 这里用 HoverOnly + Frame 显式指定帧号：不悬停时不画，悬停时只画该行，
	// 渲染器就不会把 6 行一次全叠出来（那样六个菜单项会同时变绿）。
	// BGM 由剧本自己播（title 标签之前已经 PLAY_BGM），配置留空表示沿用。
	entries := []TitleMenuEntry{
		{Key: "op", Image: "title_menu", Frames: 6, Frame: 0, X: 320, Y: 362, HoverOnly: true},
		{Key: "start", Image: "title_menu", Frames: 6, Frame: 1, X: 320, Y: 394, HoverOnly: true},
		{Key: "load", Image: "title_menu", Frames: 6, Frame: 2, X: 320, Y: 426, HoverOnly: true},
		{Key: "museum", Image: "title_menu", Frames: 6, Frame: 3, X: 320, Y: 458, HoverOnly: true},
		{Key: "config", Image: "title_menu", Frames: 6, Frame: 4, X: 320, Y: 490, HoverOnly: true},
		{Key: "quit", Image: "title_menu", Frames: 6, Frame: 5, X: 320, Y: 522, HoverOnly: true},
	}
	cfg.Title = TitlePhase{Base: "title_base", HitTable: "title_hit", Entries: entries}
}

func applyBuiltinSchoolHeavenMessage(cfg *BsxxConfiguration) {
	// message.bsg 是 800x144 的窗口框（只画边框/花纹），贴在屏幕底部
	// （600-144=456）。message_back 是同尺寸的 8 位索引图、调色板 alpha 全 0，
	// 它是原引擎用来"按形状填底色"的掩码，直接当图片画会全透明，
	// 所以窗口底色仍由 BgColor 平铺（半透明白，和原版观感一致）。
	cfg.Message = MessageWindow{
		Base:            "message_back",
		BaseOpaque:      true,
		BasePosSet:      true,
		BaseX:           0,
		BaseY:           600 - 144,
		Speed:           100,
		BgColor:         0,
		NameX:           24,
		NameY:           22,
		NameW:           560,
		NameH:           25,
		NameColor:       0xffff8822,
		TextX:           24,
		TextY:           50,
		TextW:           580,
		TextH:           78,
		Unagashi:        "unagashi",
		UnagashiX:       620,
		UnagashiY:       96,
		UnagashiCellW:   32,
		UnagashiCellH:   32,
		UnagashiColumns: 1,
		UnagashiFrames:  4,
		UnagashiSpeed:   150,
	}
}

func applyBuiltinSchoolHeavenFont(cfg *BsxxConfiguration) {
	// 16px 字号、25px 行高：对话框正文区 78px 正好 3 行。
	cfg.Font = FontSettings{
		Size:       16,
		Width:      22,
		Height:     25,
		Weight:     1000,
		Edge:       1,
		Shadow:     2,
		LineLength: 27,
		Lines:      3,
	}
}

func applyBuiltinSchoolHeavenGameMenu(cfg *BsxxConfiguration) {
	// 工具按钮没有单独的菜单条底图：它们直接画在 message 窗口框上，
	// 坐标来自 message_hit 的 9 个命中块（相对窗口左上角的 24/96 两行）。
	// btn_* 是 24x96 的 4 帧竖排图（常态/悬停/按下/不可用）。
	const baseY = 600 - 144
	button := func(key, image string, x, y int) MenuButton {
		return MenuButton{Key: key, UIButton: UIButton{Image: image, Frames: 4, X: x, Y: baseY + y}}
	}
	cfg.GameMenu = GameMenu{
		Base:     "",
		BaseX:    0,
		BaseY:    baseY,
		ShowArea: [4]int{0, baseY, 800, 144},
		Buttons: []MenuButton{
			button("voice", "btn_voice", 682, 24),
			button("skip", "btn_skip", 708, 24),
			button("quick save", "btn_qsave", 734, 24),
			button("quick load", "btn_qload", 760, 24),
			button("save", "btn_save", 656, 96),
			button("load", "btn_load", 682, 96),
			button("config", "btn_config", 708, 96),
			button("status", "btn_status", 734, 96),
			button("log", "btn_log", 760, 96),
		},
	}
}

func applyBuiltinSchoolHeavenSelect(cfg *BsxxConfiguration) {
	// item0 是 550x90 的单帧选项条，上下各有一条装饰图（item0_head/item0_foot）。
	// 文字基准点取条宽的一半（275）做居中，Y 取条高的 1/3 让三行字落在条内。
	cfg.SelectWindow = NewSelectWindowConfiguration(291, 1, []SelectDesign{
		{
			Item:         "item0",
			Interval:     0,
			TextX:        275,
			TextY:        30,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0x88ffffff,
			SelectColor:  0xffff4488,
			HeaderFooter: true,
		},
	})
}

func applyBuiltinSchoolHeavenDataScreen(cfg *BsxxConfiguration) {
	// save_base/load_base 的槽位是 2 列 x 3 行，每格 328x168：
	//   (32,16) (376,16) / (32,192) (376,192) / (32,368) (376,368)。
	// 每格右侧内嵌 160x120 的缩略图框，左上角相对槽位是 (153,8)。
	//
	// 引擎默认把 Slots 当作"缩略图左上角"来贴图和命中，所以这里直接给
	// 缩略图坐标，命中区也就是缩略图大小 —— 玩家点缩略图即可选中。
	slots := make([]UIPoint, 0, 6)
	for row := 0; row < 3; row++ {
		for column := 0; column < 2; column++ {
			slots = append(slots, UIPoint{X: 32 + 153 + column*344, Y: 16 + 8 + row*176})
		}
	}
	// 右栏 10 个页码热区：dm_hit 的 100..109 在 (728, 16+40*i)，每格 32x32。
	pages := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		pages = append(pages, UIPoint{X: 728, Y: 16 + index*40})
	}
	cfg.DataScreen = DataScreen{
		SaveBase:     "save_base",
		LoadBase:     "load_base",
		HitTable:     "dm_hit",
		NoBegin:      1,
		PageMax:      10,
		SlotsPerPage: 6,
		Slots:        slots,
		PageHits:     pages,
		NoData:       "dm_nodata",
		AutoOnly:     "dm_auto",
	}
}

func applyBuiltinSchoolHeavenLog(cfg *BsxxConfiguration) {
	// 这一代没有 log_base：回想是在剧情帧上盖半透明底色画出来的，
	// 只有滚动条（log_meter）和语音图标（log_vp）是独立素材。
	cfg.Log = LogScreen{
		HasBackColor: true,
		BackColor:    0xc0000000,
		Lines:        4,
		LeftPos:      196,
		Width:        560,
		Bar:          UIButton{Image: "log_meter", Frames: 1, X: 752, Y: 24},
		VoiceIcon:    "log_vp",
	}
}

func applyBuiltinSchoolHeavenMuseum(cfg *BsxxConfiguration) {
	// 这一代的页签是 museum_menu 里 4 行烘焙文字（CG鑑賞 / シーン鑑賞 /
	// 立ち絵鑑賞 / 戻る），常态 4 行在上、红色高亮 4 行在下（每行 18px）。
	// 引擎的页签接口按"多帧竖排图"取帧，这里用 Clip 把每一行裁出来：
	// Frames=1 + FixedFrame 让它永远画裁出的那一行，行号由 Clip 的 Y 决定。
	tab := func(row int, x int) UIButton {
		return UIButton{
			Image: "museum_menu", Frames: 1, X: x, Y: 9,
			Clip: [4]int{0, row * 18, 116, 18}, FixedFrame: true,
		}
	}
	cfg.Museum = Museum{
		Base: "museum_base",
		Modes: []MuseumMode{
			{Key: "cg", UIButton: tab(0, 40)},
			{Key: "scene", UIButton: tab(1, 176)},
			{Key: "strip", UIButton: tab(2, 312)},
		},
		Return: tab(3, 448),
		CG:     builtinSchoolHeavenCGPage(),
		Scene:  builtinSchoolHeavenScenePage(),
	}
}

// builtinSchoolHeavenThumbPoints 是鉴赏室缩略图的 5x4 网格。
//
// 缩略图素材是 120x90（cg0_thumb 是 120x1800 的 20 帧竖排条，正好
// 20 帧 = 一页 5x4）。起点 (100,74)、列距 120、行距 90，恰好铺满 800x600。
func builtinSchoolHeavenThumbPoints() []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 4; row++ {
		for column := 0; column < 5; column++ {
			points = append(points, UIPoint{X: 100 + column*120, Y: 74 + row*90})
		}
	}
	return points
}

func builtinSchoolHeavenPageButtons() []UIButton {
	// museum_page 是 24x72 的三帧页码图标（常态/悬停/当前）。
	buttons := make([]UIButton, 0, 6)
	for index := 0; index < 6; index++ {
		buttons = append(buttons, UIButton{
			Image: "museum_page", Frames: 3, X: 401 + 43*index, Y: 531,
		})
	}
	return buttons
}

func builtinSchoolHeavenCGPage() MuseumCGPage {
	return MuseumCGPage{
		Base:         "museum_base",
		PageButtons:  builtinSchoolHeavenPageButtons(),
		Thumbs:       builtinSchoolHeavenThumbPoints(),
		ThumbStrips:  []string{"cg0_thumb", "cg1_thumb", "cg2_thumb", "cg3_thumb", "cg4_thumb", "cg5_thumb"},
		ThumbX:       100,
		ThumbY:       74,
		ThumbW:       120,
		ThumbH:       90,
		Percent:      UIButton{Image: "museum_num", Frames: 10, X: 660, Y: 531},
		PercentSpace: -18,
	}
}

func builtinSchoolHeavenScenePage() MuseumScenePage {
	return MuseumScenePage{
		Base:        "museum_base",
		PageButtons: builtinSchoolHeavenPageButtons(),
		Thumbs:      builtinSchoolHeavenThumbPoints(),
		ThumbStrips: []string{"scene0_thumb", "scene1_thumb", "scene2_thumb", "scene3_thumb"},
		ThumbX:      100,
		ThumbY:      74,
		ThumbW:      120,
		ThumbH:      90,
		Percent:     UIButton{Image: "museum_num", Frames: 10, X: 660, Y: 531},
		// 场景页的两个选择模式按钮（即時再生 / 再生リストに追加）用
		// museum_play 系列的小图标；素材缺失时由渲染器跳过。
		PlayNow:     UIButton{Image: "museum_play", Frames: 1, X: 44, Y: 534},
		AddPlaylist: UIButton{Image: "museum_next", Frames: 1, X: 166, Y: 534},
	}
}

func applyBuiltinSchoolHeavenConfig(cfg *BsxxConfiguration) {
	// config_base 是整屏 800x600 的设置底图：分类框、标签、滑杆槽和
	// 「元の設定に戻す / 戻る / タイトルに戻る / ゲーム終了」全都是烘焙图。
	//
	// 这一代没有 [config screen] 段，滑杆坐标从 config_hit 的色块包围盒推导：
	//   - 两行 10 格刻度（y=66 与 y=114，x=496+18*i）：文字速度 / 自动速度；
	//   - 四条横向色块（x=472..645，y=160/184/208/232，宽 174）：BGM / SE / 音声 / BGV。
	// 滑杆底槽由底图烘焙，引擎只画当前位置的标记，不依赖 cfg_meter 的装饰图。
	// 具体哪一行对应哪一项是从色块几何推断的，需要在游戏里目视确认。
	cfg.LegacyConfig = LegacyConfigScreen{
		Base:      "config_base",
		HitTable:  "config_hit",
		CheckMark: "cfg_mark",
		Reset:     UIButton{X: 180, Y: 540},
		Back:      UIButton{X: 35, Y: 540},
		Title:     UIButton{X: 115, Y: 540},
		Exit:      UIButton{X: 309, Y: 540},
		// 文字速度 / 自动速度：两行 10 格刻度，x 每格 18px，与 config_hit 的 17px 色块对齐。
		Speeds: []LegacyGauge{schoolHeavenDotGauge(66), schoolHeavenDotGauge(114)},
		// 顺序固定为 master/BGM/SE/BGS/音声/BGV/减衰率；本作只暴露 BGM/SE/音声/BGV，其余留空占位。
		Volumes: []LegacyGauge{
			{},
			schoolHeavenMarkerGauge(160),
			schoolHeavenMarkerGauge(184),
			{},
			schoolHeavenMarkerGauge(208),
			schoolHeavenMarkerGauge(232),
		},
	}
}

// schoolHeavenDotGauge 生成一行 10 格离散刻度。刻度位置取自 config_hit 的 17px 色块，
// 间距 18px；cfg_mark 在这一代是单帧图，所以 Frames 固定为 1，不能沿用 AFDVD 的 3 帧切法。
func schoolHeavenDotGauge(y int) LegacyGauge {
	dots := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		dots = append(dots, UIPoint{X: 496 + index*18, Y: y})
	}
	return LegacyGauge{Dots: dots, DotMark: UIButton{Image: "cfg_mark", Frames: 1}}
}

// schoolHeavenMarkerGauge 生成一条横向滑杆：底槽由底图烘焙，引擎只画当前位置的竖线标记。
// 轨道 x=472 起、宽 174，与 config_hit 中对应的横向色块一致。
func schoolHeavenMarkerGauge(y int) LegacyGauge {
	return LegacyGauge{
		MarkerOnly:  true,
		Knob:        UIButton{X: 472, Y: y},
		TrackWidth:  174,
		TrackHeight: 22,
	}
}
