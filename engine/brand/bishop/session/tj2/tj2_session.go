package tj2

import (
	"path/filepath"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm/scf"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

const tj2RunBudget = 4096

type Tj2Session struct {
	name          string
	directory     string
	dataDirectory string
	extraDirs     []string
	cacheDir      string

	resources *resource.GameResourceIndex
	frame     *gfx.BgraCanvas
	images    *rendering.ImageCache
	savesDir  string
	tj2Saves  *Tj2SaveStore
	bgm       *Tj2BgmCatalog

	gameState  *state.GameState
	table      *Tj2DataTable
	profile    Tj2GameProfile
	machine    *vmtj2.Tj2Machine
	stage      *Tj2Stage
	standees   Tj2StandeeRegistry
	mapScreen  Tj2MapScreen
	classicMap Tj2ClassicMap

	mapClock         bool
	bgmTrack         int
	tickCounter      int
	menuVisible      bool
	countdownVisible bool
	countdownSeconds int
	timerVisible     bool
	disposed         bool

	lastWipe     int
	shakeX       int
	shakeY       int
	textRed      uint8
	textGreen    uint8
	textBlue     uint8
	windowStyleA int
	windowStyleB int
	nameEntries  map[int]string

	generalDword       int
	counterA           int
	counterB           int
	counterC           int
	audioDword         int
	overlayValue       int
	titleMenuVisible   bool
	galleryVisible     bool
	soundMenuVisible   bool
	cgModeVisible      bool
	sceneRecallVisible bool
	unlockedGallery    map[int]struct{}
	galleryPath        string

	// 经典分支（SCF / GK CD）的界面状态。
	//
	// 这些字段只在 IsClassic() 为真时使用：现代 TJ2（侵蝕/淫獄の放課後2）
	// 仍由宿主绘制对话框，不能因为这里的标题/分页逻辑改变它们的步进。
	titleMode  bool
	titleHover int
	// warnVisible 表示正在显示 KEIKOKU 警告画面；点击后才会真正开始 SCN000。
	warnVisible bool
	// uiReturnTitle/uiReturnSystem 记录系统界面的返回目标：从标题打开的读档
	// 关闭后要回标题，从游戏内系统菜单打开的存读档要回菜单，不能一律 resume。
	uiReturnTitle  bool
	uiReturnSystem bool
	// messageHistory 是 LOG 按钮用的对白回放（只保留最近若干页，避免长周目吃内存）。
	messageHistory []string
	// textSpeedIndex/skipUnread 是 Toku2 配置面板的本地状态；文字速度会同步
	// 到 gameState，宿主的自动推进据此调整节奏。
	textSpeedIndex int
	skipUnread     bool
	bgmOn          bool
	seOn           bool
	voiceOn        bool
	uiMode         tj2UIMode
	uiPage         int
	uiHover        int
	uiSelected     int
	uiCgOpen       bool
	uiResumeRun    bool
	uiRevision     int
	uiComposedKey  int
	uiBase         *gfx.BgraCanvas
	uiSaveThumb    *gfx.BgraCanvas
	uiKeyed        map[string]*gfx.BsgImage
	cgThumbs       []string
	soundTracks    []string
	sceneTitles    []string
	messagePages   []string
	messagePage    int
	messageHover   int
	textFont       *textrender.Renderer
	textFontFailed bool

	shakeUntil   time.Time
	fillStart    time.Time
	fillDuration time.Duration
	fillRed      uint8
	fillGreen    uint8
	fillBlue     uint8
	fxCanvas     *gfx.BgraCanvas
	fxWasActive  bool
}

func NewTj2Session(name, directory, dataDirectory string, extraDirs []string, cacheDir string) *Tj2Session {
	s := &Tj2Session{
		name:            name,
		directory:       directory,
		dataDirectory:   dataDirectory,
		extraDirs:       append([]string(nil), extraDirs...),
		cacheDir:        cacheDir,
		textRed:         255,
		textGreen:       255,
		textBlue:        255,
		nameEntries:     map[int]string{},
		unlockedGallery: map[int]struct{}{},
		bgmOn:           true,
		seOn:            true,
		voiceOn:         true,
		bgmTrack:        -1,
	}
	core.Logf("tj2: open '%s' dir='%s'", name, dataDirectory)
	s.resources = resource.Build(dataDirectory, extraDirs...)
	s.frame = gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	s.images = rendering.NewImageCache(s.resources, rendering.DefaultImageBudgetBytes, rendering.DefaultImageCapacity)
	s.savesDir = filepath.Join(dataDirectory, "save")
	if cacheDir != "" {
		// 光盘镜像解包目录在 cache 下，系统清缓存会把存档一起清掉。
		// 宿主传的 cacheDir 是 <appSupport>/kizuna/cache/kizuna，
		// 往上两层就是持久的 <appSupport>/kizuna；按原始游戏目录哈希分目录。
		persistentRoot := filepath.Join(filepath.Dir(filepath.Dir(cacheDir)), "saves")
		if dir := core.GameCacheDir(persistentRoot, directory); dir != "" {
			s.savesDir = dir
		}
	}
	s.tj2Saves = NewTj2SaveStore(s.savesDir)
	s.galleryPath = filepath.Join(s.savesDir, "tj2", "gallery.json")
	s.loadGalleryUnlocks()
	bgmCache := cacheDir
	if bgmCache == "" {
		bgmCache = filepath.Join(s.savesDir, "bgm")
	}
	s.bgm = NewTj2BgmCatalog(directory, dataDirectory, bgmCache)
	s.gameState = state.NewGameState()
	s.table = s.loadTable()
	if s.table == nil {
		s.profile = ModernTj2Profile()
	} else {
		s.profile = ProfileForSession(s.table.Dialect(), s.resources)
	}
	s.stage = NewTj2Stage(s.images, s.profile.StandeeSlotCount)
	s.stage.ConfigureProfile(s.profile)
	if s.table == nil {
		core.Logf("tj2: SCNDAT.TBL missing, session will fault")
		return s
	}
	s.bgm.ConfigureTable(s.table)
	// GK 光盘版的 BGM 在数据目录的上一级 adp/ 里（disk1/exe → 游戏根/adp），
	// 只扫 exe 目录时所有曲目都会解析失败。把上两级目录作为 BGM 的额外根，
	// 素材索引本身仍只用 dataDirectory，不会把别的作品资源混进来。
	bgmExtras := append([]string(nil), extraDirs...)
	parent := dataDirectory
	for depth := 0; depth < 2; depth++ {
		next := filepath.Dir(parent)
		if next == parent || next == "." || next == string(filepath.Separator) {
			break
		}
		bgmExtras = append(bgmExtras, next)
		parent = next
	}
	s.bgm.ConfigureExtraDirectories(bgmExtras)
	s.machine = s.createMachine(s.table)
	if s.machine == nil {
		core.Logf("tj2: machine start failed, session will fault")
		return s
	}
	if s.IsClassic() {
		// 标题的"鑑賞室"分支要求 flag[3]=1（见 SCN001）。原版把通关标记
		// 存在全局存档里；没有全局存档时默认放行，避免菜单项静默失效。
		_ = s.machine.WriteFlag(3, 1)
	}
	s.stage.SetMessageWindow(s.loadMessageWindow())
	core.Logf("tj2: started SCN000 graphics=%d", len(s.table.GraphicNames()))
	return s
}

func (s *Tj2Session) Name() string {
	return s.name
}

func (s *Tj2Session) State() *state.GameState {
	return s.gameState
}

func (s *Tj2Session) Resources() *resource.GameResourceIndex {
	return s.resources
}

func (s *Tj2Session) Frame() *gfx.BgraCanvas {
	return s.frame
}

func (s *Tj2Session) Images() *rendering.ImageCache {
	return s.images
}

func (s *Tj2Session) Width() int {
	return ScreenWidth
}

func (s *Tj2Session) Height() int {
	return ScreenHeight
}

func (s *Tj2Session) ProgramCounter() int {
	if s.machine == nil {
		return 0
	}
	return s.machine.ProgramCounter()
}

func (s *Tj2Session) InstructionStart() int {
	if s.machine == nil {
		return 0
	}
	return s.machine.InstructionStart()
}

func (s *Tj2Session) Machine() *vmtj2.Tj2Machine {
	return s.machine
}

func (s *Tj2Session) Map() *Tj2MapScreen {
	return &s.mapScreen
}

func (s *Tj2Session) ClassicMap() *Tj2ClassicMap {
	return &s.classicMap
}

func (s *Tj2Session) Stage() *Tj2Stage {
	return s.stage
}

func (s *Tj2Session) Table() *Tj2DataTable {
	return s.table
}

func (s *Tj2Session) Profile() Tj2GameProfile {
	return s.profile
}

func (s *Tj2Session) MenuVisible() bool {
	return s.menuVisible
}

func (s *Tj2Session) BgmTrack() int {
	return s.bgmTrack
}

func (s *Tj2Session) TickCounter() int {
	return s.tickCounter
}

func (s *Tj2Session) CountdownVisible() bool {
	return s.countdownVisible
}

func (s *Tj2Session) CountdownSeconds() int {
	return s.countdownSeconds
}

func (s *Tj2Session) TimerVisible() bool {
	return s.timerVisible
}

func (s *Tj2Session) LastWipe() int {
	return s.lastWipe
}

func (s *Tj2Session) ShakeX() int {
	return s.shakeX
}

func (s *Tj2Session) ShakeY() int {
	return s.shakeY
}

func (s *Tj2Session) IsClassic() bool {
	return s.profile.Variant == vmtj2.Tj2VariantClassic
}

// UsesEngineUI 表示对话框、标题、系统菜单由引擎自绘。
//
// 它和 IsClassic 分开：特別授業2 的字节码是现代方言（220/1），但界面
// 仍属于 TJ2.EXE 家族，必须走引擎自绘分支。纯现代作品返回 false，
// 继续由宿主画对话框。
func (s *Tj2Session) UsesEngineUI() bool {
	return s.profile.UIFamily != Tj2UIFamilyModern
}

// IsToku2UI 表示当前是 特別授業2 的 sp_mes1 + TJ2.EXE 标题/鉴赏家族。
func (s *Tj2Session) IsToku2UI() bool {
	return s.profile.UIFamily == Tj2UIFamilyToku2
}

// SkipUnread 对应「既読もスキップ」。宿主在自动跳过时读取 State.SkipUnread。
func (s *Tj2Session) SkipUnread() bool {
	return s.skipUnread
}

func (s *Tj2Session) RecordMapDefault(cell, destination int) {
	s.mapScreen.RecordDefault(cell, destination)
}

func (s *Tj2Session) RecordMapCells(page, cell, frame, highlight, destination int) {
	s.mapScreen.RecordCells(page, cell, frame, highlight, destination)
}

func (s *Tj2Session) SetMapClock(visible bool) {
	s.mapClock = visible
	s.mapScreen.SetClockVisible(visible)
}

func (s *Tj2Session) OpenMap() state.SessionStep {
	s.SetMenuVisible(false)
	if s.machine == nil {
		return state.SessionFaulted
	}
	if s.mapScreen.Active() {
		if s.mapScreen.Fallback() {
			return state.SessionAwaitChoice
		}
		return state.SessionAwaitSelect
	}
	page, _ := s.machine.ReadFlag(0)
	fr, _ := s.machine.ReadFlag(1)
	if s.mapScreen.Activate(s.images, s.resources, s.mapClock, int(page), int(fr)) {
		core.Logf("tj2: func10 map page=%d frame=%d tick=%d", page, fr, s.tickCounter)
		return state.SessionAwaitSelect
	}
	s.mapScreen.BeginFallback(int(page), int(fr))
	destinations := s.mapScreen.FallbackDestinations(int(page), int(fr))
	if len(destinations) == 0 {
		destinations = append(destinations, 0)
	}
	labels := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		labels = append(labels, ScenarioName(66+destination))
	}
	s.gameState.SetChoices(labels)
	core.Logf("tj2: func10 map fallback page=%d frame=%d choices=%d tick=%d", page, fr, len(labels), s.tickCounter)
	return state.SessionAwaitChoice
}

