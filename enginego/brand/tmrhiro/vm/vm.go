// Package vm 是 TMR-Hiro 的行寻址 VM。
//
// 状态机只有三步：执行、等点击、等选择。每条记录执行完就推进 PC；
// 文本记录把正文交给上层后停在等点击，选择记录把选项交给上层后停在等选择。
//
// 跳转全部按“标签名”解析：脚本里没有字节偏移，只有 `01 02 <名字>` 这类
// 记录；加载时先把标签扫成索引表（script.Labels），运行时按名查。
// 换脚本用调用栈：`01 02 <script>` 压栈并切入新脚本，`01 01 <label>`
// 是同脚本跳转；脚本执行到 `op1/0x30` 就弹栈，栈空即结束。
package vm

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"kizuna/enginego/brand/tmrhiro/script"
)

// Step 是 VM 的动作结果。
type Step int

const (
	StepContinue Step = iota
	StepAwaitInput
	StepAwaitChoice
	StepAwaitTitle
	StepFinished
)

// Choice 是一个选项。
type Choice struct {
	Text  string
	Label string
}

// Audio 是一条音频请求。
type Audio struct {
	Kind    int // 0=BGM 1=SE 2=语音 3=停 BGM 4=停语音 5=停 SE
	Name    string
	Volume  float64
	Loop    bool
	Channel uint32
}

// ImageOp 是一条图像请求。
type ImageOp struct {
	Kind  int // 0=载图(背景/立绘) 1=清立绘 2=立绘组 3=全屏CG 4=淡色 5=滤镜 6=导航
	Name  string
	Slot  int
	X, Y  int
	Scale float64
	Alpha int
	Color string
	Names []string
}

// TitleButton 是地图/菜单里的一个按钮。
type TitleButton struct {
	Label string // 点击后跳到的标签
	X, Y  int
	W, H  int
}

// Loader 按名取脚本字节。
type Loader func(name string) ([]byte, error)

// VM 是执行器。
type VM struct {
	loader Loader

	current *script.Script
	pc      int
	stack   []frame // 调用栈：返回点
	// entry 是起始脚本名；Start 用它。
	entry string
	// vars 是脚本变量（$glo/$tmp/$flg 之类，键是原样名字）。
	vars map[string]int32
	// strVars 是字符串变量（本作的 ＄α 主角名），与数值表分开存。
	strVars map[string]string
	// seen 是已读文本指纹（“跳过未读”用）。
	seen map[string]bool
	// finished 表示栈空且当前脚本执行完。
	finished bool

	// 待上层展示的内容。
	step    Step
	speaker string
	text    string
	voice   string
	choices []Choice
	audio   []Audio
	images  []ImageOp
	// waitMs 是“等待 N 毫秒”类记录（op5/0x82）；0 表示没有等待。
	waitMs int
	// title 与 chapter 来自 op5/0x51、0x15。
	title     string
	chapter   string
	chapterID int
	// hostRequest 是要求宿主执行的界面动作（config/title/gallery/exit）。
	hostRequest string
	// mapMenu 表示正在“地图/菜单定义”区间：0x92 给底图、0x23 给按钮，
	// 紧随其后的 0x10 跳转只是默认项，不执行，等玩家点按钮。
	mapMenu      bool
	titleBase    string
	titleBaseOn  string
	titleButtons []TitleButton
	// screen 是脚本定义的当前界面（标题/鉴赏/立绘/选择/确认）。与 mapMenu 并存的
	// 原因见 screen.go：0x82 开界面、0x92 给底图、0x23/0x43 给按钮、op4/0x64 给
	// 网格单元，这一整套数字是原版界面的唯一来源，必须原样保留。
	screen *Screen
	// charaView 表示脚本进入了立绘鉴赏器（op4/0xe3）。它与 screen 互斥：
	// 0xe3 之后紧跟的那条 0x10 是"戻る"的目标，不能在它之后立刻执行，
	// 否则玩家点 CHARACTER VIEWER 只会闪一下又回到 EXTRA MENU。
	charaView bool
	// percentReq 是 op5/0x30 请求的达成率（-1 表示没有请求）。
	percentReq int32
	// screenFilled 表示当前界面自上次停下后又收集到了内容；脚本页面的定义段
	// 以下一个标签收尾，靠它决定在哪里停（见 execFlow 的 0x0000）。
	screenFilled bool
	// stopFlag 表示脚本要求回到标题（op1/0x82 的模态等待里由上层决定）。
	lastErr string
}

type frame struct {
	script *script.Script
	pc     int
}

// New 建 VM。
func New(loader Loader, entry string) *VM {
	return &VM{
		loader:  loader,
		entry:   entry,
		vars:    make(map[string]int32),
		strVars: make(map[string]string),
		seen:    make(map[string]bool),
	}
}

// Current 返回当前脚本。
func (v *VM) Current() *script.Script { return v.current }

// PC 返回当前记录号（存档用）。
func (v *VM) PC() int { return v.pc }

// Step 返回当前停点。
func (v *VM) Step() Step { return v.step }

// Speaker 返回当前说话人。
func (v *VM) Speaker() string { return v.speaker }

// Text 返回当前正文。
func (v *VM) Text() string { return v.text }

// Title 返回脚本标题。
func (v *VM) Title() string { return v.title }

// Chapter 返回章节名。
func (v *VM) Chapter() string { return v.chapter }

// ChapterID 返回章节号。
func (v *VM) ChapterID() int { return v.chapterID }

// HostRequest 返回宿主动作请求。
func (v *VM) HostRequest() string { return v.hostRequest }

// WaitMs 返回本步的等待时长。
func (v *VM) WaitMs() int { return v.waitMs }

// TitleBase 返回地图底图与悬停图的名字。
func (v *VM) TitleBase() (string, string) { return v.titleBase, v.titleBaseOn }

