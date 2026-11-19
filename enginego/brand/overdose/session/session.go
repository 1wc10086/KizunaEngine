// Package session 是 Over-Dose Moon 的 core.Engine 实现。
//
// 分层与 emon/session 一致：本包只管"把各子包接起来 + 维护 core 状态"，
// arc/gfx/stage/audio/save/script 各自独立可测。所有方法在 capi 侧已由
// handle.mu 串行化，这里不额外加锁；无锁的幂等只靠字段归约。
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/arc"
	overaudio "kizuna/enginego/brand/overdose/audio"
	overgfx "kizuna/enginego/brand/overdose/gfx"
	"kizuna/enginego/brand/overdose/menus"
	oversave "kizuna/enginego/brand/overdose/save"
	overscript "kizuna/enginego/brand/overdose/script"
	overstage "kizuna/enginego/brand/overdose/stage"
	"kizuna/enginego/core"
)

const (
	frameWidth  = 800
	frameHeight = 600

	// budget 与 emon 同量级：每轮 Run 只推进有限指令，避免一次跑掉整段剧情。
	commandBudget = 4096
	// saveSlotCount 是 10 bank × 10 slot 的总槽位（原版 nMaxSlot=10 × nMaxPage=10）。
	// 对外 core.Slot.Index 用 1..100；存/读档界面再把它拆回 bank/slot。
	saveSlotCount = 100
)

var (
	errNotOpen  = fmt.Errorf("overdose session: 未打开")
	errBadEntry = fmt.Errorf("overdose session: 非法入口")
	errNoSlot   = fmt.Errorf("overdose session: 槽位无档")
)

// Session 是单局运行态：标题/剧本共用同一实例，切换只改 step 与模式。
type Session struct {
	dir      string
	archives *arc.Set
	cache    *overgfx.Cache
	stage    *overstage.Stage
	audio    *overaudio.Player
	saves    *oversave.Store
	vm       *overscript.VM
	frame    *bishopgfx.BgraCanvas
	title    string
	// textRuns 是最近一次 Compose 收集到的文字（core.TextRunReporter）。
	textRuns []core.TextRun

	// 模式互斥：atTitle 与 playing 同一时刻至多一真（gallery 另用独立按钮层）。
	atTitle bool
	// menu 只在标题态有值，Start(entry>=1) 直接入剧本时保持空。
	menu *titleMenu
	// galleryMode 区分"extra 画廊"的两类页面（CG/音乐），只影响 Compose 的图集来源。
	galleryMode    galleryKind
	galleryStarted bool

	step      core.Step
	speaker   string
	text      string
	choices   []core.Choice
	backlog   []core.TextUnit
	lastError string
	// currentScript 是 entry>=1 时 loader 的键名（main/sXXX），存档 script 字段。
	currentScript string
	// msgVisible 跟踪 0x4d/0x50/0x51（macro msgon/msgoff）与 0x25 的窗口状态。
	msgVisible bool
	waitMs     uint32
	// bgm 从 audio 快照里取，State 直接返回，不另维护一份。
	bgmFile string
	// unlockSets 是四类鉴赏解锁的快速查表，源文件为 save.SystemData。
	unlockSets galleryUnlocks
	// hostRequest 是引擎请求宿主执行的动作（如 "exit"），State 透传给宿主路由。
	hostRequest       string
	hostRequestSerial uint32
	// settings 来自系统文件，OptionSetter 回写；本轮只透传 core State 字段。
	settings oversave.Settings
	preview  core.Frame
	// scriptOrder 是 list_script 表序，Start(entry>=2) 按它取脚本名。
	scriptOrder []string

	// ---- 引擎自绘界面状态（见 ui.go 的布局契约）----
	// overlay 是当前覆盖层；overlayNone 时剧情/标题照常合成。
	overlay overlayKind
	// overlayReturn 记录关闭覆盖层后回到的 step（AwaitTitle 或 AwaitInput）。
	overlayReturn core.Step
	// saveBank 是存/读档当前页（0..9），saveMode 区分 save/load 两种界面。
	saveBank int
	saveMode overlayKind
	// extraTab/extraChara/extraPage 是鉴赏三页的游标。
	extraTab     galleryKind
	extraChara   int
	extraPage    int
	extraPlaying int
	// CG 全屏浏览的候选名与下标（左键下一张，右键退出）。
	extraViewNames []string
	extraViewIndex int
	// configDrag 标记当前拖动中的滑杆：0=无，1=窗口透明度，2..5=主/BGM/SE/VOICE。
	configDrag int
	// backlogOffset 是回想列表的滚动行偏移。
	backlogOffset int
	// backlogDragY 记录触屏拖动起点，供 dragBacklog 计算增量。
	backlogDragY int
	// confirmKind 取 "save"/"overwrite"/"load"/"quit"；confirmSlot 是 1..100。
	confirmKind string
	confirmSlot int
	confirmBack overlayKind
	// msgPages 是当前对白按 3 行分页后的文本；msgPage 是当前页。
	msgPages    [][]string
	msgPage     int
	msgRevealAt time.Time
	msgFull     string
	// choiceMenu 是引擎自绘的选择肢（0x1c 简单 / 0x1d 带坐标）。
	// Flutter 的 ChoicesLayer/SelectLayer 不参与：step 用 AwaitSelect 但
	// SelectItems 返回空，点击由 Pointer 在引擎内命中。
	choiceMenu []choiceMenuItem
	hoverX     int32
	hoverY     int32
	// lastVoice 供右键「重播语音」使用。
	lastVoice string
	// messageUnread 标记当前句在显示前是否未读（宿主跳过逻辑用）。
	messageUnread bool
	// readDirty 表示有新的已读键未落盘；Close/Save 时统一写一次，避免每句都刷盘。
	readDirty bool
	// sceneReturnExtra 表示当前剧本是鉴赏场景回放，结束后回场景页。
	sceneReturnExtra bool
	// lastTick 是上一帧合成的墙钟时间，用来把 stage.Tick 的固定 16ms 换成真实
	// 帧间隔；否则 30fps 合成下 1000ms 的转场会跑成 2 秒。
	lastTick time.Time
}

