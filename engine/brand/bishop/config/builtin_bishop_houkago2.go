package config

// builtin_bishop_houkago2.go 给「放課後2～白濁のレッスン～」（HOUKAGO2，2006，
// BSScript Rev.5）补整套内置界面。
//
// 这一代发行目录没有 bsxx.cfg：标题、对话框、存读档页、设置页、鉴赏室和地图的
// 坐标全部编译在 af2.exe 里。只按 bsxx.cfg 解析会得到一份全空配置——进游戏
// 没有标题菜单、没有消息框和工具条，标题里的「設定/鑑賞室/続きから」也全都
// 打不开，正好对应玩家看到的“主界面点不动、剧情里没有对话框”。
//
// 坐标不是按截图猜的，来源有三类可核对的数据：
//   - 命中表：title_hit / message_hit / dm_hit / config_hit / museum_hit 里
//     的色块矩形（引擎按图片像素扫描，和原版完全一致）；
//   - 素材本身：cfg_ex 上烘焙的标签、cfg_mode_* 的页签帧、dm_slot 的三态框、
//     museum_menu 的 5×3 帧、map_info_base 的空槽；
//   - af2.exe 的初始化函数：消息窗底图位置 (0,468)、底部八颗按钮的 x 与
//     y=屏高-36、cfg_ex 的 (72,118)、config/data 页共用的顶栏与底栏等。
//
// 只在这份素材指纹成立、且配置确实为空时生效；其它世代与现代作品不受影响。

var builtinHoukago2Assets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_back", "message_hit", "msg_name", "msg_info", "msg_info_back",
	"msg_info_str1", "msg_info_str2", "msg_z",
	"dm_hit", "dm_slot", "dm_slotbase", "dm_pagebase",
	"config_base", "config_hit", "cfg_ex", "cfg_button", "cfg_mark", "cfg_meter",
	"cfg_mode_save", "cfg_mode_load", "cfg_mode_config",
	"museum_base", "museum_menu", "museum_titlelistbase",
	"map_base", "map_info_base", "map_info_str", "map_info_count", "map_z_base",
}

// builtinHoukago2Excluded 是相邻世代独有的素材。任意一张出现就说明不是本作：
//   - title_menu_op / title_menu_begin：KD/AFDVD/MK2 的分图标题；
//   - dm_base / save_base / load_base：KD/MK2、学園(GK) 的整屏存档底图；
//   - gamemenu / msg_base：AFDVD 与现代作品的命名。
var builtinHoukago2Excluded = []string{
	"title_menu_op", "title_menu_begin", "title_menu_load",
	"dm_base", "save_base", "load_base", "gamemenu", "msg_base", "kaisou",
}

