package bs

import (
	"kizuna/engine/core"
)

const (
	AncientStepBudget        = 100_000
	AncientDefaultStack      = 256
	AncientDefaultRegisters  = 8
	AncientDefaultVariables  = 1
	AncientDefaultProperties = 64
)

var (
	ancientHaltCommand = BsCommand{Kind: BsCommandKindHalt}
	ancientNoneCommand = BsCommand{Kind: BsCommandKindNone}
)

type AncientScriptMachine struct {
	script      ScriptFile
	code        []byte
	dialect     BsScriptDialect
	stack       []int32
	registers   []int32
	properties  []int32
	variables   []int32
	choices     []BsChoice
	stackTop    int
	accumulator int32
	flag        int32
	pc          int
	instruction int
	halted      bool
	faulted     bool
}

func NewAncientScriptMachine(script ScriptFile, dialect BsScriptDialect, code []byte) *AncientScriptMachine {
	return &AncientScriptMachine{
		script:      script,
		code:        code,
		dialect:     dialect,
		stack:       make([]int32, ancientCapacity(script.BsStackCapacity(), AncientDefaultStack)),
		registers:   make([]int32, ancientCapacity(script.BsRegisterCapacity(), AncientDefaultRegisters)),
		variables:   make([]int32, ancientCapacity(script.BsVariableCapacity(), AncientDefaultVariables)),
		properties:  make([]int32, ancientCapacity(script.BsPropertyCapacity(), script.PropertyNameCount())),
		accumulator: -1,
	}
}

func (m *AncientScriptMachine) Dialect() BsScriptDialect { return m.dialect }

func (m *AncientScriptMachine) Halted() bool { return m.halted }

func (m *AncientScriptMachine) Faulted() bool { return m.faulted }

func (m *AncientScriptMachine) ProgramCounter() int { return m.pc }

func (m *AncientScriptMachine) InstructionStart() int { return m.instruction }

func (m *AncientScriptMachine) Choices() []BsChoice { return m.choices }

func (m *AncientScriptMachine) StackDepth() int { return m.stackTop }

func (m *AncientScriptMachine) Accumulator() int32 { return m.accumulator }

func (m *AncientScriptMachine) Flag() int32 { return m.flag }

func (m *AncientScriptMachine) Registers() []int32 { return m.registers }

func (m *AncientScriptMachine) Variables() []int32 { return m.variables }

func (m *AncientScriptMachine) Properties() []int32 { return m.properties }

func (m *AncientScriptMachine) Property(index int) int32 {
	if index >= 0 && index < len(m.properties) {
		return m.properties[index]
	}
	return -1
}

func (m *AncientScriptMachine) SetProperty(index int, value int32) {
	writeInt32Array(m.properties, index, value)
}

func (m *AncientScriptMachine) Name(index int) string { return m.script.GetName(index) }

func (m *AncientScriptMachine) Message(index int) string { return m.script.GetMessage(index) }

func (m *AncientScriptMachine) Character(index int) string {
	return m.script.GetCharacterName(index)
}

func (m *AncientScriptMachine) Fetch() BsCommand {
	if m.halted || m.faulted {
		return ancientHaltCommand
	}
	for step := 0; step < AncientStepBudget; step++ {
		if m.halted || m.faulted {
			return ancientHaltCommand
		}
		if m.pc < 0 || m.pc >= len(m.code) {
			m.halted = true
			return ancientHaltCommand
		}
		m.instruction = m.pc
		opcode := BsSemantic(m.code[m.pc], m.dialect)
		if m.tryRunInternal(opcode) {
			continue
		}
		if !BsIsState(opcode) {
			core.Logf("script: FAULT ancient opcode 0x%02X at %d", m.code[m.instruction], m.instruction)
			m.faulted = true
			m.halted = true
			return ancientHaltCommand
		}
		command := m.decode(opcode)
		if command.Kind != BsCommandKindNone {
			return command
		}
	}
	core.Logf("script: ancient step budget exhausted at %d", m.pc)
	m.faulted = true
	m.halted = true
	return ancientHaltCommand
}

func (m *AncientScriptMachine) Choose(index int) bool {
	if index < 0 || index >= len(m.choices) {
		return false
	}
	choice := m.choices[index]
	m.choices = m.choices[:0]
	if choice.LabelOffset < 0 || choice.LabelOffset >= len(m.code) {
		core.Logf("script: ancient choice %d target %d is outside the code block", index, choice.LabelOffset)
		return false
	}
	m.pc = choice.LabelOffset
	return true
}

func (m *AncientScriptMachine) ClearStack() { m.stackTop = 0 }

func (m *AncientScriptMachine) Reset() {
	m.pc = 0
	m.stackTop = 0
	m.accumulator = -1
	for i := range m.properties {
		m.properties[i] = 0
	}
	m.choices = m.choices[:0]
	m.halted = false
	m.faulted = false
	m.seekBootLabel()
}

func (m *AncientScriptMachine) Seek(programCounter int) bool {
	if programCounter < 0 || programCounter >= len(m.code) {
		core.Logf("script: ancient seek %d is outside the code block", programCounter)
		return false
	}
	m.pc = programCounter
	m.instruction = programCounter
	m.halted = false
	m.faulted = false
	return true
}

func (m *AncientScriptMachine) Capture() BsMachineState {
	stack := make([]int32, len(m.stack))
	copy(stack, m.stack)
	registers := make([]int32, len(m.registers))
	copy(registers, m.registers)
	properties := make([]int32, len(m.properties))
	copy(properties, m.properties)
	variables := make([]int32, len(m.variables))
	copy(variables, m.variables)
	choices := make([]BsChoice, len(m.choices))
	copy(choices, m.choices)
	return BsMachineState{
		Dialect:          m.dialect,
		ProgramCounter:   m.pc,
		InstructionStart: m.instruction,
		StackTop:         m.stackTop,
		Stack:            stack,
		Accumulator:      m.accumulator,
		Flag:             m.flag,
		Registers:        registers,
		Properties:       properties,
		Variables:        variables,
		Choices:          choices,
		Halted:           m.halted,
		Faulted:          m.faulted,
	}
}

func (m *AncientScriptMachine) Restore(state BsMachineState) {
	if state.Dialect != m.dialect {
		core.Logf("script: ancient restore across dialects %d -> %d", int(state.Dialect), int(m.dialect))
	}
	loadInt32Slice(m.stack, state.Stack)
	m.stackTop = core.Clamp(state.StackTop, 0, len(m.stack))
	loadInt32Slice(m.registers, state.Registers)
	loadInt32Slice(m.properties, state.Properties)
	loadInt32Slice(m.variables, state.Variables)
	m.choices = m.choices[:0]
	for _, choice := range state.Choices {
		choice.Index = len(m.choices)
		m.choices = append(m.choices, choice)
	}
	m.pc = state.ProgramCounter
	m.instruction = state.InstructionStart
	m.accumulator = state.Accumulator
	m.flag = state.Flag
	m.halted = state.Halted
	m.faulted = state.Faulted
}

func (m *AncientScriptMachine) seekBootLabel() {
	offset := m.script.FindLabel("boot")
	if offset < 0 {
		core.Logf("script: ancient entry label 'boot' is missing")
		return
	}
	m.pc = offset
}

func ancientCapacity(declared, fallback int) int {
	if declared > 0 {
		return declared
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func writeInt32Array(array []int32, index int, value int32) bool {
	if index < 0 || index >= len(array) {
		return false
	}
	array[index] = value
	return true
}
