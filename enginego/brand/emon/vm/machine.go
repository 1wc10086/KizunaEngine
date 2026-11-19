package vm

import (
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	emongfx "kizuna/enginego/brand/emon/gfx"
	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/brand/emon/stage"
	"kizuna/enginego/core"
	"kizuna/enginego/core/dotnet"
	"kizuna/enginego/core/strutil"
)

const framesPerSecond = 60

const defaultBudget = 4096

const maxFrames = 64

const actionWaitStepBudget = 256

const maxMacroArguments = 10

const imageHeaderSize = 32

const imageCacheCapacity = 32

const imageCacheBudgetBytes = int64(48 * 1024 * 1024)

var tokenSeparators = []rune{' ', '\t', '\r', '\n', '\u3000', '"', '＂', '(', ')', '（', '）', '[', ']', '［', '］', '{', '}', '｛', '｝', ',', '=', '+', '-', '*', '/', '%', '!', '<', '>', '&', '|', '^', '~'}

type Machine struct {
	context          *RuntimeContext
	frames           []*EmonFrame
	runActions       *foldMap[*EmonRunAction]
	triggers         []*EmonActionTrigger
	spriteText       *foldMap[string]
	sourceImages     *foldMap[*bishopgfx.BsgImage]
	imageCache       *foldMap[*bishopgfx.BsgImage]
	imageCacheNodes  map[string]*list.Element
	imageCacheOrder  *list.List
	imageCacheBytes  int64
	audioState       *foldMap[string]
	history          []string
	historyVoices    []string
	messageStyle     *foldMap[string]
	random           *dotnet.Random
	resolver         *machineVariableSource
	argumentResolver *machineVariableSource
	step             EmonStep
	backLog          *EmonBackLogWindow
	backLogRevision  int
	waitFrames       int
	messageLocked    bool
	// messageWaitClears 区分两种“等点击”：#Page 点击后清空文本框（原引擎
	// sub_4334C0），#Hit 点击后只隐藏继续图标、正文保留（sub_433900）。
	messageWaitClears bool
	locked            bool
	selectLocked      bool
	historyCapacity   int
	skip              bool
	skipMode          int
	auto              bool
	noSave            bool
	finished          bool
	resetRequested    bool
	promptIsYesNo     bool
	actionWaiting     bool
	// uiScreenSnapshot 记录“打开全屏 UI（存档/读档、汗メーター、系统菜单…）
	// 之前”各画面根精灵的可见性。存档是从这些界面里按下的，此时宏已经把
	// 背景关掉了；用这份快照入档，读档才能回到真正的游戏画面。
	uiScreenSnapshot map[string]bool
	// autoMenuRequested 由 #ApearMenu(AUTOSETTING) 置位，session 在下一帧
	// 打开引擎自绘的オートモード設定面板（AUTOWAIT/AUTOMIN/AUTOMAX）。
	autoMenuRequested    bool
	actionWaitFrameDepth int
	actionWaitFlags      int
	clickWaiting         bool
	polling              bool
	prompt               string
	hasPrompt            bool
	promptVariable       string
	pendingJump          string
	movie                string
	hasMovie             bool
	pendingMessage       string
	hasPendingMessage    bool
	pointerX             int
	pointerY             int
	pointerDown          bool
	hoveredSprite        string
	lastTapKey           string
	macroPrograms        *foldMap[*script.EmonProgram]
	loggedNames          *foldSet
	unresolvedJumps      *foldSet
	unresolvedLinks      *foldSet
	storyLogged          bool
	motions              map[string][]ActiveMotion
	colorSprites         *foldSet
	spriteParams         *foldMap[string]
	spriteRects          *foldMap[string]
	spriteResources      *foldMap[int]
	// spriteImages 记录每个精灵当前位图的来源（文件名或链到的精灵名），
	// 存档用。位图本身不存：读档时按名字从 archives 重新加载即可。
	spriteImages *foldMap[string]
	// spriteMasks 记录 SetSprMsk 的遮罩文件名，读档后重建。
	spriteMasks *foldMap[string]
	// spriteBaseVisible 记录 #SetSprite 声明时的可见性（displayToken 是否含
	// DISPON）。读档要把存档界面收起，但重新打开界面时脚本只会 SpriteSwitch
	// 根节点，子精灵（ＳＬ背景 等）依赖“声明时就是显示”的原始值；把它们
	// 永久留在隐藏会让读档页变成可点但全黑。
	spriteBaseVisible *foldMap[int]
	effect            *stage.EmonEffect
	editSprite        string
	editVariable      string
	areas             []ClickArea
	// spriteAreas 是 SPREREA 登记的精灵移动范围（滑杆/旋钮），不是点击热区。
	spriteAreas map[string]EmonRect
	// pendingSave 是 #MakeSave 抓下的快照，供其后由 Save 写盘；Save 不清空它，
	// 一次存档会话里的 Save(1000) 与最终 Save(slot) 共用同一快照。
	pendingSave        *EmonSaveData
	scratchSaves       map[int]*EmonSaveData
	scratchThumbs      map[int]*bishopgfx.BsgImage
	backLogFlushed     *foldMap[string]
	mouseFollow        *MouseFollow
	dragActive         bool
	lastHover          string
	cursorMoveLocked   bool
	systemStateReady   bool
	backLogCursor      int
	actionSoundChannel int
	channelFiles       map[int]string
	channelLoops       map[int]bool
	groupFiles         map[int]string
	// groupLoops 记录每个组当前声音是否循环。groupFiles 里也可能有非循环
	// 组音（StopSound(SEGROUPNOLOOP,N) 要能停掉），不能再拿 key 是否存在
	// 来推断循环属性。
	groupLoops             map[int]bool
	channelGroups          map[int]int
	channelFilesView       map[int]string
	seRoundRobin           int
	soundCompletions       []EmonSoundCompletion
	soundGate              sync.Mutex
	replayFrame            *EmonFrame
	replayTarget           int
	replayVariables        map[string]script.EmonValue
	spriteCreationOrder    *foldMap[int]
	spriteCreationSequence int
	selectOrderSequence    int

	// 既読/未読与宿主模式位：
	//   readKey         当前正文的稳定键（脚本名:源行号）
	//   currentTextRead 这一行在本次显示前是否已读（决定 0x10 既読位）
	//   hostSkip/hostAuto 宿主工具栏的 SKIP/AUTO 状态（决定 0x01/0x02/0x04）
	readKey         string
	currentTextRead bool
	hostSkip        bool
	hostAuto        bool
}

