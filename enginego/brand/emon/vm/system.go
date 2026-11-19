package vm

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"kizuna/enginego/brand/emon/save"
	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/brand/emon/stage"
	emontext "kizuna/enginego/brand/emon/text"
	"kizuna/enginego/core"
)

const (
	maxRunDepth             = 8
	quickSaveSlot           = 990
	actionWaitNoEnd         = 0x1
	actionWaitNoSpUp        = 0x2
	actionWaitSelectOff     = 0x4
	messageSelectNumberBase = 0x10000
	soundEndDrainLimit      = 64
)

type ClickArea struct {
	machine *Machine
	Sprite  string
	RawX1   string
	RawY1   string
	RawX2   string
	RawY2   string
	Kind    string
	Macro   string
}

func (a *ClickArea) Contains(x, y int) bool {
	x1 := resolveClickCoordinate(a.machine, a.RawX1)
	y1 := resolveClickCoordinate(a.machine, a.RawY1)
	x2 := resolveClickCoordinate(a.machine, a.RawX2)
	y2 := resolveClickCoordinate(a.machine, a.RawY2)
	area := OrderRect(x1, y1, RectEdge(x2, x1), RectEdge(y2, y1))
	return x >= area.X1 && x < area.X2 && y >= area.Y1 && y < area.Y2
}

func resolveClickCoordinate(machine *Machine, raw string) int {
	if value, ok := tryParseInt(raw); ok {
		return value
	}
	return machine.evalExpression(raw)
}

type MouseFollow struct {
	Sprite    string
	RawDeltaX string
	RawDeltaY string
}

func (f *MouseFollow) DeltaX(machine *Machine) int {
	return machine.evalExpression(f.RawDeltaX)
}

func (f *MouseFollow) DeltaY(machine *Machine) int {
	return machine.evalExpression(f.RawDeltaY)
}

// registerEngineSettings 预置原引擎启动时注册的系统设置变量。
// ASENURE 的 asenure.exe 用 sub_418640 把 MAINVOLUME / MUSICVOLUME /
// MessageSpeed / EffectMode 等挂在系统变量表上，并注册回调让改动立即生效；
// 这些变量随 emesys.dat 持久化。我们的 VM 没有这层注册，脚本里的
// #SetNumber(MessageSpeed=...) 会落到普通变量，重启就丢。这里按原引擎的
// 默认值补登记，已有（从 emesys.dat 恢复）的值不会被覆盖。
// 只登记引擎级、与具体剧情无关的名字，不影响其它 Emon 游戏的脚本变量。
func (m *Machine) registerEngineSettings() {
	defaults := []struct {
		name  string
		value script.EmonValue
	}{
		{"MAINVOLUME", script.EmonValueFromString("1.0")},
		{"MUSICVOLUME", script.EmonValueFromString("0.5")},
		{"VOICEVOLUME", script.EmonValueFromString("0.5")},
		{"SEVOLUME", script.EmonValueFromString("0.5")},
		{"効果音VOLUME", script.EmonValueFromString("0.5")},
		{"効果２VOLUME", script.EmonValueFromString("0.5")},
		{"MessageSpeed", script.EmonValueFromInt(1)},
		{"EffectMode", script.EmonValueFromInt(0)},
		{"ScreenMode", script.EmonValueFromInt(0)},
		{"SkipVoice", script.EmonValueFromInt(0)},
		{"未読スキップ", script.EmonValueFromInt(0)},
		// オートモード設定：原引擎 sub_410800/410840/410880 的默认值。
		{"AUTOWAIT", script.EmonValueFromInt(200)},
		{"AUTOMIN", script.EmonValueFromInt(2)},
		{"AUTOMAX", script.EmonValueFromInt(15)},
	}
	for _, entry := range defaults {
		if m.context.SystemVariables.Contains(entry.name) {
			continue
		}
		m.context.SystemVariables.Set(entry.name, entry.value)
		m.context.Variables.Remove(entry.name)
	}
}

// SettingFloat 读取引擎设置变量；音量在原引擎里是 float，脚本按文本存小数。
func (m *Machine) SettingFloat(name string, fallback float64) float64 {
	if m == nil || m.context == nil {
		return fallback
	}
	value := m.Lookup(name)
	text := strings.TrimSpace(value.AsString())
	if text == "" {
		return fallback
	}
	if parsed, err := parseFloatInvariant(text); err == nil {
		return parsed
	}
	return fallback
}

// SettingInt 读取引擎设置变量（整数语义，例如文字速度档位）。
func (m *Machine) SettingInt(name string, fallback int) int {
	if m == nil || m.context == nil {
		return fallback
	}
	text := strings.TrimSpace(m.Lookup(name).AsString())
	if text == "" {
		return fallback
	}
	if parsed, err := parseFloatInvariant(text); err == nil {
		return int(parsed)
	}
	return fallback
}

func (m *Machine) pumpSystemState() {
	if !m.systemStateReady {
		m.systemStateReady = true
		if store, ok := m.context.SystemVariables.(*script.EmonVariableStore); ok {
			if m.context.Saves.SystemState().Load(store) {
				m.dropShadowedLocalVariables()
				core.Logf("emon: system state restored variables=%d", m.context.SystemVariables.Count())
			}
			m.registerEngineSettings()
		}
		return
	}
	if store, ok := m.context.SystemVariables.(*script.EmonVariableStore); ok {
		m.context.Saves.SystemState().Pump(store)
	}
}

// FlushSystemState 在会话关闭/切游戏前强制落盘系统设置，避免玩家改完
// 音量立刻退出时被 5 秒节流窗口吞掉。
func (m *Machine) FlushSystemState() {
	m.flushSystemState()
}

func (m *Machine) flushSystemState() {
	if !m.systemStateReady {
		return
	}
	if m.context.Saves.SystemState().Write(m.CaptureSystemState()) {
		core.Logf("emon: system state flushed variables=%d", m.context.SystemVariables.Count())
	}
}

func (m *Machine) onSaveSystem(statement *script.EmonStatement) EmonStep {
	m.flushSystemState()
	// 既読记录也是“系统数据”：原引擎在 SaveSystem 时一并落盘，
	// 避免玩家存档后杀进程、下次进来这些行又变回未读。
	if m.context != nil && m.context.Reads != nil {
		m.context.Reads.Flush()
	}
	return EmonStepContinue
}

func (m *Machine) onBackLog(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	if strings.EqualFold(m.raw(statement, 0), "RESET") {
		m.ClearHistory()
		if m.backLog != nil {
			m.backLog.Maximum = max(0, len(m.history)-1)
			m.backLog.Top = m.backLog.Maximum
			m.refreshBackLog()
			m.step = EmonStepWait
		}
		return EmonStepContinue
	}
	m.spriteParams.Set("BackLog", m.joinRaw(statement, 0))
	if m.openBackLog(statement) {
		return EmonStepWait
	}
	return EmonStepContinue
}

func (m *Machine) onSetBackLog(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() < 2 {
		return EmonStepContinue
	}
	if strings.EqualFold(m.raw(statement, 0), "SOUND") {
		m.spriteParams.Set("BackLogSound", m.text(statement, 1))
		return EmonStepContinue
	}
	if !m.isReplaying() {
		m.AddHistory(m.text(statement, 1))
	}
	return EmonStepContinue
}

func (m *Machine) onBackLogMessage(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	keyword := m.raw(statement, 0)
	if strings.EqualFold(keyword, "GETSOUND") {
		if statement.ArgumentCount() < 3 {
			m.logOnce("back log message get sound needs an index and a variable")
			return EmonStepContinue
		}
		slot := m.EvalInt(statement, 1, 0)
		voice := ""
		if slot >= 0 && slot < len(m.historyVoices) {
			voice = m.historyVoices[slot]
		}
		m.writeBackLogRecord(m.raw(statement, 2), voice)
		return EmonStepContinue
	}
	if strings.EqualFold(keyword, "GETTEXT") {
		if statement.ArgumentCount() < 3 {
			m.logOnce("back log message get text needs an index and a variable")
			return EmonStepContinue
		}
		slot := m.EvalInt(statement, 1, 0)
		text := ""
		if slot >= 0 && slot < len(m.history) {
			text = m.history[slot]
		}
		m.writeBackLogRecord(m.raw(statement, 2), text)
		return EmonStepContinue
	}
	if strings.EqualFold(keyword, "NULL") {
		if statement.ArgumentCount() < 2 {
			m.logOnce("back log message null needs a variable")
			return EmonStepContinue
		}
		name := m.raw(statement, 1)
		m.backLogCursor = core.Clamp(m.backLogCursor+int(m.Lookup(name).AsInt()), 0, max(0, len(m.history)-1))
		m.SetVariable(name, script.EmonValueFromInt(int32(m.backLogCursor)))
		return EmonStepContinue
	}
	return m.flushMessageSprite(statement, keyword)
}