func (s *Tj2Session) ChooseMapCell(cell int) state.SessionStep {
	if s.machine == nil {
		return state.SessionFaulted
	}
	destination := s.mapScreen.DestinationFor(cell, s.mapScreen.Page(), s.mapScreen.Frame())
	_ = s.machine.WriteFlag(0, int32(destination))
	s.mapScreen.Deactivate()
	s.stage.Invalidate()
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	core.Logf("tj2: map click cell=%d destination=%d tick=%d", cell, destination, s.tickCounter)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) chooseMapFallback(index int) state.SessionStep {
	if s.machine == nil {
		return state.SessionFaulted
	}
	destination, ok := s.mapScreen.TryGetFallbackDestination(s.mapScreen.Page(), s.mapScreen.Frame(), index)
	if !ok {
		core.Logf("tj2: map fallback choice %d rejected", index)
		return state.SessionAwaitChoice
	}
	_ = s.machine.WriteFlag(0, int32(destination))
	s.mapScreen.Deactivate()
	s.stage.Invalidate()
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	core.Logf("tj2: map fallback choice %d destination=%d tick=%d", index, destination, s.tickCounter)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) RecordWipe(value int) {
	s.lastWipe = value
}

func (s *Tj2Session) SetShake(x, y int) {
	s.startShake(x, y, 220)
}

