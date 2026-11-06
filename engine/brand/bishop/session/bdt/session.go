package bdt

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	bdtvm "kizuna/engine/brand/bishop/vm/bdt"
	"kizuna/engine/core"
)

const EntryLabel = "999-98"

const defaultRunBudget = 4096

const noAudioTarget = 0xFFFFFFFF

var errBdtNotOpen = errors.New("bdt: engine not open")

var errBdtNoScript = errors.New("bdt: scenario missing")

type Session struct {
	dir       string
	name      string
	extraDirs []string
	cacheDir  string

	resources *resource.GameResourceIndex
	variant   BdtVariant
	frame     *gfx.BgraCanvas
	images    *rendering.ImageCache
	gameState *state.GameState
	catalog   *BdtCatalog
	stage     *BdtStage
	audio     *BdtAudioCatalog
	saves     *BdtSaveStore
	machine   *bdtvm.BdtMachine
	mapScreen *BdtMapScreen

	label          string
	backgroundName string
	sceneID        string
	screenMode     int
	bgmTrack       int
	sprites        []BdtSpriteEntry
	missingFaces   map[string]bool
	lastErr        string
	disposed       bool

	// sys 是引擎内置的系统界面（标题下的鑑賞室/設定/続きから）。
	sys *bdtSystemUI
	// manin 是 2003 年《満淫電車》的内置系统状态（标题/設定/存读档/鑑賞室/
	// 言葉責め/地图/词库）。AFDVD 路线保持 nil，所有 manin 分支都不会进入，
	// 避免给已适配作品增加回归风险。
	manin *maninRuntime
	// namePrompt 非空时宿主弹出名字输入框（manin 的「名前変更」）。
	namePrompt string
	// sfxVolume/voiceVolume 供音環境設定使用；BGM 音量走 GameState。
	sfxVolume         float64
	voiceVolume       float64
	titleMode         bool
	titleSavedWindows [3]bool
	titleSavedMessage bool
	hostRequest       string
	hostRequestSerial uint32

	pending          []core.AudioEvent
	lastVoiceSerial  int
	lastVoice        string
	hasLastVoice     bool
	lastEffectSerial int
	lastEffect       string
	hasLastEffect    bool
	lastBgm          string
	hasLastBgm       bool
	lastBgmLoop      bool
}

func NewSession() core.Engine {
	return &Session{label: EntryLabel, missingFaces: map[string]bool{}}
}

func (s *Session) Open(dir string) (core.Info, error) {
	return s.OpenExtras(dir, nil, "")
}

func (s *Session) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	if strings.TrimSpace(dir) == "" {
		return core.Info{}, errBdtNotOpen
	}
	clean := filepath.Clean(dir)
	resources := resource.Build(clean, extraDirs...)
	variant := DetectBdtVariant(resources)
	frame := gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	images := rendering.NewImageCache(resources, rendering.DefaultImageBudgetBytes, rendering.DefaultImageCapacity)
	gameState := state.NewGameState()
	catalog := NewBdtCatalog(resources)
	stage := NewBdtStage(images.Get)
	stage.SetVariant(variant)
	saves := NewBdtSaveStore(bdtSaveDirectory(clean))
	name := filepath.Base(clean)
	s.label = EntryLabel
	s.backgroundName = ""
	s.sceneID = ""
	s.screenMode = 0
	s.bgmTrack = 0
	s.sprites = nil
	s.missingFaces = map[string]bool{}
	s.lastErr = ""
	s.disposed = false
	s.pending = nil
	s.lastVoiceSerial = 0
	s.lastVoice = ""
	s.hasLastVoice = false
	s.lastEffectSerial = 0
	s.lastEffect = ""
	s.hasLastEffect = false
	s.lastBgm = ""
	s.hasLastBgm = false
	s.lastBgmLoop = false
	s.sys = nil
	s.sfxVolume = 0.75
	s.voiceVolume = 0.75
	s.dir = clean
	s.name = name
	s.extraDirs = append([]string{}, extraDirs...)
	s.cacheDir = cacheDir
	s.resources = resources
	s.variant = variant
	s.frame = frame
	s.images = images
	s.gameState = gameState
	// BDT 没有 message.bsg，对话框由引擎用内嵌字体自绘；必须打开
	// ShowMessageFrame，宿主才会在 Frame 里看到文字并隐藏 Flutter 兜底框。
	gameState.SetShowMessageFrame(true)
	s.catalog = catalog
	s.stage = stage
	s.audio = NewBdtAudioCatalog(resources, !variant.UsesContainerArchive)
	s.saves = saves
	s.mapScreen = NewBdtMapScreen()
	s.machine = nil
	// manin 的全局设置/CG 解锁与词库不放在 scene00.bdt 里，打开时从存档目录
	// 的旁车文件恢复；AFDVD 路线保持 nil，后续分支全部跳过。
	s.manin = nil
	if variant.Name == ManinVariant.Name {
		s.manin = buildManinRuntime(s, saves.DirectoryPath(), name)
	}
	core.Logf("bdt: open '%s' dir='%s'", name, clean)
	file := catalog.LoadMain()
	if file == nil {
		core.Logf("bdt: %s", "scene00.bdt missing, session will fault")
		s.lastErr = "bdt: scene00.bdt missing"
		return s.info(), nil
	}
	machine := bdtvm.NewBdtMachine(file, s, catalog)
	if s.manin != nil {
		machine.SetDialect(bdtvm.BdtDialectManin)
	}
	machine.Start(EntryLabel)
	s.machine = machine
	core.Logf("bdt: started %s labels=%d", file.Name, len(file.Labels))
	return s.info(), nil
}

