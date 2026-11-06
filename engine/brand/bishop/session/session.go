package session

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/data"
	"kizuna/engine/brand/bishop/data/scriptdb"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/lua"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/runtime"
	"kizuna/engine/brand/bishop/runtime/bs"
	"kizuna/engine/brand/bishop/runtime/bsmap"
	"kizuna/engine/brand/bishop/save"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	bsvm "kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

const commandBudget = 4096

const traceLimit = 256

const unresolvedSlot = -2

const replaySuffix = "_replay"

const noTarget = uint32(0xFFFFFFFF)

var storyEntryCandidates = []string{"opening", "prologue", "start", "newgame", "main"}

var (
	errNotOpen   = errorString("bishop session: engine not open")
	errBadEntry  = errorString("bishop session: bad entry")
	errNoSlot    = errorString("bishop session: slot has no save")
	errSaveFault = errorString("bishop session: save failed")
)

type errorString string

func (e errorString) Error() string { return string(e) }

type dispatcher interface {
	Choose(position int) *vm.SelectOption
	Dispatch(command vm.ScriptCommandId) state.SessionStep
}

type Session struct {
	dir         string
	dataDir     string
	extraDirs   []string
	lookupRoots []string
	cacheDir    string
	title       string
	width       int
	height      int
	config      *config.BsxxConfiguration
	res         *resource.GameResourceIndex
	composer    *rendering.SceneComposer
	script      *vm.BsxScriptFile
	machine     *vm.BsxMachine
	router      dispatcher
	// bsStage 是 BSScript 路线的舞台导演。只有这支路线有 Rev5 的
	// cs 合成立绘；现代 BSXScript 的立绘由 ScriptRouter 自己处理。
	bsStage     *bs.BsStageDirector
	mapCatalog  *bsmap.BsMapCatalog
	gameState   *state.GameState
	saves       *save.SaveStore
	catalog     []core.Scene
	story       map[int]bool
	sawMessage  bool
	entryUsed   bool
	storyIn     bool
	valueSlot   int
	typeSlot    int
	counterSlot int
	mapItems    []bsmap.BsMapItem
	mapBackup   []state.BsLayerEntry
	mapMask     *bsmap.BsMapMaskData
	mapSource   string
	mapShown    bool
	// mapLayout 保存当前地图布局：取消键要用 CancelLabel，悬停要用楼层列表的几何信息。
	mapLayout bsmap.BsMapLayout
	// mapHoverRow 是指针所在的楼层行，-1 表示没有悬停。
	mapHoverRow int
	// 以下字段属于満淫電車2 的「言葉責めシステムNEO」。wordUI 非 nil 时
	// 手机界面是模态的，脚本必须停住；wordActive 控制右上角仪表的显示。
	words       []md2Word
	wordsLoaded bool
	wordTemp    []int
	wordSaved   [wordSavedPages][wordSavedPerPage]int
	wordUI      *wordUIState
	wordActive  bool
	// wordBurn/wordBurnLocked 是バーニングゲージ的整场事件状态。
	// 手册：每次升级时若压力量仍为 0 就 +1（上限 4）；压力一旦上升，
	// 燃烧清零且本场事件内不再增长。必须放在 Session 而不是 wordPlay：
	// wordPlay 每回合重建，放里面会让燃烧永远停在 1。
	wordBurn       int
	wordBurnLocked bool
	// wordPlay 是本回合逐词反应的进行中状态：词标签（words%03d）之间靠它
	// 记当前词/进度，回合结算时靠它记 ecstasyBefore/stressBefore 判定跨级。
	wordPlay *wordPlayState
	// md2DressStage/md2DressPen 是満淫電車2 DRESS 画面的全局着衣阶段
	// （0 着衣/1 下着/2 全裸）与透視模式（0..3）。这一代没有角色/服装
	// 列表，设置对所有立绘统一生效。
	md2DressStage int
	md2DressPen   int
	// md2DressPersp 是 MD2 DRESS 画面拖拽透視的掩码（不能存档）。
	md2DressPersp *md2DressPerspState
	// md2Map 表示当前停在満淫電車2 这一代的精灵图车站选择画面。
	// 它和 mapSource/bsmap 是两套数据：脚本已经把 map_base 铺成背景，
	// 引擎只补画站名/头像/箭头，不能再调用 showMapStage 覆盖舞台。
	md2Map bool
	// mapPreview 是学園2 地图上已点开的地点下标，-1 表示没有预览。
	// 原版点地点后先显示 map_backN 的「Move Place」卡片，再点一次才移动。
	mapPreview int
	// mapSystemCount 是地图选择态里紧跟在角色卡之后的系统按钮数量。它们的下标是
	// len(mapItems)..len(mapItems)+mapSystemCount-1，点击只打开系统界面，不写地图结果。
	mapSystemCount int
	// mapSystemTools 与系统按钮一一对应；profile 直接给出功能键时优先用它，
	// 不再从图片名反推（GK 的 map_submenu_button 一图六用，名字认不出功能）。
	mapSystemTools []string
	// lastAutoSavePC 防止同一张地图/选择肢重复写自动档。
	lastAutoSavePC int
	// pendingAutoSave 表示地图铺好后要写一次自动档。等 Compose 把地图
	// 底图/日期卡/头像都画完再写，缩略图才是玩家看到的这一屏。
	pendingAutoSave bool
	// mapPointerX/Y 是地图上最近一次指针位置，用于画 map_site 十字准星。
	mapPointerX int
	mapPointerY int
	// staffroll 是片尾字幕的滚动状态；active 为 false 时不起作用。
	staffroll staffrollState
	// eyecatch 是章节间过场的短动画状态。
	eyecatch eyecatchState
	// skipOverlay 是宿主"跳过中"状态，用于画原版 SKIPPING 条。
	skipOverlay bool
	// selectButtons 保存上一次地图选择里的按钮，用于“点了不可用卡后
	// 剧本原地重新 SELECT_TAIL”的循环。
	//
	// KZK 的 map_repeat 在 #select_value == 1000（锁着的卡）时直接
	// GOTO map_repeat，不会重新铺卡；而 EndSelect 已经把按钮表清空，
	// 若不在下一次 SELECT_TAIL 前恢复，地图会变成一张没有热区的空图，
	// 玩家永久卡住。只在现代 BSXScript 的脚本地图里使用。
	selectButtons []state.StageButton
	replay        bool
	disposed      bool
	audio         []core.AudioEvent
	lastBgm       string
	lastBgmOn     bool
	lastEffect    string
	lastEffectN   int
	lastVoice     string
	lastVoiceN    int
	// voicePlaying 记录"我们发过一条 voice、宿主那边可能还在响"。
	// 推进对白时用它决定要不要补一条 stopVoice：见 audio.go 的 stopVoiceForAdvance。
	voicePlaying bool
	lastSeqN     int
	// titleMode 表示引擎当前停在主界面。
	// 放在 Session 而不是 GameState：它是"引擎流程状态"，不该被剧情 Reset 清掉。
	titleMode bool
	// titleBootState 是旧式 BSScript（Rev.2–Rev.6）在请求标题画面时的 VM 快照。
	//
	// 这些作品的 boot 标签负责把 #属性/@变量初始化成新周目的初始值，然后
	// 用 system call "title" 停到标题。玩家点 START 时不能像现代剧本那样
	// 用 EntryState 重建全零状态，否则 boot 算出来的初始属性会丢（KD 的
	// #result 掩码、周目计数都依赖它）。所以进标题前抓一份快照，进剧情时
	// 恢复快照再跳到 opening/prologue。
	titleBootState *vm.MachineState
	// targetAutoConfirm 表示刚在原生目标选择屏点完卡片，脚本接下来那个
	// 只有「ＯＫ／選び直す」的 SELECT 要自动选第一项。放課後2 的流程是
	// “点卡片 → 写 #result → 脚本确认”，原版也只需要一次点击。
	targetAutoConfirm bool
	// ancientLayout 表示当前作品套用了内置 Ancient 布局（特別病棟 旧版/Win10）。
	// 这类脚本的 boot 标签负责初始化新周目变量，然后 system call "title"；
	// 如果像现代剧本那样直接进标题，会跳过 boot，进剧情后属性/日期全为零。
	ancientLayout bool
	lastStep      core.Step
	loggedDay     int
	trace         []string
	// history 是引擎侧的对白历史，供「ログ」按钮/回想界面读取。
	// 放在 Session 而不是让宿主自己攒：宿主只拿到当前一句，切场景、读档后
	// 没有完整上下文；引擎在每次停在 AwaitInput 时记录最可靠。
	history    []core.TextUnit
	info       core.Info
	cg         *data.CgCatalog
	cgDone     bool
	scenes     *data.SceneCatalog
	scenesDone bool
	// global 是原引擎的全局存档（save/common.dat），保存跨周目的鉴赏解锁状态。
	// globalDone 区分"还没读"和"读了但文件不存在"（后者是正常的全新安装）。
	global     *save.GlobalSave
	globalDone bool
	// globalDirty 表示内存里的解锁状态有变化、还没落盘。
	// 落盘要写整个文件，所以攒着写：只在切场景/存档/退出时 commit。
	globalDirty bool
	// sceneOffsets 是"代码偏移 -> 场景下标"的反查表，用于判定进入了哪个可回放场景。
	sceneOffsets map[int]int
	// globalPending 记录自上次落盘以来又停了多少句对白，用来限制写盘频率。
	globalPending int
	// ui 是引擎自己绘制的系统界面（存档/读档、鉴赏室）。
	//
	// 这些界面必须由引擎画：底图、按钮图、槽位坐标和缩略图都在游戏资源 / bsxx.cfg
	// 里，宿主重画一份必然和原版错位。ui 非 nil 时脚本不推进，点击先交给界面处理。
	ui *systemUI
	// mddvd 是 MDDVD 的核心「言葉責め」界面状态（system call XA）。
	// 非 nil 时脚本停住，由 Pointer/Compose 接管；选择完成后写回
	// #atk_result / #atk_best… 再继续执行。其它作品永远为 nil。
	mddvd *mddvdPlayState
	// mddvdTimeUnits 是 XT420 解析出的帧计时器；XA 打开时消费一次。
	mddvdTimeUnits int
	// mddvdMuseumFromScript 表示当前博物馆是脚本 `sceneroom` 打开的：
	// 关闭后必须回到脚本下一条，而不是像标题鉴赏那样回标题。
	mddvdMuseumFromScript bool
	// mddvdPrefixes 是 XB1..XB6 为六名角色写入的 fNN_00 前缀。原引擎
	// 用它给每个角色的言葉責め/结果场景定位；这里保留同一份状态。
	mddvdPrefixes [7]string
	// cgResume 保存"从 CG 查看器点 CFG 打开设置"前的查看器状态。
	// 设置画面会销毁 systemUI，关闭后要回到同一张 CG / 同一差分，
	// 而不是退回标题。只有 KD 的 CG 查看器有 CFG 按钮。
	cgResume *cgViewerResume
	// menuVisible 表示游戏内底部菜单条是否展开。原引擎按鼠标位置自动显示，
	// 触屏上没有 hover，所以由右键 / ESC / 宿主菜单按钮切换。
	menuVisible bool
	menuHover   int
	// replayReturn / replayPage 记录"从鉴赏室的场景回放进来的"，回放结束后
	// 要回到鉴赏室原来的页，而不是像剧情回放那样交给宿主结束页。
	replayReturn screenKind
	replayPage   int
	// replayTitleMode 记录回放前是否停在主界面。鉴赏室是从标题进来的，
	// 回放时为了让脚本能跑会把 titleMode 清掉；结束时必须恢复，
	// 否则关闭鉴赏室会回到剧情而不是标题，也会影响存档画面的返回行为。
	replayTitleMode bool
	// hostRequest 是给宿主的界面动作（打开设置、退出播放页等）。
	hostRequest       string
	hostRequestSerial uint32
	// thumbs 缓存鉴赏室/存档画面的缩略图，避免每帧缩放整张 3MB 的 CG。
	thumbs map[string]*thumbCacheEntry
	// settings 是原版设置画面里的可调项（音量/文字速度/演出开关）。
	// 它属于引擎而不是宿主：音量要乘进 AudioEvent，换作品时默认值也来自 bsxx.cfg。
	settings GameSettings
	// dressLive 是 DRESS 画面已经确认的“角色 → 服装/脱衣阶段”。
	//
	// 原版 DRESS 不只是预览：关掉画面后正常剧情的立绘也会保持选好的
	// 服装/脱衣状态（手册的「通常画面で表示されるキャラクターの立ち絵を
	// 下着姿や全裸に変更」）。它不写进存档，所以放在 Session 而不是
	// SaveData/GameSettings；key 是角色编号（1 基），和 buNN 的 NN 一致。
	dressLive map[int]dressLiveChoice
	// configScreen 是 define.bsa 里的 system_config_ex.layout 解析结果。
	// nil 表示这部作品没有扩展设置画面，此时 title/menu 的 config 退回宿主动作。
	configScreen *config.ConfigScreen
	// actorSamples 是设置画面 "actor setting" 里每个角色的样本语音、样本 BGV
	// 和头像图名。它们只用于试听和显示头像，不参与正式剧情播放。
	actorSamples []actorSample
	// actorsLoaded 表示角色样本是否已经解析过，避免每帧重复读布局树。
	actorsLoaded bool
	// bgmSelectTable 是 define.bsa 里 bgm_select 给出的曲名映射：
	// key 是 type0 的曲名，value 是 type1 的曲名。
	bgmSelectTable map[string]string
	// bgmSelectLoaded 表示映射表是否已经读过一次。
	bgmSelectLoaded bool
	// messageHover 是鼠标命中的底部工具按钮下标；-1 表示没有悬停。
	messageHover int
	// lastMessageUnread 记录当前停在屏幕上的对白在显示前是否未读。
	// 必须在 markMessageRead 之前采样，否则永远读到"已读"。
	lastMessageUnread bool
	// lastPointerX/Y 是最后一次指针位置，供没有坐标的 Wheel 事件判断
	// 当前在 BGV/服装列表的哪一栏。
	lastPointerX int
	lastPointerY int
	// extraWindowSeenAt 是指针最后一次进入 [extra window] 显示区的时刻，
	// 用于 auto hide：离开后超过配置的毫秒数才收起面板。
	extraWindowSeenAt time.Time
	// freeSightActive 表示玩家这次对话里按过「顔/胸/股間」，自由视角的
	// 滑杆和按钮要跟着显示。ViewMode 的 0 和"看脸"同值，不能用它判断。
	freeSightActive bool
	// playlist 是场景鉴赏里玩家自己加入的"播放列表"（场景回放标签）。
	// 它是跨周目的偏好，存在 save/kizuna-playlist.json，和存档槽位无关。
	playlist       []string
	playlistLoaded bool
	// toolbarVisible 控制引擎消息框底部工具条是否显示（原版「メニュー表示」）。
	// 隐藏后仍可用 ESC/宿主菜单按钮重新显示，不会把玩家锁死。
	toolbarVisible bool
	// toolNotice 是点工具按钮后的短暂反馈（カットイン/HOLD/メニュー表示）。
	// 这些开关本身没有专属画面，给玩家一个可见的 ON/OFF 提示。
	toolNotice   string
	toolNoticeAt time.Time
	// choicePickedAt 记录刚选中选项/地图卡/工作按钮的时刻：宿主把一次点按
	// 拆成 press→release 两次调用，release 不能紧接着推进下一句或再点一次。
	choicePickedAt time.Time
	// textFont 是引擎自绘界面共用的字体渲染器（对话框、鉴赏列表、设置说明）。
	// 只解析一次内嵌字体，避免每个界面各自加载 3.5MB 字体。
	textFont *textrender.Renderer
	// unlockAll 表示目录是从资源名临时推导出来的（作品没有 cg.dat/scene.dat）。
	// 这种目录没有和原引擎名字表一致的索引，无法把 common.dat 的解锁位对到
	// 正确的图上；发行版自带「全cg」时，按全部已解锁展示比整页空白更符合预期。
	unlockAll bool
	// imageNames 缓存资源索引里的全部图片名，推导目录时只枚举一次。
	imageNames []string
	// sceneLabels 缓存"EV 图 → 真实回放标签"的探测结果，场景/CG 目录共用。
	sceneLabels map[string]string
	// sceneCovers 缓存"EV 图 → 初始构图图层"。无目录发行版按图名猜封面
	// 会漏掉多人场景的其他角色部件，这里用一次只读探测拿到剧本真正画的图。
	sceneCovers map[string][]string
	// statusLayout 是"状態"画面布局（define.bsa:status_layout）。
	// statusDone 区分"还没读"和"读了但资源不存在"，避免每帧重复读 BSA。
	statusLayout *config.StatusScreenLayout
	statusDone   bool
	// chartDB 是 CHART 流程图的格子/连线数据（define.bsa:chart.db）。
	// 它和 status_layout 是两份资源：前者描述"画什么"，后者描述"画在哪"。
	chartDB     *config.ChartDatabase
	chartDBDone bool
	// chartRestore 保存"从 CHART 进入场景回放"前的页码和滚动位置。
	// 回放结束会重新打开 CHART（beginSystemUI 会重建界面状态），
	// 没有这份快照玩家每次看完回想都会被弹回第一页。
	chartRestore *chartRestoreState
	// currentSceneLabel 是剧本当前所在的场景标签（L01_002 这种）。
	// CHART 用它标出"进行中"的格子，也让"显示当前场景"按钮有目标；
	// 标签在 markSceneUnlockedAt / registerSceneEntry 里随剧情更新。
	currentSceneLabel string
	// scriptDB 是 define.bsa 的 var_db 表（#ID_BP/#ID_TC/#ID_LOG/@ID_*）。
	// 只有 BSXScript 3.3 的 SLG 作品使用；普通 AVG 没有 var_db 时保持 nil。
	scriptDB     *scriptdb.DB
	scriptDBDone bool
	// achievements 是 define.bsa:achievement.db 的成就表。
	achievements     *data.AchievementTable
	achievementsDone bool
	// unlockedAchievements 记录本局已解锁的成就 ID；跨周目状态同时写入
	// common.dat，内存里的副本只用于"刚解锁时弹提示"和状态画面。
	unlockedAchievements map[string]bool
	// slgState 是脚本 `state training/territory/day_result` 激活的原版
	// Lua 界面状态；nil 表示当前不在 SLG 子界面。
	slgState *slgScreenState
	// slgVM 是 embed.bsa 的 Lua 运行时（训练/领地/日结果界面）。
	slgVM *lua.Runtime
	// customWindows 是 [custom info windows] 登记的 Lua 窗口（cs1..cs3/mdt），
	// 在剧本自绘地图上叠加显示。
	customWindows []string
	// slgExit 是"界面要求结束时，下一次 Run/Compose 收尾"的标记。
	// 不在 Lua 回调里直接跳剧本，避免在绘制中途改 VM 状态。
	slgExit bool
	// privateData/commonData 对应 SLG 作品 Lua 的 PrivateData / CommonData：
	// 前者每存档一份，后者跨周目共享。JSON 序列化后随存档写出。
	privateData map[string]any
	commonData  map[string]any
	// animWaitUntil 是 `is animeted` 的等待截止时间。脚本用它等一段演出
	// 播完；没有真正的动画系统时按一个短时限放行，避免 Run 内死循环。
	animWaitUntil time.Time
	// statusPage/statusScroll 是状態画面的当前页与滚动行。
	// 牝教師3 是单页成就列表，特別授業3SLG 是四页标签；两者共用这两个字段，
	// 因为同一时刻只会打开一个状態画面。
	statusPage   int
	statusScroll int
	// customPanels 是 type=176 工作画面的四块角色面板（SLIE）。
	// 按坐标替换：AM 面板被 PM 面板覆盖。
	customPanels []customUIPanel
	// customDefs/customDefMissing 缓存 define.bsa 里的 .txt 界面定义。
	customDefs       map[string]*config.CustomUIDefinition
	customDefMissing map[string]bool
	// customHover 是工作画面单选项的悬停位置。
	customHover customUIHover
	// customMapHelp 表示 SL 的 HELP 整屏说明图正在显示；任意点击关闭。
	customMapHelp bool
}

