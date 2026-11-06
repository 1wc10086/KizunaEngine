package config

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const (
	WindowSection        = "main window"
	VolumeSection        = "default volume"
	BustupSection        = "bustup setting"
	FreeSightSection     = "free-sight screen"
	ButtonObjectSection  = "button object"
	CustomMapSection     = "custom map phase"
	MessageWindowSection = "message window"
	FontSection          = "default font"
	SelectWindowSection  = "select window"

	MaximumSelectDesigns = 4

	DefaultEyePattern  = 6
	DefaultEyeInterval = 30.0
)

const (
	fixedPositionPrefix = "fixed position"

	bustupCharacterOffset = 2
	bustupCharacterLength = 2
	bustupVariantIndex    = 9
	bustupNameLength      = 10
)

type fixedPosition struct {
	key       string
	positions []int
}

// FreeSightScreen 是 [free-sight screen]（自由视角画面）的布局。
//
// 自由视角可以自由地移动镜头看立绘的各个部位：右侧一根滑杆控制上下，
// 另有"脸 / 胸 / 股间"三个按钮把视角拉到固定位置。按钮图名、帧数和
// 坐标都来自配置，换作品时不用改代码。
type FreeSightScreen struct {
	// Mode 是否启用自由视角。
	Mode bool
	// FixedPositions 是脸 / 胸 / 股间的三个固定位置。
	// 配置里写的是"画面高度 + 偏移"这种算术表达式，这里已经算成实际坐标。
	FixedPositions []int
	// Meter 是滑杆轨道的位置与尺寸：x / y / 宽 / 高。
	// 配置里的四个值是"左, 上, 右, 下"，这里换算成了左上角与尺寸。
	Meter [4]int
	// Knob 是滑杆上的滑块。
	Knob UIButton
	// Face / Bust / Hip 分别是移到脸 / 胸 / 股间的按钮。
	Face UIButton
	Bust UIButton
	Hip  UIButton
}

type BsxxConfiguration struct {
	document       *IniDocument
	fixedPositions []fixedPosition

	HasWindowSize bool
	// Mddvd 表示当前作品套用了 MDDVD 的内置布局。Session 用它启用
	// XA（言葉責め）/XR（単語整理）等这一代专属的 system call 钩子；
	// 其它作品保持 false，通用分派完全不受影响。
	Mddvd          bool
	SelectWindow   SelectWindowConfiguration
	Width          int
	Height         int
	ThumbnailRatio float64
	HairSuffix     string
	// AheSuffix 是"アヘ顔OFF"时替换アヘ顔图层的后缀（[main window] ahe suffix）。
	// 牝教師4 的 event CG 把强烈アヘ顔做成独立图层，素材旁边同时提供
	// `<name>_noahe`；开启设置后加载器优先取它，剧情/CG 鉴赏/立ち絵共用。
	AheSuffix string
	// AheOff 是运行时的"アヘ顔OFF"设置，由 session 在设置变化时同步。
	// StageDirector 用它决定是否跳过 A-H-E 图层动画；不写进配置文件。
	AheOff              bool
	BgmVolume           float64
	EffectVolume        float64
	VoiceVolume         float64
	ScreenTile          string
	BackgroundSignature string
	BackgroundSuffixMap string
	SceneNameDatabase   string
	// ChartDatabase 是 [main window] chart db 指向的 CHART 场景图数据库名
	// （不含扩展名，由 config/chart.go 解析）。
	ChartDatabase        string
	BgmSelectDatabase    string
	ForcedAnimationLevel int
	LiquidMode           string
	EyePattern           int
	// HasEyePattern 表示 bsxx.cfg 显式写了 [bustup setting] eye pattern。
	// 三射面談（SM）没有这一项，旧实现会退回默认 6，把实际的 4 帧眼部条
	// 切成 6 份（眼睛缺失/错位）；Session 只在缺项时按素材推断。
	HasEyePattern            bool
	EyeAnimationMilliseconds float64
	CounterImage             string
	CounterFrames            int
	CounterX                 int
	CounterY                 int
	CounterVariable          string
	FreeSightMode            bool
	FixedPositions           []int
	// FreeSight 是 [free-sight screen] 的自由视角画面布局。
	// FreeSightMode / FixedPositions 是同一份数据的老字段，保留是为了兼容
	// 已经在用它们的地方。
	FreeSight FreeSightScreen
	// ExtraWindow 是 [extra window] 左下角常驻小面板的布局；Base 为空表示作品没有该面板。
	ExtraWindow ExtraWindowScreen
	// Title 是主界面（标题菜单）的布局，来自 bsxx.cfg 的 [title phase]。
	// 放在配置里而不是硬编码，是因为每部作品的按钮图名和坐标都不同。
	Title TitlePhase
	// Information 是左上角"行动信息窗"（行動開始から X日目）的布局，
	// 来自 bsxx.cfg 的 [information window]。
	Information InformationWindow
	// DataScreen 是存档/读档画面的布局，来自 [data screen]。
	DataScreen DataScreen
	// Item 是 SYSTEM 菜单的「持ち物」页（MK 的内置布局）。
	Item ItemScreen
	// GameMenu 是游戏内底部菜单条的布局，来自 [game menu] + [system button]。
	GameMenu GameMenu
	// Museum 是鉴赏室（CG/场景/BGM）的布局，来自 [museum phase] 及其子段。
	Museum Museum
	// Dress 是立ち絵 / 脱衣设定画面的布局，来自 [dress screen]。
	Dress DressScreen
	// DateCard 是剧情左上角的日期/时刻卡（学園 GK）。
	DateCard DateCard
	// Message 是原版对话框（底图/名字/正文/催促图标）的布局。
	Message MessageWindow
	// Font 是 [default font] 的字体参数，引擎自绘文字时使用。
	Font FontSettings
	// Log 是 [log screen] 的回想界面布局。
	Log LogScreen
	// CustomInfoWindows 是 [custom info windows] 里登记的 Lua 窗口名
	// （00=*cs1、01=*cs2…）。它们由剧本自绘地图时叠加在右侧，引擎负责
	// 加载对应 ex script 并每帧 draw。
	CustomInfoWindows []string
	// LegacyConfig 是 bsxx.cfg 旧式 [config screen] 的布局。
	// 与 define.bsa:system_config_ex.layout 二选一，作品只会有其中一种。
	LegacyConfig LegacyConfigScreen
	// Status 是 [status screen] 的"状態"画面：配置只给布局资源名，
	// 页面/条件图标在 define.bsa 里（见 status_screen.go）。
	Status StatusScreenSettings
	// Status2 是 [status2 screen] 的「ケダモノカルテ」画面（OH/OHIE）。
	// 工具条上的 status 按钮在这两代作品里是 Karte，不是 status_layout；
	// 因此存在本段时优先打开它（见 session/systemui_karte.go）。
	Status2 Status2Screen
	// LegacyStatus 是旧式 [status screen] 的 ITEM 页（教育指導 KS/KSIE），
	// 没有 define.bsa 布局，道具图标位置直接写在 item111…item533。
	LegacyStatus LegacyStatusScreen
	// CustomMap 是 [custom map phase] 里由引擎补画的按钮。
	// 剧本自己画完整套工作画面时（SLIE）它不会被用到；剧本只画背景与
	// 工作面板时（SL），START 必须由引擎补出来，否则选择态没有按钮，
	// 玩家会卡在工作画面上无法继续。
	CustomMap CustomMapPhase
	// TargetSelect 是 [target select]（教育指導 KS）的放学目标卡片布局。
	TargetSelect TargetSelect
	// MapPhase 是 [map phase]（UK 裏教師 / UKIE）的地图布局：周数、曜日、时段与系统按钮。
	MapPhase MapPhase
	// Pcall 是 [pcall parameter]（らぶえろの設定）：好感度报表与らぶえろメーター
	// 绑定的脚本属性。段缺失时为零值，不影响其它作品。
	Pcall PcallParameter
	// Ecstasy 是 [ecstasy meter]（満淫電車2）：言葉責め系统 NEO 的兴奋/
	// 压力/连击/剩余时间仪表及反应数值。其它作品为空。
	Ecstasy EcstasyMeter
}