func (s *Tj2Session) ApplyScreenFill(red, green, blue int) {
	s.startScreenFill(red, green, blue, 400)
}

// ApplyScreenFillTimed 对应 func1 的 r,g,b,outMs,inMs 参数。
// 旧脚本用它表现"眼前发黑/白光一闪"，是短时间效果而不是常驻底色。
func (s *Tj2Session) ApplyScreenFillTimed(red, green, blue, durationMs int) {
	s.startScreenFill(red, green, blue, durationMs)
}

func (s *Tj2Session) SetTextColor(red, green, blue uint8) {
	s.textRed = red
	s.textGreen = green
	s.textBlue = blue
}

func (s *Tj2Session) SetWindowStyle(first, second int) {
	s.windowStyleA = first
	s.windowStyleB = second
}

func (s *Tj2Session) SetNameEntry(index int, name string) {
	s.nameEntries[index] = name
}

func (s *Tj2Session) UnlockGallery(id int) {
	if id < 0 {
		return
	}
	if _, ok := s.unlockedGallery[id]; ok {
		return
	}
	s.unlockedGallery[id] = struct{}{}
	s.flushGalleryUnlocks()
}

func (s *Tj2Session) ReloadMessageWindow() {
	s.stage.SetMessageWindow(s.loadMessageWindow())
	s.stage.Invalidate()
}