// thumbCacheEntry 是一张已经缩放好的缩略图。
type thumbCacheEntry struct {
	canvas *gfx.BgraCanvas
	used   int
}

var (
	_ core.Engine           = (*Session)(nil)
	_ core.OpenerExtras     = (*Session)(nil)
	_ core.MapExposer       = (*Session)(nil)
	_ core.GalleryComposer  = (*Session)(nil)
	_ core.SlotPreviewer    = (*Session)(nil)
	_ core.OptionSetter     = (*Session)(nil)
	_ core.Closer           = (*Session)(nil)
	_ core.TitleMenuExposer = (*Session)(nil)
	_ core.MovieExposer     = (*Session)(nil)
	_ core.MovieFinisher    = (*Session)(nil)
	_ core.GalleryUnlocker  = (*Session)(nil)
	_ core.KeyReceiver      = (*Session)(nil)
	_ core.NamePrompter     = (*Session)(nil)
	_ core.WheelReceiver    = (*Session)(nil)
	_ core.StepReporter     = (*Session)(nil)
	_ bsmap.BsPropertyView  = (*Session)(nil)
)

func New() core.Engine {
	return &Session{lastStep: core.StepContinue, md2DressStage: -1}
}

func (s *Session) Open(dir string) (core.Info, error) {
	return s.OpenExtras(dir, nil, "")
}