func NewMachine(context *RuntimeContext) *Machine {
	m := &Machine{
		context:              context,
		runActions:           newFoldMap[*EmonRunAction](),
		spriteText:           newFoldMap[string](),
		sourceImages:         newFoldMap[*bishopgfx.BsgImage](),
		imageCache:           newFoldMap[*bishopgfx.BsgImage](),
		imageCacheNodes:      make(map[string]*list.Element),
		imageCacheOrder:      list.New(),
		audioState:           newFoldMap[string](),
		messageStyle:         newFoldMap[string](),
		random:               dotnet.NewRandom(),
		actionWaitFrameDepth: -1,
		historyCapacity:      512,
		macroPrograms:        newFoldMap[*script.EmonProgram](),
		loggedNames:          newFoldSet(),
		unresolvedJumps:      newFoldSet(),
		unresolvedLinks:      newFoldSet(),
		motions:              make(map[string][]ActiveMotion),
		colorSprites:         newFoldSet(),
		spriteParams:         newFoldMap[string](),
		spriteRects:          newFoldMap[string](),
		spriteImages:         newFoldMap[string](),
		spriteMasks:          newFoldMap[string](),
		spriteBaseVisible:    newFoldMap[int](),
		spriteResources:      newFoldMap[int](),
		scratchSaves:         make(map[int]*EmonSaveData),
		scratchThumbs:        make(map[int]*bishopgfx.BsgImage),
		spriteAreas:          make(map[string]EmonRect),
		backLogFlushed:       newFoldMap[string](),
		actionSoundChannel:   -1,
		channelFiles:         make(map[int]string),
		channelLoops:         make(map[int]bool),
		groupFiles:           make(map[int]string),
		groupLoops:           make(map[int]bool),
		channelGroups:        make(map[int]int),
		channelFilesView:     make(map[int]string),
		spriteCreationOrder:  newFoldMap[int](),
	}
	m.resolver = &machineVariableSource{machine: m}
	m.argumentResolver = &machineVariableSource{machine: m, allowSymbolSuffix: true}
	// SkipModeFlag 是原引擎维护的系统变量：0x01 全跳过、0x02 既読跳过、
	// 0x04 AUTO、0x08 快速跳过（Ctrl）、0x10 当前行既読。脚本用 NUMTRUE
	// 触发器据此显示/隐藏 AUTO/SKIP/既読 图标，所以必须在首帧前就存在。
	m.SetSystemVariable("SkipModeFlag", script.EmonValueFromInt(0))
	return m
}

func (m *Machine) Context() *RuntimeContext {
	return m.context
}

func (m *Machine) Step() EmonStep {
	return m.step
}

func (m *Machine) MessageLocked() bool {
	return m.messageLocked
}

func (m *Machine) Locked() bool {
	return m.locked
}

func (m *Machine) SelectLocked() bool {
	return m.selectLocked
}

func (m *Machine) SkipEnabled() bool {
	return m.skip
}

func (m *Machine) SkipMode() int {
	return m.skipMode
}

func (m *Machine) AutoEnabled() bool {
	return m.auto
}

func (m *Machine) NoSave() bool {
	return m.noSave
}

func (m *Machine) ResetRequested() bool {
	return m.resetRequested
}

// HitSelect 暴露一次 SELECT 命中给会话层（读档槽位兜底用）。
func (m *Machine) HitSelect(x, y int) *stage.EmonSprite {
	if m.context == nil || m.context.Stage == nil {
		return nil
	}
	return m.context.Stage.HitTestSelect(x, y)
}

func (m *Machine) AutoMenuRequested() bool {
	return m.autoMenuRequested
}

func (m *Machine) ClearAutoMenuRequest() {
	m.autoMenuRequested = false
}

func (m *Machine) PendingPrompt() (string, bool) {
	return m.prompt, m.hasPrompt
}

func (m *Machine) PendingPromptIsYesNo() bool {
	return m.promptIsYesNo
}

func (m *Machine) PendingMovie() (string, bool) {
	return m.movie, m.hasMovie
}

func (m *Machine) PendingMessage() (string, bool) {
	return m.pendingMessage, m.hasPendingMessage
}

func (m *Machine) BackLog() []string {
	return m.history
}

func (m *Machine) BackLogActive() bool {
	return m.backLog != nil
}

func (m *Machine) BackLogRevision() int {
	return m.backLogRevision
}

func (m *Machine) SpriteTexts() map[string]string {
	return m.spriteText.Snapshot()
}

func (m *Machine) MessageStyle() map[string]string {
	return m.messageStyle.Snapshot()
}

func (m *Machine) AudioState() map[string]string {
	return m.audioState.Snapshot()
}

func (m *Machine) CachedImageCount() int {
	return m.imageCache.Len()
}

func (m *Machine) CachedImageBytes() int64 {
	return m.imageCacheBytes
}

func (m *Machine) StoredSourceImageCount() int {
	return m.sourceImages.Len()
}

func (m *Machine) CurrentProgram() string {
	if len(m.frames) == 0 {
		return ""
	}
	return m.frames[len(m.frames)-1].Program.Name
}

func (m *Machine) CurrentStatement() int32 {
	if len(m.frames) == 0 {
		return 0
	}
	return m.frames[len(m.frames)-1].Index
}

func (m *Machine) FrameDepth() int {
	return len(m.frames)
}

func (m *Machine) PointerX() int {
	return m.pointerX
}

func (m *Machine) PointerY() int {
	return m.pointerY
}

func (m *Machine) Replaying() bool {
	return m.replayFrame != nil
}

