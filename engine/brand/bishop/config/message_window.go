package config

import "kizuna/engine/core/strutil"

// message_window.go 解析 [message window] 与 [default font]。
//
// 对话框在原引擎里不是脚本画出来的：底图、名字/正文区域、催促图标的位置都来自
// 这两段配置，文字本身由引擎按字体渲染。我们沿用同一份数据，换作品时只换素材。
// 角色名颜色和正文区域都按"相对于 base pos"的坐标保存，渲染器加上 base 偏移即可。

// MessageWindow 是 [message window] 的布局。
type MessageWindow struct {
	// Base 是对话框底图（msg_base）；BaseX/BaseY 是它在屏幕上的左上角。
	Base string
	// BasePosSet 记录配置里到底有没有 `base pos`。
	// 原引擎是底部锚定的：bsxx.cfg 的 `0,768-213` 只是"屏幕高 - 底图高"的
	// 显式写法；SL（BSXScript 2.0）的配置根本不写这一项，此时不能用 0,0。
	BasePosSet bool
	BaseX      int
	BaseY      int
	// BaseOpaque 表示底图是一张"只有形状、没有 alpha"的窗口图，必须按不透明绘制。
	//
	// すくーるヘブン 初回限定版的 message_back 是 8 位索引图，调色板的 alpha 全 0，
	// 但索引值 0 表示透明、1 以上表示窗口本体（原引擎按索引判断形状）。
	// 按普通 RGBA 路径画会得到全透明，玩家看到的对话框只剩边框花纹；
	// 打开这个开关后引擎忽略 alpha 直接铺底，正文颜色也随之改成深色。
	BaseOpaque bool
	// FillBase 表示底图只有边框/装饰，窗口本体是透明的，必须先用 bg color
	// 把底图矩形填满再叠装饰。牝教師2 的 message.bsg 就是这样：像素 alpha
	// 全 0，只有 MENU/VIEW 标签有实体；不填底色时对话框整块看不见，
	// 玩家只能看到飘在场景上的按钮。
	FillBase  bool
	Speed     int
	BgColor   uint32
	NameX     int
	NameY     int
	NameW     int
	NameH     int
	NameColor uint32
	// NameBase 是角色名牌的底图（放課後2 的 msg_name：白底蓝框的牌子）。
	// 旧式作品把名字直接画在对话框底图上，没有独立名牌；留空时行为不变。
	// NameBaseX/NameBaseY 相对对话框原点，允许名牌落在窗口上沿之外。
	NameBase  string
	NameBaseX int
	NameBaseY int
	// FillRect 是"只填窗口内部"的半透明衬底（相对对话框原点）。
	// 放課後2 的 message 只有圆角边框、内部全透明，原版用 message_back
	// 的白色 mask 铺一层 65% 黑；FillRect 的宽/高为 0 时完全不生效。
	FillRect  [4]int
	FillColor uint32
	// FillMask 是比 FillRect 更精确的衬底：给一张白色形状图的名字，
	// 渲染时按 mask 的像素铺 FillColor（原版 message_back/msg_info_back）。
	// 设置后优先于 FillRect；零值对其它作品没有影响。
	FillMask string
	TextX    int
	TextY    int
	TextW    int
	TextH    int
	// TextColor/TextColorSet 是正文颜色。零值沿用旧规则（按底色亮度自动选
	// 黑/白）；浅色底图但 bg color 为 0 的作品（ヒメドレイ 的银灰对话框）
	// 需要显式指定深色字。
	TextColor       uint32
	TextColorSet    bool
	Unagashi        string
	UnagashiX       int
	UnagashiY       int
	UnagashiCellW   int
	UnagashiCellH   int
	UnagashiColumns int
	UnagashiFrames  int
	UnagashiSpeed   int

	// SpeakerBrackets 表示角色名要用【】包起来再画。
	// 旧式 Bishop 作品（学園2 等）的脚本名字表里只有「香織」，原引擎在
	// 对话框里显示的是【香織】；现代作品的 bsxx.cfg 直接给出带括号的名字，
	// 所以这个开关只由内置布局打开。
	SpeakerBrackets bool

	// SpeakerInline 表示角色名和正文排在同一行：名字先画在 TextX/TextY，
	// 正文从名字右侧开始并整体缩进（后续换行也对齐到正文起点）。
	//
	// 学園（GK）的对话框就是这样：手册截图里「【瑞希】「……」」的第一行
	// 后面跟着正文，第二行仍与正文起点对齐。传统的 NameY/TextY 两行布局
	// 会把名字单独占一行，和原版差一行；只在显式打开时生效。
	SpeakerInline bool

	// SubWindow 是对话框左侧的日期/时间小窗（学園2 的「サブウィンドウ」）。
	// 它不是独立的信息窗：只有对话框可见时才画，关窗/地图界面不会留下
	// 一块悬空的日期。数值由 Session 在每帧合成前写入。
	SubWindow MessageSubWindow

	// MultiMessages 是同时可显示的发言框数（[message window] multi messages）。
	// 本作是 2：主框 + 第二框。
	MultiMessages int
	// 第二框（mm name/mm text）的名字/正文区域与颜色。
	MMNameX     int
	MMNameY     int
	MMNameW     int
	MMNameH     int
	MMNameColor uint32
	MMNameAlign int
	MMTextX     int
	MMTextY     int
	MMTextW     int
	MMTextH     int
	MMInterval  int
}

