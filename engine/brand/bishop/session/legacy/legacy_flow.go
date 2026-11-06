package legacy

import (
	"fmt"
	"time"
	"unicode/utf8"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm/scf"
	"kizuna/engine/core"
)

const (
	legacyWaitDuration   = 500 * time.Millisecond
	legacyFadeDuration   = 400 * time.Millisecond
	legacyEndingWait     = 3 * time.Second
	legacyCharsPerSecond = 30
	legacyScrollStep     = 3
	legacyKeyEnter       = 13
	legacyKeyEscape      = 27
	legacyKeySpace       = 32
)

var (
	legacyTitleKeys   = [3]string{"start", "load", "museum"}
	legacyTitleLabels = [3]string{"最初から始める", "途中から始める", "おまけをみる"}
	// SCN000 的第一个 !b 0 在 track 表里对应 31；标题画面在原版里就播它。
	legacyTitleBgmTrack = 31
)

func (e *Engine) Start(entry uint32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.machine == nil {
		return core.StepFaulted, nil
	}
	if entry != 0 {
		return core.StepFaulted, fmt.Errorf("legacy: replay entry %d unsupported", entry)
	}
	if e.hasMenu {
		e.enterTitle()
		return core.StepAwaitTitle, nil
	}
	e.resetRuntime()
	e.machine.Start(0)
	return e.toCoreStep(e.run(legacyRunBudget)), nil
}

func (e *Engine) Run(budget uint32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.waiting() {
		return core.StepWait, nil
	}
	if e.hasFrame && (e.titleMode || e.modal != nil || e.commandActive || e.gameState.AwaitingInput()) {
		return e.currentStep(), nil
	}
	return e.toCoreStep(e.run(budget)), nil
}

func (e *Engine) Advance() (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.waiting() {
		return core.StepWait, nil
	}
	if !e.hasFrame {
		e.gameState.SetAwaitingInput(false)
		return e.toCoreStep(e.run(1)), nil
	}
	if e.titleMode || e.modal != nil || e.commandActive {
		return e.currentStep(), nil
	}
	if e.gameState.AwaitingInput() {
		return e.toCoreStep(e.advanceMessage()), nil
	}
	return e.toCoreStep(e.run(legacyRunBudget)), nil
}

func (e *Engine) advanceMessage() state.SessionStep {
	if !e.messageDone && !e.config.InstantText {
		e.messageDone = true
		e.stage.SetMessage(e.messageFull, -1)
		return state.SessionAwaitInput
	}
	e.clearMessage()
	return e.run(1)
}

func (e *Engine) Choose(index uint32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.machine == nil {
		return core.StepFaulted, nil
	}
	if e.hasFrame {
		if e.commandActive {
			return e.chooseCommand(int(index)), nil
		}
		return e.currentStep(), nil
	}
	if !e.gameState.AwaitingChoice() {
		return core.StepContinue, nil
	}
	if !e.machine.Choose(int(index)) {
		core.Logf("legacy: choice %d rejected", index)
		return core.StepAwaitChoice, nil
	}
	e.gameState.ClearChoices()
	e.gameState.SetAwaitingInput(false)
	return e.toCoreStep(e.run(legacyRunBudget)), nil
}