func (m *Machine) Run(budget int) EmonStep {
	m.tick()
	if m.backLog != nil {
		m.step = EmonStepWait
		return EmonStepWait
	}
	m.pollConditionTriggers()
	if m.clickWaiting {
		return m.step
	}
	if m.step == EmonStepWait {
		if m.waitFrames > 0 {
			return EmonStepWait
		}
		m.step = EmonStepContinue
	}
	if m.finished {
		return EmonStepFinished
	}
	if m.step == EmonStepAwaitAction {
		// ActionWait 是真正的“等玩家在界面里选一个动作”：只有选中动作
		// (ACTWAITOFF/触发器) 把 actionWaiting 清掉后才继续执行宏后续的
		// IfJump 分派。以前无条件 drain 会在没有选择时直接落进 fallback
		// (SystemWindow 打开即立刻退出)。
		if m.actionWaiting {
			return m.step
		}
		m.drainActionWaitFrames()
		return m.step
	}
	if m.step != EmonStepContinue {
		return m.step
	}
	remaining := budget
	if remaining <= 0 {
		remaining = defaultBudget
	}
	for remaining > 0 {
		remaining--
		step := m.runNext()
		if step == EmonStepContinue {
			continue
		}
		m.setStep(step)
		if m.backLog != nil {
			m.step = EmonStepWait
			return EmonStepWait
		}
		if step == EmonStepWait {
			if m.waitFrames > 0 {
				return EmonStepWait
			}
			m.step = EmonStepContinue
			continue
		}
		return step
	}
	return EmonStepContinue
}

func (m *Machine) runNext() (step EmonStep) {
	defer func() {
		if recovered := recover(); recovered != nil {
			core.Logf("emon: %s", m.statementFailure(fmt.Sprintf("%T: %v", recovered, recovered)))
			step = EmonStepContinue
		}
	}()
	return m.executeNext()
}

func (m *Machine) hasMacroFrame() bool {
	return len(m.frames) != 0 && m.frames[len(m.frames)-1].Macro != nil
}

func (m *Machine) Advance() EmonStep {
	if m.clickWaiting {
		return m.step
	}
	switch m.step {
	case EmonStepAwaitInput:
		// 工具条/界面按钮会先 push 一个 action 宏，宏里的 #Lock 是给对白
		// 推进用的；如果这里直接用 MessageLocked 拦截，宏永远不执行
		// （存档/读档/系统设置点开就卡住）。栈顶是宏时让它先跑。
		if m.MessageLocked() && !m.hasMacroFrame() {
			return m.step
		}
		if m.hasPrompt && !m.promptIsYesNo {
			return m.step
		}
		// 原引擎的第一次点击是“把本页正文立刻显示完”，而不是翻页：
		// 逐字显示由 MessageSpeed 控制，点击只负责跳过打字机。少了这一步，
		// 长文本会在打字过程中被下一次 #Page 直接覆盖，玩家永远读不到前半段。
		if m.context.Message.Body() != "" && !m.context.Message.FullyRevealed() {
			m.context.Message.RevealAll()
			m.applyMessageIndicators(m.step)
			return m.step
		}
		// 原引擎只有 #Page（sub_4334C0）会在点击后清空文本框；
		// #Hit（sub_433900）只隐藏"继续"图标，正文留着继续累积。
		if m.messageWaitClears {
			m.clearIncomingMessage()
		}
		m.messageWaitClears = false
		m.step = EmonStepContinue
		return m.Run(0)
	case EmonStepAwaitAction:
		m.drainActionWaitFrames()
		return m.step
	case EmonStepAwaitName:
		return m.step
	case EmonStepWait, EmonStepContinue:
		return m.Run(0)
	default:
		return m.step
	}
}

func (m *Machine) drainActionWaitFrames() {
	if m.step != EmonStepAwaitAction {
		return
	}
	if m.actionWaitFrameDepth < 0 {
		m.actionWaitFrameDepth = len(m.frames)
		return
	}
	if len(m.frames) <= m.actionWaitFrameDepth {
		return
	}
	remaining := actionWaitStepBudget
	for len(m.frames) > m.actionWaitFrameDepth && remaining > 0 {
		remaining--
		outcome := m.runNext()
		if m.step != EmonStepAwaitAction {
			return
		}
		if outcome != EmonStepContinue {
			if outcome == EmonStepFinished || outcome == EmonStepFaulted {
				m.setStep(outcome)
			}
			return
		}
	}
}

func (m *Machine) ChooseSelect(selectNumber int) EmonStep {
	if m.step == EmonStepAwaitName {
		return m.step
	}
	sprite := m.selectByPosition(selectNumber)
	if sprite == nil {
		core.Logf("emon: select position %d was not found", selectNumber)
		return m.step
	}
	return m.ChooseSelectSprite(sprite.Name())
}

func (m *Machine) ChooseSelectSprite(spriteName string) EmonStep {
	sprite := m.findSprite(spriteName)
	if sprite == nil {
		core.Logf("emon: select sprite '%s' was not found", spriteName)
		return m.step
	}
	return m.clickSprite(sprite)
}

