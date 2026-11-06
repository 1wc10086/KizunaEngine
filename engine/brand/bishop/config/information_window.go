package config

import (
	"strings"

	"kizuna/engine/core/strutil"
)

// InformationWindowSection 是 bsxx.cfg 里"行动信息窗"的配置段。
const InformationWindowSection = "information window"

// DigitStrip 描述一条"数字图"：一张竖排的数字条，每帧一个数字。
//
// 原引擎不画字体，而是把 0-9 预先做成竖排图，按值取帧贴上去。
// 这样换游戏不用带字体文件，数字风格也跟着素材走。
type DigitStrip struct {
	// Image 是数字条图片名。
	Image string
	// Frames 是数字条里的帧数（通常是 10，即 0..9）。
	Frames int
	// X、Y 是第一条数字的坐标；有底板时相对 base pos。
	X int
	Y int
	// Offset 是"下一桁"的横向偏移。原引擎从个位往高位画，
	// 所以这个值通常是负数（十位在个位左边）。
	Offset int
	// Start 是取帧时的固定偏移。特别病栋的 info_font 把 0-9 与
	// 月..日 排在同一条横图里，曜日要用 Start=10 才取到汉字。
	Start int
	// Horizontal 表示这是一张"横向排帧"的图（特别病栋的 info_font）。
	// 为 false 时保持原来的竖排切帧。
	Horizontal bool
	// AtlasDigits/AtlasDigitWidth/AtlasWideStart/AtlasWideWidth 描述
	// info_font 这种"变宽字形图"：前 AtlasDigits 个数字每格 AtlasDigitWidth，
	// 之后的月..日每格 AtlasWideWidth，宽字从 AtlasWideStart 开始。
	// 四者全为 0 时沿用 Horizontal 的等宽切帧。
	AtlasDigits     int
	AtlasDigitWidth int
	AtlasWideStart  int
	AtlasWideWidth  int
}

