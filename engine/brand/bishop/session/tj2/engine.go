package tj2

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"kizuna/engine/brand/bishop/audio"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

const tj2NoChannel = ^uint32(0)

type Engine struct {
	session *Tj2Session
	info    core.Info
	step    core.Step
	lastErr string

	hasLastBgm       bool
	lastBgm          string
	lastBgmLoop      bool
	lastEffectSerial int
	lastVoiceSerial  int
}

var (
	_ core.Engine           = (*Engine)(nil)
	_ core.OpenerExtras     = (*Engine)(nil)
	_ core.SlotPreviewer    = (*Engine)(nil)
	_ core.MapExposer       = (*Engine)(nil)
	_ core.Closer           = (*Engine)(nil)
	_ core.TitleMenuExposer = (*Engine)(nil)
)

// TitleMenu / SelectTitleItem 把旧版 GK 的标题菜单接到宿主统一的主界面协议。
//
// 现代 bishop 的标题是配置驱动的；TJ2 经典分支的标题在脚本自身（func16），
// 但宿主看到的仍然是同一组 TitleMenuItem，因此不需要为 GK 单独开一条 UI 通道。
func (e *Engine) TitleMenu() []core.TitleMenuItem {
	session, ok := e.ready()
	if !ok {
		return nil
	}
	return session.TitleMenu()
}

func (e *Engine) SelectTitleItem(key string) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	step := session.SelectTitleItem(key)
	e.syncAudioBaseline()
	return e.store(step), nil
}

func (e *Engine) Open(dir string) (core.Info, error) {
	return e.OpenExtras(dir, nil, "")
}

func (e *Engine) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	dataDir := resolveTj2DataDir(dir)
	name := filepath.Base(dir)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = dataDir
	}
	merged := append([]string(nil), extraDirs...)
	session := NewTj2Session(name, dir, dataDir, merged, cacheDir)
	e.session = session
	e.info = core.Info{
		Width:   ScreenWidth,
		Height:  ScreenHeight,
		Title:   name,
		Version: "",
	}
	e.step = core.StepContinue
	e.hasLastBgm = false
	e.lastBgm = ""
	e.lastBgmLoop = false
	e.lastEffectSerial = session.gameState.EffectSerial()
	e.lastVoiceSerial = session.gameState.VoiceSerial()
	if session.table == nil || session.machine == nil {
		e.lastErr = "tj2: SCNDAT.TBL missing or machine start failed"
	}
	return e.info, nil
}

func (e *Engine) Info() core.Info {
	return e.info
}

func (e *Engine) Start(entry uint32) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	if entry != 0 {
		return core.StepFinished, nil
	}
	// 只有 Toku2 家族由 EXE 标题接管；GK 经典作品的标题在脚本 func16 里，
	// 不能在这里抢跑，否则会跳过脚本自己的标题画面。
	if session.IsToku2UI() {
		return e.store(session.EnterTitle()), nil
	}
	session.Restart()
	return e.run(session, tj2RunBudget)
}

func (e *Engine) Run(budget uint32) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	return e.run(session, int(budget))
}

func (e *Engine) Advance() (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	return e.store(session.Advance()), nil
}

func (e *Engine) Choose(index uint32) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	return e.store(session.Choose(int(index))), nil
}