func isBuiltinHoukago2(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinHoukago2Assets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinHoukago2Excluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// ApplyBuiltinHoukago2Defaults 在配置为空、素材是放課後2 时填好整套界面。
// 返回 true 表示改写了 cfg；只被 ApplyBuiltinBishopDefaults 调用。
func ApplyBuiltinHoukago2Defaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || !isBuiltinHoukago2(probe) {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	// 这一代全部素材按 800x600 编译进 exe；不要跟随宿主窗口尺寸，
	// 否则探测到 1024x768 时整套界面会整体错位。
	cfg.Width = 800
	cfg.Height = 600
	// 槽位是 160x120，正好是 800x600 的 0.2；缩略图按同一比例生成。
	cfg.ThumbnailRatio = 0.2
	cfg.EyePattern = 6

	applyHoukago2Title(cfg)
	applyHoukago2Message(cfg)
	applyHoukago2GameMenu(cfg)
	applyHoukago2Information(cfg)
	applyHoukago2Select(cfg)
	applyHoukago2DataScreen(cfg)
	applyHoukago2Log(cfg)
	applyHoukago2Museum(cfg)
	applyHoukago2Config(cfg)
	applyHoukago2MapPhase(cfg)
	return true
}

// applyHoukago2Title 填标题菜单。
//
// title_menu 是 152x168 的六帧竖排图（オープニング/始めから/続きから/
// 鑑賞室/設定/終了）。六行文字在图上居中，而 title_hit 的六块热区
// （28,76,152x28 / 52,124,104x28 …）中心都落在 x=104，所以六帧统一
// 画在 x=28，每一帧只画自己的那一行。悬停不能切帧，否则六个按钮会一起
// 变成第一行「オープニング」。
func applyHoukago2Title(cfg *BsxxConfiguration) {
	keys := []string{"op", "start", "load", "museum", "config", "quit"}
	entries := make([]TitleMenuEntry, 0, len(keys))
	for index, key := range keys {
		entries = append(entries, TitleMenuEntry{
			Key: key, Image: "title_menu", Frames: 6,
			X: 28, Y: 76 + 48*index,
			Frame: index,
		})
	}
	cfg.Title = TitlePhase{
		// 原版进标题就起播 bgm08（af2.exe 的标题初始化把它压进 BGM 槽）。
		BgmName:  "bgm08",
		Base:     "title_base",
		HitTable: "title_hit",
		Entries:  entries,
	}
}

// applyHoukago2Message 填对话框、字体和名牌。
//
// message 是 800x132 的圆角边框，贴在屏幕底部：600-132=468。窗口内部是
// 透明的，原版用 message_back 的白色掩码形状做半透明衬底；这里用
// FillRect 只在边框内部铺一层 31% 白，避免亮色 CG 上的正文读不清。
// 文字区避开左上/右下圆角，底部让出按钮条（按钮在相对 y=96，即屏幕 564）。
func applyHoukago2Message(cfg *BsxxConfiguration) {
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 468,
		Speed: 100,
		// 边框内部是透明的：原版拿 message_back 的白色掩码整块乘上
		// 0xa6000000（65% 黑），连圆角一起填。FillRect 只作为 mask 缺失时的
		// 兜底，保留同样的颜色。
		FillMask: "message_back",
		FillRect: [4]int{101, 4, 598, 120}, FillColor: 0xa6000000,
		// BgColor 是设置画面「BACKGROUND」滑杆的初值：原版默认 0xa6 黑，
		// 没有 bsxx.cfg 时也要让滑杆和实际窗口一致（见 defaultSettings）。
		BgColor: 0xa6000000,
		// 名牌 msg_name 是 316x36 的白底蓝框，原版把它放在窗口上沿之外
		// (96,428)。名字文字用深蓝，压在浅色牌子上才清楚。
		NameBase: "msg_name", NameBaseX: 96, NameBaseY: -40,
		NameX: 110, NameY: -34, NameW: 290, NameH: 28,
		NameColor: 0xff1a2f6b,
		TextX:     112, TextY: 12, TextW: 580, TextH: 80,
		// unagashi 是 42x360 的 8 帧音符动画，贴在窗口右下角（相对 660,50）。
		Unagashi: "unagashi", UnagashiX: 660, UnagashiY: 50,
		UnagashiCellW: 42, UnagashiCellH: 45,
		UnagashiColumns: 1, UnagashiFrames: 8, UnagashiSpeed: 100,
	}
	// 正文区 80px 高：20 号字、行高 24 正好三行；580px 约 24 个全角字。
	cfg.Font = FontSettings{
		Size: 20, Width: 24, Height: 24, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 24, Lines: 3,
	}
}

// applyHoukago2GameMenu 填对话框底部的八颗工具按钮。
//
// 坐标和按钮图直接来自 af2.exe 的消息窗初始化：y = 屏高-36 = 564，
// x 依次是 124/196/272/356/408/460/524/604。顺序是
// QSAVE / QLOAD / SYSTEM / LOG / SKIP / AUTO / V.PLAY / FIRE，
// 与 message_hit 的八块 24px 高命中区逐一对应。
func applyHoukago2GameMenu(cfg *BsxxConfiguration) {
	buttons := []MenuButton{
		{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 124, Y: 564}},
		{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 196, Y: 564}},
		{Key: "config", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 272, Y: 564}},
		{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 356, Y: 564}},
		{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 408, Y: 564}},
		{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 460, Y: 564}},
		{Key: "voice", UIButton: UIButton{Image: "btn_vplay", Frames: 4, X: 524, Y: 564}},
		{Key: "fire", UIButton: UIButton{Image: "btn_fire", Frames: 4, X: 604, Y: 564}},
	}
	cfg.GameMenu = GameMenu{
		Overlay:    true,
		ShowArea:   [4]int{0, 468, 800, 132},
		Persistent: false,
		Buttons:    buttons,
	}
}