// StatusScreenSettings 是 [status screen] 段。
type StatusScreenSettings struct {
	// Layout 指向 define.bsa 里的布局资源，例如 status_layout。
	Layout string
	// ScriptName 是脚本驱动状态画面的名字（[status screen] script name）。
	// 特別授業3SLG 用 status_screen 这个 Lua 对象，不提供 layout；
	// 引擎按名字选择原生实现，不能当成"没有状态画面"。
	ScriptName string
	// Ancient 表示使用 Tbxdl 世代（特別病棟）的内置状態画面：
	// status_base 已经把四位角色的资料烘焙在整屏里，引擎只需要按当前
	// 选中角色把 status_commentN 叠到对应面板下方。
	Ancient bool
	// Base 是整屏底图；空时由内置值 status_base 填充。
	Base string
}

func Load(path string) (*BsxxConfiguration, error) {
	if !core.PathExists(path) {
		return newBsxxConfiguration(EmptyIniDocument()), nil
	}

	document, err := LoadIniDocument(path)
	if err != nil {
		return nil, err
	}

	return newBsxxConfiguration(document), nil
}

func newBsxxConfiguration(document *IniDocument) *BsxxConfiguration {
	configuration := &BsxxConfiguration{document: document}

	configuration.HasWindowSize = document.Has(WindowSection, "width") && document.Has(WindowSection, "height")
	configuration.Width = document.GetInt(WindowSection, "width", 1024)
	configuration.Height = document.GetInt(WindowSection, "height", 768)
	configuration.ThumbnailRatio = document.GetDouble(WindowSection, "thumbnail ratio", 0.125)
	configuration.HairSuffix = document.Get(WindowSection, "hair suffix", "")
	configuration.AheSuffix = document.Get(WindowSection, "ahe suffix", "")
	configuration.BgmVolume = clampVolume(document.GetDouble(VolumeSection, "bgm", 0.5))
	configuration.EffectVolume = clampVolume(document.GetDouble(VolumeSection, "se", 0.75))
	configuration.VoiceVolume = clampVolume(document.GetDouble(VolumeSection, "voice", 1.0))
	configuration.ScreenTile = document.Get(WindowSection, "screen tile", "scrbg")
	configuration.BackgroundSignature = document.Get(WindowSection, "bg signature", "")
	configuration.BackgroundSuffixMap = document.Get(WindowSection, "sfx bg", "")
	configuration.SceneNameDatabase = document.Get(WindowSection, "scene name db", "")
	configuration.ChartDatabase = document.Get(WindowSection, "chart db", "chart")
	configuration.BgmSelectDatabase = document.Get(WindowSection, "bgmselect db", "")
	configuration.ForcedAnimationLevel = document.GetInt(WindowSection, "forced anime level", -1)
	configuration.LiquidMode = document.Get(WindowSection, "liquid mode", "")
	configuration.HasEyePattern = document.Has(BustupSection, "eye pattern")
	configuration.EyePattern = max(1, document.GetInt(BustupSection, "eye pattern", DefaultEyePattern))
	configuration.EyeAnimationMilliseconds = math.Max(1.0, document.GetDouble(BustupSection, "eye anime interval", DefaultEyeInterval))

	originX, originY := parsePoint(document.Get(MessageWindowSection, "base pos", ""))
	counterImage, counterFrames, counterX, counterY := parseCounter(document.Get(MessageWindowSection, "x point counter", ""))
	configuration.CounterImage = counterImage
	configuration.CounterFrames = counterFrames
	configuration.CounterX = originX + counterX
	configuration.CounterY = originY + counterY
	configuration.CounterVariable = document.Get(MessageWindowSection, "x point variable", "")

	configuration.FreeSightMode = document.GetInt(FreeSightSection, "mode", 0) != 0
	configuration.FixedPositions = ParsePositions(document.Get(FreeSightSection, "fixed position", ""))
	configuration.FreeSight = parseFreeSightScreen(document)
	configuration.ExtraWindow = parseExtraWindowScreen(document)
	configuration.fixedPositions = parseFixedPositions(document)
	configuration.SelectWindow = parseSelectWindow(document)
	configuration.Title = parseTitlePhase(document)
	configuration.Information = parseInformationWindowForScreen(document, configuration.Height)
	configuration.DataScreen = parseDataScreen(document)
	configuration.GameMenu = parseGameMenu(document)
	configuration.Museum = parseMuseum(document)
	configuration.Dress = parseDressScreen(document)
	configuration.Message = parseMessageWindow(document)
	configuration.Font = parseFontSettings(document)
	configuration.Log = parseLogScreen(document)
	configuration.LegacyConfig = parseLegacyConfigScreen(document)
	configuration.CustomInfoWindows = parseCustomInfoWindows(document)
	configuration.LegacyStatus = parseLegacyStatusScreen(document)
	configuration.Status = StatusScreenSettings{
		Layout:     document.Get(StatusScreenSection, "layout", ""),
		ScriptName: document.Get(StatusScreenSection, "script name", ""),
	}
	configuration.Status2 = parseStatus2Screen(document)
	configuration.CustomMap = parseCustomMapPhase(document)
	configuration.TargetSelect = parseTargetSelect(document)
	configuration.MapPhase = parseMapPhase(document)
	configuration.Pcall = parsePcallParameter(document)
	configuration.Ecstasy = parseEcstasyMeter(document)

	return configuration
}

