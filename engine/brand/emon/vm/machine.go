package vm

import (
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	emongfx "kizuna/engine/brand/emon/gfx"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	"kizuna/engine/core"
	"kizuna/engine/core/dotnet"
	"kizuna/engine/core/strutil"
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
	context                *RuntimeContext
	frames                 []*EmonFrame
	runActions             *foldMap[*EmonRunAction]
	triggers               []*EmonActionTrigger
	spriteText             *foldMap[string]
	sourceImages           *foldMap[*bishopgfx.BsgImage]
	imageCache             *foldMap[*bishopgfx.BsgImage]
	imageCacheNodes        map[string]*list.Element
	imageCacheOrder        *list.List
	imageCacheBytes        int64
	audioState             *foldMap[string]
	history                []string
	historyVoices          []string
	messageStyle           *foldMap[string]
	random                 *dotnet.Random
	resolver               *machineVariableSource
	argumentResolver       *machineVariableSource
	step                   EmonStep
	backLog                *EmonBackLogWindow
	backLogRevision        int
	waitFrames             int
	messageLocked          bool
	locked                 bool
	selectLocked           bool
	historyCapacity        int
	skip                   bool
	skipMode               int
	auto                   bool
	noSave                 bool
	finished               bool
	resetRequested         bool
	promptIsYesNo          bool
	actionWaiting          bool
	actionWaitFrameDepth   int
	actionWaitFlags        int
	clickWaiting           bool
	polling                bool
	prompt                 string
	hasPrompt              bool
	promptVariable         string
	pendingJump            string
	movie                  string
	hasMovie               bool
	pendingMessage         string
	hasPendingMessage      bool
	pointerX               int
	pointerY               int
	pointerDown            bool
	hoveredSprite          string
	lastTapKey             string
	macroPrograms          *foldMap[*script.EmonProgram]
	loggedNames            *foldSet
	unresolvedJumps        *foldSet
	unresolvedLinks        *foldSet
	storyLogged            bool
	motions                map[string][]ActiveMotion
	colorSprites           *foldSet
	spriteParams           *foldMap[string]
	spriteRects            *foldMap[string]
	spriteResources        *foldMap[int]
	effect                 *stage.EmonEffect
	editSprite             string
	editVariable           string
	areas                  []ClickArea
	scratchSaves           map[int]*EmonSaveData
	scratchThumbs          map[int]*bishopgfx.BsgImage
	backLogFlushed         *foldMap[string]
	mouseFollow            *MouseFollow
	lastHover              string
	cursorMoveLocked       bool
	systemStateReady       bool
	backLogCursor          int
	actionSoundChannel     int
	channelFiles           map[int]string
	channelLoops           map[int]bool
	groupFiles             map[int]string
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
		spriteResources:      newFoldMap[int](),
		scratchSaves:         make(map[int]*EmonSaveData),
		scratchThumbs:        make(map[int]*bishopgfx.BsgImage),
		backLogFlushed:       newFoldMap[string](),
		actionSoundChannel:   -1,
		channelFiles:         make(map[int]string),
		channelLoops:         make(map[int]bool),
		groupFiles:           make(map[int]string),
		channelGroups:        make(map[int]int),
		channelFilesView:     make(map[int]string),
		spriteCreationOrder:  newFoldMap[int](),
	}
	m.resolver = &machineVariableSource{machine: m}
	m.argumentResolver = &machineVariableSource{machine: m, allowSymbolSuffix: true}
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

func (m *Machine) Advance() EmonStep {
	if m.clickWaiting {
		return m.step
	}
	switch m.step {
	case EmonStepAwaitInput:
		if m.MessageLocked() {
			return m.step
		}
		if m.hasPrompt && !m.promptIsYesNo {
			return m.step
		}
		m.clearIncomingMessage()
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
	m.pointerX = x
	m.pointerY = y
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
	if m.backLog != nil {
		return m.backLogPointer(x, y, rightButton)
	}
	m.releaseClickWait()
	locked := m.Locked()
	var sprite *stage.EmonSprite
	if !locked {
		sprite = m.context.Stage.HitTestSelect(x, y)
	}
	input := EmonInputLeftClick
	if rightButton {
		input = EmonInputRightClick
	}
	pendingBefore := m.actionWaiting
	m.fireTriggers(sprite, input, x, y, 0)
	outcome := m.Advance()
	name := ""
	if sprite != nil {
		name = sprite.Name()
	}
	m.logTap(x, y, locked, name, pendingBefore && !m.actionWaiting)
	return outcome
}

func (m *Machine) RightClick() {
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
	if m.Locked() {
		return
	}
	m.firePointerTriggers(sprite)
	m.drainActionWaitFrames()
}

func (m *Machine) PointerPress(x, y int) {
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
	if m.Locked() {
		return
	}
	m.firePointerTriggers(sprite)
	m.drainActionWaitFrames()
}

func (m *Machine) PointerRelease(x, y int) {
	m.pointerDown = false
	m.pointerX = x
	m.pointerY = y
	m.mouseFollow = nil
	m.SetVariable("MouseXNumber", script.EmonValueFromInt(int32(x)))
	m.SetVariable("MouseYNumber", script.EmonValueFromInt(int32(y)))
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
	if m.Locked() {
		return
	}
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
			if local.IsIdentifier() {
				return script.EmonValueFromString(local.AsString())
			}
			return local
		}
	}
	if system := m.context.SystemVariables.Lookup(name); !isMiss(system, name) {
		return system
	}
	if variable := m.context.Variables.Lookup(name); !isMiss(variable, name) {
		return variable
	}
	if symbol := m.context.Symbols.Lookup(name); !isMiss(symbol, name) {
		return symbol
	}
	if allowSymbolSuffix {
		if fused, ok := m.tryFusedSymbol(name); ok {
			return fused
		}
	}
	return script.EmonValueFromIdentifier(name)
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