func (s *Tj2Session) Run(budget int) state.SessionStep {
	if s.disposed {
		return state.SessionFinished
	}
	if s.machine == nil {
		return state.SessionFaulted
	}
	// 引擎自绘界面（标题/系统菜单/鉴赏）是"停在原地"的状态：
	// 脚本 pc 已经越过对应命令，但在玩家做出选择之前绝不能继续 Fetch。
	if s.UsesEngineUI() {
		if s.uiMode != tj2UINone {
			return s.classicUIWaitStep()
		}
		if s.titleMode {
			return state.SessionAwaitTitle
		}
	}
	if s.mapScreen.Active() {
		if s.mapScreen.Fallback() {
			return state.SessionAwaitChoice
		}
		return state.SessionAwaitSelect
	}
	if s.IsClassic() && s.classicMap.Active() && s.gameState.AwaitingChoice() {
		return state.SessionAwaitSelect
	}
	for executed := 0; executed < budget; executed++ {
		command := s.machine.Fetch()
		if s.machine.Faulted() {
			core.Logf("tj2: fault at %d", s.machine.InstructionStart())
			return state.SessionFaulted
		}
		step := s.Dispatch(command)
		if step != state.SessionContinue {
			return step
		}
		if len(command.Options) > 0 {
			return s.PublishChoices(command.Options)
		}
	}
	return state.SessionContinue
}

