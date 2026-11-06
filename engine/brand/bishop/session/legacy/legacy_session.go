package legacy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm/scf"
	"kizuna/engine/core"
)

const (
	legacyFaceFlag        = 2
	legacyRunBudget       = 4096
	noAudioTarget         = ^uint32(0)
	legacyEndingOneName   = "END01.KG"
	legacyEndingTwoName   = "END02.KG"
	// legacyTimeUpScenario 是 !p(4) 超时后载入的 SCN028（時間切れ）。
	legacyTimeUpScenario  = 28
	legacyDataSearchDepth = 6
)

var legacySkippedDataDirs = map[string]bool{"save": true, "cache": true, "audio": true, "movie": true}

var (
	_ core.Engine           = (*Engine)(nil)
	_ core.OpenerExtras     = (*Engine)(nil)
	_ core.SlotPreviewer    = (*Engine)(nil)
	_ core.MapExposer       = (*Engine)(nil)
	_ core.Closer           = (*Engine)(nil)
	_ core.TitleMenuExposer = (*Engine)(nil)
	_ core.KeyReceiver      = (*Engine)(nil)
	_ core.WheelReceiver    = (*Engine)(nil)
)

type Engine struct {
	dir              string
	dataDir          string
	cacheDir         string
	title            string
	resources        *resource.GameResourceIndex
	images           *rendering.ImageCache
	frame            *gfx.BgraCanvas
	legacySaves      *LegacySaveStore
	bgmCatalog       *LegacyBgmCatalog
	gameState        *state.GameState
	table            *LegacyDataTable
	machine          *scf.ScfMachine
	stage            *LegacyStage
	standees         *LegacyStandeeRegistry
	audioEvents      []core.AudioEvent
	bgmTrack         int
	tickCounter      int
	menuVisible      bool
	countdownVisible bool
	countdownSeconds int
	opened           bool
	closed           bool

	hasFrame   bool
	hasMenu    bool
	menuImage  *gfx.BsgImage
	frameImage *gfx.BsgImage
	config     legacyConfig
	configPath string
	titleMode  bool

	commandActive  bool
	commandChoices []scf.ScfChoice
	commandPanel   LegacyPanel
	// commandDragY 是触屏纵向拖动滚命令列表时的上一点；-1 表示本次拖动
	// 还没有起点。宿主的触屏拖动走 pointer 子操作 6，没有 wheel 事件。
	commandDragY int
	// timerSlotShown 是当前已画出的 TIME 图标槽（-1 表示没有图标），
	// 用于每帧把脚本直接写入的 flag0 变化同步到 UI。
	timerSlotShown int
	modal          *LegacyPanel
	modalStack     []LegacyPanel
	scriptDialog   bool

	messageFull  string
	messageDone  bool
	messageStart time.Time

	fadeActive  bool
	fadeStart   time.Time
	waitUntil   time.Time
	endingPhase int
}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) Open(dir string) (core.Info, error) {
	return e.OpenExtras(dir, nil, "")
}

