package vm

import (
	"math"

	"kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

const (
	BsxFrameCapacity    = 1 << 14
	BsxRegisterCapacity = 1 << 12
	BsxVariableCapacity = 1 << 12
	BsxScalarCapacity   = 1 << 10
	BsxCallCapacity     = 1 << 10
)

const SelectKindValue = -1

const BsChoiceKind = 0

const ReplayReturnAddress = -1

const bsxFrameWorkspace = 96

type BsMachineCore interface {
	Dialect() bs.BsScriptDialect
	Halted() bool
	Faulted() bool
	ProgramCounter() int
	InstructionStart() int
	StackDepth() int
	Choices() []bs.BsChoice
	Fetch() bs.BsCommand
	Choose(index int) bool
	Seek(programCounter int) bool
	Registers() []int32
	Variables() []int32
	Properties() []int32
	Property(index int) int32
	SetProperty(index int, value int32)
	ClearStack()
	Reset()
	Name(index int) string
	Message(index int) string
	Character(index int) string
}

var (
	_ BsMachineCore = (*bs.AncientScriptMachine)(nil)
	_ BsMachineCore = (*bs.RevScriptMachine)(nil)
	_ bs.ScriptFile = (*BsxScriptFile)(nil)
)

type bsxCallFrame struct {
	returnAddress int
	frameBase     int
}

type BsxMachine struct {
	script           *BsxScriptFile
	dialect          ScriptDialect
	code             []byte
	frame            []int32
	registers        []int32
	variables        []int32
	properties       []int32
	scalars          []float64
	calls            []bsxCallFrame
	options          []SelectOption
	ancient          *bs.AncientScriptMachine
	rev              *bs.RevScriptMachine
	selectOptions    []SelectOption
	command          bs.BsCommand
	pc               int
	frameBase        int
	callDepth        int
	instructionStart int
	labelArgument    int
	selectArgument   int
	deferred         bool
	deferredIsCall   bool
	deferredTarget   int
	bootJumps        int
	bsSnapshot       *bs.BsMachineState
	revSnapshot      *bs.RevMachineState
	halted           bool
	faulted          bool
	RestartBoot      bool
}

func NewBsxMachine(script *BsxScriptFile) *BsxMachine {
	m := &BsxMachine{
		script:      script,
		dialect:     script.Dialect(),
		frame:       make([]int32, BsxFrameCapacity),
		registers:   make([]int32, BsxRegisterCapacity),
		variables:   make([]int32, BsxVariableCapacity),
		properties:  make([]int32, BsxVariableCapacity),
		scalars:     make([]float64, BsxScalarCapacity),
		calls:       make([]bsxCallFrame, BsxCallCapacity),
		RestartBoot: true,
	}
	m.code = append([]byte(nil), script.Code()...)
	if dialect, ok := script.BsDialect(); ok {
		if bs.RevIsRev(dialect) {
			m.rev = bs.NewRevScriptMachine(script, dialect, m.code)
		} else {
			m.ancient = bs.NewAncientScriptMachine(script, dialect, m.code)
		}
		boot := script.FindLabel("boot")
		if boot < 0 || !m.SeekBs(boot) {
			m.pc = 0
		}
	}
	return m
}

func (m *BsxMachine) Script() *BsxScriptFile {
	return m.script
}

func (m *BsxMachine) ProgramCounter() int {
	return m.pc
}

func (m *BsxMachine) FrameBase() int {
	return m.frameBase
}

func (m *BsxMachine) Halted() bool {
	return m.halted
}

func (m *BsxMachine) Faulted() bool {
	return m.faulted
}

func (m *BsxMachine) IsBsScript() bool {
	return m.ancient != nil || m.rev != nil
}

func (m *BsxMachine) BsScript() BsMachineCore {
	if m.ancient != nil {
		return m.ancient
	}
	if m.rev != nil {
		return m.rev
	}
	return nil
}

func (m *BsxMachine) Command() bs.BsCommand {
	return m.command
}

func (m *BsxMachine) InstructionStart() int {
	return m.instructionStart
}

func (m *BsxMachine) Code() []byte {
	return m.code
}

func (m *BsxMachine) LabelArgument() int {
	return m.labelArgument
}

func (m *BsxMachine) SelectArgument() int {
	return m.selectArgument
}

func (m *BsxMachine) SelectOptions() []SelectOption {
	return m.selectOptions
}

func (m *BsxMachine) Registers() []int32 {
	if m.ancient != nil {
		return m.ancient.Registers()
	}
	if m.rev != nil {
		return m.rev.Registers()
	}
	return m.registers
}

func (m *BsxMachine) Variables() []int32 {
	if m.ancient != nil {
		return m.ancient.Variables()
	}
	if m.rev != nil {
		return m.rev.Variables()
	}
	return m.variables
}

func (m *BsxMachine) Properties() []int32 {
	if m.ancient != nil {
		return m.ancient.Properties()
	}
	if m.rev != nil {
		return m.rev.Properties()
	}
	return m.properties
}

func (m *BsxMachine) Scalars() []float64 {
	return m.scalars
}

func (m *BsxMachine) Property(index int) int32 {
	if m.ancient != nil {
		return m.ancient.Property(index)
	}
	if m.rev != nil {
		return m.rev.Property(index)
	}
	if index >= 0 && index < len(m.properties) {
		return m.properties[index]
	}
	return -1
}

func (m *BsxMachine) SetProperty(index int, value int32) {
	if m.ancient != nil {
		m.ancient.SetProperty(index, value)
		return
	}
	if m.rev != nil {
		m.rev.SetProperty(index, value)
		return
	}
	if index >= 0 && index < len(m.properties) {
		m.properties[index] = value
	}
}

func (m *BsxMachine) Jump(target int) {
	if target < 0 || target >= len(m.code) {
		core.Logf("script: FAULT jump out of range %d", target)
		m.faulted = true
		m.halted = true
		return
	}
	m.pc = target
}

func (m *BsxMachine) Fetch() ScriptCommandId {
	if m.ancient != nil {
		return m.fetchBsScript()
	}
	if m.rev != nil {
		return m.fetchRevScript()
	}
	if m.halted || m.faulted {
		return Halt
	}
	for {
		if m.halted {
			return Halt
		}
		if m.pc < 0 || m.pc >= len(m.code) {
			m.halted = true
			return Halt
		}
		code := m.code
		m.instructionStart = m.pc
		rawOpcode := code[m.pc]
		opcode := m.dialect.Translate(rawOpcode)
		switch opcode {
		case OpcodeHalt:
			boot := m.script.FindLabel("boot")
			bootJumps := m.bootJumps
			m.bootJumps++
			if m.RestartBoot && boot >= 0 && boot != m.pc && bootJumps < 8 {
				m.pc = boot
				continue
			}
			m.halted = true
			return Halt
		case OpcodeReturnOne:
			m.pc++
			return ReturnOne
		case OpcodeReturnTwo:
			return ReturnTwo
		case OpcodeGotoLabel:
			// 0x03 的操作数是"标签序号"，不是代码偏移。BSXScript 3.x 的每个子程序都用
			// `goto <labelId>` 收尾（opening 末尾 `goto 34` 跳到 main，main 末尾
			// `goto 34` 指向自身）。把序号当偏移用会让 PC 落进 init_variable 的前几条
			// 指令，实测表现为"序章播完又回序章"的死循环。序号越界说明脚本已损坏：
			// 直接 fault，而不是让 PC 继续乱跑。
			labelID := m.readCodeInt32(m.pc + 1)
			target := m.script.GetLabelOffset(int(labelID))
			if target < 0 {
				core.Logf("script: FAULT goto unknown label id %d at %d", labelID, m.pc)
				m.faulted = true
				m.halted = true
				return Halt
			}
			m.Jump(target)
			continue
		case OpcodeJump:
			m.Jump(int(m.readCodeInt32(m.pc + 1)))
			continue
		case OpcodeJumpWhenTrue:
			if m.registers[0] != 0 {
				m.Jump(int(m.readCodeInt32(m.pc + 1)))
			} else {
				m.pc += OpcodeFixedArgumentSize
			}
			continue
		case OpcodeJumpWhenFalse:
			if m.registers[0] != 0 {
				m.pc += OpcodeFixedArgumentSize
			} else {
				m.Jump(int(m.readCodeInt32(m.pc + 1)))
			}
			continue
		case OpcodeDeferLabel, OpcodeDeferLabelCall:
			m.deferred = true
			m.deferredIsCall = opcode == OpcodeDeferLabelCall
			m.deferredTarget = m.script.GetLabelOffset(int(m.readCodeInt32(m.pc + 1)))
			m.pc += OpcodeFixedArgumentSize
			continue
		case OpcodeCall:
			m.call(m.pc+OpcodeFixedArgumentSize, int(m.readCodeInt32(m.pc+1)))
			continue
		case OpcodeReturn:
			m.returnFromCall()
			continue
		case OpcodeResetFrame:
			m.pc++
			m.frameBase = 0
			continue
		case OpcodeMove:
			m.executeMove()
			continue
		case OpcodeNegate:
			m.executeNegate()
			continue
		case OpcodeIncrement, OpcodeDecrement:
			m.executeAdjust(opcode == OpcodeIncrement)
			continue
		case OpcodeComplement:
			m.executeComplement()
			continue
		case OpcodeDisplayMessage:
			m.advanceMessage()
			return DisplayMessage
		case OpcodeFetch:
			m.executeFetch()
			continue
		case OpcodeSelectArgument:
			if m.dialect.SelectTakesArgument() {
				m.selectArgument = int(m.readCodeInt32(m.pc + 1))
			}
			m.selectOptions = append(m.selectOptions[:0], m.options...)
			m.options = m.options[:0]
			size, err := m.dialect.SizeOf(code, m.instructionStart)
			if err != nil {
				m.faultWith(err)
				return Halt
			}
			m.pc += size
			return Select
		case OpcodeSelectCommand:
			m.pc++
			return SelectNext
		case OpcodeSelectOption:
			size, err := m.dialect.SizeOf(code, m.instructionStart)
			if err != nil {
				m.faultWith(err)
				return Halt
			}
			m.pc += size
			return SelectEntry
		case OpcodeSelectCommandTail:
			m.pc++
			return SelectTail
		case OpcodeDefineLabel:
			m.labelArgument = int(m.readCodeInt32(m.pc + 1))
			if m.deferred && m.deferredTarget == m.pc {
				m.deferred = false
			}
			m.pc += OpcodeFixedArgumentSize
			return DefineLabel
		case OpcodeLabelCommand:
			m.pc++
			return Character
		case OpcodeInvoke:
			m.pc += OpcodeFixedArgumentSize + (4 * int(m.readCodeInt32(m.pc+1)))
			return Invoke
		case OpcodeAttach:
			m.pc++
			return Attach
		case OpcodeEvaluate:
			m.pc++
			return Evaluate
		case OpcodeTranslate:
			m.pc++
			return Translate
		default:
			if (opcode >= OpcodeGreater && opcode <= OpcodeModulo) ||
				(opcode >= OpcodeExclusiveOr && opcode <= OpcodeShiftRight) {
				m.executeBinary(opcode)
				continue
			}
			if opcode >= OpcodeFirstCommand && opcode <= OpcodeLastCommand {
				size, err := m.dialect.SizeOf(code, m.instructionStart)
				if err != nil {
					m.faultWith(err)
					return Halt
				}
				m.pc += size
				return ScriptCommandId(opcode)
			}
			if !m.dialect.IsModern() {
				core.Logf("script: skipped %d opcode 0x%02X at %d", int(m.dialect.Kind()), rawOpcode, m.pc)
				size, err := m.dialect.SizeOf(code, m.instructionStart)
				if err != nil {
					m.faultWith(err)
					return Halt
				}
				m.pc += size
				continue
			}
			// 未知操作码在旧一代剧本里不一定是致命伤：UK（BSXScript 2.0）
			// 的全码扫描还剩十几处反编译器无法归类的双字节/多字节序列，
			// 全部落在剧情中段，与地图/主流程无关。按 1 字节跳过让脚本
			// 继续走到下一条可识别指令； fault 会让宿主直接弹出"播放结束"，
			// 玩家再也无法从坏点恢复。真正的结构性损坏由代码区长度校验
			// （ParseBsxScriptFile）和标签越界检查兜住。
			core.Logf("script: skipped unknown opcode 0x%02X at %d", rawOpcode, m.pc)
			m.pc++
			continue
		}
	}
}

func (m *BsxMachine) faultWith(err error) {
	core.Logf("script: FAULT %s", err.Error())
	m.faulted = true
	m.halted = true
}

func (m *BsxMachine) fetchBsScript() ScriptCommandId {
	ancient := m.ancient
	if ancient.Halted() || ancient.Faulted() {
		m.syncBsScript(ancient)
		return Halt
	}
	command := ancient.Fetch()
	m.command = command
	m.syncBsScript(ancient)
	if command.Kind == bs.BsCommandKindSelect {
		m.publishBsChoices(ancient)
	}
	return mapBsCommand(command.Kind)
}

func (m *BsxMachine) syncBsScript(ancient *bs.AncientScriptMachine) {
	m.pc = ancient.ProgramCounter()
	m.instructionStart = ancient.InstructionStart()
	m.halted = ancient.Halted()
	m.faulted = ancient.Faulted()
}

func (m *BsxMachine) fetchRevScript() ScriptCommandId {
	rev := m.rev
	if rev.Halted() || rev.Faulted() {
		m.syncRevScript(rev)
		return Halt
	}
	command := rev.Fetch()
	m.command = command
	m.syncRevScript(rev)
	if command.Kind == bs.BsCommandKindSelect {
		m.publishRevChoices(rev)
	}
	return mapBsCommand(command.Kind)
}

func (m *BsxMachine) syncRevScript(rev *bs.RevScriptMachine) {
	m.pc = rev.ProgramCounter()
	m.instructionStart = rev.InstructionStart()
	m.halted = rev.Halted()
	m.faulted = rev.Faulted()
}

func (m *BsxMachine) publishRevChoices(rev *bs.RevScriptMachine) {
	m.options = m.options[:0]
	m.selectOptions = m.selectOptions[:0]
	for _, choice := range rev.Choices() {
		m.selectOptions = append(m.selectOptions, SelectOption{Index: choice.Index, MessageId: int(choice.MessageId), LabelId: -1, Value: choice.Index, Kind: BsChoiceKind})
	}
}

func (m *BsxMachine) publishBsChoices(ancient *bs.AncientScriptMachine) {
	m.options = m.options[:0]
	m.selectOptions = m.selectOptions[:0]
	for _, choice := range ancient.Choices() {
		m.selectOptions = append(m.selectOptions, SelectOption{Index: choice.Index, MessageId: int(choice.MessageId), LabelId: -1, Value: 0, Kind: BsChoiceKind})
	}
}

func mapBsCommand(kind bs.BsCommandKind) ScriptCommandId {
	switch kind {
	case bs.BsCommandKindMessage:
		return DisplayMessage
	case bs.BsCommandKindImage:
		return Image
	case bs.BsCommandKindWindow:
		return WindowVisibility
	case bs.BsCommandKindClearScreen:
		return ClearScreen
	case bs.BsCommandKindTransition:
		return Transition
	case bs.BsCommandKindPlayBgm:
		return PlayBgm
	case bs.BsCommandKindStopBgm:
		return StopBgm
	case bs.BsCommandKindPlaySound:
		return PlayEffect
	case bs.BsCommandKindSelect:
		return Select
	case bs.BsCommandKindWait:
		return Wait
	case bs.BsCommandKindSystemCall:
		return SystemCall
	case bs.BsCommandKindDialog:
		return Dialog
	case bs.BsCommandKindRestart:
		return Restart
	case bs.BsCommandKindLayerControl:
		return BsLayerControl
	case bs.BsCommandKindLayerImage:
		return BsLayerImage
	case bs.BsCommandKindLayerRename:
		return BsLayerRename
	case bs.BsCommandKindStopSound:
		return BsStopSound
	case bs.BsCommandKindHalt, bs.BsCommandKindQuit:
		return Halt
	case bs.BsCommandKindNone:
		return None
	default:
		core.Logf("script: unmapped BSScript command %d", int(kind))
		return None
	}
}

func (m *BsxMachine) CaptureBsState() (bs.BsMachineState, bool) {
	if m.ancient != nil {
		return m.ancient.Capture(), true
	}
	if m.rev != nil {
		return m.rev.Capture().ToBsMachineState(), true
	}
	return bs.BsMachineState{}, false
}

func (m *BsxMachine) CaptureRevState() (bs.RevMachineState, bool) {
	if m.rev != nil {
		return m.rev.Capture(), true
	}
	return bs.RevMachineState{}, false
}

func (m *BsxMachine) RestoreBsState(state bs.BsMachineState) bool {
	if m.ancient != nil {
		m.ancient.Restore(state)
		m.syncBsScript(m.ancient)
		return true
	}
	if m.rev != nil {
		m.rev.Restore(bs.RevMachineStateFromBs(state))
		m.syncRevScript(m.rev)
		return true
	}
	return false
}

func (m *BsxMachine) RestoreRevState(state bs.RevMachineState) bool {
	if m.rev == nil {
		return false
	}
	m.rev.Restore(state)
	m.syncRevScript(m.rev)
	return true
}

func (m *BsxMachine) SeekBs(programCounter int) bool {
	if m.ancient != nil {
		if !m.ancient.Seek(programCounter) {
			return false
		}
		m.syncBsScript(m.ancient)
		return true
	}
	if m.rev != nil {
		if !m.rev.Seek(programCounter) {
			return false
		}
		m.syncRevScript(m.rev)
		return true
	}
	return false
}

func (m *BsxMachine) SeekBsLabel(name string) bool {
	offset := m.script.FindLabel(name)
	return offset >= 0 && m.SeekBs(offset)
}

// SeekLabel 跳到任意方言的剧本标签。
//
// SeekBsLabel 只实现 BSScript（Rev/Ancient）的 Seek；BSXScript 3.x 的现代
// 机器没有 LabelSeek，SLG 的 `state training` 收尾和 NextTraining 会失败。
// 现代机器直接改 PC 即可：目标标签是自己包含 RESET_FRAME 的独立入口。
func (m *BsxMachine) SeekLabel(name string) bool {
	offset := m.script.FindLabel(name)
	if offset < 0 {
		return false
	}
	if m.ancient != nil || m.rev != nil {
		return m.SeekBs(offset)
	}
	if m.faulted {
		return false
	}
	m.halted = false
	m.Jump(offset)
	return true
}

func (m *BsxMachine) Frame(slot int) int32 {
	index := m.frameBase + slot
	if index >= 0 && index < len(m.frame) {
		return m.frame[index]
	}
	return 0
}

// SetFrame 写 frame 槽。SLG 的宿主命令把返回值放在 frame[0]，脚本紧接着
// 用 NE/JMP_F 判断；没有这个入口，命令只能"执行但结果丢失"。
func (m *BsxMachine) SetFrame(slot int, value int32) {
	index := m.frameBase + slot
	if index >= 0 && index < len(m.frame) {
		m.frame[index] = value
	}
}

func (m *BsxMachine) Operand(offset int) int32 {
	return m.readCodeInt32(m.instructionStart + offset)
}

func (m *BsxMachine) OperandWord(offset int) int16 {
	position := m.instructionStart + offset
	if position < 0 || position+1 >= len(m.code) {
		return 0
	}
	return int16(m.code[position]) | (int16(m.code[position+1]) << 8)
}

func (m *BsxMachine) ResolveMessage(index int) string {
	return m.script.GetMessage(index)
}

func (m *BsxMachine) ResolveCharacterName(index int) string {
	return m.script.GetCharacterName(index)
}

func (m *BsxMachine) ReadInlineString(offset int) string {
	index := m.Operand(offset)
	if index >= 0 {
		return m.script.GetMessage(int(index))
	}
	return ""
}

func (m *BsxMachine) ApplyDeferredJump() {
	if !m.deferred {
		return
	}
	m.deferred = false
	m.Jump(m.deferredTarget)
}

func (m *BsxMachine) HasDeferredJump() bool {
	return m.deferred
}

func (m *BsxMachine) DeferredIsCall() bool {
	return m.deferredIsCall
}

func EntryState(programCounter int) MachineState {
	return MachineState{
		ProgramCounter:      programCounter,
		FrameBase:           0,
		CallDepth:           1,
		Frame:               make([]int32, bsxFrameWorkspace),
		Registers:           make([]int32, BsxRegisterCapacity),
		Variables:           make([]int32, BsxVariableCapacity),
		Properties:          make([]int32, BsxVariableCapacity),
		Scalars:             make([]float64, BsxScalarCapacity),
		CallReturnAddresses: []int32{ReplayReturnAddress},
		CallFrameBases:      []int32{0},
	}
}

func (m *BsxMachine) CaptureState() MachineState {
	if m.ancient != nil {
		snapshot := m.ancient.Capture()
		m.bsSnapshot = &snapshot
		return MachineState{
			ProgramCounter:      snapshot.ProgramCounter,
			FrameBase:           0,
			CallDepth:           m.ancient.StackDepth(),
			Frame:               []int32{},
			Registers:           append([]int32{}, snapshot.Registers...),
			Variables:           append([]int32{}, snapshot.Variables...),
			Properties:          append([]int32{}, snapshot.Properties...),
			Scalars:             []float64{},
			CallReturnAddresses: []int32{},
			CallFrameBases:      []int32{},
			Halted:              snapshot.Halted,
		}
	}
	if m.rev != nil {
		snapshot := m.rev.Capture()
		m.revSnapshot = &snapshot
		return MachineState{
			ProgramCounter:      snapshot.ProgramCounter,
			FrameBase:           m.rev.StackDepth(),
			CallDepth:           m.rev.StackDepth() / 2,
			Frame:               []int32{},
			Registers:           append([]int32{}, snapshot.Flags...),
			Variables:           append([]int32{}, snapshot.Variables...),
			Properties:          append([]int32{}, snapshot.Properties...),
			Scalars:             []float64{},
			CallReturnAddresses: []int32{},
			CallFrameBases:      []int32{},
			Halted:              snapshot.Halted,
		}
	}
	base := m.frameBase
	if base < 0 {
		base = 0
	}
	frameLength := len(m.frame)
	if want := base + bsxFrameWorkspace; want < frameLength {
		frameLength = want
	}
	if frameLength < 0 {
		frameLength = 0
	}
	frame := make([]int32, frameLength)
	copy(frame, m.frame)
	returnAddresses := make([]int32, m.callDepth)
	frameBases := make([]int32, m.callDepth)
	for i := 0; i < m.callDepth; i++ {
		returnAddresses[i] = int32(m.calls[i].returnAddress)
		frameBases[i] = int32(m.calls[i].frameBase)
	}
	return MachineState{
		ProgramCounter:      m.pc,
		FrameBase:           m.frameBase,
		CallDepth:           m.callDepth,
		Frame:               frame,
		Registers:           append([]int32(nil), m.registers...),
		Variables:           append([]int32(nil), m.variables...),
		Properties:          append([]int32(nil), m.properties...),
		Scalars:             append([]float64(nil), m.scalars...),
		CallReturnAddresses: returnAddresses,
		CallFrameBases:      frameBases,
		Deferred:            m.deferred,
		DeferredIsCall:      m.deferredIsCall,
		DeferredTarget:      m.deferredTarget,
		Halted:              m.halted,
	}
}

func (m *BsxMachine) RestoreState(state MachineState) {
	if m.ancient != nil {
		m.restoreBsState(state)
		return
	}
	if m.rev != nil {
		m.restoreRevState(state)
		return
	}
	frameLength := len(m.frame)
	if len(state.Frame) < frameLength {
		frameLength = len(state.Frame)
	}
	for i := range m.frame {
		m.frame[i] = 0
	}
	copy(m.frame, state.Frame[:frameLength])
	for i := range m.registers {
		m.registers[i] = 0
	}
	copy(m.registers, state.Registers)
	for i := range m.variables {
		m.variables[i] = 0
	}
	copy(m.variables, state.Variables)
	for i := range m.properties {
		m.properties[i] = 0
	}
	copy(m.properties, state.Properties)
	for i := range m.scalars {
		m.scalars[i] = 0
	}
	copy(m.scalars, state.Scalars)
	m.callDepth = len(m.calls)
	if state.CallDepth < m.callDepth {
		m.callDepth = state.CallDepth
	}
	if m.callDepth < 0 {
		m.callDepth = 0
	}
	for i := 0; i < m.callDepth; i++ {
		m.calls[i] = bsxCallFrame{returnAddress: int(state.CallReturnAddresses[i]), frameBase: int(state.CallFrameBases[i])}
	}
	m.pc = state.ProgramCounter
	m.frameBase = state.FrameBase
	m.deferred = state.Deferred
	m.deferredIsCall = state.DeferredIsCall
	m.deferredTarget = state.DeferredTarget
	m.bootJumps = 0
	m.halted = state.Halted
	m.faulted = false
}

func (m *BsxMachine) restoreBsState(state MachineState) {
	snapshot := m.bsSnapshot
	m.bsSnapshot = nil
	if snapshot != nil && snapshot.ProgramCounter == state.ProgramCounter {
		m.ancient.Restore(*snapshot)
	} else {
		m.ancient.Restore(bsMachineStateFromMachineState(state, m.ancient.Dialect()))
		core.Logf("script: BSScript state restored from PC %d only", state.ProgramCounter)
	}
	m.syncBsScript(m.ancient)
	m.bootJumps = 0
	m.faulted = false
	m.options = m.options[:0]
	m.selectOptions = m.selectOptions[:0]
}

func (m *BsxMachine) restoreRevState(state MachineState) {
	snapshot := m.revSnapshot
	m.revSnapshot = nil
	if snapshot != nil && snapshot.ProgramCounter == state.ProgramCounter {
		m.rev.Restore(*snapshot)
	} else {
		m.rev.Restore(bs.RevMachineStateFromBs(bsMachineStateFromMachineState(state, m.rev.Dialect())))
		core.Logf("script: BSScript rev state restored from PC %d only", state.ProgramCounter)
	}
	m.syncRevScript(m.rev)
	m.bootJumps = 0
	m.faulted = false
	m.options = m.options[:0]
	m.selectOptions = m.selectOptions[:0]
}

func bsMachineStateFromMachineState(state MachineState, dialect bs.BsScriptDialect) bs.BsMachineState {
	return bs.BsMachineState{
		Dialect:          dialect,
		ProgramCounter:   state.ProgramCounter,
		InstructionStart: state.ProgramCounter,
		StackTop:         0,
		Stack:            []int32{},
		Accumulator:      -1,
		Flag:             0,
		Registers:        state.Registers,
		Properties:       state.Properties,
		Variables:        state.Variables,
		Choices:          []bs.BsChoice{},
		Halted:           state.Halted,
		Faulted:          false,
	}
}

func (m *BsxMachine) executeBinary(opcode byte) {
	destination := m.readCodeInt32(m.pc + 2)
	firstKind := m.readCodeUInt16(m.pc + 6)
	firstIndex := m.readCodeInt32(m.pc + 8)
	secondKind := m.readCodeUInt16(m.pc + 12)
	secondIndex := m.readCodeInt32(m.pc + 14)
	m.pc += OpcodeBinarySize
	left := m.readOperand(int(firstKind), firstIndex)
	right := m.readOperand(int(secondKind), secondIndex)
	var result int32
	switch opcode {
	case OpcodeGreater:
		if left > right {
			result = 1
		}
	case OpcodeLess:
		if left < right {
			result = 1
		}
	case OpcodeGreaterOrEqual:
		if left >= right {
			result = 1
		}
	case OpcodeLessOrEqual:
		if left <= right {
			result = 1
		}
	case OpcodeNotEqual:
		if left != right {
			result = 1
		}
	case OpcodeEqual:
		if left == right {
			result = 1
		}
	case OpcodeOr:
		result = left | right
	case OpcodeAnd:
		result = left & right
	case OpcodeAdd:
		result = left + right
	case OpcodeSubtract:
		result = left - right
	case OpcodeMultiply:
		result = left * right
	case OpcodeDivide:
		if right != 0 {
			result = left / right
		}
	case OpcodeModulo:
		if right != 0 {
			result = left % right
		}
	case OpcodeExclusiveOr:
		result = left ^ right
	case OpcodeShiftLeft:
		result = left << (uint32(right) & 31)
	case OpcodeShiftRight:
		result = left >> (uint32(right) & 31)
	}
	if destination >= 0 && int(destination) < len(m.registers) {
		m.registers[destination] = result
	}
}

func (m *BsxMachine) executeMove() {
	destinationRegister := m.readCodeInt32(m.pc + 2)
	destinationKind := m.readCodeUInt16(m.pc + 6)
	destinationIndex := m.readCodeInt32(m.pc + 8)
	sourceKind := m.readCodeUInt16(m.pc + 12)
	sourceIndex := m.readCodeInt32(m.pc + 14)
	m.pc += OpcodeBinarySize
	value := m.readOperand(int(sourceKind), sourceIndex)
	m.writeOperand(int(destinationKind), destinationIndex, value)
	if destinationRegister >= 0 && int(destinationRegister) < len(m.registers) {
		m.registers[destinationRegister] = value
	}
}

func (m *BsxMachine) executeNegate() {
	destination := m.readCodeInt32(m.pc + 2)
	kind := m.readCodeUInt16(m.pc + 6)
	index := m.readCodeInt32(m.pc + 8)
	m.pc += OpcodeUnarySize
	value := -m.readOperand(int(kind), index)
	if destination >= 0 && int(destination) < len(m.registers) {
		m.registers[destination] = value
	}
}

func (m *BsxMachine) executeAdjust(increment bool) {
	destination := m.readCodeInt32(m.pc + 2)
	kind := m.readCodeUInt16(m.pc + 6)
	index := m.readCodeInt32(m.pc + 8)
	m.pc += OpcodeUnarySize
	var delta int32 = -1
	if increment {
		delta = 1
	}
	if OperandKind(kind) == OperandScalar {
		slot := scalarSlot(index)
		if slot >= 0 && slot < len(m.scalars) {
			m.scalars[slot] += float64(delta)
			if destination >= 0 && int(destination) < len(m.registers) {
				m.registers[destination] = int32(math.Floor(m.scalars[slot]))
			}
		}
		return
	}
	value := m.readOperand(int(kind), index) + delta
	m.writeOperand(int(kind), index, value)
	if destination >= 0 && int(destination) < len(m.registers) {
		m.registers[destination] = value
	}
}

func (m *BsxMachine) executeComplement() {
	destination := m.readCodeInt32(m.pc + 2)
	kind := m.readCodeUInt16(m.pc + 6)
	index := m.readCodeInt32(m.pc + 8)
	m.pc += OpcodeUnarySize
	value := ^m.readOperand(int(kind), index)
	if destination >= 0 && int(destination) < len(m.registers) {
		m.registers[destination] = value
	}
}

func (m *BsxMachine) advanceMessage() {
	size, err := m.dialect.SizeOf(m.code, m.instructionStart)
	if err != nil {
		m.faultWith(err)
		return
	}
	m.pc += size
}

func (m *BsxMachine) executeFetch() {
	m.options = append(m.options, SelectOption{
		Index:     int(m.Frame(2)),
		MessageId: int(m.readCodeInt32(m.pc + 1)),
		LabelId:   int(m.readCodeInt32(m.pc + 5)),
		Value:     int(m.readCodeInt32(m.pc + 9)),
		Kind:      int(m.Frame(1)),
	})
	m.pc += OpcodeFetchSize
}

func (m *BsxMachine) ClearSelectOptions() {
	m.selectOptions = m.selectOptions[:0]
	m.options = m.options[:0]
}

func (m *BsxMachine) ChooseOption(index int) bool {
	if m.ancient != nil {
		m.selectOptions = m.selectOptions[:0]
		m.options = m.options[:0]
		if !m.ancient.Choose(index) {
			return false
		}
		m.syncBsScript(m.ancient)
		return true
	}
	if m.rev != nil {
		m.selectOptions = m.selectOptions[:0]
		m.options = m.options[:0]
		if !m.rev.Choose(index) {
			return false
		}
		m.syncRevScript(m.rev)
		return true
	}
	chosen := -1
	for i := range m.selectOptions {
		if m.selectOptions[i].Index == index {
			chosen = i
			break
		}
	}
	if chosen < 0 && index >= 0 && index < len(m.selectOptions) {
		chosen = index
	}
	if chosen < 0 {
		return false
	}
	option := m.selectOptions[chosen]
	m.selectOptions = m.selectOptions[:0]
	m.options = m.options[:0]
	// 现代 BSXScript 的两选项（鉴赏"全編/本編のみ"、黒の教室 剧情选项）
	// 都是 Kind=-2：FETCH 的第三个字段是"选项值/消息号"，不是标签号。
	// 原引擎选完后把结果写进 #select_value / #select_type，然后从 SELECT
	// 后面继续执行，由剧本自己比较 #select_type 分支。
	//
	// 旧实现对 Kind<0 做"延迟跳转"，跳进 GetLabelOffset(1/2)
	// （init_position / shorts_check 之类的子程序），表现就是选项点了没反应，
	// 或者直接 HALT 回鉴赏室。只有 Kind>=0 才是真正的跳转选项。
	if option.Kind < 0 {
		return true
	}
	target := m.script.GetLabelOffset(option.LabelId)
	if target < 0 {
		return false
	}
	m.Jump(target)
	return true
}

func (m *BsxMachine) readOperand(kind int, index int32) int32 {
	switch OperandKind(kind) {
	case OperandImmediate, OperandImmediateAlt:
		return index
	case OperandVariable:
		if index >= 0 && int(index) < len(m.variables) {
			return m.variables[index]
		}
		return 0
	case OperandProperty:
		if index >= 0 && int(index) < len(m.properties) {
			return m.properties[index]
		}
		return 0
	case OperandFrame:
		return m.Frame(int(index))
	case OperandRegister:
		if index >= 0 && int(index) < len(m.registers) {
			return m.registers[index]
		}
		return 0
	case OperandScalar:
		slot := scalarSlot(index)
		if slot >= 0 && slot < len(m.scalars) {
			return int32(math.Floor(m.scalars[slot]))
		}
		return 0
	default:
		return 0
	}
}

func (m *BsxMachine) writeOperand(kind int, index int32, value int32) {
	switch OperandKind(kind) {
	case OperandVariable:
		if index >= 0 && int(index) < len(m.variables) {
			m.variables[index] = value
		}
	case OperandProperty:
		if index >= 0 && int(index) < len(m.properties) {
			m.properties[index] = value
		}
	case OperandFrame:
		slot := m.frameBase + int(index)
		if slot >= 0 && slot < len(m.frame) {
			m.frame[slot] = value
		}
	case OperandRegister:
		if index >= 0 && int(index) < len(m.registers) {
			m.registers[index] = value
		}
	case OperandScalar:
		slot := scalarSlot(index)
		if slot >= 0 && slot < len(m.scalars) {
			m.scalars[slot] = float64(value)
		}
	}
}

func scalarSlot(index int32) int {
	if index >= 0 && int(index) < BsxScalarCapacity {
		return int(index)
	}
	return -1
}

func (m *BsxMachine) call(returnAddress int, labelId int) {
	target := m.script.GetLabelOffset(labelId)
	if target < 0 {
		m.faulted = true
		m.halted = true
		return
	}
	if m.callDepth < len(m.calls) {
		m.calls[m.callDepth] = bsxCallFrame{returnAddress: returnAddress, frameBase: m.frameBase}
		m.callDepth++
	}
	m.Jump(target)
}

func (m *BsxMachine) returnFromCall() {
	if m.callDepth > 0 {
		m.callDepth--
		m.pc = m.calls[m.callDepth].returnAddress
		return
	}
	boot := m.script.FindLabel("boot")
	bootJumps := m.bootJumps
	m.bootJumps++
	if m.RestartBoot && boot >= 0 && boot != m.pc && bootJumps < 8 {
		core.Logf("script: init finished at %d, enter boot %d", m.pc, boot)
		m.pc = boot
		return
	}
	m.halted = true
}

func (m *BsxMachine) readCodeInt32(position int) int32 {
	if position < 0 || position+4 > len(m.code) {
		return 0
	}
	return int32(m.code[position]) |
		(int32(m.code[position+1]) << 8) |
		(int32(m.code[position+2]) << 16) |
		(int32(m.code[position+3]) << 24)
}

func (m *BsxMachine) readCodeUInt16(position int) uint16 {
	if position < 0 || position+2 > len(m.code) {
		return 0
	}
	return uint16(m.code[position]) | (uint16(m.code[position+1]) << 8)
}