func (m *Machine) flushMessageSprite(statement *script.EmonStatement, spriteName string) EmonStep {
	if statement.ArgumentCount() < 2 {
		m.logOnce("back log message '" + spriteName + "' needs a variable")
		return EmonStepContinue
	}
	sprite := m.findSprite(spriteName)
	if sprite == nil {
		m.logOnce("back log message sprite '" + spriteName + "' was not found")
		return EmonStepContinue
	}
	isMessageBox := spriteName != "" && strings.EqualFold(spriteName, m.messageBoxSprite())
	if sprite.TextSetting() == nil && sprite.Text() == nil && !isMessageBox {
		m.logOnce("back log message sprite '" + spriteName + "' has no message lines")
		return EmonStepContinue
	}
	content := m.messageSpriteText(spriteName, sprite, isMessageBox)
	pending := content
	if flushed, ok := m.backLogFlushed.Get(spriteName); ok {
		if strings.HasPrefix(content, flushed) {
			pending = content[len(flushed):]
		} else {
			pending = content
		}
	}
	m.backLogFlushed.Set(spriteName, content)
	if pending != "" && !m.isReplaying() {
		for _, line := range strings.Split(pending, "\n") {
			m.AddHistory(strings.TrimSpace(line))
		}
	}
	m.backLogCursor = max(0, len(m.history)-1)
	if m.backLog != nil {
		m.backLog.Maximum = m.backLogCursor
		m.refreshBackLog()
	}
	m.SetVariable(m.raw(statement, 1), script.EmonValueFromInt(int32(m.backLogCursor)))
	return EmonStepContinue
}

func (m *Machine) messageSpriteText(name string, sprite *stage.EmonSprite, isMessageBox bool) string {
	content := ""
	if text := sprite.Text(); text != nil {
		content = text.Content()
	}
	if content == "" {
		if stored, ok := m.spriteText.Get(name); ok {
			content = stored
		}
	}
	if content == "" && isMessageBox {
		content = m.context.Message.Body()
	}
	return content
}

func (m *Machine) writeBackLogRecord(name, value string) {
	if name == "" {
		return
	}
	if value == "" {
		m.SetVariable(name, script.EmonValueFromInt(0))
		return
	}
	m.SetVariable(name, script.EmonValueFromString(value))
}

func (m *Machine) openBackLog(statement *script.EmonStatement) bool {
	outerName := m.text(statement, 0)
	innerName := m.text(statement, 1)
	var outer *stage.EmonSprite
	if outerName != "" {
		outer = m.context.Stage.Find(outerName)
	}
	var inner *stage.EmonSprite
	if innerName != "" {
		inner = m.context.Stage.Find(innerName)
	}
	var setting *emontext.EmonSpriteTextSetting
	if inner != nil {
		setting = inner.TextSetting()
	}
	if inner == nil || setting == nil {
		m.logOnce("back log '" + innerName + "' has no text box")
		return false
	}
	cursorName := m.text(statement, 2)
	var cursor *stage.EmonSprite
	if cursorName != "" {
		cursor = m.context.Stage.Find(cursorName)
	}
	window := NewEmonBackLogWindow(outer, inner, cursor, setting)
	for slot := 0; slot < 4; slot++ {
		start := int32(3 + slot*6)
		if start+5 >= statement.ArgumentCount() {
			break
		}
		window.Rects[slot] = EmonRect{X1: m.EvalInt(statement, start, 0), Y1: m.EvalInt(statement, start+1, 0), X2: m.EvalInt(statement, start+2, 0), Y2: m.EvalInt(statement, start+3, 0)}
		window.OffsetX[slot] = m.EvalInt(statement, start+4, 0)
		window.OffsetY[slot] = m.EvalInt(statement, start+5, 0)
	}
	if window.Cursor != nil {
		cursor := window.Cursor
		window.SavedCursorSprite = m.cursorSprite()
		window.SavedCursorX = m.cursorX()
		window.SavedCursorY = m.cursorY()
		window.CursorHasSourceRect = cursor.HasSourceRect()
		window.CursorSourceX = cursor.SourceX()
		window.CursorSourceY = cursor.SourceY()
		window.CursorSourceWidth = cursor.SourceWidth()
		window.CursorSourceHeight = cursor.SourceHeight()
		m.setCursorSprite(cursor.Name())
	}
	if outer != nil {
		outer.SetVisible(false)
	}
	inner.SetVisible(true)
	m.backLog = window
	window.Maximum = max(0, len(m.history)-1)
	window.Top = window.Maximum
	m.applyBackLogZone(-1)
	m.refreshBackLog()
	core.Logf("emon: back log opened on '%s' at %d record(s), window %d line(s)", innerName, len(m.history), window.Capacity)
	return true
}

func (m *Machine) closeBackLog() {
	window := m.backLog
	if window == nil {
		return
	}
	m.backLog = nil
	m.backLogRevision++
	if window.Outer != nil {
		window.Outer.SetVisible(window.OuterVisible)
	}
	if text := window.Inner.Text(); text != nil {
		text.Clear()
	}
	window.Inner.SetVisible(window.InnerVisible)
	if window.Cursor != nil {
		cursor := window.Cursor
		if window.CursorHasSourceRect {
			cursor.SetSourceRect(window.CursorSourceX, window.CursorSourceY, window.CursorSourceWidth, window.CursorSourceHeight)
		} else {
			cursor.ClearSourceRect()
		}
		m.setCursorSprite(window.SavedCursorSprite)
		m.setCursorX(window.SavedCursorX)
		m.setCursorY(window.SavedCursorY)
	}
	if m.step == EmonStepWait {
		m.step = EmonStepContinue
	}
	core.Logf("emon: back log closed")
}

func (m *Machine) refreshBackLog() {
	window := m.backLog
	if window == nil {
		return
	}
	content := window.Inner.Text()
	if content == nil {
		content = emontext.NewEmonSpriteText()
	}
	window.Inner.SetText(content)
	content.SetContent(window.Build(m.history, emontext.ResolveGlyphSource()))
	m.backLogRevision++
}

func (m *Machine) scrollBackLog(direction int, page bool) bool {
	window := m.backLog
	if window == nil {
		return false
	}
	step := 1
	if page {
		step = window.Capacity
	}
	if direction < 0 {
		if window.Top <= 0 {
			m.step = EmonStepWait
			return true
		}
		window.Top = max(0, window.Top-step)
	} else {
		if window.Top >= window.Maximum {
			m.closeBackLog()
			return false
		}
		window.Top = min(window.Maximum, window.Top+step)
	}
	m.refreshBackLog()
	m.step = EmonStepWait
	return true
}

func (m *Machine) backLogKey(keyCode int) {
	switch keyCode {
	case 0x0D, 0x1B, 0x20:
		m.closeBackLog()
	case 0x21:
		m.scrollBackLog(-1, true)
	case 0x22:
		m.scrollBackLog(1, true)
	case 0x26:
		m.scrollBackLog(-1, false)
	case 0x28:
		m.scrollBackLog(1, false)
	default:
		m.step = EmonStepWait
	}
}

func (m *Machine) backLogPointer(x, y int, rightButton bool) EmonStep {
	window := m.backLog
	if window == nil {
		return EmonStepContinue
	}
	if rightButton {
		m.closeBackLog()
		return m.step
	}
	zone := backLogZone(m.context.Height(), y)
	m.applyBackLogZone(zone)
	switch zone {
	case 0:
		m.scrollBackLog(-1, true)
	case 1:
		m.scrollBackLog(-1, false)
	case 2:
		m.scrollBackLog(1, false)
	case 3:
		m.scrollBackLog(1, true)
	default:
		m.step = EmonStepWait
	}
	m.backLogVoice(zone, x, y)
	return m.step
}

func (m *Machine) backLogVoice(zone, x, y int) {
	window := m.backLog
	if zone != -1 || window == nil {
		return
	}
	metrics := window.Metrics
	left := window.Inner.AbsoluteX() + window.Setting.X()
	top := window.Inner.AbsoluteY() + window.Setting.Y()
	if x < left || x >= left+metrics.WrapWidth || y < top || y >= top+metrics.LineHeight*window.Capacity {
		return
	}
	row := (y - top) / max(1, metrics.LineHeight)
	index := window.Top + row
	if index < 0 || index >= len(m.history) || index >= len(m.historyVoices) {
		return
	}
	if voice := m.historyVoices[index]; voice != "" {
		m.PlayVoiceSound(voice, EmonSoundVoice, false)
	}
}

func (m *Machine) applyBackLogZone(zone int) {
	window := m.backLog
	if window == nil || window.Zone == zone {
		return
	}
	window.Zone = zone
	cursor := window.Cursor
	if cursor == nil {
		return
	}
	slot := window.ZoneArgument()
	if slot < 0 {
		cursor.ClearSourceRect()
		m.setCursorX(0)
		m.setCursorY(0)
		return
	}
	rect := window.Rects[slot]
	cursor.SetSourceRect(rect.X1, rect.Y1, max(1, rect.X2-rect.X1+1), max(1, rect.Y2-rect.Y1+1))
	m.setCursorX(window.OffsetX[slot])
	m.setCursorY(window.OffsetY[slot])
}

