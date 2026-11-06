package config

import (
	"strconv"
	"strings"
)

// custom_map.go 解析 [custom map phase]。
//
// 这个段被两代作品用成两种东西：
//   - SLIE 的剧本自己用 IMAGE/IMAGE_EX 画完整套工作画面，这里只读
//     result/type 等变量名；
//   - SL（BSXScript 2.0）的剧本只画背景和 AM/PM 标题，角色面板、六格
//     工作按钮、体力/好感度条、进度条和 START/HELP/存档按钮全部由引擎
//     按本段配置绘制。
//
// 所以除了补画按钮，这里还解析 SL 的完整工作画面布局。坐标一律保留
// 原值，不做缩放：SL 是 1024x576，配置就是按这个分辨率写的。

// CustomMapPhase 是 [custom map phase] 的布局与按钮。
type CustomMapPhase struct {
	// BG / Base / Base2 是背景与两张整屏底图（Base=午前，Base2=午後）。
	BG    string
	Base  string
	Base2 string

	// Vitality / Love 是体力/好感度竖排图；Frames 是档数。
	Vitality UIButton
	Love     UIButton
	// Down 是体力归零时盖在角色面板上的 "Down..." 图。
	Down UIButton
	// StepBase 是工作进度条的底槽；Steps 是 2/3/4/5 段进度条。
	StepBase UIButton
	Steps    []UIButton
	// HelpImage 是 HELP 按钮打开的整屏说明图。
	HelpImage string
	// BlinkInterval 是进度条闪烁间隔（毫秒），目前只记录。
	BlinkInterval int
	// DisableOpacity 是工作不可用时压在按钮上的暗度（0..255）。
	DisableOpacity int

	// Positions 是角色面板内的固定坐标。
	WorkPositions []UIPoint
	BreakPos      UIPoint
	ExpPos        UIPoint
	VitalityPos   UIPoint
	LovePos       UIPoint

	// Characters 是 1..4 号角色的工作表。
	Characters []CustomMapCharacter

	// 引擎补画的按钮（绝对屏幕坐标）。
	Start  UIButton
	Help   UIButton
	QSave  UIButton
	QLoad  UIButton
	Save   UIButton
	Load   UIButton
	Config UIButton
	Log    UIButton
	// Item 是"状態/アイテム"入口（GK2IEDL 的 item button，学園2 的 status）。
	// 剧本自己画了按钮图、但没在 [button object] 里登记时，引擎仍要能打开
	// 状态画面，否则地图上会有一个点了没反应的图标。
	Item UIButton

	// TypeVariable 是 [custom map phase] type（#custom_map_type）。
	// SL 的剧本写 1 表示午前、0 表示午後；空表示不区分。
	TypeVariable string
	// ResultTable 是 [custom map phase] result table：按钮 ID → #result 值。
	//
	// OH/OHIE 的地图卡在 [button object] 里写的是"图片, 帧数"（OH 只写
	// 帧数），按钮值由这张表按按钮 ID 给出：1→1、2→2…5→5、6→0，
	// 下标 0 的 -1 是"没有按钮"的哨兵。OHIE 的 ID 写在主键第一列；
	// OH 没有显式 ID，由图名里的 map_chN / map_nexttime 推断（见
	// MapButtonNamedID）。没有这张表的作品仍沿用 [button object] 主键
	// 里的数值语义。
	ResultTable []int
	// TargetCards 表示这一屏的 type=160 按钮是教育指導 IE 的目标卡：
	// 剧本先用 IMAGE_EX 在 (0,0) 登记卡片，再用 LAYER_PARAM type=6/0
	// 按 #target.no 摆到计算出的卡位。引擎必须给每张卡分配连续层号，
	// 否则 MoveButtonByLayer 找不到它们，所有卡都堆在左上角。
	TargetCards bool
}

// CustomMapCharacter 是一个角色的工作画面数据。
type CustomMapCharacter struct {
	BaseX, BaseY int
	// AmNames / PmNames 是六个工作按钮的图片（含帧数）。
	AmNames [6]UIButton
	PmNames [6]UIButton
	// AmMax / PmMax 是每个工作当前的最大等级（2..5），决定用哪张
	// 进度条图（work_2step…work_5step）。
	AmMax [6]int
	PmMax [6]int
	// Break 是"休憩させる"按钮（值 99）。
	Break UIButton
}

// Empty 判断配置里有没有可用于补画/渲染的内容。
func (c CustomMapPhase) Empty() bool {
	if c.HasLayout() {
		return false
	}
	return c.Start.Empty() && c.Help.Empty() && c.QSave.Empty() && c.QLoad.Empty() &&
		c.Save.Empty() && c.Load.Empty() && c.Config.Empty() && c.Log.Empty()
}

// HasLayout 判断是否配置了 SL 风格的工作画面。
func (c CustomMapPhase) HasLayout() bool {
	return len(c.Characters) > 0 && (len(c.Base) > 0 || len(c.Base2) > 0)
}

// HasResultTable 表示这一代作品用按钮 ID 查表得到 #result，而不是
// 把 [button object] 的第一列直接当值。OH/OHIE 的地图必须走这条路。
func (c CustomMapPhase) HasResultTable() bool {
	return len(c.ResultTable) > 0
}

// ResultValue 按键 ID 取 #result 值。表是 1 基的：下标 0 是 -1 哨兵。
func (c CustomMapPhase) ResultValue(id int) (int, bool) {
	if id < 0 || id >= len(c.ResultTable) {
		return 0, false
	}
	return c.ResultTable[id], true
}

// WorkNames 返回某个角色在指定时段的六个工作按钮。
func (c CustomMapCharacter) WorkNames(am bool) [6]UIButton {
	if am {
		return c.AmNames
	}
	return c.PmNames
}

