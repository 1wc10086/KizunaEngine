package tj2

import (
	"fmt"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// 经典分支（GK CD 等）的系统界面全部在引擎侧绘制。
//
// 为什么不让 Flutter 画：标题底图、SP09 信息框、鉴赏室底图都是游戏自己的
// 640x480 素材，Flutter 侧只拿到一个整数大小的帧位图；把坐标/命中再做一遍
// 既会重复实现，也会随缩放产生错位。这里遵守和现代 bishop 分支相同的约定：
//   - 引擎把界面画进帧；
//   - 宿主只负责把点击坐标转发到 Pointer；
//   - 需要停在界面时返回 SessionAwaitSelect，脚本不会继续跑。
type tj2UIMode int

const (
	tj2UINone tj2UIMode = iota
	tj2UISave
	tj2UILoad
	tj2UIGallery
	tj2UICG
	tj2UISound
	tj2UIScene
	// Toku2/TJ2.EXE 风格的系统层：标题菜单之外的鉴赏入口、
	// 游戏内系统菜单、配置和回想。它们和上面的 GK 界面共用槽位/翻页
	// 状态机，但布局与素材按 profile 分流。
	tj2UIGalleryRoot
	tj2UISystem
	tj2UIConfig
	tj2UIBacklog
)

const (
	tj2UISavePageSlots = 12
	tj2UISaveColumns   = 3
	tj2UISaveRows      = 4
	tj2UICGGridColumns = 6
	tj2UICGGridRows    = 4
	tj2UICGPerPage     = tj2UICGGridColumns * tj2UICGGridRows
	tj2UIScenePerPage  = 6
	tj2UISceneFirst    = 5
	tj2UISceneLast     = 42
)

const (
	tj2UIColorWhite = 0xFFFFFFFF
	tj2UIColorCream = 0xFFFFE8C0
	tj2UIColorGold  = 0xFFC8A060
	tj2UIColorDim   = 0xFFB0A090
)

// 标题四个热区。文字已经烘焙在 TITLE.KG 上，热区只需要覆盖文字本身，
// 坐标按 640x480 原图量出（初めから y≈248、続きから y≈291、
// 鑑賞室 y≈334、終了 y≈377）。
type tj2TitleItem struct {
	Key   string
	Label string
	X     int
	Y     int
	W     int
	H     int
}

var tj2TitleItems = []tj2TitleItem{
	{Key: "start", Label: "初めから", X: 60, Y: 238, W: 200, H: 44},
	{Key: "load", Label: "続きから", X: 60, Y: 281, W: 200, H: 44},
	{Key: "museum", Label: "鑑賞室", X: 60, Y: 324, W: 200, H: 44},
	{Key: "quit", Label: "終了", X: 60, Y: 367, W: 200, H: 44},
}

// 信息框右下角的四个按钮。SP09 的顺序是 HIDE/SAVE/LOAD/SKIP；
// Toku2 的 sp_mes1 顺序是 SKIP/SAVE/LOAD/LOG，由 messageButtonRects 分流。
const (
	tj2MessageButtonHide = iota + 1
	tj2MessageButtonSave
	tj2MessageButtonLoad
	tj2MessageButtonSkip
	tj2MessageButtonLog
)

type tj2Rect struct{ X, Y, W, H int }

var tj2MessageButtonRects = map[int]tj2Rect{
	tj2MessageButtonHide: {X: 590, Y: 346, W: 46, H: 26},
	tj2MessageButtonSave: {X: 590, Y: 373, W: 46, H: 27},
	tj2MessageButtonLoad: {X: 590, Y: 401, W: 46, H: 27},
	tj2MessageButtonSkip: {X: 590, Y: 429, W: 46, H: 27},
}

var tj2GalleryItemRects = []tj2Rect{
	{X: 160, Y: 144, W: 320, H: 56},
	{X: 160, Y: 224, W: 320, H: 56},
	{X: 160, Y: 304, W: 320, H: 56},
	{X: 160, Y: 416, W: 320, H: 48},
}

var (
	tj2CGPrevRect = tj2Rect{X: 130, Y: 415, W: 100, H: 50}
	tj2CGExitRect = tj2Rect{X: 275, Y: 415, W: 80, H: 50}
	tj2CGNextRect = tj2Rect{X: 415, Y: 415, W: 100, H: 50}

	tj2SoundPlayRect = tj2Rect{X: 335, Y: 415, W: 90, H: 50}
	tj2SoundStopRect = tj2Rect{X: 430, Y: 415, W: 90, H: 50}
	tj2SoundExitRect = tj2Rect{X: 525, Y: 415, W: 80, H: 50}

	tj2SavePrevRect = tj2Rect{X: 150, Y: 428, W: 80, H: 30}
	tj2SaveNextRect = tj2Rect{X: 250, Y: 428, W: 80, H: 30}
	tj2SaveBackRect = tj2Rect{X: 410, Y: 428, W: 80, H: 30}
)

func tj2Hit(x, y int, rect tj2Rect) bool {
	return x >= rect.X && x < rect.X+rect.W && y >= rect.Y && y < rect.Y+rect.H
}

func (s *Tj2Session) classicMessageFrameVisible() bool {
	if !s.UsesEngineUI() {
		return s.gameState.ShowMessageFrame()
	}
	return !s.titleMode && s.uiMode == tj2UINone && !s.warnVisible && s.gameState.MessageWindowVisible()
}

func (s *Tj2Session) messageWindowVisibleForHost() bool {
	if s.UsesEngineUI() {
		return s.classicMessageFrameVisible()
	}
	return s.gameState.MessageWindowVisible()
}

func (s *Tj2Session) TitleVisible() bool {
	return s.titleMode
}

func (s *Tj2Session) classicUIWaitStep() state.SessionStep {
	// 标题/系统界面统一用 AwaitSelect：宿主不会画选择肢，但会保持
	// 帧合成并转发点击；AwaitChoice 会触发宿主的选项浮层，盖住引擎画的界面。
	return state.SessionAwaitSelect
}

func (s *Tj2Session) classicSystemUIVisible() bool {
	return s.uiMode != tj2UINone
}

// titleMenuItemsForProfile 返回当前标题的按钮。
//
// 特別授業2 的按钮烘焙在 title03 里（x=464，y=256 起，每项 48px）；
// GK 光盘系沿用旧的左侧坐标。两套 key 相同，宿主只按矩形铺热区。
func (s *Tj2Session) titleMenuItemsForProfile() []tj2TitleItem {
	if !s.IsToku2UI() {
		return tj2TitleItems
	}
	return []tj2TitleItem{
		{Key: "start", Label: "初めから", X: 464, Y: 256, W: 160, H: 40},
		{Key: "load", Label: "続きから", X: 464, Y: 304, W: 160, H: 40},
		{Key: "museum", Label: "鑑賞室", X: 464, Y: 352, W: 160, H: 40},
		{Key: "quit", Label: "終了", X: 464, Y: 400, W: 160, H: 40},
	}
}

// TitleMenu 返回主界面菜单项。宿主只拿矩形铺热区，画面由引擎合成。
func (s *Tj2Session) TitleMenu() []core.TitleMenuItem {
	if !s.UsesEngineUI() || !s.titleMode {
		return nil
	}
	items := s.titleMenuItemsForProfile()
	out := make([]core.TitleMenuItem, 0, len(items))
	for _, entry := range items {
		enabled := true
		if s.IsToku2UI() && entry.Key == "load" {
			// 原版是单存档（GAMEFLG.DAT），没有存档时“続きから”不可点。
			enabled = s.QuickSaveExists()
		}
		out = append(out, core.TitleMenuItem{
			Key:     entry.Key,
			Label:   entry.Label,
			X:       int32(entry.X),
			Y:       int32(entry.Y),
			Width:   uint32(entry.W),
			Height:  uint32(entry.H),
			Enabled: enabled,
		})
	}
	return out
}

// SelectTitleItem 分派标题点击。
//
// 特別授業2 的标题是 EXE 自己的界面，不写脚本 flag：四个动作分别由
// 引擎打开警告/读档/鉴赏/退出。GK 光盘系仍沿用"写 flag[0] 让脚本分派"。
func (s *Tj2Session) SelectTitleItem(key string) state.SessionStep {
	if !s.UsesEngineUI() || !s.titleMode || s.machine == nil {
		return s.classicUIWaitStep()
	}
	if s.IsToku2UI() {
		return s.selectToku2TitleItem(key)
	}
	index := -1
	for i, entry := range tj2TitleItems {
		if entry.Key == key {
			index = i
			break
		}
	}
	if index < 0 {
		return state.SessionAwaitTitle
	}
	if err := s.machine.WriteFlag(0, int32(index)); err != nil {
		core.Logf("tj2: title select %s flag0 failed: %v", key, err)
		return state.SessionAwaitTitle
	}
	s.titleMode = false
	s.titleHover = -1
	s.stage.Invalidate()
	s.gameState.SetAwaitingInput(false)
	core.Logf("tj2: title select %s -> flag0=%d", key, index)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) SetTitleHover(x, y int) {
	hover := -1
	for i, entry := range s.titleMenuItemsForProfile() {
		if tj2Hit(x, y, tj2Rect{X: entry.X, Y: entry.Y, W: entry.W, H: entry.H}) {
			hover = i
			break
		}
	}
	s.titleHover = hover
	s.stage.Invalidate()
}

// enterClassicTitle 在 func16 处停住并切到标题。
func (s *Tj2Session) enterClassicTitle() state.SessionStep {
	s.titleMode = true
	s.titleHover = -1
	s.uiMode = tj2UINone
	s.messagePages = nil
	s.messagePage = 0
	s.menuVisible = false
	s.gameState.SetText("")
	s.gameState.SetAwaitingInput(false)
	s.gameState.SetMessageWindowVisible(false)
	s.gameState.ClearChoices()
	// 标题画面没有 BGM。返回标题（从鉴赏室/结局）时残留的剧情 BGM
	// 不应该继续压在 TITLE.KG 上。
	s.gameState.ClearBgm()
	s.bgmTrack = -1
	s.stage.Invalidate()
	core.Logf("tj2: classic title menu tick=%d", s.tickCounter)
	return state.SessionAwaitTitle
}

// ---- 消息框按钮 ----

// messageButtonRects 返回当前 profile 的按钮命中区。
//
// 特別授業2 的 sp_mes1 是 608x112 的整体图，按钮烘焙在右侧：
// 图内 x≈570..600、y≈6/32/58/82，整图放在 (16,352)，所以绝对坐标
// 从 (586,358) 起、每行 24px。GK 的 SP09 部件表位置不同，保留旧值。
func (s *Tj2Session) messageButtonRects() map[int]tj2Rect {
	if !s.IsToku2UI() {
		return tj2MessageButtonRects
	}
	return map[int]tj2Rect{
		tj2MessageButtonSkip: {X: 586, Y: 358, W: 32, H: 24},
		tj2MessageButtonSave: {X: 586, Y: 382, W: 32, H: 24},
		tj2MessageButtonLoad: {X: 586, Y: 406, W: 32, H: 24},
		tj2MessageButtonLog:  {X: 586, Y: 430, W: 32, H: 24},
	}
}

func (s *Tj2Session) messageButtonAt(x, y int) int {
	for button, rect := range s.messageButtonRects() {
		if tj2Hit(x, y, rect) {
			return button
		}
	}
	return 0
}

func (s *Tj2Session) drawMessageButtonHover() {
	if s.messageHover == 0 || !s.classicMessageFrameVisible() {
		return
	}
	rect, ok := s.messageButtonRects()[s.messageHover]
	if !ok {
		return
	}
	if s.IsToku2UI() {
		// sp_mes2 是 72x94 的双列按钮表：左列常态、右列红字悬停。
		// 每个格子 36x23.5，取右列对应行缩小到按钮区，避免压到边框。
		overlay := s.images.Get("sp_mes2")
		if overlay == nil {
			return
		}
		row := 0
		switch s.messageHover {
		case tj2MessageButtonSkip:
			row = 0
		case tj2MessageButtonSave:
			row = 1
		case tj2MessageButtonLoad:
			row = 2
		case tj2MessageButtonLog:
			row = 3
		}
		srcY := row * overlay.Height / 4
		srcH := overlay.Height / 4
		// 按按钮矩形缩放，避免 36px 宽的格子压到窗框上留下绿边。
		s.frame.BlitScaledRegion(overlay, rect.X, rect.Y, rect.W, rect.H, 255,
			overlay.Width/2, srcY, overlay.Width/2, srcH)
		return
	}
	// SP09 的按钮没有悬停帧，用一层半透明蓝提示可点。
	s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 70, 70)
}