func (e *Engine) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	// 经典分支的标题、信息框按钮、存读档/鉴赏界面都由引擎自己命中。
	// 这些界面返回 AwaitSelect/AwaitTitle，脚本不会因为一次点击被推进两次。
	if step, handled := session.classicPointer(op, int(x), int(y)); handled {
		e.syncAudioBaseline()
		// 悬停只改引擎内部高亮，不能把宿主手里的 step 改成别的
		// （例如 AwaitChoice 被一次鼠标移动改成 AwaitInput）。
		if step == state.SessionContinue || op == 0 {
			return e.step, nil
		}
		return e.store(step), nil
	}
	switch op {
	case 0:
		if x == -1 && y == -1 {
			session.SetHoveredButton(-1)
			session.SetMapHover(-1, -1)
			return e.step, nil
		}
		if session.IsClassic() && session.ClassicMap().Active() {
			session.SetHoveredButton(session.ClassicMap().RowAt(int(x), int(y)))
			return e.step, nil
		}
		session.SetMapHover(int(x), int(y))
		return e.step, nil
	case 1, 3:
		if session.Map().Active() && !session.Map().Fallback() {
			return e.store(session.ChooseMapPixel(int(x), int(y))), nil
		}
		if session.IsClassic() && session.ClassicMap().Active() && session.gameState.AwaitingChoice() {
			row := session.ClassicMap().RowAt(int(x), int(y))
			if row >= 0 && row < len(session.gameState.Choices()) {
				return e.store(session.ChooseButton(row)), nil
			}
		}
		return e.store(session.Advance()), nil
	case 5:
		return core.StepContinue, nil
	default:
		return e.step, nil
	}
}

func (e *Engine) Compose(dst unsafe.Pointer, n int) (bool, error) {
	session, ok := e.ready()
	if !ok {
		return false, nil
	}
	dirty := session.Compose()
	if dst == nil || n <= 0 {
		return dirty, nil
	}
	frame := session.frame.Buffer
	if len(frame) > n {
		return false, nil
	}
	buf := unsafe.Slice((*byte)(dst), len(frame))
	copy(buf, frame)
	return dirty, nil
}

func (e *Engine) Scenes() []core.Scene {
	return nil
}

func (e *Engine) Slots() []core.Slot {
	session, ok := e.ready()
	if !ok {
		return nil
	}
	infos := session.ListSlots()
	slots := make([]core.Slot, 0, len(infos))
	for _, info := range infos {
		savedAt := ""
		if info.Exists {
			savedAt = info.SavedAt.Local().Format(time.RFC3339)
		}
		slots = append(slots, core.Slot{
			Index:   uint32(info.Index),
			Title:   info.Summary,
			SavedAt: savedAt,
			Summary: info.Summary,
			Exists:  info.Exists,
		})
	}
	return slots
}

func (e *Engine) Save(slot uint32, title string) error {
	session, ok := e.ready()
	if !ok {
		return errors.New("tj2: engine not open")
	}
	if !session.SaveTo(int(slot)) {
		e.lastErr = "tj2: save failed"
		return errors.New("tj2: save failed")
	}
	return nil
}

func (e *Engine) Load(slot uint32) (core.Step, error) {
	session, ok := e.ready()
	if !ok {
		return core.StepFaulted, errors.New("tj2: engine not open")
	}
	if !session.LoadFrom(int(slot)) {
		e.lastErr = "tj2: load failed"
		return core.StepFaulted, errors.New("tj2: load failed")
	}
	e.syncAudioBaseline()
	return e.store(state.SessionAwaitInput), nil
}

func (e *Engine) AssetSize(path string) int {
	session, ok := e.ready()
	if !ok {
		return 0
	}
	location, found := session.resources.TryGet(path)
	if !found {
		return 0
	}
	size := location.Size()
	if size < 0 {
		return 0
	}
	return int(size)
}

func (e *Engine) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	session, ok := e.ready()
	if !ok || dst == nil || n <= 0 {
		return 0
	}
	location, found := session.resources.TryGet(path)
	if !found {
		return 0
	}
	data := location.Read()
	if len(data) > n {
		data = data[:n]
	}
	buf := unsafe.Slice((*byte)(dst), len(data))
	copy(buf, data)
	return len(data)
}

