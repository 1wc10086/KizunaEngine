// Package session 是 TMR-Hiro 的 core.Engine 实现。
//
// 分层与其它品牌一致：本包只做“接线与状态”，pac/grd/script/vm/config/audio/
// stage/save 各自独立可测。所有方法在 capi 侧由 handle.mu 串行化，这里不额外
// 加锁（grd 缓存与 stage 的宏表确实有互斥，但那是因为它们也被合成线程读）。
//
// 与原版的对应关系：
//   - start（srp.pac 的 `start`）= 启动器；
//   - `00_00` 起是章节主脚本，`00_01` 起是章节内分段，`01_xx` 是路线分支；
//   - 标题菜单由脚本自己画（title3/title3_on + _system 的 TWB_* 坐标），
//     这里只把“可选的项目与热区”结构化交给宿主，点击仍回到脚本的标签。
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/tmrhiro/audio"
	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/brand/tmrhiro/grd"
	"kizuna/enginego/brand/tmrhiro/pac"
	"kizuna/enginego/brand/tmrhiro/save"
	"kizuna/enginego/brand/tmrhiro/stage"
	"kizuna/enginego/brand/tmrhiro/vm"
	"kizuna/enginego/core"
)

const (
	frameWidth  = stage.FrameWidth
	frameHeight = stage.FrameHeight
	// commandBudget 与其它品牌同量级：一轮 Run 只推进有限指令。
	commandBudget = 4096
	// saveSlots 是原版 SAVE_COUNT_MAX=10 × SAVE_PAGE_MAX=10 的总槽位。
	saveSlots = 100
	// quickSlot 是 QSAVE 的固定槽（原版快捷键存档，不列在存档页里）。
	quickSlot = 900
	// entryStart 是启动脚本名（srp.pac 里的 `start`）。
	entryStart = "start"
	// titleScript 是标题脚本（`start` 的 TITLE2 标签由它驱动）。
	titleScript = "start_title"
)

var (
	errNotOpen = fmt.Errorf("tmrhiro: 引擎未打开")
	errNoSlot  = fmt.Errorf("tmrhiro: 槽位无档")
)

// Session 是单局运行态。
type Session struct {
	dir      string
	cacheDir string
	set      *pac.Set
	cache    *grd.Cache
	stage    *stage.Stage
	vm       *vm.VM
	store    *save.Store
	audio    *audio.Materializer
	textRuns []core.TextRun

	system *config.Ini
	// ui 是 _system 解析出的界面布局（消息框/工具条/存读档/设置/鉴赏）。
	// 与 system 并存：system 保留原始键值（音量默认值、字数上限等仍在用），
	// ui 只保存"已经按字段顺序解释过的结构"，避免每个绘制点重复拆字符串。
	ui        *config.UILayout
	macros    map[string][]string // fm_Z01_0001 → 部件
	charas    map[string][]string // fm_C01a_01_0_0_0_01 → 部件（立绘宏）
	cgList    []string
	sceneList []string

	title   string
	chapter string
	step    core.Step
	lastErr string

	// atTitle 表示停在标题（Start(0)）；playing 表示在剧情里。
	atTitle bool

	speaker string
	text    string
	bgm     string
	voice   string
	choices []core.Choice
	// choiceHover 是选项菜单当前悬停的行（-1 表示没有）。
	choiceHover int

	// 标题菜单：由脚本（`start` 的 0x92/0x23/0x10）与 AGRD 的 title3 贴图驱动。
	titleMenu  []core.TitleMenuItem
	titleBase  string
	titleHover string
	hoveredKey string
	// 消息框可见与未读标记（宿主用来决定是否隐藏自己的对话框）。
	messageVisible bool
	messageUnread  bool
	// 逐字显示进度：revealedText 是当前已显示的前缀，revealAt 是本句起点。
	// revealDone 表示整句已显示完（催促图标从这时才画）。
	revealedText string
	revealAt     time.Time
	revealRunes  int
	revealDone   bool
	showPrompt   bool
	// lastMessageWasRead 决定正文用 TEXT_COLOR 还是 TEXT_READ_COLOR。
	lastMessageWasRead bool
	// hoveredToolbar 是底部工具条当前悬停的按钮下标（-1 表示没有）。
	hoveredToolbar int
	// chara 是立绘鉴赏器的运行态（角色/服装/表情/缩放）。
	chara charaView
	// screen 是脚本定义的界面（鉴赏/立绘/选择/确认），由 vm.ScreenState 同步。
	screen      *vm.Screen
	screenHover int
	// overlay 是引擎自绘的模态界面（暂停菜单/存读档/设置/回想/确认）。
	overlay overlayKind
	// overlaySaveMode 区分存/读档界面（同一套布局，动作不同）。
	overlaySaveMode bool
	// 界面内的分页与选中项。
	overlayPage  int
	overlayHover int
	// overlayHoverX/Y 是最后一次指针位置（界面悬停反馈用）。
	overlayHoverX int
	overlayHoverY int
	overlayScroll int
	// confirm 是确认框的上下文（存/读/删档/退出游戏/达成率）。
	confirm *confirmState
	// 界面里的小状态：提示条、滚轮拖动基准 y、鉴赏解锁集合、设置开关。
	notice        string
	noticeAt      time.Time
	backlogDragY  int
	unlockedCG    map[string]bool
	unlockedScene map[int]bool
	effects       bool
	autoVoiceCut  bool
	readColorOn   bool
	vulgarWord    bool
	// windowBaseDraw 是「テキストウィンドウのベースを表示する」开关：
	// 关掉时原版只留文字、不画对话框底图。
	windowBaseDraw bool
	// systemChoices 是设备类选项（右键行为、窗口模式、确认框…）的选中档位。
	// 这些在触屏移植里没有可观察行为，但设置页要能回显玩家点的位置。
	systemChoices map[string]int
	// openedAt 是本次打开游戏的时刻（存档占位日期用）。
	openedAt time.Time
	// waitMs 是脚本 `op5/0x82` 的等待（走时间而不是点击）。
	waitMs    uint32
	waitUntil time.Time

	// 名字输入（NAME）：原版的系统对话框，优先于其它界面。
	nameEntry nameEntry

	// hostRequest 是引擎请求宿主执行的界面动作（settings/title/gallery/exit）。
	hostRequest       string
	hostRequestSerial uint32

	// 音频：本步 pending，宿主每次 opAudio 取走。
	pending []core.AudioEvent
	// bgmVolume/seVolume/voiceVolume 来自 SYS_DEF_SOUND_VOLUME（65/90/80/80）。
	bgmVolume, seVolume, voiceVolume float64
	// autoSpeed/textSpeed 来自 SYS_DEF_AUTOSPEED/TEXTSPEED（0..10）。
	autoSpeed, textSpeed int
	skipUnread           bool
	ceaseSkip            bool
	autoWait             int
	// autoActive 是宿主的 AUTO 状态（原版 AUTO 也加速正文与停顿）。
	autoActive bool

	// preview 是宿主传来的鉴赏预览图（core.GalleryComposer）。
	preview core.Frame

	// backlog 是已读文本（回想界面用）。
	backlog    []core.TextUnit
	backlogSeq uint32
	// seen 是从原版全局存档继承的已读集合（目前只保留在本会话）。
	settingsPath string
	dirty        bool
}

// New 建 Session。
func New() *Session { return &Session{} }

// Open 打开游戏目录。
func (s *Session) Open(dir string) (core.Info, error) { return s.OpenExtras(dir, nil, "") }

// OpenExtras 打开游戏目录（额外资源根与缓存目录由宿主传入）。
func (s *Session) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	gameDir, ok := pac.FindGameDir(dir)
	if !ok {
		return core.Info{}, fmt.Errorf("tmrhiro: 找不到 srp.pac（%s）", dir)
	}
	set, err := pac.OpenSet(gameDir)
	if err != nil {
		return core.Info{}, err
	}
	if s.set != nil {
		s.set.Close()
	}
	s.dir = gameDir
	s.set = set

	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "kizuna-tmrhiro")
	}
	if abs, err := filepath.Abs(cacheDir); err == nil {
		cacheDir = abs
	}
	s.cacheDir = core.GameCacheDir(cacheDir, gameDir)
	if s.cacheDir == "" {
		s.cacheDir = filepath.Join(cacheDir, "tmrhiro")
	}
	_ = os.MkdirAll(filepath.Join(s.cacheDir, "audio"), 0o755)
	s.audio = audio.NewMaterializer(filepath.Join(s.cacheDir, "audio"))

	s.cache = grd.NewCache(func(name string) ([]byte, error) { return s.set.Entry(name) })
	s.stage = stage.NewStage(s.cache)

	s.loadConfigs()
	s.loadMacros()

	s.vm = vm.New(func(name string) ([]byte, error) { return s.set.Entry(name) }, entryStart)

	// 存档目录放在游戏目录下（不可写时退回缓存），与其它品牌一致。
	saveDir := gameDir
	if !dirWritable(gameDir) {
		saveDir = s.cacheDir
	}
	s.store = save.New(filepath.Join(saveDir, "kizuna-save"))
	s.settingsPath = filepath.Join(saveDir, "kizuna-settings.json")
	s.loadSettings()

	s.openedAt = time.Now()
	s.atTitle = true
	s.unlockedCG = make(map[string]bool)
	s.unlockedScene = make(map[int]bool)
	s.step = core.StepAwaitTitle
	s.title = "TMR-Hiro"
	s.messageVisible = false
	s.dirty = true
	s.buildTitleMenu()

	core.Logf("tmrhiro: open %s (%d archives, %d entries)", gameDir, len(set.Archives()), set.TotalEntries())
	return core.Info{
		Width:      frameWidth,
		Height:     frameHeight,
		Title:      s.title,
		Version:    "1.0.0",
		EntryCount: set.TotalEntries(),
	}, nil
}

