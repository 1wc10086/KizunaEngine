// 本文件把脚本里的"界面定义记录"收集成结构化状态。
//
// TMR-Hiro 的界面全部由脚本定义，而不是宿主写死：`op1/0x82` 开一张模态界面，
// 随后 `0x92` 给底图/悬停图、`0x23`/`0x43` 给按钮、`op4/0x64` 给网格单元与
// 列表行、`op4/0xc3` 给"空/未完成/底图/悬停"四张图，最后一条 `op1/0x10`
// 是默认项。原引擎就按这些数字画像素，所以这里必须原样保留坐标与名字。
//
// 为什么集中在一个文件：四条记录互相引用（网格的单元尺寸、页面按钮的标签、
// 网格里缩略图的来源都分散在上面几条记录里），拆开会让"按标签跳转"的
// 语义被抄成四份。
package vm

import (
	"encoding/binary"
	"fmt"
	"strings"

	"kizuna/enginego/brand/tmrhiro/script"
)

// ScreenItemKind 是网格单元的类型（op4/0x64 的第一个 u16）。
//
// 原版用同一个记录描述五种东西，靠这个数字区分：
//
//	1  按钮（x/y/w/h + 跳转标签）
//	2  界面定义结束（值为 0 的占位，等同"默认项"）
//	10 CG 单元（x/y/w/h + 该格的宏名列表，缩略图取第一个）
//	20 场景单元（x/y/w/h + 场景脚本名）
//	30 BGM 行（x/y/w/h + 曲目名）
//	31 BGM 停止按钮
type ScreenItemKind int

const (
	ScreenItemButton   ScreenItemKind = 1
	ScreenItemEnd      ScreenItemKind = 2
	ScreenItemCG       ScreenItemKind = 10
	ScreenItemScene    ScreenItemKind = 20
	ScreenItemBGM      ScreenItemKind = 30
	ScreenItemBGMStop  ScreenItemKind = 31
	ScreenItemUnknown  ScreenItemKind = 0
	screenItemMinField                = 4 // 类型 + x/y/w/h 共 5 个 u16
)

// ScreenItem 是一个单元。
type ScreenItem struct {
	Kind ScreenItemKind
	X, Y int
	W, H int
	// Label 是按钮的跳转标签（Kind=1 时有效）。
	Label string
	// Names 是 CG 单元的宏名列表；场景单元是单个脚本名；BGM 行是曲目名。
	Names []string
	// Hover 在悬停时用 "-on" 图里的同一矩形画高亮；由 session 决定。
}

// Screen 是一张由脚本定义的模态界面。
type Screen struct {
	// Kind 是界面的用途（由 session 按图形集推断：标题/鉴赏/角色/选择/其它）。
	Kind ScreenKind
	// Base/BaseOn 是底图与悬停图（0x92）。
	Base   string
	BaseOn string
	// Graphic 是 op4/0xc3 的四张图：空位、未完成、底图、悬停。
	Graphic ScreenGraphic
	// Buttons 是 0x23/0x43/0x64(type 1) 收集到的按钮。
	Buttons []ScreenItem
	// Items 是 0x64 收集到的网格/列表单元。
	Items []ScreenItem
	// Cells 是 0x64 记录里声明的网格排版（第一行的 x/y/w/h 与行列数，
	// 原版对每一格单独给矩形，这里只保留"有多少格"用于布局兜底）。
	CellCount int
	// CharaSelect 是 0xb2 的立绘选择界面：底图、空位图、按钮图。
	Chara BaseImages
	// Hovered 是当前悬停的按钮下标。
	Hovered int
	// Default 是最后那条 0x10 的标签（点击空白处/取消时使用）。
	Default string
	// Percent 是 op5/0x30 设置的达成率（-1 表示未设置）。
	Percent int
}

// ScreenGraphic 是 op4/0xc3 的四张图。
type ScreenGraphic struct {
	Empty      string
	Incomplete string
	Base       string
	BaseOn     string
}

// BaseImages 是 0xb2 的"底图,空位图,按钮图"三元组。
type BaseImages struct {
	Base   string
	Empty  string
	Button string
}

// ScreenKind 由 session 判定（依据图形集与调用者的标签），这里只留枚举。
type ScreenKind int

const (
	ScreenNone ScreenKind = iota
	ScreenTitle
	ScreenView
	ScreenChara
	ScreenSelect
)