// applyHoukago2Information 填右下角的「第N週 X曜日」小窗。
//
// msg_info 是 88x128 的边框，af2.exe 把它放在 (708,468)（对话框右侧）。
// msg_info_str1 是 22x88 的四帧「第/週/曜/日」，msg_info_str2 是 26x442 的
// 十七帧「0..9 / 月火水木金 / 週末」。一周按 6 天换算（月～週末），
// 第 day 天 = 第 day/6 週、第 day%6 个工作日。
func applyHoukago2Information(cfg *BsxxConfiguration) {
	cfg.Information = InformationWindow{
		Base: "msg_info", BaseX: 708, BaseY: 468,
		DayVariable: "#day", DayZero: true,
		// 没有"特殊日期"图案；门槛必须设成大数，否则 0 >= 0 会被当成
		// 未知日期而整块跳过，周数/曜日永远画不出来。
		DayBorder: 1 << 30,
		WeekStrip: DigitStrip{
			Image: "msg_info_str2", Frames: 17, Start: 1,
			X: 31, Y: 12,
		},
		WeekDayStrip: DigitStrip{
			Image: "msg_info_str2", Frames: 17, Start: 10,
			X: 9, Y: 48,
		},
		WeekDays: 6, WeekStart: 1,
		// 第 6 天是「週末」：msg_info_str2 的 15/16 帧是「週」「末」，
		// 画在「曜」「日」两个字牌的位置上，并跳过那两个字牌。
		// 坐标相对信息窗原点 (708,468)：744/766 绝对坐标对应 36/58。
		WeekendFirst: DigitStrip{
			Image: "msg_info_str2", Frames: 17, Start: 15,
			X: 36, Y: 48,
		},
		WeekendSecond: DigitStrip{
			Image: "msg_info_str2", Frames: 17, Start: 16,
			X: 58, Y: 48,
		},
		WeekendReplacesLabels: true,
		// 面板内部同样用原版的 message 底色（65% 黑）：msg_info_str2 的数字
		// 是黑芯蓝边，压在深底上只剩蓝色轮廓，和原版截图一致；铺白底反而
		// 会把黑芯数字吞掉、整块发灰。
		FillMask: "msg_info_back",
		FillRect: [4]int{716, 472, 72, 118}, FillColor: 0xa6000000,
		Labels: []UIButton{
			{Image: "msg_info_str1", Frames: 4, Start: 0, FixedFrame: true, X: 716, Y: 480},
			{Image: "msg_info_str1", Frames: 4, Start: 1, FixedFrame: true, X: 766, Y: 480},
			{Image: "msg_info_str1", Frames: 4, Start: 2, FixedFrame: true, X: 744, Y: 516},
			{Image: "msg_info_str1", Frames: 4, Start: 3, FixedFrame: true, X: 766, Y: 516},
		},
	}
}

// applyHoukago2Select 填选择肢窗口。
//
// item0 是 520x72（两帧 520x36：常态/悬停），屏幕居中；正文行高与选项条
// 一致，DatumLine 取旧作通用的 363。
func applyHoukago2Select(cfg *BsxxConfiguration) {
	cfg.SelectWindow = NewSelectWindowConfiguration(363, 1, []SelectDesign{
		{
			Item: "item0", Frames: 2, Interval: 4,
			TextX: 0, TextY: 8,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0x88ffffff,
			SelectColor:  0xffff4488,
		},
	})
}

