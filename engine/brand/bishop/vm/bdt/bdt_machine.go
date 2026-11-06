package bdt

import (
	"strings"

	"kizuna/engine/core"
)

const (
	BdtXCount         = 32
	BdtZCount         = 32
	BdtVariableCap    = 128
	BdtFlagCap        = 600
	BdtMaxCallDepth   = 64
	BdtMaxChoices     = 8
	BdtTextSpeedMax   = 12
	BdtXBankMax       = 20000
	BdtByteMax        = 255
	BdtSpriteSlotSize = 8
)

const (
	bdtStepLimit            = 100000
	bdtConsequentDepthLimit = 16
)

type BdtDialect int

const (
	BdtDialectContainer BdtDialect = iota
	BdtDialectManin
)

type bdtSpritePlacement struct {
	name string
	x    int32
	y    int32
	mode int32
}

type bdtPendingChoice struct {
	label string
	text  string
}

type BdtMachine struct {
	file        *BdtScriptFile
	host        BdtHost
	loader      BdtScriptLoader
	lines       []BdtLine
	positions   map[int]int
	variables   [BdtVariableCap]int32
	sceneFlags  [BdtFlagCap]int32
	systemFlags [BdtFlagCap]int32
	x           []int32
	z           []int32
	calls       []int
	choices     []BdtChoiceOption
	pending     []bdtPendingChoice
	spriteUsed  [BdtSpriteSlotSize]bool
	spriteSlots [BdtSpriteSlotSize]bdtSpritePlacement

	dialect        BdtDialect
	maninTimeLimit int
	maninTargets   [6]string
	state          *BdtMachineState

	pc               int
	instructionStart int
	stop             BdtStop
	halted           bool
	faulted          bool
	waitMilliseconds int
	resumePc         int
	textSpeed        int
	consequentDepth  int
	wjFile           string
	wjLabel          string
	czFirst          int32
	czSecond         int32
}

func NewBdtMachine(file *BdtScriptFile, host BdtHost, loader BdtScriptLoader) *BdtMachine {
	if file == nil {
		panic("bdt: nil script file")
	}
	if host == nil {
		panic("bdt: nil host")
	}
	if loader == nil {
		panic("bdt: nil script loader")
	}
	m := &BdtMachine{
		file:   file,
		host:   host,
		loader: loader,
		lines:  file.Lines,
		x:      make([]int32, BdtXCount),
		z:      make([]int32, BdtZCount),
	}
	m.state = &BdtMachineState{
		X:           m.x,
		Z:           m.z,
		Flags:       m.sceneFlags[:],
		SystemFlags: m.systemFlags[:],
		CallStack:   m.calls,
	}
	m.rebuildIndex()
	return m
}

func (m *BdtMachine) State() *BdtMachineState {
	m.state.ProgramCounter = m.lineIndex(m.pc)
	m.state.X = m.x
	m.state.Z = m.z
	m.state.Flags = m.sceneFlags[:]
	m.state.SystemFlags = m.systemFlags[:]
	m.state.CallStack = m.calls
	m.state.Stop = m.stop
	return m.state
}

func (m *BdtMachine) Dialect() BdtDialect {
	return m.dialect
}

func (m *BdtMachine) SetDialect(dialect BdtDialect) {
	m.dialect = dialect
}

func (m *BdtMachine) SetXValue(index int, value int32) {
	if index < 0 || index >= len(m.x) {
		return
	}
	m.x[index] = value
	m.syncStores()
}

func (m *BdtMachine) SetZValue(index int, value int32) {
	if index < 0 || index >= len(m.z) {
		return
	}
	m.z[index] = value
	m.syncStores()
}

func (m *BdtMachine) ManinTimeLimit() int {
	return m.maninTimeLimit
}

func (m *BdtMachine) ManinTargets() []string {
	out := make([]string, len(m.maninTargets))
	copy(out, m.maninTargets[:])
	return out
}

func (m *BdtMachine) CloseManinStop() {
	if m.stop == BdtStopAttack || m.stop == BdtStopOrganize || m.stop == BdtStopStaffRoll {
		m.stop = BdtStopNone
	}
}

