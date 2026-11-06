package tj2

import (
	"fmt"
	"strings"
	"time"

	"kizuna/engine/core"
	"kizuna/engine/core/textcodec"
)

const MaxCallDepth = 16

const (
	condCapacity      = 8
	fetchStepLimit    = 100000
	maxTransferRounds = 1024
	minNameSlotCount  = 16
)

type ScfEntry struct {
	Head     int
	Next     int
	Variable int
}

type ScfFile interface {
	Code() []byte
	Data() []byte
	EntryCount() int
	Entries() []ScfEntry
	PoolOffset(index int) int
	PoolText(offset int) (string, error)
	Match(transfer int) byte
	Set(transfer int) byte
	TransferBase(transfer int) int
	TransferCount() int
	JumpTargetCount() int
	JumpTarget(index int) int
}

type Tj2FunctionSlot struct {
	Number int
	Text   string
}

type Tj2NameRecord struct {
	First  string
	Second string
}

type Tj2Macro struct {
	From string
	To   string
}

type Tj2FlagAlias struct {
	Source int
	Target int
}

type Tj2DataTable interface {
	Dialect() Tj2Dialect
	FlagCount() int
	SourceCount() int
	NameRecords() []Tj2NameRecord
	FlagAliases() []Tj2FlagAlias
	Macros() []Tj2Macro
	GraphicNames() []string
	GraphicName(index int) string
	TrackNumbers() []string
	SoundNames() []string
	SoundName(index int) string
	FunctionSlots() []Tj2FunctionSlot
}

type Tj2Machine struct {
	table  Tj2DataTable
	loader func(scenario int) (ScfFile, error)

	flags  []byte
	source []byte
	names  []string

	condStack [condCapacity]int
	frames    []tj2CallFrame
	choices   []Tj2Choice

	script ScfFile

	code     []byte
	poolFile []byte

	poolEnd     int
	codeLength  int
	pc          int
	instruction int
	scenario    int

	latch          int
	locked         int
	skip           int
	pendingOffsetX int
	pendingOffsetY int
	loopHead       int
	condDepth      int
	messageOffset  int
	pendingWaitMs  int
	waitBaseMs     int64
	parenArmed     bool
	halted         bool
	faulted        bool
	faultText      string
}

func NewTj2Machine(table Tj2DataTable, loader func(scenario int) (ScfFile, error)) *Tj2Machine {
	d := table.Dialect()
	flagSize := d.FlagBufferSize(table.FlagCount())
	if flagSize < 16 {
		flagSize = 16
	}
	m := &Tj2Machine{
		table:         table,
		loader:        loader,
		flags:         make([]byte, flagSize),
		source:        make([]byte, max(0, table.SourceCount())),
		messageOffset: -1,
	}
	m.resetNameSlots()
	return m
}

func (m *Tj2Machine) Table() Tj2DataTable {
	return m.table
}

func (m *Tj2Machine) Halted() bool {
	return m.halted
}

func (m *Tj2Machine) Faulted() bool {
	return m.faulted
}

func (m *Tj2Machine) ProgramCounter() int {
	return m.pc
}

func (m *Tj2Machine) InstructionStart() int {
	return m.instruction
}

func (m *Tj2Machine) ScenarioNumber() int {
	return m.scenario
}

func (m *Tj2Machine) PendingWaitMilliseconds() int {
	return m.pendingWaitMs
}

func (m *Tj2Machine) Choices() []Tj2Choice {
	return m.choices
}

func (m *Tj2Machine) Flags() []byte {
	return m.flags
}

func (m *Tj2Machine) Source() []byte {
	return m.source
}

func (m *Tj2Machine) Dialect() Tj2Dialect {
	return m.table.Dialect()
}

func (m *Tj2Machine) ReadFlag(index int) (int32, error) {
	if index < 0 || index >= m.table.FlagCount() {
		return 0, fmt.Errorf("Flag index out of range: %d.", index)
	}
	return m.readFlag(index), nil
}

func (m *Tj2Machine) WriteFlag(index int, value int32) error {
	if index < 0 || index >= m.table.FlagCount() {
		return fmt.Errorf("Flag index out of range: %d.", index)
	}
	m.writeFlag(index, value)
	return nil
}

func (m *Tj2Machine) ClearPendingWait() {
	m.pendingWaitMs = 0
}

func (m *Tj2Machine) Start(scenarioNumber int) {
	m.halted = false
	m.faulted = false
	m.faultText = ""
	m.choices = m.choices[:0]
	m.frames = m.frames[:0]
	clear(m.condStack[:])
	m.condDepth = 0
	m.skip = 0
	m.pendingOffsetX = 0
	m.pendingOffsetY = 0
	m.loopHead = 0
	m.latch = 0
	m.locked = 0
	m.parenArmed = false
	m.pendingWaitMs = 0
	m.waitBaseMs = 0
	m.messageOffset = -1
	m.instruction = 0
	m.resetNameSlots()
	m.applyFlagAliases()
	m.loadScenarioInternal(scenarioNumber)
}