func (m *Machine) Tap(x, y int, rightButton bool) EmonStep {
	m.dragActive = false
	m.pointerX = x
	m.pointerY = y
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
	if m.backLog != nil {
		return m.backLogPointer(x, y, rightButton)
	}
	m.releaseClickWait()
	// #Lock 只锁“推进对白”（见 onLock / Advance 里的 MessageLocked 判定），
	// 不锁精灵触发器。原引擎的 sub_422510 只动消息送り标志，而标题菜单正是
	// 在 #Lock 之后的轮询循环里等 SPRCLICK 把 SelectNumber 设好；若这里按
	// Lock 跳过命中测试，标题所有按钮都会变成死键（NOZOMI 的回归）。
	locked := m.Locked()
	sprite := m.context.Stage.HitTestSelect(x, y)
	input := EmonInputLeftClick
	if rightButton {
		input = EmonInputRightClick
	}
	pendingBefore := m.actionWaiting
	framesBefore := len(m.frames)
	fired := m.fireTriggers(sprite, input, x, y, 0)
	// 工具条按钮（ボタン1-9/A/B 等）は SPRCLICK で宏を立ち上げるシステム UI
	// で、原引擎では对白を進めない。ここでそのまま Advance() すると
	// AwaitInput のメッセージが clearIncomingMessage で消え、進んだことに
	// なる。触发器が発火した点击はメッセージを残したまま宏だけを走らせる
	// (宏本体は次の Run で進む)。発火しなかった点击だけが对白を進める。
	if fired {
		// 触发器点击不推进对白，但要立刻把 action 宏跑起来：只把宏留在队列里、
		// 等下一次点击才执行时，宿主看到的是 AwaitInput（ticker 不会主动 Run），
		// 存档/读档/系统设置点一下没反应。这里只跑宏，不调用 Advance，
		// 所以当前这句对白不会被 clearIncomingMessage 清掉。
		//
		// 例外：已经处于 #ActionWait 的界面（系统设置、存档页）里点选一个
		// 设置项时，原引擎只执行该项的 run action 并继续等待；若这里无条件
		// 把 step 改成 Continue，宏会越过 ActionWait 直接落到 fallback（点一下
		// 文字速度就退出系统界面）。只有触发器里的 ACTWAITOFF 真把
		// actionWaiting 清掉（选择“返回/结束”等）才继续跑宏。
		if m.actionWaiting {
			// 等待中的界面点了“压入宏”的按钮（オート設定実行、读档槽位的
			// 备注键等）：只执行新压入的宏，不能清掉外层 ActionWait。
			if len(m.frames) > framesBefore {
				m.drainActionWaitFrames()
			}
			name := ""
			if sprite != nil {
				name = sprite.Name()
			}
			m.logTap(x, y, locked, name, pendingBefore && !m.actionWaiting)
			return m.step
		}
		if m.hasMacroFrame() {
			m.step = EmonStepContinue
			outcome := m.Run(0)
			name := ""
			if sprite != nil {
				name = sprite.Name()
			}
			m.logTap(x, y, locked, name, pendingBefore && !m.actionWaiting)
			return outcome
		}
		outcome := m.step
		name := ""
		if sprite != nil {
			name = sprite.Name()
		}
		m.logTap(x, y, locked, name, pendingBefore && !m.actionWaiting)
		return outcome
	}
	outcome := m.Advance()
	name := ""
	if sprite != nil {
		name = sprite.Name()
	}
	m.logTap(x, y, locked, name, pendingBefore && !m.actionWaiting)
	return outcome
}

func (m *Machine) RightClick() {
	m.dragActive = false
	if m.backLog != nil {
		m.closeBackLog()
		return
	}
	m.releaseClickWait()
	if m.fireTriggers(nil, EmonInputRightClick, m.pointerX, m.pointerY, 0) {
		m.Advance()
	}
}

func (m *Machine) Wheel(delta int) {
	if m.backLog != nil {
		if delta > 0 {
			m.scrollBackLog(-1, true)
		} else {
			m.scrollBackLog(1, true)
		}
		return
	}
	m.releaseClickWait()
	input := EmonInputWheelUp
	if delta < 0 {
		input = EmonInputWheelDown
	}
	if m.fireTriggers(nil, input, m.pointerX, m.pointerY, 0) {
		m.Advance()
	}
}

func (m *Machine) PressKey(keyCode int) {
	if m.backLog != nil {
		m.backLogKey(keyCode)
		return
	}
	m.releaseClickWait()
	if m.fireTriggers(nil, EmonInputKey, m.pointerX, m.pointerY, keyCode) {
		m.Advance()
	}
}

func (m *Machine) releaseClickWait() {
	if !m.clickWaiting {
		return
	}
	m.clickWaiting = false
	if m.step == EmonStepWait {
		m.step = EmonStepContinue
	}
}

func (m *Machine) clickSprite(sprite *stage.EmonSprite) EmonStep {
	pendingBefore := m.actionWaiting
	m.fireTriggers(sprite, EmonInputLeftClick, m.pointerX, m.pointerY, 0)
	outcome := m.Advance()
	m.logTap(m.pointerX, m.pointerY, false, sprite.Name(), pendingBefore && !m.actionWaiting)
	return outcome
}

func (m *Machine) selectByPosition(position int) *stage.EmonSprite {
	if position < 1 {
		return nil
	}
	index := position - 1
	for _, sprite := range m.context.Stage.SelectSprites() {
		if !m.isChainVisible(sprite) {
			continue
		}
		if index == 0 {
			return sprite
		}
		index--
	}
	return nil
}

func (m *Machine) logTap(x, y int, locked bool, spriteName string, consumed bool) {
	var key string
	if spriteName == "" {
		key = fmt.Sprintf("miss:%d,%d", x, y)
	} else {
		key = "hit:" + spriteName
	}
	if key == m.lastTapKey {
		return
	}
	m.lastTapKey = key
	detail := "sprite none"
	if spriteName != "" {
		detail = "sprite '" + spriteName + "'"
	}
	core.Logf("emon: tap (%d,%d) locked %v: %s wait-consumed %v", x, y, locked, detail, consumed)
}

func (m *Machine) SetPointer(x, y int) {
	if x == m.pointerX && y == m.pointerY {
		return
	}
	m.pointerX = x
	m.pointerY = y
	if m.backLog != nil {
		m.backLogPointer(x, y, false)
		return
	}
	sprite := m.context.Stage.HitTest(x, y)
	if sprite != nil {
		m.hoveredSprite = sprite.Name()
	} else {
		m.hoveredSprite = ""
	}
	// 与 Tap 同理：#Lock 锁的是对白推进，不是 SPRMOUSEOVER/SPROVER。
	// BOKUONE 的对话框工具条（Q.SAVE/SAVE/LOAD…）初始藏在窗口下方，
	// 正是靠底部 1024x40 的 メニュー選択範囲 上的 SPRMOUSEOVER 执行
	// 「メニュー表示」宏滑入；若锁定期间丢弃 hover，工具条永远不会出现。
	m.firePointerTriggers(sprite)
	m.drainActionWaitFrames()
}

func (m *Machine) PointerPress(x, y int) {
	m.dragActive = false
	m.pointerDown = true
	m.pointerX = x
	m.pointerY = y
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
	if m.backLog != nil {
		m.backLogPointer(x, y, false)
		return
	}
	sprite := m.context.Stage.HitTest(x, y)
	if sprite != nil {
		m.hoveredSprite = sprite.Name()
	} else {
		m.hoveredSprite = ""
	}
	// 按住也要派发触发器：设置页的滑杆用 SPRCLICK 建立拖动、
	// 部分界面用“按住不放”改变按钮状态；Lock 期间同样需要。
	m.firePointerTriggers(sprite)
	m.drainActionWaitFrames()
}

