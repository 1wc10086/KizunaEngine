// Package lua runs the Bishop "ex script" (Lua) layer used by the newer SLG
// titles: training, territory and day-result are full-screen Lua programs in
// embed.bsa, not script-drawn images.
//
// 设计边界：Lua 只负责界面与交互，数据/存档仍由 Go 侧持有。
//   - DIB 是资源的只读视图 + 目标画布，绘制直接落到引擎的 BGRA 画布；
//   - GetDB/SetDB/AddDB 走 brand/bishop/data/scriptdb（与剧本命令共用同一份表）；
//   - PrivateData/CommonData 在退出界面/存档时同步回 Go 的 map，由存档格式保存。
//
// 这样 Lua 脚本可以原样运行，而引擎不需要理解训练/领地的业务规则。
package lua

import (
	"time"

	lua "github.com/yuin/gopher-lua"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/data/scriptdb"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core/textrender"
)

// Host 是 Lua 层需要的剧本侧能力。窄接口让 lua 包不依赖 session，
// 也方便在没有完整 session 的测试里驱动。
type Host interface {
	Width() int
	Height() int
	// Backdrop 返回当前剧情画面，Lua 界面铺底用；返回 nil 时清黑。
	Backdrop() *gfx.BgraCanvas
	// Image 取游戏资源里的图片（BSA/散文件）。
	Image(name string) (*gfx.BsgImage, bool)
	// HasImage 只查资源是否存在，不解码；DIB.Create 用它决定返回 nil。
	HasImage(name string) bool
	// Script 取 embed 里的 Lua 源文件（带或不带 .lua 后缀）。
	Script(name string) ([]byte, bool)
	Fonts() *textrender.Renderer
	// Config 读 bsxx.cfg 的原始值（GetCFGString 用）。
	Config(section, key string) string
	// Property/SetProperty 读写剧本变量（#var）。
	Property(name string) int32
	SetProperty(name string, value int32)
	// DB/CSV 是 var_db 数值表与原始 CSV（OpenCSVDB 用）。
	DB() *scriptdb.DB
	CSV(name string) [][]string
	Achievements() *data.AchievementTable
	AchievementUnlocked(id string) bool
	UnlockAchievement(id string)
	// GalleryRate 返回鉴赏达成率（GetMuseumAchievementRate）。
	GalleryRate(kind string) int32
	// CallLabel 由 Lua 的 ScenarioLabelCall 触发，跳剧本标签。
	CallLabel(label string) bool
	// Notice 记录一行引擎日志（print）。
	Notice(text string)
}

// RuntimeConfig 是运行时的静态信息。
type RuntimeConfig struct {
	// ScriptExtension 是 [main window] ex script ext（通常是 lua）。
	ScriptExtension string
}

// Runtime 是一个游戏的 Lua 虚拟机。
type Runtime struct {
	L    *lua.LState
	host Host
	cfg  RuntimeConfig

	// stateTable/stateName 是当前激活的自绘界面（training/territory/day_result）。
	stateTable *lua.LTable
	stateName  string
	// target 是本次 draw 的目标画布，由 session 每帧传入。
	target *gfx.BgraCanvas

	loaded map[string]bool
	// pendingLabel 是 Lua 调 ScenarioLabelCall 后要跳转的剧本标签。
	pendingLabel string
	// pendingKey 是 PushKey 排队的按键，下一次 Update 时执行。
	pendingKey int
	// decision 表示当前指针事件是"确定"（ScrollBar 用它判断是否可拖）。
	decision bool
	// errorLogged 防止 draw/update 每帧报同一个错把日志刷爆。
	errorLogged bool
	// clock 从运行时创建开始计时，覆盖 os.clock（Lua 5.1 的 os.clock 是 CPU 时间，
	// day_result 用 os.clock 做演出计时，CPU 时间在真机上几乎不动）。
	clock time.Time
}

