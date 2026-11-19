package config

// builtin_bishop_gk2.go 给 2003 年的 学園2～淫虐の図式～（BSScript Rev.2）补内置布局。
//
// 这一代把标题菜单、对话框、顶部工具条和存读档页的坐标编译在原引擎 exe 里，
// 发行目录只有 bss.ini，没有 bsxx.cfg。配置为空时引擎会直接落进剧情、
// 对话框退回宿主 Flutter 实现，玩家看到的是一套和原作完全不同的界面。
//
// 坐标来源（都可对照游戏素材本身）：
//   - 标题：title_hit 的 8 个命中块 + title_menu 的 8 帧按钮图；
//   - 对话框：message/message_hit 的边框、message_hit 的按钮块，
//     以及 btn_skip/auto/voice/hide/up/down 图头自带的屏幕坐标；
//   - 顶部工具条：gamemenu/gamemenu_hit 与 btn_* 图头坐标；
//   - 存读档：dm_hit 的 9 个 160x120 槽位与 10 个页码块，dm_page 的三态页码；
//   - 日期小窗：date_info（时段图）、map_date/mw_date（地图日期数字）。
//
// 这些素材同时满足"标题单图 8 帧、message 140 高、dm_save/dm_load 并存、
// 有 map_hit/map_base"时才判定为这一代，不会误伤 AFDVD（title_menu_* 分图）、
// KD（kaisou/dm_base）或现代作品（有 bsxx.cfg 时根本不会走到这里）。

var builtinGK2Assets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_hit", "gamemenu", "gamemenu_hit",
	"dm_save", "dm_load", "dm_hit", "dm_page",
	"date_info", "map_base", "map_hit",
	"item", "btn_skip", "btn_auto", "btn_voice",
	"btn_hide", "btn_up", "btn_down",
}

// builtinGK2Excluded 是其它世代独有的素材。学園2 的标题是单张 8 帧图，
// 没有 title_menu_op 这类分图；也没有 KD 的 kaisou / dm_base。
var builtinGK2Excluded = []string{
	"title_menu_op", "title_menu_begin", "kaisou", "dm_base", "msg_base", "item0",
}

