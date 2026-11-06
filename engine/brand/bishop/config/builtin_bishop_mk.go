package config

// builtin_bishop_mk.go 给「牝教師～淫辱の教室～」（MK，BSScript Rev.5）补内置布局。
//
// 2006 年的发行目录没有 bsxx.cfg：标题菜单、对话框、工具条、存读档页、
// 回想、鉴赏室、设置和地图的坐标全部编译在 exe 里，剧本是 Rev.5 的 bsx.dat。
// 只按 bsxx.cfg 解析会得到一份全空配置——进游戏没有标题、没有对话框、
// 工具条/存读档/鉴赏室/设置全部打不开，和原版差得很远。
//
// 坐标不是凭截图猜的，来源有三处可核对的数据：
//   - 命中表：title_hit 的 6 个 172x36 块、message_hit 的 9 颗按钮、
//     dm_hit 的 9 个 160x120 槽位与 10 个页码块、config_hit 的逐像素热区、
//     map_hit 的“時間を潰す”块（448,528,104x64）。
//   - 素材本身：message 800x104、museum_menu 15 帧（5 行 x 3 状态）、
//     museum_titlelist 19 行曲名、cfg_meter 五条色条等。
//   - cfg_ex 的烘焙标签与 config_hit 的对齐位置：cfg_ex 必须画在 (182,98)，
//     全画面/音声ループ/角色头像/重置按钮的热区才会逐像素落在标签上。
//
// 只在这份素材指纹成立、且配置确实为空时生效；其它世代与现代作品不受影响。

var builtinMkAssets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_hit",
	"dm_slotbase", "dm_hit", "dm_slot",
	"museum_titlelist", "map_week", "map_time",
}

// builtinMkExcluded 是相邻世代独有的素材，用来把 MK 与其它 Rev 作品分开：
//   - MK2 有 dm_base，KD 有 kaisou/dm_save，学園2 有 save_base/load_base，
//   - AFDVD 的标题是 title_menu_op 分图，现代作品有 msg_base / gamemenu。
//
// MK 自己这些都不带，所以这些名字任意一个出现就说明不是 MK。
var builtinMkExcluded = []string{
	"dm_base", "gamemenu", "save_base", "load_base",
	"kaisou", "dm_save", "title_menu_op", "msg_base",
}

func isBuiltinMk(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinMkAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinMkExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	return true
}

// ApplyBuiltinMkDefaults 在配置为空、素材是牝教師（MK）时填好整套界面。
// 返回 true 表示改写了 cfg；只被 ApplyBuiltinBishopDefaults 调用。
func ApplyBuiltinMkDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || !isBuiltinMk(probe) {
		return false
	}
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	// 所有素材都是 800x600 的固定坐标；这里不跟随宿主窗口尺寸，
	// 否则探测到 1024x768 时整套界面会整体错位。
	cfg.Width = 800
	cfg.Height = 600
	// dm_slot 的槽位是 160x120，正好是 800x600 的 0.2；缩略图按同一比例生成。
	cfg.ThumbnailRatio = 0.2

	applyBuiltinMkTitle(cfg)
	applyBuiltinMkMessage(cfg)
	applyBuiltinMkDataScreen(cfg)
	applyBuiltinMkLog(cfg)
	applyBuiltinMkMuseum(cfg)
	applyBuiltinMkConfig(cfg)
	applyBuiltinMkMap(cfg)
	applyBuiltinMkSelect(cfg)
	return true
}

