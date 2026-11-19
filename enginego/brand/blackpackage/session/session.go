// Package session 把 blackpackage 的归档、图像、音频、VM 接成 core.Engine。
//
// 设计边界：VM 只产出“这一帧发生了什么”（文本、菜单、音频、等待），
// session 负责把事件翻译成 core 的 Step/State/Audio，并维护标题、
// 存档和缓存目录。这样 opcode 语义可以单独测，不需要启动 Flutter。
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"kizuna/enginego/brand/blackpackage/archive"
	"kizuna/enginego/brand/blackpackage/audio"
	"kizuna/enginego/brand/blackpackage/save"
	"kizuna/enginego/brand/blackpackage/script"
	"kizuna/enginego/brand/blackpackage/stage"
	"kizuna/enginego/core"
)

const (
	frameWidth    = 800
	frameHeight   = 600
	commandBudget = 4096
	saveSlots     = 20
	titleScript   = "x0_start.so4"
	gameScript    = "a0_000.so4"
)

type Session struct {
	dir      string
	cacheDir string
	set      *archive.Set
	stage    *stage.Stage
	vm       *script.VM
	store    *save.Store

	step        core.Step
	atTitle     bool
	title       string
	lastErr     string
	lastEmitted string
	backlog     []core.TextUnit
	backlogSeq  uint32

	speaker string
	text    string
	choices []core.Choice

	// 音频：VM 事件转成 core.AudioEvent；WA1 物化到 cacheDir/audio。
	pending      []core.AudioEvent
	materialized map[string]string
	// 最近一次 BGM 名，State 用。
	bgm string

	// 标题菜单：原版 X0_START 的“スタート”。这里只暴露 START，
	// 设置/读档由 Flutter 的设置页和下方槽位接口负责。
	titleMenu []core.Choice
	// dirty 让 Compose 不必每帧全量重绘；VM 暂停时仍需要至少一次。
	dirty bool
	// endingAt 用于 STAFF 滚动：原版滚完会回标题；我们没有逐帧回调，
	// 若滚动脚本长时间没有新对白，就把这一局视为通关。
	endingAt time.Time
	// 引擎自绘界面（信息框/标题子界面/选择肢）。全部画进 stage.Frame，
	// 宿主不再弹 Flutter 对话框。
	ui           *uiScreen
	titleOverlay string
	msgButtons   []uiButton
	// textRuns 是最近一次 Compose 收集到的文字（core.TextRunReporter）。
	textRuns     []core.TextRun
	bgmVolume    float64
	seVolume     float64
	voiceVolume  float64
	autoSpeed    int
	// seenImages/seenBgm/seenScenes 是周目内解锁的记录，供鉴赏使用。
	seenImages map[string]bool
	seenBgm    map[string]bool
	seenScenes map[string]bool
	// 右键系统菜单（原版 A0_MENU 的那一组命令）。VM 是单上下文，
	// 直接启动 a0_menu.so4 会丢掉当前剧情，所以用宿主选择肢模拟同样的命令。
	sysMenu bool
	sysPrev core.Step
	// saveDir 是玩家看得见的存档目录：优先游戏目录（X0WIN），不可写时
	// 退回缓存 user/。槽位文件同时镜像成原版命名的 X0WINnn.DAT。
	saveDir string
	// userDir 是可写的用户数据目录（<saveDir>/user）：原版写在游戏目录下的
	// X0WIN*.DAT 与全局配置都重定向到这里；时钟水位也放这里。
	userDir string
	// moviePath/movieSerial 交给宿主的平台解码器播放（core.MovieExposer）。
	moviePath   string
	movieSerial int
	// hostRequest 是引擎请求宿主执行的动作（settings/exit）。
	hostRequest string
	hostSerial  uint32
	// initDone 记录是否已经跑过原版的 INIT.SO4（见 ensureInit）。
	initDone bool
}

var _ core.Engine = (*Session)(nil)

// New 建未打开会话。
func New() *Session {
	return &Session{
		step:         core.StepAwaitTitle,
		atTitle:      true,
		title:        "BLACKPACKAGE",
		materialized: make(map[string]string),
		seenImages:   make(map[string]bool),
		seenBgm:      make(map[string]bool),
		seenScenes:   make(map[string]bool),
		bgmVolume:    1,
		seVolume:     1,
		voiceVolume:  1,
		autoSpeed:    50,
		titleMenu: []core.Choice{
			{Name: "START", Value: 1},
			{Name: "CONFIG", Value: 2},
		},
	}
}

// Open 定位游戏目录并打开归档。
func (s *Session) Open(dir string) (core.Info, error) {
	return s.open(dir, "")
}

func (s *Session) OpenExtras(dir string, _ []string, cacheDir string) (core.Info, error) {
	return s.open(dir, cacheDir)
}