// TitleButtons 返回当前地图按钮（只在标题态有效）。
func (v *VM) TitleButtons() []TitleButton {
	if v.step != StepAwaitTitle {
		return nil
	}
	return append([]TitleButton(nil), v.titleButtons...)
}

// TitleSelect 处理一个按钮点击：跳到它的标签并继续执行。
//
// 这里同时服务两种点击：标题菜单（START/LOAD/OMAKE/…）与脚本界面自己的按钮
// （界面按钮的标签同样是"跳到某页"）。区别只在调用方：session 已经按"当前
// 是不是标题态"分了层，VM 这边行为一致。
// SelectLabel 是从"脚本界面"跳转的容错版本：它不要求 VM 当前停在
// StepAwaitTitle，而是把 PC 直接指向标签再跑。界面点击的目标标签就是脚本
// 里的普通标签，停点差异（AwaitTitle / AwaitInput / Continue）不该让点击失效，
// 否则就会出现"数据閲覧里点格子没反应"。
func (v *VM) SelectLabel(label string) Step {
	if v.current == nil || label == "" {
		return v.step
	}
	// 先按标签跳；标签不在本脚本时按换脚本调用处理（view06_04_02 这类片段）。
	if target := v.current.Label(label); target >= 0 {
		v.pc = target
		v.step = StepContinue
		v.screenReset()
		v.screenFilled = false
		v.mapMenu = false
		v.charaView = false
		return v.Run(0)
	}
	v.stack = append(v.stack, frame{script: v.current, pc: v.pc})
	if err := v.enter(label, 0); err != nil {
		v.pop()
		return v.step
	}
	v.step = StepContinue
	v.screenReset()
	v.screenFilled = false
	v.charaView = false
	return v.Run(0)
}

func (v *VM) TitleSelect(label string) Step {
	// 停止点接受 StepAwaitTitle（标题/脚本界面）与 StepAwaitInput（鉴赏页
	// 收尾时停在等输入）：两者都是"等玩家点某个格子/按钮"。
	if (v.step != StepAwaitTitle && v.step != StepAwaitInput) || v.current == nil {
		return v.step
	}
	target := v.current.Label(label)
	if target < 0 {
		return v.step
	}
	// 记录跳转目标：同脚本跳转必须先清界面状态，否则新的定义段会被
	// "上一页还 filled"判定顶掉（见 execFlow 的 0x0000）。
	v.pc = target
	v.step = StepContinue
	// 清掉上一张界面的按钮/单元，但保留底图：鉴赏的每一页只有"0xc3 + 0x64"，
	// 没有新的 0x82/0x92；底图由 0xc3 覆盖（见 applyPageGraphic）。
	// 标题的底图也会被 0xc3 覆盖，因此不会串页。
	v.screenReset()
	v.titleButtons = nil
	v.mapMenu = false
	return v.Run(0)
}

// Vars 返回变量副本（存档用）。
func (v *VM) Vars() map[string]int32 {
	out := make(map[string]int32, len(v.vars))
	for k, value := range v.vars {
		out[k] = value
	}
	return out
}

// SetVars 覆盖变量（读档用）。
func (v *VM) SetVars(vars map[string]int32) {
	v.vars = make(map[string]int32, len(vars))
	for k, value := range vars {
		v.vars[k] = value
	}
}

