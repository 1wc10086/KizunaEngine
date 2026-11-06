package session

import (
	"strings"

	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/data/scriptdb"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/lua"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

// slg_lua.go 把 embed.bsa 的 Lua 界面接到 Session 上。
//
// 剧本用 SELECT_TAIL "state training|territory|day_result" 进入原版自绘界面：
// 这三个名字对应 Lua 全局表，表提供 initialize/update/draw/doaction。
// 引擎在界面激活期间暂停脚本、每帧 draw、把指针事件转成 update/doaction；
// 界面要求结束时跳到同名剧本标签（label "state training" 等）继续剧情。
//
// 数据/存档的所有权仍在 Go：Lua 的 PrivateData/CommonData 在退出界面和存档时
// 同步回 Session 的 map；DB 直接共用 scriptdb，不复制第二份。

type slgLuaHost struct {
	session *Session
}

func (h *slgLuaHost) Width() int  { return h.session.width }
func (h *slgLuaHost) Height() int { return h.session.height }

func (h *slgLuaHost) Backdrop() *gfx.BgraCanvas {
	if h.session.composer == nil {
		return nil
	}
	return h.session.composer.Frame()
}

func (h *slgLuaHost) Image(name string) (*gfx.BsgImage, bool) {
	if h.session.composer == nil {
		return nil, false
	}
	image := h.session.composer.Load(name)
	return image, image != nil
}

func (h *slgLuaHost) HasImage(name string) bool {
	if h.session.res == nil || name == "" {
		return false
	}
	_, ok := h.session.res.TryGet(name)
	return ok
}

func (h *slgLuaHost) Script(name string) ([]byte, bool) {
	s := h.session
	if s.res == nil || name == "" {
		return nil, false
	}
	for _, candidate := range []string{name, name + ".lua", strings.ToLower(name) + ".lua"} {
		if location, ok := s.res.TryGet(candidate); ok && location != nil {
			return location.Read(), true
		}
	}
	return nil, false
}

func (h *slgLuaHost) Fonts() *textrender.Renderer { return h.session.textFont }

func (h *slgLuaHost) Config(section, key string) string {
	if h.session.config == nil {
		return ""
	}
	return h.session.config.Raw(section, key, "")
}

func (h *slgLuaHost) Property(name string) int32 {
	s := h.session
	if s.script == nil || s.machine == nil {
		return 0
	}
	slot := s.script.GetPropertyIndex(name)
	if slot < 0 {
		return 0
	}
	return s.machine.Property(slot)
}

func (h *slgLuaHost) SetProperty(name string, value int32) {
	s := h.session
	if s.script == nil || s.machine == nil {
		return
	}
	slot := s.script.GetPropertyIndex(name)
	if slot < 0 {
		return
	}
	s.machine.SetProperty(slot, value)
}

func (h *slgLuaHost) DB() *scriptdb.DB { return h.session.ensureScriptDB() }

func (h *slgLuaHost) CSV(name string) [][]string {
	content, ok := h.session.resourceReader()(name)
	if !ok {
		return nil
	}
	return scriptdb.ParseCSV(content)
}

func (h *slgLuaHost) Achievements() *data.AchievementTable {
	return h.session.ensureAchievements()
}

func (h *slgLuaHost) AchievementUnlocked(id string) bool {
	return h.session.achievementUnlocked(id)
}

func (h *slgLuaHost) UnlockAchievement(id string) {
	h.session.unlockAchievement(id)
}

func (h *slgLuaHost) GalleryRate(kind string) int32 {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "cg", "strip":
		return int32(h.session.cgUnlockPercent())
	case "scene":
		return int32(h.session.sceneUnlockPercent())
	}
	return 0
}

func (h *slgLuaHost) CallLabel(label string) bool {
	if h.session.machine == nil || label == "" {
		return false
	}
	return h.session.machine.SeekLabel(label)
}

func (h *slgLuaHost) Notice(text string) {
	core.Logf("%s", text)
}

// openSLGLua 在 Open 时尝试启动 Lua。没有 embed 脚本的作品直接跳过。
func (s *Session) openSLGLua() {
	if s.res == nil {
		return
	}
	if _, ok := s.res.TryGet("boot"); !ok {
		if _, ok := s.res.TryGet("boot.lua"); !ok {
			return
		}
	}
	s.slgVM = lua.New(&slgLuaHost{session: s}, lua.RuntimeConfig{})
	if s.slgVM == nil {
		core.Logf("lua: ex script runtime unavailable")
		return
	}
	if len(s.privateData) > 0 || len(s.commonData) > 0 {
		s.slgVM.RestoreFromGo(s.privateData, s.commonData)
	}
	// [custom info windows] 0x=*cs1 这类窗口不是 boot 加载的，按配置补加载。
	for _, name := range s.config.CustomInfoWindows {
		if s.slgVM.LoadScript(name) && s.slgVM.HasWindow(name) {
			s.customWindows = append(s.customWindows, name)
		}
	}
	core.Logf("lua: ex script runtime ready (%d custom window(s))", len(s.customWindows))
}