func (e *Engine) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.waiting() {
		return core.StepWait, nil
	}
	if !e.hasFrame {
		if (op == 1 || op == 3 || op == 4) && e.gameState.AwaitingInput() {
			return e.Advance()
		}
		return e.currentStep(), nil
	}
	if op == 4 || op == 5 {
		return e.escape(), nil
	}
	// 触屏拖动（子操作 6）：命令列表超过 10 行时用纵向拖动滚动。
	// 宿主没有 wheel 事件可用，所以必须在这里接。
	if op == 6 {
		if e.commandActive && e.commandPanel.Kind != LegacyPanelNone {
			return e.dragCommand(int(y)), nil
		}
		return e.currentStep(), nil
	}
	if op != 1 && op != 3 {
		return e.currentStep(), nil
	}
	// 新的按下/点击会开启一段新的拖动；不重置锚点的话，抬手后再拖
	// 会拿上一次的终点算位移，列表会突然跳一段。
	e.commandDragY = -1
	px, py := int(x), int(y)
	if e.modal != nil {
		return e.clickModal(px, py), nil
	}
	if e.titleMode {
		if index, ok := TitleItemAt(px, py); ok {
			return e.SelectTitleItem(legacyTitleKeys[index])
		}
		return e.currentStep(), nil
	}
	if e.commandActive {
		// 子清单是居中的模态窗口，会盖住底栏按钮；命中优先给面板，
		// 否则 y=354..381 的底栏热区会抢走面板最后两行的点击。
		if e.commandPanel.Kind != LegacyPanelNone {
			if hit, ok := e.commandPanel.HitTest(px, py); ok {
				return e.clickCommand(hit), nil
			}
			return e.currentStep(), nil
		}
		if index, ok := FrameButtonAt(px, py); ok {
			return e.commandFrameButton(index), nil
		}
		return e.currentStep(), nil
	}
	if index, ok := FrameButtonAt(px, py); ok {
		if index == 2 {
			return e.openSystem(), nil
		}
		return e.currentStep(), nil
	}
	if e.gameState.AwaitingInput() {
		return e.Advance()
	}
	return e.currentStep(), nil
}

func (e *Engine) PressKey(code uint32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.waiting() {
		return core.StepWait, nil
	}
	if code == legacyKeyEscape {
		return e.escape(), nil
	}
	if (code == legacyKeyEnter || code == legacyKeySpace) && e.hasFrame && e.gameState.AwaitingInput() && e.modal == nil && !e.commandActive && !e.titleMode {
		return e.Advance()
	}
	return e.currentStep(), nil
}

func (e *Engine) Wheel(delta int32) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if e.modal != nil || !e.commandActive || delta == 0 {
		return e.currentStep(), nil
	}
	step := legacyScrollStep
	if delta < 0 {
		step = -step
	}
	maxScroll := len(e.commandPanel.Items) - legacyListRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	e.commandPanel.Scroll = min(max(e.commandPanel.Scroll+step, 0), maxScroll)
	e.syncPanel()
	return e.currentStep(), nil
}

func (e *Engine) dragCommand(y int) core.Step {
	if e.commandDragY < 0 {
		e.commandDragY = y
		return e.currentStep()
	}
	// 12px 一行，单次回调最多滚 3 行；避免两段拖动之间没有 tap 时
	// 第一帧拿上一次的终点算出巨大跳变。
	delta := (y - e.commandDragY) / 12
	if delta == 0 {
		return e.currentStep()
	}
	delta = min(max(delta, -3), 3)
	e.commandDragY += delta * 12
	maxScroll := len(e.commandPanel.Items) - legacyListRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	e.commandPanel.Scroll = min(max(e.commandPanel.Scroll+delta, 0), maxScroll)
	e.syncPanel()
	return e.currentStep()
}

func (e *Engine) TitleMenu() []core.TitleMenuItem {
	// 标题上打开读档面板时必须先收起宿主的热区：TitleLayer 是覆盖层，
	// 它会抢走 y=346..471 的点击，导致引擎自绘的「データの選択」列表点不动。
	if !e.hasMenu || !e.titleMode || e.modal != nil {
		return nil
	}
	items := make([]core.TitleMenuItem, 0, len(legacyTitleKeys))
	for index, rect := range legacyTitleButtonRects {
		items = append(items, core.TitleMenuItem{
			Key:     legacyTitleKeys[index],
			Label:   legacyTitleLabels[index],
			X:       int32(rect.X),
			Y:       int32(rect.Y),
			Width:   uint32(rect.W),
			Height:  uint32(rect.H),
			Enabled: true,
		})
	}
	return items
}