func (c *BsxxConfiguration) StandingOffsetY() int {
	return c.StandingOffsetFor("")
}

func (c *BsxxConfiguration) StandingOffsetFor(bustupName string) int {
	if !c.FreeSightMode {
		return 0
	}

	if positions := c.fixedPositionFor(bustupName); len(positions) > 0 && positions[0] > 0 {
		return c.Height - positions[0]
	}

	if len(c.FixedPositions) > 0 && c.FixedPositions[0] > 0 {
		return c.Height - c.FixedPositions[0]
	}

	return 0
}

func (c *BsxxConfiguration) ApplyScreenSize(window state.ScreenSize) {
	if window.Width <= 0 || window.Height <= 0 {
		return
	}

	c.Width = window.Width
	c.Height = window.Height
}

func (c *BsxxConfiguration) Raw(section, key, fallback string) string {
	return c.document.Get(section, key, fallback)
}

func (c *BsxxConfiguration) MapButtonValue(name string) int {
	// 两代配置的写法不同：
	//   SM   command_x = 2
	//        command_x value = 0xb0
	//   SMIE command_x = 0xb0, 2
	// 旧实现只读主键且只认十进制，SMIE 的十六进制全部变成 0，自定义地图
	// 的 #select_value 永远指向第一个分支（点「上条母娘を犯す」会走成
	// 「家に帰る」）。这里优先读 `<name> value`，再读主键第一个字段，
	// 两种整数格式都接受。
	if raw := c.document.Get(ButtonObjectSection, name+" value", ""); len(raw) > 0 {
		if value, ok := parseButtonInteger(raw); ok {
			return value
		}
	}
	parts := strutil.SplitTrimNonEmpty(c.document.Get(ButtonObjectSection, name, ""), ",")
	if len(parts) == 0 {
		// SLIE 的剧本用 IMAGE_EX type=160 画 START，但 [button object] 里
		// 没有它；值写在 define.bsa 的 work/work_start.txt（return_value: 1）。
		// 剧本紧接着用 SELECT_TAIL 的 #select_value == 1 判断"开始了"，
		// 返回 0 会让它走错分支。
		if strings.EqualFold(name, "work_start") {
			return 1
		}
		return 0
	}
	if value, ok := parseButtonInteger(parts[0]); ok {
		return value
	}
	return 0
}

// parseButtonInteger 接受十进制和 0x 十六进制，兼容两种 [button object] 写法。
//
// 按钮值并不总是"小整数"：学園3/学園3IE 的 action card 把
// "卡片种类（bit31）+ 角色号（bit12..15）+ 行动号（低 12 位）"打包成
// 0x80001001 这类值。用 ParseInt(..., 32) 会因为它超过 MaxInt32 直接报
// out of range，整张卡的值变成 0，点卡后剧本收到 0、永远走不进行动分支。
// 这里按无符号 32 位解析再转 int32：既保留 bit31 的语义（属性是 int32，
// 剧本用 `& -2147483648` 判高位），也不影响其它作品的小数值。
func parseButtonInteger(value string) (int, bool) {
	text := strings.TrimSpace(value)
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		parsed, err := strconv.ParseUint(text[2:], 16, 32)
		if err != nil {
			return 0, false
		}
		return int(int32(uint32(parsed))), true
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return int(int32(uint32(parsed))), true
}