func (e *Engine) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	dataDir := findLegacyDataDir(dir)
	roots := make([]string, 0, len(extraDirs)+1)
	roots = append(roots, extraDirs...)
	resources := resource.Build(dataDir, roots...)
	e.dir = dir
	e.dataDir = dataDir
	e.title = legacyTitle(dir, dataDir)
	e.resources = resources
	e.frame = gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	e.images = rendering.NewImageCache(resources, rendering.DefaultImageBudgetBytes, rendering.DefaultImageCapacity)
	e.gameState = state.NewGameState()
	e.stage = NewLegacyStage()
	e.standees = NewLegacyStandeeRegistry()
	saveDir := filepath.Join(dataDir, "save")
	_ = os.MkdirAll(saveDir, 0o755)
	e.legacySaves = NewLegacySaveStore(saveDir)
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = filepath.Join(saveDir, "cache")
	}
	e.cacheDir = cacheDir
	e.bgmCatalog = NewLegacyBgmCatalog(dataDir, filepath.Join(saveDir, "bgm"))
	e.audioEvents = nil
	e.menuImage = e.images.Get(MenuImageName)
	e.frameImage = e.images.Get(WindowFrameName)
	e.hasMenu = e.menuImage != nil
	e.hasFrame = e.frameImage != nil
	e.configPath = filepath.Join(saveDir, legacyConfigFileName)
	e.config = loadLegacyConfig(e.configPath)
	e.titleMode = false
	e.commandDragY = -1
	e.timerSlotShown = -1
	e.table = e.loadTable()
	if e.table == nil {
		core.Logf("legacy: SCNDAT.TBL missing, session will fault")
	} else {
		e.machine = scf.NewScfMachine(e.table, e.loadScenario)
		e.machine.SetPageBreaks(e.hasFrame)
		e.stage.SetWindowFrame(e.frameImage)
		core.Logf("legacy: table loaded graphics=%d hasFrame=%t hasMenu=%t", len(e.table.GraphicNames()), e.hasFrame, e.hasMenu)
	}
	e.opened = true
	e.closed = false
	core.Logf("legacy: open '%s' dir='%s'", e.title, dataDir)
	return core.Info{Width: ScreenWidth, Height: ScreenHeight, Title: e.title, SceneCount: 1, EntryCount: 1}, nil
}

func legacyTitle(dir, dataDir string) string {
	title := filepath.Base(filepath.Clean(dataDir))
	if strings.EqualFold(title, "Group1") {
		title = filepath.Base(filepath.Clean(dir))
	}
	return title
}

func (e *Engine) Info() core.Info {
	return core.Info{Width: ScreenWidth, Height: ScreenHeight, Title: e.title, SceneCount: 1, EntryCount: 1}
}

func (e *Engine) Scenes() []core.Scene {
	return []core.Scene{{ID: 0, Name: "SCN000", Title: e.title}}
}

func (e *Engine) Slots() []core.Slot {
	infos := e.legacySaves.List()
	out := make([]core.Slot, len(infos))
	for index, info := range infos {
		savedAt := ""
		if info.Exists {
			savedAt = info.SavedAt.Format(time.RFC3339)
		}
		out[index] = core.Slot{Index: uint32(info.Index), SavedAt: savedAt, Summary: info.Summary, Exists: info.Exists}
	}
	return out
}

func (e *Engine) Save(slot uint32, title string) error {
	if e.closed || e.machine == nil {
		return fmt.Errorf("legacy: save slot %d unavailable", slot)
	}
	thumbnail := e.frame.ScaleTo(ThumbnailWidth, ThumbnailHeight)
	machine := e.machine.Capture()
	saved := &LegacySaveState{
		Version:         1,
		ScenarioNumber:  e.machine.ScenarioNumber(),
		Machine:         &machine,
		BackgroundName:  e.stage.BackgroundName(),
		MenuVisible:     e.menuVisible,
		Standees:        append([]LegacyStandeeEntry(nil), e.standees.Entries()...),
		FaceName:        e.stage.FaceName(),
		BgmTrack:        e.bgmTrack,
		Speaker:         e.gameState.Speaker(),
		Text:            e.gameState.Text(),
		Summary:         e.summarize(),
		SavedAtTicks:    TimeToTicks(time.Now().UTC()),
		Thumbnail:       append([]byte(nil), thumbnail.Pixels()...),
		ThumbnailWidth:  thumbnail.Width,
		ThumbnailHeight: thumbnail.Height,
		CommandActive:   e.commandActive,
	}
	if !e.legacySaves.Write(saved, int(slot)) {
		return fmt.Errorf("legacy: save slot %d failed", slot)
	}
	core.Logf("legacy: save slot %d written", slot)
	return nil
}