// New 创建运行时并执行 boot.lua。
//
// boot.lua 会 LoadExScript 出训练/领地/日结果/状态四个界面，并提前
// OpenCSVDB/DIB.Create。任何一步失败都不阻止游戏本体：返回 nil 时
// session 会退回原来的行为（不显示 Lua 界面）。
func New(host Host, cfg RuntimeConfig) *Runtime {
	if host == nil {
		return nil
	}
	L := lua.NewState()
	runtime := &Runtime{
		L:      L,
		host:   host,
		cfg:    cfg,
		loaded: make(map[string]bool),
		clock:  time.Now(),
	}
	runtime.register()
	boot, ok := host.Script("boot")
	if !ok {
		host.Notice("lua: boot script not found")
		L.Close()
		return nil
	}
	if err := L.DoString(string(boot)); err != nil {
		host.Notice("lua: boot failed: " + err.Error())
		L.Close()
		return nil
	}
	return runtime
}

// Close 释放 VM。
func (r *Runtime) Close() {
	if r == nil || r.L == nil {
		return
	}
	r.L.Close()
	r.L = nil
}

// StateName 返回当前界面名；空串表示没有激活的界面。
func (r *Runtime) StateName() string {
	if r == nil {
		return ""
	}
	return r.stateName
}

// Active 表示 Lua 是否接管了画面。
func (r *Runtime) Active() bool {
	return r != nil && r.stateTable != nil
}

// EnterState 激活 Lua 全局表 name，并调用它的 initialize(target)。
func (r *Runtime) EnterState(name string) bool {
	if r == nil || r.L == nil || name == "" {
		return false
	}
	value := r.L.GetGlobal(name)
	table, ok := value.(*lua.LTable)
	if !ok {
		r.host.Notice("lua: state '" + name + "' is not defined")
		return false
	}
	r.stateTable = table
	r.stateName = name
	r.errorLogged = false
	r.target = gfx.NewBgraCanvas(maxInt(r.host.Width(), 1), maxInt(r.host.Height(), 1))
	if backdrop := r.host.Backdrop(); backdrop != nil && backdrop.Width > 0 && backdrop.Height > 0 {
		r.target = gfx.NewBgraCanvas(backdrop.Width, backdrop.Height)
		r.target.CopyFrom(backdrop)
	}
	if err := r.callMethod(table, "initialize", r.canvasValue(r.target)); err != nil {
		r.host.Notice("lua: " + name + ":initialize failed: " + err.Error())
	}
	return true
}

// LeaveState 清掉当前界面（界面没有统一的 shutdown 钩子）。
func (r *Runtime) LeaveState() {
	if r == nil {
		return
	}
	r.stateTable = nil
	r.stateName = ""
	r.pendingKey = 0
	r.decision = false
}

// DrawOnto 把当前界面画到 session 的画布上。
//
// session 的画布已经是本帧剧情画面，Lua 的 draw 直接叠在上面；
// 不额外复制整屏，避免每帧多一次 1366x768 的 memcpy。
func (r *Runtime) DrawOnto(canvas *gfx.BgraCanvas) bool {
	if r == nil || r.stateTable == nil || canvas == nil {
		return false
	}
	r.target = canvas
	if err := r.callMethod(r.stateTable, "draw", r.canvasValue(canvas)); err != nil {
		if !r.errorLogged {
			r.errorLogged = true
			r.host.Notice("lua: " + r.stateName + ":draw failed: " + err.Error())
		}
		return false
	}
	return true
}

// Update 处理指针移动/点击。click=true 时在 update 之后调用 doaction(KEY_A)。
// 返回 exit 表示界面要求结束（进入剧本标签）。
func (r *Runtime) Update(x, y int, click bool) bool {
	if r == nil || r.stateTable == nil {
		return false
	}
	if err := r.callMethod(r.stateTable, "update", lua.LNumber(x), lua.LNumber(y)); err != nil {
		r.host.Notice("lua: " + r.stateName + ":update failed: " + err.Error())
	}
	if key := r.pendingKey; key != 0 {
		r.pendingKey = 0
		if r.doAction(key) {
			return true
		}
	}
	if click {
		r.decision = true
		exit := r.doAction(keyA)
		r.decision = false
		if exit {
			return true
		}
	}
	return false
}

