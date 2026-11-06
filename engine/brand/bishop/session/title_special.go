package session

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
)

// title_special.go 处理标题画面的隐藏菜单（special）。
//
// 原作的 [title phase] 除了常规按钮，还会给一个 `special = title_menu_sp`
// 入口，并用两行配置控制它是否出现：
//
//	use special mode = 1
//	use conditions   = @harem == 0xf
//
// `@harem` 是 BSXScript 的“变量表”（@ = variable），不是属性表（# = property）。
// 通关后脚本把若干条路线的完成位写进 @harem，凑满 0xf 才会出现隐藏菜单。
// 因此这里必须按变量表读取；用 GetPropertyIndex 会永远找不到 harem，
// 菜单要么永远不出现，要么只能不加条件地暴露给新玩家。

// titleVisibleEntries 返回当前应该显示/可点的标题菜单项。
//
// 隐藏菜单按 use special mode 与 use conditions 过滤。其它菜单项原样返回，
// 顺序也不变——GameState 的按钮下标和这份切片的顺序一一对应。
func (s *Session) titleVisibleEntries() []config.TitleMenuEntry {
	if s.config == nil {
		return nil
	}
	entries := s.config.Title.Entries
	visible := make([]config.TitleMenuEntry, 0, len(entries))
	for _, entry := range entries {
		if s.titleEntryVisible(entry) {
			visible = append(visible, entry)
		}
	}
	return visible
}

// titleKeyVisible 判断某个菜单键当前是否可见。
//
// 宿主可能拿着旧的标题热区列表来点击；入口不存在或条件不满足时，
// 这里再判一次，避免"看不见的按钮"仍然能进隐藏路线。
func (s *Session) titleKeyVisible(key string) bool {
	for _, entry := range s.titleVisibleEntries() {
		if entry.Key == key {
			return true
		}
	}
	return false
}

func (s *Session) titleEntryVisible(entry config.TitleMenuEntry) bool {
	if entry.Key != TitleKeySpecial {
		return true
	}
	title := s.config.Title
	if !title.UseSpecialMode {
		return false
	}
	condition := strings.TrimSpace(title.UseConditions)
	if condition == "" {
		return true
	}
	return s.titleConditionMet(condition)
}

// titleConditionMet 求值 `use conditions`。
//
// 语法与状态画面的 condition 一致：&& 表示同时成立、|| 表示任一成立，
// 单个条件形如 `@变量 运算符 整数`。整数支持 0xf 这种十六进制写法
// （本作的条件正是 `@harem == 0xf`）。
func (s *Session) titleConditionMet(condition string) bool {
	for _, alternative := range strings.Split(condition, "||") {
		if s.titleConditionAnd(alternative) {
			return true
		}
	}
	return false
}

func (s *Session) titleConditionAnd(condition string) bool {
	for _, term := range strings.Split(condition, "&&") {
		if !s.titleConditionTerm(term) {
			return false
		}
	}
	return true
}

func (s *Session) titleConditionTerm(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" {
		return true
	}
	for _, operator := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		index := strings.Index(term, operator)
		if index < 0 {
			continue
		}
		name := strings.TrimSpace(term[:index])
		right := strings.TrimSpace(term[index+len(operator):])
		value, ok := parseTitleNumber(right)
		if !ok {
			return false
		}
		current, ok := s.titleScriptValue(name)
		if !ok {
			// 变量还没被脚本创建时按 0 处理：条件写 == 0xf 不会误显示，
			// 写 != 0 也不会把未通关误判成已通关。
			current = 0
		}
		switch operator {
		case "==":
			return current == value
		case "!=":
			return current != value
		case ">=":
			return current >= value
		case "<=":
			return current <= value
		case ">":
			return current > value
		case "<":
			return current < value
		}
	}
	return false
}

// titleScriptValue 读取条件里的脚本值。
//
// `@名字` 查变量表，`#名字` 查属性表；没写前缀时先按属性、再按变量，
// 兼容其它作品可能出现的简写。变量/属性的存储是分开的，不能混用。
func (s *Session) titleScriptValue(name string) (int, bool) {
	if s.script == nil || s.machine == nil || name == "" {
		return 0, false
	}
	switch name[0] {
	case '@':
		index := s.script.GetVariableIndex(name)
		variables := s.machine.Variables()
		if index < 0 || index >= len(variables) {
			return 0, false
		}
		return int(variables[index]), true
	case '#':
		return s.titlePropertyValue(name)
	default:
		if value, ok := s.titlePropertyValue(name); ok {
			return value, true
		}
		index := s.script.GetVariableIndex(name)
		variables := s.machine.Variables()
		if index < 0 || index >= len(variables) {
			return 0, false
		}
		return int(variables[index]), true
	}
}

func (s *Session) titlePropertyValue(name string) (int, bool) {
	index := s.script.GetPropertyIndex(name)
	if index < 0 {
		return 0, false
	}
	return int(s.machine.Property(index)), true
}

// parseTitleNumber 解析条件右侧的整数，支持十进制、负数和 0x 十六进制。
func parseTitleNumber(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	negative := false
	if text[0] == '+' || text[0] == '-' {
		negative = text[0] == '-'
		text = text[1:]
	}
	if text == "" {
		return 0, false
	}
	var value int64
	var err error
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		value, err = strconv.ParseInt(text[2:], 16, 64)
	} else {
		value, err = strconv.ParseInt(text, 10, 64)
	}
	if err != nil || int64(int(value)) != value {
		return 0, false
	}
	if negative {
		value = -value
	}
	return int(value), true
}
