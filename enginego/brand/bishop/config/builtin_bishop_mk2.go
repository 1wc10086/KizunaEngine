package config

// builtin_bishop_mk2.go 给「牝教師2～淫悦の学舎～」（MK2，BSScript Rev.6）
// 补内置布局。
//
// 这部作品的发行目录没有 bsxx.cfg：标题菜单、对话框、工具条、存读档页、
// 回想和鉴赏室的坐标全部编译在 mk2.exe 里。只按 bsxx.cfg 解析会得到一份
// 全空配置——进游戏直接落进 opening、对话框退回宿主 Flutter、
// 标题/鉴赏室/设置全部打不开，和原版差得很远。
//
// 坐标来源都是可核对的素材本身，而不是凭截图猜：
//   - 标题：title_menu_* 的 BSG 图头 OffsetX/OffsetY 正好是 (620,424)、
//     (548,484) 这些屏幕坐标；title_hit 的 5 个命中块也逐像素吻合。
//   - 对话框：message 是 800x124 的底图；message_hit 的命中块给出
//     底部 7 颗系统按钮与右侧 7 颗工具按钮的精确矩形。
//   - 存读档：dm_hit 的 13 个值 = 12 个 160x120 槽位 + 1 个页码块，
//     cfg_mode_* 的图头偏移给出底部 SAVE/LOAD/CONFIG/ITEM 的位置。
//   - 鉴赏室：museum_menu 的 5 个部件、museum_titlelist 的 20 条曲名、
//     museum_bgmctrl 的播放控制部件都带屏幕坐标。
//
// 只在这个素材指纹成立时生效；KD/AFDVD/GK2 与现代作品都不会命中。

var builtinMk2Assets = []string{
	"title_base", "title_hit",
	"title_menu_op", "title_menu_begin", "title_menu_load",
	"title_menu_museum", "title_menu_setting", "title_menu_quit",
	"message", "message_hit", "unagashi",
	"dm_base", "dm_hit", "dm_slot", "dm_auto", "dm_nodata",
	"map_base1", "map_ch101", "map_hit",
	"museum_base", "museum_menu", "museum_titlelist",
	"cfg_mode_save", "cfg_mode_load", "cfg_mode_config",
}

// builtinMk2Excluded 是相邻世代独有的素材：
//   - 学園2 的标题是单张 title_menu（8 帧），没有 title_menu_op 分图；
//   - KD 有 dm_save/dm_load/kaisou，对话框是 message+kaisou 那一套；
//   - MK2IE 有 msg_base/gamemenu_base 的现代命名，走 bsxx.cfg。
var builtinMk2Excluded = []string{
	"title_menu", "dm_save", "dm_load", "kaisou", "msg_base", "gamemenu_base",
}