// MapButtonFrames 返回自定义地图按钮的帧数。
//
// [button object] 有两种写法：
//
//	SMIE  card16 = 0x80001002, 3      // 值, 帧数
//	GK3   card16 = 3                  // 帧数
//	      card16 value = 0x80001002   // 值单独一行
//
// 学園3（BSXScript 2.0）用后一种写法。旧实现只看第二列，单列主键被当成
// "没有帧数"而回退 2 帧，行动卡是 128x576 的三帧竖排图，按 2 帧切会把
// 一半卡面画出来、命中区也错位。主键只有一列但存在 `<name> value` 时，
// 那一列就是帧数。
func (c *BsxxConfiguration) MapButtonFrames(name string) int {
	parts := strutil.SplitTrimNonEmpty(c.document.Get(ButtonObjectSection, name, ""), ",")
	if len(parts) >= 2 {
		if frames, ok := parseInt(parts[1]); ok && frames > 0 {
			return frames
		}
	}
	if len(parts) == 1 && c.document.Has(ButtonObjectSection, name+" value") {
		if frames, ok := parseInt(parts[0]); ok && frames > 0 {
			return frames
		}
	}
	if strings.HasPrefix(strings.ToLower(name), "work_") {
		return 4
	}
	// 教育指導 IE 的目标卡 target0…target9 是 120x570 的三帧图
	// （黑/灰/彩色）；[button object] 只写值不写帧数，缺省 2 帧会把
	// 卡片裁成半张、并且永远只画黑帧。只在 TargetCards 作品里补 3。
	if c.CustomMap.TargetCards && strings.HasPrefix(strings.ToLower(name), "target") {
		return 3
	}
	return 0
}

// MapButtonTableFrames 在存在 [custom map phase] result table 时返回帧数。
//
// 这一代作品的 [button object] 有两种写法：
//
//	OH    map_ch1_norm = 5          // 只有帧数，值由 result table 按顺序给
//	OHIE  map_ch1_norm = 1, 5       // 按钮 ID, 帧数
//
// 旧实现只把单列当值、双列第二项当帧数，于是 OH 的 5 帧卡被切成 2 帧，
// 1210px 的竖排图一次画出两张半角色卡——地图上看起来就是"同一个角色
// 出现两张卡"。这里只在 result table 存在时启用，避免影响其它作品。
func (c *BsxxConfiguration) MapButtonTableFrames(name string) int {
	parts := strutil.SplitTrimNonEmpty(c.document.Get(ButtonObjectSection, name, ""), ",")
	switch len(parts) {
	case 0:
		return 0
	case 1:
		if frames, ok := parseInt(parts[0]); ok && frames > 0 {
			return frames
		}
	default:
		if frames, ok := parseInt(parts[1]); ok && frames > 0 {
			return frames
		}
	}
	return 0
}

// MapButtonResultID 返回按钮在 result table 里的 ID。
//
// 双列写法（OHIE 的 `1, 5`）第一列就是 ID；单列写法（OH）没有显式 ID，
// 由调用方按登记顺序分配 1、2、3…，两个返回值都是 (0,false)。
func (c *BsxxConfiguration) MapButtonResultID(name string) (int, bool) {
	parts := strutil.SplitTrimNonEmpty(c.document.Get(ButtonObjectSection, name, ""), ",")
	if len(parts) < 2 {
		return 0, false
	}
	if id, ok := parseButtonInteger(parts[0]); ok {
		return id, true
	}
	return 0, false
}

// MapButtonNamedID 从 OH 的地图卡图名推断 result table 的 ID。
//
// OH 的 [button object] 单列只写帧数，没有显式 ID；按"本屏第几个按钮"
// 编号在热卡片模式下会错位——剧本只给今天有事件的女主角画 hot 卡，
// 只有二号角色可点时她会是第一个登记的按钮，序号 1 会错误地触发一号
// 角色的剧情。图名本身带角色槽位（map_ch2_hot），所以优先按名字取：
//
//	map_chN_*      → N（1..5）
//	map_nexttime   → result table 的最后一格（OH 是 6 → #result=0）
//
// 返回值 ok=false 时由调用方退回登记序号。
func (c *BsxxConfiguration) MapButtonNamedID(name string) (int, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if index := strings.IndexByte(lower, '.'); index > 0 {
		lower = lower[:index]
	}
	if strings.HasPrefix(lower, "map_ch") {
		digits := lower[len("map_ch"):]
		end := 0
		for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
			end++
		}
		if end > 0 {
			if value, err := strconv.Atoi(digits[:end]); err == nil && value > 0 {
				return value, true
			}
		}
	}
	if strings.HasPrefix(lower, "map_nexttime") && len(c.CustomMap.ResultTable) > 1 {
		return len(c.CustomMap.ResultTable) - 1, true
	}
	return 0, false
}