func (s *Session) OpenExtras(dir string, extraDirs []string, cacheDir string) (core.Info, error) {
	dataDir, err := resolveDataDir(dir)
	if err != nil {
		return core.Info{}, err
	}
	scriptPath, err := findScript(dataDir)
	if err != nil {
		return core.Info{}, err
	}
	// 旧版把脚本、Graphics2、system.prm 分在 Disk1/Disk2/Disk3：
	// dataDir 只是脚本目录，资源索引还要把同级 Disk* 都收进来。
	// 顺序上 dataDir 永远第一，额外根只补缺失的名字，不会覆盖主根。
	roots := resourceRoots(dir, dataDir)
	for _, extra := range extraDirs {
		roots = append(roots, extra)
	}
	res := resource.Build(dataDir, roots[1:]...)
	cfg, err := config.Load(findConfigurationInRoots(roots))
	if err != nil {
		res.Dispose()
		return core.Info{}, err
	}
	// 三射面談（SM）的 bsxx.cfg 没有 eye pattern。留空时会退回 6，而素材
	// 实际是 4 帧，表现为立绘眼睛消失/一列多行。只在缺项时按素材推断，
	// 配置里写了的作品不受影响。
	if !cfg.HasEyePattern {
		if inferred := inferEyePattern(res); inferred > 1 {
			cfg.EyePattern = inferred
			core.Logf("session: eye pattern inferred from face assets = %d", inferred)
		}
	}
	var window *state.ScreenSize
	if cfg.HasWindowSize {
		window = &state.ScreenSize{Width: cfg.Width, Height: cfg.Height}
	}
	if resolved := state.ScreenSizeResolve(probeIndex{res: res}, window); resolved != nil {
		cfg.ApplyScreenSize(*resolved)
	}
	width := cfg.Width
	if width <= 0 {
		width = 1024
	}
	height := cfg.Height
	if height <= 0 {
		height = 768
	}
	core.Logf("session: open '%s' dir='%s'", filepath.Base(dir), dataDir)
	script, err := vm.LoadBsxScriptFile(scriptPath)
	if err != nil {
		res.Dispose()
		return core.Info{}, err
	}
	core.Logf("session: script '%s' labels=%d messages=%d", script.Signature(), script.LabelCount(), script.MessageCount())
	// 旧式 BSScript（Rev.2–Rev.6）的作品常常不带 bsxx.cfg：原引擎把标题菜单、
	// 对话框和存读档页的布局直接编译进自己的 exe。这里按"配置为空 + 资源签名
	// 吻合"补一份内置布局，否则 KD 这类作品没有标题、消息框和工具条。
	// 只填空缺项，已有配置的现代作品完全不受影响。
	if config.ApplyBuiltinBishopDefaults(cfg, config.BuiltinProbe{
		Exists:    res.Exists,
		RevScript: script.IsBsScript() && script.IsLegacy() && script.LayoutKind() != vm.LayoutAncient,
		// Ancient 布局（BSScript 无 Rev 号）单独标记：MDDVD 的内置布局
		// 只吃这一代，不能把 Rev 作品误判进来。
		AncientScript: script.IsBsScript() && script.LayoutKind() == vm.LayoutAncient,
		Width:         width,
		Height:        height,
		// Header 只给需要按图头尺寸确认世代的指纹使用（スレイブポリス
		// 的 title_menu/message）；其它内置布局仍然只看素材名，不受影响。
		Header: func(name string) (config.BuiltinImage, bool) {
			location, ok := res.TryGet(name)
			if !ok {
				return config.BuiltinImage{}, false
			}
			return config.ParseBuiltinImage(location.ReadPrefix(0x40))
		},
	}) {
		if cfg.Width > 0 {
			width = cfg.Width
		}
		if cfg.Height > 0 {
			height = cfg.Height
		}
		core.Logf("session: applied built-in BSScript layout (%dx%d)", width, height)
	}
	// MDDVD（満淫電車 DVD特別版，Ancient 布局、无配置）必须先判：
	// 它的素材签名（title_higo/higo_base/cg0_base/dm_slot/aw_mobile）与
	// 特別病棟 Ancient 内置布局完全不相交，判错会把标题和对话框画成另一部作品。
	if config.ApplyBuiltinMddvdDefaults(cfg, config.BuiltinProbe{
		Exists:        res.Exists,
		AncientScript: script.IsBsScript() && script.LayoutKind() == vm.LayoutAncient,
		Width:         width,
		Height:        height,
	}) {
		width = cfg.Width
		height = cfg.Height
		core.Logf("session: applied built-in MDDVD layout (%dx%d)", width, height)
	}
	if script.LayoutKind() == vm.LayoutAncient && config.ApplyBuiltinAncientDefaults(cfg, config.BuiltinProbe{
		Exists: res.Exists,
		Width:  width,
		Height: height,
	}) {
		width = cfg.Width
		height = cfg.Height
		s.ancientLayout = true
		core.Logf("session: applied built-in BSScript (Ancient) layout (%dx%d)", width, height)
	}
	// 教育指導（KS）的 bsxx.cfg 没有 [system button]，原引擎把消息窗
	// 工具条的坐标编译在 exe 里。只在按钮表为空时补，KSIE 自己的配置优先。
	if config.ApplyBuiltinKsToolbar(cfg, config.BuiltinProbe{
		Exists: res.Exists,
		Width:  width,
		Height: height,
	}) {
		core.Logf("session: applied built-in KS message toolbar (%d button(s))", len(cfg.GameMenu.Buttons))
	}
	// 最后一道兜底：cfg 里缺 [title phase] / [message window]、但素材确实
	// 存在时，按 BSG 图头的坐标补出标题与对话框。只填空缺段，已有配置
	// 的作品（侵蝕/淫獄/MK2IE 等）完全不受影响。
	if config.ApplyBuiltinGenericFallbacks(cfg, config.BuiltinProbe{
		Exists: res.Exists,
		Width:  width,
		Height: height,
		Header: func(name string) (config.BuiltinImage, bool) {
			location, ok := res.TryGet(name)
			if !ok {
				return config.BuiltinImage{}, false
			}
			return config.ParseBuiltinImage(location.ReadPrefix(0x40))
		},
	}) {
		core.Logf("session: applied generic title/message fallback")
	}
	// 牝教師（MK/MKIE）的 #day 从 0 起，而原作第一天是火曜日；map_week
	// 从 MONDAY 开始排 7 帧。内置的 MK 布局已经写 1，MKIE 的 bsxx.cfg 没有
	// 这一项，按同一套素材指纹补上，避免地图上的曜日每天慢一天。
	if cfg.MapPhase.WeekDayShift == 0 && cfg.MapPhase.WeekDay.Image != "" && res.Exists("miyu1_norm") {
		cfg.MapPhase.WeekDayShift = 1
	}
	machine := vm.NewBsxMachine(script)
	machine.RestartBoot = false
	gameState := state.NewGameState()
	// 姓名占位符先用 Startup.ini 的玩家设定，其次用已知作品的默认值；
	// TextDirector 在拼消息前替换。
	gameState.SetPlayerName(resolvePlayerName(dir, dataDir))
	movies := collectMoviesInRoots(roots)
	gameState.SetMovieFiles(movies)
	core.Logf("session: %d movie file(s)", len(movies))
	for _, movie := range movies {
		core.Logf("session: movie '%s'", movie)
	}
	composer := rendering.NewSceneComposer(res, width, height, cfg.EyePattern, cfg.EyeAnimationMilliseconds,
		cfg.Counter(), cfg.Information, cfg.GameMenu, cfg.Message, cfg.Font)
	composer.SetOverlay(s.drawCustomOverlay)
	// gameRoot 仍然传 dataDir（存档/缓存路径以脚本目录为准），
	// system.prm 由 profile 优先从资源索引读取，索引已经覆盖所有 Disk 根。
	catalog := bsmap.BuildBsMapCatalog(res, script, dataDir)
	bsmap.BsMapCatalogInstall(catalog)
	// BsSystemDirector 用自己包里的接口变量判断"有没有地图可开"；它只依赖
	// IsEmpty，不直接引用 bsmap（避免 vm/bs 与 bsmap 互相导入）。以前这里只
	// 装了 bsmap 的全局变量，d 那边永远是 nil，KD 这类 BSScript 作品每次
	// system call "map" 都被当成"无候选地点"继续执行，主循环于是空转到
	// 下一次 restart，表现就是序章结束后直接 GAME OVER/回标题。
	bs.InstalledMapCatalog = catalog
	core.Logf("session: map catalog bsscript=%v", machine.IsBsScript())
	var router dispatcher
	if machine.IsBsScript() {
		router = bs.NewBsScriptRouter(machine, machine.BsScript(), gameState, cfg, res)
	} else {
		router = runtime.NewScriptRouter(machine, script, gameState, cfg, res)
	}
	// 缓存按游戏分目录：宿主传进来的是缓存根（cache/kizuna），这里再加一层由游戏
	// 目录算出的 md5 子目录。同一个游戏每次游玩都落到同一个目录，不同游戏之间
	// 音频物化文件、engine.log 互不覆盖；卸载游戏时也能整目录删掉。
	// 传空表示宿主没有可写的缓存根（无平台实现/无权限），退回进程临时目录。
	if cacheDir == "" {
		cacheDir = core.GameCacheDir(defaultCacheDir(), dataDir)
	} else {
		cacheDir = core.GameCacheDir(cacheDir, dataDir)
	}
	core.EnsureDir(cacheDir)
	core.Logf("session: cache dir '%s'", cacheDir)
	s.dir = dir
	s.dataDir = dataDir
	s.extraDirs = append([]string(nil), extraDirs...)
	s.lookupRoots = append([]string(nil), roots...)
	s.cacheDir = cacheDir
	s.title = filepath.Base(dir)
	s.width = width
	s.height = height
	s.config = cfg
	s.res = res
	// 设置默认值来自 [default volume]，必须在 config 赋值之后再加载。
	s.loadGameSettings()
	s.composer = composer
	s.applyDressSuffix()
	// 设置文件里的显示项要在第一次合成之前铺进渲染器：
	// 否则关掉「x countdown」后还要等玩家进一次设置画面才生效。
	s.applyDisplaySettings()
	s.script = script
	s.machine = machine
	s.router = router
	// MDDVD 的 XA/XT 是作品专属调用；只有内置 MDDVD 布局生效时
	// 才安装，避免影响其它 BSScript 作品的通用 system call 分派。
	s.installMddvdPlayHandler()
	if bsRouter, ok := router.(*bs.BsScriptRouter); ok {
		s.bsStage = bsRouter.StageDirector()
		// 旧式脚本用 `system call "sceneroom"` 主动打开场景鉴赏；
		// 引擎侧必须接住，否则脚本只记一条日志继续跑，玩家看不到界面。
		bsRouter.SetScreenHandler(s.handleBsScreenRequest)
	}
	// DRESS 的服装/脱衣状态要在后续立绘解析前生效（见 dress_live.go）。
	if modern, ok := s.router.(*runtime.ScriptRouter); ok {
		modern.SetBustupOverride(s.applyDressToBustupName)
	}
	// SLG 宿主命令（SELECT/SELECT_TAIL/TRANSLATE 参数）只在有 var_db 的
	// 作品里出现；回调内部会自己判断字符串，普通 AVG 不受影响。
	s.installSLGHost()
	s.mapCatalog = catalog
	s.gameState = gameState
	s.messageHover = -1
	s.toolbarVisible = true
	if font, err := textrender.New(); err == nil {
		s.textFont = font
	} else {
		core.Logf("textrender: %v", err)
	}
	// embed.bsa 的训练/领地/日结果界面是 Lua 写的；启动时执行 boot.lua
	// 把界面对象与数据表准备好。没有 embed 的作品直接跳过。
	s.openSLGLua()

	// 扩展设置画面来自 define.bsa 的 system_config_ex.layout；解析失败只丢这个
	// 界面，不影响游戏本体。没有这个文件的作品会继续走宿主设置页。
	//
	// 光有 layout 不代表能用：黒の教室 的 define.bsa 里带的是通用版 layout，
	// 但它实际使用 bsxx.cfg 的旧式 [config screen] 和 cfg_* 素材，layout 引用的
	// cfgx_base 等图一张都不存在。直接打开会得到纯黑画面，所以先确认底图存在，
	// 缺图就丢弃这个界面，让调用方回退到宿主设置页。
	if location, ok := res.TryGet("system_config_ex.layout"); ok {
		if layout, err := config.LoadConfigScreen(location.Read()); err == nil {
			if len(layout.Base) == 0 {
				s.configScreen = layout
				core.Logf("settings: loaded %d category page(s)", len(layout.Categories))
			} else if _, found := res.TryGet(layout.Base); found {
				s.configScreen = layout
				core.Logf("settings: loaded %d category page(s)", len(layout.Categories))
			} else {
				core.Logf("settings: layout base '%s' is missing, using host settings", layout.Base)
			}
		} else {
			core.Logf("settings: layout parse failed: %v", err)
		}
	}
	s.saves = save.NewSaveStoreWithSlots(dataSaveDir(dataDir), save.GridSlotCount)
	s.story = map[int]bool{}
	for _, candidate := range storyEntryCandidates {
		if offset := script.FindLabel(candidate); offset >= 0 {
			s.story[offset] = true
		}
	}
	s.sawMessage = false
	s.entryUsed = false
	s.storyIn = false
	s.valueSlot = unresolvedSlot
	s.typeSlot = unresolvedSlot
	s.counterSlot = unresolvedSlot
	s.mapItems = nil
	s.mapBackup = nil
	s.mapMask = nil
	s.mapSource = ""
	s.mapShown = false
	s.mddvd = nil
	s.mddvdTimeUnits = 0
	s.mddvdMuseumFromScript = false
	s.replay = false
	s.disposed = false
	s.audio = nil
	s.lastBgm = ""
	s.lastBgmOn = false
	s.lastEffect = ""
	s.lastEffectN = 0
	s.lastVoice = ""
	s.lastVoiceN = 0
	s.voicePlaying = false
	s.lastSeqN = 0
	s.lastStep = core.StepContinue
	s.trace = nil
	s.history = nil
	s.cg = nil
	s.cgDone = false
	s.scenes = nil
	s.scenesDone = false
	s.catalog = s.buildCatalog()
	s.info = core.Info{
		Width:      width,
		Height:     height,
		Title:      s.title,
		Version:    script.Signature(),
		SceneCount: len(s.catalog),
		EntryCount: len(s.catalog),
	}
	return s.info, nil
}

