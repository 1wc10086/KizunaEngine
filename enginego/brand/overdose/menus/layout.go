package menus

// 标题/存档/配置的布局与资源名。
//
// 坐标来源：sq_menu / sq_save / sq_setting / sq_extra_* 是 Squirrel 2.x
// 编译字节码；用 nutcracker 反编译后可直接读到 ui.Register 的实参，
// 因此本文件的矩形是原版精确坐标，不是“按图片尺寸猜”的约定布局。
// 只有引擎侧滑出面板（sq_adv 的 _ShowMenuPanel 是空实现）无法从脚本取得，
// 对应位置在 session/ui.go 里注明。
//   - 标题按钮 218x38，x=582，y=341 起每行 50；
//   - 存档按钮 save_slotNN 253x84、银行 save_bankNN 37x42，
//     10 bank × 10 slot，slot 为 2 列 × 5 行；
//   - 配置/回想/鉴赏的背景均为 800x600 整图，按钮按 sq_* 的
//     ui.Register 实参给出命中矩形。
// 修改前先回看 tmpwork/ui_exact_spec.md；不要用“视觉居中”去覆盖这里。
//
// 命名约定与 graphic.arc 的真实条目一一对应（已逐个用
// Set.Image / EntrySize 核实存在性）：
//   - title_*: title_bg + start/load/config/extra/exit（各带 -o 按下态）
//   - save_*: save_bg + save_bank01..10 + save_slot01..10 + save_return
//     + save_thumbnail；load_* 同理（但 load_return 在图里不存在，
//     读档的返回按钮复用 save_return/cd_return，见 SaveLayout）
//   - config_*: config_bg + title/return/exit/reset/on/off/rt1..3/
//     slow/normal/fast/voice/test/pin/mw/txt/voff/von
//   - log_*: log_bg（=log_overlay 800x600）+ return/up/dw/slider/
//     voice/pin（backlog 的背景在 sq_backlog 里是截图 + log_overlay）
//   - cg_*: cg_bg + cg_btn1..3 + cg_page1..5 + cg_base01..20
//   - sce_*: sce_bg1..19 + sce_l/sce_r（场景鉴赏背景）
//   - bgm_*: bgm_bg + bgm_btn1..10（音乐鉴赏）
//   - confirm_*: confirm_bg + confirm_yes/no + confirm_msg_*
//   - extra 入口在 spec 里记为 extra_cg/music/scene，但 graphic.arc
//     里没有 extra_* 条目——实际按钮是 cg_btn1/2/3（sq_extra_cg
//     里的 btnCG/btnScene/btnMusic），这里按真实资源名布局，
//     ExtraLayout 的 Image 字段直接给 cg_btn*。

// TitleBG 是标题背景资源名（不带 .cpb）。
const TitleBG = "title_bg"

// TitleBgm 是标题 BGM（sq_menu OnCreate 里 SetMenuBGMFile("bgm01")）。
const TitleBgm = "bgm01"

// titleButtons 是标题五按钮的纵列排布。坐标来自 sq_menu.nut 反编译：
// ui.Register(btnStart, 582, 341)、step 50，按钮图 218x38。
// 不要用“居中”去猜：原版就是贴右侧 582..800 的一列。
var titleButtons = []struct {
	key   string
	image string
}{
	{"start", "title_start"},
	{"load", "title_load"},
	{"config", "title_config"},
	{"extra", "title_extra"},
	{"quit", "title_exit"},
}

// TitleMenu 返回标题菜单五项（顺序即展示顺序）。
func TitleMenu() []MenuItem {
	out := make([]MenuItem, 0, len(titleButtons))
	for i, b := range titleButtons {
		item := MenuItem{
			Key:        b.key,
			Image:      b.image,
			ImageOn:    b.image + "-o",
			Label:      b.key,
			TitleLabel: b.key,
			Rect:       Rect{X: 582, Y: 341 + i*50, W: 218, H: 38},
		}
		out = append(out, item)
	}
	return out
}

