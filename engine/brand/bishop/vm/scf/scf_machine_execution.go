package scf

import (
	"fmt"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

func (m *ScfMachine) Fetch() ScfCommand {
	if m.halted {
		if m.faulted {
			return ScfCommandFault(m.faultText)
		}
		return ScfCommandHalt
	}
	if len(m.pending) > 0 {
		command := m.pending[0]
		m.pending = m.pending[1:]
		return command
	}
	for step := 0; step < scfFetchStepLimit; step++ {
		m.messageOffset = -1
		m.instructionStart = m.pc
		if uint(m.pc) >= uint(m.codeLength) {
			m.halted = true
			return ScfCommandHalt
		}
		prefix := m.code[m.pc]
		m.pc++
		var command *ScfCommand
		switch prefix {
		case ScfOpcodeBang:
			command = m.executeBang()
		case ScfOpcodeHash:
			command = m.executeHash()
		case ScfOpcodeLabel:
			command = m.executeEntries()
		case ScfOpcodeWrite:
			command = m.executeWrite()
		}
		if command != nil {
			return *command
		}
		if m.parenArmed {
			m.parenArmed = false
			m.skip--
		}
		if prefix == ScfOpcodeLatch {
			tag, ok := m.tryReadU16()
			if !ok {
				return m.fail("scf ( truncated")
			}
			if m.latch != tag {
				m.skip++
				m.parenArmed = true
			}
		}
	}
	return m.fail("scf step limit")
}

func (m *ScfMachine) fail(message string) ScfCommand {
	core.Logf("scf: %s", message)
	m.faulted = true
	m.halted = true
	m.faultText = message
	return ScfCommandFault(message)
}

func (m *ScfMachine) faultResult() *ScfCommand {
	command := ScfCommandFault(m.faultText)
	return &command
}

func (m *ScfMachine) tryReadByte() (byte, bool) {
	if uint(m.pc) >= uint(m.codeLength) {
		return 0, false
	}
	value := m.code[m.pc]
	m.pc++
	return value, true
}

func (m *ScfMachine) tryReadU16() (int, bool) {
	if m.pc+2 > m.codeLength {
		return 0, false
	}
	value := int(m.code[m.pc])<<8 | int(m.code[m.pc+1])
	m.pc += 2
	return value, true
}

func (m *ScfMachine) trySkip(length int) bool {
	if m.pc+length > m.codeLength {
		return false
	}
	m.pc += length
	return true
}

func (m *ScfMachine) tryReadCString() (int, int, bool) {
	start := m.pc
	for m.pc < m.codeLength && m.code[m.pc] != 0 {
		m.pc++
	}
	if uint(m.pc) >= uint(m.codeLength) {
		return 0, 0, false
	}
	length := m.pc - start
	m.pc++
	return start, length, true
}

func (m *ScfMachine) executeBang() *ScfCommand {
	letter, ok := m.tryReadByte()
	if !ok {
		command := m.fail("scf ! truncated")
		return &command
	}
	switch letter {
	case 'a':
		return m.bangRefresh()
	case 'b':
		return m.bangBgm()
	case 'c':
		return m.bangClear()
	case 'e':
		return m.bangSound()
	case 'g':
		return m.bangBackground()
	case 'i':
		return m.bangLatchClear()
	case 'j':
		return m.bangJump()
	case 'm':
		return m.bangLatchSet()
	case 'n':
		return m.bangSlotClear()
	case 'p':
		return m.bangFunction()
	case 'r':
		return m.bangLoadDialog()
	case 's':
		return m.bangMessage()
	case 't':
		return m.bangTrace()
	case 'w':
		return m.bangSaveDialog()
	case 'x':
		return m.bangStandee()
	default:
		command := m.fail(fmt.Sprintf("scf unknown !%c at %d", letter, m.instructionStart))
		return &command
	}
}

func (m *ScfMachine) executeHash() *ScfCommand {
	letter, ok := m.tryReadByte()
	if !ok {
		command := m.fail("scf # truncated")
		return &command
	}
	switch letter {
	case 'g':
		return m.hashCall()
	case 'r':
		return m.hashReturn()
	case 'i':
		return m.hashIf()
	case 'n':
		return m.hashEndIf()
	case 'l':
		return m.hashToggle()
	case 'w':
		return m.hashMark()
	case 'e':
		return m.hashJumpMark()
	default:
		command := m.fail(fmt.Sprintf("scf unknown #%c at %d", letter, m.instructionStart))
		return &command
	}
}

func (m *ScfMachine) bangRefresh() *ScfCommand {
	if !m.trySkip(4) {
		command := m.fail("scf !a truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	command := ScfCommand{Kind: ScfCommandKindRefresh}
	return &command
}

func (m *ScfMachine) bangBgm() *ScfCommand {
	index, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !b truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	tracks := m.table.TrackNumbers()
	if uint(index) >= uint(len(tracks)) {
		command := m.fail(fmt.Sprintf("scf !b index %d", index))
		return &command
	}
	name := tracks[index]
	track, _ := strutil.TolerantAtoi(name)
	if len(name) == 0 || track <= 0 {
		track = -1
	}
	command := ScfCommand{Kind: ScfCommandKindPlayBgm, Value: track}
	return &command
}

func (m *ScfMachine) bangClear() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	clear(m.flags)
	clear(m.source)
	command := ScfCommand{Kind: ScfCommandKindClear}
	return &command
}

func (m *ScfMachine) bangSound() *ScfCommand {
	index, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !e truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	names := m.table.SoundNames()
	if uint(index) >= uint(len(names)) {
		command := m.fail(fmt.Sprintf("scf !e index %d", index))
		return &command
	}
	command := ScfCommand{Kind: ScfCommandKindPlaySound, Index: index, Text: names[index]}
	return &command
}

func (m *ScfMachine) bangBackground() *ScfCommand {
	index, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !g truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	names := m.table.GraphicNames()
	if uint(index) >= uint(len(names)) {
		command := m.fail(fmt.Sprintf("scf !g index %d", index))
		return &command
	}
	command := ScfCommand{Kind: ScfCommandKindBackground, Index: index, Text: strings.ToUpper(names[index] + ".KG")}
	return &command
}

func (m *ScfMachine) bangLatchClear() *ScfCommand {
	if _, _, ok := m.tryReadCString(); !ok {
		command := m.fail("scf !i truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	command := ScfCommand{Kind: ScfCommandKindNone}
	return &command
}

func (m *ScfMachine) bangJump() *ScfCommand {
	scenario, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !j truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	if !m.loadScenarioInternal(scenario) {
		return m.faultResult()
	}
	command := ScfCommand{Kind: ScfCommandKindLoadScenario, Value: scenario}
	return &command
}

func (m *ScfMachine) bangLatchSet() *ScfCommand {
	if _, _, ok := m.tryReadCString(); !ok {
		command := m.fail("scf !m truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	m.latch = 1
	command := ScfCommand{Kind: ScfCommandKindNone}
	return &command
}

func (m *ScfMachine) bangSlotClear() *ScfCommand {
	slot, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !n truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	if uint(slot) >= uint(len(m.names)) {
		command := m.fail(fmt.Sprintf("scf !n slot %d", slot))
		return &command
	}
	m.names[slot] = ""
	command := ScfCommand{Kind: ScfCommandKindNone}
	return &command
}

func (m *ScfMachine) bangFunction() *ScfCommand {
	slot, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !p truncated")
		return &command
	}
	start, length, ok := m.tryReadCString()
	if !ok {
		command := m.fail("scf !p truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	slots := m.table.FunctionSlots()
	if uint(slot) >= uint(len(slots)) {
		command := m.fail(fmt.Sprintf("scf !p slot %d", slot))
		return &command
	}
	command := ScfCommand{
		Kind:  ScfCommandKindFunction,
		Index: slot,
		Value: slot,
		Text:  decodeCodeText(m.code, start, length),
	}
	return &command
}

func (m *ScfMachine) bangLoadDialog() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	command := ScfCommand{Kind: ScfCommandKindLoadGameDialog}
	return &command
}

func (m *ScfMachine) bangSaveDialog() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	command := ScfCommand{Kind: ScfCommandKindSaveGameDialog}
	return &command
}

func (m *ScfMachine) bangTrace() *ScfCommand {
	if _, _, ok := m.tryReadCString(); !ok {
		command := m.fail("scf !t truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	command := ScfCommand{Kind: ScfCommandKindNone}
	return &command
}

func (m *ScfMachine) bangStandee() *ScfCommand {
	index, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf !x truncated")
		return &command
	}
	show, ok := m.tryReadByte()
	if !ok {
		command := m.fail("scf !x truncated")
		return &command
	}
	if m.skip != 0 {
		return nil
	}
	names := m.table.GraphicNames()
	if uint(index) >= uint(len(names)) {
		command := m.fail(fmt.Sprintf("scf !x index %d", index))
		return &command
	}
	flag := 0
	if show == 1 {
		flag = 1
	}
	command := ScfCommand{
		Kind:  ScfCommandKindStandee,
		Index: index,
		Flag:  flag,
		Text:  strings.ToUpper(names[index] + ".KG"),
	}
	return &command
}

func (m *ScfMachine) bangMessage() *ScfCommand {
	start, length, ok := m.tryReadCString()
	if !ok {
		command := m.fail("scf !s truncated")
		return &command
	}
	m.messageOffset = m.instructionStart
	if m.skip != 0 {
		command := ScfCommand{Kind: ScfCommandKindRefresh}
		return &command
	}
	if m.pageBreaks {
		m.decodePagedMessage(start, length)
	} else {
		m.decodeMessage(start, length)
	}
	if len(m.pending) == 0 {
		return nil
	}
	command := m.pending[0]
	m.pending = m.pending[1:]
	return &command
}

func (m *ScfMachine) hashCall() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	target, ok := m.tryReadU16()
	if !ok {
		command := m.fail("scf #g truncated")
		return &command
	}
	if len(m.frames) >= ScfMaxCallDepth {
		command := m.fail("scf #g depth")
		return &command
	}
	m.frames = append(m.frames, m.captureFrame(m.scenario, m.pc))
	if !m.loadScenarioInternal(target) {
		return m.faultResult()
	}
	return nil
}

func (m *ScfMachine) hashReturn() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	if len(m.frames) == 0 {
		command := m.fail("scf #r empty")
		return &command
	}
	frame := m.frames[len(m.frames)-1]
	m.frames = m.frames[:len(m.frames)-1]
	if !m.loadScenarioInternal(frame.scenario) {
		return m.faultResult()
	}
	if frame.pc < 0 || frame.pc > m.codeLength {
		command := m.fail("scf #r address")
		return &command
	}
	m.pc = frame.pc
	m.instructionStart = m.pc
	m.condDepth = core.Clamp(frame.condDepth, 0, ScfCondCapacity-1)
	copy(m.condStack[:], frame.condStack[:])
	copy(m.loopSlots[:], frame.loopSlots[:])
	copy(m.elseSlots[:], frame.elseSlots[:])
	m.latch = frame.latch
	m.messageOffset = frame.messageOffset
	clear(m.source)
	copy(m.source, frame.source)
	return nil
}

func (m *ScfMachine) hashIf() *ScfCommand {
	value, used := EvaluateExpression(m.code, m.pc, m.flags)
	if used < 0 {
		command := m.fail("scf #i expression")
		return &command
	}
	m.pc += used
	if uint(m.condDepth) >= uint(ScfCondCapacity) {
		command := m.fail("scf #i depth")
		return &command
	}
	if value {
		m.condStack[m.condDepth] = 1
	} else {
		m.condStack[m.condDepth] = 0
		m.skip++
	}
	m.condDepth++
	return nil
}

func (m *ScfMachine) hashEndIf() *ScfCommand {
	m.condDepth--
	if m.condDepth < 0 {
		m.condDepth = 0
		command := m.fail("scf #n without #i")
		return &command
	}
	depth := m.condDepth
	if m.elseSlots[depth] != 0 {
		// 有 else：条件为真时 #l 把 else 块 skip 了，这里补回；
		// 条件为假时 #l 已经把 then 的 skip 抵消，这里不能再减。
		if m.condStack[depth] == 1 {
			m.skip--
		}
		m.elseSlots[depth] = 0
	} else if m.condStack[depth] == 0 {
		m.skip--
	}
	m.condStack[depth] = 0
	return nil
}

func (m *ScfMachine) hashToggle() *ScfCommand {
	// SCN002 的 #i flag2==0 / #l / #n 证明 #l 是 else，而不是循环开关。
	// #i 条件为假时 then 块被 skip（skip=1），#l 要把 skip 减回去让 else 块执行；
	// 条件为真时 then 已执行，#l 要 skip else 块。真正结束由 #n 收尾。
	depth := m.condDepth - 1
	if depth < 0 || depth >= ScfCondCapacity {
		command := m.fail("scf #l depth")
		return &command
	}
	m.elseSlots[depth] = 1
	if m.condStack[depth] == 0 {
		m.skip--
	} else {
		m.skip++
	}
	return nil
}

func (m *ScfMachine) hashMark() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	m.loopSlots[0] = m.pc
	return nil
}

func (m *ScfMachine) hashJumpMark() *ScfCommand {
	if m.skip != 0 {
		return nil
	}
	if m.loopSlots[0] < 0 || m.loopSlots[0] > m.codeLength {
		command := m.fail("scf #e address")
		return &command
	}
	m.pc = m.loopSlots[0]
	return nil
}

func (m *ScfMachine) executeWrite() *ScfCommand {
	if m.pc+5 > m.codeLength {
		command := m.fail("scf [ truncated")
		return &command
	}
	index := int(m.code[m.pc])<<8 | int(m.code[m.pc+1])
	op := m.code[m.pc+3]
	value := m.code[m.pc+4]
	m.pc += 5
	if m.skip == 0 {
		if op == '+' {
			value = value + m.flags[index]
		} else if op == '-' {
			value = m.flags[index] - value
		}
		m.flags[index] = value
	}
	return nil
}

func (m *ScfMachine) decodeMessage(start, length int) {
	text := decodeCodeText(m.code, start, length)
	var body strings.Builder
	body.Grow(len(text))
	cursor := 0
	for cursor < len(text) {
		if text[cursor] != '_' {
			body.WriteByte(text[cursor])
			cursor++
			continue
		}
		if cursor+1 >= len(text) {
			break
		}
		control := text[cursor+1]
		if control == 'f' || control == 'F' {
			end := strings.IndexByte(text[cursor+2:], ':')
			if end < 0 {
				break
			}
			end += cursor + 2
			m.pending = append(m.pending, ScfCommand{
				Kind:  ScfCommandKindStandee,
				Index: -1,
				Flag:  2,
				Text:  text[cursor+2 : end],
			})
			cursor = end + 1
		} else if control == 'r' || control == 'R' || control == 'n' || control == 't' || control == 'T' {
			body.WriteByte('\n')
			cursor += 2
		} else {
			cursor += 2
		}
	}
	m.pending = append(m.pending, ScfCommand{Kind: ScfCommandKindMessage, Text: body.String()})
}

func (m *ScfMachine) decodePagedMessage(start, length int) {
	text := decodeCodeText(m.code, start, length)
	var body strings.Builder
	rows := 0
	flush := func() {
		page := strings.TrimRight(body.String(), "\n")
		body.Reset()
		rows = 0
		if page != "" {
			m.pending = append(m.pending, ScfCommand{Kind: ScfCommandKindMessage, Text: page})
		}
	}
	cursor := 0
	for cursor < len(text) {
		if text[cursor] != '_' {
			body.WriteByte(text[cursor])
			cursor++
			continue
		}
		if cursor+1 >= len(text) {
			break
		}
		control := text[cursor+1]
		switch {
		case control == 'f' || control == 'F':
			end := strings.IndexByte(text[cursor+2:], ':')
			if end < 0 {
				cursor = len(text)
				continue
			}
			end += cursor + 2
			flush()
			if name := text[cursor+2 : end]; name != "" {
				m.pending = append(m.pending, ScfCommand{
					Kind:  ScfCommandKindStandee,
					Index: -1,
					Flag:  2,
					Text:  name,
				})
			}
			cursor = end + 1
		case control == 'r' || control == 'R':
			flush()
			cursor += 2
		case control == 'n' || control == 't' || control == 'T':
			body.WriteByte('\n')
			rows++
			if rows >= 4 {
				flush()
			}
			cursor += 2
		default:
			cursor += 2
		}
	}
	flush()
}

func decodeCodeText(code []byte, start, length int) string {
	if length <= 0 {
		return ""
	}
	filtered := make([]byte, 0, length)
	for i := 0; i < length; i++ {
		if value := code[start+i]; value != 0xFF {
			filtered = append(filtered, value)
		}
	}
	return DecodeText(filtered, DetectTextEncoding(filtered))
}