func isBuiltinGK2(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinGK2Assets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinGK2Excluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

func applyBuiltinGK2(cfg *BsxxConfiguration) {
	cfg.Width = 800
	cfg.Height = 600
	// 存档缩略图 160x120 正好是 800x600 的 0.2，和 dm_hit 的槽位尺寸一致。
	cfg.ThumbnailRatio = 0.2
	applyBuiltinGK2Title(cfg)
	applyBuiltinGK2Message(cfg)
	applyBuiltinGK2GameMenu(cfg)
	applyBuiltinGK2Select(cfg)
	applyBuiltinGK2DataScreen(cfg)
	applyBuiltinGK2Log(cfg)
	// 鉴赏室与设置画面同样是原引擎内建的：发行目录没有 bsxx.cfg 可读，
	// 只能按素材签名给出固定布局。两者都只填空缺项，不影响其它作品。
	applyBuiltinGK2Museum(cfg)
	applyBuiltinGK2Config(cfg)
}

func applyBuiltinGK2Title(cfg *BsxxConfiguration) {
	// title_menu 是 184x320 的 8 帧竖排图（每帧 184x40），
	// title_hit 的命中块给出每帧的屏幕位置：
	//   0=始めから(266) 1=続きから(306) 2=設定(506) 3=音楽鑑賞(366)
	//   4=CG鑑賞(406) 5=シーン鑑賞(446) 6=終了(546) 7=オープニング(226)
	// 帧号与命中块的掩码值一一对应，所以 Frame 必须显式给出。
	entries := []TitleMenuEntry{
		{Key: "start", Image: "title_menu", Frames: 8, Frame: 0, X: 571, Y: 266},
		{Key: "load", Image: "title_menu", Frames: 8, Frame: 1, X: 571, Y: 306},
		{Key: "config", Image: "title_menu", Frames: 8, Frame: 2, X: 571, Y: 506},
		{Key: "museum_music", Image: "title_menu", Frames: 8, Frame: 3, X: 571, Y: 366},
		{Key: "museum_cg", Image: "title_menu", Frames: 8, Frame: 4, X: 571, Y: 406},
		{Key: "museum_scene", Image: "title_menu", Frames: 8, Frame: 5, X: 571, Y: 446},
		{Key: "quit", Image: "title_menu", Frames: 8, Frame: 6, X: 571, Y: 546},
		{Key: "op", Image: "title_menu", Frames: 8, Frame: 7, X: 571, Y: 226},
	}
	// 标题 BGM 与重制版（GK2IEDL）一致，都是 bgm01；脚本的标题 system call
	// 不会自己发 BGM，原引擎是在进入标题时由引擎起播的。
	cfg.Title = TitlePhase{BgmName: "bgm01", Base: "title_base", HitTable: "title_hit", Entries: entries}
}

func applyBuiltinGK2Message(cfg *BsxxConfiguration) {
	// message 是 800x140 的整窗底图，贴在屏幕底部 (0,460)：顶部 24px 是
	// gamemenu 工具条，所以对话框占 460..600。左上是日期小窗，右侧正文。
	// 底图自己带半透明（内部 alpha=128 的黑、边框约 180 的绿），
	// 所以 bg color 必须为 0，直接按像素 alpha 画；再乘一层半透明黑
	// 会把窗户压成几乎全黑，和原作截图不符。
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 460,
		Speed: 100, BgColor: 0,
		// 名字画在正文框顶部，原作显示为【香織】。
		NameX: 190, NameY: 6, NameW: 530, NameH: 22, NameColor: 0xffffffff,
		TextX: 190, TextY: 30, TextW: 535, TextH: 104,
		SpeakerBrackets: true,
		Unagashi:        "unagashi", UnagashiX: 720, UnagashiY: 95,
		UnagashiCellW: 30, UnagashiCellH: 30, UnagashiColumns: 1,
		UnagashiFrames: 4, UnagashiSpeed: 200,
		SubWindow: MessageSubWindow{
			Enabled: true,
			// 小窗内框 44..170（宽 126），日期文字在其中居中。
			X: 44, Y: 2, Width: 126,
			DayVariable: "#day", TimeVariable: "#time",
			DayText: true, DayY: 4, DayFontSize: 15, DayColor: 0xffffffff,
			// date_info 的 0..11 帧是时段：朝/午前/昼休み/午後/放課後-残4..1/
			// 下校時間/夜/自室/無線部室。剧本 #time 从 2（朝）开始计数，
			// 所以帧号要减 2；#time=6 正好是「放課後-残4」。
			TimeImage: "date_info", TimeFrames: 48, TimeX: 11, TimeY: 30,
			TimeZero: true, TimeOffset: -2,
		},
	}
	// 16 号字、行高 24；正文宽 535，约 29 个全角字，最多 4 行。
	cfg.Font = FontSettings{Size: 16, Width: 16, Height: 24, Weight: 1000, Edge: 1, Shadow: 1, LineLength: 29, Lines: 4}
}

func applyBuiltinGK2GameMenu(cfg *BsxxConfiguration) {
	// 顶部工具条：gamemenu 是 800x24 的底图，8 个按钮的图头坐标正好是
	// gamemenu_hit 的命中块（39/130/221/…/676, y=0）。
	// 按钮图是 4 帧竖排（常态/悬停/按下/不可用）。
	cfg.GameMenu = GameMenu{
		Base: "gamemenu", BaseX: 0, BaseY: 0,
		ShowArea:   [4]int{0, 0, 800, 600},
		Persistent: true,
		Buttons: []MenuButton{
			{Key: "quick save", Persistent: true, UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 39, Y: 0}},
			{Key: "quick load", Persistent: true, UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 130, Y: 0}},
			{Key: "save", Persistent: true, UIButton: UIButton{Image: "btn_save", Frames: 4, X: 221, Y: 0}},
			{Key: "load", Persistent: true, UIButton: UIButton{Image: "btn_load", Frames: 4, X: 312, Y: 0}},
			{Key: "config", Persistent: true, UIButton: UIButton{Image: "btn_config", Frames: 4, X: 403, Y: 0}},
			{Key: "log", Persistent: true, UIButton: UIButton{Image: "btn_log", Frames: 4, X: 494, Y: 0}},
			{Key: "dress", Persistent: true, UIButton: UIButton{Image: "btn_change", Frames: 4, X: 585, Y: 0}},
			{Key: "quit", Persistent: true, UIButton: UIButton{Image: "btn_quit", Frames: 4, X: 676, Y: 0}},
			// 对话框内左下角：SKIP/AUTO/V.PLAY，图头坐标 (43/88/132,106)。
			{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 43, Y: 566}},
			{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 88, Y: 566}},
			{Key: "voice", UIButton: UIButton{Image: "btn_voice", Frames: 4, X: 132, Y: 566}},
			// 右上角三个圆钮：关窗 / 上移 / 下移 (737, 3/29/55)。
			// 上移/下移还没有接入三档窗口位置，先保留按钮并给出反馈日志。
			{Key: "window hide", UIButton: UIButton{Image: "btn_hide", Frames: 4, X: 737, Y: 463}},
			{Key: "window up", UIButton: UIButton{Image: "btn_up", Frames: 4, X: 737, Y: 489}},
			{Key: "window down", UIButton: UIButton{Image: "btn_down", Frames: 4, X: 737, Y: 515}},
		},
	}
}

