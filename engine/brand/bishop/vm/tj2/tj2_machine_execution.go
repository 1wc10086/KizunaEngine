package tj2

import (
	"fmt"
	"math"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

func (m *Tj2Machine) Fetch() Tj2Command {
	if m.halted {
		if m.faulted {
			return Tj2Command{Kind: Tj2CommandKindFaulted, Text: m.faultText}
		}
		return Tj2Command{Kind: Tj2CommandKindHalt}
	}
	for step := 0; step < fetchStepLimit; step++ {
		m.instruction = m.pc
		if m.pc < 0 || m.pc >= m.codeLength {
			m.halted = true
			return Tj2Command{Kind: Tj2CommandKindHalt}
		}
		prefix := m.code[m.pc]
		m.pc++
		var command *Tj2Command
		switch prefix {
		case OpBang:
			command = m.executeBang()
		case OpHash:
			command = m.executeHash()
		case OpLabel:
			command = m.executeEntries()
		case OpWrite:
			command = m.executeWrite()
		default:
			command = nil
		}
		if command != nil {
			return *command
		}
		if m.parenArmed {
			m.parenArmed = false
			m.skip--
		}
		if prefix == OpLatch {
			tag, ok := m.tryReadU16()
			if !ok {
				return m.fail("tj2 ( truncated")
			}
			if m.latch != tag {
				m.skip++
				m.parenArmed = true
			}
		}
	}
	return m.fail("tj2 step limit")
}

func (m *Tj2Machine) fail(message string) Tj2Command {
	core.Logf("tj2: %s", message)
	m.faulted = true
	m.halted = true
	m.faultText = message
	return Tj2Command{Kind: Tj2CommandKindFaulted, Text: message}
}

func (m *Tj2Machine) executeBang() *Tj2Command {
	letter, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 ! truncated")
		return &cmd
	}
	switch letter {
	case 'a':
		return m.bangOverlay()
	case 'b':
		return m.bangBgm()
	case 'c':
		return m.bangClear()
	case 'e':
		return m.bangSound(false)
	case 'g':
		return m.bangBackground()
	case 'i':
		return m.bangLatchClear()
	case 'j':
		return m.bangJump()
	case 'm':
		return m.bangLatchSet()
	case 'n':
		return m.bangName()
	case 'p':
		return m.bangFunction()
	case 'r':
		return m.bangLoadDialog()
	case 's':
		return m.bangMessage()
	case 't':
		return m.bangChapter()
	case 'v':
		return m.bangSound(true)
	case 'w':
		return m.bangSaveDialog()
	case 'x':
		return m.bangStandee()
	default:
		cmd := m.fail(fmt.Sprintf("tj2 unknown !%c at %d", letter, m.instruction))
		return &cmd
	}
}

func (m *Tj2Machine) executeHash() *Tj2Command {
	letter, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 # truncated")
		return &cmd
	}
	switch letter {
	case 'W':
		return m.hashWipe()
	case 'b':
		return m.hashBgmModifier()
	case 'e':
		return m.hashJumpMark()
	case 'g':
		return m.hashCall()
	case 'i':
		return m.hashIf()
	case 'l':
		return m.hashToggle()
	case 'n':
		return m.hashEndIf()
	case 'r':
		return m.hashReturn()
	case 's':
		return m.hashLock()
	case 't':
		return m.hashTimer()
	case 'w':
		return m.hashMark()
	case 'x':
		return m.hashOffset()
	default:
		cmd := m.fail(fmt.Sprintf("tj2 unknown #%c at %d", letter, m.instruction))
		return &cmd
	}
}

