package stage

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	overgfx "kizuna/enginego/brand/overdose/gfx"
)

// Stage 是 800x600 的画布合成器：背景 -> 立绘槽(0..4) -> CG -> 覆盖色。
// 所有过渡都预先装进一帧（curr + prev 快照），Compose 时按进度二分混合。
type Stage struct {
	cache *overgfx.Cache

	background string
	chars      [CharSlots]Slot
	cg         string // 全屏 CG 图名（ev 宏）；非空盖在立绘之上

	// 快照：过渡起始时的整帧。Tick 只改 progress，Compose 每次都重跑全层合成。
	// 1.9MB 一张，预先退役两张 snapshot 只在转场期间有效，转场结束即回收，避免常驻双倍内存。
	prevBg    string
	prevChars [CharSlots]Slot
	prevCG    string
	hasPrev   bool

	fade      FadeKind
	fadeAlpha int // 0..255；Fade(…, toBlack=false) 走白色时由白覆盖
	fadeDurMs int
	fadeFrom  int
	fadeTo    int
	// fadeElapsed 是幕布独立时钟。Why 不能和 transition 共用 elapsedMs：
	// H 场景里 evd 每 0.4~1s 换一张 CG，每次都会开始新交叉淡并重置
	// transition 的 elapsed；共用时幕布进度被打回 0，黑幕反复回到 255，
	// 看起来就是"黑色闪烁转场"。
	fadeElapsed int

	// flash 是 フラッシュ/絶頂 系宏的瞬时白屏，独立于 scene fade 计时。
	flashWhite    bool
	flashAlpha    int
	flashDurMs    int
	flashHoldMs   int
	flashElapsed  int
	filter        string // fg_filter / モザイク / 回想 的后期滤镜
	filterDurMs   int    // >0 表示临时滤镜（モザイク）的剩余时长
	filterElapsed int

	// layers 是 0x27/0x28 的附加精灵层（idx 0..3）。原版用它做
	// 白闪/黑幕、CG 叠层和 interrupt 演出；不画会让这些场景缺一层。
	layers [16]OverlayLayer

	transKind TransitionKind
	prevTick  *Status // 供 Tick 返回“本帧刚完成”的一次性信号（见 fadeDone 字段）

	scaleMode overgfx.ScaleMode // 立绘缩放模式（默认双线性；UI 贴图可切最近邻）

	durationMs int  // 当前过渡总时长
	elapsedMs  int  // 已累加时长
	fadeDone   bool // Tick 内置位、Compose 读后清
	dirty      bool
}

// NewStage 建舞台。cache 可空——空时合成只剩黑底（测试与纯逻辑路径用）。
func NewStage(cache *overgfx.Cache) *Stage {
	s := &Stage{cache: cache}
	s.chars[0].Scale = 1
	return s
}

// Visible 报告是否在 CG 模式（全屏 CG 盖过立绘/背景转场）。
func (s *Stage) Visible() bool { return s.cg != "" }

// State 返回可序列化的舞台快照。
func (s *Stage) State() State {
	out := State{
		Background: s.background,
		CG:         s.cg,
		FadeKind:   s.fade,
		FadeAlpha:  s.fadeAlpha,
		Filter:     s.filter,
	}
	for i := range s.layers {
		if s.layers[i].Visible {
			out.Layers = append(out.Layers, s.layers[i])
		}
	}
	out.Characters = append(make([]Slot, 0, CharSlots), s.chars[:]...)
	return out
}

// Restore 从快照恢复。Fade 瞬时按快照值落，不恢复过渡中段——
// 存档只在“消息间断”点写入，恢复后不丢转场尾巴是伪需求。
func (s *Stage) Restore(st State) {
	s.background = st.Background
	s.cg = st.CG
	// 存档点都在消息间断；不恢复持久 fade，避免旧档里卡住的黑色覆盖层
	// 把整个读档画面盖死。Curtain/flash 本来就是瞬时效果，也不恢复。
	s.fade = FadeNone
	s.fadeAlpha = 0
	s.filter = st.Filter
	if s.filter == "mosaic" {
		// 马赛克是 2000ms 的临时演出；旧档里如果正处于马赛克，读档不应该
		// 让它变成永久滤镜。
		s.filter = ""
	}
	s.filterDurMs = 0
	s.filterElapsed = 0
	s.flashDurMs = 0
	s.flashAlpha = 0
	for i := range s.layers {
		s.layers[i] = OverlayLayer{}
	}
	for _, layer := range st.Layers {
		if layer.Index >= 0 && layer.Index < len(s.layers) {
			s.layers[layer.Index] = layer
		}
	}
	for i := 0; i < CharSlots && i < len(st.Characters); i++ {
		s.chars[i] = st.Characters[i]
		// 读档不恢复"补间中段"：目标几何已经在 Slot.X/Y/Scale 里，
		// 继续跑旧动画会从存档前的 From 值滑一下，像瞬移。
		s.chars[i].AnimMs = 0
		s.chars[i].AnimElapsed = 0
	}
	s.hasPrev = false
	s.transKind = TransitionNone
	s.elapsedMs = 0
	s.fadeElapsed = 0
	s.fadeDurMs = 0
	s.fadeDone = false
	s.dirty = true
}

