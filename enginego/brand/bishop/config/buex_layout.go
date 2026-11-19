package config

// buex_layout.go 解析 define.bsa 里的 museum_buex_layout（立ち絵鑑賞室的布局）。
//
// 来源：原引擎的 define.bsa 里带有一份 "museum_buex_layout" 文档，描述鉴赏室
// 五个页面的底图、按钮、开关排布、缩略图和存档位坐标。文档用的是和其它 layout
// 一样的方言（键可以不写引号、键值对之间没有逗号、';' 之后是注释、坐标可以写成
// "215+135*0" 这样的算式），所以直接复用 config_layout.go 里的
// ParseLayoutDocument，再按鉴赏室的语义转成强类型。
//
// 为什么这里把"舞台表"原样保留（开关组、角色编号表、ぬぎぬぎ换算表）：
// 这些数字是原引擎写死的排布数据，编号和图片资源里的帧一一对应（例如角色的
// 表情编号 13 表示第 13 号表情图）。任何"聪明"的换算或重新编号都会让鉴赏室
// 的按钮和图片错位，所以这里只做搬运，不做解释。

// BuexMenuEntry 是鉴赏室顶部主菜单的一个入口：Key 是 main_menu 里的键名，
// Button 是对应的图片和坐标。
//
// 用切片而不是 map：主菜单是按固定顺序画出来的，而 main_menu 在文档里是个
// 对象，通用解析器不保留书写顺序，所以顺序必须在这里写死。
type BuexMenuEntry struct {
	Key    string
	Button ConfigButton
}

// BuexSwitchGroup 是一组等间距排列的小开关（ポーズ/視点/ぬぎぬぎ/服装/下着/表情）。
// X、Y 是第一个开关的坐标，SpaceX、SpaceY 是相邻开关的间距，Count 是开关个数。
type BuexSwitchGroup struct {
	X, Y, SpaceX, SpaceY, Count int
	// Columns 是每行的开关数（配置里的 repeat_x）。开关不是沿对角线排的：
	// 超过 Columns 颗就换到下一行，行距用 SpaceY。以前把 SpaceX/SpaceY
	// 同时按 index 递增，开关会斜着排下去（視点第二颗就歪到右下方）。
	Columns int
}

// BuexCharacter 是鉴赏室里的一个角色：图标按钮 + 该角色的服装/表情编号表。
type BuexCharacter struct {
	Icon ConfigButton
	// Wear 是服装编号表，UnderWear 是默认下着编号。
	Wear      []int
	UnderWear int
	// Faces 是表情编号表，按"页 x 编号"两层排列（第一页 1..12，第二页 13..24）。
	Faces [][]int
}

// BuexColorBar 是取色器里的一根色带：条本身，以及两端的最小/最大按钮。
// 三者的坐标决定了滑块可以拖到的范围，所以坐标必须原样保留。
type BuexColorBar struct {
	Bar ConfigButton
	Min ConfigButton
	Max ConfigButton
}

// BuexColorPicker 是消息编辑页的取色器。
type BuexColorPicker struct {
	Base string
	Hit  string
	// Radio 是单选按钮图片名；Targets 是各个可点击色块的位置（hit 编号）。
	Radio   string
	Targets []UIPoint
	Close   ConfigButton
	Bars    []BuexColorBar
}

// BuexBackgroundSelect 是背景选择页。
type BuexBackgroundSelect struct {
	Base string
	// Subject 是背景标题相对缩略图左上角的位置。
	Subject UIPoint
	// NumberFont 是画在缩略图上的页码字图。
	NumberFont ConfigButton
	PageGroups [][]ConfigButton
	ThumbPos   []UIPoint
}

// BuexSetting 是鉴赏室的立ち絵页（setting_bu）：底图 + 按钮 + 开关组 + 角色表。
type BuexSetting struct {
	Base string
	// Buttons 按名字取用（例如 "hair_mode"），所以用 map。
	Buttons map[string]ConfigButton
	// PosRegion 是姿势/视角指示器的活动区域 [左, 上, 右, 下]。
	PosRegion [4]int
	Switches  []BuexSwitchGroup
	// PoseSignature、ViewSignature 是姿势/视角的指示文字（"ab"、"fu"）。
	PoseSignature string
	ViewSignature string
	// StripConvert 是ぬぎぬぎ转换表，索引和值都按原样使用。
	StripConvert []int
	Characters   []BuexCharacter
}