func (s *Session) info() core.Info {
	return core.Info{Width: ScreenWidth, Height: ScreenHeight, Title: s.name, SceneCount: 1, EntryCount: 1}
}

func (s *Session) Info() core.Info {
	return s.info()
}

func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil {
		return core.StepFaulted, errBdtNoScript
	}
	if s.hasLastBgm {
		s.pending = append(s.pending, core.AudioEvent{Kind: 3, Channel: noAudioTarget, Group: noAudioTarget})
	}
	if s.hasLastVoice {
		s.pending = append(s.pending, core.AudioEvent{Kind: 4, Channel: noAudioTarget, Group: noAudioTarget})
	}
	if s.hasLastEffect {
		s.pending = append(s.pending, core.AudioEvent{Kind: 5, Channel: noAudioTarget, Group: noAudioTarget})
	}
	s.resetManinRun()
	if s.manin != nil {
		s.maninApplySettings()
	}
	s.namePrompt = ""
	s.label = EntryLabel
	s.backgroundName = ""
	s.sceneID = ""
	s.screenMode = 0
	s.bgmTrack = 0
	s.sprites = nil
	s.stage.ClearSprites()
	s.stage.SetBackground("", 0)
	s.stage.SetFace("")
	s.stage.ShowMessage("", "")
	s.stage.SetScreenMode(0)
	for part := 0; part < 3; part++ {
		s.stage.SetWindow(part, false)
	}
	s.mapScreen.Deactivate()
	s.titleMode = false
	s.gameState.ResetForRestart()
	s.lastVoiceSerial = s.gameState.VoiceSerial()
	s.lastVoice = ""
	s.hasLastVoice = false
	s.lastEffectSerial = s.gameState.EffectSerial()
	s.lastEffect = ""
	s.hasLastEffect = false
	s.lastBgm = ""
	s.hasLastBgm = false
	s.machine.Start(EntryLabel)
	return core.StepContinue, nil
}

func (s *Session) Run(budget uint32) (core.Step, error) {
	steps := int(budget)
	if steps <= 0 {
		steps = defaultRunBudget
	}
	return s.runSteps(steps), nil
}

func (s *Session) runSteps(budget int) core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.machine == nil {
		return core.StepFaulted
	}
	// manin 的言葉責め逐句播放 / 片尾滚动不执行脚本，但每次 Run 都要推进
	// 一格；放在循环外，避免每执行一条脚本都多跑一次计时状态机。
	if step, handled := s.maninRunPending(); handled {
		return step
	}
	if s.maninScreenActive() {
		// 界面/言葉責め/地图期间脚本必须停住；宿主可能因为 StepWait 或
		// 自动模式继续 pump，不能让它把底下的剧本跑掉。
		return s.maninStep()
	}
	if s.titleMode {
		return core.StepAwaitTitle
	}
	for executed := 0; executed < budget; executed++ {
		if s.safeStep() {
			continue
		}
		step := s.dispatch()
		if step != core.StepContinue {
			return step
		}
	}
	return core.StepContinue
}

func (s *Session) safeStep() (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			core.Logf("bdt: step failed: %v", r)
			panicked = true
		}
	}()
	s.machine.Step()
	return false
}

func (s *Session) Advance() (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	if step, handled := s.maninRunPending(); handled {
		return step, nil
	}
	if s.bdtSystemActive() || s.titleMode {
		return s.currentStep(), nil
	}
	s.gameState.SetAwaitingInput(false)
	s.machine.Resume()
	return s.runSteps(1), nil
}

func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	if !s.gameState.AwaitingChoice() {
		return core.StepContinue, nil
	}
	accepted := s.machine.Choose(int(index))
	if !accepted {
		core.Logf("bdt: choice %d rejected", index)
		return core.StepAwaitChoice, nil
	}
	s.gameState.ClearChoices()
	s.gameState.SetAwaitingInput(false)
	return s.runSteps(defaultRunBudget), nil
}

