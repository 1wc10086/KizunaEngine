package gothic

import (
	"errors"
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
	vmgothic "kizuna/engine/brand/bishop/vm/gothic"
	"kizuna/engine/core"
)

const (
	gothicDefaultPlayerName    = "悠"
	gothicVoiceEffectThreshold = 1629
	gothicDefaultRunBudget     = 4096
)

var gothicTitleMenuLabels = []string{"はじめから", "つづきから", "おもいで", "せってい", "おわり"}

type Session struct {
	dir      string
	dataDir  string
	cacheDir string
	title    string
	profile  gothicProfile
	// ui 非 nil 时脚本停住，由引擎自绘的标题/设置/鉴赏/存读档界面接管点击。
	// 它和 mapState 是两套模态：地图是脚本驱动的选择，ui 是引擎系统菜单。
	ui *gothicUI
	// global 是 MAZE 的跨周目数据（CG/H 解锁、玩家名、汁设定等）。
	// globalPath 放在 save/gothic 下，不写原版 prof.dat/usr0.dat。
	global      *gothicGlobalData
	globalPath  string
	globalDirty bool
	// cgNames 是鉴赏室目录顺序；cgSet 用来快速判断一张图是否属于目录。
	cgNames []string
	cgSet   map[string]bool
	// hImages 缓存 H1SCENE..H21SCENE 的缩略图名。
	hImages map[int]string
	// saveSlotCount 是当前作品的存档槽数：MAZE 原版 15 槽，Toku1 24 槽。
	saveSlotCount int
	// hostRequest 交给宿主的界面动作（exit/settings 等），Serial 防重复。
	hostRequest       string
	hostRequestSerial uint32
	// nameForOption 表示这次 AwaitName 是设置画面的“名前変更”。
	nameForOption  bool
	resources      *resource.GameResourceIndex
	images         *rendering.ImageCache
	gameState      *state.GameState
	catalog        *GothicResourceCatalog
	font           *GothicFontFile
	stage          *GothicStage
	audioCatalog   *GothicAudioCatalog
	saves          *GothicSaveStore
	saveDir        string
	program        *vmgothic.GothicProgram
	machine        *vmgothic.GothicMachine
	mapState       *GothicMapState
	hoverBase1     *gfx.BsgImage
	hoverBase2     *gfx.BsgImage
	hoverMask      *gfx.BsgImage
	hoverPixels    []byte
	hoverComposite *gfx.BsgImage
	hoverPage      int
	hoverCol       int
	hoverRow       int
	pendingWave    []int32
	controls       map[string][4]int32
	hasTitleMenu   bool
	pageLines      []string
	// pageBreakPending 表示当前页已经停在 `%`/`%%` 上，下一次 Advance
	// 先清页再继续脚本。它与 gameState.AwaitingInput 分开：等待状态
	// 属于脚本，清页属于渲染。
	pageBreakPending bool
	// choiceActive/choiceItems 是引擎自绘的选择肢（MAZE 原版选项直接
	// 写在画面上，点击文字选择）。宿主只拿透明热区。
	choiceActive bool
	choiceItems  []string
	// 文字送り：原版的 ページ単位/高速/標準/低速。revealLimit 是当前
	// 已显示的字数，-1 表示整页直接显示。
	revealStart    time.Time
	revealActive   bool
	revealTotal    int
	revealLimit    int
	playerName     string
	nameResolved   bool
	nameWaitIssued bool
	awaitingName   bool
	startMenuOpen  bool
	titleMode      bool
	// dateVisible/dateWeek… 是 Toku1 地图画面左上角的日期时间牌。
	// 原版由 set date1 命令写入，只在移动画面显示。
	dateVisible bool
	dateWeek    int
	dateDay     int
	dateHour    int
	dateMinute  int
	// skipStartMenu/suppressLogo 用于“结局后回标题再开始”：VM 要重新跑
	// boot 初始化变量，但不能再闪一次 logo/停一次标题。
	skipStartMenu bool
	suppressLogo  bool
	// replayMode 表示当前是从回想界面 Call 进 HnSCENE 的回放。
	replayMode bool
	// 回想回放前的 VM 快照；#return 后恢复，避免 H 场景里的变量/跳转污染主线。
	replayDepth    int
	replaySnapshot vmgothic.GothicMachineState
	replayVars     []vmgothic.GothicVariableEntry
	replayPage     int
	frameMain      bool
	frameUpper     bool
	frameLower     bool
	aniOn          bool
	pendingMusic   string
	// bgmName 是原版曲名，存档里存名字而不是切片后的绝对路径。
	bgmName         string
	hasPendingMusic bool
	background      string
	backgroundLayer int
	effectStamp     time.Time
	disposed        bool
	opened          bool
	lastError       string
	map2Waiting     bool
	// mapColorIndex 表示当前 g map 的像素值要按“色块 -> 1..4”换算。
	mapColorIndex      bool
	lastBgm            string
	lastBgmHas         bool
	lastVoice          string
	lastVoiceHas       bool
	lastVoiceSerial    int
	lastEffect         string
	lastEffectHas      bool
	lastEffectSerial   int
	lastVoiceSeqSerial int
}