// New 新建未打开的 Session。
func New() *Session {
	return &Session{
		step:    core.StepAwaitTitle,
		atTitle: true,
		// 对话框等第一句对白（或 msgon）再显示，避免标题→剧情切换时
		// 先闪一个空的黑框。setMessage 会把它置 true。
		msgVisible: false,
		settings:   oversave.DefaultSettings(),
	}
}

// Open 按 arc.FindGameDir 定位游戏目录，打开归档集并建 gfx/cache/stage/audio/save。
// audio 根用 core.GameCacheDir 哈希隔离：同名 ogg 在多游戏间不顶掉（沿用 emon 口径）。
func (s *Session) Open(dir string) (core.Info, error) {
	return s.open(dir, "")
}

func (s *Session) open(dir, cacheRoot string) (core.Info, error) {
	gameDir, ok := arc.FindGameDir(dir)
	if !ok {
		return core.Info{}, fmt.Errorf("overdose: 找不到游戏数据目录（%s）", dir)
	}
	// 上次 Open 留下的归档必须关：留着只会把旧文件句柄让给新局。
	if s.archives != nil {
		s.archives.Close()
	}
	set, err := arc.OpenSet(gameDir)
	if err != nil {
		return core.Info{}, err
	}
	s.dir = gameDir
	s.archives = set
	s.title = "Over-Dose Moon"
	core.Logf("overdose: open %s (script=%d system=%d graphic=%d)", gameDir,
		archiveCount(set.Script), archiveCount(set.System), archiveCount(set.Graphic))
	s.cache = overgfx.FromArchiveSet(set)
	s.stage = overstage.NewStage(s.cache)
	baseCache := cacheRoot
	if baseCache == "" {
		baseCache = filepath.Join(os.TempDir(), "kizuna-overdose")
	}
	cacheDir := core.GameCacheDir(baseCache, gameDir)
	audioRoot := filepath.Join(cacheDir, "audio")
	s.audio = overaudio.New(set, audioRoot)
	s.saves = oversave.New(filepath.Join(gameDir, "save"))
	// 系统文件（解锁+设置）在此恢复到内存：默认全锁，gallery 不得凭空放行。
	s.loadSystem()
	s.frame = bishopgfx.NewBgraCanvas(frameWidth, frameHeight)
	s.frame.Clear(0, 0, 0, 255)
	// VM 的 Loader 优先吃散装汉化剧本（script/ 等覆盖目录），找不到才解
	// script.arc 里的同名条目；UTF-8/CP932 由 script 包的字符串解码自动区分。
	s.vm = overscript.New(s.loadScriptEntry)
	s.scriptOrder = loadScriptOrder(set)
	// macro 宏走 stage.HandleMacroEvent，VM 不再跑 macro.asb（见 overdose_spec §6）。
	s.vm.SetMacroHook(func(name string, args []overscript.Value) error {
		// rand_int：op 0x0e 在引擎里的 handler 是 0x4209aa 的 `ret`（handlers.json
		// 的 14 号地址），本身不写变量；macro.asb 里它只求值后返回。旧实现强行
		// 把 var(1,92) 置 0 是额外行为，会让脚本自己的随机写入失效，已删除。
		if name == "ev" || name == "evd" {
			// ev/evd 的图名在全局字符串 var0；扫 CG 解锁在 stage 之后补。
			if bg, err := s.vm.Variables().GetString(2, 0); err == nil && bg != "" {
				s.unlock("cg", bg)
			}
		}
		_ = overstage.HandleMacroEvent(s.stage, s.vm.Variables(), name, args)
		return nil
	})
	return core.Info{Width: frameWidth, Height: frameHeight, Title: s.title, Version: "1.0.0"}, nil
}

// archiveCount 只用于 Open 日志；nil 归档记 0。
func archiveCount(a *arc.Archive) int {
	if a == nil {
		return 0
	}
	return a.Count
}

// OpenExtras 让宿主传入自己的缓存根（App 私有目录）。缺这个接口时 capi 会退回
// Open，音频物化会落到系统临时目录，重启/清理后玩家要重新解压几百 MB。
func (s *Session) OpenExtras(dir string, _ []string, cacheDir string) (core.Info, error) {
	info, err := s.open(dir, cacheDir)
	return info, err
}

// Close 释放归档句柄；引擎销毁时 capi 会优先走 core.Closer，
// 不用等 GC 去关 1.1G 的 graphic.arc。
func (s *Session) Close() error {
	// 已读标记是批量的：退出时补一次落盘，否则本次游玩读过的句子下次仍算未读。
	if s.readDirty {
		s.persistSystem()
	}
	if s.archives != nil {
		s.archives.Close()
		s.archives = nil
	}
	return nil
}

// Info 返回 Open 时确定的画布与标题。
func (s *Session) Info() core.Info {
	return core.Info{Width: frameWidth, Height: frameHeight, Title: s.title, Version: "1.0.0"}
}