func isBuiltinMk2(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinMk2Assets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinMk2Excluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// ApplyBuiltinMk2Defaults 在配置为空、素材是牝教師2 老版时填好整套界面。
// 返回 true 表示改写了 cfg；只被 ApplyBuiltinBishopDefaults 调用。
func ApplyBuiltinMk2Defaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || !isBuiltinMk2(probe) {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	width, height := probe.Width, probe.Height
	if width <= 0 || height <= 0 {
		width, height = 800, 600
	}
	cfg.Width = width
	cfg.Height = height
	cfg.ThumbnailRatio = 0.2
	applyBuiltinMk2Title(cfg)
	applyBuiltinMk2Message(cfg, height)
	applyBuiltinMk2GameMenu(cfg, height)
	applyBuiltinMk2DataScreen(cfg)
	applyBuiltinMk2Log(cfg)
	applyBuiltinMk2Museum(cfg)
	applyBuiltinMk2Dress(cfg)
	applyBuiltinMk2ItemScreen(cfg)
	applyBuiltinMk2Config(cfg)
	return true
}

func applyBuiltinMk2Title(cfg *BsxxConfiguration) {
	// 标题 BGM：原引擎进入标题时起播 bgm01，和 MK2IE 的 bsxx.cfg 一致。
	// 菜单图都是 2 帧（常态/悬停），坐标直接用图头里烘焙的屏幕坐标。
	entries := []TitleMenuEntry{
		{Key: "op", Image: "title_menu_op", Frames: 2, X: 620, Y: 424},
		{Key: "start", Image: "title_menu_begin", Frames: 2, X: 548, Y: 484},
		{Key: "load", Image: "title_menu_load", Frames: 2, X: 684, Y: 484},
		{Key: "museum", Image: "title_menu_museum", Frames: 2, X: 496, Y: 544},
		{Key: "config", Image: "title_menu_setting", Frames: 2, X: 624, Y: 544},
		{Key: "quit", Image: "title_menu_quit", Frames: 2, X: 716, Y: 544},
	}
	cfg.Title = TitlePhase{
		BgmName:  "bgm01",
		Base:     "title_base",
		HitTable: "title_hit",
		Entries:  entries,
	}
}

func applyBuiltinMk2Message(cfg *BsxxConfiguration, height int) {
	// message 是 800x124 的整窗底图，贴在屏幕底部：600-124 = 476。
	// 底图自带 alpha，不能再叠一层底色，否则窗口会被压暗。
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: height - 124,
		// message.bsg 的像素 alpha 全 0，只有 MENU/VIEW 标签有实体：
		// 原引擎用 bg color 填出深色窗口。0xE0181414 是偏暖的近黑，
		// 与手册截图里的深褐窗口一致；正文按亮度自动用白字。
		Speed: 100, BgColor: 0xe0181414, FillBase: true,
		NameX: 24, NameY: 6, NameW: 620, NameH: 22, NameColor: 0xffff8822,
		TextX: 24, TextY: 30, TextW: 620, TextH: 72,
		Unagashi:      "unagashi",
		UnagashiX:     700,
		UnagashiY:     70,
		UnagashiCellW: 32, UnagashiCellH: 32,
		UnagashiColumns: 1, UnagashiFrames: 30, UnagashiSpeed: 16,
	}
	// 16 号字、行高 22，正文宽 620 约 38 个半角/19 个全角字，最多 3 行。
	cfg.Font = FontSettings{
		Size: 16, Width: 20, Height: 22, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 38, Lines: 3,
		EudcFontFace: "bishop.ttf",
	}
}

func applyBuiltinMk2GameMenu(cfg *BsxxConfiguration, height int) {
	// 坐标来自 message_hit 的命中块。底部 7 颗是存档/设置/回想，
	// 右侧 7 颗是 SKIP/AUTO/VOICE、VIEW 上下、CUT IN/DRESS。
	// 按钮图都是 4 帧竖排（常态/悬停/按下/不可用）；CUT IN/DRESS 的
	// 单帧是 66x32，命中块 56x24 落在药丸内部，所以绘制点要往左上挪 4..5px。
	buttons := []MenuButton{
		{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 28, Y: height - 20}},
		{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 120, Y: height - 20}},
		{Key: "save", UIButton: UIButton{Image: "btn_save", Frames: 4, X: 224, Y: height - 20}},
		{Key: "load", UIButton: UIButton{Image: "btn_load", Frames: 4, X: 312, Y: height - 20}},
		{Key: "config", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 396, Y: height - 20}},
		{Key: "status", UIButton: UIButton{Image: "btn_status", Frames: 4, X: 500, Y: height - 20}},
		{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 596, Y: height - 20}},
		{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 736, Y: height - 120}},
		{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 736, Y: height - 92}},
		{Key: "voice", UIButton: UIButton{Image: "btn_voice", Frames: 4, X: 736, Y: height - 64}},
		{Key: "view up", UIButton: UIButton{Image: "btn_viewu", Frames: 4, X: 772, Y: height - 90}},
		{Key: "view down", UIButton: UIButton{Image: "btn_viewd", Frames: 4, X: 772, Y: height - 64}},
		{Key: "cut in", UIButton: UIButton{Image: "btn_cutin", Frames: 4, X: 658, Y: height - 112}},
		{Key: "dress", UIButton: UIButton{Image: "btn_dress", Frames: 4, X: 658, Y: height - 80}},
	}
	cfg.GameMenu = GameMenu{
		// 没有 gamemenu 底图：底下的 message 已经带 MENU/VIEW 标签。
		Overlay:    true,
		ShowArea:   [4]int{0, height - 124, cfg.Width, 124},
		Persistent: false,
		Buttons:    buttons,
	}
}