func (m *BdtMachine) Stop() BdtStop {
	return m.stop
}

func (m *BdtMachine) ProgramCounter() int {
	return m.lineIndex(m.pc)
}

func (m *BdtMachine) InstructionStart() int {
	return m.lineIndex(m.instructionStart)
}

func (m *BdtMachine) Faulted() bool {
	return m.faulted
}

func (m *BdtMachine) Halted() bool {
	return m.halted
}

func (m *BdtMachine) Choices() []BdtChoiceOption {
	return m.choices
}

func (m *BdtMachine) X() []int32 {
	return m.x
}

func (m *BdtMachine) Z() []int32 {
	return m.z
}

func (m *BdtMachine) WaitMilliseconds() int {
	return m.waitMilliseconds
}

func (m *BdtMachine) WjArmed() bool {
	return len(m.wjLabel) > 0
}

func (m *BdtMachine) WjFile() string {
	return m.wjFile
}

func (m *BdtMachine) WjLabel() string {
	return m.wjLabel
}

func (m *BdtMachine) TextSpeed() int {
	return m.textSpeed
}

func (m *BdtMachine) Start(label string) {
	for i := range m.variables {
		m.variables[i] = 0
	}
	for i := range m.x {
		m.x[i] = 0
	}
	for i := range m.z {
		m.z[i] = 0
	}
	for i := range m.sceneFlags {
		m.sceneFlags[i] = 0
	}
	for i := range m.systemFlags {
		m.systemFlags[i] = 0
	}
	for i := range m.spriteUsed {
		m.spriteUsed[i] = false
	}
	m.calls = m.calls[:0]
	m.choices = m.choices[:0]
	m.pending = m.pending[:0]
	m.waitMilliseconds = 0
	m.resumePc = 0
	m.textSpeed = 0
	m.consequentDepth = 0
	m.wjFile = ""
	m.wjLabel = ""
	m.czFirst = 0
	m.czSecond = 0
	m.halted = false
	m.faulted = false
	m.stop = BdtStopNone
	m.pc = 0
	m.instructionStart = 0
	if !m.seekLabel(label) {
		core.Logf("bdt: start label missing: %s", label)
		m.faulted = true
		m.halt()
	}
}

func (m *BdtMachine) Step() {
	if m.halted {
		m.stop = BdtStopHalt
		return
	}
	if m.stop != BdtStopNone {
		return
	}
	m.syncStores()
	for i := 0; i < bdtStepLimit; i++ {
		m.instructionStart = m.pc
		if m.pc < 0 || m.pc >= len(m.lines) {
			m.halt()
			return
		}
		m.executeLine(m.lines[m.pc])
		if m.halted {
			m.stop = BdtStopHalt
			return
		}
		if m.stop != BdtStopNone {
			return
		}
	}
	core.Logf("bdt: step limit exceeded at %d", m.lineIndex(m.pc))
}

func (m *BdtMachine) Choose(index int) bool {
	if m.stop != BdtStopChoice {
		return false
	}
	if index < 0 || index >= len(m.choices) {
		return false
	}
	label := m.choices[index].Label
	m.choices = m.choices[:0]
	m.pending = m.pending[:0]
	m.stop = BdtStopNone
	if !m.jumpTo(label) {
		m.pc = m.resumePc
	}
	return true
}

func (m *BdtMachine) JumpToLabel(label string) bool {
	target := strings.TrimSpace(label)
	if len(target) == 0 {
		return false
	}
	if !m.jumpTo(target) {
		return false
	}
	if m.stop == BdtStopMap {
		m.stop = BdtStopNone
	}
	return true
}

func (m *BdtMachine) Resume() {
	if m.stop == BdtStopWait || m.stop == BdtStopText || m.stop == BdtStopInput || m.stop == BdtStopScreen {
		m.stop = BdtStopNone
	}
}

func (m *BdtMachine) CloseMap() {
	if m.stop == BdtStopMap {
		m.stop = BdtStopNone
	}
}

