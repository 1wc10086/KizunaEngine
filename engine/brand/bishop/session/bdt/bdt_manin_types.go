package bdt

import "kizuna/engine/core"

// bdt_manin_types.go 集中定义「満淫電車」（manin）路线的状态与常量。
//
// BDT 包同时承载 AFDVD（容器归档）与 manin（散装 cg/*.zbm）两代作品。两者
// 脚本方言相近但界面、存档和内置系统完全不同，所以 manin 的全部可变状态都
// 收进 maninRuntime，Session 只持有一个指针：AFDVD 打开时该指针为 nil，任何
// manin 分支都不会被走到，不会影响已适配的作品。

const (
	maninScreenWidth  = 640
	maninScreenHeight = 480

	// maninWordBankCap 是「データ欄」上限；memory 是临时记忆槽，两者都按
	// 原作满 21 条处理。后者满了以后最旧的被覆盖，前者满了要玩家选一条覆盖。
	maninWordBankCap = 21
	maninMemoryCap   = 21

	// 興奮度 / ストレス度上限与等级步长来自原作手册：48 点满，每 12 点升一级。
	maninGaugeMax       = 48
	maninGaugeLevelStep = 12

	// maninDefaultTimeLimit 是 XT 没给出时的兜底；XT420 在原作里表示
	// 每轮（3 句）允许操作的时间预算，这里换算成「可执行轮数」= XT/100。
	maninDefaultTimeLimit = 400
)

// maninScreen 是盖在剧情帧上的内置界面类型。
type maninScreen int

const (
	maninScreenNone maninScreen = iota
	maninScreenSystem
	maninScreenSave
	maninScreenLoad
	maninScreenMuseum
	maninScreenMusic
	maninScreenCG
	maninScreenScene
	maninScreenHch
	maninScreenVulgar
	maninScreenMap
	maninScreenAttack
	maninScreenOrganize
	maninScreenLog
	maninScreenStaffRoll
	maninScreenName
)

// maninAttackPhase 是言葉責め界面的内部阶段。
const (
	maninAttackSelect = iota
	maninAttackConfirm
	maninAttackExecute
	maninAttackDone
)

// maninWord 是 atktbl.bdt 的一行：ID 对应脚本里的 <nn>，六个等级对应六名
// 女主角（X29=1..6）。等级直接决定反应，不另设随机表，便于和原作数值一致。
type maninWord struct {
	ID     int
	Text   string
	Grades [6]byte
	// Rank 是整理的显示等级（II..V），由六人平均反应推导：原作没有单独的
	// 等级表，rank_a..e 只是图标，平均等级是最能解释「V 平均好反应」的算法。
	Rank int
}

type maninWordCatalog struct {
	Words []maninWord
	byID  map[int]int
}

// maninCharacter 是 ctbl.rbd 的一个角色/出现点表。Threshold 为进行值门槛，
// Stations 对应 6 个时间段（朝1/朝2/昼1/昼2/夜1/夜2）的车站名。
type maninCharacter struct {
	Index int
	Name  string
	Rows  []maninCharRow
}

type maninCharRow struct {
	Threshold int
	Stations  [6]string
}

// maninSettings 对应 SYSTEM 画面上的开关。字段名保持可读，序列化到
// manin_global.json；画面显示用 s_sys01 底图 + 状态高亮，不重绘文字。
type maninSettings struct {
	Fullscreen     bool
	EffectOn       bool
	BgmOn          bool
	SeOn           bool
	VoiceOn        bool
	VoiceByGirl    [6]bool
	MessageFast    bool
	SkipFast       int // 0=通常 1=高速 2=次の選択肢まで
	SkipUnread     bool
	AutoSpeed      int // 0..10
	VolumeBgm      float64
	VolumeSe       float64
	VolumeVoice    float64
	VulgarUnlocked bool
}