func applyBuiltinMk2DataScreen(cfg *BsxxConfiguration) {
	// dm_hit 的 12 个 160x120 命中块是 4 列 x 3 行，第一格是 Auto Save。
	// 页码条在右上角，页数是 10。
	screen := DataScreen{
		SaveBase: "dm_base", LoadBase: "dm_base", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 12,
		AutoSlotIndex: 0,
		AutoOnly:      "dm_auto",
		NoData:        "dm_nodata",
		// dm_slot 是 176x408 的三态框（常态/悬停/空格），单帧 176x136。
		SlotFrame:         "dm_slot",
		SlotFrameFrames:   3,
		SlotFrameHover:    1,
		SlotFrameSelected: 1,
		SlotThumb:         [4]int{8, 8, 160, 120},
		// 底部四个模式页签的图头偏移就在 (24,536)/(136,536)/(252,536)/(396,536)；
		// ITEM 页在引擎里和 status 共用同一个「持ち物」界面，这里不放热区，
		// 避免和 cfg_mode_status 的图混淆。
		SaveMode: UIButton{Image: "cfg_mode_save", Frames: 3, X: 24, Y: 536},
		LoadMode: UIButton{Image: "cfg_mode_load", Frames: 3, X: 136, Y: 536},
		Config:   UIButton{Image: "cfg_mode_config", Frames: 3, X: 252, Y: 536},
		Item:     UIButton{Image: "cfg_mode_status", Frames: 3, X: 396, Y: 536},
		// BACK/TITLE/QUIT 是 cfg_button 的第 3/4/5 帧，固定帧不随悬停切换。
		Back:  UIButton{Image: "cfg_button", Frames: 6, X: 520, Y: 536, Start: 3, FixedFrame: true},
		Title: UIButton{Image: "cfg_button", Frames: 6, X: 608, Y: 536, Start: 4, FixedFrame: true},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, X: 696, Y: 536, Start: 5, FixedFrame: true},
	}
	columns := [4]int{32, 224, 416, 608}
	rows := [3]int{106, 240, 374}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// 右上角的页码条：dm_pagebase 是一整条底图，10 个页码等分它的宽度。
	// 数字本身由 dm_pagenum 的 3 帧绘制，未悬停/当前/悬停分别取 0/1/2。
	for index := 0; index < 10; index++ {
		screen.PageHits = append(screen.PageHits, UIPoint{X: 404 + index*36, Y: 8})
	}
	screen.PageSelectFrame = UIButton{Image: "dm_pagebase", X: 400, Y: 8}
	screen.SelectFrame = UIButton{Image: "dm_slotshadow", Frames: 1, X: -1, Y: -1}
	cfg.DataScreen = screen
}

func applyBuiltinMk2Log(cfg *BsxxConfiguration) {
	// 这一代没有专属回想底图：沿用当前剧情帧 + 半透明黑，右侧 log_meter
	// 滚动条，有语音的行左侧画 log_vp 图标。和学園2 的回想结构一致。
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 7, LeftPos: 16, Width: 740,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 745, Y: 24},
		VoiceIcon: "log_vp",
	}
}