func (s *Session) Info() core.Info {
	return s.info
}

func (s *Session) Start(entry uint32) (core.Step, error) {
	if s.machine == nil {
		return core.StepFaulted, errNotOpen
	}
	if entry == 0 {
		s.gameState.Reset()
		s.clearMapStage()
		s.machine.RestoreState(vm.EntryState(0))
		s.machine.RestartBoot = !s.script.IsLegacy()
		s.titleBootState = nil
		s.targetAutoConfirm = false
		s.replay = false
		s.storyIn = false
		s.entryUsed = false
		s.sawMessage = false
		s.lastAutoSavePC = -1
		s.resetAudioCache()
		s.lastStep = core.StepContinue
		s.pollAudio()
		// 打开就把总存档准备好：原引擎一进游戏就有一份 save/common.dat，
		// 鉴赏界面、进度标记都依赖它。缺失时在这里补一份空档。
		s.ensureGlobalSave()
		// 旧式 BSScript 的标题是脚本自己请求的：boot 标签把新周目的初始属性
		// 算好，再执行 system call "title" 停下。必须先跑 boot，不能像现代
		// 剧本那样直接铺标题——RestoreState(EntryState(0)) 会把 PC 拉回 0
		// （KD 的 0 是 macKDFlash 子程序），既跳过 boot 初始化，又会在 title
		// 的 syscall 之后顺势落入 opening。
		if boot := s.revBootOffset(); boot >= 0 {
			if s.machine.SeekBs(boot) {
				core.Logf("session: rev boot @ %d, waiting for the script to request the title", boot)
				return core.StepContinue, nil
			}
		}
		// Ancient 作品（特別病棟 旧版/Win10）同样是 boot 初始化后 system call title。
		// 只在套用了内置 Ancient 布局时走这条路径：别的 Ancient 脚本入口约定
		// 可能不同，贸然 Seek 到 boot 会让没有 title 调用的作品直接跑完序章。
		if s.ancientLayout {
			if boot := s.script.FindLabel("boot"); boot >= 0 {
				if s.machine.SeekBs(boot) {
					core.Logf("session: ancient boot @ %d, waiting for the script to request the title", boot)
					return core.StepContinue, nil
				}
			}
		}
		// 没有 boot 标签（或 seek 失败）时保持老行为：现代作品先停在标题，
		// 玩家点 START 才跑脚本；直接跑 boot 会把标题画面的演出整段跳掉。
		if step := s.enterTitle(); step == state.SessionAwaitTitle {
			s.lastStep = core.StepAwaitTitle
			return core.StepAwaitTitle, nil
		}
		return core.StepContinue, nil
	}
	index := int(entry)
	if index < 0 || index >= len(s.catalog) {
		return core.StepFaulted, errBadEntry
	}
	if !s.startReplay(s.catalog[index].Name) {
		return core.StepFaulted, errBadEntry
	}
	s.lastStep = core.StepContinue
	s.pollAudio()
	return core.StepContinue, nil
}

func (s *Session) Run(budget uint32) (core.Step, error) {
	// 系统界面/菜单条展开时脚本必须停住：它们是模态界面，背后继续跑指令会
	// 让玩家在存读档时把剧情推过去（甚至把刚保存的位置改掉）。
	if s.ui != nil || s.menuVisible {
		return s.lastStep, nil
	}
	// 手机（言葉責め/言葉整理）是模态界面：选择完成前不推进脚本。
	if s.wordUI != nil {
		return s.lastStep, nil
	}
	// MDDVD 的言葉責め界面是模态的：选择结束前不推进脚本。
	if s.mddvd != nil {
		return s.lastStep, nil
	}
	if s.gameState != nil && s.gameState.AwaitingChoice() {
		return s.lastStep, nil
	}
	step := s.run(int(budget))
	return s.finishRun(step), nil
}

func (s *Session) run(budget int) state.SessionStep {
	defer s.pollAudio()
	if s.disposed {
		return state.SessionFinished
	}
	if s.machine == nil {
		return state.SessionFaulted
	}
	// 主界面上脚本是停住的：Run 是宿主每帧都会调的心跳，如果这里继续执行指令，
	// 标题会在一帧之内被剧情冲掉（玩家根本没机会点 START）。只有 SelectTitleItem
	// 选了 START 之后 titleMode 才会被清掉，那时才真正开始跑脚本。
	if s.titleMode {
		return state.SessionAwaitTitle
	}
	if s.slgState != nil {
		// Lua 界面激活期间脚本不推进；每帧的 update 在 Compose/Run 里驱动。
		if s.slgVM != nil && s.slgVM.Update(s.lastPointerX, s.lastPointerY, false) {
			return s.finishSLGState()
		}
		return state.SessionAwaitInput
	}
	if budget <= 0 {
		budget = commandBudget
	}
	for executed := 0; executed < budget; executed++ {
		if s.replay && s.machine.Halted() {
			return s.finishReplay()
		}
		command := s.machine.Fetch()
		if s.machine.Faulted() {
			s.record("fault at " + strconv.Itoa(s.machine.InstructionStart()))
			return state.SessionFaulted
		}
		if !s.storyIn && s.story[s.machine.InstructionStart()] {
			s.storyIn = true
			core.Logf("session: story entry reached")
		}
		// 每执行到一条指令就判一次"是不是进了某个可回放场景"。
		// 用 map 查而不是字符串比较，热路径上只是一次哈希。
		s.markSceneUnlockedAt(s.machine.InstructionStart())
		if !s.machine.IsBsScript() {
			s.prepareMapSelectButtons(command)
		}
		step := s.router.Dispatch(command)
		if command == vm.DisplayMessage {
			// 台词里的 <U n> 是责め言葉占位符：替换成原文并记进手机记忆。
			s.applyWordCodes()
		}
		if step == state.SessionAwaitChoice {
			// 手册：选择肢出现之前自动存档。选项已经 FETCH 完，Save 会
			// 连同 ChoiceOptions 一起写进扩展段，读档能恢复同一组选项。
			s.autoSaveOnce()
		}
		if step == state.SessionContinue && !s.machine.IsBsScript() &&
			command == vm.SelectTail &&
			isCustomMapStatement(s.script.GetName(int(s.machine.Frame(1)))) &&
			len(s.mapItems) == 0 &&
			len(s.gameState.Buttons()) == 0 && len(s.selectButtons) > 0 {
			// 剧本原地重发 SELECT_TAIL（KZK 的锁卡循环）：ApplySelect 看到
			// 按钮表为空会返回 Continue，脚本会一路空转到 SELECT_TAIL，
			// 玩家就卡在一张没有热区的空地图上。这里补回上一屏按钮。
			s.restoreSelectButtons()
			step = state.SessionAwaitSelect
		}
		if step == state.SessionAwaitTitle && !s.titleMode {
			// 脚本主动请求标题（旧式 BSScript 的 boot/title 标签、结局后的
			// 归位都可能走这里）：先把 VM 状态收好，再切到引擎标题。
			//
			// 快照只对旧式脚本有意义——它们的 boot 已经把新周目初始值写进
			// 属性/变量；现代剧本由 Session 在选 START 后从标签重新初始化。
			if s.script.IsLegacy() {
				snapshot := s.machine.CaptureState()
				s.titleBootState = &snapshot
			}
			if s.enterTitle() == state.SessionAwaitTitle {
				return state.SessionAwaitTitle
			}
			// 作品没有标题配置：退回"继续跑脚本"，不要卡在一张空标题上。
			step = state.SessionContinue
		}
		if command == vm.SelectEntry {
			// SELECT_ENTRY(0x36) 的参数不是跳转目标，而是"鉴赏条目"：
			//   arg >= 0：某场景的结尾，参数是它的回放标签号。原引擎在这里
			//             把场景登记进鉴赏；剧情要继续往下走（后面通常跟 RET
			//             返回调用者/地图循环）。旧实现一律当结束，剧情跑几段
			//             就报播放结束。
			//   arg == -1：scene_replay_end 的收尾，表示回放结束，回鉴赏室。
			// 两种都只消耗这条指令，不改变 PC 之外的状态。
			labelID := int(s.machine.Operand(1))
			if labelID < 0 {
				if s.replay {
					return s.finishReplay()
				}
				return state.SessionFinished
			}
			s.registerSceneEntry(labelID)
		}
		if s.machine.IsBsScript() && s.machine.Command().Kind == bsvm.BsCommandKindRestart {
			s.clearMapStage()
		}
		if step == state.SessionAwaitChoice {
			s.buildSelectWindow()
			if index := s.autoSelectChoiceIndex(); index >= 0 {
				core.Logf("choice: auto-select index %d (setting=%d)", index, s.settings.AutoSelect)
				step = s.applyChoiceInRun(index)
				continue
			}
			// 放課後2 的原生目标选择点完卡后，脚本还会发一个只有
			// 「ＯＫ／選び直す」的 SELECT。原版在这里不需要再点一次，
			// 直接把第一项（ＯＫ）确认掉，后面的 Target 标签按 #result 分派。
			if s.targetAutoConfirm {
				s.targetAutoConfirm = false
				// 只自动确认"刚点完目标卡"后面那个两项确认菜单。
				// 如果脚本中间插入了别的选择肢，宁可交还玩家，也不能
				// 拿一个无关的选项替玩家按下。
				if len(s.gameState.SelectItems()) == 2 {
					s.stopVoiceForAdvance()
					s.gameState.SetAwaitingInput(false)
					option := s.router.Choose(0)
					if option == nil {
						core.Logf("ui: target auto-confirm failed to take the OK choice")
					} else {
						s.gameState.ClearChoices()
						step = state.SessionContinue
						continue
					}
				} else {
					core.Logf("ui: target auto-confirm skipped (%d choices)", len(s.gameState.SelectItems()))
				}
			}
		}
		if step == state.SessionAwaitMap {
			step = s.beginMapSelect()
		}
		if step == state.SessionAwaitTarget {
			step = s.beginTargetSelect()
		}
		if step == state.SessionAwaitStaffroll {
			step = s.beginStaffroll()
		}
		if step == state.SessionAwaitEyecatch {
			step = s.beginEyecatch()
		}
		if len(s.gameState.Text()) > 0 && !s.sawMessage {
			s.sawMessage = true
			s.machine.RestartBoot = !s.script.IsLegacy()
		}
		if step == state.SessionFinished && s.replay {
			// 回放脚本以 HALT 结束时不能把 SessionFinished 交给宿主：
			// 宿主会直接退出播放页（看起来就是"回放后回到游戏库/主界面"）。
			// 正确做法是回到场景鉴赏/播放列表。
			return s.finishReplay()
		}
		if step == state.SessionFinished && s.tryStoryEntry() {
			continue
		}
		if step == state.SessionAwaitInput {
			// 只有真的停在"等玩家点击"时才算这句被看到：跳过/自动播放中途掠过的
			// 句子不该记成已读，否则对白回顾里会出现玩家从没看过的内容。
			// 先采样"之前是否读过"再标记：宿主用这个标志实现
			// 「未読文章もスキップする」关闭时遇未读停下。
			messageID := s.gameState.MessageId()
			s.lastMessageUnread = messageID >= 0 && len(s.gameState.Text()) > 0 && !s.messageRead(messageID)
			s.markMessageRead(messageID)
			s.recordHistory()
			// 停在对话上说明这一屏已经完整呈现给玩家了，此时才判定"看到了哪些 CG"。
			s.scanStageUnlocks()
			// 攒着写：全局存档是整份 40KB 重写，每句都写会把闪存写穿
			// （一局几万句）。每 globalCommitEvery 句落一次，配合存档/退出/切场景
			// 的强制提交，最多丢几十句的"已读"标记，不会丢 CG/场景解锁。
			s.globalPending++
			if s.globalPending >= globalCommitEvery {
				s.globalPending = 0
				s.commitGlobal()
			}
		}
		if step != state.SessionContinue {
			return step
		}
	}
	return state.SessionContinue
}