// applyHoukago2DataScreen 填存读档页。
//
// 这一代没有 save_base/load_base 整屏底图：config_base 同时充当存档、读档和
// 设置的装饰底（顶部三颗页签、底部三颗导航键都烘焙在里面）。dm_hit 给出
// 4x3 个 160x120 槽位、顶部 10 个 24x24 页码块和底部三条导航键。
func applyHoukago2DataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase: "config_base", LoadBase: "config_base", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 12,
		// dm_slot 是 176x136 的三态框：0/2 透明、1 是蓝色选中框。
		// dm_slotbase 是槽位下面的木纹底板，空格子靠它才不会露出黑块。
		SlotFrame: "dm_slot", SlotFrameFrames: 3,
		SlotFrameHover: 1, SlotFrameSelected: 1,
		SlotBase:      "dm_slotbase",
		SlotThumb:     [4]int{0, 0, 160, 120},
		AutoSlotIndex: -1,
		NoData:        "dm_nodata",
		// 顶部三颗模式页签：cfg_mode_save/load/config 各 3 帧（常态/悬停/当前）。
		SaveMode: UIButton{Image: "cfg_mode_save", Frames: 3, X: 376, Y: 4},
		LoadMode: UIButton{Image: "cfg_mode_load", Frames: 3, X: 502, Y: 4},
		Config:   UIButton{Image: "cfg_mode_config", Frames: 3, X: 628, Y: 4},
		// 左上角 plaque 的大号模式名（cfg_title 的 300x180 三帧）。
		TitleLabel: UIButton{Image: "cfg_title", Frames: 3, X: 20, Y: 12},
		// 底部 BACK/TITLE/EXIT：cfg_button 是 128x216 的六帧，0..2 是白字，
		// 3..5 是按下态。这里固定画白字那一组，避免悬停时整块变黑。
		Back:  UIButton{Image: "cfg_button", Frames: 6, Start: 0, FixedFrame: true, X: 192, Y: 560, Hit: [4]int{192, 560, 128, 36}},
		Title: UIButton{Image: "cfg_button", Frames: 6, Start: 1, FixedFrame: true, X: 336, Y: 560, Hit: [4]int{336, 560, 128, 36}},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, Start: 2, FixedFrame: true, X: 480, Y: 560, Hit: [4]int{480, 560, 128, 36}},
	}
	// 覆盖已有存档时的确认窗口。dm_slotviewer 是 464x208 的窗口，居中；
	// dm_yn 是 112x144 的四帧（決定/取り消し 各两态），这里固定画常态那两帧。
	screen.Confirm = SlotViewer{
		Base: "dm_slotviewer", BaseX: 168, BaseY: 196,
		OK:     UIButton{Image: "dm_yn", Frames: 4, Start: 0, FixedFrame: true, X: 216, Y: 344},
		Cancel: UIButton{Image: "dm_yn", Frames: 4, Start: 1, FixedFrame: true, X: 456, Y: 344},
	}
	// 4 列 x 3 行，列距 192、行距 136，与 dm_hit 的槽位坐标一致。
	columns := [4]int{32, 224, 416, 608}
	rows := [3]int{136, 272, 408}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// 页码条「Page Select」是 344x56 的底板，10 个 24x24 空槽从 x=440 起、
	// 间距 32；页码数字用 dm_number 的 0..9 十帧叠在槽里。
	screen.PageSelectFrame = UIButton{Image: "dm_pagebase", Frames: 1, X: 408, Y: 80}
	screen.PageNumbers = UIButton{Image: "dm_number", Frames: 10}
	screen.PageNumberStart = 1
	for index := 0; index < 10; index++ {
		x := 440 + 32*index
		screen.PageHits = append(screen.PageHits, UIPoint{X: x, Y: 88})
		screen.PageButtons = append(screen.PageButtons, UIButton{
			Image: "dm_pagenum", Frames: 3, X: x, Y: 88,
		})
		screen.PageNumberPos = append(screen.PageNumberPos, UIPoint{X: x + 6, Y: 92})
	}
	cfg.DataScreen = screen
}

// applyHoukago2Log 填回想界面：没有专属底图，用半透明黑 + 右侧滚动条。
func applyHoukago2Log(cfg *BsxxConfiguration) {
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 7, LeftPos: 112, Width: 576,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 748, Y: 24},
		VoiceIcon: "log_vp",
	}
}