// SaveLayout 描述存档/读档的网格布局。坐标来自 sq_save.nut 反编译：
//   - bank: 265 + i*46, y=15，37x42
//   - slot: 257 + (i/5)*258, 80 + (i%5)*92，253x84（2 列 × 5 行）
//   - return: 603,541，148x38
//   - 缩略图 198x148，预览锚点 30,222
type SaveLayout struct {
	Background string // save_bg / load_bg
	BankPrefix string // "save_bank" / "load_bank"
	SlotPrefix string // "save_slot" / "load_slot"
	Return     string // 返回按钮：save_return（load 复用它，见下）

	Banks int // 10
	Slots int // 10

	BankRects  []Rect // 长度 Banks
	SlotRects  []Rect // 长度 Slots（当前 bank 下的 10 个槽）
	ReturnRect Rect

	Thumbnail string // save_thumbnail（槽位缩略图占位 107x80）
	ThumbRect Rect   // 原版大缩略图 198x148 的预览区
}

// SaveBankImage 返回第 bank（1 起）个银行按钮的资源名。
func SaveBankImage(prefix string, bank int) string {
	return prefix + pad2(bank)
}

// SaveSlotImage 返回第 slot（1 起）个槽位按钮的资源名。
func SaveSlotImage(prefix string, slot int) string {
	return prefix + pad2(slot)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + itoaSmall(v)
	}
	return itoaSmall(v)
}