func (s *Session) open(dir, cacheDir string) (core.Info, error) {
	gameDir, ok := archive.FindGameDir(dir)
	if !ok {
		return core.Info{}, fmt.Errorf("blackpackage: 找不到 X0WIN.DAT/LST（%s）", dir)
	}
	if s.set != nil {
		s.set.Close()
	}
	set, err := archive.Open(gameDir)
	if err != nil {
		return core.Info{}, err
	}
	s.dir = gameDir
	s.set = set
	s.stage = stage.New(set)
	s.stage.SetSet(set)
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "kizuna-blackpackage")
	}
	if abs, err := filepath.Abs(cacheDir); err == nil {
		cacheDir = abs
	}
	s.cacheDir = core.GameCacheDir(cacheDir, gameDir)
	if s.cacheDir == "" {
		s.cacheDir = filepath.Join(cacheDir, "x0win")
	}
	_ = os.MkdirAll(filepath.Join(s.cacheDir, "audio"), 0o755)
	// save/ 和 user/ 属于玩家数据，放在游戏所在目录（X0WIN）里；只有该目录
	// 不可写时才退回应用缓存。音频物化仍留在应用缓存，避免把游戏目录塞满。
	dataDir := gameDir
	if !dirWritable(gameDir) {
		dataDir = s.cacheDir
	}
	s.saveDir = dataDir
	s.store = save.New(filepath.Join(dataDir, "save"))
	s.store.Clean()
	// 游戏目录里的 X0WINnn.DAT 是玩家可见的镜像；缓存被清掉时把它们
	// 导回槽位库，读档界面仍能看到。
	for slot := 1; slot <= saveSlots; slot++ {
		dst := s.store.Path(uint32(slot))
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		src := filepath.Join(s.saveDir, fmt.Sprintf("X0WIN%02d.DAT", slot))
		if raw, err := os.ReadFile(src); err == nil {
			_ = os.WriteFile(dst, raw, 0o644)
		}
	}
	s.vm = script.New(set, s.stage)
	// 原版把存档/全局数据写成当前目录下的 X0WIN*.DAT；这里统一放在
	// 游戏目录（不可写时才用缓存）。游戏内 SAVE/LOAD 与槽位存档都落盘到这里。
	userDir := filepath.Join(s.saveDir, "user")
	_ = os.MkdirAll(userDir, 0o755)
	s.userDir = userDir
	s.vm.SetStoreDir(userDir)
	// 时钟接着上次离开的值走（见 saveClockWatermark）：原版 timeGetTime 是
	// 开机计时，跨进程连续；我们的时钟挂在 VM 上，需要自己接。
	s.loadClockWatermark()
	s.loadSettings(userDir)
	s.atTitle = true
	s.step = core.StepAwaitTitle
	s.lastErr = ""
	s.choices = nil
	s.text = ""
	s.speaker = ""
	s.bgm = ""
	s.dirty = true
	core.Logf("blackpackage: open %s (%d entries)", gameDir, set.Count())
	return core.Info{
		Width:      frameWidth,
		Height:     frameHeight,
		Title:      "BLACKPACKAGE",
		Version:    "1.0.0",
		SceneCount: 0,
		EntryCount: 1,
	}, nil
}

// Close 释放归档。
func (s *Session) Close() error {
	if s.set != nil {
		s.set.Close()
		s.set = nil
	}
	return nil
}

// Info 返回静态信息。
func (s *Session) Info() core.Info {
	return core.Info{Width: frameWidth, Height: frameHeight, Title: "BLACKPACKAGE", Version: "1.0.0", EntryCount: 1}
}

// CurrentStep 供 OpFrame 读当前步。
func (s *Session) CurrentStep() core.Step { return s.step }

// Start 按入口启动。0=标题，1=正篇第一本，2+=配置（退回标题）。
func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.vm == nil || s.set == nil {
		return core.StepFaulted, fmt.Errorf("blackpackage: 未打开")
	}
	switch entry {
	case 0:
		return s.startTitle()
	case 1:
		return s.startGame()
	default:
		// 其它入口没有独立剧本；回到标题，避免宿主以为启动了。
		return s.startTitle()
	}
}

// ensureInit 复刻原版进程启动时固定执行的 INIT.SO4。
//
// 为什么必须跑：INIT 负责初始化整局共享的全局变量（尤其是 0xb3d4=0xf5af）、
// 登记系统音效、建立 0..7 号层和消息窗，最后由 SCRIPT_LOAD 进入 a0_000.so4。
// 剧情脚本用这些变量做分支，X0_E000@0x1947c 的“本句有语音”条件就是
// `(vars[0xb3d4] & 0x20) != 0 && vars[0xadf6] == 0`。以前直接从 x0_start
// 起跑、跳过 INIT，0xb3d4 恒为 0，于是所有语音分支都被判成“没有语音”，
// 这是“游玩时听不到角色语音”的根因。BGM 的全局音量/通道表同样在这里
// 初始化，所以 INIT 也是音乐正常的前提。
//
// INIT 只在进程启动时跑一次；它最后会切到 a0_000，之后标题/新游戏都复用
// 这份状态，不要重复执行（重复会叠加音效登记并重置玩家设置）。
func (s *Session) ensureInit() {
	if s.initDone || s.vm == nil {
		return
	}
	s.initDone = true
	// 原版进程启动时屏幕层已由 WinMain 建好。INIT 会把背景/窗口画进这一层，
	// 所以必须在它之前补出缺失的屏幕层；否则 INIT 的 blit 找不到目标，
	// 之后玩家会看到全黑或只有文字。
	if !s.stage.HasLayer(stage.ScreenLayer) {
		s.stage.CreateLayer(stage.ScreenLayer, []int32{0, 0, 0, frameWidth, frameHeight, 0, 0})
	}
	if err := s.vm.Start("init.so4"); err != nil {
		s.lastErr = err.Error()
		return
	}
	for i := 0; i < 600; i++ {
		step, err := s.vm.Run(commandBudget)
		if err != nil {
			s.lastErr = err.Error()
			break
		}
		// INIT 里的提示框（仅在缺少 DirectSound/注册表的环境出现）不是剧情
		// 对白；这里把事件搬走，稍后统一清空文本。
		for _, ev := range s.vm.Audio {
			s.enqueueAudio(ev)
		}
		s.vm.Audio = nil
		s.vm.TakeImages()
		// SCRIPT_LOAD 会把当前脚本换成 a0_000（原版启动画面入口）。
		if s.vm.CurrentScript() != "init.so4" {
			break
		}
		if step == script.StepFinished || step == script.StepAwaitMovie {
			break
		}
	}
	s.vm.Text = ""
	s.vm.Speaker = ""
	s.vm.Choices = nil
	s.text, s.speaker = "", ""
	s.choices = nil
	s.step = core.StepContinue
	s.dirty = true
}