func (s *Tj2Session) messageButtonAction(button int) state.SessionStep {
	switch button {
	case tj2MessageButtonHide:
		s.gameState.SetMessageWindowVisible(false)
		s.stage.Invalidate()
		return state.SessionAwaitInput
	case tj2MessageButtonSave:
		if s.IsToku2UI() {
			return s.quickSave()
		}
		return s.openClassicSave(false)
	case tj2MessageButtonLoad:
		if s.IsToku2UI() {
			return s.quickLoad()
		}
		return s.openClassicLoad(false)
	case tj2MessageButtonSkip:
		return s.Skip()
	case tj2MessageButtonLog:
		if s.IsToku2UI() {
			return s.openBacklog()
		}
		return state.SessionAwaitInput
	default:
		return state.SessionAwaitInput
	}
}

// Skip 连续推进到下一个需要玩家决定的节点。
//
// 原版的 SKIP 在"下次选择肢/地图/未读"处停；这里没有未读表，所以停在
// AwaitChoice/AwaitSelect/影片/结束时。上限 600 次是兜底：坏脚本里的
// 无输入死循环不能把宿主卡住。
func (s *Tj2Session) Skip() state.SessionStep {
	if s.uiMode != tj2UINone || s.titleMode {
		return s.classicUIWaitStep()
	}
	step := state.SessionAwaitInput
	for i := 0; i < 600; i++ {
		step = s.Advance()
		switch step {
		case state.SessionAwaitInput, state.SessionWait:
			continue
		default:
			core.Logf("tj2: skip stopped at %d after %d steps", int(step), i+1)
			return step
		}
	}
	core.Logf("tj2: skip hit budget")
	return step
}

