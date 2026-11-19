package config

import (
	"errors"
	"strconv"
	"strings"
)

// status_screen.go 解析 [status screen] 指向的布局资源（define.bsa:status_layout）。
//
// 原引擎的"状態"画面不是写死的界面：它用一份描述性布局列出底图、返回按钮，
// 以及每个角色/事件图标的坐标和显示条件。条件直接引用脚本属性
// （例如 `#status_ch01 == 1`、`#status_ch01_ev01 != 0`），引擎只在条件成立时
// 把对应的 status_chNN_xx 图贴到画面上。这样剧本改数值、画面就跟着变，
// 不需要在引擎里逐部作品硬编码。
//
// 布局语法是 JSON 的宽松变体：键可以不加引号，成员之间可以省略逗号，
// 坐标允许 `63 + 133 * 2` 这样的算式。这里用一个小型递归下降解析器处理，
// 而不是拼一个正则——正则遇到嵌套的 layout/slot 很容易把条件字符串吞掉。
type StatusScreenLayout struct {
	Base string
	// ModeButtons 是画面上固定的操作按钮（本作只有 inventory_back）。
	ModeButtons []StatusModeButton
	// Pages 按布局文件里的书写顺序保存；每页有自己的底图和条件图标。
	Pages []StatusPage
}

// StatusModeButton 是状態画面上的固定按钮。
//
// MaxState 是竖排按钮图的状态帧数（常态/悬停）；No 是原引擎的按钮编号，
// 本作只用来区分按钮，不参与逻辑。
type StatusModeButton struct {
	Name     string
	MaxState int
	X        int
	Y        int
	No       int
}

// StatusPage 是状態画面的一页（本作只有 inventory）。
type StatusPage struct {
	Key   string
	Base  string
	Slots []StatusSlot
	// Achievement 是"成就列表"页的可选排版。牝教師3 的 status_layout
	// 没有 slot，整页由 name/notes 两列文字 + 滚动条组成；这些键在
	// 早期解析器里会被静默丢弃，于是状态画面只剩一张底图。
	Achievement *StatusAchievementPage
	// Chart 是 CHART（场景流程图）页的排版。这一页没有 slot，格子数据在
	// chart.db 里（见 chart.go）；即便整页只有 chart 键也必须保留，
	// 否则状态画面会把它当成空页，CHART 按钮就无处可画。
	Chart *ChartLayout
}

// ChartSceneName 是 CHART 页右上角的场景名绘制方式。
//
// Align 是 "left"/"right"（原引擎的 0/1/2 已由布局写成字符串），Edge 是文字
// 描边颜色；场景名本身由剧本变量提供，不在这份布局里。
type ChartSceneName struct {
	X, Y  int
	Align string
	Edge  uint32
}

// ChartConfirm 是 CHART 页的确认框：底图 + 确定/取消。
//
// 只有玩家点了"跳到当前场景"之类的动作才会弹出，用来确认是否放弃当前进度。
type ChartConfirm struct {
	Base   ConfigButton
	OK     ConfigButton
	Cancel ConfigButton
}

// ChartLayout 是 status_layout 里 chart 页的排版。
//
// CHART 画面只占屏幕右侧一条：DrawArea 是 [左, 上, 右, 下]，和布局里其它
// 坐标一样属于原生屏幕坐标。格子缩略图、连线来自 chart.db，这里只给出画布
// 范围、按钮和缩略图缩放比例。
type ChartLayout struct {
	DrawArea [4]int
	// Base 是画布底图，通常只画在 DrawArea 内。
	Base string
	// Loading 是读取 chart.db 时的转圈动画。
	Loading ConfigButton
	// ThumbRatio 是缩略图相对原图的缩放比例（原引擎用 .094）。
	ThumbRatio float64
	// SceneName 是当前场景名的位置和描边。
	SceneName ChartSceneName
	// Buttons 是画布上的翻页/跳转按钮，按书写顺序保存。
	Buttons []ConfigButton
	// NowScene 是名含 now_scene 的"跳到当前场景"按钮；它同时留在 Buttons 里，
	// 按名字处理的逻辑不必再遍历数组。
	NowScene ConfigButton
	Confirm  ChartConfirm
}

// StatusAchievementPage 描述成就列表页。
type StatusAchievementPage struct {
	ScrollBar   StatusScrollBar
	FontSize    int
	NameX       int
	NameY       int
	RowGap      int
	NotesX      int
	NotesY      int
	LockColor   uint32
	UnlockColor uint32
}

