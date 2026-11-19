// Package script 实现 FFA SYSTEM 的 SO4 字节码运行时。
//
// SO4 不是“编译后的机器码”，而是一串带长度前缀的记录：
//
//	u16 opcode, u16 总长度（含 4 字节头）, payload
//
// 控制流用绝对字节偏移（op 0x21a/0x21b/0x24f），变量是一块 int32 数组，
// 字符串/图片/窗口描述都放在这块数组里，通过 0x400 个“buffer base”
// 映射到不同槽位。表达式是独立的字节码（0x01 立即数、0x02 取变量、0x8b 加…
// 以 0x00 结束）。这里逐条复刻原解释器 0x400000 处 EXE 的语义。
package script

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"kizuna/enginego/brand/blackpackage/archive"
)

const (
	numBuffers  = 0x400
	bufStride   = 0x40 // dwords per buffer
	bufBase0    = 0x1b58
	varDwords   = 0x20000
	valStackMax = 0x2000
	ctxMax      = 0x100
)

// Step 是 VM 对外暴露的停止原因。与 core.Step 保持同值，session 直接转换。
type Step int32

const (
	StepContinue    Step = 0
	StepAwaitInput  Step = 1
	StepAwaitChoice Step = 2
	StepAwaitSelect Step = 3
	StepWait        Step = 4
	StepFinished    Step = 5
	StepFaulted     Step = 6
	// StepAwaitMovie 与 core.StepAwaitMovie 同值：脚本已请求影片，
	// 宿主播完（或跳过）后调用 FinishMovie 继续。
	StepAwaitMovie Step = 9
)

// Choice 是一个选择项。Value 是脚本给的值（op 的 index），Text 是展开后的文本。
type Choice struct {
	Value int32
	Text  string
}

// Stage 是 VM 需要的绘图层。session 用 800x600 RGBA 软件层实现。
type Stage interface {
	CreateLayer(idx int32, args []int32)
	FreeLayer(idx int32)
	LayerRect(idx int32, x, y, w, h int32)
	LoadImage(layer int32, name string, args []int32)
	Fill(layer int32, x, y, w, h, color int32)
	Chip(args []int32)
	Blit(sel int32, args []int32)
	Redraw()
	SetActive(layer int32)
	// SetWindowColor 记录 op 0x102 给的窗口颜色，供 0x16 建窗使用。
	SetWindowColor(idx, color int32)
	// FillRect 对应 0x6/0xa2：在层上填一个纯色矩形。
	FillRect(layer, x, y, w, h, color int32)
	// CopyRect 对应 0x7/0xa6：层间矩形拷贝。
	CopyRect(src, dst, sx, sy, w, h, dx, dy int32)
	// ImageSize 返回已解码图片尺寸；0xa3 用它把载入矩形回写给脚本。
	ImageSize(name string) (int, int)
}

// Audio 是 VM 投给宿主的音频事件。Kind 与 core.AudioEvent 一致：
// 0=BGM,1=SE,2=Voice,3=停BGM,4=停Voice,5=停SE。
type Audio struct {
	Kind  uint32
	Name  string
	Loop  bool
	ID    int32
	Flags int32
	// Volume 是脚本给的音量（0..1，已归一）。0 表示"用引擎设置里的类别音量"。
	Volume float64
}

// Record 是一条已解析的记录。
type Record struct {
	Off     int
	Op      uint16
	Len     int
	Payload []byte
}

// Script 是一个 SO4 文件。Offsets 用文件内绝对字节偏移。
type Script struct {
	Name string
	Data []byte
	Recs map[int]Record
}

// Parse 把 SO4 字节切成记录。原版遇到 len<4 或越界会停；这里返回 error，
// 调用方把坏脚本当成不可用，而不是让整局崩。
func Parse(name string, data []byte) (*Script, error) {
	s := &Script{Name: name, Data: data, Recs: make(map[int]Record)}
	for off := 0; off+4 <= len(data); {
		op := binary.LittleEndian.Uint16(data[off : off+2])
		ln := int(binary.LittleEndian.Uint16(data[off+2 : off+4]))
		if ln < 4 || off+ln > len(data) {
			return nil, fmt.Errorf("blackpackage: %s 记录越界 @%#x (op=%#x len=%d)", name, off, op, ln)
		}
		s.Recs[off] = Record{Off: off, Op: op, Len: ln, Payload: data[off+4 : off+ln]}
		off += ln
	}
	return s, nil
}