func backLogZone(height, y int) int {
	if height <= 0 {
		return -1
	}
	if y < (height*16)/100 {
		return 0
	}
	if y < (height*40)/100 {
		return 1
	}
	if y <= (height*60)/100 {
		return -1
	}
	if y <= (height*84)/100 {
		return 2
	}
	return 3
}

func (m *Machine) onMakeSave(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() != 0 {
		m.logOnce("make save arguments are ignored")
	}
	// 只抓快照不写盘：宏随后会隐藏 HUD 计算各槽属性，直接在此处把 990 落盘会
	// 存下半成品；后续 Save(1000)/Save(slot) 用这份 pendingSave 补字段即可。
	if data := m.CaptureSessionState(quickSaveSlot, m.context.Message.Body()); data != nil {
		m.pendingSave = data
	}
	return EmonStepContinue
}

func (m *Machine) onSave(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() == 0 {
		core.Logf("emon: save needs a slot")
		return EmonStepContinue
	}
	m.writeSaveSlot(statement, false)
	return EmonStepContinue
}

func (m *Machine) onReSetSaveData(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() == 0 {
		core.Logf("emon: reset save data needs a slot")
		return EmonStepContinue
	}
	m.writeSaveSlot(statement, true)
	return EmonStepContinue
}

func (m *Machine) writeSaveSlot(statement *script.EmonStatement, update bool) {
	slot := m.EvalInt(statement, 0, 0)
	// Save 的字段必须严格按参数顺序落盘：CheckSaveData 是按位置回写的，
	// 把 REALTIME 和普通变量分两轮收集会把 変数１/変数３ 的日期、时间、
	// 注释全部错位（ASENURE 的存档界面正是这样读的）。所以先按顺序记下
	// 每个 token（含 REALTIME 占位），再一次性求值。
	type saveToken struct {
		raw      string
		realtime bool
	}
	tokens := []saveToken{}
	var thumbnailSprite *string
	capture := []int{}
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		raw := m.raw(statement, i)
		if strings.EqualFold(raw, "THUMBNAIL") {
			if i+1 < statement.ArgumentCount() {
				value := m.raw(statement, i+1)
				thumbnailSprite = &value
				capture = m.EvalIntegers(statement, i+2)
			}
			break
		}
		if isRealtimeKeyword(raw) {
			tokens = append(tokens, saveToken{raw: raw, realtime: true})
			continue
		}
		if raw != "" {
			tokens = append(tokens, saveToken{raw: raw})
		}
	}
	// 最后一个普通变量在原引擎里同时是读取界面显示的注释；它仍是 Save
	// 的一个字段，CheckSaveData 按位置回写时必须能拿到。
	lastVariable := -1
	for i := len(tokens) - 1; i >= 0; i-- {
		if !tokens[i].realtime {
			lastVariable = i
			break
		}
	}
	now := time.Now()
	comment := ""
	fields := []script.EmonValue{}
	for i, token := range tokens {
		if token.realtime {
			fields = append(fields, script.EmonValueFromString(saveRealtimeText(token.raw, now)))
			continue
		}
		if i == lastVariable {
			value := m.Lookup(token.raw)
			if isMiss(value, token.raw) {
				value = script.EmonValueFromString("")
			}
			comment = value.AsString()
			fields = append(fields, value)
			continue
		}
		fields = append(fields, m.saveFieldValue(token.raw, now))
	}
	// update=true（ReSetSaveData 改备注）只改已有存档：重新 Capture 会覆盖
	// 剧情指针与画面，这里直接读存档，只更新 Comment/Fields/缩略图。
	if update {
		existing := m.findSaveData(slot)
		if existing == nil {
			return
		}
		// 深拷贝：findSaveData 可能返回 store 里的指针，直接改会污染缓存。
		data := save.EmonSaveDataFromPayload(existing.ToPayload())
		data.Slot = int32(slot)
		data.Comment = comment
		data.Fields = fields
		if data.SystemVariables == nil {
			data.SystemVariables = m.CaptureSystemState()
		}
		if slot < 0 || slot > save.MaxSlot {
			m.scratchSaves[slot] = data
		} else if !m.context.Saves.Write(data) {
			core.Logf("emon: save slot %d failed", slot)
			return
		}
		if thumbnailSprite != nil && *thumbnailSprite != "" && len(capture) >= 4 {
			captureX := capture[0]
			captureY := capture[1]
			m.captureThumbnail(slot, *thumbnailSprite, captureX, captureY, captureX+capture[2], captureY+capture[3])
		}
		return
	}
	// MakeSave 在宏改动 HUD 之前已抓快照；Save 若再 Capture 一次，会存下宏隐藏
	// 后的 HUD（ZUBUNURE 独占 / BONYUMANAMI 会話）致读档消失。优先用 pendingSave
	// 的深拷贝（避免后续修改污染快照），只补 Slot/Comment/Fields/SystemVariables。
	// pendingSave 不清空：Save(1000) 与最终 Save(slot) 共用同一快照。
	var data *EmonSaveData
	if m.pendingSave != nil {
		data = save.EmonSaveDataFromPayload(m.pendingSave.ToPayload())
	} else {
		data = m.CaptureSessionState(slot, comment)
	}
	if data == nil {
		return
	}
	data.Slot = int32(slot)
	data.Comment = comment
	data.SavedAt = now
	// 存档的字段是 Save 参数逐个求值的结果，不是“最后一次赋值”；此处补齐系统变量快照，
	// 与 emesys.dat 的独立持久化互不冲突（Load 仍走 RestoreState 的既有路径）。
	if data.SystemVariables == nil {
		data.SystemVariables = m.CaptureSystemState()
	}
	data.Fields = fields
	if slot < 0 || slot > save.MaxSlot {
		m.scratchSaves[slot] = data
	} else if !m.context.Saves.Write(data) {
		core.Logf("emon: save slot %d failed", slot)
	}
	if thumbnailSprite != nil && *thumbnailSprite != "" && len(capture) >= 4 {
		captureX := capture[0]
		captureY := capture[1]
		m.captureThumbnail(slot, *thumbnailSprite, captureX, captureY, captureX+capture[2], captureY+capture[3])
	}
}

// saveFieldValue 返回 Save 字段 i 的存档值：已定义的变量直接存其值，未定义的
// 年月日時分秒 占位名（クイックセーブ 的 何月/何日/何時 等）才填入当前时刻分量。
// 不得像旧实现那样对所有字段调用 fillSaveVariable，否则会把 変数１..５ 等
// 工作变量在 Save 执行时改写成日期串。
func (m *Machine) saveFieldValue(name string, now time.Time) script.EmonValue {
	if value := m.Lookup(name); !isMiss(value, name) {
		return value
	}
	m.fillSaveVariable(name, now)
	return m.Lookup(name)
}

// saveRealtimeText 格式化 Save 参数里 REALTIME_TYPEA/B 的插入字段。
// 与原引擎一致：A 为完整日期时刻，B 为短版 YY/MM/DD HH:MM。
func saveRealtimeText(keyword string, now time.Time) string {
	if strings.EqualFold(keyword, "REALTIME_TYPEB") {
		return now.Format("06/01/02 15:04")
	}
	return now.Format("2006-01-02 15:04:05")
}

func (m *Machine) fillSaveVariable(name string, now time.Time) {
	switch {
	case strings.ContainsRune(name, '月'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Month())))
	case strings.ContainsRune(name, '日'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Day())))
	case strings.ContainsRune(name, '時'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Hour())))
	case strings.ContainsRune(name, '分'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Minute())))
	case strings.ContainsRune(name, '年'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Year())))
	case strings.ContainsRune(name, '秒'):
		m.SetVariable(name, script.EmonValueFromInt(int32(now.Second())))
	default:
		m.SetVariable(name, script.EmonValueFromString(now.Format("2006/01/02")))
	}
}

func isRealtimeKeyword(name string) bool {
	return strings.EqualFold(name, "REALTIME_TYPEA") || strings.EqualFold(name, "REALTIME_TYPEB")
}

func (m *Machine) onLoad(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	slot := m.EvalInt(statement, 0, 0)
	data := m.findSaveData(slot)
	if data == nil {
		m.logOnce("load slot " + strconv.Itoa(slot) + " is empty")
		return EmonStepContinue
	}
	if !m.RestoreState(data) {
		core.Logf("emon: load slot %d failed", slot)
		return EmonStepContinue
	}
	// 存档点可能是在存档/读档界面里按下的：SpriteStates 里带着界面精灵的
	// DISPON，原样恢复会让读档后仍停在界面。清掉 UI 才能像原版一样回剧情。
	m.clearLoadUi()
	// 画面根节点按存档前的快照恢复（背景/CG/调教界面），否则只剩对白文字。
	if !m.restoreScreenRoots(data.ScreenRoots) {
		m.restoreGameScreen()
	}
	m.restoreModeHud()
	m.pendingSave = nil
	return EmonStepContinue
}