func (e *Engine) SelectTitleItem(key string) (core.Step, error) {
	if e.closed {
		return core.StepFinished, nil
	}
	if !e.titleMode || e.machine == nil {
		return e.currentStep(), nil
	}
	switch key {
	case "start":
		e.resetForNewGame()
		e.machine.Start(0)
		e.titleMode = false
		return e.toCoreStep(e.run(legacyRunBudget)), nil
	case "museum":
		e.resetForNewGame()
		e.machine.Start(0)
		if flags := e.machine.Flags(); len(flags) > 2 {
			flags[0] = 2
			flags[2] = 0
			if e.config.Clear {
				flags[2] = 1
			}
		}
		e.titleMode = false
		return e.toCoreStep(e.run(legacyRunBudget)), nil
	case "load":
		e.openModal(e.slotPanel(LegacyPanelLoad))
		return e.currentStep(), nil
	}
	return e.currentStep(), nil
}

func (e *Engine) Text() core.TextState {
	awaiting := uint32(0)
	switch {
	case e.hasFrame && (e.commandActive || e.modal != nil):
		awaiting = 2
	case e.gameState.AwaitingChoice():
		awaiting = 2
	case e.gameState.AwaitingInput():
		awaiting = 1
	}
	return core.TextState{Speaker: e.gameState.Speaker(), Text: e.gameState.Text(), Awaiting: awaiting}
}

func (e *Engine) Choices() []core.Choice {
	if e.hasFrame {
		return nil
	}
	texts := e.gameState.Choices()
	out := make([]core.Choice, len(texts))
	for index, text := range texts {
		out[index] = core.Choice{Name: text, Value: int32(index)}
	}
	return out
}

func (e *Engine) State() core.State {
	bgm, _ := e.gameState.Bgm()
	pc, scene, start := 0, 0, 0
	if e.machine != nil {
		pc = e.machine.ProgramCounter()
		scene = e.machine.ScenarioNumber()
		start = e.machine.InstructionStart()
	}
	messageWindow := e.gameState.MessageWindowVisible()
	if e.hasFrame {
		messageWindow = !e.titleMode
	}
	return core.State{
		Bgm:                  bgm,
		PC:                   uint32(pc),
		Scene:                uint32(scene),
		Animating:            e.animating(),
		WaitMs:               e.remainingWaitMs(),
		MessageWindowVisible: messageWindow,
		// 标题上的读档面板同样是引擎自绘；把 engineMessageWindow 置真，
		// 宿主才会收起底部工具栏，避免它盖住 y=346..440 的面板按钮。
		EngineMessageWindow: e.hasFrame && (!e.titleMode || e.modal != nil),
		ReplayActive:         false,
		InstructionStart:     uint32(start),
		CeaseSkip:            true,
		SkipUnread:           true,
		AutoWait:             50,
	}
}

func (e *Engine) currentStep() core.Step {
	if e.machine == nil || e.machine.Faulted() {
		return core.StepFaulted
	}
	if e.titleMode {
		return core.StepAwaitTitle
	}
	if e.remainingWaitMs() > 0 {
		return core.StepWait
	}
	if e.modal != nil || e.commandActive {
		return core.StepAwaitChoice
	}
	if e.machine.Halted() {
		return core.StepFinished
	}
	if e.gameState.AwaitingChoice() {
		return core.StepAwaitChoice
	}
	if e.gameState.AwaitingInput() {
		return core.StepAwaitInput
	}
	return core.StepContinue
}

func (e *Engine) animating() bool {
	if e.hasFrame && e.gameState.AwaitingInput() && !e.messageDone && !e.config.InstantText {
		return true
	}
	if e.fadeActive && time.Since(e.fadeStart) < legacyFadeDuration {
		return true
	}
	return e.endingPhase != 0
}

func (e *Engine) remainingWaitMs() uint32 {
	if e.waitUntil.IsZero() {
		return 0
	}
	remain := time.Until(e.waitUntil)
	if remain <= 0 {
		return 0
	}
	return uint32(remain / time.Millisecond)
}