// MapButtonHoverFrame 读取 `<name> on` 的悬停帧号。
//
// 两帧作品默认悬停画第 1 帧，但 OH 的卡是 5 帧竖排、悬停帧是第 4 帧；
// 固定画第 1 帧会显示成另一张表情。没有 `on` 时返回 -1，调用方保持原行为。
func (c *BsxxConfiguration) MapButtonHoverFrame(name string) int {
	parts := strutil.SplitTrimNonEmpty(c.document.Get(ButtonObjectSection, name+" on", ""), ",")
	if len(parts) == 0 {
		return -1
	}
	if frame, ok := parseInt(parts[0]); ok && frame >= 0 {
		return frame
	}
	return -1
}

// parseCustomInfoWindows 读取 [custom info windows]。
//
// 键是两位连号（显示顺序），值形如 `*cs1`；`*` 表示加载同名 ex script。
func parseCustomInfoWindows(document *IniDocument) []string {
	section := "custom info windows"
	entries := document.Section(section)
	if len(entries) == 0 {
		return nil
	}
	names := make([]string, 0, len(entries))
	for index := 0; index < 100; index++ {
		key := ""
		if index < 10 {
			key = "0" + strconv.Itoa(index)
		} else {
			key = strconv.Itoa(index)
		}
		value := strings.TrimSpace(document.Get(section, key, ""))
		if value == "" {
			continue
		}
		value = strings.TrimPrefix(value, "*")
		if value != "" {
			names = append(names, value)
		}
	}
	return names
}

func (c *BsxxConfiguration) SelectResultVariable() string {
	return c.document.Get(CustomMapSection, "result", "")
}

func (c *BsxxConfiguration) SelectTypeVariable() string {
	return c.document.Get(CustomMapSection, "type", "")
}

// SelectObjectVariable 返回 [custom map phase] select object 配置的变量名。
//
// 教育指導 IE 的目标选择用 #target.select_no：剧本按它把被选中的卡片
// 移到 (340,142) 并播放确认动画。值必须是 1 基的按钮序号（target.no
// 从 1 开始比较），不能沿用引擎内部的 0 基下标。
func (c *BsxxConfiguration) SelectObjectVariable() string {
	return c.document.Get(CustomMapSection, "select object", "")
}

func (c *BsxxConfiguration) Counter() *state.StageCounter {
	if len(c.CounterImage) > 0 && c.CounterFrames > 0 {
		return &state.StageCounter{
			Image:  c.CounterImage,
			Frames: c.CounterFrames,
			X:      c.CounterX,
			Y:      c.CounterY,
		}
	}

	return nil
}

func (c *BsxxConfiguration) fixedPositionFor(bustupName string) []int {
	units := utf16.Encode([]rune(bustupName))
	if len(units) < bustupNameLength || bustupCharacterOffset+bustupCharacterLength > len(units) {
		return nil
	}

	key := string(utf16.Decode(units[bustupCharacterOffset:bustupCharacterOffset+bustupCharacterLength])) +
		string(utf16.Decode(units[bustupVariantIndex:bustupVariantIndex+1]))

	for index := range c.fixedPositions {
		if strings.EqualFold(c.fixedPositions[index].key, key) {
			return c.fixedPositions[index].positions
		}
	}

	return nil
}

func parseSelectWindow(document *IniDocument) SelectWindowConfiguration {
	designs := make([]SelectDesign, 0, MaximumSelectDesigns)
	for index := 0; index < MaximumSelectDesigns; index++ {
		suffix := strconv.Itoa(index)
		item := document.Get(SelectWindowSection, "design"+suffix+" item", "")
		if len(item) == 0 {
			break
		}

		textX, textY := parsePoint(document.Get(SelectWindowSection, "design"+suffix+" text pos", ""))
		fallback := DefaultSelectDesign()
		designs = append(designs, SelectDesign{
			Item:         item,
			Interval:     document.GetInt(SelectWindowSection, "design"+suffix+" interval", 10),
			TextX:        textX,
			TextY:        textY,
			Alignment:    document.GetInt(SelectWindowSection, "design"+suffix+" alignment", SelectDesignAlignCenter),
			DefaultColor: parseColor(document.Get(SelectWindowSection, "design"+suffix+" default color", ""), fallback.DefaultColor),
			AlreadyColor: parseColor(document.Get(SelectWindowSection, "design"+suffix+" already color", ""), fallback.AlreadyColor),
			SelectColor:  parseColor(document.Get(SelectWindowSection, "design"+suffix+" select color", ""), fallback.SelectColor),
			HeaderFooter: document.GetInt(SelectWindowSection, "design"+suffix+" header/footer", 0) != 0,
		})
	}

	return NewSelectWindowConfiguration(
		document.GetInt(SelectWindowSection, "vertical datum line", SelectWindowDefaultDatumLine),
		document.GetInt(SelectWindowSection, "option2 auto type", 0),
		designs)
}

func parseFixedPositions(document *IniDocument) []fixedPosition {
	var positions []fixedPosition
	for _, entry := range document.Section(FreeSightSection) {
		if len(entry.Key) <= len(fixedPositionPrefix) || !strings.EqualFold(entry.Key[:len(fixedPositionPrefix)], fixedPositionPrefix) {
			continue
		}

		suffix := strings.TrimSpace(entry.Key[len(fixedPositionPrefix):])
		if len(suffix) == 0 {
			continue
		}

		parsed := ParsePositions(entry.Value)
		if len(parsed) == 0 {
			continue
		}

		positions = setFixedPosition(positions, suffix, parsed)
	}

	return positions
}