// Drag 处理触屏拖动：ScrollBar 只有在 IsHitDecisionKey() 为真时才滚动。
func (r *Runtime) Drag(x, y int) bool {
	if r == nil || r.stateTable == nil {
		return false
	}
	r.decision = true
	defer func() { r.decision = false }()
	if err := r.callMethod(r.stateTable, "update", lua.LNumber(x), lua.LNumber(y)); err != nil {
		r.host.Notice("lua: " + r.stateName + ":update failed: " + err.Error())
	}
	return false
}

// Key 处理点击/右键/滚轮。KEY_A/KEY_B 的 true 返回值表示结束界面；
// WU/WD 只用于滚动，返回值忽略（原 Lua 对滚轮也返回 true）。
func (r *Runtime) Key(key int) bool {
	if r == nil || r.stateTable == nil {
		return false
	}
	if key == keyWU || key == keyWD {
		r.doAction(key)
		return false
	}
	return r.doAction(key)
}

func (r *Runtime) doAction(key int) bool {
	fn := r.L.GetField(r.stateTable, "doaction")
	if fn == lua.LNil {
		return false
	}
	if err := r.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, r.stateTable, lua.LNumber(key)); err != nil {
		r.host.Notice("lua: " + r.stateName + ":doaction failed: " + err.Error())
		return false
	}
	result := r.L.Get(-1)
	r.L.Pop(1)
	return lua.LVAsBool(result)
}

// PendingLabel 返回并清空 Lua 侧请求的剧本标签。
func (r *Runtime) PendingLabel() string {
	if r == nil {
		return ""
	}
	label := r.pendingLabel
	r.pendingLabel = ""
	return label
}

// HasFunction 判断 TRANSLATE 的字符串是不是 VM 里的 Lua 函数。
func (r *Runtime) HasFunction(statement string) bool {
	if r == nil || r.L == nil {
		return false
	}
	name := callName(statement)
	if name == "" {
		return false
	}
	return r.L.GetGlobal(name) != lua.LNil
}

// Call 执行一条 TRANSLATE 语句（函数调用或赋值）。
func (r *Runtime) Call(statement string) bool {
	if r == nil || r.L == nil || statement == "" {
		return false
	}
	if err := r.L.DoString(statement); err != nil {
		r.host.Notice("lua: translate '" + statement + "' failed: " + err.Error())
		return false
	}
	return true
}

// HasWindow 判断 [custom info windows] 的名称是否定义成 Lua 表。
func (r *Runtime) HasWindow(name string) bool {
	if r == nil || r.L == nil {
		return false
	}
	_, ok := r.L.GetGlobal(name).(*lua.LTable)
	return ok
}

// UpdateWindow 调用自定义信息窗的 update(target, store)。返回值忽略：
// 它只表示是否需要重绘，而 session 每帧都会重画。
func (r *Runtime) UpdateWindow(name string, canvas *gfx.BgraCanvas) {
	table := r.windowTable(name)
	if table == nil || canvas == nil {
		return
	}
	value := r.canvasValue(canvas)
	if err := r.callMethod(table, "update", value, value); err != nil {
		if !r.errorLogged {
			r.errorLogged = true
			r.host.Notice("lua: window '" + name + ":update' failed: " + err.Error())
		}
	}
}