func (s *Session) Pointer(op uint32, x, y int32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.machine == nil {
		return core.StepFaulted, nil
	}
	switch op {
	case 0:
		if s.manin != nil {
			if x >= 0 && y >= 0 {
				s.maninHover(int(x), int(y))
			}
			return s.currentStep(), nil
		}
		if x < 0 || y < 0 {
			s.gameState.SetHoveredButton(-1)
		} else {
			s.SetMapHover(int(x), int(y))
		}
		return s.currentStep(), nil
	case 4:
		// 右键：系统界面优先关闭；标题/剧情保持原行为。
		if s.maninScreenActive() {
			s.maninBack()
			if s.maninScreenActive() {
				return s.maninStep(), nil
			}
			if s.titleMode {
				return core.StepAwaitTitle, nil
			}
			return s.currentStep(), nil
		}
		if s.bdtSystemActive() {
			s.closeSystemUI()
			return core.StepAwaitTitle, nil
		}
		if s.manin != nil && !s.titleMode && !s.manin.messageHidden &&
			s.gameState.MessageWindowVisible() {
			// 原作右键消去消息框；下一次左键只负责重新显示，不推进脚本。
			s.manin.messageHidden = true
			return s.currentStep(), nil
		}
		return s.currentStep(), nil
	case 2:
		return s.currentStep(), nil
	case 1, 3:
		if s.maninScreenActive() {
			s.pointerManinUI(int(x), int(y))
			if s.maninScreenActive() {
				return s.maninStep(), nil
			}
			if s.titleMode {
				return core.StepAwaitTitle, nil
			}
			// 地图/读档/界面关闭后可能已经执行了脚本，必须返回真实步，
			// 否则宿主会多停一帧，把新对白挡住。
			return s.currentStep(), nil
		}
		if s.bdtSystemActive() {
			s.pointerSystemUI(int(x), int(y))
			if s.bdtSystemActive() || s.titleMode {
				return core.StepAwaitTitle, nil
			}
			// 读档成功会关闭界面并离开标题，必须把恢复后的真实步返回给宿主，
			// 否则播放页会多停留在标题一步，把刚恢复的对白挡住。
			return s.currentStep(), nil
		}
		if s.titleMode {
			if key := s.titleKeyAt(x, y); key != "" {
				return s.SelectTitleItem(key)
			}
			return core.StepAwaitTitle, nil
		}
		if s.mapScreen.Active() && s.machine.Stop() == bdtvm.BdtStopMap {
			return s.ChooseMapPixel(int(x), int(y)), nil
		}
		if s.manin != nil && s.manin.messageHidden {
			// 第一次点击只把对话框叫回来，不消耗这一句。
			s.manin.messageHidden = false
			return s.currentStep(), nil
		}
		// manin 的工具条（SAVE/LOAD/SKIP/AUTO/LOG/SYS）画在消息框顶边，
		// 必须先于"推进对白"消费，否则点按钮会顺带翻页。
		if s.manin != nil && s.maninToolbarClick(int(x), int(y)) {
			return s.currentStep(), nil
		}
		return s.Advance()
	case 5:
		if s.maninScreenActive() {
			s.maninBack()
			if s.maninScreenActive() {
				return s.maninStep(), nil
			}
			if s.titleMode {
				return core.StepAwaitTitle, nil
			}
			return s.currentStep(), nil
		}
		if s.bdtSystemActive() {
			s.closeSystemUI()
			return core.StepAwaitTitle, nil
		}
		return s.CancelMapSelect(), nil
	}
	return s.currentStep(), nil
}

func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if s.disposed {
		return false, nil
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				core.Logf("bdt: compose failed: %v", r)
			}
		}()
		showMessage := s.gameState.ShowMessageFrame()
		if s.manin != nil {
			if s.manin.messageHidden {
				showMessage = false
			}
			if s.manin.screen == maninScreenAttack && s.manin.attack.Phase != maninAttackExecute {
				// 选词阶段手机/词表占满屏幕，底下残留的上一句对白不该透出来。
				showMessage = false
			}
		}
		s.stage.ComposeTo(s.frame, showMessage)
		if s.manin != nil && !s.titleMode && !s.maninScreenActive() {
			s.drawManinSubwindow(s.frame)
		}
		if s.manin != nil && s.titleMode && !s.maninScreenActive() {
			s.drawManinTitle(s.frame)
		}
		if s.maninScreenActive() {
			s.drawManinUI(s.frame)
		} else if s.sys != nil && s.sys.kind != bdtSysNone {
			s.drawSystemUI(s.frame)
		}
	}()
	need := ScreenWidth * ScreenHeight * 4
	if dst == nil || n < need || len(s.frame.Buffer) < need {
		return false, nil
	}
	copy(unsafe.Slice((*byte)(dst), need), s.frame.Buffer[:need])
	return true, nil
}

func (s *Session) Scenes() []core.Scene {
	return []core.Scene{{ID: 0, Name: ScenarioName, Title: s.name}}
}

func (s *Session) Slots() []core.Slot {
	infos := s.saves.List()
	if s.manin != nil {
		// manin 的存档页是 3 页 ×19 行，需要 0..56 全部可见；AFDVD 仍用
		// BdtSaveStore.List() 的 24 槽默认值。
		infos = s.saves.ListCount(bdtSlotCount)
	}
	out := make([]core.Slot, 0, len(infos))
	for _, info := range infos {
		slot := core.Slot{Index: uint32(info.Index), Exists: info.Exists}
		if info.Exists {
			slot.Title = info.Summary
			slot.Summary = info.Summary
			if info.HasTime {
				slot.SavedAt = bdtTicksToTime(info.SavedAt).Local().Format(time.RFC3339)
			}
		}
		out = append(out, slot)
	}
	return out
}