func setFixedPosition(positions []fixedPosition, key string, values []int) []fixedPosition {
	for index := range positions {
		if strings.EqualFold(positions[index].key, key) {
			positions[index].positions = values
			return positions
		}
	}

	return append(positions, fixedPosition{key: key, positions: values})
}

// parseFreeSightScreen 读取 [free-sight screen]。
//
// 这一段是自由视角画面自己的布局：滑杆轨道、滑块和三个定位按钮。
// 全部按配置解析，作品不同就换素材，不用改代码。
func parseFreeSightScreen(document *IniDocument) FreeSightScreen {
	screen := FreeSightScreen{
		Mode:           document.GetInt(FreeSightSection, "mode", 0) != 0,
		FixedPositions: ParsePositions(document.Get(FreeSightSection, "fixed position", "")),
		Knob:           parseUIButton(document.Get(FreeSightSection, "knob", "")),
		Face:           parseUIButton(document.Get(FreeSightSection, "face button", "")),
		Bust:           parseUIButton(document.Get(FreeSightSection, "bust button", "")),
		Hip:            parseUIButton(document.Get(FreeSightSection, "hip button", "")),
	}

	// meter region 写的是"左, 上, 右, 下"，这里换算成左上角与尺寸，
	// 界面层直接拿来画轨道，不用再自己做减法。
	region := ParsePositions(document.Get(FreeSightSection, "meter region", ""))
	if len(region) >= 4 {
		left, top, right, bottom := region[0], region[1], region[2], region[3]
		width := right - left
		height := bottom - top
		if width < 0 {
			width = 0
		}
		if height < 0 {
			height = 0
		}
		screen.Meter = [4]int{left, top, width, height}
	}

	return screen
}

// ExtraWindowSection 是 bsxx.cfg 里的 [extra window] 段名。
const ExtraWindowSection = "extra window"

// LevelmeterWindowSection 是 [levelmeter window] 段名。
//
// UK（裏教師 2001）没有 [extra window]，而是用这一段描述"等级 + 圆形计量表"：
// 底图 lvmt_base、等级图 lvmt_no（Lv0…Lv6）、计量表 lvmt_meter（13 档）。
// 两段的数据形状相同，解析成同一个结构，绘制代码只认 ExtraWindowScreen。
const LevelmeterWindowSection = "levelmeter window"

// FollowWindowSection 是 [follow window view] 段名。
const FollowWindowSection = "follow window view"

// ExtraWindowScreen 是 [extra window] / [levelmeter window] 的布局。
//
// 两种版式共用这个结构：
//   - 等级/计量表（UK、UKIE、GK2IEDL）：LevelImage / MeterImage 各裁一帧；
//   - 多角色欲望度（CS）：DesireVariables + Infos + extra_bar 渐变条。
//
// 鼠标进入 ShowArea 时在 Base 处贴底图；零值语义：Base 为空表示作品没有
// 该面板，Frames 为 0 表示不画。
type ExtraWindowScreen struct {
	// Base 是底图图名；空表示未配置。
	Base string
	// BaseX/BaseY 是底图左上角坐标。
	BaseX int
	BaseY int
	// ShowArea 是触发显示的矩形，格式为 x, y, 宽, 高。
	ShowArea [4]int
	// LevelImage 是等级图（竖排帧）；LevelFrames 为帧数。
	LevelImage  string
	LevelFrames int
	// LevelX/LevelY 是等级帧相对 BaseX/BaseY 的偏移。配置里没写坐标时为 0，
	// UKIE 的 extra_level 就是整张七帧图直接叠在底图原点上。
	LevelX int
	LevelY int
	// MeterImage 是计量表图（竖排帧）；MeterFrames 为帧数。
	MeterImage  string
	MeterFrames int
	// MeterX/MeterY 是计量表帧相对底图的偏移。
	MeterX int
	MeterY int
	// AutoHideMS > 0 时，指针离开显示区后经过该毫秒数自动隐藏；0 表示不自动隐藏。
	AutoHideMS int
	// LevelProperty 是读取等级的变量名，默认 #hearts。
	// 剧本用 `set levelmeter #point` 绑定变量后会覆盖它（见 Session）。
	LevelProperty string
	// FollowsMessageWindow 表示 [follow window view] mesg = game：
	// 面板跟随对话框的显示状态，对话框收起时一起收起。
	FollowsMessageWindow bool

	// DesireVariables 是 [extra window] desire 列出的属性名（香純/綾/…）。
	//
	// 这一版式与 UK 的 level/meter 不同：底图只画标题，每一行的人物头像、
	// 名称和"desire for conquest"字样由 infoN 图提供，数值由 extra_bar
	// 的裁剪宽度和右侧百分比文本表示。没有 desire 行的作品仍走 level/meter。
	DesireVariables []string
	// MaxValue 是欲望度满值（[extra window] max value），默认 100。
	MaxValue int
	// PlayTimeMS 是原引擎的条动画时长；当前只用于保持配置完整。
	PlayTimeMS int
	// NumberImage/NumberFrames 是 0-9 数字条（exwindow_number）。
	NumberImage  string
	NumberFrames int
	// UnitImage/UnitFrames 是 "%"/"MAX" 两帧图（exwindow_unit）。
	UnitImage  string
	UnitFrames int
	// Infos 是 info1..infoN 与对应 bar 的布局，按 info 编号从小到大排列。
	Infos []ExtraWindowInfoRow
}