// LoadSlot 直接读取指定槽位（宿主兜底用）。正常流程走脚本的 #Load；
// 但当设备上的点选没有把槽位编号传进宏时，会话层可以用它保证读档可用。
// ClearLoadUi 供会话层在直接读档后收起存档界面。
func (m *Machine) ClearLoadUi() {
	m.clearLoadUi()
}

func (m *Machine) LoadSlot(slot int) bool {
	if m.isReplaying() {
		return false
	}
	data := m.findSaveData(slot)
	if data == nil {
		return false
	}
	if !m.RestoreState(data) {
		return false
	}
	m.clearLoadUi()
	if !m.restoreScreenRoots(data.ScreenRoots) {
		m.restoreGameScreen()
	}
	m.restoreModeHud()
	m.pendingSave = nil
	return true
}

func (m *Machine) onCheckSaveData(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		core.Logf("emon: check save data needs a slot")
		return EmonStepContinue
	}
	slot := m.EvalInt(statement, 0, 0)
	data := m.findSaveData(slot)
	names := []string{}
	var thumbnailSprite *string
	draw := []int{}
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		raw := m.raw(statement, i)
		if strings.EqualFold(raw, "THUMBNAIL") {
			if i+1 < statement.ArgumentCount() {
				value := m.raw(statement, i+1)
				thumbnailSprite = &value
				draw = m.EvalIntegers(statement, i+2)
			}
			break
		}
		if strings.EqualFold(raw, "NULL") {
			// NULL 只占一个字段位置，不写回任何变量（与原引擎跳过记录一致）。
			names = append(names, "")
			continue
		}
		if raw != "" {
			names = append(names, raw)
		}
	}
	if len(names) == 1 {
		exists := 0
		if data != nil {
			exists = 1
		}
		m.SetVariable(names[0], script.EmonValueFromInt(int32(exists)))
	} else {
		// 原引擎 CheckSaveData 按存档里 Save 参数的顺序，把每个字段原样写回同序的
		// 变量名；NULL 是空洞、THUMBNAIL 只画缩略图。Fields 为空的是旧存档，
		// 此时才退化为「日期/时刻/备注」的近似映射。
		for i := 0; i < len(names); i++ {
			if names[i] == "" {
				continue
			}
			if data != nil && len(data.Fields) > 0 {
				if i < len(data.Fields) {
					m.SetVariable(names[i], data.Fields[i])
				} else {
					m.SetVariable(names[i], script.EmonValueFromString(""))
				}
			} else {
				date := ""
				clock := ""
				comment := ""
				if data != nil {
					date = data.SavedAt.Format("2006/01/02")
					clock = data.SavedAt.Format("15:04")
					comment = data.Comment
				}
				fields := []string{date, "", clock, comment}
				value := ""
				if i < len(fields) {
					value = fields[i]
				}
				m.SetVariable(names[i], script.EmonValueFromString(value))
			}
		}
	}
	if thumbnailSprite != nil && *thumbnailSprite != "" && len(draw) >= 4 {
		m.drawThumbnail(slot, *thumbnailSprite, draw[0], draw[1], draw[2], draw[3])
	}
	return EmonStepContinue
}

func (m *Machine) findSaveData(slot int) *EmonSaveData {
	if scratch, ok := m.scratchSaves[slot]; ok {
		return scratch
	}
	if slot < 0 || slot > save.MaxSlot {
		return nil
	}
	return m.context.Saves.Read(slot)
}

func (m *Machine) onRun(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		m.logOnce("run needs a target")
		return EmonStepContinue
	}
	target := m.text(statement, 0)
	if target == "" {
		m.logOnce("run target is empty")
		return EmonStepContinue
	}
	openURL := m.context.OpenUrl
	if openURL == nil {
		m.logOnce("run '" + target + "' ignored")
		return EmonStepContinue
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				core.Logf("emon: run '%s' failed: %v", target, recovered)
			}
		}()
		openURL(target)
	}()
	return EmonStepContinue
}

func (m *Machine) onMessageBox(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	parts := []string{}
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		parts = append(parts, m.text(statement, i))
	}
	m.setPendingMessage(strings.Join(parts, "\n"))
	m.messageWaitClears = true
	return EmonStepAwaitInput
}

func (m *Machine) onGetStringMessage(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	prompt := m.text(statement, 0)
	if statement.ArgumentCount() > 2 {
		prompt = m.text(statement, 1)
	}
	m.setPrompt(prompt, m.raw(statement, statement.ArgumentCount()-1), false)
	return EmonStepAwaitName
}

func (m *Machine) onGetYesNoMessage(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	prompt := m.text(statement, 0)
	if statement.ArgumentCount() > 3 {
		prompt = m.text(statement, 1)
	}
	m.setPrompt(prompt, m.raw(statement, statement.ArgumentCount()-1), true)
	return EmonStepAwaitName
}

func (m *Machine) onRunAction(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		name := m.raw(statement, i)
		if name == "" {
			continue
		}
		if _, ok := m.runActions.Get(name); ok {
			m.executeRunAction(name, 0)
		} else if macro, ok := m.context.Macros.TryGet(name); ok {
			m.invokeMacro(statement, macro)
		} else {
			m.logOnce("run action '" + name + "' was not found")
		}
	}
	return EmonStepContinue
}

func (m *Machine) onDelRunAction(statement *script.EmonStatement) EmonStep {
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		if name := m.raw(statement, i); name != "" {
			m.runActions.Delete(name)
		}
	}
	return EmonStepContinue
}