// ScreenState 返回当前界面（无界面时返回 nil）。
func (v *VM) ScreenState() *Screen {
	if v == nil || v.screen == nil {
		return nil
	}
	return v.screen
}

// ScreenSelect 处理脚本界面的点击：跳到标签并继续执行。
//
// 与 TitleSelect 的区别只有一点：脚本界面是"脚本自己开出来的模态页"，
// 选完后界面关闭，VM 继续在同一个脚本里跑（不回到标题态）。
func (v *VM) ScreenSelect(label string) Step {
	if v.current == nil || label == "" {
		return v.step
	}
	v.screen = nil
	v.titleButtons = nil
	v.mapMenu = false
	if target := v.current.Label(label); target >= 0 {
		v.pc = target
		v.step = StepContinue
		return v.Run(0)
	}
	// 标签不在本脚本：当作换脚本调用（view06_04_02 这类回放片段）。
	v.stack = append(v.stack, frame{script: v.current, pc: v.pc})
	if err := v.enter(label, 0); err != nil {
		v.pop()
		return v.step
	}
	v.step = StepContinue
	return v.Run(0)
}

// screenActive 报告"脚本是否停在界面定义区间"。
func (v *VM) screenActive() bool { return v.screen != nil }

// screenReset 清掉界面的按钮/单元与定义标记，保留底图。
//
// 为什么保留底图：鉴赏/立绘的每一页只有 `0xc3 + 0x64` 两段定义（没有新的
// 0x82/0x92）。若整体丢弃，翻页那一帧会先画一帧空背景；而 0xc3 总会带上
// 本页的底图，所以保留旧值只是过渡，不会真的串页。
func (v *VM) screenReset() {
	if v.screen == nil {
		v.screen = &Screen{Hovered: -1, Percent: -1}
		return
	}
	base, baseOn := v.screen.Base, v.screen.BaseOn
	graphic := v.screen.Graphic
	v.screen = &Screen{Hovered: -1, Percent: v.screen.Percent, Base: base, BaseOn: baseOn, Graphic: graphic}
	v.screenFilled = false
}

// beginScreen 开一张新界面（op1/0x82）。
//
// 每次 0x82 都清空上一张的定义：view 脚本在 PERCENT 确认框里也会再开一张，
// 若把上一张的按钮留着，确认框的点击会落到鉴赏的格子上。
func (v *VM) beginScreen() {
	v.screen = &Screen{Hovered: -1, Percent: -1}
	// screenFilled 标记"自上次停在界面后又收集到了内容"：脚本页面的定义段
	// 以下一个标签收尾，靠它决定什么时候停（见 execFlow 的 0x0000）。
	v.screenFilled = false
}

// setScreenBase 处理 0x92（底图定义）。
//
// 载荷：`u16 layers, u16 panel, "<base>,<base_on>"`。前 4 字节是层数与面板号，
// 名字从偏移 4 开始。
func (v *VM) setScreenBase(r *script.Record) {
	if v.screen == nil {
		v.beginScreen()
	}
	payload := r.Raw
	if len(payload) <= 4 {
		return
	}
	v.screen.Base, v.screen.BaseOn = splitPair(script.DecodeText(payload[4:]))
}

// addScreenButton 处理 0x23（按钮）。
func (v *VM) addScreenButton(r *script.Record) {
	if v.screen == nil {
		v.beginScreen()
	}
	button, ok := parseButton(r.Raw)
	if !ok {
		return
	}
	v.screen.Buttons = append(v.screen.Buttons, ScreenItem{
		Kind: ScreenItemButton, X: button.X, Y: button.Y, W: button.W, H: button.H, Label: button.Label,
	})
	v.screenFilled = true
}

// addCharaButton 处理 0x43（立绘选择按钮）。
//
// 载荷（实测 0x43 的记录）：前 8 字节是**两个矩形**，随后是立绘宏名，
// 最后是 `,,<跳转标签>`：
//
//	u16 pad(0x2020), u16 x, u16 y, u16 ?, u16 ?, u16 ?, u16 ?, u16 w, u16 h, ...
//
// 例如 `20 20 f3 00 22 00 00 00 00 00 f0 00 55 02 00 00 55 02 ... ,,maria_03_01`：
// 卡片在 (243,34)、尺寸 240x597，三张卡步进 272（240+32）。以前把第 3、4 个
// u16 当宽高（34,0）→ 热区高度为 0，卡片点不动、也画不出来。
func (v *VM) addCharaButton(r *script.Record) {
	if v.screen == nil {
		v.beginScreen()
	}
	payload := r.Raw
	if len(payload) < 16 {
		return
	}
	text := script.DecodeText(payload)
	label := ""
	if comma := strings.LastIndex(text, ",,"); comma >= 0 {
		label = strings.TrimSpace(text[comma+2:])
	}
	if label == "" {
		return
	}
	v.screen.Buttons = append(v.screen.Buttons, ScreenItem{
		Kind:  ScreenItemButton,
		X:     int(binary.LittleEndian.Uint16(payload[2:4])),
		Y:     int(binary.LittleEndian.Uint16(payload[4:6])),
		W:     int(binary.LittleEndian.Uint16(payload[12:14])),
		H:     int(binary.LittleEndian.Uint16(payload[14:16])),
		Label: label,
	})
	v.screenFilled = true
}