// LeaveTitle 结束 T 指令造成的标题停止态，脚本从 T 之后的下一条继续执行。
// 不复用 Resume：Resume 会被其他停止态的推进路径调用，标题只能由玩家选 START 解除。
func (m *BdtMachine) LeaveTitle() {
	if m.stop == BdtStopTitle {
		m.stop = BdtStopNone
	}
}

func (m *BdtMachine) Capture() *BdtMachineState {
	m.syncStores()
	out := &BdtMachineState{
		ProgramCounter: m.lineIndex(m.pc),
		X:              make([]int32, len(m.x)),
		Z:              make([]int32, len(m.z)),
		Flags:          make([]int32, len(m.sceneFlags)),
		SystemFlags:    make([]int32, len(m.systemFlags)),
		CallStack:      make([]int, len(m.calls)),
		Stop:           m.stop,
	}
	copy(out.X, m.x)
	copy(out.Z, m.z)
	copy(out.Flags, m.sceneFlags[:])
	copy(out.SystemFlags, m.systemFlags[:])
	copy(out.CallStack, m.calls)
	return out
}

func (m *BdtMachine) Restore(state *BdtMachineState) {
	if state == nil {
		panic("bdt: nil machine state")
	}
	m.choices = m.choices[:0]
	m.pending = m.pending[:0]
	m.calls = m.calls[:0]
	if state.CallStack != nil {
		for _, entry := range state.CallStack {
			if len(m.calls) >= BdtMaxCallDepth {
				break
			}
			m.calls = append(m.calls, entry)
		}
	}
	if state.X != nil {
		if len(state.X) >= BdtVariableCap {
			copy(m.variables[:], state.X)
			for i := 0; i < BdtXCount; i++ {
				m.x[i] = m.variables[96+i]
			}
			for i := 0; i < BdtZCount; i++ {
				m.z[i] = m.variables[i]
			}
		} else {
			for i := 0; i < BdtXCount && i < len(state.X); i++ {
				m.x[i] = state.X[i]
				m.variables[96+i] = state.X[i]
			}
		}
	}
	if state.Z != nil {
		for i := 0; i < BdtZCount && i < len(state.Z); i++ {
			m.z[i] = state.Z[i]
			m.variables[i] = state.Z[i]
		}
	}
	if state.Flags != nil {
		copy(m.sceneFlags[:], state.Flags)
	}
	if state.SystemFlags != nil {
		copy(m.systemFlags[:], state.SystemFlags)
	}
	m.syncStores()
	m.waitMilliseconds = 0
	m.halted = state.Stop == BdtStopHalt
	m.faulted = false
	m.stop = state.Stop
	if !m.seekLabelIndex(state.ProgramCounter) {
		core.Logf("bdt: restore position missing: %d", state.ProgramCounter)
		m.halt()
	}
}

func (m *BdtMachine) halt() {
	m.halted = true
	m.stop = BdtStopHalt
}

func (m *BdtMachine) lineIndex(position int) int {
	if position >= 0 && position < len(m.lines) {
		return m.lines[position].Index
	}
	return position
}

func (m *BdtMachine) rebuildIndex() {
	m.positions = make(map[int]int, len(m.lines))
	for i := range m.lines {
		m.positions[m.lines[i].Index] = i
	}
}

func (m *BdtMachine) seekLabelIndex(index int) bool {
	position, ok := m.positions[index]
	if !ok {
		return false
	}
	m.pc = position
	return true
}

func (m *BdtMachine) seekLabel(label string) bool {
	if index, ok := m.file.TryGetLabel(label); ok {
		return m.seekLabelIndex(index)
	}
	return false
}

func (m *BdtMachine) jumpTo(label string) bool {
	target := strings.TrimSpace(label)
	if len(target) == 0 {
		core.Logf("bdt: empty jump target")
		return false
	}
	if index, ok := m.file.TryGetLabel(target); ok {
		if m.seekLabelIndex(index) {
			return true
		}
	}
	other := m.loadScript(target)
	if other != nil {
		if index, ok := other.TryGetLabel(target); ok {
			m.file = other
			m.lines = other.Lines
			m.rebuildIndex()
			return m.seekLabelIndex(index)
		}
	}
	core.Logf("bdt: unknown label: %s", target)
	return false
}