func (m *Tj2Machine) bangOverlay() *Tj2Command {
	if !m.trySkip(4) {
		cmd := m.fail("tj2 !a truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	return &Tj2Command{Kind: Tj2CommandKindNone}
}

func (m *Tj2Machine) bangBgm() *Tj2Command {
	index, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !b truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if index < 0 || index >= len(m.table.TrackNumbers()) {
		cmd := m.fail(fmt.Sprintf("tj2 !b index %d", index))
		return &cmd
	}
	return &Tj2Command{Kind: Tj2CommandKindPlayBgm, Index: index}
}

func (m *Tj2Machine) bangClear() *Tj2Command {
	if m.blocked() {
		return nil
	}
	clear(m.flags)
	clear(m.source)
	m.applyFlagAliases()
	return &Tj2Command{Kind: Tj2CommandKindClearFlags}
}

func (m *Tj2Machine) bangSound(voice bool) *Tj2Command {
	index, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail(m.bangSoundTruncated(voice))
		return &cmd
	}
	if _, ok := m.tryReadByte(); !ok {
		cmd := m.fail(m.bangSoundTruncated(voice))
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if index < 0 || index >= len(m.table.SoundNames()) {
		letter := 'e'
		if voice {
			letter = 'v'
		}
		cmd := m.fail(fmt.Sprintf("tj2 !%c index %d", letter, index))
		return &cmd
	}
	kind := Tj2CommandKindPlaySound
	if voice {
		kind = Tj2CommandKindPlayVoice
	}
	return &Tj2Command{Kind: kind, Index: index, Text: m.table.SoundName(index) + ".ADP"}
}

func (m *Tj2Machine) bangSoundTruncated(voice bool) string {
	if voice {
		return "tj2 !v truncated"
	}
	return "tj2 !e truncated"
}

func (m *Tj2Machine) bangBackground() *Tj2Command {
	index, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !g truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if index < 0 || index >= len(m.table.GraphicNames()) {
		cmd := m.fail(fmt.Sprintf("tj2 !g index %d", index))
		return &cmd
	}
	offsetX := m.pendingOffsetX
	offsetY := m.pendingOffsetY
	m.pendingOffsetX = 0
	m.pendingOffsetY = 0
	return &Tj2Command{
		Kind:    Tj2CommandKindBackground,
		Index:   index,
		Text:    strings.ToUpper(m.table.GraphicName(index) + ".KG"),
		OffsetX: offsetX,
		OffsetY: offsetY,
	}
}

func (m *Tj2Machine) bangLatchClear() *Tj2Command {
	if _, _, ok := m.tryReadCString(); !ok {
		cmd := m.fail("tj2 !i truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	return &Tj2Command{Kind: Tj2CommandKindNone}
}

func (m *Tj2Machine) bangJump() *Tj2Command {
	scenario, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !j truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if !m.loadScenarioInternal(scenario) {
		return &Tj2Command{Kind: Tj2CommandKindFaulted, Text: m.faultText}
	}
	return &Tj2Command{Kind: Tj2CommandKindNone}
}

func (m *Tj2Machine) bangLatchSet() *Tj2Command {
	if _, _, ok := m.tryReadCString(); !ok {
		cmd := m.fail("tj2 !m truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	m.latch = 1
	return &Tj2Command{Kind: Tj2CommandKindNone}
}

func (m *Tj2Machine) bangName() *Tj2Command {
	slot, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !n truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if slot < 0 || slot >= len(m.table.Macros()) || slot >= len(m.names) {
		cmd := m.fail(fmt.Sprintf("tj2 !n slot %d", slot))
		return &cmd
	}
	m.names[slot] = m.table.Macros()[slot].To
	return &Tj2Command{Kind: Tj2CommandKindName, Index: slot, Text: m.table.Macros()[slot].To}
}

func (m *Tj2Machine) bangFunction() *Tj2Command {
	slot, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !p truncated")
		return &cmd
	}
	start, length, ok := m.tryReadCString()
	if !ok {
		cmd := m.fail("tj2 !p truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if slot < 0 || slot >= len(m.table.FunctionSlots()) {
		cmd := m.fail(fmt.Sprintf("tj2 !p slot %d", slot))
		return &cmd
	}
	parsed, _ := strutil.TolerantAtoi(m.table.FunctionSlots()[slot].Text)
	return &Tj2Command{
		Kind:  Tj2CommandKindFunction,
		Value: parsed,
		Text:  decodeBytes(m.code, start, length),
	}
}

func (m *Tj2Machine) bangLoadDialog() *Tj2Command {
	if m.blocked() {
		return nil
	}
	// 剧本在 !r 处必须停住等玩家选档。以前返回 None 会让脚本一路跑过
	// "继续游戏"分支，标题菜单点"続きから"等于没反应。
	return &Tj2Command{Kind: Tj2CommandKindLoadDialog}
}

func (m *Tj2Machine) bangSaveDialog() *Tj2Command {
	if m.blocked() {
		return nil
	}
	return &Tj2Command{Kind: Tj2CommandKindSaveDialog}
}

func (m *Tj2Machine) bangChapter() *Tj2Command {
	start, length, ok := m.tryReadCString()
	if !ok {
		cmd := m.fail("tj2 !t truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	text := decodeBytes(m.code, start, length)
	core.Logf("tj2: chapter %s", text)
	return &Tj2Command{Kind: Tj2CommandKindChapter, Text: text}
}

func (m *Tj2Machine) bangStandee() *Tj2Command {
	index, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 !x truncated")
		return &cmd
	}
	layer, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 !x truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	if index < 0 || index >= len(m.table.GraphicNames()) {
		cmd := m.fail(fmt.Sprintf("tj2 !x index %d", index))
		return &cmd
	}
	offsetX := m.pendingOffsetX
	offsetY := m.pendingOffsetY
	m.pendingOffsetX = 0
	m.pendingOffsetY = 0
	return &Tj2Command{
		Kind:    Tj2CommandKindStandee,
		Index:   index,
		Flag:    int(layer),
		Text:    strings.ToUpper(m.table.GraphicName(index) + ".KG"),
		OffsetX: offsetX,
		OffsetY: offsetY,
	}
}

func (m *Tj2Machine) bangMessage() *Tj2Command {
	if m.pc+3 > m.codeLength {
		cmd := m.fail("tj2 !s truncated")
		return &cmd
	}
	number := int(m.code[m.pc]) | int(m.code[m.pc+1])<<8 | int(m.code[m.pc+2])<<16
	m.pc += 3
	start, length, ok := m.tryReadCString()
	if !ok {
		cmd := m.fail("tj2 !s truncated")
		return &cmd
	}
	if m.blocked() {
		return nil
	}
	m.messageOffset = number
	text := m.expandMacros(decodeBytes(m.code, start, length))
	return &Tj2Command{Kind: Tj2CommandKindMessage, Text: text}
}

func (m *Tj2Machine) hashWipe() *Tj2Command {
	number, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #W truncated")
		return &cmd
	}
	if m.skip != 0 {
		return nil
	}
	return &Tj2Command{Kind: Tj2CommandKindWipe, Value: int(number)}
}

func (m *Tj2Machine) hashBgmModifier() *Tj2Command {
	second, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #b truncated")
		return &cmd
	}
	if second == 'd' {
		if m.skip != 0 && !m.trySkip(1) {
			cmd := m.fail("tj2 #bd truncated")
			return &cmd
		}
		return nil
	}
	if _, ok := m.tryReadByte(); !ok {
		cmd := m.fail("tj2 #b truncated")
		return &cmd
	}
	return nil
}

func (m *Tj2Machine) hashJumpMark() *Tj2Command {
	if m.loopHead == -1 {
		m.loopHead = 0
		m.skip--
	} else if m.skip == 0 {
		m.pc = m.loopHead
	}
	return nil
}

func (m *Tj2Machine) hashCall() *Tj2Command {
	target, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 #g truncated")
		return &cmd
	}
	if m.skip != 0 {
		return nil
	}
	if len(m.frames) >= MaxCallDepth {
		cmd := m.fail("tj2 #g depth")
		return &cmd
	}
	condStack := make([]int, len(m.condStack))
	copy(condStack, m.condStack[:])
	source := make([]byte, len(m.source))
	copy(source, m.source)
	m.frames = append(m.frames, tj2CallFrame{
		scenario:      m.scenario,
		pc:            m.pc,
		condDepth:     m.condDepth,
		condStack:     condStack,
		latch:         m.latch,
		messageOffset: m.messageOffset,
		source:        source,
	})
	if !m.loadScenarioInternal(target) {
		return &Tj2Command{Kind: Tj2CommandKindFaulted, Text: m.faultText}
	}
	return nil
}

func (m *Tj2Machine) hashIf() *Tj2Command {
	value, used := EvaluateExpression(m.code, m.pc, m.flags, ptrDialect(m.table.Dialect()))
	if used < 0 {
		cmd := m.fail("tj2 #i expression")
		return &cmd
	}
	m.pc += used
	if m.condDepth < 0 || m.condDepth >= condCapacity {
		cmd := m.fail("tj2 #i depth")
		return &cmd
	}
	if value {
		m.condStack[m.condDepth] = 17
	} else {
		m.condStack[m.condDepth] = 0
	}
	if !value {
		m.skip++
	}
	m.condDepth++
	return nil
}

func (m *Tj2Machine) hashEndIf() *Tj2Command {
	m.condDepth--
	if m.condDepth < 0 {
		m.condDepth = 0
		cmd := m.fail("tj2 #n without #i")
		return &cmd
	}
	if (m.condStack[m.condDepth] & 1) == 0 {
		m.skip--
	}
	return nil
}

func (m *Tj2Machine) hashToggle() *Tj2Command {
	if m.condDepth < 0 || m.condDepth >= condCapacity {
		cmd := m.fail("tj2 #l depth")
		return &cmd
	}
	if m.condDepth == 0 {
		if m.pc < m.codeLength && m.code[m.pc] == 'i' {
			m.pc++
			_, used := EvaluateExpression(m.code, m.pc, m.flags, ptrDialect(m.table.Dialect()))
			if used < 0 {
				cmd := m.fail("tj2 #li expression")
				return &cmd
			}
			m.pc += used
		}
		return nil
	}
	slot := m.condStack[m.condDepth-1]
	if m.pc < m.codeLength && m.code[m.pc] == 'i' {
		m.pc++
		value, used := EvaluateExpression(m.code, m.pc, m.flags, ptrDialect(m.table.Dialect()))
		if used < 0 {
			cmd := m.fail("tj2 #li expression")
			return &cmd
		}
		m.pc += used
		if slot != 0 {
			if slot == 17 {
				m.condStack[m.condDepth-1] = 16
				m.skip++
			}
		} else if value {
			m.skip--
			m.condStack[m.condDepth-1] = 17
		}
	} else if slot != 0 {
		if slot == 17 {
			m.skip++
			m.condStack[m.condDepth-1] = 16
		}
	} else {
		m.condStack[m.condDepth-1] = 17
		m.skip--
	}
	return nil
}

func (m *Tj2Machine) hashReturn() *Tj2Command {
	reset := m.pc+4 <= m.codeLength &&
		m.code[m.pc] == 'e' &&
		m.code[m.pc+1] == 's' &&
		m.code[m.pc+2] == 'e' &&
		m.code[m.pc+3] == 't'
	if reset {
		m.pc += 4
	}
	if m.skip != 0 {
		return nil
	}
	if reset {
		m.messageOffset = -1
		return &Tj2Command{Kind: Tj2CommandKindNone}
	}
	if len(m.frames) == 0 {
		core.Logf("tj2: tj2 #r empty")
		m.halted = true
		m.faulted = false
		m.faultText = ""
		return &Tj2Command{Kind: Tj2CommandKindHalt}
	}
	frame := m.frames[len(m.frames)-1]
	m.frames = m.frames[:len(m.frames)-1]
	if !m.loadScenarioInternal(frame.scenario) {
		return &Tj2Command{Kind: Tj2CommandKindFaulted, Text: m.faultText}
	}
	if frame.pc < 0 || frame.pc > m.codeLength {
		cmd := m.fail("tj2 #r address")
		return &cmd
	}
	m.pc = frame.pc
	m.instruction = m.pc
	m.condDepth = core.Clamp(frame.condDepth, 0, condCapacity)
	copy(m.condStack[:], frame.condStack)
	m.latch = frame.latch
	m.messageOffset = frame.messageOffset
	clear(m.source)
	copy(m.source, frame.source)
	return nil
}

func (m *Tj2Machine) hashLock() *Tj2Command {
	second, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #s truncated")
		return &cmd
	}
	if second == 'e' {
		m.locked = 0
	} else if second == 'f' {
		m.locked = 1
	}
	return nil
}

func (m *Tj2Machine) hashTimer() *Tj2Command {
	second, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #t truncated")
		return &cmd
	}
	if second == 'r' {
		m.waitBaseMs = tickMilliseconds()
		return nil
	}
	if second == 'w' {
		count, ok := m.tryReadU16()
		if !ok {
			cmd := m.fail("tj2 #tw truncated")
			return &cmd
		}
		if m.skip != 0 || m.locked != 0 {
			return nil
		}
		m.pendingWaitMs = int(max(int64(0), m.waitBaseMs+100*int64(count)-tickMilliseconds()))
		return &Tj2Command{Kind: Tj2CommandKindWait, Value: m.pendingWaitMs}
	}
	if second != 't' {
		return nil
	}
	ticks, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #tt truncated")
		return &cmd
	}
	if m.skip != 0 || m.locked != 0 {
		return nil
	}
	m.pendingWaitMs = 10 * int(ticks)
	return &Tj2Command{Kind: Tj2CommandKindWait, Value: m.pendingWaitMs}
}

func (m *Tj2Machine) hashMark() *Tj2Command {
	brk := m.pc < m.codeLength && m.code[m.pc] == 'b'
	if brk {
		m.pc++
	}
	if m.skip != 0 {
		return nil
	}
	if brk {
		m.loopHead = -1
		m.skip = 1
	} else {
		m.loopHead = m.pc
	}
	return nil
}

func (m *Tj2Machine) hashOffset() *Tj2Command {
	second, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 #xl truncated")
		return &cmd
	}
	if second != 'l' {
		return nil
	}
	if m.pc+2 > m.codeLength {
		cmd := m.fail("tj2 #xl truncated")
		return &cmd
	}
	dx := int(int8(m.code[m.pc]))
	dy := int(int8(m.code[m.pc+1]))
	m.pc += 2
	if m.locked == 0 {
		m.pendingOffsetX = dx
		m.pendingOffsetY = dy
	}
	return nil
}