// Close 释放归档缓存。
func (s *Session) Close() error {
	if s.set != nil {
		s.set.Close()
		s.set = nil
	}
	if s.cache != nil {
		s.cache.Purge()
	}
	return nil
}

// Info 返回静态信息。
func (s *Session) Info() core.Info {
	title := s.title
	if title == "" {
		title = "TMR-Hiro"
	}
	entries := 0
	if s.set != nil {
		entries = s.set.TotalEntries()
	}
	return core.Info{Width: frameWidth, Height: frameHeight, Title: title, Version: "1.0.0", EntryCount: entries}
}

// CurrentStep 供 OpFrame 读当前步。
func (s *Session) CurrentStep() core.Step { return s.step }

// Start 按入口启动：0=标题，1=正篇（start），2+=标题。
func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.vm == nil || s.set == nil {
		return core.StepFaulted, errNotOpen
	}
	switch entry {
	case 0:
		return s.startTitle()
	case 1:
		return s.startGame()
	default:
		return s.startTitle()
	}
}

func (s *Session) startTitle() (core.Step, error) {
	s.openedAt = time.Now()
	s.atTitle = true
	s.unlockedCG = make(map[string]bool)
	s.unlockedScene = make(map[int]bool)
	s.step = core.StepAwaitTitle
	s.messageVisible = false
	s.text = ""
	s.speaker = ""
	s.hostRequest = ""
	// 标题菜单必须由脚本生成：`start` 前半段负责品牌页面与标题底图，
	// 随后的 0x82/0x92/0x23/0x10 才是菜单定义。不跑脚本就画不出菜单，
	// 宿主只能看到一张没有按钮的底图。
	if s.vm != nil {
		if err := s.vm.Start(entryStart); err != nil {
			s.lastErr = err.Error()
		} else if step := s.vm.Run(commandBudget); step != vm.StepAwaitTitle {
			// 脚本没停在标题（例如没有按钮定义）时仍按标题态处理，
			// 让宿主用固定布局兜底，而不是掉进剧情。
			core.Logf("tmrhiro: start 未停在标题 (step=%d)", step)
		}
		// absorb 必须跑：start 的 TITLE2 段用 `op3/0x00 BGM01` 起标题曲，
		// 这类音频事件只在 absorb 里被物化并排进 s.pending。以前这里直接返回，
		// 事件留在 VM 的 drain 队列里，玩家要在标题页点一下（触发一次 pump）
		// 才能听到音乐——就是"进主界面没音乐、点了才播"的根因。
		s.absorb()
	}
	s.buildTitleMenu()
	// 预处理标题背景与悬停图缓存，避免第一次合成时卡住一帧。
	base, hover := "title3", "title3_on"
	if s.vm != nil {
		if b, h := s.vm.TitleBase(); b != "" {
			base, hover = b, h
		}
	}
	s.cache.Get(base)
	if hover != "" {
		s.cache.Get(hover)
	}
	return s.step, nil
}

func (s *Session) startGame() (core.Step, error) {
	s.atTitle = false
	s.messageVisible = false
	s.titleMenu = nil
	// 标题层的画布与悬停状态必须立刻作废：Start(0) 跑过 start 脚本后
	// vm 停在 TITLE2 的界面定义上，composeTitle 会一直画标题底图，
	// 玩家看到的仍是标题页（点 START 像没反应）。
	s.titleBase = ""
	s.titleHover = ""
	s.hoveredKey = ""
	s.screen = nil
	// 标题画面的整屏图（title2/title3）必须从舞台上撤掉：原版进正篇时会
	// 用 bg 覆盖它，但 start 的 TITLE2 段把 title3 当"整屏图"贴上去之后，
	// 我们的 SetCG 会把它留在 cg 层，进正篇后仍然盖在背景上。
	s.stage.SetCG("")
	s.stage.ClearChars()
	// 新游戏走 `start` 脚本，但必须从 START 标签开始，而不是第一条记录：
	// 第一条记录往前是品牌 logo、警告页与标题菜单（`start` 的 TOP/
	// TITLE/TITLE2 段），玩家的“開始”按的是 START，那一段负责姓名输入
	// 与章节初始化（op5/0x14、op5/0x41、sys_name_change）。
	//
	// 直接从记录 0 跑会先停在标题菜单，看起来像“按了开始又回标题”。
	if err := s.vm.StartAt(entryStart, "START"); err != nil {
		s.lastErr = err.Error()
		s.step = core.StepFaulted
		return s.step, err
	}
	return s.pump(commandBudget)
}

// Run 推进脚本。
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

// Advance 在等输入/等选择时推进。
func (s *Session) Advance() (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.atTitle {
		return core.StepAwaitTitle, nil
	}
	if s.step == core.StepAwaitChoice {
		// 选择肢是模态的：点击画面不能越过菜单。
		return s.step, nil
	}
	if s.step == core.StepAwaitMovie {
		return s.step, nil
	}
	// 模态界面/脚本界面开着时不推进剧情（原版的界面是纯模态的）。
	if s.overlay != overlayNone || s.isScreenStep() {
		return s.step, nil
	}
	// 正文还没逐字显示完：第一次点击要"立刻全显"，而不是直接跳下一句。
	// 这是原版最基础的手感，缺了会变成"一句话被点掉一半"。
	//
	// 只有"真的在逐字"时才能拦：revealAt 为零值表示这句还没开始（刚换句、
	// 合成线程还没来得及跑），此时点击必须照常推进，否则玩家点了没反应。
	// 这个判断之前写反过，症状是"进游戏后第一句永远推不动"。
	if s.revealAt.IsZero() || s.revealDone || s.revealedText == s.text {
		// 已经显示完（或还没开始）：直接推进。
	} else {
		s.revealDone = true
		s.revealedText = s.text
		s.dirty = true
		return s.step, nil
	}
	s.advanceVM()
	return s.pump(commandBudget)
}

// Choose 处理选项点击（下标从 0 开始，与 Choices 顺序一致）。
func (s *Session) Choose(index uint32) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.step != core.StepAwaitChoice {
		return s.step, nil
	}
	s.vm.Choose(int(index))
	s.step = core.StepContinue
	return s.pump(commandBudget)
}