func (m *Machine) PointerRelease(x, y int) {
	m.dragActive = false
	m.pointerDown = false
	m.pointerX = x
	m.pointerY = y
	m.mouseFollow = nil
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
}

// PointerDrag 处理触屏拖动：拖动期间按“按住”语义派发 SPRDRAG/SPRMOUSEMOVE，
// 但结束后恢复原来的按下状态。宿主只发 drag 不发 press/release，若把
// pointerDown 永久置真，下一次普通 hover 也会拖走滑块；这里只在本帧有效。
func (m *Machine) PointerDrag(x, y int) {
	// 一次拖动手势只在第一帧补发一次左键触发器：设置界面的音量滑杆是靠
	// 滑杆范围上的 SPRCLICK → SPRMOUSEMOVE 建立“旋钮跟随鼠标”的，之后
	// 每帧移动不再重复点击，避免连续播放点击音/重复执行动作。
	if !m.dragActive {
		m.dragActive = true
		// 拖动同样不受 #Lock 限制：锁定发生在对白等待时，而设置页的滑杆
		// 就在这种状态下操作；丢掉第一帧 SPRCLICK 会让 SPRMOUSEMOVE 的
		// 跟随关系建立不起来，滑杆无法拖动。
		m.fireTriggers(m.context.Stage.HitTestSelect(x, y), EmonInputLeftClick, x, y, 0)
	}
	previous := m.pointerDown
	m.pointerDown = true
	m.PointerMove(x, y)
	m.pointerDown = previous
}

func (m *Machine) PointerMove(x, y int) {
	moved := x != m.pointerX || y != m.pointerY
	if !moved && !m.pointerDown {
		return
	}
	m.pointerX = x
	m.pointerY = y
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
	if m.backLog != nil {
		m.backLogPointer(x, y, false)
		return
	}
	sprite := m.context.Stage.HitTest(x, y)
	if sprite != nil {
		m.hoveredSprite = sprite.Name()
	} else {
		m.hoveredSprite = ""
	}
	// 拖动/移动期间的触发器同样不因 #Lock 停发：滑杆跟随、车条、hover 高亮
	// 都属于界面操作，与“对白能否推进”无关（原引擎两者是不同标志）。
	m.firePointerTriggers(sprite)
	m.drainActionWaitFrames()
	if !m.pointerDown || !moved || !m.fireDragTriggers(sprite, x, y) {
		return
	}
	m.Advance()
}

func (m *Machine) RequestJump(target string) {
	if strings.TrimSpace(target) == "" {
		return
	}
	m.pendingJump = target
	if m.step != EmonStepFinished {
		m.step = EmonStepContinue
	}
}

func (m *Machine) SubmitText(text string) {
	if m.step != EmonStepAwaitName && !m.hasPrompt {
		return
	}
	if strings.TrimSpace(text) == "" {
		var kept string
		if m.promptVariable != "" {
			stored := m.Lookup(m.promptVariable)
			if !isMiss(stored, m.promptVariable) {
				kept = stored.AsString()
			}
		}
		m.logOnce("empty text for prompt '" + m.prompt + "' kept '" + kept + "'")
		m.finishPrompt()
		return
	}
	value := script.EmonValueFromString(text)
	if m.promptVariable != "" {
		m.SetVariable(m.promptVariable, value)
	}
	m.finishPrompt()
}

func (m *Machine) SubmitYesNo(yes bool) {
	if !m.hasPrompt {
		return
	}
	if m.promptVariable != "" {
		if yes {
			m.SetVariable(m.promptVariable, script.EmonValueFromInt(1))
		} else {
			m.SetVariable(m.promptVariable, script.EmonValueFromInt(0))
		}
	}
	m.finishPrompt()
}

func (m *Machine) DismissMessage() {
	if !m.hasPendingMessage {
		return
	}
	m.pendingMessage = ""
	m.hasPendingMessage = false
	m.step = EmonStepContinue
}

func (m *Machine) finishPrompt() {
	m.prompt = ""
	m.hasPrompt = false
	m.promptVariable = ""
	m.promptIsYesNo = false
	m.step = EmonStepContinue
}

func (m *Machine) clearIncomingMessage() {
	m.pendingMessage = ""
	m.hasPendingMessage = false
	m.context.Message.Clear()
}

func (m *Machine) setPendingMessage(text string) {
	m.pendingMessage = text
	m.hasPendingMessage = true
}

func (m *Machine) setPrompt(prompt, variable string, yesNo bool) {
	m.prompt = prompt
	m.hasPrompt = true
	m.promptVariable = variable
	m.promptIsYesNo = yesNo
}

func (m *Machine) setMovie(movie string) {
	m.movie = movie
	m.hasMovie = true
}

func (m *Machine) tick() {
	frames := m.frameTicks()
	if m.waitFrames > 0 {
		m.waitFrames -= frames
		if m.waitFrames < 0 {
			m.waitFrames = 0
		}
	}
	m.pollSoundEndTriggers()
}

func (m *Machine) PumpFrame() {
	// 逐字显示期间“继续”图标要等正文显示完才出现（applyMessageIndicators
	// 用 FullyRevealed 判定）；#Page 之后没有别的语句会再刷新它，所以每帧
	// 重算一次。只改两个精灵的可见性，开销可以忽略。
	m.applyMessageIndicators(m.step)
	// 条件触发器原引擎按帧轮询。只放在 Run 里会导致停在 AwaitInput 时
	// 既不刷新 AUTO/SKIP/既読 图标，也不响应 #FlagOn 一类后台事件。
	m.pollConditionTriggers()
	// 动画（SetSprMtn）的 OFFACTION 通常写成「アクションＯＦＦ」(ACTWAITOFF)。
	// 选择肢的退场动画就是在 #ActionWait 期间靠这个回调结束等待的；以前只在
	// executeNext/fireTriggers 里推进动画，停在 ActionWait 时永远不会播放到
	// 结束，选择肢点击后会卡死在第二次 ActionWait（看起来像点选项没反应）。
	m.pumpMotions()
	m.refreshScreenRootsBaseline()
	m.pollSoundEndTriggers()
}