func (e *Engine) Load(slot uint32) (core.Step, error) {
	if e.closed || e.machine == nil {
		return core.StepFaulted, fmt.Errorf("legacy: load slot %d unavailable", slot)
	}
	saved := e.legacySaves.Read(int(slot))
	if saved == nil || saved.Machine == nil {
		core.Logf("legacy: save slot %d missing", slot)
		return core.StepFaulted, fmt.Errorf("legacy: save slot %d missing", slot)
	}
	e.machine.Restore(*saved.Machine)
	e.bgmTrack = saved.BgmTrack
	e.menuVisible = saved.MenuVisible
	e.tickCounter = 0
	e.standees.Clear()
	for _, entry := range saved.Standees {
		if entry.Name != "" {
			e.standees.Register(entry.Index, entry.Name)
		}
	}
	e.restoreLayers(saved)
	e.gameState.SetSpeaker(saved.Speaker)
	e.gameState.SetText(saved.Text)
	e.gameState.SetMessageWindowVisible(true)
	e.gameState.ClearChoices()
	e.titleMode = false
	e.resetTransient()
	e.restoreBgm()
	e.gameState.SetBgmLoop(true)
	e.gameState.ClearSoundEffect()
	e.setTimerIcon(e.timerSlot())
	awaiting := saved.Text != ""
	e.gameState.SetAwaitingInput(awaiting)
	if awaiting && e.hasFrame {
		e.messageFull = saved.Text
		e.messageDone = true
	}
	// 存档时停在命令菜单：SCF 的 PC 已经在 label 之后，直接继续会执行第一条
	// 命令块（例如“观察”文本）。重新从 entry 0 建菜单，行为与原版读档一致。
	if saved.CommandActive && e.hasFrame {
		if cmd := e.machine.RootOptions(); cmd.Kind == scf.ScfCommandKindOptions {
			e.setCommandOptions(cmd.Options)
			core.Logf("legacy: save slot %d loaded at command menu", slot)
			return core.StepAwaitChoice, nil
		}
	}
	core.Logf("legacy: save slot %d loaded scenario %d", slot, saved.ScenarioNumber)
	if awaiting {
		return core.StepAwaitInput, nil
	}
	return core.StepContinue, nil
}

func (e *Engine) AssetSize(path string) int {
	if e.resources == nil {
		return 0
	}
	location, ok := e.resources.TryGet(path)
	if !ok {
		return 0
	}
	size := location.Size()
	if size < 0 {
		return 0
	}
	return int(size)
}

func (e *Engine) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if e.resources == nil || dst == nil || n <= 0 {
		return 0
	}
	location, ok := e.resources.TryGet(path)
	if !ok {
		return 0
	}
	data := location.Read()
	if len(data) == 0 {
		return 0
	}
	size := len(data)
	if n < size {
		size = n
	}
	copy(unsafe.Slice((*byte)(dst), size), data[:size])
	return size
}

func (e *Engine) Audio() []core.AudioEvent {
	out := e.audioEvents
	e.audioEvents = nil
	if out == nil {
		return nil
	}
	return out
}

func (e *Engine) TextList() []core.TextUnit {
	return nil
}

func (e *Engine) LastError() string {
	return ""
}

func (e *Engine) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	saved := e.legacySaves.Read(int(slot))
	if saved == nil {
		return core.SlotPreview{}, false
	}
	return core.SlotPreview{
		SavedAt: savedAtString(saved.SavedAtTicks),
		Summary: saved.Summary,
		Speaker: saved.Speaker,
		Text:    saved.Text,
		Thumb: core.Frame{
			Width:  saved.ThumbnailWidth,
			Height: saved.ThumbnailHeight,
			Pixels: append([]byte(nil), saved.Thumbnail...),
		},
	}, true
}

func (e *Engine) MapButtons() []core.MapButton {
	return nil
}

func (e *Engine) MapMask() (bool, int, int) {
	return false, 0, 0
}

func (e *Engine) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	if e.resources != nil {
		e.resources.Dispose()
	}
	return nil
}

func (e *Engine) ComposeImages(names []string) *gfx.BsgImage {
	canvas := gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	drawn := false
	for _, name := range names {
		image := e.images.Get(name)
		if image == nil {
			continue
		}
		drawn = true
		canvas.Draw(image, image.OffsetX, image.OffsetY)
	}
	if !drawn {
		return nil
	}
	return canvas.ToImage(0, 0)
}

func (e *Engine) ComposePreview(image *gfx.BsgImage) {
	e.frame.Clear(0, 0, 0, 255)
	e.frame.Draw(image, image.OffsetX, image.OffsetY)
}

