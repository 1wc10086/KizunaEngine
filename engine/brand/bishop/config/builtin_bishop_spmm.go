package config

// builtin_bishop_spmm.go 给「スレイブポリス みちる＆美鈴」（Slave Police，
// Bishop 2005，BSScript Rev.3，DISK1）补一套内置布局。
//
// 这部作品发行目录里没有 bsxx.cfg：标题、对话框、工具条、存读档页、
// 设置页和鉴赏室的坐标都编译在 spmm.exe 里。只按 bsxx.cfg 解析会
// 得到一份全空配置——进游戏没有标题菜单、没有对话框、工具条
// 全部消失。现有内置布局的素材签名都匹配不上这一代，所以这里
// 单独给它一套指纹与坐标；新字段（DataScreen.Backdrop/MenuSave/
// MenuLoad/MenuConfig、LegacyConfigScreen.Menu、SelectDesign.
// SelectedItem）为它而加，其它作品保持零值、不受影响。
//
// 坐标来源（可核对）：
//   - 标题：title_hit 的命中块给出六个菜单项的位置；title_menu 是
//     单张六帧竖排图（オープニング/始めから/続きから/鑑賞室/設定/
//     終了），帧号 = 菜单项下标，见 title_hit 与 spmm.exe 初始化;
//   - 对话框与工具按钮：spmm.exe 初始化函数的按钮注册段
//     （0x41b681-0x41b7cf）直接给出消息框原点 (60,468) 与工具按钮
//     的绝对坐标；message/title_menu 的图头尺寸见下述签名检查;
//   - 存读档/设置：dm_hit / config_hit 的色块矩形与 save_frame/
//     load_frame / config_frame 的底图烘焙标签位置逐一对应;
//   - 鉴赏室：museum_menu 的页签行与 museum_pageline / museum_page
//     的底栏。
//
// 只在"配置确实为空 + 素材签名吻合"时生效；签名用绝大多数作品没有
// 的组合（btn_hide/btn_up 等独立按钮图 + config_menu + museum 系列
// 全套）加上两个图头尺寸的精确匹配，避免把别的旧作误判进来。

// builtinSpmmAssets 是 スレイブポリス 的素材签名。
//
// 标题/对话框/工具条/存读档/设置/鉴赏室的全部素材必须同时存在，
// 其中 btn_hide、btn_up、config_menu、config_circur、museum_pageline
// 在其它世代基本没有，组合在一起只会在这一代发行版里成立。
var builtinSpmmAssets = []string{
	"title_base", "title_hit", "title_menu",
	"message", "message_hit",
	"unagashi", "next_select",
	"config_base", "config_frame", "config_menu",
	"config_hit", "config_check", "config_circur", "cfg_meter",
	"save_frame", "save_menu", "load_frame", "load_menu", "dm_hit",
	"museum_base", "museum_menu", "museum_page",
	"museum_num", "museum_pageline",
	"item0", "item_slct0",
	"btn_auto", "btn_skip", "btn_hide",
	"btn_qsave", "btn_qload",
	"btn_save", "btn_load", "btn_config",
	"btn_voice", "btn_log", "btn_up",
}

// builtinSpmmExcluded 是其它世代独有的素材。
//
// save_base/load_base（学園 GK / School Heaven）、gamemenu（AFDVD /
// 现代作品）、kaisou（KD 回想）、dm_slotbase（Himedorei/MK/MK2）任一
// 出现都说明不是这一代；title_menu_op/title_menu_begin 是分图标题
// 的世代，message_back 是 School Heaven 的窗口掩码。
var builtinSpmmExcluded = []string{
	"message_back", "save_base", "load_base", "msg_base",
	"title_menu_op", "title_menu_begin",
	"dm_slotbase", "kaisou", "gamemenu",
}