func (s *Tj2Session) Advance() state.SessionStep {
	if s.disposed {
		return state.SessionFinished
	}
	if s.UsesEngineUI() {
		if s.uiMode != tj2UINone {
			return s.classicUIWaitStep()
		}
		if s.titleMode {
			return state.SessionAwaitTitle
		}
		if s.warnVisible {
			return s.dismissWarning()
		}
		// 右键隐藏信息框后，下一次点击先恢复窗口，不消耗这一页对白。
		if !s.gameState.MessageWindowVisible() && s.gameState.Text() != "" {
			s.gameState.SetMessageWindowVisible(true)
			s.stage.Invalidate()
			return state.SessionAwaitInput
		}
		// 同一条 !s 里可能有多页（_t 分页 / 超过 4 行）。先翻页，
		// 全部翻完才让 VM 继续执行下一条命令。
		if s.messagePage+1 < len(s.messagePages) {
			s.messagePage++
			s.gameState.SetText(s.messagePages[s.messagePage])
			s.appendMessageHistory(s.messagePages[s.messagePage])
			s.gameState.SetAwaitingInput(true)
			s.stage.Invalidate()
			return state.SessionAwaitInput
		}
		s.messagePages = nil
		s.messagePage = 0
	}
	s.gameState.SetAwaitingInput(false)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) Choose(index int) state.SessionStep {
	if s.disposed {
		return state.SessionFinished
	}
	if s.machine == nil {
		return state.SessionFaulted
	}
	if s.UsesEngineUI() && (s.uiMode != tj2UINone || s.titleMode) {
		if s.titleMode {
			return state.SessionAwaitTitle
		}
		return s.classicUIWaitStep()
	}
	if s.mapScreen.Active() && s.mapScreen.Fallback() {
		return s.chooseMapFallback(index)
	}
	if s.mapScreen.Active() {
		return state.SessionAwaitSelect
	}
	if s.IsClassic() && s.classicMap.Active() && s.gameState.AwaitingChoice() {
		return state.SessionAwaitSelect
	}
	if !s.gameState.AwaitingChoice() {
		return state.SessionContinue
	}
	if !s.machine.ChooseDisplay(index) {
		core.Logf("tj2: choice %d rejected", index)
		return state.SessionAwaitChoice
	}
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) Compose() bool {
	if s.disposed {
		return false
	}
	if s.warnVisible {
		return s.composeWarning()
	}
	if s.UsesEngineUI() {
		if s.uiMode != tj2UINone {
			if s.IsToku2UI() && s.composeToku2UIScreen() {
				return true
			}
			return s.composeClassicSystemUI()
		}
		// 闪白/震动的绘制会改写 frame；stage 的缓存若认为"没变"就不会
		// 重画底图。效果期间强制重画底图，效果刚结束的那一帧也要再画一次，
		// 否则最后一帧的位移/残色会留在屏幕上（宿主随后停止合成）。
		if active := s.effectsActive(); active {
			s.fxWasActive = true
			s.stage.Invalidate()
		} else if s.fxWasActive {
			s.fxWasActive = false
			s.stage.Invalidate()
		}
		if s.titleMode {
			return s.composeTitleScreen()
		}
	}
	if s.IsClassic() && s.classicMap.Active() {
		return s.applyScreenEffects(s.classicMap.ComposeTo(s.frame, s.images))
	}
	if s.mapScreen.Active() && !s.mapScreen.Fallback() {
		return s.mapScreen.ComposeTo(s.frame)
	}
	if s.UsesEngineUI() {
		visible := s.classicMessageFrameVisible()
		dirty := s.stage.ComposeTo(s.frame, visible)
		if visible {
			s.drawClassicMessageText()
			s.drawMessageButtonHover()
		}
		s.drawEngineMenuButton()
		return s.applyScreenEffects(dirty)
	}
	return s.stage.ComposeTo(s.frame, s.gameState.ShowMessageFrame())
}

func (s *Tj2Session) MapButtons() []state.GameMapButton {
	if !s.IsClassic() || !s.classicMap.Active() || !s.gameState.AwaitingChoice() {
		return []state.GameMapButton{}
	}
	buttons := make([]state.GameMapButton, 0, Tj2ClassicRowCount)
	for row := 0; row < Tj2ClassicRowCount; row++ {
		buttons = append(buttons, state.GameMapButton{
			Index: row,
			// Image 只用来告诉宿主"卡面已由引擎画进帧"：Flutter 的
			// MapLayer 对空 Image 的按钮会补一圈半透明白框，叠在 MVM01
			// 的雕花地点按钮上会很脏。这里给一个非空占位名，宿主不会去取图。
			Image:  "classic-map",
			X:      Tj2ClassicHitX,
			Y:      Tj2ClassicHitTop + row*Tj2ClassicHitHeight,
			Width:  Tj2ClassicHitWidth,
			Height: Tj2ClassicHitHeight,
			Value:  row,
			Label:  s.classicMap.LabelFor(row),
		})
	}
	return buttons
}