func (s *Session) startTitle() (core.Step, error) {
	// 原版先跑 INIT 再显示标题；标题脚本会用到 INIT 建立的层与音效表。
	s.ensureInit()
	s.atTitle = true
	s.step = core.StepAwaitTitle
	s.choices = nil
	s.text = ""
	s.speaker = ""
	// 标题图走独立的 title 通道：它不属于游戏画布，画进 1 号层会污染
	// 第 0 页，开局后标题大图会残留在剧情上。
	if _, err := s.set.Entry("X0_TIT0.PT1"); err == nil {
		s.stage.SetTitleImage("X0_TIT0.PT1")
	} else {
		s.stage.SetTitleImage("")
	}
	s.dirty = true
	return s.step, nil
}

func (s *Session) startGame() (core.Step, error) {
	// 原版的“开始游戏”发生在 INIT 已经跑完的进程里；这里补上同一步。
	s.stage.SetTitleImage("")
	s.ensureInit()
	s.atTitle = false
	s.choices = nil
	s.text = ""
	s.speaker = ""
	// “初めから”要进正篇：a0_000 是原版的菜单/选择脚本，直接续跑它只会停在
	// 标题菜单上。正确的入口是 x0_start.so4（它内部再切到 x0_op00）。
	if err := s.vm.Start(titleScript); err != nil {
		if err2 := s.vm.Start(gameScript); err2 != nil {
			s.lastErr = err2.Error()
			return core.StepFaulted, err2
		}
	}
	s.dirty = true
	return s.Run(commandBudget)
}

// Run 推进 VM。
func (s *Session) Run(budget uint32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, fmt.Errorf("blackpackage: 未打开")
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	b := int(budget)
	if b <= 0 {
		b = commandBudget
	}
	// 原版在开场/动画里会多次 yield “无文本的输入等待”（点击跳过 OP、
	// 等淡入淡出）。若把这种等待交给宿主，玩家会看到一个空对话框。
	// 这里自动跳过空文本等待，最多 64 次，避免坏脚本死循环。
	//
	// OP/演出脚本还会在“每帧回调”里轮询 vars[0xb3d0]：原版由主循环的
	// 回调或点击把它置位。我们没有独立的回调线程，所以连续多轮推进都
	// 没有新文本时，按一次“跳过”处理，和玩家点击 OP 的效果一致。
	beforeText := s.text
	for i := 0; i < 16; i++ {
		step, err := s.vm.Run(b)
		if err != nil {
			s.lastErr = err.Error()
			return core.StepFaulted, err
		}
		s.absorb(step)
		// STAFF 滚动没有可推进的对白；超过 4 秒仍停在滚动脚本就视为通关，
		// 避免玩家永远停在演职员表（原版此时回标题）。
		if s.vm.CurrentScript() == "a0_staff.so4" {
			if s.endingAt.IsZero() {
				s.endingAt = time.Now()
			} else if time.Since(s.endingAt) > 4*time.Second {
				s.step = core.StepFinished
				return s.step, nil
			}
		} else {
			s.endingAt = time.Time{}
		}
		if step == script.StepContinue {
			// 预算耗尽仍没有新对白/选项：这是在跑演出循环（OP、淡入淡出）。
			// 交给宿主的 16ms tick 继续 pump；同时按一次“跳过”置位，
			// 让脚本里的 vars[0xb3d0] 轮询在下一次 pump 收尾。
			if s.text == beforeText && len(s.choices) == 0 {
				s.vm.SignalSkip()
			}
			return core.StepContinue, nil
		}
		if s.step == core.StepAwaitInput {
			t := cleanText(s.text)
			if t == "" || len([]rune(t)) < 2 || t == s.lastEmitted {
				continue
			}
			s.lastEmitted = t
		}
		return s.step, nil
	}
	return s.step, nil
}

