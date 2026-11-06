package session

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// custom_ui.go 实现 Bishop 的工作/行程画面（お仕事タイム）。
//
// 这套画面有两代数据来源：
//
//   - SLIE（BSXScript 3.3）：剧本发 IMAGE_EX type=176，按名字指向
//     define.bsa 里的 work/*.txt。txt 描述四块角色面板的图层（体力/好感度、
//     六格工作进度、Down 覆盖层）和单选按钮组（写回 #chN_select_work）。
//   - SL（BSXScript 2.0）：define.bsa 没有这些 txt，剧本只画背景和
//     AM/PM 标题；角色面板、六格工作、进度条和按钮全部来自
//     bsxx.cfg 的 [custom map phase]。
//
// 两者共用同一套交互语义：每个角色选一个工作（AM 1..6 / PM 7..12），
// 或选“休憩”99；START 把选择结果交给剧本继续。
//
// 放在 Session 而不是 rendering 包的原因：取帧/条件都依赖脚本属性表
// （#ch1_vitality、#briefing.ch1opacity1、#ch1_select_work），渲染包
// 不该依赖 vm。SceneComposer 只提供“剧情层之上、舞台按钮之下”的
// overlay 回调，层序由它保证。

const (
	customHoverNone = iota
	customHoverPanelButton
	customHoverWork
	customHoverBreak
)

// customUIPanel 是一块 type=176 自定义窗口。
//
// 用坐标而不是名字做键：SLIE 先画四块 AM 面板，再画四块 PM 面板，坐标
// 相同、名字不同；按坐标替换才能让 PM 接替 AM，而不是两套叠在一起。
type customUIPanel struct {
	name string
	x, y int
	def  *config.CustomUIDefinition
}

type customUIHover struct {
	kind   int
	panel  int
	group  int
	button int
}

func (h customUIHover) matches(kind, panel, group, button int) bool {
	return h.kind == kind && h.panel == panel && h.group == group && h.button == button
}

// ---------------------------------------------------------------- 定义加载

// customDefinition 从资源包里的同名 .txt 读取 type=176 定义。
//
// 不能直接用 TryGet(name)：同名图片（work_start.bsg）和定义
// （work/work_start.txt）都存在于资源索引里，TryGet 不保证返回 txt。
// 这里按扩展名精确遍历一次，并按名字缓存，避免每帧扫 BSA 目录。
func (s *Session) customDefinition(name string) *config.CustomUIDefinition {
	if len(name) == 0 {
		return nil
	}
	if definition, ok := s.customDefs[name]; ok {
		return definition
	}
	if s.customDefMissing != nil && s.customDefMissing[name] {
		return nil
	}
	if s.customDefs == nil {
		s.customDefs = make(map[string]*config.CustomUIDefinition)
	}
	if s.customDefMissing == nil {
		s.customDefMissing = make(map[string]bool)
	}
	if s.res == nil {
		s.customDefMissing[name] = true
		return nil
	}
	for _, archive := range s.res.Archives() {
		for _, entry := range archive.Entries() {
			base := path.Base(strings.ReplaceAll(entry.Path, "\\", "/"))
			extension := path.Ext(base)
			if !strings.EqualFold(extension, ".txt") {
				continue
			}
			if !strings.EqualFold(strings.TrimSuffix(base, extension), name) {
				continue
			}
			data, err := archive.ReadAll(entry)
			if err != nil {
				core.Logf("custom ui: read '%s' failed: %v", name, err)
				break
			}
			definition, err := config.ParseCustomUIDefinition(data)
			if err != nil {
				core.Logf("custom ui: parse '%s' failed: %v", name, err)
				break
			}
			s.customDefs[name] = definition
			return definition
		}
	}
	s.customDefMissing[name] = true
	return nil
}

// ---------------------------------------------------------------- 面板生命周期