func NewSession() *Session {
	s := &Session{
		hoverPage: -2147483648,
		hoverCol:  -2147483648,
		hoverRow:  -2147483648,
	}
	s.gameState = state.NewGameState()
	s.mapState = NewGothicMapState()
	s.controls = map[string][4]int32{}
	s.playerName = gothicDefaultPlayerName
	s.frameMain = true
	s.frameUpper = true
	s.frameLower = true
	s.backgroundLayer = 4
	s.effectStamp = time.Now().UTC()
	s.font = NewGothicFontFile("")
	s.stage = NewGothicStage(nil, s.font)
	s.saves = NewGothicSaveStore("")
	s.lastVoiceSerial = s.gameState.VoiceSerial()
	s.lastEffectSerial = s.gameState.EffectSerial()
	s.lastVoiceSeqSerial = s.gameState.VoiceSequenceSerial()
	return s
}

func (s *Session) Open(dir string) (core.Info, error) {
	return s.OpenExtras(dir, nil, "")
}

func (s *Session) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	if s.opened {
		s.Close()
	}
	s.disposed = false
	dataDir := DetectGothicDataDir(dir)
	if dataDir == "" {
		dataDir = dir
	}
	s.dir = dir
	s.dataDir = dataDir
	s.title = filepath.Base(dataDir)
	resources := resource.Build(dataDir, extraDirs...)
	s.resources = resources
	s.images = rendering.NewImageCache(resources, 256<<20, 512)
	s.gameState = state.NewGameState()
	s.catalog = NewGothicResourceCatalog(dataDir, resources, s.images)
	fontPath := findGothicFile(dataDir, "GOTHIC2.DAT")
	s.font = NewGothicFontFile(fontPath)
	s.profile = detectGothicProfile(dataDir, s.font)
	// 两套原版 EXE 都只有 15 个存档位（TOKU?.USR / Save1..Save15）。
	// 早期把 Toku1 写成 24 槽是猜测值，会让存档面板溢出屏幕。
	s.saveSlotCount = 15
	s.stage = NewGothicStage(s.catalog, s.font)
	if s.profile == gothicProfileToku1 {
		// p_kaiwa 是 Toku1 的 550x110 对话框底图；放在 (45,330) 时
		// 正文原点 (96,340) 正好留出 51px 左边距、10px 上边距。
		if window := s.catalog.Resolve("p_kaiwa"); window != nil {
			s.stage.SetMessageWindow(window, 45, 330)
		} else {
			core.Logf("gothic: p_kaiwa missing, Toku1 message window disabled")
		}
	}
	s.saveDir = filepath.Join(dataDir, "save")
	if cacheDir != "" {
		// 光盘镜像解包目录在 cache 下，系统清缓存会把存档一起清掉。
		// 宿主传的 cacheDir 是 <appSupport>/kizuna/cache/kizuna，
		// 往上两层就是持久的 <appSupport>/kizuna；按原始游戏目录哈希
		// 再分一层，重新解包镜像后仍能找回同一部作品的存档。
		persistentRoot := filepath.Join(filepath.Dir(filepath.Dir(cacheDir)), "saves")
		if dir := core.GameCacheDir(persistentRoot, s.dir); dir != "" {
			s.saveDir = dir
		}
	}
	audioCache := filepath.Join(s.saveDir, "bgm")
	if cacheDir != "" {
		audioCache = cacheDir
	}
	s.cacheDir = audioCache
	s.audioCatalog = NewGothicAudioCatalog(dataDir, resources, audioCache)
	s.saves = NewGothicSaveStore(filepath.Join(s.saveDir, "gothic"))
	s.globalPath = filepath.Join(s.saveDir, "gothic", "global.json")
	s.global = newGothicGlobalData()
	if s.profile == gothicProfileMaze {
		s.global = loadGothicGlobal(s.globalPath)
	}
	if s.profile == gothicProfileMaze || s.profile == gothicProfileToku1 {
		// Toku1 的鉴赏目录同样来自 PROF.DAT；没有它鉴赏室会是空的。
		s.loadGothicProfileCatalog()
	}
	s.cgNames = append([]string{}, s.cgNames...)
	s.ui = nil
	s.replayMode = false
	s.hostRequest = ""
	s.hostRequestSerial = 0
	s.mapState = NewGothicMapState()
	s.controls = map[string][4]int32{}
	s.playerName = gothicDefaultPlayerName
	s.frameMain = true
	s.frameUpper = true
	s.frameLower = true
	s.backgroundLayer = 4
	s.effectStamp = time.Now().UTC()
	s.hoverPage = -2147483648
	s.hoverCol = -2147483648
	s.hoverRow = -2147483648
	s.hasTitleMenu = s.profile == gothicProfileMaze ||
		findGothicFile(dataDir, "PACK.PK") != "" ||
		findGothicFile(dataDir, "SP.PLA") != ""
	s.lastBgmHas = false
	s.lastVoiceHas = false
	s.lastVoiceSerial = s.gameState.VoiceSerial()
	s.lastEffectHas = false
	s.lastEffectSerial = s.gameState.EffectSerial()
	s.lastVoiceSeqSerial = s.gameState.VoiceSequenceSerial()
	scriptPath := findGothicFile(dataDir, "MAINCON.SCS")
	program, err := vmgothic.LoadGothicProgram(scriptPath)
	if err != nil {
		core.Logf("gothic: script load failed '%s': %v", scriptPath, err)
		s.lastError = err.Error()
	} else {
		s.program = program
		s.machine = vmgothic.NewGothicMachine(program)
		core.Logf("gothic: script '%s' statements=%d menus=%d", scriptPath, program.Count(), len(program.Menus()))
	}
	s.opened = true
	core.Logf("gothic: open dir='%s'", dataDir)
	return core.Info{Width: GothicScreenWidth, Height: GothicScreenHeight, Title: s.title}, nil
}