func (m *Tj2Machine) executeWrite() *Tj2Command {
	index, ok := m.tryReadU16()
	if !ok {
		cmd := m.fail("tj2 [ truncated")
		return &cmd
	}
	if index < 0 || index >= m.table.FlagCount() {
		cmd := m.fail(fmt.Sprintf("tj2 [ index %d", index))
		return &cmd
	}
	if _, ok := m.tryReadByte(); !ok {
		cmd := m.fail("tj2 [ truncated")
		return &cmd
	}
	op, ok := m.tryReadByte()
	if !ok {
		cmd := m.fail("tj2 [ truncated")
		return &cmd
	}
	var accumulator int32
	var values []int32
	var operators []byte
	var priorities []int
	if op == '=' {
		accumulator = 0
	} else {
		accumulator = m.readFlag(index)
		if op == 0 {
			return m.settleWrite(index, accumulator, values, operators, priorities, 0)
		}
	}
	current := 0
	ceiling := 0
	for {
		priority := current
		if op != '=' && op != '+' && op != '-' {
			priority = current + 1
		}
		for m.pc < m.codeLength && m.code[m.pc] == '(' {
			current += 2
			if current > ceiling {
				ceiling = current
			}
			m.pc++
		}
		if m.pc < 0 || m.pc >= m.codeLength {
			cmd := m.fail("tj2 [ truncated")
			return &cmd
		}
		head := m.code[m.pc]
		m.pc++
		var value int32
		if head == 'F' {
			slot, ok := m.tryReadU16()
			if !ok {
				cmd := m.fail("tj2 [ truncated")
				return &cmd
			}
			if slot < 0 || slot >= m.table.FlagCount() {
				cmd := m.fail(fmt.Sprintf("tj2 [ slot %d", slot))
				return &cmd
			}
			value = m.readFlag(slot)
		} else {
			width := m.table.Dialect().LiteralWidth()
			if m.pc+width > m.codeLength {
				cmd := m.fail("tj2 [ truncated")
				return &cmd
			}
			value = 0
			for i := 0; i < width; i++ {
				value = (value << 8) | int32(m.code[m.pc])
				m.pc++
			}
		}
		values = append(values, value)
		operators = append(operators, op)
		priorities = append(priorities, priority)
		op, ok = m.tryReadByte()
		if !ok {
			cmd := m.fail("tj2 [ truncated")
			return &cmd
		}
		for m.pc < m.codeLength && m.code[m.pc] == ')' {
			current -= 2
			m.pc++
		}
		if op == 0 {
			break
		}
	}
	return m.settleWrite(index, accumulator, values, operators, priorities, ceiling)
}