// applyBuiltinMk2Museum 填鉴赏室。素材坐标都来自 BSS-Composition 的
// 部件偏移：museum_menu 的左侧 5 个页签、museum_titlelist 的 20 条曲名、
// museum_bgmctrl 的播放控制，都不需要额外猜。
func applyBuiltinMk2Museum(cfg *BsxxConfiguration) {
	museum := &cfg.Museum
	museum.Base = "museum_base"
	museum.Modes = []MuseumMode{
		{Key: "music", State: 1, ActiveFrame: 1, UIButton: UIButton{Image: "museum_menu", Part: 1, Frames: 3, X: 76, Y: 20}},
		{Key: "cg", State: 2, ActiveFrame: 1, UIButton: UIButton{Image: "museum_menu", Part: 2, Frames: 3, X: 72, Y: 72}},
		{Key: "scene", State: 4, ActiveFrame: 1, UIButton: UIButton{Image: "museum_menu", Part: 3, Frames: 3, X: 48, Y: 124}},
		{Key: "strip", State: 8, ActiveFrame: 1, UIButton: UIButton{Image: "museum_menu", Part: 4, Frames: 4, X: 24, Y: 176}},
	}
	museum.Return = UIButton{Image: "museum_menu", Part: 5, Frames: 2, X: 56, Y: 256}

	// 曲名不是文字：museum_titlelist 的 part 2..21 是 20 条烘焙好的曲名，
	// 每条都带自己的屏幕坐标（从 (316,60) 一路斜向左下到 (140,516)）。
	// 因此 X/Y 必须取部件偏移，不能像 KD 那样从左上角按固定行距排。
	tracks := make([]UIButton, 0, 20)
	names := make([]string, 0, 20)
	titlePartOffsets := [][2]int{
		{316, 60}, {304, 84}, {296, 108}, {288, 132}, {276, 156},
		{268, 180}, {260, 204}, {248, 228}, {240, 252}, {232, 276},
		{224, 300}, {216, 324}, {204, 348}, {196, 372}, {188, 396},
		{176, 420}, {168, 444}, {160, 468}, {148, 492}, {140, 516},
	}
	for index := 0; index < 20; index++ {
		tracks = append(tracks, UIButton{
			Image: "museum_titlelist", Part: index + 2, Frames: 1,
			X: titlePartOffsets[index][0], Y: titlePartOffsets[index][1],
		})
		names = append(names, "bgm"+twoDigits(index+1))
	}
	museum.Music.Tracks = tracks
	museum.Music.Names = names
	// bgmctrl 的部件：1 是控制盒（260x108 在 518,444），2..5 是四颗
	// 62x72 两帧按钮（播放/停止/上一首/下一首），6/7 是 MIN/MAX 标签。
	// 音量条 museum_volume 是 200x12 的横向填充条，图头偏移 544,500。
	museum.Music.ControlBase = UIButton{Image: "museum_bgmctrl", Frames: 1, Part: 1, X: 518, Y: 444}
	museum.Music.Play = UIButton{Image: "museum_bgmctrl", Part: 2, Frames: 2, X: 520, Y: 444}
	museum.Music.Stop = UIButton{Image: "museum_bgmctrl", Part: 3, Frames: 2, X: 582, Y: 444}
	museum.Music.Prev = UIButton{Image: "museum_bgmctrl", Part: 4, Frames: 2, X: 644, Y: 444}
	museum.Music.Next = UIButton{Image: "museum_bgmctrl", Part: 5, Frames: 2, X: 706, Y: 444}
	museum.Music.Max = UIButton{Image: "museum_bgmctrl", Part: 6, Frames: 2, X: 520, Y: 524}
	museum.Music.Min = UIButton{Image: "museum_bgmctrl", Part: 7, Frames: 2, X: 706, Y: 524}
	museum.Music.Volume = UIButton{Image: "museum_volume", Frames: 1, X: 544, Y: 500}
	museum.Music.VolumeFill = true
	museum.Music.VolumeRegion = [4]int{544, 500, 200, 12}

	// 立ち絵鉴赏：museum_chs 的部件 1 是装饰底板，部件 2..8 是 7 张角色卡
	// （底板在 219,20，卡片各自带烘焙坐标）。点卡进入 bu*/gb* 组合查看器。
	// 卡片的角色顺序不是 1..7：对比卡片头像与 bu 立绘后得到
	// part2..part8 → bu01, bu02, bu03, bu04, bu06, bu07, bu05。
	museum.MK2Strip = true
	museum.StripBackdrop = UIButton{Image: "museum_chs", Part: 1, Frames: 1, X: 219, Y: 20}
	museum.StripPortraits = []UIButton{
		{Image: "museum_chs", Part: 2, Frames: 1, X: 410, Y: 221}, // bu01
		{Image: "museum_chs", Part: 3, Frames: 1, X: 410, Y: 29},  // bu02
		{Image: "museum_chs", Part: 4, Frames: 1, X: 600, Y: 216}, // bu03
		{Image: "museum_chs", Part: 5, Frames: 1, X: 602, Y: 20},  // bu04
		{Image: "museum_chs", Part: 6, Frames: 1, X: 220, Y: 216}, // bu06
		{Image: "museum_chs", Part: 7, Frames: 1, X: 220, Y: 402}, // bu07
		{Image: "museum_chs", Part: 8, Frames: 1, X: 410, Y: 417}, // bu05
	}

	// CG / シーン页：5 列 x 4 行，格子和 KD 同为 120x90 缩略图。
	// 页码用 museum_num 的 12 帧竖向数字条（20 一帧），页数取 10。
	museum.CG.UnderBase = ""
	museum.CG.Base = ""
	museum.CG.ThumbX, museum.CG.ThumbY = 220, 30
	museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	museum.CG.Thumbs = mk2MuseumThumbGrid()
	// cg0_thumb..cg7_thumb 是 8 组预制缩略图条（120x1800，20 帧），
	// 原版直接切条而不是逐张合成；没有条的组会退回合成。
	museum.CG.ThumbStrips = museumThumbStrips("cg", 8)
	museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	museum.CG.PageButtons = mk2MuseumPageButtons(10)
	museum.Scene.ThumbX, museum.Scene.ThumbY = 220, 30
	museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	museum.Scene.Thumbs = mk2MuseumThumbGrid()
	museum.Scene.ThumbStrips = museumThumbStrips("scene", 5)
	museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	museum.Scene.PageButtons = mk2MuseumPageButtons(6)
	// CG 查看器没有独立的控制盒素材：右下角复用 cfg_button 的 BACK 帧，
	// 至少保证玩家能从全屏 CG 退回网格（右键/ESC 也保留退出）。
	museum.Viewer = CGViewer{
		PanelX: 640, PanelY: 520,
		Prev:       UIButton{Image: "cfg_mode_save", Frames: 3, X: 640, Y: 552},
		Exit:       UIButton{Image: "cfg_mode_config", Frames: 3, X: 690, Y: 552},
		Next:       UIButton{Image: "cfg_mode_load", Frames: 3, X: 760, Y: 552},
		ButtonBase: 0, HoverOffset: 1, ToggleHoverOffset: 1,
	}
}