func (s *Session) finishRun(step state.SessionStep) core.Step {
	mapped := mapSessionStep(step)
	s.lastStep = mapped
	return mapped
}

func mapSessionStep(step state.SessionStep) core.Step {
	switch step {
	case state.SessionContinue:
		return core.StepContinue
	case state.SessionAwaitInput:
		return core.StepAwaitInput
	case state.SessionAwaitChoice:
		return core.StepAwaitChoice
	case state.SessionAwaitSelect:
		return core.StepAwaitSelect
	case state.SessionWait:
		return core.StepWait
	case state.SessionAwaitTitle:
		return core.StepAwaitTitle
	case state.SessionAwaitMovie:
		return core.StepAwaitMovie
	case state.SessionFinished:
		return core.StepFinished
	default:
		return core.StepFaulted
	}
}

func (s *Session) tryStoryEntry() bool {
	if s.entryUsed || s.storyIn {
		return false
	}
	if s.sawMessage && (!s.script.IsLegacy() || s.isRevScript()) {
		return false
	}
	s.entryUsed = true
	for _, name := range storyEntryCandidates {
		offset := s.script.FindLabel(name)
		if offset < 0 {
			continue
		}
		core.Logf("session: entry produced no story - jumping to '%s'", name)
		s.gameState.Reset()
		s.clearMapStage()
		s.machine.RestoreState(vm.EntryState(offset))
		s.machine.RestartBoot = !s.script.IsLegacy()
		s.sawMessage = false
		return true
	}
	return false
}

func (s *Session) isRevScript() bool {
	dialect, ok := s.script.BsDialect()
	return ok && bsvm.RevIsRev(dialect)
}

// revBootOffset 返回旧式 Rev 脚本的 boot 标签；非 Rev 或没有 boot 时返回 -1。
//
// 只有 Rev.2–Rev.6 把"初始化 → system call title"写在 boot 里；Ancient 的
// 入口约定不同，现代 BSXScript 由 Session 直接进标题，都不能套这条路径。
func (s *Session) revBootOffset() int {
	if s.script == nil || !s.isRevScript() {
		return -1
	}
	return s.script.FindLabel("boot")
}

// finishReplay 处理"回放脚本跑完"。
//
// 从鉴赏室进入的回放要回到原页面，而不是像旧版那样返回 SessionFinished
// 让宿主弹出结束页——玩家只是看了一段回想，应该继续挑下一段。
func (s *Session) finishReplay() state.SessionStep {
	s.replay = false
	core.Logf("replay: finished")
	if s.replayReturn == uiNone {
		return state.SessionFinished
	}
	kind := s.replayReturn
	page := s.replayPage
	s.replayReturn = uiNone
	s.titleMode = s.replayTitleMode
	if s.gameState != nil {
		s.gameState.SetText("")
		s.gameState.SetSpeaker("")
		s.gameState.SetMessageWindowVisible(false)
	}
	// 回放里可能还在播语音/换了 BGM；回到鉴赏室时必须收干净，
	// 否则语音会压着界面继续响、标题 BGM 也回不来。
	s.stopVoiceForAdvance()
	if s.titleMode && s.config != nil && len(s.config.Title.BgmName) > 0 {
		s.gameState.SetBgm(s.config.Title.BgmName)
	}
	// CHART 也是"状態"画面的一种页。beginSystemUI 只会重建一个空的
	// systemUI，不会重新解析 chart.db/页码，所以这里走 openStatusScreen，
	// 让它按 chartRestore 恢复玩家刚才看的页和滚动位置。
	if kind == uiStatus && s.openStatusScreen() {
		return state.SessionAwaitInput
	}
	s.beginSystemUI(kind)
	if s.ui != nil && kind == uiMuseumScene {
		s.ui.scenePage = page
	}
	if s.ui != nil && kind == uiScenePlaylist {
		s.ui.playlistPage = page
	}
	return state.SessionAwaitInput
}

func (s *Session) Advance() (core.Step, error) {
	if s.machine == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.staffrollActive() {
		return s.lastStep, nil
	}
	if s.eyecatchActive() {
		return s.lastStep, nil
	}
	if s.ui != nil {
		return s.lastStep, nil
	}
	if s.wordUI != nil {
		return s.lastStep, nil
	}
	if s.mddvd != nil {
		return s.lastStep, nil
	}
	if s.menuVisible {
		s.toggleGameMenu()
		return s.lastStep, nil
	}
	if s.slgState != nil {
		// 触屏上的"推进"不能穿透自绘界面；点击由 Pointer 的 doaction 处理。
		return s.lastStep, nil
	}
	if s.gameState != nil && s.gameState.AwaitingChoice() {
		// 选项没选之前脚本必须停住；Run/Advance 只是宿主心跳，不能替玩家做选择。
		return s.lastStep, nil
	}
	if s.titleMode {
		// 主界面上点空白处：原引擎什么都不做（不推进脚本、不跳过标题）。
		return core.StepAwaitTitle, nil
	}
	// 文字还没显示完（含"新消息还没画过第一帧"）时，第一次推进只把整句
	// 补完，不跳过这一句。判断必须用 MessageRevealPending：新消息在第一次
	// Compose 之前 revealText 还是上一句，MessageAnimating 会误报 false。
	if s.composer != nil && s.composer.MessageRevealPending(s.gameState.Text(), s.gameState.Speaker()) {
		s.composer.CompleteMessageReveal()
		return s.lastStep, nil
	}
	s.stopVoiceForAdvance()
	s.gameState.SetAwaitingInput(false)
	step := s.run(commandBudget)
	return s.finishRun(step), nil
}

func (s *Session) Choose(index uint32) (core.Step, error) {
	s.lastAutoSavePC = -1
	if s.machine == nil {
		return core.StepFaulted, errNotOpen
	}
	if s.ui != nil || s.menuVisible {
		return s.lastStep, nil
	}
	if !s.gameState.AwaitingChoice() {
		return core.StepContinue, nil
	}
	s.stopVoiceForAdvance()
	s.gameState.SetAwaitingInput(false)
	option := s.router.Choose(int(index))
	s.gameState.ClearChoices()
	if option != nil {
		// 两个方言都要先把机器里待选的 option 清掉：现代剧本选完后不会跳转，
		// 而是继续执行 SELECT 后面的指令，用 #select_type 自己分支。
		s.machine.ChooseOption(option.Index)
		if !s.machine.IsBsScript() {
			s.writeChoiceResult(*option)
		}
	}
	return s.finishRun(s.run(commandBudget)), nil
}

