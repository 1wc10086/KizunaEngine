package config

// builtin_bishop_mddvd.go 给『満淫電車 DVD特別版』（MDDVD，BSScript Ancient）补内置布局。
//
// 这一代发行目录里没有 bsxx.cfg，也没有 define.bsa：标题、对话框、工具条、
// 存读档、设置、鉴赏室全部由原引擎按自己的对象表绘制。素材本身已经把坐标
// 烘进去了（title_hit / config_hit / dm_hit / cgN_hit 的彩色命中块），
// 所以下面的坐标全部来自这些命中块和 BSG 图头，而不是估算。
//
// 只在这个作品独有的素材签名同时存在时生效：MDDVD 有 title_higo、higo_base、
// cg0_base、sc0_base、music_base、dm_slot、next_select、aw_mobile；同代的
// 特別病棟（Tbxdl）没有这些图，KD/AFDVD 也没有。配置里已有标题/对话框的
// 作品一律不动。

var builtinMddvdAssets = []string{
	"title_base", "title_menu", "title_hit", "title_higo",
	"message", "message_hit", "unagashi", "item",
	"config_base", "config_hit",
	"dm_save", "dm_load", "dm_slot", "dm_number", "dm_page",
	"cg0_base", "sc0_base", "music_base",
	"log_meter", "log_vp",
	"aw_mobile", "next_select", "higo_base",
}

// ApplyBuiltinMddvdDefaults 填入 MDDVD 的全套界面。只在 Ancient 脚本、
// 配置为空且素材签名吻合时返回 true。
func ApplyBuiltinMddvdDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || probe.Exists == nil || !probe.AncientScript {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	for _, name := range builtinMddvdAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	cfg.Width = 800
	cfg.Height = 600
	cfg.Mddvd = true
	// 存档缩略图按 1/15 缩放：槽位行只有 44px 高，0.2（160x120）会撑出
	// 槽位框。64x40 正好放在 dm_slot 内，和原作的小截图比例一致。
	cfg.ThumbnailRatio = 1.0 / 15.0
	applyBuiltinMddvdTitle(cfg)
	applyBuiltinMddvdMessage(cfg)
	applyBuiltinMddvdToolbar(cfg)
	applyBuiltinMddvdSelect(cfg)
	applyBuiltinMddvdDataScreen(cfg)
	applyBuiltinMddvdConfig(cfg)
	applyBuiltinMddvdMuseum(cfg)
	applyBuiltinMddvdLog(cfg)
	return true
}

func applyBuiltinMddvdTitle(cfg *BsxxConfiguration) {
	// title_base 是 800x600 的整屏底图，菜单文字已经烤在右侧（y=256..496）。
	// title_menu 是 272x256 的 8 帧高亮条：未悬停时不画，只画 title_base
	// 自带的白字；悬停时叠对应帧，文字会变亮。special 的「卑語モード」
	// 不在底图上，单独用 title_higo（272x32）常驻显示在 y=216。
	entries := []TitleMenuEntry{
		{Key: "special", Image: "title_higo", Frames: 1, X: 496, Y: 216, Frame: 0},
		{Key: "start", Image: "title_menu", Frames: 8, X: 496, Y: 256, Frame: 0, HoverOnly: true},
		{Key: "load", Image: "title_menu", Frames: 8, X: 496, Y: 296, Frame: 1, HoverOnly: true},
		{Key: "config", Image: "title_menu", Frames: 8, X: 496, Y: 336, Frame: 2, HoverOnly: true},
		{Key: "museum_music", Image: "title_menu", Frames: 8, X: 496, Y: 376, Frame: 3, HoverOnly: true},
		{Key: "museum_cg", Image: "title_menu", Frames: 8, X: 496, Y: 416, Frame: 4, HoverOnly: true},
		{Key: "museum_scene", Image: "title_menu", Frames: 8, X: 496, Y: 456, Frame: 5, HoverOnly: true},
		{Key: "quit", Image: "title_menu", Frames: 8, X: 496, Y: 496, Frame: 6, HoverOnly: true},
	}
	cfg.Title = TitlePhase{
		Base:           "title_base",
		HitTable:       "title_hit",
		BgmName:        "bgm21",
		UseSpecialMode: true,
		Entries:        entries,
	}
}