// ---- 打开/关闭系统界面 ----

func (s *Tj2Session) beginClassicUI(mode tj2UIMode, resume bool) {
	s.uiMode = mode
	s.uiPage = 0
	s.uiHover = -1
	s.uiSelected = -1
	s.uiCgOpen = false
	s.uiResumeRun = resume
	s.titleMode = false
	s.titleHover = -1
	s.uiRevision++
	s.uiComposedKey = -1
	s.stage.Invalidate()
	core.Logf("tj2: classic UI open mode=%d resume=%v tick=%d", int(mode), resume, s.tickCounter)
}

func (s *Tj2Session) captureClassicSaveThumb() {
	// 缩略图必须取"打开存档画面之前"的那一帧，否则会把存档界面本身存进去。
	if s.frame == nil {
		return
	}
	s.stage.ComposeTo(s.frame, s.classicMessageFrameVisible())
	if s.classicMessageFrameVisible() {
		s.drawClassicMessageText()
	}
	s.uiSaveThumb = s.frame.ScaleTo(s.profile.SaveThumbnailWidth, s.profile.SaveThumbnailHeight)
}

func (s *Tj2Session) openClassicSave(resume bool) state.SessionStep {
	s.captureClassicSaveThumb()
	s.beginClassicUI(tj2UISave, resume)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) openClassicLoad(resume bool) state.SessionStep {
	s.beginClassicUI(tj2UILoad, resume)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) openClassicGallery() state.SessionStep {
	s.beginClassicUI(tj2UIGallery, true)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) openClassicCG() state.SessionStep {
	s.classicThumbNames()
	// 原版 CG 鉴赏只显示"看过"的缩略图，解锁状态在全局存档里。
	// 旧作没有可读的全局存档格式，func21 的编号也不是缩略图下标；
	// 与其显示一个全空的鉴赏室，这里列出全部缩略图，保证功能可用。
	s.beginClassicUI(tj2UICG, true)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) openClassicSound() state.SessionStep {
	if s.table != nil {
		s.soundTracks = append([]string(nil), s.table.TrackNumbers()...)
	}
	s.beginClassicUI(tj2UISound, true)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) openClassicScene() state.SessionStep {
	if s.sceneTitles == nil {
		titles := make([]string, 0, tj2UISceneLast-tj2UISceneFirst+1)
		for number := tj2UISceneFirst; number <= tj2UISceneLast; number++ {
			title := s.sceneChapter(number)
			if len(title) == 0 {
				title = fmt.Sprintf("シーン %d", number)
			}
			titles = append(titles, title)
		}
		s.sceneTitles = titles
	}
	s.beginClassicUI(tj2UIScene, true)
	return s.classicUIWaitStep()
}