func isBuiltinSpmm(probe BuiltinProbe) bool {
	if probe.Exists == nil {
		return false
	}
	for _, name := range builtinSpmmAssets {
		if !probe.Exists(name) {
			return false
		}
	}
	for _, name := range builtinSpmmExcluded {
		if probe.Exists(name) {
			return false
		}
	}
	// 仅靠素材名还不够：message 680x128 与 title_menu 236x174 是
	// 这一代的窗口/按钮尺寸。其它 Rev.3 作品也可能带同名图，
	// 用图头尺寸把真正的 スレイブポリス 钉死；没有 Header 探测
	// 能力时不要凭名字猜（Header == nil 直接不命中）。
	if probe.Header == nil {
		return false
	}
	menu, ok := probe.Header("title_menu")
	if !ok || menu.Width != 236 || menu.Height != 174 {
		return false
	}
	message, ok := probe.Header("message")
	if !ok || message.Width != 680 || message.Height != 128 {
		return false
	}
	return true
}

// ApplyBuiltinSpmmDefaults 在配置为空、素材是 スレイブポリス 时填好
// 整套界面：标题、对话框+字体、工具条、选择肢、存读档页、设置页、
// 鉴赏室和回想。返回 true 表示改写了 cfg；只被
// ApplyBuiltinBishopDefaults 调用，且必须在其它内置检查之前。
func ApplyBuiltinSpmmDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || !isBuiltinSpmm(probe) {
		return false
	}
	// 已有配置的作品一律不动：哪怕只有一个标题或对话框，混入
	// 内置坐标也会把本来正确的画面画歪。
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	// 这一代全部素材按 800x600 编译进 exe；不要跟随宿主窗口尺寸，
	// 否则探测到 1024x768 时整套界面会整体错位。
	cfg.Width = 800
	cfg.Height = 600
	// 槽位缩略图 160x120 正好是 800x600 的 0.2，与 save_frame 里
	// 烘焙的缩略图框一致。
	cfg.ThumbnailRatio = 0.2

	applySpmmTitle(cfg)
	applySpmmMessage(cfg)
	applySpmmGameMenu(cfg)
	applySpmmSelect(cfg)
	applySpmmDataScreen(cfg)
	applySpmmConfig(cfg)
	applySpmmMuseum(cfg)
	applySpmmLog(cfg)
	return true
}

// applySpmmTitle 填标题菜单。
//
// title_menu 是 236x174 的单张六帧竖排图（每帧 236x29)：
// frame 0 オープニング、1 始めから、2 続きから、3 鑑賞室、
// 4 設定、5 終了。底图 title_base 已经把菜单文字烘焙进去，
// title_menu 是"高亮版本"；原引擎平时只画底图，悬停时才把
// 对应帧叠上去，所以用 HoverOnly + Frame 显式指定帧号，
// 不悬停时不画，悬停时只画该行，否则六行会同时变色。
// BGM 由剧本自己播（标题标签之前 PLAY_BGM），留空表示沿用。
func applySpmmTitle(cfg *BsxxConfiguration) {
	keys := []string{"op", "start", "load", "museum", "config", "quit"}
	entries := make([]TitleMenuEntry, 0, len(keys))
	for index, key := range keys {
		entries = append(entries, TitleMenuEntry{
			Key: key, Image: "title_menu", Frames: 6, Frame: index,
			X: 40, Y: 403 + 29*index, HoverOnly: true,
		})
	}
	cfg.Title = TitlePhase{
		Base:     "title_base",
		HitTable: "title_hit",
		// spmm.exe 的标题初始化把 bgm01 压进 BGM 槽；剧本 boot 也播同一首，
		// 这里写死保证从读档/鉴赏回标题时音乐不会空掉。
		BgmName: "bgm01",
		Entries: entries,
	}
}