func applyBuiltinMddvdMessage(cfg *BsxxConfiguration) {
	// message 是 800x150 的透明边框，原引擎贴在屏幕底部 (0,450)。
	// 左侧是 120x120 的名牌框，竖线之后才是正文；正文从 x=165 起，
	// 不能沿用通用兜底的 3% 边距（24px），否则文字会压到名牌和边框上。
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 450,
		Speed: 100, BgColor: 0,
		NameX: 30, NameY: 16, NameW: 105, NameH: 24, NameColor: 0xffffffff,
		TextX: 165, TextY: 28, TextW: 610, TextH: 104,
		TextColor: 0xffffffff, TextColorSet: true,
		Unagashi: "unagashi", UnagashiX: 735, UnagashiY: 550,
		UnagashiCellW: 40, UnagashiCellH: 40, UnagashiColumns: 1,
		UnagashiFrames: 4, UnagashiSpeed: 400,
	}
	// 20 号字、26px 行高，正文区 610x104 正好 4 行、每行约 30 个全角字。
	cfg.Font = FontSettings{
		Size: 20, Width: 20, Height: 26, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 30, Lines: 4,
	}
}

func applyBuiltinMddvdToolbar(cfg *BsxxConfiguration) {
	// 9 个工具按钮各自是 65x80 的 4 帧图（每帧 65x20），图头已经写了
	// 屏幕坐标 (190,450)…(710,450)；message_hit 的命中块也从 x=180 起。
	// 底图本身不另画工具条，Overlay 表示按钮直接叠在 message 的上沿。
	cfg.GameMenu = GameMenu{
		ShowArea: [4]int{0, 450, 800, 150},
		Overlay:  true,
	}
	entries := [9]struct {
		key   string
		image string
		x     int
	}{
		{"quick save", "btn_qsave", 190},
		{"quick load", "btn_qload", 255},
		{"save", "btn_save", 320},
		{"load", "btn_load", 385},
		{"skip", "btn_skip", 450},
		{"auto", "btn_auto", 515},
		{"voice", "btn_voice", 580},
		{"log", "btn_log", 645},
		{"config", "btn_config", 710},
	}
	for _, entry := range entries {
		cfg.GameMenu.Buttons = append(cfg.GameMenu.Buttons, MenuButton{
			Key: entry.key,
			UIButton: UIButton{
				Image: entry.image, Frames: 4, X: entry.x, Y: 450,
			},
		})
	}
}

func applyBuiltinMddvdSelect(cfg *BsxxConfiguration) {
	// item 是 470x30 的单帧选项条，文字在条内居中。
	cfg.SelectWindow = NewSelectWindowConfiguration(288, 1, []SelectDesign{
		{
			Item: "item", Frames: 1, TextX: 235, TextY: 4,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff, AlreadyColor: 0xff888888, SelectColor: 0xffff6688,
		},
	})
}