// StatusScrollBar 是成就页右侧的滚动条。
type StatusScrollBar struct {
	Name  string
	X     int
	Y     int
	Width int
	MaxNo int
}

// StatusSlot 是一张按条件显示的图。
type StatusSlot struct {
	Image     string
	X         int
	Y         int
	Condition string
}

// ParseStatusScreen 解析 status_layout 的文本。
func ParseStatusScreen(data []byte) (*StatusScreenLayout, error) {
	parser := &statusParser{data: data}
	value, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	root, ok := value.(*statusObject)
	if !ok {
		return nil, errors.New("status layout: root is not an object")
	}

	layout := &StatusScreenLayout{Base: statusString(root.get("base"))}
	if buttons, ok := root.get("mode button").([]any); ok {
		for _, item := range buttons {
			object, ok := item.(*statusObject)
			if !ok {
				continue
			}
			button := StatusModeButton{
				Name:     statusString(object.get("name")),
				MaxState: statusInt(object.get("max_state"), 2),
				No:       statusInt(object.get("no"), 0),
			}
			if pos, ok := object.get("pos").([]any); ok && len(pos) >= 2 {
				button.X = statusInt(pos[0], 0)
				button.Y = statusInt(pos[1], 0)
			}
			if button.Name != "" {
				layout.ModeButtons = append(layout.ModeButtons, button)
			}
		}
	}

	pages, _ := root.get("layout").(*statusObject)
	if pages != nil {
		for _, key := range pages.keys {
			pageObject, ok := pages.values[key].(*statusObject)
			if !ok {
				continue
			}
			page := StatusPage{Key: key, Base: statusString(pageObject.get("base"))}
			if slots, ok := pageObject.get("slot").([]any); ok {
				for _, item := range slots {
					slotObject, ok := item.(*statusObject)
					if !ok {
						continue
					}
					slot := StatusSlot{
						Image:     statusString(slotObject.get("name")),
						Condition: statusString(slotObject.get("condition")),
					}
					if pos, ok := slotObject.get("pos").([]any); ok && len(pos) >= 2 {
						slot.X = statusInt(pos[0], 0)
						slot.Y = statusInt(pos[1], 0)
					}
					if slot.Image != "" {
						page.Slots = append(page.Slots, slot)
					}
				}
			}
			page.Achievement = parseStatusAchievement(pageObject)
			// layout 下的每个键本身就是一页：inventory 页把 base/slot 写在
			// 键的值里，chart 页也把 draw_area/base/button 直接写在键的值里。
			// 因此这里解析的是 pageObject 本身，而不是它下面的 "chart" 键
			// （早期实现多取了一层，页面永远解析成空）。
			if key == "chart" || pageObject.get("draw_area") != nil || pageObject.get("confirm") != nil {
				page.Chart = parseStatusChart(pageObject)
			}
			layout.Pages = append(layout.Pages, page)
		}
	}
	if layout.Base == "" && len(layout.Pages) == 0 {
		return nil, errors.New("status layout: nothing to draw")
	}
	return layout, nil
}

// parseStatusAchievement 解析成就页的排版。只有同时存在 name/notes 或
// scroll bar 时才认为它是成就页；普通 slot 页没有这些键。
func parseStatusAchievement(page *statusObject) *StatusAchievementPage {
	out := &StatusAchievementPage{RowGap: 30, FontSize: 17, UnlockColor: 0xffffffff, LockColor: 0xff606060}
	found := false
	if bar, ok := page.get("scroll bar").(*statusObject); ok {
		out.ScrollBar.Name = statusString(bar.get("name"))
		out.ScrollBar.X = statusInt(bar.get("x"), 0)
		out.ScrollBar.Y = statusInt(bar.get("y"), 0)
		out.ScrollBar.Width = statusInt(bar.get("width"), 0)
		out.ScrollBar.MaxNo = statusInt(bar.get("max_no"), 0)
		found = true
	}
	if font, ok := page.get("font").(*statusObject); ok {
		out.FontSize = maxInt(statusInt(font.get("size"), out.FontSize), 8)
	}
	if name, ok := page.get("name").(*statusObject); ok {
		out.NameX = statusInt(name.get("x"), 0)
		out.NameY = statusInt(name.get("y"), 0)
		if interval, ok := name.get("interval").(*statusObject); ok {
			out.RowGap = maxInt(absInt(statusInt(interval.get("y"), out.RowGap)), 1)
		}
		found = true
	}
	if notes, ok := page.get("notes").(*statusObject); ok {
		out.NotesX = statusInt(notes.get("x"), 0)
		out.NotesY = statusInt(notes.get("y"), 0)
	}
	if color, ok := page.get("color").(*statusObject); ok {
		out.LockColor = statusColor(color.get("lock"), out.LockColor)
		out.UnlockColor = statusColor(color.get("unlock"), out.UnlockColor)
	}
	if !found {
		return nil
	}
	return out
}