// applySpmmMessage 填对话框与字体。
//
// message 是 680x128 的窗口框，原引擎把它贴在屏幕底部：
// 600-128=472 的方形框在这里实际落在 (60,468)，框的右下角让出
// 工具按钮。spmm.exe 初始化函数（看见"消息窗初始化"段）把
// 名字/正文区域与催促图标的位置也直接编译进去：名字在右上，
// 正文 536x86，unagashi 催促图标在右下角。字体 17 号、行高 24、
// 正文区 86px 正好放 3 行。
func applySpmmMessage(cfg *BsxxConfiguration) {
	cfg.Message = MessageWindow{
		Base:       "message",
		BasePosSet: true,
		BaseX:      60,
		BaseY:      468,
		Speed:      100,
		BgColor:    0,
		// 名字区在窗口右上：黑字压在亮色底图上，用默认白字，
		// 与原版 spmm.exe 的固定白色一致。
		NameX: 24, NameY: 6, NameW: 190, NameH: 22, NameColor: 0xffffffff,
		TextX: 32, TextY: 36, TextW: 536, TextH: 86,
		TextColor: 0xffffffff, TextColorSet: true,
		// unagashi 是 32x256 的 8 帧催促动画（右下角）。
		Unagashi:        "unagashi",
		UnagashiX:       546,
		UnagashiY:       95,
		UnagashiCellW:   32,
		UnagashiCellH:   32,
		UnagashiColumns: 1,
		UnagashiFrames:  8,
		UnagashiSpeed:   80,
	}
	// 正文区 86px 高：17 号字、行高 24，三行占 72px，剩 14px 给
	// 下边框。536px 约 24 个全角字（LineLength 与之对应）。
	cfg.Font = FontSettings{
		Size: 17, Width: 22, Height: 24, Weight: 1000,
		Edge: 1, Shadow: 1, LineLength: 24, Lines: 3,
	}
}

// applySpmmGameMenu 填对话框右侧的工具按钮。
//
// 按钮坐标全部是屏幕绝对坐标，来源是 spmm.exe 消息按钮初始化
// （文件偏移 0x41b681-0x41b7cf 的按钮注册段）：每颗按钮 24x21，
// 三列（x=644/672/700）x 顶部行（y=501/524/547/570）。
// btn_* 是 24x84 的 4 帧竖排图（常态/悬停/按下/不可用）。
// 键名与 [system button] 一致：hide 是菜单显示切换，up 是
// 回想上翻，change（btn_change）是换装。没有单独的菜单条底图：
// 按钮直接画在 message 窗口框上，ShowArea 即窗口矩形。
func applySpmmGameMenu(cfg *BsxxConfiguration) {
	button := func(key, image string, x, y int) MenuButton {
		return MenuButton{Key: key, UIButton: UIButton{Image: image, Frames: 4, X: x, Y: y}}
	}
	cfg.GameMenu = GameMenu{
		Base:     "",
		BaseX:    60,
		BaseY:    468,
		ShowArea: [4]int{60, 468, 680, 128},
		Buttons: []MenuButton{
			button("skip", "btn_skip", 644, 501),
			button("auto ex", "btn_auto", 672, 501),
			button("menu display", "btn_hide", 700, 501),
			button("quick save", "btn_qsave", 644, 524),
			button("quick load", "btn_qload", 672, 524),
			button("config", "btn_config", 700, 524),
			button("save", "btn_save", 644, 547),
			button("load", "btn_load", 672, 547),
			button("dress", "btn_change", 700, 547),
			button("voice", "btn_voice", 644, 570),
			button("log", "btn_log", 672, 570),
			button("log", "btn_up", 700, 570),
		},
	}
}

// applySpmmSelect 填选择肢窗口。
//
// item0 是 470x64 的 2 帧选项条（常态/悬停）；item_slct0 是
// 同尺寸的选中态。文字在条内居中：470 宽取一半 235，高度 64
// 取 5px 让文字落在条形中央。选中颜色与默认同为白色，区分
// 靠底色（命中悬停）而非文字颜色，与原版一致。
func applySpmmSelect(cfg *BsxxConfiguration) {
	cfg.SelectWindow = NewSelectWindowConfiguration(300, 1, []SelectDesign{
		{
			Item:         "item0",
			Frames:       2,
			SelectedItem: "item_slct0",
			TextX:        235,
			TextY:        5,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0x88ffffff,
			SelectColor:  0xffffffff,
		},
	})
}