func (e *Engine) BgmTrack() int {
	return e.bgmTrack
}

func (e *Engine) TickCounter() int {
	return e.tickCounter
}

func (e *Engine) MenuVisible() bool {
	return e.menuVisible
}

func (e *Engine) CountdownVisible() bool {
	return e.countdownVisible
}

func (e *Engine) CountdownSeconds() int {
	return e.countdownSeconds
}

func (e *Engine) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if e.closed {
		return false, nil
	}
	e.updateAnimation()
	if !e.titleMode {
		e.syncTimerIcon()
	}
	// Seraph 的对话框/工具栏是 frame.KG 的一部分，必须一直画；宿主设置里的
	// showMessageFrame 只适用于有 Flutter 兜底消息框、没有 frame.KG 的作品。
	dirty := e.stage.ComposeTo(e.frame, e.hasFrame || e.gameState.ShowMessageFrame())
	if dirty && dst != nil && n > 0 {
		pixels := e.frame.Pixels()
		size := len(pixels)
		if n < size {
			size = n
		}
		copy(unsafe.Slice((*byte)(dst), size), pixels[:size])
	}
	return dirty, nil
}

func (e *Engine) run(budget uint32) state.SessionStep {
	if e.closed {
		return state.SessionFinished
	}
	if e.machine == nil {
		return state.SessionFaulted
	}
	for executed := uint32(0); executed < budget; executed++ {
		command := e.machine.Fetch()
		if e.machine.Faulted() {
			core.Logf("legacy: fault at %d", e.machine.InstructionStart())
			return state.SessionFaulted
		}
		if step := e.dispatch(command); step != state.SessionContinue {
			return step
		}
	}
	return state.SessionContinue
}

func (e *Engine) resetRuntime() {
	e.stage.ClearAll()
	e.standees.Clear()
	e.bgmTrack = 0
	e.tickCounter = 0
	e.menuVisible = false
	e.countdownVisible = false
	e.countdownSeconds = 0
	e.audioEvents = nil
	e.timerSlotShown = -1
	e.gameState.ResetForRestart()
}

func (e *Engine) toCoreStep(step state.SessionStep) core.Step {
	switch step {
	case state.SessionAwaitInput:
		return core.StepAwaitInput
	case state.SessionAwaitChoice:
		return core.StepAwaitChoice
	case state.SessionAwaitSelect:
		return core.StepAwaitSelect
	case state.SessionWait:
		return core.StepWait
	case state.SessionFinished:
		return core.StepFinished
	case state.SessionFaulted:
		return core.StepFaulted
	case state.SessionAwaitTitle:
		return core.StepAwaitTitle
	default:
		return core.StepContinue
	}
}

func (e *Engine) loadTable() *LegacyDataTable {
	location, ok := e.resources.TryGet(TableName)
	if !ok {
		return nil
	}
	data := location.Read()
	if len(data) == 0 {
		return nil
	}
	table, err := LoadLegacyDataTable(data)
	if err != nil {
		core.Logf("legacy: SCNDAT.TBL load failed: %v", err)
		return nil
	}
	return table
}

func (e *Engine) loadScenario(number int) (*scf.ScfScriptFile, error) {
	name := ScenarioName(number)
	location, ok := e.resources.TryGet(name)
	if !ok {
		core.Logf("legacy: scenario %s not found", name)
		return nil, fmt.Errorf("legacy: scenario %s not found", name)
	}
	data := location.Read()
	if len(data) == 0 {
		core.Logf("legacy: scenario %s load failed: empty", name)
		return nil, fmt.Errorf("legacy: scenario %s empty", name)
	}
	script, err := scf.LoadScfScriptFile(data)
	if err != nil {
		core.Logf("legacy: scenario %s load failed: %v", name, err)
		return nil, err
	}
	return script, nil
}