func (e *Engine) Audio() []core.AudioEvent {
	session, ok := e.ready()
	if !ok {
		return nil
	}
	st := session.gameState
	events := []core.AudioEvent{}
	if bgm, has := st.Bgm(); !has {
		if e.hasLastBgm {
			e.hasLastBgm = false
			e.lastBgm = ""
			events = append(events, core.AudioEvent{Kind: 3, Channel: tj2NoChannel, Group: tj2NoChannel})
		}
	} else {
		loop := st.BgmLoop()
		if !e.hasLastBgm || e.lastBgm != bgm || e.lastBgmLoop != loop {
			e.hasLastBgm = true
			e.lastBgm = bgm
			e.lastBgmLoop = loop
			path := audio.Materialize(bgm, session.resources, session.cacheDir)
			if path == "" {
				path = bgm
			}
			events = append(events, core.AudioEvent{Kind: 0, Path: path, Volume: st.BgmVolume(), Loop: loop, Channel: tj2NoChannel, Group: tj2NoChannel})
		}
	}
	if serial := st.EffectSerial(); serial != e.lastEffectSerial {
		e.lastEffectSerial = serial
		if name, has := st.SoundEffect(); has {
			path := audio.Materialize(name, session.resources, session.cacheDir)
			if path == "" {
				path = name
			}
			events = append(events, core.AudioEvent{Kind: 1, Path: path, Volume: 1.0, Channel: tj2NoChannel, Group: tj2NoChannel})
		}
	}
	if serial := st.VoiceSerial(); serial != e.lastVoiceSerial {
		e.lastVoiceSerial = serial
		if name, has := st.Voice(); has {
			path := audio.Materialize(name, session.resources, session.cacheDir)
			if path == "" {
				path = name
			}
			events = append(events, core.AudioEvent{Kind: 2, Path: path, Volume: 1.0, Channel: tj2NoChannel, Group: tj2NoChannel})
		}
	}
	if len(events) == 0 {
		return nil
	}
	return events
}

func (e *Engine) Text() core.TextState {
	session, ok := e.ready()
	if !ok {
		return core.TextState{}
	}
	awaiting := uint32(0)
	if session.gameState.AwaitingInput() || session.gameState.AwaitingChoice() {
		awaiting = 1
	}
	return core.TextState{
		Speaker:  session.gameState.Speaker(),
		Text:     session.gameState.Text(),
		Awaiting: awaiting,
	}
}

func (e *Engine) Choices() []core.Choice {
	session, ok := e.ready()
	if !ok {
		return nil
	}
	names := session.gameState.Choices()
	choices := make([]core.Choice, 0, len(names))
	for i, name := range names {
		choices = append(choices, core.Choice{Name: name, Value: int32(i)})
	}
	return choices
}

func (e *Engine) State() core.State {
	session, ok := e.ready()
	if !ok {
		return core.State{}
	}
	bgm, _ := session.gameState.Bgm()
	return core.State{
		Bgm:                  bgm,
		PC:                   uint32(session.ProgramCounter()),
		Scene:                uint32(session.scenarioNumber()),
		Animating:            session.fxAnimating(),
		WaitMs:               uint32(session.gameState.WaitMilliseconds()),
		MessageWindowVisible: session.messageWindowVisibleForHost(),
		// 自绘界面的作品由引擎把信息框画进帧里；宿主必须隐藏 Flutter
		// MessageBox，否则黑底圆角框会盖在游戏自己的窗框上。
		EngineMessageWindow: session.UsesEngineUI() && session.messageWindowVisibleForHost(),
		AwaitingName:        false,
		// 配置面板里的「既読もスキップ」直接影响宿主的自动推进策略。
		SkipUnread:       session.SkipUnread(),
		BackLogActive:    false,
		ReplayActive:     false,
		InstructionStart: uint32(session.InstructionStart()),
	}
}

func (e *Engine) TextList() []core.TextUnit {
	return nil
}

func (e *Engine) LastError() string {
	return e.lastErr
}