// waiting 在等待时间到期时推进结局/淡出阶段；返回 true 表示仍需等待。
func (e *Engine) waiting() bool {
	if e.waitUntil.IsZero() {
		return false
	}
	if time.Now().Before(e.waitUntil) {
		return true
	}
	e.waitUntil = time.Time{}
	// fadeout 的 wait 结束后必须把黑色遮罩撤掉。旧实现只在换背景时清，
	// 于是 SCN007 里“音效 → fadeout → 台词”这种没有换背景的序列会永久黑屏。
	if e.fadeActive {
		e.clearFade()
	}
	switch e.endingPhase {
	case 1:
		e.endingPhase = 2
		e.stage.SetEnding(e.images.Get(legacyEndingTwoName))
		e.waitUntil = time.Now().Add(legacyEndingWait)
		return true
	case 2:
		// 结局图片放完直接回标题。SCN127 后面还有 write(2=1)/!j 0，
		// 如果继续执行会在 flag0 非 0/1/2 时让 SCN000 原地循环；
		// clear 标记已经在 applyEndingRoll 里落盘，不需要再跑那几条指令。
		e.endingPhase = 0
		e.stage.SetEnding(nil)
		if e.hasMenu {
			e.enterTitle()
		}
		return false
	}
	return false
}

func (e *Engine) updateAnimation() {
	if e.closed || !e.hasFrame {
		return
	}
	e.waiting()
	if e.gameState.AwaitingInput() && e.messageFull != "" {
		if e.config.InstantText {
			e.messageDone = true
		}
		reveal := -1
		if !e.messageDone {
			count := int(time.Since(e.messageStart).Seconds() * legacyCharsPerSecond)
			if count >= utf8.RuneCountInString(e.messageFull) {
				e.messageDone = true
			} else {
				reveal = count
			}
		}
		e.stage.SetMessage(e.messageFull, reveal)
	}
	if e.fadeActive {
		elapsed := time.Since(e.fadeStart)
		alpha := 255
		if elapsed < legacyFadeDuration {
			alpha = int(elapsed * 255 / legacyFadeDuration)
		}
		e.stage.SetFade(alpha)
	}
}

func (e *Engine) enterTitle() {
	e.resetTransient()
	e.gameState.SetAwaitingInput(false)
	e.gameState.SetText("")
	e.gameState.SetSpeaker("")
	e.startTitleBgm()
	e.titleMode = true
	// 标题不显示游戏内时钟；从游戏内 !j 0 回标题时必须显式清掉，
	// 否则 TIME 图标会叠在 menu.KG 上。
	e.timerSlotShown = -1
	e.stage.SetTimerIcon(nil)
	e.stage.SetTitle(e.menuImage)
}

// startTitleBgm 标题画面播 SCN000 的 !b 0。!b 的操作数是 track 表下标，
// 表第 0 项是 "31"，所以这里直接 Resolve(31)。我们的标题由引擎自绘、
// 不执行 SCN000，必须在这里补上原版进入标题时的那次起播。
func (e *Engine) startTitleBgm() {
	if e.bgmCatalog == nil || !e.config.Bgm {
		e.gameState.ClearBgm()
		e.enqueueAudio(stopBgmEvent())
		return
	}
	path := e.bgmCatalog.Resolve(legacyTitleBgmTrack)
	if path == "" {
		e.gameState.ClearBgm()
		e.enqueueAudio(stopBgmEvent())
		return
	}
	e.bgmTrack = legacyTitleBgmTrack
	e.gameState.SetBgm(path)
	e.enqueueAudio(playBgmEvent(path, e.gameState.BgmVolume()))
}

func (e *Engine) resetForNewGame() {
	e.resetRuntime()
	e.resetTransient()
	e.enqueueAudio(stopBgmEvent())
	if e.machine != nil {
		clear(e.machine.Flags())
		clear(e.machine.Source())
	}
}