func (e *Engine) dispatch(command scf.ScfCommand) state.SessionStep {
	switch command.Kind {
	case scf.ScfCommandKindBackground:
		return e.applyBackground(command)
	case scf.ScfCommandKindStandee:
		return e.applyStandee(command)
	case scf.ScfCommandKindMessage:
		return e.applyMessage(command)
	case scf.ScfCommandKindFunction:
		return e.applyFunction(command)
	case scf.ScfCommandKindPlayBgm:
		return e.applyBgm(command)
	case scf.ScfCommandKindPlaySound:
		return e.applySound(command)
	case scf.ScfCommandKindClear:
		return e.applyClear()
	case scf.ScfCommandKindLoadScenario:
		// Seraph 的 !j 0 是“回标题”而不是“重新执行 SCN000”。坏结局
		// (SCN028/029) 和通关结尾都会 !j 0；如果照字面加载 SCN000，
		// flag0 不是 0/1/2 时 SCN000 会原地跳自己，flag0==0 时又会直接
		// 进序章。hasMenu 的作品把 0 当标题入口，其余作品保持旧行为。
		if e.hasMenu && command.Value == 0 && !e.titleMode {
			e.enterTitle()
			return state.SessionAwaitTitle
		}
		return state.SessionContinue
	case scf.ScfCommandKindLoadGameDialog:
		return e.applySaveDialog("load")
	case scf.ScfCommandKindSaveGameDialog:
		return e.applySaveDialog("save")
	case scf.ScfCommandKindOptions:
		return e.applyOptions(command)
	case scf.ScfCommandKindHalt:
		return state.SessionFinished
	case scf.ScfCommandKindFaulted:
		core.Logf("legacy: script fault: %s", command.Text)
		return state.SessionFaulted
	default:
		return state.SessionContinue
	}
}

func (e *Engine) applyBackground(command scf.ScfCommand) state.SessionStep {
	name := strings.TrimSpace(command.Text)
	if name == "" {
		core.Logf("legacy: background with empty name, keeping current")
		return state.SessionContinue
	}
	if !e.menuVisible && strings.EqualFold(e.stage.BackgroundName(), name) {
		return state.SessionContinue
	}
	image := e.images.Get(name)
	if image == nil {
		core.Logf("legacy: background '%s' missing, keeping current", name)
		return state.SessionContinue
	}
	e.stage.SetBackground(name, image)
	e.stage.ClearStandees()
	e.standees.Clear()
	e.menuVisible = false
	e.clearFade()
	core.Logf("legacy: background '%s' %dx%d", name, image.Width, image.Height)
	return state.SessionContinue
}

func (e *Engine) applyStandee(command scf.ScfCommand) state.SessionStep {
	if command.Flag == legacyFaceFlag {
		e.applyFace(command.Text)
		return state.SessionContinue
	}
	if command.Flag == 1 {
		name := strings.TrimSpace(command.Text)
		if !e.standees.Register(command.Index, name) {
			core.Logf("legacy: standee registry full, ignoring index %d", command.Index)
			return state.SessionContinue
		}
		e.rebuildStandees()
		core.Logf("legacy: standee show %d '%s'", command.Index, name)
		return state.SessionContinue
	}
	if command.Flag == 0 {
		if e.standees.Unregister(command.Index) {
			e.rebuildStandees()
		}
		core.Logf("legacy: standee hide %d", command.Index)
		return state.SessionContinue
	}
	core.Logf("legacy: standee flag %d ignored", command.Flag)
	return state.SessionContinue
}

func (e *Engine) applyFace(text string) {
	name := strings.TrimSpace(text)
	if name == "" {
		e.stage.SetFace("", nil)
		return
	}
	image := e.images.Get(name)
	if image == nil {
		core.Logf("legacy: face '%s' missing", name)
		return
	}
	if IsBlankCard(image) {
		core.Logf("legacy: face '%s' blank card, clearing face area", name)
		e.stage.SetFace("", nil)
		return
	}
	e.stage.SetFace(name, image)
}

func (e *Engine) rebuildStandees() {
	e.stage.ClearStandees()
	for slot, entry := range e.standees.Entries() {
		if slot >= LegacyStandeeCapacity {
			break
		}
		e.showStandeeSlot(&entry, slot)
	}
}

