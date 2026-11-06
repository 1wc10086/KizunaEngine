package bs

import (
	"kizuna/engine/core"
)

const (
	RevStepBudget        = 100_000
	RevDefaultStack      = 256
	RevDefaultFlags      = 2
	RevDefaultVariables  = 1
	RevDefaultProperties = 64
	RevCallbackCapacity  = 1024
)

const (
	revSoundChannel = 3
	revVoiceChannel = 1
)

var (
	revHaltCommand = BsCommand{Kind: BsCommandKindHalt}
	revNoneCommand = BsCommand{Kind: BsCommandKindNone}
)

type RevScriptMachine struct {
	script      ScriptFile
	code        []byte
	dialect     BsScriptDialect
	stack       []int32
	flags       []int32
	variables   []int32
	properties  []int32
	callbacks   []int32
	counters    []byte
	choices     []BsChoice
	lastSelect  []BsChoice
	stackTop    int
	callbackTop int
	accumulator int32
	watch       int
	pc          int
	instruction int
	selectHold  bool
	selectLog   bool
	watchLog    bool
	halted      bool
	faulted     bool
}

func NewRevScriptMachine(script ScriptFile, dialect BsScriptDialect, code []byte) *RevScriptMachine {
	count := script.BsChoiceSlotCount()
	if count < 1 {
		count = 1
	}
	return &RevScriptMachine{
		script:      script,
		code:        code,
		dialect:     dialect,
		stack:       make([]int32, revCapacity(script.BsGlobalCapacity(), RevDefaultStack)),
		flags:       make([]int32, revCapacity(script.BsFlagCapacity(), RevDefaultFlags)),
		variables:   make([]int32, revCapacity(script.BsVariableArrayCapacity(), RevDefaultVariables)),
		properties:  make([]int32, revCapacity(script.BsPropertyArrayCapacity(), RevDefaultProperties)),
		callbacks:   make([]int32, RevCallbackCapacity*4),
		counters:    make([]byte, count),
		accumulator: -1,
		watch:       -1,
	}
}

func (m *RevScriptMachine) Dialect() BsScriptDialect { return m.dialect }

func (m *RevScriptMachine) Halted() bool { return m.halted }

func (m *RevScriptMachine) Faulted() bool { return m.faulted }

func (m *RevScriptMachine) ProgramCounter() int { return m.pc }

func (m *RevScriptMachine) InstructionStart() int { return m.instruction }

func (m *RevScriptMachine) Choices() []BsChoice { return m.choices }

func (m *RevScriptMachine) StackDepth() int { return m.stackTop }

func (m *RevScriptMachine) Accumulator() int32 { return m.accumulator }

func (m *RevScriptMachine) Watch() int { return m.watch }

func (m *RevScriptMachine) Flag() int32 {
	if len(m.flags) > 0 {
		return m.flags[0]
	}
	return 0
}

func (m *RevScriptMachine) Registers() []int32 { return m.flags }

func (m *RevScriptMachine) Variables() []int32 { return m.variables }

func (m *RevScriptMachine) Properties() []int32 { return m.properties }

func (m *RevScriptMachine) Property(index int) int32 {
	if index >= 0 && index < len(m.properties) {
		return m.properties[index]
	}
	return -1
}

func (m *RevScriptMachine) SetProperty(index int, value int32) {
	writeInt32Array(m.properties, index, value)
}

func (m *RevScriptMachine) Name(index int) string { return m.script.GetName(index) }

func (m *RevScriptMachine) Message(index int) string { return m.script.GetMessage(index) }

func (m *RevScriptMachine) Character(index int) string {
	return m.script.GetCharacterName(index)
}

func (m *RevScriptMachine) Fetch() BsCommand {
	if m.halted || m.faulted {
		return revHaltCommand
	}
	if !RevIsRev(m.dialect) {
		core.Logf("script: rev machine created for non rev dialect %d", int(m.dialect))
		m.faulted = true
		m.halted = true
		return revHaltCommand
	}
	for step := 0; step < RevStepBudget; step++ {
		if m.halted || m.faulted {
			return revHaltCommand
		}
		if m.pc < 0 || m.pc >= len(m.code) {
			m.halted = true
			return revHaltCommand
		}
		m.instruction = m.pc
		opcode := m.code[m.pc]
		if m.tryRunInternal(opcode) {
			continue
		}
		if !RevIsMessage(m.dialect, opcode) && !RevIsState(m.dialect, opcode) &&
			!RevIsNoOperation(m.dialect, opcode) && !isRevControlOpcode(opcode) {
			core.Logf("script: FAULT rev opcode 0x%02X at %d (%d)", opcode, m.instruction, RevVersion(m.dialect))
			m.faulted = true
			m.halted = true
			return revHaltCommand
		}
		command := m.decode(opcode)
		if command.Kind != BsCommandKindNone {
			return command
		}
	}
	core.Logf("script: rev step budget exhausted at %d", m.pc)
	m.faulted = true
	m.halted = true
	return revHaltCommand
}