func (s *Tj2Session) closeClassicUI(resume bool) state.SessionStep {
	s.uiMode = tj2UINone
	s.uiHover = -1
	s.uiCgOpen = false
	s.uiRevision++
	s.uiComposedKey = -1
	s.uiSaveThumb = nil
	s.stage.Invalidate()
	core.Logf("tj2: classical UI close resume=%v", resume)
	// 从标题/系统菜单打开的存读档，关闭时必须回到打开它的界面，
	// 不能直接 resume：标题状态下脚本还停在 SCN000 开头。
	if s.uiReturnSystem {
		s.uiReturnSystem = false
		s.uiMode = tj2UISystem
		return state.SessionAwaitSelect
	}
	if s.uiReturnTitle {
		s.uiReturnTitle = false
		return s.EnterTitle()
	}
	if resume {
		s.gameState.SetAwaitingInput(false)
		return s.Run(tj2RunBudget)
	}
	return state.SessionAwaitInput
}

func (s *Tj2Session) finishClassicLoad() state.SessionStep {
	s.uiMode = tj2UINone
	s.uiHover = -1
	s.uiCgOpen = false
	s.uiRevision++
	s.uiComposedKey = -1
	s.uiSaveThumb = nil
	s.titleMode = false
	s.uiReturnTitle = false
	s.uiReturnSystem = false
	// messagePages/MessagePage 已由 LoadFrom 按存档恢复，不能在这里清掉，
	// 否则在长对白第 2 页存档、读档后会丢掉第 3 页之后的内容。
	s.gameState.SetAwaitingInput(false)
	s.stage.Invalidate()
	return state.SessionAwaitInput
}

func (s *Tj2Session) writeClassicFlag(index, value int) {
	if s.machine == nil {
		return
	}
	if err := s.machine.WriteFlag(index, int32(value)); err != nil {
		core.Logf("tj2: classic flag %d write failed: %v", index, err)
	}
}

// ---- 指针事件 ----

// classicPointer 处理标题/系统界面/信息框按钮。返回 handled=false 时
// 调用方继续走原有的地图/推进逻辑。
func (s *Tj2Session) classicPointer(op uint32, x, y int) (state.SessionStep, bool) {
	// 纯现代分支（宿主画对话框/选项）保持原样：这里整段跳过，
	// 否则 ShowMessageFrame 默认 true 会把普通点击吞掉。
	if !s.UsesEngineUI() {
		return 0, false
	}
	if s.warnVisible {
		// 警告画面只有"继续"一个动作，整屏都是热区。
		if op == 1 || op == 3 || op == 4 {
			return s.dismissWarning(), true
		}
		return state.SessionAwaitInput, true
	}
	if s.titleMode {
		if op == 0 {
			s.SetTitleHover(x, y)
		}
		return state.SessionAwaitTitle, true
	}
	if s.uiMode != tj2UINone {
		return s.classicUIPointer(op, x, y), true
	}
	if s.IsToku2UI() && s.engineMenuButtonHit(x, y) && (op == 1 || op == 3) {
		s.openToku2System()
		return s.classicUIWaitStep(), true
	}
	// 地图激活时，信息框即使还处于"可见"标志也不能抢指针事件，
	// 否则 MapLayer 的悬停/点击会全部被当成信息框操作。
	if s.classicMap.Active() || (s.mapScreen.Active() && !s.mapScreen.Fallback()) {
		return 0, false
	}
	if s.classicMessageFrameVisible() {
		switch op {
		case 0:
			hover := s.messageButtonAt(x, y)
			if hover != s.messageHover {
				s.messageHover = hover
				s.stage.Invalidate()
			}
			return state.SessionAwaitInput, true
		case 1, 3:
			if button := s.messageButtonAt(x, y); button != 0 {
				return s.messageButtonAction(button), true
			}
		case 4:
			if s.IsToku2UI() {
				s.openToku2System()
				return s.classicUIWaitStep(), true
			}
			s.gameState.SetMessageWindowVisible(false)
			s.stage.Invalidate()
			return state.SessionAwaitInput, true
		}
		return 0, false
	}
	if op == 1 || op == 3 || op == 4 {
		// 信息框被隐藏时，点击/右键先把当前页叫回来，不推进脚本。
		if s.gameState.Text() != "" {
			s.gameState.SetMessageWindowVisible(true)
			s.stage.Invalidate()
			return state.SessionAwaitInput, true
		}
	}
	return 0, false
}

func (s *Tj2Session) classicUIPointer(op uint32, x, y int) state.SessionStep {
	switch op {
	case 0:
		s.updateClassicUIHover(x, y)
		return s.classicUIWaitStep()
	case 4:
		return s.closeClassicUIFromRightClick()
	case 1, 3:
		return s.classicUIClick(x, y)
	default:
		return s.classicUIWaitStep()
	}
}

func (s *Tj2Session) closeClassicUIFromRightClick() state.SessionStep {
	if s.IsToku2UI() {
		switch s.uiMode {
		case tj2UICG, tj2UISound, tj2UIScene:
			if s.uiCgOpen {
				s.uiCgOpen = false
				s.uiRevision++
				return state.SessionAwaitSelect
			}
			return s.openGalleryRoot()
		case tj2UIGalleryRoot:
			return s.leaveGalleryRoot()
		case tj2UISystem:
			s.uiMode = tj2UINone
			s.uiRevision++
			s.stage.Invalidate()
			return s.currentStepForUI()
		case tj2UIConfig:
			s.uiMode = tj2UISystem
			s.uiRevision++
			return state.SessionAwaitSelect
		case tj2UIBacklog:
			s.uiMode = tj2UINone
			s.uiRevision++
			return s.currentStepForUI()
		case tj2UISave, tj2UILoad:
			return s.closeClassicUI(s.uiResumeRun)
		}
	}
	switch s.uiMode {
	case tj2UIGallery:
		// 右键等价于"タイトルへ戻る"。
		s.writeClassicFlag(0, 3)
		return s.closeClassicUI(true)
	case tj2UICG:
		if s.uiCgOpen {
			s.uiCgOpen = false
			s.uiRevision++
			return s.classicUIWaitStep()
		}
		return s.closeClassicUI(true)
	case tj2UISound:
		s.stopClassicSound()
		return s.closeClassicUI(true)
	case tj2UIScene:
		s.writeClassicFlag(0, 0xFF)
		return s.closeClassicUI(true)
	case tj2UISave, tj2UILoad:
		return s.closeClassicUI(s.uiResumeRun)
	default:
		return s.classicUIWaitStep()
	}
}