// Pointer 处理点击：标题点菜单，剧情里点对话框（推进）。
func (s *Session) Pointer(op uint32, x, y int32) (core.Step, error) {
	// 姓名输入（NAME）是原版的系统对话框，优先级最高：它开着时整屏点击都归它。
	if s.nameEntryActive() {
		if handled, err := s.pointerNameEntry(op, int(x), int(y)); handled {
			return s.step, err
		}
	}
	// 模态界面（确认框/暂停菜单/存读档/设置/回想）优先级最高：
	// 开着的界面必须吃掉点击，否则点格子会连带推进一句对白。
	if step, handled, err := s.pointerOverlay(op, x, y); handled {
		return step, err
	}
	// 脚本定义的界面（鉴赏/立绘/选择/确认）同样吃掉点击，点击把标签交回脚本。
	// 立绘鉴赏器（CHARACTER VIEWER）自己处理点击：列表/缩放/戻る都不走脚本标签。
	if s.charaViewActive() {
		if handled, err := s.pointerCharaView(op, int(x), int(y)); handled {
			return s.step, err
		}
	}
	// 只有"脚本界面真的收进了格子/按钮"时才让它吃点击：标题界面本身也是
	// 一个 screen（title3 + 四个按钮），但它由标题层/标题菜单处理。
	if s.screen != nil && !s.atTitle && s.pointerScreenOwns() {
		if step, handled, err := s.pointerScreenMenu(op, int(x), int(y)); handled {
			return step, err
		}
	}
	if s.atTitle {
		// 标题上有模态界面（读档页/确认框）时，点击已经在 pointerOverlay
		// 里被吃掉；这里必须直接返回，否则读档页格子会撞上标题热区，
		// 变成"点了格子却触发了标题按钮"（任务再開卡住的表现之一）。
		if s.overlay != overlayNone {
			return s.step, nil
		}
		if op == 0 {
			s.Hover(x, y)
			return s.step, nil
		}
		// 标题态只认标题热区：没命中热区的点击一律吞掉。
		//
		// 这条判断以前是"没命中就返回"没错，但**从データ閲覧返回标题**时
		// atTitle 会被下一次 pump 改回 false（脚本界面残留），于是点击落到
		// 下面的剧情分支 → Advance()，玩家看到标题底图还在、正文却已经推进
		// 到下一句（"界面显示还是主界面，但已经出现信息框的剧情文本"）。
		// 现在标题态在 Pointer 里直接拦住：命中热区才动作，其余什么都不做。
		if key, ok := s.hitTitle(x, y); ok {
			return s.SelectTitleItem(key)
		}
		return s.step, nil
	}
	if s.step == core.StepAwaitChoice {
		// 选项菜单也由引擎自绘（sys_select 三行图）；点击命中那一行才选择，
		// 点空白处不推进对白（原版的模态性）。
		if s.step == core.StepAwaitChoice && (op == 1 || op == 3) {
			if index := s.choiceHit(int(x), int(y)); index >= 0 {
				return s.Choose(uint32(index))
			}
		}
		if op == 0 {
			s.choiceHover = s.choiceHit(int(x), int(y))
			return s.step, nil
		}
		return s.step, nil
	}
	// 对话框底部工具条：先让它吃点击（否则点 SAVE/AUTO 会推进一句）。
	if op == 0 || op == 1 || op == 3 {
		if index := s.hitToolbar(int(x), int(y)); index >= 0 {
			if op == 0 {
				s.hoveredToolbar = index
				return s.step, nil
			}
			s.hoveredToolbar = -1
			if s.handleToolbarClick(index) {
				return s.step, nil
			}
		} else if op == 0 && s.hoveredToolbar >= 0 {
			s.hoveredToolbar = -1
		}
	}
	// 右键/ESC 打开暂停菜单（原版行为），这是最常用的一条入口。
	if op == 4 {
		return s.openMenuAndReturn()
	}
	// 宿主用 KzPointer：0=move 1=press 2=release 3=tap 4=右键 5=取消 6=拖动。
	// 剧情里只有 tap/press 推进对白；move 走 Hover（标题用），拖动留给
	// 回想/滚动类界面，右键表示“打开菜单”。
	switch op {
	case 0: // move
		s.Hover(x, y)
		return s.step, nil
	case 1, 3: // press / tap
		return s.Advance()
	case 4: // 右键
		return s.rightClick()
	}
	return s.step, nil
}

// pump 把 VM 推进到下一个停点，并把产生的图像/音频请求落到舞台上。
// settleTitleState 按当前停点与脚本界面决定“是不是标题态”。
//
// 三处调用（pump / 标题按钮 / 脚本界面按钮）必须用同一套判据，否则回到
// 标题后 atTitle 会被某一处单独改错——症状就是"画面还是标题，点击却推进剧情"。
//   - 界面有网格单元（鉴赏/场景/BGM）：不是标题；
//   - 界面底图是 title*（start 的 TITLE2 段）：是标题，重建菜单；
//   - 其余界面（EXTRA MENU 这类）：不是标题，也不建菜单。
func (s *Session) settleTitleState() {
	if s.step != core.StepAwaitTitle && s.step != core.StepAwaitSelect {
		return
	}
	if s.screen != nil && len(s.screen.Items) > 0 {
		s.atTitle = false
		s.titleMenu = nil
		return
	}
	base := ""
	if s.screen != nil {
		base = strings.ToLower(s.screen.Base)
	}
	if s.screen == nil || strings.HasPrefix(base, "title") {
		s.atTitle = true
		s.buildTitleMenu()
		return
	}
	s.atTitle = false
	s.titleMenu = nil
}

func (s *Session) pump(budget int) (core.Step, error) {
	step := s.vm.Run(budget)
	// 先把映射好的 step 写进 s.step 再 absorb：absorb 要按"这一句是停在等输入
	// 还是选项"决定对话框是否可见，用上一次的 step 会让第一句永远不可见。
	s.step = s.mapStep(step)
	s.absorb()
	if s.step == core.StepAwaitTitle {
		// 脚本自己弹出了地图/标题菜单（`start` 的 0x82→0x92/0x23→0x10），
		// 这时“标题态”由脚本驱动，不是宿主的 Start(0)：菜单项从 VM 取，
		// 点击也回到 VM 的标签。
		//
		// 但鉴赏/立绘界面同样以 0x82…0x10 收尾，它们的菜单项是"格子"而不是
		// 标题按钮：此时不能进标题态，否则宿主的标题层会盖在界面上、标题按钮
		// 也会被建出来（点一下等于点了标题）。
		s.settleTitleState()
	}
	if s.step == core.StepFaulted {
		s.lastErr = s.vm.LastError()
		return s.step, fmt.Errorf("%s", s.lastErr)
	}
	return s.step, nil
}

func (s *Session) advanceVM() {
	step := s.vm.Advance()
	s.step = s.mapStep(step)
	s.absorb()
}

// mapStep 把 VM 的停点映射成 core.Step。
//
// 等待（op5/0x82 的毫秒延时）走 StepWait：宿主按 WaitMs 定时再推进，
// 而不是把这几百毫秒当成“等玩家点击”。这一点对开场演出很关键——
// logo/警告页的时间轴全靠它。
func (s *Session) mapStep(step vm.Step) core.Step {
	// 姓名输入开着时，任何停点都先映射成 AwaitName：宿主据此弹 IME 层，
	// 其余界面（标题/对话）暂时不可交互。
	if s.nameEntryActive() {
		return core.StepAwaitName
	}
	if s.waitMs > 0 && step == vm.StepContinue {
		return core.StepWait
	}
	// 脚本定义的界面（鉴赏/立绘）由引擎自己画进帧里，宿主不该再弹自己的列表；
	// 映射成 AwaitSelect 让宿主停住"推进对白"，同时保留标题态的语义。
	if step == vm.StepAwaitTitle && s.screen != nil && !s.atTitle {
		return core.StepAwaitSelect
	}
	switch step {
	case vm.StepAwaitInput:
		return core.StepAwaitInput
	case vm.StepAwaitChoice:
		return core.StepAwaitChoice
	case vm.StepAwaitTitle:
		return core.StepAwaitTitle
	case vm.StepFinished:
		return core.StepFinished
	default:
		return core.StepContinue
	}
}