func (m *Tj2Machine) Capture() Tj2MachineState {
	frames := make([]Tj2CallFrameState, len(m.frames))
	for i := range frames {
		frames[i] = captureFrameState(&m.frames[i])
	}
	flags := make([]byte, len(m.flags))
	copy(flags, m.flags)
	source := make([]byte, len(m.source))
	copy(source, m.source)
	names := make([]string, len(m.names))
	copy(names, m.names)
	condStack := make([]int, len(m.condStack))
	copy(condStack, m.condStack[:])
	return Tj2MachineState{
		ProgramCounter: m.pc,
		ScenarioNumber: m.scenario,
		Flags:          flags,
		Source:         source,
		NameSlots:      names,
		CondDepth:      m.condDepth,
		CondStack:      condStack,
		LoopSlots:      derivedLoopSlots(m.condStack[:]),
		LoopHead:       m.loopHead,
		Latch:          m.latch,
		MessageOffset:  m.messageOffset,
		Halted:         m.halted,
		Faulted:        m.faulted,
		Frames:         frames,
	}
}

func (m *Tj2Machine) Restore(state Tj2MachineState) {
	m.choices = m.choices[:0]
	m.frames = m.frames[:0]
	for i := range state.Frames {
		if len(m.frames) >= MaxCallDepth {
			break
		}
		saved := &state.Frames[i]
		frame := newTj2CallFrame()
		frame.scenario = saved.ScenarioNumber
		frame.pc = saved.ProgramCounter
		frame.condDepth = core.Clamp(saved.CondDepth, 0, condCapacity)
		frame.condStack = paddedSlots(saved.CondStack)
		frame.latch = saved.Latch
		frame.messageOffset = saved.MessageOffset
		frame.source = m.sizedSource(saved.Source)
		m.frames = append(m.frames, *frame)
	}
	clear(m.flags)
	copy(m.flags, state.Flags)
	clear(m.source)
	copy(m.source, state.Source)
	m.names = m.names[:0]
	m.names = append(m.names, state.NameSlots...)
	for len(m.names) < minNameSlotCount {
		m.names = append(m.names, "")
	}
	m.condDepth = core.Clamp(state.CondDepth, 0, condCapacity)
	copySlots(state.CondStack, m.condStack[:])
	m.latch = state.Latch
	m.messageOffset = state.MessageOffset
	m.skip = 0
	m.loopHead = state.LoopHead
	m.locked = 0
	m.parenArmed = false
	m.pendingWaitMs = 0
	m.halted = state.Halted
	m.faulted = state.Faulted
	m.faultText = ""
	if state.Faulted {
		m.faultText = "tj2 restored faulted state"
	}
	script, err := m.loader(state.ScenarioNumber)
	if err != nil {
		core.Logf("tj2: tj2 scenario %d loader failed: %v", state.ScenarioNumber, err)
		script = nil
	}
	if script == nil {
		m.fail(fmt.Sprintf("tj2 missing scenario %d", state.ScenarioNumber))
		return
	}
	m.script = script
	m.scenario = state.ScenarioNumber
	m.code = append([]byte(nil), script.Code()...)
	m.codeLength = len(m.code)
	m.poolFile = script.Data()
	m.poolEnd = len(m.poolFile) - m.codeLength
	m.pc = core.Clamp(state.ProgramCounter, 0, m.codeLength)
	m.instruction = m.pc
	m.applyFlagAliases()
}

func (m *Tj2Machine) loadScenarioInternal(scenario int) bool {
	script, err := m.loader(scenario)
	if err != nil {
		core.Logf("tj2: tj2 scenario %d loader failed: %v", scenario, err)
		script = nil
	}
	if script == nil {
		m.fail(fmt.Sprintf("tj2 missing scenario %d", scenario))
		return false
	}
	m.script = script
	m.scenario = scenario
	m.choices = m.choices[:0]
	m.code = append([]byte(nil), script.Code()...)
	m.codeLength = len(m.code)
	m.poolFile = script.Data()
	m.poolEnd = len(m.poolFile) - m.codeLength
	m.pc = 0
	m.instruction = 0
	m.latch = 0
	m.condDepth = 0
	clear(m.source)
	return true
}

func (m *Tj2Machine) resetNameSlots() {
	m.names = m.names[:0]
	for _, record := range m.table.NameRecords() {
		m.names = append(m.names, record.First)
		m.names = append(m.names, record.Second)
	}
	for len(m.names) < minNameSlotCount {
		m.names = append(m.names, "")
	}
}

func (m *Tj2Machine) applyFlagAliases() {
	for _, alias := range m.table.FlagAliases() {
		m.writeFlag(alias.Target, m.readFlag(alias.Source))
	}
}