func (s *Session) Info() core.Info {
	return core.Info{Width: GothicScreenWidth, Height: GothicScreenHeight, Title: s.title}
}

func (s *Session) Start(entry uint32) (core.Step, error) {
	_ = entry
	return s.run(gothicDefaultRunBudget)
}

func (s *Session) Run(budget uint32) (core.Step, error) {
	return s.run(budget)
}

func (s *Session) run(budget uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.ui != nil {
		return s.uiStep(), nil
	}
	if s.pageBreakPending && s.gameState.AwaitingInput() {
		// Run 不能越过段末停顿：自动/跳过逻辑可能在 AwaitInput 时仍发 Run，
		// 如果这里继续 Fetch，整页文本会被直接读掉。
		return core.StepAwaitInput, nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	if budget == 0 {
		budget = gothicDefaultRunBudget
	}
	for executed := uint32(0); executed < budget; executed++ {
		action := s.machine.Fetch()
		step := s.dispatchAction(action)
		s.syncGlobalFlags()
		if s.replayMode && s.machine.CallDepth() <= s.replayDepth {
			// 回想标签的 #return 已经执行，PC 回到调用点。此时必须
			// 恢复进回想前的状态并退回网格，不能顺着主线继续跑。
			s.finishReplay()
			return s.uiStep(), nil
		}
		if step != core.StepContinue {
			return s.withNameStep(step), nil
		}
	}
	return s.withNameStep(core.StepContinue), nil
}

func (s *Session) withNameStep(step core.Step) core.Step {
	if step == core.StepAwaitInput && s.awaitingName {
		return core.StepAwaitName
	}
	return step
}

func (s *Session) Advance() (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.ui != nil {
		if s.ui.mode == uiStaffRoll {
			return s.staffNext(), nil
		}
		return s.uiStep(), nil
	}
	if s.nameWaitIssued {
		s.nameWaitIssued = false
		s.awaitingName = false
	}
	s.gameState.SetAwaitingInput(false)
	if s.revealActive {
		// 第一次点击先把整页文字显示完；第二次才清页推进。
		// 这里必须重新置 AwaitingInput，否则宿主下一帧会认为已经可以
		// 继续，直接跑掉下一页。
		s.finishReveal()
		s.gameState.SetAwaitingInput(true)
		return core.StepAwaitInput, nil
	}
	if s.pageBreakPending {
		s.clearPage()
	}
	return s.run(gothicDefaultRunBudget)
}

func (s *Session) SubmitName(name string) bool {
	text := strings.TrimSpace(name)
	if len(text) == 0 {
		return false
	}
	runes := []rune(text)
	if len(runes) > 24 {
		text = string(runes[:24])
	}
	if s.nameForOption {
		// 原版设置页把姓/名分成两个输入框，宿主只有一个文本对话框。
		// 用空格分隔时前半为姓、后半为名；没有空格就整体当姓。
		parts := strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == '　' })
		if s.global != nil {
			if len(parts) >= 2 {
				s.global.PlayerLast = parts[0]
				s.global.PlayerFirst = parts[1]
			} else {
				s.global.PlayerLast = text
				s.global.PlayerFirst = ""
			}
			s.globalDirty = true
			s.flushGlobal()
			s.playerName = strings.TrimSpace(s.global.PlayerLast + s.global.PlayerFirst)
		}
		s.nameForOption = false
		s.awaitingName = false
		s.nameWaitIssued = false
		core.Logf("gothic: option name updated")
		return true
	}
	s.playerName = text
	s.nameResolved = true
	s.awaitingName = false
	core.Logf("gothic: inputname submitted len=%d", len([]rune(s.playerName)))
	return true
}

func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.ui != nil {
		return s.uiStep(), nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	if !s.gameState.AwaitingChoice() {
		return core.StepContinue, nil
	}
	if s.startMenuOpen {
		return s.chooseStartMenu(int(index)), nil
	}
	if !s.machine.Choose(int32(index) + 1) {
		core.Logf("gothic: choice %d rejected", index)
		return core.StepAwaitChoice, nil
	}
	s.gameState.ClearChoices()
	s.stage.ClearMenuItems()
	s.choiceActive = false
	s.choiceItems = nil
	s.gameState.SetAwaitingInput(false)
	return s.run(gothicDefaultRunBudget)
}