// applyBuiltinMkTitle 填标题菜单。
//
// title_menu 是 172x216 的六帧竖排图（オープニング/始めから/続きから/
// 鑑賞室/設定/終了），每一项固定画自己那一帧；title_hit 的块从
// (540,246) 起、行距 48、每块 172x36，和这里的坐标一致。
// 标题 BGM 留空：原作在进入标题前没有 BGM，由剧本自己起播。
func applyBuiltinMkTitle(cfg *BsxxConfiguration) {
	keys := []string{"op", "start", "load", "museum", "config", "quit"}
	entries := make([]TitleMenuEntry, 0, len(keys))
	for index, key := range keys {
		entries = append(entries, TitleMenuEntry{
			Key: key, Image: "title_menu", Frames: 6,
			X: 540, Y: 246 + 48*index,
			// Frame 是常态帧：六项共用一张图，不能靠悬停切帧，
			// 否则所有按钮会一起变成第 1 帧（始めから）。
			Frame: index,
		})
	}
	cfg.Title = TitlePhase{
		// 原版进标题就起播 bgm01（MKIE 的 [title phase] 同样写 bgm01）。
		BgmName:  "bgm01",
		Base:     "title_base",
		HitTable: "title_hit",
		Entries:  entries,
	}
}

// applyBuiltinMkMessage 填对话框、字体和底部工具条。
//
// message 是 800x104 的窗口：像素 alpha 为 0，只有黑色边框和右侧深色面板
// 是实体，所以必须 FillBase 铺一层底色，否则对话框整块看不见。
func applyBuiltinMkMessage(cfg *BsxxConfiguration) {
	cfg.Message = MessageWindow{
		Base: "message", BasePosSet: true, BaseX: 0, BaseY: 496,
		FillBase: true, BgColor: 0xC0182028, Speed: 100,
		// 名字/正文都避开左侧 24px 的圆角与右侧 x=718 起的工具面板。
		NameX: 108, NameY: 4, NameW: 240, NameH: 24, NameColor: 0xffffffff,
		TextX: 108, TextY: 30, TextW: 580, TextH: 66,
		// unagashi 是 32x960 的 30 帧催促动画，贴在正文区右下角。
		Unagashi: "unagashi", UnagashiX: 660, UnagashiY: 70,
		UnagashiCellW: 32, UnagashiCellH: 32,
		UnagashiColumns: 1, UnagashiFrames: 30, UnagashiSpeed: 16,
	}
	// 16 号字、行高 22，正文 66px 正好三行；640px 宽约 38 个半角字。
	cfg.Font = FontSettings{
		Size: 16, Width: 22, Height: 22, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 38, Lines: 3,
	}
	// 工具按钮坐标全来自 message_hit：左列 16..40（AUTO/SKIP/V.PLAY），
	// 48..100 是 DRESS/VIEW 两颗 52x32 的宽按钮，右列 718..798 是
	// Q.SAVE/Q.LOAD/SYSTEM/LOG。按钮都画在 message 图之上，所以 Overlay。
	// btn_config 的图写着 SYSTEM，在原作里打开存档/系统页。
	buttons := []MenuButton{
		{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 16, Y: 500}},
		{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 16, Y: 524}},
		{Key: "voice", UIButton: UIButton{Image: "btn_vplay", Frames: 4, X: 16, Y: 548}},
		{Key: "dress", UIButton: UIButton{Image: "btn_dress", Frames: 4, X: 48, Y: 504}},
		{Key: "view up", UIButton: UIButton{Image: "btn_view", Frames: 4, X: 48, Y: 536}},
		{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 718, Y: 500}},
		{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 718, Y: 524}},
		{Key: "save", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 718, Y: 548}},
		{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 718, Y: 572}},
	}
	cfg.GameMenu = GameMenu{
		Overlay:    true,
		ShowArea:   [4]int{0, 496, 800, 104},
		Persistent: false,
		Buttons:    buttons,
	}
}