// Seen 返回已读指纹（存档用）。
func (v *VM) Seen() []string {
	out := make([]string, 0, len(v.seen))
	for key := range v.seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// SetSeen 覆盖已读指纹。
func (v *VM) SetSeen(keys []string) {
	v.seen = make(map[string]bool, len(keys))
	for _, key := range keys {
		v.seen[key] = true
	}
}

// MarkRead 标记一句已读。
func (v *VM) MarkRead(speaker, text string) { v.seen[readKey(speaker, text)] = true }

// IsRead 报告一句是否已读。
func (v *VM) IsRead(speaker, text string) bool { return v.seen[readKey(speaker, text)] }

func readKey(speaker, text string) string { return speaker + "\x00" + text }

// Drain 取走本步产生的音频/图像请求（取走即清空）。
func (v *VM) DrainAudio() []Audio {
	out := v.audio
	v.audio = nil
	return out
}

// DrainImages 取走本步产生的图像请求。
func (v *VM) DrainImages() []ImageOp {
	out := v.images
	v.images = nil
	return out
}

// Choices 返回当前选项。
func (v *VM) Choices() []Choice {
	if v.step != StepAwaitChoice {
		return nil
	}
	return append([]Choice(nil), v.choices...)
}

// LastError 返回最近错误。
func (v *VM) LastError() string { return v.lastErr }

// Start 从入口脚本的第一条记录开始。入口脚本由 session 决定（通常是 "start"）。
func (v *VM) Start(entry string) error {
	v.stack = nil
	v.finished = false
	v.step = StepContinue
	// 换脚本/从头开始必须丢掉上一张界面：start 的 TITLE2 段开过标题界面，
	// 进正篇后 vm.screen 若还留着，session 会把它当"当前界面"继续画，
	// 玩家看到的仍是标题底图（点 START 像没反应）。
	v.screen = nil
	v.mapMenu = false
	v.titleBase, v.titleBaseOn = "", ""
	v.titleButtons = nil
	v.clearStep()
	name := entry
	if name == "" {
		name = v.entry
	}
	return v.enter(name, 0)
}

// StartAt 从某个脚本的指定标签开始（标题按钮跳转用）。
func (v *VM) StartAt(scriptName, label string) error {
	if err := v.Start(scriptName); err != nil {
		return err
	}
	if label == "" || v.current == nil {
		return nil
	}
	target := v.current.Label(label)
	if target < 0 {
		return fmt.Errorf("tmrhiro/vm: %s 里没有标签 %q", scriptName, label)
	}
	v.pc = target
	return nil
}

func (v *VM) enter(name string, pc int) error {
	s, err := v.load(name)
	if err != nil {
		v.lastErr = err.Error()
		return err
	}
	v.current = s
	v.pc = pc
	v.step = StepContinue
	return nil
}

func (v *VM) load(name string) (*script.Script, error) {
	if v.loader == nil {
		return nil, fmt.Errorf("tmrhiro/vm: 没有脚本加载器")
	}
	raw, err := v.loader(name)
	if err != nil {
		return nil, fmt.Errorf("tmrhiro/vm: 取脚本 %q 失败: %w", name, err)
	}
	return script.Parse(name, raw)
}

// Run 连续执行，直到停点/结束/预算耗尽。
func (v *VM) Run(budget int) Step {
	if v.finished {
		return StepFinished
	}
	if budget <= 0 {
		budget = 4096
	}
	for i := 0; i < budget; i++ {
		if v.step != StepContinue {
			return v.step
		}
		if v.current == nil {
			v.finished = true
			v.step = StepFinished
			return v.step
		}
		if v.pc < 0 || v.pc >= len(v.current.Records) {
			// 越界视为脚本结束：原版读到末尾同样会返回调用者。
			if v.pop() {
				continue
			}
			v.finish()
			return v.step
		}
		rec := v.current.Records[v.pc]
		v.pc++
		v.exec(&rec)
	}
	return v.step
}

// Advance 在等点击时继续下一条记录。
func (v *VM) Advance() Step {
	if v.finished {
		return StepFinished
	}
	v.clearStep()
	v.step = StepContinue
	return v.Run(0)
}

// Choose 处理选项点击。index 是选项下标。
func (v *VM) Choose(index int) Step {
	if v.step != StepAwaitChoice || index < 0 || index >= len(v.choices) {
		return v.step
	}
	label := v.choices[index].Label
	v.choices = nil
	v.clearStep()
	v.step = StepContinue
	if v.current != nil {
		if target := v.current.Label(label); target >= 0 {
			v.pc = target
			return v.Run(0)
		}
	}
	// 选项目标不在本脚本里：当作换脚本调用。
	if err := v.enter(label, 0); err != nil {
		v.step = StepFinished
		return v.step
	}
	return v.Run(0)
}

func (v *VM) clearStep() {
	v.speaker = ""
	v.text = ""
	v.voice = ""
	v.choices = nil
	v.waitMs = 0
}

func (v *VM) finish() {
	v.finished = true
	v.step = StepFinished
}

func (v *VM) pop() bool {
	if len(v.stack) == 0 {
		return false
	}
	top := v.stack[len(v.stack)-1]
	v.stack = v.stack[:len(v.stack)-1]
	v.current = top.script
	v.pc = top.pc
	return true
}

// exec 执行一条记录。
func (v *VM) exec(r *script.Record) {
	switch r.Op {
	case script.OpText:
		v.execText(r)
	case script.OpFlow:
		v.execFlow(r)
	case script.OpImage:
		v.execImage(r)
	case script.OpAudio:
		v.execAudio(r)
	case script.OpSystem:
		v.execSystem(r)
	case script.OpMisc:
		v.execMisc(r)
	}
}

// execText 处理正文。
//
// 四种形态：
//
//	0x00 旁白      正文
//	0x20 有人名    名字,正文
//	0x30 人名+语音 名字,正文,语音资源名
//	0x60 心声      名字,正文  （显示时加括号，原版是同一套对话框）
//
// 记录 0x05/0x41/0x50 是标记（“本脚本第几章”一类），不改变显示，直接跳过。
func (v *VM) execText(r *script.Record) {
	switch r.Arg {
	case 0x0000:
		v.speaker = ""
		v.text = v.ReplaceVars(r.Text())
		v.step = StepAwaitInput
	case 0x0002, 0x0020:
		fields := script.SplitText(r.Raw, 2)
		if len(fields) >= 2 {
			v.speaker, v.text = v.ReplaceVars(strings.TrimSpace(fields[0])), v.ReplaceVars(fields[1])
		} else {
			v.text = v.ReplaceVars(r.Text())
		}
		v.step = StepAwaitInput
	case 0x0030:
		fields := script.SplitText(r.Raw, 3)
		if len(fields) >= 3 {
			v.speaker = v.ReplaceVars(strings.TrimSpace(fields[0]))
			v.text = v.ReplaceVars(fields[1])
			v.voice = strings.TrimSpace(fields[2])
		} else if len(fields) == 2 {
			// 名字里带剧本变量：`＄α,...` 的主角名由 op5/0x41 在姓名输入后写进
			// vars["＄α"]（见 ReplaceVars）。以前这里只 TrimSpace，对话框上就
			// 直接显示字面量「＄α」，玩家看到的主角名永远是错的。
			v.speaker = v.ReplaceVars(strings.TrimSpace(fields[0]))
			v.text = v.ReplaceVars(fields[1])
		} else {
			v.text = v.ReplaceVars(r.Text())
		}
		if v.voice != "" {
			v.audio = append(v.audio, Audio{Kind: 2, Name: v.voice, Volume: 1, Channel: 2})
		}
		v.step = StepAwaitInput
	case 0x0060:
		fields := script.SplitText(r.Raw, 2)
		if len(fields) >= 2 {
			v.speaker, v.text = v.ReplaceVars(strings.TrimSpace(fields[0])), "（"+v.ReplaceVars(fields[1])+"）"
		} else {
			v.text = v.ReplaceVars(r.Text())
		}
		v.step = StepAwaitInput
	default:
		// 0x05/0x41/0x50 等标记：记录一下，不改变停点。
	}
}

// execFlow 处理控制流。
func (v *VM) execFlow(r *script.Record) {
	switch r.Arg {
	case 0x0000: // 标签定义
		// 脚本界面的"定义段"以一条标签结束：view 脚本里 MENU 的按钮列完之后
		// 紧跟的就是下一个页面的标签（PERCENT100），原版在这一刻停下等玩家
		// 点击，而不是顺着标签继续执行（否则 MENU 会被 PERCENT100 的底图
		// 顶掉——玩家一进データ閲覧就看到"100%にする"的确认框）。
		if v.screen != nil && v.screenFilled {
			v.mapMenu = false
			v.screenFilled = false
			v.step = StepAwaitTitle
			// 默认项（点击空白/取消）取本段之后第一条 0x10 的标签；
			// 屏幕自己的返回按钮不依赖它，这里只是兜底。
			v.screen.Default = v.screenDefaultAhead()
			return
		}
		// 没有新内容就按普通标签继续（例如鉴赏页之间的 MENU_RESET）。
		// 这里不能停：0x0000 同时出现在"界面定义段收尾"和"普通段落"两处。
		if name := string(r.Raw); name == "end" {
			// 本脚本结束：弹栈回调用者，栈空则结束。
			if !v.pop() {
				v.finish()
			}
		}
	case 0x0010, 0x0020: // 跳转 / 调用
		// 参数是 `mode flag name`：flag 1 = 同脚本跳到标签，flag 2 = 调用脚本
		// （压栈后返回）。本作两种都出现在同一 arg 下（`1/0x10 01 02 00_00`
		// 是“进正篇”，`1/0x10 01 01 TOP` 是“回标题顶部”），所以按 flag 分派，
		// 不能按 arg 分派。
		v.execJump(r)
	case 0x0030: // 脚本结束标记
		if !v.pop() {
			v.finish()
		}
	case 0x0040: // 条件跳转
		v.execConditional(r)
	case 0x0041: // 选项菜单
		v.execChoice(r)
	case 0x0082:
		// 地图/菜单开始（模态）。每条 0x082 开启一张新地图：清掉上一张的
		// 底图与按钮，等后续 0x92（底图）+ 0x23（按钮）重新定义。
		//
		// 注意：view 脚本在“刮数据”页（PERCENT100）里也会再开一张地图，
		// 此时**不能**把玩家从当前地图弹回标题态；停成标题态只在同一张
		// 地图定义收尾（那条 0x10）时发生。
		v.mapMenu = true
		v.beginScreen()
	case 0x0092:
		// 地图底图定义：`00 00 <n> 00 <base>,<hover>`，前 4 字节是层数与
		// 面板编号；名字从偏移 4 开始（按最后一个逗号拆底图与悬停图）。
		v.setScreenBase(r)
		v.mapMenu = true
		// 标题底图只由 title3 这一张决定：本作的标题图名固定是 title3，
		// 鉴赏/立绘页另有自己的底图（menu/cgmodeNN）。以前这里把任何一张
		// 界面底图都写成标题底图，进 OMAKE 后标题层会画成"鉴赏的底图"。
		if v.screen != nil && strings.HasPrefix(v.screen.Base, "title") {
			v.titleBase, v.titleBaseOn = v.screen.Base, v.screen.BaseOn
		}
	case 0x0023:
		// 地图按钮：`u16 x, u16 y, u16 w, u16 h, name`。
		v.addScreenButton(r)
		if button, ok := parseButton(r.Raw); ok {
			v.titleButtons = append(v.titleButtons, button)
		}
	case 0x0043:
		// 立绘一览按钮（`<x> <y> <w> <h> ... ,,<label>`）：与 0x23 同构，
		// 只是多一层立绘选择参数，这里同样收成按钮。
		v.addCharaButton(r)
		if button, ok := parseGalleryButton(r.Raw); ok {
			v.titleButtons = append(v.titleButtons, button)
		}
	case 0x00b2:
		// 立绘选择界面：`<flag> <count> "<base>,<empty>,<button>"`。
		// stage 用这三张图画出"空位 + 按钮"的立绘选择页。
		v.addCharaBase(r)
	default:
		// 其余（0x23 地图、0x29/0x2b/0x43 按钮、0xb2 切场景）交给 session 处理。
		v.screenCommand(r)
	}
}

// execJump 处理 `op1/0x10` 与 `op1/0x20` 的跳转/调用。
//
// 参数 `mode flag name`：
//   - flag 1：本脚本内跳到标签（TOP/LOAD/END 等按钮都走它）；
//   - flag 2：调用另一个脚本（压栈，脚本结束时弹栈返回）。
//
// 名字里带逗号的是“宏+参数”写法（`effect,effect scene_end`），取逗号后的部分。
func (v *VM) execJump(r *script.Record) {
	payload := r.Raw
	if len(payload) < 3 {
		return
	}
	mode := payload[0]
	flag := payload[1]
	name := strings.TrimSpace(script.DecodeText(payload[2:]))
	if comma := strings.Index(name, ","); comma >= 0 {
		name = strings.TrimSpace(name[comma+1:])
	}
	if name == "" {
		return
	}
	// 进入正篇是 `01 02 00_00`（mode=1 flag=2 名字=00_00），回标题顶部是
	// `01 01 TOP`（mode=1 flag=1），所以分派只看 flag。mode 非 0 时原版
	// 还会带“返回标签”等附加字段，本作没有用到，忽略。
	_ = mode
	if (v.mapMenu || v.screenFilled) && flag == 1 && v.step == StepContinue {
		// 界面定义区间的最后一条跳转是“默认项”，不执行：
		// 停成界面态等玩家点按钮，点击后由 TitleSelect/ScreenSelect 跳到目标标签。
		// 这条标签同时是界面的“取消/返回”（点击空白处或按 ESC 时用）。
		//
		// 注意不能只看 mapMenu：鉴赏的每一页（CGMODE01..）都是"若干 0xc3/0x64
		// + 这条 0x10"的独立段落，页面之间没有新的 0x82，只能靠 screenFilled
		// 判定"这一段是界面定义"，否则 0x10 会被执行成"跳回本页开头"。
		v.setScreenDefault(name)
		v.mapMenu = false
		v.screenFilled = false
		v.step = StepAwaitTitle
		return
	}
	if flag == 1 {
		if v.current != nil {
			if target := v.current.Label(name); target >= 0 {
				// 同脚本跳转 = 切页。不能在这里把界面清空：下一个页面的定义段
				// 只有 0xc3 + 0x64（没有 0x82），清空会让新页面失去底图。
				// 页面内容由后续记录覆盖（见 setScreenGraphic / addScreenItem）。
				v.pc = target
				return
			}
		}
		// 标签不在本脚本：当作调用处理，比静默跳过更容易发现脚本错误。
	}
	v.stack = append(v.stack, frame{script: v.current, pc: v.pc})
	if err := v.enter(name, 0); err != nil {
		v.pop()
	}
}

func (v *VM) screenCommand(r *script.Record) {
	v.images = append(v.images, ImageOp{Kind: 10, Name: fmt.Sprintf("flow:0x%x", r.Arg), Color: string(r.Raw)})
}

// execConditional 处理条件跳转。
//
// 参数形态（8 字节前缀 + 标签名）：
//
//	u16 mode, u16 index, u16 expect, u16 flag, 标签名
//
// 本作的用法是“路线旗标已置位则跳过整段剧情”，例如
// `00 04 00 01 | 00 00 00 01 | OPENING`：mode=0x0400（“读档/二周目”），
// expect=1。变量名取 `$gloNN` 的编号（index 的低字节）。
// 判不出变量时不跳：宁可让玩家多看一段可选剧情，也不要卡死主线。
func (v *VM) execConditional(r *script.Record) {
	payload := r.Raw
	if len(payload) <= 8 {
		return
	}
	mode := int(payload[0]) | int(payload[1])<<8
	index := int(payload[2]) | int(payload[3])<<8
	expect := int(payload[4]) | int(payload[5])<<8
	label := script.DecodeText(payload[8:])
	if label == "" {
		return
	}
	key := conditionalKey(mode, index)
	if key == "" {
		return
	}
	if v.vars[key] != int32(expect>>8) {
		return
	}
	if target := v.current.Label(label); target >= 0 {
		v.pc = target
	}
}

// conditionalKey 把条件跳转的参数映射到变量名。
//
// 只对“$gloNN 已置位”这一种最常见语义建映射：index 的高字节是组号、
// 低字节是组内编号，与脚本里的 $glo111 这类命名一致。识别不了就返回空串，
// 由调用方选择不跳。
func conditionalKey(mode, index int) string {
	if mode>>8 == 0x04 {
		// “通关旗标”组：$glo100 系列。
		return fmt.Sprintf("$glo%d", 100+index>>8)
	}
	if mode>>8 == 0x02 {
		return fmt.Sprintf("$glo%d", index>>8)
	}
	return ""
}

// execChoice 处理选项。
//
// 形态：`00 00 <count> <sep> <label?> <text1>,<label1> <text2>,<label2> …`
// 其中 count 是选项数，条目之间用空格分隔，每条是“文本,跳转标签”。
func (v *VM) execChoice(r *script.Record) {
	payload := r.Raw
	if len(payload) < 4 {
		return
	}
	count := int(payload[2])
	if count > 8 {
		count = 8
	}
	body := script.DecodeText(payload[4:])
	items := splitChoiceItems(body)
	v.choices = v.choices[:0]
	for _, item := range items {
		parts := strings.SplitN(item, ",", 2)
		if len(parts) < 2 {
			continue
		}
		v.choices = append(v.choices, Choice{
			Text:  strings.TrimSpace(parts[0]),
			Label: strings.TrimSpace(parts[1]),
		})
	}
	if len(v.choices) > count {
		v.choices = v.choices[:count]
	}
	if len(v.choices) == 0 {
		return
	}
	v.step = StepAwaitChoice
}

// splitChoiceItems 拆选项条目。
//
// 条目的形态是 `d<文本>,<标签>`：`d`（0x64）是原版的“条目起始”标记，
// 条目之间以空格分隔。文本本身可能含空格，所以以“空格 + d”为界；
// 拆出条目后要把开头的 d 去掉，否则菜单上会显示成 “dスキップしない”。
func splitChoiceItems(body string) []string {
	out := make([]string, 0, 4)
	start := 0
	flush := func(end int) {
		item := strings.TrimSpace(body[start:end])
		item = strings.TrimPrefix(item, "d")
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	for i := 0; i < len(body); i++ {
		if body[i] != ' ' {
			continue
		}
		if i+1 < len(body) && body[i+1] == 'd' {
			flush(i)
			start = i + 1
		}
	}
	flush(len(body))
	return out
}

// execImage 处理图像。
func (v *VM) execImage(r *script.Record) {
	switch r.Arg {
	case 0x0000:
		// `01 00 <name>`：载图（背景或立绘部件）。
		payload := r.Raw
		if len(payload) < 2 {
			return
		}
		name := strings.TrimSpace(script.DecodeText(payload[2:]))
		if name == "" {
			return
		}
		if strings.HasPrefix(name, "bg") || strings.HasPrefix(name, "BG") {
			v.images = append(v.images, ImageOp{Kind: 0, Name: name})
			return
		}
		// fm_ 宏名是立绘；title2 这类是整屏底图。
		v.images = append(v.images, ImageOp{Kind: 0, Name: name})
	case 0x0010, 0x0080, 0x0091:
		// 背景位移：`01 00 01 <name>`；本作只用于淡入同一张图。
		fields := script.SplitRaw(r.Raw, 2)
		if len(fields) >= 2 {
			v.images = append(v.images, ImageOp{Kind: 0, Name: strings.TrimSpace(fields[1])})
		}
	case 0x008C:
		v.images = append(v.images, ImageOp{Kind: 1})
	case 0x00BC:
		v.execCharGroup(r, false)
	case 0x00CB:
		v.execCharGroup(r, true)
	case 0x00C8:
		// 全屏 CG：`01 00 <file>`，下一记录才给名字时由 session 兜底。
		if len(r.Raw) > 2 {
			v.images = append(v.images, ImageOp{Kind: 3, Name: strings.TrimSpace(script.DecodeText(r.Raw[2:]))})
		}
	case 0x00E6:
		fields := script.SplitRaw(r.Raw, 2)
		if len(fields) >= 2 {
			v.images = append(v.images, ImageOp{Kind: 5, Name: strings.TrimSpace(fields[len(fields)-1])})
		}
	case 0x1009, 0x1019:
		// 颜色淡入：`FF FF FF 00 0A 00 32 00 64 00`（颜色 + 三档时长）。
		v.images = append(v.images, ImageOp{Kind: 4, Color: string(r.Raw[:min(3, len(r.Raw))])})
	case 0x10EF:
		payload := r.Raw
		if len(payload) > 1 {
			v.images = append(v.images, ImageOp{Kind: 6, Name: strings.TrimSpace(script.DecodeText(payload[1:]))})
		}
	}
}

func (v *VM) execCharGroup(r *script.Record, multi bool) {
	payload := r.Raw
	body := ""
	if multi {
		// `01 00 <count>,<name>/x/sx/sy,<name>/…`
		if len(payload) < 2 {
			return
		}
		body = script.DecodeText(payload[2:])
	} else {
		// `01 00 01,<name>/x/sx/sy`
		if len(payload) < 2 {
			return
		}
		body = script.DecodeText(payload[2:])
	}
	// 去掉“<count>,”前缀。
	if comma := strings.Index(body, ","); comma >= 0 {
		body = body[comma+1:]
	}
	parts := strings.Split(body, ",")
	for i, part := range parts {
		fields := strings.Split(part, "/")
		if len(fields) == 0 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		// 部件写法是 `name/y/xzoom/yzoom`（原版与 lune2renpy 一致）：
		// 第二个字段是 y 偏移，三四个字段是缩放。x 由槽位决定——原版把
		// 单人/双人/三人分别摆在中、左右、左右中（见 slotX）。
		op := ImageOp{Kind: 2, Name: strings.TrimSpace(fields[0]), Alpha: 255, Scale: 1}
		if len(fields) >= 4 {
			op.Y = atoiOr(fields[1], 0)
			op.Scale = atofOr(fields[2], 1)
			if sx := atofOr(fields[3], op.Scale); sx != 0 {
				op.Scale = sx
			}
		}
		op.Slot = i
		op.X = slotX(len(parts), i)
		v.images = append(v.images, op)
	}
}

// slotX 返回第 i 个立绘在屏幕上的横向偏移。
//
// 原版最多同时摆 3 个：单人居中、双人左右、三人左中右；偏移量取
// _system 的 CHARA_SPR_LEFT/RIGHT（本作是 -300/+300），四人以上复用三人
// 的排列并向下叠（本作没有四人同屏的剧本）。
func slotX(count, index int) int {
	switch {
	case count <= 1:
		return 0
	case count == 2:
		if index == 0 {
			return -300
		}
		return 300
	default:
		switch index {
		case 0:
			return -300
		case 1:
			return 300
		default:
			return 0
		}
	}
}

// execAudio 处理音频。
//
// 判据不用“低字节是一张命令表”，而是分成两类，这两类在数据里不会混淆：
//
//   - payload 为空 → 停止该通道上的声音（0x10/0xac/0xca/0x29/0xe2/0x92…）
//   - payload 非空 → 播一条资源；名字前面可能带 4 字节控制头（`00 00 0f 27`）
//
// 0x0000 是 BGM（循环、通道 0），其余按通道号播一次性声音。0x0020 的短
// payload（`88 13`）是“声音剩余时长”，0x0046/0x0064 的 payload 是角色名
// 或编号（名牌/主题），都不发播放事件。
//
// 为什么不按低字节建表：同一条命令在不同世代里低字节不同（0x2c/0x2d/0x30
// 都是 SE），而“空 payload = 停止”这个判据对整个语料成立，且不会把停止
// 误判成播放（播放一条空名字只会被静默丢弃，停止被当成播放则会叠音）。
func (v *VM) execAudio(r *script.Record) {
	channel := script.AudioChannel(r.Op, r.Arg)
	payload := r.Raw
	low := r.Arg & 0x00FF

	if len(payload) == 0 {
		// 停止：BGM 通道单独一类，其余停该通道的 SE/语音。
		if low == 0x10 || r.Arg == 0x0010 {
			v.audio = append(v.audio, Audio{Kind: 3, Channel: 1})
			return
		}
		v.audio = append(v.audio, Audio{Kind: 4, Channel: channel})
		return
	}

	name := func() string {
		// BGM/SE 的 payload 前缀是固定控制字节 `00 00 0f 27`（音量/参数），
		// 名字从第 4 字节开始；语音类没有前缀。
		if len(payload) >= 4 && payload[0] == 0 && payload[1] == 0 {
			return strings.TrimSpace(script.DecodeText(payload[4:]))
		}
		return strings.TrimSpace(script.DecodeText(payload))
	}

	switch low {
	case 0x00:
		v.audio = append(v.audio, Audio{Kind: 0, Name: name(), Volume: 1, Loop: true, Channel: 1})
		return
	case 0x20:
		// `88 13` = 5000ms：声音剩余时长/淡出参数，不单独播放。
		return
	case 0x46, 0x64:
		// 角色名/主题：名牌与主题图形，不是音频。
		return
	}
	if name() == "" {
		return
	}
	// 0x10c2 / 0x11c2 / 0x12c2 是“循环 SE”，0x1003 / 0x1019 之类是语音。
	loop := low == 0xc2
	kind := 1
	if loop {
		kind = 1
	}
	v.audio = append(v.audio, Audio{Kind: kind, Name: name(), Volume: 1, Loop: loop, Channel: channel})
}

// execSystem 处理系统命令。
//
// 0x0064 是鉴赏目录的定义（`00 00 <cols> <rows> <x><y><w><h> <名字表>`），
// 由 `view` 脚本驱动。原版在这里画一整页自绘画廊；我们把“打开鉴赏”
// 交给宿主（它已经有跨品牌的鉴赏页），目录数据通过 CgGroups 暴露：
// 少写一份自绘画廊，也不会和宿主已有的 UI 冲突。
func (v *VM) execSystem(r *script.Record) {
	switch r.Arg {
	case 0x0051:
		// 旧世代把"打开鉴赏"交给宿主；本作走脚本自绘（见 0x64/0xc3）。
		if v.screen == nil {
			v.hostRequest = "gallery"
		}
	case 0x0064:
		// 鉴赏目录的单元定义：原版在这里把"格子坐标 + 名字"交给渲染器，
		// 由脚本自己画一整页。宿主不再接 gallery 请求，改由引擎按这些数字
		// 复刻同一页，玩家点格子时脚本按标签跳转。
		//
		// 不能在这里停成 StepAwaitInput：一页有 20 条 0x64（20 个格子），
		// 每条都停一次会让整页永远画不出来。界面定义段由"下一条标签"或
		// 末尾的 0x10 收尾（见 execFlow）。
		v.addScreenItem(r)
	case 0x00e3:
		// 立绘鉴赏器（CHARACTER VIEWER）：原版在这里切到"立绘查看"模式，
		// 画面由引擎按 _charaview 的布局自绘。紧跟其后的 0x10 是"戻る"，
		// 所以这里必须停下等玩家操作，而不是继续执行。
		v.charaView = true
		v.step = StepAwaitTitle
		return
	case 0x00c3:
		// 界面的四张图：空位/未完成/底图/悬停（command 4 的 0xc3）。
		// 以前这条落在 switch 的 default 里被当成"未知系统命令"丢掉，
		// 结果鉴赏页永远没有底图（玩家只看到上一页的 EXTRA MENU）。
		v.setScreenGraphic(r)
	case 0x0071:
		// 官网链接：原版用 ShellExecute 开浏览器。宿主已支持 url: 前缀的
		// 宿主动作（见 player_screen 的 hostRequest 分派），这里照用。
		// 记录里的值本身就带协议头（本作是 "lune-soft.jp/game/...“），
		// 缺协议时补 https://，避免宿主把它当相对路径。
		link := strings.TrimSpace(string(r.Raw))
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			link = "https://" + link
		}
		v.hostRequest = "url:" + link
	}
}

// execMisc 处理杂项：章节号/标题/等待/结束。
func (v *VM) execMisc(r *script.Record) {
	switch r.Arg {
	case 0x0014:
		v.chapterID = atoiOr(string(r.Raw), v.chapterID)
	case 0x0015:
		v.chapter = r.Text()
	case 0x0051:
		v.title = r.Text()
		if v.title == "" {
			v.title = string(r.Raw)
		}
	case 0x0082:
		v.waitMs = atoiOr(string(r.Raw), 0)
		// 等待是“本帧还要继续”，不是等点击：由 session 按时间推进。
	case 0x0030:
		// 界面的达成率（op5/0x30）：载荷是**一个字节**的档位（1 = 100%、0 = 0%），
		// 不是 ASCII 数字。原版由 MEMORY COMPLETE 的 PERCENT100_YES /
		// PERCENT000_YES 触发，效果是把全局存档的 CG/场景解锁位全置 1 或全清。
		//
		// 队里第二条（len>1）是"同一次请求的附加字节"，不是另一个数值：
		// 以前把它读成 u16，任何带附加字段的记录都会被算成天文数字。
		level := 0
		if len(r.Raw) > 0 {
			level = int(r.Raw[0])
		}
		if level != 0 {
			level = 1
		}
		v.setPercentValue(level * 100)
		v.percentReq = int32(level * 100)
	case 0x001E:
		// 结束标记：回标题。
		if !v.pop() {
			v.finish()
			v.hostRequest = "title"
		}
	case 0x0000:
		// `op5/0x00` 是空操作（出现在每个脚本开头，原版用来占位）。

	}
}

// splitPair 拆 "a,b"。
func splitPair(value string) (string, string) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) < 2 {
		return strings.TrimSpace(parts[0]), ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// parseButton 解析 `u16 x, u16 y, u16 w, u16 h, name`。
func parseButton(raw []byte) (TitleButton, bool) {
	if len(raw) < 9 {
		return TitleButton{}, false
	}
	button := TitleButton{
		X: le16(raw[0:2]), Y: le16(raw[2:4]), W: le16(raw[4:6]), H: le16(raw[6:8]),
		Label: strings.TrimSpace(script.DecodeText(raw[8:])),
	}
	if button.Label == "" || button.W == 0 || button.H == 0 {
		return TitleButton{}, false
	}
	return button, true
}

// parseGalleryButton 解析 `"  <x><y>... ,,<label>"`（0x43 的立绘一览按钮）。
func parseGalleryButton(raw []byte) (TitleButton, bool) {
	text := script.DecodeText(raw)
	label := ""
	if idx := strings.LastIndex(text, ","); idx >= 0 {
		label = strings.TrimSpace(text[idx+1:])
	}
	if label == "" || len(raw) < 4 {
		return TitleButton{}, false
	}
	// 前面的坐标是“立绘格子的 x/y/w/h”，本作固定为 32x32 的图标格。
	return TitleButton{X: 0, Y: 0, W: 0, H: 0, Label: label}, true
}

func le16(raw []byte) int { return int(binary.LittleEndian.Uint16(raw)) }

func atoiOr(value string, def int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	n := 0
	sign := 1
	for i := 0; i < len(value); i++ {
		c := value[i]
		if i == 0 && c == '-' {
			sign = -1
			continue
		}
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n * sign
}

func atofOr(value string, def float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return def
	}
	var result float64
	var frac, scale float64 = 0, 1
	dot := false
	neg := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case c == '-' && i == 0:
			neg = true
		case c == '.':
			dot = true
		case c >= '0' && c <= '9':
			if dot {
				scale /= 10
				frac += float64(c-'0') * scale
			} else {
				result = result*10 + float64(c-'0')
			}
		default:
			return def
		}
	}
	result += frac
	if neg {
		result = -result
	}
	return result
}