func isRevControlOpcode(opcode byte) bool { return opcode <= 0x02 }

func (m *RevScriptMachine) Choose(index int) bool {
	if index < 0 || index >= len(m.choices) {
		return false
	}
	choice := m.choices[index]
	m.choices = m.choices[:0]
	m.callbackTop = 0
	m.selectHold = false
	if choice.LabelOffset < 0 || choice.LabelOffset >= len(m.code) {
		core.Logf("script: rev choice %d target %d is outside the code block", index, choice.LabelOffset)
		return false
	}
	m.MarkChoiceRead(choice.Index)
	m.pc = choice.LabelOffset
	return true
}

func (m *RevScriptMachine) ChoiceCounters() []byte { return m.counters }

func (m *RevScriptMachine) MarkChoiceRead(slotId int) {
	if slotId < 0 || slotId >= len(m.counters) {
		core.Logf("script: rev choice slot %d is outside the seen array", slotId)
		return
	}
	if m.counters[slotId] != 0xFF {
		m.counters[slotId]++
	}
}

func (m *RevScriptMachine) ClearStack() { m.stackTop = 0 }

func (m *RevScriptMachine) Reset() {
	m.pc = 0
	m.stackTop = 0
	m.callbackTop = 0
	m.accumulator = -1
	m.watch = -1
	for i := range m.stack {
		m.stack[i] = 0
	}
	for i := range m.flags {
		m.flags[i] = 0
	}
	for i := range m.properties {
		m.properties[i] = 0
	}
	for i := range m.callbacks {
		m.callbacks[i] = 0
	}
	for i := range m.counters {
		m.counters[i] = 0
	}
	m.choices = m.choices[:0]
	m.lastSelect = nil
	m.selectHold = false
	m.halted = false
	m.faulted = false
	m.seekLabel("boot")
}

func (m *RevScriptMachine) Seek(programCounter int) bool {
	if programCounter < 0 || programCounter >= len(m.code) {
		core.Logf("script: rev seek %d is outside the code block", programCounter)
		return false
	}
	m.pc = programCounter
	m.instruction = programCounter
	m.halted = false
	m.faulted = false
	return true
}

func (m *RevScriptMachine) Capture() RevMachineState {
	stack := make([]int32, len(m.stack))
	copy(stack, m.stack)
	flags := make([]int32, len(m.flags))
	copy(flags, m.flags)
	variables := make([]int32, len(m.variables))
	copy(variables, m.variables)
	properties := make([]int32, len(m.properties))
	copy(properties, m.properties)
	callbacks := make([]int32, len(m.callbacks))
	copy(callbacks, m.callbacks)
	counters := make([]byte, len(m.counters))
	copy(counters, m.counters)
	choices := make([]BsChoice, len(m.choices))
	copy(choices, m.choices)
	return RevMachineState{
		Dialect:          m.dialect,
		ProgramCounter:   m.pc,
		InstructionStart: m.instruction,
		StackTop:         m.stackTop,
		Stack:            stack,
		Accumulator:      m.accumulator,
		Watch:            m.watch,
		Flags:            flags,
		Variables:        variables,
		Properties:       properties,
		CallbackTop:      m.callbackTop,
		Callbacks:        callbacks,
		ChoiceCounters:   counters,
		Choices:          choices,
		Halted:           m.halted,
		Faulted:          m.faulted,
	}
}

func (m *RevScriptMachine) Restore(state RevMachineState) {
	if state.Dialect != m.dialect {
		core.Logf("script: rev restore across dialects %d -> %d", int(state.Dialect), int(m.dialect))
	}
	loadInt32Slice(m.stack, state.Stack)
	m.stackTop = core.Clamp(state.StackTop, 0, len(m.stack))
	loadInt32Slice(m.flags, state.Flags)
	loadInt32Slice(m.variables, state.Variables)
	loadInt32Slice(m.properties, state.Properties)
	loadInt32Slice(m.callbacks, state.Callbacks)
	m.callbackTop = core.Clamp(state.CallbackTop, 0, len(m.callbacks))
	for i := range m.counters {
		m.counters[i] = 0
	}
	copy(m.counters, state.ChoiceCounters)
	m.choices = m.choices[:0]
	m.choices = append(m.choices, state.Choices...)
	m.lastSelect = nil
	m.selectHold = false
	m.pc = state.ProgramCounter
	m.instruction = state.InstructionStart
	m.accumulator = state.Accumulator
	m.watch = state.Watch
	m.halted = state.Halted
	m.faulted = state.Faulted
}

func revCapacity(declared, fallback int) int {
	return max(declared, max(fallback, 1))
}
