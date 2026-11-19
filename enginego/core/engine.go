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
	// PromptIsYesNo 表示 AwaitingName 对应的不是文字输入，而是原引擎的
	// GetYesNoMessage：宿主应显示“是/否”而不是文本框。
	PromptIsYesNo bool
	// EnginePrompt 表示 emon 已经把 prompt 对话框画进帧里。宿主不能再叠
	// NameDialog；文字输入只保留一个透明的 IME 输入层。
	EnginePrompt     bool
	BackLogActive    bool
	ReplayActive     bool
	InstructionStart uint32
	NamePrompt       string
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


// TextRun 是一段交给宿主绘制的文字。引擎不再把字形栅格进帧位图：布局
// （折行、基线、行距）仍由 Go 决定，Flutter 只负责用平台字体把这段文字
// 画到 (X, Y) 的基线上。这样既不需要内置 TTF，也保留了原版排版坐标。
//
// 坐标是引擎坐标系（与帧位图同尺寸）。Y 是文字基线，不是顶边：
// Flutter 的字体 ascent 与旧内嵌字体不同，按顶边对齐会让整行上下漂移。
type TextRun struct {
	Text string
	X    int32
	Y    int32
	Size int32
	// Color/Outline 均为 0xAARRGGBB。Outline 的 alpha 跟随正文淡出。
	Color   uint32
	Outline uint32
	// Shadow 是阴影偏移（像素，0 表示不画阴影）。
	Shadow int32
	// Align 保留给宿主：0=左，1=居中，2=右。引擎已把 X 算成行的实际起点，
	// 宿主按左对齐绘制即可；字段保留是为了未来支持逐字重排。
	Align uint32
	// Width 是该行可用的最大宽度（0 表示不限制），用于宿主的调试与裁剪。
	Width int32
	Bold  bool
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

// TextRunSignature 计算一组文字覆盖层的稳定指纹。协议层用它判断"位图没变但
// 文字变了"：逐字显示、悬停高亮、淡入都只改文字，不碰帧位图，宿主仍需要
// 一次新帧才能重画文字。FNV-1a 足够快（每帧几十条 run），也不需要加密强度。
func TextRunSignature(runs []TextRun) uint64 {
	const prime = 1099511628211
	hash := uint64(14695981039346656037)
	mix := func(value uint64) {
		hash ^= value
		hash *= prime
	}
	for i := range runs {
		run := &runs[i]
		mix(uint64(uint32(run.X)))
		mix(uint64(uint32(run.Y)))
		mix(uint64(uint32(run.Size)))
		mix(uint64(run.Color))
		mix(uint64(run.Outline))
		mix(uint64(uint32(run.Shadow)))
		mix(uint64(uint32(run.Width)))
		if run.Bold {
			mix(1)
		}
		mix(uint64(len(run.Text)))
		for j := 0; j < len(run.Text); j++ {
			mix(uint64(run.Text[j]))
		}
	}
	return hash
}