func (e *Engine) showStandeeSlot(entry *LegacyStandeeEntry, slot int) {
	if entry == nil || entry.Name == "" {
		return
	}
	image := e.images.Get(entry.Name)
	if image == nil {
		core.Logf("legacy: standee '%s' missing", entry.Name)
		return
	}
	e.stage.SetStandee(slot, image)
}

func (e *Engine) applyMessage(command scf.ScfCommand) state.SessionStep {
	text := CleanText(command.Text)
	if e.hasFrame && text == "" {
		return state.SessionContinue
	}
	e.gameState.SetSpeaker("")
	e.gameState.SetText(text)
	e.gameState.SetMessageWindowVisible(true)
	e.gameState.SetAwaitingInput(true)
	if e.hasFrame {
		e.endCommand()
		e.closeAllModals()
		e.messageFull = text
		e.messageDone = false
		e.messageStart = time.Now()
		e.stage.SetMessage(text, 0)
	}
	return state.SessionAwaitInput
}

func (e *Engine) applyOptions(command scf.ScfCommand) state.SessionStep {
	if len(command.Options) == 0 {
		e.gameState.ClearChoices()
		core.Logf("legacy: options empty, continuing")
		return state.SessionContinue
	}
	texts := make([]string, 0, len(command.Options))
	for _, option := range command.Options {
		texts = append(texts, CleanText(option.Text))
	}
	e.gameState.SetChoices(texts)
	if !e.hasFrame {
		return state.SessionAwaitChoice
	}
	e.gameState.SetMessageWindowVisible(true)
	e.setCommandOptions(command.Options)
	return state.SessionAwaitChoice
}

func (e *Engine) applySaveDialog(kind string) state.SessionStep {
	if !e.hasFrame {
		e.gameState.SetAwaitingChoice(false)
		core.Logf("legacy: %s dialog requested without frame, ignored", kind)
		return state.SessionAwaitInput
	}
	e.scriptDialog = true
	if kind == "load" {
		e.openModal(e.slotPanel(LegacyPanelLoad))
	} else {
		e.openModal(e.slotPanel(LegacyPanelSave))
	}
	return state.SessionAwaitChoice
}

func (e *Engine) applyFunction(command scf.ScfCommand) state.SessionStep {
	switch command.Value {
	case legacySlotWait:
		return e.applyWait()
	case legacySlotFadeOut:
		return e.applyFadeOut()
	case legacySlotTimeSet:
		return e.applyTimeSet()
	case legacySlotTimeGet:
		return e.applyTimeGet()
	case legacySlotEnding:
		return e.applyEndingCheck()
	case legacySlotEndingRoll:
		return e.applyEndingRoll()
	}
	core.Logf("legacy: function slot %d ignored text='%s'", command.Value, command.Text)
	return state.SessionContinue
}

func (e *Engine) applyWait() state.SessionStep {
	e.waitUntil = time.Now().Add(legacyWaitDuration)
	return state.SessionWait
}

func (e *Engine) applyFadeOut() state.SessionStep {
	e.fadeActive = true
	e.fadeStart = time.Now()
	e.waitUntil = e.fadeStart.Add(legacyFadeDuration)
	return state.SessionWait
}

func (e *Engine) applyTimeSet() state.SessionStep {
	if e.machine == nil {
		return state.SessionContinue
	}
	flags := e.machine.Flags()
	if len(flags) < 2 {
		return state.SessionContinue
	}
	flags[0] = 0
	flags[1] = 0
	e.setTimerIcon(0)
	return state.SessionContinue
}

// applyTimeGet 实现 SCF 的 !p(3)：只把「当日经过的小时数」加一，绝不在这里
// 跨天。原版把跨天逻辑放在脚本里：调用 !p(3) 后紧接着判断 #i(flag0>7)，
// 命中时 #g 12 执行 SCN012（`[flag0]=0`、`[flag1]+=1`）。SCN060 等脚本还会
// 用这个 >7 分支跳去别的场景，所以引擎若自行回绕，会吞掉脚本分支。
func (e *Engine) applyTimeGet() state.SessionStep {
	if e.machine == nil {
		return state.SessionContinue
	}
	flags := e.machine.Flags()
	if len(flags) < 2 {
		return state.SessionContinue
	}
	flags[0]++
	return state.SessionContinue
}