func (m *Machine) frameTicks() int {
	delta := m.context.FrameDelta
	if isNaN(delta) || delta <= 0 {
		return 1
	}
	frames := roundAwayFromZero(delta * framesPerSecond)
	if frames < 1 {
		return 1
	}
	return frames
}

func (m *Machine) statementFailure(detail string) string {
	name := "?"
	var index int32
	if len(m.frames) != 0 {
		name = m.frames[len(m.frames)-1].Program.Name
		index = m.frames[len(m.frames)-1].Index
	}
	return fmt.Sprintf("statement failed in '%s' at index %d: %s", name, index, detail)
}

func (m *Machine) argument(statement *script.EmonStatement, index int32) script.EmonArgument {
	return statement.Argument(index)
}

func (m *Machine) raw(statement *script.EmonStatement, index int32) string {
	arg := statement.Argument(index)
	return arg.Raw
}

func (m *Machine) text(statement *script.EmonStatement, index int32) string {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return ""
	}
	if arg.WasQuoted {
		return arg.Raw
	}
	return m.valueOf(arg.Raw, false).AsString()
}

func (m *Machine) labelText(statement *script.EmonStatement, index int32) string {
	arg := statement.Argument(index)
	raw := arg.Raw
	if raw == "" || arg.WasQuoted {
		return raw
	}
	value := m.Lookup(raw)
	if isMiss(value, raw) {
		return raw
	}
	return value.AsString()
}

func (m *Machine) value(statement *script.EmonStatement, index int32) script.EmonValue {
	arg := statement.Argument(index)
	if arg.Raw == "" {
		return script.EmonValueFromString("")
	}
	return m.valueOf(arg.Raw, arg.WasQuoted)
}

func (m *Machine) valueOf(raw string, quoted bool) script.EmonValue {
	if raw == "" {
		return script.EmonValueFromString("")
	}
	if quoted {
		return script.EmonValueFromString(raw)
	}
	return script.Evaluate(raw, m.argumentResolver)
}

func (m *Machine) condition(expression string) bool {
	return expression != "" && script.EvaluateCondition(expression, m.resolver)
}

func (m *Machine) conditionAt(statement *script.EmonStatement, index int32) bool {
	return m.condition(m.raw(statement, index))
}

func (m *Machine) conditionsFrom(statement *script.EmonStatement, start int32) bool {
	for i := start; i < statement.ArgumentCount(); i++ {
		if !m.conditionAt(statement, i) {
			return false
		}
	}
	return true
}

func (m *Machine) Lookup(name string) script.EmonValue {
	return m.lookup(name, false)
}

func (m *Machine) LookupArgument(name string) script.EmonValue {
	return m.lookup(name, true)
}

func (m *Machine) lookup(name string, allowSymbolSuffix bool) script.EmonValue {
	if name == "" {
		return script.EmonValueFromString("")
	}
	if args := m.topArguments(); args != nil {
		local := args.Lookup(name)
		if !isMiss(local, name) {
			return storedValue(local)
		}
	}
	// 已赋值变量里可能保存着 identifier 形态（脚本写 #SetNumber(コメント=オープニング)
	// 这种不带引号的字符串）。原引擎只有字符串/数字类型，查出来应一律按字符串用；
	// 若把 identifier 原样交给表达式求值器，変数１==0 会走 tryGetInteger 得到 (0,true)，
	// 把 "オープニング" 误判成 0（ASENURE 的存档页因此把有存档的槽位当空槽）。
	// 未定义名字的最后兜底仍保留 identifier，用来表达“查不到”。
	if system := m.context.SystemVariables.Lookup(name); !isMiss(system, name) {
		return storedValue(system)
	}
	if variable := m.context.Variables.Lookup(name); !isMiss(variable, name) {
		return storedValue(variable)
	}
	if symbol := m.context.Symbols.Lookup(name); !isMiss(symbol, name) {
		return storedValue(symbol)
	}
	if allowSymbolSuffix {
		if fused, ok := m.tryFusedSymbol(name); ok {
			return fused
		}
	}
	return script.EmonValueFromIdentifier(name)
}

// storedValue 把已经赋过值的 identifier 统一成字符串。identifier 只应表示
// “这个名字还没定义”；一旦它作为值被存进变量/宏参数，它就是普通字符串。
func storedValue(value script.EmonValue) script.EmonValue {
	if value.IsIdentifier() {
		return script.EmonValueFromString(value.AsString())
	}
	return value
}

func (m *Machine) tryFusedSymbol(name string) (script.EmonValue, bool) {
	miss := script.EmonValueFromIdentifier(name)
	if !isSymbolToken(name) {
		return miss, false
	}
	matched := ""
	fused := script.EmonValueFromString("")
	for _, entry := range m.context.Symbols.Entries() {
		if len(entry.Name) >= len(name) || len(entry.Name) <= len(matched) {
			continue
		}
		if !strutil.HasPrefixFold(name, entry.Name) {
			continue
		}
		matched = entry.Name
		fused = entry.Value.Add(script.EmonValueFromString(name[len(entry.Name):]))
	}
	if matched == "" {
		return miss, false
	}
	return fused, true
}

func isSymbolToken(text string) bool {
	if text == "" {
		return false
	}
	first := []rune(text)[0]
	if first == '.' {
		return false
	}
	if first >= '0' && first <= '9' {
		return false
	}
	if first >= '０' && first <= '９' {
		return false
	}
	for _, separator := range tokenSeparators {
		if strings.ContainsRune(text, separator) {
			return false
		}
	}
	return true
}

func isMiss(value script.EmonValue, name string) bool {
	return !value.IsInteger() && value.AsString() == name
}

func (m *Machine) dropShadowedLocalVariables() {
	dropped := 0
	for name := range m.context.SystemVariables.Snapshot() {
		if !m.context.Variables.Contains(name) {
			continue
		}
		m.context.Variables.Remove(name)
		dropped++
	}
	if dropped != 0 {
		core.Logf("emon: load dropped %d local variables shadowing the system store", dropped)
	}
}

func (m *Machine) noteSystemWrite(name string, value script.EmonValue) {
	if value.IsInteger() {
		m.logOnce(fmt.Sprintf("system variable %s=%d", name, value.AsInt()))
	}
}