// InformationWindow 是行动信息窗（「行動開始から X日目」）的配置。
//
// 窗口屏幕位置由 base pos 决定（可能在右上角，例如 淫獄の放課後2）；
// 其余坐标（日期、单位、时段）都相对这个基点。
//
// 我们的引擎原本完全没有这个窗：剧情照常推进，但玩家看不到"现在是第几天、
// 什么时段"，而这在 Bishop 的作品里是关键的进度提示（很多分支按天解锁）。
type InformationWindow struct {
	// Base 是底板图片名（info_base）；空表示不画底板。
	Base string
	// BaseX、BaseY 是底板位置。
	BaseX int
	BaseY int
	// DayVariable 是日期变量名（例如 #day）；空表示不显示日期。
	// 原引擎同时接受 day 与 date 两个键，解析时会自动回退。
	DayVariable string
	// DayStrip 是日期数字条；DayStrip.Image 为空表示没有可用数字图。
	DayStrip DigitStrip
	// DayUnit 是"日目"这类单位图（info_unit）。
	DayUnit string
	// DayUnitX、DayUnitY 是单位图位置。
	DayUnitX int
	DayUnitY int
	// DaySpecial 是"日期进入特殊区间"时替换整个日期显示的图（info_sp）。
	DaySpecial string
	// DaySpecialX、DaySpecialY 是它的位置。
	DaySpecialX int
	DaySpecialY int
	// DayBorder 是"特殊日期"的门槛：日期值 >= 这个数时不再逐位画数字，
	// 而是整块换用 DaySpecial。原引擎用它表达"日期未知/尚未开始"这类状态
	// （变量被置成大数当哨兵），此时显示固定图案而不是一个荒唐的天数。
	DayBorder int
	// DayAdjustment 是显示用的偏移：画面显示 day + DayAdjustment。
	DayAdjustment int
	// TimeVariable 是时段变量名（例如 #time）；空表示不显示时段。
	TimeVariable string
	// TimeStrip 是时段图（info_time），按 TimeVariable 的值取帧。
	TimeStrip DigitStrip
	// TimeZoneLow、TimeZoneHigh 是时段的显示区间；变量值落在区间内才画。
	TimeZoneLow  int
	TimeZoneHigh int
	// WeekStrip / WeekDayStrip 是另一种日期表现：不画十进制数字，而是用
	// "N週"（week no）与"X曜日"（week day）两条竖排图。黒の教室 用的是这种
	// 版式——脚本只给 #day 一个从 1 开始的连续计数，周数和星期由引擎换算。
	// 图分别有 9 帧（1..9 週）和 6 帧（月..土，周日照常休息）。
	WeekStrip    DigitStrip
	WeekDayStrip DigitStrip
	// WeekDays 是"一周几天"的换算基数。
	//
	// 黒の教室 用 6（月～土，周日休息）；KD 的作品把日期编码成
	// "周×10 + 曜日"（30 = 第3週日曜日），所以用 10。0 表示沿用旧的 6 天。
	WeekDays int
	// WeekStart 是日计数起点（0 或 1）。旧的 6 天版式从 1 开始；
	// KD 的 0 = オープニング，因此从 0 开始。
	WeekStart int
	// DayZero / TimeZero 表示 0 也是合法值。
	// 多数作品用 0 表示"还没开始"，但 KD 的 #day=0 是 オープニング、
	// #time=0 是「朝」，必须显示出来。
	DayZero  bool
	TimeZero bool
	// WeekDayFrom 是"从第几天开始显示曜日"。
	// KD 的 #day < 10 是 オープニング；底栏只放得下"オープニング"，
	// 再叠"日曜日"会互相压字，所以从第 10 天（第1週）才画曜日。
	WeekDayFrom int
	// FollowsMessageWindow 表示 [follow window view] mesg = game：
	// 信息窗跟随对话框的显示状态。CS 把这块小窗放在画面右下角，
	// 进地图/选择态时原版不显示它；没有这一段的作品维持原来的可见性。
	FollowsMessageWindow bool
	// Labels 是信息窗上的固定字牌。放課後2 的「第 / 週 / 曜 / 日」四个字
	// 是 msg_info_str1 的四帧；底图只画空框，字必须由引擎叠上去。
	// 注意：Labels 用的是绝对屏幕坐标，不像 WeekStrip 那样叠加 base pos。
	Labels []UIButton
	// WeekendFirst/WeekendSecond 是「週末」两帧（msg_info_str2 的第 15/16 帧）。
	// 一周最后一天不再画「曜/日」两个字牌，而是在 WeekDayStrip 与第二个字牌
	// 的位置画「週」「末」。WeekendReplacesLabels 为真时跳过 Labels 末两位。
	WeekendFirst          DigitStrip
	WeekendSecond         DigitStrip
	WeekendReplacesLabels bool
	// FillRect/FillColor 是窗口内部的半透明衬底（绝对屏幕坐标）。
	// 放課後2 的 msg_info 只有边框、内部透明，不垫底时数字会直接压在 CG 上。
	// 宽/高为 0 时不生效。
	FillRect  [4]int
	FillColor uint32
	// FillMask 是白色形状图（msg_info_back），按像素铺 FillColor，
	// 比 FillRect 更贴近原版的圆角窗口；设置后优先于 FillRect。
	FillMask string
}

// Empty 判断配置是否可用；不可用时调用方应完全跳过这个窗。
func (w InformationWindow) Empty() bool {
	return w.Base == "" && w.DayStrip.Image == "" &&
		w.WeekStrip.Image == "" && w.WeekDayStrip.Image == "" &&
		w.TimeStrip.Image == ""
}

func parseInformationWindow(document *IniDocument) InformationWindow {
	return parseInformationWindowForScreen(document, 0)
}