func (s *Tj2Session) HasMapMask() bool {
	return s.mapScreen.Active() && !s.mapScreen.Fallback()
}

func (s *Tj2Session) MapMaskWidth() int {
	if !s.HasMapMask() {
		return 0
	}
	return Tj2MapWidth
}

func (s *Tj2Session) MapMaskHeight() int {
	if !s.HasMapMask() {
		return 0
	}
	return Tj2MapHeight
}

func (s *Tj2Session) ChooseMapPixel(x, y int) state.SessionStep {
	if s.disposed {
		return state.SessionFinished
	}
	if s.machine == nil {
		return state.SessionFaulted
	}
	if !s.mapScreen.Active() {
		return state.SessionContinue
	}
	if s.mapScreen.Fallback() {
		return state.SessionAwaitChoice
	}
	cell := s.mapScreen.CellAt(x, y)
	if cell < Tj2MapMinCell || cell > Tj2MapMaxCell {
		return state.SessionAwaitSelect
	}
	return s.ChooseMapCell(cell)
}

func (s *Tj2Session) CancelMapSelect() state.SessionStep {
	return state.SessionContinue
}

func (s *Tj2Session) SetHoveredButton(index int) {
	if !s.IsClassic() || !s.classicMap.Active() {
		return
	}
	s.classicMap.SetHovered(index)
	s.classicMap.SetSelected(index)
	s.gameState.SetHoveredButton(index)
}

func (s *Tj2Session) SetMapHover(x, y int) {
	if s.disposed || !s.mapScreen.Active() || s.mapScreen.Fallback() {
		return
	}
	s.mapScreen.Hover(x, y)
}

func (s *Tj2Session) ChooseButton(index int) state.SessionStep {
	if s.disposed {
		return state.SessionFinished
	}
	if s.machine == nil {
		return state.SessionFaulted
	}
	if !s.IsClassic() || !s.classicMap.Active() {
		return state.SessionContinue
	}
	if !s.gameState.AwaitingChoice() {
		return state.SessionContinue
	}
	if index < 0 || index >= Tj2ClassicRowCount || index >= len(s.gameState.Choices()) {
		core.Logf("tj2: classic map row %d rejected", index)
		return state.SessionAwaitSelect
	}
	if !s.machine.ChooseDisplay(index) {
		core.Logf("tj2: classic map row %d rejected", index)
		return state.SessionAwaitSelect
	}
	s.classicMap.SetSelected(index)
	s.classicMap.SetActive(false)
	s.stage.Invalidate()
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	core.Logf("tj2: classic map row %d chosen tick=%d", index, s.tickCounter)
	return s.Run(tj2RunBudget)
}

func (s *Tj2Session) StartReplay(sceneID string) bool {
	return false
}