func (m *Machine) onSetAction(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() < 3 {
		core.Logf("emon: set action needs a name, a trigger and a handler")
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	trigger := m.raw(statement, 1)
	kind := parseTriggerKind(trigger)
	allowLocked := parseTriggerUnnever(trigger)
	keyCode := parseTriggerKeyCode(trigger)
	var selector string
	var condition string
	var handler string
	if isSpriteTrigger(kind) {
		if statement.ArgumentCount() > 2 {
			selector = m.raw(statement, 2)
		}
		handler = m.joinHandlers(statement, 3)
		m.removeTriggersWhere(func(existing *EmonActionTrigger) bool {
			return !existing.AllowLocked &&
				strings.EqualFold(existing.Name, name) &&
				existing.Kind == kind &&
				strings.EqualFold(existing.Selector, selector)
		})
	} else {
		selector = trigger
		if (kind == EmonActionFlagOn || kind == EmonActionFlagOff) && statement.ArgumentCount() > 2 {
			selector = m.raw(statement, 2)
		}
		if statement.ArgumentCount() > 3 && !isASCIIDigits(m.raw(statement, 2)) {
			condition = m.raw(statement, 2)
		}
		if statement.ArgumentCount() > 3 {
			handler = m.joinHandlers(statement, 3)
		} else {
			handler = m.raw(statement, 2)
		}
		m.removeTriggersWhere(func(existing *EmonActionTrigger) bool {
			return !existing.AllowLocked &&
				strings.EqualFold(existing.Name, name) &&
				existing.Kind == kind
		})
	}
	soundVoice := kind == EmonActionSoundEnd && triggerTokenHasPart(trigger, "VOICE")
	soundEffect := kind == EmonActionSoundEnd && (triggerTokenHasPart(trigger, "SECH") || !soundVoice)
	soundChannel := -1
	if kind == EmonActionSoundEnd {
		soundChannel = parseTriggerSoundChannel(trigger)
	}
	m.triggers = append(m.triggers, &EmonActionTrigger{
		Name:                    name,
		Kind:                    kind,
		Selector:                selector,
		Condition:               condition,
		Handler:                 handler,
		KeyCode:                 keyCode,
		AllowLocked:             allowLocked,
		SoundVoice:              soundVoice,
		SoundEffectChannel:      soundEffect,
		SoundEffectChannelIndex: soundChannel,
		OneTime:                 kind == EmonActionSoundEnd && triggerTokenHasPart(trigger, "ONETIME"),
	})
	return EmonStepContinue
}

func (m *Machine) joinHandlers(statement *script.EmonStatement, start int32) string {
	parts := []string{}
	for i := start; i < statement.ArgumentCount(); i++ {
		if raw := m.raw(statement, i); raw != "" {
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, "\n")
}

func (m *Machine) onDelAction(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if statement.ArgumentCount() == 1 {
		m.removeTriggersWhere(func(existing *EmonActionTrigger) bool {
			return strings.EqualFold(existing.Name, name)
		})
		return EmonStepContinue
	}
	for i := int32(1); i < statement.ArgumentCount(); i++ {
		token := strings.ToUpper(m.raw(statement, i))
		m.removeTriggersWhere(func(existing *EmonActionTrigger) bool {
			return strings.EqualFold(existing.Name, name) &&
				strings.Contains(strings.ToUpper(existing.Selector), token)
		})
	}
	return EmonStepContinue
}

func (m *Machine) onSetRunAction(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() == 0 {
		return EmonStepContinue
	}
	name := m.raw(statement, 0)
	if name == "" {
		return EmonStepContinue
	}
	m.runActions.Set(name, &EmonRunAction{Name: name, Ops: m.parseRunOps(statement)})
	return EmonStepContinue
}

func (m *Machine) onActionWait(statement *script.EmonStatement) EmonStep {
	if m.isReplaying() {
		return EmonStepContinue
	}
	flags := 0
	for i := int32(0); i < statement.ArgumentCount(); i++ {
		flags |= parseActionWaitFlags(m.raw(statement, i))
	}
	m.actionWaitFlags = flags
	m.actionWaiting = true
	m.actionWaitFrameDepth = len(m.frames)
	return EmonStepAwaitAction
}

func parseActionWaitFlags(token string) int {
	flags := 0
	for _, part := range strings.Split(token, "@") {
		switch strings.ToUpper(strings.TrimSpace(part)) {
		case "MTNNOEND":
			flags |= actionWaitNoEnd
		case "MTNNOSPUP":
			flags |= actionWaitNoSpUp
		case "SELECTOFF":
			flags |= actionWaitSelectOff
		}
	}
	return flags
}

func (m *Machine) onClearIntersept(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("clear intersept arguments are ignored")
	}
	m.lastHover = ""
	m.hoveredSprite = ""
	for _, trigger := range m.triggers {
		trigger.Latched = false
	}
	return EmonStepContinue
}

func (m *Machine) parseRunOps(statement *script.EmonStatement) []EmonActionOp {
	ops := []EmonActionOp{}
	argc := int(statement.ArgumentCount())
	i := 1
	for i < argc {
		token := m.raw(statement, int32(i))
		parts := runOperationParts(token)
		if len(parts) == 0 {
			i++
			continue
		}
		supported := true
		for _, part := range parts {
			if !isRunOperation(part) {
				supported = false
				break
			}
		}
		if !supported {
			m.logOnce("run action operation '" + token + "' is not supported")
			break
		}
		i++
		allSingle := true
		for _, part := range parts {
			if runOperationArgumentCount(part) != 1 {
				allSingle = false
				break
			}
		}
		if allSingle {
			if i >= argc {
				break
			}
			arg := m.raw(statement, int32(i))
			i++
			for _, part := range parts {
				ops = append(ops, EmonActionOp{Code: part, Args: []string{arg}})
			}
			continue
		}
		for _, part := range parts {
			ops = append(ops, m.consumeRunOperation(statement, &i, part))
		}
	}
	return ops
}

func runOperationParts(token string) []string {
	parts := []string{}
	for _, part := range strings.Split(token, "@") {
		if name := strings.ToUpper(strings.TrimSpace(part)); name != "" {
			parts = append(parts, name)
		}
	}
	return parts
}

func isRunOperation(code string) bool {
	switch code {
	case "ACTION", "MACRO", "NUMBER", "SOUNDPLAY", "SOUNDSTOP",
		"SPRON", "SPROFF", "SPRANMON", "SPRANMOFF", "SPRMTNON", "SPRMTNOFF",
		"SPRSELECTON", "SPRSELECTOFF", "SPREFCON", "SPRITESELECTRUN", "SPRRELMOVE",
		"FLAGON", "FLAGOFF", "SETSPRITEPOS", "SETSPRITERECT", "SETSPRITETRANCE",
		"SETSPRITECOLORSCALE", "SPRMOUSEMOVE", "SPREREA", "ACTWAITOFF":
		return true
	}
	return false
}

func runOperationArgumentCount(code string) int {
	switch code {
	case "SPRON", "SPROFF", "SPRANMON", "SPRANMOFF", "SPRMTNON", "SPRMTNOFF",
		"SPRSELECTON", "SPRSELECTOFF", "SPREFCON", "SPRITESELECTRUN",
		"FLAGON", "FLAGOFF", "ACTION", "MACRO", "NUMBER", "SOUNDPLAY":
		return 1
	case "SETSPRITETRANCE", "SETSPRITECOLORSCALE":
		return 2
	case "SETSPRITEPOS", "SPRMOUSEMOVE":
		return 3
	case "SPREREA":
		return 7
	}
	return 0
}

func (m *Machine) consumeRunOperation(statement *script.EmonStatement, index *int, code string) EmonActionOp {
	switch code {
	case "SETSPRITERECT":
		return m.consumeRectOp(statement, index)
	case "SPRRELMOVE":
		return m.consumeRelativeMoveOp(statement, index)
	}
	return m.consumeRunOp(statement, index, code, runOperationArgumentCount(code))
}

func (m *Machine) consumeRunOp(statement *script.EmonStatement, index *int, code string, count int) EmonActionOp {
	args := []string{}
	argc := int(statement.ArgumentCount())
	for taken := 0; taken < count && *index < argc; taken++ {
		args = append(args, m.raw(statement, int32(*index)))
		*index++
	}
	return EmonActionOp{Code: code, Args: args}
}

func (m *Machine) consumeRectOp(statement *script.EmonStatement, index *int) EmonActionOp {
	args := []string{}
	if *index < int(statement.ArgumentCount()) {
		args = append(args, m.raw(statement, int32(*index)))
		*index++
	}
	m.appendRunRectArguments(statement, index, &args)
	return EmonActionOp{Code: "SETSPRITERECT", Args: args}
}

func (m *Machine) consumeRelativeMoveOp(statement *script.EmonStatement, index *int) EmonActionOp {
	args := []string{}
	argc := int(statement.ArgumentCount())
	for taken := 0; taken < 4 && *index < argc; taken++ {
		args = append(args, m.raw(statement, int32(*index)))
		*index++
	}
	m.appendRunRectArguments(statement, index, &args)
	return EmonActionOp{Code: "SPRRELMOVE", Args: args}
}

func (m *Machine) appendRunRectArguments(statement *script.EmonStatement, index *int, args *[]string) {
	values := 0
	argc := int(statement.ArgumentCount())
	for values < 4 && *index < argc {
		token := m.raw(statement, int32(*index))
		counted := rectTokenValueCount(m.LookupArgument(token).AsString())
		if counted == 0 {
			break
		}
		*args = append(*args, token)
		values += counted
		*index++
	}
}

func rectTokenValueCount(expanded string) int {
	for _, part := range strings.Split(expanded, ",") {
		token := strings.TrimSpace(part)
		if IsDefaultRectToken(token) || IsMessageSizeRectToken(token) {
			return 4
		}
	}
	if integers, ok := tryParseIntList(expanded); ok {
		return len(integers)
	}
	return 0
}

func (m *Machine) executeRunAction(name string, depth int) {
	if depth > maxRunDepth {
		core.Logf("emon: run action '%s' is nested too deep", name)
		return
	}
	action, ok := m.runActions.Get(name)
	if !ok {
		return
	}
	for _, op := range action.Ops {
		m.executeRunOp(op, depth)
	}
}

func (m *Machine) executeRunOp(op EmonActionOp, depth int) {
	switch op.Code {
	case "ACTION":
		if len(op.Args) > 0 && op.Args[0] != "" {
			m.executeRunAction(op.Args[0], depth+1)
		}
	case "MACRO":
		target := ""
		if len(op.Args) > 0 {
			target = op.Args[0]
		}
		if macro, ok := m.context.Macros.TryGet(target); ok && len(op.Args) > 0 {
			if program := m.getMacroProgram(macro); program != nil {
				m.pushMacroFrame(macro.Name, program, nil)
			}
		} else {
			m.logOnce("run action macro '" + target + "' was not found")
		}
	case "NUMBER":
		if len(op.Args) > 0 {
			m.applyAssignments(op.Args[0], m.SetVariable)
		}
	case "SOUNDPLAY":
		if len(op.Args) > 0 {
			file := strings.TrimSpace(strings.Split(m.Lookup(op.Args[0]).AsString(), ",")[0])
			if file != "" {
				channel := m.seRoundRobin % soundEffectChannels
				m.playChannelSound(channel, file, EmonSoundEffect, false)
				m.actionSoundChannel = channel
				m.seRoundRobin++
			}
		}
	case "SPRON", "SPROFF":
		if len(op.Args) > 0 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				sprite.SetVisible(op.Code == "SPRON")
			}
		}
	case "SPRSELECTON", "SPRSELECTOFF":
		if len(op.Args) > 0 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				if op.Code == "SPRSELECTON" {
					sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsSelect)
				} else {
					sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsSelect)
				}
				m.EnsureSelectOrder(sprite)
			}
		}
	case "SPRANMON", "SPRANMOFF":
		if len(op.Args) > 0 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				if op.Code == "SPRANMON" {
					sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsAnimation)
				} else {
					sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsAnimation)
				}
			}
		}
	case "SPRMTNON", "SPRMTNOFF":
		if len(op.Args) > 0 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				if op.Code == "SPRMTNON" {
					sprite.SetFlags(sprite.Flags() | stage.EmonSpriteFlagsMotion)
				} else {
					sprite.SetFlags(sprite.Flags() &^ stage.EmonSpriteFlagsMotion)
					delete(m.motions, strings.ToUpper(op.Args[0]))
				}
			}
		}
	case "SPREFCON", "SPRITESELECTRUN":
		m.logOnce("run action operation '" + op.Code + "' is not applied")
	case "SETSPRITEPOS":
		if len(op.Args) >= 3 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				sprite.SetX(m.evalExpression(op.Args[1]))
				sprite.SetY(m.evalExpression(op.Args[2]))
			}
		}
	case "SETSPRITERECT":
		m.executeRectOp(op.Args)
	case "SETSPRITETRANCE":
		if len(op.Args) > 1 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				if alpha, ok := tryParseInt(m.Lookup(op.Args[1]).AsString()); ok {
					sprite.SetAlpha(alpha)
				}
			}
		}
	case "SETSPRITECOLORSCALE":
		if len(op.Args) > 1 {
			if sprite := m.findSprite(op.Args[0]); sprite != nil {
				if level, ok := tryParseInt(m.Lookup(op.Args[1]).AsString()); ok {
					amount := core.Clamp(level, 0, 256)
					if m.colorSprites.Contains(op.Args[0]) {
						sprite.SetAlpha(amount * 255 / 256)
					} else {
						sprite.SetColorAmount(amount)
					}
				}
			}
		}
	// SPREREA:精灵移动范围（滑杆/旋钮），非点击热区。
	// 后续拖动、SPRMOUSEMOVE 跟随都要被 clampSpriteToArea 限制在该矩形内。
	case "SPREREA":
		m.applySpriteArea(op.Args)
	case "SPRMOUSEMOVE":
		if len(op.Args) >= 3 {
			m.applyMouseFollow(&MouseFollow{Sprite: op.Args[0], RawDeltaX: op.Args[1], RawDeltaY: op.Args[2]}, true)
		}
	case "ACTWAITOFF":
		m.resolveActionWaitCore()
	case "FLAGON", "FLAGOFF":
		if len(op.Args) > 0 && op.Args[0] != "" {
			value := int32(0)
			if op.Code == "FLAGON" {
				value = 1
			}
			m.SetVariable(op.Args[0], script.EmonValueFromInt(value))
		}
	case "SOUNDSTOP":
		if m.actionSoundChannel >= 0 {
			if !m.isReplaying() {
				m.context.Audio.StopChannel(m.actionSoundChannel)
			}
			m.SetChannelFile(m.actionSoundChannel, "")
			delete(m.channelLoops, m.actionSoundChannel)
			m.actionSoundChannel = -1
		}
	case "SPRRELMOVE":
		m.logOnce("run action operation 'SPRRELMOVE' is not applied")
	}
}