func itoaSmall(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [4]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// SaveLayoutFor 返回存档（mode=save）或读档（mode=load）的布局。
// load 的返回按钮在 graphic.arc 里没有 load_return 条目
// （已用 EntrySize 核实缺席），sq_load 的返回沿用 save_return 的
// 视觉——这里统一给 Return="save_return”，避免 session 按名
// 找 load_return 直接报“无条目”。
func SaveLayoutFor(mode string) SaveLayout {
	bg, bankP, slotP := "save_bg", "save_bank", "save_slot"
	if mode == "load" {
		bg, bankP, slotP = "load_bg", "load_bank", "load_slot"
	}
	l := SaveLayout{
		Background: bg,
		BankPrefix: bankP,
		SlotPrefix: slotP,
		Return:     "save_return",
		Banks:      10,
		Slots:      10,
		Thumbnail:  "save_thumbnail",
		ThumbRect:  Rect{X: 30, Y: 222, W: 198, H: 148},
	}
	for i := 0; i < 10; i++ {
		l.BankRects = append(l.BankRects, Rect{X: 265 + i*46, Y: 15, W: 37, H: 42})
	}
	for i := 0; i < 10; i++ {
		l.SlotRects = append(l.SlotRects, Rect{
			X: 257 + (i/5)*258,
			Y: 80 + (i%5)*92,
			W: 253,
			H: 84,
		})
	}
	l.ReturnRect = Rect{X: 603, Y: 541, W: 148, H: 38}
	return l
}

// ConfigLayout 描述配置画面。全部矩形来自 sq_setting.nut 的 ui.Register 实参，
// 不要按“看起来差不多”再调：原版按钮就是 60x30 / 48x30 的贴图，错 3px 就会
// 与 config_bg 里烘死的标签错位。
type ConfigLayout struct {
	Background string // config_bg
	Title      string // config_title
	Return     string // config_return
	Exit       string // config_exit
	Reset      string // config_reset
	On         string // config_on（开关开）
	Off        string // config_off（开关关）

	// 文字速度三档 / 自动速度三档 / 点击右键三档
	TextSlow, TextNormal, TextFast        string
	AutoSlow, AutoNormal, AutoFast        string
	ClickRight1, ClickRight2, ClickRight3 string

	Window   string // config_mw（消息窗预览）
	TextBg   string // config_txt（消息窗示例文字）
	VoiceOn  string // config_von
	VoiceOff string // config_voff
	Voice    string // config_voice（角色语音开关）
	Test     string // config_test（音频测试按钮）
	Pin      string // config_pin（滑杆柄，21x21）
	Slider   string // 滑杆柄复用 pin

	TitleRect  Rect
	ReturnRect Rect
	ExitRect   Rect
	ResetRect  Rect

	// 消息窗预览：config_mw/config_txt 同锚点 621x37。
	PreviewRect Rect

	FullOnRect    Rect
	FullOffRect   Rect
	EffectOnRect  Rect
	EffectOffRect Rect
	UnreadOnRect  Rect
	UnreadOffRect Rect
	ClickRects    [3]Rect
	TextRects     [3]Rect
	AutoRects     [3]Rect
	CharaRects    [3]Rect
	TestRects     [2]Rect

	// 滑杆：轨道宽 298（CreateSlider 的行程参数），柄 21x21。
	WndSliderRect    Rect
	AudioSliderRects [4]Rect
	SliderTravel     int
	PinSize          int
}

// ConfigLayoutFor 返回配置画面布局（sq_setting.nut 的精确坐标）。
func ConfigLayoutFor() ConfigLayout {
	l := ConfigLayout{
		Background: "config_bg",
		Title:      "config_title",
		Return:     "config_return",
		Exit:       "config_exit",
		Reset:      "config_reset",
		On:         "config_on",
		Off:        "config_off",

		TextSlow: "config_slow", TextNormal: "config_normal", TextFast: "config_fast",
		AutoSlow: "config_slow", AutoNormal: "config_normal", AutoFast: "config_fast",
		ClickRight1: "config_rt1", ClickRight2: "config_rt2", ClickRight3: "config_rt3",

		Window: "config_mw", TextBg: "config_txt",
		VoiceOn: "config_von", VoiceOff: "config_voff", Voice: "config_voice",
		Test: "config_test", Pin: "config_pin", Slider: "config_pin",

		TitleRect:  Rect{X: 582, Y: 451, W: 218, H: 38},
		ReturnRect: Rect{X: 582, Y: 501, W: 218, H: 38},
		ExitRect:   Rect{X: 582, Y: 551, W: 218, H: 38},
		ResetRect:  Rect{X: 582, Y: 401, W: 218, H: 38},

		PreviewRect: Rect{X: 116, Y: 209, W: 621, H: 37},

		FullOnRect:    Rect{X: 219, Y: 64, W: 60, H: 30},
		FullOffRect:   Rect{X: 303, Y: 64, W: 60, H: 30},
		EffectOnRect:  Rect{X: 219, Y: 104, W: 60, H: 30},
		EffectOffRect: Rect{X: 303, Y: 104, W: 60, H: 30},
		UnreadOnRect:  Rect{X: 598, Y: 104, W: 60, H: 30},
		UnreadOffRect: Rect{X: 684, Y: 104, W: 60, H: 30},

		WndSliderRect: Rect{X: 187, Y: 188, W: 298, H: 21},
		SliderTravel:  298,
		PinSize:       21,
	}
	l.ClickRects = [3]Rect{
		{X: 173, Y: 144, W: 68, H: 30},
		{X: 241, Y: 144, W: 68, H: 30},
		{X: 309, Y: 144, W: 68, H: 30},
	}
	l.TextRects = [3]Rect{
		{X: 592, Y: 64, W: 48, H: 30},
		{X: 652, Y: 64, W: 48, H: 30},
		{X: 712, Y: 64, W: 48, H: 30},
	}
	l.AutoRects = [3]Rect{
		{X: 592, Y: 144, W: 48, H: 30},
		{X: 652, Y: 144, W: 48, H: 30},
		{X: 712, Y: 144, W: 48, H: 30},
	}
	l.CharaRects = [3]Rect{
		{X: 82, Y: 291, W: 198, H: 90},
		{X: 312, Y: 291, W: 198, H: 90},
		{X: 542, Y: 291, W: 198, H: 90},
	}
	l.TestRects = [2]Rect{
		{X: 488, Y: 476, W: 36, H: 30},
		{X: 488, Y: 516, W: 36, H: 30},
	}
	l.AudioSliderRects = [4]Rect{
		{X: 145, Y: 401, W: 298, H: 21},
		{X: 145, Y: 441, W: 298, H: 21},
		{X: 145, Y: 481, W: 298, H: 21},
		{X: 145, Y: 521, W: 298, H: 21},
	}
	return l
}

// BacklogLayout 描述回想（backlog）画面。
// sq_backlog 的背景是“当前画面截图 + log_overlay 800x600”，
// 这里只给静态层：log_bg/log_overlay 同为 800x600，
// session 侧截图叠在 overlay 下面，不要把 log_bg 当作只有
// return/up/dw 的小图（实测 log_return 仅 32x20）。
type BacklogLayout struct {
	Screenshot string // 截屏（运行时画面，非静态资源）
	Overlay    string // log_overlay（800x600）
	Background string // log_bg（同 overlay，实测 800x600）

	Return string // log_return（32x20，右下角）
	Up     string // log_up（20x20）
	Down   string // log_dw（20x20，注意原名是 dw 不是 down）
	Slider string // log_slider（20x525，右侧滚动条）
	Voice  string // log_voice（语音回放按钮）
	Pin    string // log_pin（滑杆柄/滚动钉）

	ReturnRect Rect
	UpRect     Rect
	DownRect   Rect
	SliderRect Rect
	VoiceRect  Rect
	ListRect   Rect // 文本滚动区（overlay 内的文字区）
}

// BacklogLayoutFor 返回回想画面布局（sq_backlog.nut）：
// overlay (0,0)、slider (746,40) 20x525、up (746,40)、down (746,545)、
// return (754,6) 32x20、文字区 (40,40)。
func BacklogLayoutFor() BacklogLayout {
	return BacklogLayout{
		Overlay:    "log_overlay",
		Background: "log_bg",
		Return:     "log_return",
		Up:         "log_up",
		Down:       "log_dw",
		Slider:     "log_slider",
		Voice:      "log_voice",
		Pin:        "log_pin",
		ReturnRect: Rect{X: 754, Y: 6, W: 32, H: 20},
		UpRect:     Rect{X: 746, Y: 40, W: 20, H: 20},
		DownRect:   Rect{X: 746, Y: 545, W: 20, H: 20},
		SliderRect: Rect{X: 746, Y: 40, W: 20, H: 525},
		VoiceRect:  Rect{X: 39, Y: 0, W: 32, H: 20},
		ListRect:   Rect{X: 40, Y: 40, W: 700, H: 520},
	}
}

// ExtraLayout 描述 extra（鉴赏）入口与三页签。坐标来自 sq_extra_*.nut：
// tabs (17/217/417,538)、return (637,552)、CG 20 格 5x4、音乐 10 条、
// 场景 12 格 3x4 + 左右翻页。
type ExtraLayout struct {
	Background string // cg_bg（CG 页）/ sce_bgN（场景页）/ bgm_bg（音乐页）
	CgTab      ExtraScreen
	SceneTab   ExtraScreen
	MusicTab   ExtraScreen
	Return     string // cd_return
	ReturnRect Rect

	CgBackground    string // cg_bg（800x600）
	SceneBackground string // scene_base 框架 + sce_bgN（场景页按页换背景）
	MusicBackground string // bgm_bg（800x600）

	PagePrefix   string // cg_page（页签 1..5，37x42）
	BasePrefix   string // cg_base（缩略图底 01..20，132x100）
	SceneBase    string // scene_base（132x100，与 cg_base 同尺寸）
	MusicBtnPref string // bgm_btn（1..10）

	PageRects [5]Rect // cg_page1..5：562+40*i, 20
	CGCellW   int     // 140
	CGCellH   int     // 110
	CGGridX   int     // 56
	CGGridY   int     // 82
	CGCols    int     // 5
	CGPerPage int     // 20

	MusicRects [10]Rect

	SceneCellW   int // 140
	SceneCellH   int // 120
	SceneGridX   int // 335
	SceneGridY   int // 162
	SceneCols    int // 3
	ScenePerPage int // 12
	Prev         string
	Next         string
	PrevRect     Rect // 432,76,40,50
	NextRect     Rect // 606,76,40,50
}

// ExtraLayoutFor 返回 extra 入口布局。
func ExtraLayoutFor() ExtraLayout {
	l := ExtraLayout{
		Background:      "cg_bg",
		CgTab:           ExtraScreen{Key: "cg", Image: "cg_btn1", Rect: Rect{X: 17, Y: 538, W: 198, H: 38}},
		SceneTab:        ExtraScreen{Key: "scene", Image: "cg_btn2", Rect: Rect{X: 217, Y: 538, W: 198, H: 38}},
		MusicTab:        ExtraScreen{Key: "music", Image: "cg_btn3", Rect: Rect{X: 417, Y: 538, W: 198, H: 38}},
		Return:          "cd_return",
		ReturnRect:      Rect{X: 637, Y: 552, W: 148, H: 38},
		CgBackground:    "cg_bg",
		SceneBackground: "scene_base",
		MusicBackground: "bgm_bg",
		PagePrefix:      "cg_page",
		BasePrefix:      "cg_base",
		SceneBase:       "scene_base",
		MusicBtnPref:    "bgm_btn",
		CGCellW:         140,
		CGCellH:         110,
		CGGridX:         56,
		CGGridY:         82,
		CGCols:          5,
		CGPerPage:       20,
		SceneCellW:      140,
		SceneCellH:      120,
		SceneGridX:      335,
		SceneGridY:      162,
		SceneCols:       3,
		ScenePerPage:    12,
		Prev:            "sce_l",
		Next:            "sce_r",
		PrevRect:        Rect{X: 432, Y: 76, W: 40, H: 50},
		NextRect:        Rect{X: 606, Y: 76, W: 40, H: 50},
	}
	for i := 0; i < 5; i++ {
		l.PageRects[i] = Rect{X: 562 + i*40, Y: 20, W: 37, H: 42}
	}
	for i := 0; i < 10; i++ {
		x := 36
		y := 149 + 50*i
		if i >= 5 {
			x = 416
			y = 199 + 50*(i-5)
		}
		l.MusicRects[i] = Rect{X: x, Y: y, W: 352, H: 42}
	}
	return l
}

// ConfirmLayout 描述确认弹窗。
// 原版有 6 种消息图（menu/save/load/qsave/qload/setting/quit/overwrite），
// session 按场景取 ConfirmMsgFor(kind) 即可，不要自造消息名。
type ConfirmLayout struct {
	Background string // confirm_bg（430x254）
	Yes        string // confirm_yes（82x58）
	No         string // confirm_no（100x58）
	MsgPrefix  string // confirm_msg_（后接 menu/save/load/qsave/qload/setting/quit/overwrite）

	BgRect  Rect
	YesRect Rect
	NoRect  Rect
	MsgRect Rect
}

// ConfirmLayoutFor 返回确认弹窗布局（居中 430x254）。
func ConfirmLayoutFor() ConfirmLayout {
	return ConfirmLayout{
		Background: "confirm_bg",
		Yes:        "confirm_yes",
		No:         "confirm_no",
		MsgPrefix:  "confirm_msg_",
		BgRect:     Rect{X: 185, Y: 173, W: 430, H: 254},
		YesRect:    Rect{X: 285, Y: 335, W: 82, H: 58},
		NoRect:     Rect{X: 425, Y: 335, W: 100, H: 58},
		MsgRect:    Rect{X: 185, Y: 173, W: 430, H: 254},
	}
}

// ConfirmMsgFor 返回某确认场景的消息图名。
// kind 取 menu/save/load/qsave/qload/setting/quit/overwrite 之一，
// 非法 kind 直接返回空（调用方据此不显示弹窗，不要替它兜底）。
func ConfirmMsgFor(kind string) string {
	switch kind {
	case "menu", "save", "load", "qsave", "qload", "setting", "quit", "overwrite":
		return "confirm_msg_" + kind
	}
	return ""
}

// AdvButton 是游戏内菜单（sq_adv）的按钮资源名，方便 session 的
// 工具条/消息窗口命中判定与之对齐。
type AdvButton struct {
	Key   string
	Image string
}

// AdvButtons 返回游戏内菜单按钮（顺序即 sq_adv 的创建序）。
func AdvButtons() []AdvButton {
	return []AdvButton{
		{"exit", "adv_close"},
		{"config", "adv_config"},
		{"save", "adv_save"},
		{"qsave", "adv_qsave"},
		{"load", "adv_load"},
		{"qload", "adv_qload"},
		{"log", "adv_log"},
		{"voice", "adv_voice"},
		{"auto", "adv_auto"},
		{"skip", "adv_skip"},
		{"scene", "adv_scene"},
	}
}

// MessageWindowImage 是消息窗口底图（adv_msg_wnd 800x180）。
const MessageWindowImage = "adv_msg_wnd"

// AdvBarButton 是 adv 窗口工具栏按钮（sq_adv.nut 的 y=574 一排）。
// 原版只注册这六个；save/load/qsave/qload/exit 是隐藏面板里的，不进工具栏。
type AdvBarButton struct {
	Key   string
	Image string
	Rect  Rect
}

// AdvBar 返回对话框工具栏六按钮，顺序即原版注册序。
func AdvBar() []AdvBarButton {
	return []AdvBarButton{
		{"auto", "adv_auto", Rect{X: 240, Y: 574, W: 76, H: 20}},
		{"skip", "adv_skip", Rect{X: 319, Y: 574, W: 76, H: 20}},
		{"log", "adv_log", Rect{X: 398, Y: 574, W: 76, H: 20}},
		{"config", "adv_config", Rect{X: 635, Y: 574, W: 76, H: 20}},
		{"voice", "adv_voice", Rect{X: 716, Y: 574, W: 32, H: 20}},
		{"close", "adv_close", Rect{X: 753, Y: 574, W: 32, H: 20}},
	}
}

// ActualSize 是已核实的真实像素尺寸（800x600 坐标系外的图）。
// session 命中判定优先用真实尺寸，Rect 只提供锚点；
// 返回 (0,0) 表示未核实，调用方按 Rect.W/H 兜底。
func ActualSize(image string) (int, int) {
	switch image {
	case "title_bg", "save_bg", "load_bg", "config_bg", "log_bg", "log_overlay",
		"cg_bg", "sce_bg1", "bgm_bg":
		return 800, 600
	case "title_start", "title_load", "title_config", "title_extra", "title_exit",
		"config_title", "config_return", "config_exit", "config_reset":
		return 218, 38
	case "save_bank01", "load_bank01", "cg_page1":
		return 37, 42
	case "save_slot01", "load_slot01":
		return 253, 84
	case "save_return", "cd_return":
		return 148, 38
	case "save_thumbnail":
		return 107, 80
	case "cg_base01", "scene_base":
		return 132, 100
	case "thumb_cga01-01":
		return 128, 96
	case "log_slider":
		return 20, 525
	case "log_return":
		return 32, 20
	case "log_up", "log_dw":
		return 20, 20
	case "config_pin":
		return 21, 21
	case "config_mw", "config_txt":
		return 621, 37
	case "confirm_bg":
		return 430, 254
	case "confirm_yes":
		return 82, 58
	case "confirm_no":
		return 100, 58
	case "adv_msg_wnd":
		return 800, 180
	case "cg_btn1", "cg_btn2", "cg_btn3":
		return 198, 38
	case "sce_l", "sce_r":
		return 40, 50
	}
	return 0, 0
}