// Start 按入口分派：entry==0 开标题菜单，entry>=1 直接跑脚本 entry。
func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if entry == 0 {
		s.atTitle = true
		s.galleryMode = galleryNone
		s.galleryStarted = false
		s.menu = newTitleMenu(s.cache)
		s.overlay = overlayNone
		s.msgPages = nil
		s.msgFull = ""
		s.msgPage = 0
		s.messageUnread = false
		s.choiceMenu = nil
		// 回标题时清空舞台，避免上局遗留的 fade/图层在下一局开头盖黑。
		s.stage.Restore(overstage.State{})
		s.playTitleBGM()
		s.step = core.StepAwaitTitle
		return core.StepAwaitTitle, nil
	}
	scriptName, ok := s.scriptForEntry(entry)
	if !ok {
		return core.StepFaulted, errBadEntry
	}
	s.atTitle = false
	s.menu = nil
	s.galleryMode = galleryNone
	s.galleryStarted = false
	s.overlay = overlayNone
	s.sceneReturnExtra = false
	s.msgPages = nil
	s.msgFull = ""
	s.msgPage = 0
	s.messageUnread = false
	s.choiceMenu = nil
	// 新游戏从干净舞台开始：上局的 fade/图层/立绘不能带进来。
	s.stage.Restore(overstage.State{})
	// 原版从标题进剧情前会 SysStopFade 掉菜单 BGM；不淡出的话 bgm01 会一直
	// 顶在剧情第一段 BGM 前，听起来像两首叠在一起。
	if s.audio != nil {
		s.audio.FadeOut(overaudio.Channel{Kind: overaudio.CategoryMusic, Idx: 0}, 1000)
	}
	s.currentScript = scriptName
	core.Logf("overdose: start entry=%d script=%s", entry, scriptName)
	if err := s.vm.Start(scriptName); err != nil {
		s.lastError = err.Error()
		return core.StepFaulted, err
	}
	return s.pump(commandBudget)
}

// scriptForEntry 把 Start(entry) 的数字入口映射到脚本名。
// entry=1 是 main.asb 对应的 "main"；其余依次走 list_script 表序。
// 表在 system.arc 里，Open 后按顺序取，表缺失时回退到 main。
func (s *Session) scriptForEntry(entry uint32) (string, bool) {
	if entry == 1 {
		return "main", true
	}
	if len(s.scriptOrder) == 0 {
		return "main", true
	}
	if int(entry)-1 < len(s.scriptOrder) {
		return s.scriptOrder[entry-1], true
	}
	return "", false
}

// Run 推进 VM 一轮预算并映射为 core.Step。
func (s *Session) Run(budget uint32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	b := int(budget)
	if b <= 0 {
		b = commandBudget
	}
	return s.pump(b)
}

// Advance 在等输入时推进一步。
// 注意：Flutter 的自动播放/打字机都直接走 Engine.Advance，不经过 Pointer，
// 所以“先补完当前页 / 再翻下一页 / 最后才推进脚本”必须放在这里，
// 否则自动模式会跳过未显示完的半句。
func (s *Session) Advance() (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	if s.overlay != overlayNone {
		return s.step, nil
	}
	// 选择肢是模态的：点击画面/自动推进都不能越过菜单，必须点具体项。
	if s.step == core.StepAwaitSelect {
		return s.step, nil
	}
	// 打字机：第一次点击补完整页；已完整则翻到下一页。
	if s.msgVisible && s.msgFull != "" {
		if !s.messagePageDone() {
			s.msgRevealAt = time.Time{} // 零值 = 立即全显
			return core.StepAwaitInput, nil
		}
		if s.msgPage+1 < len(s.msgPages) {
			s.msgPage++
			s.msgRevealAt = time.Now()
			return core.StepAwaitInput, nil
		}
	}
	if s.vm.Done() {
		return s.finishScript()
	}
	if err := s.vm.Advance(); err != nil {
		s.lastError = err.Error()
		return core.StepFaulted, err
	}
	return s.pump(commandBudget)
}

// finishScript 统一处理脚本结束：鉴赏场景回放结束后回场景页，
// 其余保持 StepFinished 交给宿主。
func (s *Session) finishScript() (core.Step, error) {
	if s.sceneReturnExtra {
		s.sceneReturnExtra = false
		s.atTitle = true
		s.menu = newTitleMenu(s.cache)
		s.overlay = overlayExtra
		s.extraTab = galleryScene
		s.overlayReturn = core.StepAwaitTitle
		s.playTitleBGM()
		s.step = core.StepAwaitTitle
		return core.StepAwaitTitle, nil
	}
	s.step = core.StepFinished
	return core.StepFinished, nil
}

// Choose 把选项下标交给 VM，VM 内部落入 op 0x1a 锁住的变量。
func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	if err := s.vm.Choose(int(index)); err != nil {
		s.lastError = err.Error()
		return core.StepFaulted, err
	}
	s.choiceMenu = nil
	s.hoverX, s.hoverY = -1, -1
	return s.pump(commandBudget)
}

