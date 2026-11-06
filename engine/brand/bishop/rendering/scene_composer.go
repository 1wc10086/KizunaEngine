package rendering

import (
	"math"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

const (
	minimumTransitionSeconds = 0.08
	maximumTransitionSeconds = 1.5
	blackLuminance           = 12
	blackCoverage            = 92
)

type SceneComposer struct {
	// hairOverlay 打开后，角色层合成完会在同一位置叠 hair_<立绘名>。
	// 満淫電車2IE 的 GraphicsEV_hair.bsa 用这种命名存放阴毛/腋毛图层；
	// 其它作品没有该素材，开关保持关闭。
	hairOverlay bool
	// effectsOff 对应「画面効果 OFF」/画面効果なし：跳过转场动画，
	// 新场景直接出现。默认 false，其它作品行为不变。
	effectsOff bool
	Images     *ImageCache

	frame      *gfx.BgraCanvas
	history    *gfx.BgraCanvas
	surfaces   map[int]*LayerSurface
	live       map[int]struct{}
	expired    []int
	transition *ScreenTransition
	blink      *BlinkAnimation
	counter    *state.StageCounter
	// information 画左上角的「行動開始から X日目」信息窗；未配置时为 nil。
	information *InformationWindowRenderer
	// informationValues 由 Session 每次 Compose 前写入，渲染器只负责画。
	informationValues InformationWindowValues
	// subDay/subTime 是对话框左侧日期小窗的值；和上面的信息窗分开，
	// 因为学園2 没有独立的角落信息窗，只在对话框里画日期。
	subDay  int
	subTime int
	// message 画原版对话框与底部工具条；未配置时为 nil。
	message *MessageWindowRenderer
	// messageVisible/messageHover 由 Session 每次 Compose 前写入：Session 才知道
	// 当前是不是标题/系统界面/菜单。渲染器只按这两个值决定画不画、哪个按钮悬停。
	messageVisible bool
	// interfaceOpen 由 Session 每帧写入：回想/存读档/设置/鉴赏等系统界面打开时，
	// 常驻工具条会盖在界面上（MKIE 的回想底栏就是被 SKIP/VOICE… 压住）。
	// 系统界面自己负责画需要的按钮，这里只是不再画游戏内工具条。
	interfaceOpen  bool
	messageButtons bool
	messageNotice  string
	messageHover   int
	// counterEnabled 对应原版设置里的「x countdown」开关：关掉时
	// 射精计数器整块不画。HUD 的显示开关归 Session 的设置所有，
	// 渲染器只接收结果，避免把设置结构体带进 rendering 包。
	counterEnabled bool
	// messageSpeed 是原版设置里的文字速度（0…100）。0 表示不逐字显示，
	// 直接整段出现；>0 时按字符推进。revealStart/revealText 记录当前句
	// 开始显示的时刻和文本，换句时重置。
	messageSpeed   int
	revealStart    time.Time
	revealText     string
	revealSpeaker  string
	revealComplete bool
	start          time.Time
	// overlay 是 Session 注入的自绘 HUD（工作/行程画面）。
	// 它必须画在剧情图层之后、按钮之前：底图和角色面板不能盖住
	// START/HELP 等舞台按钮，但又要盖住剧本已画的背景与横幅。
	overlay func(*gfx.BgraCanvas)
	// skipOverlay 是宿主"跳过中"状态：原引擎在这时把 next_select.bsg 的
	// SKIPPING 条贴在屏幕顶端并闪烁。它由 SetOption(1) 从 Flutter 同步。
	skipOverlay bool

	transitionStarted float64
	transitionSerial  int
	historySerial     int
	transitionRunning bool
	transitionChained bool
	historyDark       bool
	frameReady        bool
	drawnRevision     int
	drawnCounter      int
	drawnCounterValue int
	drawnFrame        int
	transitionDrawn   bool
	// 对话框不是按 StageRevision 变化的：文本、说话人、显示开关和按钮悬停
	// 都要触发重绘，否则一句话换了一句、画面却还是上一帧。
	drawnText    string
	drawnSpeaker string
	// drawnReveal 是上次合成时的逐字进度；不比较它，第一次只画 0 个字符
	// 也会被当成"已经画过整句"，后续永远不再重绘（文本不显示/旧句不消失）。
	drawnReveal int
	// 第二消息框的绘制状态；目前只用于调试，主流程暂不写入。
	drawnMultiText      string
	drawnMultiSpeaker   string
	drawnMultiVisible   bool
	drawnMessageVisible bool
	drawnMessageHover   int
	drawnMessageButtons bool
	drawnMessageNotice  string
	drawnViewMode       int
	// drawnSkipOverlay 记录上一帧的跳过覆盖层开关；true 时每帧都要重画以闪烁。
	drawnSkipOverlay bool
	// drawnSelectRevision 是上次绘制的选择肢 revision；选项/悬停变化时必须重画。
	drawnSelectRevision int
}

func NewSceneComposer(resources *resource.GameResourceIndex, width, height int, eyeFrames int, eyeMilliseconds float64, counter *state.StageCounter, information config.InformationWindow, gameMenu config.GameMenu, message config.MessageWindow, fontConfig config.FontSettings) *SceneComposer {
	images := NewImageCache(resources, DefaultImageBudgetBytes, DefaultImageCapacity)
	composer := &SceneComposer{
		Images:              images,
		frame:               gfx.NewBgraCanvas(width, height),
		history:             gfx.NewBgraCanvas(width, height),
		surfaces:            map[int]*LayerSurface{},
		live:                map[int]struct{}{},
		transition:          NewScreenTransition(images, width),
		blink:               NewBlinkAnimation(eyeFrames, eyeMilliseconds),
		counter:             counter,
		counterEnabled:      true,
		start:               time.Now(),
		transitionSerial:    -1,
		historySerial:       -1,
		drawnRevision:       -1,
		drawnCounter:        -1,
		drawnCounterValue:   -1,
		drawnFrame:          -1,
		drawnMessageHover:   -2,
		drawnViewMode:       -1,
		drawnReveal:         -2,
		drawnSelectRevision: -1,
		subDay:              -1,
		subTime:             -1,
	}
	if !information.Empty() {
		composer.information = NewInformationWindowRenderer(images, information)
	}
	if !message.Empty() {
		composer.message = NewMessageWindowRenderer(images, message, fontConfig, gameMenu)
	}
	return composer
}

// SetMessageWindowState 更新对话框的显示状态与悬停按钮。
//
// Session 决定什么时候能显示（标题/存档/菜单/影片上不显示），渲染器只负责画。
// 状态变化必须让 needsRedraw 看到，否则引擎停在 AwaitInput 时不会主动重绘。
func (c *SceneComposer) SetMessageWindowState(visible bool, hover int, buttons bool, notice string) {
	if c.messageVisible == visible && c.messageHover == hover && c.messageButtons == buttons && c.messageNotice == notice {
		return
	}
	c.messageVisible = visible
	c.messageHover = hover
	c.messageButtons = buttons
	c.messageNotice = notice
}

// SetInformationWindow 更新信息窗要显示的值（日期 / 时段）。
//
// 值由 Session 从脚本变量里读出来传进来：渲染器不该知道 #day 是哪个属性槽，
// 那样每换一部作品就要改渲染代码。
func (c *SceneComposer) SetInformationWindow(values InformationWindowValues) {
	if c.informationValues == values {
		return
	}
	c.informationValues = values
	// 信息窗变了要重画，否则 drawnRevision 没变会直接跳过合成。
	c.Invalidate()
}

// SetMessageSubWindow 更新对话框左侧日期小窗的日期/时段。
//
// 值变化时让缓存失效，否则对话框缓存键仍是上一帧的日期，画面不会更新。
func (c *SceneComposer) SetMessageSubWindow(day, time int) {
	if c.subDay == day && c.subTime == time {
		return
	}
	c.subDay = day
	c.subTime = time
	c.Invalidate()
}

// SetCounterEnabled 打开/关闭射精计数器的 HUD 显示。
//
// 原版设置里的「x countdown」只是显示开关，不改变脚本里的计数变量；
// 关掉后数值照常推进，只是不画出来。
// SetMessageSpeed 设置文字速度（0…100）。0 表示整段立即显示。
func (c *SceneComposer) SetMessageSpeed(speed int) {
	speed = core.Clamp(speed, 0, 100)
	if c.messageSpeed == speed {
		return
	}
	c.messageSpeed = speed
	// 速度变了就从现在重新计时，避免拖滑杆时文字瞬间跳到末尾。
	c.revealStart = time.Now()
	c.revealComplete = speed == 0
	c.Invalidate()
}

// SetMessageWindowOffset 设置学園2 对话框三档位置的垂直位移。
func (c *SceneComposer) SetMessageWindowOffset(offset int) {
	if c.message == nil {
		return
	}
	c.message.SetWindowOffset(offset)
	c.Invalidate()
}

// MessageAnimating 表示当前这句已经进入逐字显示、但还没显示完。
//
// 只能用于"这一句已经开始画过至少一帧"的判断；新消息在第一次 Compose
// 之前 revealText 还是上一句，这个函数会返回 false。宿主据此判断是否
// 继续合成时必须改用 MessageRevealPending(text, speaker)。
func (c *SceneComposer) MessageAnimating() bool {
	return c.message != nil && c.messageVisible && !c.revealComplete && c.revealText != ""
}

// MessageRevealPending 表示当前句还没开始逐字显示，或还没显示完。
//
// 关键点：新消息由 MessageDirector 写入 GameState 后，宿主会立刻同步一次
// State。此时第一次 Compose 还没跑，revealText 仍是上一句，MessageAnimating
// 返回 false；宿主只合成一帧就停下，这一帧的 reveal 是 0 个字符，于是正文
// 不显示。玩家点下一句时 Advance 才发现 MessageAnimating 为真，补完整句，
// 正文只闪现一帧就又被下一句清掉——正是"点击后才出现一帧文本"的现象。
// 所以 State.Animating 和点击判定都必须用这个函数，把"还没开始"也算作待动画。
func (c *SceneComposer) MessageRevealPending(text, speaker string) bool {
	if c == nil || c.message == nil || !c.messageVisible || len(text) == 0 || c.messageSpeed <= 0 {
		return false
	}
	return !c.revealComplete || c.revealText != text || c.revealSpeaker != speaker
}

// CompleteMessageReveal 把当前句立即显示完（玩家点击时先补完整句）。
func (c *SceneComposer) CompleteMessageReveal() {
	if c.revealComplete {
		return
	}
	c.revealComplete = true
	// 把起点推到很久以前，下一次 Compose 的 elapsed 一定超过整句长度。
	c.revealStart = time.Now().Add(-time.Hour)
	c.Invalidate()
}

// messageReveal 计算当前句该显示到第几个字符；返回 -1 表示整段。
func (c *SceneComposer) messageReveal(text, speaker string) int {
	if c.message == nil || !c.messageVisible || len(text) == 0 || c.messageSpeed <= 0 {
		c.revealComplete = true
		return -1
	}
	if c.revealText != text || c.revealSpeaker != speaker {
		c.revealText = text
		c.revealSpeaker = speaker
		c.revealStart = time.Now()
		c.revealComplete = false
	}
	total := len([]rune(text))
	// 5…60 字符/秒：速度 0 已经走"立即显示"分支，50 时约 32 字/秒，
	// 一句 25 字大约 0.8 秒，接近原版默认节奏。
	charsPerSecond := 5 + float64(c.messageSpeed)*0.55
	shown := int(time.Since(c.revealStart).Seconds() * charsPerSecond)
	if shown >= total {
		c.revealComplete = true
		return -1
	}
	if shown < 0 {
		shown = 0
	}
	return shown
}

// SetOverlay 注册一张"剧情层之上、舞台按钮之下"的自绘 HUD。
//
// 工作/行程画面（type=176 自定义窗口和 SL 的 [custom map phase]）都由
// Session 持有数据，渲染器只负责在正确的层序调用它；把布局/条件判断留在
// Session 可以避免 rendering 包依赖 vm 和脚本属性表。
// SetSkipOverlay 切换"SKIPPING"覆盖层。只在宿主跳过时显示，和原引擎一致。
func (c *SceneComposer) SetSkipOverlay(value bool) {
	if c.skipOverlay == value {
		return
	}
	c.skipOverlay = value
	c.Invalidate()
}

// SetEffectsEnabled 控制转场/画面效果是否播放。
func (c *SceneComposer) SetEffectsEnabled(enabled bool) {
	off := !enabled
	if c.effectsOff == off {
		return
	}
	c.effectsOff = off
	c.Invalidate()
}

// SetHairOverlay 打开/关闭 hair_<立绘名> 叠加层。
func (c *SceneComposer) SetHairOverlay(enabled bool) {
	if c.hairOverlay == enabled {
		return
	}
	c.hairOverlay = enabled
	c.Invalidate()
}

func (c *SceneComposer) SetOverlay(draw func(*gfx.BgraCanvas)) {
	c.overlay = draw
}

// SetCounterEnabled 打开/关闭射精计数器的 HUD 显示。
func (c *SceneComposer) SetCounterEnabled(enabled bool) {
	if c.counterEnabled == enabled {
		return
	}
	c.counterEnabled = enabled
	c.Invalidate()
}

// SetMessageBackground 覆盖对话框底色的 ARGB（0xAARRGGBB）。
//
// Session 把设置画面里调好的 RGBA 传进来；渲染器只接收最终颜色，
// 不读设置结构体，保持 rendering 包不依赖 session。
func (c *SceneComposer) SetMessageBackground(argb uint32) {
	if c.message == nil {
		return
	}
	if c.message.SetBackgroundColor(argb) {
		c.Invalidate()
	}
}

func (c *SceneComposer) Frame() *gfx.BgraCanvas {
	return c.frame
}

func (c *SceneComposer) Width() int {
	return c.frame.Width
}

func (c *SceneComposer) Height() int {
	return c.frame.Height
}

func (c *SceneComposer) Buffer() []byte {
	return c.frame.Buffer
}

func (c *SceneComposer) TransitionRunning() bool {
	return c.transitionRunning
}

func (c *SceneComposer) Load(name string) *gfx.BsgImage {
	return c.Images.Get(name)
}

func (c *SceneComposer) Compose(gameState *state.GameState) bool {
	c.blink.Advance(time.Since(c.start).Seconds() * 1000.0)
	c.advanceTransition(gameState)
	// 逐字进度要在 needsRedraw 之前算：否则第一次合成画 0 个字符、
	// 却把整句记成"已绘制"，后面的 tick 直接跳过，文本永远不出现。
	reveal := c.messageReveal(gameState.Text(), gameState.Speaker())
	if !c.needsRedraw(gameState, reveal) {
		return false
	}
	c.drawnRevision = gameState.StageRevision()
	c.drawnFrame = c.blink.Frame()
	c.drawnCounter = gameState.CounterValue()
	c.beginTransition(gameState)
	c.frame.Clear(0, 0, 0, 255)
	running := c.transitionRunning
	drawn := c.drawLayers(gameState)
	c.applyFill(gameState)
	if c.overlay != nil {
		c.overlay(c.frame)
	}
	if c.skipOverlay {
		c.drawSkipOverlay()
	}
	c.drawButtons(gameState)
	c.frameReady = c.frameReady || drawn
	if running {
		c.transition.Apply(c.frame, c.history, gameState.TransitionType(), c.transitionProgress(gameState), !c.historyDark)
	}
	// 对话框是 HUD：底图、名字、正文、催促图标和底部工具按钮都画在场景之上。
	// 正文按 message speed 逐字显示；reveal < 0 表示整段立即出现。
	if c.message != nil {
		c.message.SetSubWindowValues(c.subDay, c.subTime)
		c.message.DrawRevealMulti(c.frame, gameState.Text(), gameState.Speaker(), c.messageVisible, c.messageButtons, c.messageHover, c.messageNotice, reveal, gameState.MultiText(), gameState.MultiSpeaker(), gameState.MultiMessageVisible())
		// 常驻工具条（学園2 顶部菜单）在对话框收起时仍然显示；对话框可见时
		// 由 MessageWindowRenderer 的按钮层统一画，避免同一排按钮画两遍。
		if !c.messageVisible && !c.interfaceOpen && gameState.MenuBarVisible() {
			if c.message.DrawMenuBar(c.frame, c.messageHover) {
				drawn = true
			}
		}
	}
	// 信息窗画在对话框之后：KD 把「第3週 日曜日」放在对话框底栏左侧，
	// 先画会被半透明的 message 底图压暗。其它作品的信息窗在屏幕角落，
	// 顺序不影响。
	c.information.Draw(c.frame, c.informationValues)
	// 选择肢是 HUD：画在对话框之上（脚本在选择时通常已经把消息窗收掉）。
	if c.drawSelectItems(gameState) {
		drawn = true
	}
	// 射精计数器在对话框之后画：它的坐标（9,681）落在对话框底图内部，
	// 先画会被半透明的 msg_base 压暗，玩家几乎看不清。
	c.drawCounter(gameState)
	c.drawnText = gameState.Text()
	c.drawnSpeaker = gameState.Speaker()
	c.drawnReveal = reveal
	c.drawnMultiText = gameState.MultiText()
	c.drawnMultiSpeaker = gameState.MultiSpeaker()
	c.drawnMultiVisible = gameState.MultiMessageVisible()
	c.drawnMessageVisible = c.messageVisible
	c.drawnMessageHover = c.messageHover
	c.drawnMessageButtons = c.messageButtons
	c.drawnMessageNotice = c.messageNotice
	c.drawnViewMode = gameState.ViewMode()
	c.drawnSkipOverlay = c.skipOverlay
	c.drawnSelectRevision = gameState.SelectRevision()
	c.transitionDrawn = running
	return true
}

func (c *SceneComposer) Invalidate() {
	c.drawnRevision = -1
}

// SetInterfaceOpen 告诉合成器当前是否打开了系统界面。
//
// 工具条是游戏 HUD，系统界面打开时必须让位；否则回想/存读档的底栏会
// 和 SKIP/VOICE/AUTO 那一排叠在一起。状态变化时让整帧重画一次。
func (c *SceneComposer) SetInterfaceOpen(open bool) {
	if c.interfaceOpen == open {
		return
	}
	c.interfaceOpen = open
	c.Invalidate()
}

// ReloadImages 强制所有舞台层按新的图片缓存重新合成。
//
// Invalidate 只让整帧重画，LayerSurface 仍按 layer.Revision 命中旧缓存；
// 学園2 的 DRESS 切换换了 clNNNNN 的素材后缀，必须把 surface 一起丢掉，
// 否则玩家按 DRESS 后角色还是旧图。
func (c *SceneComposer) ReloadImages() {
	c.surfaces = map[int]*LayerSurface{}
	c.live = map[int]struct{}{}
	c.expired = nil
	c.Invalidate()
}

func (c *SceneComposer) DrawImage(image *gfx.BsgImage, x, y int) {
	c.Invalidate()
	if image.Width <= 0 || image.Height <= 0 {
		return
	}
	if image.Width <= c.frame.Width && image.Height <= c.frame.Height {
		c.frame.Blit(image, x, y, 255)
		return
	}
	scale := math.Min(float64(c.frame.Width)/float64(image.Width), float64(c.frame.Height)/float64(image.Height))
	width := max(1, int(math.RoundToEven(float64(image.Width)*scale)))
	height := max(1, int(math.RoundToEven(float64(image.Height)*scale)))
	c.frame.BlitScaled(image, (c.frame.Width-width)/2, (c.frame.Height-height)/2, width, height, 255)
}

func (c *SceneComposer) advanceTransition(gameState *state.GameState) {
	if !gameState.TransitionActive() {
		c.transitionRunning = false
		return
	}
	if gameState.TransitionSerial() != c.transitionSerial {
		c.transitionSerial = gameState.TransitionSerial()
		c.transitionChained = c.transitionRunning
		if !c.transitionRunning {
			c.transitionRunning = true
			c.transitionStarted = time.Since(c.start).Seconds()
		}
	} else {
		c.transitionRunning = true
	}
	if !c.frameReady || c.transitionProgress(gameState) >= 1 {
		c.transitionRunning = false
		gameState.EndTransition()
	}
}

func (c *SceneComposer) transitionProgress(gameState *state.GameState) float64 {
	duration := clampFloat(gameState.TransitionDuration(), minimumTransitionSeconds, maximumTransitionSeconds)
	return clampFloat((time.Since(c.start).Seconds()-c.transitionStarted)/duration, 0, 1)
}

func (c *SceneComposer) needsRedraw(gameState *state.GameState, reveal int) bool {
	return c.transitionDrawn ||
		c.transitionRunning ||
		c.drawnRevision != gameState.StageRevision() ||
		c.drawnFrame != c.blink.Frame() ||
		c.drawnCounter != gameState.CounterValue() ||
		gameState.FillAlpha() > 0 ||
		c.drawnText != gameState.Text() ||
		c.drawnSpeaker != gameState.Speaker() ||
		c.drawnReveal != reveal ||
		c.drawnMultiText != gameState.MultiText() ||
		c.drawnMultiSpeaker != gameState.MultiSpeaker() ||
		c.drawnMultiVisible != gameState.MultiMessageVisible() ||
		c.drawnMessageVisible != c.messageVisible ||
		c.drawnMessageHover != c.messageHover ||
		c.drawnMessageButtons != c.messageButtons ||
		c.drawnMessageNotice != c.messageNotice ||
		c.drawnViewMode != gameState.ViewMode() ||
		c.drawnSkipOverlay != c.skipOverlay ||
		// 覆盖层显示期间按 250ms 闪烁，必须持续重画。
		c.skipOverlay ||
		c.drawnSelectRevision != gameState.SelectRevision() ||
		// 逐帧动画每帧内容都不同，不能靠 revision 判断。
		gameState.HasAnimators() ||
		// 关键帧动画改变的是位移/透明度，同样需要按墙钟重画。
		gameState.HasLayerMotions()
}

// drawSkipOverlay 画 tbxdl 的 SKIPPING 条。
//
// next_select.bsg 是 800x96 的两帧竖排图（每帧 800x48），原引擎在跳过时
// 贴在 (0,0) 并闪烁。没有该素材的作品不会进入这段。
func (c *SceneComposer) drawSkipOverlay() {
	image := c.Images.Get("next_select")
	if image == nil || image.Height <= 0 {
		return
	}
	frames := max(1, image.FrameCount)
	if frames <= 1 {
		frames = 2
	}
	height := image.Height / frames
	if height <= 0 {
		return
	}
	frame := int(time.Since(c.start).Milliseconds()/250) % frames
	c.frame.DrawRegion(image, 0, frame*height, image.Width, height, 0, 0)
}

func (c *SceneComposer) beginTransition(gameState *state.GameState) {
	if gameState.TransitionSerial() == c.historySerial {
		return
	}
	c.historySerial = gameState.TransitionSerial()
	if c.effectsOff {
		// 画面效果关闭：直接把当前帧当成历史，下一帧不再做 wipe。
		c.transitionRunning = false
		c.history.CopyFrom(c.frame)
		c.historyDark = isMostlyBlack(c.history)
		return
	}
	if !c.transitionChained {
		c.history.CopyFrom(c.frame)
		c.historyDark = isMostlyBlack(c.history)
	}
	if name, ok := gameState.MaskImage(); ok && len(name) > 0 {
		c.transition.SetMask(name)
	}
}

func (c *SceneComposer) drawLayers(gameState *state.GameState) bool {
	clear(c.live)
	drawn := c.drawLayerRange(gameState, 0, state.OverlayPlane)
	if c.drawOverlays(gameState) {
		drawn = true
	}
	if c.drawLayerRange(gameState, state.OverlayPlane, len(gameState.Layers())) {
		drawn = true
	}
	if len(c.surfaces) > len(c.live) {
		c.releaseSurfaces()
	}
	return drawn
}

func (c *SceneComposer) drawLayerRange(gameState *state.GameState, start, end int) bool {
	drawn := false
	layers := gameState.Layers()
	for index := start; index < end; index++ {
		layer := layers[index]
		// 对话框底图有时也是剧本里的普通图层（MK 把 `message` 贴在 (0,496)）。
		// 对话框不可见时（地图/选择态）必须跳过它，否则地图左下角会露出
		// 对话框的左边框；可见时仍按原样画，保持对话框的现有观感。
		if c.message != nil && !c.messageVisible {
			if base := c.message.BaseImage(); len(base) > 0 && layer.Name() == base {
				continue
			}
		}
		// 关键帧动画（LAYER_PARAM type=6）只改变图层的位置/透明度，
		// 图本身仍由 LayerSurface 正常合成；先处理它，避免被逐帧动画分支抢走。
		if motion := gameState.LayerMotion(index); motion != nil {
			if !layer.IsDrawable() || !layer.Visible() {
				continue
			}
			dx, dy, opacity, ok := motion.Transform(time.Now())
			baseAlpha := resolveAlpha(layer.Alpha())
			alpha := int(float64(baseAlpha)*opacity + 0.5)
			if !ok || alpha <= 0 {
				continue
			}
			c.live[layer.Id()] = struct{}{}
			surface, exists := c.surfaces[layer.Id()]
			if !exists {
				surface = NewLayerSurface(c.frame.Width, c.frame.Height)
				c.surfaces[layer.Id()] = surface
			}
			surface.Refresh(layer, c.Images, c.blink.Frame(), gameState.ViewMode())
			c.frame.BlitCanvasRegion(surface.Canvas(), 0, 0, c.frame.Width, c.frame.Height, dx, dy, alpha)
			drawn = true
			continue
		}
		// 逐帧动画优先于静态图：モーションプラス 的分层 CG 在 Image 之后
		// 用 Attach 挂一个 *_anim 容器，这里按墙钟取当前帧整屏绘制。
		if animator := gameState.LayerAnimator(index); animator != nil {
			alpha := resolveAlpha(layer.Alpha())
			if !layer.Visible() || alpha <= 0 {
				continue
			}
			if image := animator.Frame(time.Now()); image != nil {
				c.frame.Blit(image, 0, 0, alpha)
				drawn = true
			}
			continue
		}
		if !layer.IsDrawable() {
			continue
		}
		alpha := resolveAlpha(layer.Alpha())
		if alpha <= 0 {
			continue
		}
		c.live[layer.Id()] = struct{}{}
		surface, ok := c.surfaces[layer.Id()]
		if !ok {
			surface = NewLayerSurface(c.frame.Width, c.frame.Height)
			c.surfaces[layer.Id()] = surface
		}
		surface.Refresh(layer, c.Images, c.blink.Frame(), gameState.ViewMode())
		c.drawHairOverlay(surface, layer)
		c.frame.BlitCanvas(surface.Canvas(), alpha)
		drawn = true
	}
	return drawn
}

func (c *SceneComposer) drawOverlays(gameState *state.GameState) bool {
	drawn := false
	for _, overlay := range gameState.Overlays() {
		image := c.Images.Get(overlay.Image)
		if image == nil || image.Width <= 0 || image.Height <= 0 {
			continue
		}
		x, y := c.overlayOrigin(overlay, image)
		c.frame.Draw(image, x, y)
		drawn = true
	}
	return drawn
}

func (c *SceneComposer) overlayOrigin(overlay state.StageOverlay, image *gfx.BsgImage) (int, int) {
	switch overlay.Anchor {
	case state.StageOverlayBottomBand:
		return max(0, overlay.X), max(0, c.frame.Height-image.Height-overlay.Y)
	case state.StageOverlayTopRight:
		return max(0, c.frame.Width-image.Width-overlay.X), max(0, overlay.Y)
	default:
		return max(0, overlay.X), max(0, overlay.Y)
	}
}

func (c *SceneComposer) releaseSurfaces() {
	c.expired = c.expired[:0]
	for id := range c.surfaces {
		if _, ok := c.live[id]; !ok {
			c.expired = append(c.expired, id)
		}
	}
	for _, id := range c.expired {
		delete(c.surfaces, id)
	}
	c.expired = c.expired[:0]
}

func (c *SceneComposer) drawButtons(gameState *state.GameState) {
	buttons := gameState.Buttons()
	for index := range buttons {
		button := buttons[index]
		if len(button.Image) == 0 {
			continue
		}
		frames := max(1, button.Frames)
		image := c.Images.Get(button.Image)
		if image == nil {
			continue
		}
		if !button.Atlas && image.Height < frames {
			continue
		}
		frameHeight := image.Height / frames
		hovered := index == gameState.HoveredButton()
		if button.HoverOnly && !hovered {
			continue
		}
		frame := 0
		// 目标卡的帧序是"黑 / 灰 / 彩色"：常态画第 1 帧（灰），悬停
		// 画第 2 帧（彩色）。旧逻辑默认第 0 帧，目标选择画面会是一排
		// 黑块。target_ok/target_cancel 也是同样的 3 帧约定。
		if frames >= 3 && strings.HasPrefix(strings.ToLower(button.Image), "target") {
			frame = 1
			if hovered {
				frame = 2
			}
		} else if button.HoverOnly {
			frame = core.Clamp(button.Frame, 0, frames-1)
		} else if button.Value >= 900 && button.Value < 1000 && frames == 2 {
			// mros 的时间推进按钮（map_skip1..4 = 901..904）两帧语义与
			// 普通地图卡相反：第 1 帧是常态（红色箭头），第 0 帧是悬停/
			// 按下态（光标括号）。按第 0 帧画会让右侧“時間を潰す”一排
			// 没有箭头，和原版地图不一致。900 段是这一族按钮的保留值，
			// 其它作品的 7000/8000 段不受影响。
			if hovered {
				frame = 0
			} else {
				frame = 1
			}
		} else if button.Frame >= 0 {
			// 固定帧按钮：学園2 的 title_menu 把 8 个菜单项竖排在同一张图里，
			// 每一项固定画自己那一帧；悬停时不能切到 frame 1，否则所有按钮
			// 会一起变成「始めから」。
			frame = core.Clamp(button.Frame, 0, frames-1)
		} else if button.Disabled && frames >= 4 {
			// 工作画面的按钮图是 4 帧：常态/悬停/选中/不可用。
			// START 在四人未选齐工作时必须显示灰态，玩家才知道不能点。
			frame = 3
		} else if !button.Disabled && hovered && frames > 1 {
			frame = 1
			if button.HasHoverFrame {
				frame = core.Clamp(button.HoverFrame, 0, frames-1)
			}
		}
		offsetX, offsetY := image.OffsetX, image.OffsetY
		if button.IgnoreImageOffset {
			// 标题菜单：X/Y 来自配置/热区，图片头里的坐标只是素材原位，
			// 再叠加一次会把按钮移到屏幕外。
			offsetX, offsetY = 0, 0
		}
		if button.Atlas {
			// 精灵图按钮：Clip 是单帧矩形，帧沿配置方向排列；悬停态可能
			// 在图上另一块，由 HoverClip 指定。
			srcX, srcY, width, height := button.Clip[0], button.Clip[1], button.Clip[2], button.Clip[3]
			if frame > 0 && button.HoverClip[2] > 0 && button.HoverClip[3] > 0 {
				srcX, srcY = button.HoverClip[0], button.HoverClip[1]
				width, height = button.HoverClip[2], button.HoverClip[3]
			} else if button.AtlasAxis == 1 {
				srcX += frame * width
			} else {
				srcY += frame * height
			}
			if width <= 0 || height <= 0 ||
				srcX < 0 || srcY < 0 || srcX+width > image.Width || srcY+height > image.Height {
				continue
			}
			c.frame.DrawRegion(image, srcX, srcY, width, height,
				button.X+offsetX, button.Y+offsetY)
			continue
		}
		c.frame.DrawRegion(
			image,
			0,
			frame*frameHeight,
			image.Width,
			frameHeight,
			button.X+offsetX,
			button.Y+offsetY)
	}
}

func (c *SceneComposer) drawCounter(gameState *state.GameState) {
	if c.counter == nil || gameState.CounterValue() < 0 {
		return
	}
	image := c.Images.Get(c.counter.Image)
	if image == nil {
		return
	}
	frames := max(1, c.counter.Frames)
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return
	}
	value := core.Clamp(gameState.CounterValue(), 0, frames-1)
	if value != c.drawnCounterValue {
		c.drawnCounterValue = value
		core.Logf("hud: counter %d", value)
	}
	c.frame.DrawRegion(
		image,
		0,
		value*frameHeight,
		image.Width,
		frameHeight,
		c.counter.X+image.OffsetX,
		c.counter.Y+image.OffsetY)
}