// absorb 把 VM 本步的请求翻译成舞台/音频状态。
//
// 顺序很关键：先图像后音频。原版“载图 → 播 BGM → 台词”是同一批记录，
// 若先播音频再切图，同帧会出现旧背景配新音乐的错位。
func (s *Session) absorb() {
	for _, op := range s.vm.DrainImages() {
		switch op.Kind {
		case 0:
			// 载图：fm_ 宏与整屏图都可能是背景；title2/title3/menu 这类名字
			// 走“整屏图”路径，其余当背景。
			if strings.HasPrefix(op.Name, "fm_") {
				// 立绘宏本身不带槽位与坐标：脚本随后一定会用 0xbc/0xcb
				// 给出完整信息，这里只占住槽 0（x=0、y=0、scale=1），
				// 避免出现“先设了却画到屏幕外”的中间帧。
				//
				// fm_A01_*/B*/C*/D*/E*/F* 是事件 CG（整屏），Z01/Z02 是立绘。
				// 事件 CG 也要记进鉴赏解锁集，否则玩家通关后図鑑仍是空的。
				if isEventCG(op.Name) {
					s.stage.SetCG(op.Name)
					s.unlockCG(op.Name)
					// H 场景脚本里出现事件 CG 即视为"这段回想已看"：
					// 原版把解锁位写进全局存档，MEMORY（シーン回想）页据此
					// 决定格子是否可点。以前只在 op2/0xc8（整屏 CG）里解锁，
					// 本作的 H 场景全部用 op2/0x00 + fm_B/A 宏，于是通关后
					// 回想页仍是全 NO DATA。
					if current := s.vm.Current(); current != nil {
						s.unlockScene(current.Name)
					}
				} else {
					s.stage.SetChar(0, op.Name, 0, 0, 1)
				}
			} else if isScreenGraphic(op.Name) {
				s.stage.SetCG(op.Name)
				s.unlockCG(op.Name)
				// 名字输入画面：原版把 sys_name_change 当整屏图贴上去之后
				// 立刻弹系统对话框。这里同样在这一刻进入姓名输入态。
				if strings.EqualFold(op.Name, "sys_name_change") {
					s.beginNameEntry()
				}
			} else if isTitleGraphic(op.Name) {
				// title2/title3 是标题画面的整屏**背景**，不是覆盖在场景上的 CG。
				// 归到 CG 层会让它一直压在剧情画面上——从标题点「任務開始」
				// 或读档后，玩家看到的是"标题图 + 剧情文本"。
				s.stage.SetBackground(op.Name)
				s.stage.SetCG("")
			} else {
				s.stage.SetBackground(op.Name)
				s.stage.SetCG("")
			}
		case 1:
			s.stage.ClearChars()
		case 2:
			// 脚本给的是“相对屏幕中心的偏移”，直接交给 stage 换算，
			// 不要在这里加 frameHeight/2：stage 的公式已经带了中心线。
			s.stage.SetChar(op.Slot, op.Name, op.X, op.Y, op.Scale)
		case 3:
			s.stage.SetCG(op.Name)
			s.unlockCG(op.Name)
			// 事件 CG 出现即解锁当前脚本对应的场景回想（原版把"这一段剧情放过"
			// 记进全局存档）。以前只有 CG 集被置位、场景集永远为空，
			// MEMORY（シーン回想）页永远是 NO DATA。
			if current := s.vm.Current(); current != nil {
				s.unlockScene(current.Name)
			}
		case 4:
			// 颜色淡入：记录成覆盖色，实际透明度由时长驱动，这里先给 0。
			s.stage.Fade(op.Color, 0)
		case 5:
			s.stage.SetFilter(op.Name)
		case 6, 10:
			// 情欲导航与未知命令：只记日志，不改变画面。
		}
	}
	for _, ev := range s.vm.DrainAudio() {
		s.enqueueAudio(ev)
	}
	if speaker := s.vm.Speaker(); speaker != "" || s.vm.Text() != "" {
		text := s.vm.Text()
		// 换句才重置逐字进度与已读标记：同一句在选项/自动播放的多次 absorb 里
		// 会反复读到，每次都重置会让正文永远停在第一个字；每次都重读
		// IsRead 则会在"刚标记已读"之后把同一句判成已读（整屏文字变黄）。
		if text != s.text || speaker != s.speaker {
			s.revealedText = ""
			s.revealAt = time.Time{}
			s.revealRunes = 0
			s.revealDone = false
			// 原版"未読"指的是"这句在显示之前没读过"。判定必须在 MarkRead 之前，
			// 而且只在换句时做一次。
			s.lastMessageWasRead = s.vm.IsRead(speaker, text)
			s.messageUnread = !s.lastMessageWasRead
			if s.step == core.StepAwaitInput {
				s.pushBacklog(speaker, text)
			}
		}
		s.speaker = speaker
		s.text = text
		s.messageVisible = s.step == core.StepAwaitInput || s.step == core.StepAwaitChoice
		if s.step == core.StepAwaitInput && !s.lastMessageWasRead {
			s.vm.MarkRead(speaker, text)
		}
	} else if s.step != core.StepAwaitInput {
		s.messageVisible = false
	}
	if s.vm.Step() == vm.StepAwaitChoice {
		s.applyChoices(s.vm.Choices())
	} else {
		s.choices = nil
	}
	if title := s.vm.Title(); title != "" {
		s.title = title
	}
	if chapter := s.vm.Chapter(); chapter != "" {
		s.chapter = chapter
	}
	switch req := s.vm.HostRequest(); {
	case req == "title":
		s.requestHost("title")
	case req == "settings":
		s.requestHost("settings")
	case req == "gallery":
		s.requestHost("gallery")
	case req == "exit":
		s.requestHost("exit")
	case strings.HasPrefix(req, "url:"):
		// 官网/帮助链接：宿主按 url: 前缀拦下来交给系统浏览器，
		// 不落到界面动作分支（见 player_screen 的 hostRequest 分派）。
		s.requestHost(req)
	}
	// 脚本定义的界面（鉴赏/立绘/选择/确认）由 VM 收集；这里把它的状态
	// 同步进 session，Compose 与 Pointer 都按它工作。
	s.syncScreen()
	// MEMORY COMPLETE 的 100%/0% 请求：把解锁集全置/全清（原版就是改全局存档）。
	if percent := s.vm.TakePercent(); percent >= 0 {
		s.applyPercent(percent)
		if percent >= 100 {
			s.notice = "CG・回想の達成率を100%にしました"
		} else {
			s.notice = "CG・回想の達成率を0%にしました"
		}
	}
	s.waitMs = uint32(s.vm.WaitMs())
	if s.waitMs > 0 {
		s.waitUntil = time.Now().Add(time.Duration(s.waitMs) * time.Millisecond)
	} else {
		s.waitUntil = time.Time{}
	}
	s.dirty = true
}

// applyChoices 把 VM 选项转成宿主的 Choice 列表。
func (s *Session) applyChoices(items []vm.Choice) {
	s.choices = s.choices[:0]
	for _, item := range items {
		s.choices = append(s.choices, core.Choice{Name: item.Text, Value: int32(len(s.choices))})
	}
}

// isTitleGraphic 判断名字是不是标题画面的整屏背景（title1/title2/title3）。
//
// 与原版一致：这些是"换背景"，不是叠在场景上的 CG。start 脚本会先贴
// title2、再贴 title3，进正篇时由 bg 记录覆盖；把它们当 CG 留在 cg 层，
// 剧情里就会一直看到标题图。
func isTitleGraphic(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "title")
}

// isScreenGraphic 判断名字是不是整屏界面图（菜单/选择底图）。
func isScreenGraphic(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "menu"):
		return true
	case strings.HasPrefix(lower, "cha_select"), strings.HasPrefix(lower, "select"):
		return true
	case strings.HasPrefix(lower, "sys_"):
		return true
	}
	return false
}

// enqueueAudio 把 VM 音频请求物化成宿主事件。
func (s *Session) enqueueAudio(ev vm.Audio) {
	switch ev.Kind {
	case 3:
		s.bgm = ""
		s.pending = append(s.pending, core.AudioEvent{Kind: 3, Channel: ev.Channel})
		return
	case 4:
		s.pending = append(s.pending, core.AudioEvent{Kind: 4, Channel: ev.Channel})
		return
	case 5:
		s.pending = append(s.pending, core.AudioEvent{Kind: 5, Channel: ev.Channel})
		return
	}
	if ev.Name == "" {
		return
	}
	raw := s.set.EntryOrEmpty(ev.Name)
	if len(raw) == 0 {
		core.Logf("tmrhiro: 音频 %q 不存在", ev.Name)
		return
	}
	path, err := s.audio.Write(ev.Name, raw)
	if err != nil {
		core.Logf("tmrhiro: 音频 %q 物化失败: %v", ev.Name, err)
		return
	}
	kind := ev.Kind
	if kind > 2 || kind < 0 {
		kind = 1
	}
	volume := 1.0
	switch kind {
	case 0:
		s.bgm = ev.Name
		volume = s.bgmVolume
	case 1:
		volume = s.seVolume
	case 2:
		volume = s.voiceVolume
	}
	if ev.Volume > 0 && ev.Volume < 1 {
		volume *= ev.Volume
	}
	s.pending = append(s.pending, core.AudioEvent{
		Kind: uint32(kind), Path: path, Volume: volume,
		Loop: ev.Loop || kind == 0, Channel: ev.Channel,
	})
}

// pushBacklog 把一句对白记入回想。上限 200 句，避免长周目吃内存。
func (s *Session) pushBacklog(speaker, text string) {
	if text == "" {
		return
	}
	s.backlogSeq++
	name := "backlog"
	if s.vm != nil && s.vm.Current() != nil {
		name = s.vm.Current().Name
	}
	s.backlog = append(s.backlog, core.TextUnit{
		ID:   fmt.Sprintf("%d", s.backlogSeq),
		File: name,
		Text: joinSpeaker(speaker, text),
	})
	if len(s.backlog) > 200 {
		s.backlog = s.backlog[len(s.backlog)-200:]
	}
}

func joinSpeaker(speaker, text string) string {
	if speaker == "" {
		return text
	}
	return speaker + "「" + text + "」"
}

// forceHost 发一个宿主动作，且强制递增序号（用于“同一个按钮可能连点”的场景）。
func (s *Session) forceHost(action string) {
	s.hostRequest = action
	s.hostRequestSerial++
}

// requestHost 记下宿主动作请求（serial 单调递增，宿主据此判断“新请求”）。
func (s *Session) requestHost(action string) {
	if action == "" || action == s.hostRequest {
		return
	}
	s.hostRequest = action
	s.hostRequestSerial++
}

// Compose 合成一帧。
func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if s.stage == nil {
		return false, errNotOpen
	}
	buf := unsafe.Slice((*byte)(dst), n)
	if len(buf) < frameWidth*frameHeight*4 {
		return false, fmt.Errorf("tmrhiro: 缓冲不足")
	}
	now := time.Now()
	// 标题底图只在"停在标题菜单"时画。脚本自己开的脚本界面（鉴赏/立绘）
	// 会先把 atTitle 置 false，但 start 脚本的 title2/title3 记录仍在舞台上，
	// 这里再兜一层：界面的 Base 不为空就直接画界面，不画标题。
	// 统一路径：先画场景/标题（都落在同一张舞台画布），再叠界面层。
	// 标题也算"一层画面"，这样从标题打开的读档页/鉴赏页才能叠在它上面。
	if s.atTitle && s.screen == nil {
		s.stage.Clear(0, 0, 0, 0xFF)
		s.composeTitle()
		s.composeOverlay()
		s.stage.BlitTo(buf)
	} else {
		// 舞台（背景/立绘/CG/覆盖色）先画，再叠引擎自绘的界面层。
		//
		// 界面层必须画进帧位图：原版的对话框底图是"白色渐变 + alpha"遮罩，
		// 靠 D3D 的反相混合画成深色（见 stage.InvertImage），Flutter 侧
		// 用普通 alpha 叠加只会得到一整片白。工具条、选项菜单、暂停菜单、
		// 存读档/设置/回想同样都是原版的贴图 + 原版坐标，交给宿主重画会
		// 出现"按钮图是原版、热区是另一套"的错位。
		s.stage.Compose(buf)
		if s.tickReveal(now) {
			s.dirty = true
		}
		s.tickNotice(now)
		s.composeMessageWindow()
		s.composeChoiceMenu()
		s.composeScreen()
		s.composeNameEntry()
		if s.charaViewActive() {
			s.composeCharaView()
		}
		s.composeOverlay()
		// 界面层画在同一张画布上：必须再拷一次，否则宿主只能看到背景。
		s.stage.BlitTo(buf)
	}
	s.textRuns = s.stage.Canvas().TakeRuns()
	return true, nil
}