func (s *Session) Save(slot uint32, title string) error {
	if s.disposed || s.machine == nil {
		return errBdtNotOpen
	}
	if s.titleMode {
		return fmt.Errorf("bdt: cannot save on title screen")
	}
	summary := title
	if strings.TrimSpace(summary) == "" {
		if s.manin != nil {
			summary = s.maninSummary()
		} else {
			summary = s.summarize()
		}
	}
	thumbnail := s.frame.ScaleTo(ThumbnailWidth, ThumbnailHeight)
	x := append([]int32{}, s.machine.X()...)
	z := append([]int32{}, s.machine.Z()...)
	st := &BdtSaveState{
		Label:           s.label,
		Machine:         s.machine.Capture(),
		X:               x,
		Z:               z,
		BackgroundName:  s.backgroundName,
		Sprites:         append([]BdtSpriteEntry{}, s.sprites...),
		BgmTrack:        s.bgmTrack,
		Speaker:         s.gameState.Speaker(),
		Text:            s.gameState.Text(),
		Summary:         summary,
		SavedAtTicks:    bdtTimeToTicks(time.Now().UTC()),
		Thumbnail:       append([]byte{}, thumbnail.Buffer...),
		ThumbnailWidth:  thumbnail.Width,
		ThumbnailHeight: thumbnail.Height,
	}
	if !s.saves.Write(st, int(slot)) {
		return fmt.Errorf("bdt: save slot %d failed", slot)
	}
	if s.manin != nil {
		maninEnsureDirs(s.maninSaveDir())
		saveManinSlotData(s.maninSaveDir(), int(slot), s.maninCaptureSlot())
		saveManinGlobal(s.maninSaveDir(), s.manin.global)
		s.manin.hasSave = true
	}
	core.Logf("bdt: save slot %d written", slot)
	return nil
}

func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.disposed || s.machine == nil {
		return core.StepFaulted, errBdtNotOpen
	}
	st := s.saves.Read(int(slot))
	if st == nil || st.Machine == nil {
		core.Logf("bdt: save slot %d missing", slot)
		return core.StepFaulted, fmt.Errorf("bdt: save slot %d missing", slot)
	}
	s.leaveTitle()
	s.machine.Restore(st.Machine)
	if s.manin != nil {
		s.maninRestoreSlot(int(slot))
		s.stage.SetMessageColor(0xffffffff)
	}
	s.label = st.Label
	s.bgmTrack = st.BgmTrack
	s.backgroundName = st.BackgroundName
	s.sprites = append([]BdtSpriteEntry{}, st.Sprites...)
	s.restoreLayers()
	s.gameState.SetSpeaker(st.Speaker)
	s.gameState.SetText(st.Text)
	s.gameState.SetMessageWindowVisible(true)
	s.gameState.SetAwaitingInput(false)
	s.gameState.ClearChoices()
	if s.bgmTrack > 0 {
		if name, ok := s.audio.ResolveBgm(s.bgmTrack); ok {
			s.gameState.SetBgm(name)
		} else {
			s.gameState.ClearBgm()
		}
	} else {
		s.gameState.ClearBgm()
	}
	s.gameState.SetBgmLoop(true)
	s.gameState.ClearSoundEffect()
	if st.Machine.Stop == bdtvm.BdtStopMap {
		if s.manin != nil {
			// manin 的地图不是 system.prm 驱动的 AFDVD 地图，直接重开
			// s_map1 界面；脚本停在 MAP 后面，选站后恢复即可。
			s.maninEnterScreen(maninScreenMap)
			s.gameState.BeginSelect()
		} else if s.mapScreen.Activate(s.resources) {
			s.gameState.BeginSelect()
		} else {
			core.Logf("bdt: %s", "map rearm failed")
		}
	} else {
		s.mapScreen.Deactivate()
		s.gameState.EndSelect()
	}
	s.lastVoiceSerial = s.gameState.VoiceSerial()
	s.lastEffectSerial = s.gameState.EffectSerial()
	core.Logf("bdt: save slot %d loaded label '%s'", slot, st.Label)
	return s.currentStep(), nil
}

func (s *Session) AssetSize(path string) int {
	if s.resources == nil {
		return 0
	}
	location, ok := s.resources.TryGet(path)
	if !ok {
		return 0
	}
	size := location.Size()
	if size < 0 {
		return 0
	}
	if size > int64(maxInt32) {
		return maxInt32
	}
	return int(size)
}

func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if dst == nil || n <= 0 || s.resources == nil {
		return 0
	}
	location, ok := s.resources.TryGet(path)
	if !ok {
		return 0
	}
	data := location.Read()
	if len(data) > n {
		data = data[:n]
	}
	return copy(unsafe.Slice((*byte)(dst), n)[:len(data)], data)
}

