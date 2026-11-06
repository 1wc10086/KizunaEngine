package config

// builtin_bishop_afdvd.go 给 Bishop 2008 DVD 版（AFDVD，BSScript Rev.6，800x600）补内置布局。
//
// 该版发行目录没有 bsxx.cfg，标题、对话框、工具条和存读档页的坐标编译在原引擎里。
// 坐标取自 Graphics.bsa 各素材的命中块（title_hit、message_hit、gamemenu_hit、dm_hit），
// 并对照手册截图；文字区与字号为截图估计值，未逐像素核对。
//
// 不能复用 KD 的内置布局（builtin_bishop_legacy.go）：两者同为 Rev 脚本、同为 800x600，
// 但标题 BGM、对话框位置、工具条和存档页的素材与坐标都不同。KD 依赖的
// kaisou/dm_save/dm_load 在 AFDVD 中不存在，签名判断就靠这一点区分。

var builtinAFDVDAssets = []string{
	"title_base", "title_hit",
	"title_menu_op", "title_menu_begin", "title_menu_load",
	"title_menu_museum", "title_menu_setting", "title_menu_quit",
	"message", "message_hit",
	"gamemenu", "gamemenu_hit",
	"dm_base", "dm_hit",
	"config_base", "config_hit",
	"btn_skip", "btn_auto", "btn_voice", "btn_dress", "btn_fire",
	"btn_qsave", "btn_qload", "btn_save", "btn_load", "btn_config", "btn_log",
	"item0",
}

// builtinAFDVDExcluded 是 KD / Ancient 签名独有的素材，AFDVD 发行版里不存在。
var builtinAFDVDExcluded = []string{"kaisou", "dm_save", "dm_load", "msg_base"}

func isBuiltinAFDVD(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinAFDVDAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinAFDVDExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// applyBuiltinAFDVD 填入 AFDVD 的标题、对话框、工具条、选择肢、存档页和回想。
// 设置画面（LegacyConfig）和鉴赏室暂不填：素材映射未确认，留空时由宿主设置页和通用目录兜底。
func applyBuiltinAFDVD(cfg *BsxxConfiguration) {
	cfg.Width = 800
	cfg.Height = 600
	// 160x120 的存档缩略图对应 800x600 的 0.2，与 KD 保持一致。
	cfg.ThumbnailRatio = 0.2
	applyBuiltinAFDVDTitle(cfg)
	applyBuiltinAFDVDMessage(cfg)
	applyBuiltinAFDVDGameMenu(cfg)
	applyBuiltinAFDVDSelect(cfg)
	applyBuiltinAFDVDDataScreen(cfg)
	applyBuiltinAFDVDLog(cfg)
	applyBuiltinAFDVDMuseum(cfg)
	applyBuiltinAFDVDConfig(cfg)
}

func applyBuiltinAFDVDTitle(cfg *BsxxConfiguration) {
	// title_menu_* 是 2 帧按钮（常态帧 0、悬停帧 1）；Frame 为 -1 表示沿用引擎默认切帧。
	// 纵向位置来自 title_hit：六个命中块是 y=228..271、278..321、…、478..521，
	// 间距 50。旧值整体多加了 50（278 起），标题文字因此比原作低一行，
	// 最后一个"終了"几乎贴到版权行；按钮图高 46，命中块高 44，取命中块左上角即可。
	entries := []TitleMenuEntry{
		{Key: "op", Image: "title_menu_op", Frames: 2, X: 64, Y: 228, Frame: -1},
		{Key: "start", Image: "title_menu_begin", Frames: 2, X: 64, Y: 278, Frame: -1},
		{Key: "load", Image: "title_menu_load", Frames: 2, X: 64, Y: 328, Frame: -1},
		{Key: "museum", Image: "title_menu_museum", Frames: 2, X: 64, Y: 378, Frame: -1},
		{Key: "config", Image: "title_menu_setting", Frames: 2, X: 64, Y: 428, Frame: -1},
		{Key: "quit", Image: "title_menu_quit", Frames: 2, X: 64, Y: 478, Frame: -1},
	}
	// 标题 BGM 与 Immoral 版的配置一致（bgm02），不能沿用 KD 的 bgm24。
	cfg.Title = TitlePhase{BgmName: "bgm02", Base: "title_base", HitTable: "title_hit", Entries: entries}
}

func applyBuiltinAFDVDMessage(cfg *BsxxConfiguration) {
	// message 为 800x152 底图，贴在屏幕底部 (0,448)；底图自带框，BgColor 为 0 不叠半透明底色（同 Ancient）。
	// message.bsg 是 800x152 的**空心边框**：可见框体在 x=35..619、y=0..123，
	// 内部完全透明（原引擎让场景透出来）。名字/正文必须画在框内，
	// 旧值 x=16 落在左边框外侧，玩家看到文字贴着屏幕左边、框里反而空的；
	// 这里按框内边距取 x=48，正文从 y=32 起排 3 行（行高 26 = 32..110）。
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 448, Speed: 100, BgColor: 0,
		NameX: 48, NameY: 6, NameW: 540, NameH: 20, NameColor: 0xffff8822,
		TextX: 48, TextY: 26, TextW: 540, TextH: 84,
		Unagashi: "unagashi", UnagashiX: 580, UnagashiY: 88,
		UnagashiCellW: 32, UnagashiCellH: 32, UnagashiColumns: 1, UnagashiFrames: 4, UnagashiSpeed: 400,
	}
	// 16 号字、行高 26、3 行；正文区宽 540，约 33 个全角字。
	cfg.Font = FontSettings{Size: 16, Width: 16, Height: 26, Weight: 1000, Edge: 1, Shadow: 1, LineLength: 33, Lines: 3}
}

