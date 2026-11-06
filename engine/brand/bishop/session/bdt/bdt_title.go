package bdt

import "kizuna/engine/core"

// 标题界面是 BDT 引擎内置的功能，不是普通脚本画面：
//   - 脚本里单独一条 T 指令表示"打开引擎内置标题并等待玩家选择"，选 START 后 pc 已指向下一条，
//     直接继续执行即可；
//   - TM<n> 是标题音乐（第 n 轨，循环播放），不是等待。
//
// 这是 BSScript/BDT 脚本约定，按普通等待处理的话标题画面永远不会出现。
const bdtTitleBackground = "s_title1"

// 菜单命中矩形来自 grp.zbm 的 s_title3（纯色命中条）：7 行，每行高 36，行距 40。
const (
	bdtTitleX      = 50
	bdtTitleY      = 182
	bdtTitleWidth  = 200
	bdtTitleHeight = 36
	bdtTitleStep   = 40
)

type bdtTitleItem struct {
	key   string
	label string
}

// 顺序与 s_title2 上的文字一致。
var bdtTitleItems = []bdtTitleItem{
	{key: "start", label: "始めから"},
	{key: "load", label: "続きから"},
	{key: "config", label: "設定"},
	{key: "museum_music", label: "音楽鑑賞室"},
	{key: "museum_cg", label: "CG鑑賞室"},
	{key: "museum_scene", label: "シーン鑑賞室"},
	{key: "quit", label: "終了"},
}

var _ core.TitleMenuExposer = (*Session)(nil)

// beginTitle 进入内置标题：记下剧情的窗口与消息框状态，清空人物和消息，再把 s_title1 铺成全屏。
// 剧情背景名不改动，离开标题时由 leaveTitle 原样恢复。
func (s *Session) beginTitle() core.Step {
	if s.sys != nil {
		s.sys.kind = bdtSysNone
	}
	if s.manin != nil {
		// 标题是剧情的"暂停"而不是新周目：词库/站位保留，回来继续用。
		s.manin.screen = maninScreenNone
		s.manin.prevScreen = maninScreenNone
		s.manin.attack = maninAttackState{}
		s.manin.org = maninOrganizerState{}
		s.manin.face = ""
	}
	if s.titleMode {
		return core.StepAwaitTitle
	}
	s.titleMode = true
	s.titleSavedWindows = s.stage.Windows()
	s.titleSavedMessage = s.gameState.MessageWindowVisible()
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.SetSpeaker("")
	s.gameState.SetText("")
	s.stage.ClearSprites()
	s.stage.SetFace("")
	s.stage.ShowMessage("", "")
	for part := 0; part < 3; part++ {
		s.stage.SetWindow(part, false)
	}
	s.stage.SetBackground(bdtTitleBackground, 0)
	core.Logf("bdt: title screen")
	return core.StepAwaitTitle
}

// leaveTitle 退出内置标题，恢复标题前的人物、背景、窗口与消息框显示状态。
// s.sprites 和 s.backgroundName 在标题期间没有被改动，直接回放即可。
func (s *Session) leaveTitle() {
	if !s.titleMode {
		return
	}
	s.titleMode = false
	s.restoreLayers()
	s.stage.SetBackground(s.backgroundName, 0)
	for part, visible := range s.titleSavedWindows {
		s.stage.SetWindow(part, visible)
	}
	s.gameState.SetMessageWindowVisible(s.titleSavedMessage)
}

// TitleMenu 暴露标题菜单矩形给宿主；Flutter 按引擎坐标缩放。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	if s.disposed || !s.titleMode || s.bdtSystemActive() {
		return nil
	}
	if s.manin != nil {
		return s.maninTitleItems()
	}
	items := make([]core.TitleMenuItem, 0, len(bdtTitleItems))
	for index, item := range bdtTitleItems {
		items = append(items, core.TitleMenuItem{
			Key:     item.key,
			Label:   item.label,
			X:       bdtTitleX,
			Y:       int32(bdtTitleY + bdtTitleStep*index),
			Width:   bdtTitleWidth,
			Height:  bdtTitleHeight,
			Enabled: true,
		})
	}
	return items
}

func (s *Session) titleKeyAt(x, y int32) string {
	for _, item := range s.TitleMenu() {
		if x >= item.X && x < item.X+int32(item.Width) && y >= item.Y && y < item.Y+int32(item.Height) {
			return item.Key
		}
	}
	return ""
}

// SelectTitleItem 处理标题菜单选择。只有 START 会离开标题继续剧情；
// 读档、设置、退出是宿主界面，引擎保持在标题上，由宿主处理完再回到这里。
func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil {
		return core.StepFaulted, errBdtNotOpen
	}
	if !s.titleMode {
		return s.currentStep(), nil
	}
	if s.manin != nil {
		return s.maninSelectTitleItem(key)
	}
	switch key {
	case "start":
		s.leaveTitle()
		s.machine.LeaveTitle()
		return s.runSteps(defaultRunBudget), nil
	case "load":
		// 続きから：打开原版的 呼出帳（s_sys011），不再退回 Flutter 的存读档面板。
		s.openSystemUI(bdtSysLoad)
	case "config":
		// 設定：打开原版的 音環境設定（s_sys012）；点击标题可在音/画面间切换。
		s.openSystemUI(bdtSysConfigSound)
	case "museum_music":
		s.openSystemUI(bdtSysMusic)
	case "museum_cg":
		s.openSystemUI(bdtSysCG)
	case "museum_scene":
		s.openSystemUI(bdtSysScene)
	case "quit":
		s.setHostRequest("exit")
	default:
		core.Logf("bdt: unknown title item '%s'", key)
	}
	return core.StepAwaitTitle, nil
}

func (s *Session) setHostRequest(kind string) {
	s.hostRequest = kind
	s.hostRequestSerial++
	core.Logf("bdt: host request '%s' #%d", kind, s.hostRequestSerial)
}