func (e *Engine) resetTransient() {
	e.modal = nil
	e.modalStack = nil
	e.scriptDialog = false
	e.commandActive = false
	e.commandDragY = -1
	e.commandChoices = nil
	e.commandPanel = LegacyPanel{}
	e.messageFull = ""
	e.messageDone = false
	e.waitUntil = time.Time{}
	e.endingPhase = 0
	e.clearFade()
	e.gameState.ClearChoices()
	e.stage.SetPanel(nil)
	e.stage.SetMessage("", 0)
	e.stage.SetEnding(nil)
	e.stage.SetTitle(nil)
}

func (e *Engine) clearFade() {
	e.fadeActive = false
	e.stage.SetFade(0)
}

func (e *Engine) clearMessage() {
	e.messageFull = ""
	e.messageDone = false
	e.stage.SetMessage("", 0)
	e.gameState.SetAwaitingInput(false)
}

func (e *Engine) endCommand() {
	e.commandActive = false
	e.commandDragY = -1
	e.commandChoices = nil
	e.commandPanel = LegacyPanel{}
	e.gameState.ClearChoices()
	e.syncPanel()
}

func (e *Engine) setCommandOptions(options []scf.ScfChoice) {
	e.gameState.SetChoices(choiceTexts(options))
	e.commandActive = true
	e.commandDragY = -1
	e.commandChoices = append([]scf.ScfChoice(nil), options...)
	// 第一层只有「見てみよう / どうしようかな」两个类目，原版用 frame.KG
	// 底栏这三个固定按钮表达，不弹列表窗口。把第一层做成隐藏菜单，玩家点
	// 底栏按钮进入对应子清单；子清单才画「コマンド選択」列表。
	if isRootOptions(options) {
		e.commandPanel = LegacyPanel{}
	} else {
		e.commandPanel = buildCommandPanel(options)
	}
	e.syncPanel()
}

func (e *Engine) isRootCommand() bool {
	return isRootOptions(e.commandChoices)
}

func (e *Engine) chooseCommand(index int) core.Step {
	if !e.machine.Choose(index) {
		core.Logf("legacy: command %d rejected", index)
		return e.currentStep()
	}
	e.endCommand()
	e.gameState.SetAwaitingInput(false)
	return e.toCoreStep(e.run(legacyRunBudget))
}

func (e *Engine) commandFrameButton(index int) core.Step {
	if index == 2 {
		return e.openSystem()
	}
	if !e.isRootCommand() {
		cmd := e.machine.RootOptions()
		if cmd.Kind != scf.ScfCommandKindOptions {
			return e.currentStep()
		}
		e.setCommandOptions(cmd.Options)
	}
	choice, ok := rootChoice(e.commandChoices, index)
	if !ok {
		return e.currentStep()
	}
	return e.chooseCommand(choice.Index)
}

// rootChoice 在第一层命令里取 0=見てみよう（Entry==0）或 1=どうしようかな（其余项）。
func rootChoice(choices []scf.ScfChoice, button int) (scf.ScfChoice, bool) {
	for _, choice := range choices {
		if button == 0 && choice.Entry == 0 {
			return choice, true
		}
		if button == 1 && choice.Entry != 0 {
			return choice, true
		}
	}
	return scf.ScfChoice{}, false
}

func (e *Engine) clickCommand(hit LegacyPanelHit) core.Step {
	switch hit.Kind {
	case "item":
		if hit.Index >= 0 && hit.Index < len(e.commandChoices) {
			return e.chooseCommand(e.commandChoices[hit.Index].Index)
		}
	case "back":
		if !e.isRootCommand() {
			if cmd := e.machine.RootOptions(); cmd.Kind == scf.ScfCommandKindOptions {
				e.setCommandOptions(cmd.Options)
			}
		}
	}
	return e.currentStep()
}

func (e *Engine) openModal(panel LegacyPanel) {
	if e.modal != nil {
		e.modalStack = append(e.modalStack, *e.modal)
	}
	e.modal = &panel
	e.syncPanel()
}

func (e *Engine) closeModal() {
	if n := len(e.modalStack); n > 0 {
		top := e.modalStack[n-1]
		e.modalStack = e.modalStack[:n-1]
		e.modal = &top
	} else {
		e.modal = nil
	}
	e.syncPanel()
}