// Pointer 处理所有指针语义。op 编号见 KzPointer：
// 0=move 1=press 2=release 3=tap 4=rightClick 5=cancelMap 6=drag。
//
// 关键修正：Flutter 触屏点击走的是 tap(3)，不是 press(1)。旧实现只认 1，
// 导致触屏在剧情里永远推不动对白——自动测试直接调 Advance，掩盖了这个 bug。
// 现在 1/3 统一按“点一下”处理；2/5 忽略；0 只更新悬停（本品牌不需要）。
func (s *Session) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	switch op {
	case 0, 2, 5:
		// 抬手/取消：tap 已经原子完成，这里不能再推进一次。
		if op == 0 {
			s.hoverX, s.hoverY = x, y
		}
		if op == 2 {
			s.configDrag = 0
		}
		return s.step, nil
	case 6:
		// 触屏纵向拖动：只给设置滑杆/回想滚动用，绝不推进对白。
		if s.overlay == overlayConfig {
			return s.dragConfig(x), nil
		}
		if s.overlay == overlayBacklog {
			return s.dragBacklog(y), nil
		}
		return s.step, nil
	case 4:
		return s.rightClickAt()
	}
	if op != 1 && op != 3 {
		return s.step, nil
	}

	// 覆盖层优先：模态界面上的点击绝不能穿透到剧情推进。
	if s.overlay != overlayNone {
		if s.overlay == overlayConfirm {
			if step, hit := s.pointerConfirmOverlay(x, y); hit {
				return step, nil
			}
			return s.step, nil
		}
		if step, hit := s.pointerOverlay(x, y); hit {
			return step, nil
		}
		return s.step, nil
	}

	if s.atTitle {
		if s.menu != nil {
			if key, hit := s.menu.hit(x, y); hit {
				return s.selectTitleItem(key)
			}
		}
		return core.StepAwaitTitle, nil
	}
	// 选择菜单在图层最上面，点击只认菜单项；点空白不会推进对白。
	if s.step == core.StepAwaitSelect {
		if pos, ok := s.hitChoiceMenu(x, y); ok {
			return s.Choose(uint32(pos))
		}
		return s.step, nil
	}
	// 对话框工具栏优先于点画面推进：按钮在 y=574，不能让它同时触发下一句。
	if s.msgVisible {
		if key, ok := s.hitAdvBar(x, y); ok {
			return s.selectAdvBar(key)
		}
	}
	return s.Advance()
}

// pointerOverlay 把点击分发给当前覆盖层。返回 hit=false 表示点空了，
// 调用方不再做任何事（不能退化成“点画面推进”）。
func (s *Session) pointerOverlay(x, y int32) (core.Step, bool) {
	switch s.overlay {
	case overlaySave, overlayLoad:
		return s.pointerSaveOverlay(x, y)
	case overlayConfig:
		return s.pointerConfigOverlay(x, y)
	case overlayBacklog:
		return s.pointerBacklogOverlay(x, y)
	case overlayExtra:
		return s.pointerExtraOverlay(x, y)
	case overlayCGView:
		return s.pointerCGViewOverlay(x, y)
	}
	return s.step, false
}

// rightClickAt 实现 sq_adv.OnMouseClickRight 的三种模式。
func (s *Session) rightClickAt() (core.Step, error) {
	if s.overlay != overlayNone {
		if s.overlay == overlayConfirm {
			// 确认框右键等同“否”。
			if step, hit := s.pointerConfirmOverlay(int32(menus.ConfirmLayoutFor().NoRect.X+1),
				int32(menus.ConfirmLayoutFor().NoRect.Y+1)); hit {
				return step, nil
			}
			return s.step, nil
		}
		if s.overlay == overlayCGView {
			// 原版 CG 全屏右键是 task.Pop：回到 CG 鉴赏页而不是标题。
			s.overlay = overlayExtra
			s.extraTab = galleryCG
			s.step = core.StepAwaitTitle
			return s.step, nil
		}
		return s.closeOverlay(), nil
	}
	if s.atTitle {
		return s.step, nil
	}
	switch s.settings.ClickRightMode {
	case 1:
		// 原版 mode1 是“截图后关窗”（sq_wm_close）：只是把消息窗收起来，
		// 不是退出确认。下一句对白会由 setMessage 重新打开。
		s.msgVisible = false
		return s.step, nil
	case 3:
		if s.lastVoice != "" && s.audio != nil {
			s.audio.Play(overaudio.PlayRequest{
				Channel: overaudio.Channel{Kind: overaudio.CategoryVoice, Idx: 0},
				Name:    s.lastVoice,
			})
		}
		return s.step, nil
	default:
		ret := s.step
		if ret == core.StepContinue || ret == core.StepFinished || ret == core.StepFaulted {
			ret = core.StepAwaitInput
		}
		return s.openOverlay(overlayConfig, ret), nil
	}
}

// Compose 把当前画面画到 dst（800x600 BGRA）。
// 标题态画 title_bg，剧本态走 stage.Compose；覆盖层在两者之上自绘。
// 所有覆盖层都由引擎画进帧里，Flutter 只负责把点击坐标送回来。
func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if dst == nil || n < frameWidth*frameHeight*4 {
		return false, nil
	}
	canvas := s.frame
	if canvas == nil {
		return false, nil
	}
	canvas.Clear(0, 0, 0, 255)
	if s.atTitle {
		s.composeTitle(canvas)
	} else {
		// Tick 是 stage 的时钟：转场/fade 全靠每帧推进。用真实帧间隔而不是
		// 固定 16ms——宿主最高 30fps 合成，固定值会把 1000ms 转场拖成 2 秒。
		dt := int64(16)
		now := time.Now()
		if !s.lastTick.IsZero() {
			if ms := now.Sub(s.lastTick).Milliseconds(); ms > 0 {
				dt = ms
			}
		}
		s.lastTick = now
		if dt > 100 {
			dt = 100
		}
		s.stage.Tick(int(dt))
		s.stage.Compose(canvas)
		if s.overlay == overlayNone {
			s.composeMessageWindow(canvas)
			if s.step == core.StepAwaitSelect {
				s.composeChoiceMenu(canvas)
			}
		}
	}
	// 覆盖层：确认框可以叠在存/读档或剧情上，所以单独在最后画。
	switch s.overlay {
	case overlaySave, overlayLoad:
		s.composeSaveOverlay(canvas)
	case overlayConfig:
		s.composeConfigOverlay(canvas)
	case overlayBacklog:
		s.composeBacklogOverlay(canvas)
	case overlayExtra:
		s.composeExtraOverlay(canvas)
	case overlayCGView:
		s.composeCGViewOverlay(canvas)
	case overlayConfirm:
		s.composeConfirmOverlay(canvas)
	}
	// 宿主预览（gallery 的 SetPreview 落到 preview 字段）叠上图集缩略图区。
	if s.preview.Width > 0 && s.overlay == overlayNone && !s.atTitle {
		s.drawPreview(canvas)
	}
	buf := unsafe.Slice((*byte)(dst), n)[:frameWidth*frameHeight*4]
	copy(buf, canvas.Buffer)
	s.textRuns = canvas.TakeRuns()
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