// ComposeWritesBGRA 返回 false：本引擎内部是 BGRA，交给宿主前必须换位。
//
// 协议约定（见 capi 的 opCompose / frameResponse）：wire 上的像素一律是
// RGBA8888，Dart 侧 FramePainter 用 PixelFormat.rgba8888 解码。stage 用
// bishop.BgraCanvas，内部字节序是 B,G,R,A；声明"已经写好 BGRA"会跳过 capi
// 的 R/B 换位，整帧以 BGRA 直送 rgba8888 解码器 → 红蓝互换（血红的标题
// 变青色、深蓝背景变橙褐）。所有颜色错误的根因就在这里。
func (s *Session) ComposeWritesBGRA() bool { return false }

// composeTitle 把标题底图与悬停高亮画到**舞台画布**上。
//
// 为什么不再用独立画布：标题态同样要叠引擎自绘的界面层（从标题点「任務再開」
// 打开的是读档页、点「データ閲覧」进的是鉴赏页）。若标题画在另一张画布上，
// 界面层就画不到同一块缓冲，最后整帧要么只剩标题（点了 LOAD 没反应）、
// 要么只剩界面（标题消失）。
func (s *Session) composeTitle() {
	canvas := s.stage.Canvas()
	base := s.titleBase
	if base == "" {
		base = "title3"
	}
	if img := s.cache.Get(base); img != nil {
		canvas.Blit(bishopgfx.NewBsgImage(img.Width, img.Height, 0, 0, img.Pixels), 0, 0, 255)
	}
	// 悬停高亮：title3_on 是把**所有**按钮都画成亮色的整屏图，不能整张铺开
	// （那样四个按钮会同时发亮）。原版只取悬停那一块的矩形，盖在底图上。
	if s.hoveredKey != "" {
		hover := s.titleHover
		if hover == "" {
			hover = "title3_on"
		}
		if img := s.cache.Get(hover); img != nil {
			if hotspot := s.titleHotspotByKey(s.hoveredKey); hotspot.W > 0 {
				canvas.BlitRegion(
					bishopgfx.NewBsgImage(img.Width, img.Height, 0, 0, img.Pixels),
					hotspot.X, hotspot.Y, hotspot.W, hotspot.H,
					hotspot.X, hotspot.Y, 255)
			}
		}
	}
}

// TextRuns 返回最近一次 Compose 的文字。
func (s *Session) TextRuns() []core.TextRun {
	if len(s.textRuns) == 0 {
		return nil
	}
	out := make([]core.TextRun, len(s.textRuns))
	copy(out, s.textRuns)
	return out
}

// ---- core 状态/文本 ----

// Text 返回当前停点对白。
func (s *Session) Text() core.TextState {
	aw := uint32(0)
	if s.step == core.StepAwaitInput {
		aw = 1
	}
	return core.TextState{Speaker: s.speaker, Text: s.text, Awaiting: aw}
}

// Choices 返回当前选项。
func (s *Session) Choices() []core.Choice {
	if s.step != core.StepAwaitChoice {
		return nil
	}
	return append([]core.Choice(nil), s.choices...)
}

// Audio 取走本步的音频事件。
func (s *Session) Audio() []core.AudioEvent {
	out := s.pending
	s.pending = nil
	return out
}

// State 返回宿主快照。
func (s *Session) State() core.State {
	animating := s.atTitle || s.messageVisible
	pc := uint32(0)
	if s.vm != nil {
		pc = uint32(s.vm.PC())
	}
	return core.State{
		Bgm:                  s.bgm,
		PC:                   pc,
		Animating:            animating,
		WaitMs:               s.waitMs,
		MessageWindowVisible: s.messageVisible,
		// 姓名输入：宿主按 AwaitingName 弹 IME 层。对话框本身由引擎画
		// （EnginePrompt=true），宿主不再叠自己的 NameDialog。
		AwaitingName: s.nameEntryActive(),
		EnginePrompt: s.nameEntryActive(),
		NamePrompt:   "主人公の名前を設定してください。",
		// 对话框由引擎画进帧位图（见 composeMessageWindow）：原版的底图是
		// "白色渐变 + 反相混合"的遮罩，Flutter 用普通 alpha 叠只会得到一片白；
		// 排版（字体、行距、名字位置、工具条）也与宿主完全不同。
		EngineMessageWindow: true,
		MessageUnread:       s.messageUnread,
		CeaseSkip:           s.ceaseSkip,
		SkipUnread:          s.skipUnread,
		AutoWait:            s.autoWait,
		HostRequest:         s.hostRequest,
		HostRequestSerial:   s.hostRequestSerial,
	}
}

// TextList 返回回想。
func (s *Session) TextList() []core.TextUnit {
	return append([]core.TextUnit(nil), s.backlog...)
}

// LastError 返回最近错误。
func (s *Session) LastError() string { return s.lastErr }

// Slots 返回槽位列表（1..100 的正式槽位，不含快存槽）。
func (s *Session) Slots() []core.Slot {
	out := make([]core.Slot, 0, saveSlots)
	for slot := uint32(1); slot <= saveSlots; slot++ {
		snapshot, err := s.store.Load(slot)
		item := core.Slot{Index: slot}
		if err == nil {
			item.Exists = true
			item.Title = snapshot.Title
			item.SavedAt = snapshot.SavedAt
			item.Summary = snapshot.Summary
		}
		out = append(out, item)
	}
	return out
}

// Save 存档。
//
// 槽位 0 是“快存槽”（原版 QSAVE）：id 固定为 900，与 1..100 的正式槽位
// 分开存放，不会被读档页列出，也不会挤掉玩家点过的位置。
func (s *Session) Save(slot uint32, title string) error {
	if s.vm == nil || s.store == nil {
		return errNotOpen
	}
	if s.step == core.StepAwaitChoice {
		return fmt.Errorf("tmrhiro: 选择肢上不能存档")
	}
	if slot == 0 {
		slot = quickSlot
	}
	snap := save.Slot{
		Title:   firstNonEmpty(title, s.title),
		SavedAt: save.FormatClock(slot, time.Now()),
		Summary: joinSpeaker(s.speaker, s.text),
		Speaker: s.speaker,
		Text:    s.text,
		Bgm:     s.bgm,
		Vars:    s.vm.Vars(),
		Seen:    s.vm.Seen(),
	}
	state := s.vm.SnapshotState()
	snap.Script = state.Script
	snap.Index = state.PC
	snap.CallStack = state.CallStack
	chars := s.stage.Chars()
	for i := range chars {
		if chars[i].Name == "" {
			continue
		}
		snap.Stage.Chars = append(snap.Stage.Chars, save.CharSlot{
			Name: chars[i].Name, X: chars[i].X, Y: chars[i].Y, Scale: chars[i].Scale,
		})
	}
	snap.Stage.Background = s.stage.Background()
	snap.Stage.CG = s.stage.CG()
	snap.Stage.FadeColor, snap.Stage.FadeAlpha = s.stage.FadeState()
	if err := s.store.Save(slot, snap); err != nil {
		return err
	}
	// 缩略图：原版存档界面左侧那张 128x72 的画面缩略图。存档时截当前画面，
	// 按最近邻缩到缩略图尺寸（与原版一样不追求画质）。
	if img := s.stage.Snapshot(); img != nil {
		if thumb, ok := shrinkFrame(img, thumbWidth, thumbHeight); ok {
			_ = s.store.SaveThumb(slot, thumb)
		}
	}
	return nil
}