func (s *Session) Pointer(op uint32, x, y int32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.ui != nil {
		switch op {
		case 0: // move
			return s.uiHover(int(x), int(y)), nil
		case 1, 3: // press / tap（tap 是引擎内原子 press+release）
			return s.uiClick(int(x), int(y)), nil
		case 4:
			return s.Advance()
		case 5:
			return s.uiBack(), nil
		}
		// release(2) 只抬起手指，不能重复触发按钮。
		return s.uiStep(), nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	if (s.profile == gothicProfileMaze || s.profile == gothicProfileToku1) && s.ui == nil {
		// 右上角的小菜单按钮：Android 上没有键盘 ESC/右键，宿主工具条
		// 在横屏又会自动收起，必须留一个常驻入口。
		if op == 1 || op == 3 {
			if x >= 588 && x < 636 && y >= 4 && y < 38 {
				s.openIngame()
				return s.uiStep(), nil
			}
		}
	}
	if s.profile == gothicProfileToku1 && s.ui == nil && !s.titleMode && (op == 1 || op == 3) {
		if x >= 536 && x < 624 && y >= 444 && y < 472 {
			s.openIngame()
			return s.uiStep(), nil
		}
	}
	if s.choiceActive {
		switch op {
		case 0:
			if index := s.choiceIndexAt(int(x), int(y)); index >= 0 {
				s.gameState.SetHoveredButton(index)
				s.stage.Invalidate()
			}
			return core.StepAwaitSelect, nil
		case 1, 3:
			if index := s.choiceIndexAt(int(x), int(y)); index >= 0 {
				return s.Choose(uint32(index))
			}
			return core.StepAwaitSelect, nil
		}
	}
	switch op {
	case 0:
		s.setMapHover(int(x), int(y))
		return s.currentStep(), nil
	case 5:
		return s.cancelMapSelect(), nil
	case 1, 3:
		if s.mapState.RouteActive() || s.mapState.ClickMapActive() || s.map2Waiting {
			return s.chooseMapPixel(int(x), int(y)), nil
		}
		if s.gameState.AwaitingInput() {
			return s.Advance()
		}
		return s.currentStep(), nil
	case 4: // right click：原版右键菜单
		if (s.profile == gothicProfileMaze || s.profile == gothicProfileToku1) && s.machine != nil {
			s.openIngame()
			return s.uiStep(), nil
		}
		if s.gameState.AwaitingInput() {
			return s.Advance()
		}
		return s.currentStep(), nil
	default:
		return s.currentStep(), nil
	}
}

func (s *Session) currentStep() core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.awaitingName {
		return core.StepAwaitName
	}
	if s.ui != nil {
		return s.uiStep()
	}
	if s.choiceActive {
		return core.StepAwaitSelect
	}
	if s.machine == nil {
		return core.StepFaulted
	}
	if s.awaitingName {
		return core.StepAwaitName
	}
	if s.mapState.RouteActive() || s.mapState.ClickMapActive() || s.gameState.Selecting() {
		return core.StepAwaitSelect
	}
	if s.gameState.AwaitingChoice() {
		return core.StepAwaitChoice
	}
	if s.gameState.AwaitingInput() {
		return core.StepAwaitInput
	}
	return core.StepContinue
}

func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	dirty := s.renderFrame()
	if !dirty {
		return false, nil
	}
	need := GothicScreenWidth * GothicScreenHeight * 4
	if n < need {
		return false, errors.New("gothic: compose buffer too small")
	}
	buf := unsafe.Slice((*byte)(dst), need)
	copy(buf, s.stage.Frame.Buffer)
	return true, nil
}

func (s *Session) renderFrame() bool {
	if s.disposed {
		return false
	}
	if s.stage.EffectActive {
		now := time.Now().UTC()
		elapsed := now.Sub(s.effectStamp).Seconds()
		s.effectStamp = now
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed > 1 {
			elapsed = 1
		}
		s.stage.AdvanceEffect(elapsed)
	}
	s.updateReveal()
	dirty := s.stage.Compose()
	if s.ui == nil && s.profile == gothicProfileToku1 && s.dateVisible && s.mapState.Active != GothicMapNone {
		s.drawDateCard(s.stage.Frame)
		dirty = true
	}
	// 系统界面是覆盖层：它不属于舞台图层，Compose 的脏标记管不到。
	// 只要 ui 存在就重画一遍覆盖层，悬停/翻页才能立刻反映到画面上。
	if s.ui != nil {
		s.drawGothicUI()
		dirty = true
	}
	if s.choiceActive && s.ui == nil {
		s.drawChoiceHighlight()
		dirty = true
	}
	if s.ui == nil && (s.profile == gothicProfileMaze || s.profile == gothicProfileToku1) && s.machine != nil {
		s.drawMenuButton()
		dirty = true
	}
	if s.ui == nil && s.profile == gothicProfileToku1 && !s.titleMode && s.gameState.MessageWindowVisible() {
		// 原版在信息框下方常驻一个 CONFIG 按钮；Android 上这块区域
		// 不会被宿主工具栏覆盖，保留它比只靠右上角菜单更接近原作。
		s.stage.Frame.FillBlend(536, 444, 88, 28, 30, 24, 18, 220)
		s.drawUIBorder(s.stage.Frame, 536, 444, 88, 28, false)
		s.drawUIText(s.stage.Frame, "設定", 561, 448, 2)
	}
	return dirty
}

func (s *Session) Scenes() []core.Scene {
	return nil
}

func (s *Session) Slots() []core.Slot {
	result := make([]core.Slot, s.saveSlotCount)
	for slot := 0; slot < s.saveSlotCount; slot++ {
		info := s.saves.Inspect(slot)
		savedAt := ""
		if info.HasSavedAt {
			savedAt = info.SavedAt.Format(time.RFC3339)
		}
		result[slot] = core.Slot{Index: uint32(slot), SavedAt: savedAt, Summary: info.Summary, Exists: info.Exists}
	}
	return result
}