func (m *Machine) SetVariable(name string, value script.EmonValue) {
	if name == "" {
		return
	}
	if args := m.topArguments(); args != nil {
		if !isMiss(args.Lookup(name), name) {
			args.Set(name, value)
			return
		}
	}
	if m.context.SystemVariables.Contains(name) {
		if !m.isReplaying() {
			m.context.SystemVariables.Set(name, value)
			m.noteSystemWrite(name, value)
		}
		return
	}
	m.context.Variables.Set(name, value)
}

func (m *Machine) SetSystemVariable(name string, value script.EmonValue) {
	if name == "" {
		return
	}
	m.context.SystemVariables.Set(name, value)
	m.noteSystemWrite(name, value)
	m.context.Variables.Remove(name)
}

func (m *Machine) DeclareSystemVariable(name string, value script.EmonValue) {
	if name == "" {
		return
	}
	m.context.Variables.Remove(name)
	if m.isReplaying() || m.context.SystemVariables.Contains(name) {
		return
	}
	m.context.SystemVariables.Set(name, value)
	m.noteSystemWrite(name, value)
}

func (m *Machine) SetSymbol(name string, value script.EmonValue) {
	if name != "" {
		m.context.Symbols.Set(name, value)
		m.SetVariable(name, value)
	}
}

func (m *Machine) topArguments() *script.EmonVariableStore {
	if len(m.frames) == 0 {
		return nil
	}
	if macro := m.frames[len(m.frames)-1].Macro; macro != nil {
		return macro.Arguments
	}
	return nil
}

func (m *Machine) findSprite(name string) *stage.EmonSprite {
	if name == "" || isMainSprite(name) {
		return m.context.Stage.Root()
	}
	return m.context.Stage.Find(name)
}

func (m *Machine) resolveSprite(name string) *stage.EmonSprite {
	if name == "" || isMainSprite(name) {
		return m.context.Stage.Root()
	}
	return m.context.Stage.GetOrCreate(name, nil)
}

func isMainSprite(name string) bool {
	return strings.EqualFold(name, "メイン") || strings.EqualFold(name, "Main")
}

func (m *Machine) LoadImage(name string) *bishopgfx.BsgImage {
	if strings.TrimSpace(name) == "" || strings.EqualFold(name, "NULL") {
		return nil
	}
	if cached, exists := m.imageCache.Get(name); exists {
		m.touchImage(name)
		return cached
	}
	image := m.readImage(name)
	if image == nil {
		core.Logf("emon: image '%s' was not found", name)
		return nil
	}
	m.cacheImage(name, image)
	return image
}

func (m *Machine) cacheImage(name string, image *bishopgfx.BsgImage) {
	if previous, exists := m.imageCache.Get(name); exists {
		m.imageCacheBytes -= int64(previous.ByteCount())
	}
	m.imageCache.Set(name, image)
	m.imageCacheBytes += int64(image.ByteCount())
	m.touchImage(name)
	for m.imageCache.Len() > imageCacheCapacity || m.imageCacheBytes > imageCacheBudgetBytes {
		oldest := m.imageCacheOrder.Front()
		if oldest == nil || oldest.Value.(string) == name {
			break
		}
		m.dropImage(oldest.Value.(string))
	}
}

func (m *Machine) touchImage(name string) {
	key := strings.ToUpper(name)
	if node, exists := m.imageCacheNodes[key]; exists {
		m.imageCacheOrder.Remove(node)
		m.imageCacheNodes[key] = m.imageCacheOrder.PushBack(name)
		return
	}
	m.imageCacheNodes[key] = m.imageCacheOrder.PushBack(name)
}

func (m *Machine) dropImage(name string) {
	if image, exists := m.imageCache.Get(name); exists {
		m.imageCacheBytes -= int64(image.ByteCount())
		m.imageCache.Delete(name)
	}
	key := strings.ToUpper(name)
	if node, exists := m.imageCacheNodes[key]; exists {
		delete(m.imageCacheNodes, key)
		m.imageCacheOrder.Remove(node)
	}
}

func (m *Machine) ReleaseCaches() {
	m.imageCache.Clear()
	m.imageCacheNodes = make(map[string]*list.Element)
	m.imageCacheOrder.Init()
	m.imageCacheBytes = 0
	m.sourceImages.Clear()
	m.macroPrograms.Clear()
	m.scratchThumbs = make(map[int]*bishopgfx.BsgImage)
	core.Logf("emon: image caches released")
}

func cropImage(source *bishopgfx.BsgImage, left, top, columns, rows int) *bishopgfx.BsgImage {
	pixels := make([]byte, columns*rows*4)
	sourcePixels := source.Pixels
	sourceStride := source.Width * 4
	rowBytes := columns * 4
	for row := 0; row < rows; row++ {
		sourceOffset := (top+row)*sourceStride + left*4
		take := rowBytes
		if remaining := len(sourcePixels) - sourceOffset; remaining < take {
			take = remaining
		}
		if take <= 0 {
			continue
		}
		copy(pixels[row*rowBytes:], sourcePixels[sourceOffset:sourceOffset+take])
	}
	return bishopgfx.NewBsgImage(columns, rows, 0, 0, pixels)
}

func scaleImage(source *bishopgfx.BsgImage, width, height int) *bishopgfx.BsgImage {
	pixels := make([]byte, width*height*4)
	if source.Width <= 0 || source.Height <= 0 {
		return bishopgfx.NewBsgImage(width, height, 0, 0, pixels)
	}
	sourcePixels := source.Pixels
	sourceStride := source.Width * 4
	for row := 0; row < height; row++ {
		sourceRow := int(int64(row) * int64(source.Height) / int64(height) * int64(sourceStride))
		targetOffset := row * width * 4
		for column := 0; column < width; column++ {
			sourceOffset := sourceRow + int(int64(column)*int64(source.Width)/int64(width))*4
			if sourceOffset+4 > len(sourcePixels) {
				continue
			}
			copy(pixels[targetOffset+column*4:], sourcePixels[sourceOffset:sourceOffset+4])
		}
	}
	return bishopgfx.NewBsgImage(width, height, 0, 0, pixels)
}