// Snapshot 是存档用的 VM 状态。
type Snapshot struct {
	Script    string   `json:"script"`
	PC        int      `json:"pc"`
	CallStack []string `json:"call_stack,omitempty"`
}

// SnapshotState 导出当前位置。
func (v *VM) SnapshotState() Snapshot {
	out := Snapshot{PC: v.pc}
	if v.current != nil {
		out.Script = v.current.Name
	}
	for _, f := range v.stack {
		if f.script != nil {
			out.CallStack = append(out.CallStack, fmt.Sprintf("%s:%d", f.script.Name, f.pc))
		}
	}
	return out
}

// Restore 从快照恢复。调用栈里保存的是“返回点”，恢复时按相反顺序重建。
func (v *VM) Restore(s Snapshot) error {
	v.stack = nil
	for _, entry := range s.CallStack {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			continue
		}
		s, err := v.load(parts[0])
		if err != nil {
			return err
		}
		v.stack = append(v.stack, frame{script: s, pc: atoiOr(parts[1], 0)})
	}
	if s.Script == "" {
		return fmt.Errorf("tmrhiro/vm: 快照里没有脚本名")
	}
	// 恢复后不能沿用上一段脚本留下的界面定义。
	v.ClearScreen()
	if err := v.enter(s.Script, s.PC); err != nil {
		return err
	}
	// 恢复后重新执行当前记录（保存时 PC 指向“下一条”，因此这里退一格）。
	v.pc = max(0, s.PC-1)
	v.step = StepContinue
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SetVar 写一个脚本变量（姓名输入用：主角名进 ＄α）。
func (v *VM) SetVar(key, value string) {
	if v == nil || key == "" {
		return
	}
	if v.vars == nil {
		v.vars = make(map[string]int32)
	}
	// 本作的变量表只有 int32 槽位；名字这类字符串值放不进数值槽，
	// 所以另存一份字符串表，正文渲染时优先查它。
	if v.strVars == nil {
		v.strVars = make(map[string]string)
	}
	v.strVars[key] = value
}

