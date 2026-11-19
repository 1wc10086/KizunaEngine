package stage

// 舞台状态（§6 宏语义的落地点）。
// session 只调公开 API（SetBackground/SetCharacter/ShowCG/Fade/Tick/Compose），
// 内部按“背景 + 5 立绘槽 + 可选全屏 CG + 覆盖色”四层合成，转场用帧计数推进。
//
// 为什么 800x600 写死：原引擎的画布即 800x600（sysenv.tbl 的窗口大小），
// CPB 背景与 CG 恰好同尺寸；写成可配只会让缩放口径与引擎帧对不上。

const (
	Width     = 800
	Height    = 600
	CharSlots = 5

	// 立绘原图高 600：x 坐标是脚位/中心锚点（宏给的 400/200/600/160/640），
	// y 默认 0（底贴底）；角色图宽一般 200~400，按 x 居中切图是本层的约定。
	charAnchorY = 0
)

// State 是舞台的可观察快照：落存档时直接序列化，session 不必再拆内部层。
type State struct {
	Background string         `json:"background"`
	Characters []Slot         `json:"characters"` // 固定 CharSlots 槽，空槽 name=""
	CG         string         `json:"cg"`         // 非空即全屏 CG 模式（盖过立绘）
	FadeKind   FadeKind       `json:"fade_kind"`  // none/black/white
	FadeAlpha  int            `json:"fade_alpha"` // 0..255 当前覆盖
	Filter     string         `json:"filter"`     // fg_filter 的夜/黄昏/ sepia 名
	Layers     []OverlayLayer `json:"layers,omitempty"`
}

// OverlayLayer 是 0x27 建出的附加精灵层。X 是中心锚，Y 是底部锚，
// 与原版 0x27 的 x / -y 组装一致。
type OverlayLayer struct {
	Index   int     `json:"index"`
	Name    string  `json:"name"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Alpha   int     `json:"alpha"`
	Scale   float64 `json:"scale"`
	Visible bool    `json:"visible"`
	// Centered=true 用立绘的中心/底部锚（レイヤー 宏的 0x33 语义）；
	// false 用左上角锚（0x27 建全屏黑/白幕或左上角叠图）。
	Centered bool `json:"centered,omitempty"`
}

// Slot 是一条立绘层：图名 + 画布坐标 + 缩放 + 透明度 + 可见位。
type Slot struct {
	Name  string  `json:"name"`
	X     int     `json:"x"`     // 锚点 X（居中锚，见 placeChar）
	Y     int     `json:"y"`     // 锚点 Y（默认 0）
	Scale float64 `json:"scale"` // 1.0 = 原尺寸；fg_sizeL = 1.2、fg_sizeS = 0.66 近似（见 macro.go）
	Alpha int     `json:"alpha"` // 0..255，最终不透明度

	// 原引擎 0x33 是"给同一条层设新目标并动画过去"，不是叠两张图。
	// 早期实现用 prev/cur 两张帧交叉淡，同一角色会在两个 x 各画一次，
	// 看起来像"两个相同立绘"。这里改成单层补间：只画一张，位置/缩放/透明
	// 从 From* 线性插值到目标值，AnimMs 走完即清。
	FromX       int     `json:"from_x,omitempty"`
	FromY       int     `json:"from_y,omitempty"`
	FromScale   float64 `json:"from_scale,omitempty"`
	FromAlpha   int     `json:"from_alpha,omitempty"`
	AnimMs      int     `json:"anim_ms,omitempty"`
	AnimElapsed int     `json:"anim_elapsed,omitempty"`
}

// FadeKind 覆盖色（宏 暗転/白転 的落点）。
type FadeKind int

const (
	FadeNone FadeKind = iota
	FadeBlack
	FadeWhite
)

// TransitionKind 区分“背景交叉淡”与“立绘交叉淡”——
// 原引擎 trans_bg 走 1000ms、trans_fg 走 250ms，时长不同，合并成一个会互相顶掉进度。
type TransitionKind int

const (
	TransitionNone TransitionKind = iota
	TransitionBackground
	TransitionChar
)

// Status 向 session 暴露的动画进度：session 每帧 Tick 后只看它决定是否继续等脚本走。
type Status struct {
	TransitionKind TransitionKind
	Progress       float64 // 0..1（1 已完成）
	FadeActive     bool
	FadeDone       bool // 本帧内 Fade 刚完成（session 可据此放行补续脚本）
}

// PositionKind 是 §6 五档位置的常量化（x 坐标即 spec 表里的值）。
type PositionKind int

// 档位与 macro.asb 的 0x33 idx/x 一一对应（fgl/fgr 是远档 160/640，
// facel/facer 才是 200/600；早期把 fgl 当 200 会让远近层互相覆盖）。
const (
	PosCenter   PositionKind = iota // 0 / 400（fg/face）
	PosLeft                         // 1 / 200（facel、fgl_set）
	PosRight                        // 2 / 600（facer、fgr_set）
	PosFarLeft                      // 3 / 160（facell、fgl、fgll_set）
	PosFarRight                     // 4 / 640（facerr、fgr、fgrr_set）
)

// PositionX 返回常量档的 x（spec §6 的五档）。
func PositionX(p PositionKind) int {
	switch p {
	case PosCenter:
		return 400
	case PosLeft:
		return 200
	case PosRight:
		return 600
	case PosFarLeft:
		return 160
	case PosFarRight:
		return 640
	}
	return 400
}
