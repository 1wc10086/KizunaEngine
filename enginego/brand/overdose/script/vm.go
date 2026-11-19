package script

import (
	"errors"
	"fmt"
	"strings"
)

// Opcode 常量（§5.3）。数字放名字里只是防绕回原文的方向指示，
// 真正语义对照在 dispatch 表里；如需反查 Spec 行号看 vm.go 的大表。
const (
	OpEnd            = 0x00
	OpNop            = 0x01
	OpLabelDef       = 0x02
	OpCheckpoint     = 0x03
	OpCondJump       = 0x04
	OpJump           = 0x05
	OpCondInverse    = 0x06
	OpElseMark       = 0x07
	OpReturn         = 0x08
	OpCallScript     = 0x0a
	OpGosubLabel     = 0x0b
	OpTailCall       = 0x0c
	OpWait           = 0x0d
	OpPairPut        = 0x0e
	OpSetBool        = 0x0f
	OpSetInt         = 0x10
	OpSetString      = 0x11
	OpClearVar       = 0x12
	OpWaitInput      = 0x19
	OpShowChoice     = 0x1a
	OpDispatchMul    = 0x1b
	OpAddChoice      = 0x1c
	OpAdvancedChoice = 0x1d
	OpClearMsg       = 0x1e
	OpDialogue       = 0x1f
	OpMsgCtrl        = 0x25
	OpSpriteLayer    = 0x27
	OpLayerToggle    = 0x28
	OpLayerComp      = 0x2c
	OpLayerPlace     = 0x33
	OpAnimA          = 0x34
	OpAnimB          = 0x36
	OpAnimC          = 0x37
	OpAnimD          = 0x38
	OpAnimDur        = 0x3b
	OpWindowOff      = 0x4d
	OpWindowTog0     = 0x50
	OpWindowTog1     = 0x51
	OpBGM            = 0x58
	OpWaitAudioA     = 0x59
	OpSE             = 0x5c
	OpWaitAudioB     = 0x5d
	OpVoice          = 0x64
	OpResetHook      = 0x68
)

// EventKind 列出 VM 只产“事件”不做呈现的分类。Session 按事件驱动 core 状态。
type EventKind string

const (
	EventDialogue EventKind = "dialogue"
	EventChoice   EventKind = "choice"
	EventBGM      EventKind = "bgm"
	EventSE       EventKind = "se"
	EventVoice    EventKind = "voice"
	EventImageSet EventKind = "image-set"
	EventVarSet   EventKind = "var-set"
	EventWait     EventKind = "wait"
	EventFinished EventKind = "finished"
	EventMacro    EventKind = "macro"
	EventLabel    EventKind = "label"
	EventRaw      EventKind = "raw"
	// EventLayerSet/Clear 对应 0x27/0x28/0x34 的附加精灵层。
	EventLayerSet   EventKind = "layer-set"
	EventLayerClear EventKind = "layer-clear"
)

type ChoiceEntry struct {
	Index   int32
	Text    string
	Enabled bool
	// Image/X/Y 只用于 0x1d（adv_select2 这类带坐标的选择肢）。
	// 0x1c 的简单选项 Image 为空，session 按原版公式居中排版。
	Image string
	X     int32
	Y     int32
}

type Event struct {
	Kind      EventKind
	Speaker   string
	Text      string
	File      string
	Channel   int32
	Volume    int32
	Loop      bool
	Fade      int32
	Ms        int32
	WaitMs    int32
	Skippable bool
	Value     string
	Raw       string
	Choices   []ChoiceEntry
	// 0x27 图层参数。
	Layer int
	X     int32
	Y     int32
	Alpha int32
	Scale float64
}

// Loader 按脚本名取 AST。Agent B 的 arc 层可以直接闭包适配，不必另命名。
type Loader func(script string) (*Script, error)

var errScriptNotFound = errors.New("脚本不存在")

// MacroHandler 在 op 0x0a 的 macro 分支上替换默认记事件。
// Why 与 VM 分派分离：宏是纯呈现（§6 声明不解释 macro.asb），
// session 落实宏时也不必改 VM。
type MacroHandler func(name string, args []Value) error

func noopMacro(name string, args []Value) error { return nil }

// MacroHandlerName 是 VM 默认钩子；覆盖后按台生效。
var MacroHandlerName MacroHandler = noopMacro