// WorkMax 返回某个角色在指定时段的六个工作最大等级。
func (c CustomMapCharacter) WorkMax(am bool) [6]int {
	if am {
		return c.AmMax
	}
	return c.PmMax
}

func parseCustomMapPhase(document *IniDocument) CustomMapPhase {
	phase := CustomMapPhase{
		BG:             document.Get(CustomMapSection, "bg", ""),
		Base:           document.Get(CustomMapSection, "base", ""),
		Base2:          document.Get(CustomMapSection, "base2", ""),
		Vitality:       parseUIButton(document.Get(CustomMapSection, "vitality", "")),
		Love:           parseUIButton(document.Get(CustomMapSection, "love", "")),
		Down:           parseUIButton(document.Get(CustomMapSection, "down", "")),
		StepBase:       parseUIButton(document.Get(CustomMapSection, "step base", "")),
		HelpImage:      document.Get(CustomMapSection, "help image", ""),
		BlinkInterval:  document.GetInt(CustomMapSection, "blink interval", 500),
		DisableOpacity: parseDisableOpacity(document.Get(CustomMapSection, "disable opacity", "")),
		Start:          parseUIButton(document.Get(CustomMapSection, "start", "")),
		Help:           parseUIButtonFirst(document, "help", "help button"),
		QSave:          parseUIButtonFirst(document, "qsave button", "quick save button"),
		QLoad:          parseUIButtonFirst(document, "qload button", "quick load button"),
		Save:           parseUIButtonFirst(document, "save button"),
		Load:           parseUIButtonFirst(document, "load button"),
		Config:         parseUIButtonFirst(document, "config button"),
		Log:            parseUIButtonFirst(document, "log button"),
		Item:           parseUIButtonFirst(document, "item button", "status button"),
		TypeVariable:   document.Get(CustomMapSection, "type", ""),
	}
	resultVariable := strings.TrimSpace(document.Get(CustomMapSection, "result", ""))
	selectObject := strings.TrimSpace(document.Get(CustomMapSection, "select object", ""))
	phase.TargetCards = strings.HasPrefix(strings.ToLower(resultVariable), "#target") || len(selectObject) > 0
	for _, part := range strings.Split(document.Get(CustomMapSection, "result table", ""), ",") {
		if value, ok := parseInt(strings.TrimSpace(part)); ok {
			phase.ResultTable = append(phase.ResultTable, value)
		}
	}
	if phase.DisableOpacity <= 0 {
		phase.DisableOpacity = 0x80
	}
	for index := 0; index < 4; index++ {
		phase.Steps = append(phase.Steps, parseUIButton(document.Get(CustomMapSection, "step"+strconv.Itoa(index), "")))
	}
	if phase.Steps[0].Empty() {
		phase.Steps = nil
	}
	for index := 0; index < 6; index++ {
		if point, ok := parseUIPoint(document.Get(CustomMapSection, "work"+strconv.Itoa(index+1)+" pos", "")); ok {
			phase.WorkPositions = append(phase.WorkPositions, point)
		}
	}
	if point, ok := parseUIPoint(document.Get(CustomMapSection, "break pos", "")); ok {
		phase.BreakPos = point
	}
	if point, ok := parseUIPoint(document.Get(CustomMapSection, "work exp pos", "")); ok {
		phase.ExpPos = point
	}
	if point, ok := parseUIPoint(document.Get(CustomMapSection, "vitality pos", "")); ok {
		phase.VitalityPos = point
	}
	if point, ok := parseUIPoint(document.Get(CustomMapSection, "love pos", "")); ok {
		phase.LovePos = point
	}
	phase.Characters = parseCustomMapCharacters(document)
	return phase
}

// parseCustomMapCharacters 读取 chN base pos / work name / break / max level。
//
// 键有两种写法：`ch1 work name am1`（带空格）和 `ch1_work_am1`（下划线，
// 只给最大等级）。两种都按 1 基角色编号解析。
func parseCustomMapCharacters(document *IniDocument) []CustomMapCharacter {
	characters := make([]CustomMapCharacter, 0, 4)
	for index := 1; index <= 4; index++ {
		character := CustomMapCharacter{}
		if point, ok := parseUIPoint(document.Get(CustomMapSection, "ch"+strconv.Itoa(index)+" base pos", "")); ok {
			character.BaseX, character.BaseY = point.X, point.Y
		}
		for slot := 1; slot <= 6; slot++ {
			character.AmNames[slot-1] = parseUIButton(document.Get(CustomMapSection,
				"ch"+strconv.Itoa(index)+" work name am"+strconv.Itoa(slot), ""))
			character.PmNames[slot-1] = parseUIButton(document.Get(CustomMapSection,
				"ch"+strconv.Itoa(index)+" work name pm"+strconv.Itoa(slot), ""))
			character.AmMax[slot-1] = document.GetInt(CustomMapSection,
				"ch"+strconv.Itoa(index)+"_work_am"+strconv.Itoa(slot), 0)
			character.PmMax[slot-1] = document.GetInt(CustomMapSection,
				"ch"+strconv.Itoa(index)+"_work_pm"+strconv.Itoa(slot), 0)
		}
		character.Break = parseUIButton(document.Get(CustomMapSection, "ch"+strconv.Itoa(index)+" break", ""))
		characters = append(characters, character)
	}
	return characters
}

func parseUIButtonFirst(document *IniDocument, keys ...string) UIButton {
	for _, key := range keys {
		if button := parseUIButton(document.Get(CustomMapSection, key, "")); !button.Empty() {
			return button
		}
	}
	return UIButton{}
}

func parseDisableOpacity(value string) int {
	if len(value) == 0 {
		return 0
	}
	if parsed, ok := ParseColor(value); ok {
		return int(parsed & 0xff)
	}
	return 0
}