func applyBuiltinMddvdDataScreen(cfg *BsxxConfiguration) {
	// dm_save/dm_load 是 800x600 的整屏底图；dm_slot 是 608x44 的双帧槽位框
	// （常态/悬停），dm_hit 的 9 个大命中块是槽位、10 个小块是左侧页码。
	// 注意必须打开 SlotFrameGeneric：dm_slot 这个名字在学園2 有另一套
	// 170x130 的旧几何，不走通用槽位分支。
	screen := DataScreen{
		SaveBase: "dm_save", LoadBase: "dm_load",
		SlotFrame: "dm_slot", SlotFrameFrames: 2, SlotFrameGeneric: true,
		SlotSize: [2]int{608, 44},
		// 缩略图放在行内左侧，正文从 x=84 起；行高 44，文字三行（号码/
		// 时间/摘要）用 14px 行距，正好落在槽位框内。
		SlotThumb:       [4]int{10, 4, 64, 36},
		SlotTextX:       84,
		SlotTextY:       3,
		SlotTextLineGap: 13,
		SlotTextColor:   0xff202020,
		NoBegin:         1,
		PageMax:         10,
		SlotsPerPage:    9,
		SlotEmptyFrame:  -1,
		SlotFrameHover:  1,
		Back:            mddvdNavButton("cfg_back", 30, 547, 0),
		Exit:            mddvdNavButton("cfg_quit", 284, 547, 0),
		Title:           mddvdNavButton("cfg_return", 534, 547, 0),
	}
	// 9 个槽位：dm_hit 的大块从 (179,120) 起，行距 46。
	for row := 0; row < 9; row++ {
		screen.Slots = append(screen.Slots, UIPoint{X: 176, Y: 116 + row*46})
	}
	// 左侧页码是 10 个六边形按钮，按 (83,104)、(23,144)… 交错排列；
	// 数字由 dm_number（10 帧 40x40）补画，页码从 1 开始。
	pages := [10]UIPoint{
		{X: 83, Y: 104}, {X: 23, Y: 144}, {X: 83, Y: 184}, {X: 23, Y: 224},
		{X: 83, Y: 264}, {X: 23, Y: 304}, {X: 83, Y: 344}, {X: 23, Y: 384},
		{X: 83, Y: 424}, {X: 23, Y: 464},
	}
	for _, point := range pages {
		screen.PageButtons = append(screen.PageButtons, UIButton{
			Image: "dm_page", Frames: 2, X: point.X, Y: point.Y,
			Hit: [4]int{point.X, point.Y, 60, 40},
		})
		screen.PageNumberPos = append(screen.PageNumberPos, UIPoint{X: point.X + 21, Y: point.Y})
	}
	screen.PageNumbers = UIButton{Image: "dm_number", Frames: 10}
	screen.PageNumberStart = 1
	cfg.DataScreen = screen
}

func applyBuiltinMddvdConfig(cfg *BsxxConfiguration) {
	// config_base 的标签和刻度全部烤在底图上，config_hit 提供可点块。
	// 值本身是 cfg_* 的多帧图：0 常态、1 悬停、2 选中。
	// 画面表示/未読/スキップ速度的 ON/OFF 两张图共用同一个显示位置，
	// 用 Exclusive 只画当前值；画面表示只有一颗按钮，点击循环切换。
	cfg.LegacyConfig = LegacyConfigScreen{
		Base: "config_base", HitTable: "config_hit",
		ToggleHitWidth: 160,
		ValueGroups: []LegacyValueGroup{
			{Key: "screen mode", Exclusive: true, ExclusiveToggle: true, Options: []LegacyValueOption{
				{Button: UIButton{Image: "cfg_full", Frames: 3, X: 144, Y: 132}, Value: 1},
				{Button: UIButton{Image: "cfg_window", Frames: 3, X: 144, Y: 132}, Value: 0},
			}},
			{Key: "screen effect", Exclusive: true, Options: []LegacyValueOption{
				{Button: UIButton{Image: "cfg_effect_normal", Frames: 3, X: 35, Y: 227}, Value: 1},
				{Button: UIButton{Image: "cfg_effect_frameskip", Frames: 3, X: 144, Y: 227}, Value: 2},
				{Button: UIButton{Image: "cfg_effect_off", Frames: 3, X: 35, Y: 258}, Value: 0},
			}},
			{Key: "skip unread", Exclusive: true, Options: []LegacyValueOption{
				{Button: UIButton{Image: "cfg_noread_on", Frames: 3, X: 287, Y: 132}, Value: 1},
				{Button: UIButton{Image: "cfg_noread_off", Frames: 3, X: 397, Y: 132}, Value: 0},
			}},
			{Key: "skip speed", Exclusive: true, Options: []LegacyValueOption{
				{Button: UIButton{Image: "cfg_skip_normal", Frames: 3, X: 287, Y: 227}, Value: 0},
				{Button: UIButton{Image: "cfg_skip_fast", Frames: 3, X: 397, Y: 227}, Value: 1},
				{Button: UIButton{Image: "cfg_skip_next", Frames: 2, X: 287, Y: 258}, Value: 2},
			}},
			{Key: "voice loop", Exclusive: true, Options: []LegacyValueOption{
				{Button: UIButton{Image: "cfg_loop_on", Frames: 3, X: 539, Y: 456}, Value: 1},
				{Button: UIButton{Image: "cfg_loop_off", Frames: 3, X: 652, Y: 456}, Value: 0},
			}},
		},
		// 消息/自动速度是底图上的 10 个菱形刻度；config_knobv 竖条作为
		// 当前位置标记，点哪个点就设成对应档位。
		Speeds: []LegacyGauge{
			mddvdDotGauge(68),
			mddvdDotGauge(315),
		},
		Volumes: []LegacyGauge{
			{},
			mddvdVerticalGauge(563),
			mddvdVerticalGauge(643),
			{},
			mddvdVerticalGauge(723),
		},
		Characters: []LegacyCharacter{
			{Button: UIButton{Image: "cfg_haruka", Frames: 3, X: 35, Y: 455}},
			{Button: UIButton{Image: "cfg_yuzu", Frames: 3, X: 104, Y: 455}},
			{Button: UIButton{Image: "cfg_sayaka", Frames: 3, X: 172, Y: 455}},
			{Button: UIButton{Image: "cfg_masumi", Frames: 3, X: 241, Y: 455}},
			{Button: UIButton{Image: "cfg_akira", Frames: 3, X: 310, Y: 455}},
			{Button: UIButton{Image: "cfg_chihiro", Frames: 3, X: 379, Y: 455}},
			{Button: UIButton{Image: "cfg_etc", Frames: 3, X: 447, Y: 455}},
		},
		CharacterOffFrame:      2,
		CharacterSelectedFrame: 1,
		Back:                   mddvdNavButton("cfg_back", 30, 547, 0),
		Exit:                   mddvdNavButton("cfg_quit", 284, 547, 0),
		Title:                  mddvdNavButton("cfg_return", 534, 547, 0),
	}
}