func (s *Tj2Session) updateClassicUIHover(x, y int) {
	hover := s.classicUIHoverAt(x, y)
	if hover == s.uiHover {
		return
	}
	s.uiHover = hover
	s.uiRevision++
}

func (s *Tj2Session) classicUIHoverAt(x, y int) int {
	if s.IsToku2UI() {
		switch s.uiMode {
		case tj2UISystem, tj2UIGalleryRoot, tj2UIConfig, tj2UIBacklog:
			return s.toku2UIHoverAt(x, y)
		case tj2UICG:
			if s.uiCgOpen {
				return -1
			}
			start := s.uiPage * toku2CGPerPage
			for index := 0; index < toku2CGPerPage && start+index < len(s.cgThumbs); index++ {
				if tj2Hit(x, y, toku2CellRect(index)) {
					return start + index
				}
			}
			return -1
		case tj2UIScene:
			if s.uiCgOpen {
				return -1
			}
			start := s.uiPage * toku2ScenePerPage
			for index := 0; index < toku2ScenePerPage && start+index < len(s.cgThumbs); index++ {
				if tj2Hit(x, y, toku2CellRect(index)) {
					return start + index
				}
			}
			return -1
		case tj2UISound:
			if index := s.toku2MusicRowAt(x, y); index >= 0 {
				return index
			}
			return -1
		}
	}
	switch s.uiMode {
	case tj2UIGallery:
		for index, rect := range tj2GalleryItemRects {
			if tj2Hit(x, y, rect) {
				return index
			}
		}
	case tj2UISave, tj2UILoad:
		if slot := s.classicSaveSlotAt(x, y); slot >= 0 {
			return slot
		}
	case tj2UICG:
		if s.uiCgOpen {
			return -1
		}
		start := s.uiPage * tj2UICGPerPage
		for index := 0; index < tj2UICGPerPage && start+index < len(s.cgThumbs); index++ {
			if tj2Hit(x, y, s.classicCGCellRect(index)) {
				return start + index
			}
		}
	case tj2UISound:
		if index := s.classicSoundRowAt(x, y); index >= 0 {
			return index
		}
	case tj2UIScene:
		if index := s.classicSceneRowAt(x, y); index >= 0 {
			return index
		}
	}
	return -1
}

// toku2MusicRowAt 把音乐格点击换算成曲目下标；19 首 = 左 10 + 右 9。
func (s *Tj2Session) toku2MusicRowAt(x, y int) int {
	for i := 0; i < len(s.soundTracks) && i < 19; i++ {
		if tj2Hit(x, y, s.toku2MusicRowRect(i)) {
			return i
		}
	}
	return -1
}

func (s *Tj2Session) classicSaveSlotAt(x, y int) int {
	for row := 0; row < tj2UISaveRows; row++ {
		for column := 0; column < tj2UISaveColumns; column++ {
			rect := s.classicSaveCellRect(row, column)
			if tj2Hit(x, y, rect) {
				slot := s.uiPage*tj2UISavePageSlots + row*tj2UISaveColumns + column
				if slot < 0 || slot >= tj2SaveSlotCount {
					return -1
				}
				return slot
			}
		}
	}
	return -1
}

func (s *Tj2Session) classicSaveCellRect(row, column int) tj2Rect {
	return tj2Rect{X: 44 + column*190, Y: 72 + row*88, W: 184, H: 84}
}

func (s *Tj2Session) classicCGCellRect(index int) tj2Rect {
	row := index / tj2UICGGridColumns
	column := index % tj2UICGGridColumns
	return tj2Rect{X: 20 + column*100, Y: 78 + row*82, W: 100, H: 80}
}

func (s *Tj2Session) classicSoundRowAt(x, y int) int {
	for index := 0; index < len(s.soundTracks); index++ {
		rect := s.classicSoundRowRect(index)
		if tj2Hit(x, y, rect) {
			return index
		}
	}
	return -1
}

func (s *Tj2Session) classicSoundRowRect(index int) tj2Rect {
	if index < 8 {
		return tj2Rect{X: 20, Y: 62 + index*48, W: 290, H: 44}
	}
	return tj2Rect{X: 320, Y: 62 + (index-8)*48, W: 300, H: 44}
}

func (s *Tj2Session) classicSceneRowAt(x, y int) int {
	for row := 0; row < tj2UIScenePerPage; row++ {
		rect := tj2Rect{X: 50, Y: 94 + row*48, W: 540, H: 42}
		if tj2Hit(x, y, rect) {
			index := s.uiPage*tj2UIScenePerPage + row
			if index < len(s.sceneTitles) {
				return index
			}
		}
	}
	return -1
}