func (s *Session) Audio() []core.AudioEvent {
	var out []core.AudioEvent
	if len(s.pending) > 0 {
		out = append(out, s.pending...)
		s.pending = nil
	}
	if s.gameState == nil {
		return out
	}
	if serial := s.gameState.VoiceSerial(); serial != s.lastVoiceSerial {
		s.lastVoiceSerial = serial
		if name, ok := s.gameState.Voice(); ok {
			s.lastVoice = name
			s.hasLastVoice = true
			out = append(out, core.AudioEvent{Kind: 2, Path: s.materialize(name), Volume: s.voiceVolume, Channel: noAudioTarget, Group: noAudioTarget})
		} else {
			s.lastVoice = ""
			s.hasLastVoice = false
			out = append(out, core.AudioEvent{Kind: 4, Channel: noAudioTarget, Group: noAudioTarget})
		}
	}
	if serial := s.gameState.EffectSerial(); serial != s.lastEffectSerial {
		s.lastEffectSerial = serial
		if name, ok := s.gameState.SoundEffect(); ok {
			s.lastEffect = name
			s.hasLastEffect = true
			out = append(out, core.AudioEvent{Kind: 1, Path: s.materialize(name), Volume: s.sfxVolume, Channel: noAudioTarget, Group: noAudioTarget})
		} else {
			s.lastEffect = ""
			s.hasLastEffect = false
			out = append(out, core.AudioEvent{Kind: 5, Channel: noAudioTarget, Group: noAudioTarget})
		}
	}
	if name, ok := s.gameState.Bgm(); ok {
		loop := s.gameState.BgmLoop()
		if !s.hasLastBgm || s.lastBgm != name || s.lastBgmLoop != loop {
			s.lastBgm = name
			s.hasLastBgm = true
			s.lastBgmLoop = loop
			out = append(out, core.AudioEvent{Kind: 0, Path: s.materialize(name), Volume: s.gameState.BgmVolume(), Loop: loop, Channel: noAudioTarget, Group: noAudioTarget})
		}
	} else if s.hasLastBgm {
		s.lastBgm = ""
		s.hasLastBgm = false
		out = append(out, core.AudioEvent{Kind: 3, Channel: noAudioTarget, Group: noAudioTarget})
	}
	return out
}

func (s *Session) materialize(name string) string {
	if s.resources == nil {
		return name
	}
	if path := audio.Materialize(name, s.resources, s.cacheDir); path != "" {
		return path
	}
	return name
}

func (s *Session) Text() core.TextState {
	awaiting := uint32(0)
	if s.gameState != nil && s.gameState.AwaitingInput() {
		awaiting = 1
	}
	if s.gameState == nil {
		return core.TextState{}
	}
	return core.TextState{Speaker: s.gameState.Speaker(), Text: s.gameState.Text(), Awaiting: awaiting}
}

func (s *Session) Choices() []core.Choice {
	if s.gameState == nil {
		return nil
	}
	texts := s.gameState.Choices()
	if len(texts) == 0 {
		return nil
	}
	out := make([]core.Choice, 0, len(texts))
	for i, text := range texts {
		out = append(out, core.Choice{Name: text, Value: int32(i)})
	}
	return out
}

// messageWindowVisible 表示这一帧的对话框已经由 BDT 引擎画进帧里。
// 只有停在稳定对白（非标题、有正文）时才算，宿主据此隐藏 Flutter 的 MessageBox。
func (s *Session) messageWindowVisible() bool {
	if s.gameState == nil || s.titleMode {
		return false
	}
	// manin 的 SYSTEM/存读档/言葉責め/整理/LOG 都是引擎全屏绘制，宿主不能再
	// 叠一层 Flutter 对话框；这里统一报 true 让宿主隐藏自己的 MessageBox。
	if s.maninScreenActive() {
		return true
	}
	return s.gameState.ShowMessageFrame() &&
		s.gameState.MessageWindowVisible() &&
		len(s.gameState.Text()) > 0
}

func (s *Session) State() core.State {
	if s.machine == nil || s.gameState == nil {
		return core.State{}
	}
	bgm := ""
	if name, ok := s.gameState.Bgm(); ok {
		bgm = name
	}
	pc := s.machine.ProgramCounter()
	if pc < 0 {
		pc = 0
	}
	start := s.machine.InstructionStart()
	if start < 0 {
		start = 0
	}
	wait := s.gameState.WaitMilliseconds()
	if wait < 0 {
		wait = 0
	}
	return core.State{
		Bgm:                  bgm,
		PC:                   uint32(pc),
		Scene:                0,
		Animating:            false,
		WaitMs:               uint32(wait),
		MessageWindowVisible: s.gameState.MessageWindowVisible(),
		EngineMessageWindow:  s.messageWindowVisible(),
		ReplayActive:         false,
		InstructionStart:     uint32(start),
		HostRequest:          s.hostRequest,
		HostRequestSerial:    s.hostRequestSerial,
		AwaitingName:         s.namePrompt != "",
		NamePrompt:           s.namePrompt,
	}
}

func (s *Session) TextList() []core.TextUnit {
	return nil
}

func (s *Session) LastError() string {
	return s.lastErr
}

func (s *Session) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true
	if s.resources != nil {
		s.resources.Dispose()
	}
	return nil
}