func (s *Session) Save(slot uint32, title string) error {
	_ = title
	if s.disposed || s.machine == nil {
		return errors.New("gothic: session not ready")
	}
	if slot >= uint32(s.saveSlotCount) {
		return errors.New("gothic: slot out of range")
	}
	if s.choiceActive {
		// 选择肢的 VM 状态含 menu 指针，当前存档格式不保存它；
		// 在选择肢上存档会在读档时从 #item 中间继续，直接禁止。
		return errors.New("gothic: cannot save on choice")
	}
	thumbnail := s.captureThumbnail()
	screen := s.captureScreenWithoutText()
	snapshot := s.machine.Snapshot()
	speaker := s.gameState.Speaker()
	text := s.gameState.Text()
	summary := s.summarize()
	saveState := &GothicSaveState{
		StatementIndex:  s.machine.ProgramCounter(),
		Variables:       s.machine.Variables().Snapshot(),
		CallStack:       append([]int32{}, snapshot.Calls...),
		Branches:        append([]bool{}, snapshot.Branches...),
		PageLines:       append([]string{}, s.pageLines...),
		BackgroundLayer: int32(s.backgroundLayer),
		MessageVisible:  s.messageVisible(),
		PageBreak:       s.pageBreakPending,
		Screen:          screen,
		Background:      s.background,
		Speaker:         speaker,
		Text:            text,
		Summary:         summary,
		Bgm:             s.bgmName,
		PlayerName:      s.playerName,
		Thumbnail:       append([]byte{}, thumbnail.Buffer...),
		ThumbnailWidth:  int32(thumbnail.Width),
		ThumbnailHeight: int32(thumbnail.Height),
	}
	if !s.saves.Save(int(slot), saveState) {
		return errors.New("gothic: save failed")
	}
	core.Logf("gothic: save slot %d written", slot)
	return nil
}

// captureThumbnail 在存读档界面打开时先合成一帧“游戏本身”的画面，
// 否则缩略图会把 15 格存档 UI 存进去。界面状态暂时摘下，合成后恢复。
func (s *Session) captureThumbnail() *gfx.BgraCanvas {
	savedUI := s.ui
	s.ui = nil
	s.stage.Invalidate()
	s.stage.Compose()
	thumbnail := s.stage.Frame.ScaleTo(GothicThumbnailWidth, GothicThumbnailHeight)
	s.ui = savedUI
	s.stage.Invalidate()
	return thumbnail
}

// captureScreenWithoutText 存一张不含对白的整屏快照。读档时正文会由
// PageLines 重新合成；如果快照里已经带了字，读档画面会出现双层文字。
func (s *Session) captureScreenWithoutText() []byte {
	savedVisible := s.stage.messageVisible
	s.stage.SetMessageVisible(false)
	s.stage.Invalidate()
	s.stage.Compose()
	out := append([]byte{}, s.stage.Frame.Buffer...)
	s.stage.SetMessageVisible(savedVisible)
	s.stage.Invalidate()
	return out
}

func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.disposed || s.machine == nil {
		return core.StepFaulted, errors.New("gothic: session not ready")
	}
	if slot >= uint32(s.saveSlotCount) {
		return core.StepFaulted, errors.New("gothic: slot out of range")
	}
	state := s.saves.Load(int(slot))
	if state == nil {
		core.Logf("gothic: save slot %d missing", slot)
		return core.StepFaulted, errors.New("gothic: save slot missing")
	}
	s.ui = nil
	s.replayMode = false
	s.titleMode = false
	s.skipStartMenu = false
	s.suppressLogo = false
	s.machine.Variables().Restore(state.Variables)
	s.machine.Restore(vmgothic.GothicMachineState{
		PC:       state.StatementIndex,
		Calls:    state.CallStack,
		Branches: state.Branches,
	})
	s.clearTransientState()
	if len(state.PlayerName) > 0 {
		s.playerName = state.PlayerName
	} else {
		s.playerName = gothicDefaultPlayerName
	}
	s.nameResolved = len(state.PlayerName) > 0
	layer := int(state.BackgroundLayer)
	if layer <= 0 {
		layer = 4
	}
	s.backgroundLayer = layer
	s.pageLines = s.pageLines[:0]
	if len(state.Background) > 0 {
		s.stage.LoadBase(layer, state.Background)
		s.background = state.Background
	}
	if len(state.Screen) == GothicScreenWidth*GothicScreenHeight*4 {
		s.stage.SetSnapshot(gfx.NewBsgImage(GothicScreenWidth, GothicScreenHeight, 0, 0, state.Screen))
	}
	s.gameState.SetSpeaker(state.Speaker)
	if len(state.PageLines) > 0 {
		s.pageLines = append(s.pageLines[:0], state.PageLines...)
	} else if len(state.Text) > 0 {
		rows := strings.Split(state.Text, "\n")
		for _, row := range rows {
			s.pageLines = append(s.pageLines, row)
		}
	}
	if len(s.pageLines) > 0 {
		s.syncMessage()
	} else {
		s.gameState.SetText("")
		s.stage.SetMessageLines(nil)
	}
	s.pageBreakPending = state.PageBreak
	s.frameMain = state.MessageVisible
	s.frameUpper = false
	s.frameLower = false
	s.stage.SetMessageVisible(state.MessageVisible)
	s.revealActive = false
	s.revealLimit = -1
	s.stage.SetMessageReveal(-1)
	s.gameState.SetMessageWindowVisible(true)
	s.gameState.SetWaitMilliseconds(0)
	s.gameState.SetAwaitingInput(state.PageBreak)
	s.gameState.ClearChoices()
	s.choiceActive = false
	s.choiceItems = nil
	s.bgmName = state.Bgm
	s.gameState.ClearBgm()
	s.gameState.ClearSoundEffect()
	s.lastBgm = ""
	s.lastBgmHas = false
	bgmPath := ""
	if filepath.IsAbs(state.Bgm) {
		if info, err := os.Stat(state.Bgm); err == nil && !info.IsDir() {
			bgmPath = state.Bgm
		}
	} else {
		bgmPath = s.audioCatalog.ResolveMusic(state.Bgm)
	}
	if bgmPath != "" {
		s.gameState.SetBgm(bgmPath)
		s.gameState.SetBgmLoop(true)
	}
	s.stage.Invalidate()
	s.renderFrame()
	core.Logf("gothic: save slot %d loaded statement %d", slot, state.StatementIndex)
	if state.PageBreak {
		// 存档点在一次段末停顿上：恢复后必须继续停在这一页，
		// 不能让宿主自旋的 Run 把下一页直接读掉。
		return core.StepAwaitInput, nil
	}
	return core.StepContinue, nil
}