type ctx struct {
	name string
	ip   int
}

// VM 是单个存档槽/单局运行态。所有状态都在这里，session 只做转发。
type VM struct {
	set   *archive.Set
	stage Stage

	scripts map[string]*Script
	vars    []byte // int32 数组，按小端访问；同时是字符串 buffer 的存储
	bufBase [numBuffers]uint32
	stack   []int32
	ctxs    []ctx
	cur     int

	ip       int
	loaded   bool
	steps    int
	lastErr  string
	finished bool

	// 最近一次 yield 的内容。session 在 Run 返回后取走。
	Speaker string
	Text    string
	Choices []Choice
	WaitMs  uint32
	Audio   []Audio

	// 菜单暂存：op 0x15 逐条登记，等到 0x11/0x9 的 yield 再交给宿主。
	menu []Choice
	// 音频 id -> 文件名。
	sounds map[int32]string
	// 音频通道表。原版是 4 个独立播放器（0x463b20[0..3]），脚本用同一个
	// 通道号先播后停；停止时必须按“这条通道上最后播的是什么”来决定停
	// BGM / SE / 语音，否则一个 stop 会误杀整类声音（本作通道 1 同时承载
	// 语音和音效）。chanKind[ch] 记录该通道最后的类别，port 见 enqueue。
	chanKind [4]uint32
	chanName [4]string
	// flags 是原版 0x48eae0 的字节标志表（op 0xac 写 / 0xad 读），
	// 本作 16988 次写、用于剧情分支与界面开关，必须实现。
	flags [0x100]int8
	// uiFlags 是 op 0xd8 的系统开关表（0x483224 起，按 selector 分槽）。
	// 其中 auto/skip/消息窗可见性会被 session 读出来。
	uiFlags [0x24]int32
	// msgVisible 是 op 0xb4/0xb5 与 0xe6 维护的“对话框是否显示”。
	msgVisible bool
	// textClear 是 op 0xe6 sel 9 的“清空文本”请求。
	textClear bool
	// autoFlag / skipFlag 来自 0xe6 sel 1/2（AUTO/SKIP 状态，脚本会读回）。
	autoFlag bool
	skipFlag bool
	// 当前 active layer（op 0x11/0x9 的第一个参数）。
	active int32
	// 文本流控制。
	flag475c04 bool
	// 自动/跳过等系统开关。
	autoMode bool
	// 调用深度保护，避免坏脚本把 ctx 撐爆。
	callDepth int
	// start 是 VM 的单调时钟起点。原版 timeGetTime 返回开机毫秒数，
	// 用 UnixMilli 直接截断成 int32 会在脚本的“已过 N 毫秒”比较里溢出。
	start time.Time
	// storeDir 是可写的用户数据目录（存档、配置）。Android 上游戏目录
	// 只读，原版写到当前目录的 X0WIN*.DAT 必须重定向到这里。
	storeDir string
	// movie 是当前等待宿主播放的影片名（空串表示没有）。
	movie string
	// images 是本次 Run 中 0xa3 载入过的图片名（session 用来记录鉴赏解锁）。
	images []string
	// LastImageRaw / LastImageExpanded 是最近一次 op 0xa3 的原始/展开名，调试用。
	LastImageRaw      string
	FirstImageRaw     string
	FirstImageBase    uint32
	LastImageExpanded string
	lastResource      string
	// Trace 是最近的执行轨迹，调试/测试用；生产里只保留 64 条环形记录。
	Trace []string
	// winSerial 是 0x16 创建窗口时分配的假句柄序号（见 exec.go）。
	winSerial int32
	// noopTicks 统计“x = x - 0”这种空转赋值连续执行的次数。原版有些淡入
	// 淡出循环靠 DirectDraw 回调把变量递减到 0 才退出（本作 X0_OP00@0x7fb
	// 的 aca4 就是这种）。我们没有那个回调，如果无限空转就会卡死在 OP。
	// 连续空转超过阈值时把目标变量清零，等价于回调把动画走完。
	noopTicks map[int32]int
}