func (s *Session) MapButtons() []core.MapButton {
	if s.disposed || s.mapScreen == nil || !s.mapScreen.Active() {
		return nil
	}
	hotspots := s.mapScreen.Hotspots()
	out := make([]core.MapButton, 0, len(hotspots))
	for _, hotspot := range hotspots {
		out = append(out, core.MapButton{
			Index:  uint32(hotspot.ID - 1),
			X:      int32(hotspot.X),
			Y:      int32(hotspot.Y),
			Width:  uint32(hotspot.Width),
			Height: uint32(hotspot.Height),
			Value:  int32(hotspot.ID),
		})
	}
	return out
}

func (s *Session) MapMask() (bool, int, int) {
	if s.disposed || s.mapScreen == nil || !s.mapScreen.Active() || !s.mapScreen.HasMask() {
		return false, 0, 0
	}
	return true, s.mapScreen.MaskWidth(), s.mapScreen.MaskHeight()
}

func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	st := s.saves.Read(int(slot))
	if st == nil {
		return core.SlotPreview{}, false
	}
	preview := core.SlotPreview{
		SavedAt: bdtTicksToTime(st.SavedAtTicks).Local().Format(time.RFC3339),
		Summary: st.Summary,
		Speaker: st.Speaker,
		Text:    st.Text,
	}
	if st.ThumbnailWidth > 0 && st.ThumbnailHeight > 0 && len(st.Thumbnail) == st.ThumbnailWidth*st.ThumbnailHeight*4 {
		preview.Thumb = core.Frame{Width: st.ThumbnailWidth, Height: st.ThumbnailHeight, Pixels: st.Thumbnail}
	}
	return preview, true
}

func (s *Session) ChooseMapPixel(x, y int) core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.machine == nil {
		return core.StepFaulted
	}
	if !s.mapScreen.Active() || s.machine.Stop() != bdtvm.BdtStopMap {
		return core.StepContinue
	}
	hotspot := s.mapScreen.HotspotAt(x, y)
	if hotspot <= 0 {
		return core.StepAwaitSelect
	}
	return s.resolveMapHotspot(hotspot)
}

func (s *Session) CancelMapSelect() core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.mapScreen == nil || !s.mapScreen.Active() {
		return core.StepContinue
	}
	return core.StepAwaitSelect
}

func (s *Session) SetHoveredButton(index int) {
	if s.disposed || s.gameState == nil {
		return
	}
	s.gameState.SetHoveredButton(index)
}

func (s *Session) SetMapHover(x, y int) {
	if s.disposed || s.mapScreen == nil || !s.mapScreen.Active() || !s.mapScreen.HasMask() {
		return
	}
	hotspot := s.mapScreen.HotspotAt(x, y)
	if hotspot <= 0 {
		s.gameState.SetHoveredButton(-1)
	} else {
		s.gameState.SetHoveredButton(hotspot - 1)
	}
}

func (s *Session) ChooseButton(index int) core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.machine == nil {
		return core.StepFaulted
	}
	if !s.mapScreen.Active() || s.machine.Stop() != bdtvm.BdtStopMap {
		return core.StepContinue
	}
	hotspot := index + 1
	if hotspot < 1 || hotspot > BdtMapHotspotCount {
		return core.StepAwaitSelect
	}
	return s.resolveMapHotspot(hotspot)
}

func (s *Session) resolveMapHotspot(hotspot int) core.Step {
	label := s.mapScreen.ResolveHotspot(hotspot, s.mapVariables())
	if strings.TrimSpace(label) == "" {
		core.Logf("bdt: map hotspot %d unresolved, resuming", hotspot)
		s.mapScreen.Deactivate()
		s.gameState.EndSelect()
		s.machine.CloseMap()
		return s.runSteps(defaultRunBudget)
	}
	if !s.machine.JumpToLabel(label) {
		core.Logf("bdt: map jump rejected: %s", label)
		return core.StepAwaitSelect
	}
	s.label = label
	s.mapScreen.Deactivate()
	s.gameState.EndSelect()
	core.Logf("bdt: map hotspot %d -> %s", hotspot, label)
	return s.runSteps(defaultRunBudget)
}

func (s *Session) mapVariables() []int {
	raw := s.machine.X()
	out := make([]int, len(raw))
	for i, value := range raw {
		out[i] = int(value)
	}
	return out
}

func (s *Session) dispatch() core.Step {
	if s.machine == nil {
		return core.StepFaulted
	}
	if s.machine.Faulted() {
		s.lastErr = fmt.Sprintf("bdt: fault at %d", s.machine.InstructionStart())
		core.Logf("bdt: fault at %d", s.machine.InstructionStart())
		return core.StepFaulted
	}
	var step core.Step
	func() {
		defer func() {
			if r := recover(); r != nil {
				core.Logf("bdt: dispatch failed: %v", r)
				step = core.StepContinue
			}
		}()
		step = s.applyStop()
	}()
	return step
}