func (m *BdtMachine) loadScript(id string) (other *BdtScriptFile) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				core.Logf("bdt: script loader failed: %v", r)
				other = nil
			}
		}()
		other = m.loader.Load(id)
	}()
	return other
}

func (m *BdtMachine) syncStores() {
	for i := 0; i < BdtXCount; i++ {
		m.variables[96+i] = m.x[i]
	}
	for i := 0; i < BdtZCount; i++ {
		m.variables[i] = m.z[i]
	}
}

func (m *BdtMachine) getVariable(basis int, number int32) int32 {
	if basis == 96 {
		xn := number & 31
		if xn < BdtXCount {
			return m.x[xn]
		}
		return m.variables[96+xn]
	}
	if basis == 0 {
		zn := number & 31
		if zn < BdtZCount {
			return m.z[zn]
		}
		return m.variables[zn]
	}
	if basis == 32 {
		return m.variables[32+(number&15)]
	}
	if basis == 48 {
		return m.variables[48+(number&15)]
	}
	return m.variables[64+(number&31)]
}

func (m *BdtMachine) setVariable(basis int, number int32, value int32) {
	if basis == 96 {
		value = bdtClampInt32(value, 0, BdtXBankMax)
	} else {
		value = bdtClampInt32(value, 0, BdtByteMax)
	}
	if basis == 96 {
		xn := number & 31
		if xn < BdtXCount {
			m.x[xn] = value
		}
		m.variables[96+xn] = value
		return
	}
	if basis == 0 {
		zn := number & 31
		if zn < BdtZCount {
			m.z[zn] = value
		}
		m.variables[zn] = value
		return
	}
	if basis == 32 {
		m.variables[32+(number&15)] = value
		return
	}
	if basis == 48 {
		m.variables[48+(number&15)] = value
		return
	}
	m.variables[64+(number&31)] = value
}

func bdtBankBasis(bank byte) int {
	switch bank {
	case 'X', 'x':
		return 96
	case 'L', 'l':
		return 32
	case 'H', 'h':
		return 48
	case 'T', 't':
		return 64
	default:
		return 0
	}
}

func bdtToUpper(bank byte) byte {
	if bank >= 'a' && bank <= 'z' {
		return bank - ('a' - 'A')
	}
	return bank
}

func (m *BdtMachine) resolveExpressionAtom(bank byte, index int32) int32 {
	switch bdtToLower(bank) {
	case 'c':
		if index >= 0 && index < BdtFlagCap {
			return m.systemFlags[index]
		}
		return 0
	case 'g':
		if index >= 0 && index < BdtFlagCap {
			return m.sceneFlags[index]
		}
		return 0
	case 'x':
		return m.getVariable(96, index&31)
	case 'z':
		return m.getVariable(0, index&31)
	case 'l':
		return m.getVariable(32, index&15)
	case 'h':
		return m.getVariable(48, index&15)
	case 't':
		return m.getVariable(64, index&31)
	default:
		return 0
	}
}

func bdtClampInt32(value, low, high int32) int32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (m *BdtMachine) executeLine(line BdtLine) {
	text := stripBdtCarriage(line.Text)
	switch line.Kind {
	case BdtLineKindEmpty, BdtLineKindComment, BdtLineKindLabel:
		m.pc++
		return
	case BdtLineKindText:
		m.executeQuote(text)
		return
	case BdtLineKindVoice:
		m.executeVoice(text)
		return
	case BdtLineKindDialogue:
		m.executeDialogue(text)
		return
	default:
		m.executeCommand(text)
		return
	}
}