// Load 读档。
func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.vm == nil || s.store == nil {
		return core.StepFaulted, errNotOpen
	}
	if slot == 0 {
		slot = quickSlot
	}
	snap, err := s.store.Load(slot)
	if err != nil {
		return core.StepFaulted, fmt.Errorf("%w", errNoSlot)
	}
	// 读档后彻底离开标题层与脚本界面：标题底图/悬停图/菜单项若留着，
	// 下一次 Compose 会继续按"标题态"画，玩家会看到读档后的剧情文本
	// 压在标题图上（"点任務再開卡在标题但出现剧情文本"的另一半原因）。
	s.atTitle = false
	s.titleMenu = nil
	s.titleBase = ""
	s.titleHover = ""
	s.hoveredKey = ""
	s.overlay = overlayNone
	s.screen = nil
	s.confirm = nil
	s.stage.SetCG("")
	s.stage.ClearChars()
	s.vm.SetVars(snap.Vars)
	s.vm.SetSeen(snap.Seen)
	s.vm.ClearScreen()
	s.screen = nil
	if err := s.vm.Restore(vm.Snapshot{
		Script:    snap.Script,
		PC:        snap.Index,
		CallStack: snap.CallStack,
	}); err != nil {
		s.lastErr = err.Error()
		return core.StepFaulted, err
	}
	s.stage.ClearChars()
	s.stage.SetBackground(snap.Stage.Background)
	s.stage.SetCG(snap.Stage.CG)
	for i, char := range snap.Stage.Chars {
		s.stage.SetChar(i, char.Name, char.X, char.Y, char.Scale)
	}
	s.stage.Fade(snap.Stage.FadeColor, snap.Stage.FadeAlpha)
	s.speaker = snap.Speaker
	s.text = snap.Text
	s.bgm = snap.Bgm
	s.messageVisible = true
	s.advanceVM()
	return s.pump(commandBudget)
}

// PreviewSlot 返回槽位预览。
func (s *Session) PreviewSlot(slot uint32) (core.SlotPreview, bool) {
	if s.store == nil {
		return core.SlotPreview{}, false
	}
	snap, err := s.store.Load(slot)
	if err != nil {
		return core.SlotPreview{}, false
	}
	return core.SlotPreview{
		SavedAt: snap.SavedAt,
		Summary: snap.Summary,
		Speaker: snap.Speaker,
		Text:    snap.Text,
	}, true
}

// AssetSize 返回资源字节数。
func (s *Session) AssetSize(path string) int {
	if s.set == nil {
		return 0
	}
	size := s.set.Size(path)
	if size <= 0 {
		return 0
	}
	if size > 1<<30 {
		return 0
	}
	return int(size)
}

// ReadAsset 把资源字节写到 dst。
func (s *Session) ReadAsset(path string, dst unsafe.Pointer, n int) int {
	if s.set == nil || dst == nil || n <= 0 {
		return 0
	}
	raw, err := s.set.Entry(path)
	if err != nil {
		return 0
	}
	return copy(unsafe.Slice((*byte)(dst), n), raw)
}

// ---- 配置/宏/设置 ----

// loadConfigs 读 _system/_var/_charaview 等文本配置。
func (s *Session) loadConfigs() {
	if raw := s.set.EntryOrEmpty("_system"); len(raw) > 0 {
		if ini, err := config.ParseIni(raw); err == nil {
			s.system = ini
			s.ui = config.ParseUI(ini)
		} else {
			core.Logf("tmrhiro: _system 解析失败: %v", err)
		}
	}
	if raw := s.set.EntryOrEmpty("_charaview"); len(raw) > 0 {
		if ini, err := config.ParseIni(raw); err == nil {
			chara := config.ParseCharaView(ini)
			s.ui.Chara = chara
			core.Logf("tmrhiro: 立绘鉴赏器 %d 角色, %d 背景, 列表 (%d,%d)",
				len(chara.Characters), len(chara.BGList), chara.ListX, chara.ListY)
		} else {
			core.Logf("tmrhiro: _charaview 解析失败: %v", err)
		}
	}
	if raw := s.set.EntryOrEmpty("_var"); len(raw) > 0 {
		if list, err := config.ParseList(raw); err == nil {
			s.loadVarTables(list)
		}
	}
	if s.system != nil {
		// 默认音量：SYS_DEF_SOUND_VOLUME=65,90,80,80（BGM/SE/Voice/BGS）。
		values := s.system.Ints("SYSTEM", "SYS_DEF_SOUND_VOLUME")
		if len(values) >= 3 {
			s.bgmVolume = clampUnit(float64(values[0]) / 100)
			s.seVolume = clampUnit(float64(values[1]) / 100)
			s.voiceVolume = clampUnit(float64(values[2]) / 100)
		} else {
			s.bgmVolume, s.seVolume, s.voiceVolume = 0.65, 0.9, 0.8
		}
		s.autoSpeed = s.system.Int("SYSTEM", "SYS_DEF_AUTOSPEED", 6)
		s.textSpeed = s.system.Int("SYSTEM", "SYS_DEF_TEXTSPEED", 6)
		s.autoWait = autoDelay(s.autoSpeed)
		if v := s.system.Int("SYSTEM", "SYS_DEF_SKIP", 0); v > 0 {
			s.ceaseSkip = true
		}
		if v := s.system.Int("SYSTEM", "SYS_DEF_S_SKIP", 0); v > 0 {
			s.skipUnread = true
		}
	}
}

// loadVarTables 从 _var 里取鉴赏用的名字表。
//
// _var 的分节是 [HCG]（CG 名）、[HSC]（H 场景名），后面可能还有别的段。
// 只是名字列表，直接按序保存；鉴赏页面的格子布局由 op 4 的 0x64 记录给出。
func (s *Session) loadVarTables(list []string) {
	section := ""
	for _, line := range list {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToUpper(strings.Trim(line, "[]"))
			continue
		}
		switch section {
		case "HCG":
			s.cgList = append(s.cgList, line)
		case "HSC":
			s.sceneList = append(s.sceneList, line)
		}
	}
	core.Logf("tmrhiro: cg 目录 %d 项, 场景 %d 项", len(s.cgList), len(s.sceneList))
}

// loadMacros 读 _filemacro / _charafilemacro。
func (s *Session) loadMacros() {
	if raw := s.set.EntryOrEmpty("_filemacro"); len(raw) > 0 {
		if table, err := config.ParseMacro(raw); err == nil {
			s.macros = table
		}
	}
	if raw := s.set.EntryOrEmpty("_charafilemacro"); len(raw) > 0 {
		if table, err := config.ParseMacro(raw); err == nil {
			s.charas = table
		}
	}
	merged := make(map[string][]string, len(s.macros)+len(s.charas))
	for k, v := range s.macros {
		merged[k] = v
	}
	for k, v := range s.charas {
		merged[k] = v
	}
	s.stage.SetMacroTable(merged)
	core.Logf("tmrhiro: 宏表 %d 项", len(merged))
}