// maninGlobal 是跨存档的全局数据（CG/场景解锁、设置、主角名）。
// 解锁用 map 而不是切片：鉴赏室每帧都要按名字查，切片线性扫会把 197 个
// CG × 24 个格子的绘制拖成 O(n²)。
type maninGlobal struct {
	Name          string
	Settings      maninSettings
	UnlockedCG    map[string]bool
	UnlockedScene map[string]bool
	// VulgarGirls 是标题「卑語モード」的 6 名角色解锁位 + その他（索引 6）。
	// 原作把 7 个标志放在存档对象里，任意一位有值就显示 hgon 横幅；
	// 这里按同样的粒度保存，6 人全开时点亮第 7 位。
	VulgarGirls [7]bool
}

// maninSlotData 是随槽位保存的 manin 运行时数据。BDT 的二进制槽格式不变，
// 这里作为旁车 JSON 写在同一槽号旁边，避免破坏 AFDVD 的存档兼容性。
type maninSlotData struct {
	Words    []int
	Memory   []int
	Caption  string
	Station  int
	Day      int
	TimeSlot int
}

// maninLogLine 是 LOG 画面的最小回放单元。
type maninLogLine struct {
	Speaker string
	Text    string
}

type maninAttackState struct {
	Active     bool
	Target     int
	TimeLimit  int
	TurnsLeft  int
	Arousal    int
	Stress     int
	Burning    int
	Level      int
	Phase      int
	Selected   []int
	Queue      []int
	QueueIndex int
	Cursor     int
	Page       int
	Message    string
	Result     int
}

type maninOrganizerState struct {
	Active    bool
	Mode      int // 0=メニュー 1=登録
	Page      int
	Cursor    int
	Overwrite int // -1 表示未进入覆盖模式
	Pending   int // 覆盖模式下待写入的词 ID
}

type maninSaveState struct {
	Mode     bool // true=SAVE, false=LOAD
	Page     int
	Row      int
	Slots    []core.Slot
	ReturnTo maninScreen
}

type maninGalleryState struct {
	Kind    maninScreen
	Page    int
	Thumb   int
	Preview string
}

type maninMapState struct {
	Hover    int
	ReturnTo maninScreen
}

type maninLogState struct {
	Scroll int
}

type maninStaffRollState struct {
	Index int
	Frame int
}

// maninRuntime 是 manin 路线的全部状态。Screen 决定 Compose/Pointer 走哪个
// 子界面；words/memory 是責め言葉库存，随槽位保存。
type maninRuntime struct {
	catalog *maninWordCatalog
	global  maninGlobal
	slot    maninSlotData

	words  []int
	memory []int

	screen     maninScreen
	prevScreen maninScreen
	hover      int

	caption       string
	station       int
	face          string
	messageHidden bool

	sys      maninSettings
	save     maninSaveState
	gallery  maninGalleryState
	mapState maninMapState
	attack   maninAttackState
	org      maninOrganizerState
	log      maninLogState
	staff    maninStaffRollState

	nameOpen bool
	logLines []maninLogLine

	nameFamily string
	nameGiven  string
	nameField  int

	// vulgarPlayback 表示当前正在播放标题「卑語モード」选中的 60x 场景；
	// 右键/返回要回标题，而不是按普通剧情继续。
	vulgarPlayback bool

	// characters 是 ctbl.rbd 解析结果，地图上用它决定每个角色当前在哪个站。
	characters       []maninCharacter
	charactersLoaded bool

	// cgNames 是 gtbl.rbd 的图片名顺序，鉴赏室按 24 格一页切分；首次
	// 打开鉴赏室时解析并缓存，避免每帧读盘。
	cgNames []string
	// cgVariants 以组代表名为键，保存同组的所有帧名（g101/g101p1...）。
	// 鉴赏室按组显示一张缩略图，解锁判定要任一变体已看过。
	cgVariants map[string][]string
	// hasSave 缓存「是否有存档」，标题悬停会高频调用 maninTitleItems；
	// 每次都扫 100 个槽位文件会让桌面 hover 明显卡顿。
	hasSave bool
}
