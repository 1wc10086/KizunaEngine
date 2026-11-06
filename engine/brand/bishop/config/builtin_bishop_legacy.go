package config

// builtin_bishop_legacy.go 给"配置为空、但资源是旧式 BSScript 作品"的游戏补内置布局。
//
// 背景：BSScript Rev.2–Rev.6 时代的作品（KD 毎日けだものっ!! 等）把标题菜单、
// 对话框、工具条、存读档页的布局直接编译在引擎 exe 里，发行目录里没有
// bsxx.cfg。只按 bsxx.cfg 解析会得到一份全空配置：没有标题（一进游戏就落进
// opening）、没有消息框（只剩宿主 Flutter 的对话框）、没有存读档热区。
//
// 这些坐标来自两处可核对的数据：
//   - 标题/消息框/工具条：kd.exe 的初始化函数（0x444000 附近、0x4238b0 附近）；
//   - 存读档页/回想页：游戏资源里的 dm_hit / dm_save / dm_load / kaisou。
//
// 该函数只在"配置确实为空 + 资源签名吻合"时生效，已有配置的现代作品不受影响。

// BuiltinProbe 是检测内置布局所需的只读环境。
type BuiltinProbe struct {
	// Exists 判断资源索引里是否有这张图（不含扩展名，例如 title_base）。
	Exists func(name string) bool
	// RevScript 表示剧本是 BSScript Rev.2–Rev.6。
	RevScript bool
	// AncientScript 表示剧本是 BSScript（Ancient 布局，无 Rev 号）。
	// MDDVD 这一代没有 bsxx.cfg，内置布局只能按 Ancient 签名 + 素材指纹
	// 判断；Rev 作品保持 RevScript=true、这里为 false。
	AncientScript bool
	// Width/Height 是屏幕探测得到的尺寸；<=0 时按 800x600 处理。
	Width  int
	Height int
	// Header 读取 BSG 图头的尺寸/偏移/帧数，供"从素材本身推导布局"的
	// 通用兜底使用；不需要时留空。
	Header func(name string) (BuiltinImage, bool)
}

// builtinBishopAssets 是这代作品的资源签名。
//
// 不能只判断 title_base：现代作品也有同名图。这里要求标题、消息框、
// 工具条、存读档页和回想的素材同时存在，基本只会在同一代引擎的发行版里成立。
var builtinBishopAssets = []string{
	"title_base", "title_hit",
	"title_menu_op", "title_menu_begin", "title_menu_load",
	"title_menu_museum", "title_menu_setting", "title_menu_quit",
	"message", "message_hit", "unagashi",
	"btn_voice", "btn_skip",
	"dm_save", "dm_load", "dm_hit",
	"kaisou",
}