func (e *Engine) closeAllModals() {
	e.modal = nil
	e.modalStack = nil
	e.scriptDialog = false
	e.syncPanel()
}

// closeModalStep 关闭最上层面板；若是剧本触发的存档/读档对话框则恢复脚本执行。
func (e *Engine) closeModalStep() core.Step {
	resume := e.scriptDialog && len(e.modalStack) == 0
	e.closeModal()
	if !resume {
		return e.currentStep()
	}
	e.scriptDialog = false
	return e.toCoreStep(e.run(legacyRunBudget))
}

func (e *Engine) syncPanel() {
	if e.stage == nil {
		return
	}
	switch {
	case e.modal != nil:
		e.stage.SetPanel(e.modal)
	case e.commandActive && e.commandPanel.Kind != LegacyPanelNone:
		e.stage.SetPanel(&e.commandPanel)
	default:
		e.stage.SetPanel(nil)
	}
}

func (e *Engine) openSystem() core.Step {
	e.openModal(e.systemPanel())
	return e.currentStep()
}

func (e *Engine) escape() core.Step {
	if !e.hasFrame || (e.titleMode && e.modal == nil) {
		return e.currentStep()
	}
	if e.modal != nil {
		return e.closeModalStep()
	}
	return e.openSystem()
}

func (e *Engine) clickModal(x, y int) core.Step {
	hit, ok := e.modal.HitTest(x, y)
	if !ok {
		return e.currentStep()
	}
	if hit.Kind == "back" {
		return e.closeModalStep()
	}
	switch e.modal.Kind {
	case LegacyPanelSystem:
		return e.clickSystem(hit.Kind)
	case LegacyPanelSave:
		if hit.Kind == "save" {
			return e.saveSlotStep(hit.Index)
		}
	case LegacyPanelLoad:
		if hit.Kind == "load" {
			return e.loadSlotStep(hit.Index)
		}
	}
	return e.currentStep()
}

func (e *Engine) clickSystem(kind string) core.Step {
	switch kind {
	case "instant":
		e.config.InstantText = true
	case "typewriter":
		e.config.InstantText = false
	case "save":
		e.openModal(e.slotPanel(LegacyPanelSave))
		return e.currentStep()
	case "load":
		e.openModal(e.slotPanel(LegacyPanelLoad))
		return e.currentStep()
	case "bgm":
		e.config.Bgm = !e.config.Bgm
		e.applyBgmSwitch()
	default:
		return e.currentStep()
	}
	e.persistConfig()
	e.refreshSystem()
	return e.currentStep()
}

func (e *Engine) refreshSystem() {
	if e.modal != nil && e.modal.Kind == LegacyPanelSystem {
		e.modal.InstantText = e.config.InstantText
		e.modal.BgmEnabled = e.config.Bgm
	}
	e.syncPanel()
}

func (e *Engine) applyBgmSwitch() {
	if e.config.Bgm {
		e.restoreBgm()
		return
	}
	e.enqueueAudio(stopBgmEvent())
}

func (e *Engine) refreshSlotPanel(status string) {
	if e.modal == nil {
		return
	}
	panel := e.slotPanel(e.modal.Kind)
	panel.Status = status
	*e.modal = panel
	e.syncPanel()
}

func (e *Engine) saveSlotStep(slot int) core.Step {
	status := "セーブしました"
	if err := e.Save(uint32(slot), ""); err != nil {
		core.Logf("legacy: save slot %d: %v", slot, err)
		status = "セーブに失敗しました"
	} else if e.scriptDialog && len(e.modalStack) == 0 {
		return e.closeModalStep()
	}
	e.refreshSlotPanel(status)
	return e.currentStep()
}

func (e *Engine) loadSlotStep(slot int) core.Step {
	step, err := e.Load(uint32(slot))
	if err != nil {
		core.Logf("legacy: load slot %d: %v", slot, err)
		e.refreshSlotPanel("データがありません")
		return e.currentStep()
	}
	return step
}