// parseStatusChart 解析 chart 页——CHART 画面（场景流程图）。
//
// 注意参数就是 layout 下 chart 这一页的对象本身：布局文件写的是
// `layout: { chart: { draw_area: ... } }`，而不是 `chart: { chart: ... }`。
// 这一页和状態画面的 inventory 页共用同一份布局文件，但不是"按条件贴图"：
// 底图/按钮/确认框在这份布局里，格子与连线来自 chart.db。
func parseStatusChart(object *statusObject) *ChartLayout {
	if object == nil {
		return nil
	}

	chart := &ChartLayout{Base: statusString(object.get("base"))}
	if area, ok := object.get("draw_area").([]any); ok {
		for index := 0; index < len(chart.DrawArea) && index < len(area); index++ {
			chart.DrawArea[index] = statusInt(area[index], 0)
		}
	}
	if button, ok := parseStatusChartButton(object.get("loading")); ok {
		chart.Loading = button
	}

	// thumb_ratio 是缩略图相对原图的缩放（原引擎用 .094）。布局里写成
	// 0.094 或 .094 两种形式；缺失或坏值时沿用原引擎的默认比例，
	// 否则缩略图会按 1:1 画出去，盖满整个画布。
	chart.ThumbRatio = statusFloat(object.get("thumb_ratio"))
	if chart.ThumbRatio <= 0 {
		chart.ThumbRatio = defaultChartThumbRatio
	}

	if scene, ok := object.get("scene_name").(*statusObject); ok {
		if pos, ok := scene.get("pos").([]any); ok && len(pos) >= 2 {
			chart.SceneName.X = statusInt(pos[0], 0)
			chart.SceneName.Y = statusInt(pos[1], 0)
		}
		chart.SceneName.Align = statusString(scene.get("align"))
		chart.SceneName.Edge = statusColor(scene.get("edge"), 0)
	}

	if buttons, ok := object.get("button").([]any); ok {
		for _, item := range buttons {
			button, ok := parseStatusChartButton(item)
			if !ok {
				continue
			}
			chart.Buttons = append(chart.Buttons, button)
			if strings.Contains(button.Name, "now_scene") {
				chart.NowScene = button
			}
		}
	}

	if confirm, ok := object.get("confirm").(*statusObject); ok {
		chart.Confirm.Base, _ = parseStatusChartButton(confirm.get("base"))
		chart.Confirm.OK, _ = parseStatusChartButton(confirm.get("ok"))
		chart.Confirm.Cancel, _ = parseStatusChartButton(confirm.get("cancel"))
	}

	return chart
}

// parseStatusChartButton 把 chart 页里的按钮对象转成 ConfigButton。
// confirm.base 只有名字没有坐标/帧数（它只是底图），解析时按零值处理。
func parseStatusChartButton(value any) (ConfigButton, bool) {
	object, ok := value.(*statusObject)
	if !ok {
		return ConfigButton{}, false
	}
	name := statusString(object.get("name"))
	if name == "" {
		return ConfigButton{}, false
	}

	button := ConfigButton{Name: name, MaxState: statusInt(object.get("max_state"), 0)}
	if pos, ok := object.get("pos").([]any); ok && len(pos) >= 2 {
		button.X = statusInt(pos[0], 0)
		button.Y = statusInt(pos[1], 0)
	}
	return button, true
}