// composeTitle 画标题底图 + 五按钮，再按需要叠覆盖层。
// 注意覆盖层不画 title 按钮：与 Flutter TitleLayer 的规则一致——
// 覆盖层打开时 TitleMenu() 返回空，按钮热区随之消失。
func (s *Session) composeTitle(canvas *bishopgfx.BgraCanvas) {
	if s.overlay == overlayNone || s.overlay == overlayConfirm {
		if img := s.cache.Get("title_bg"); img != nil {
			canvas.Blit(img, 0, 0, 255)
		}
		if s.menu != nil {
			for _, it := range s.menu.items {
				name := "title_" + it.Key
				if it.Key == "quit" {
					name = "title_exit"
				}
				if img := s.cache.Get(name); img != nil {
					canvas.Blit(img, int(it.X), int(it.Y), 255)
				}
			}
		}
	}
}

// drawPreview 把 preview（宿主图片）画进标题画廊的固定缩略区里。
func (s *Session) drawPreview(canvas *bishopgfx.BgraCanvas) {
	if s.preview.Width <= 0 || s.preview.Height <= 0 || len(s.preview.Pixels) < s.preview.Width*s.preview.Height*4 {
		return
	}
	img := bishopgfx.NewBsgImage(s.preview.Width, s.preview.Height, 0, 0, s.preview.Pixels)
	canvas.Blit(img, 200, 100, 255)
}

// ---------------- core 状态/文本 ----------------

// Text 返回当前停点对白；不在停点时保持最后一句（宿主打字机所需）。
func (s *Session) Text() core.TextState {
	aw := uint32(0)
	if s.step == core.StepAwaitInput {
		aw = 1
	}
	return core.TextState{Speaker: s.speaker, Text: s.text, Awaiting: aw}
}

// Choices 返回 VM 当前选项（仅在 StepAwaitChoice 有效，其余置空）。
func (s *Session) Choices() []core.Choice {
	if s.step != core.StepAwaitChoice {
		return nil
	}
	return append([]core.Choice(nil), s.choices...)
}

// State 是宿主每帧的快照：PC 取 VM 栈顶字节偏移（op 0x04/0x05 的 field 口径）。
func (s *Session) State() core.State {
	var pc uint32
	if s.vm != nil {
		stack := s.vm.SnapshotStack()
		if len(stack) > 0 {
			pc = uint32(stack[len(stack)-1].Offset)
		}
	}
	bgm := s.bgmFile
	if s.audio != nil {
		if snap := s.audio.Snapshot(); snap.BgmFile != "" {
			bgm = snap.BgmFile
		}
	}
	// 动画位必须同时覆盖舞台转场与打字机：宿主只在 Animating=true 时持续
	// 请求合成；漏掉任一项，画面会停在第一帧等下一次点击。
	animating := !s.atTitle && s.msgVisible && s.msgFull != "" && !s.messagePageDone()
	if s.stage != nil && !s.atTitle && s.stage.Animating() {
		animating = true
	}
	return core.State{
		Bgm:                  bgm,
		PC:                   pc,
		Animating:            animating,
		MessageWindowVisible: s.msgVisible,
		WaitMs:               s.waitMs,
		// 对话框由引擎画进帧里（adv_msg_wnd + 打字机）：Flutter 必须隐藏
		// 自己的 MessageBox，否则会叠出第二个黑框。标题/覆盖层不画对话框，
		// 但此时宿主也不该显示 MessageBox，所以只按“是否在剧情里”判断。
		EngineMessageWindow: !s.atTitle,
		MessageUnread:       s.messageUnread,
		CeaseSkip:           s.settings.SkipEnabled,
		SkipUnread:          s.settings.SkipReadOnly,
		AutoWait:            int(s.settings.AutoDelayMs),
		HostRequest:         s.hostRequest,
		HostRequestSerial:   s.hostRequestSerial,
	}
}

// TextList 返回 backlog（每句一行，带文件/行号）。
func (s *Session) TextList() []core.TextUnit {
	out := make([]core.TextUnit, len(s.backlog))
	copy(out, s.backlog)
	return out
}

// Cell 取 backlog 第 i 条的原文（供 gallery/预览按序取用）。
func (s *Session) backlogCell(i int) core.TextUnit {
	if i < 0 || i >= len(s.backlog) {
		return core.TextUnit{}
	}
	return s.backlog[i]
}

// LastError 返回最近一次引擎内错误串。
func (s *Session) LastError() string { return s.lastError }

// Audio 直接转发 audio.Player 的待发事件（绝对路径已在 Play 时物化）。
func (s *Session) Audio() []core.AudioEvent {
	if s.audio == nil {
		return nil
	}
	return s.audio.Events()
}

// ---------------- 资产 ----------------

// AssetSize 返回 arc 条目的存储长度（未解包口径）。
func (s *Session) AssetSize(path string) int {
	if s.archives == nil {
		return 0
	}
	size, ok := s.archives.EntrySize(path)
	if !ok {
		return 0
	}
	if size > int64(1<<62) {
		return 0
	}
	return int(size)
}