// addCharaBase 处理 0xb2（立绘选择界面定义）。
//
// 载荷：`u16 flag, u16 count, "<base>,<empty>,<button>"`。flag 表示是否显示
// 立绘（1 = 显示），count 是可选立绘数。
func (v *VM) addCharaBase(r *script.Record) {
	if len(r.Raw) < 4 {
		return
	}
	text := script.DecodeText(r.Raw[4:])
	fields := strings.Split(text, ",")
	if v.screen == nil {
		v.beginScreen()
	}
	if len(fields) >= 3 {
		v.screen.Chara = BaseImages{
			Base:   strings.TrimSpace(fields[0]),
			Empty:  strings.TrimSpace(fields[1]),
			Button: strings.TrimSpace(fields[2]),
		}
	}
	if v.screen.Kind == ScreenNone {
		v.screen.Kind = ScreenChara
	}
}

// addScreenItem 处理 op4/0x64（网格单元/列表行/页面按钮）。
//
// 载荷：`u16 kind, u16 x, u16 y, u16 w, u16 h, <名字表>`。kind 见 ScreenItemKind。
func (v *VM) addScreenItem(r *script.Record) {
	payload := r.Raw
	if len(payload) < screenItemMinField*2 {
		return
	}
	if v.screen == nil {
		v.beginScreen()
	}
	kind := ScreenItemKind(binary.LittleEndian.Uint16(payload[0:2]))
	// 字段是矩形的左右上下（x0,y0,x1,y1），不是 x,y,w,h：
	// 原版的鉴赏格子写作 `10 96 81 48 01 e6 00`（150,129 → 328,230），
	// 若当成 w/h 会把 178x101 的格子画成 328x230，nodata 图块互相重叠。
	x0 := int(binary.LittleEndian.Uint16(payload[2:4]))
	y0 := int(binary.LittleEndian.Uint16(payload[4:6]))
	x1 := int(binary.LittleEndian.Uint16(payload[6:8]))
	y1 := int(binary.LittleEndian.Uint16(payload[8:10]))
	item := ScreenItem{
		Kind: kind,
		X:    x0,
		Y:    y0,
		W:    x1 - x0,
		H:    y1 - y0,
	}
	if item.W <= 0 || item.H <= 0 {
		return
	}
	body := strings.TrimSpace(script.DecodeText(payload[10:]))
	switch kind {
	case ScreenItemButton:
		if body != "" {
			item.Label = body
			v.screen.Buttons = append(v.screen.Buttons, item)
		}
	case ScreenItemCG:
		item.Names = splitNames(body)
		v.screen.Items = append(v.screen.Items, item)
		v.screen.CellCount++
		v.screenFilled = true
	case ScreenItemScene, ScreenItemBGM:
		item.Names = []string{body}
		v.screen.Items = append(v.screen.Items, item)
		v.screen.CellCount++
		v.screenFilled = true
	case ScreenItemBGMStop:
		v.screen.Items = append(v.screen.Items, item)
	case ScreenItemEnd:
		// 结束占位：标签是"默认项"，点击空白处才用。
		if body != "" {
			v.screen.Default = body
		}
	}
}

// applyPageGraphic 是把 op4/0xc3 的四张图装进界面的实现。
func (v *VM) setScreenGraphic(r *script.Record) {
	payload := r.Raw
	if len(payload) < 4 {
		return
	}
	names := splitNames(strings.TrimSpace(script.DecodeText(payload[4:])))
	if v.screen == nil {
		v.beginScreen()
	}
	if len(names) >= 4 {
		v.applyPageGraphic(ScreenGraphic{Empty: names[0], Incomplete: names[1], Base: names[2], BaseOn: names[3]})
		v.screenFilled = true
	}
}