type frame struct {
	script *Script
	offset int
}

type VM struct {
	loader        Loader
	vars          *VariableStore
	stack         []frame
	events        []Event
	lastErr       error
	done          bool
	macroHook     MacroHandler
	visited       map[string]bool
	choiceOptions []ChoiceEntry
	// pendingChoice 是 op 0x1a 锁住的落变量：下一次 Choose 把选中值写到这里；
	// 不是每次都写（真实脚本把 0 占位也多，写进去会多落 int var 0）。
	pendingChoice int32

	// handler/blocked 是 pump 的“停点”契约：对话与选择必须让宿主先消费，
	// 否则一次 Step 会把整段剧情跑完，宿主只看到最后一帧。
	handler EventHandler
	blocked bool
}

// New 新建 VM。Loader 回操作性 AST；找不到文件时直接报错，
// session 侧据此降级为空操作。
func New(loader Loader) *VM {
	return &VM{
		loader:    loader,
		vars:      &VariableStore{},
		macroHook: MacroHandlerName,
		visited:   make(map[string]bool),
	}
}

func NewWithStore(loader Loader, vs *VariableStore) *VM {
	vm := New(loader)
	if vs != nil {
		vm.vars = vs
	}
	return vm
}

func (vm *VM) Variables() *VariableStore { return vm.vars }

func (vm *VM) Events() []Event {
	out := make([]Event, len(vm.events))
	copy(out, vm.events)
	return out
}

type EventHandler interface {
	HandleEvent(Event) error
}

type EventHandlerFunc func(Event) error

func (f EventHandlerFunc) HandleEvent(e Event) error { return f(e) }

// Step 以预算推进 VM；budget>0 时顶死循环，budget<=0 走默认 4096。
func (vm *VM) Step(budget int, handler EventHandler) (int, error) {
	if vm.done {
		return 0, nil
	}
	if budget <= 0 {
		budget = 4096
	}
	return vm.pump(handler, budget)
}

// Start 从入口脚本开始，不存在则返回 errScriptNotFound。
func (vm *VM) Start(scriptName string) error {
	scr, err := loadScript(vm.loader, scriptName)
	if err != nil {
		return err
	}
	vm.stack = []frame{{script: scr, offset: 0}}
	vm.visited[scriptName] = true
	return nil
}

// StartAt 从指定脚本的 label 处起跑（鉴赏场景回放用），label 为空时等价于 Start。
// 找不到 label 直接报错：从头跑会播错整段场景，宁可 fault，也不要猜偏移。
// 复用同一 VM 与已挂上的 macro hook，只换单帧栈，不重建 VM。
// done 必须在此清零：上一幕演完的完成位会让紧接着的 pump 直接走 finishScript。
func (vm *VM) StartAt(scriptName string, label string) error {
	scr, err := loadScript(vm.loader, scriptName)
	if err != nil {
		return err
	}
	offset := 0
	if label != "" {
		off, ok := scr.labelOffset(label)
		if !ok {
			return fmt.Errorf("vm: 脚本 %q 无 label %q", scriptName, label)
		}
		offset = off
	}
	vm.stack = []frame{{script: scr, offset: offset}}
	vm.done = false
	vm.visited[scriptName] = true
	return nil
}

// Advance 让 session 在等输入时推进一步（即完成当前 wait 的那步）。
func (vm *VM) Advance() error {
	if vm.done {
		return nil
	}
	_, err := vm.pump(nil, 1)
	return err
}

// Choose 将 index 的选项落进 op 0x1a 锁住的变量，并继续运行。
// index 按 0x1c 收集顺序计数；非零目标才真写（0 是长期占位）。
func (vm *VM) Choose(index int) error {
	if len(vm.choiceOptions) == 0 {
		return errors.New("没有可选项在等待")
	}
	if index < 0 || index >= len(vm.choiceOptions) {
		return fmt.Errorf("index %d 超出选项数 %d", index, len(vm.choiceOptions))
	}
	// 0x1a 的实参是目标整数变量下标，本作样本全是 0；不能把 0 当“无目标”。
	// 选项记录里的 Index 是脚本比较用的值（scm3 用 1..8），不是数组位置，
	// 宿主传的是位置，这里必须映射回 Index。
	dst := vm.pendingChoice
	value := vm.choiceOptions[index].Index
	// 不在这里清 choiceOptions：原脚本会在循环里再次 0x1a 复用同一批选项；
	// 清掉会让第二次菜单变成“没有可选项在等待”。新菜单由 addChoice 在
	// 选项下标回卷（<= 首个下标）时重置。
	if dst >= 0 && dst < 100 {
		_ = vm.vars.SetInt(1, uint16(dst), value)
	}
	return vm.Advance()
}