// ReadAsset 把条目字节原样写到 dst（音频等宿主拉取用）。
func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if dst == nil || n <= 0 || s.archives == nil {
		return 0
	}
	raw, err := s.archives.Entry(path)
	if err != nil {
		return 0
	}
	return copy(unsafe.Slice((*byte)(dst), n), raw)
}

// ---------------- 扩展接口 ----------------

// Scenes 把 scene_pageNN 表拼成 core.Scene 列（顺序即页面+行序）。
func (s *Session) Scenes() []core.Scene {
	return buildScenes(s.archives, s.unlockSets.scenes)
}

// Slots 列出 saveSlotCount 个槽位（10 个，1..10）。
func (s *Session) Slots() []core.Slot {
	out := make([]core.Slot, 0, saveSlotCount)
	for i := 1; i <= saveSlotCount; i++ {
		slot := core.Slot{Index: uint32(i)}
		if s.saves == nil {
			out = append(out, slot)
			continue
		}
		data, err := s.saves.Load(i)
		if err != nil || data == nil {
			out = append(out, slot)
			continue
		}
		slot.Exists = true
		slot.Title = data.Title
		slot.Summary = data.Summary
		if !data.SavedAt.IsZero() {
			slot.SavedAt = data.SavedAt.Format(time.RFC3339)
		}
		out = append(out, slot)
	}
	return out
}

// Save 落存档：VM 栈/变量/stage/音频/消息五件套原子写盘（save.Store 负责落盘）。
func (s *Session) Save(slot uint32, title string) error {
	if s.vm == nil || s.saves == nil {
		return errNotOpen
	}
	if s.step == core.StepAwaitSelect || s.step == core.StepAwaitChoice {
		return fmt.Errorf("overdose: 选择肢上不能存档")
	}
	if s.readDirty {
		s.persistSystem()
	}
	data := &oversave.SlotData{
		Title:     title,
		Summary:   s.summarize(),
		SavedAt:   time.Now(),
		Script:    s.currentScript,
		PC:        int(s.currentPC()),
		Variables: oversave.ExportVariables(s.vm.Variables()),
		Message:   oversave.MessageState{Speaker: s.speaker, Text: s.text},
	}
	for _, fr := range s.vm.SnapshotStack() {
		data.Stack = append(data.Stack, oversave.FrameState{Script: fr.Script, Offset: fr.Offset})
	}
	st := s.stage.State()
	data.Stage = oversave.StageState{
		Background:     st.Background,
		CG:             st.CG,
		MessageVisible: s.msgVisible,
	}
	for _, ch := range st.Characters {
		data.Stage.Characters = append(data.Stage.Characters, oversave.LayerState{
			Name:  ch.Name,
			X:     int32(ch.X),
			Alpha: int32(ch.Alpha),
		})
	}
	for _, layer := range st.Layers {
		data.Stage.Layers = append(data.Stage.Layers, oversave.LayerState{
			Index: int32(layer.Index), Name: layer.Name,
			X: int32(layer.X), Y: int32(layer.Y), Alpha: int32(layer.Alpha),
			Scale: layer.Scale, Visible: layer.Visible, Centered: layer.Centered,
		})
	}
	// 音频只存 BGM 单槽：SE/voice 不跨帧保持，存了也无法恢复原声。
	if snap := s.audio.Snapshot(); snap.BgmFile != "" {
		data.Audio = oversave.AudioState{BgmFile: snap.BgmFile, BgmLoop: snap.BgmLoop, BgmVol: snap.BgmVol}
	}
	core.Logf("overdose: save slot %d script=%s pc=%d", slot, s.currentScript, data.PC)
	return s.saves.Save(int(slot), data)
}

// Load 从存档恢复 VM 栈、变量、stage、音频并停在等输入。
func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.vm == nil || s.saves == nil {
		return core.StepFaulted, errNotOpen
	}
	data, err := s.saves.Load(int(slot))
	if err != nil {
		return core.StepFaulted, err
	}
	if data == nil {
		return core.StepFaulted, errNoSlot
	}
	// 变量用新 VariableStore 导入，避免旧值残留；新 VM 用 NewWithStore 挂上同一表。
	vs := &overscript.VariableStore{}
	oversave.ImportVariables(vs, data.Variables)
	s.vm = overscript.NewWithStore(s.loadScriptEntry, vs)
	s.vm.SetMacroHook(func(name string, args []overscript.Value) error {
		_ = overstage.HandleMacroEvent(s.stage, s.vm.Variables(), name, args)
		return nil
	})
	// 直接 Restore VM 栈（偏移经 instructionAt 校验，跨版本错位会拒读）。
	vmState := overscript.VMState{Done: false}
	for _, fr := range data.Stack {
		vmState.Stack = append(vmState.Stack, overscript.StackFrame{Script: fr.Script, Offset: fr.Offset})
	}
	if err := s.vm.Restore(vmState); err != nil {
		return core.StepFaulted, err
	}
	s.currentScript = data.Script
	core.Logf("overdose: load slot %d script=%s", slot, data.Script)
	s.atTitle = false
	s.menu = nil
	// stage 恢复：背景/CG/人物三件套。
	st := overstage.State{Background: data.Stage.Background, CG: data.Stage.CG}
	for _, ch := range data.Stage.Characters {
		st.Characters = append(st.Characters, overstage.Slot{Name: ch.Name, X: int(ch.X), Alpha: int(ch.Alpha)})
	}
	for _, layer := range data.Stage.Layers {
		st.Layers = append(st.Layers, overstage.OverlayLayer{
			Index: int(layer.Index), Name: layer.Name,
			X: int(layer.X), Y: int(layer.Y), Alpha: int(layer.Alpha),
			Scale: layer.Scale, Visible: layer.Visible, Centered: layer.Centered,
		})
	}
	s.stage.Restore(st)
	s.speaker = data.Message.Speaker
	// 读档后当前句已经“显示过”，不能重播打字机：直接落成全显。
	s.setMessageImmediate(data.Message.Text)
	s.msgVisible = true
	s.messageUnread = false
	s.choiceMenu = nil
	s.overlay = overlayNone
	s.sceneReturnExtra = false
	s.step = core.StepAwaitInput
	// BGM 恢复单槽（不重发 Play，由宿主自行续播）。
	if data.Audio.BgmFile != "" {
		s.audio.Restore(overaudio.SaveState{BgmFile: data.Audio.BgmFile, BgmLoop: data.Audio.BgmLoop, BgmVol: data.Audio.BgmVol})
		s.bgmFile = data.Audio.BgmFile
	}
	return core.StepAwaitInput, nil
}