// BuexEdit 是消息编辑页（edit_message）。
type BuexEdit struct {
	Base string
	// SubMenu 是页首的子菜单按钮，顺序有意义。
	SubMenu []ConfigButton
	// Commands 是编辑命令按钮（字号/字色/排版/气泡/删除），按名字取用。
	Commands map[string]ConfigButton
	Picker   BuexColorPicker
}

// BuexStore 是存档页（data_store）。
type BuexStore struct {
	Base string
	// Komaban 是存档画面用到的"嵌合板"图片名。
	Komaban string
	// Commands 是存档命令按钮（存档/读档/写入槽位/导出/清除），按名字取用。
	Commands map[string]ConfigButton
	ThumbPos []UIPoint
	SlotPos  []UIPoint
}

// BuexLayout 是 museum_buex_layout 的顶层结构。
type BuexLayout struct {
	// Menu 是顶部主菜单，按固定顺序排列。
	Menu []BuexMenuEntry
	// Notices 是提示图片（键是用途：bgv/bg/bu/msgwnd/freemsg/data）。
	Notices map[string]string
	Loading ConfigButton
	Bg      BuexBackgroundSelect
	Setting BuexSetting
	Edit    BuexEdit
	Store   BuexStore
	// MultiStandee 表示立ち絵页可以同时展示多个可见角色。
	//
	// 它不来自 museum_buex_layout，而是旧式 [bu viewer] 的
	// [museum phase] multi bu = 1（牝教師3）。新版布局没有这一项，
	// 保持单角色展示，避免改变 侵蝕/淫獄 已经调好的画面。
	MultiStandee bool
	// EnableViewControl 对应旧式 [bu viewer] 的 enable view control：
	// 立ち絵页要画自由視点バー和顔/胸/腰按钮，近距離立绘按它们裁切。
	EnableViewControl bool
}

// buexMainMenuOrder 是鉴赏室顶部菜单的固定画序。
//
// 顺序写在代码里而不是依赖文档顺序：main_menu 是对象，通用解析器会丢顺序，
// 而原引擎就是按这个次序把按钮摆开的。
var buexMainMenuOrder = []string{
	"setting_bgv",
	"bg_select",
	"setting_bu",
	"edit_message",
	"data_store",
	"back_to_thumbs",
}

// LoadBuexLayout 解析整份 museum_buex_layout。
//
// 只有语法错误才返回 error；缺失的可选段会得到零值，因此残缺的文档（比如
// 只有 main_menu 的旧版本）也能安全调用。
func LoadBuexLayout(data []byte) (*BuexLayout, error) {
	root, err := ParseLayoutDocument(data)
	if err != nil {
		return nil, err
	}

	layout := &BuexLayout{}
	layout.Menu = parseBuexMenu(root["main_menu"])
	layout.Notices = parseBuexNotices(root["notice"])
	if button, ok := ParseButton(root["loading"]); ok {
		layout.Loading = button
	}
	// 根部的 setting_bgv 段在原文档里是空的（背景鉴赏页没有额外内容），
	// 这里刻意不读它。
	layout.Bg = parseBuexBackgroundSelect(root["bg_select"])
	layout.Setting = parseBuexSetting(root["setting_bu"])
	layout.Edit = parseBuexEdit(root["edit_message"])
	layout.Store = parseBuexStore(root["data_store"])

	return layout, nil
}

// parseBuexMenu 解析 main_menu：按固定次序取出按钮，解析不了的键直接跳过
// （某个键缺图时只少一个入口，不该让整个鉴赏室开不起来）。
func parseBuexMenu(value any) []BuexMenuEntry {
	object, ok := Object(value)
	if !ok {
		return nil
	}

	menu := make([]BuexMenuEntry, 0, len(buexMainMenuOrder))
	for _, key := range buexMainMenuOrder {
		button, ok := ParseButton(object[key])
		if !ok {
			continue
		}
		menu = append(menu, BuexMenuEntry{Key: key, Button: button})
	}
	return menu
}

// parseBuexNotices 解析 notice 段：键是用途，值是图片名。
//
// 这里用 map 而不是切片：提示图是按名字取的，原始顺序没有意义。
func parseBuexNotices(value any) map[string]string {
	object, ok := Object(value)
	if !ok {
		return nil
	}

	notices := make(map[string]string, len(object))
	for key, item := range object {
		text, ok := Text(item)
		if !ok || len(text) == 0 {
			continue
		}
		notices[key] = text
	}
	return notices
}