// flagCapacity 返回可寻址的 flag 数量。
func (m *Tj2Machine) flagCapacity() int {
	if m.table.Dialect().WideFlags() {
		return len(m.flags) / 2
	}
	return len(m.flags)
}

func (m *Tj2Machine) readFlag(index int) int32 {
	if index < 0 || index >= m.flagCapacity() {
		// 旧版 EXSCHED 表（学園 2001）的版本号落在"有 flag alias 表"的
		// 区间里，但实际没有别名段；越界读必须回落 0，不能 panic。
		return 0
	}
	if m.table.Dialect().WideFlags() {
		return int32(m.flags[2*index])<<8 | int32(m.flags[2*index+1])
	}
	return int32(m.flags[index])
}

func (m *Tj2Machine) writeFlag(index int, value int32) {
	if index < 0 || index >= m.flagCapacity() {
		return
	}
	d := m.table.Dialect()
	value = core.ClampInt32(value, 0, int32(d.FlagMaximum))
	if d.WideFlags() {
		m.flags[2*index] = byte(value >> 8)
		m.flags[2*index+1] = byte(value)
		return
	}
	m.flags[index] = byte(value)
}

func (m *Tj2Machine) tryReadByte() (byte, bool) {
	if m.pc < 0 || m.pc >= m.codeLength {
		return 0, false
	}
	value := m.code[m.pc]
	m.pc++
	return value, true
}

func (m *Tj2Machine) tryReadU16() (int, bool) {
	if m.pc < 0 || m.pc+2 > m.codeLength {
		return 0, false
	}
	value := int(m.code[m.pc])<<8 | int(m.code[m.pc+1])
	m.pc += 2
	return value, true
}

func (m *Tj2Machine) trySkip(length int) bool {
	if length < 0 || m.pc+length > m.codeLength {
		return false
	}
	m.pc += length
	return true
}

func (m *Tj2Machine) tryReadCString() (int, int, bool) {
	start := m.pc
	for m.pc < m.codeLength && m.code[m.pc] != 0 {
		m.pc++
	}
	if m.pc < 0 || m.pc >= m.codeLength {
		return 0, 0, false
	}
	length := m.pc - start
	m.pc++
	return start, length, true
}

func (m *Tj2Machine) blocked() bool {
	return m.skip != 0 || m.locked != 0
}

func captureFrameState(frame *tj2CallFrame) Tj2CallFrameState {
	condStack := make([]int, len(frame.condStack))
	copy(condStack, frame.condStack)
	source := make([]byte, len(frame.source))
	copy(source, frame.source)
	return Tj2CallFrameState{
		ProgramCounter: frame.pc,
		ScenarioNumber: frame.scenario,
		Source:         source,
		CondDepth:      frame.condDepth,
		CondStack:      condStack,
		LoopSlots:      derivedLoopSlots(frame.condStack),
		Latch:          frame.latch,
		MessageOffset:  frame.messageOffset,
	}
}

func derivedLoopSlots(condStack []int) []int {
	slots := make([]int, condCapacity)
	for depth := 1; depth < len(slots) && depth-1 < len(condStack); depth++ {
		slots[depth] = condStack[depth-1]
	}
	return slots
}

func paddedSlots(source []int) []int {
	slots := make([]int, condCapacity)
	copySlots(source, slots)
	return slots
}

func copySlots(source []int, target []int) {
	clear(target)
	for i := 0; i < len(target) && i < len(source); i++ {
		target[i] = source[i]
	}
}

func (m *Tj2Machine) sizedSource(source []byte) []byte {
	cp := make([]byte, len(m.source))
	copy(cp, source)
	return cp
}

func decodeBytes(code []byte, start, length int) string {
	if length <= 0 {
		return ""
	}
	filtered := make([]byte, 0, length)
	for i := 0; i < length; i++ {
		if value := code[start+i]; value != 0xFF {
			filtered = append(filtered, value)
		}
	}
	return textcodec.DecodeCP932(filtered)
}

func (m *Tj2Machine) expandMacros(text string) string {
	macros := m.table.Macros()
	if len(macros) == 0 || len(text) == 0 {
		return text
	}
	var body strings.Builder
	body.Grow(len(text))
	cursor := 0
	for cursor < len(text) {
		var match string
		var replacement string
		matched := false
		for i := range macros {
			from := macros[i].From
			if len(from) == 0 || cursor+len(from) > len(text) {
				continue
			}
			hit := true
			for j := 0; j < len(from); j++ {
				if text[cursor+j] != from[j] {
					hit = false
					break
				}
			}
			if hit {
				match = from
				replacement = macros[i].To
				matched = true
				break
			}
		}
		if !matched {
			body.WriteByte(text[cursor])
			cursor++
		} else {
			body.WriteString(replacement)
			cursor += len(match)
		}
	}
	return body.String()
}

func tickMilliseconds() int64 {
	return time.Now().UnixMilli()
}