// ---------------- 事件泵 ----------------

// pump 跑 VM 并把事件映射成 core.Step。
func (s *Session) pump(budget int) (core.Step, error) {
	if s.vm.Done() {
		return s.finishScript()
	}
	_, err := s.vm.Step(budget, handler{s: s})
	if err != nil {
		s.lastError = err.Error()
		s.step = core.StepFaulted
		return core.StepFaulted, err
	}
	if s.vm.Done() {
		return s.finishScript()
	}
	// VM blocked 时按事件给出的 stopkind 决定 core step。
	switch {
	case s.step == core.StepAwaitChoice:
		return core.StepAwaitChoice, nil
	case s.step == core.StepAwaitSelect:
		return core.StepAwaitSelect, nil
	case s.step == core.StepAwaitInput:
		return core.StepAwaitInput, nil
	default:
		// wait 类事件已由 handler 记成 StepWait，blocked 兜底按等输入。
		if s.step == core.StepWait {
			return core.StepWait, nil
		}
		s.step = core.StepAwaitInput
		return core.StepAwaitInput, nil
	}
}

// handler 是 script.EventHandler 的适配：VM 只产事件，session 落舞台/音频/文本。
type handler struct{ s *Session }

func (h handler) HandleEvent(e overscript.Event) error {
	s := h.s
	switch e.Kind {
	case overscript.EventDialogue:
		if e.Text != "" || e.Speaker != "" {
			s.speaker = e.Speaker
			file, line := s.currentPosition()
			// 已读判定必须在 setMessage 之前做：同一个脚本偏移只算一次。
			// 键用 script#offset（VM 的字节偏移），与存档里的 PC 同口径。
			key := file + "#" + line
			s.unlockSets.ensure()
			s.messageUnread = !s.unlockSets.set[key]
			if s.messageUnread {
				s.unlockSets.set[key] = true
				s.readDirty = true
			}
			s.setMessage(e.Text)
			s.step = core.StepAwaitInput
			// Flutter 的 BacklogView 把 ID 当标题（说话人），Text 当正文，
			// 位置放 File；把三者拼进 Text 会让回想里出现 "文本|名字|偏移"。
			s.backlog = append(s.backlog, core.TextUnit{
				ID:   e.Speaker,
				Text: e.Text,
				File: file + ":" + line,
			})
		}
	case overscript.EventChoice:
		s.choices = s.choices[:0]
		for _, c := range e.Choices {
			s.choices = append(s.choices, core.Choice{Name: c.Text, Value: c.Index})
		}
		s.setChoiceMenu(e.Choices)
		if len(e.Choices) == 0 {
			// 空菜单（脚本异常）不能把宿主卡在“等选择”上。
			s.step = core.StepAwaitInput
		} else {
			// 统一走 AwaitSelect：两种菜单都由引擎画，Flutter 不叠居中列表。
			s.step = core.StepAwaitSelect
		}
	case overscript.EventBGM:
		s.audio.Play(overaudio.PlayRequest{
			Channel: overaudio.Channel{Kind: overaudio.CategoryMusic, Idx: e.Channel},
			Name:    e.File,
			Loop:    e.Loop,
			Volume:  s.scaleVolume(overaudio.CategoryMusic, e.Volume),
		})
		s.bgmFile = e.File
		s.unlock("bgm", e.File)
	case overscript.EventSE:
		s.audio.Play(overaudio.PlayRequest{
			Channel: overaudio.Channel{Kind: overaudio.CategorySound, Idx: e.Channel},
			Name:    e.File,
			Loop:    e.Loop,
			Volume:  s.scaleVolume(overaudio.CategorySound, e.Volume),
		})
	case overscript.EventVoice:
		// 角色别ボイス設定：syu*/sky*/smi_* 分别是 YUI/KYOKO/MIYUKI。
		// 关闭某角色后，原版在引擎音频层直接不播该角色的语音。
		if !s.voiceAllowed(e.File) {
			return nil
		}
		s.lastVoice = e.File
		s.audio.Play(overaudio.PlayRequest{
			Channel: overaudio.Channel{Kind: overaudio.CategoryVoice, Idx: e.Channel},
			Name:    e.File,
			Loop:    e.Loop,
			Volume:  s.scaleVolume(overaudio.CategoryVoice, e.Volume),
		})
	case overscript.EventWait:
		if e.WaitMs > 0 {
			s.waitMs = uint32(e.WaitMs)
			s.step = core.StepWait
		} else {
			s.waitMs = 0
		}
	case overscript.EventMacro:
		// 舞台效果已经由 VM 的 MacroHandler 调 stage.HandleMacroEvent 落地；
		// 这里绝不能再调一次，否则同样的 crossfade/fade 会被叠两遍。
		// 同时补上宏体内部的 msgon/msgoff：我们不执行 macro.asb，但 bg/ev
		// 宏体末尾都会 msgon，暗転/白転/场景切换开头会 msgoff。
		switch e.Text {
		case "msgon", "bg", "ev", "evd":
			s.msgVisible = true
		case "msgoff", "暗転", "白転", "scenestart", "map_close", "場面転換閉", "時間経過閉", "終端", "終端短":
			s.msgVisible = false
		}
	case overscript.EventLayerSet:
		s.stage.SetOverlayLayer(e.Layer, e.File, int(e.X), int(e.Y), int(e.Alpha), e.Scale, false)
	case overscript.EventLayerClear:
		s.stage.ClearOverlayLayer(e.Layer)
	case overscript.EventRaw:
		// 未识别 opcode/表现指令只记一次类型行，避免日志被同一条刷屏。
		if e.Text == "unknown" {
			core.Logf("overdose: %s", e.Raw)
		}
	case overscript.EventFinished:
		s.step = core.StepFinished
	}
	return nil
}