// ApplyBuiltinBishopDefaults 在作品没有配置、但资源符合旧式 BSScript 作品时，
// 填好标题、消息框、工具条、选择肢、存读档页和回想页。只填空缺项。
func ApplyBuiltinBishopDefaults(cfg *BsxxConfiguration, probe BuiltinProbe) bool {
	if cfg == nil || !probe.RevScript || probe.Exists == nil {
		return false
	}
	// 已有配置的作品（哪怕只有一个标题或对话框）一律不动：不同作品的
	// 按钮坐标差异很大，混入内置值会把本来正确的画面画歪。
	if !cfg.Title.Empty() || !cfg.Message.Empty() {
		return false
	}
	// スレイブポリス みちる＆美鈴（SPMM，Rev.3）是更弱的旧一代：
	// 素材签名是"单张 6 帧 title_menu + 680x128 message +
	// btn_hide/btn_up 独立按钮 + config_menu/config_circur +
	// save_frame/load_frame + museum_pageline"，且 title_menu 236x174、
	// message 680x128 的图头尺寸精确匹配。它与 School Heaven
	// （save_base/load_base）互斥，但和 KD 的通用签名（title_menu_op）
	// 不冲突；放在最前面判，避免任何更宽的通用检查把它吞掉。
	// 只在 probe.RevScript 且 Title/Message 都为空时生效。
	if isBuiltinSpmm(probe) {
		if ApplyBuiltinSpmmDefaults(cfg, probe) {
			return true
		}
	}
	// ヒメドレイ（2007，Rev.6）与 MK/AFDVD 同为旧式素材，但标题/对话框/
	// 存读档/设置/鉴赏室都是自己的一套；指纹包含 dm_slot + dm_slotbase +
	// cfg_ex + museum_menu，且排除 dm_base/kaisou/gamemenu。放在最前面，
	// 免得掉进 KD 的通用检查后整体放弃。
	if isBuiltinHimedorei(probe) {
		if ApplyBuiltinHimedoreiDefaults(cfg, probe) {
			return true
		}
	}
	// 放課後2（HOUKAGO2，Rev.5）的素材签名是"单张六帧 title_menu + 800x132
	// message + msg_name 名牌 + dm_slotbase + cfg_ex + map_info_str"。它和
	// MK/MK2/KD 都不重合（那几代没有 msg_name/msg_info/map_info_str），
	// 放在 MK 前判，避免以后新增指纹时被更宽的检查吃掉。
	if isBuiltinHoukago2(probe) {
		if ApplyBuiltinHoukago2Defaults(cfg, probe) {
			return true
		}
	}
	// 牝教師（MK，BSScript Rev.5）是更早的一代：单张 6 帧 title_menu、message
	// 800x104、dm_slotbase/dm_slot 与 museum_titlelist/map_week/map_time 并存。
	// 它和牝教師2 老版（MK2）都带 dm_slot，必须先判 MK，再判 MK2。
	if isBuiltinMk(probe) {
		if ApplyBuiltinMkDefaults(cfg, probe) {
			return true
		}
	}
	// 牝教師2 老版（MK2）的素材签名是"分图标题 + message 800x124 + dm_base/dm_slot"，
	// 与下面几代都不同；判在 MK 之后，免得掉进 KD 检查后整体放弃。
	if isBuiltinMk2(probe) {
		if ApplyBuiltinMk2Defaults(cfg, probe) {
			return true
		}
	}
	// 学園（GK/GKDVD/GKWindows10）的素材签名是"title_menu 单图 + 800x176
	// message + save_base/load_base + dm_slot + map_place/date_base/map_face"，
	// 与学園2/AFDVD/KD 都不同。放在学園2 之前判，避免它掉进后面任何一代。
	if isBuiltinGK(probe) {
		applyBuiltinGK(cfg, probe)
		return true
	}
	// 学園2 的素材签名（单张 8 帧 title_menu、140 高 message、dm_save/dm_load
	// 并存）与 AFDVD/KD 都不同，先判它；三者互斥，不会互相覆盖。
	if isBuiltinGK2(probe) {
		applyBuiltinGK2(cfg)
		return true
	}
	// AFDVD 与 KD 同为 Rev 脚本，但素材签名互斥；先判 AFDVD，免得它掉进下面的 KD 检查后整体放弃。
	if isBuiltinAFDVD(probe) {
		applyBuiltinAFDVD(cfg)
		return true
	}
	// すくーるヘブン らぶえろハーレム☆ももいろタイフーン 初回限定版（Rev.3、无 cfg）：
	// 素材签名是"独立 btn_* 工具按钮 + message 800x144 + save_base/load_base +
	// museum_menu"，与 KD/AFDVD/GK2 都不同，放在 KD 检查之前。
	if isBuiltinSchoolHeaven(probe) {
		applyBuiltinSchoolHeaven(cfg)
		return true
	}
	for _, name := range builtinBishopAssets {
		if !probe.Exists(name) {
			return false
		}
	}

	width, height := probe.Width, probe.Height
	if width <= 0 || height <= 0 {
		width, height = 800, 600
	}
	cfg.Width = width
	cfg.Height = height
	// KD 的存档格是 160x120，正好是 800x600 的 0.2；缩略图也按这个比例生成，
	// 否则槽位命中区和实际缩略图会缩成 100x75。
	cfg.ThumbnailRatio = 0.2

	applyBuiltinTitle(cfg)
	applyBuiltinMessage(cfg, height)
	applyBuiltinFont(cfg)
	applyBuiltinGameMenu(cfg, height)
	applyBuiltinSelect(cfg)
	applyBuiltinDataScreen(cfg)
	applyBuiltinDataNav(cfg)
	applyBuiltinLog(cfg)
	applyBuiltinInformation(cfg)
	applyBuiltinMuseum(cfg)
	applyBuiltinConfigScreen(cfg)
	return true
}

func applyBuiltinTitle(cfg *BsxxConfiguration) {
	cfg.Title = TitlePhase{
		BgmName:  "bgm24",
		Base:     "title_base",
		HitTable: "title_hit",
		Entries: []TitleMenuEntry{
			{Key: "op", Image: "title_menu_op", Frames: 2, X: 416, Y: 254},
			{Key: "start", Image: "title_menu_begin", Frames: 2, X: 416, Y: 304},
			{Key: "load", Image: "title_menu_load", Frames: 2, X: 416, Y: 354},
			{Key: "museum", Image: "title_menu_museum", Frames: 2, X: 416, Y: 404},
			{Key: "config", Image: "title_menu_setting", Frames: 2, X: 416, Y: 454},
			{Key: "quit", Image: "title_menu_quit", Frames: 2, X: 416, Y: 504},
		},
	}
}

func applyBuiltinMessage(cfg *BsxxConfiguration, height int) {
	// message.bsg 是 800x150 的窗口，原引擎把它贴在屏幕底部（base = 屏高-150）。
	cfg.Message = MessageWindow{
		Base:            "message",
		BasePosSet:      true,
		BaseX:           0,
		BaseY:           height - 150,
		Speed:           100,
		BgColor:         0x7fffffff,
		NameX:           31,
		NameY:           11,
		NameW:           570,
		NameH:           25,
		NameColor:       0xffff8822,
		TextX:           31,
		TextY:           36,
		TextW:           570,
		TextH:           76,
		Unagashi:        "unagashi",
		UnagashiX:       616,
		UnagashiY:       90,
		UnagashiCellW:   24,
		UnagashiCellH:   24,
		UnagashiColumns: 1,
		UnagashiFrames:  2,
		UnagashiSpeed:   500,
	}
}

