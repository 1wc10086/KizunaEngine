package bs

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	vmbs "kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

const TitleLabel = "title"

const (
	bsMapKeyword      = "map"
	bsZeturinProperty = "#zeturin"
	bsZeturinStride   = 10
)

const (
	bsTextSpeedDefault = 420
	bsTextSpeedMinimum = 1
	bsTextSpeedMaximum = 540
)

var bsDateMirror = [][2]string{{"#system1", "#GKR_week_no"}, {"#system2", "#GKR_week"}}

var bsClockMirror = [][2]string{{"#system1", "#GKR_ampm"}, {"#system2", "#GKR_hour"}, {"#system3", "#GKR_minute"}}

var bsPointMirror = [][2]string{{"#system1", "#GKR_flower"}}

type MapCatalog interface {
	IsEmpty() bool
}

var InstalledMapCatalog MapCatalog

type BsSystemDirector struct {
	machine *vm.BsxMachine
	script  vm.BsMachineCore
	state   *state.GameState
	logged  map[string]struct{}
	// screenHandler 把脚本主动请求的系统界面（sceneroom 等）交给 Session。
	// 系统导演本身不能引用 session，否则会形成循环导入。
	screenHandler func(keyword string) state.SessionStep
	// callHandler 是作品专属的系统调用钩子（MDDVD 的 XA/XR/XM/XB/XT）。
	// 返回 handled=true 时由它决定 SessionStep，导演不再走通用分派。
	callHandler func(name string) (state.SessionStep, bool)
}

func NewBsSystemDirector(machine *vm.BsxMachine, script vm.BsMachineCore, state *state.GameState) *BsSystemDirector {
	return &BsSystemDirector{machine: machine, script: script, state: state, logged: map[string]struct{}{}}
}

func (d *BsSystemDirector) SetScreenHandler(handler func(keyword string) state.SessionStep) {
	d.screenHandler = handler
}

// SetSystemCallHandler 安装作品专属的系统调用钩子。
func (d *BsSystemDirector) SetSystemCallHandler(handler func(name string) (state.SessionStep, bool)) {
	d.callHandler = handler
}

func (d *BsSystemDirector) Wait() state.SessionStep {
	milliseconds := max(0, int(d.machine.Command().First))
	if d.closesMessageWindow() {
		d.state.SetMessageWindowVisible(false)
	}
	d.state.SetWaitMilliseconds(milliseconds)
	core.Logf("script: wait %dms window=%v", milliseconds, d.state.MessageWindowVisible())
	return state.SessionWait
}