func (s *Tj2Session) classicUIClick(x, y int) state.SessionStep {
	if s.IsToku2UI() {
		switch s.uiMode {
		case tj2UISystem:
			return s.toku2SystemClick(x, y)
		case tj2UIGalleryRoot:
			return s.toku2GalleryRootClick(x, y)
		case tj2UIConfig:
			return s.toku2ConfigClick(x, y)
		case tj2UIBacklog:
			s.uiMode = tj2UINone
			s.uiRevision++
			s.stage.Invalidate()
			return s.currentStepForUI()
		case tj2UICG:
			if s.uiCgOpen {
				if s.toku2FullClick(x, y, s.cgThumbs) {
					return state.SessionAwaitSelect
				}
				return state.SessionAwaitSelect
			}
			step, _ := s.toku2GalleryClick(x, y, s.cgThumbs, "cgmode")
			return step
		case tj2UIScene:
			if s.uiCgOpen {
				if s.toku2FullClick(x, y, s.cgThumbs) {
					return state.SessionAwaitSelect
				}
				return state.SessionAwaitSelect
			}
			step, _ := s.toku2GalleryClick(x, y, s.cgThumbs, "scene")
			return step
		case tj2UISound:
			return s.toku2MusicClick(x, y)
		}
	}
	switch s.uiMode {
	case tj2UIGallery:
		for index, rect := range tj2GalleryItemRects {
			if tj2Hit(x, y, rect) {
				s.writeClassicFlag(0, index)
				return s.closeClassicUI(true)
			}
		}
	case tj2UISave:
		if slot := s.classicSaveSlotAt(x, y); slot >= 0 {
			s.SaveTo(slot)
			s.uiRevision++
			return s.classicUIWaitStep()
		}
		return s.classicSaveButtons(x, y)
	case tj2UILoad:
		if slot := s.classicSaveSlotAt(x, y); slot >= 0 {
			if s.LoadFrom(slot) {
				return s.finishClassicLoad()
			}
			return s.classicUIWaitStep()
		}
		return s.classicSaveButtons(x, y)
	case tj2UICG:
		return s.classicCGClick(x, y)
	case tj2UISound:
		return s.classicSoundClick(x, y)
	case tj2UIScene:
		return s.classicSceneClick(x, y)
	}
	return s.classicUIWaitStep()
}

func (s *Tj2Session) classicSaveButtons(x, y int) state.SessionStep {
	switch {
	case tj2Hit(x, y, tj2SavePrevRect):
		if s.uiPage > 0 {
			s.uiPage--
			s.uiHover = -1
			s.uiRevision++
		}
	case tj2Hit(x, y, tj2SaveNextRect):
		if (s.uiPage+1)*tj2UISavePageSlots < tj2SaveSlotCount {
			s.uiPage++
			s.uiHover = -1
			s.uiRevision++
		}
	case tj2Hit(x, y, tj2SaveBackRect):
		return s.closeClassicUI(s.uiResumeRun)
	}
	return s.classicUIWaitStep()
}

func (s *Tj2Session) classicCGClick(x, y int) state.SessionStep {
	if s.uiCgOpen {
		switch {
		case tj2Hit(x, y, tj2CGPrevRect):
			s.changeClassicCG(-1)
		case tj2Hit(x, y, tj2CGNextRect):
			s.changeClassicCG(1)
		case tj2Hit(x, y, tj2CGExitRect):
			s.uiCgOpen = false
			s.uiRevision++
		}
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2CGPrevRect) {
		if s.uiPage > 0 {
			s.uiPage--
			s.uiHover = -1
			s.uiRevision++
		}
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2CGNextRect) {
		pages := s.classicCGPageCount()
		if s.uiPage+1 < pages {
			s.uiPage++
			s.uiHover = -1
			s.uiRevision++
		}
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2CGExitRect) {
		return s.closeClassicUI(true)
	}
	start := s.uiPage * tj2UICGPerPage
	for index := 0; index < tj2UICGPerPage && start+index < len(s.cgThumbs); index++ {
		if !tj2Hit(x, y, s.classicCGCellRect(index)) {
			continue
		}
		s.uiSelected = start + index
		s.uiCgOpen = true
		s.uiRevision++
		return s.classicUIWaitStep()
	}
	return s.classicUIWaitStep()
}

func (s *Tj2Session) classicCGPageCount() int {
	count := len(s.cgThumbs)
	if count == 0 {
		return 1
	}
	return (count + tj2UICGPerPage - 1) / tj2UICGPerPage
}

func (s *Tj2Session) changeClassicCG(delta int) {
	count := len(s.cgThumbs)
	if count == 0 {
		return
	}
	s.uiSelected += delta
	if s.uiSelected < 0 {
		s.uiSelected = count - 1
	}
	if s.uiSelected >= count {
		s.uiSelected = 0
	}
	s.uiPage = s.uiSelected / tj2UICGPerPage
	s.uiRevision++
}

func (s *Tj2Session) classicSoundClick(x, y int) state.SessionStep {
	if tj2Hit(x, y, tj2SoundPlayRect) {
		s.playClassicSound()
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2SoundStopRect) {
		s.stopClassicSound()
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2SoundExitRect) {
		s.stopClassicSound()
		return s.closeClassicUI(true)
	}
	if index := s.classicSoundRowAt(x, y); index >= 0 {
		s.uiSelected = index
		s.uiRevision++
	}
	return s.classicUIWaitStep()
}

func (s *Tj2Session) playClassicSound() {
	if s.uiSelected < 0 || s.uiSelected >= len(s.soundTracks) {
		return
	}
	path := s.bgm.Resolve(s.uiSelected)
	if len(path) == 0 {
		core.Logf("tj2: sound gallery track %d has no audio", s.uiSelected)
		return
	}
	s.bgmTrack = s.uiSelected
	s.gameState.SetBgm(path)
	s.gameState.SetBgmLoop(true)
	core.Logf("tj2: sound gallery play %d '%s'", s.uiSelected, path)
}

func (s *Tj2Session) stopClassicSound() {
	s.bgmTrack = -1
	s.gameState.ClearBgm()
}