// mk2MuseumPageButtons 生成左上角一列竖向页码按钮。
// museum_num 是 20x240、12 帧的数字条（单帧 20x20），第 1 帧起是 1..9。
func mk2MuseumPageButtons(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		buttons = append(buttons, UIButton{
			Image: "museum_num", Frames: 12,
			X: 20, Y: 96 + index*22,
			Start: index + 1, FixedFrame: true,
		})
	}
	return buttons
}

// applyBuiltinMk2Config 填旧式设置画面。
//
// cfg_ex 是一张 800x600 的复合底图（有效面板 672x480 在 64,24），上面
// 已经烘焙好所有文字标签；引擎只负责画滑块、角色页签与底部按钮。
// 坐标按手册 config.png（240x180 缩放到 800x600）换算，滑块的行位置与
// 原作的三组横条对齐；具体哪一条对应哪个音量沿用引擎的通道下标。
func applyBuiltinMk2Config(cfg *BsxxConfiguration) {
	layout := LegacyConfigScreen{
		Base:            "cfg_ex",
		CheckMark:       "cfg_mark",
		CheckMarkFrames: 3,
		RadioButton:     "cfg_mark",
		RadioFrames:     3,
		// cfg_mark 是 16x48 的三帧（方框/圆点/菱形），单帧 16x16。
		// 角色页签 0 常态、1 关闭语音（灰）。
		CharacterOffFrame: 1,
	}
	// 颜色：cfg_ex 的 BACK GROUND 区有四条空槽（赤/緑/青/濃度），
	// 用 cfg_meter 的填充模式画当前分量。四条槽的 y 从 config.png 换算。
	layout.Colors = make([]LegacyGauge, 4)
	for index := 0; index < 4; index++ {
		layout.Colors[index] = LegacyGauge{
			Bar:        UIButton{Image: "cfg_meter", Frames: 5, X: 470, Y: 297 + index*33},
			Fill:       true,
			TrackWidth: 280,
		}
	}
	// 音量：四条横条（BGM/環境音/音声/効果音），通道下标与引擎的
	// kindBgm/kindSe/kindVoice/kindBgs 对齐。
	layout.Volumes = make([]LegacyGauge, 7)
	for index := 0; index < 4; index++ {
		layout.Volumes[index+1] = LegacyGauge{
			Bar:        UIButton{Image: "cfg_meter", Frames: 5, X: 470, Y: 415 + index*25},
			Fill:       true,
			TrackWidth: 280,
		}
	}
	// 文字速度 / 自动速度：底图已经画好轨道与刻度，只画当前位置的竖线标记。
	layout.Speeds = []LegacyGauge{
		{Knob: UIButton{Image: "cfg_mark", Frames: 3, X: 470, Y: 125}, MarkerOnly: true, TrackWidth: 280, TrackHeight: 16},
		{Knob: UIButton{Image: "cfg_mark", Frames: 3, X: 470, Y: 158}, MarkerOnly: true, TrackWidth: 280, TrackHeight: 16},
	}
	// 单选组：画面効果（なし/コマ数固定/時間固定）与自動選択設定
	// （手動/中に出す/外に出す）。cfg_mark 第 2 帧是红色圆点。
	layout.RadioGroups = []LegacyRadioGroup{
		{Key: "screen effect", MarkFrame: 1, Points: []UIPoint{
			{X: 205, Y: 183}, {X: 205, Y: 210}, {X: 205, Y: 236},
		}},
		{Key: "auto select pos", MarkFrame: 1, Points: []UIPoint{
			{X: 205, Y: 258}, {X: 205, Y: 285}, {X: 205, Y: 312},
		}},
	}
	// 右侧 MESSAGE 区：未読もスキップ（勾选）与スキップ速度三选一（菱形）。
	layout.SkipUnread = UIPoint{X: 550, Y: 208}
	layout.SkipModes = []UIPoint{
		{X: 550, Y: 235}, {X: 550, Y: 261}, {X: 550, Y: 287},
	}
	layout.SkipModeMarkFrame = 2
	// 左下角 8 个角色页签（2 行 x 4 列），cfg_ch1..8 是 64x64 单帧头像。
	names := []string{"cfg_ch1", "cfg_ch2", "cfg_ch3", "cfg_ch4", "cfg_ch5", "cfg_ch6", "cfg_ch7", "cfg_ch8"}
	for index, name := range names {
		layout.Characters = append(layout.Characters, LegacyCharacter{
			Button: UIButton{
				Image: name, Frames: 1,
				X: 100 + (index%4)*76,
				Y: 398 + (index/4)*72,
			},
		})
	}
	// 底部 SAVE/LOAD/CONFIG/ITEM 由 cfg_mode_* 直接在存档页承担；
	// 设置页只有 BACK/TITLE/QUIT 三个文字按钮（cfg_button 的 3/4/5 帧）。
	layout.Back = UIButton{Image: "cfg_button", Frames: 6, X: 520, Y: 536, Start: 3, FixedFrame: true}
	layout.Title = UIButton{Image: "cfg_button", Frames: 6, X: 608, Y: 536, Start: 4, FixedFrame: true}
	layout.Exit = UIButton{Image: "cfg_button", Frames: 6, X: 696, Y: 536, Start: 5, FixedFrame: true}
	layout.Reset = UIButton{Image: "cfg_reset", Frames: 2, X: 60, Y: 300}
	cfg.LegacyConfig = layout
}