func (m *BdtMachine) executeCommand(text string) {
	if len(text) == 0 {
		m.pc++
		return
	}
	switch text[0] {
	case '$', '#':
		m.pc++
		return
	case '%':
		m.halt()
		return
	case '!':
		m.executeBang(text)
		return
	case '"', '@':
		m.executeQuote(text)
		return
	case '?':
		m.executeQuestion(text)
		return
	case 'G':
		if strings.HasPrefix(text, "GS") {
			m.executeGoSub(text[2:])
		} else {
			m.executeGoto(text[1:])
		}
		return
	case 'R':
		if strings.HasPrefix(text, "RET") {
			m.executeReturn()
		} else if strings.HasPrefix(text, "R1") {
			m.executeReturnWait()
		} else if len(text) == 1 {
			m.executeReturnWait()
		} else if strings.HasPrefix(text, "RA") || strings.HasPrefix(text, "RC") {
			m.pc++
		} else {
			m.unknown(text)
		}
		return
	case 'E':
		m.executeChapterEnd()
		return
	case 'J':
		m.executeJump(text[1:])
		return
	case 'I':
		if len(text) >= 2 && text[1] == 'F' {
			m.executeConditional(text)
		} else if strings.HasPrefix(text, "IN") {
			m.pc++
			m.stop = BdtStopInput
		} else {
			m.unknown(text)
		}
		return
	case 'U':
		m.executeAssign(text[1:])
		return
	case 'Z':
		m.executeSystemFlag(text[1:])
		return
	case 'F':
		if strings.HasPrefix(text, "FC") {
			m.executeSceneFlag(text[2:], 0)
		} else if strings.HasPrefix(text, "FS") {
			m.executeSceneFlag(text[2:], 1)
		} else if strings.HasPrefix(text, "FD") {
			m.pc++
		} else {
			m.unknown(text)
		}
		return
	case 'X':
		m.executeExtended(text)
		return
	case 'W':
		m.executeWait(text)
		return
	case 'M':
		m.executeMusic(text)
		return
	case 'B':
		m.executeWindow(text)
		return
	case 'C':
		m.executeFigure(text)
		return
	case 'T':
		m.executeType(text)
		return
	case 'L':
		m.executeCaption(text)
		return
	case 'V':
		m.executeSeCommand(text)
		return
	case 'D':
		if strings.HasPrefix(text, "DEL") {
			m.calls = m.calls[:0]
			m.pc++
		} else {
			m.unknown(text)
		}
		return
	case 'S':
		if strings.HasPrefix(text, "STOP2") {
			m.pc++
			m.callHost(func() { m.host.Redraw() }, "redraw")
			return
		}
		if strings.HasPrefix(text, "SROL") && m.dialect == BdtDialectManin {
			m.pc++
			if h, ok := m.host.(ManinHost); ok {
				m.callHost(func() { h.StartManinStaffRoll() }, "manin-staffroll")
			}
			m.stop = BdtStopStaffRoll
			return
		}
		if strings.HasPrefix(text, "SROL") {
			m.pc++
			return
		}
		m.executeSelect(text)
		return
	case 'H', 'A', 'Q', '~', '.', '/', '+':
		m.pc++
		return
	default:
		m.unknown(text)
		return
	}
}

func (m *BdtMachine) unknown(text string) {
	core.Logf("bdt: unknown command: %s", text)
	m.pc++
}

func (m *BdtMachine) callHost(action func(), what string) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("bdt: host %s failed: %v", what, r)
		}
	}()
	action()
}

func stripBdtCarriage(text string) string {
	if len(text) == 0 {
		return ""
	}
	if text[len(text)-1] == '\r' {
		return text[:len(text)-1]
	}
	return text
}

func extractBdtQuoted(text string) (string, bool) {
	if len(text) == 0 {
		return "", false
	}
	first := strings.IndexByte(text, '"')
	if first < 0 {
		return "", false
	}
	last := strings.LastIndexByte(text, '"')
	if last <= first {
		return "", false
	}
	return text[first+1 : last], true
}

func stripBdtEdgeQuotes(text string) string {
	if len(text) > 0 && text[0] == '"' {
		text = text[1:]
	}
	if len(text) > 0 && text[len(text)-1] == '"' {
		return text[:len(text)-1]
	}
	return text
}