// ExtraWindowInfoRow 是 [extra window] 的一行欲望度布局。
//
// infoN 提供整行底图（头像 + 名称 + 深色轨道），bar 提供 150x12 的
// 绿→红渐变。原引擎按数值裁剪渐变条的左段，再在条中央画百分比文本。
type ExtraWindowInfoRow struct {
	Image  string
	Frames int
	X      int
	Y      int

	BarImage  string
	BarFrames int
	BarX      int
	BarY      int
}

// HasDesireMeter 判断这个面板是不是"多角色欲望度"版式。
func (w ExtraWindowScreen) HasDesireMeter() bool {
	return len(w.DesireVariables) > 0 && len(w.Infos) > 0
}

func parseExtraWindowScreen(document *IniDocument) ExtraWindowScreen {
	// 先找 [extra window]；UK 等旧作只有 [levelmeter window]，两段一样解析。
	section := ExtraWindowSection
	if !document.Has(ExtraWindowSection, "base") && document.Has(LevelmeterWindowSection, "base") {
		section = LevelmeterWindowSection
	}

	screen := ExtraWindowScreen{
		Base:          document.Get(section, "base", ""),
		AutoHideMS:    max(0, document.GetInt(section, "auto hide", 0)),
		LevelProperty: "#hearts",
	}
	screen.BaseX, screen.BaseY = parsePoint(document.Get(section, "base pos", ""))

	if area := ParsePositions(document.Get(section, "show area", "")); len(area) >= 4 {
		screen.ShowArea = [4]int{area[0], area[1], area[2], area[3]}
	}

	// level/meter 写的是"图名, 帧数[, x, y]"：UK 的 levelmeter 带坐标，
	// UKIE 的 extra window 只写图名和帧数（坐标默认 0）。
	screen.LevelImage, screen.LevelFrames, screen.LevelX, screen.LevelY =
		parseExtraWindowStrip(document.Get(section, "level", ""))
	screen.MeterImage, screen.MeterFrames, screen.MeterX, screen.MeterY =
		parseExtraWindowStrip(document.Get(section, "meter", ""))

	// [extra window] 的多角色版式（CS）：desire 列出每个角色一条属性，
	// infoN 画整行底图，infoN bar 画渐变条。数字/单位图没有坐标，
	// 原引擎把它们缩放后居中画在渐变条上（见 session/extrawindow.go）。
	screen.DesireVariables = strutil.SplitTrimNonEmpty(document.Get(section, "desire", ""), ",")
	if len(screen.DesireVariables) > 0 {
		screen.MaxValue = document.GetInt(section, "max value", 100)
		if screen.MaxValue <= 0 {
			screen.MaxValue = 100
		}
		screen.PlayTimeMS = max(0, document.GetInt(section, "play time", 0))
		screen.NumberImage, screen.NumberFrames, _, _ =
			parseExtraWindowStrip(document.Get(section, "number font", ""))
		screen.UnitImage, screen.UnitFrames, _, _ =
			parseExtraWindowStrip(document.Get(section, "unit", ""))
		screen.Infos = parseExtraWindowInfoRows(document, section)
	}

	// [follow window view] mesg = game：信息窗与等级面板跟随对话框的
	// 显示状态（UKIE 写了这一段，其它作品不一定有）。
	screen.FollowsMessageWindow = strings.EqualFold(
		strings.TrimSpace(document.Get(FollowWindowSection, "mesg", "")), "game")

	return screen
}

// parseExtraWindowInfoRows 收集 infoN / infoN bar。
//
// key 形如 "info1"、"info1 bar"。编号决定行顺序（不是配置里的书写顺序），
// 因为原引擎按下标 1..N 逐行绘制；缺失的行留空，不把后面的行上移。
func parseExtraWindowInfoRows(document *IniDocument, section string) []ExtraWindowInfoRow {
	rows := map[int]*ExtraWindowInfoRow{}
	maxIndex := 0

	for _, entry := range document.Section(section) {
		key := strings.ToLower(strings.TrimSpace(entry.Key))
		if !strings.HasPrefix(key, "info") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(key, "info"))
		isBar := false
		if strings.HasSuffix(rest, " bar") {
			isBar = true
			rest = strings.TrimSpace(strings.TrimSuffix(rest, " bar"))
		}
		index, ok := parseInt(rest)
		if !ok || index <= 0 {
			continue
		}
		row := rows[index]
		if row == nil {
			row = &ExtraWindowInfoRow{}
			rows[index] = row
		}
		image, frames, x, y := parseExtraWindowStrip(entry.Value)
		if isBar {
			row.BarImage, row.BarFrames, row.BarX, row.BarY = image, frames, x, y
		} else {
			row.Image, row.Frames, row.X, row.Y = image, frames, x, y
		}
		maxIndex = max(maxIndex, index)
	}

	result := make([]ExtraWindowInfoRow, 0, maxIndex)
	for index := 1; index <= maxIndex; index++ {
		if row := rows[index]; row != nil {
			result = append(result, *row)
			continue
		}
		result = append(result, ExtraWindowInfoRow{})
	}
	return result
}

// parseExtraWindowStrip 解析"图名, 帧数[, x, y]"。
//
// 帧数缺失或非正数时整条图视为不存在；坐标缺省为 0，正好覆盖 UKIE
// （level = extra_level, 7）与 UK（level = lvmt_no, 7, 0, 0）两种写法。
func parseExtraWindowStrip(value string) (image string, frames int, x int, y int) {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) < 2 || len(parts[0]) == 0 {
		return "", 0, 0, 0
	}
	parsed, ok := parseInt(parts[1])
	if !ok || parsed <= 0 {
		return "", 0, 0, 0
	}
	if len(parts) > 2 {
		x = parseCoordinate(parts[2])
	}
	if len(parts) > 3 {
		y = parseCoordinate(parts[3])
	}
	return parts[0], parsed, x, y
}