// parseBuexBackgroundSelect 解析 bg_select 段。
func parseBuexBackgroundSelect(value any) BuexBackgroundSelect {
	section := BuexBackgroundSelect{}
	object, ok := Object(value)
	if !ok {
		return section
	}

	section.Base, _ = Text(object["base"])
	if subject, ok := Object(object["subject"]); ok {
		// subject.size 是小数（.5），表示标题相对本编字号的缩放比例。
		// 通用解析器只认整数，这里刻意不读它，免得把 0 当成缩放比例用。
		if pos, ok := Array(subject["pos"]); ok {
			section.Subject.X, section.Subject.Y = buexPoint(pos)
		}
	}
	if button, ok := ParseButton(object["number_font"]); ok {
		section.NumberFont = button
	}
	section.PageGroups = parseBuexPageGroups(object["page_select"])
	section.ThumbPos = parseBuexPoints(object["thumb_pos"])

	return section
}

// parseBuexPageGroups 解析 page_select：外层是页签组，内层是组里的按钮。
//
// 空组没有任何含义，直接跳过；但其余组的先后次序必须保留（页签按组依次显示）。
func parseBuexPageGroups(value any) [][]ConfigButton {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	groups := make([][]ConfigButton, 0, len(array))
	for _, item := range array {
		buttons := parseBuexButtons(item)
		if len(buttons) == 0 {
			continue
		}
		groups = append(groups, buttons)
	}
	return groups
}

// parseBuexSetting 解析 setting_bu 段。
func parseBuexSetting(value any) BuexSetting {
	section := BuexSetting{}
	object, ok := Object(value)
	if !ok {
		return section
	}

	section.Base, _ = Text(object["base"])
	section.Buttons = parseBuexButtonMap(object["buttons"])
	if region, ok := Array(object["pos_region"]); ok {
		for index := 0; index < 4 && index < len(region); index++ {
			section.PosRegion[index], _ = Number(region[index])
		}
	}
	section.Switches = parseBuexSwitchGroups(object["switchs"])
	section.PoseSignature, _ = Text(object["pose_signature"])
	section.ViewSignature, _ = Text(object["view_signature"])
	section.StripConvert = parseBuexInts(object["ngng_convert_table"])
	section.Characters = parseBuexCharacters(object["characters"])

	return section
}

// parseBuexSwitchGroups 解析 switchs：每组给出第一个开关的坐标、间距和个数。
//
// space / repeat_x 缺失时按 0 处理，这样只有一颗开关的组也能解析。
func parseBuexSwitchGroups(value any) []BuexSwitchGroup {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	groups := make([]BuexSwitchGroup, 0, len(array))
	for _, item := range array {
		object, ok := Object(item)
		if !ok {
			continue
		}
		group := BuexSwitchGroup{}
		if pos, ok := Array(object["pos"]); ok {
			group.X, group.Y = buexPoint(pos)
		}
		if space, ok := Array(object["space"]); ok {
			group.SpaceX, group.SpaceY = buexPoint(space)
		}
		// repeat_x 是"每行几颗"，不是总颗数：总颗数由语义决定
		// （服装表长度、脱衣档位数、表情表长度），否则 3SLG 的
		// ぬぎぬぎ 4 列会把 6 档裁成 4 颗、表情 6 列会把 13 颗裁成 6 颗。
		group.Columns, _ = Number(object["repeat_x"])
		groups = append(groups, group)
	}
	return groups
}

// parseBuexCharacters 解析角色表：图标按钮 + 服装/表情编号表。
func parseBuexCharacters(value any) []BuexCharacter {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	characters := make([]BuexCharacter, 0, len(array))
	for _, item := range array {
		object, ok := Object(item)
		if !ok {
			continue
		}
		character := BuexCharacter{}
		if button, ok := ParseButton(object["icon"]); ok {
			character.Icon = button
		}
		if info, ok := Object(object["info"]); ok {
			character.Wear = parseBuexInts(info["wear"])
			character.UnderWear, _ = Number(info["under_wear"])
			character.Faces = parseBuexIntRows(info["face"])
		}
		characters = append(characters, character)
	}
	return characters
}

// parseBuexEdit 解析 edit_message 段。
func parseBuexEdit(value any) BuexEdit {
	section := BuexEdit{}
	object, ok := Object(value)
	if !ok {
		return section
	}

	section.Base, _ = Text(object["base"])
	section.SubMenu = parseBuexButtons(object["sub_menu"])
	section.Commands = parseBuexButtonMap(object["edit_command"])
	section.Picker = parseBuexColorPicker(object["color_picker"])

	return section
}