// applyPageGraphic 写入页面的四张图。
//
// 底图与悬停图**总是**跟着这一组覆盖：鉴赏的每一页都自带一份 0xc3
// （nodata,incomplete,cgmodeNN,cgmodeNN_on），换页时必须换底图；若只在
// Base 为空时写，玩家翻页后底图还停在第一页。
func (v *VM) applyPageGraphic(graphic ScreenGraphic) {
	if v.screen == nil {
		v.beginScreen()
	}
	v.screen.Graphic = graphic
	v.screen.Base, v.screen.BaseOn = graphic.Base, graphic.BaseOn
}

// setPercentValue 记录界面右上角要显示的达成率（0..100）。
func (v *VM) setPercentValue(percent int) {
	if v.screen == nil {
		v.beginScreen()
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	v.screen.Percent = percent
}

// setScreenDefault 处理界面定义区间末尾的那条 0x10：记下默认标签并停在界面。
func (v *VM) setScreenDefault(label string) {
	if v.screen == nil {
		v.beginScreen()
	}
	v.screen.Default = label
}

// screenDefaultAhead 从当前 PC 往后找第一条同脚本跳转（0x10）的标签。
//
// 原版把"点击空白处"的默认项写在界面定义段之后；定义段以标签收尾（见
// execFlow 的 0x0000），所以那一刻还没有读到 0x10。这里向前扫描同一脚本的
// 记录，把第一条 `01 01 <label>` 的标签当作默认项。
func (v *VM) screenDefaultAhead() string {
	if v.current == nil {
		return ""
	}
	for i := v.pc; i < len(v.current.Records); i++ {
		record := &v.current.Records[i]
		if record.Op != script.OpFlow || record.Arg != 0x0010 {
			continue
		}
		payload := record.Raw
		if len(payload) < 3 || payload[1] != 1 {
			continue
		}
		name := strings.TrimSpace(script.DecodeText(payload[2:]))
		if comma := strings.Index(name, ","); comma >= 0 {
			name = strings.TrimSpace(name[comma+1:])
		}
		return name
	}
	return ""
}

// splitNames 把逗号分隔的名字表拆开（空项丢弃）。
func splitNames(body string) []string {
	parts := strings.Split(body, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ScreenButtons 返回当前脚本界面的按钮（标题菜单构建用）。
func (v *VM) ScreenButtons() []ScreenItem {
	if v == nil || v.screen == nil {
		return nil
	}
	out := make([]ScreenItem, len(v.screen.Buttons))
	copy(out, v.screen.Buttons)
	return out
}

// ScreenDefaultLabel 返回界面的取消/返回标签。
func (v *VM) ScreenDefaultLabel() string {
	if v == nil || v.screen == nil {
		return ""
	}
	return v.screen.Default
}

// DebugScreenVM 是诊断用：界面与 PC 概况（日志/单测用）。
func (v *VM) DebugScreenVM() string {
	if v == nil || v.screen == nil {
		return "no screen"
	}
	return fmt.Sprintf("base=%q buttons=%d items=%d filled=%v pc=%d step=%d mapMenu=%v",
		v.screen.Base, len(v.screen.Buttons), len(v.screen.Items), v.screenFilled, v.pc, v.step, v.mapMenu)
}

// CharaViewActive 报告是否停在立绘鉴赏器。
func (v *VM) CharaViewActive() bool {
	return v != nil && v.charaView
}

// LeaveCharaView 退出立绘鉴赏器（点"戻る"或跳转到别的页面时调用）。
func (v *VM) LeaveCharaView() { v.charaView = false }

// TakePercent 取走 op5/0x30 的达成率请求（-1 表示没有）。
func (v *VM) TakePercent() int {
	if v == nil || v.percentReq < 0 {
		return -1
	}
	value := int(v.percentReq)
	v.percentReq = -1
	return value
}

// ClearScreen 丢弃当前界面定义。
//
// 读档/换脚本/回标题时必须调用：界面定义（底图/按钮/格子）属于"当时那一段
// 脚本的演出"，存档只记 PC；不清掉会让读档后的第一帧把上一段的界面
// （例如标题的 title2/title3）又画一遍。
func (v *VM) ClearScreen() {
	v.screen = nil
	v.screenFilled = false
	v.mapMenu = false
	v.charaView = false
	v.titleBase, v.titleBaseOn = "", ""
	v.titleButtons = nil
}