// applyBuiltinMkDataScreen 填存读档页。
//
// 这一代没有 dm_base/dm_save 整屏底图：白色装饰底图就是 config_base，
// dm_hit 给出 3x3 个 160x120 槽位、顶部 10 个页码块和底部三颗导航键。
// 第 1 列（x=32）不在 dm_hit 里，但槽位本身按坐标命中，仍可正常存读。
func applyBuiltinMkDataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase: "config_base", LoadBase: "config_base", HitTable: "dm_hit",
		NoBegin: 1, PageMax: 10, SlotsPerPage: 12,
		ModeLabelBackdrop: true,
		// dm_slot 是 176x136 的三态框（常态/悬停/选中），比槽位大 16px；
		// 缩略图内缩 8px 才不会盖住边框。
		SlotFrame: "dm_slot", SlotFrameFrames: 3,
		SlotFrameHover: 1, SlotFrameSelected: 1,
		SlotThumb:     [4]int{8, 8, 160, 120},
		AutoSlotIndex: -1,
		NoData:        "dm_nodata",
		// cfg_title 是 SAVE/LOAD/CONFIG/ITEM 四个 193x30 标签，取固定帧。
		SaveMode: UIButton{Image: "cfg_title", Frames: 4, Start: 0, FixedFrame: true, X: 24, Y: 0},
		LoadMode: UIButton{Image: "cfg_title", Frames: 4, Start: 1, FixedFrame: true, X: 224, Y: 0},
		Config:   UIButton{Image: "cfg_title", Frames: 4, Start: 2, FixedFrame: true, X: 424, Y: 0},
		Item:     UIButton{Image: "cfg_title", Frames: 4, Start: 3, FixedFrame: true, X: 600, Y: 0},
		// cfg_button 的 3/4/5 帧是白字 EXIT/TITLE/QUIT；热区取 config_hit/
		// dm_hit 的三条整宽块，点按钮附近也能命中。
		Back:  UIButton{Image: "cfg_button", Frames: 6, Start: 3, FixedFrame: true, X: 67, Y: 552, Hit: [4]int{0, 543, 238, 57}},
		Title: UIButton{Image: "cfg_button", Frames: 6, Start: 4, FixedFrame: true, X: 327, Y: 552, Hit: [4]int{225, 543, 308, 57}},
		Exit:  UIButton{Image: "cfg_button", Frames: 6, Start: 5, FixedFrame: true, X: 606, Y: 552, Hit: [4]int{517, 543, 283, 57}},
	}
	// 4 列 x 3 行，列距 192、行距 152，和 dm_hit 的槽位坐标一致。
	columns := [4]int{32, 224, 416, 608}
	rows := [3]int{88, 240, 392}
	for _, y := range rows {
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// 页码 1..5 在上一行，6..10 在下一行；dm_pagenum 是 36x40 的三态块，
	// dm_number 的 12x16 数字叠在块上（PageNumberStart=1 取 1..9 帧）。
	screen.PageNumbers = UIButton{Image: "dm_number", Frames: 10}
	screen.PageNumberStart = 1
	for index := 0; index < 10; index++ {
		x := 392 + 36*index
		if index >= 5 {
			x = 584 + 36*(index-5)
		}
		screen.PageHits = append(screen.PageHits, UIPoint{X: x, Y: 32})
		screen.PageButtons = append(screen.PageButtons, UIButton{
			Image: "dm_pagenum", Frames: 3, X: x, Y: 32,
		})
		screen.PageNumberPos = append(screen.PageNumberPos, UIPoint{X: x + 12, Y: 32 + 12})
	}
	// PAGE SELECT 底板是 392x72，覆盖两排共 10 个页码块。
	screen.PageSelectFrame = UIButton{Image: "dm_pagebase", Frames: 1, X: 392, Y: 0}
	cfg.DataScreen = screen

	// SYSTEM → ITEM：cfg_item_base 是 520x424 的角色/道具面板，右侧
	// 3x5 个 64x64 格子放 cfg_item 的 14 个道具图（第 15 帧是 X）。
	item := ItemScreen{
		Base: "cfg_item_base", BaseX: 140, BaseY: 88,
		Icon: "cfg_item", IconFrames: 15,
		Selected: UIButton{Image: "dm_item", Frames: 1, X: -1, Y: -1},
		Back:     UIButton{Image: "cfg_button", Frames: 6, Start: 3, FixedFrame: true, X: 67, Y: 552, Hit: [4]int{0, 543, 238, 57}},
		Title:    UIButton{Image: "cfg_button", Frames: 6, Start: 4, FixedFrame: true, X: 327, Y: 552, Hit: [4]int{225, 543, 308, 57}},
		Exit:     UIButton{Image: "cfg_button", Frames: 6, Start: 5, FixedFrame: true, X: 606, Y: 552, Hit: [4]int{517, 543, 283, 57}},
	}
	for row := 0; row < 5; row++ {
		for column := 0; column < 3; column++ {
			item.Slots = append(item.Slots, UIPoint{X: 390 + 72*column, Y: 110 + 70*row})
		}
	}
	cfg.Item = item
}