// normalizeMessage 把原版的说话人前缀拆出来。
// 剧本常见形式：`$s1_「台词」`（$s1 是主角名 buffer）或 `ありす_「台词」`。
// 若 buffer 还没被名字输入填过，$s1 会原样留下；这时回退到“俺”。
func normalizeMessage(speaker, text string) (string, string) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "$s") || strings.HasPrefix(text, "$S") {
		// 去掉 $s<digits>_ 前缀。
		i := 2
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			i++
		}
		if i < len(text) && text[i] == '_' {
			if speaker == "" || strings.HasPrefix(speaker, "$s") || strings.HasPrefix(speaker, "$S") {
				speaker = ""
			}
			text = text[i+1:]
		}
	}
	if speaker == "" {
		if j := strings.Index(text, "_「"); j > 0 && j < 16 {
			speaker = strings.TrimSpace(text[:j])
			text = text[j+1:]
		}
	}
	if strings.HasPrefix(speaker, "$s") || strings.HasPrefix(speaker, "$S") {
		speaker = ""
	}
	return strings.TrimSpace(speaker), text
}

// cleanText 去掉 CP932 控制字节（原版用 0x02/0x04/0x05 表示强调、速度、
// 停顿）。Flutter 的 Text 不认识这些字节，留着会显示成方块。
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

func (s *Session) absorb(st script.Step) {
	// 文本/菜单/音频是“本帧事件”，先搬运再决定 Step。
	s.speaker, s.text = normalizeMessage(s.vm.Speaker, cleanText(s.vm.Text))
	if s.text != "" && (len(s.backlog) == 0 || s.backlog[len(s.backlog)-1].Text != s.text) {
		s.backlogSeq++
		s.backlog = append(s.backlog, core.TextUnit{ID: fmt.Sprintf("live:%d", s.backlogSeq), File: s.vm.CurrentScript(), Text: s.text})
		if len(s.backlog) > 500 {
			s.backlog = s.backlog[len(s.backlog)-500:]
		}
	}
	if st == script.StepAwaitChoice && len(s.vm.Choices) > 0 {
		// 选择肢也由引擎画进帧并自己命中，宿主不再显示 Flutter 选择框。
		s.choices = nil
		s.buildChoiceUI()
	} else if st != script.StepAwaitChoice {
		s.choices = nil
	}
	for _, ev := range s.vm.Audio {
		s.enqueueAudio(ev)
	}
	s.vm.Audio = nil
	for _, ev := range s.vm.TakeImages() {
		name := strings.ToLower(ev)
		if strings.HasSuffix(name, ".pt1") && !strings.HasSuffix(name, "m.pt1") {
			s.seenImages[ev] = true
		}
	}
	if name := s.vm.CurrentScript(); name != "" {
		s.seenScenes[name] = true
	}
	switch st {
	case script.StepWait:
		s.step = core.StepWait
	case script.StepFinished:
		s.step = core.StepFinished
	case script.StepAwaitChoice:
		s.step = core.StepAwaitInput
	case script.StepAwaitMovie:
		s.step = core.StepAwaitMovie
		name := s.vm.Movie()
		if name != "" {
			s.moviePath = filepath.Join(s.dir, name)
			if _, err := os.Stat(s.moviePath); err != nil {
				// 有些发行版把 MPG 作为独立条目放在数据包旁边。
				s.moviePath = filepath.Join(s.dir, filepath.Base(name))
			}
			s.movieSerial++
		}
	case script.StepAwaitInput:
		s.step = core.StepAwaitInput
	case script.StepFaulted:
		s.step = core.StepFaulted
	default:
		s.step = core.StepContinue
	}
	s.dirty = true
}

// Advance 在等输入时继续脚本；标题下则开始游戏。
func (s *Session) Advance() (core.Step, error) {
	if s.ui != nil {
		return s.step, nil
	}
	if s.titleOverlay == "view" {
		s.titleOverlay = "gallery"
		s.openGallery()
		return s.step, nil
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	if s.step == core.StepAwaitChoice {
		return s.step, nil
	}
	if s.step == core.StepFinished {
		return s.step, nil
	}
	return s.Run(commandBudget)
}

// Choose 选一个选择肢。
func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.ui != nil {
		if int(index) < len(s.ui.Buttons) {
			s.uiAction(s.ui, s.ui.Buttons[index])
		}
		return s.step, nil
	}
	if s.step != core.StepAwaitChoice {
		return s.step, nil
	}
	if _, err := s.vm.Choose(int(index)); err != nil {
		s.lastErr = err.Error()
		return core.StepFaulted, err
	}
	s.choices = nil
	return s.Run(commandBudget)
}