func (m *Tj2Machine) settleWrite(index int, accumulator int32, values []int32, operators []byte, priorities []int, ceiling int) *Tj2Command {
	if m.skip == 0 {
		for level := ceiling + 1; level >= 0; level-- {
			target := -1
			for term := 0; term < len(values); term++ {
				if priorities[term] == level {
					var folded int32
					if target < 0 {
						folded = foldWrite(accumulator, operators[term], values[term])
					} else {
						folded = foldWrite(values[target], operators[term], values[term])
					}
					if target < 0 {
						accumulator = folded
					} else {
						values[target] = folded
					}
					operators[term] = 0
					values[term] = 0
				} else if priorities[term] < level {
					target = term
				}
			}
		}
		if accumulator < 0 {
			accumulator = 0
		} else if accumulator > int32(m.table.Dialect().FlagMaximum) {
			accumulator = int32(m.table.Dialect().FlagMaximum)
		}
		m.writeFlag(index, accumulator)
		for _, alias := range m.table.FlagAliases() {
			if alias.Source == index {
				m.writeFlag(alias.Target, accumulator)
			}
		}
	}
	return nil
}

func foldWrite(dest int32, op byte, value int32) int32 {
	switch op {
	case '%':
		if value == 0 {
			return dest
		}
		return dest % value
	case '&':
		return dest & value
	case '*':
		return dest * value
	case '+':
		return dest + value
	case '-':
		return dest - value
	case '/':
		if value == 0 || (dest == math.MinInt32 && value == -1) {
			return dest
		}
		return dest / value
	case '=':
		return value
	case '^':
		return dest ^ value
	case '|':
		return dest | value
	default:
		return dest
	}
}