// applyHoukago2Museum 填鉴赏室。
//
// museum_base 左侧已经烘焙了 5 个空白页签槽（8..184，y=42+48i）；museum_menu
// 是 176x540 的 15 帧图：0..4 常态、5..9 悬停、10..14 选中。页签坐标与槽位
// 逐像素重合，所以 X=8、行距 48、命中区 176x36。
func applyHoukago2Museum(cfg *BsxxConfiguration) {
	museum := &cfg.Museum
	museum.Base = "museum_base"
	modes := []struct {
		key   string
		state uint32
		row   int
	}{
		{"music", 1, 0},
		{"cg", 2, 1},
		{"scene", 4, 2},
		{"strip", 8, 3},
	}
	for _, item := range modes {
		y := 42 + 48*item.row
		museum.Modes = append(museum.Modes, MuseumMode{
			Key: item.key, State: item.state,
			UIButton: UIButton{
				Image: "museum_menu", Frames: 15, X: 8, Y: y,
				FrameOffset: item.row, FrameStep: 5,
				Hit: [4]int{8, y, 176, 36},
			},
		})
	}
	museum.Return = UIButton{
		Image: "museum_menu", Frames: 15, X: 8, Y: 234,
		FrameOffset: 4, FrameStep: 5,
		Hit: [4]int{8, 234, 176, 36},
	}

	// BGM 页：右侧是 512x600 的乐谱本，20 行曲名已经烘焙在纸上。
	// 命中条直接照 museum_hit：从 (342,57) 起、行距 22、153x37；
	// 左下角四颗播放控制（60x108 的三帧图）按 af2.exe 的
	// (18/82/146/210,480) 摆放，音量条贴在控制键下方。
	music := &museum.Music
	music.PageBase = UIButton{Image: "museum_titlelistbase", Frames: 1, X: 288, Y: 0}
	music.BakedTracks = true
	for index := 0; index < 20; index++ {
		y := 57 + 22*index
		music.Tracks = append(music.Tracks, UIButton{
			Frames: 1,
			Hit:    [4]int{342, y, 153, 37},
		})
		music.Names = append(music.Names, "bgm"+twoDigits(index+1))
	}
	music.Play = UIButton{Image: "museum_play", Frames: 3, X: 82, Y: 480}
	music.Stop = UIButton{Image: "museum_stop", Frames: 3, X: 18, Y: 480}
	music.Prev = UIButton{Image: "museum_prev", Frames: 3, X: 146, Y: 480}
	music.Next = UIButton{Image: "museum_next", Frames: 3, X: 210, Y: 480}
	music.Volume = UIButton{Image: "museum_volume", Frames: 1, X: 18, Y: 536}
	music.VolumeFill = true
	music.VolumeRegion = [4]int{18, 536, 200, 12}

	// CG / シーン页：4 列 x 5 行，格子 120x90。坐标取自 af2.exe 的
	// 缩略图循环：x=(列+2)*132（264/396/528/660），y=52+行*102。
	// cg0_thumb..cg6_thumb 与 scene0_thumb..scene3_thumb 是 120x1800 的
	// 20 帧预制缩略图条，直接切条比逐格合成快得多，也符合原版。
	museum.CG.ThumbX, museum.CG.ThumbY = houkago2MuseumGridOrigin()
	museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	museum.CG.Thumbs = houkago2MuseumGrid()
	museum.CG.ThumbStrips = houkago2ThumbStrips("cg", 7)
	museum.CG.PageButtons = houkago2MuseumPages(7)
	museum.Scene.ThumbX, museum.Scene.ThumbY = houkago2MuseumGridOrigin()
	museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	museum.Scene.Thumbs = houkago2MuseumGrid()
	museum.Scene.ThumbStrips = houkago2ThumbStrips("scene", 4)
	museum.Scene.PageButtons = houkago2MuseumPages(4)

	// CG 查看器：cgm_base 是 128x48 的底盒，cgm_btn 的 8 帧是
	// 常态（前一张/关闭/后一张/CFG）与红色悬停。
	museum.Viewer = CGViewer{
		Base:       UIButton{Image: "cgm_base", Frames: 1},
		PanelX:     652,
		PanelY:     540,
		Prev:       UIButton{Image: "cgm_btn", Frames: 8, Start: 0, X: 660, Y: 552},
		Exit:       UIButton{Image: "cgm_btn", Frames: 8, Start: 1, X: 684, Y: 552},
		Next:       UIButton{Image: "cgm_btn", Frames: 8, Start: 2, X: 708, Y: 552},
		Config:     UIButton{Image: "cgm_btn", Frames: 8, Start: 3, X: 732, Y: 552},
		ButtonBase: 0, HoverOffset: 4, ToggleHoverOffset: 4,
	}

	// 立ち絵页：museum_ch1..5 是 192x220 的角色卡，排成 3+2。
	// 立ち絵页：museum_ch1..5 是 192x220 的角色卡。坐标取自 af2.exe：
	// 碧在正中央 (419,192)，其余四人分居四角 (255,12)/(583,12)/(255,372)/(583,372)，
	// 中央卡片盖在四角接缝上，形成原版的“扇形叠放”效果。
	centerX, centerY := 419, 192
	cornerX := [2]int{255, 583}
	cornerY := [2]int{12, 372}
	positions := [5]UIPoint{
		{X: centerX, Y: centerY},
		{X: cornerX[0], Y: cornerY[0]},
		{X: cornerX[1], Y: cornerY[0]},
		{X: cornerX[0], Y: cornerY[1]},
		{X: cornerX[1], Y: cornerY[1]},
	}
	for index := 0; index < 5; index++ {
		museum.StripPortraits = append(museum.StripPortraits, UIButton{
			Image: "museum_ch" + itoa(index+1), Frames: 1,
			X: positions[index].X, Y: positions[index].Y,
		})
	}
}