func (s *Session) applyStop() core.Step {
	switch s.machine.Stop() {
	case bdtvm.BdtStopNone:
		return core.StepContinue
	case bdtvm.BdtStopWait:
		s.gameState.SetWaitMilliseconds(s.machine.WaitMilliseconds())
		return core.StepWait
	case bdtvm.BdtStopText, bdtvm.BdtStopInput:
		s.gameState.SetAwaitingInput(true)
		return core.StepAwaitInput
	case bdtvm.BdtStopChoice:
		return s.applyChoice()
	case bdtvm.BdtStopMap:
		return s.applyMap()
	case bdtvm.BdtStopTitle:
		return s.beginTitle()
	case bdtvm.BdtStopAttack, bdtvm.BdtStopOrganize, bdtvm.BdtStopStaffRoll:
		// 这三种 stop 只能由 manin 方言的 XA/XR/SROL 触发；界面已经由
		// StartManin* 打开，这里只需把当前界面状态翻译成 Step。界面若已被
		// 关闭（读档/右键），必须清掉 stop 否则会永远卡住。
		if s.manin != nil && s.maninScreenActive() {
			return s.maninStep()
		}
		s.machine.CloseManinStop()
		return core.StepContinue
	case bdtvm.BdtStopScreen:
		s.machine.Resume()
		return core.StepContinue
	case bdtvm.BdtStopHalt:
		return core.StepFinished
	}
	return core.StepContinue
}

func (s *Session) applyChoice() core.Step {
	options := s.machine.Choices()
	if len(options) == 0 {
		core.Logf("bdt: %s", "choice stop with no options, continuing")
		return core.StepContinue
	}
	s.maninAutoSave()
	texts := make([]string, 0, len(options))
	for _, option := range options {
		texts = append(texts, CleanBdtText(option.Text))
	}
	s.gameState.SetChoices(texts)
	return core.StepAwaitChoice
}

func (s *Session) applyMap() core.Step {
	if s.manin != nil {
		// manin 的地图是 s_map1 + s_map3 命中色块，选站写 X31 后脚本自己
		// J31 分派；不需要 AFDVD 的 system.prm 表。进地图前自动存档，
		// 对应原作的「マップ画面直前」自动保存。
		s.maninAutoSave()
		s.maninEnterScreen(maninScreenMap)
		s.gameState.BeginSelect()
		return core.StepAwaitSelect
	}
	if s.mapScreen.Active() {
		s.gameState.BeginSelect()
		return core.StepAwaitSelect
	}
	if !s.mapScreen.Activate(s.resources) {
		core.Logf("bdt: %s", "map unavailable, resuming without select")
		s.mapScreen.Deactivate()
		s.gameState.EndSelect()
		s.machine.CloseMap()
		return core.StepContinue
	}
	s.gameState.BeginSelect()
	core.Logf("bdt: %s", "map awaiting select")
	return core.StepAwaitSelect
}

func (s *Session) currentStep() core.Step {
	if s.disposed {
		return core.StepFinished
	}
	if s.machine == nil {
		return core.StepFaulted
	}
	if s.machine.Faulted() {
		return core.StepFaulted
	}
	if s.maninScreenActive() {
		return s.maninStep()
	}
	switch s.machine.Stop() {
	case bdtvm.BdtStopWait:
		return core.StepWait
	case bdtvm.BdtStopText, bdtvm.BdtStopInput:
		return core.StepAwaitInput
	case bdtvm.BdtStopChoice:
		return core.StepAwaitChoice
	case bdtvm.BdtStopMap:
		return core.StepAwaitSelect
	case bdtvm.BdtStopTitle:
		return core.StepAwaitTitle
	case bdtvm.BdtStopHalt:
		return core.StepFinished
	}
	return core.StepContinue
}

func (s *Session) SetBackground(name string, effectCode int) {
	s.backgroundName = name
	s.stage.SetBackground(name, effectCode)
	s.maninNoteBackground(name)
}

func (s *Session) SetSprite(slot int, name string, x, y, mode int) {
	entry := BdtSpriteEntry{Slot: slot, Name: name, X: x, Y: y, Mode: mode}
	found := false
	for i, candidate := range s.sprites {
		if candidate.Slot == slot {
			s.sprites[i] = entry
			found = true
			break
		}
	}
	if !found {
		s.sprites = append(s.sprites, entry)
	}
	s.stage.SetSprite(slot, name, x, y, mode)
}

func (s *Session) ClearSprite(slot int) {
	kept := s.sprites[:0]
	for _, candidate := range s.sprites {
		if candidate.Slot != slot {
			kept = append(kept, candidate)
		}
	}
	s.sprites = kept
	s.stage.ClearSprite(slot)
}

func (s *Session) ClearSprites() {
	s.sprites = nil
	s.stage.ClearSprites()
}

func (s *Session) SetWindow(part int, visible bool) {
	s.stage.SetWindow(part, visible)
}

func (s *Session) ShowMessage(speaker, text, voiceCode string) {
	expanded, hasWord := s.expandManinMessage(text)
	s.gameState.SetSpeaker(CleanBdtText(speaker))
	s.gameState.SetText(CleanBdtText(expanded))
	s.gameState.SetMessageWindowVisible(true)
	s.stage.ShowMessage(s.gameState.Speaker(), s.gameState.Text())
	if s.manin != nil {
		// 原作把含責め言葉的台词显示成粉色；其他台词必须恢复白色，否则
		// 上一句的粉色会带到所有后续对白。
		s.manin.messageHidden = false
		if hasWord {
			s.stage.SetMessageColor(0xffffb0c8)
		} else {
			s.stage.SetMessageColor(0xffffffff)
		}
		s.maninLogMessage(s.gameState.Speaker(), s.gameState.Text())
	}
	if len(strings.TrimSpace(voiceCode)) > 0 {
		s.PlayVoice(voiceCode)
	}
}