// SetBackground 设背景（bg 宏）。立即生效；若带过渡请用 BeginBackgroundCrossfade。
// 空名只清背景，不改立绘（原引擎 bg 之后立绘保留）。
// 关键：bg 表示离开全屏 CG 模式。原版 ev/evd 之后常直接 bg 回场景，
// 如果不清 s.cg，Compose 会永远优先画旧 CG，背景/立绘再也不会更新。
func (s *Stage) SetBackground(name string) {
	s.background = name
	s.cg = ""
	// 原版 暗転/白転 的黑/白是 0 号层，会一直留到下一次 bg/ev 才被替换。
	// 关键是"淡入中"也算幕布待替换：宏体里的 1000ms 等待我们不执行，
	// bg/ev 往往在幕布只到 10% 时就到了；若此时 ClearFade，幕布会继续
	// 淡到全黑并盖住新图（表现为黑屏/CG 不显示）。这里先补满再淡出。
	// 只在"待替换幕布"时改道淡出；已在淡出（to==0）的幕布必须让它跑完。
	// 这里若 ClearFade，快速连续的 bg 会把剩余黑幕瞬间抹掉，就是黑闪。
	s.consumeCurtain()
	s.ClearMosaic()
	s.dirty = true
}

// SetCharacter 把图名放进槽 slot（0..4），x 为中心锚。越界槽位静默忽略。
func (s *Stage) SetCharacter(slot int, name string, x int) {
	if slot < 0 || slot >= CharSlots {
		return
	}
	ch := &s.chars[slot]
	ch.Name = name
	ch.X = x
	ch.Y = charAnchorY
	if ch.Scale == 0 {
		ch.Scale = 1
	}
	if ch.Alpha == 0 {
		ch.Alpha = 255
	}
	s.dirty = true
}

// SetCharacterFull 是 SetCharacter 的全参数版（缩放/透明度由 fg_size*/fg_filter 等宏传）。
func (s *Stage) SetCharacterFull(slot int, name string, x, y int, scale float64, alpha int) {
	if slot < 0 || slot >= CharSlots {
		return
	}
	ch := &s.chars[slot]
	ch.Name = name
	ch.X = x
	ch.Y = y
	if scale == 0 {
		scale = 1
	}
	ch.Scale = scale
	ch.Alpha = clampAlpha(alpha, 255)
	s.dirty = true
}

// ClearCharacters 对应 0x34(-1)：一次清空全部立绘层（暗転/モザイク/終端用）。
func (s *Stage) ClearCharacters() {
	for i := range s.chars {
		s.chars[i] = Slot{Scale: 1}
	}
	s.dirty = true
}

// ClearCharacter 清一槽（fg_del 系宏）。不清缓存——同名图下次可直接命中。
func (s *Stage) ClearCharacter(slot int) {
	if slot < 0 || slot >= CharSlots {
		return
	}
	s.chars[slot].Name = ""
	s.dirty = true
}

// SetCG 设全屏 CG（ev 宏）。HideCG 即清（见下）。
func (s *Stage) SetCG(name string) {
	if name == "" {
		// ev/evd 读的是 global str(2,0)；变量为空时原引擎是把 0 号层设成
		// 空图（等于撤掉 CG）。旧实现直接 return，会让上一张 CG 永远固定
		// 在画面上——正是“过一会 CG 不动了/场景全黑”的一类根因。
		s.HideCG()
		return
	}
	// ev/evd 在原引擎是 0x27 给 0 号层设图，动画时间 1000ms；
	// 这里做整帧交叉淡（旧帧快照 → 新 CG），保证不会硬切。
	s.beginFrameCrossfade(1000)
	s.cg = name
	s.ClearMosaic()
	if s.consumeCurtain() {
		// 幕布仍在（含淡入中）：内容已经换成 CG，直接让幕布淡出即可。
		// 不能让它继续淡到 255——那会把整段 CG 盖黑。
		s.hasPrev = false
		s.transKind = TransitionNone
	}
	s.dirty = true
}

