package scf

import "fmt"

func (m *ScfMachine) Choose(index int) bool {
	if m.halted || m.faulted {
		return false
	}
	if index < 0 || index >= len(m.choices) {
		m.fail(fmt.Sprintf("scf choice index %d", index))
		return false
	}
	entry := m.choices[index].Entry
	if entry < 0 || entry >= m.script.EntryCount() {
		m.fail(fmt.Sprintf("scf choice entry %d", entry))
		return false
	}
	head := m.script.Entries()[entry].Head
	if head != 0 {
		if !m.rebuildOptions(head) {
			return false
		}
		// 子清单可能因为条件全部隐藏而为空。此时不能返回 true，否则 run 会
		// 继续从 label 后面的第一条命令块执行，等于“点了类目就直接选第一项”。
		if len(m.choices) == 0 {
			return false
		}
		m.pending = append(m.pending, ScfCommand{Kind: ScfCommandKindOptions, Options: cloneChoices(m.choices)})
		return true
	}
	return m.chooseTransfer(m.script.Entries()[entry].Variable)
}

func (m *ScfMachine) executeEntries() *ScfCommand {
	if !m.rebuildOptions(0) {
		return m.faultResult()
	}
	if len(m.choices) == 0 {
		command := ScfCommand{Kind: ScfCommandKindNone}
		return &command
	}
	command := ScfCommand{Kind: ScfCommandKindOptions, Options: cloneChoices(m.choices)}
	return &command
}

func (m *ScfMachine) rebuildOptions(start int) bool {
	m.choices = m.choices[:0]
	if m.script.EntryCount() <= 0 {
		return true
	}
	// Seraph 的两级命令树：第一层选 entry0（見る）或 entry0.next（行動），
	// 第二层只应显示被选中节点的 head 链。旧实现无论 start 是否为 0 都先收集
	// entry0 链，导致第二层菜单里混入“見る/行動”两个类目，玩家点到类目又会
	// 递归展开。start==0 才是根菜单。
	if !m.collectChain(start) {
		return false
	}
	return true
}

func (m *ScfMachine) collectChain(entry int) bool {
	if m.script.EntryCount() <= 0 {
		return true
	}
	current := entry
	steps := 0
	for {
		steps++
		if steps > m.script.EntryCount() {
			m.fail(fmt.Sprintf("scf entry chain cycle at %d", current))
			return false
		}
		if current < 0 || current >= m.script.EntryCount() {
			m.fail(fmt.Sprintf("scf entry chain %d", current))
			return false
		}
		offset := m.script.PoolOffset(current)
		take, ok := m.tryPoolCondition(offset)
		if !ok {
			return false
		}
		if take {
			text, err := m.script.PoolText(offset)
			if err != nil {
				m.fail(fmt.Sprintf("scf pool text %d: %v", offset, err))
				return false
			}
			m.choices = append(m.choices, ScfChoice{Index: len(m.choices), Entry: current, Text: text})
		}
		current = m.script.Entries()[current].Next
		if current == 0 {
			break
		}
	}
	return true
}

func (m *ScfMachine) tryPoolCondition(offset int) (bool, bool) {
	pool := m.poolBase() + offset
	poolEnd := min(len(m.poolFile)-m.script.CodeLength(), len(m.poolFile))
	if pool < 0 || pool >= poolEnd {
		m.fail(fmt.Sprintf("scf pool address %d", offset))
		return false, false
	}
	if m.poolFile[pool] != 0 {
		return true, true
	}
	cursor := pool + 1
	for cursor < poolEnd && m.poolFile[cursor] != 0 {
		cursor++
	}
	if uint(cursor) >= uint(poolEnd) {
		m.fail(fmt.Sprintf("scf pool text %d", offset))
		return false, false
	}
	take, used := EvaluateExpression(m.poolFile, cursor+1, m.flags)
	if used < 0 {
		m.fail(fmt.Sprintf("scf pool expression %d", offset))
		return false, false
	}
	return take, true
}

func (m *ScfMachine) poolBase() int {
	return 10 + (6 * m.script.EntryCount()) + (2 * m.script.EntryCount()) + (4 * m.script.TransferCount()) + (2 * m.script.JumpTargetCount())
}

func (m *ScfMachine) chooseTransfer(variable int) bool {
	if variable < 0 || variable >= m.script.TransferCount() || variable >= len(m.source) {
		m.fail(fmt.Sprintf("scf transfer variable %d", variable))
		return false
	}
	match := m.script.Match(variable)
	set := m.script.Set(variable)
	jumpBase := m.script.TransferBase(variable)
	matched := false
	for round := 0; round < scfMaxTransferRounds; round++ {
		if m.source[variable] == match {
			if matched {
				m.fail("scf duplicate transfer match")
				return false
			}
			m.source[variable] = set
			matched = true
		}
		slot := jumpBase + int(m.source[variable])
		m.source[variable]++
		if slot < 0 || slot >= m.script.JumpTargetCount() {
			m.fail(fmt.Sprintf("scf jump slot %d", slot))
			return false
		}
		target := m.script.JumpTarget(slot)
		if uint(target) >= uint(m.codeLength) {
			m.fail(fmt.Sprintf("scf jump target %d", target))
			return false
		}
		position := target + 1
		advance := true
		if m.code[target] == 0 {
			var used int
			advance, used = EvaluateExpression(m.code, position, m.flags)
			if used < 0 {
				m.fail("scf jump condition")
				return false
			}
			position += used
		}
		if advance {
			m.pc = position
			m.instructionStart = position
			return true
		}
	}
	m.fail("scf transfer loop")
	return false
}

// RootOptions 重建 entry 0 的第一层命令（見る/やめ 等），供「戻る」使用。
func (m *ScfMachine) RootOptions() ScfCommand {
	if m.script == nil {
		return ScfCommand{Kind: ScfCommandKindNone}
	}
	if !m.rebuildOptions(0) {
		return ScfCommandFault(m.faultText)
	}
	if len(m.choices) == 0 {
		return ScfCommand{Kind: ScfCommandKindNone}
	}
	return ScfCommand{Kind: ScfCommandKindOptions, Options: cloneChoices(m.choices)}
}

func cloneChoices(choices []ScfChoice) []ScfChoice {
	cloned := make([]ScfChoice, len(choices))
	copy(cloned, choices)
	return cloned
}