func (m *Machine) applySpriteArea(args []string) {
	if len(args) < 7 {
		return
	}
	sprite := m.findSprite(args[0])
	if sprite == nil {
		return
	}
	// 坐标支持变量/负数（如负数表示相对 p1 的偏移），先求值再用 OrderRect 规整。
	x1 := resolveClickCoordinate(m, args[1])
	y1 := resolveClickCoordinate(m, args[2])
	x2 := resolveClickCoordinate(m, args[3])
	y2 := resolveClickCoordinate(m, args[4])
	area := OrderRect(x1, y1, RectEdge(x2, x1), RectEdge(y2, y1))
	if m.spriteAreas == nil {
		m.spriteAreas = make(map[string]EmonRect)
	}
	m.spriteAreas[strings.ToUpper(sprite.Name())] = area
	m.clampSpriteToArea(sprite, area)
	// args[5] 声明后续变化的执行方式，args[6] 是目标：与原有热区一致，MACRO 压帧、ACTION 跑 RunAction。
	kind := strings.ToUpper(args[5])
	if kind == "ACTION" {
		m.executeRunAction(args[6], 0)
		return
	}
	if kind == "MACRO" {
		if macro, ok := m.context.Macros.TryGet(args[6]); ok {
			if program := m.getMacroProgram(macro); program != nil {
				m.pushMacroFrame(macro.Name, program, nil)
			}
			return
		}
	}
	m.logOnce("run action area kind '" + args[5] + "' is not supported")
}

func (m *Machine) clampSpriteToArea(sprite *stage.EmonSprite, area EmonRect) {
	if sprite == nil {
		return
	}
	// 右/下边界要减去精灵尺寸，让整个精灵留在矩形内。若范围比精灵还窄，
	// 用 low 兜底，避免 Clamp(low > high) 在 low/high 之间跳到右边界。
	maxX := area.X2 - sprite.Width()
	if maxX < area.X1 {
		maxX = area.X1
	}
	maxY := area.Y2 - sprite.Height()
	if maxY < area.Y1 {
		maxY = area.Y1
	}
	sprite.SetX(core.Clamp(sprite.X(), area.X1, maxX))
	sprite.SetY(core.Clamp(sprite.Y(), area.Y1, maxY))
	// 这里不能再 fireMoveTriggers：调用链本身就是 SPRMOVE → SPREREA →
	// clamp → SPRMOVE，重入会无限递归。
}

func (m *Machine) executeRectOp(args []string) {
	if len(args) == 0 {
		return
	}
	sprite := m.findSprite(args[0])
	if sprite == nil {
		return
	}
	rect := []int{}
	fullImage := false
	for _, token := range args[1:] {
		expanded := m.LookupArgument(token).AsString()
		for _, part := range strings.Split(expanded, ",") {
			text := strings.TrimSpace(part)
			if IsDefaultRectToken(text) || IsMessageSizeRectToken(text) {
				fullImage = true
				continue
			}
			if value, ok := tryParseInt(text); ok {
				rect = append(rect, value)
			}
		}
	}
	if fullImage {
		m.ResetToFullImage(sprite)
		return
	}
	if len(rect) == 0 {
		return
	}
	source := sprite.ResolveImage()
	if source == nil {
		source = m.SourceImage(sprite.Name())
	}
	m.applyRecordedRect(sprite, ResolveRect(rectHead(rect), source))
}

func (m *Machine) executeHandlerTarget(handler string) {
	if handler == "" {
		return
	}
	for _, name := range strings.Split(handler, "\n") {
		if target := strings.TrimSpace(name); target != "" {
			m.executeSingleHandler(target)
		}
	}
}

func (m *Machine) executeSingleHandler(handler string) {
	if m.runActions.Contains(handler) {
		m.executeRunAction(handler, 0)
		return
	}
	if macro, ok := m.context.Macros.TryGet(handler); ok {
		if program := m.getMacroProgram(macro); program != nil {
			m.pushMacroFrame(macro.Name, program, nil)
		}
		return
	}
	m.logOnce("action handler '" + handler + "' was not found")
}

func (m *Machine) resolveActionWaitCore() {
	m.actionWaiting = false
	m.actionWaitFrameDepth = -1
	m.waitFrames = 0
	m.mouseFollow = nil
	if m.actionWaitFlags&actionWaitSelectOff != 0 {
		m.selectLocked = true
	}
	m.actionWaitFlags = 0
	if m.step == EmonStepAwaitAction || m.step == EmonStepAwaitChoice {
		m.step = EmonStepContinue
	}
}

func (m *Machine) clearActionEphemera() {
	m.mouseFollow = nil
	m.lastHover = ""
	m.pointerDown = false
}

func (m *Machine) pollConditionTriggers() {
	m.pumpSystemState()
	if m.polling || len(m.triggers) == 0 {
		return
	}
	m.polling = true
	defer func() { m.polling = false }()
	snapshot := append([]*EmonActionTrigger(nil), m.triggers...)
	for _, trigger := range snapshot {
		if trigger.Kind != EmonActionCondition && trigger.Kind != EmonActionFlagOn && trigger.Kind != EmonActionFlagOff {
			continue
		}
		if !m.triggerEnabled(trigger) {
			continue
		}
		active := false
		switch trigger.Kind {
		case EmonActionCondition:
			active = m.conditionHolds(trigger)
		case EmonActionFlagOn:
			active = m.flagValue(trigger)
		default:
			active = !m.flagValue(trigger)
		}
		if active == trigger.Latched {
			continue
		}
		trigger.Latched = active
		if active {
			m.executeHandlerTarget(trigger.Handler)
		}
	}
}
func (m *Machine) pollSoundEndTriggers() {
	if m.polling || len(m.soundCompletions) == 0 {
		return
	}
	m.polling = true
	defer func() { m.polling = false }()
	suppress := m.skip
	m.soundGate.Lock()
	count := len(m.soundCompletions)
	if count > soundEndDrainLimit {
		count = soundEndDrainLimit
	}
	batch := make([]EmonSoundCompletion, count)
	copy(batch, m.soundCompletions[:count])
	m.soundCompletions = append([]EmonSoundCompletion(nil), m.soundCompletions[count:]...)
	m.soundGate.Unlock()
	for _, completion := range batch {
		m.fireSoundEndCompletion(completion, suppress)
	}
}