// HideCG 离开 CG 模式（背景与立绘回到可见）。
func (s *Stage) HideCG() {
	s.cg = ""
	s.dirty = true
}

// ShowCG 是 SetCG 的别名，给 session 直白的调用名。
func (s *Stage) ShowCG(name string) { s.SetCG(name) }

// SetOverlayLayer 是 0x27 的落点：idx 0..3；name 为空清层，idx<0 清全部。
// x 是中心锚，y 是底部锚；alpha 0..255，scale 1=原尺寸。
func (s *Stage) SetOverlayLayer(idx int, name string, x, y, alpha int, scale float64, centered bool) {
	if name == "" {
		s.ClearOverlayLayer(idx)
		return
	}
	if idx < 0 {
		for i := range s.layers {
			s.layers[i] = OverlayLayer{Index: i, Name: name, X: x, Y: y, Alpha: clampAlpha(alpha, 255), Scale: scale, Visible: true, Centered: centered}
		}
		s.dirty = true
		return
	}
	if idx >= len(s.layers) {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	s.layers[idx] = OverlayLayer{Index: idx, Name: name, X: x, Y: y, Alpha: clampAlpha(alpha, 255), Scale: scale, Visible: true, Centered: centered}
	s.dirty = true
}

// ClearOverlayLayer 对应 0x28/0x34：idx<0 或 >=4 清全部（原版 -1 扩清）。
func (s *Stage) ClearOverlayLayer(idx int) {
	if idx < 0 || idx >= len(s.layers) {
		for i := range s.layers {
			s.layers[i] = OverlayLayer{}
		}
		s.dirty = true
		return
	}
	s.layers[idx] = OverlayLayer{}
	s.dirty = true
}

// Filter 返回当前 fg_filter 的附加滤镜名（原引擎由 global int var 90 决定，
// session 侧持有；这里命名的字符串只做透传与恢复用，不参与合成）。
func (s *Stage) SetFilter(name string) {
	s.filter = name
	s.filterDurMs = 0
	s.filterElapsed = 0
	s.dirty = true
}

// TempFilter 是 モザイク 这类“持续一段演出”的滤镜：原宏会等 2000ms 再
// msgon，效果只属于当前场景/图层，不能永久留到后面的 HCG。到时自动清。
func (s *Stage) TempFilter(name string, durationMs int) {
	s.filter = name
	s.filterElapsed = 0
	s.filterDurMs = max(0, durationMs)
	s.dirty = true
}

// ClearMosaic 在换背景/CG 时撤掉马赛克：新场景/新 CG 不可能继承上一段的
// 局部马赛克。sepia 等持久滤镜不在这里清（回想需要跨场景保留）。
func (s *Stage) ClearMosaic() {
	if s.filter == "mosaic" {
		s.filter = ""
		s.filterDurMs = 0
		s.filterElapsed = 0
		s.dirty = true
	}
}

// Flash 触发瞬时白屏（フラッシュ/絶頂 系宏）。durationMs<=0 视为 150ms。
// 与 scene fade 分开计时：flash 会自己衰减到 0，不阻塞脚本。
func (s *Stage) Flash(durationMs int, white bool) {
	if durationMs <= 0 {
		durationMs = 150
	}
	s.flashWhite = white
	s.flashAlpha = 255
	s.flashHoldMs = 0
	s.flashDurMs = durationMs
	s.flashElapsed = 0
	s.dirty = true
}

// Curtain 是 暗転/白転 的定时幕布：先保持全黑/全白 holdMs，再用 300ms 淡出。
// 不能用持久 Fade：原版暗転是 0x27 黑层 + 1000ms 等待，场景切换后会被
// bg/ev 或后续图层替换；持久化会让整段游戏停在黑屏。
// Curtain 对应 暗転/白転：把 0 号层换成全黑/全白，1000ms 内淡入后**保持**。
// 原引擎不会自动把它撤掉——下一句 bg/ev 用同一层设新图时才替换。
// 旧实现 hold+fade-out 会在每次转场后自动露出旧画面，表现为反复黑闪。
func (s *Stage) Curtain(holdMs int, white bool) {
	if holdMs <= 0 {
		holdMs = 1000
	}
	if white {
		s.fade = FadeWhite
	} else {
		s.fade = FadeBlack
	}
	s.startFade(holdMs, 255)
}

// curtainPending 判断是否存在"正在盖向全黑/全白"的幕布（含淡入中）。
// 不能只认 fadeAlpha==255：宏体里的等待我们不执行，bg/ev 常在淡入
// 刚开始时就到达；把这种情况漏掉会让幕布继续淡到全黑并盖住新内容。
func (s *Stage) curtainPending() bool {
	return s.fade != FadeNone && s.fadeTo == 255
}

// consumeCurtain 消费待替换幕布：先补满，再 1000ms 淡出。
// 先补满是原引擎"黑层先到位、随后新层替换"的顺序；直接 ClearFade
// 会从中间灰直接跳到新图，看起来就是黑色闪烁。
func (s *Stage) consumeCurtain() bool {
	if !s.curtainPending() {
		return false
	}
	s.fadeAlpha = 255
	s.startFade(1000, 0)
	return true
}

// ClearSceneContent 对应 暗転/map_close/終端 里连续出现的 0x28(-1)+0x34(-1)：
// 原版会把 0 号效果层与全部立绘层清空，再铺黑幕。旧实现只铺幕布、
// 不清立绘，后续 bg 一换就会把上一场的人物带进新场景。
func (s *Stage) ClearSceneContent() {
	s.background = ""
	s.cg = ""
	for i := range s.chars {
		s.chars[i] = Slot{Scale: 1}
	}
	for i := range s.layers {
		s.layers[i] = OverlayLayer{}
	}
	s.ClearMosaic()
	s.dirty = true
}

// ClearFade 清掉持久 scene fade（场景内容已切换时调用）。
// 不清 flash/curtain：暗転结束后的 bg 要让定时幕布自然跑完。
func (s *Stage) ClearFade() {
	s.fade = FadeNone
	s.fadeAlpha = 0
	s.fadeDurMs = 0
	s.fadeDone = false
}

// Fade 启动覆盖色过渡（暗転/白転）。durationMs<=0 时立即到位。
// toBlack=true → 黑；false → 白。白覆盖从 255 起还是 0 起由调用方决定方向：
// 这里只设方向，起点是当前 alpha、终点 255（淡入）或 0（淡出）按 modeBit 推断。
// 为避开歧义，Fade 按“到 255”为统一目标， FadeOutToClear 另提供撤下路径。
func (s *Stage) Fade(durationMs int, toBlack bool) {
	s.fade = FadeBlack
	if !toBlack {
		s.fade = FadeWhite
	}
	s.startFade(durationMs, 255)
}

// FadeOutToClear 把覆盖色淡出到 0 并清 FadeKind（暗転/白転 宏退出时用）。
func (s *Stage) FadeOutToClear(durationMs int) {
	if s.fade == FadeNone {
		return
	}
	s.startFade(durationMs, 0)
}

func (s *Stage) startFade(durationMs, target int) {
	s.fadeFrom = s.fadeAlpha
	s.fadeTo = target
	s.fadeDurMs = max(0, durationMs)
	s.fadeElapsed = 0
	if s.fadeDurMs == 0 {
		s.fadeAlpha = target
		s.fadeDone = true
		if target == 0 {
			s.fade = FadeNone
		}
	} else if target == 255 {
		// 淡入时先不置 done，等 Tick 走满
		s.fadeDone = false
	}
	s.dirty = true
}

// BeginBackgroundCrossfade 开始背景交叉淡（bg 宏的转场形）。
// 旧背景在 prevBg，进度内旧新二分混合；duration<=0 时直接切新背景。
func (s *Stage) BeginBackgroundCrossfade(newBg string, durationMs int) {
	if s.curtainPending() {
		// 幕布→新背景：先把幕布补满（如果还在淡入），再淡出到新背景。
		s.background = newBg
		s.cg = ""
		s.ClearMosaic()
		s.fadeAlpha = 255
		s.startFade(max(0, durationMs), 0)
		s.hasPrev = false
		s.transKind = TransitionNone
		s.dirty = true
		return
	}
	s.prevBg = s.background
	s.prevChars = s.chars
	s.prevCG = s.cg
	s.hasPrev = true
	s.transKind = TransitionBackground
	s.background = newBg
	// 背景转场同样意味着退出 CG：prevCG 已抓到快照里，旧 CG 会在转场
	// 前半段淡出，新帧只画背景/立绘。
	s.cg = ""
	// 不 ClearFade：淡出中的幕布要自然跑完，否则连续切 bg 会瞬间露底。
	s.ClearMosaic()
	s.durationMs = max(0, durationMs)
	s.elapsedMs = 0
	s.dirty = true
	if s.durationMs == 0 {
		s.hasPrev = false
		s.transKind = TransitionNone
		s.fadeDone = true
	}
}

// beginFrameCrossfade 抓当前整帧作为 prev，供 ev/bg 的整帧交叉淡使用。
func (s *Stage) beginFrameCrossfade(durationMs int) {
	s.prevBg = s.background
	s.prevChars = s.chars
	s.prevCG = s.cg
	s.hasPrev = true
	s.transKind = TransitionBackground
	s.durationMs = max(0, durationMs)
	s.elapsedMs = 0
	if s.durationMs == 0 {
		s.hasPrev = false
		s.transKind = TransitionNone
		s.fadeDone = true
	}
}

// BeginCharCrossfade 开始立绘交叉淡（trans_fg/trans_face 宏）。
// 旧立绘在 prevChars，进度内二分混合；duration<=0 立即落位。
func (s *Stage) BeginCharCrossfade(chars [CharSlots]Slot, durationMs int) {
	s.prevBg = s.background
	s.prevChars = s.chars
	s.prevCG = s.cg
	s.hasPrev = true
	s.transKind = TransitionChar
	s.chars = chars
	s.durationMs = max(0, durationMs)
	s.elapsedMs = 0
	s.dirty = true
	if s.durationMs == 0 {
		s.hasPrev = false
		s.transKind = TransitionNone
		s.fadeDone = true
	}
}

// CommitTransition 是转场提交（trans_bg/trans_fg/trans_face 宏）：
// 若转场中途被叫停，认为已完成（原引擎到此即切到新层）。
func (s *Stage) CommitTransition() {
	s.hasPrev = false
	s.transKind = TransitionNone
	s.fadeDone = true
	s.dirty = true
}

// Tick 推进时钟。dtMs 为本帧毫秒。返回 Status 供 session 决定是否继续等动画。
// Animating 报告是否有正在推进的转场/淡入淡出。session 的 State 用它
// 驱动宿主持续请求合成；不能靠 Tick 才知道，因为 Tick 只在 Compose 里跑。
func (s *Stage) Animating() bool {
	if s.transKind != TransitionNone && s.durationMs > 0 && s.elapsedMs < s.durationMs {
		return true
	}
	if s.fade != FadeNone && s.fadeDurMs > 0 && s.fadeElapsed < s.fadeDurMs {
		return true
	}
	if s.flashDurMs > 0 || s.filterDurMs > 0 {
		return true
	}
	for i := range s.chars {
		if s.chars[i].AnimMs > 0 {
			return true
		}
	}
	return false
}

func (s *Stage) Tick(dtMs int) Status {
	if dtMs > 0 {
		s.elapsedMs += dtMs
		s.fadeElapsed += dtMs
		for i := range s.chars {
			ch := &s.chars[i]
			if ch.AnimMs > 0 {
				ch.AnimElapsed += dtMs
				if ch.AnimElapsed >= ch.AnimMs {
					ch.AnimElapsed = ch.AnimMs
					ch.AnimMs = 0
					ch.FromX, ch.FromY = ch.X, ch.Y
					ch.FromScale, ch.FromAlpha = ch.Scale, ch.Alpha
				}
			}
		}
		if s.filterDurMs > 0 {
			s.filterElapsed += dtMs
			if s.filterElapsed >= s.filterDurMs {
				s.filter = ""
				s.filterDurMs = 0
				s.filterElapsed = 0
			}
		}
		if s.flashDurMs > 0 {
			s.flashElapsed += dtMs
			if s.flashElapsed >= s.flashDurMs {
				s.flashAlpha = 0
				s.flashDurMs = 0
				s.flashHoldMs = 0
				s.flashElapsed = 0
			} else if s.flashElapsed <= s.flashHoldMs {
				s.flashAlpha = 255
			} else {
				fade := s.flashDurMs - s.flashHoldMs
				if fade <= 0 {
					s.flashAlpha = 0
				} else {
					s.flashAlpha = 255 - 255*(s.flashElapsed-s.flashHoldMs)/fade
				}
			}
		}
	}
	st := Status{TransitionKind: s.transKind, FadeActive: s.fade != FadeNone, FadeDone: s.fadeDone}
	// 幕布用独立 fadeElapsed；transition 的新旧帧进度不受换 CG 重置。
	if s.fade != FadeNone && s.fadeDurMs > 0 {
		if s.fadeElapsed >= s.fadeDurMs {
			s.fadeAlpha = s.fadeTo
			s.fadeDone = true
			st.FadeDone = true
			// 只在“淡出到 0”时清 FadeKind；淡入到 255 后保留覆盖（白转后白板在）。
			if s.fadeTo == 0 {
				s.fade = FadeNone
				st.FadeActive = false
			}
		} else {
			span := s.fadeTo - s.fadeFrom
			s.fadeAlpha = s.fadeFrom + span*s.fadeElapsed/s.fadeDurMs
		}
	}
	if s.transKind != TransitionNone && s.durationMs > 0 {
		st.Progress = float64(s.elapsedMs) / float64(s.durationMs)
		if st.Progress >= 1 {
			st.Progress = 1
			s.hasPrev = false
			s.transKind = TransitionNone
			s.fadeDone = true
			st.FadeDone = true
			st.TransitionKind = TransitionNone
			s.durationMs = 0
			s.elapsedMs = 0
		}
	} else if s.transKind != TransitionNone {
		st.Progress = 1
	}
	// 读后不清：Compose 与 session 都会读同一帧位，直到下一 Tick 覆盖。
	return st
}

// progress 给合成层用的转场进度（0..1）。
func (s *Stage) progress() float64 {
	if s.transKind == TransitionNone || s.durationMs <= 0 {
		return 1
	}
	p := float64(s.elapsedMs) / float64(s.durationMs)
	if p > 1 {
		p = 1
	}
	return p
}

// Compose 把当前画面画到 dst（800x600）。dst 必须 800x600，否则按图面重分配（见 resize）——
// 实际上 frame 就是 800x600，这里要求的只是防御：低于该尺寸直接裁画。
func (s *Stage) Compose(dst *bishopgfx.BgraCanvas) {
	if dst == nil {
		return
	}
	if dst.Width != Width || dst.Height != Height {
		dst.Clear(0, 0, 0, 255)
		return
	}
	// 先把整帧清黑（Alpha=255），其后全部用覆盖式混合，目标色不会透。
	dst.Clear(0, 0, 0, 255)

	// 不再为 CG 走"直贴"快路径：ev 在原引擎里也是给 0 号层设图并动画 1000ms，
	// 直贴会绕过交叉淡，看起来像硬切；统一由 drawFrame 把 CG 画在最后即可。
	p := s.progress()
	if s.hasPrev && p < 1 {
		// 转场中：先旧帧（prev 快照）画满，再用 1-p 的权重把新帧叠上。
		// Why 顺序：先旧后新，新帧按 p 的“显露比”混合 —— 与人眼感知的交叉淡一致。
		dst.Clear(0, 0, 0, 255)
		// 旧帧只换背景/CG，立绘一律用当前帧：角色层有自己 1000ms 的补间，
		// 若这里用 prevChars，背景转场期间换表情会把"旧脸+新脸"同时画出来。
		s.drawFrame(dst, s.prevBg, s.chars, s.prevCG, 1)
		mix := int(p * 255)
		if mix > 0 {
			if mix >= 255 {
				dst.Clear(0, 0, 0, 255)
				s.drawFrame(dst, s.background, s.chars, s.cg, 1)
			} else {
				// 新帧画到临时画布再按 mix 叠——直接双遍画同一 dst 也等效，
				// 这里用 Overlay 混合第二遍的每像素权重：更慢但只要一遍特殊路径。
				s.drawFrameMixed(dst, s.background, s.chars, s.cg, mix)
			}
		}
	} else {
		s.drawFrame(dst, s.background, s.chars, s.cg, 1)
	}
	s.drawLayers(dst)
	// 滤镜/闪白/幕布必须走 finishFrame；旧代码在非 CG 路径只调 applyFade，
	// 导致 sepia/フラッシュ 在普通场景里永远不出现。
	s.finishFrame(dst)
}

// drawLayers 把 0x27 附加层按 idx 顺序叠在背景/立绘/CG 之上、滤镜与淡入淡出之下。
// 0x27 的 x 是中心、y 是底部；scale=1 时和立绘同一锚点规则。
func (s *Stage) drawLayers(dst *bishopgfx.BgraCanvas) {
	for i := range s.layers {
		layer := s.layers[i]
		if !layer.Visible || layer.Name == "" || layer.Alpha <= 0 {
			continue
		}
		img := s.imageOf(layer.Name)
		if img == nil {
			continue
		}
		scale := layer.Scale
		if scale <= 0 {
			scale = 1
		}
		dw := int(float64(img.Width) * scale)
		dh := int(float64(img.Height) * scale)
		if dw <= 0 || dh <= 0 {
			continue
		}
		draw := img
		if dw != img.Width || dh != img.Height {
			draw = overgfx.ScaleImage(img, dw, dh, s.scaleMode)
			if draw == nil {
				continue
			}
		}
		x, y := layer.X, layer.Y
		if layer.Centered {
			x = layer.X - dw/2
			y = layer.Y + (Height - dh)
		}
		dst.Blit(draw, x, y, layer.Alpha)
	}
}

// finishFrame 是舞台收尾：滤镜 → 场景淡入淡出 → 瞬时白屏。
// 顺序固定，flash 必须在最上层，否则会被 sepia/mosaic 染色。
func (s *Stage) finishFrame(dst *bishopgfx.BgraCanvas) {
	s.applyFilter(dst)
	s.applyFade(dst)
	s.applyFlash(dst)
	s.dirty = false
}

func (s *Stage) applyFlash(dst *bishopgfx.BgraCanvas) {
	if s.flashDurMs <= 0 || s.flashAlpha <= 0 {
		return
	}
	a := clampAlpha(s.flashAlpha, 255)
	if s.flashWhite {
		dst.Overlay(255, 255, 255, a)
	} else {
		dst.Overlay(0, 0, 0, a)
	}
}

// applyFilter 实现原版 0x2c 的整屏后期：sepia / mosaic / night / evening。
// 原引擎的滤镜作用在背景与立绘层，消息窗不受影响——session 在舞台之后才画
// 对话框，因此这里直接处理 dst 是安全的。
func (s *Stage) applyFilter(dst *bishopgfx.BgraCanvas) {
	switch s.filter {
	case "sepia":
		for off := 0; off+4 <= len(dst.Buffer); off += 4 {
			b, g, r := int(dst.Buffer[off]), int(dst.Buffer[off+1]), int(dst.Buffer[off+2])
			gray := (r*299 + g*587 + b*114) / 1000
			dst.Buffer[off] = byte(min(255, gray*80/100))
			dst.Buffer[off+1] = byte(min(255, gray*95/100))
			dst.Buffer[off+2] = byte(min(255, gray*125/100))
		}
	case "mosaic":
		s.mosaic(dst, 12)
	case "night":
		for off := 0; off+4 <= len(dst.Buffer); off += 4 {
			dst.Buffer[off] = byte(int(dst.Buffer[off]) * 8 / 10)
			dst.Buffer[off+1] = byte(int(dst.Buffer[off+1]) * 8 / 10)
			dst.Buffer[off+2] = byte(int(dst.Buffer[off+2]) * 9 / 10)
		}
	case "evening":
		for off := 0; off+4 <= len(dst.Buffer); off += 4 {
			dst.Buffer[off] = byte(int(dst.Buffer[off]) * 7 / 10)
			dst.Buffer[off+1] = byte(int(dst.Buffer[off+1]) * 8 / 10)
			dst.Buffer[off+2] = byte(min(255, int(dst.Buffer[off+2])*10/10))
		}
	}
}

func (s *Stage) mosaic(dst *bishopgfx.BgraCanvas, block int) {
	if block <= 1 {
		return
	}
	for y := 0; y < dst.Height; y += block {
		for x := 0; x < dst.Width; x += block {
			off := (y*dst.Width + x) * 4
			if off+3 >= len(dst.Buffer) {
				continue
			}
			b, g, r, a := dst.Buffer[off], dst.Buffer[off+1], dst.Buffer[off+2], dst.Buffer[off+3]
			maxY := min(y+block, dst.Height)
			maxX := min(x+block, dst.Width)
			for yy := y; yy < maxY; yy++ {
				for xx := x; xx < maxX; xx++ {
					o := (yy*dst.Width + xx) * 4
					dst.Buffer[o], dst.Buffer[o+1], dst.Buffer[o+2], dst.Buffer[o+3] = b, g, r, a
				}
			}
		}
	}
}

func (s *Stage) drawFrame(dst *bishopgfx.BgraCanvas, bg string, chars [CharSlots]Slot, cg string, alpha int) {
	if bg != "" {
		if img := s.imageOf(bg); img != nil {
			s.drawFull(dst, img)
		}
	}
	for i := 0; i < CharSlots; i++ {
		ch := chars[i]
		if ch.Name == "" {
			continue
		}
		if img := s.imageOf(ch.Name); img != nil {
			s.drawChar(dst, img, ch)
		}
	}
	if cg != "" {
		if img := s.imageOf(cg); img != nil {
			s.drawFull(dst, img)
		}
	}
}

// 与 drawFrame 同形，但整体按 mix(0..255) 混合到 dst 上（转场第二遍用）。
// 特殊构造（snapshot 画布）是为了与 bishop 的 crossFade 逐字节对齐：
// mix=255 时整图直写，mix=0 时不画。
func (s *Stage) drawFrameMixed(dst *bishopgfx.BgraCanvas, bg string, chars [CharSlots]Slot, cg string, mix int) {
	if mix <= 0 {
		return
	}
	tmp := bishopgfx.NewBgraCanvas(Width, Height)
	tmp.Clear(0, 0, 0, 255)
	s.drawFrame(tmp, bg, chars, cg, 1)
	// per-pixel 混合（与 bishop crossFade 同口径的近似：目标像素按 mix 向新图偏）。
	for off := 0; off+4 <= len(dst.Buffer) && off+4 <= len(tmp.Buffer); off += 4 {
		inv := 255 - mix
		dst.Buffer[off] = byte((int(dst.Buffer[off])*inv + int(tmp.Buffer[off])*mix + 127) / 255)
		dst.Buffer[off+1] = byte((int(dst.Buffer[off+1])*inv + int(tmp.Buffer[off+1])*mix + 127) / 255)
		dst.Buffer[off+2] = byte((int(dst.Buffer[off+2])*inv + int(tmp.Buffer[off+2])*mix + 127) / 255)
		dst.Buffer[off+3] = 255
	}
}

// drawFull 把 800x600 图整张贴满（尺寸不等时居中裁剪/留黑——对全屏图无所谓，
// / 对放错尺寸的 CG 则宁可露底也不拉伸；拉伸走 ScaleImage 的显式路径）。
func (s *Stage) drawFull(dst *bishopgfx.BgraCanvas, img *overgfx.Image) {
	if img.Width == Width && img.Height == Height {
		dst.Blit(img, 0, 0, 255)
		return
	}
	// 居中贴入（裁/垫统一走 BlitRegion，不走 BlitScaled，避免背景被拉扁）
	x := (Width - img.Width) / 2
	y := (Height - img.Height) / 2
	dst.Blit(img, x, y, 255)
}

func (s *Stage) drawChar(dst *bishopgfx.BgraCanvas, img *overgfx.Image, ch Slot) {
	// 补间：原 0x33 的层动画是同一层从旧几何滑到新几何（1000ms），
	// 单张图、单次绘制，不会出现两张相同立绘。
	xTarget, yTarget := ch.X, ch.Y
	scale, alpha := ch.Scale, ch.Alpha
	if scale == 0 {
		scale = 1
	}
	if ch.AnimMs > 0 {
		t := float64(ch.AnimElapsed) / float64(ch.AnimMs)
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		xTarget = ch.FromX + int(float64(ch.X-ch.FromX)*t)
		yTarget = ch.FromY + int(float64(ch.Y-ch.FromY)*t)
		fromScale := ch.FromScale
		if fromScale == 0 {
			fromScale = 1
		}
		scale = fromScale + (scale-fromScale)*t
		if ch.FromAlpha != 0 {
			alpha = ch.FromAlpha + int(float64(ch.Alpha-ch.FromAlpha)*t)
		}
	}
	alpha = clampAlpha(alpha, 255)
	if alpha <= 0 {
		return
	}
	dw := int(float64(img.Width) * scale)
	dh := int(float64(img.Height) * scale)
	if dw <= 0 || dh <= 0 {
		return
	}
	// 锚点是中心 X（宏的 400/200/600…）：左右居中，底贴底（Y=0 → 底部对齐）。
	x := xTarget - dw/2
	y := yTarget + (Height - dh)
	draw := img
	if dw != img.Width || dh != img.Height {
		draw = overgfx.ScaleImage(img, dw, dh, s.scaleMode)
		if draw == nil {
			return
		}
	}
	// 大规模缩放的双线性图缓存命中后可直接复用，但立绘原尺寸多不变——
	// 1:1 时 ScaleImage 直接返回原图，不多复制。
	dst.Blit(draw, x, y, alpha)
}

func (s *Stage) imageOf(name string) *overgfx.Image {
	if s.cache == nil {
		return nil
	}
	return s.cache.Get(name)
}

func (s *Stage) applyFade(dst *bishopgfx.BgraCanvas) {
	if s.fade == FadeNone || s.fadeAlpha <= 0 {
		return
	}
	a := clampAlpha(s.fadeAlpha, 255)
	if s.fade == FadeBlack {
		dst.Overlay(0, 0, 0, a)
	} else {
		dst.Overlay(255, 255, 255, a)
	}
}

// Clamp 反向透传给 stage 的测试/合成用（避免 stage 侧重复守）
func clampAlpha(v, def int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	if v == 0 && def != 0 {
		return def // 0 视为未设，取满不透明（与 0x33 的 alpha 默认 255 一致）
	}
	return v
}
