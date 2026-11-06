package config

import (
	"strconv"
	"strings"

	"kizuna/engine/core/strutil"
)

// MapPhaseSection 是 bsxx.cfg 里地图阶段的布局段（UK 裏教師、UKIE）。
const MapPhaseSection = "map phase"

// MapPhaseButton 是地图底部的一颗系统按钮（時間を進める/存档/读档/设置/回想）。
type MapPhaseButton struct {
	Tool   string // "next_time", "qsave", "qload", "save", "load", "config", "log", "status"
	Image  string
	Frames int // 竖排帧数；0 视为 1
	X, Y   int
}

// MapPhase 是 [map phase] 的布局。UK/UKIE 的 map.dat 驱动地图用它画
// 周数/曜日/时段与系统按钮；没有该段的作品保持零值。
type MapPhase struct {
	Background   string     // base
	HitImage     string     // hit table（map_hit）
	Overlay      string     // base on：通常状态的路线标记层（map_on）
	OverlayHover string     // base over：鼠标悬停时的路线标记层（map_over）
	Frame        UIButton   // frame：地图边框/DAY 卡片（map_frame[, frames, x, y]）
	WeekNo       DigitStrip // week no
	WeekDay      DigitStrip // week day
	// WeekDayShift 是脚本的曜日编号与素材帧序之间的偏移。
	// 多数作品 0=日曜日，而牝教師（MK/MKIE）的 map_week 是从 MONDAY 开始的
	// 7 帧；不补 -1 时周一会被画成周日、整周错一天。零值保持旧行为。
	WeekDayShift  int
	TimeZone      DigitStrip // time zone；缺省时由 time zone image + pos 拼成
	TimeVariable  string     // 时段变量，缺省 "#time"
	DayVariable   string     // 日期变量：date，缺省回退到 day
	DayStrip      DigitStrip // day no：日期数字条
	DayUnit       UIButton   // day unit：日期单位图（info_unit "DAY"）
	DaySpecial    UIButton   // day sp：日期越过 day sp border 时整块替换的图
	DayBorder     int        // day sp border：特殊日期的门槛
	DayAdjustment int        // day adjustment：显示用的偏移
	DayOffset     int        // day no offset：下一桁的横向偏移
	// MoveCount 是「剩余可移动次数」的竖排帧。放課後2 的地图没有单独的数字，
	// 而是把 3/2/1 烘进 map_info_count 的三帧里，值来自脚本的 #time（4..6）。
	// 其它作品保持零值，不会多画东西。
	MoveCount         DigitStrip
	MoveCountVariable string // 缺省 "#time"
	MoveCountBase     int    // 显示帧 = 变量 - base；零值表示不画
	// MeterBase/Meter 是地图上的「絶倫メーター」：map_z_base 是外框，
	// msg_z 是 7 帧竖排瓶子。值来自 #zeturin，每 MeterStep 点升一级。
	MeterBase     UIButton
	Meter         DigitStrip
	MeterVariable string // 缺省 "#zeturin"
	MeterStep     int    // 每级所需点数；零值表示不画
	// TimeBases 是按时段替换整张地图的配置（base by time / base over by time /
	// base on by time + time range）。三射面談把 7、8 时段换成夜景图。
	TimeBases []MapPhaseTimeBase
	Buttons   []MapPhaseButton

	// 以下是 BSXScript 1.0 / 満淫電車2 这一代的精灵图地图。脚本自己铺
	// map_base 底图（按时段换 map_base1/2），引擎只补画站点、角色头像、
	// 悬停箭头和「時間を進める」按钮。
	//
	// Atlas 为真时 Place/Character/NextTime/Arrow 的 Clip 都相对 Background。
	Atlas bool
	// Places 是 6 个车站：BasePos 是图标基准点，Icon 是 map_base 里的
	// 站名牌矩形，StateVar 是 `#map_placeN`（角色在位掩码）。
	Places []MapPlace
	// Characters 是 ch0..ch4 的头像矩形。
	Characters []UIButton
	// IconPositions 是「n 个角色同时在场」时的相对坐标表。下标 1..5，
	// 0 位不用；每项长度等于 n。
	IconPositions [][]UIPoint
	// PlaceNamePos 是站名相对基准点的偏移（站名牌本身通常已经带名字）。
	PlaceNamePos UIPoint
	// NextTime 是「時間を進める」按钮，NextTimePos 是屏幕落点。
	NextTime    UIButton
	NextTimePos UIPoint
	NextTimeDir int
	// Arrow 是悬停站点的动画箭头（patterns = 1, 15 横向 15 帧），
	// ArrowPos 是相对站点基准点的偏移。
	Arrow    UIButton
	ArrowPos UIPoint
	// ResultVariable 是选择结果写回的属性名（#map_result）。
	ResultVariable string
}

// MapPlace 是一个车站图标及其状态变量。
type MapPlace struct {
	BasePos  UIPoint
	Icon     UIButton
	StateVar string
}