// mddvdDotGauge 生成一行 10 个离散速度刻度。x0 是第一个点的中心，
// 每个点间隔 15px；底图已经画出菱形，引擎只补当前标记。
func mddvdDotGauge(x0 int) LegacyGauge {
	dots := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		dots = append(dots, UIPoint{X: x0 + index*15, Y: 365})
	}
	return LegacyGauge{
		Dots:    dots,
		DotMark: UIButton{Image: "config_knobv", Frames: 1},
	}
}

// mddvdVerticalGauge 生成一条竖向音量滑杆：底图轨道从 y=118 到 y=328，
// config_knobv 是 10x30 的滑块。命中区会按 max(24, 滑块宽) 放宽。
func mddvdVerticalGauge(x int) LegacyGauge {
	return LegacyGauge{
		Vertical:    true,
		Knob:        UIButton{Image: "config_knobv", Frames: 1, X: x, Y: 118},
		TrackHeight: 210,
	}
}

// mddvdNavButton 生成底部的 戻る/終了/タイトル 三颗导航按钮。cfg_* 是
// 227x141 的三帧图（每帧 47 高），Start 固定画自己那一帧。
func mddvdNavButton(image string, x, y, start int) UIButton {
	return UIButton{Image: image, Frames: 3, X: x, Y: y, Start: start, FixedFrame: true}
}