func (e *Engine) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	session, ok := e.ready()
	if !ok {
		return core.SlotPreview{}, false
	}
	preview, found := session.PreviewSlot(int(slot))
	if !found {
		return core.SlotPreview{}, false
	}
	out := core.SlotPreview{
		SavedAt: preview.SavedAt.Local().Format(time.RFC3339),
		Summary: preview.Summary,
		Speaker: preview.Speaker,
		Text:    preview.Text,
	}
	if len(preview.Thumbnail) > 0 && preview.ThumbnailWidth > 0 && preview.ThumbnailHeight > 0 &&
		len(preview.Thumbnail) == preview.ThumbnailWidth*preview.ThumbnailHeight*4 {
		pixels := append([]byte(nil), preview.Thumbnail...)
		out.Thumb = core.Frame{Width: preview.ThumbnailWidth, Height: preview.ThumbnailHeight, Pixels: pixels}
	}
	return out, true
}

func (e *Engine) MapButtons() []core.MapButton {
	session, ok := e.ready()
	if !ok {
		return nil
	}
	buttons := session.MapButtons()
	out := make([]core.MapButton, 0, len(buttons))
	for _, button := range buttons {
		out = append(out, core.MapButton{
			Index:  uint32(button.Index),
			Image:  button.Image,
			X:      int32(button.X),
			Y:      int32(button.Y),
			Width:  uint32(button.Width),
			Height: uint32(button.Height),
			Value:  int32(button.Value),
			Label:  button.Label,
		})
	}
	return out
}

func (e *Engine) MapMask() (bool, int, int) {
	session, ok := e.ready()
	if !ok {
		return false, 0, 0
	}
	if !session.HasMapMask() {
		return false, 0, 0
	}
	return true, session.MapMaskWidth(), session.MapMaskHeight()
}

func (e *Engine) Close() error {
	if e.session != nil {
		e.session.Dispose()
	}
	return nil
}

func (e *Engine) ready() (*Tj2Session, bool) {
	if e.session == nil || e.session.disposed {
		return nil, false
	}
	return e.session, true
}

func (e *Engine) run(session *Tj2Session, budget int) (core.Step, error) {
	if budget < 0 {
		budget = 0
	}
	return e.store(session.Run(budget)), nil
}

func (e *Engine) store(step state.SessionStep) core.Step {
	out := tj2StepToCore(step)
	e.step = out
	if out == core.StepFaulted {
		e.lastErr = "tj2: session faulted"
	}
	return out
}

func (e *Engine) syncAudioBaseline() {
	if e.session == nil {
		return
	}
	st := e.session.gameState
	if bgm, has := st.Bgm(); has {
		e.hasLastBgm = true
		e.lastBgm = bgm
		e.lastBgmLoop = st.BgmLoop()
	} else {
		e.hasLastBgm = false
		e.lastBgm = ""
	}
	e.lastEffectSerial = st.EffectSerial()
	e.lastVoiceSerial = st.VoiceSerial()
}

func tj2StepToCore(step state.SessionStep) core.Step {
	switch step {
	case state.SessionAwaitInput:
		return core.StepAwaitInput
	case state.SessionAwaitChoice:
		return core.StepAwaitChoice
	case state.SessionAwaitSelect:
		return core.StepAwaitSelect
	case state.SessionAwaitTitle:
		return core.StepAwaitTitle
	case state.SessionWait:
		return core.StepWait
	case state.SessionFinished:
		return core.StepFinished
	case state.SessionFaulted:
		return core.StepFaulted
	default:
		return core.StepContinue
	}
}

func resolveTj2DataDir(dir string) string {
	if hasTj2Anchor(dir) {
		return dir
	}
	type entry struct {
		path  string
		depth int
	}
	queue := []entry{{path: dir, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children, err := os.ReadDir(current.path)
		if err != nil {
			continue
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			full := filepath.Join(current.path, child.Name())
			if hasTj2Anchor(full) {
				return full
			}
			if current.depth+1 < 4 {
				queue = append(queue, entry{path: full, depth: current.depth + 1})
			}
		}
	}
	return dir
}

func hasTj2Anchor(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, item := range entries {
		if !item.IsDir() && strings.EqualFold(item.Name(), "SCENE.PFT") {
			return true
		}
	}
	return false
}

func (s *Tj2Session) scenarioNumber() int {
	if s.machine == nil {
		return 0
	}
	return s.machine.ScenarioNumber()
}