// Pointer 处理点击。标题任意点击进游戏；剧情里点击等同 Advance。
func (s *Session) Pointer(op uint32, x, y int32) (core.Step, error) {
	// 宿主约定（KzPointer）：0=移动 1=按下 2=抬起 3=点击(原子按+抬) 4=右键 6=拖动。
	// 正常点击走 3；旧库回退才会发 1→2。以前只处理 1，导致宿主的 tap 全部
	// 被丢掉——标题按钮和剧情点击都表现为“没反应”。
	if s.ui != nil {
		if op == 3 || op == 2 {
			s.uiClick(x, y)
		}
		return s.step, nil
	}
	if s.titleOverlay == "view" {
		if op == 3 || op == 2 {
			s.titleOverlay = "gallery"
			s.openGallery()
		}
		return s.step, nil
	}
	if s.atTitle {
		// 命中框和宿主 TitleLayer 用同一组矩形；点击空白不启动游戏。
		if op == 3 || op == 2 {
			for _, it := range s.TitleMenu() {
				if x >= it.X && x < it.X+int32(it.Width) && y >= it.Y && y < it.Y+int32(it.Height) {
					return s.SelectTitleItem(it.Key)
				}
			}
		}
		return core.StepAwaitTitle, nil
	}
	switch op {
	case 3, 2:
		// 信息框底部工具按钮优先；其它位置推进对白。
		if y >= 548 {
			for _, b := range s.messageButtonRects() {
				if x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
					// 不挂 uiScreen，否则 clear/auto/skip 会把信息框自己挡掉。
					s.uiAction(&uiScreen{Kind: "message"}, b)
					return s.step, nil
				}
			}
		}
		return s.Advance()
	case 4:
		s.openSystemMenu()
		return s.step, nil
	}
	return s.step, nil
}

// Compose 把舞台合成到宿主缓冲（RGBA；capi 会换位成 BGRA）。
func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if dst == nil || n < frameWidth*frameHeight*4 {
		return false, nil
	}
	if s.stage == nil {
		return false, nil
	}
	s.stage.Compose()
	// 文字只属于本次合成：drawUI/drawMessageWindow 会追加新的条目。
	s.textRuns = s.textRuns[:0]
	if s.ui != nil {
		s.drawUI(s.stage.Frame)
	} else {
		s.drawMessageWindow(s.stage.Frame)
	}
	out := unsafe.Slice((*byte)(dst), frameWidth*frameHeight*4)
	copy(out, s.stage.Frame)
	s.dirty = false
	return true, nil
}

// TextRuns 返回最近一次 Compose 收集到的文字（core.TextRunReporter）。
func (s *Session) TextRuns() []core.TextRun {
	if len(s.textRuns) == 0 {
		return nil
	}
	out := make([]core.TextRun, len(s.textRuns))
	copy(out, s.textRuns)
	return out
}

// ComposeWritesBGRA 返回 true：stage 直接写宿主需要的字节序。
//
// 宿主的 FramePainter 用 PixelFormat.rgba8888 解码；capi 在实现返回 false 时
// 会把 dst 做一次 R/B 交换。stage.Frame 本来就是 RGBA，所以必须返回 true
// 跳过那趟交换，否则整帧红蓝互换——所有图片颜色错误的根因就在这里。
func (s *Session) ComposeWritesBGRA() bool { return true }

// Scenes 暂无鉴赏目录（原版没有独立 CG 鉴赏；素材以剧情图为主）。
func (s *Session) Scenes() []core.Scene { return nil }

// Slots 返回槽位列表。
func (s *Session) Slots() []core.Slot {
	if s.store == nil {
		return nil
	}
	return s.store.List(saveSlots)
}

// Save 写槽位。
func (s *Session) Save(slot uint32, title string) error {
	if s.vm == nil || s.store == nil {
		return fmt.Errorf("blackpackage: 未打开")
	}
	summary := s.text
	if len([]rune(summary)) > 40 {
		summary = string([]rune(summary)[:40]) + "…"
	}
	// 连画面一起存：原版读档后不会重画已经画过的内容，只靠变量/IP
	// 恢复会看到“上一屏残留”。stage 快照只取画布第 0 页，不含引擎 UI。
	var screen []byte
	if s.stage != nil {
		screen = s.stage.ScreenSnapshot()
	}
	meta := save.Meta{
		Title:   title,
		Summary: summary,
		Speaker: s.speaker,
		Text:    s.text,
		Bgm:     s.bgm,
	}
	if err := s.store.Save(slot, meta, s.vm.Snapshot(), screen); err != nil {
		return err
	}
	// 把时钟水位写到 user/clock：下次启动时 VM 从这里接着计时。
	// 没有这一步，重启后的新时钟从 0 开始，旧存档里记录的绝对时刻
	// （动画开始时间）会远大于当前时钟，读档后演出循环要白等很久。
	s.saveClockWatermark()
	// 原版存档名是 X0WINnn.DAT；把同一份槽位数据镜像过去，玩家能在
	// 游戏目录里看到文件（内容是本引擎的 JSON 快照）。
	if raw, err := os.ReadFile(s.store.Path(slot)); err == nil {
		_ = os.WriteFile(filepath.Join(s.saveDir, fmt.Sprintf("X0WIN%02d.DAT", slot)), raw, 0o644)
	}
	s.saveSettings()
	return nil
}

// clockWatermarkPath 是时钟水位文件（user/clock）。
func (s *Session) clockWatermarkPath() string {
	return filepath.Join(s.userDir, "clock")
}