// activateSLGState 进入 Lua 自绘界面。
func (s *Session) activateSLGState(name string) bool {
	if s.slgVM == nil {
		return false
	}
	if s.slgState != nil {
		// 同一个界面重复进入（脚本里的自循环）不需要重新 initialize。
		return s.slgState.name == name
	}
	if s.gameState != nil {
		// Lua 界面是全屏模态：隐藏剧情对话框，否则底图透明处会透出上一句，
		// 弹幕/回想按钮的热区也会和 Lua 的坐标重叠。
		s.gameState.SetText("")
		s.gameState.SetSpeaker("")
		s.gameState.SetMessageWindowVisible(false)
		s.gameState.ClearChoices()
	}
	s.menuVisible = false
	if !s.slgVM.EnterState(name) {
		return false
	}
	s.slgState = &slgScreenState{name: name}
	s.slgExit = false
	s.lastStep = core.StepAwaitInput
	s.composer.Invalidate()
	core.Logf("slg: lua state '%s' active", name)
	return true
}

// finishSLGState 结束界面并把脚本跳到同名标签。
func (s *Session) finishSLGState() state.SessionStep {
	ui := s.slgState
	if ui == nil {
		return state.SessionContinue
	}
	label := ""
	if s.slgVM != nil {
		label = s.slgVM.PendingLabel()
		if s.privateData == nil {
			s.privateData = make(map[string]any)
		}
		if s.commonData == nil {
			s.commonData = make(map[string]any)
		}
		s.slgVM.SyncToGo(s.privateData, s.commonData)
		s.slgVM.LeaveState()
	}
	s.slgState = nil
	s.slgExit = false
	// `state X` 是**阻塞调用**，不是跳转：VM 在 Fetch 时已经把 PC 移到
	// SELECT_TAIL 之后。训练界面的下一段是 handler 标签（NextTraining 等），
	// 领地的下一段是 RET，日结果是收尾演出——继续执行就是原引擎的行为。
	// 只有 Lua 的 NextTraining 显式要求跳某个场景标签时才 Seek。
	if label != "" {
		if s.machine == nil || !s.machine.SeekLabel(label) {
			core.Logf("slg: state '%s' requested label '%s' but it is missing", ui.name, label)
			return state.SessionContinue
		}
		core.Logf("slg: state '%s' -> label '%s'", ui.name, label)
	} else {
		core.Logf("slg: state '%s' finished, resuming script", ui.name)
	}
	s.composer.Invalidate()
	return s.run(commandBudget)
}

// pointerSLGState 把指针事件转成 Lua 的 update/doaction。
func (s *Session) pointerSLGState(op uint32, x, y int32) (core.Step, error) {
	if s.slgVM == nil {
		return s.lastStep, nil
	}
	switch op {
	case 0:
		if s.slgVM.Update(int(x), int(y), false) {
			return s.finishRun(s.finishSLGState()), nil
		}
	case 1, 3:
		if s.slgVM.Update(int(x), int(y), true) {
			return s.finishRun(s.finishSLGState()), nil
		}
	case 4:
		if s.slgVM.Key(0x42) { // KEY_B
			return s.finishRun(s.finishSLGState()), nil
		}
	case 6:
		s.slgVM.Drag(int(x), int(y))
	}
	s.composer.Invalidate()
	return s.lastStep, nil
}

// drawSLGState 在每帧合成时驱动 Lua 的 update/draw。
//
// 这里调用 update 是为了 day_result 的计时演出：宿主在没有输入时仍然会
// 以 Animating 为真持续合成，update 每帧推进 os.clock，动画才会走。
func (s *Session) drawSLGState(canvas *gfx.BgraCanvas) bool {
	if s.slgState == nil || s.slgVM == nil {
		return false
	}
	if s.slgVM.Update(s.lastPointerX, s.lastPointerY, false) {
		s.slgExit = true
	}
	if s.slgExit {
		s.finishSLGState()
		return true
	}
	if s.slgVM.DrawOnto(canvas) {
		return true
	}
	return false
}

// syncSLGData 在存档前把 Lua 的 PrivateData/CommonData 拉到 Go map。
func (s *Session) syncSLGData() {
	if s.slgVM == nil {
		return
	}
	if s.privateData == nil {
		s.privateData = make(map[string]any)
	}
	if s.commonData == nil {
		s.commonData = make(map[string]any)
	}
	s.slgVM.SyncToGo(s.privateData, s.commonData)
}