func (d *BsSystemDirector) SystemCall() state.SessionStep {
	name := d.script.Name(int(d.machine.Command().First))
	if len(name) == 0 {
		d.logOnce("system call with an unresolved name, continuing")
		return state.SessionContinue
	}
	keyword := strings.ToLower(name)
	if d.callHandler != nil {
		if step, handled := d.callHandler(name); handled {
			return step
		}
	}
	switch {
	case keyword == "del":
		d.script.ClearStack()
		d.logOnce("system call 'del' -> vm operand stack cleared")
		return state.SessionContinue
	case keyword == TitleLabel:
		// BSScript 旧作的标题不是由宿主直接打开，而是脚本主动请求：
		// boot 初始化完变量后会落到 title 标签，执行 0x30（system call）
		// 并把名字表里的 "title" 压进操作数栈。原引擎在这里停住脚本、
		// 切回标题画面；如果像以前那样只记日志继续执行，title 标签会
		// 顺势落入 opening，玩家看到的就是"一进游戏就直接播剧情"。
		d.titleCall(name)
		return state.SessionAwaitTitle
	case keyword == bsMapKeyword:
		return d.TryOpenMap()
	case keyword == "target":
		// 学園2 周结的目标选择画面：脚本在这里停下，等引擎把玩家选的
		// #target 写回后再继续。没有对应布局的作品由 Session 直接放行。
		return state.SessionAwaitTarget
	case keyword == "staffroll":
		// 片尾字幕是阻塞画面：脚本要等字幕滚完/玩家跳过才继续回标题。
		return state.SessionAwaitStaffroll
	case strings.HasPrefix(keyword, "eyecatch"):
		// 章节间的过场是阻塞的短动画；原版播完才回 main。
		return state.SessionAwaitEyecatch
	case strings.HasPrefix(keyword, "set levelmeter"):
		// GK2IEDL 用它指定左下角 [extra window] 的等级变量（bsxx.dat 里是
		// `set levelmeter #hearts`）。名字后面带 "#变量" 时取变量名，没有就
		// 让面板按配置里的默认值走。
		if slot := strings.IndexByte(name, '#'); slot >= 0 {
			d.state.SetExtraMeterProperty(strings.TrimSpace(name[slot:]))
		}
		d.logOnce("system call '" + name + "' binds the extra window level meter")
		return state.SessionContinue
	case keyword == "op" && d.hasMovie(name):
		// 旧作标题 OP 是根目录的 op.avi，脚本用 system call "op" 请求播放。
		// 只有影片真的索引到才阻塞；没有影片的作品保持原来的"记录后继续"。
		d.requestMovie(name)
		return state.SessionAwaitMovie
	case isBsScreen(keyword):
		if d.screenHandler != nil {
			if step := d.screenHandler(keyword); step != state.SessionContinue {
				return step
			}
		}
		d.screenCall(name, keyword)
		return state.SessionContinue
	default:
		d.keywordCall(name, keyword)
		return state.SessionContinue
	}
}

func (d *BsSystemDirector) TryOpenMap() state.SessionStep {
	if InstalledMapCatalog == nil || InstalledMapCatalog.IsEmpty() {
		core.Logf("script: map 无候选地点，按原行为继续")
		return state.SessionContinue
	}
	return state.SessionAwaitMap
}

func (d *BsSystemDirector) titleCall(name string) {
	// 具体切标题的状态由 Session 在收到 SessionAwaitTitle 后完成：
	// 这里只负责让脚本停下，不能在回放/系统界面里直接改 GameState。
	d.logOnce("system call '" + name + "' requests the engine title screen")
}

func (d *BsSystemDirector) screenCall(name, keyword string) {
	d.logOnce("system call '" + name + "' requests the engine '" + keyword + "' screen, not implemented; continuing after the call")
}

// hasMovie 判断 system call 的名字是否对应一部已索引的影片。
//
// 只有影片真的存在才拦截：其他作品的 "op" 调用若没有影片文件，必须保持原来的
// "记录后继续"，否则会让本来不播片的地方停下来等宿主。
func (d *BsSystemDirector) hasMovie(name string) bool {
	_, ok := d.state.MoviePath(name)
	return ok
}

// requestMovie 把影片请求交给宿主；脚本随后停在 SessionAwaitMovie，等宿主回调 movieDone。
func (d *BsSystemDirector) requestMovie(name string) {
	d.state.SetMovie(name)
	d.state.SetMovieSerial(d.state.MovieSerial() + 1)
	d.logOnce("system call '" + name + "' plays a movie")
}

func isBsScreen(keyword string) bool {
	switch keyword {
	case "staffroll", "sceneroom", "higoroom", "target", "keikoku", "op":
		return true
	}
	return strings.HasPrefix(keyword, "eyecatch")
}

func (d *BsSystemDirector) seekInterfaceLabel(name string) bool {
	target := d.machine.Script().FindLabel(name)
	if target <= d.machine.InstructionStart() {
		return false
	}
	if !d.machine.SeekBs(target) {
		return false
	}
	core.Logf("script: system call '%s' -> label '%s' at %d", name, name, target)
	return true
}