func (s *Session) Pointer(op uint32, x int32, y int32) (core.Step, error) {
	s.lastPointerX = int(x)
	s.lastPointerY = int(y)
	if s.machine == nil || s.disposed {
		return core.StepFaulted, errNotOpen
	}
	if s.staffrollActive() {
		if op == 1 || op == 3 || op == 5 {
			s.finishStaffroll()
		}
		return s.lastStep, nil
	}
	if s.wordUI != nil {
		return s.pointerWordUI(op, int(x), int(y))
	}
	if s.eyecatchActive() {
		if op == 1 || op == 3 || op == 5 {
			s.finishEyecatch()
		}
		return s.lastStep, nil
	}
	if s.ui != nil {
		return s.pointerSystemUI(op, x, y)
	}
	if s.mddvd != nil {
		return s.pointerMddvd(op, int(x), int(y))
	}
	if s.slgState != nil {
		return s.pointerSLGState(op, x, y)
	}
	if s.titleMode {
		return s.pointerTitle(op, x, y)
	}
	// 选项选中后，紧随其后的 release 只用于抬起手指，不能当成"点击推进"。
	if op == 3 && !s.choicePickedAt.IsZero() {
		elapsed := time.Since(s.choicePickedAt)
		s.choicePickedAt = time.Time{}
		if elapsed < 250*time.Millisecond {
			return s.lastStep, nil
		}
	}
	if s.md2Map {
		// 地图上仍然显示顶部/底部游戏菜单：先让菜单按钮吃点击，
		// 否则点 SAVE/LOG 会落进地图热区。
		if step, handled := s.pointerGameMenuDirect(op, x, y); handled {
			return step, nil
		}
		return s.pointerMD2Map(op, int(x), int(y))
	}
	if s.gameState.AwaitingChoice() {
		// 満淫電車2 的选择肢画面上游戏菜单仍然可用（原版可以存/读/设置）；
		// 其它作品的选项状态尚未写入 SaveData，保持原来的拦截行为。
		if s.config != nil && !s.config.Ecstasy.Empty() {
			if step, handled := s.pointerGameMenuDirect(op, x, y); handled {
				return step, nil
			}
		}
		index := hitSelectItem(s.gameState.SelectItems(), int(x), int(y))
		switch op {
		case 0:
			s.gameState.SetSelectHover(index)
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return s.lastStep, nil
		case 1, 3:
			if index < 0 {
				return s.lastStep, nil
			}
			s.choicePickedAt = time.Now()
			return s.Choose(uint32(index))
		}
	}
	// 引擎自绘对话框时，底部工具条常驻；先让它拿点击，避免点按钮时又推进一句。
	if step, handled := s.pointerMessageWindow(op, x, y); handled {
		return step, nil
	}
	if step, handled := s.pointerGameMenu(op, x, y); handled {
		return step, nil
	}
	if op == 0 && s.showGameMenuOnHover(int(x), int(y)) {
		return s.lastStep, nil
	}
	if s.gameState != nil && s.mapSource == "custom" {
		if op == 0 || op == 1 || op == 3 {
			s.updateCustomUIHover(int(x), int(y))
		}
		if (op == 1 || op == 3) && s.handleCustomUIClick(int(x), int(y)) {
			// 工作画面的工具/选项按钮同样吃 press+release：选中后立即
			// 打开系统界面，紧随其后的 release 不能再去点新界面。
			s.choicePickedAt = time.Now()
			return s.lastStep, nil
		}
	}
	switch op {
	case 5:
		if s.mapPreview >= 0 {
			// 右键先收起预览卡，不取消整张地图。
			s.mapPreview = -1
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return s.lastStep, nil
		}
		return s.finishRun(s.cancelMapSelect()), nil
	case 0:
		if s.gameState.Selecting() {
			s.mapPointerX, s.mapPointerY = int(x), int(y)
			index := s.mapPointerHit(int(x), int(y))
			s.gameState.SetHoveredButton(index)
			// mapmenu 的悬停列按行补画；装饰项的 Value 为 0，不参与高亮。
			s.mapHoverRow = -1
			if s.mapSource == bsmap.BsMapTbxdlSystemParameterFile && index >= 0 && index < len(s.mapItems) {
				if row := s.mapItems[index].Value - 1; row >= 0 {
					s.mapHoverRow = row
				}
			}
			if s.composer != nil {
				s.composer.Invalidate()
			}
		}
		return s.lastStep, nil
	case 1, 3:
		if s.gameState.Selecting() {
			s.mapPointerX, s.mapPointerY = int(x), int(y)
			index := s.mapPointerHit(int(x), int(y))
			if s.mapSource == bsmap.BsMapGakuen2Source {
				// 原版地图是两段式：第一次点地点只打开 map_backN 的
				// 「Move Place」预览卡，再点一次才真正移动。
				if s.mapPreview >= 0 {
					s.choicePickedAt = time.Now()
					return s.finishRun(s.chooseButton(s.mapPreview)), nil
				}
				if index < 0 {
					return s.lastStep, nil
				}
				s.mapPreview = index
				// 同一次点按的 release 不能当成“第二下确认”；用与选择肢
				// 相同的 250ms 守卫吃掉它。
				s.choicePickedAt = time.Now()
				if s.composer != nil {
					s.composer.Invalidate()
				}
				return s.lastStep, nil
			}
			if index < 0 {
				return s.lastStep, nil
			}
			// 宿主对一次点按会先发 press(1) 再发 release(3)。地图卡选中后
			// 剧情已经停在第一句（AwaitingInput），紧跟着的 release 会被
			// 当成“再点一下”而跳过第一句。用与选择肢相同的 250ms 守卫
			// 吃掉这次 release。
			s.choicePickedAt = time.Now()
			return s.finishRun(s.chooseButton(index)), nil
		}
		if s.gameState.AwaitingInput() {
			// 与 Advance 一致：先补完逐字显示（包括还没画第一帧的新消息），
			// 再让下一次点击推进脚本。
			if s.composer != nil && s.composer.MessageRevealPending(s.gameState.Text(), s.gameState.Speaker()) {
				s.composer.CompleteMessageReveal()
				return s.lastStep, nil
			}
			s.stopVoiceForAdvance()
			s.gameState.SetAwaitingInput(false)
			step := s.run(commandBudget)
			if s.replay && s.machine.Halted() {
				// 回放以 HALT 收尾时必须走 finishReplay 恢复鉴赏界面；
				// 只把 replay 清掉会把 SessionFinished 交给宿主，看起来就是
				// "看完一段被踢回游戏库"。
				return s.finishRun(s.finishReplay()), nil
			}
			return s.finishRun(step), nil
		}
		return s.lastStep, nil
	case 4:
		if s.gameState.Selecting() {
			return s.finishRun(s.cancelMapSelect()), nil
		}
		// 剧情中右键 = 原引擎的游戏菜单；触屏上由宿主工具栏的菜单按钮
		// 发 ESC，走同一条路径。
		s.toggleGameMenu()
		return s.lastStep, nil
	default:
		return s.lastStep, nil
	}
}

// hitSelectItem 返回坐标命中的选项下标；-1 表示没点到。
func hitSelectItem(items []state.SelectItem, x, y int) int {
	for _, item := range items {
		if x >= item.X && y >= item.Y && x < item.X+max(1, item.Width) && y < item.Y+max(1, item.Height) {
			return item.Index
		}
	}
	return -1
}

// prepareMapSelectButtons 维护“上一次地图按钮”快照的生命周期。
//
//   - 背景重画（IMAGE/IMAGE_EX type=1/15/0x81）说明已经离开上一屏地图，
//     旧卡不能带到新地图上；
//   - 新地图自己画出了按钮时，快照也失去意义；
//   - HALT/RESTART 会重置脚本，同样作废。
func (s *Session) prepareMapSelectButtons(command vm.ScriptCommandId) {
	switch command {
	case vm.Image, vm.ImageExtended:
		sub := int(s.machine.Frame(2))
		background := sub == 1 || sub == 0x81 || (command == vm.Image && sub == 15)
		if background {
			s.selectButtons = nil
		}
	case vm.Halt, vm.Restart:
		s.selectButtons = nil
	case vm.SelectTail:
		if len(s.gameState.Buttons()) > 0 {
			s.selectButtons = nil
		}
	}
}

func (s *Session) hitButton(x int, y int) int {
	buttons := s.gameState.Buttons()
	for index := range buttons {
		button := buttons[index]
		// 地图卡的 _hit 掩码是“相对按钮绘制原点”的；KZK/屈辱2 的卡图
		// 整屏铺在 (0,0)，只有掩码里的那一小块卡面可点。掩码存在时
		// 绝不回退到整图矩形，否则多张卡会重叠成同一个热区。
		if button.HitMask != nil {
			if button.HitMask.Hit(x-button.X, y-button.Y) {
				return index
			}
			continue
		}
		left, top, width, height := s.buttonRect(button)
		if width <= 0 || height <= 0 {
			continue
		}
		if x >= left && y >= top && x < left+width && y < top+height {
			return index
		}
	}
	return -1
}