// parseInformationWindowForScreen 解析信息窗；screenHeight > 0 时启用坐标修正。
// 修正只针对 UK 这类绝对坐标写法，见函数末尾的说明。
func parseInformationWindowForScreen(document *IniDocument, screenHeight int) InformationWindow {
	// 日期变量在不同 Bishop 作品里可能写成 day 或 date（同一含义的别名）；
	// 只认 day 会让只写 date 的作品完全没有日期。
	dayVariable := document.Get(InformationWindowSection, "day", "")
	if len(dayVariable) == 0 {
		dayVariable = document.Get(InformationWindowSection, "date", "")
	}
	window := InformationWindow{
		Base:         document.Get(InformationWindowSection, "base", ""),
		DayVariable:  dayVariable,
		DayUnit:      "",
		TimeVariable: document.Get(InformationWindowSection, "time", ""),
	}
	window.BaseX, window.BaseY = parsePoint(document.Get(InformationWindowSection, "base pos", ""))

	window.DayStrip = parseDigitStrip(document.Get(InformationWindowSection, "day no", ""))
	// 数字条的"下一桁偏移"是单独一项，不在 day no 那行里。
	window.DayStrip.Offset = document.GetInt(InformationWindowSection, "day no offset", 0)
	// 満淫電車2 这一代把日期/时段写成 `day image` + `day pos` + `day info`
	// （单元格宽、高、横向帧数、总帧数），变量名固定是 #day/#time。
	if window.DayStrip.Image == "" {
		if image := document.Get(InformationWindowSection, "day image", ""); image != "" {
			strip := DigitStrip{Image: image}
			strip.X, strip.Y = parsePoint(document.Get(InformationWindowSection, "day pos", ""))
			if info := ParsePositions(document.Get(InformationWindowSection, "day info", "")); len(info) >= 4 {
				strip.Frames = info[3]
			}
			strip.Offset = document.GetInt(InformationWindowSection, "day space", 0)
			window.DayStrip = strip
		}
	}
	if window.TimeVariable == "" {
		window.TimeVariable = "#time"
	}
	if window.DayVariable == "" && window.DayStrip.Image != "" {
		window.DayVariable = "#day"
	}
	// 黒の教室 的版式：week no / week day 与 day no 互斥，有它们时不再画十进制日期。
	window.WeekStrip = parseDigitStrip(document.Get(InformationWindowSection, "week no", ""))
	window.WeekDayStrip = parseDigitStrip(document.Get(InformationWindowSection, "week day", ""))
	// 缺省时取曜日图的帧数：UK 的曜日图是 7 帧（日..土），仍按 6 换算会算错周数。
	weekDays := document.GetInt(InformationWindowSection, "week days", 0)
	if weekDays <= 0 {
		weekDays = 6
		if window.WeekDayStrip.Frames > 1 {
			weekDays = window.WeekDayStrip.Frames
		}
	}
	window.WeekDays = weekDays
	window.WeekStart = 1
	// 7 帧曜日图（日..土）的作品以星期日为第 0 天：UK 的脚本自己就是
	// `#week_no = #day / 7`、`#week_day = #day % 7`，所以第 1 天是月曜日
	// （frame 1）。沿用 1 起算会把每一天的曜日都挪前一天。
	if weekDays == 7 {
		window.WeekStart = 0
	}
	if document.Has(InformationWindowSection, "week start") {
		window.WeekStart = document.GetInt(InformationWindowSection, "week start", 1)
	}
	window.DayZero = document.GetInt(InformationWindowSection, "show day zero", 0) != 0
	window.TimeZero = document.GetInt(InformationWindowSection, "show time zero", 0) != 0
	window.WeekDayFrom = document.GetInt(InformationWindowSection, "week day from", 0)

	window.DayBorder = document.GetInt(InformationWindowSection, "day sp border", 1<<30)
	window.DayAdjustment = document.GetInt(InformationWindowSection, "day adjustment", 0)
	// [follow window view] mesg = game：信息窗和欲望度面板一样跟随对话框。
	// CS 把这块小窗放在右下角，地图/选择态下原版不显示；只对明确写了
	// 这一段的作品生效，其它作品保持原来的"常驻"行为。
	window.FollowsMessageWindow = strings.EqualFold(
		strings.TrimSpace(document.Get(FollowWindowSection, "mesg", "")), "game")

	unit := strutil.SplitTrimNonEmpty(document.Get(InformationWindowSection, "day unit", ""), ",")
	if len(unit) > 0 {
		window.DayUnit = unit[0]
		if len(unit) > 2 {
			window.DayUnitX = parseCoordinate(unit[2])
		}
		if len(unit) > 3 {
			window.DayUnitY = parseCoordinate(unit[3])
		}
	}

	special := strutil.SplitTrimNonEmpty(document.Get(InformationWindowSection, "day sp", ""), ",")
	if len(special) > 0 {
		window.DaySpecial = special[0]
		if len(special) > 2 {
			window.DaySpecialX = parseCoordinate(special[2])
		}
		if len(special) > 3 {
			window.DaySpecialY = parseCoordinate(special[3])
		}
	}

	window.TimeStrip = parseDigitStrip(document.Get(InformationWindowSection, "time zone", ""))
	if window.TimeStrip.Image == "" {
		// 満淫電車2 把时段写成 `time image` + `time pos` + `time info`
		// （单元格宽、高、横向帧数、总帧数），变量是 1 基的 #time。
		if image := document.Get(InformationWindowSection, "time image", ""); image != "" {
			strip := DigitStrip{Image: image}
			strip.X, strip.Y = parsePoint(document.Get(InformationWindowSection, "time pos", ""))
			if info := ParsePositions(document.Get(InformationWindowSection, "time info", "")); len(info) >= 4 {
				strip.Frames = info[3]
			}
			window.TimeStrip = strip
		}
	}
	// 时段只在变量的值落在区间内时显示；没配就当成"任何值都显示"。
	rangeValues := ParsePositions(document.Get(InformationWindowSection, "time zone range", ""))
	if len(rangeValues) >= 2 {
		window.TimeZoneLow = rangeValues[0]
		window.TimeZoneHigh = rangeValues[1]
	} else {
		// 没有写 range 时按 0 基处理：渲染器用 value - TimeZoneLow 取帧，
		// 旧默认 -1<<30 会让 frame 变成一个天文数字、永远 clamp 到最后一帧
		// （教育指導 KS 的 info_time 就会一直显示"下校時刻"）。
		window.TimeZoneLow = 0
		window.TimeZoneHigh = 1 << 30
	}
	if window.TimeStrip.Image != "" && document.Has(InformationWindowSection, "time image") &&
		!document.Has(InformationWindowSection, "time zone range") {
		// #time 直接就是帧号：info_time 的第 0 帧是空白边框，第 1 帧
		// 才是 MORNING。1 基值减 1 会永远显示空白。
		window.TimeZoneLow = 0
		window.TimeZoneHigh = max(1, window.TimeStrip.Frames-1)
	}
	// UK 的 bsxx.cfg 把坐标写成了绝对屏幕值，UKIE 等作品则相对 base pos。
	// 渲染器对所有坐标都会加上 base pos，于是 UK 的 y 被推到屏幕外，周/曜日图看不见。
	// 判别方式：加上 base pos 越出屏幕、减去 base pos 落在屏内，就视作绝对坐标并还原。
	// 只修正 Y：UK 的 BaseX 为 0，X 方向本来就不受影响。
	if screenHeight > 0 && window.BaseY > 0 {
		absolute := func(y int) int {
			if y+window.BaseY >= screenHeight && y-window.BaseY >= 0 && y-window.BaseY < screenHeight {
				return y - window.BaseY
			}
			return y
		}
		window.DayStrip.Y = absolute(window.DayStrip.Y)
		window.WeekStrip.Y = absolute(window.WeekStrip.Y)
		window.WeekDayStrip.Y = absolute(window.WeekDayStrip.Y)
		window.TimeStrip.Y = absolute(window.TimeStrip.Y)
		window.DayUnitY = absolute(window.DayUnitY)
		window.DaySpecialY = absolute(window.DaySpecialY)
	}
	return window
}

// parseDigitStrip 解析 "图片名, 帧数, X, Y" 这种数字条配置。
func parseDigitStrip(value string) DigitStrip {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) < 4 || len(parts[0]) == 0 {
		return DigitStrip{}
	}
	strip := DigitStrip{Image: parts[0]}
	strip.Frames, _ = parseInt(parts[1])
	strip.X = parseCoordinate(parts[2])
	strip.Y = parseCoordinate(parts[3])
	return strip
}