// StrVar 读一个字符串变量（没有时返回空串）。
func (v *VM) StrVar(key string) string {
	if v == nil || v.strVars == nil {
		return ""
	}
	return v.strVars[key]
}

// StrVars 返回全部字符串变量的副本（存档用）。
func (v *VM) StrVars() map[string]string {
	if v == nil || len(v.strVars) == 0 {
		return nil
	}
	out := make(map[string]string, len(v.strVars))
	for key, value := range v.strVars {
		out[key] = value
	}
	return out
}

// SetStrVars 覆盖字符串变量（读档用）。
func (v *VM) SetStrVars(values map[string]string) {
	v.strVars = make(map[string]string, len(values))
	for key, value := range values {
		v.strVars[key] = value
	}
}

// ReplaceVars 把正文/名字里的脚本变量（＄α、＄β…）换成实际值。
//
// 本作的剧本在名字栏直接写 `＄α`（主角名），台词正文里也会出现 `＄α様`。
// 原引擎在显示前做变量替换；不替换的话对话框上就是字面的「＄α」。
// 只处理以 ＄（全角美元）开头的单字变量名，避免误伤正文里的普通字符。
func (v *VM) ReplaceVars(text string) string {
	if text == "" || len(v.strVars) == 0 {
		return text
	}
	out := []rune(text)
	for i := 0; i < len(out); i++ {
		if out[i] != '＄' {
			continue
		}
		// ＄ 后面跟着一个全角字母（α/β/γ…）：查表替换。
		if i+1 >= len(out) {
			break
		}
		key := string(out[i : i+2])
		if value, ok := v.strVars[key]; ok && value != "" {
			replacement := []rune(value)
			out = append(out[:i], append(replacement, out[i+2:]...)...)
			i += len(replacement) - 1
			continue
		}
		i++
	}
	return string(out)
}

// Replay 重新执行"当前停住的那一条记录"。
//
// 姓名输入确认后要用它：那一句正文里的 ＄α 在第一次执行时被替换成了字面
// 变量名，重跑一次才能拿到玩家刚填进去的名字（原版同样是"填完名字回到
// 这一句"）。只回退一格 PC，不改变调用栈与脚本位置。
func (v *VM) Replay() {
	if v == nil || v.current == nil || v.pc <= 0 {
		return
	}
	v.pc--
	v.step = StepContinue
}