// saveClockWatermark 记下当前 timeGetTime 值。
//
// 为什么需要落盘：脚本把“动画开始时刻”当普通变量存进存档，读档时拿它和
// 当前时钟比较。原版 timeGetTime 是开机计时，进程重启后仍然往后走；我们的
// 时钟以 VM 启动为零点，重启后归零，存档里的时刻就“在未来”，淡入淡出循环
// 要等几分钟才退出。把水位写盘、下次启动接上，行为就和原版一致。
func (s *Session) saveClockWatermark() {
	if s.vm == nil || s.userDir == "" {
		return
	}
	_ = os.WriteFile(s.clockWatermarkPath(), []byte(strconv.FormatUint(uint64(s.vm.Tick()), 10)), 0o644)
}

// loadClockWatermark 把 VM 时钟接到上次离开的位置。文件缺失/损坏时忽略
// （新装或刚清过缓存，从头计时即可）。
func (s *Session) loadClockWatermark() {
	if s.vm == nil {
		return
	}
	raw, err := os.ReadFile(s.clockWatermarkPath())
	if err != nil {
		return
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 32)
	if err != nil || v == 0 {
		return
	}
	s.vm.SetClockBase(uint32(v))
}


// 读槽位。
func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.vm == nil || s.store == nil {
		return core.StepFaulted, fmt.Errorf("blackpackage: 未打开")
	}
	snap, rec, err := s.store.Load(slot)
	if err != nil {
		// 兼容直接从 X0WINnn.DAT 读档（例如从游戏目录拷回来的存档）。
		if raw, e2 := os.ReadFile(filepath.Join(s.saveDir, fmt.Sprintf("X0WIN%02d.DAT", slot))); e2 == nil {
			snap, rec, err = s.store.LoadBytes(raw)
		}
	}
	if err != nil {
		return core.StepFaulted, err
	}
	if err := s.vm.Restore(snap); err != nil {
		return core.StepFaulted, err
	}
	// 恢复存档瞬间的画面（老存档可能没有），保证背景/立绘与存档时一致。
	if screen := rec.Screen(); len(screen) > 0 && s.stage != nil {
		s.stage.RestoreScreen(screen)
	}
	s.atTitle = false
	// 从标题画面读档时，标题图还挂在 stage 上；不清掉它会盖住游戏画面，
	// 只露出引擎自绘的信息框（玩家看到“背景还是主界面，但多了对话框”）。
	s.stage.SetTitleImage("")
	// titleOverlay 是标题的“鉴赏/回想”子状态：留着它会让 Pointer/Advance
	// 一直走鉴赏分支（读档后点画面没反应、反而跳回标题）。
	s.titleOverlay = ""
	s.text = rec.Text
	s.speaker = rec.Speaker
	// lastEmitted 记的是读档前那一句；不重置会让读档后的第一句被当成
	// “重复文本”跳过，剧情看起来停在原地。
	s.lastEmitted = ""
	s.choices = nil
	// 读档后的 BGM：新进程里宿主音频是停的，原版会接着播存档时那首。
	// 同一进程内读档也没问题——重发一次 play 等于从头播这首曲子，
	// 和原版“读档时重新启动 BGM”一致。
	if rec.Bgm != "" {
		s.emitBgm(rec.Bgm)
	}
	s.step = core.StepAwaitInput
	core.Logf("blackpackage: load slot %d (%s)", slot, rec.Title)
	return s.step, nil
}

// ---- 影片（core.MovieExposer / MovieFinisher）----

// CurrentMovie 返回待播影片的绝对路径。
func (s *Session) CurrentMovie() string { return s.moviePath }

// MovieSerial 返回影片请求序号。
func (s *Session) MovieSerial() int { return s.movieSerial }

// FinishMovie 由宿主播完（或跳过）影片后调用。
func (s *Session) FinishMovie() (core.Step, error) {
	s.moviePath = ""
	if s.vm != nil {
		s.vm.FinishMovie()
	}
	return s.Run(commandBudget)
}

// AssetSize / ReadAsset 让宿主能直接取素材（调试、缩略图）。
func (s *Session) AssetSize(path string) int {
	if s.set == nil {
		return 0
	}
	b, err := s.set.Entry(path)
	if err != nil {
		return 0
	}
	return len(b)
}

func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if s.set == nil || dst == nil || n <= 0 {
		return 0
	}
	b, err := s.set.Entry(path)
	if err != nil {
		return 0
	}
	if len(b) > n {
		b = b[:n]
	}
	copy(unsafe.Slice((*byte)(dst), n), b)
	return len(b)
}

// Audio 取走本帧音频事件。
func (s *Session) Audio() []core.AudioEvent {
	out := s.pending
	s.pending = nil
	return out
}

// Text 当前对白。
func (s *Session) Text() core.TextState {
	return core.TextState{Speaker: s.speaker, Text: s.text, Awaiting: uint32(s.step)}
}

// Choices 当前选择肢。选择肢已经改由引擎画进帧并自己命中，
// 这里恒为空，避免宿主再叠一层 Flutter 选择框。
func (s *Session) Choices() []core.Choice { return nil }