// currentPosition 返回当前脚本名与指令序号（backlog 的 file/line）。
func (s *Session) currentPosition() (string, string) {
	stack := s.vm.SnapshotStack()
	if len(stack) == 0 {
		return s.currentScript, "0"
	}
	top := stack[len(stack)-1]
	// 字节偏移直接以十进制写进 backlog：宿主只按序显示，不做语义解析。
	return top.Script, fmt.Sprintf("%d", top.Offset)
}

func (s *Session) currentPC() uint32 {
	stack := s.vm.SnapshotStack()
	if len(stack) == 0 {
		return 0
	}
	return uint32(stack[len(stack)-1].Offset)
}

func (s *Session) summarize() string {
	text := strings.TrimSpace(s.text)
	if s.speaker != "" {
		text = s.speaker + ": " + text
	}
	return text
}

// ---------------- 系统文件 ----------------

type galleryUnlocks struct {
	cg     map[string]bool
	scenes map[string]bool
	bgm    map[string]bool
	set    map[string]bool
}

func (g *galleryUnlocks) ensure() {
	if g.cg == nil {
		g.cg = map[string]bool{}
		g.scenes = map[string]bool{}
		g.bgm = map[string]bool{}
		g.set = map[string]bool{}
	}
}

// loadSystem 加载 save 系统文件；没有就全空（全锁是正确默认）。
func (s *Session) loadSystem() {
	s.unlockSets.ensure()
	if s.saves == nil {
		return
	}
	data, err := s.saves.System().Load()
	if err != nil || data == nil {
		return
	}
	for _, n := range data.CgUnlocks {
		s.unlockSets.cg[n] = true
	}
	for _, n := range data.SceneUnlocks {
		s.unlockSets.scenes[n] = true
	}
	for _, n := range data.MusicUnlocks {
		s.unlockSets.bgm[n] = true
	}
	for _, n := range data.ReadTextKeys {
		s.unlockSets.set[n] = true
	}
	s.settings = oversave.NormalizeSettings(data.Settings)
}

// noteScript 记录“这个脚本文件已经跑过”，供场景鉴赏的已读判定使用。
// 原版 GetFlagScriptAll 按脚本文件+选择支标记；我们没有逐条选择支覆盖，
// 用“脚本被加载过”作为近似，并把键记进系统文件的 ReadTextKeys（加 script: 前缀），
// 避免与对白已读键冲突。同一脚本只落盘一次。
func (s *Session) noteScript(name string) {
	name = strings.TrimSuffix(name, ".asb")
	if name == "" || name == "macro" {
		return
	}
	s.unlockSets.ensure()
	key := "script:" + name
	if s.unlockSets.set[key] {
		return
	}
	s.unlockSets.set[key] = true
	core.Logf("overdose: load script %s", name)
	s.persistSystem()
}

// unlock 记一类解锁并落盘系统文件（多次解锁不过度写盘：同一名只记一次）。
func (s *Session) unlock(kind, name string) {
	if name == "" {
		return
	}
	s.unlockSets.ensure()
	var set map[string]bool
	switch kind {
	case "bgm":
		set = s.unlockSets.bgm
	case "cg":
		set = s.unlockSets.cg
	case "scene":
		set = s.unlockSets.scenes
	default:
		set = s.unlockSets.set
	}
	if set[name] {
		return
	}
	set[name] = true
	s.persistSystem()
}

// persistSystem 写系统文件；时间戳用 unix 秒，旧解锁不丢只增。
func (s *Session) persistSystem() {
	if s.saves == nil {
		return
	}
	s.settings.Initialized = true
	data := &oversave.SystemData{
		CgUnlocks:    keysOf(s.unlockSets.cg),
		SceneUnlocks: keysOf(s.unlockSets.scenes),
		MusicUnlocks: keysOf(s.unlockSets.bgm),
		ReadTextKeys: keysOf(s.unlockSets.set),
		Settings:     s.settings,
	}
	_ = s.saves.System().Save(data)
	s.readDirty = false
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// splitVarKey 把 save 的 "ns:idx" 键拆开（Load 中的变量表对称）。
func splitVarKey(key string) (string, string, bool) {
	i := strings.Index(key, ":")
	if i < 0 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

// SetOption 落设置（宿主的选项面板经此回写系统文件）。
func (s *Session) SetOption(key uint32, value bool) {
	// 过度映射会把未来 key 错位成别的开关：只认已知的三项。
	switch key {
	case 1:
		s.settings.SkipEnabled = value
	case 2:
		s.settings.SkipReadOnly = value
	case 3:
		s.settings.AutoEnabled = value
	}
	s.persistSystem()
}

// CurrentStep 报告当前步（OpFrame 轮询用）。
func (s *Session) CurrentStep() core.Step { return s.step }