// applyBuiltinMkLog 填回想界面：没有 kaisou 底图，用半透明黑 + 右侧滚动条。
func applyBuiltinMkLog(cfg *BsxxConfiguration) {
	cfg.Log = LogScreen{
		HasBackColor: true, BackColor: 0xcc000000,
		Lines: 8, LeftPos: 20, Width: 740,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 748, Y: 24},
		VoiceIcon: "log_vp",
	}
}

// applyBuiltinMkMuseum 填鉴赏室。
//
// museum_menu 是 202x540 的状态优先复合图：0..4 常态、5..9 悬停、
// 10..14 选中，所以页签用 FrameOffset/FrameStep 折算行号，命中用
// 202x36 的横条；页签文字已经烘焙在 museum_base 里，X=0 正好盖住。
func applyBuiltinMkMuseum(cfg *BsxxConfiguration) {
	museum := &cfg.Museum
	museum.Base = "museum_base"
	museum.MKStrip = true
	modes := []struct {
		key   string
		state uint32
		row   int
		y     int
	}{
		{"music", 1, 0, 48},
		{"cg", 2, 1, 100},
		{"scene", 4, 2, 152},
		{"strip", 8, 3, 204},
	}
	for _, item := range modes {
		museum.Modes = append(museum.Modes, MuseumMode{
			Key: item.key, State: item.state,
			UIButton: UIButton{
				Image: "museum_menu", Frames: 15, X: 0, Y: item.y,
				FrameOffset: item.row, FrameStep: 5,
				Hit: [4]int{0, item.y, 202, 36},
			},
		})
	}
	museum.Return = UIButton{
		Image: "museum_menu", Frames: 15, X: 0, Y: 256,
		FrameOffset: 4, FrameStep: 5,
		Hit: [4]int{0, 256, 202, 36},
	}

	// BGM 页：museum_titlelistbase 是右侧的曲名底板，museum_titlelist 的
	// 19 帧是烘焙好的 01..19 曲名，每行 20px；播放控制盒在底板下方，
	// 四个 62x36 按钮嵌在它的顶排空槽里，音量条是 200x12 的黄色电平条。
	music := &museum.Music
	music.Base = ""
	music.PageBase = UIButton{Image: "museum_titlelistbase", Frames: 1, X: 290, Y: 24}
	for index := 0; index < 19; index++ {
		music.Tracks = append(music.Tracks, UIButton{
			Image: "museum_titlelist", Frames: 19, FixedFrame: true,
			Start: index, X: 320, Y: 44 + 20*index,
		})
		music.Names = append(music.Names, "bgm"+twoDigits(index+1))
	}
	music.ControlBase = UIButton{Image: "museum_bgmctrl", Frames: 1, X: 290, Y: 486}
	music.Play = UIButton{Image: "museum_play", Frames: 3, X: 291, Y: 487}
	music.Stop = UIButton{Image: "museum_stop", Frames: 3, X: 355, Y: 487}
	music.Prev = UIButton{Image: "museum_prev", Frames: 3, X: 419, Y: 487}
	music.Next = UIButton{Image: "museum_next", Frames: 3, X: 483, Y: 487}
	music.Volume = UIButton{Image: "museum_volume", Frames: 1, X: 318, Y: 538}
	music.VolumeFill = true
	music.VolumeRegion = [4]int{318, 538, 200, 12}

	// CG / シーン页：4x4 格，格子 120x90，左边 270px 留给页签。
	// cg0_thumb..cg7_thumb / scene0_thumb..scene3_thumb 是预制缩略图条，
	// 每条 20 帧，直接切条比逐格合成快得多，也符合原版。
	museum.CG.Base = ""
	museum.CG.ThumbX, museum.CG.ThumbY = 270, 150
	museum.CG.ThumbW, museum.CG.ThumbH = 120, 90
	museum.CG.Thumbs = mkMuseumGrid()
	museum.CG.ThumbStrips = museumThumbStrips("cg", 8)
	museum.CG.PageButtons = mkMuseumPageButtons(8)
	museum.Scene.Base = ""
	museum.Scene.ThumbX, museum.Scene.ThumbY = 270, 150
	museum.Scene.ThumbW, museum.Scene.ThumbH = 120, 90
	museum.Scene.Thumbs = mkMuseumGrid()
	museum.Scene.ThumbStrips = museumThumbStrips("scene", 4)
	museum.Scene.PageButtons = mkMuseumPageButtons(4)

	// CG 查看器：右下角 cgm_base 是 128x48 的底盒，cgm_btn 的 8 帧是
	// 常态（0..3：前一张/关闭/后一张/CFG）与红色悬停（4..7）。
	museum.Viewer = CGViewer{
		Base:   UIButton{Image: "cgm_base", Frames: 1},
		PanelX: 652, PanelY: 540,
		Prev:       UIButton{Image: "cgm_btn", Frames: 8, Start: 0, X: 660, Y: 552},
		Exit:       UIButton{Image: "cgm_btn", Frames: 8, Start: 1, X: 684, Y: 552},
		Next:       UIButton{Image: "cgm_btn", Frames: 8, Start: 2, X: 708, Y: 552},
		Config:     UIButton{Image: "cgm_btn", Frames: 8, Start: 3, X: 732, Y: 552},
		ButtonBase: 0, HoverOffset: 4, ToggleHoverOffset: 4,
	}

	// 立ち絵鉴赏：六张 72x528 的整身立绘并排，页签与返回键叠在左侧。
	for index := 0; index < 6; index++ {
		museum.StripPortraits = append(museum.StripPortraits, UIButton{
			Image: "museum_ch" + itoa(index+1), Frames: 1,
			X: 270 + 72*index, Y: 40,
		})
	}
}