// State 每次轮询的快照。
func (s *Session) State() core.State {
	st := core.State{
		Bgm:                  s.bgm,
		PC:                   uint32(s.vmCurrentIP()),
		MessageWindowVisible: false,
		// 对白框、工具按钮、选择肢、标题子界面都由引擎画进帧。
		EngineMessageWindow: !s.atTitle,
		WaitMs:              0,
		AutoWait:            s.autoSpeed,
		// 标题的「環境設定/続きから/終了」、游戏内菜单的 SAVE/LOAD/LOG
		// 都靠这两个字段通知宿主开界面；漏填会让按钮看起来完全没反应。
		HostRequest:       s.hostRequest,
		HostRequestSerial: s.hostSerial,
	}
	if s.step == core.StepWait {
		st.WaitMs = s.vmWaitMs()
		st.Animating = true
	}
	return st
}

func (s *Session) vmCurrentIP() int {
	if s.vm == nil {
		return 0
	}
	// 直接用 IP()：以前这里调用 Snapshot()，每次 State 轮询都会整份拷贝
	// 512KB 变量表（外加标志/音频表），宿主每帧都轮询一次，纯属浪费。
	return s.vm.IP()
}

func (s *Session) vmWaitMs() uint32 {
	if s.vm == nil {
		return 0
	}
	return s.vm.WaitMs
}

// TextList 扫描剧本里的 0x9/0x488 文本，供翻译/检索。只在第一次调用时做，
// 失败返回已扫到的部分。
func (s *Session) TextList() []core.TextUnit {
	if len(s.backlog) > 0 {
		return append([]core.TextUnit(nil), s.backlog...)
	}
	if s.set == nil {
		return nil
	}
	var out []core.TextUnit
	for _, name := range s.set.Names() {
		if !strings.HasSuffix(strings.ToLower(name), ".so4") {
			continue
		}
		raw, err := s.set.Entry(name)
		if err != nil {
			continue
		}
		sc, err := script.Parse(name, raw)
		if err != nil {
			continue
		}
		for off, rec := range sc.Recs {
			switch rec.Op {
			case 0x0009, 0x0488:
				r := &exprReaderForText{pay: rec.Payload}
				// 跳过前两个表达式（layer/offset），剩下的才是文本。
				if rec.Op == 0x0009 {
					r.skipExpr()
				} else {
					r.skipExpr()
					r.skipExpr()
				}
				text := r.cstr()
				if text != "" {
					out = append(out, core.TextUnit{ID: fmt.Sprintf("%s:%x", name, off), File: name, Text: text})
				}
			}
		}
	}
	return out
}

// LastError 最近错误。
func (s *Session) LastError() string { return s.lastErr }

// ---- 音频物化 ----

// enqueueAudio 把 VM 的音频事件翻译成宿主事件。
//
// 关键点（与原版 4 通道播放器对齐）：每个事件都带 Channel，宿主按通道
// 维护一个播放槽。播放在通道上会替换该通道上一次的声音，停止也只停这条
// 通道——这样“先 stop 再 play”不会误杀别的类别，BGM 也不会被语音的
// stop 打断。类别（BGM/SE/语音）由 VM 按文件名判定后放在 Kind 里。
func (s *Session) enqueueAudio(ev script.Audio) {
	ch := uint32(0)
	if ev.ID >= 0 && ev.ID < 4 {
		ch = uint32(ev.ID)
	}
	if ev.Kind == 3 || ev.Kind == 4 || ev.Kind == 5 {
		if ev.ID >= 0 && ev.ID < 4 {
			ch = uint32(ev.ID)
		}
		if ev.Kind == 3 {
			// 停止 BGM 时同步状态里的曲名，宿主状态栏不会停在旧曲目上。
			s.bgm = ""
		}
		s.pending = append(s.pending, core.AudioEvent{
			Kind: ev.Kind, Channel: ch, Group: 0xFFFFFFFF,
		})
		return
	}
	if ev.Name == "" {
		return
	}
	path := s.materialize(ev.Name)
	kind := ev.Kind
	if kind > 2 {
		kind = 1
	}
	if kind == 0 {
		s.bgm = ev.Name
		s.seenBgm[strings.ToLower(ev.Name)] = true
	}
	// 最终音量 = 玩家设置里的类别音量 × 脚本音量。脚本音量缺失（0）时按 1。
	base := s.seVolume
	switch kind {
	case 0:
		base = s.bgmVolume
	case 2:
		base = s.voiceVolume
	}
	vol := base
	if ev.Volume > 0 {
		vol = base * ev.Volume
	}
	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	s.pending = append(s.pending, core.AudioEvent{
		Kind: kind, Path: path, Volume: vol, Loop: ev.Loop || kind == 0,
		Channel: ch, Group: 0xFFFFFFFF,
	})
}

// emitBgm 在读档后重新播存档时的 BGM。走 enqueueAudio 复用同一条
// “物化 -> 类别音量 -> pending 事件”的路径，避免另开一条音量/循环
// 规则，读档播放与正常播放必须是同一套行为。
func (s *Session) emitBgm(name string) {
	if name == "" {
		return
	}
	s.enqueueAudio(script.Audio{Kind: 0, Name: name, Loop: true, ID: 0})
}

