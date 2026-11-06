package tj2

import "fmt"

func (m *Tj2Machine) Choose(index int) bool {
	if m.halted || m.faulted {
		return false
	}
	var choice *Tj2Choice
	for i := range m.choices {
		if m.choices[i].Option == index {
			choice = &m.choices[i]
			break
		}
	}
	if choice == nil {
		// 宿主传错的 option 值不应该把 VM 打成 faulted：保持现状，
		// 让玩家还能继续点其它选项。
		return false
	}
	return m.chooseChoice(choice)
}

// ChooseDisplay 按“屏幕上的第几项”选择。
//
// 脚本的 option 值不一定是 0..n-1（旧脚本会用 entry/flag 编号），
// 宿主看到的 Choices 是重新编号的顺序列表；不在这里映射一次，
// 玩家点第一项时 VM 会找不到 option 而卡在同一个选择肢。
func (m *Tj2Machine) ChooseDisplay(index int) bool {
	if m.halted || m.faulted {
		return false
	}
	if index < 0 || index >= len(m.choices) {
		return false
	}
	choice := m.choices[index]
	return m.chooseChoice(&choice)
}

func (m *Tj2Machine) chooseChoice(choice *Tj2Choice) bool {
	entry := choice.Index
	if entry < 0 || entry >= m.script.EntryCount() {
		m.fail(fmt.Sprintf("tj2 choice entry %d", entry))
		return false
	}
	head := m.script.Entries()[entry].Head
	if head != 0 {
		if !m.rebuildOptions(head) {
			return false
		}
		m.choices = m.choices[:0]
		return true
	}
	if !m.chooseTransfer(m.script.Entries()[entry].Variable) {
		return false
	}
	m.choices = m.choices[:0]
	return true
}

func (m *Tj2Machine) executeEntries() *Tj2Command {
	if !m.rebuildOptions(0) {
		return &Tj2Command{Kind: Tj2CommandKindFaulted, Text: m.faultText}
	}
	if len(m.choices) == 0 {
		return nil
	}
	options := make([]Tj2Choice, len(m.choices))
	copy(options, m.choices)
	return &Tj2Command{Kind: Tj2CommandKindNone, Options: options}
}

func (m *Tj2Machine) rebuildOptions(start int) bool {
	m.choices = m.choices[:0]
	if m.script.EntryCount() > 0 && !m.collectChain(0) {
		return false
	}
	if start != 0 && !m.collectChain(start) {
		return false
	}
	return true
}

func (m *Tj2Machine) collectChain(entry int) bool {
	if m.script.EntryCount() <= 0 {
		return true
	}
	current := entry
	steps := 0
	for {
		steps++
		if steps > m.script.EntryCount() {
			m.fail(fmt.Sprintf("tj2 entry chain cycle at %d", current))
			return false
		}
		if current < 0 || current >= m.script.EntryCount() {
			m.fail(fmt.Sprintf("tj2 entry chain %d", current))
			return false
		}
		offset := m.script.PoolOffset(current)
		if offset < 0 || offset >= m.poolEnd-m.poolBase() {
			m.fail(fmt.Sprintf("tj2 pool address %d", offset))
			return false
		}
		take, ok := m.tryPoolCondition(offset)
		if !ok {
			return false
		}
		if take {
			text, err := m.script.PoolText(offset)
			if err != nil {
				m.fail(fmt.Sprintf("tj2 pool text %d: %v", offset, err))
				return false
			}
			m.choices = append(m.choices, Tj2Choice{Index: current, Option: len(m.choices), Text: text})
		}
		current = m.script.Entries()[current].Next
		if current == 0 {
			break
		}
	}
	return true
}

func (m *Tj2Machine) tryPoolCondition(offset int) (bool, bool) {
	pool := m.poolBase() + offset
	if pool < 0 || pool >= m.poolEnd {
		m.fail(fmt.Sprintf("tj2 pool address %d", offset))
		return false, false
	}
	if m.poolFile[pool] != 0 {
		return true, true
	}
	cursor := pool + 1
	for cursor < m.poolEnd && m.poolFile[cursor] != 0 {
		cursor++
	}
	if cursor < 0 || cursor >= m.poolEnd {
		m.fail(fmt.Sprintf("tj2 pool text %d", offset))
		return false, false
	}
	take, used := EvaluateExpression(m.poolFile, cursor+1, m.flags, ptrDialect(m.table.Dialect()))
	if used < 0 {
		m.fail(fmt.Sprintf("tj2 pool expression %d", offset))
		return false, false
	}
	return take, true
}

func (m *Tj2Machine) poolBase() int {
	return 10 + (6 * m.script.EntryCount()) + (2 * m.script.EntryCount()) + (4 * m.script.TransferCount()) + (2 * m.script.JumpTargetCount())
}

func (m *Tj2Machine) chooseTransfer(variable int) bool {
	if variable < 0 || variable >= m.script.TransferCount() || variable >= len(m.source) {
		m.fail(fmt.Sprintf("tj2 transfer variable %d", variable))
		return false
	}
	match := m.script.Match(variable)
	set := m.script.Set(variable)
	jumpBase := m.script.TransferBase(variable)
	matched := false
	for round := 0; round < maxTransferRounds; round++ {
		if m.source[variable] == match {
			if matched {
				m.fail("tj2 duplicate transfer match")
				return false
			}
			m.source[variable] = set
			matched = true
		}
		slot := jumpBase + int(m.source[variable])
		m.source[variable]++
		if slot < 0 || slot >= m.script.JumpTargetCount() {
			m.fail(fmt.Sprintf("tj2 jump slot %d", slot))
			return false
		}
		target := m.script.JumpTarget(slot)
		if target < 0 || target >= m.codeLength {
			m.fail(fmt.Sprintf("tj2 jump target %d", target))
			return false
		}
		position := target + 1
		advance := true
		if m.code[target] == 0 {
			var used int
			advance, used = EvaluateExpression(m.code, position, m.flags, ptrDialect(m.table.Dialect()))
			if used < 0 {
				m.fail("tj2 jump condition")
				return false
			}
			position += used
		}
		if advance {
			m.pc = position
			m.instruction = position
			return true
		}
	}
	m.fail("tj2 transfer loop")
	return false
}

func ptrDialect(d Tj2Dialect) *Tj2Dialect {
	return &d
}