func (s *Session) ShowFace(name string) {
	face := strings.TrimSpace(name)
	if s.manin != nil {
		s.manin.face = face
	}
	if len(face) == 0 {
		s.stage.SetFace("")
		return
	}
	if s.images.Get(face) == nil {
		s.noteMissingFace(face)
		s.stage.SetFace("")
		return
	}
	s.stage.SetFace(face)
}

func (s *Session) noteMissingFace(face string) {
	if !s.missingFaces[face] {
		s.missingFaces[face] = true
		core.Logf("bdt: face '%s' not found, skipped", face)
	}
}

func (s *Session) PlayVoice(code string) {
	if len(strings.TrimSpace(code)) == 0 {
		core.Logf("bdt: voice '%s' missing", code)
		return
	}
	if s.manin != nil && !s.maninVoiceAllowed(code) {
		return
	}
	path, ok := s.audio.ResolveVoice(code)
	if !ok {
		core.Logf("bdt: voice '%s' missing", code)
		return
	}
	s.gameState.SetVoice(path)
	s.gameState.SetVoiceSerial(s.gameState.VoiceSerial() + 1)
}

// maninVoiceAllowed 实现 SYSTEM 里的「個別音声設定」：V01xxxx = 遥香、
// V02 = 柚…V06 = 千尋。关闭某个角色只影响她的语音，不影响 SE/BGM。
func (s *Session) maninVoiceAllowed(code string) bool {
	if s.manin == nil {
		return true
	}
	if !s.manin.sys.VoiceOn {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(code))
	if len(lower) < 3 || lower[0] != 'v' {
		return true
	}
	// 语音编号形如 V0101001：V + 两位角色号 + 场景/句号。角色号是 01..06。
	index := BdtAtoi(lower[1:3])
	if index < 1 || index > 6 {
		return true
	}
	return s.manin.sys.VoiceByGirl[index-1]
}

func (s *Session) SetBgm(track, mode int) {
	if track <= 0 {
		s.bgmTrack = 0
		s.gameState.ClearBgm()
		core.Logf("bdt: %s", "bgm stop")
		return
	}
	s.bgmTrack = track
	path, ok := s.audio.ResolveBgm(track)
	if ok {
		s.gameState.SetBgm(path)
	} else {
		s.gameState.ClearBgm()
	}
	s.gameState.SetBgmLoop(mode != 1)
	if !ok {
		core.Logf("bdt: bgm track %d no audio file", track)
	} else {
		core.Logf("bdt: bgm track %d '%s'", track, path)
	}
}

func (s *Session) StopBgm() {
	s.bgmTrack = 0
	s.gameState.ClearBgm()
}

func (s *Session) PlaySe(name string) {
	if len(strings.TrimSpace(name)) == 0 {
		core.Logf("bdt: se '%s' missing, skipped", name)
		return
	}
	path, ok := s.audio.ResolveSe(name)
	if !ok {
		core.Logf("bdt: se '%s' missing, skipped", name)
		return
	}
	s.gameState.SetSoundEffect(path)
	s.gameState.SetEffectSerial(s.gameState.EffectSerial() + 1)
	core.Logf("bdt: se '%s'", path)
}

func (s *Session) SetEffect(effectCode int) {
	core.Logf("bdt: effect %d ignored", effectCode)
}

func (s *Session) SetSceneId(sceneId string) {
	s.sceneID = sceneId
}

func (s *Session) SetScreenMode(mode int) {
	s.screenMode = mode
	s.stage.SetScreenMode(mode)
}

func (s *Session) Redraw() {
	s.stage.Redraw()
}

func (s *Session) restoreLayers() {
	s.stage.ClearSprites()
	for _, entry := range s.sprites {
		s.stage.SetSprite(entry.Slot, entry.Name, entry.X, entry.Y, entry.Mode)
	}
	if len(s.backgroundName) > 0 {
		s.stage.SetBackground(s.backgroundName, 0)
	}
}

func (s *Session) summarize() string {
	text := strings.ReplaceAll(s.gameState.Text(), "\n", " ")
	text = strings.TrimSpace(text)
	if len(s.gameState.Speaker()) > 0 {
		text = s.gameState.Speaker() + ": " + text
	}
	runes := []rune(text)
	if len(runes) <= 42 {
		return text
	}
	return string(runes[:42])
}

const dotNetTicksOffset = int64(621355968000000000)

func bdtTimeToTicks(t time.Time) int64 {
	utc := t.UTC()
	secs := utc.Unix()
	nanos := int64(utc.Nanosecond())
	return secs*10000000 + nanos/100 + dotNetTicksOffset
}

func bdtTicksToTime(ticks int64) time.Time {
	relative := ticks - dotNetTicksOffset
	secs := relative / 10000000
	nanos := (relative % 10000000) * 100
	return time.Unix(secs, nanos).UTC()
}