func statusColor(value any, fallback uint32) uint32 {
	if number, ok := value.(int); ok {
		return uint32(number)
	}
	text := strings.TrimSpace(statusString(value))
	if text == "" {
		return fallback
	}
	text = strings.TrimPrefix(strings.TrimPrefix(text, "0x"), "0X")
	var parsed uint64
	for index := 0; index < len(text); index++ {
		character := text[index]
		var digit byte
		switch {
		case character >= '0' && character <= '9':
			digit = character - '0'
		case character >= 'a' && character <= 'f':
			digit = character - 'a' + 10
		case character >= 'A' && character <= 'F':
			digit = character - 'A' + 10
		default:
			return fallback
		}
		parsed = parsed*16 + uint64(digit)
	}
	if parsed > 0xffffffff {
		return fallback
	}
	return uint32(parsed)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// ---------------------------------------------------------------- 词法/语法

// statusObject 保留成员的书写顺序：页面顺序必须和布局文件一致。
type statusObject struct {
	keys   []string
	values map[string]any
}

func (o *statusObject) get(key string) any {
	if o == nil {
		return nil
	}
	return o.values[key]
}

type statusParser struct {
	data []byte
	pos  int
}

func (p *statusParser) parseValue() (any, error) {
	p.skipSpace()
	if p.pos >= len(p.data) {
		return nil, errors.New("status layout: unexpected end")
	}
	switch p.data[p.pos] {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		return p.parseString()
	}
	if p.data[p.pos] == '0' && p.pos+1 < len(p.data) &&
		(p.data[p.pos+1] == 'x' || p.data[p.pos+1] == 'X') {
		// 0xAARRGGBB 颜色字面量。不能交给坐标算式：它读到 0 就停，
		// 后面的 xff... 会被当成垃圾，结果 lock/unlock 颜色都是透明。
		p.pos += 2
		value := 0
		digits := 0
		for p.pos < len(p.data) {
			character := p.data[p.pos]
			var digit int
			switch {
			case character >= '0' && character <= '9':
				digit = int(character - '0')
			case character >= 'a' && character <= 'f':
				digit = int(character-'a') + 10
			case character >= 'A' && character <= 'F':
				digit = int(character-'A') + 10
			default:
				digit = -1
			}
			if digit < 0 {
				break
			}
			value = value*16 + digit
			digits++
			p.pos++
		}
		if digits > 0 {
			return value, nil
		}
		return 0, nil
	}
	if p.data[p.pos] == '-' || (p.data[p.pos] >= '0' && p.data[p.pos] <= '9') || p.data[p.pos] == '(' {
		return p.parseExpression()
	}
	return p.parseIdentifier(), nil
}

func (p *statusParser) parseObject() (*statusObject, error) {
	p.pos++ // {
	object := &statusObject{values: map[string]any{}}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, errors.New("status layout: unterminated object")
		}
		if p.data[p.pos] == '}' {
			p.pos++
			return object, nil
		}
		if p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		key, err := p.parseKey()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, errors.New("status layout: missing ':' after " + key)
		}
		p.pos++
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if _, exists := object.values[key]; !exists {
			object.keys = append(object.keys, key)
		}
		object.values[key] = value
	}
}

func (p *statusParser) parseArray() ([]any, error) {
	p.pos++ // [
	items := []any{}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) {
			return nil, errors.New("status layout: unterminated array")
		}
		if p.data[p.pos] == ']' {
			p.pos++
			return items, nil
		}
		if p.data[p.pos] == ',' {
			p.pos++
			continue
		}
		start := p.pos
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		if p.pos == start {
			// 解析器没有前进说明遇到了它不认识的字符（例如拼错的注释）。
			// 直接报错，绝不在这里空转——这类循环会把整个引擎线程卡死。
			return nil, errors.New("status layout: parser made no progress in array")
		}
		items = append(items, value)
	}
}

func (p *statusParser) parseKey() (string, error) {
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == '"' {
		return p.parseString()
	}
	return p.parseIdentifier(), nil
}

func (p *statusParser) parseString() (string, error) {
	if p.data[p.pos] != '"' {
		return "", errors.New("status layout: expected string")
	}
	p.pos++
	var builder strings.Builder
	for p.pos < len(p.data) {
		value := p.data[p.pos]
		p.pos++
		if value == '"' {
			return builder.String(), nil
		}
		if value == '\\' && p.pos < len(p.data) {
			builder.WriteByte(p.data[p.pos])
			p.pos++
			continue
		}
		builder.WriteByte(value)
	}
	return "", errors.New("status layout: unterminated string")
}

// parseIdentifier 读取裸键/裸值，包含 #、-、. 等属性名里可能出现的字符。
func (p *statusParser) parseIdentifier() string {
	start := p.pos
	for p.pos < len(p.data) {
		value := p.data[p.pos]
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '_' || value == '#' || value == '.' {
			p.pos++
			continue
		}
		break
	}
	return string(p.data[start:p.pos])
}

// parseExpression 解析坐标里的整数算式（`63 + 133 * 2`）。
func (p *statusParser) parseExpression() (any, error) {
	start := p.pos
	depth := 0
	for p.pos < len(p.data) {
		value := p.data[p.pos]
		if value == '(' {
			depth++
		} else if value == ')' {
			depth--
		} else if depth == 0 && (value == ',' || value == ']' || value == '}' || value == '\n' || value == '\r') {
			break
		}
		p.pos++
	}
	expression := strings.TrimSpace(string(p.data[start:p.pos]))
	value, ok := evalStatusExpression(expression)
	if !ok {
		// 小数字段（chart 页的 thumb_ratio: 0.094）不是坐标算式：按浮点
		// 字面量收下并保留小数位。坐标走的仍是上面的整数求值。
		if number, err := strconv.ParseFloat(expression, 64); err == nil {
			return number, nil
		}
		return 0, nil
	}
	return value, nil
}