func (vm *VM) SetMacroHook(h MacroHandler) *VM {
	if h != nil {
		vm.macroHook = h
	}
	return vm
}

func (vm *VM) Visited(name string) bool { return vm.visited[name] }

func (vm *VM) Choices() []ChoiceEntry { return cloneChoices(vm.choiceOptions) }

func (vm *VM) Done() bool { return vm.done }

func (vm *VM) pendingChoiceWrite(dst int32) { vm.pendingChoice = dst }

func (vm *VM) pump(handler EventHandler, budget int) (int, error) {
	vm.handler = handler
	defer func() { vm.handler = nil }()
	steps := 0
	for steps < budget && !vm.done {
		ins, err := vm.peek()
		if err != nil {
			vm.done = true
			return steps, err
		}
		if ins == nil {
			// 空帧/尾巴：peek 已当作空返回处理，补一次则 mark done
			vm.returnFrom()
			steps++
			continue
		}
		if err := vm.exec(ins, handler); err != nil {
			vm.done = true
			return steps, err
		}
		steps++
		if vm.blocked {
			vm.blocked = false
			return steps, nil
		}
	}
	return steps, nil
}

func (vm *VM) peek() (*Instruction, error) {
	if len(vm.stack) == 0 {
		return nil, nil
	}
	top := &vm.stack[len(vm.stack)-1]
	if top.script == nil {
		return nil, nil
	}
	ins, ok := top.script.instructionAt(top.offset)
	if !ok {
		return nil, nil
	}
	return ins, nil
}

func (vm *VM) advancePC() {
	if len(vm.stack) == 0 {
		return
	}
	top := &vm.stack[len(vm.stack)-1]
	ins, ok := top.script.instructionAt(top.offset)
	if !ok {
		return
	}
	top.offset += ins.Size
}

func (vm *VM) returnFrom() {
	if len(vm.stack) == 0 {
		vm.done = true
		vm.emit(Event{Kind: EventFinished})
		return
	}
	vm.stack = vm.stack[:len(vm.stack)-1]
	if len(vm.stack) == 0 {
		vm.emit(Event{Kind: EventFinished})
		vm.done = true
	}
}

// maxRecordedEvents 限制调试事件环形缓冲。Events() 只给测试/诊断用，
// session 走 handler 实时消费；无界增长在几十万步的实机游玩里会持续占内存。
const maxRecordedEvents = 512

func (vm *VM) emit(e Event) {
	if len(vm.events) >= maxRecordedEvents {
		copy(vm.events, vm.events[len(vm.events)-maxRecordedEvents+1:])
		vm.events = vm.events[:maxRecordedEvents-1]
	}
	vm.events = append(vm.events, e)
	if vm.handler != nil {
		_ = vm.handler.HandleEvent(e)
	}
	// 空文本的 dialogue 是 0x1e 清屏，不是给玩家看的停点。
	if e.Kind == EventDialogue && (e.Text != "" || e.Speaker != "") {
		vm.blocked = true
	}
	if e.Kind == EventChoice || e.Kind == EventFinished {
		vm.blocked = true
	}
}

func (vm *VM) jumpTo(off int) {
	if len(vm.stack) == 0 {
		vm.done = true
		return
	}
	top := &vm.stack[len(vm.stack)-1]
	top.offset = off
}

func (vm *VM) push(scr *Script, offset int) {
	vm.stack = append(vm.stack, frame{script: scr, offset: offset})
}

// SnapshotCamera 供 save 用，导出 (script, offset, 栈深)。
func (vm *VM) SnapshotStack() []struct {
	Script string
	Offset int
} {
	out := make([]struct {
		Script string
		Offset int
	}, len(vm.stack))
	for i, f := range vm.stack {
		out[i].Script = f.script.name()
		out[i].Offset = f.offset
	}
	return out
}