// houkago2MuseumGridOrigin 返回 CG/シーン页首格坐标（af2.exe 的缩略图循环）。
func houkago2MuseumGridOrigin() (int, int) { return 264, 52 }

// houkago2MuseumGrid 是 CG/シーン页的 4x5 缩略图网格。
func houkago2MuseumGrid() []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 5; row++ {
		for column := 0; column < 4; column++ {
			points = append(points, UIPoint{X: 264 + 132*column, Y: 52 + 102*row})
		}
	}
	return points
}

// houkago2ThumbStrips 生成 cgN_thumb / sceneN_thumb 的竖排缩略图条名单。
func houkago2ThumbStrips(prefix string, count int) []string {
	strips := make([]string, 0, count)
	for index := 0; index < count; index++ {
		strips = append(strips, prefix+itoa(index)+"_thumb")
	}
	return strips
}

// houkago2MuseumPages 生成页码块：museum_page 是 36x108 的三帧图（每帧 36x36），
// 原版在左栏排成 5+5 两行——x=30+40*(i%5)，第一行 y=488、第二行 y=528。
func houkago2MuseumPages(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		y := 488
		if index >= 5 {
			y = 528
		}
		buttons = append(buttons, UIButton{
			Image: "museum_page", Frames: 3,
			X: 30 + 40*(index%5), Y: y,
		})
	}
	return buttons
}