// showCustomUI 添加/替换一块 type=176 面板。
func (s *Session) showCustomUI(name string, x, y int) {
	definition := s.customDefinition(name)
	if definition == nil {
		core.Logf("custom ui: definition '%s' is missing", name)
		return
	}
	for index := range s.customPanels {
		if s.customPanels[index].x == x && s.customPanels[index].y == y {
			s.customPanels[index] = customUIPanel{name: name, x: x, y: y, def: definition}
			s.composer.Invalidate()
			return
		}
	}
	s.customPanels = append(s.customPanels, customUIPanel{name: name, x: x, y: y, def: definition})
	s.composer.Invalidate()
	core.Logf("custom ui: panel '%s' at (%d,%d)", name, x, y)
}

// clearCustomUI 清掉工作画面状态。
//
// 剧本切到新背景（IMAGE/IMAGE_EX type=1/2）或玩家点了 START/HELP 后，
// 上一屏的面板必须消失；否则新场景上会残留角色面板和单选按钮。
func (s *Session) clearCustomUI() {
	if len(s.customPanels) > 0 {
		core.Logf("custom ui: cleared %d panel(s)", len(s.customPanels))
	}
	s.customMapHelp = false
	s.customHover = customUIHover{}
	if len(s.customPanels) == 0 {
		return
	}
	s.customPanels = nil
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// ---------------------------------------------------------------- 属性与条件

func (s *Session) propertySlot(name string) int {
	if s.script == nil || len(name) == 0 || name[0] != '#' {
		return -1
	}
	return s.script.GetPropertyIndex(name)
}

func (s *Session) propertyValue(name string) int {
	slot := s.propertySlot(name)
	if slot < 0 || s.machine == nil {
		return 0
	}
	return int(s.machine.Property(slot))
}

// propertyExpression 求值一个"属性引用或属性引用 ± 常数"的取值表达式。
//
// define.bsa 的 layout 数据里 condition_state 不总是纯属性名：らぶえろハーレム
// 的 favoreport_point 图层写的是 `#favo_point.aoi + 5`（第 0..4 帧是差分底，
// 第 5 起才是好感度等级帧）。旧实现把整串当属性名查表，必然查不到、恒取 0，
// 报表 8 行数值全部显示错误。原引擎这类字段是"先读属性、再做 ± N 偏移"，
// 所以在取帧入口统一支持 ± 常数后缀；纯属性名走原路径不受影响。
func (s *Session) propertyExpression(expression string) int {
	token := strings.TrimSpace(expression)
	if len(token) == 0 {
		return 0
	}
	// 只按第一个 +/- 切一次：属性名本身可能含字母，但不会含算术符号；
	// 常数部分允许再带一个负号（`#a + -5` 这种写法按 ParseInt 语义处理）。
	for index := 0; index < len(token); index++ {
		if token[index] != '+' && token[index] != '-' {
			continue
		}
		if index == 0 {
			continue
		}
		left := s.propertyValue(strings.TrimSpace(token[:index]))
		right, err := strconv.Atoi(strings.TrimSpace(token[index:]))
		if err != nil {
			// 右侧不是纯数字时退回整串当属性名，保持旧行为。
			return s.propertyValue(token)
		}
		return left + right
	}
	return s.propertyValue(token)
}

func (s *Session) setScriptProperty(name string, value int) bool {
	slot := s.propertySlot(name)
	if slot < 0 || s.machine == nil {
		return false
	}
	s.machine.SetProperty(slot, int32(value))
	return true
}

// evalCondition 求值 type=176/工作画面里的条件表达式。
//
// 数据里只出现四种形式：`#prop == N`、`#prop != N`、`#prop < N`、
// 以及用 `||` / `&&` 连接的组合（work_start.txt 的禁用条件就是四个
// `== 0` 的或）。没有括号和算术，所以按 || / && 切分再逐个比较即可；
// 不认识的写法按“条件成立”处理，宁可多显示一层，也不要把界面整块藏掉。
func (s *Session) evalCondition(expression string) bool {
	expression = strings.TrimSpace(expression)
	if len(expression) == 0 {
		return true
	}
	for _, orPart := range strings.Split(expression, "||") {
		all := true
		for _, andPart := range strings.Split(orPart, "&&") {
			if !s.evalConditionAtom(andPart) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func (s *Session) evalConditionAtom(atom string) bool {
	atom = strings.TrimSpace(strings.Trim(strings.TrimSpace(atom), "()"))
	if len(atom) == 0 {
		return true
	}
	for _, operator := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if index := strings.Index(atom, operator); index >= 0 {
			left := s.conditionOperand(atom[:index])
			right := s.conditionOperand(atom[index+len(operator):])
			switch operator {
			case "==":
				return left == right
			case "!=":
				return left != right
			case "<=":
				return left <= right
			case ">=":
				return left >= right
			case "<":
				return left < right
			case ">":
				return left > right
			}
		}
	}
	return s.conditionOperand(atom) != 0
}

func (s *Session) conditionOperand(token string) int {
	token = strings.TrimSpace(token)
	if len(token) == 0 {
		return 0
	}
	if token[0] == '#' {
		return s.propertyValue(token)
	}
	if value, err := strconv.ParseInt(token, 0, 32); err == nil {
		return int(value)
	}
	return 0
}

// ---------------------------------------------------------------- 绘制

// drawCustomOverlay 由 SceneComposer 在剧情层之后、舞台按钮之前调用。
func (s *Session) drawCustomOverlay(canvas *gfx.BgraCanvas) {
	if s.gameState == nil || !s.gameState.Selecting() || s.mapSource != "custom" {
		return
	}
	if s.customMapHelp {
		// 帮助图是整屏的，必须盖住按钮；它由 drawSystemOverlays 最后画，
		// 这里跳过底下的工作面板，避免两层叠字。
		return
	}
	if len(s.customPanels) > 0 {
		s.drawCustomPanels(canvas)
		return
	}
	s.drawCustomMapWork(canvas)
}

func (s *Session) drawCustomFrame(canvas *gfx.BgraCanvas, name string, x, y, frame, frames, alpha int) bool {
	if len(name) == 0 || alpha <= 0 || s.composer == nil {
		return false
	}
	image := s.composer.Load(name)
	if image == nil {
		return false
	}
	if frames <= 0 {
		frames = 1
	}
	frame = clampInt(frame, 0, frames-1)
	height := image.Height / frames
	if height <= 0 {
		return false
	}
	canvas.BlitRegion(image, 0, frame*height, image.Width, height,
		x+image.OffsetX, y+image.OffsetY, alpha)
	return true
}

func (s *Session) drawCustomPanels(canvas *gfx.BgraCanvas) {
	for panelIndex := range s.customPanels {
		panel := &s.customPanels[panelIndex]
		if panel.def == nil {
			continue
		}
		for _, object := range panel.def.Objects {
			if len(object.ValidCondition) > 0 && !s.evalCondition(object.ValidCondition) {
				continue
			}
			frame := 0
			if len(object.ConditionState) > 0 {
				frame = s.propertyExpression(object.ConditionState)
			}
			alpha := 255
			if len(object.OpacityProperty) > 0 {
				if s.propertySlot(object.OpacityProperty) >= 0 {
					alpha = clampInt(s.propertyValue(object.OpacityProperty), 0, 255)
				}
			} else if object.Opacity >= 0 {
				alpha = clampInt(object.Opacity, 0, 255)
			}
			s.drawCustomFrame(canvas, object.Image, panel.x+object.X, panel.y+object.Y,
				frame, object.MaxState, alpha)
		}
		for groupIndex, group := range panel.def.Groups {
			current := s.propertyValue(group.Result)
			for buttonIndex, button := range group.Buttons {
				disabled := len(button.DisableCondition) > 0 && s.evalCondition(button.DisableCondition)
				frame := 0
				switch {
				case disabled && button.MaxState >= 4:
					frame = 3
				case current == button.Value:
					frame = 2
				case s.customHover.matches(customHoverPanelButton, panelIndex, groupIndex, buttonIndex):
					frame = 1
				}
				s.drawCustomFrame(canvas, button.Image,
					panel.x+button.X, panel.y+button.Y, frame, button.MaxState, 255)
			}
		}
	}
}

// customMapAM 判断当前是午前（true）还是午後（false）。
//
// SL 的剧本在 SELECT_TAIL 前写 #custom_map_type：1=午前、0=午後
// （它由 #time == 2 决定）。变量不存在时按午前处理，避免一进来就是 PM。
func (s *Session) customMapAM() bool {
	if s.config == nil {
		return true
	}
	variable := s.config.CustomMap.TypeVariable
	if len(variable) == 0 || s.propertySlot(variable) < 0 {
		return true
	}
	return s.propertyValue(variable) != 0
}

func (s *Session) drawCustomMapWork(canvas *gfx.BgraCanvas) {
	if s.config == nil {
		return
	}
	layout := s.config.CustomMap
	if !layout.HasLayout() {
		return
	}
	am := s.customMapAM()
	s.drawCustomFrame(canvas, layout.BG, 0, 0, 0, 1, 255)
	base := layout.Base
	if !am {
		base = layout.Base2
	}
	if len(base) == 0 {
		if am {
			base = layout.Base2
		} else {
			base = layout.Base
		}
	}
	s.drawCustomFrame(canvas, base, 0, 0, 0, 1, 255)
	for index := range layout.Characters {
		s.drawCustomMapCharacter(canvas, index, am)
	}
}

func (s *Session) drawCustomMapCharacter(canvas *gfx.BgraCanvas, index int, am bool) {
	layout := s.config.CustomMap
	if index >= len(layout.Characters) {
		return
	}
	character := layout.Characters[index]
	panelX, panelY := character.BaseX, character.BaseY
	vitality := s.propertyValue(fmt.Sprintf("#ch%d_vitality", index+1))
	love := s.propertyValue(fmt.Sprintf("#ch%d_love", index+1))

	if len(layout.Vitality.Image) > 0 {
		s.drawCustomFrame(canvas, layout.Vitality.Image,
			panelX+layout.VitalityPos.X, panelY+layout.VitalityPos.Y,
			vitality, layout.Vitality.Frames, 255)
	}
	if len(layout.Love.Image) > 0 {
		s.drawCustomFrame(canvas, layout.Love.Image,
			panelX+layout.LovePos.X, panelY+layout.LovePos.Y,
			love, layout.Love.Frames, 255)
	}

	names := character.WorkNames(am)
	maxLevels := character.WorkMax(am)
	selected := s.propertyValue(fmt.Sprintf("#ch%d_select_work", index+1))
	for slot := 0; slot < len(layout.WorkPositions) && slot < 6; slot++ {
		point := layout.WorkPositions[slot]
		x := panelX + point.X
		y := panelY + point.Y
		if len(layout.StepBase.Image) > 0 {
			s.drawCustomFrame(canvas, layout.StepBase.Image,
				x+layout.ExpPos.X, y+layout.ExpPos.Y, 0, layout.StepBase.Frames, 255)
		}
		maxLevel := maxLevels[slot]
		if maxLevel > 0 && len(layout.Steps) > 0 {
			step := layout.Steps[clampInt(maxLevel-2, 0, len(layout.Steps)-1)]
			if len(step.Image) > 0 {
				level := s.propertyValue(workLevelProperty(index+1, am, slot+1))
				s.drawCustomFrame(canvas, step.Image,
					x+layout.ExpPos.X, y+layout.ExpPos.Y, level, step.Frames, 255)
			}
		}
		if names[slot].Empty() {
			continue
		}
		value := slot + 1
		if !am {
			value = slot + 7
		}
		frame := 0
		switch {
		case maxLevel <= 0 && names[slot].Frames >= 4:
			frame = 3
		case selected == value:
			frame = 2
		case s.customHover.matches(customHoverWork, index, 0, slot):
			frame = 1
		}
		s.drawCustomFrame(canvas, names[slot].Image, x, y, frame, names[slot].Frames, 255)
	}

	if !character.Break.Empty() {
		x := panelX + layout.BreakPos.X
		y := panelY + layout.BreakPos.Y
		frame := 0
		switch {
		case selected == 99:
			frame = 2
		case s.customHover.matches(customHoverBreak, index, 0, 0):
			frame = 1
		}
		s.drawCustomFrame(canvas, character.Break.Image, x, y, frame, character.Break.Frames, 255)
	}

	if vitality == 0 {
		s.drawCustomFrame(canvas, layout.Down.Image,
			panelX+layout.Down.X, panelY+layout.Down.Y, 0, layout.Down.Frames, 255)
	}
}

func workLevelProperty(character int, am bool, slot int) string {
	period := "am"
	if !am {
		period = "pm"
	}
	return fmt.Sprintf("#ch%d_work_%s%d", character, period, slot)
}

// ---------------------------------------------------------------- 命中与悬停

func (s *Session) customFrameRect(name string, x, y, frames int) (int, int, int, int, bool) {
	if len(name) == 0 || s.composer == nil {
		return 0, 0, 0, 0, false
	}
	image := s.composer.Load(name)
	if image == nil || image.Width <= 0 || image.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	if frames <= 0 {
		frames = 1
	}
	height := image.Height / frames
	if height <= 0 {
		return 0, 0, 0, 0, false
	}
	return x + image.OffsetX, y + image.OffsetY, image.Width, height, true
}

func hitRect(x, y, left, top, width, height int) bool {
	return x >= left && y >= top && x < left+width && y < top+height
}

func (s *Session) customPanelButtonAt(x, y int) (int, int, int, bool) {
	for panelIndex := range s.customPanels {
		panel := &s.customPanels[panelIndex]
		if panel.def == nil {
			continue
		}
		for groupIndex, group := range panel.def.Groups {
			for buttonIndex, button := range group.Buttons {
				left, top, width, height, ok := s.customFrameRect(button.Image,
					panel.x+button.X, panel.y+button.Y, button.MaxState)
				if ok && hitRect(x, y, left, top, width, height) {
					return panelIndex, groupIndex, buttonIndex, true
				}
			}
		}
	}
	return 0, 0, 0, false
}

func (s *Session) customWorkButtonAt(x, y int) (int, int, bool) {
	if s.config == nil {
		return 0, 0, false
	}
	layout := s.config.CustomMap
	if !layout.HasLayout() {
		return 0, 0, false
	}
	am := s.customMapAM()
	for index := range layout.Characters {
		character := layout.Characters[index]
		names := character.WorkNames(am)
		for slot := 0; slot < len(layout.WorkPositions) && slot < 6; slot++ {
			if names[slot].Empty() {
				continue
			}
			left, top, width, height, ok := s.customFrameRect(names[slot].Image,
				character.BaseX+layout.WorkPositions[slot].X,
				character.BaseY+layout.WorkPositions[slot].Y,
				names[slot].Frames)
			if ok && hitRect(x, y, left, top, width, height) {
				return index, slot, true
			}
		}
		if !character.Break.Empty() {
			left, top, width, height, ok := s.customFrameRect(character.Break.Image,
				character.BaseX+layout.BreakPos.X,
				character.BaseY+layout.BreakPos.Y,
				character.Break.Frames)
			if ok && hitRect(x, y, left, top, width, height) {
				return index, -1, true
			}
		}
	}
	return 0, 0, false
}

// updateCustomUIHover 更新工作画面的悬停状态；返回是否发生变化。
func (s *Session) updateCustomUIHover(x, y int) bool {
	hover := customUIHover{}
	if s.gameState != nil && s.gameState.Selecting() && s.mapSource == "custom" && !s.customMapHelp {
		if panel, group, button, ok := s.customPanelButtonAt(x, y); ok {
			hover = customUIHover{kind: customHoverPanelButton, panel: panel, group: group, button: button}
		} else if character, slot, ok := s.customWorkButtonAt(x, y); ok {
			if slot < 0 {
				hover = customUIHover{kind: customHoverBreak, panel: character}
			} else {
				hover = customUIHover{kind: customHoverWork, panel: character, button: slot}
			}
		}
	}
	if hover == s.customHover {
		return false
	}
	s.customHover = hover
	if s.composer != nil {
		s.composer.Invalidate()
	}
	return true
}

// handleCustomUIClick 处理工作画面上的点击；返回 true 表示已消费。
func (s *Session) handleCustomUIClick(x, y int) bool {
	if s.customMapHelp {
		s.customMapHelp = false
		if s.composer != nil {
			s.composer.Invalidate()
		}
		return true
	}
	if s.gameState == nil || !s.gameState.Selecting() || s.mapSource != "custom" {
		return false
	}
	if panel, group, buttonIndex, ok := s.customPanelButtonAt(x, y); ok {
		definition := s.customPanels[panel].def
		groupDefinition := definition.Groups[group]
		button := groupDefinition.Buttons[buttonIndex]
		if len(button.DisableCondition) > 0 && s.evalCondition(button.DisableCondition) {
			return true
		}
		if s.setScriptProperty(groupDefinition.Result, button.Value) {
			core.Logf("custom ui: %s = %d", groupDefinition.Result, button.Value)
			s.refreshCustomMapButtons()
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return true
		}
		return true
	}
	if character, slot, ok := s.customWorkButtonAt(x, y); ok {
		if s.propertyValue(fmt.Sprintf("#ch%d_vitality", character+1)) == 0 {
			// "Down..." 覆盖层期间角色不能工作；原引擎也不允许点。
			return true
		}
		am := s.customMapAM()
		if slot >= 0 && s.config != nil && character < len(s.config.CustomMap.Characters) {
			if maxLevel := s.config.CustomMap.Characters[character].WorkMax(am)[slot]; maxLevel <= 0 {
				return true
			}
		}
		value := slot + 1
		if slot < 0 {
			value = 99
		} else if !am {
			value = slot + 7
		}
		if s.setScriptProperty(fmt.Sprintf("#ch%d_select_work", character+1), value) {
			core.Logf("custom ui: #ch%d_select_work = %d", character+1, value)
			s.refreshCustomMapButtons()
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return true
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------- START/工具按钮

// ensureCustomMapButtons 在剧本没有自己画工具栏时补上配置里的按钮。
//
// SLIE 的剧本用 IMAGE_EX type=160 自己加了 START/HELP/存档；SL 只画了
// 背景和 AM/PM 标题，工具栏必须由引擎按 [custom map phase] 补出来。
// 工具按钮按功能键去重（customMapToolKey）：剧本可能用 map_sys_save 画，
// 配置却写 work_sys_save，只比图名会叠出两套位置不同的按钮，所以同功能键
// 视为同一颗，保留先画的（剧本自己的位置）。START/HELP 等非工具按钮仍按图名去重。
func (s *Session) ensureCustomMapButtons() {
	if s.config == nil || s.gameState == nil {
		return
	}
	phase := s.config.CustomMap
	add := func(button config.UIButton, value int) {
		if button.Empty() {
			return
		}
		key := customMapToolKey(button.Image)
		for _, existing := range s.gameState.Buttons() {
			if existing.Image == button.Image {
				return
			}
			if key != "" && customMapToolKey(existing.Image) == key {
				return
			}
		}
		s.gameState.AddButton(state.NewStageButtonWithFrames(button.Image, button.X, button.Y, value, button.Frames))
	}
	add(phase.Start, 1)
	add(phase.Help, 2)
	add(phase.QSave, 0)
	add(phase.QLoad, 0)
	add(phase.Save, 0)
	add(phase.Load, 0)
	add(phase.Config, 0)
	add(phase.Log, 0)
	add(phase.Item, 0)
}

// refreshCustomMapButtons 根据 define.bsa 的按钮定义更新禁用状态。
//
// work_start.txt 的 conditions.disable 要求四名角色都选了工作才可点。
// 单选按钮每次变化后都要重算，否则玩家选满四人后 START 仍是灰的。
func (s *Session) refreshCustomMapButtons() {
	if s.config == nil || s.gameState == nil {
		return
	}
	changed := false
	buttons := s.gameState.Buttons()
	for index := range buttons {
		definition := s.customDefinition(buttons[index].Image)
		disabled := false
		if definition != nil && len(definition.DisableCondition) > 0 {
			disabled = s.evalCondition(definition.DisableCondition)
		}
		if buttons[index].Disabled != disabled {
			buttons[index].Disabled = disabled
			changed = true
		}
	}
	if changed && s.composer != nil {
		s.composer.Invalidate()
	}
}

// customMapToolKey 把剧本/配置里的按钮图名归一成系统工具键。
//
// 工作画面和自定义地图用的是同一批功能，但两代作品的前缀不同：
// SLIE 是 work_sys_*，すくーるヘブン/GK2IEDL 的脚本自绘地图是 map_sys_*，
// 还有作品直接在 [custom map phase] 里写 qsave/save 这类裸名。只认前缀会让
// "地图上的存档/设置按钮点了没反应"，所以这里先剥前缀再按功能名匹配。
func customMapToolKey(image string) string {
	name := strings.ToLower(strings.TrimSpace(image))
	name = strings.TrimSuffix(name, ".bsg")
	for _, prefix := range []string{"work_sys_", "map_sys_", "btn_", "work_", "map_"} {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}
	switch name {
	case "qsave", "quick save", "quicksave":
		return "qsave"
	case "qload", "quick load", "quickload":
		return "qload"
	case "save":
		return "save"
	case "load":
		return "load"
	case "config", "setting", "settings":
		return "config"
	case "log", "backlog", "history":
		return "log"
	case "dress", "dressing", "striptease":
		return "dress"
	case "next", "next_time", "nexttime", "next time":
		return "next"
	case "status", "item", "report":
		return "status"
	}
	return ""
}

// handleCustomMapTool 处理工作画面工具栏里的存档/设置/回想按钮。
//
// 返回 true 表示已经消费，调用方不要把它当成一次“选择结果”继续跑剧本。
// 原引擎这些按钮会打开各自界面，工作画面保持不动；这里同样只打开
// 系统界面，不改 select 状态。
func (s *Session) handleCustomMapTool(button state.StageButton) bool {
	return s.handleCustomMapToolKey(customMapToolKey(button.Image))
}

// handleCustomMapToolKey 按功能键执行系统按钮动作。
//
// 学園（GK）的地图工具条是一张 map_submenu_button，六个按钮共用同一张图；
// 功能来自 profile 的 Tool 字段，不能再从图片名反推。
func (s *Session) handleCustomMapToolKey(tool string) bool {
	switch tool {
	case "qsave":
		s.quickSave()
		s.setToolNotice("クイックセーブ")
		return true
	case "qload":
		s.quickLoad()
		s.setToolNotice("クイックロード")
		return true
	case "save":
		if !s.openDataScreen(true) {
			core.Logf("custom ui: save screen unavailable")
		}
		return true
	case "load":
		if !s.openDataScreen(false) {
			core.Logf("custom ui: load screen unavailable")
		}
		return true
	case "config":
		if !s.openConfigScreen() {
			s.setHostRequest("settings")
		}
		return true
	case "log":
		if !s.openLogScreen() {
			s.setHostRequest("backlog")
		}
		return true
	case "dress":
		if !s.openDressScreen() {
			core.Logf("custom ui: dress screen unavailable")
		}
		return true
	case "status":
		if !s.openStatusScreen() {
			core.Logf("custom ui: status screen unavailable")
		}
		return true
	}
	return false
}