func (c *SceneComposer) applyFill(gameState *state.GameState) {
	alpha := int(math.RoundToEven(clampFloat(gameState.FillAlpha(), 0, 1) * 255))
	if alpha <= 0 {
		return
	}
	c.frame.Overlay(gameState.FillBlue(), gameState.FillGreen(), gameState.FillRed(), alpha)
}

func isMostlyBlack(canvas *gfx.BgraCanvas) bool {
	pixels := canvas.Buffer
	total := 0
	dark := 0
	for offset := 0; offset < len(pixels); offset += 64 {
		luminance := int(pixels[offset])*29 + int(pixels[offset+1])*150 + int(pixels[offset+2])*77
		total++
		if luminance>>8 < blackLuminance {
			dark++
		}
	}
	return total > 0 && dark*100/total >= blackCoverage
}

func resolveAlpha(value float64) int {
	if value >= 1 {
		return 255
	}
	return int(math.RoundToEven(clampFloat(value, 0, 1) * 255))
}

// drawHairOverlay 把 hair_<立绘名> 叠在角色层的合成画布上。
//
// 只处理角色层：背景/前景的 hair_* 命名可能只是普通素材。找不到对应
// 图层时静默跳过，HairMode 关闭的作品完全不受影响。
func (c *SceneComposer) drawHairOverlay(surface *LayerSurface, layer *state.StageLayer) {
	if !c.hairOverlay || surface == nil || layer == nil || layer.Kind() != state.StageLayerCharacter {
		return
	}
	for _, artwork := range layer.Artworks() {
		if len(artwork.Image) < 2 || artwork.Image[0] != 'b' || artwork.Image[1] != 'u' {
			continue
		}
		hair := c.Images.Get("hair_" + artwork.Image)
		if hair == nil {
			continue
		}
		surface.Canvas().Draw(hair, artwork.X+hair.OffsetX, artwork.Y+hair.OffsetY)
	}
}