func (d *BsSystemDirector) keywordCall(name, keyword string) {
	if strings.HasPrefix(keyword, "lx") {
		d.logOnce("system call '" + name + "' records the current progress point")
		return
	}
	if strings.HasPrefix(keyword, "xt") {
		d.logOnce("system call '" + name + "' sets the text speed to " + strconv.Itoa(bsTextSpeed(keyword)))
		return
	}
	if strings.HasPrefix(keyword, "xb") {
		d.logOnce("system call '" + name + "' writes a bookmark slot")
		return
	}
	if keyword == "xa" || keyword == "xe" || keyword == "xm" || keyword == "xr" {
		d.logOnce("system call '" + name + "' requests an engine sub-screen, not implemented; continuing")
		return
	}
	if keyword == "日付指定" || keyword == "時分指定" || keyword == "ウインドウポイント" {
		d.mirror(keyword)
		return
	}
	if d.applyZeturin(keyword) {
		return
	}
	if isBsEngineUpdate(keyword) {
		d.logOnce("system call '" + name + "' updates engine-side state, not implemented; continuing")
		return
	}
	// 剧本还有一种"场景标签调用"：SYSCALL "hrn_ev14_00" 不是宿主命令，而是让
	// 引擎先执行同名标签（结局、回想等场景）再继续，原版据此在片尾前播一段
	// 收尾剧情。只认 xxx_evNN_NN / xxx_mesNN_NN 这种带两位数字后缀的名字，
	// 否则任意标记名只要撞上后面的标签就会跳过大段剧情。
	if isBsSceneLabelCall(keyword) && d.seekInterfaceLabel(name) {
		return
	}
	d.logOnce("system call '" + name + "' is recorded as a script marker, continuing")
}

// isBsSceneLabelCall 判断系统调用名是不是"场景标签"。
//
// 形如 hrn_ev14_00 / mik_mes08_03：前缀 + _ev 或 _mes + 编号 + _两位数字。
// 只做形状判断，不查标签是否存在——查不到时 seekInterfaceLabel 会自己返回 false。
func isBsSceneLabelCall(keyword string) bool {
	if !strings.Contains(keyword, "_ev") && !strings.Contains(keyword, "_mes") {
		return false
	}
	underscore := strings.LastIndexByte(keyword, '_')
	if underscore <= 0 || len(keyword)-underscore != 3 {
		return false
	}
	return keyword[underscore+1] >= '0' && keyword[underscore+1] <= '9' &&
		keyword[underscore+2] >= '0' && keyword[underscore+2] <= '9'
}

func (d *BsSystemDirector) mirror(name string) {
	var pairs [][2]string
	switch name {
	case "日付指定":
		pairs = bsDateMirror
	case "時分指定":
		pairs = bsClockMirror
	default:
		pairs = bsPointMirror
	}
	applied := 0
	for _, pair := range pairs {
		sourceSlot := d.machine.Script().GetPropertyIndex(pair[0])
		destinationSlot := d.machine.Script().GetPropertyIndex(pair[1])
		if sourceSlot < 0 || destinationSlot < 0 {
			continue
		}
		d.machine.SetProperty(destinationSlot, d.machine.Property(sourceSlot))
		applied++
	}
	d.logOnce("system call '" + name + "' applied " + strconv.Itoa(applied) + " engine property mirror(s)")
}

func (d *BsSystemDirector) applyZeturin(keyword string) bool {
	if len(keyword) < 4 || keyword[0] != 'z' {
		return false
	}
	operation := keyword[1]
	if operation != '+' && operation != '-' && operation != '=' {
		return false
	}
	if keyword[3] != '0' {
		return false
	}
	digit := int(keyword[2] - '0')
	if digit != 0 && digit != 2 && digit != 4 && digit != 6 && digit != 8 {
		return false
	}
	slot := d.machine.Script().GetPropertyIndex(bsZeturinProperty)
	if slot < 0 {
		return false
	}
	amount := int32(bsZeturinStride * digit)
	current := d.machine.Property(slot)
	var value int32
	switch operation {
	case '+':
		value = current + amount
	case '-':
		value = current - amount
		if value < 0 {
			value = 0
		}
	default:
		value = amount
	}
	d.machine.SetProperty(slot, value)
	d.logOnce("system call '" + keyword + "' sets " + bsZeturinProperty + " " + strconv.Itoa(int(current)) + " -> " + strconv.Itoa(int(value)))
	return true
}