// loadSettings 读玩家设置（音量/速度/跳过策略）。
func (s *Session) loadSettings() {
	raw, err := os.ReadFile(s.settingsPath)
	if err != nil {
		return
	}
	var stored struct {
		Bgm        float64 `json:"bgm"`
		Se         float64 `json:"se"`
		Voice      float64 `json:"voice"`
		AutoSpeed  int     `json:"auto_speed"`
		TextSpeed  int     `json:"text_speed"`
		SkipUnread bool    `json:"skip_unread"`
		CeaseSkip  bool    `json:"cease_skip"`
		// 设置界面的其余开关：画面效果、AUTO 时语音截断、已读色、淫语着色、
		// 对话框底图，以及设备类选项的档位（system_choices）。
		Effects        bool           `json:"effects"`
		AutoVoiceCut   bool           `json:"auto_voice_cut"`
		ReadColorOn    bool           `json:"read_color_on"`
		VulgarWord     bool           `json:"vulgar_word"`
		WindowBaseDraw bool           `json:"window_base_draw"`
		SystemChoices  map[string]int `json:"system_choices"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return
	}
	if stored.Bgm > 0 {
		s.bgmVolume = clampUnit(stored.Bgm)
	}
	if stored.Se > 0 {
		s.seVolume = clampUnit(stored.Se)
	}
	if stored.Voice > 0 {
		s.voiceVolume = clampUnit(stored.Voice)
	}
	if stored.AutoSpeed > 0 {
		s.autoSpeed = stored.AutoSpeed
	}
	if stored.TextSpeed > 0 {
		s.textSpeed = stored.TextSpeed
	}
	s.skipUnread = stored.SkipUnread
	s.ceaseSkip = stored.CeaseSkip
	s.effects = stored.Effects
	s.autoVoiceCut = stored.AutoVoiceCut
	s.readColorOn = stored.ReadColorOn
	s.vulgarWord = stored.VulgarWord
	s.windowBaseDraw = stored.WindowBaseDraw
	if stored.SystemChoices != nil {
		s.systemChoices = stored.SystemChoices
	}
	s.autoWait = autoDelay(s.autoSpeed)
}

// saveSettings 写玩家设置。
func (s *Session) saveSettings() {
	stored := map[string]any{
		"bgm":              s.bgmVolume,
		"se":               s.seVolume,
		"voice":            s.voiceVolume,
		"auto_speed":       s.autoSpeed,
		"text_speed":       s.textSpeed,
		"skip_unread":      s.skipUnread,
		"cease_skip":       s.ceaseSkip,
		"effects":          s.effects,
		"auto_voice_cut":   s.autoVoiceCut,
		"read_color_on":    s.readColorOn,
		"vulgar_word":      s.vulgarWord,
		"window_base_draw": s.windowBaseDraw,
		"system_choices":   s.systemChoices,
	}
	raw, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	_ = core.AtomicWrite(s.settingsPath, raw)
}

// SetOption 处理宿主设置项。
//
// key 与宿主协议（KzOption）一致：
//   - 0 showMessageFrame：本作不画消息窗（见 Compose 的注释），忽略；
//   - 1 skipOverlay：宿主处于跳过状态。原版此时会加速文字并且不停在
//     已读句上；我们把它翻译成“跳过时不停未读”（与 bishop 的 SkipUnread 同义）；
//   - 2 autoMode：宿主处于自动播放。不影响引擎逻辑（自动推进由宿主计时）。
//
// 只有真正影响引擎行为的项才落盘，避免每次点 AUTO 都写一次配置。
func (s *Session) SetOption(key uint32, value bool) {
	switch key {
	case 0:
		// 保留给“引擎自绘消息框”。本品牌由宿主画，收到也什么都不做。
		return
	case 1:
		if s.skipUnread == value {
			return
		}
		s.skipUnread = value
	case 2:
		// AUTO 状态：宿主是推进方，引擎只需要镜像（逐字速度与确定图标受它影响）。
		s.autoActive = value
		return
	}
	s.saveSettings()
}

// ---- 标题菜单 ----

// buildTitleMenu 从 AGRD 的 title3（底图）与 _system 的 TWB_* 生成菜单项。
//
// 本作的标题按钮不是独立图片，而是 title3 底图里已经画好的四行文字
// （任务开始/任務再開/データ閲覧/終了）与一行“公式サイトへ”。热区来自
// start 脚本的 op1/0x32：
//
//	<u16 x1> <u16 y1> <u16 w?> <u16 h?> <name>
//
// 这里按“名字 → 标签”建表，坐标按 1280x720 的固定布局给（脚本里的热区
// 用的是 480x270 的参考系，乘以 2.667 才是 1280x720）。
func (s *Session) buildTitleMenu() {
	s.titleMenu = nil
	if s.vm == nil {
		return
	}
	base, hover := s.vm.TitleBase()
	s.titleBase, s.titleHover = base, hover
	// 按钮热区是脚本给的（`start` 的 0x23 记录）；标签名（START/LOAD/…）
	// 就是点击后要跳到的标签。坐标已经是 1280x720 引擎坐标系，直接用。
	// 按钮标签来自脚本的 0x23 记录；热区取自"标题图上真实画出的高亮块"。
	// 优先用 screen.Buttons（VM 会把 0x23 收进界面），退回 titleButtons。
	buttons := s.vm.ScreenButtons()
	if len(buttons) == 0 {
		for _, button := range s.vm.TitleButtons() {
			buttons = append(buttons, vm.ScreenItem{Kind: vm.ScreenItemButton, Label: button.Label,
				X: button.X, Y: button.Y, W: button.W, H: button.H})
		}
	}
	for index, button := range buttons {
		hot := s.titleHotspot(index, vm.TitleButton{Label: button.Label, X: button.X, Y: button.Y, W: button.W, H: button.H})
		enabled := button.Label != "LOAD" || s.hasSave()
		s.titleMenu = append(s.titleMenu, core.TitleMenuItem{
			Key: button.Label, Label: s.titleLabel(button.Label),
			X: int32(hot.X), Y: int32(hot.Y),
			Width: uint32(hot.W), Height: uint32(hot.H),
			Enabled: enabled,
		})
	}
}

// titleHotspots 是标题四行文字与「公式サイトへ」的命中矩形。
//
// 实测来源：把 AGRD 的 title3 与 title3_on 逐像素比对，差异区域就是原版
// 画在标题图上的高亮块（本作 1280x720）：
//
//	START      988..1279 x 342..394
//	LOAD       988..1279 x 402..454
//	OMAKE      988..1279 x 462..514
//	END        988..1279 x 522..574
//	HOME_PAGE   56..350  x 640..695
//
// 这些数字写死是刻意的：标题图是固定素材，逐帧比对 3.7MB 两张全屏图会
// 让每次进标题多花几十毫秒，而结果永远不变。
var titleHotspots = []config.Rect{
	{X: 988, Y: 342, W: 292, H: 53},
	{X: 988, Y: 402, W: 292, H: 53},
	{X: 988, Y: 462, W: 292, H: 53},
	{X: 988, Y: 522, W: 292, H: 53},
	{X: 56, Y: 640, W: 295, H: 56},
}

// titleHotspotByKey 按标签取标题按钮的矩形（悬停高亮用）。
func (s *Session) titleHotspotByKey(key string) config.Rect {
	for index, item := range s.titleMenu {
		if item.Key != key {
			continue
		}
		if index < len(titleHotspots) {
			return titleHotspots[index]
		}
		return config.Rect{X: int(item.X), Y: int(item.Y), W: int(item.Width), H: int(item.Height)}
	}
	return config.Rect{}
}

// titleHotspot 取第 index 个按钮的矩形；脚本给的按钮数超过实测表时退回
// 记录里的坐标（至少不会完全点不到）。
func (s *Session) titleHotspot(index int, button vm.TitleButton) config.Rect {
	if index >= 0 && index < len(titleHotspots) {
		return titleHotspots[index]
	}
	return config.Rect{X: button.X, Y: button.Y, W: button.W, H: button.H}
}

// titleLabel 把脚本里的标签名翻成界面文字。
//
// 标题底图（title3）上已经画好了“任務開始／任務再開／データ閲覧／終了”
// 这些字，宿主不需要再画一遍；这里给的是无障碍/键盘导航用的文本。
func (s *Session) titleLabel(label string) string {
	switch label {
	case "START":
		return "任務開始"
	case "LOAD":
		return "任務再開"
	case "OMAKE":
		return "データ閲覧"
	case "END":
		return "終了"
	case "HOME_PAGE":
		return "公式サイトへ"
	}
	return label
}

func (s *Session) hasSave() bool {
	if s.store == nil {
		return false
	}
	return len(s.store.Existing()) > 0
}

// hitTitle 返回命中的菜单项 key。
func (s *Session) hitTitle(x, y int32) (string, bool) {
	for _, item := range s.titleMenu {
		if !item.Enabled {
			continue
		}
		if x >= item.X && x < item.X+int32(item.Width) && y >= item.Y && y < item.Y+int32(item.Height) {
			return item.Key, true
		}
	}
	return "", false
}

// TitleMenu 返回主界面菜单项。
func (s *Session) TitleMenu() []core.TitleMenuItem {
	// 标题上打开读档页时菜单必须暂时隐藏：宿主会把 titleMenu 画成一层
	// 透明的可点区域，盖在引擎自绘的读档页上，点击就落不到格子。
	if !s.atTitle || s.overlay != overlayNone {
		return nil
	}
	return append([]core.TitleMenuItem(nil), s.titleMenu...)
}

// SelectTitleItem 处理标题点击。
func (s *Session) SelectTitleItem(key string) (core.Step, error) {
	if s.vm == nil {
		return core.StepFaulted, errNotOpen
	}
	// 标题按钮/脚本界面按钮共用这一个入口；只有后者会带界面（screen），
	// 前者的 key 是 START/LOAD/OMAKE/END/HOME_PAGE。
	//
	// 关键区别：LOAD 在标题上是"打开读档页"，在脚本界面里（鉴赏的 MEMORY /
	// CG 格子）是"跳到那个页面"。以前不区分，脚本界面的每一格都被当成
	// 标题的 LOAD 处理 → 点了没反应（玩家看到的就是 CHARACTER VIEWER、
	// MEMORY COMPLETE 点不动）。
	if s.screen != nil && !s.atTitle {
		if step, _, err := s.jumpScreenLabel(key); err == nil {
			return step, nil
		}
	}
	// 清空上一次的宿主动作：标题按钮可能连点，宿主靠 serial 去重，
	// 旧值留着会让“OMAKE 打开鉴赏”被误读成“仍然要打开存档列表”。
	s.hostRequest = ""
	switch key {
	case "START", "start", "op":
		// 进正篇：不是“脚本内跳转”，而是从 start 脚本重新起跑到 START 标签，
		// 这样品牌页/警告页/标题的初始化（姓名表、章节号）都会按原版顺序发生。
		return s.startGame()
	case "LOAD", "load":
		// 读档界面由引擎自绘（sys_load_base + sys_load_button，与 SAVE 同一套
		// 布局）：原版的读档页就是游戏画面的一部分，交给 Flutter 会丢掉
		// 缩略图、日期与页签。
		s.openSlotBoard(false)
		return s.step, nil
	case "END", "quit":
		// 原版的「終了」是弹确认框再退出（不是直接关窗口）。
		s.overlay = overlayConfirm
		s.confirm = &confirmState{prompt: "ゲームを終了しますか？", action: "exit"}
		return s.step, nil
	case "HOME_PAGE", "home":
		// 公式サイトへ：交给系统浏览器（宿主的 url: 分派）。
		url := "https://lune-soft.jp/game/20683"
		if s.system != nil {
			if link := s.system.Get("GAMESYSTEM", "HP_ADDRESS"); link != "" {
				url = "https://" + link
			}
		}
		s.requestHost("url:" + url)
		return s.step, nil
	}
	// 其余（START/OMAKE/…）都是脚本内跳转：交给 VM 按标签继续执行。
	// `start` 的 OMAKE 会先调 view 脚本（0x0040/0x0064 里发 gallery 请求），
	// 再回到标题——所以这里不能假设“离开标题”，要看跳转后的实际停点。
	//
	// OMAKE 的目标是鉴赏页：脚本侧只是切到 view 脚本并等输入，宿主收到
	// gallery 后自己打开鉴赏列表。为避免把玩家留在半截谐调的画面里，
	// VM 停在 AwaitInput 时我们仍然回标题态（菜单还在），
	// 画面由 view 脚本的合成结果提供。
	step := s.vm.TitleSelect(key)
	s.absorb()
	s.step = s.mapStep(step)
	if s.step == core.StepAwaitTitle || s.step == core.StepAwaitSelect {
		// 脚本自己开的界面（鉴赏/立绘）接管画面：由 settleTitleState 判断
		// 它到底是标题页还是别的界面（标题页要继续画标题层，其余都不能）。
		s.settleTitleState()
		// 界面底图与悬停图预取，避免第一次合成卡住一帧。
		if s.screen != nil {
			if s.screen.Base != "" {
				s.cache.Get(s.screen.Base)
			}
			if s.screen.BaseOn != "" {
				s.cache.Get(s.screen.BaseOn)
			}
		}
		return s.step, nil
	}
	s.atTitle = false
	s.titleMenu = nil
	return s.step, nil
}

// rightClick 打开暂停菜单（原版右键/ESC = 屏幕顶端那排「セーブ / ロード /
// システム / タイトル画面 / ゲーム終了 / 回想終了」）。
//
// 菜单由引擎自绘（sys_window_menu 底图 + sys_window_button 按钮），因为
// 原版的菜单是整个流程的中枢：存读档、设置、回标题、退出游戏都从这里进。
// 交给 Flutter 等于把玩家踢出引擎自己的状态机。
func (s *Session) rightClick() (core.Step, error) {
	if s.atTitle || s.step == core.StepAwaitChoice {
		return s.step, nil
	}
	s.openMenu()
	return s.step, nil
}

// openMenuAndReturn 是 openMenu 的 (Step, error) 包装（Pointer 用）。
func (s *Session) openMenuAndReturn() (core.Step, error) {
	s.openMenu()
	return s.step, nil
}

// Hover 记录悬停项（标题层据此切 title3_on）。
//
// 宿主每帧都会发 hover（甚至连点期间也会排队），所以这里只改一个字段、
// 不触发合成：标题图的换帧由下一次 OpFrame 的 Compose 自然带出，
// 否则每次鼠标移动都要重画一整张 1280x720 的底图。
// Hover 记录悬停项（标题层据此切 title3_on / 暂停菜单据此换高亮帧）。
//
// 两条来源：标题页的按钮热区（START/LOAD/OMAKE/END），以及画面顶端那条
// 48px 的菜单带（原版鼠标移到屏幕最上方就会浮出「セーブ / ロード / …」）。
// 以前只有标题热区，暂停菜单只能靠右键/ESC 打开，触屏玩家永远看不到 MENU。
func (s *Session) Hover(x, y int32) {
	if s.atTitle {
		if key, ok := s.hitTitle(x, y); ok {
			s.hoveredKey = key
			return
		}
		s.hoveredKey = ""
		return
	}
	// 剧情里：指针停在顶栏就展开暂停菜单（与右键等价的原版入口）。
	if s.overlay == overlayNone && s.step != core.StepAwaitChoice && y >= 0 && y < 48 {
		s.openMenu()
		s.overlayHover = s.hitMenuButton(int(x), int(y))
		return
	}
	if s.overlay == overlayMenu {
		s.overlayHover = s.hitMenuButton(int(x), int(y))
	}
}

// hitMenuButton 返回暂停菜单命中的按钮下标（-1 表示没有）。
func (s *Session) hitMenuButton(x, y int) int {
	if s.ui == nil {
		return -1
	}
	for index, button := range s.ui.Menu.Buttons {
		if !button.Enabled {
			continue
		}
		rect := config.Rect{X: button.X, Y: button.Y, W: button.W, H: button.H}
		if rect.Contains(x, y) {
			return index
		}
	}
	return -1
}

// ---- 小工具 ----

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// autoDelay 把 SYS_DEF_AUTOSPEED（0..10，越大越快）换算成毫秒。
//
// 原版设置页把 10 档画成一条滑杆；换算公式取“1 档 2400ms、10 档 300ms”
// 的线性插值，与 bishop 的 AutoWait 口径一致，宿主不需要再维护第二个公式。
func autoDelay(speed int) int {
	if speed <= 0 {
		speed = 1
	}
	if speed > 10 {
		speed = 10
	}
	return 2400 - (speed-1)*(2100/9)
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, "kizuna-write-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var _ = sort.Strings

// DebugStage 是诊断用：返回舞台层的一句话状态（引擎日志/单测用）。
func (s *Session) DebugStage() string {
	if s.stage == nil {
		return "no stage"
	}
	return fmt.Sprintf("atTitle=%v screen=%v overlay=%v %s", s.atTitle, s.screen != nil, s.overlay, s.stage.Describe())
}

// DebugScreen 是诊断用：返回当前脚本界面的概况（日志/单测用）。
func (s *Session) DebugScreen() string {
	if s.screen == nil {
		return "no screen"
	}
	return fmt.Sprintf("base=%q on=%q graphic=%+v buttons=%d items=%d default=%q percent=%d",
		s.screen.Base, s.screen.BaseOn, s.screen.Graphic, len(s.screen.Buttons), len(s.screen.Items), s.screen.Default, s.screen.Percent)
}

// pointerScreenOwns 报告当前脚本界面是否由引擎自己处理点击。
//
// 标题页同样以 0x82…0x10 定义，但它的按钮必须在标题层/标题菜单那边处理
// （宿主画标题层、点击回 SelectTitleItem）。用"有没有网格单元"区分：
// 鉴赏/立绘/确认框都有 0x64 收进来的单元，标题没有。
func (s *Session) pointerScreenOwns() bool {
	if s.screen == nil {
		return false
	}
	// 标题页本身不带网格单元，但它的底图是 title*，仍归标题层处理。
	// 反过来，鉴赏/立绘/确认框里的"按钮页"（EXTRA MENU、PERCENT100）底图是
	// menu/menu100，必须由引擎吃点击——以前这里用 atTitle 兜底，而
	// settleTitleState 在 EXTRA MENU 上把 atTitle 置成了 false 之前，
	// 点击会被标题分支吞掉，于是"データ閲覧点击没反应"。
	base := strings.ToLower(s.screen.Base)
	if strings.HasPrefix(base, "title") {
		return false
	}
	// 鉴赏/立绘/确认框都有网格单元或 0xc3 的四张图；菜单类页面只有 0x23
	// 按钮（底图来自 0x92），同样归引擎处理（标题页的底图是 title*，
	// 且 atTitle 已经把它排除在外）。
	return len(s.screen.Items) > 0 || s.screen.Graphic.Base != "" ||
		strings.HasPrefix(s.screen.Base, "menu") ||
		strings.HasPrefix(s.screen.Base, "cgmode") ||
		strings.HasPrefix(s.screen.Base, "hsmode") ||
		strings.HasPrefix(s.screen.Base, "cha_select")
}

// DebugHit / DebugButtons 是诊断用：界面点击调试（日志/单测用）。
func (s *Session) DebugHit(x, y int) string {
	index := s.hitScreenButton(x, y)
	return fmt.Sprintf("index=%d label=%q hoverOwns=%v", index, s.screenLabel(index), s.pointerScreenOwns())
}

func (s *Session) DebugButtons() string {
	if s.screen == nil {
		return "no screen"
	}
	out := ""
	for _, b := range s.screen.Buttons {
		out += fmt.Sprintf("[%s %d,%d %dx%d] ", b.Label, b.X, b.Y, b.W, b.H)
	}
	return out
}

// DebugChara 是诊断用：立绘鉴赏器的配置与当前组合（日志/单测用）。
func (s *Session) DebugChara() string {
	if s.ui == nil {
		return "no ui"
	}
	v := s.ui.Chara
	return fmt.Sprintf("list=(%d,%d %dx%d step=%d) bg=%d chars=%d entries=%d costumes=%v expr=%d macro=%q",
		v.ListX, v.ListY, v.ListW, v.ListH, v.ListStep, len(v.BGList), len(v.Characters), len(v.Entries), s.charaCostumes(), len(s.charaExpressions()), s.charaMacro())
}

// DebugUnlock 是诊断用：解锁集合的规模（日志/单测用）。
func (s *Session) DebugUnlock() string {
	return fmt.Sprintf("cg=%d scene=%d", len(s.unlockedCG), len(s.unlockedScene))
}

// DebugVarCounts 是诊断用：鉴赏目录规模（日志/单测用）。
func (s *Session) DebugVarCounts() string {
	return fmt.Sprintf("cgList=%d sceneList=%d", len(s.cgList), len(s.sceneList))
}