func (m *Machine) fireSoundEndCompletion(completion EmonSoundCompletion, suppress bool) {
	snapshot := append([]*EmonActionTrigger(nil), m.triggers...)
	for _, trigger := range snapshot {
		if trigger.Kind != EmonActionSoundEnd {
			continue
		}
		if completion.Voice {
			if !trigger.SoundVoice {
				continue
			}
		} else if !trigger.SoundEffectChannel {
			continue
		} else if trigger.SoundEffectChannelIndex >= 0 && completion.Channel >= 0 && trigger.SoundEffectChannelIndex != completion.Channel {
			continue
		}
		if !m.triggerEnabled(trigger) || suppress || !m.conditionHolds(trigger) {
			continue
		}
		m.executeHandlerTarget(trigger.Handler)
		if trigger.OneTime {
			m.removeTrigger(trigger)
		}
	}
}

func (m *Machine) flagValue(trigger *EmonActionTrigger) bool {
	if trigger.Selector == "" {
		return false
	}
	if audio, ok := m.mutableAudioState().Get(trigger.Selector); ok {
		return audio == "1" || audio == "ON" || audio == "TRUE"
	}
	return m.Lookup(trigger.Selector).AsInt() != 0
}

// triggerEnabled 判定一个触发器当前是否可执行。
//
// 原引擎的 #Lock（sub_422510）只冻结“对白送り”，不冻结 SPRCLICK/SPROVER/
// FLAGON 等触发器：标题菜单就是在 #Lock 之后轮询 SelectNumber，工具条也是
// 在对白锁定（等待点击）期间靠 hover 滑入。早期实现把 #Lock 当成全局输入锁，
// 结果标题按钮、存档/读档工具条全部变成死键。这里不再按 locked 过滤。
func (m *Machine) triggerEnabled(trigger *EmonActionTrigger) bool {
	return trigger != nil
}

func (m *Machine) conditionHolds(trigger *EmonActionTrigger) bool {
	return trigger.Condition == "" || m.condition(trigger.Condition)
}

func (m *Machine) fireTriggers(sprite *stage.EmonSprite, input EmonInputKind, x, y, keyCode int) bool {
	m.pumpMotions()
	fired := false
	if input == EmonInputLeftClick && sprite != nil {
		if m.fireSpriteTriggers(EmonActionSpriteClick, sprite) {
			fired = true
		}
	}
	if input != EmonInputNone {
		snapshot := append([]*EmonActionTrigger(nil), m.triggers...)
		for _, trigger := range snapshot {
			if !inputMatches(trigger, input, keyCode) {
				continue
			}
			if !m.triggerEnabled(trigger) || !m.conditionHolds(trigger) {
				continue
			}
			m.executeHandlerTarget(trigger.Handler)
			fired = true
		}
	}
	if input != EmonInputLeftClick {
		return fired
	}
	if m.fireClickAreas(x, y) {
		fired = true
	}
	return fired
}

func (m *Machine) fireSpriteTriggers(kind EmonActionKind, sprite *stage.EmonSprite) bool {
	if !m.hasTriggerKind(kind) {
		return false
	}
	fired := m.fireSpriteTriggersOne(kind, sprite)
	// 原引擎的点选沿精灵链向上冒泡：存档槽位的备注文字是 SELECT 子精灵
	// （点击=改备注，SelectNumber+=10），它同时位于整个槽位的 SELECT 父精灵
	// （点击=读/存，SelectNumber=N）内。只触发最上层子精灵会把点击备注变成
	// 改备注甚至无反应；补触发带 SELECT 的父精灵后最终 SelectNumber=N，
	// 与玩家“点整个槽位读/存档”的预期一致。普通按钮没有带触发器的父级。
	for parent := sprite.Parent(); parent != nil; parent = parent.Parent() {
		if parent.Flags()&stage.EmonSpriteFlagsSelect == 0 {
			continue
		}
		if m.fireSpriteTriggersOne(kind, parent) {
			fired = true
		}
	}
	return fired
}

func (m *Machine) fireSpriteTriggersOne(kind EmonActionKind, sprite *stage.EmonSprite) bool {
	fired := false
	snapshot := append([]*EmonActionTrigger(nil), m.triggers...)
	for _, trigger := range snapshot {
		if trigger.Kind != kind {
			continue
		}
		if !strings.EqualFold(trigger.Selector, sprite.Name()) {
			continue
		}
		if !m.triggerEnabled(trigger) || !m.conditionHolds(trigger) {
			continue
		}
		m.executeHandlerTarget(trigger.Handler)
		fired = true
	}
	return fired
}

func (m *Machine) fireClickAreas(x, y int) bool {
	if len(m.areas) == 0 {
		return false
	}
	fired := false
	snapshot := append([]ClickArea(nil), m.areas...)
	for _, area := range snapshot {
		if !area.Contains(x, y) {
			continue
		}
		if target := m.findSprite(area.Sprite); target != nil && !m.isChainVisible(target) {
			continue
		}
		m.executeHandlerTarget(area.Macro)
		fired = true
	}
	return fired
}

func (m *Machine) fireDragTriggers(sprite *stage.EmonSprite, x, y int) bool {
	m.pumpMotions()
	fired := false
	if sprite != nil {
		if m.fireSpriteTriggers(EmonActionSpriteDrag, sprite) {
			fired = true
		}
		if m.fireSpriteTriggers(EmonActionSpriteClick, sprite) {
			fired = true
		}
	}
	if m.fireClickAreas(x, y) {
		fired = true
	}
	return fired
}

func (m *Machine) fireMoveTriggers(sprite *stage.EmonSprite) bool {
	return m.fireSpriteTriggers(EmonActionSpriteMove, sprite)
}

func (m *Machine) hasTriggerKind(kind EmonActionKind) bool {
	for _, trigger := range m.triggers {
		if trigger.Kind == kind {
			return true
		}
	}
	return false
}

func inputMatches(trigger *EmonActionTrigger, input EmonInputKind, keyCode int) bool {
	switch trigger.Kind {
	case EmonActionClick:
		return input == EmonInputLeftClick
	case EmonActionMiddleClick:
		return input == EmonInputMiddleClick
	case EmonActionRightClick:
		return input == EmonInputRightClick
	case EmonActionWheelUp:
		return input == EmonInputWheelUp
	case EmonActionWheelDown:
		return input == EmonInputWheelDown
	case EmonActionKey:
		return input == EmonInputKey && (trigger.KeyCode == 0 || trigger.KeyCode == keyCode)
	}
	return false
}

func (m *Machine) onCursolMoveLock(statement *script.EmonStatement) EmonStep {
	if statement.ArgumentCount() != 0 {
		m.logOnce("cursol move lock arguments are ignored")
	}
	m.cursorMoveLocked = true
	return EmonStepContinue
}

func (m *Machine) firePointerTriggers(sprite *stage.EmonSprite) {
	m.pumpMotions()
	if m.cursorMoveLocked {
		m.cursorMoveLocked = false
		m.hoveredSprite = ""
		m.lastHover = ""
		m.updateMessageSelect(nil)
	} else {
		m.updateMessageSelect(sprite)
		hovered := ""
		if sprite != nil {
			hovered = sprite.Name()
		}
		if !strings.EqualFold(hovered, m.lastHover) {
			left := m.lastHover
			m.lastHover = hovered
			snapshot := append([]*EmonActionTrigger(nil), m.triggers...)
			for _, trigger := range snapshot {
				if trigger.Kind == EmonActionSpriteOut && strings.EqualFold(trigger.Selector, left) {
					m.executeHandlerTarget(trigger.Handler)
				} else if trigger.Kind == EmonActionSpriteOver && strings.EqualFold(trigger.Selector, hovered) {
					m.executeHandlerTarget(trigger.Handler)
				}
			}
		}
	}
	if m.mouseFollow == nil {
		return
	}
	m.applyMouseFollow(m.mouseFollow, false)
}

func (m *Machine) updateMessageSelect(sprite *stage.EmonSprite) {
	value := 0
	if sprite != nil {
		if setting := sprite.TextSetting(); setting != nil && setting.MessageSelectMode() == emontext.EmonSpriteMessageSelectLine {
			lineHeight := max(1, setting.Metrics().LineHeight)
			offset := m.pointerY - (sprite.AbsoluteY() + setting.X())
			index := -1
			if offset >= 0 {
				index = offset / lineHeight
			}
			if index >= 0 && index < m.messageSelectLineCount(sprite, setting) {
				value = messageSelectNumberBase | (index + 1)
			}
		}
	}
	if value == 0 && m.Lookup("SprMsgSelectNumber").AsInt() == 0 {
		return
	}
	m.SetVariable("SprMsgSelectNumber", script.EmonValueFromInt(int32(value)))
}