// MessageSubWindow 是对话框左侧的日期/时间小窗（学園2 的「サブウィンドウ」）。
//
// 原引擎把三行信息画在这个小窗里：
//   - 「第N週 （X）」：周数与星期，由引擎根据 #day 换算后用字体绘制；
//   - 时段：「朝 / 放課後-残4 / 夜…」，素材 date_info 的 0..11 帧；
//   - 地点：「教室1F / 保健室…」，同一张 date_info 的后续帧。
//
// 地点行的编号是原引擎内部的 place 编号，和剧本的 #place 并不一致
// （#place 是背景/地点的内部 ID，同一地点的不同场景会复用），所以这一行
// 先不画，避免显示错名字。日期和时段是玩家判断进度必需的信息。
type MessageSubWindow struct {
	Enabled bool
	// X/Y/Width 是小窗的绘制区域（相对对话框原点），日期文字在其中居中。
	X     int
	Y     int
	Width int
	// DayVariable/TimeVariable 是要读取的脚本变量名。
	DayVariable  string
	TimeVariable string
	// DayText 为真时把日期画成文字（第N週 （X））。
	DayText     bool
	DayY        int
	DayFontSize int
	DayColor    uint32
	// TimeImage 是时段图（date_info），按 #time 取帧。
	TimeImage  string
	TimeFrames int
	TimeX      int
	TimeY      int
	// TimeZero 表示 #time=0 也要显示。
	TimeZero bool
	// TimeOffset 是取图时的修正量：学園2 的 date_info 从「朝」开始，
	// 而剧本的 #time=2 才是早上（#time=1 是开场演出），所以帧号 = #time-2。
	TimeOffset int
}

// Empty 判断这个作品是否配置了对话框。
func (m MessageWindow) Empty() bool {
	return len(m.Base) == 0
}

// MultiEnabled 判断作品是否真的定义了第二消息框。
// 只看 mm 坐标：内置布局没有这两项。BSScript 的 window part=2 是关闭按钮，
// 不能靠它启用，由调用方另行排除。
func (m MessageWindow) MultiEnabled() bool {
	return m.MMTextW > 0 || m.MMNameW > 0
}

// FontSettings 是 [default font] 的字体参数。
type FontSettings struct {
	Size         int
	Width        int
	Height       int
	Weight       int
	Edge         int
	Shadow       int
	LineLength   int
	Lines        int
	FontFace     string
	EudcFontFace string
}