// Snapshot 导出整 VM 运行态：调用栈、已访集、pending choice 目标、完成位。
// 存档只在对白/选项停点写入（与 blocked 契约同一时点），恢复后 Advance
// 从停点继续单步即可，中间帧全部靠 offset 唯一标识——丢 pending 会让
// Choose 写错变量（0 占位检查依赖它），丢 visited 会让重访判定漂移。
func (vm *VM) Snapshot() VMState {
	st := VMState{Done: vm.done, PendingChoice: vm.pendingChoice}
	for _, f := range vm.stack {
		st.Stack = append(st.Stack, StackFrame{Script: f.script.name(), Offset: f.offset})
	}
	for name := range vm.visited {
		st.Visited = append(st.Visited, name)
	}
	return st
}

// Restore 从 Snapshot 回写：按名重载脚本（loader 必须能按名取回 AST），
// 偏移落地前校验落在指令头上——存档里的字节偏移依赖脚本版本，跨版本
// 写错位就会退化为“从该偏移起静默执行垃圾”，这里宁可拒读也不赌。
func (vm *VM) Restore(st VMState) error {
	vm.stack = vm.stack[:0]
	for _, fr := range st.Stack {
		scr, err := loadScript(vm.loader, fr.Script)
		if err != nil {
			return err
		}
		if _, ok := scr.instructionAt(fr.Offset); !ok {
			return fmt.Errorf("vm: 存档偏移 %d 不在脚本 %q 的指令边界上", fr.Offset, fr.Script)
		}
		vm.stack = append(vm.stack, frame{script: scr, offset: fr.Offset})
	}
	vm.visited = make(map[string]bool, len(st.Visited))
	for _, name := range st.Visited {
		vm.visited[name] = true
	}
	vm.choiceOptions = nil
	vm.pendingChoice = st.PendingChoice
	vm.done = st.Done
	vm.blocked = false
	return nil
}

// VMState 是 VM 存档形态：栈帧只存脚本名+字节偏移（与 0x04/0x05 的 field
// 同口径），便于与 save.FrameState 一一对应映射。
type VMState struct {
	Stack         []StackFrame `json:"stack,omitempty"`
	Visited       []string     `json:"visited,omitempty"`
	PendingChoice int32        `json:"pendingChoice,omitempty"`
	Done          bool         `json:"done,omitempty"`
}

// StackFrame 是一帧调用栈：脚本名 + 流内字节偏移。
type StackFrame struct {
	Script string `json:"script"`
	Offset int    `json:"offset"`
}