func (s *Tj2Session) classicSceneClick(x, y int) state.SessionStep {
	if tj2Hit(x, y, tj2CGPrevRect) {
		if s.uiPage > 0 {
			s.uiPage--
			s.uiHover = -1
			s.uiRevision++
		}
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2CGNextRect) {
		if (s.uiPage+1)*tj2UIScenePerPage < len(s.sceneTitles) {
			s.uiPage++
			s.uiHover = -1
			s.uiRevision++
		}
		return s.classicUIWaitStep()
	}
	if tj2Hit(x, y, tj2CGExitRect) {
		s.writeClassicFlag(0, 0xFF)
		return s.closeClassicUI(true)
	}
	if index := s.classicSceneRowAt(x, y); index >= 0 {
		s.writeClassicFlag(0, index)
		return s.closeClassicUI(true)
	}
	return s.classicUIWaitStep()
}

// ---- 绘制 ----

func (s *Tj2Session) composeClassicSystemUI() bool {
	// 系统界面是静态的（除了悬停/翻页），没必要每帧重读 24 个槽位文件、
	// 重渲染所有文字。stage revision + UI revision 没变就复用上一帧。
	key := s.stage.Revision()*1000003 + s.uiRevision
	if s.uiComposedKey == key {
		return false
	}
	s.uiComposedKey = key
	if s.uiBase == nil {
		s.uiBase = gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	}
	// Stage 的缓存以 revision 为准；UI 打开时已经 Invalidate 过，所以
	// 这里第一次一定会把底图重画进 uiBase，之后直接复用。
	s.stage.ComposeTo(s.uiBase, false)
	s.frame.CopyFrom(s.uiBase)
	switch s.uiMode {
	case tj2UIGallery:
		s.drawClassicGallery()
	case tj2UICG:
		s.drawClassicCG()
	case tj2UISound:
		s.drawClassicSound()
	case tj2UIScene:
		s.drawClassicScene()
	case tj2UISave, tj2UILoad:
		s.drawClassicSaveLoad()
	}
	return true
}

func (s *Tj2Session) classicKeyedImage(name string) *gfx.BsgImage {
	if s.uiKeyed == nil {
		s.uiKeyed = map[string]*gfx.BsgImage{}
	}
	if image, ok := s.uiKeyed[name]; ok {
		return image
	}
	raw := s.images.Get(name)
	if raw == nil {
		return nil
	}
	keyed := WithGreenKey(raw)
	s.uiKeyed[name] = keyed
	return keyed
}

func (s *Tj2Session) drawClassicGallery() {
	if overlay := s.classicKeyedImage("GALLERYB.KG"); overlay != nil {
		s.frame.Blit(overlay, 0, 0, 255)
	}
	if s.uiHover >= 0 && s.uiHover < len(tj2GalleryItemRects) {
		rect := tj2GalleryItemRects[s.uiHover]
		s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 70, 60)
	}
}

func (s *Tj2Session) drawClassicCG() {
	if s.IsToku2UI() {
		if s.uiCgOpen {
			s.drawToku2FullView(s.cgThumbs)
			return
		}
		pages := (len(s.cgThumbs) + toku2CGPerPage - 1) / toku2CGPerPage
		s.drawToku2GalleryGrid("cgmode", s.cgThumbs, s.uiPage)
		s.drawToku2GalleryButtons(s.uiPage, pages)
		return
	}
	if s.uiCgOpen {
		s.drawClassicCGView()
		return
	}
	start := s.uiPage * tj2UICGPerPage
	for index := 0; index < tj2UICGPerPage && start+index < len(s.cgThumbs); index++ {
		name := s.cgThumbs[start+index]
		image := s.images.Get(name)
		if image == nil {
			continue
		}
		rect := s.classicCGCellRect(index)
		s.frame.BlitScaled(image, rect.X+2, rect.Y+2, 96, 72, 255)
		if s.uiHover == start+index {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 70, 70)
		}
	}
	s.drawUIText(fmt.Sprintf("%d / %d", s.uiPage+1, s.classicCGPageCount()),
		536, 26, 16, 20, 90, 1, tj2UIColorWhite)
}

func (s *Tj2Session) drawClassicCGView() {
	name := ""
	if s.uiSelected >= 0 && s.uiSelected < len(s.cgThumbs) {
		name = classicFullCGName(s.cgThumbs[s.uiSelected])
	}
	drawn := false
	if len(name) > 0 {
		if image := s.images.Get(name); image != nil {
			s.frame.Draw(image, image.OffsetX, image.OffsetY)
			drawn = true
		}
	}
	if !drawn {
		s.frame.Clear(0, 0, 0, 255)
		s.drawUIText("NO IMAGE", 270, 220, 24, 28, 200, 1, tj2UIColorCream)
	}
	if overlay := s.classicKeyedImage("CGMODEB.KG"); overlay != nil {
		s.frame.Blit(overlay, 0, 0, 255)
	}
	total := len(s.cgThumbs)
	if total > 0 {
		s.drawUIText(fmt.Sprintf("%d / %d", s.uiSelected+1, total),
			536, 18, 16, 20, 90, 1, tj2UIColorWhite)
	}
}

func (s *Tj2Session) drawClassicSound() {
	if s.IsToku2UI() {
		s.drawToku2Music()
		return
	}
	for index := range s.soundTracks {
		rect := s.classicSoundRowRect(index)
		if s.uiSelected == index {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 240, 150, 90, 80)
		} else if s.uiHover == index {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 70, 50)
		}
	}
	if s.uiSelected >= 0 && s.uiSelected < len(s.soundTracks) {
		s.drawUIText("♪ "+s.soundTracks[s.uiSelected], 20, 432, 14, 18, 290, 1, tj2UIColorCream)
	}
}