func parseColor(value string, fallback uint32) uint32 {
	text := strings.TrimSpace(value)
	if len(text) >= 2 && strings.EqualFold(text[:2], "0x") {
		text = text[2:]
	}

	if len(text) == 0 {
		return fallback
	}

	var parsed uint64
	for index := 0; index < len(text); index++ {
		digit, ok := hexDigit(text[index])
		if !ok {
			return fallback
		}

		parsed = parsed<<4 | uint64(digit)
		if parsed > 0xFFFFFFFF {
			return fallback
		}
	}

	return uint32(parsed)
}

func hexDigit(character byte) (uint32, bool) {
	switch {
	case character >= '0' && character <= '9':
		return uint32(character - '0'), true
	case character >= 'a' && character <= 'f':
		return uint32(character-'a') + 10, true
	case character >= 'A' && character <= 'F':
		return uint32(character-'A') + 10, true
	}

	return 0, false
}

func ParsePositions(value string) []int {
	positions := make([]int, 0, 4)
	if len(value) == 0 {
		return positions
	}

	for _, part := range strutil.SplitTrimNonEmpty(value, ",") {
		positions = append(positions, parseCoordinate(part))
	}

	return positions
}

func parsePoint(value string) (int, int) {
	parts := strutil.SplitTrim(value, ",")
	if len(parts) >= 2 {
		return parseCoordinate(parts[0]), parseCoordinate(parts[1])
	}

	return 0, 0
}

// parseCoordinate 解析配置里的坐标表达式。
//
// bsxx.cfg 大量使用 "28+140*3" 这种算术写法（槽位/缩略图网格），
// 早期实现只是把数字相加，乘号被忽略，结果第 4 列之后的槽位全部叠在左上角：
// 存档格子看起来正常（基图自带网格），但缩略图/命中区会错位。
// 这里用一个小型递归下降解析器，支持 + - * / 和括号，优先级与 C 一致。
func parseCoordinate(value string) int {
	parser := coordParser{text: value}
	result := parser.expression()
	return result
}

type coordParser struct {
	text string
	pos  int
}

func (p *coordParser) skipSpaces() {
	for p.pos < len(p.text) {
		switch p.text[p.pos] {
		case ' ', '\t':
			p.pos++
		default:
			return
		}
	}
}

func (p *coordParser) expression() int {
	value := p.term()
	for {
		p.skipSpaces()
		if p.pos >= len(p.text) {
			return value
		}
		switch p.text[p.pos] {
		case '+':
			p.pos++
			value += p.term()
		case '-':
			p.pos++
			value -= p.term()
		default:
			return value
		}
	}
}

func (p *coordParser) term() int {
	value := p.factor()
	for {
		p.skipSpaces()
		if p.pos >= len(p.text) {
			return value
		}
		switch p.text[p.pos] {
		case '*':
			p.pos++
			value *= p.factor()
		case '/':
			p.pos++
			divisor := p.factor()
			if divisor != 0 {
				value /= divisor
			}
		case '%':
			// 工作画面的六格按钮用 `240 + 128 * (i % 2)` 排成两列。
			// 不支持 % 时 3 号之后的 X 会被算成 496/624/…，按钮跑到
			// 相邻角色面板甚至屏幕外。
			p.pos++
			divisor := p.factor()
			if divisor != 0 {
				value %= divisor
			}
		default:
			return value
		}
	}
}

func (p *coordParser) factor() int {
	p.skipSpaces()
	sign := 1
	for p.pos < len(p.text) {
		if p.text[p.pos] == '-' {
			sign = -sign
			p.pos++
			p.skipSpaces()
			continue
		}
		if p.text[p.pos] == '+' {
			p.pos++
			p.skipSpaces()
			continue
		}
		break
	}
	if p.pos < len(p.text) && p.text[p.pos] == '(' {
		p.pos++
		value := p.expression()
		p.skipSpaces()
		if p.pos < len(p.text) && p.text[p.pos] == ')' {
			p.pos++
		}
		return sign * value
	}
	value, digits := 0, 0
	for p.pos < len(p.text) && p.text[p.pos] >= '0' && p.text[p.pos] <= '9' {
		value = value*10 + int(p.text[p.pos]-'0')
		p.pos++
		digits++
	}
	if digits == 0 {
		// 非数字字符（例如注释残留）：跳过它，避免调用方陷入死循环。
		if p.pos < len(p.text) {
			p.pos++
		}
		return 0
	}
	return sign * value
}

func parseCounter(value string) (string, int, int, int) {
	parts := strutil.SplitTrim(value, ",")
	if len(parts) < 4 || len(parts[0]) == 0 {
		return "", 0, 0, 0
	}

	var frames int
	if parsed, ok := parseInt(parts[1]); ok {
		frames = parsed
	}

	// 坐标和布局里的其它值一样可能是算术表达式（"867-5"），
	// 用 parseCoordinate 而不是 parseInt，避免算式被静默当成 0。
	x := parseCoordinate(parts[2])
	y := parseCoordinate(parts[3])

	if frames > 0 {
		return parts[0], frames, x, y
	}

	return "", frames, x, y
}

func clampVolume(value float64) float64 {
	if value > 0 && value <= 1 {
		return value
	}

	return 1.0
}