func (s *Tj2Session) ComposeImages(names []string) *gfx.BsgImage {
	canvas := gfx.NewBgraCanvas(s.Width(), s.Height())
	drawn := false
	for _, name := range names {
		image := s.images.Get(name)
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

func (s *Tj2Session) ComposePreview(image *gfx.BsgImage) {
	s.frame.Clear(0, 0, 0, 255)
	s.frame.Draw(image, image.OffsetX, image.OffsetY)
}

func (s *Tj2Session) Restart() {
	if s.machine != nil {
		s.machine.Start(0)
		// 旧版 GK 用 flag[3] 表示"通关后解锁鉴赏室"。原作把这份状态放在
		// 全局存档里；我们目前没有全局存档，标题又始终显示"鑑賞室"，
		// 所以每次开新周目都把它置 1，避免菜单点下去被脚本静默忽略。
		if s.IsClassic() {
			_ = s.machine.WriteFlag(3, 1)
		}
	}
	s.mapScreen.Deactivate()
	s.classicMap.SetActive(false)
	s.titleMode = false
	s.titleHover = -1
	s.warnVisible = false
	s.uiReturnTitle = false
	s.uiReturnSystem = false
	s.messageHistory = nil
	s.uiMode = tj2UINone
	s.uiPage = 0
	s.uiHover = -1
	s.uiSelected = -1
	s.uiCgOpen = false
	s.uiResumeRun = false
	s.uiRevision++
	s.uiComposedKey = -1
	s.uiSaveThumb = nil
	s.messagePages = nil
	s.messagePage = 0
	s.messageHover = 0
	s.shakeUntil = time.Time{}
	s.fillStart = time.Time{}
	s.fillDuration = 0
	s.fxWasActive = false
	s.stage.ClearAll()
	s.standees.Clear()
	s.gameState.SetSpeaker("")
	s.gameState.SetText("")
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearBgm()
	s.gameState.SetBgmLoop(true)
	s.gameState.ClearSoundEffect()
	s.gameState.ClearVoice()
	s.gameState.SetWaitMilliseconds(0)
	// 引擎自绘信息框的分支在标题/启动画面阶段不应显示信息框；第一条 !s 会重新打开。
	s.gameState.SetMessageWindowVisible(!s.UsesEngineUI())
	s.bgmTrack = -1
	s.menuVisible = false
	s.tickCounter = 0
	s.timerVisible = false
	s.countdownVisible = false
	s.countdownSeconds = 0
	s.stage.SetMessageWindow(s.loadMessageWindow())
}

func (s *Tj2Session) Dispose() {
	if s.disposed {
		return
	}
	s.disposed = true
	s.flushGalleryUnlocks()
	s.resources.Dispose()
}

func (s *Tj2Session) SetMenuVisible(visible bool) {
	s.menuVisible = visible
}

func (s *Tj2Session) AddTick() {
	s.tickCounter++
}

func (s *Tj2Session) SetBgmTrack(track int) {
	s.bgmTrack = track
}

func (s *Tj2Session) ResolveBgm(track int) string {
	return s.bgm.Resolve(track)
}

func (s *Tj2Session) ShowCountdown(seconds int) {
	s.countdownSeconds = seconds
	s.countdownVisible = true
}

func (s *Tj2Session) HideCountdown() {
	s.countdownVisible = false
}

func (s *Tj2Session) SetTimerVisible(visible bool) {
	s.timerVisible = visible
}

func (s *Tj2Session) ClearAll() {
	s.stage.ClearAll()
	s.standees.Clear()
	s.menuVisible = false
	s.timerVisible = false
}

func (s *Tj2Session) loadMessageWindow() *gfx.BsgImage {
	name := s.profile.MessageWindowImage
	image := s.images.Get(name)
	if image == nil {
		core.Logf("tj2: message window '%s' missing", name)
		return nil
	}
	core.Logf("tj2: message window '%s' %dx%d", name, image.Width, image.Height)
	return image
}

func (s *Tj2Session) loadTable() *Tj2DataTable {
	location, ok := s.resources.TryGet(TableName)
	if !ok {
		return nil
	}
	table, err := LoadTj2DataTable(location.Read())
	if err != nil {
		core.Logf("tj2: SCNDAT.TBL load failed: %v", err)
		return nil
	}
	return table
}

func (s *Tj2Session) createMachine(table *Tj2DataTable) *vmtj2.Tj2Machine {
	machine := vmtj2.NewTj2Machine(table, s.loadScenario)
	machine.Start(0)
	if machine.Faulted() {
		core.Logf("tj2: machine start failed, session will fault")
		return nil
	}
	return machine
}

func (s *Tj2Session) loadScenario(number int) (vmtj2.ScfFile, error) {
	name := ScenarioName(number)
	location, ok := s.resources.TryGet(name)
	if !ok {
		core.Logf("tj2: scenario %s not found", name)
		return nil, errTj2ScenarioMissing
	}
	file, err := scf.LoadScfScriptFile(location.Read())
	if err != nil {
		core.Logf("tj2: scenario %s load failed: %v", name, err)
		return nil, err
	}
	return file.AsTj2File(), nil
}

func clampTj2Byte(value int) uint8 {
	return uint8(core.Clamp(value, 0, 255))
}

var errTj2ScenarioMissing = tj2MissingError{}

type tj2MissingError struct{}

func (tj2MissingError) Error() string {
	return "tj2 scenario missing"
}