func (s *Session) AssetSize(path string) int {
	if s.resources == nil {
		return 0
	}
	location, ok := s.resources.TryGet(path)
	if !ok || location == nil {
		return 0
	}
	size := location.Size()
	if size < 0 {
		return 0
	}
	return int(size)
}

func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if s.resources == nil || dst == nil || n <= 0 {
		return 0
	}
	location, ok := s.resources.TryGet(path)
	if !ok || location == nil {
		return 0
	}
	data := location.Read()
	if len(data) == 0 {
		return 0
	}
	buf := unsafe.Slice((*byte)(dst), n)
	return copy(buf, data)
}

func (s *Session) Audio() []core.AudioEvent {
	var events []core.AudioEvent
	bgm, hasBgm := s.gameState.Bgm()
	if s.global != nil && s.profile == gothicProfileMaze && !s.global.BgmOn {
		hasBgm = false
	}
	if hasBgm != s.lastBgmHas || bgm != s.lastBgm {
		if hasBgm {
			events = append(events, core.AudioEvent{Kind: 0, Path: s.materializeAudio(bgm), Volume: s.gameState.BgmVolume(), Loop: s.gameState.BgmLoop(), Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		} else {
			events = append(events, core.AudioEvent{Kind: 3, Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		}
		s.lastBgm = bgm
		s.lastBgmHas = hasBgm
	}
	voice, hasVoice := s.gameState.Voice()
	voiceSerial := s.gameState.VoiceSerial()
	if voiceSerial != s.lastVoiceSerial || hasVoice != s.lastVoiceHas || voice != s.lastVoice {
		if hasVoice {
			events = append(events, core.AudioEvent{Kind: 2, Path: s.materializeAudio(voice), Volume: 1, Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		}
		s.lastVoice = voice
		s.lastVoiceHas = hasVoice
		s.lastVoiceSerial = voiceSerial
	}
	sequence := s.gameState.VoiceSequence()
	sequenceSerial := s.gameState.VoiceSequenceSerial()
	if sequenceSerial != s.lastVoiceSeqSerial {
		if len(sequence) > 0 {
			names := make([]string, 0, len(sequence))
			for _, name := range sequence {
				names = append(names, s.materializeAudio(name))
			}
			events = append(events, core.AudioEvent{Kind: 6, Names: names, Loop: s.gameState.VoiceSequenceLoop(), Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		}
		s.lastVoiceSeqSerial = sequenceSerial
	}
	effect, hasEffect := s.gameState.SoundEffect()
	if s.global != nil && s.profile == gothicProfileMaze && !s.global.SeOn {
		hasEffect = false
	}
	effectSerial := s.gameState.EffectSerial()
	if effectSerial != s.lastEffectSerial || hasEffect != s.lastEffectHas || effect != s.lastEffect {
		if hasEffect {
			events = append(events, core.AudioEvent{Kind: 1, Path: s.materializeAudio(effect), Volume: 1, Channel: 0xFFFFFFFF, Group: 0xFFFFFFFF})
		}
		s.lastEffect = effect
		s.lastEffectHas = hasEffect
		s.lastEffectSerial = effectSerial
	}
	return events
}

func (s *Session) materializeAudio(name string) string {
	if path := audio.Materialize(name, s.resources, s.cacheDir); path != "" {
		return path
	}
	return name
}

func (s *Session) Text() core.TextState {
	awaiting := uint32(0)
	if s.gameState.AwaitingInput() || s.gameState.AwaitingChoice() || s.awaitingName || s.mapState.RouteActive() || s.mapState.ClickMapActive() {
		awaiting = 1
	}
	return core.TextState{Speaker: s.gameState.Speaker(), Text: s.gameState.Text(), Awaiting: awaiting}
}

func (s *Session) Choices() []core.Choice {
	choices := s.gameState.Choices()
	if len(choices) == 0 {
		return nil
	}
	out := make([]core.Choice, 0, len(choices))
	for index, name := range choices {
		out = append(out, core.Choice{Name: name, Value: int32(index)})
	}
	return out
}

func (s *Session) State() core.State {
	programCounter := uint32(0)
	instructionStart := uint32(0)
	if s.machine != nil {
		programCounter = uint32(s.machine.ProgramCounter())
		instructionStart = uint32(s.machine.InstructionStart())
	}
	bgm, _ := s.gameState.Bgm()
	waitMs := s.gameState.WaitMilliseconds()
	if waitMs < 0 {
		waitMs = 0
	}
	animating := !s.disposed && (s.stage.EffectActive || s.revealActive)
	return core.State{
		Bgm:                  bgm,
		PC:                   programCounter,
		Animating:            animating,
		WaitMs:               uint32(waitMs),
		MessageWindowVisible: s.gameState.MessageWindowVisible(),
		// MAZE 的正文由引擎直接画在画面上，没有 [message window] 配置。
		// 若继续让 Flutter 叠一层 MessageBox，玩家会同时看到顶部原文和
		// 底部转译框，两个文本框抢同一个点击。Toku1 仍交给宿主兜底。
		// Toku1 现在也由引擎画 p_kaiwa 对话框；宿主不能再用黑圆角框叠一层。
		EngineMessageWindow: true,
		AwaitingName:        s.awaitingName,
		ReplayActive:        false,
		InstructionStart:    instructionStart,
	}
}

func (s *Session) CurrentStep() core.Step {
	return s.currentStep()
}

func (s *Session) TextList() []core.TextUnit {
	return nil
}

func (s *Session) LastError() string {
	return s.lastError
}

func (s *Session) MapButtons() []core.MapButton {
	if !s.mapState.RouteActive() {
		return nil
	}
	buttons := s.gameState.Buttons()
	result := make([]core.MapButton, 0, len(buttons))
	for index, button := range buttons {
		result = append(result, core.MapButton{
			Index:  uint32(index),
			Image:  button.Image,
			X:      int32(button.X),
			Y:      int32(button.Y),
			Width:  uint32(button.Width),
			Height: uint32(button.Height),
			Value:  int32(button.Value),
		})
	}
	return result
}

func (s *Session) MapMask() (bool, int, int) {
	if s.mapState.ClickMapActive() || s.mapState.RouteActive() {
		return true, GothicScreenWidth, GothicScreenHeight
	}
	return false, 0, 0
}

func (s *Session) ComposeImages(names []string) (core.Frame, bool) {
	if s.catalog == nil && s.images == nil {
		return core.Frame{}, false
	}
	canvas := gfx.NewBgraCanvas(GothicScreenWidth, GothicScreenHeight)
	drawn := false
	for _, name := range names {
		var image *gfx.BsgImage
		if s.catalog != nil {
			image = s.catalog.Resolve(name)
		}
		if image == nil && s.images != nil {
			image = s.images.Get(name)
		}
		if image == nil {
			continue
		}
		drawn = true
		canvas.Draw(image, image.OffsetX, image.OffsetY)
	}
	if !drawn {
		return core.Frame{}, false
	}
	return core.Frame{Width: GothicScreenWidth, Height: GothicScreenHeight, Pixels: append([]byte{}, canvas.Buffer...)}, true
}

func (s *Session) SetPreview(f core.Frame) {
	s.stage.Frame.Clear(0, 0, 0, 255)
	if f.Width <= 0 || f.Height <= 0 || len(f.Pixels) != f.Width*f.Height*4 {
		return
	}
	image := gfx.NewBsgImage(f.Width, f.Height, 0, 0, append([]byte{}, f.Pixels...))
	s.stage.Frame.Draw(image, 0, 0)
}

func (s *Session) SelectFrame(name string) (core.Frame, bool) {
	_ = name
	return core.Frame{}, false
}

func (s *Session) CgGroups() []core.CgGroup {
	return nil
}

func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	if slot >= GothicSaveSlotCount {
		return core.SlotPreview{}, false
	}
	preview := s.saves.PreviewSlot(int(slot))
	if preview == nil {
		return core.SlotPreview{}, false
	}
	savedAt := ""
	if preview.HasSavedAt {
		savedAt = preview.SavedAt.Format(time.RFC3339)
	}
	out := core.SlotPreview{SavedAt: savedAt, Summary: preview.Summary, Speaker: preview.Speaker, Text: preview.Text}
	if preview.ThumbnailWidth > 0 && preview.ThumbnailHeight > 0 && len(preview.Thumbnail) == preview.ThumbnailWidth*preview.ThumbnailHeight*4 {
		out.Thumb = core.Frame{Width: preview.ThumbnailWidth, Height: preview.ThumbnailHeight, Pixels: append([]byte{}, preview.Thumbnail...)}
	}
	return out, true
}

// PressKey 处理宿主送来的按键。ESC/菜单键是引擎系统菜单的入口；
// 系统界面打开时只处理返回，避免方向键误触剧情。
func (s *Session) PressKey(code uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.ui != nil {
		switch code {
		case 27:
			return s.uiBack(), nil
		case 13, 32:
			if s.ui.mode == uiStaffRoll {
				return s.staffNext(), nil
			}
			return s.uiStep(), nil
		}
		return s.uiStep(), nil
	}
	if code == 27 || code == 0x70 /* F1 */ {
		if (s.profile == gothicProfileMaze || s.profile == gothicProfileToku1) && s.machine != nil {
			s.openIngame()
			return s.uiStep(), nil
		}
	}
	return s.currentStep(), nil
}

// TitleMenu 返回标题界面的按钮。MAZE 的按钮在引擎里逐项绘制；Toku1 的标题
// 底图已烘焙，但按钮仍由宿主命中，所以两者都要返回。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	if s.ui == nil || s.ui.mode != uiTitle {
		return nil
	}
	if s.isToku1Title() {
		return toku1TitleItems(s.ui.titleLoadEnabled)
	}
	if s.profile != gothicProfileMaze {
		return nil
	}
	anySave := false
	for slot := 0; slot < s.saveSlotCount; slot++ {
		if s.saves.Inspect(slot).Exists {
			anySave = true
			break
		}
	}
	return []core.TitleMenuItem{
		// 坐标来自 MAZE.EXE 的 FormMouseUp：五个 SetRect 分别是
		// (456,148)-(616,182)、(456,205)-(616,239)、(456,262)-(616,296)、
		// (456,319)-(616,353)、(456,376)-(616,410)。
		{Key: "start", Label: "初めから", X: 456, Y: 148, Width: 160, Height: 34, Enabled: true},
		{Key: "load", Label: "続きから", X: 456, Y: 205, Width: 160, Height: 34, Enabled: anySave},
		{Key: "config", Label: "オプション", X: 456, Y: 262, Width: 160, Height: 34, Enabled: true},
		{Key: "museum", Label: "鑑賞室", X: 456, Y: 319, Width: 160, Height: 34, Enabled: true},
		{Key: "quit", Label: "終了", X: 456, Y: 376, Width: 160, Height: 34, Enabled: true},
	}
}

func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if s.ui == nil || s.ui.mode != uiTitle {
		return s.currentStep(), nil
	}
	switch key {
	case "start":
		s.titleMode = false
		s.uiClose()
		s.stage.Reset()
		s.clearMessage()
		s.stage.Invalidate()
		return s.run(gothicDefaultRunBudget)
	case "load":
		s.openSlots(false, uiTitle)
		return core.StepAwaitSelect, nil
	case "config":
		s.openOption(uiTitle)
		return core.StepAwaitSelect, nil
	case "museum":
		s.openGalleryMenu()
		return core.StepAwaitSelect, nil
	case "quit":
		s.requestHost("exit")
		return core.StepAwaitTitle, nil
	}
	return core.StepAwaitTitle, nil
}

func (s *Session) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true
	// 退出前把本轮解锁写盘；应用被系统回收时 KzEngineDestroy 会走这里。
	s.flushGlobal()
	if s.font != nil {
		s.font.Dispose()
	}
	if s.resources != nil {
		s.resources.Dispose()
	}
	return nil
}

func (s *Session) isToku1() bool {
	return s.font.GlyphWidth == 22
}

func (s *Session) clearTransientState() {
	s.mapState.Reset()
	s.dateVisible = false
	s.dateWeek = 0
	s.dateDay = 0
	s.dateHour = 0
	s.dateMinute = 0
	s.resetHoverCache()
	s.map2Waiting = false
	s.pendingWave = s.pendingWave[:0]
	s.startMenuOpen = false
	s.nameWaitIssued = false
	s.awaitingName = false
	s.nameResolved = false
	s.choiceActive = false
	s.choiceItems = nil
	s.playerName = gothicDefaultPlayerName
	s.frameMain = true
	s.frameUpper = true
	s.frameLower = true
	s.aniOn = false
	s.controls = map[string][4]int32{}
	s.gameState.EndSelect()
	s.gameState.ClearChoices()
}

func (s *Session) replayTo(target int) {
	if s.machine == nil || target <= 0 {
		return
	}
	guard := s.machine.Program().Count() + 1024
	for step := 0; step < guard; step++ {
		if s.machine.Halted() || int(s.machine.ProgramCounter()) >= target {
			break
		}
		s.machine.Fetch()
	}
}

func (s *Session) summarize() string {
	text := strings.TrimSpace(strings.ReplaceAll(s.gameState.Text(), "\n", " "))
	speaker := s.gameState.Speaker()
	if len(speaker) > 0 {
		text = speaker + ": " + text
	}
	runes := []rune(text)
	if len(runes) <= 42 {
		return text
	}
	return string(runes[:42])
}

// findGothicFile 在 root 及其子目录里按大小写不敏感的方式查找文件。
//
// 光盘镜像解包后是否保留原大小写取决于镜像里是否带 Joliet/RockRidge：
// 本作（MAZE）不带 Joliet，解包结果是全小写的 maincon.scs / gothic2.dat。
// 原实现用 filepath.Join(root, "MAINCON.SCS") 直接拼接，Linux/Android 上
// 大小写敏感，于是脚本和字体都读不到，Start 直接 Faulted。这里逐目录
// 比对目录项而不是靠 os.Stat 猜测，保证两种解包结果都能打开。
func findGothicFile(root, name string) string {
	if len(root) == 0 || len(name) == 0 {
		return ""
	}
	type visit struct {
		directory string
		depth     int
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return ""
	}
	queue := []visit{{directory: root, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(current.directory)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.EqualFold(entry.Name(), name) {
				return filepath.Join(current.directory, entry.Name())
			}
		}
		if current.depth >= 4 {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				queue = append(queue, visit{directory: filepath.Join(current.directory, entry.Name()), depth: current.depth + 1})
			}
		}
	}
	return ""
}

// findGothicDirectory 与 findGothicFile 同理，用于 GRAPHICS 这类目录名。
func findGothicDirectory(root, name string) string {
	if len(root) == 0 || len(name) == 0 {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), name) {
			return filepath.Join(root, entry.Name())
		}
	}
	return ""
}