// AtlasMap 判断这个 [map phase] 是否是精灵图世代。只有满淫电车2/IE
// 这一代同时有 place0..5 与 ch0..4 的矩形写法；其它作品的 MapPhase
// 不填这些字段，保持原路径。
func (m MapPhase) AtlasMap() bool {
	return m.Atlas && len(m.Places) > 0 && m.NextTime.Atlas
}

// MapPhaseTimeBase 是一组按时段切换的地图底图。
type MapPhaseTimeBase struct {
	Low  int
	High int
	Base string
	Over string
	On   string
}

// mapPhaseButtonKeys 按配置里的键列出可能出现的系统按钮。
var mapPhaseButtonKeys = []string{
	"next time",
	"qsave button",
	"qload button",
	"save button",
	"load button",
	"config button",
	"log button",
	"dress button",
	"status button",
	"item button",
}

// Empty 判断地图阶段是否没有可画的内容。
func (m MapPhase) Empty() bool {
	return m.Background == "" && m.HitImage == "" && m.Overlay == "" &&
		m.Frame.Empty() && m.WeekNo.Image == "" && m.WeekDay.Image == "" &&
		m.TimeZone.Image == "" && m.DayStrip.Image == "" &&
		m.MoveCount.Image == "" && m.Meter.Image == "" && m.MeterBase.Empty() &&
		len(m.Buttons) == 0 && len(m.TimeBases) == 0
}

// BaseForTime 返回某个时段对应的底图组。
//
// 原引擎的 time rangeN / base by timeN 是"落在区间内就整组换图"：三射面談
// 的 7、8 时段换成夜间校舍。没有匹配的区间时返回 false，调用方沿用 base。
func (m MapPhase) BaseForTime(value int) (MapPhaseTimeBase, bool) {
	for _, base := range m.TimeBases {
		if value >= base.Low && value <= base.High {
			return base, true
		}
	}
	return MapPhaseTimeBase{}, false
}

func parseMapPhase(document *IniDocument) MapPhase {
	phase := MapPhase{
		Background:    document.Get(MapPhaseSection, "base", ""),
		HitImage:      document.Get(MapPhaseSection, "hit table", ""),
		Overlay:       document.Get(MapPhaseSection, "base on", ""),
		OverlayHover:  document.Get(MapPhaseSection, "base over", ""),
		Frame:         parseUIButton(document.Get(MapPhaseSection, "frame", "")),
		WeekNo:        parseDigitStrip(document.Get(MapPhaseSection, "week no", "")),
		WeekDay:       parseDigitStrip(document.Get(MapPhaseSection, "week day", "")),
		WeekDayShift:  document.GetInt(MapPhaseSection, "week day shift", 0),
		TimeZone:      parseMapPhaseTimeStrip(document),
		TimeVariable:  mapPhaseFirstValue(document.Get(MapPhaseSection, "time", ""), "#time"),
		DayVariable:   mapPhaseFirstValue(document.Get(MapPhaseSection, "date", ""), document.Get(MapPhaseSection, "day", "")),
		DayStrip:      parseDigitStrip(document.Get(MapPhaseSection, "day no", "")),
		DayUnit:       parseUIButton(document.Get(MapPhaseSection, "day unit", "")),
		DaySpecial:    parseUIButton(document.Get(MapPhaseSection, "day sp", "")),
		DayBorder:     document.GetInt(MapPhaseSection, "day sp border", 1<<30),
		DayAdjustment: document.GetInt(MapPhaseSection, "day adjustment", 0),
		DayOffset:     document.GetInt(MapPhaseSection, "day no offset", 0),
	}
	phase.TimeBases = parseMapPhaseTimeBases(document)

	for _, key := range mapPhaseButtonKeys {
		if button, ok := parseMapPhaseButton(key, document.Get(MapPhaseSection, key, "")); ok {
			phase.Buttons = append(phase.Buttons, button)
		}
	}

	parseAtlasMapPhase(document, &phase)
	return phase
}