// New 建 VM。vars 按 0x20000 dwords 分配：buffer 1023 的基址 0x11b18 加
// 256 字节仍远小于这个值，足够原版所有脚本使用。
func New(set *archive.Set, stage Stage) *VM {
	v := &VM{
		set:     set,
		stage:   stage,
		scripts: make(map[string]*Script),
		vars:    make([]byte, varDwords*4),
		stack:   make([]int32, 0, valStackMax),
		ctxs:    make([]ctx, 0, ctxMax),
		sounds:    make(map[int32]string),
		noopTicks: make(map[int32]int),
		start:     time.Now(),
	}
	for i := 0; i < numBuffers; i++ {
		v.bufBase[i] = uint32(bufBase0 + i*bufStride)
	}
	return v
}

// SetStage 允许 session 在 Open 后再挂舞台（避免循环依赖）。
func (v *VM) SetStage(s Stage) { v.stage = s }

// SetStoreDir 指定可写目录；原版写在游戏目录下的 X0WIN*.DAT 会落在这里。
func (v *VM) SetStoreDir(dir string) { v.storeDir = dir }

// TakeImages 取走本次 Run 载入的图片名。
func (v *VM) TakeImages() []string {
	out := v.images
	v.images = nil
	return out
}

// Movie 返回待播影片名（不含目录）。空串表示当前没有影片。
func (v *VM) Movie() string { return v.movie }

// FinishMovie 清除影片请求，并按原版回调置位演出脚本轮询的标志。
func (v *VM) FinishMovie() {
	v.movie = ""
	v.setVar(0xb3d0, 1)
	v.setVar(0xb3e7, 1)
}

// SignalSkip 模拟原版主循环在“点击跳过 OP/演出”时写入的轮询标志。
// 0xb3d0 是 OP 脚本 0x24f 等待的变量；置 1 后演出循环会立即收尾。
func (v *VM) SignalSkip() { v.setVar(0xb3d0, 1) }

// LastError 返回最近一次错误。
func (v *VM) LastError() string { return v.lastErr }

// Finished 表示顶层脚本已 RET。
func (v *VM) Finished() bool { return v.finished }

// Vars 暴露变量数组（只读用途：session 取主角名/选项值）。
func (v *VM) Vars() []byte { return v.vars }

// BufBase 返回 buffer n 的 dword 基址。
func (v *VM) BufBase(n int) uint32 {
	if n < 0 || n >= numBuffers {
		return 0
	}
	return v.bufBase[n]
}

// Start 按名字载入顶层脚本并把 IP 归零。名字大小写不敏感，允许省略扩展名。
func (v *VM) Start(name string) error {
	s, err := v.loadScript(name)
	if err != nil {
		return err
	}
	v.ctxs = v.ctxs[:0]
	v.ctxs = append(v.ctxs, ctx{name: s.Name})
	v.cur = 0
	v.ip = 0
	v.loaded = true
	v.finished = false
	v.steps = 0
	return nil
}

// IP 返回当前脚本内的字节偏移（调试/诊断）。
func (v *VM) IP() int { return v.ip }

// CurrentScript 返回当前顶层脚本名。
func (v *VM) CurrentScript() string {
	if v.cur < 0 || v.cur >= len(v.ctxs) {
		return ""
	}
	return v.ctxs[v.cur].name
}

