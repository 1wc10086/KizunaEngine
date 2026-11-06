package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/brand/emon/emecfg"
	_ "kizuna/engine/brand/emon/glyph"
	"kizuna/engine/brand/emon/save"
	"kizuna/engine/brand/emon/stage"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/brand/emon/vm"
	"kizuna/engine/core"
	"kizuna/engine/library"
)

const (
	commandBudget        = 4096
	composeBudget        = 256
	saveSlotCount        = 24
	maximumDelta         = 0.25
	messageCharsPerSec   = 30
	disposeGateWaitMs    = 3000
	noTarget             = ^uint32(0)
	maxCompletionDrain   = 1024
	audioNameMapLimit    = 512
	audioCacheBudgetMB   = 256
	audioCacheBudgetFile = 512
	previewWidth         = 192
	previewHeight        = 144
	previewFontSize      = 14
	previewLineHeight    = 16
	previewCharsPerLine  = 12
	previewLines         = 7
	summaryLimit         = 42
	selectItemMinimum    = 44
	defaultTitle         = "game"
)

var (
	errNotOpen  = errorString("emon session: engine not open")
	errBadEntry = errorString("emon session: bad entry")
	errNoSlot   = errorString("emon session: slot has no save")
	errSaveFail = errorString("emon session: save failed")
	errLocked   = errorString("emon session: busy")
)

type errorString string

func (e errorString) Error() string { return string(e) }

type Session struct {
	dir            string
	extraDirs      []string
	cacheDir       string
	audioCacheDir  string
	archiveDir     string
	title          string
	width          int
	height         int
	config         *emecfg.EmecfgFile
	archives       *eme.EmeArchiveSet
	profile        *emecfg.EmonGameProfile
	ctx            *vm.RuntimeContext
	composer       *stage.EmonStageComposer
	stage          *stage.EmonStage
	message        *emontext.EmonMessageBox
	audio          *vm.EmonAudioEngine
	machine        *vm.Machine
	saves          *save.EmonSaveStore
	frame          *bishopgfx.BgraCanvas
	masks          *stage.EmonCachedMaskPatterns
	gate           sync.Mutex
	audioGate      sync.Mutex
	audioNameGate  sync.Mutex
	audioEvents    []core.AudioEvent
	audioNames     map[string]string
	bgmArchiveName string
	effectArchive  string
	bgm            string
	bgmVolume      float64
	bgmLoop        bool
	step           vm.EmonStep
	mapped         core.Step
	instruction    int32
	stageSignature int
	presentSig     int
	frameSerial    int
	handedSerial   int
	blackHanded    bool
	clock          time.Time
	lastSeconds    float64
	selectNames    []string
	selectSig      int
	selectShown    bool
	speaker        string
	text           string
	awaitingInput  bool
	disposed       bool
	opened         bool
	trace          []string
	info           core.Info
}

var (
	_ core.Engine          = (*Session)(nil)
	_ core.OpenerExtras    = (*Session)(nil)
	_ core.NamePrompter    = (*Session)(nil)
	_ core.AudioNotifier   = (*Session)(nil)
	_ core.MapExposer      = (*Session)(nil)
	_ core.GalleryComposer = (*Session)(nil)
	_ core.SlotPreviewer   = (*Session)(nil)
	_ core.KeyReceiver     = (*Session)(nil)
	_ core.WheelReceiver   = (*Session)(nil)
	_ core.Closer          = (*Session)(nil)
)

func New() *Session {
	return &Session{
		audioNames:     map[string]string{},
		mapped:         core.StepContinue,
		presentSig:     -1,
		stageSignature: -1,
		handedSerial:   -1,
	}
}

func (s *Session) Open(dir string) (core.Info, error) {
	return s.OpenExtras(dir, nil, "")
}