// applySpmmDataScreen 填存读档页。
//
// 存读档页没有 save_base/load_base 整屏底图：save_frame/load_frame
// 是槽位框（save_menu/load_menu 是底部页签），config_base 充当
// 整屏背景同时垫在存档/读档/设置三页下面，dm_hit 给出槽位与
// 页签的命中块。槽位是 3 列 x 2 行：x=216/408/600，y=100/336；
// 槽内缩略图 160x120 相对槽位是 (10,10)，文字行在 140px 处，
// 行距 24，深灰字。
func applySpmmDataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		Backdrop: "config_base",
		SaveBase: "save_frame",
		LoadBase: "load_frame",
		MenuSave: "save_menu",
		MenuLoad: "load_menu",
		HitTable: "dm_hit",
		NoBegin:  1,
		PageMax:  1,
		// 这一代没有自动存档格：第 1 格就是普通手动档。DataScreen 的
		// AutoSlotIndex 零值是 0，必须显式写 -1，否则第一格会被当成
		// AUTO SAVE ONLY 而拒绝手动保存。
		AutoSlotIndex:   -1,
		SlotSize:        [2]int{180, 220},
		SlotThumb:       [4]int{10, 10, 160, 120},
		SlotTextX:       14,
		SlotTextY:       140,
		SlotTextLineGap: 24,
		SlotTextColor:   0xff202020,

		SaveMode: spmmHitButton("", 32, 64, 136, 16),
		LoadMode: spmmHitButton("", 32, 93, 136, 16),
		Config:   spmmHitButton("", 32, 123, 136, 16),
		Back:     spmmHitButton("", 16, 560, 168, 40),
		Title:    spmmHitButton("", 592, 576, 112, 16),
		Exit:     spmmHitButton("", 720, 576, 72, 16),
	}
	for _, y := range []int{100, 336} {
		for _, x := range []int{216, 408, 600} {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	cfg.DataScreen = screen
}

// spmmHitButton 生成"只有命中矩形、画在底图上"的按钮。
//
// 存档/设置页底部的页签与 BACK/TITLE/EXIT 全部烘焙在底图里
// (config_base / save_menu / load_menu / config_menu)，引擎只需
// 登记 dm_hit / config_hit 给出的热区；image 为空表示不独立
// 画图，X/Y 只是命中矩形的左上角。
func spmmHitButton(image string, x, y, w, h int) UIButton {
	return UIButton{Image: image, Frames: 1, X: x, Y: y, Hit: [4]int{x, y, w, h}}
}

// applySpmmConfig 填旧式设置画面。
//
// config_frame 是 800x600 的整屏设置底图（标签/滑杆槽全部烘焙），
// config_base 垫底，config_menu 是顶部的 SAVE/LOAD/CONFIG 页签，
// config_hit 给出全部控件的命中块。单选标记用 config_circur
// （空心圆），复选标记用 config_check（圆圈对勾），都是单帧。
// 控件位置按 config_hit 的色块包围盒取：速度刻度 x=470..678
// （10 格不均：前 9 格 23px、末格 24px，与色块对齐），音量条
// 高 18px 的横条。
func applySpmmConfig(cfg *BsxxConfiguration) {
	layout := LegacyConfigScreen{
		Base:            "config_frame",
		Backdrop:        "config_base",
		Menu:            "config_menu",
		HitTable:        "config_hit",
		CheckMark:       "config_check",
		RadioButton:     "config_circur",
		CheckMarkFrames: 1,
		RadioFrames:     1,
		ScreenMode:      UIPoint{X: 441, Y: 72},
		SkipUnread:      UIPoint{X: 441, Y: 241},
		VoiceLoop:       UIPoint{X: 441, Y: 282},
		Back:            spmmHitButton("", 16, 560, 168, 40),
		Reset:           spmmHitButton("", 504, 576, 72, 16),
		Title:           spmmHitButton("", 592, 576, 112, 16),
		Exit:            spmmHitButton("", 720, 576, 72, 16),
		ModeSave:        spmmHitButton("", 32, 64, 136, 16),
		ModeLoad:        spmmHitButton("", 32, 93, 136, 16),
		ModeConfig:      spmmHitButton("", 32, 123, 136, 16),
		RadioGroups: []LegacyRadioGroup{
			{Key: "screen effect", MarkFrame: 0, Points: []UIPoint{
				{X: 447, Y: 97}, {X: 503, Y: 97}, {X: 615, Y: 97},
			}},
			{Key: "skip mode", MarkFrame: 0, Points: []UIPoint{
				{X: 447, Y: 209}, {X: 513, Y: 209}, {X: 580, Y: 209},
			}},
		},
		Speeds: []LegacyGauge{
			spmmDotGauge(152),
			spmmDotGauge(181),
		},
		ExtraToggles: []LegacyToggle{
			{Key: "character voice 0", X: 441, Y: 312},
			{Key: "character voice 1", X: 441, Y: 343},
			{Key: "character voice 2", X: 441, Y: 373},
		},
	}
	// Volumes 顺序固定为 master/BGM/SE/BGS/音声/BGV/减衰率；
	// 本作只暴露 BGM/SE/音声/BGV 四条，其余留空占位。
	layout.Volumes = make([]LegacyGauge, 7)
	layout.Volumes[1] = spmmVolumeGauge(430)
	layout.Volumes[2] = spmmVolumeGauge(520)
	layout.Volumes[3] = spmmVolumeGauge(460)
	layout.Volumes[4] = spmmVolumeGauge(490)
	cfg.LegacyConfig = layout
}

// spmmDotGauge 生成一行 10 格速度刻度。刻度 x 序列取自
// config_hit 的色块：470,493,516,539,562,585,608,631,654,678
// （前 9 格 23px、末格 24px），标记用 config_circur 单帧。
func spmmDotGauge(y int) LegacyGauge {
	xs := []int{470, 493, 516, 539, 562, 585, 608, 631, 654, 678}
	dots := make([]UIPoint, 0, len(xs))
	for _, x := range xs {
		dots = append(dots, UIPoint{X: x, Y: y})
	}
	return LegacyGauge{Dots: dots, DotMark: UIButton{Image: "config_circur", Frames: 1}}
}

// spmmVolumeGauge 生成一条音量填充条：底图已经画好 `[0% +----+ 100%]`
// 的轨道，cfg_meter 是 188x18 的蓝色渐变填充段。引擎按当前值裁出左侧
// 一段贴上去，比只画一根竖线更接近原版的"条状音量表"。
func spmmVolumeGauge(y int) LegacyGauge {
	return LegacyGauge{
		Bar:  UIButton{Image: "cfg_meter", Frames: 1, X: 452, Y: y},
		Fill: true,
	}
}

// applySpmmMuseum 填鉴赏室。
//
// museum_base 是整屏底图。页签是 museum_menu 的 8 帧图：row 0..3
// 是 CG/シーン/立ち絵/戻る 四个标签，帧号 = row*4 + 状态（0 常态、
// 1 选中、2 悬停、3 不可用）。用 Clip 裁出每行 136x24 的标签，
// X=8、Y=24，页签与第 4 行"戻る"都画在左侧栏。cg/scene/strip
// 切换由 ActiveFrame=1 的选中帧完成；return 行 row 3 走 Return。
//
// 缩略图是预制条 cgN_thumb/sceneN_thumb（每帧 120x90）：CG 页
// 5 张条、20 格（5x4），场景页 2 张条、20 格。页码用 museum_page 的
// 3 帧图标，CG 页 5 页、场景页 2 页；达成率数字条 museum_num 是
// 12 帧（0..9 与 %），放在 museum_pageline 的右侧。
func applySpmmMuseum(cfg *BsxxConfiguration) {
	tab := func(row int) UIButton {
		return UIButton{
			Image: "museum_menu", Frames: 8,
			Clip:        [4]int{0, 2, 136, 24},
			FrameOffset: row, FrameStep: 4,
			// 每个页签的落点必须按行下移：Clip 只截取源图的一行，
			// Y 决定这行画到屏幕哪里。三个页签 + 戻る共用同一张图，
			// Y 全写 24 会让它们叠在 MENU 标题下面互相覆盖。
			X: 8, Y: 24 + 24*row,
		}
	}
	cgPage := MuseumCGPage{
		Base:         "museum_base",
		PageBase:     UIButton{Image: "museum_pageline", Frames: 1, X: 200, Y: 526},
		Thumbs:       spmmMuseumGrid(),
		ThumbX:       200,
		ThumbY:       84,
		ThumbW:       120,
		ThumbH:       90,
		ThumbStrips:  []string{"cg0_thumb", "cg1_thumb", "cg2_thumb", "cg3_thumb", "cg4_thumb"},
		Percent:      UIButton{Image: "museum_num", Frames: 12, X: 700, Y: 532},
		PercentSpace: -20,
	}
	for index := 0; index < 5; index++ {
		cgPage.PageButtons = append(cgPage.PageButtons, UIButton{
			Image: "museum_page", Frames: 3, X: 430 + 30*index, Y: 532,
		})
	}
	scenePage := MuseumScenePage{
		Base:         "museum_base",
		PageBase:     UIButton{Image: "museum_pageline", Frames: 1, X: 200, Y: 526},
		Thumbs:       spmmMuseumGrid(),
		ThumbX:       200,
		ThumbY:       84,
		ThumbW:       120,
		ThumbH:       90,
		ThumbStrips:  []string{"scene0_thumb", "scene1_thumb"},
		Percent:      UIButton{Image: "museum_num", Frames: 12, X: 700, Y: 532},
		PercentSpace: -20,
	}
	for index := 0; index < 2; index++ {
		scenePage.PageButtons = append(scenePage.PageButtons, UIButton{
			Image: "museum_page", Frames: 3, X: 430 + 30*index, Y: 532,
		})
	}
	cfg.Museum = Museum{
		Base: "museum_base",
		Modes: []MuseumMode{
			{Key: "cg", ActiveFrame: 1, UIButton: tab(0)},
			{Key: "scene", ActiveFrame: 1, UIButton: tab(1)},
			{Key: "strip", ActiveFrame: 1, UIButton: tab(2)},
		},
		Return: tab(3),
		StripPortraits: []UIButton{
			{Image: "museum_ch1", Frames: 1, X: 240, Y: 0},
			{Image: "museum_ch2", Frames: 1, X: 320, Y: 0},
			{Image: "museum_ch3", Frames: 1, X: 400, Y: 0},
			{Image: "museum_ch4", Frames: 1, X: 480, Y: 0},
		},
		CG:    cgPage,
		Scene: scenePage,
	}
}

// spmmMuseumGrid 返回鉴赏室缩略图的 5x4 网格。
//
// 起点 (200,84)，列距 120、行距 90，恰好铺满五列四行（与
// cgN_thumb / sceneN_thumb 的单帧 120x90 对齐）。
func spmmMuseumGrid() []UIPoint {
	points := make([]UIPoint, 0, 20)
	for row := 0; row < 4; row++ {
		for column := 0; column < 5; column++ {
			points = append(points, UIPoint{X: 200 + 120*column, Y: 84 + 90*row})
		}
	}
	return points
}

// applySpmmLog 填回想界面。
//
// 这一代没有 kaisou 整屏底图：回想在剧情帧上盖一层半透明黑色
// (0xc0000000)，只有右侧的滚动条（log_meter）与上下箭头
// (log_uarrow/log_darrow) 是独立素材，语音图标是 log_vp。
func applySpmmLog(cfg *BsxxConfiguration) {
	cfg.Log = LogScreen{
		HasBackColor: true,
		BackColor:    0xc0000000,
		Lines:        4,
		LeftPos:      120,
		Width:        560,
		Bar:          UIButton{Image: "log_meter", Frames: 1, X: 740, Y: 24},
		Up:           UIButton{Image: "log_uarrow", Frames: 1, X: 740, Y: 0},
		Down:         UIButton{Image: "log_darrow", Frames: 1, X: 740, Y: 576},
		VoiceIcon:    "log_vp",
	}
}