// mk2MuseumThumbGrid 是牝教師2 鉴赏室的缩略图网格。
//
// cgN_thumb / sceneN_thumb 都是 120x1800 的预制条（20 帧），原版一页 20 格：
// 左侧 0..200 留给 BGM/CG/シーン/立ち絵 页签，网格从 x=220 起排 4 列，
// 纵向 5 行。旧的 5 列网格会把第一列压在页签底下，玩家看不到那一列。
func mk2MuseumThumbGrid() []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 5; row++ {
		for column := 0; column < 4; column++ {
			points = append(points, UIPoint{X: 220 + column*140, Y: 30 + row*106})
		}
	}
	return points
}

// applyBuiltinMk2Dress 打开牝教師2 老版的ぬぎぬぎ設定画面。
//
// 这一代没有 strip_base / costume_* / strip_ch* 这些界面素材：原引擎把
// 布局编译在 exe 里，立绘用的是 bu<角色2><服装2><阶段2><表情2> 的 400x600
// 图。这里只置一个开关，画面由 systemui_mk2_dress.go 按角色卡 + 阶段按钮
// 自绘（沿用 museum_chs 的角色卡与 bu 命名规则）。
func applyBuiltinMk2Dress(cfg *BsxxConfiguration) {
	cfg.Dress = DressScreen{
		MK2:     true,
		Base:    "museum_base",
		ShowBu:  true,
		Targets: cfg.Museum.StripPortraits,
	}
}

// applyBuiltinMk2ItemScreen 打开牝教師2 老版自绘的 ITEM 画面。
func applyBuiltinMk2ItemScreen(cfg *BsxxConfiguration) {
	cfg.Item = ItemScreen{MK2: true}
}