func (p *statusParser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		case ';':
			// status_layout 和 define.bsa 的其它布局一样用 ';' 开始行注释。
			// 不跳过注释会让 parseArray 在注释行上读到空标识符且游标不前进，
			// 整份 status_layout（含 CHART 页）解析永远不返回——真机上就是
			// 点 CHART 后画面卡死/没有任何反应。
			for p.pos < len(p.data) && p.data[p.pos] != '\n' && p.data[p.pos] != '\r' {
				p.pos++
			}
		default:
			return
		}
	}
}

// evalStatusExpression 计算只含整数与 + - * / ( ) 的坐标算式。
func evalStatusExpression(text string) (int, bool) {
	tokens := make([]string, 0, 8)
	for index := 0; index < len(text); {
		value := text[index]
		if value == ' ' || value == '\t' {
			index++
			continue
		}
		if value >= '0' && value <= '9' {
			// 多位数字要一次读完：逐字符拆成 token 会把 901 变成 9、0、1。
			start := index
			for index < len(text) && text[index] >= '0' && text[index] <= '9' {
				index++
			}
			tokens = append(tokens, text[start:index])
			continue
		}
		if value == '+' || value == '-' || value == '*' || value == '/' ||
			value == '(' || value == ')' {
			tokens = append(tokens, string(value))
			index++
			continue
		}
		return 0, false
	}
	if len(tokens) == 0 {
		return 0, false
	}
	expression := &statusExpr{tokens: tokens}
	value, ok := expression.parseAdd()
	if !ok || expression.pos != len(tokens) {
		return 0, false
	}
	return value, true
}

// statusExpr 是坐标算式的小型递归下降求值器（+ - * / 与括号）。
type statusExpr struct {
	tokens []string
	pos    int
}

func (e *statusExpr) parseAdd() (int, bool) {
	left, ok := e.parseMul()
	if !ok {
		return 0, false
	}
	for e.pos < len(e.tokens) && (e.tokens[e.pos] == "+" || e.tokens[e.pos] == "-") {
		op := e.tokens[e.pos]
		e.pos++
		right, ok := e.parseMul()
		if !ok {
			return 0, false
		}
		if op == "+" {
			left += right
		} else {
			left -= right
		}
	}
	return left, true
}

func (e *statusExpr) parseMul() (int, bool) {
	left, ok := e.parseAtom()
	if !ok {
		return 0, false
	}
	for e.pos < len(e.tokens) && (e.tokens[e.pos] == "*" || e.tokens[e.pos] == "/") {
		op := e.tokens[e.pos]
		e.pos++
		right, ok := e.parseAtom()
		if !ok {
			return 0, false
		}
		if op == "*" {
			left *= right
		} else {
			if right == 0 {
				return 0, false
			}
			left /= right
		}
	}
	return left, true
}

func (e *statusExpr) parseAtom() (int, bool) {
	if e.pos >= len(e.tokens) {
		return 0, false
	}
	token := e.tokens[e.pos]
	if token == "+" || token == "-" {
		e.pos++
		value, ok := e.parseAtom()
		if !ok {
			return 0, false
		}
		if token == "-" {
			return -value, true
		}
		return value, true
	}
	if token == "(" {
		e.pos++
		value, ok := e.parseAdd()
		if !ok {
			return 0, false
		}
		if e.pos < len(e.tokens) && e.tokens[e.pos] == ")" {
			e.pos++
		}
		return value, true
	}
	value, err := strconv.Atoi(token)
	if err != nil {
		return 0, false
	}
	e.pos++
	return value, true
}

// ---------------------------------------------------------------- 取值辅助

func statusString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	}
	return ""
}

func statusInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case string:
		if parsed, err := strconv.Atoi(typed); err == nil {
			return parsed
		}
	}
	return fallback
}

// statusFloat 读取小数字段。布局里的写法有两种：0.094 会被词法器当成算式
// 解析成 float64；.094 以 '.' 开头，词法器把它当裸标识符返回字符串。
// 两种都要认，缺失或坏值返回 0 交给调用方决定默认值。
func statusFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			return parsed
		}
	}
	return 0
}