func (s *Session) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	if dir == "" {
		return core.Info{}, errNotOpen
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return core.Info{}, errNotOpen
	}
	if s.opened {
		s.Close()
	}
	s.dir = dir
	s.extraDirs = append([]string(nil), extraDirs...)
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "kizuna-emon-audio")
	}
	s.cacheDir = cacheDir
	s.audioCacheDir = filepath.Join(cacheDir, time.Now().Format("06-01-02"))
	s.audioNames = map[string]string{}
	s.audioEvents = nil
	s.bgmArchiveName = ""
	s.effectArchive = ""
	s.bgm = ""
	s.bgmVolume = 1
	s.bgmLoop = false
	s.step = vm.EmonStepContinue
	s.mapped = core.StepContinue
	s.instruction = 0
	s.stageSignature = -1
	s.presentSig = -1
	s.frameSerial = 0
	s.handedSerial = -1
	s.blackHanded = false
	s.selectNames = nil
	s.selectSig = 0
	s.selectShown = false
	s.speaker = ""
	s.text = ""
	s.awaitingInput = false
	s.disposed = false
	s.trace = nil
	config := loadConfiguration(dir, extraDirs)
	s.config = config
	archiveDir := resolveArchiveDirectory(dir, extraDirs, config)
	s.archiveDir = archiveDir
	archives := eme.OpenDirectory(archiveDir)
	s.archives = archives
	s.profile = emecfg.ForEmonGameProfile(config, archives)
	width := emecfg.DefaultWindowWidth
	height := emecfg.DefaultWindowHeight
	if config != nil {
		if config.WindowWidth > 0 {
			width = config.WindowWidth
		}
		if config.WindowHeight > 0 {
			height = config.WindowHeight
		}
	}
	s.width = width
	s.height = height
	s.title = s.profile.Name
	if s.title == "" {
		s.title = filepath.Base(dir)
	}
	frame := bishopgfx.NewBgraCanvas(width, height)
	frame.Clear(0, 0, 0, 255)
	s.frame = frame
	audio := vm.NewEmonAudioEngine(archiveDir, archives.Read)
	s.audio = audio
	audio.CommandIssued = s.onAudioCommand
	s.saves = save.NewEmonSaveStore(save.DefaultDirectory(dir))
	s.clock = time.Now()
	s.lastSeconds = 0
	if config == nil {
		s.record("emecfg.ecf missing, session will fault")
		core.Logf("emon: open '%s' without configuration, session will fault", filepath.Base(dir))
		s.machine = nil
		s.info = core.Info{Width: width, Height: height, Title: s.title}
		s.opened = true
		return s.info, nil
	}
	ctx := vm.CreateRuntimeContext(config, archives, audio, &savesAdapter{store: s.saves}, archiveDir)
	composer, ok := ctx.Composer.(*stage.EmonStageComposer)
	if !ok {
		s.record("stage composer unavailable")
		s.machine = nil
		s.info = core.Info{Width: width, Height: height, Title: s.title}
		s.opened = true
		return s.info, nil
	}
	stg, ok := ctx.Stage.(*stage.EmonStage)
	if !ok {
		s.record("stage unavailable")
		s.machine = nil
		s.info = core.Info{Width: width, Height: height, Title: s.title}
		s.opened = true
		return s.info, nil
	}
	msg, ok := ctx.Message.(*emontext.EmonMessageBox)
	if !ok {
		s.record("message box unavailable")
		s.machine = nil
		s.info = core.Info{Width: width, Height: height, Title: s.title}
		s.opened = true
		return s.info, nil
	}
	s.ctx = ctx
	s.composer = composer
	s.stage = stg
	s.message = msg
	glyphs := emontext.CurrentGlyphSource()
	if glyphs == nil {
		glyphs = emontext.ResolveGlyphSource()
	}
	composer.SetGlyphSource(glyphs)
	emecfg.ApplyTextSymbols(composer.TextStyle(), config.Symbols)
	s.masks = stage.NewEmonCachedMaskPatterns(&maskSource{load: s.loadImage})
	composer.SetMaskPatterns(s.masks)
	for _, macro := range config.MacroList {
		ctx.Macros.Add(macro.Name, macro)
	}
	machine := vm.NewMachine(ctx)
	s.machine = machine
	machine.RequestJump(config.StartScript)
	core.Logf("emon: open '%s' %dx%d archives=%d macros=%d symbols=%d sprites=%d", s.title, width, height, len(archives.Names), len(config.MacroList), len(config.Symbols), s.profile.SpriteFileCount())
	s.info = core.Info{Width: width, Height: height, Title: s.title, Version: config.StartScript}
	s.opened = true
	return s.info, nil
}

func (s *Session) Info() core.Info {
	return s.info
}

func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.machine == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	if entry != 0 {
		return core.StepFaulted, errBadEntry
	}
	startScript := emecfg.StartScriptFileName
	if s.config != nil && s.config.StartScript != "" {
		startScript = s.config.StartScript
	}
	s.gate.Lock()
	defer s.gate.Unlock()
	s.machine.RequestJump(startScript)
	s.step = vm.EmonStepContinue
	s.mapped = core.StepContinue
	return core.StepContinue, nil
}

