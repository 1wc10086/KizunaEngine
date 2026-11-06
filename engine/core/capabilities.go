package core

type OpenerExtras interface {
	OpenExtras(dir string, extraDirs []string, cacheDir string) (Info, error)
}

type NamePrompter interface {
	SubmitName(name string) bool
}

type AudioNotifier interface {
	NotifyAudioCompleted(file string, voice bool, channel uint32)
}

type MapButton struct {
	Index  uint32
	Image  string
	X      int32
	Y      int32
	Width  uint32
	Height uint32
	Value  int32
	Label  string
}

type MapExposer interface {
	MapButtons() []MapButton
	MapMask() (bool, int, int)
}

type SelectItem struct {
	Index        uint32
	Text         string
	Frame        string
	X            int32
	Y            int32
	Width        uint32
	Height       uint32
	TextX        int32
	TextY        int32
	Alignment    uint32
	DefaultColor uint32
	SelectColor  uint32
}

type SelectItemExposer interface {
	SelectItems() []SelectItem
}

type CgGroup struct {
	Name     string
	Variants [][]string
}

type SlotPreview struct {
	SavedAt string
	Summary string
	Speaker string
	Text    string
	Thumb   Frame
}

type GalleryComposer interface {
	ComposeImages(names []string) (Frame, bool)
	SetPreview(f Frame)
	SelectFrame(name string) (Frame, bool)
	CgGroups() []CgGroup
}

// GalleryUnlocker 暴露"鉴赏条目是否已解锁"。
//
// 解锁状态来自原引擎的全局存档（common.dat），不是我们自己的槽位存档；
// 因此单独一个接口，而不是塞进 CgGroup：布局（分组/变体）和解锁状态
// 来自两份完全不同的数据源，混在一起会让任何一方变化都要动同一个结构。
type GalleryUnlocker interface {
	// CgUnlockedByName 按图片名查询（cg.dat 名字表里的名字）。
	// 用名字而不是下标：宿主拿到的变体就是名字列表，让它去关心名字表下标
	// 只会把引擎内部的目录布局泄漏到协议层。
	CgUnlockedByName(name string) bool
	// SceneUnlocked 按 scene.dat 行序查询。
	SceneUnlocked(index int) bool
}

type SlotPreviewer interface {
	PreviewSlot(slot uint32) (SlotPreview, bool)
}

type OptionSetter interface {
	SetOption(key uint32, value bool)
}

// TitleMenu 是"主界面（标题菜单）"的抽象。Flutter 侧不关心品牌怎么实现，
// 只按项数画按钮、把点击回传给引擎。
type TitleMenuItem struct {
	Key    string
	Label  string
	X      int32
	Y      int32
	Width  uint32
	Height uint32
	// Enabled=false 表示这一项当前不可用（例如没有存档时的"继续游戏"）。
	Enabled bool
}

// MovieExposer 暴露"当前该播的影片文件"给宿主。
//
// 影片解码必须交给平台能力（Android 上是 fvp/libmdk 的 MediaCodec 通路），
// 引擎侧的 Go 代码不该自己解 MPEG-1；所以只把绝对路径交出去，
// 播完由宿主回调 Advance 继续脚本。
type MovieExposer interface {
	// CurrentMovie 返回待播影片的绝对路径；空串表示当前没有影片。
	CurrentMovie() string
	// MovieSerial 返回影片请求序号，宿主据此判断"是不是新的一条"。
	MovieSerial() int
}

// MovieFinisher 处理"影片播完"的后续。引擎区分标题 OP（回主界面）
// 与剧本影片（继续脚本），宿主不必知道这个差别。
type MovieFinisher interface {
	FinishMovie() (Step, error)
}

type TitleMenuExposer interface {
	// TitleMenu 返回当前主界面菜单项；空切片表示当前不在主界面。
	TitleMenu() []TitleMenuItem
	// SelectTitleItem 处理主界面点击，key 取自 TitleMenuItem.Key。
	SelectTitleItem(key string) (Step, error)
}

type KeyReceiver interface {
	PressKey(code uint32) (Step, error)
}

type WheelReceiver interface {
	Wheel(delta int32) (Step, error)
}

type Closer interface {
	Close() error
}

// StepReporter 暴露引擎"当前步"而不推进脚本。OpFrame 用它和 text/state/audio 一起在
// 一次 RPC 里返回；未实现的品牌该字段写 StepContinue。
type StepReporter interface {
	CurrentStep() Step
}