func (s *Tj2Session) drawClassicScene() {
	if s.IsToku2UI() {
		if s.uiCgOpen {
			s.drawToku2FullView(s.cgThumbs)
			return
		}
		pages := (len(s.cgThumbs) + toku2ScenePerPage - 1) / toku2ScenePerPage
		s.drawToku2GalleryGrid("scene", s.cgThumbs, s.uiPage)
		s.drawToku2GalleryButtons(s.uiPage, pages)
		return
	}
	start := s.uiPage * tj2UIScenePerPage
	for row := 0; row < tj2UIScenePerPage && start+row < len(s.sceneTitles); row++ {
		y := 94 + row*48
		rect := tj2Rect{X: 50, Y: y, W: 540, H: 42}
		if s.uiHover == start+row {
			s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 80, 80)
		} else {
			s.frame.FillBlend(rect.X+40, rect.Y, rect.W-80, rect.H, 0, 0, 0, 120)
		}
		title := s.sceneTitles[start+row]
		s.drawUIText(fmt.Sprintf("%02d  %s", start+row+1, title),
			70, y+8, 20, 24, 500, 1, tj2UIColorWhite)
	}
	pages := (len(s.sceneTitles) + tj2UIScenePerPage - 1) / tj2UIScenePerPage
	if pages < 1 {
		pages = 1
	}
	// 页码压在 SCENEA 的浅色窗框上，补一层暗底保证可读。
	s.frame.FillBlend(534, 20, 64, 20, 0, 0, 0, 140)
	s.drawUIText(fmt.Sprintf("%d / %d", s.uiPage+1, pages),
		540, 22, 16, 20, 80, 1, tj2UIColorWhite)
}

func (s *Tj2Session) drawClassicSaveLoad() {
	// 压暗游戏画面，保证槽位文字在亮背景上也读得清。
	s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, 0, 0, 0, 150)
	panel := tj2Rect{X: 20, Y: 14, W: 600, H: 452}
	s.frame.FillBlend(panel.X, panel.Y, panel.W, panel.H, 14, 10, 7, 232)
	s.drawUIBorder(panel)

	title := "セーブ"
	if s.uiMode == tj2UILoad {
		title = "ロード"
	}
	s.drawUIText(title, 40, 26, 24, 28, 200, 1, tj2UIColorCream)
	s.drawUIText(fmt.Sprintf("PAGE %d / 2", s.uiPage+1), 480, 32, 16, 20, 120, 1, tj2UIColorGold)

	for row := 0; row < tj2UISaveRows; row++ {
		for column := 0; column < tj2UISaveColumns; column++ {
			slot := s.uiPage*tj2UISavePageSlots + row*tj2UISaveColumns + column
			if slot >= tj2SaveSlotCount {
				continue
			}
			s.drawClassicSaveSlot(slot, s.classicSaveCellRect(row, column))
		}
	}

	s.drawUIButton(tj2SavePrevRect, "前頁")
	s.drawUIButton(tj2SaveNextRect, "次頁")
	s.drawUIButton(tj2SaveBackRect, "戻る")
}

func (s *Tj2Session) drawClassicSaveSlot(slot int, rect tj2Rect) {
	info := s.tj2Saves.Inspect(slot)
	if s.uiHover == slot {
		s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 230, 140, 80, 70)
	}
	s.frame.Fill(rect.X+2, rect.Y+2, rect.W-4, 1, 70, 55, 40, 255)
	s.frame.Fill(rect.X+2, rect.Y+rect.H-3, rect.W-4, 1, 70, 55, 40, 255)
	if save := s.tj2Saves.Read(slot); save != nil {
		if len(save.Thumbnail) == save.ThumbnailWidth*save.ThumbnailHeight*4 && save.ThumbnailWidth > 0 {
			image := gfx.NewBsgImage(save.ThumbnailWidth, save.ThumbnailHeight, 0, 0, save.Thumbnail)
			s.frame.BlitScaled(image, rect.X+4, rect.Y+5, 84, 63, 255)
		}
	}
	number := fmt.Sprintf("No.%02d", slot+1)
	s.drawUIText(number, rect.X+96, rect.Y+4, 16, 18, 84, 1, tj2UIColorCream)
	if info.Exists {
		s.drawUIText(info.SavedAt.Format("01/02 15:04"), rect.X+96, rect.Y+26, 12, 14, 84, 1, tj2UIColorDim)
		s.drawUIText(clipTj2Text(info.Summary, 12), rect.X+96, rect.Y+44, 12, 14, 84, 2, tj2UIColorWhite)
	} else {
		s.drawUIText("----/-- --:--", rect.X+96, rect.Y+26, 12, 14, 84, 1, tj2UIColorDim)
		s.drawUIText("EMPTY", rect.X+96, rect.Y+44, 12, 14, 84, 1, tj2UIColorDim)
	}
}

func (s *Tj2Session) drawUIBorder(rect tj2Rect) {
	s.frame.Fill(rect.X, rect.Y, rect.W, 2, 96, 160, 200, 255)
	s.frame.Fill(rect.X, rect.Y+rect.H-2, rect.W, 2, 96, 160, 200, 255)
	s.frame.Fill(rect.X, rect.Y, 2, rect.H, 96, 160, 200, 255)
	s.frame.Fill(rect.X+rect.W-2, rect.Y, 2, rect.H, 96, 160, 200, 255)
}

func (s *Tj2Session) drawUIButton(rect tj2Rect, label string) {
	s.frame.FillBlend(rect.X, rect.Y, rect.W, rect.H, 30, 22, 14, 205)
	s.drawUIBorder(rect)
	s.drawUIText(label, rect.X+9, rect.Y+6, 16, 20, rect.W-18, 1, tj2UIColorCream)
}

func clipTj2Text(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}