// parseBuexColorPicker 解析取色器：底图/判定图/单选按钮图 + 色块位置 + 色带。
func parseBuexColorPicker(value any) BuexColorPicker {
	picker := BuexColorPicker{}
	object, ok := Object(value)
	if !ok {
		return picker
	}

	picker.Base, _ = Text(object["base"])
	picker.Hit, _ = Text(object["hit"])
	picker.Radio, _ = Text(object["radio_button"])
	picker.Targets = parseBuexPoints(object["target"])
	if button, ok := ParseButton(object["close"]); ok {
		picker.Close = button
	}
	picker.Bars = parseBuexColorBars(object["color_bar"])

	return picker
}

// parseBuexColorBars 解析 color_bar：每根色带由条本身和两端的最小/最大按钮组成。
func parseBuexColorBars(value any) []BuexColorBar {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	bars := make([]BuexColorBar, 0, len(array))
	for _, item := range array {
		object, ok := Object(item)
		if !ok {
			continue
		}
		bar := BuexColorBar{}
		if button, ok := ParseButton(object["bar"]); ok {
			bar.Bar = button
		}
		if button, ok := ParseButton(object["min"]); ok {
			bar.Min = button
		}
		if button, ok := ParseButton(object["max"]); ok {
			bar.Max = button
		}
		bars = append(bars, bar)
	}
	return bars
}

// parseBuexStore 解析 data_store 段。
func parseBuexStore(value any) BuexStore {
	section := BuexStore{}
	object, ok := Object(value)
	if !ok {
		return section
	}

	section.Base, _ = Text(object["base"])
	section.Komaban, _ = Text(object["komaban"])
	section.Commands = parseBuexButtonMap(object["edit_command"])
	section.ThumbPos = parseBuexPoints(object["thumb_pos"])
	section.SlotPos = parseBuexPoints(object["slot_pos"])

	return section
}

// parseBuexButtons 解析按钮数组，跳过解析不了的元素。
func parseBuexButtons(value any) []ConfigButton {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	buttons := make([]ConfigButton, 0, len(array))
	for _, item := range array {
		if button, ok := ParseButton(item); ok {
			buttons = append(buttons, button)
		}
	}
	return buttons
}

// parseBuexButtonMap 解析"名字 -> 按钮"的对象。
//
// 键原样保留（不认识的键也留着），这样换成别的作品时新增的按钮不会被丢掉。
func parseBuexButtonMap(value any) map[string]ConfigButton {
	object, ok := Object(value)
	if !ok {
		return nil
	}

	buttons := make(map[string]ConfigButton, len(object))
	for key, item := range object {
		if button, ok := ParseButton(item); ok {
			buttons[key] = button
		}
	}
	return buttons
}

// parseBuexPoints 解析一串 [x, y] 坐标。
//
// 坐标数组必须至少有两个数字才收下：缩略图和存档位的数量是写死的，
// 少收一个会让后面的资源整体错位。
func parseBuexPoints(value any) []UIPoint {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	points := make([]UIPoint, 0, len(array))
	for _, item := range array {
		pair, ok := Array(item)
		if !ok || len(pair) < 2 {
			continue
		}
		x, _ := Number(pair[0])
		y, _ := Number(pair[1])
		points = append(points, UIPoint{X: x, Y: y})
	}
	return points
}

// parseBuexInts 解析整数数组。
func parseBuexInts(value any) []int {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	values := make([]int, 0, len(array))
	for _, item := range array {
		number, ok := Number(item)
		if !ok {
			continue
		}
		values = append(values, number)
	}
	return values
}

// parseBuexIntRows 解析二维整数表（表情表按"页 x 编号"两层排）。
func parseBuexIntRows(value any) [][]int {
	array, ok := Array(value)
	if !ok {
		return nil
	}

	rows := make([][]int, 0, len(array))
	for _, item := range array {
		row := parseBuexInts(item)
		if len(row) == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// buexPoint 把 [x, y] 数组读成坐标，缺项按 0 处理。
func buexPoint(pair []any) (int, int) {
	var x, y int
	if len(pair) > 0 {
		x, _ = Number(pair[0])
	}
	if len(pair) > 1 {
		y, _ = Number(pair[1])
	}
	return x, y
}