func (s *Session) Compose(dst unsafe.Pointer, n int) (bool, error) {
	if dst == nil || n <= 0 || s.composer == nil || s.gameState == nil {
		return false, nil
	}
	if s.width <= 0 || s.height <= 0 || n < s.width*s.height*4 {
		return false, nil
	}
	if s.machine != nil && s.machine.IsBsScript() {
		s.gameState.SetCounterValue(-1)
	} else {
		s.gameState.SetCounterValue(s.readCounter())
	}
	// START 的可用性取决于四个 #chN_select_work：每帧按按钮定义重算一次。
	// 只有工作画面在选择态时才需要，其它地图不受影响。
	if s.mapSource == "custom" {
		s.refreshCustomMapButtons()
	}
	s.composer.SetInformationWindow(s.informationWindowValues())
	subDay, subTime := s.messageSubWindowValues()
	s.composer.SetMessageSubWindow(subDay, subTime)
	notice := ""
	if s.toolNotice != "" {
		if time.Since(s.toolNoticeAt) < 1200*time.Millisecond {
			notice = s.toolNotice
		} else {
			s.toolNotice = ""
		}
	}
	s.composer.SetMessageWindowState(s.messageWindowVisible(), s.messageHover, s.toolbarVisible, notice)
	// 系统界面（回想/存读档/设置/鉴赏）打开时不再画游戏内工具条。
	s.composer.SetInterfaceOpen(s.ui != nil || s.slgState != nil || s.mddvd != nil || s.wordUI != nil)
	// 満淫電車2IE 的 hair mode：只有确实带 hair 图层素材的作品才打开，
	// 其它作品的 HairMode 默认值不会误触发。
	s.composer.SetHairOverlay(s.settings.HairMode && s.res != nil && s.res.Exists("hair_bu01f0001a0"))
	// 画面效果 OFF / 効果なし：跳过转场 wipe。MD2 的设置页有「画面効果」。
	s.composer.SetEffectsEnabled(s.settings.AllEffect && s.settings.EffectMode != 0)
	if s.md2Map {
		// 悬停箭头的帧随墙钟变化，缓存必须每帧失效。
		s.composer.Invalidate()
	}
	if s.wordUI != nil {
		// 手机界面的悬停/选择状态是逐帧变化的覆盖层。
		s.composer.Invalidate()
	}
	redrawn := s.composer.Compose(s.gameState)
	// 学園（GK）的日期/时刻卡是剧情 HUD，不属于对话框缓存；地图/标题/
	// 系统界面各自处理自己的日期显示，这里只在剧情里补画。
	if s.drawDateCard(s.composer.Frame()) {
		redrawn = true
	}
	// 系统界面是覆盖层：即使底图没有变化也要重画/重传，否则悬停、翻页、
	// 开关菜单都会被宿主上一帧的旧画面盖住。
	if s.drawSystemOverlays(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawMddvdPlay(s.composer.Frame()) {
		redrawn = true
	}
	// Tbxdl 的楼层列表悬停行是"后补"的：舞台层只贴常态列，合成后再把
	// 悬停列对应行盖上去。它不影响其它地图（source 必须是 tbxdl 的 system.prm）。
	if s.drawMapHover(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawMapCardHover(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawMD2MapOverlay(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawWordUI(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawWordMeter(s.composer.Frame()) {
		redrawn = true
	}
	// [extra window]（GK2IEDL 左下角等级面板）与 [free-sight screen]（自由视角
	// 滑杆）都是配置驱动的 HUD：没有对应配置的作品这两个函数直接返回 false。
	if s.drawExtraWindow(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawFreeSight(s.composer.Frame()) {
		redrawn = true
	}
	if s.drawStaffroll(s.composer.Frame()) {
		redrawn = true
		s.tickStaffroll()
	}
	if s.drawEyecatch(s.composer.Frame()) {
		redrawn = true
		s.tickEyecatch()
	}
	s.gameState.SetFillAlpha(0)
	if s.pendingAutoSave && s.mapShown && s.ui == nil {
		s.pendingAutoSave = false
		s.autoSave()
	}
	if !redrawn {
		return false, nil
	}
	pixels := s.composer.Frame().Pixels()
	buf := unsafe.Slice((*byte)(dst), n)[:s.width*s.height*4]
	copy(buf, pixels)
	return true, nil
}

func (s *Session) readCounter() int {
	if s.counterSlot == unresolvedSlot {
		s.counterSlot = s.script.GetPropertyIndex(s.config.CounterVariable)
	}
	if s.counterSlot >= 0 {
		return int(s.machine.Property(s.counterSlot))
	}
	return -1
}

func (s *Session) Scenes() []core.Scene {
	out := make([]core.Scene, len(s.catalog))
	copy(out, s.catalog)
	// 带上解锁状态：鉴赏界面的"场景回放"列表要按全局存档把没看过的置灰。
	// 放在这里一次性算完，宿主不用为每个场景再问一次引擎。
	for index := range out {
		// out[0] 是"开始剧情"的伪场景，不对应 scene.dat 的任何一行；
		// 后面的场景要减一才是全局存档里的下标。
		out[index].Unlocked = s.sceneUnlockedAt(index - 1)
	}
	return out
}

// sceneUnlockedAt 按 scene.dat 行序查询解锁状态。
func (s *Session) sceneUnlockedAt(index int) bool {
	if s.unlockAll {
		return true
	}
	global := s.ensureGlobal()
	if global == nil {
		return false
	}
	return global.SceneUnlockedAt(index)
}

func (s *Session) Slots() []core.Slot {
	if s.saves == nil {
		return nil
	}
	infos := s.saves.List()
	out := make([]core.Slot, 0, len(infos))
	for _, info := range infos {
		slot := core.Slot{Summary: info.Summary, Exists: info.Exists}
		if info.Index >= 0 {
			slot.Index = uint32(info.Index)
		}
		if info.HasSavedAt {
			iso := info.SavedAt.Format(time.RFC3339)
			slot.Title = iso
			slot.SavedAt = iso
		}
		out = append(out, slot)
	}
	return out
}

func (s *Session) Save(slot uint32, title string) error {
	if s.machine == nil || s.composer == nil {
		return errNotOpen
	}
	if s.slgState != nil {
		// 界面中间态不在 SaveData 里；存下来再读会停在一个没有热区的画面上。
		core.Logf("save: refused while lua state '%s' is active", s.slgState.name)
		return errSaveFault
	}
	// 言葉責め回合的进度（已选词/累计兴奋/下一句下标）不在扩展存档里；
	// 中途存档再读会停在一句反应台词上，而 wordPlay 已丢失，14951 无法
	// 接着结算。宁可拒绝，也不要生成一个读不回来的档。
	if s.wordPlay != nil {
		core.Logf("save: refused during an active word turn")
		return errSaveFault
	}
	// 手册「シーン鑑賞中はセーブ・ロード・クイックセーブ・クイックロード
	// を行う事はできません」：回放中存档会把鉴赏画面/回放 PC 写进槽位。
	if s.replay {
		core.Logf("save: refused during scene replay")
		return errSaveFault
	}
	width := roundHalfEven(float64(s.width) * s.config.ThumbnailRatio)
	if width < 1 {
		width = 1
	}
	height := roundHalfEven(float64(s.height) * s.config.ThumbnailRatio)
	if height < 1 {
		height = 1
	}
	// 系统界面打开时 composer 帧已经被存档画面盖住：缩略图必须用打开界面前
	// 抓下的底层画面，否则每个槽位都会存成同一张存档界面。
	source := s.composer.Frame()
	if s.ui != nil && s.ui.backdrop != nil {
		source = s.ui.backdrop
	}
	thumbnail := source.ScaleTo(width, height)
	var pixels []byte
	if thumbnail != nil {
		pixels = append([]byte(nil), thumbnail.Pixels()...)
	}
	// Lua 的 PrivateData/CommonData 是训练配方/领地摆件的来源；存档前
	// 必须先同步，否则刚排好的训练计划会留在 VM 里丢掉。
	s.syncSLGData()
	machineState := s.machine.CaptureState()
	// 把 PC 倒回"玩家看到的那一屏"的指令头：
	//   - 地图选择态：倒回 SELECT_TAIL。读档后重新铺同一张地图，而不是
	//     带着被清空的 #result 从选择之后继续（这一格会被静默跳过）；
	//   - 对话态：倒回 DisplayMessage。否则继承的 PC 已经在这句之后，
	//     读档会直接跳到下一句，玩家存下来的那一句被吃掉。
	// 打开存读档界面时 beginSystemUI 会把文本清空并存进 ui.savedText，
	// 所以界面里存档要看 savedText，宿主直接调用 Save 时看 gameState。
	if start := s.machine.InstructionStart(); start >= 0 {
		if s.gameState.Selecting() && (s.mapSource == bsmap.BsMapDataFileName || s.mapSource == bsmap.BsMapGkSource || s.mapSource == "md2") {
			machineState.ProgramCounter = start
		} else if s.lastStep == core.StepAwaitInput && s.saveRewindToMessage() {
			machineState.ProgramCounter = start
		}
	}
	// 脚本自绘地图（牝教師4 等）的按钮只存在于当前帧，不在 SaveData 里。
	// 这里把按钮单独存进扩展段；掩码类地图（KZK/屈辱2）的整屏掩码无法
	// 经济地序列化，直接拒绝存档，避免读回来所有卡重叠成同一个矩形。
	customMap := s.gameState.Selecting() && s.mapSource == "custom"
	if customMap {
		for _, button := range s.gameState.Buttons() {
			if button.HitMask != nil {
				core.Logf("save: refuse custom map with pixel hit mask")
				return errSaveFault
			}
		}
	}
	captured := save.Capture(s.gameState, machineState, s.title, s.summarize(), pixels, width, height)
	if s.wordsLoaded {
		captured.HasWordMemory = true
		captured.WordTemp = append([]int(nil), s.wordTemp...)
		captured.WordSaved = s.wordSaved
		captured.WordActive = s.wordActive
	}
	if s.gameState.AwaitingChoice() && s.machine != nil {
		for _, option := range s.machine.SelectOptions() {
			captured.ChoiceOptions = append(captured.ChoiceOptions, save.SaveChoiceOption{
				Index: option.Index, MessageId: option.MessageId,
				LabelId: option.LabelId, Value: option.Value, Kind: option.Kind,
			})
		}
	}
	if s.gameState.Selecting() && s.mapSource == bsmap.BsMapGkSource {
		// GK 的地图由 MapPrms + profile 重建；读档时重新铺同一张图，
		// 不需要把 12 个地点按钮逐个序列化。
		captured.MapSource = bsmap.BsMapGkSource
	}
	if s.gameState.Selecting() && s.mapSource == bsmap.BsMapDataFileName {
		// map.dat 地图（放課後2）同样由 profile 重建。不记来源的话，读档会
		// 把地图层当成剧情层恢复，玩家既看不到地图按钮也无法继续选择。
		captured.MapSource = bsmap.BsMapDataFileName
	}
	if s.gameState.Selecting() && s.mapSource == "md2" {
		// 精灵图地图同样由 beginMD2MapSelect 重建，读档后重新注册站点热区。
		captured.MapSource = "md2"
	}
	if customMap {
		captured.MapSource = "custom"
		for _, button := range s.gameState.Buttons() {
			captured.MapButtons = append(captured.MapButtons, save.SaveMapButton{
				Image: button.Image, X: button.X, Y: button.Y, Value: button.Value,
				Frames: button.Frames, Width: button.Width, Height: button.Height,
				Layer: button.Layer, Disabled: button.Disabled,
				HoverFrame: button.HoverFrame, HasHoverFrame: button.HasHoverFrame,
			})
		}
	}
	// SLG 的数值表/训练计划/成就不是剧本变量，必须单独随槽位写出；
	// 否则读档后训练次数和当日配方会回到 CSV 初始值。
	if s.scriptDB != nil {
		captured.ScriptDB = s.scriptDB.Snapshot()
	}
	captured.PrivateData = s.privateData
	captured.CommonData = s.commonData
	captured.UnlockedAchievements = s.unlockedAchievements
	if !s.saves.Write(captured, int(slot)) {
		return errSaveFault
	}
	if s.ui != nil {
		// 写完让存档画面下一帧重读当前页，玩家能立刻看到新缩略图。
		s.ui.slotPage = -1
	}
	// 存档是玩家认可的"进度点"，顺带把鉴赏解锁写进全局存档：
	// 玩家就算之后强退，这些 CG 也不会丢。
	s.commitGlobal()
	core.Logf("save: slot %d written", slot)
	return nil
}

func (s *Session) Load(slot uint32) (core.Step, error) {
	if s.machine == nil {
		return core.StepFaulted, errNotOpen
	}
	// 鉴赏回放期间读档会直接跳出回放、把鉴赏界面留在栈上；
	// 手册明确禁止，和 Save 一样在入口拒绝。
	if s.replay {
		return core.StepFaulted, errSaveFault
	}
	stored := s.saves.Read(int(slot))
	if stored == nil {
		return core.StepFaulted, errNoSlot
	}
	// 读档要先把攒着的鉴赏解锁落盘：读档会重置剧情状态，
	// 如果不先写，刚解锁的那几张 CG 就跟着丢了。
	s.commitGlobal()
	// 从主界面读档时要退出标题模式，否则 run() 会一直停在标题。
	s.leaveTitle()
	s.gameState.Reset()
	s.clearMapStage()
	stored.ApplyTo(s.gameState)
	s.restoreWordMemory(stored)
	s.wordActive = stored.WordActive && s.config != nil && !s.config.Ecstasy.Empty()
	s.machine.RestoreState(stored.Machine)
	if db := s.ensureScriptDB(); db != nil && stored.ScriptDB != nil {
		db.Restore(stored.ScriptDB)
	}
	// 旧档没有这些段时保留原值：不能把跨周目数据读成空。
	if stored.PrivateData != nil {
		s.privateData = stored.PrivateData
	}
	if stored.CommonData != nil {
		s.commonData = stored.CommonData
	}
	if stored.UnlockedAchievements != nil {
		s.unlockedAchievements = stored.UnlockedAchievements
	}
	if s.slgVM != nil {
		s.slgVM.RestoreFromGo(s.privateData, s.commonData)
	}
	s.slgState = nil
	s.animWaitUntil = time.Time{}
	s.rebuildBsLayers()
	s.resetAudioCache()
	// 脚本自绘地图的按钮已经随存档保存；恢复后直接回到选择态，
	// 不再让脚本从 SELECT_TAIL 后面继续（那会把这一格当成已选过）。
	if stored.MapSource == "custom" && len(stored.MapButtons) > 0 {
		s.restoreCustomMapScreen(stored.MapButtons)
		core.Logf("save: slot %d loaded onto custom map (%d button(s))", slot, len(stored.MapButtons))
		return s.finishRun(state.SessionAwaitSelect), nil
	}
	if len(stored.ChoiceOptions) > 0 && s.machine != nil {
		options := make([]vm.SelectOption, 0, len(stored.ChoiceOptions))
		for _, option := range stored.ChoiceOptions {
			options = append(options, vm.SelectOption{
				Index: option.Index, MessageId: option.MessageId,
				LabelId: option.LabelId, Value: option.Value, Kind: option.Kind,
			})
		}
		s.machine.SetSelectOptions(options)
		// 重新走一次 SELECT 的展示逻辑：PC 已经停在 SELECT 之后，不能再
		// 执行 FETCH，否则会把选项重复追加到寄存器里。
		step := s.router.Dispatch(vm.Select)
		if step == state.SessionAwaitChoice {
			// PresentChoices 只写文本；选项条布局必须像正常 run 循环那样
			// 再建一次，否则读档后画面有选择题文本却没有可点的条。
			s.buildSelectWindow()
			if index := s.autoSelectChoiceIndex(); index >= 0 {
				s.applyChoiceInRun(index)
				return s.finishRun(s.run(commandBudget)), nil
			}
		}
		core.Logf("save: slot %d restored %d choice option(s)", slot, len(options))
		return s.finishRun(step), nil
	}
	if stored.MapSource == "md2" {
		s.gameState.SetText("")
		s.gameState.SetSpeaker("")
		s.gameState.SetMessageWindowVisible(false)
		// beginMD2MapSelect 会写自动档；读档时把当前 PC 记为已保存，
		// 避免刚读出来就用同一张地图覆盖掉存档。
		s.lastAutoSavePC = s.machine.InstructionStart()
		if s.beginMD2MapSelect() {
			return s.finishRun(state.SessionAwaitSelect), nil
		}
	}
	if stored.MapSource == bsmap.BsMapGkSource || stored.MapSource == bsmap.BsMapDataFileName {
		// 存档是在地图上做的：PC 已经倒回 SELECT_TAIL，重新解析地图并铺图，
		// 玩家继续选地点。不要 run，否则脚本会把 SELECT_TAIL 当成"已经选过"，
		// 直接跳进某个地点。
		// 注意：地图前的剧情层（mapBackup）没有进存档，读档后离开地图时
		// 舞台为空；下一次地点事件会重新设置背景，不影响流程。
		s.gameState.SetText("")
		s.gameState.SetSpeaker("")
		s.gameState.SetMessageWindowVisible(false)
		s.gameState.SetBsLayers(nil)
		s.clearMapStage()
		if s.beginMapSelect() == state.SessionAwaitSelect {
			core.Logf("save: slot %d loaded onto map (%s, %d item(s))", slot, stored.MapSource, len(s.mapItems))
			return s.finishRun(state.SessionAwaitSelect), nil
		}
	}
	core.Logf("save: slot %d loaded", slot)
	return s.finishRun(s.run(commandBudget)), nil
}

func (s *Session) rebuildBsLayers() {
	if !s.machine.IsBsScript() {
		return
	}
	entries := make([]state.BsLayerEntry, 0, state.PlaneCount)
	for plane := 0; plane < bs.LayerLimit; plane++ {
		layer := s.gameState.LayerAt(plane)
		if layer == nil || len(layer.Artworks()) == 0 {
			break
		}
		artworks := append([]state.LayerArtwork(nil), layer.Artworks()...)
		entries = append(entries, state.NewBsLayerEntry(layer.Name(), artworks[0].X, artworks[0].Y, layer.Kind(), artworks))
	}
	if len(entries) == 0 {
		return
	}
	s.gameState.SetBsLayers(entries)
	core.Logf("save: bsscript layer table rebuilt from %d plane(s)", len(entries))
}

// saveRewindToMessage 判断这次存档是否应该倒回当前对白的指令头。
//
// 只看"确实停在对话上"：剧情文本非空（界面里存档时文本在 ui.savedText）。
// 地图/选择肢/影片有各自的暂停语义，不能套这条。
func (s *Session) saveRewindToMessage() bool {
	if s.gameState == nil {
		return false
	}
	if s.ui != nil {
		return len(s.ui.savedText) > 0 || s.ui.savedMulti
	}
	return len(s.gameState.Text()) > 0 || len(s.gameState.MultiText()) > 0
}

func (s *Session) summarize() string {
	text := strings.TrimSpace(strings.ReplaceAll(s.gameState.Text(), "\n", " "))
	if speaker := s.gameState.Speaker(); len(speaker) > 0 {
		text = speaker + ": " + text
	}
	runes := []rune(text)
	if len(runes) > 42 {
		text = string(runes[:42])
	}
	return text
}

func (s *Session) AssetSize(path string) int {
	if s.res == nil {
		return 0
	}
	location, ok := s.res.TryGet(path)
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
	if dst == nil || n <= 0 || s.res == nil {
		return 0
	}
	location, ok := s.res.TryGet(path)
	if !ok || location == nil {
		return 0
	}
	raw := location.Read()
	if len(raw) == 0 {
		return 0
	}
	return copy(unsafe.Slice((*byte)(dst), n), raw)
}

func (s *Session) Audio() []core.AudioEvent {
	out := s.audio
	s.audio = nil
	return out
}

func (s *Session) Text() core.TextState {
	if s.gameState == nil {
		return core.TextState{}
	}
	// 系统界面打开时不能把上一句对白交给宿主：Flutter 的对话框是独立图层，
	// 会浮在存档/鉴赏画面上并挡住点击。
	if s.ui != nil {
		return core.TextState{}
	}
	var awaiting uint32
	if s.gameState.AwaitingInput() || s.gameState.AwaitingChoice() {
		awaiting = 1
	}
	return core.TextState{Speaker: s.gameState.Speaker(), Text: s.gameState.Text(), Awaiting: awaiting}
}

func (s *Session) Choices() []core.Choice {
	if s.gameState == nil {
		return nil
	}
	// 选项由引擎按 [select window] 自绘（原版样式）。返回空列表让宿主的
	// Flutter 覆盖层收起，点击统一从 Pointer 进来，避免两套界面叠在一起。
	if len(s.gameState.SelectItems()) > 0 {
		return nil
	}
	names := s.gameState.Choices()
	if len(names) == 0 {
		return nil
	}
	out := make([]core.Choice, 0, len(names))
	for index, name := range names {
		out = append(out, core.Choice{Name: name, Value: int32(index)})
	}
	return out
}

// CurrentStep 返回引擎当前停留的步骤，不推进脚本、不产生副作用。
//
// 宿主的 OpFrame(37) 会把 step 和 text/state 一起取走；没有这个接口时
// 引擎只能写 StepContinue，宿主就得靠上一次调用的返回值推测当前状态。
// 点击标题后、或者系统界面盖在剧情上时，这种推测很容易把 step 当成
// "还能继续跑"而在下一帧把剧情推过去，所以这里直接暴露真实值。
func (s *Session) CurrentStep() core.Step {
	return s.lastStep
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
	hostRequest, hostSerial := s.hostRequestState()
	return core.State{
		Bgm:   bgm,
		PC:    uint32(pc),
		Scene: uint32(start),
		// 工具按钮的 ON/OFF 提示只显示 1.2 秒；宿主只在 isAnimating 时持续合成，
		// 这里把提示期也算进去，提示到期后引擎会重绘一帧把文字清掉。
		Animating:            s.slgState != nil || s.gameState.TransitionActive() || s.gameState.HasAnimatedLayer() || s.toolNotice != "" || (s.composer != nil && s.composer.MessageRevealPending(s.gameState.Text(), s.gameState.Speaker())),
		WaitMs:               uint32(wait),
		MessageWindowVisible: s.gameState.MessageWindowVisible() && s.ui == nil,
		EngineMessageWindow:  s.messageWindowVisible(),
		AwaitingName:         false,
		BackLogActive:        false,
		ReplayActive:         s.replay,
		InstructionStart:     uint32(start),
		NamePrompt:           "",
		HostRequest:          hostRequest,
		HostRequestSerial:    hostSerial,
		MessageUnread:        s.lastMessageUnread,
		CeaseSkip:            s.settings.CeaseSkip,
		SkipUnread:           s.settings.SkipUnread,
		// +1 作为哨兵：0 表示"这套引擎没有オート速度设置"，宿主会回退到
		// 旧的文本长度公式；Bishop 的 ClickWait=0 仍然能表达为 1。
		AutoWait: s.settings.ClickWait + 1,
	}
}

func (s *Session) TextList() []core.TextUnit {
	if len(s.history) == 0 {
		return nil
	}
	out := make([]core.TextUnit, len(s.history))
	copy(out, s.history)
	return out
}

// recordHistory 记录当前停在屏幕上的这句对白。
//
// 同一句可能因为选项/等待被重复渲染，连续重复只记一次；上限 200 句足够
// 回想界面滚动，也避免长时间游玩把内存堆满。
func (s *Session) recordHistory() {
	if s.gameState == nil {
		return
	}
	text := s.gameState.Text()
	if len(text) == 0 {
		// 第二个消息框（multi messages）只有 multiText 时也要进回想。
		text = s.gameState.MultiText()
	}
	if len(text) == 0 {
		return
	}
	speaker := s.gameState.Speaker()
	if speaker == "" {
		speaker = s.gameState.MultiSpeaker()
	}
	voice := ""
	if name, ok := s.gameState.Voice(); ok {
		voice = name
	}
	if n := len(s.history); n > 0 && s.history[n-1].Text == text && s.history[n-1].ID == speaker {
		return
	}
	s.history = append(s.history, core.TextUnit{
		ID:   speaker,
		Text: text,
		File: voice,
	})
	const limit = 200
	if len(s.history) > limit {
		s.history = append([]core.TextUnit(nil), s.history[len(s.history)-limit:]...)
	}
}

func (s *Session) LastError() string {
	return strings.Join(s.trace, "\n")
}

func (s *Session) record(message string) {
	if len(s.trace) < traceLimit {
		s.trace = append(s.trace, message)
	}
}

func (s *Session) Close() error {
	if s.disposed {
		return nil
	}
	s.disposed = true
	// 退出前把鉴赏解锁落盘：这是玩家最不愿意丢的数据，不能等下一次存档。
	s.commitGlobal()
	// 顺带保证总存档存在（新安装、或玩家删过 save 目录时补一份）。
	s.ensureGlobalSave()
	if s.mapCatalog != nil && bsmap.BsMapCatalogInstalled() == s.mapCatalog {
		bsmap.BsMapCatalogInstall(nil)
	}
	if s.slgVM != nil {
		s.slgVM.Close()
		s.slgVM = nil
	}
	if s.res != nil {
		s.res.Dispose()
	}
	return nil
}