func loadConfiguration(dir string, extraDirs []string) *emecfg.EmecfgFile {
	candidates := []string{filepath.Join(dir, emecfg.ConfigurationFileName)}
	for _, root := range extraDirs {
		if root == "" {
			continue
		}
		candidates = append(candidates, filepath.Join(root, emecfg.ConfigurationFileName))
	}
	for _, candidate := range candidates {
		config, err := emecfg.LoadEmecfgFile(candidate)
		if err != nil {
			core.Logf("emon: %s load failed: %s", emecfg.ConfigurationFileName, err.Error())
			continue
		}
		if config != nil {
			return config
		}
	}
	if located, ok := library.FindFile(dir, emecfg.ConfigurationFileName); ok {
		config, err := emecfg.LoadEmecfgFile(located)
		if err != nil {
			core.Logf("emon: %s load failed: %s", emecfg.ConfigurationFileName, err.Error())
			return nil
		}
		return config
	}
	return nil
}

func resolveArchiveDirectory(dir string, extraDirs []string, config *emecfg.EmecfgFile) string {
	configured := emecfg.DefaultDataDirectoryName
	if config != nil && config.DataDirectory != "" {
		configured = config.DataDirectory
	}
	roots := append([]string{dir}, extraDirs...)
	var candidates []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		candidates = append(candidates, filepath.Join(root, configured))
	}
	candidates = append(candidates, roots...)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if hasEmeFiles(candidate) {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return dir
}

func hasEmeFiles(directory string) bool {
	children, err := os.ReadDir(directory)
	if err != nil {
		return false
	}
	for _, child := range children {
		if child.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(child.Name()), ".eme") {
			return true
		}
	}
	return false
}

func sanitizeName(name string) string {
	var buffer []byte
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '/' || c == '\\' || c < 0x20 || c == 0x7f {
			continue
		}
		switch c {
		case '<', '>', ':', '"', '|', '?', '*':
			continue
		}
		buffer = append(buffer, c)
	}
	safe := strings.TrimSpace(string(buffer))
	safe = strings.TrimRight(safe, ".")
	if safe == "" {
		return defaultTitle
	}
	return safe
}

func (s *Session) record(message string) {
	if len(s.trace) < 256 {
		s.trace = append(s.trace, message)
	}
}

type savesAdapter struct {
	store *save.EmonSaveStore
}

func (a *savesAdapter) SystemState() vm.EmonSystemState {
	return a.store.SystemState()
}

func (a *savesAdapter) Write(data *vm.EmonSaveData) bool {
	return a.store.Write(data)
}

func (a *savesAdapter) Read(slot int) *vm.EmonSaveData {
	return a.store.Read(slot)
}

func (a *savesAdapter) Exists(slot int) bool {
	return a.store.Exists(slot)
}

func (a *savesAdapter) Delete(slot int) bool {
	return a.store.Delete(slot)
}

func (s *Session) loadImage(name string) *bishopgfx.BsgImage {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if s.machine == nil {
		return nil
	}
	return s.machine.LoadImage(name)
}

type maskSource struct {
	load func(string) *bishopgfx.BsgImage
}

func (m *maskSource) Get(pattern int) *bishopgfx.BsgImage {
	if m == nil || m.load == nil {
		return nil
	}
	return m.load(stage.MaskPatternFileName(pattern))
}

func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if dst == nil || n <= 0 || s.frame == nil {
		return false, nil
	}
	if s.width <= 0 || s.height <= 0 || n < s.width*s.height*4 {
		return false, nil
	}
	if !s.gate.TryLock() {
		return false, nil
	}
	defer s.gate.Unlock()
	if s.disposed {
		return false, nil
	}
	if s.machine == nil || s.ctx == nil {
		s.frame.Clear(0, 0, 0, 255)
		if s.blackHanded {
			return false, nil
		}
		s.blackHanded = true
		pixels := s.frame.Pixels()
		buf := unsafe.Slice((*byte)(dst), n)[:s.width*s.height*4]
		copy(buf, pixels)
		return true, nil
	}
	s.pump()
	s.composeFrame()
	if s.handedSerial == s.frameSerial {
		return false, nil
	}
	s.handedSerial = s.frameSerial
	pixels := s.frame.Pixels()
	buf := unsafe.Slice((*byte)(dst), n)[:s.width*s.height*4]
	copy(buf, pixels)
	return true, nil
}