func applyBuiltinMddvdMuseum(cfg *BsxxConfiguration) {
	// 5 个 CG 分组各有一张整屏底图（cg0_base..cg4_base），底图里烤着
	// 当前页签高亮；场景 2 页同理。缩略图由引擎按 cg.dat/scene.dat 合成，
	// 底图的黑色空槽正好是 6 列 x 4 行（第一行左侧让给标题）。
	cgThumbs := mddvdThumbGrid()
	sceneThumbs := mddvdThumbGrid()
	cfg.Museum = Museum{
		Modes: []MuseumMode{
			{Key: "music"},
			{Key: "cg"},
			{Key: "scene"},
		},
		// CG/场景/音乐页的 戻る 烤在底图里，热区统一覆盖右下角。
		Return: UIButton{Image: "", Hit: [4]int{612, 500, 152, 68}},
		CG: MuseumCGPage{
			Base:        "cg0_base",
			Bases:       []string{"cg0_base", "cg1_base", "cg2_base", "cg3_base", "cg4_base"},
			PageButtons: mddvdPageTabs(5),
			Thumbs:      cgThumbs,
			ThumbX:      37, ThumbY: 110, ThumbW: 100, ThumbH: 75,
			Percent:      UIButton{Image: "cg_number", Frames: 10, X: 145, Y: 512},
			PercentSpace: 12,
		},
		Scene: MuseumScenePage{
			Base:  "sc0_base",
			Bases: []string{"sc0_base", "sc1_base"},
			// 场景页只有 2 个圆形页签，位置在 x=348/404（CG 的 5 个页签
			// 才从 264 开始）；沿用 CG 的间距会把第 1 页签放到空白处。
			PageButtons: []UIButton{
				{Hit: [4]int{348, 504, 48, 48}},
				{Hit: [4]int{404, 504, 48, 48}},
			},
			Thumbs: sceneThumbs,
			ThumbX: 37, ThumbY: 110, ThumbW: 100, ThumbH: 75,
		},
		Music: MuseumMusicPage{
			Base:        "music_base",
			BakedTracks: true,
			TrackCount:  20,
			Tracks:      mddvdMusicTracks(),
			Names:       mddvdMusicNames(),
		},
		// 这一代没有独立的 CG 查看器素材：看大图时按原版直接铺满屏幕，
		// 点画面切下一张，返回键回鉴赏室。右下角仍留一个透明返回热区，
		// 方便触屏玩家不必依赖系统返回键。
		Viewer: CGViewer{ClickAdvance: true, Exit: UIButton{Hit: [4]int{612, 520, 152, 48}}},
	}
}

// mddvdPageTabs 生成 CG/场景页底部的 5 个/2 个圆形页码热区。
// x=264 起、间距 56、48x48；图里当前页的圆形高亮已经烤在各自底图上。
// mddvdMusicNames 是 20 首曲目的资源名：Bgm.snn 里是 bgm02..bgm21。
// 曲名本身已经烤在 music_base 上，这里只给播放事件用的文件名。
func mddvdMusicNames() []string {
	names := make([]string, 0, 20)
	for track := 2; track <= 21; track++ {
		names = append(names, "bgm"+twoDigits(track))
	}
	return names
}

func mddvdPageTabs(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		x := 264 + index*56
		buttons = append(buttons, UIButton{Hit: [4]int{x, 504, 48, 48}})
	}
	return buttons
}

// mddvdThumbGrid 返回 23 个缩略图格：第一行从左起第 2 格开始（左上角
// 让给「CG鑑賞」标题），其余三行每行 6 格。
func mddvdThumbGrid() []UIPoint {
	points := make([]UIPoint, 0, 23)
	for row := 0; row < 4; row++ {
		start := 0
		if row == 0 {
			start = 1
		}
		for column := start; column < 6; column++ {
			points = append(points, UIPoint{X: 37 + column*125, Y: 110 + row*100})
		}
	}
	return points
}

// mddvdMusicTracks 是音乐鉴赏的 20 行命中区：左列 10 行、右列 10 行，
// 曲名已经烤在 music_base 里。点击行直接播放对应 bgm02..bgm21。
func mddvdMusicTracks() []UIButton {
	tracks := make([]UIButton, 0, 20)
	for index := 0; index < 20; index++ {
		x, y := 40, 112+index*40
		if index >= 10 {
			x, y = 410, 112+(index-10)*40
		}
		tracks = append(tracks, UIButton{Hit: [4]int{x, y, 350, 35}})
	}
	return tracks
}

func applyBuiltinMddvdLog(cfg *BsxxConfiguration) {
	// 没有回想底图：用半透明黑铺满屏幕，右侧 log_meter 滚动条，
	// 每句左侧画 log_vp 语音图标。
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 10, LeftPos: 60, Width: 660, FontSize: 18,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 752, Y: 24},
		VoiceIcon: "log_vp",
	}
}