func isBsEngineUpdate(keyword string) bool {
	return strings.HasPrefix(keyword, "pts") ||
		strings.HasPrefix(keyword, "love") ||
		strings.HasPrefix(keyword, "scroll") ||
		strings.HasPrefix(keyword, "dress") ||
		strings.HasPrefix(keyword, "kd") ||
		strings.HasPrefix(keyword, "汁")
}

func bsTextSpeed(keyword string) int {
	if len(keyword) <= 2 {
		return bsTextSpeedDefault
	}
	value, err := strconv.Atoi(keyword[2:])
	if err != nil {
		return bsTextSpeedDefault
	}
	if value >= bsTextSpeedMinimum && value <= bsTextSpeedMaximum {
		return value
	}
	return bsTextSpeedDefault
}

func (d *BsSystemDirector) Dialog() state.SessionStep {
	d.state.SetMessageWindowVisible(false)
	core.Logf("script: dialog requested - modal name input is not implemented, continuing")
	return state.SessionContinue
}

func (d *BsSystemDirector) Halt() state.SessionStep {
	core.Logf("script: halt - the %s script finished", bsDialectName(d.script.Dialect()))
	return state.SessionFinished
}

func (d *BsSystemDirector) Restart() state.SessionStep {
	d.state.ResetForRestart()
	d.state.ClearBgm()
	d.state.ClearSoundEffect()
	d.state.SetEffectSerial(d.state.EffectSerial() + 1)
	d.state.ClearVoice()
	d.state.SetVoiceSerial(d.state.VoiceSerial() + 1)
	if !d.isAncient() {
		core.Logf("script: restart: the vm resumed at its own boot label")
		return state.SessionContinue
	}
	if !d.machine.SeekBsLabel(TitleLabel) {
		core.Logf("script: restart: label '%s' is missing", TitleLabel)
		return state.SessionContinue
	}
	core.Logf("script: restart -> '%s' @ %d", TitleLabel, d.machine.ProgramCounter())
	return state.SessionContinue
}

func (d *BsSystemDirector) isAncient() bool {
	switch d.script.Dialect() {
	case vmbs.BsScriptAncientWide, vmbs.BsScriptAncientCompact:
		return true
	}
	return false
}

func (d *BsSystemDirector) logOnce(message string) {
	if _, ok := d.logged[message]; ok {
		return
	}
	d.logged[message] = struct{}{}
	core.Logf("script: %s", message)
}

func bsDialectName(dialect vmbs.BsScriptDialect) string {
	switch dialect {
	case vmbs.BsScriptAncientWide:
		return "AncientWide"
	case vmbs.BsScriptAncientCompact:
		return "AncientCompact"
	case vmbs.BsScriptRev2:
		return "Rev2"
	case vmbs.BsScriptRev3:
		return "Rev3"
	case vmbs.BsScriptRev4:
		return "Rev4"
	case vmbs.BsScriptRev5:
		return "Rev5"
	case vmbs.BsScriptRev6:
		return "Rev6"
	default:
		return "Unknown"
	}
}

func (d *BsSystemDirector) closesMessageWindow() bool {
	if d.isAncient() {
		offset := d.machine.InstructionStart() + 1
		code := d.machine.Code()
		return offset < len(code) && code[offset] != 0
	}
	snapshot, ok := d.machine.CaptureRevState()
	if !ok {
		return false
	}
	slot := snapshot.StackTop + 1
	return slot >= 0 && slot < len(snapshot.Stack) && snapshot.Stack[slot] != 0
}