// applyHoukago2Config 填旧式设置画面。
//
// cfg_ex 是 656x404 的设置内容，原版画在 (72,118)；config_base 是顶栏
// （SAVE/LOAD/CONFIG 三颗页签）和底栏（BACK/TITLE/EXIT）的装饰底。
// 所有控件坐标都是屏幕绝对坐标，正好落在 cfg_ex 烘焙的标签/滑杆上。
func applyHoukago2Config(cfg *BsxxConfiguration) {
	layout := LegacyConfigScreen{
		Base:     "cfg_ex",
		HitTable: "config_hit",
		Backdrop: "config_base",
		BaseX:    72,
		BaseY:    118,
		// 左上角 plaque 固定画 CONFIG 帧（cfg_title 的第 3 帧）。
		TitleLabel: UIButton{Image: "cfg_title", Frames: 3, Start: 2, FixedFrame: true, X: 20, Y: 12},
		// cfg_mark 是 16x48 的三帧：方框 / 圆点 / 菱形。
		// 复选框用方框，画面效果/スキップ処理的单选用圆点。
		CheckMark:       "cfg_mark",
		CheckMarkFrames: 3,
		RadioButton:     "cfg_mark",
		RadioFrames:     3,
		ToggleHitWidth:  140,
		// SCREEN 面板：全画面表示 / アンチエイリアス / カットイン。
		ScreenMode: UIPoint{X: 224, Y: 148},
		ExtraToggles: []LegacyToggle{
			{Key: "anti alias", X: 96, Y: 228},
			{Key: "cutin", X: 96, Y: 248},
		},
		// VOICE 面板：音声をループする。
		VoiceLoop: UIPoint{X: 96, Y: 308},
		// MESSAGE 面板：未読もスキップ + 三个スキップ処理。
		SkipUnread:        UIPoint{X: 516, Y: 188},
		SkipModeMarkFrame: 1,
		SkipModes: []UIPoint{
			{X: 516, Y: 208}, {X: 516, Y: 228}, {X: 516, Y: 248},
		},
	}
	// 画面効果：なし / コマ数固定 / 時間固定。
	layout.RadioGroups = []LegacyRadioGroup{
		{Key: "screen effect", MarkFrame: 1, HitWidth: 92, Points: []UIPoint{
			{X: 224, Y: 168}, {X: 224, Y: 188}, {X: 224, Y: 208},
		}},
	}
	// メッセージ速度 / オート速度：cfg_ex 画好 10 个刻度点，
	// config_hit 的 10 个 12x12 热区在 (532..676, 148/168)，间距 16。
	layout.Speeds = []LegacyGauge{
		{Dots: houkago2SpeedDots(148), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
		{Dots: houkago2SpeedDots(168), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
	}
	// BACKGROUND 四条色条：赤/緑/青/濃度。cfg_meter 的帧序是
	// 0 青(音量) / 1 蓝 / 2 绿 / 3 红 / 4 灰，所以赤取 3、緑取 2、青取 1、
	// 濃度取 4；按值裁宽度填进底图的空槽。
	meterFrames := [4]int{3, 2, 1, 4}
	layout.Colors = make([]LegacyGauge, 4)
	for index := 0; index < 4; index++ {
		layout.Colors[index] = LegacyGauge{
			Bar: UIButton{
				Image: "cfg_meter", Frames: 5, X: 437, Y: 308 + 20*index,
				Start: meterFrames[index], FixedFrame: true,
			},
			Fill: true, TrackWidth: 274,
		}
	}
	// VOLUME 四条：烘焙标签顺序是 BGM / 環境音 / 声 / 効果音，
	// 引擎通道下标是 1=BGM、2=SE、3=BGS、4=VOICE，所以按 1,3,4,2 填。
	layout.Volumes = make([]LegacyGauge, 5)
	volumeRows := []struct {
		channel int
		y       int
	}{{1, 432}, {3, 452}, {4, 472}, {2, 492}}
	for _, row := range volumeRows {
		layout.Volumes[row.channel] = LegacyGauge{
			Bar: UIButton{
				Image: "cfg_meter", Frames: 5, X: 437, Y: row.y,
				Start: 0, FixedFrame: true,
			},
			Fill: true, TrackWidth: 274,
		}
	}
	// VOICE 面板的六个角色头像（64x64）。
	characterPoints := []UIPoint{
		{X: 100, Y: 330}, {X: 172, Y: 330}, {X: 244, Y: 330},
		{X: 100, Y: 402}, {X: 172, Y: 402}, {X: 244, Y: 402},
	}
	for index := 0; index < 6; index++ {
		layout.Characters = append(layout.Characters, LegacyCharacter{
			Button: UIButton{
				Image: "cfg_ch" + itoa(index+1), Frames: 1,
				X: characterPoints[index].X, Y: characterPoints[index].Y,
			},
		})
	}
	// 底部三颗导航键与存档页共用同一套坐标/帧。
	layout.Back = UIButton{Image: "cfg_button", Frames: 6, Start: 0, FixedFrame: true, X: 192, Y: 560, Hit: [4]int{192, 560, 128, 36}}
	layout.Title = UIButton{Image: "cfg_button", Frames: 6, Start: 1, FixedFrame: true, X: 336, Y: 560, Hit: [4]int{336, 560, 128, 36}}
	layout.Exit = UIButton{Image: "cfg_button", Frames: 6, Start: 2, FixedFrame: true, X: 480, Y: 560, Hit: [4]int{480, 560, 128, 36}}
	// 顶部三颗模式页签，和存档页一致。
	layout.ModeSave = UIButton{Image: "cfg_mode_save", Frames: 3, X: 376, Y: 4}
	layout.ModeLoad = UIButton{Image: "cfg_mode_load", Frames: 3, X: 502, Y: 4}
	// 设置页始终是 CONFIG 页，固定画当前帧（第 3 帧）。
	layout.ModeConfig = UIButton{Image: "cfg_mode_config", Frames: 3, Start: 2, FixedFrame: true, X: 628, Y: 4}
	// 「初期設定に戻す」是 224x64 的两帧文字，叠在 config_hit 的 240x32 块上。
	layout.Reset = UIButton{
		Image: "cfg_reset", Frames: 2, X: 84, Y: 486,
		Hit: [4]int{84, 486, 240, 32},
	}
	cfg.LegacyConfig = layout
}

// houkago2SpeedDots 返回一行 10 个速度刻度（x=532..676，16px 一格）。
func houkago2SpeedDots(y int) []UIPoint {
	points := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		points = append(points, UIPoint{X: 532 + 16*index, Y: y})
	}
	return points
}

// applyHoukago2MapPhase 填地图顶部的「第N週 X曜日」卡。
//
// map_info_base 是 164x60 的边框，原版放在 (4,4)；map_info_str 是
// 20x240 的十二帧：1..5 是周数、月火水木金土日 是曜日。日期按一周
// 6 天换算（月～週末），与信息窗保持一致。地图底图与系统菜单由
// BsMapProfileHoukago2 按 map.dat 补。
func applyHoukago2MapPhase(cfg *BsxxConfiguration) {
	cfg.MapPhase = MapPhase{
		Background: "map_base",
		HitImage:   "map_hit",
		Frame:      UIButton{Image: "map_info_base", Frames: 1, X: 4, Y: 4},
		// 周数数字槽与曜日槽：af2.exe 的 map_info_str 画在相对 (44,8)/(20,32)，
		// map_info_base 在 (4,4)，所以屏幕坐标是 (48,12)/(24,36)。
		WeekNo: DigitStrip{
			Image: "map_info_str", Frames: 12,
			X: 48, Y: 12,
		},
		WeekDay: DigitStrip{
			Image: "map_info_str", Frames: 12, Start: 5,
			X: 24, Y: 36,
		},
		// 行动序号：map_info_count 3 帧（1/2/3，每帧 40x44）。
		// af2.exe 只在控制值 4..6 时画，帧号 = 值-4；本作这个值是 #time，
		// 所以第 1/2/3 次移动分别显示 1/2/3（不是剩余次数）。
		MoveCount: DigitStrip{
			Image: "map_info_count", Frames: 3,
			X: 108, Y: 12,
		},
		MoveCountVariable: "#time",
		MoveCountBase:     4,
		// 絶倫メーター：map_z_base 外框在 (724,4)，msg_z 是 7 帧竖排瓶子
		// （每帧 56x110），画在框内 (732,12)；每 20 点升一级。
		MeterBase: UIButton{Image: "map_z_base", Frames: 1, X: 724, Y: 4},
		Meter: DigitStrip{
			Image: "msg_z", Frames: 7,
			X: 732, Y: 12,
		},
		MeterVariable: "#zeturin",
		MeterStep:     20,
		DayVariable:   "#day",
	}
}
