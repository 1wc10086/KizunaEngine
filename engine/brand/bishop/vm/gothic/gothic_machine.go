package gothic

import (
	"strings"

	"kizuna/engine/core"
)

const gothicFetchStepLimit = 1000000

type GothicMachine struct {
	program          *GothicProgram
	variables        *GothicVariableTable
	calls            []int32
	branches         []bool
	menu             *GothicMenu
	last             *GothicStatement
	pc               int32
	instructionStart int32
	halted           bool
}

func NewGothicMachine(program *GothicProgram) *GothicMachine {
	if program == nil {
		program = NewGothicProgram(nil)
	}
	m := &GothicMachine{program: program, variables: NewGothicVariableTable()}
	for _, name := range program.DeclaredVariables() {
		m.variables.Declare(name, 0)
	}
	for _, name := range gothicSystemVariables {
		m.variables.Declare(name, 0)
	}
	if program.Count() > 0 {
		m.last = program.Statements()[0]
	}
	return m
}

func (m *GothicMachine) Program() *GothicProgram {
	return m.program
}

func (m *GothicMachine) ProgramCounter() int32 {
	return m.pc
}

func (m *GothicMachine) InstructionStart() int32 {
	return m.instructionStart
}

func (m *GothicMachine) Halted() bool {
	return m.halted
}

func (m *GothicMachine) Variables() *GothicVariableTable {
	return m.variables
}

func (m *GothicMachine) CallDepth() int {
	return len(m.calls)
}

func (m *GothicMachine) CanRewind() bool {
	return !m.halted && (m.pc > 0 || len(m.calls) > 0)
}

func (m *GothicMachine) Fetch() (action *GothicAction) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("gothic: fetch fatal: %v", r)
			m.halted = true
			action = m.finishNow()
		}
	}()
	if m.halted {
		return m.finishNow()
	}
	for step := 0; step < gothicFetchStepLimit; step++ {
		m.instructionStart = m.pc
		if m.pc < 0 || int(m.pc) >= m.program.Count() {
			return m.finishNow()
		}
		statement := m.program.Statements()[m.pc]
		m.last = statement
		entered := m.pc
		action := func() (a *GothicAction) {
			defer func() {
				if r := recover(); r != nil {
					core.Logf("gothic: fetch fault at %d: %v", entered, r)
					if m.pc == entered {
						m.pc++
					}
				}
			}()
			a = m.step(statement)
			return a
		}()
		if action != nil {
			return action
		}
		if m.halted {
			return m.finishNow()
		}
	}
	core.Logf("gothic: fetch step limit exceeded")
	return m.finishNow()
}

func (m *GothicMachine) Seek(label string) bool {
	target := m.program.FindLabel(label)
	if target < 0 || int(target) >= m.program.Count() {
		return false
	}
	m.branches = nil
	m.menu = nil
	m.pc = target
	return true
}

func (m *GothicMachine) Call(label string) bool {
	name := unquoteGothic(label)
	target := int32(-1)
	if len(name) > 0 {
		target = m.program.FindLabel(name)
	}
	if target < 0 || int(target) >= m.program.Count() {
		return false
	}
	m.calls = append(m.calls, m.pc)
	m.pc = target
	return true
}

func (m *GothicMachine) Choose(slot int32) bool {
	menu := m.menu
	if menu == nil || menu.Items == nil {
		return false
	}
	for _, item := range menu.Items {
		if item != nil && item.Slot == slot {
			m.pc = item.StatementIndex
			m.menu = nil
			return true
		}
	}
	return false
}

func (m *GothicMachine) Reset() {
	m.pc = 0
	m.instructionStart = 0
	m.calls = nil
	m.branches = nil
	m.menu = nil
	m.halted = false
}

func (m *GothicMachine) step(statement *GothicStatement) *GothicAction {
	switch statement.Kind {
	case GothicStatementEmpty:
		if startsCommand(statement.Raw) {
			return m.execute(statement)
		}
		m.pc++
		return nil
	case GothicStatementLabel:
		m.pc++
		return nil
	case GothicStatementAssignment:
		m.assign(statement)
		m.pc++
		return nil
	case GothicStatementText:
		m.pc++
		return NewGothicAction(GothicActionText, statement, 0, statement.Raw, nil)
	case GothicStatementPageBreak:
		m.pc++
		return NewGothicAction(GothicActionPageBreak, statement, pageBreakValue(statement), statement.Raw, nil)
	case GothicStatementWaitBreak:
		core.Logf("gothic: waitbreak skipped at %d", statement.Index)
		m.pc++
		return nil
	case GothicStatementCommand:
		return m.execute(statement)
	case GothicStatementFinish:
		m.pc++
		return m.finishNow()
	default:
		core.Logf("gothic: unknown statement kind at %d", statement.Index)
		m.pc++
		return nil
	}
}

func (m *GothicMachine) assign(statement *GothicStatement) {
	name := strings.TrimSpace(statement.Name)
	expression := assignmentExpression(statement)
	if len(name) == 0 || strings.EqualFold(name, "int") {
		core.Logf("gothic: bad assignment at %d", statement.Index)
		return
	}
	m.variables.Set(name, GothicEvaluateValue(expression, m.variables))
}

func (m *GothicMachine) finishNow() *GothicAction {
	m.halted = true
	statement := m.last
	if statement == nil {
		statement = m.fallbackStatement()
	}
	return NewGothicAction(GothicActionFinish, statement, 0, statement.Raw, nil)
}

func (m *GothicMachine) fallbackStatement() *GothicStatement {
	if m.program.Count() > 0 {
		return m.program.Statements()[0]
	}
	core.Logf("gothic: empty program")
	return &GothicStatement{}
}

// GothicMachineState 是存档/回想需要的 VM 最小快照。
// menu 不包含在内：存档只在 AwaitInput 时发生，回想也只从对白/网格进入；
// 如果以后要在选择肢上存档，需要把 GothicMenu 的指针恢复一并补上。
type GothicMachineState struct {
	PC              int32
	InstructionStar int32
	Calls           []int32
	Branches        []bool
	Halted          bool
}

func (m *GothicMachine) Snapshot() GothicMachineState {
	if m == nil {
		return GothicMachineState{}
	}
	return GothicMachineState{
		PC:              m.pc,
		InstructionStar: m.instructionStart,
		Calls:           append([]int32{}, m.calls...),
		Branches:        append([]bool{}, m.branches...),
		Halted:          m.halted,
	}
}

func (m *GothicMachine) Restore(state GothicMachineState) {
	if m == nil {
		return
	}
	m.pc = state.PC
	m.instructionStart = state.InstructionStar
	m.calls = append(m.calls[:0], state.Calls...)
	m.branches = append(m.branches[:0], state.Branches...)
	m.halted = state.Halted
	m.menu = nil
}