func (v *VM) loadScript(name string) (*Script, error) {
	if v.set == nil {
		return nil, fmt.Errorf("blackpackage: 未打开资源包")
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if s, ok := v.scripts[key]; ok {
		return s, nil
	}
	data, err := v.set.Entry(name)
	if err != nil {
		// 允许只给主干名：A0_000 -> A0_000.SO4。
		if !strings.HasSuffix(key, ".so4") {
			if d2, err2 := v.set.Entry(name + ".so4"); err2 == nil {
				data = d2
				key += ".so4"
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	s, err := Parse(key, data)
	if err != nil {
		return nil, err
	}
	v.scripts[key] = s
	// 同时按主干名登记，后续 $s 变量引用更容易命中。
	v.scripts[strings.TrimSuffix(key, ".so4")] = s
	return s, nil
}

func (v *VM) script() *Script {
	if v.cur < 0 || v.cur >= len(v.ctxs) {
		return nil
	}
	return v.scripts[strings.ToLower(v.ctxs[v.cur].name)]
}

// Run 执行最多 budget 条记录，遇到 yield/结束就返回。
// budget<=0 时用 4096，和其它品牌一致。
func (v *VM) Run(budget int) (Step, error) {
	if !v.loaded {
		return StepFaulted, fmt.Errorf("blackpackage: VM 未启动")
	}
	if budget <= 0 {
		budget = 4096
	}
	for i := 0; i < budget; i++ {
		if v.finished {
			return StepFinished, nil
		}
		s := v.script()
		if s == nil {
			v.fail("当前脚本不存在")
			return StepFaulted, fmt.Errorf("%s", v.lastErr)
		}
		rec, ok := s.Recs[v.ip]
		if !ok {
			v.fail(fmt.Sprintf("IP 不在记录边界: %#x", v.ip))
			return StepFaulted, fmt.Errorf("%s", v.lastErr)
		}
		v.steps++
		if len(v.Trace) >= 64 {
			v.Trace = v.Trace[1:]
		}
		v.Trace = append(v.Trace, fmt.Sprintf("%s:%#x op=%#x", s.Name, v.ip, rec.Op))
		step, err := v.exec(s, rec)
		if err != nil {
			v.fail(err.Error())
			return StepFaulted, err
		}
		if step != StepContinue {
			return step, nil
		}
	}
	return StepContinue, nil
}

func (v *VM) fail(msg string) {
	v.lastErr = "blackpackage: " + msg
}

// ---- 基础读写 ----

func (v *VM) varAt(slot int32) int32 {
	i := int(slot) * 4
	if i < 0 || i+4 > len(v.vars) {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(v.vars[i : i+4]))
}

func (v *VM) setVar(slot int32, val int32) {
	i := int(slot) * 4
	if i < 0 || i+4 > len(v.vars) {
		return
	}
	binary.LittleEndian.PutUint32(v.vars[i:i+4], uint32(val))
}

func (v *VM) varPtr(slot int32) []byte {
	i := int(slot) * 4
	if i < 0 || i+4 > len(v.vars) {
		return nil
	}
	return v.vars[i : i+4]
}

func (v *VM) bufBytes(buf int32, off int32) []byte {
	if buf < 0 || int(buf) >= numBuffers {
		return nil
	}
	base := int(v.bufBase[buf]) * 4
	start := base + int(off)
	if start < 0 || start >= len(v.vars) {
		return nil
	}
	return v.vars[start:]
}

func (v *VM) readCString(buf int32, off int32) string {
	b := v.bufBytes(buf, off)
	if b == nil {
		return ""
	}
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return decodeCP932(b[:n])
}

func (v *VM) readCStringAt(buf int32, off int32, max int) []byte {
	b := v.bufBytes(buf, off)
	if b == nil {
		return nil
	}
	n := 0
	for n < len(b) && n < max && b[n] != 0 {
		n++
	}
	return b[:n]
}

func decodeCP932(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	out, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

func encodeCP932(s string) []byte {
	if s == "" {
		return nil
	}
	out, _, err := transform.Bytes(japanese.ShiftJIS.NewEncoder(), []byte(s))
	if err != nil {
		return []byte(s)
	}
	return out
}

// ---- 表达式 ----

type exprReader struct {
	vm  *VM
	pay []byte
	pos int
}

func (r *exprReader) more() bool { return r.pos < len(r.pay) }

func (r *exprReader) byte() byte {
	if r.pos >= len(r.pay) {
		return 0
	}
	b := r.pay[r.pos]
	r.pos++
	return b
}

func (r *exprReader) u32() uint32 {
	if r.pos+4 > len(r.pay) {
		r.pos = len(r.pay)
		return 0
	}
	v := binary.LittleEndian.Uint32(r.pay[r.pos : r.pos+4])
	r.pos += 4
	return v
}

// eval 执行一段以 0x00 结尾的表达式，返回栈顶的“值”（0x42e4f0）。
func (v *VM) eval(r *exprReader) int32 {
	val, _ := v.evalTyped(r)
	return val
}

// evalSlot 返回栈顶的“地址”（0x42e920）：原版求值器全程维护一张类型栈，
// 直接由 0x02 <slot> 压入的项其类型就是 slot 号，运算结果为 -1。
// 目标槽位类操作码（赋值、循环计数、缓冲区写入）必须用它，否则会把
// vars[slot] 的值当成槽号，写坏整片变量区。
func (v *VM) evalSlot(r *exprReader) int32 {
	_, slot := v.evalTyped(r)
	return slot
}

// evalTyped 是 0x42e4f0/0x42e920 的公共实现：同时维护值栈和类型栈。
// 语义：int32 算术、有符号除法/取模、0x9b store；未知 token 逐字节跳过。
func (v *VM) evalTyped(r *exprReader) (int32, int32) {
	var st []int32
	var slots []int32
	push := func(x int32) { st = append(st, x); slots = append(slots, -1) }
	for r.more() {
		op := r.byte()
		if op == 0 {
			break
		}
		switch op {
		case 0x01:
			push(int32(r.u32()))
		case 0x02:
			n := int32(r.u32())
			st = append(st, v.varAt(n))
			slots = append(slots, n)
		case 0x83:
			if len(st) > 0 {
				st[len(st)-1] = -st[len(st)-1]
				slots[len(slots)-1] = -1
			}
		case 0x84:
			if len(st) > 0 {
				st[len(st)-1] = ^st[len(st)-1]
				slots[len(slots)-1] = -1
			}
		case 0x85:
			if len(st) > 0 {
				if st[len(st)-1] == 0 {
					st[len(st)-1] = 1
				} else {
					st[len(st)-1] = 0
				}
				slots[len(slots)-1] = -1
			}
		case 0x86:
			// 取栈顶类型（地址）作为值再压入；结果本身不是地址。
			if len(st) > 0 {
				push(slots[len(slots)-1])
			}
		case 0x87:
			// 间接取值：新类型 = 原值（地址），新值 = vars[地址]。
			if len(st) > 0 {
				slot := st[len(st)-1]
				st[len(st)-1] = v.varAt(slot)
				slots[len(slots)-1] = slot
			}
		case 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e,
			0x8f, 0x90, 0x91, 0x92, 0x93, 0x94,
			0x95, 0x96, 0x97, 0x98, 0x99:
			b := int32(0)
			if len(st) > 0 {
				b = st[len(st)-1]
				st = st[:len(st)-1]
				slots = slots[:len(slots)-1]
			}
			a := int32(0)
			if len(st) > 0 {
				a = st[len(st)-1]
			}
			var x int32
			switch op {
			case 0x88:
				x = a * b
			case 0x89:
				if b != 0 {
					x = a / b
				}
			case 0x8a:
				if b != 0 {
					x = a % b
				}
			case 0x8b:
				x = a + b
			case 0x8c:
				x = a - b
			case 0x8d:
				x = a << uint(b&31)
			case 0x8e:
				x = a >> uint(b&31)
			case 0x8f:
				if a >= b {
					x = 1
				}
			case 0x90:
				if a > b {
					x = 1
				}
			case 0x91:
				if a <= b {
					x = 1
				}
			case 0x92:
				if a < b {
					x = 1
				}
			case 0x93:
				if a == b {
					x = 1
				}
			case 0x94:
				if a != b {
					x = 1
				}
			case 0x95:
				x = a & b
			case 0x96:
				x = a ^ b
			case 0x97:
				x = a | b
			case 0x98:
				if a != 0 && b != 0 {
					x = 1
				}
			case 0x99:
				if a != 0 || b != 0 {
					x = 1
				}
			}
			if len(st) == 0 {
				push(x)
			} else {
				st[len(st)-1] = x
				slots[len(slots)-1] = -1
			}
		case 0x9b:
			// store: 栈顶是值，下一项是目标 slot。
			val := int32(0)
			if len(st) > 0 {
				val = st[len(st)-1]
				st = st[:len(st)-1]
				slots = slots[:len(slots)-1]
			}
			slot := int32(0)
			if len(st) > 0 {
				slot = st[len(st)-1]
			}
			v.setVar(slot, val)
			if len(st) == 0 {
				push(val)
			} else {
				st[len(st)-1] = val
				slots[len(slots)-1] = -1
			}
		case 0x9a:
			// 保留：原版 map 到无效 handler；忽略。
		default:
			// >0x9b 或未知 token 被原版逐字节跳过。
		}
	}
	if len(st) == 0 {
		return 0, -1
	}
	return st[len(st)-1], slots[len(slots)-1]
}

func (r *exprReader) cstr() string {
	start := r.pos
	for r.pos < len(r.pay) && r.pay[r.pos] != 0 {
		r.pos++
	}
	s := decodeCP932(r.pay[start:r.pos])
	if r.pos < len(r.pay) {
		r.pos++
	}
	return s
}

// readRawU32 读一个裸 u32（目标偏移/选择器），不是表达式。
func (r *exprReader) readRawU32() uint32 { return r.u32() }

// ---- buffer 基址/字符串写入 ----

func (v *VM) setBufferBase(buf int32, base int32) {
	if buf < 0 || int(buf) >= numBuffers {
		return
	}
	v.bufBase[buf] = uint32(base)
}

func (v *VM) copyStringToBuf(buf int32, off int32, s []byte) {
	v.noteResource(s)
	b := v.bufBytes(buf, off)
	if b == nil {
		return
	}
	n := len(s)
	if n >= len(b) {
		n = len(b) - 1
	}
	copy(b[:n], s[:n])
	if n < len(b) {
		b[n] = 0
	}
}

func (v *VM) writeBytesToBuf(buf int32, off int32, s []byte) {
	b := v.bufBytes(buf, off)
	if b == nil {
		return
	}
	if len(s) > len(b) {
		s = s[:len(b)]
	}
	copy(b, s)
}

func (v *VM) copyBuffer(dstBuf, dstOff, srcBuf, srcOff int32) {
	src := v.readCStringAt(srcBuf, srcOff, 0x10000)
	v.copyStringToBuf(dstBuf, dstOff, src)
}

func (v *VM) concatBuffer(dstBuf, dstOff int32, s []byte) {
	v.noteResource(s)
	b := v.bufBytes(dstBuf, dstOff)
	if b == nil {
		return
	}
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	if n >= len(b)-1 {
		return
	}
	space := len(b) - n - 1
	if len(s) > space {
		s = s[:space]
	}
	copy(b[n:n+len(s)], s)
	b[n+len(s)] = 0
}

// ---- 调试/兼容 ----

// SetVar 供 session 在进入新游戏时写主角名等。
func (v *VM) SetVar(slot int32, val int32) { v.setVar(slot, val) }

// StringVar 读一个 buffer 槽里的 C 字符串（主角名等）。
func (v *VM) StringVar(buf int32) string { return v.readCString(buf, 0) }

// SetStringVar 写一把 C 字符串到 buffer 0。
func (v *VM) SetStringVar(buf int32, s string) { v.copyStringToBuf(buf, 0, encodeCP932(s)) }

// unsafeBytes 为了兼容未来需要直接拿 vars 的调用方；当前未使用。
var _ = unsafe.Pointer(nil)

// Snapshot 是可序列化的运行态，供存档用。除了当前上下文与变量/buffer 基址，
// 还要保存“脚本自己能读回来的状态”：时钟、标志表、系统开关表、音频通道记账。
//
// 判定标准很简单：凡是 opcode 能读到、并且会影响后续执行路径的字段就必须进快照。
// 漏一个的后果不是画面花掉，而是读档后脚本走进另一条分支、或者卡在等一个
// 永远不会被置位的变量上（玩家看到的就是“读档后过几句话就卡住”）。
type Snapshot struct {
	Script  string     `json:"script"`
	IP      int        `json:"ip"`
	Cur     int        `json:"cur"`
	Ctxs    []CtxState `json:"ctxs"`
	Vars    []byte     `json:"vars"`
	BufBase []uint32   `json:"bufbase"`
	Stack   []int32    `json:"stack"`
	Active  int32      `json:"active"`
	// Tick 是存档瞬间的 timeGetTime 值（毫秒）。
	//
	// 原版 timeGetTime 是“开机以来的毫秒数”，脚本把动画开始时刻直接写进变量，
	// 之后反复用 `当前时刻 <= 开始时刻 + 时长` 决定演出是否结束（本作 OP/立绘
	// 淡入淡出全是这个写法）。我们的时钟以 VM 启动为零点，重启 App 后归零；
	// 读一份“已经玩了几十分钟”的存档时，存档里的绝对时刻远大于归零后的时钟，
	// 那些循环会白等到时钟追上为止（几分钟）。存下存档瞬间的时钟值、读档时
	// 把时钟接回去，效果等价于原版开机计时跨存档连续。
	Tick uint32 `json:"tick,omitempty"`
	// Flags / UIFlags 是 op 0xac/0xad 的 256 字节标志表和 op 0xd8 的 0x24 槽
	// 系统开关表（本作写 flags 上万次，用于剧情分支与界面开关）。它们属于
	// 存档必须还原的“脚本可见状态”，否则读档后的分支判定和存档前不一致。
	Flags   []int8  `json:"flags,omitempty"`
	UIFlags []int32 `json:"uiflags,omitempty"`
	// MsgVisible / Flag475c04 同样是脚本可读的界面/文本流状态。
	MsgVisible bool `json:"msg_visible,omitempty"`
	Flag475c04 bool `json:"flag475c04,omitempty"`
	// Sounds 是“音频 id -> 文件名”的注册表，ChanKind/ChanName 是 4 个播放通道
	// 上最后播的是哪一类声音。原版 stop 按通道停，类别记错会把 BGM 当语音停掉
	// （或反之）。读档后脚本会继续用通道号 stop，所以这三项也要跟着存档走。
	Sounds   map[int32]string `json:"sounds,omitempty"`
	ChanKind []uint32         `json:"chan_kind,omitempty"`
	ChanName []string         `json:"chan_name,omitempty"`
}

// CtxState 是一个脚本上下文。
type CtxState struct {
	Name string `json:"name"`
	IP   int    `json:"ip"`
}

// Snapshot 导出当前状态。
func (v *VM) Snapshot() Snapshot {
	s := Snapshot{
		Script:  v.CurrentScript(),
		IP:      v.ip,
		Cur:     v.cur,
		Vars:    append([]byte(nil), v.vars...),
		BufBase: make([]uint32, numBuffers),
		Stack:   append([]int32(nil), v.stack...),
		Active:  v.active,
	}
	copy(s.BufBase, v.bufBase[:])
	for _, c := range v.ctxs {
		s.Ctxs = append(s.Ctxs, CtxState{Name: c.name, IP: c.ip})
	}
	s.Tick = v.tickMs()
	s.Flags = append([]int8(nil), v.flags[:]...)
	s.UIFlags = append([]int32(nil), v.uiFlags[:]...)
	s.MsgVisible = v.msgVisible
	s.Flag475c04 = v.flag475c04
	if len(v.sounds) > 0 {
		s.Sounds = make(map[int32]string, len(v.sounds))
		for id, name := range v.sounds {
			s.Sounds[id] = name
		}
	}
	s.ChanKind = append([]uint32(nil), v.chanKind[:]...)
	s.ChanName = append([]string(nil), v.chanName[:]...)
	return s
}

// Restore 从存档恢复。脚本必须仍能从资源包读到；变量/buffer 基址原样恢复。
func (v *VM) Restore(s Snapshot) error {
	if s.Script == "" {
		return fmt.Errorf("blackpackage: 存档没有脚本名")
	}
	scr, err := v.loadScript(s.Script)
	if err != nil {
		return err
	}
	if len(s.Vars) != len(v.vars) {
		return fmt.Errorf("blackpackage: 存档变量表长度不符")
	}
	copy(v.vars, s.Vars)
	if len(s.BufBase) == numBuffers {
		copy(v.bufBase[:], s.BufBase)
	}
	v.stack = append(v.stack[:0], s.Stack...)
	v.ctxs = v.ctxs[:0]
	if len(s.Ctxs) == 0 {
		v.ctxs = append(v.ctxs, ctx{name: scr.Name, ip: s.IP})
	} else {
		for _, c := range s.Ctxs {
			v.ctxs = append(v.ctxs, ctx{name: c.Name, ip: c.IP})
		}
	}
	v.cur = s.Cur
	if v.cur < 0 || v.cur >= len(v.ctxs) {
		v.cur = 0
	}
	v.active = s.Active
	// IP 以快照里的 v.ip 为准。
	//
	// 以前这里有个 `if v.ip == 0 { v.ip = ctxs[cur].ip }` 的兜底，那是个隐患：
	// ctxs[cur].ip 是“该上下文调用子脚本时的返回地址”，平时并不跟随执行位置
	// 更新。脚本跑回偏移 0 时存档，兜底会把它跳到很久以前的返回地址上，
	// 表现是读档后剧情突然跳到别处或者卡住。解析出的 ctx（len(Ctxs)==0 时）
	// 已经用 s.IP 填好 ip，这里不需要兜底。
	v.ip = s.IP
	// 标志表/开关表：旧存档（没有这些字段）长度对不上，保留当前值而不是清零，
	// 至少不会把同进程里刚积累的剧情标志清掉。
	if len(s.Flags) == len(v.flags) {
		copy(v.flags[:], s.Flags)
	}
	if len(s.UIFlags) == len(v.uiFlags) {
		copy(v.uiFlags[:], s.UIFlags)
	}
	v.msgVisible = s.MsgVisible
	v.flag475c04 = s.Flag475c04
	if s.Sounds != nil {
		v.sounds = s.Sounds
	}
	if len(s.ChanKind) == len(v.chanKind) {
		copy(v.chanKind[:], s.ChanKind)
	}
	if len(s.ChanName) == len(v.chanName) {
		copy(v.chanName[:], s.ChanName)
	}
	// 时钟接回存档瞬间（只前进不后退），见 Snapshot.Tick 的说明。
	v.SetClockBase(s.Tick)
	v.loaded = true
	v.finished = false
	return nil
}

// SetClockBase 让 tickMs() 从现在开始接着 base 毫秒往下走。
//
// 读档用它把“存档瞬间的时钟”接回来；当前时钟已经大于存档值时不回拨，
// 这与原版“开机计时一直往前走”一致：动画循环读到“已过时长”就会正常收尾，
// 不会因为时钟被重置而白等一段已经过去的演出时间。
func (v *VM) SetClockBase(base uint32) {
	if base == 0 {
		return
	}
	if v.tickMs() >= base {
		return
	}
	v.start = time.Now().Add(-time.Duration(base) * time.Millisecond)
}

// Tick 返回当前 timeGetTime 值（毫秒）。session 用它把时钟水位写盘，
// 让下次启动还能接着走，旧存档里的绝对时刻也不会大于新时钟。
func (v *VM) Tick() uint32 { return v.tickMs() }

// Seek 脚本内跳转，读档/标题菜单用。
func (v *VM) Seek(off int) {
	if off >= 0 {
		v.ip = off
	}
}

// VarAt 读变量槽（调试/测试）。
func (v *VM) VarAt(slot int32) int32 { return v.varAt(slot) }

func (v *VM) noteResource(s []byte) {
	low := strings.ToLower(string(s))
	if strings.HasSuffix(low, ".pt1") || strings.HasSuffix(low, ".wa1") || strings.HasPrefix(low, "x0_") || strings.HasPrefix(low, "y00") || strings.HasPrefix(low, "effect") {
		v.lastResource = strings.TrimSpace(string(s))
	}
}

// LastResourceName 在 vars 的字节区里找最后一个像资源名的 C 字符串。
// X0 把当前图片/语音名写进字符串 buffer，再由 $sr<off> 引用；当 buffer
// 基址被脚本改过、off 又无法直接命中时，用这个回退可以保持画面正确。
func (v *VM) LastResourceName() string {
	if v.lastResource != "" {
		return v.lastResource
	}
	best := ""
	for i := 0; i < len(v.vars); i++ {
		if v.vars[i] == 0 {
			continue
		}
		// 只从可打印 ASCII 起点开始，避免把二进制当名字。
		c := v.vars[i]
		if c < 'a' || c > 'z' {
			continue
		}
		j := i
		for j < len(v.vars) && j-i < 64 && v.vars[j] != 0 {
			j++
		}
		s := string(v.vars[i:j])
		low := strings.ToLower(s)
		if strings.HasSuffix(low, ".pt1") || strings.HasSuffix(low, ".wa1") ||
			(strings.HasPrefix(low, "x0_") && len(low) >= 5) {
			best = s
		}
		i = j
	}
	return best
}
