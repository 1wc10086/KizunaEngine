package scf

import (
	"fmt"

	"kizuna/engine/core"
)

const (
	ScfFlagCapacity     = 65536
	ScfCondCapacity     = 8
	ScfMaxCallDepth     = 8
	ScfMinNameSlotCount = 16
)

const (
	scfFetchStepLimit    = 100000
	scfMaxTransferRounds = 1024
)

type ScfFunctionSlot struct {
	Number string
	Name   string
}

type ScfDataTable interface {
	FlagCount() int
	SourceCount() int
	GraphicNames() []string
	TrackNumbers() []string
	SoundNames() []string
	FunctionSlots() []ScfFunctionSlot
	SlotNames() []string
}

type ScfScenarioLoader func(scenario int) (*ScfScriptFile, error)

type scfCallFrame struct {
	scenario      int
	pc            int
	condDepth     int
	condStack     [ScfCondCapacity]int
	loopSlots     [ScfCondCapacity]int
	// elseSlots 标记某个 #i 是否出现过配对的 #l（else）。SCF 的 #l 不是
	// “循环开关”，而是 else：#i 条件为假时执行 #l 后面的块，否则跳过。
	elseSlots     [ScfCondCapacity]int
	latch         int
	messageOffset int
	source        []byte
}

type ScfMachine struct {
	table  ScfDataTable
	loader ScfScenarioLoader

	flags  []byte
	source []byte
	names  []string

	condStack [ScfCondCapacity]int
	loopSlots [ScfCondCapacity]int
	elseSlots [ScfCondCapacity]int
	frames    []scfCallFrame
	choices   []ScfChoice
	pending   []ScfCommand

	script *ScfScriptFile

	code             []byte
	poolFile         []byte
	codeLength       int
	pc               int
	instructionStart int
	scenario         int
	latch            int
	skip             int
	condDepth        int
	messageOffset    int
	parenArmed       bool
	halted           bool
	faulted          bool
	faultText        string
	pageBreaks       bool
}

// SetPageBreaks 开启后 _r/_n/_t 按原版 4 行一页拆分消息，并把 _f 头像命令放在页首。
func (m *ScfMachine) SetPageBreaks(enabled bool) {
	m.pageBreaks = enabled
}

func NewScfMachine(table ScfDataTable, loader ScfScenarioLoader) *ScfMachine {
	m := &ScfMachine{
		table:         table,
		loader:        loader,
		flags:         make([]byte, ScfFlagCapacity),
		source:        make([]byte, max(0, table.SourceCount())),
		messageOffset: -1,
	}
	m.resetNameSlots()
	return m
}

func (m *ScfMachine) Table() ScfDataTable {
	return m.table
}

func (m *ScfMachine) Script() *ScfScriptFile {
	return m.script
}

func (m *ScfMachine) Halted() bool {
	return m.halted
}

func (m *ScfMachine) Faulted() bool {
	return m.faulted
}

func (m *ScfMachine) ProgramCounter() int {
	return m.pc
}

func (m *ScfMachine) InstructionStart() int {
	return m.instructionStart
}

func (m *ScfMachine) ScenarioNumber() int {
	return m.scenario
}

func (m *ScfMachine) Choices() []ScfChoice {
	return m.choices
}

func (m *ScfMachine) Flags() []byte {
	return m.flags
}

func (m *ScfMachine) Source() []byte {
	return m.source
}

func (m *ScfMachine) MessageOffset() int {
	return m.messageOffset
}

func (m *ScfMachine) Start(scenarioNumber int) {
	m.halted = false
	m.faulted = false
	m.faultText = ""
	m.choices = m.choices[:0]
	m.pending = m.pending[:0]
	m.frames = m.frames[:0]
	clear(m.condStack[:])
	clear(m.loopSlots[:])
	clear(m.elseSlots[:])
	m.condDepth = 0
	m.skip = 0
	m.parenArmed = false
	m.messageOffset = -1
	m.instructionStart = 0
	m.resetNameSlots()
	m.loadScenarioInternal(scenarioNumber)
}

func (m *ScfMachine) Capture() ScfMachineState {
	flagLength := core.Clamp(m.table.FlagCount(), 0, len(m.flags))
	flags := make([]byte, flagLength)
	copy(flags, m.flags[:flagLength])
	frames := make([]ScfMachineState, len(m.frames))
	for i := range frames {
		frames[i] = captureFrameState(&m.frames[i])
	}
	return ScfMachineState{
		ProgramCounter: m.pc,
		ScenarioNumber: m.scenario,
		Flags:          flags,
		Source:         m.sizedSource(m.source),
		NameSlots:      append([]string(nil), m.names...),
		CallDepth:      len(m.frames),
		Frames:         frames,
		Halted:         m.halted,
		Faulted:        m.faulted,
		CondDepth:      m.condDepth,
		CondStack:      append([]int(nil), m.condStack[:]...),
		LoopSlots:      append([]int(nil), m.loopSlots[:]...),
		ElseSlots:      append([]int(nil), m.elseSlots[:]...),
		Latch:          m.latch,
		MessageOffset:  m.messageOffset,
	}
}