// applyEndingCheck 实现 SCF 的 !p(4)：进入地图/场景时检查天数。SCN013 的
// 时间 HUD 在 flag1==0..6 分别显示一日目..七日目，flag1==7 直接 !j 28，
// 所以 flag1>=7 就是時間切れ。不能只依赖 SCN013，因为 SCN107/108 等脚本
// 消耗时间后不会调用 HUD（#g 13），必须由 !p(4) 兜底。
// flags[0] 是当天所过小时数，flags[1] 是天数。
func (e *Engine) applyEndingCheck() state.SessionStep {
	if e.machine == nil {
		return state.SessionContinue
	}
	flags := e.machine.Flags()
	if len(flags) < 2 || flags[1] < 7 {
		return state.SessionContinue
	}
	core.Logf("legacy: time limit reached (day=%d), loading SCN028", flags[1])
	// Start 会重置调用栈/条件栈但不碰 flags，正好符合原引擎「保留时间状态再跳坏结局」的行为。
	e.machine.Start(legacyTimeUpScenario)
	return state.SessionContinue
}

func (e *Engine) applyEndingRoll() state.SessionStep {
	e.config.Clear = true
	e.persistConfig()
	e.endingPhase = 1
	e.stage.SetEnding(e.images.Get(legacyEndingOneName))
	e.waitUntil = time.Now().Add(legacyEndingWait)
	return state.SessionWait
}

// setTimerIcon 的 slot 为 -1 时清空时钟图标：flag0==8 表示这一天的小时已经
// 用完、脚本还没执行 SCN012 回绕，此时原版没有 TIME 图可画，不能错误地
// 退回 TIME1（０時間）造成“时间倒流”的观感。
func (e *Engine) setTimerIcon(slot int) {
	if slot < 0 || slot > 7 {
		e.stage.SetTimerIcon(nil)
		return
	}
	e.stage.SetTimerIcon(e.images.Get(TimerIconName(slot + 1)))
}

func (e *Engine) timerSlot() int {
	if e.machine == nil {
		return -1
	}
	flags := e.machine.Flags()
	if len(flags) == 0 || flags[0] > 7 {
		return -1
	}
	return int(flags[0])
}

// syncTimerIcon 每帧把 flag0 同步到时钟图标。SCN012 用 [flag] 直接写状态，
// 不经过任何函数槽，若只在 !p(3)/!p(2) 时刷新，跨天后图标会停在旧位置。
func (e *Engine) syncTimerIcon() {
	if e.machine == nil {
		return
	}
	slot := e.timerSlot()
	if slot == e.timerSlotShown {
		return
	}
	e.timerSlotShown = slot
	e.setTimerIcon(slot)
}

func (e *Engine) applyBgm(command scf.ScfCommand) state.SessionStep {
	if command.Value <= 0 {
		e.bgmTrack = 0
		e.gameState.ClearBgm()
		e.enqueueAudio(stopBgmEvent())
		core.Logf("legacy: bgm stop")
		return state.SessionContinue
	}
	e.bgmTrack = command.Value
	path := e.bgmCatalog.Resolve(command.Value)
	if path == "" {
		e.gameState.ClearBgm()
		e.enqueueAudio(stopBgmEvent())
		core.Logf("legacy: bgm track %d no audio file", command.Value)
	} else {
		e.gameState.SetBgm(path)
		if e.config.Bgm {
			e.enqueueAudio(playBgmEvent(path, e.gameState.BgmVolume()))
		}
		core.Logf("legacy: bgm track %d '%s'", command.Value, path)
	}
	e.gameState.SetBgmLoop(true)
	return state.SessionContinue
}

func (e *Engine) restoreBgm() {
	path := ""
	if e.bgmTrack > 0 {
		path = e.bgmCatalog.Resolve(e.bgmTrack)
	}
	if path == "" {
		e.gameState.ClearBgm()
		e.enqueueAudio(stopBgmEvent())
		return
	}
	e.gameState.SetBgm(path)
	if e.config.Bgm {
		e.enqueueAudio(playBgmEvent(path, e.gameState.BgmVolume()))
	}
}