// parseAtlasMapPhase 解析 BSXScript 1.0 的精灵图地图。
//
// 配置只用 `placeN = x,y,w,h` 描述站名牌，`placeN base pos` 描述落点，
// `placeN state` 描述角色在位掩码；这些键和旧 [map phase] 的按钮写法
// 完全不同，所以单独解析，避免把站名牌当成 `image,frames,x,y`。
func parseAtlasMapPhase(document *IniDocument, phase *MapPhase) {
	atlas := phase.Background
	if atlas == "" {
		return
	}
	for index := 0; index < 8; index++ {
		suffix := strconv.Itoa(index)
		icon, ok := parseAtlasButton(document, MapPhaseSection, "place"+suffix, atlas)
		if !ok {
			break
		}
		baseX, baseY := parsePoint(document.Get(MapPhaseSection, "place"+suffix+" base pos", ""))
		phase.Places = append(phase.Places, MapPlace{
			BasePos:  UIPoint{X: baseX, Y: baseY},
			Icon:     icon,
			StateVar: document.Get(MapPhaseSection, "place"+suffix+" state", ""),
		})
	}
	for index := 0; index < 8; index++ {
		icon, ok := parseAtlasButton(document, MapPhaseSection, "ch"+strconv.Itoa(index), atlas)
		if !ok {
			break
		}
		phase.Characters = append(phase.Characters, icon)
	}
	// iconN/M pos：N 是人数（1..5），M 是第几个头像（1..N）。
	phase.IconPositions = make([][]UIPoint, 6)
	for count := 1; count <= 5; count++ {
		for slot := 1; slot <= count; slot++ {
			key := "icon" + strconv.Itoa(slot) + "/" + strconv.Itoa(count) + " pos"
			point, ok := parseUIPoint(document.Get(MapPhaseSection, key, ""))
			if !ok {
				break
			}
			phase.IconPositions[count] = append(phase.IconPositions[count], point)
		}
	}
	phase.PlaceNamePos, _ = parseUIPoint(document.Get(MapPhaseSection, "place name pos", ""))
	if next, ok := parseAtlasButton(document, MapPhaseSection, "next time", atlas); ok {
		phase.NextTime = next
		phase.NextTimePos, _ = parseUIPoint(document.Get(MapPhaseSection, "next time pos", ""))
	}
	phase.NextTimeDir = document.GetInt(MapPhaseSection, "next time dir", 0)
	if arrow, ok := parseAtlasButton(document, MapPhaseSection, "arrow", atlas); ok {
		phase.Arrow = arrow
		phase.ArrowPos, _ = parseUIPoint(document.Get(MapPhaseSection, "arrow pos", ""))
	}
	phase.ResultVariable = document.Get(MapPhaseSection, "result", "#map_result")
	if zone := document.Get(MapPhaseSection, "time zone", ""); zone != "" {
		phase.TimeVariable = zone
	}
	phase.Atlas = len(phase.Places) > 0
}

// parseMapPhaseTimeBases 解析 time rangeN 与 base/over/on by timeN。
//
// 键里的编号 N 是同一个时段槽：time rangeN 给区间，三个 by timeN 给该区间
// 使用的底图。编号不连续时按实际存在的键取，不假设从 0 开始连续。
func parseMapPhaseTimeBases(document *IniDocument) []MapPhaseTimeBase {
	result := []MapPhaseTimeBase{}
	for index := 0; index < 16; index++ {
		suffix := strconv.Itoa(index)
		ranges := ParsePositions(document.Get(MapPhaseSection, "time range"+suffix, ""))
		if len(ranges) < 2 {
			continue
		}
		base := MapPhaseTimeBase{
			Low:  ranges[0],
			High: ranges[1],
			Base: document.Get(MapPhaseSection, "base by time"+suffix, ""),
			Over: document.Get(MapPhaseSection, "base over by time"+suffix, ""),
			On:   document.Get(MapPhaseSection, "base on by time"+suffix, ""),
		}
		if base.Base == "" && base.Over == "" && base.On == "" {
			continue
		}
		result = append(result, base)
	}
	return result
}

// parseMapPhaseTimeStrip 优先读 time zone 一行；UK 把图和坐标拆成两个键写，
// 这种写法的图固定是 4 帧（早/昼/夕/夜），所以这里直接补上 4 帧。
func parseMapPhaseTimeStrip(document *IniDocument) DigitStrip {
	if strip := parseDigitStrip(document.Get(MapPhaseSection, "time zone", "")); strip.Image != "" {
		return strip
	}

	image := document.Get(MapPhaseSection, "time zone image", "")
	if image == "" {
		return DigitStrip{}
	}

	x, y := parsePoint(document.Get(MapPhaseSection, "time zone image pos", ""))
	return DigitStrip{Image: image, Frames: 4, X: x, Y: y}
}

// parseMapPhaseButton 解析 "image, frames, x, y"。帧数可省略：只有三段时
// 第二段就是 X，此时按 1 帧处理，避免把坐标误当成帧数。
func parseMapPhaseButton(key, value string) (MapPhaseButton, bool) {
	parts := strutil.SplitTrimNonEmpty(value, ",")
	if len(parts) == 0 {
		return MapPhaseButton{}, false
	}

	button := MapPhaseButton{Tool: mapPhaseTool(key), Image: parts[0], Frames: 1}
	coords := parts[1:]
	if len(parts) >= 4 {
		if frames, ok := parseInt(parts[1]); ok && frames > 0 {
			button.Frames = frames
		}
		coords = parts[2:]
	}
	if len(coords) >= 1 {
		button.X = parseCoordinate(coords[0])
	}
	if len(coords) >= 2 {
		button.Y = parseCoordinate(coords[1])
	}

	return button, true
}

// mapPhaseTool 把配置键映射成引擎内部的工具名。"item button" 在配置里是
// 状态/アイテム入口，与 status 共用同一个画面，所以归到 status。
func mapPhaseTool(key string) string {
	if key == "item button" {
		return "status"
	}

	return strings.ReplaceAll(strings.TrimSuffix(key, " button"), " ", "_")
}

func mapPhaseFirstValue(primary, fallback string) string {
	if primary != "" {
		return primary
	}

	return fallback
}