func (m *ScfMachine) Restore(state ScfMachineState) {
	m.pending = m.pending[:0]
	m.choices = m.choices[:0]
	m.frames = m.frames[:0]
	for i := range state.Frames {
		if len(m.frames) >= ScfMaxCallDepth {
			break
		}
		saved := &state.Frames[i]
		m.frames = append(m.frames, scfCallFrame{
			scenario:      saved.ScenarioNumber,
			pc:            saved.ProgramCounter,
			condDepth:     core.Clamp(saved.CondDepth, 0, ScfCondCapacity-1),
			condStack:     paddedSlots(saved.CondStack),
			loopSlots:     paddedSlots(saved.LoopSlots),
			elseSlots:     paddedSlots(saved.ElseSlots),
			latch:         saved.Latch,
			messageOffset: saved.MessageOffset,
			source:        append([]byte(nil), saved.Source...),
		})
	}
	clear(m.flags)
	copy(m.flags, state.Flags)
	clear(m.source)
	copy(m.source, state.Source)
	m.names = m.names[:0]
	m.names = append(m.names, state.NameSlots...)
	for len(m.names) < ScfMinNameSlotCount {
		m.names = append(m.names, "")
	}
	m.condDepth = core.Clamp(state.CondDepth, 0, ScfCondCapacity-1)
	copySlots(state.CondStack, m.condStack[:])
	copySlots(state.LoopSlots, m.loopSlots[:])
	copySlots(state.ElseSlots, m.elseSlots[:])
	m.latch = state.Latch
	m.messageOffset = state.MessageOffset
	m.skip = 0
	m.parenArmed = false
	m.halted = state.Halted
	m.faulted = state.Faulted
	m.faultText = ""
	if state.Faulted {
		m.faultText = "scf restored faulted state"
	}
	script := m.loadScenario(state.ScenarioNumber)
	if script == nil {
		m.fail(fmt.Sprintf("scf missing scenario %d", state.ScenarioNumber))
		return
	}
	m.script = script
	m.scenario = state.ScenarioNumber
	m.code = append([]byte(nil), script.Code()...)
	m.codeLength = len(m.code)
	m.poolFile = script.Data()
	m.pc = core.Clamp(state.ProgramCounter, 0, m.codeLength)
	m.instructionStart = m.pc
}

func (m *ScfMachine) loadScenarioInternal(scenario int) bool {
	script := m.loadScenario(scenario)
	if script == nil {
		m.fail(fmt.Sprintf("scf missing scenario %d", scenario))
		return false
	}
	m.script = script
	m.scenario = scenario
	m.code = append([]byte(nil), script.Code()...)
	m.codeLength = len(m.code)
	m.poolFile = script.Data()
	m.pc = 0
	m.instructionStart = 0
	m.latch = 0
	m.condDepth = 0
	clear(m.source)
	return true
}

func (m *ScfMachine) loadScenario(scenario int) *ScfScriptFile {
	if m.loader == nil {
		return nil
	}
	script, err := m.loader(scenario)
	if err != nil {
		core.Logf("scf: scenario %d loader failed: %v", scenario, err)
		return nil
	}
	return script
}

func (m *ScfMachine) resetNameSlots() {
	m.names = m.names[:0]
	m.names = append(m.names, m.table.SlotNames()...)
	if len(m.names) == 0 {
		for _, slot := range m.table.FunctionSlots() {
			m.names = append(m.names, slot.Number)
		}
	}
	for len(m.names) < ScfMinNameSlotCount {
		m.names = append(m.names, "")
	}
}

func (m *ScfMachine) captureFrame(scenario, pc int) scfCallFrame {
	return scfCallFrame{
		scenario:      scenario,
		pc:            pc,
		condDepth:     m.condDepth,
		condStack:     m.condStack,
		loopSlots:     m.loopSlots,
		elseSlots:     m.elseSlots,
		latch:         m.latch,
		messageOffset: m.messageOffset,
		source:        append([]byte(nil), m.source...),
	}
}

func captureFrameState(frame *scfCallFrame) ScfMachineState {
	return ScfMachineState{
		ProgramCounter: frame.pc,
		ScenarioNumber: frame.scenario,
		Source:         append([]byte(nil), frame.source...),
		CondDepth:      frame.condDepth,
		CondStack:      append([]int(nil), frame.condStack[:]...),
		LoopSlots:      append([]int(nil), frame.loopSlots[:]...),
		ElseSlots:      append([]int(nil), frame.elseSlots[:]...),
		Latch:          frame.latch,
		MessageOffset:  frame.messageOffset,
	}
}

func paddedSlots(source []int) [ScfCondCapacity]int {
	var slots [ScfCondCapacity]int
	copySlots(source, slots[:])
	return slots
}

func copySlots(source []int, target []int) {
	clear(target)
	for i := 0; i < len(target) && i < len(source); i++ {
		target[i] = source[i]
	}
}

func (m *ScfMachine) sizedSource(source []byte) []byte {
	copySource := make([]byte, len(m.source))
	copy(copySource, source)
	return copySource
}