func (s *Session) materialize(name string) string {
	if p, ok := s.materialized[name]; ok {
		return p
	}
	raw, err := s.set.Entry(name)
	if err != nil && !strings.HasSuffix(strings.ToLower(name), ".wa1") {
		raw, err = s.set.Entry(name + ".wa1")
	}
	if err != nil {
		return ""
	}
	wav, err := audio.DecodeWA1(raw)
	if err != nil {
		return ""
	}
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, name)
	if !strings.HasSuffix(strings.ToLower(safe), ".wav") {
		safe += ".wav"
	}
	p := filepath.Join(s.cacheDir, "audio", safe)
	// 宿主用平台解码器按绝对路径读文件；相对路径会以 Dart 进程的 CWD
	// 解析，Android 上通常找不到，表现为“音频事件有、但完全没有声音”。
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if err := os.WriteFile(p, wav, 0o644); err != nil {
		return ""
	}
	s.materialized[name] = p
	return p
}

// ---- 主界面 ----

// TitleMenu 返回标题按钮布局。
//
// X0_TIT0.PT1 把 7 个竖排标签直接画在右侧：
// 初めから / 続きから / 画像閲覧 / シーン回想 / 音楽モード / 環境設定 / 終了。
// 热区必须逐列对齐这些像素（列心 722/676/632/587/542/497/450），
// 否则玩家点画面上看得见的字，引擎却把点击判到画面左侧的空白处。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	if !s.atTitle || s.ui != nil || s.titleOverlay != "" {
		return nil
	}
	return []core.TitleMenuItem{
		{Key: "start", Label: "初めから", X: 700, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "load", Label: "続きから", X: 654, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "museum", Label: "画像閲覧", X: 610, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "scene", Label: "シーン回想", X: 565, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "music", Label: "音楽モード", X: 520, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "config", Label: "環境設定", X: 475, Y: 415, Width: 44, Height: 180, Enabled: true},
		{Key: "quit", Label: "終了", X: 428, Y: 500, Width: 44, Height: 95, Enabled: true},
	}
}

// SelectTitleItem 处理标题点击。
func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if !s.atTitle {
		return s.step, nil
	}
	switch key {
	case "start":
		s.clearUI()
		s.titleOverlay = ""
		return s.startGame()
	case "load":
		s.openLoadScreen()
		return s.step, nil
	case "config":
		s.openConfigScreen()
		return s.step, nil
	case "quit":
		s.hostRequest = "exit"
		s.hostSerial++
		return core.StepAwaitTitle, nil
	case "museum":
		s.openGallery()
		return s.step, nil
	case "music":
		s.openMusicMode()
		return s.step, nil
	case "scene":
		s.openSceneRecall()
		return s.step, nil
	default:
		return core.StepAwaitTitle, nil
	}
}

// DebugVM 返回当前脚本与偏移（测试/诊断）。
func (s *Session) DebugVM() (string, int) {
	if s.vm == nil {
		return "", 0
	}
	return s.vm.CurrentScript(), s.vm.IP()
}

// DebugLayers 返回舞台层摘要（测试/日志）。
func (s *Session) DebugLayers() string {
	if s.stage == nil {
		return "nil"
	}
	return s.stage.DebugLayers()
}

// LoadLog 返回最近载入的图片名（测试/诊断）。
func (s *Session) LoadLog() []string {
	if s.stage == nil {
		return nil
	}
	return append([]string(nil), s.stage.LoadLog...)
}

// ---- 设置持久化 ----

type settingsFile struct {
	BGM   float64 `json:"bgm"`
	SE    float64 `json:"se"`
	Voice float64 `json:"voice"`
	Auto  int     `json:"auto"`
}

func dirWritable(dir string) bool {
	f := filepath.Join(dir, ".kizuna-write-test")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		return false
	}
	_ = os.Remove(f)
	return true
}

func (s *Session) settingsPaths() []string {
	out := make([]string, 0, 2)
	if s.saveDir != "" {
		out = append(out, filepath.Join(s.saveDir, "X0WINCFG.DAT"))
	}
	if s.cacheDir != "" {
		out = append(out, filepath.Join(s.cacheDir, "user", "X0WINCFG.DAT"))
	}
	return out
}

func (s *Session) loadSettings(userDir string) {
	paths := []string{filepath.Join(userDir, "X0WINCFG.DAT")}
	if s.saveDir != "" && s.saveDir != userDir {
		paths = append([]string{filepath.Join(s.saveDir, "X0WINCFG.DAT")}, paths...)
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg settingsFile
		if json.Unmarshal(raw, &cfg) != nil {
			continue
		}
		if cfg.BGM <= 0 && cfg.SE <= 0 && cfg.Voice <= 0 && cfg.Auto == 0 {
			continue
		}
		s.bgmVolume, s.seVolume, s.voiceVolume, s.autoSpeed = cfg.BGM, cfg.SE, cfg.Voice, cfg.Auto
		return
	}
}

func (s *Session) saveSettings() {
	cfg := settingsFile{BGM: s.bgmVolume, SE: s.seVolume, Voice: s.voiceVolume, Auto: s.autoSpeed}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	for _, p := range s.settingsPaths() {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, raw, 0o644)
	}
}