func readCanvas(source *bishopgfx.BgraCanvas) *bishopgfx.BsgImage {
	buffer := make([]byte, len(source.Buffer))
	copy(buffer, source.Buffer)
	return bishopgfx.NewBsgImage(source.Width, source.Height, 0, 0, buffer)
}

func cropCanvas(source *bishopgfx.BgraCanvas, left, top, columns, rows int) *bishopgfx.BsgImage {
	return cropImage(bishopgfx.NewBsgImage(source.Width, source.Height, 0, 0, source.Buffer), left, top, columns, rows)
}

func (m *Machine) readImage(name string) *bishopgfx.BsgImage {
	for _, candidate := range imageNameCandidates(name) {
		entry, archive := m.context.Archives.Locate(candidate)
		if entry != nil && archive != nil {
			content, err := archive.ReadEntry(entry)
			if err != nil {
				core.Logf("emon: image '%s' failed: %s", name, err.Error())
				return nil
			}
			if entry.IsImage() && len(content) > imageHeaderSize {
				return emongfx.Decode(content[:imageHeaderSize], content[imageHeaderSize:], int(entry.FrameSize), int(entry.FrameInitPos))
			}
			return emongfx.LoadBmp(content)
		}
		if direct := m.context.Archives.Read(candidate); len(direct) != 0 {
			return emongfx.LoadBmp(direct)
		}
		if m.context.RootDirectory != "" {
			for _, fileName := range imageCandidates(candidate) {
				path := filepath.Join(m.context.RootDirectory, fileName)
				if _, err := os.Stat(path); err != nil {
					continue
				}
				content, err := os.ReadFile(path)
				if err != nil {
					core.Logf("emon: image '%s' failed: %s", name, err.Error())
					return nil
				}
				return emongfx.LoadBmp(content)
			}
		}
	}
	return nil
}

func imageNameCandidates(name string) []string {
	candidates := []string{name}
	if index := indexOfToken(name, ".png"); index >= 0 {
		candidates = append(candidates, name[:index]+".bmp"+name[index+len(".png"):])
	}
	if strutil.HasSuffixFold(name, ".png.bmp") {
		candidates = append(candidates, name[:len(name)-8]+".bmp")
	}
	if len(name) > 1 && (name[0] == 's' || name[0] == 'S') && strutil.HasSuffixFold(name[1:], ".bmp") {
		candidates = append(candidates, name[1:])
	}
	return candidates
}

func imageCandidates(name string) []string {
	candidates := []string{name}
	for _, extension := range []string{".bmp", ".png"} {
		if !strutil.HasSuffixFold(name, extension) {
			candidates = append(candidates, name+extension)
		}
	}
	return candidates
}

func (m *Machine) Crop(source *bishopgfx.BsgImage, x, y, width, height int) *bishopgfx.BsgImage {
	if width <= 0 || height <= 0 {
		return nil
	}
	left := core.Clamp(x, 0, max(0, source.Width-1))
	top := core.Clamp(y, 0, max(0, source.Height-1))
	columns := min(width, source.Width-left)
	rows := min(height, source.Height-top)
	if columns <= 0 || rows <= 0 {
		return nil
	}
	if left == 0 && top == 0 && columns == source.Width && rows == source.Height {
		return source
	}
	return cropImage(source, left, top, columns, rows)
}

func (m *Machine) StoreSource(name string, image *bishopgfx.BsgImage) {
	m.sourceImages.Set(name, image)
}

func (m *Machine) SourceImage(name string) *bishopgfx.BsgImage {
	if image, exists := m.sourceImages.Get(name); exists {
		return image
	}
	return nil
}

func (m *Machine) SetSpriteText(name, text string) {
	if name == "" {
		return
	}
	if text == "" {
		m.spriteText.Delete(name)
	} else {
		m.spriteText.Set(name, text)
	}
	if strings.EqualFold(name, "名前") {
		m.context.Message.SetSpeaker(text)
	}
}

func (m *Machine) AddHistory(text string) {
	if text == "" {
		return
	}
	m.history = append(m.history, text)
	m.historyVoices = append(m.historyVoices, m.currentVoiceFile())
	for len(m.history) > m.historyCapacity {
		m.history = m.history[1:]
		m.historyVoices = m.historyVoices[1:]
	}
	for len(m.historyVoices) > len(m.history) {
		m.historyVoices = m.historyVoices[:len(m.historyVoices)-1]
	}
}

func (m *Machine) currentVoiceFile() string {
	if voice, exists := m.audioState.Get("voice"); exists {
		return voice
	}
	return ""
}

func (m *Machine) ClearHistory() {
	m.history = nil
	m.historyVoices = nil
}

func (m *Machine) RandomValue(count int) script.EmonValue {
	if count <= 0 {
		return script.EmonValueFromInt(0)
	}
	return script.EmonValueFromInt(int32(m.random.Next(count)))
}

func (m *Machine) randomValue(count int) script.EmonValue {
	return m.RandomValue(count)
}

func (m *Machine) mutableAudioState() *foldMap[string] {
	return m.audioState
}

func (m *Machine) setStep(step EmonStep) EmonStep {
	m.step = step
	if step == EmonStepFinished {
		m.finished = true
	}
	return step
}

func (m *Machine) isReplaying() bool {
	return m.replayFrame != nil
}

func (m *Machine) nextReplayPick(destination string, count int) int {
	if count <= 0 {
		return 0
	}
	if m.isReplaying() && destination != "" {
		saved := int(m.Lookup(destination).AsInt())
		if saved >= 0 && saved < count {
			return saved
		}
	}
	return m.random.Next(count)
}

func (m *Machine) finishReplay() {
	m.replayFrame = nil
	if m.replayVariables != nil {
		m.context.Variables.LoadFrom(m.replayVariables)
		m.dropShadowedLocalVariables()
		m.replayVariables = nil
	}
	m.reissueAudio()
}

func (m *Machine) evalExpression(expression string) int {
	if strings.TrimSpace(expression) == "" {
		return 0
	}
	return int(script.Evaluate(expression, m.resolver).AsInt())
}

type machineVariableSource struct {
	machine           *Machine
	allowSymbolSuffix bool
}

func (s *machineVariableSource) Lookup(name string) script.EmonValue {
	return s.machine.lookup(name, s.allowSymbolSuffix)
}