func applyBuiltinGK2Select(cfg *BsxxConfiguration) {
	// item 是 470x60 的 2 帧选项条（常态/悬停），居中后左边界 165。
	// 文字画在条的中线上，颜色与绿/红 UI 保持一致。
	cfg.SelectWindow = NewSelectWindowConfiguration(300, 1, []SelectDesign{
		{
			Item:         "item",
			Interval:     8,
			TextX:        235,
			TextY:        4,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0xff909090,
			SelectColor:  0xffff5555,
		},
	})
}

func applyBuiltinGK2DataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase: "dm_save", LoadBase: "dm_load", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 9,
		// 3x3 槽位，坐标取自 dm_hit 的 160x120 命中块。
		// 底部三个文字按钮：<< BACK / TITLE / QUIT。
		Back:  UIButton{Image: "cfg_back", Frames: 2, X: 30, Y: 558},
		Title: UIButton{Image: "cfg_return", Frames: 2, X: 400, Y: 558},
		Exit:  UIButton{Image: "cfg_quit", Frames: 2, X: 630, Y: 558},
		// dm_page 把 10 个页码的常态/当前/悬停三种状态各占 11 帧、共 33 帧
		// 竖排在一张图里；页码本身没有独立底图，引擎按页下标直接取帧。
		PageNumbers: UIButton{Image: "dm_page", Frames: 33, FixedFrame: true},
		// dm_slot 是 170x130 的三态槽位框：0 常态（绿）、1 悬停（粉）、
		// 2 空格（灰）。dm_nodata/dm_auto 是格子里的文字占位。
		SlotFrame:       "dm_slot",
		SlotFrameFrames: 3,
		SlotFrameHover:  1,
		SlotFrameSelected: 1,
		AutoSlotIndex:   0,
		AutoOnly:        "dm_auto",
		NoData:          "dm_nodata",
		// 缩略图按 160x120 生成：dm_slot 的槽位是 160x120，右栏预览区
		// （dm_save 上 620,83 起的空框）也是 160x120，尺寸必须一致，
		// 否则格子里的图和右栏大图都要缩放。
		SlotThumb: [4]int{0, 0, 160, 120},
		Confirm: SlotViewer{
			// 攻略度条：dm_save 底图里已经画好六名角色的姓名和空条框
			// （629..770 宽 142、高 18，行距 40），所以这里不重复写名字，
			// 只把 dm_meter 的 138x14 条画进框内：frame0 轨道、frame1 按
			// pts/100 截取填充。
			MeterImage: "dm_meter", MeterFrames: 2, MeterRect: [4]int{630, 299, 138, 14}, MeterGap: 40,
			MeterProperties: [6]string{"#hrn_pts", "#mik_pts", "#kwr_pts", "#izm_pts", "#ayn_pts", "#kyk_pts"},
			MeterMax: 100, MeterTrackFrame: 0, MeterFillFrame: 1,
		},
	}
	columns := [3]int{30, 220, 410}
	rows := [3]int{83, 239, 395}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// 顶部 10 个页签：命中块是 41~51px 宽的格子，28x28 的命中框放在
	// 每格中心，既不会压到相邻页，也能点得到。
	centers := [10]int{25, 71, 112, 154, 195, 237, 278, 319, 360, 402}
	for _, center := range centers {
		screen.PageHits = append(screen.PageHits, UIPoint{X: center - 14, Y: 18})
		screen.PageNumberPos = append(screen.PageNumberPos, UIPoint{X: center - 12, Y: 21})
	}
	cfg.DataScreen = screen
}

func applyBuiltinGK2Log(cfg *BsxxConfiguration) {
	// 回想没有专属底图：用当前剧情帧 + 半透明黑，右侧 log_meter 滚动条，
	// 有语音的行左侧画 log_vp 图标。
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 7, LeftPos: 16, Width: 740,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 745, Y: 24},
		VoiceIcon: "log_vp",
	}
}