func (m *Machine) messageSelectLineCount(sprite *stage.EmonSprite, setting *emontext.EmonSpriteTextSetting) int {
	isMessageBox := strings.EqualFold(sprite.Name(), m.messageBoxSprite())
	content := m.messageSpriteText(sprite.Name(), sprite, isMessageBox)
	if content == "" {
		return 0
	}
	wrapped := len(emontext.WrapSpriteText(setting, content, emontext.ResolveGlyphSource()))
	return min(wrapped, max(1, setting.Metrics().MaxLines))
}

func (m *Machine) applyMouseFollow(follow *MouseFollow, store bool) {
	target := m.findSprite(follow.Sprite)
	if target == nil {
		if store {
			m.mouseFollow = nil
		}
		return
	}
	if store {
		m.mouseFollow = follow
	}
	if !store && !m.pointerDown {
		return
	}
	x := m.pointerX + follow.DeltaX(m)
	y := m.pointerY + follow.DeltaY(m)
	// SPREREA 登记的滑杆/旋钮有活动范围：超出时夹回矩形内。
	if area, ok := m.spriteAreas[strings.ToUpper(target.Name())]; ok {
		maxX := area.X2 - target.Width()
		if maxX < area.X1 {
			maxX = area.X1
		}
		maxY := area.Y2 - target.Height()
		if maxY < area.Y1 {
			maxY = area.Y1
		}
		x = core.Clamp(x, area.X1, maxX)
		y = core.Clamp(y, area.Y1, maxY)
	}
	if target.X() == x && target.Y() == y {
		return
	}
	target.SetX(x)
	target.SetY(y)
	m.fireMoveTriggers(target)
}

func isSpriteTrigger(kind EmonActionKind) bool {
	switch kind {
	case EmonActionSpriteClick, EmonActionSpriteOver, EmonActionSpriteOut,
		EmonActionSpriteMove, EmonActionSpriteDrag, EmonActionSpriteOn,
		EmonActionSpriteOff, EmonActionSpriteAnmOn, EmonActionSpriteAnmOff,
		EmonActionEffectOn, EmonActionMotionOn, EmonActionMotionOff:
		return true
	}
	return false
}

func parseTriggerKind(token string) EmonActionKind {
	switch triggerTokenName(token) {
	case "MOUSELEFTCLICK":
		return EmonActionClick
	case "MOUSEMIDDLECLICK":
		return EmonActionMiddleClick
	case "MOUSERIGHTCLICK":
		return EmonActionRightClick
	case "MOUSEUPWHEEL":
		return EmonActionWheelUp
	case "MOUSEDOWNWHEEL":
		return EmonActionWheelDown
	case "NUMTRUE":
		return EmonActionCondition
	case "ACTWAITOFF":
		return EmonActionActWaitOff
	case "SPRCLICK":
		return EmonActionSpriteClick
	case "SPROVER":
		return EmonActionSpriteOver
	case "SPRNOTOVER":
		return EmonActionSpriteOut
	case "SPRDRAG":
		return EmonActionSpriteDrag
	case "SPRMOUSEMOVE", "SPRMOVE":
		return EmonActionSpriteMove
	case "SPRANMON":
		return EmonActionSpriteAnmOn
	case "SPRANMOFF":
		return EmonActionSpriteAnmOff
	case "SPRMTNON":
		return EmonActionMotionOn
	case "SPRMTNOFF":
		return EmonActionMotionOff
	case "SPREFCON":
		return EmonActionEffectOn
	case "SPRON":
		return EmonActionSpriteOn
	case "SPROFF":
		return EmonActionSpriteOff
	case "FLGON":
		return EmonActionFlagOn
	case "FLGOFF":
		return EmonActionFlagOff
	case "SOUNDEND":
		return EmonActionSoundEnd
	}
	if strings.HasPrefix(triggerTokenName(token), "KEYCLICK") {
		return EmonActionKey
	}
	return EmonActionNone
}

func parseTriggerUnnever(token string) bool {
	rest := token
	at := strings.Index(rest, "@")
	if at < 0 {
		return false
	}
	rest = rest[at+1:]
	for {
		at = strings.Index(rest, "@")
		part := rest
		if at >= 0 {
			part = rest[:at]
		}
		if strings.EqualFold(strings.TrimSpace(part), "UNNEVER") {
			return true
		}
		if at < 0 {
			return false
		}
		rest = rest[at+1:]
	}
}

func triggerTokenHasPart(token, name string) bool {
	for _, part := range strings.Split(token, "@") {
		if strings.EqualFold(strings.TrimSpace(part), name) {
			return true
		}
	}
	return false
}

func parseTriggerKeyCode(token string) int {
	rest := token
	at := strings.Index(rest, "@")
	if at < 0 {
		return 0
	}
	rest = rest[at+1:]
	for {
		at = strings.Index(rest, "@")
		part := rest
		if at >= 0 {
			part = rest[:at]
		}
		if trimmed := strings.TrimSpace(part); len(trimmed) >= 3 && strings.EqualFold(trimmed[:3], "KEY") {
			return triggerKeyCode(trimmed[3:])
		}
		if at < 0 {
			return 0
		}
		rest = rest[at+1:]
	}
}

func parseTriggerSoundChannel(token string) int {
	for _, part := range strings.Split(token, "@") {
		value := strings.TrimSpace(part)
		if isASCIIDigits(value) {
			if channel, ok := tryParseInt(value); ok {
				return channel
			}
		}
	}
	return -1
}

func triggerTokenName(token string) string {
	upper := strings.ToUpper(strings.TrimSpace(token))
	if at := strings.Index(upper, "@"); at >= 0 {
		return strings.TrimSpace(upper[:at])
	}
	return upper
}

func triggerKeyCode(name string) int {
	normalized := strings.TrimLeft(name, "_")
	if normalized == "" {
		return 0
	}
	switch normalized {
	case "BACK", "BACKSPACE":
		return 0x08
	case "TAB":
		return 0x09
	case "RETURN", "ENTER":
		return 0x0D
	case "SHIFT":
		return 0x10
	case "CTRL", "CONTROL":
		return 0x11
	case "ALT":
		return 0x12
	case "ESC", "ESCAPE":
		return 0x1B
	case "SPACE":
		return 0x20
	case "PAGEUP":
		return 0x21
	case "PAGEDOWN":
		return 0x22
	case "LEFT":
		return 0x25
	case "UP":
		return 0x26
	case "RIGHT":
		return 0x27
	case "DOWN":
		return 0x28
	case "INSERT":
		return 0x2D
	case "DELETE":
		return 0x2E
	}
	first := unicode.ToUpper(rune(normalized[0]))
	if first >= 'A' && first <= 'Z' {
		return int(first)
	}
	return 0
}

func (m *Machine) removeTriggersWhere(match func(*EmonActionTrigger) bool) {
	kept := m.triggers[:0]
	for _, trigger := range m.triggers {
		if !match(trigger) {
			kept = append(kept, trigger)
		}
	}
	for i := len(kept); i < len(m.triggers); i++ {
		m.triggers[i] = nil
	}
	m.triggers = kept
}

func (m *Machine) removeTrigger(target *EmonActionTrigger) {
	for i, trigger := range m.triggers {
		if trigger == target {
			m.triggers = append(m.triggers[:i], m.triggers[i+1:]...)
			return
		}
	}
}

func (m *Machine) captureThumbnail(slot int, spriteName string, x1, y1, x2, y2 int) {
	canvas := m.context.Composer.Current()
	left := core.Clamp(min(x1, x2), 0, max(0, canvas.Width-1))
	top := core.Clamp(min(y1, y2), 0, max(0, canvas.Height-1))
	width := min(core.Abs(x2-x1), canvas.Width-left)
	height := min(core.Abs(y2-y1), canvas.Height-top)
	if width <= 0 || height <= 0 {
		return
	}
	cropped := cropCanvas(canvas, left, top, width, height)
	sprite := m.context.Stage.GetOrCreate(spriteName, nil)
	sprite.SetImage(cropped)
	sprite.ClearSourceRect()
	sprite.SetSize(cropped.Width, cropped.Height)
	m.StoreSource(spriteName, cropped)
	m.scratchThumbs[slot] = cropped
}

func (m *Machine) drawThumbnail(slot int, spriteName string, x, y, width, height int) {
	sprite := m.context.Stage.GetOrCreate(spriteName, nil)
	sprite.SetX(x)
	sprite.SetY(y)
	thumb, ok := m.scratchThumbs[slot]
	if !ok {
		if width > 0 && height > 0 {
			sprite.SetSize(width, height)
		}
		return
	}
	if width > 0 && height > 0 && (width != thumb.Width || height != thumb.Height) {
		thumb = scaleImage(thumb, width, height)
	}
	sprite.SetImage(thumb)
	sprite.ClearSourceRect()
	sprite.SetSize(thumb.Width, thumb.Height)
	m.StoreSource(spriteName, thumb)
}