func (vm *VM) exec(ins *Instruction, handler EventHandler) error {
	op := ins.Opcode
	field := ins.Field
	switch op {
	case OpEnd:
		vm.advancePC()
		vm.emit(Event{Kind: EventFinished})
		vm.done = true
		return nil
	case OpNop, OpCheckpoint, OpLabelDef, OpElseMark:
		vm.advancePC()
		return nil
	case OpCondJump:
		v, err := EvalValue(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if v.AsBool() {
			vm.advancePC()
		} else {
			vm.jumpTo(int(field))
		}
		return nil
	case OpJump:
		vm.jumpTo(int(field))
		return nil
	case OpCondInverse:
		// 原 handlers 表里 0x06 与 0x04 同入口（见 opcodes_spec §1）：
		// 真则继续、假则跳到 field，不是"条件取反"。全量剧本 0 处出现，
		// 这里照原表实现，避免留下与引擎不一致的假分支。
		v, err := EvalValue(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if v.AsBool() {
			vm.advancePC()
		} else {
			vm.jumpTo(int(field))
		}
		return nil
	case OpReturn:
		vm.returnFrom()
		return nil
	case 0x09, 0x16, 0x17:
		// 真实文件里只各出现过一次，且无 handler 可对：只记 Raw 事件且放行。
		args, _ := EvalArgs(ins.Args, vm.vars)
		vm.emit(Event{Kind: EventRaw, Raw: fmt.Sprintf("%x", op), Text: "uncertain: rare", Value: joinArgs(args)})
		vm.advancePC()
		return nil
	case OpCallScript:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		scriptName, labelName, err := decodeScriptLabel(vals)
		if err != nil {
			return err
		}
		if scriptName == "macro" {
			vm.emit(Event{Kind: EventMacro, Text: labelName, Raw: scriptName})
			if vm.macroHook != nil {
				if err := vm.macroHook(labelName, vals); err != nil {
					return err
				}
			}
			vm.advancePC()
			return nil
		}
		scr, err := loadScript(vm.loader, scriptName)
		if err != nil {
			if errors.Is(err, errScriptNotFound) {
				vm.advancePC()
				return nil
			}
			return err
		}
		offset, found := labelOffset(scr, labelName)
		if !found {
			// 原 0x0a handler 在 0x4309eb 查不到 label 时直接返回，不压帧。
			// Why 不能回落到偏移 0：那会把整个剧本从头再播一遍（HCG/场景
			// 无限重复的直接原因），必须把"标签缺失"当成空调用。
			vm.advancePC()
			return nil
		}
		// 调用是“先让当前帧进到下一条，再把子帧压栈”：
		// 必须在压栈前推进父帧，否则 advancePC 推进的是子帧，跳过子函数首条。
		vm.advancePC()
		vm.push(scr, offset)
		vm.visited[scriptName] = true
		return nil
	case OpGosubLabel:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if len(vals) == 0 {
			vm.advancePC()
			return nil
		}
		name := vals[0].AsString()
		if len(vm.stack) == 0 {
			return errors.New("空帧栈 gosub")
		}
		top := &vm.stack[len(vm.stack)-1]
		off, ok := top.script.labelOffset(name)
		if !ok {
			vm.advancePC()
			return nil
		}
		// 0x0b 在原引擎里是“跳到同脚本 label”的纯跳转（opcodes_spec §2：
		// 0x4301c2 只写 task+0x28/0x30，不压帧）。压帧会把 `gosub end`
		// 之后的 0x08 变成"返回调用点"，于是剧本回到菜单/开场无限循环。
		// 只有 0x0a 才是压帧调用；0x0b 的 label 由 0x08 结束整个任务或
		// 返回上一层 0x0a。
		vm.jumpTo(off)
		return nil
	case OpTailCall:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		scriptName, labelName, err := decodeScriptLabel(vals)
		if err != nil {
			return err
		}
		scr, err := loadScript(vm.loader, scriptName)
		if err != nil {
			if errors.Is(err, errScriptNotFound) {
				vm.advancePC()
				return nil
			}
			return err
		}
		offset, found := labelOffset(scr, labelName)
		if !found {
			vm.advancePC()
			return nil
		}
		// 0x0c 是"切换脚本"而非调用：原 handler 只把 script/label 写进全局
		// 待换名缓冲，由任务循环下一拍加载；用当前帧顶替即可等价实现，
		// 且绝不能 push（否则旧脚本永远留在栈底）。
		if len(vm.stack) == 0 {
			vm.push(scr, offset)
		} else {
			top := &vm.stack[len(vm.stack)-1]
			top.script = scr
			top.offset = offset
		}
		vm.visited[scriptName] = true
		return nil
	case OpWait:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		ms, skip := firstIntAndBool(vals)
		vm.emit(Event{Kind: EventWait, WaitMs: ms, Ms: ms, Skippable: skip})
		vm.advancePC()
		return nil
	case OpPairPut:
		ev := newEvaluator(vm.vars)
		if err := ev.run(ins.Args); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpSetBool:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.applySetBool(vals); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpSetInt:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.applySetInt(vals); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpSetString:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.applySetString(vals, ins); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpClearVar:
		// 0x12：一个 var-ref 清零。原样本 (4:(2,0))——字符串槽清零即 ""。
		for _, a := range ins.Args {
			if a.Type == NodeVarRef {
				kind, _ := kindOf(a.Namespace)
				switch kind {
				case VarBool:
					_ = vm.vars.SetBool(a.Namespace, a.Index, false)
				case VarInt:
					_ = vm.vars.SetInt(a.Namespace, a.Index, 0)
				default:
					_ = vm.vars.SetString(a.Namespace, a.Index, "")
				}
			}
		}
		vm.advancePC()
		return nil
	case OpWaitInput:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		waitClick := false
		if len(vals) >= 1 {
			waitClick = vals[0].AsBool()
		}
		vm.emit(Event{Kind: EventWait, Skippable: waitClick})
		vm.advancePC()
		return nil
	case OpShowChoice:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		dst := int32(0)
		if len(vals) >= 1 {
			dst = vals[0].AsInt()
		}
		vm.pendingChoiceWrite(dst)
		vm.emit(Event{Kind: EventChoice, Choices: cloneChoices(vm.choiceOptions)})
		vm.advancePC()
		return nil
	case OpDispatchMul:
		// (1,-1,1,-1…) 这种拍脑袋内部参数：按原文注释不做跳，
		// 避免把 0/1 值班当成真分支跑偏；session 若后续能逐字核对再回写。
		vm.emit(Event{Kind: EventRaw, Raw: "0x1b", Text: "uncertain: ambiguous dispatch"})
		vm.advancePC()
		return nil
	case OpAddChoice:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.addChoice(vals); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpAdvancedChoice:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.addAdvancedChoice(vals); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpClearMsg:
		vm.emit(Event{Kind: EventDialogue, Text: ""})
		vm.advancePC()
		return nil
	case OpDialogue:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		speaker, body := parseDialogue(vals)
		vm.emit(Event{Kind: EventDialogue, Speaker: speaker, Text: body})
		vm.advancePC()
		return nil
	case OpSpriteLayer:
		// 0x27：idx, image, x, y, depth, alpha%, scale/10, time/1000。
		// 参数必须按“位置”取，不能走 EvalArgs：真实脚本里有 NodeNeg(17)
		// 作为负号标记（如 x=660, neg, 0），按表达式求值会把 x 变成 -660。
		layer, image, geom, ok := vm.layerArgs(ins.Args)
		if !ok {
			vm.emit(Event{Kind: EventRaw, Raw: "0x27", Text: "layer args short"})
			vm.advancePC()
			return nil
		}
		x, y, alpha, scale := int32(0), int32(0), int32(255), 1.0
		if len(geom) > 0 {
			x = geom[0]
		}
		if len(geom) > 1 {
			y = geom[1]
		}
		if len(geom) > 3 {
			alpha = geom[3] * 255 / 100
		}
		if len(geom) > 4 {
			if v := float64(geom[4]) / 10; v > 0 {
				scale = v
			}
		}
		vm.emit(Event{
			Kind: EventLayerSet, Layer: layer, File: image,
			X: x, Y: -y, Alpha: alpha, Scale: scale,
		})
		vm.advancePC()
		return nil
	case OpMsgCtrl, OpLayerComp, OpLayerPlace:
		vals, _ := EvalArgs(ins.Args, vm.vars)
		vm.emit(Event{Kind: EventRaw, Raw: fmt.Sprintf("%x", op), Text: "presentation", Value: joinArgs(vals)})
		vm.advancePC()
		return nil
	case OpLayerToggle:
		vals, _ := EvalArgs(ins.Args, vm.vars)
		v := int32(-1)
		if len(vals) >= 1 {
			v = vals[0].AsInt()
		}
		vm.emit(Event{Kind: EventLayerClear, Layer: int(v)})
		vm.advancePC()
		return nil
	case OpAnimA:
		// 0x34 在宏里是“清层/重建”；直接出现时同样按清层处理。
		vals, _ := EvalArgs(ins.Args, vm.vars)
		layer := -1
		if len(vals) >= 1 {
			layer = int(vals[0].AsInt())
		}
		vm.emit(Event{Kind: EventLayerClear, Layer: layer})
		vm.advancePC()
		return nil
	case OpAnimB, OpAnimC, OpAnimD, OpAnimDur:
		vm.advancePC()
		return nil
	case OpWindowOff, OpWindowTog0, OpWindowTog1, OpResetHook:
		vm.advancePC()
		return nil
	case OpBGM, OpSE, OpVoice:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		if err := vm.audioCmd(op, vals); err != nil {
			return err
		}
		vm.advancePC()
		return nil
	case OpWaitAudioA, OpWaitAudioB:
		vals, err := EvalArgs(ins.Args, vm.vars)
		if err != nil {
			return err
		}
		_ = vals // (chan, ms, skip) 形态：chan 阈值在此不单独建 wait；统按 ms 记
		ms, skip := waitShape(vals)
		vm.emit(Event{Kind: EventWait, WaitMs: ms, Ms: ms, Skippable: skip})
		vm.advancePC()
		return nil
	default:
		vals, _ := EvalArgs(ins.Args, vm.vars)
		vm.emit(Event{Kind: EventRaw, Raw: fmt.Sprintf("op 0x%x @ %#x argc %d", op, ins.Offset, ins.Argc), Text: "unknown", Value: joinArgs(vals)})
		vm.advancePC()
		return nil
	}
}

func loadScript(loader Loader, name string) (*Script, error) {
	if loader == nil {
		return nil, errors.New("VM 无 loader")
	}
	scr, err := loader(name)
	if err != nil {
		return nil, err
	}
	if scr == nil {
		return nil, errScriptNotFound
	}
	return scr, nil
}

// labelOffset 返回标签偏移与是否存在。存在性必须单独返回：
// 0x0a 查不到 label 时原引擎是"空调用"，把 0 当合法偏移会重播整个剧本。
func labelOffset(scr *Script, label string) (int, bool) {
	if label == "" {
		return 0, true
	}
	return scr.labelOffset(label)
}

func decodeScriptLabel(vals []Value) (script, label string, err error) {
	if len(vals) < 1 {
		return "", "", errors.New("call 需要 script 参")
	}
	script = vals[0].AsString()
	if len(vals) >= 2 {
		label = vals[1].AsString()
	}
	return script, label, nil
}

func firstIntAndBool(vals []Value) (int32, bool) {
	ms := int32(0)
	skip := false
	if len(vals) >= 1 {
		ms = vals[0].AsInt()
	}
	if len(vals) >= 2 {
		skip = vals[1].AsBool()
	}
	return ms, skip
}

func waitShape(vals []Value) (int32, bool) {
	// (chan, ms, skip)：第一位是 chan 名，第二位是毫秒。
	if len(vals) < 2 {
		return 0, false
	}
	ms := vals[1].AsInt()
	skip := false
	if len(vals) >= 3 {
		skip = vals[2].AsBool()
	}
	return ms, skip
}

func (vm *VM) audioCmd(op uint16, vals []Value) error {
	chanID := int32(0)
	file := ""
	loop := false
	vol := int32(0)
	fade := int32(0)
	if len(vals) >= 1 {
		chanID = vals[0].AsInt()
	}
	if len(vals) >= 2 {
		file = vals[1].AsString()
	}
	if len(vals) >= 3 {
		loop = vals[2].AsBool()
	}
	if len(vals) >= 4 {
		vol = vals[3].AsInt()
	}
	if len(vals) >= 5 {
		fade = vals[4].AsInt()
	}
	ev := Event{Raw: fmt.Sprintf("%x", op), Channel: chanID, File: file, Loop: loop, Volume: vol, Fade: fade}
	switch op {
	case OpBGM:
		ev.Kind = EventBGM
	case OpSE:
		ev.Kind = EventSE
	case OpVoice:
		ev.Kind = EventVoice
	default:
		ev.Kind = EventRaw
	}
	vm.emit(ev)
	return nil
}

func (vm *VM) applySetBool(vals []Value) error {
	if len(vals) < 3 {
		return fmt.Errorf("0x0f 需 3 参（实 %d）", len(vals))
	}
	idx := vals[0].AsInt()
	value := vals[1].AsBool()
	scope := vals[2].AsBool()
	ns := uint16(0)
	if scope {
		ns = 3
	}
	if err := vm.vars.SetBool(ns, uint16(idx), value); err != nil {
		return err
	}
	vm.emit(Event{Kind: EventVarSet, Value: fmt.Sprintf("bool:%d=%t:%d", idx, value, ns)})
	return nil
}

func (vm *VM) applySetInt(vals []Value) error {
	if len(vals) < 3 {
		return fmt.Errorf("0x10 需 3 参（实 %d）", len(vals))
	}
	idx := vals[0].AsInt()
	value := vals[1].AsInt()
	scope := vals[2].AsBool()
	ns := uint16(1)
	if scope {
		ns = 4
	}
	if err := vm.vars.SetInt(ns, uint16(idx), value); err != nil {
		return err
	}
	vm.emit(Event{Kind: EventVarSet, Value: fmt.Sprintf("int:%d=%d:%d", idx, value, ns)})
	return nil
}

func (vm *VM) applySetString(vals []Value, ins *Instruction) error {
	if len(vals) < 3 {
		return fmt.Errorf("0x11 需 3 参（实 %d）", len(vals))
	}
	idx := vals[0].AsInt()
	text := vals[1].AsString()
	scope := vals[2].AsBool()
	ns := uint16(2)
	if scope {
		ns = 5
	}
	if err := vm.vars.SetString(ns, uint16(idx), text); err != nil {
		return err
	}
	vm.emit(Event{Kind: EventVarSet, Value: fmt.Sprintf("str:%d=%s:%d", idx, text, ns)})
	// global string var 0=背景、1=人物 的 ImageSet 分开事件给 session
	if ns == 2 && (idx == 0 || idx == 1) {
		vm.emit(Event{Kind: EventImageSet, Value: text})
	}
	return nil
}

// layerArgs 按 0x27/0x33 的位置语义拆参数：
// 第 0 项是层号、第 1 项是图名（字符串或字符串变量），其余按顺序是
// int 几何。NodeNeg(17) 是负号标记，作用于紧随其后的 int，不参与表达式求值。
func (vm *VM) layerArgs(args []ArgNode) (idx int, image string, geom []int32, ok bool) {
	type slot struct {
		isStr bool
		str   string
		num   int32
		neg   bool
	}
	var slots []slot
	for _, arg := range args {
		switch arg.Type {
		case NodeEnd:
			continue
		case NodeNeg:
			slots = append(slots, slot{neg: true})
		case NodeString:
			slots = append(slots, slot{isStr: true, str: arg.Str})
		case NodeVarRef:
			str, _ := vm.vars.GetString(arg.Namespace, arg.Index)
			slots = append(slots, slot{isStr: true, str: str})
		case NodeInt:
			slots = append(slots, slot{num: arg.Int})
		}
	}
	if len(slots) < 2 || slots[0].isStr || !slots[1].isStr {
		return 0, "", nil, false
	}
	idx = int(slots[0].num)
	image = slots[1].str
	neg := false
	for _, s := range slots[2:] {
		if s.neg {
			neg = !neg
			continue
		}
		if s.isStr {
			continue
		}
		v := s.num
		if neg {
			v = -v
			neg = false
		}
		geom = append(geom, v)
	}
	return idx, image, geom, true
}

func (vm *VM) addChoice(vals []Value) error {
	idx := int32(0)
	text := ""
	enabled := true
	if len(vals) >= 1 {
		idx = vals[0].AsInt()
	}
	if len(vals) >= 2 {
		text = vals[1].AsString()
	}
	if len(vals) >= 3 {
		enabled = vals[2].AsBool()
	}
	return vm.appendChoice(ChoiceEntry{Index: idx, Text: text, Enabled: enabled})
}

// addAdvancedChoice 是 0x1d：INT(0)=值、STR(1)=按钮图（adv_select2）、
// INT(2/3)=屏幕坐标、STR(4)=显示文本、BOOL(5)=enabled。STR(1) 不是可翻译
// 文本，而是原版 CreateSurfaceButton 的底图名，必须原样保留。
func (vm *VM) addAdvancedChoice(vals []Value) error {
	if len(vals) < 6 {
		return fmt.Errorf("0x1d 参数不足: %d", len(vals))
	}
	return vm.appendChoice(ChoiceEntry{
		Index:   vals[0].AsInt(),
		Image:   vals[1].AsString(),
		X:       vals[2].AsInt(),
		Y:       vals[3].AsInt(),
		Text:    vals[4].AsString(),
		Enabled: vals[5].AsBool(),
	})
}

// appendChoice 统一处理“新菜单第一项下标回卷”的重置语义：
// 0x1c/0x1d 都会在上一批选项结束后从较小下标重新开始。
func (vm *VM) appendChoice(entry ChoiceEntry) error {
	if len(vm.choiceOptions) > 0 && entry.Index <= vm.choiceOptions[0].Index {
		vm.choiceOptions = nil
	}
	vm.choiceOptions = append(vm.choiceOptions, entry)
	return nil
}

func joinArgs(vals []Value) string {
	var b strings.Builder
	for i, v := range vals {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(v.Display())
	}
	return b.String()
}

func parseDialogue(vals []Value) (speaker, body string) {
	if len(vals) == 0 {
		return "", ""
	}
	if len(vals) == 1 {
		return "", vals[0].AsString()
	}
	return vals[0].AsString(), vals[1].AsString()
}

func cloneChoices(in []ChoiceEntry) []ChoiceEntry {
	out := make([]ChoiceEntry, len(in))
	copy(out, in)
	return out
}