func applyBuiltinFont(cfg *BsxxConfiguration) {
	// 16px 字号、25px 行高对应 800x600 下的 3 行正文（75px 高）。
	cfg.Font = FontSettings{
		Size:         16,
		Width:        22,
		Height:       25,
		Weight:       1000,
		Edge:         1,
		Shadow:       1,
		LineLength:   34,
		Lines:        3,
		EudcFontFace: "bsxx.ttf",
	}
}

func applyBuiltinGameMenu(cfg *BsxxConfiguration, height int) {
	// 没有 gamemenu_base：底条就是 message.bsg 的下半部分，按钮直接贴在上面。
	// 坐标来自 kd.exe 的 0x414450 调用，都是屏幕绝对坐标。
	cfg.GameMenu = GameMenu{
		Base:     "",
		BaseX:    0,
		BaseY:    height - 150,
		ShowArea: [4]int{0, height - 150, cfg.Width, 150},
		Buttons: []MenuButton{
			{Key: "voice", UIButton: UIButton{Image: "btn_voice", Frames: 4, X: 668, Y: 464}},
			{Key: "skip", UIButton: UIButton{Image: "btn_skip", Frames: 4, X: 740, Y: 464}},
			{Key: "auto ex", UIButton: UIButton{Image: "btn_auto", Frames: 4, X: 740, Y: 500}},
			{Key: "dress", UIButton: UIButton{Image: "btn_dress", Frames: 4, X: 740, Y: 536}},
			{Key: "view up", UIButton: UIButton{Image: "btn_f", Frames: 4, X: 704, Y: 464}},
			{Key: "view middle", UIButton: UIButton{Image: "btn_b", Frames: 4, X: 704, Y: 500}},
			{Key: "view down", UIButton: UIButton{Image: "btn_w", Frames: 4, X: 704, Y: 536}},
			{Key: "quick save", UIButton: UIButton{Image: "btn_qsave", Frames: 4, X: 307, Y: 572}},
			{Key: "quick load", UIButton: UIButton{Image: "btn_qload", Frames: 4, X: 372, Y: 572}},
			{Key: "save", UIButton: UIButton{Image: "btn_save", Frames: 4, X: 437, Y: 572}},
			{Key: "load", UIButton: UIButton{Image: "btn_load", Frames: 4, X: 502, Y: 572}},
			{Key: "config", UIButton: UIButton{Image: "btn_config", Frames: 4, X: 567, Y: 572}},
			{Key: "log", UIButton: UIButton{Image: "btn_log", Frames: 4, X: 632, Y: 572}},
		},
	}
}

func applyBuiltinSelect(cfg *BsxxConfiguration) {
	// item0.bsg 是 624x64（2 帧，实际 624x32），居中后左边界是 (800-624)/2=88，
	// 文字基准点取条宽的一半（312）做居中。
	cfg.SelectWindow = NewSelectWindowConfiguration(288, 1, []SelectDesign{
		{
			Item:         "item0",
			Interval:     8,
			TextX:        312,
			TextY:        4,
			Alignment:    SelectDesignAlignCenter,
			DefaultColor: 0xffffffff,
			AlreadyColor: 0xff8d8c8c,
			SelectColor:  0xffff0000,
		},
	})
}

func applyBuiltinDataScreen(cfg *BsxxConfiguration) {
	screen := DataScreen{
		SaveBase:     "dm_save",
		LoadBase:     "dm_load",
		HitTable:     "dm_hit",
		NoBegin:      1,
		PageMax:      10,
		SlotsPerPage: 12,
	}
	// 4 列 x 3 行，坐标直接取 dm_hit 的彩色命中块。
	// 列间距不是等距的（28,224,424,620），不能用一个步长硬算。
	columns := [4]int{28, 224, 424, 620}
	for row := 0; row < 3; row++ {
		y := 136 + row*156
		for _, x := range columns {
			screen.Slots = append(screen.Slots, UIPoint{X: x, Y: y})
		}
	}
	// Page Jump 的 1…10 已经画在 dm_save/dm_load 底图上，只登记热区。
	for row := 0; row < 2; row++ {
		for column := 0; column < 5; column++ {
			screen.PageHits = append(screen.PageHits, UIPoint{
				X: 626 + column*28,
				Y: 338 + row*28,
			})
		}
	}
	cfg.DataScreen = screen
}

func applyBuiltinLog(cfg *BsxxConfiguration) {
	// kaisou.bsg 是整屏回想底图；log_meter 是右侧长条滚动条，log_vp 是语音图标。
	cfg.Log = LogScreen{
		Base:      "kaisou",
		Lines:     5,
		LeftPos:   100,
		Width:     600,
		Bar:       UIButton{Image: "log_meter", Frames: 1, X: 750, Y: 30},
		VoiceIcon: "log_vp",
	}
}