func (e *Engine) applySound(command scf.ScfCommand) state.SessionStep {
	file := WithWaveExtension(command.Text)
	if file == "" {
		return state.SessionContinue
	}
	if !e.resources.Exists(file) {
		core.Logf("legacy: se '%s' missing, skipped", file)
		return state.SessionContinue
	}
	e.gameState.SetSoundEffect(file)
	e.gameState.SetEffectSerial(e.gameState.EffectSerial() + 1)
	path := audio.Materialize(file, e.resources, e.cacheDir)
	if path == "" {
		path = file
	}
	e.enqueueAudio(core.AudioEvent{Kind: 1, Path: path, Volume: 1, Channel: noAudioTarget, Group: noAudioTarget})
	core.Logf("legacy: se '%s'", file)
	return state.SessionContinue
}

func (e *Engine) applyClear() state.SessionStep {
	e.stage.ClearLayers0To3()
	e.stage.SetFace("", nil)
	e.stage.SetTimerIcon(nil)
	e.standees.Clear()
	e.menuVisible = false
	e.commandActive = false
	e.commandChoices = nil
	e.commandPanel = LegacyPanel{}
	e.modal = nil
	e.modalStack = nil
	e.scriptDialog = false
	e.stage.SetPanel(nil)
	e.stage.SetEnding(nil)
	e.clearMessage()
	e.gameState.SetText("")
	e.gameState.SetSpeaker("")
	e.gameState.SetAwaitingInput(false)
	e.gameState.ClearChoices()
	return state.SessionContinue
}

func (e *Engine) restoreLayers(saved *LegacySaveState) {
	e.stage.ClearAll()
	if saved.BackgroundName != "" {
		background := e.images.Get(saved.BackgroundName)
		if background == nil {
			core.Logf("legacy: save background '%s' missing", saved.BackgroundName)
		} else {
			e.stage.SetBackground(saved.BackgroundName, background)
		}
	}
	e.rebuildStandees()
	e.applyFace(saved.FaceName)
}

func (e *Engine) summarize() string {
	text := strings.TrimSpace(strings.ReplaceAll(e.gameState.Text(), "\n", " "))
	if e.gameState.Speaker() != "" {
		text = e.gameState.Speaker() + ": " + text
	}
	return truncateUtf16Units(text, 42)
}

func (e *Engine) enqueueAudio(event core.AudioEvent) {
	e.audioEvents = append(e.audioEvents, event)
}

func stopBgmEvent() core.AudioEvent {
	return core.AudioEvent{Kind: 3, Channel: noAudioTarget, Group: noAudioTarget}
}

func playBgmEvent(path string, volume float64) core.AudioEvent {
	return core.AudioEvent{Kind: 0, Path: path, Volume: volume, Loop: true, Channel: noAudioTarget, Group: noAudioTarget}
}

func truncateUtf16Units(text string, limit int) string {
	if utf16Length(text) <= limit {
		return text
	}
	length := 0
	end := 0
	for _, r := range text {
		width := 1
		if r > 0xFFFF {
			width = 2
		}
		if length+width > limit {
			break
		}
		length += width
		end += len(string(r))
	}
	return text[:end]
}

func utf16Length(text string) int {
	length := 0
	for _, r := range text {
		length++
		if r > 0xFFFF {
			length++
		}
	}
	return length
}

func savedAtString(ticks int64) string {
	return TicksToTime(ticks).Format(time.RFC3339)
}

func findLegacyDataDir(dir string) string {
	if hasLegacyAnchor(dir) {
		return dir
	}
	type candidate struct {
		path  string
		depth int
	}
	queue := []candidate{{path: dir, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= legacyDataSearchDepth {
			continue
		}
		entries, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || legacySkippedDataDirs[strings.ToLower(entry.Name())] {
				continue
			}
			child := filepath.Join(current.path, entry.Name())
			if hasLegacyAnchor(child) {
				return child
			}
			queue = append(queue, candidate{path: child, depth: current.depth + 1})
		}
	}
	return dir
}

func hasLegacyAnchor(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), ScnFileName) {
			return true
		}
	}
	return false
}
