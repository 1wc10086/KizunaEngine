package core

import "unsafe"

type Step int

const (
	StepContinue    Step = 0
	StepAwaitInput  Step = 1
	StepAwaitChoice Step = 2
	StepAwaitSelect Step = 3
	StepWait        Step = 4
	StepFinished    Step = 5
	StepFaulted     Step = 6
	StepAwaitName   Step = 7
	// StepAwaitTitle 表示"引擎停在主界面（标题菜单）"。
	// 单独占一个 step 而不是复用 StepAwaitSelect：两者在宿主侧要画的东西完全不同
	// （地图要贴按钮图 + 命中掩码，标题要画菜单项），混用会让宿主分不清该渲染哪个。
	StepAwaitTitle Step = 8
	// StepAwaitMovie 表示"脚本在等影片播完"。
	// 影片由宿主的平台解码器播（引擎不解 MPEG-1），播放期间脚本必须停住：
	// 否则同一轮 run() 会把影片后面的对白全部执行掉，玩家看到的是
	// 影片还没播完、剧情已经跳过一大段。宿主播完调 OpMovieDone 才会继续。
	StepAwaitMovie Step = 9
)

type Info struct {
	Width      int
	Height     int
	Title      string
	Version    string
	SceneCount int
	EntryCount int
}

type Scene struct {
	ID     uint32
	Name   string
	Title  string
	Group  uint32
	Index  uint32
	Images []string
	// Unlocked 表示这个场景是否已在鉴赏里解锁（来自原引擎的全局存档）。
	Unlocked bool
}

type Slot struct {
	Index   uint32
	Title   string
	SavedAt string
	Summary string
	Exists  bool
}

type AudioEvent struct {
	Kind    uint32
	Path    string
	Volume  float64
	Loop    bool
	Channel uint32
	Group   uint32
	Names   []string
	Frames  uint32
}

type TextState struct {
	Speaker  string
	Text     string
	Awaiting uint32
}

type Choice struct {
	Name  string
	Value int32
}

type State struct {
	Bgm                  string
	PC                   uint32
	Scene                uint32
	Animating            bool
	WaitMs               uint32
	MessageWindowVisible bool
	// EngineMessageWindow 表示这一帧的对话框是引擎自己画进帧里的。
	// 宿主据此决定是否隐藏 Flutter 的 MessageBox：只靠设置开关不够，
	// 没有 [message window] 配置的作品仍需要 Flutter 兜底。
	EngineMessageWindow bool
	AwaitingName        bool
	BackLogActive       bool
	ReplayActive        bool
	InstructionStart    uint32
	NamePrompt          string
	// HostRequest 是引擎请求宿主执行的界面动作（打开设置、退出游戏等）。
	//
	// 引擎自己画不出 Flutter 的设置页，也不该直接关掉宿主页面；把"要做什么"
	// 放进每次状态快照里，由宿主按自己的路由处理。Serial 单调递增，宿主据此
	// 判断是不是新请求（同一请求会被状态轮询反复读到）。
	HostRequest       string
	HostRequestSerial uint32
	// MessageUnread 表示当前停在屏幕上的这句对白在显示前是“未读”。
	// 宿主用它实现原版「未読文章もスキップする」：关闭时，跳过遇到
	// 未读句子必须停下。必须由引擎在 markMessageRead 之前算出来，宿主
	// 自己无法区分“刚读到”和“以前读过”。
	MessageUnread bool
	// CeaseSkip/SkipUnread 是原版设置里的「選択肢でスキップを解除する」
	// 与「未読文章もスキップする」。宿主的自动推进逻辑据此决定何时停。
	CeaseSkip  bool
	SkipUnread bool
	// AutoWait 是「オート再生速度」（0..100，越大越快），来自引擎设置。
	// 宿主原来用固定文本长度公式，改了这个滑块也不会生效。
	AutoWait int
}

type TextUnit struct {
	ID   string
	File string
	Text string
}

type Frame struct {
	Width  int
	Height int
	Pixels []byte
}

type Engine interface {
	Open(dir string) (Info, error)
	Info() Info
	Start(entry uint32) (Step, error)
	Run(budget uint32) (Step, error)
	Advance() (Step, error)
	Choose(index uint32) (Step, error)
	Pointer(op uint32, x int32, y int32) (Step, error)
	Compose(dst unsafe.Pointer, n int) (bool, error)
	Scenes() []Scene
	Slots() []Slot
	Save(slot uint32, title string) error
	Load(slot uint32) (Step, error)
	AssetSize(path string) int
	ReadAsset(path string, dst unsafe.Pointer, n int) int
	Audio() []AudioEvent
	Text() TextState
	Choices() []Choice
	State() State
	TextList() []TextUnit
	LastError() string
}