// DrawWindow 调用自定义信息窗的 draw(target, x, y)。
func (r *Runtime) DrawWindow(name string, canvas *gfx.BgraCanvas, x, y int) bool {
	table := r.windowTable(name)
	if table == nil || canvas == nil {
		return false
	}
	// 原引擎在调用窗口 draw 之前先铺窗口底图：CHInfoWindow:draw 只画数值、
	// 标记和状态图标，self.base 是引擎按 base_pos 贴上去的。漏掉这一步
	// 就只剩图标和数字悬在黑底上。
	if base, ok := r.L.GetField(table, "base").(lua.LString); ok && string(base) != "" {
		if image, found := r.host.Image(string(base)); found && image != nil {
			baseX, baseY := x, y
			if pos, ok := r.L.GetField(table, "base_pos").(*lua.LTable); ok {
				baseX += int(tableNumber(pos, 1))
				baseY += int(tableNumber(pos, 2))
			}
			canvas.Draw(image, baseX, baseY)
		}
	}
	if err := r.callMethod(table, "draw", r.canvasValue(canvas), lua.LNumber(x), lua.LNumber(y)); err != nil {
		if !r.errorLogged {
			r.errorLogged = true
			r.host.Notice("lua: window '" + name + ":draw' failed: " + err.Error())
		}
		return false
	}
	return true
}

func tableNumber(table *lua.LTable, index int) float64 {
	if table == nil {
		return 0
	}
	if number, ok := table.RawGetInt(index).(lua.LNumber); ok {
		return float64(number)
	}
	return 0
}

func (r *Runtime) windowTable(name string) *lua.LTable {
	if r == nil || r.L == nil || name == "" {
		return nil
	}
	table, _ := r.L.GetGlobal(name).(*lua.LTable)
	return table
}

// SyncToGo 把 Lua 的 PrivateData/CommonData 同步到 Go map。
//
// boot.lua 的 StorePrivateData 接收一个表，把 PrivateData 深拷贝进去；
// 我们调用它后再把表转成 Go 结构，避免在 lua 包里实现两套序列化。
func (r *Runtime) SyncToGo(private, common map[string]any) {
	if r == nil || r.L == nil {
		return
	}
	if table := r.storeGlobal("StorePrivateData"); table != nil {
		mergeMap(private, tableToGo(table))
	}
	if table := r.storeGlobal("StoreCommonData"); table != nil {
		mergeMap(common, tableToGo(table))
	}
}

// RestoreFromGo 把 Go map 写回 Lua 的 PrivateData/CommonData。
func (r *Runtime) RestoreFromGo(private, common map[string]any) {
	if r == nil || r.L == nil {
		return
	}
	if len(private) > 0 {
		r.restoreGlobal("RestorePrivateData", goToTable(private))
	}
	if len(common) > 0 {
		r.restoreGlobal("RestoreCommonData", goToTable(common))
	}
}

// storeGlobal 调用 boot.lua 里的 StoreXxx(outTable)，返回填充后的表。
func (r *Runtime) storeGlobal(name string) *lua.LTable {
	fn := r.L.GetGlobal(name)
	if fn == lua.LNil {
		return nil
	}
	out := r.L.NewTable()
	if err := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, out); err != nil {
		r.host.Notice("lua: " + name + " failed: " + err.Error())
		return nil
	}
	return out
}

func (r *Runtime) restoreGlobal(name string, table *lua.LTable) {
	fn := r.L.GetGlobal(name)
	if fn == lua.LNil {
		return
	}
	if err := r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, table); err != nil {
		r.host.Notice("lua: " + name + " failed: " + err.Error())
	}
}

// callMethod 调用 table:name(args...)，忽略返回值。
func (r *Runtime) callMethod(table *lua.LTable, name string, args ...lua.LValue) error {
	fn := r.L.GetField(table, name)
	if fn == lua.LNil {
		return nil
	}
	callArgs := make([]lua.LValue, 0, len(args)+1)
	callArgs = append(callArgs, table)
	callArgs = append(callArgs, args...)
	return r.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, callArgs...)
}

// canvasValue 用 userdata 包一层画布，Lua 只把它传给绘制函数。
func (r *Runtime) canvasValue(canvas *gfx.BgraCanvas) lua.LValue {
	if canvas == nil {
		return lua.LNil
	}
	userdata := r.L.NewUserData()
	userdata.Value = &dib{canvas: canvas, width: canvas.Width, height: canvas.Height}
	return userdata
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