func applyBuiltinAFDVDGameMenu(cfg *BsxxConfiguration) {
	// 对话框上的 SKIP/AUTO/VOICE/DRESS/FIRE 与底部系统条共用一个 GameMenu，按钮坐标均为屏幕绝对坐标。
	// fire 在引擎内没有对应 action，只显示按钮；不能映射成 ADVANCE，否则误触会推进剧情。
	cfg.GameMenu = GameMenu{
		Base: "gamemenu", BaseX: 0, BaseY: 572,
		ShowArea: [4]int{0, 448, 800, 152},
		Buttons: []MenuButton{
			{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 628, Y: 452}},
			{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 700, Y: 452}},
			{Key: "voice", UIButton: UIButton{Image: "btn_voice", Frames: 4, X: 628, Y: 488}},
			{Key: "dress", UIButton: UIButton{Image: "btn_dress", Frames: 4, X: 700, Y: 488}},
			{Key: "fire", UIButton: UIButton{Image: "btn_fire", Frames: 4, X: 628, Y: 524}},
			{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 90, Y: 576}},
			{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 194, Y: 576}},
			{Key: "save", UIButton: UIButton{Image: "btn_save", Frames: 4, X: 298, Y: 576}},
			{Key: "load", UIButton: UIButton{Image: "btn_load", Frames: 4, X: 402, Y: 576}},
			{Key: "config", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 506, Y: 576}},
			{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 610, Y: 576}},
		},
	}
}

func applyBuiltinAFDVDSelect(cfg *BsxxConfiguration) {
	// item0 为 376x24 的 2 帧选项条，文字 X 取条宽一半（188）并居中。
	cfg.SelectWindow = NewSelectWindowConfiguration(288, 1, []SelectDesign{
		{
			Item: "item0", TextX: 188, TextY: 3, Alignment: SelectDesignAlignCenter,
			DefaultColor: 0xffffffff, AlreadyColor: 0xff888888, SelectColor: 0xffff4488,
		},
	})
}

func applyBuiltinAFDVDDataScreen(cfg *BsxxConfiguration) {
	// dm_base 是 800x600 整屏底图，存读共用；槽位坐标取自 dm_hit 命中块，4 列 x 3 行，每页 12 槽。
	screen := DataScreen{
		SaveBase: "dm_base", LoadBase: "dm_base", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 12,
		// cfg_mode_* 是 100x60 的三帧图（每帧 100x20）。它们对应 dm_hit
		// 底部中间的 99x20 命中块，不在顶部；顶部右侧的 90x40 命中块才是
		// 戻る/タイトル/終了，素材未确认，暂不配。
		SaveMode: UIButton{Image: "cfg_mode_save", Frames: 3, X: 298, Y: 576},
		LoadMode: UIButton{Image: "cfg_mode_load", Frames: 3, X: 402, Y: 576},
		Config:   UIButton{Image: "cfg_mode_config", Frames: 3, X: 506, Y: 576},
		// dm_pagebase 是 384x32 的绿色页码条，1..10 已经烘焙在图里；
		// PageHits 只负责点击，不重复画数字。
		PageSelectFrame: UIButton{Image: "dm_pagebase", Frames: 1, X: 232, Y: 524},
		// cfg_button 是 90x240 的六帧图：前三帧是 戻る/タイトル/終了 的常态，
		// 后三帧是同三项的悬停态。触屏不会触发悬停，固定 Start 即可；
		// 桌面端悬停帧号会落在下一项，但不影响点击。
		Back:  UIButton{Image: "cfg_button", Frames: 6, X: 511, Y: 4},
		Title: UIButton{Image: "cfg_button", Frames: 6, X: 605, Y: 4, Start: 1},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, X: 699, Y: 4, Start: 2},
		// dm_base 本身只是一块蓝色底板，没有槽位框；dm_slotbase 是
		// 163x124 的白色渐变框。走 SlotFrame 路径同时得到框和缩略图。
		SlotFrame:       "dm_slotbase",
		SlotFrameFrames: 1,
		SlotSize:        [2]int{160, 120},
		SlotThumb:       [4]int{2, 2, 160, 120},
		SlotTextX:       4,
		SlotTextY:       4,
		SlotTextLineGap: 16,
		SlotTextColor:   0xffffffff,
		AutoSlotIndex:   -1,
	}
	columns := [4]int{32, 224, 416, 608}
	rows := [3]int{100, 236, 372}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	for index := 0; index < screen.PageMax; index++ {
		screen.PageHits = append(screen.PageHits, UIPoint{X: 244 + 32*index, Y: 532})
	}
	cfg.DataScreen = screen
}

func applyBuiltinAFDVDLog(cfg *BsxxConfiguration) {
	// 没有 kaisou 底图：Base 留空，用半透明黑底整屏铺底；log_meter 为右侧滚动条，log_vp 为语音图标。
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000, Lines: 6, LeftPos: 16, Width: 760,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 752, Y: 24},
		VoiceIcon: "log_vp",
	}
}