func parseMessageWindow(document *IniDocument) MessageWindow {
	window := MessageWindow{
		Base:      document.Get(MessageWindowSection, "base window", ""),
		Speed:     document.GetInt(MessageWindowSection, "speed", 100),
		BgColor:   parseColor(document.Get(MessageWindowSection, "bg color", "0x80000000"), 0x80000000),
		NameColor: parseColor(document.Get(MessageWindowSection, "name color", "0xffff8822"), 0xffff8822),
	}
	// base pos 通常写 `0,768-213`，也就是"屏幕高 - 底图高"；但 SL 这类
	// BSXScript 2.0 作品的配置完全不写它。必须把"没写"和"写了 0,0"区分开，
	// 否则下面渲染时会把它当成屏幕左上角，对话框跑到画面顶部去。
	window.BasePosSet = document.Has(MessageWindowSection, "base pos")
	window.BaseX, window.BaseY = parsePoint(document.Get(MessageWindowSection, "base pos", ""))

	if values := ParsePositions(document.Get(MessageWindowSection, "name", "")); len(values) >= 4 {
		window.NameX, window.NameY, window.NameW, window.NameH = values[0], values[1], values[2], values[3]
	}
	if values := ParsePositions(document.Get(MessageWindowSection, "text", "")); len(values) >= 4 {
		window.TextX, window.TextY, window.TextW, window.TextH = values[0], values[1], values[2], values[3]
	}

	// unagashi info 是 "格宽, 格高, 横方向图案数, 图案总数"。
	window.Unagashi = ""
	if parts := strutil.SplitTrimNonEmpty(document.Get(MessageWindowSection, "unagashi icon", ""), ","); len(parts) > 0 {
		window.Unagashi = parts[0]
	}
	window.UnagashiX, window.UnagashiY = parsePoint(document.Get(MessageWindowSection, "unagashi pos", ""))
	info := ParsePositions(document.Get(MessageWindowSection, "unagashi info", ""))
	if len(info) >= 4 {
		window.UnagashiCellW, window.UnagashiCellH, window.UnagashiColumns, window.UnagashiFrames = info[0], info[1], info[2], info[3]
	}
	window.UnagashiSpeed = document.GetInt(MessageWindowSection, "unagashi speed", 100)
	window.MultiMessages = document.GetInt(MessageWindowSection, "multi messages", 0)
	window.MMNameColor = parseColor(document.Get(MessageWindowSection, "mm name color", "0xffff8822"), 0xffff8822)
	window.MMNameAlign = document.GetInt(MessageWindowSection, "mm name align", 0)
	window.MMInterval = document.GetInt(MessageWindowSection, "mm interval", 0)
	if values := ParsePositions(document.Get(MessageWindowSection, "mm name", "")); len(values) >= 4 {
		window.MMNameX, window.MMNameY, window.MMNameW, window.MMNameH = values[0], values[1], values[2], values[3]
	}
	if values := ParsePositions(document.Get(MessageWindowSection, "mm text", "")); len(values) >= 4 {
		window.MMTextX, window.MMTextY, window.MMTextW, window.MMTextH = values[0], values[1], values[2], values[3]
	}

	return window
}

func parseFontSettings(document *IniDocument) FontSettings {
	font := FontSettings{
		Size:         document.GetInt(FontSection, "size", 17),
		Width:        document.GetInt(FontSection, "width", 26),
		Height:       document.GetInt(FontSection, "height", 26),
		Weight:       document.GetInt(FontSection, "weight", 400),
		Edge:         document.GetInt(FontSection, "edge", 0),
		Shadow:       document.GetInt(FontSection, "shadow", 0),
		LineLength:   document.GetInt(FontSection, "line length", 25),
		Lines:        document.GetInt(FontSection, "number of lines", 3),
		FontFace:     document.Get(FontSection, "fontface", ""),
		EudcFontFace: document.Get(FontSection, "eudcfontface", ""),
	}
	if font.Size <= 0 {
		font.Size = 17
	}
	if font.Height <= 0 {
		font.Height = font.Size + 9
	}
	if font.Lines <= 0 {
		font.Lines = 3
	}
	return font
}