// mkMuseumGrid 是 CG/シーン页的 4x4 缩略图网格：列距 128（120 图 + 8 间隙），
// 行距 98（90 图 + 8 间隙），从 (270,150) 起排，避开左侧页签。
func mkMuseumGrid() []UIPoint {
	points := make([]UIPoint, 0, 16)
	for row := 0; row < 4; row++ {
		for column := 0; column < 4; column++ {
			points = append(points, UIPoint{X: 270 + 128*column, Y: 150 + 98*row})
		}
	}
	return points
}

// mkMuseumPageButtons 生成底部一排页码：museum_page 是 25x25 的三帧块，
// 页数不多时按 28px 间距排开即可。
func mkMuseumPageButtons(count int) []UIButton {
	buttons := make([]UIButton, 0, count)
	for index := 0; index < count; index++ {
		buttons = append(buttons, UIButton{
			Image: "museum_page", Frames: 3,
			X: 270 + 28*index, Y: 552,
		})
	}
	return buttons
}

// applyBuiltinMkConfig 填旧式设置画面。
//
// cfg_ex 是 600x404 的设置内容，画在 (182,98)；config_base 是整屏白色装饰底图。
// 这个偏移不是居中估算：把 cfg_ex 放到 (182,98) 后，config_hit 的每个热区
// （全画面 324,128 / 音声ループ 196,252 / 六个角色头像 204,276… /
// 重置 188,460）都逐像素落在 cfg_ex 的烘焙标签和控件上。
// 控件坐标一律是屏幕绝对坐标，不随 BaseX/BaseY 变化。
func applyBuiltinMkConfig(cfg *BsxxConfiguration) {
	layout := LegacyConfigScreen{
		Base:     "cfg_ex",
		HitTable: "config_hit",
		Backdrop: "config_base",
		BaseX:    182,
		BaseY:    98,
		// cfg_mark 是 15x45 的三帧（方框/圆点/菱形），勾选用方框、
		// 画面効果单选画圆点、スキップ処理画菱形。
		CheckMark:       "cfg_mark",
		CheckMarkFrames: 3,
		RadioButton:     "cfg_mark",
		RadioFrames:     3,
		// 全画面表示；与「画面効果」第一项在同一列，y 相隔 20px。
		ScreenMode: UIPoint{X: 326, Y: 128},
		// 音声をループする，位于 VOICE 面板顶部。
		VoiceLoop: UIPoint{X: 196, Y: 252},
		// MESSAGE 面板：未読もスキップ + 三个スキップ処理选项。
		SkipUnread:        UIPoint{X: 601, Y: 168},
		SkipModeMarkFrame: 2,
		SkipModes: []UIPoint{
			{X: 601, Y: 188}, {X: 601, Y: 208}, {X: 601, Y: 228},
		},
	}
	// 画面効果：なし / コマ数固定 / 時間固定。空心圆底图配圆点帧。
	layout.RadioGroups = []LegacyRadioGroup{
		{Key: "screen effect", MarkFrame: 1, Points: []UIPoint{
			{X: 326, Y: 148}, {X: 326, Y: 168}, {X: 326, Y: 188},
		}},
	}
	// メッセージ速度 / オート速度：cfg_ex 画好 10 个刻度点，config_hit 的
	// 10 个 16x16 热区在 (592..751, 128/148)。用离散 Dots（菱形标记）而不是
	// MarkerOnly：两行只隔 20px，MarkerOnly 的 32px 命中带会互相压住。
	layout.Speeds = []LegacyGauge{
		{Dots: mkSpeedDots(128), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
		{Dots: mkSpeedDots(148), DotMark: UIButton{Image: "cfg_mark", Frames: 3}},
	}
	// BACK GROUND 四条色条：赤/緑/青/濃度，cfg_meter 的第 1..4 帧正好是
	// 红/绿/蓝/灰，按值裁宽度填进底图空槽。
	layout.Colors = make([]LegacyGauge, 4)
	for index := 0; index < 4; index++ {
		layout.Colors[index] = LegacyGauge{
			Bar: UIButton{
				Image: "cfg_meter", Frames: 5, X: 512, Y: 308 + 24*index,
				Start: index + 1, FixedFrame: true,
			},
			Fill: true, TrackWidth: 258,
		}
	}
	// VOLUME 四条：烘焙标签的顺序是 BGM / 環境音 / 音声 / 効果音，
	// 而引擎通道下标是 1=BGM、2=SE、3=BGS(環境音)、4=VOICE(音声)，
	// 所以按标签顺序取 1,3,4,2，不能简单写 1..4。
	layout.Volumes = make([]LegacyGauge, 5)
	volumeRows := []struct {
		channel int
		y       int
	}{{1, 440}, {3, 457}, {4, 474}, {2, 491}}
	for _, row := range volumeRows {
		layout.Volumes[row.channel] = LegacyGauge{
			Bar: UIButton{
				Image: "cfg_meter", Frames: 5, X: 512, Y: row.y,
				Start: 0, FixedFrame: true,
			},
			Fill: true, TrackWidth: 258,
		}
	}
	// 六个角色头像 64x64（config_hit 的 3x2 块）。
	characterPoints := []UIPoint{
		{X: 204, Y: 276}, {X: 276, Y: 276}, {X: 348, Y: 276},
		{X: 204, Y: 348}, {X: 276, Y: 348}, {X: 348, Y: 348},
	}
	for index := 0; index < 6; index++ {
		layout.Characters = append(layout.Characters, LegacyCharacter{
			Button: UIButton{
				Image: "cfg_ch" + itoa(index+1), Frames: 1,
				X: characterPoints[index].X, Y: characterPoints[index].Y,
			},
		})
	}
	// 底部三颗导航键：cfg_button 的 3/4/5 帧是白字，热区取 config_hit 的
	// 三条整宽块（与存读档页共用同一套底栏）。
	layout.Back = UIButton{Image: "cfg_button", Frames: 6, Start: 3, FixedFrame: true, X: 67, Y: 552, Hit: [4]int{0, 543, 238, 57}}
	layout.Title = UIButton{Image: "cfg_button", Frames: 6, Start: 4, FixedFrame: true, X: 327, Y: 552, Hit: [4]int{225, 543, 308, 57}}
	layout.Exit = UIButton{Image: "cfg_button", Frames: 6, Start: 5, FixedFrame: true, X: 606, Y: 552, Hit: [4]int{517, 543, 283, 57}}
	// cfg_reset 是 188x36 的两帧文字（常态/悬停），叠在 cfg_ex 烘焙好的
	// 暗红按钮条上；热区比文字宽，和 config_hit 的 240x40 一致。
	layout.Reset = UIButton{
		Image: "cfg_reset", Frames: 2, X: 188, Y: 460,
		Hit: [4]int{188, 460, 240, 40},
	}
	cfg.LegacyConfig = layout
}

// mkSpeedDots 返回一行 10 个刻度点（x=592..751，16px 一格）。
func mkSpeedDots(y int) []UIPoint {
	points := make([]UIPoint, 0, 10)
	for index := 0; index < 10; index++ {
		points = append(points, UIPoint{X: 592 + 16*index, Y: y})
	}
	return points
}

// applyBuiltinMkMap 填地图阶段：背景 + 周数/曜日/时段数字条 + 五颗系统按钮。
//
// map_base 是纯航拍照片，左上角的周数/曜日/时段卡由数字条叠成；
// map_hit 的 (448,528) 块也正好是 map_nexttime 的 104x64 命中区。
func applyBuiltinMkMap(cfg *BsxxConfiguration) {
	cfg.MapPhase = MapPhase{
		Background: "map_base",
		HitImage:   "map_hit",
		WeekNo:     DigitStrip{Image: "map_week_no", Frames: 6, X: 10, Y: 10},
		WeekDay:    DigitStrip{Image: "map_week", Frames: 7, X: 10, Y: 36},
		// 剧本的 #day 从 0 起，而原作第一天是火曜日；map_week 从 MONDAY
		// 开始，所以曜日帧要 +1，否则每天都会显示成前一天。
		WeekDayShift: 1,
		TimeZone:     DigitStrip{Image: "map_time", Frames: 4, X: 20, Y: 92},
		DayVariable:  "#day",
		TimeVariable: "#time",
		// 按钮由 BsMapProfileMkLegacy 按 map_hit 的精确命中块补，避免重复进按钮表。
	}
}

// applyBuiltinMkSelect 填选择肢窗口。
//
// item0 是 520x64（两帧 520x32），加上 520x32 的 head/foot 装饰，
// 一块选项正好 96px 高；DatumLine 363 与其它旧作一致。
func applyBuiltinMkSelect(cfg *BsxxConfiguration) {
	cfg.SelectWindow = NewSelectWindowConfiguration(363, 1, []SelectDesign{
		{
			Item:         "item0",
			Interval:     4,
			TextX:        0,
			TextY:        16,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0x88ffffff,
			SelectColor:  0xffff4488,
			HeaderFooter: true,
		},
	})
}
