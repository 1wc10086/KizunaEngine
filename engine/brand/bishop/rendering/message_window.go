package rendering

import (
	"image"
	"strconv"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

const (
	// defaultLineGap 是配置没有给行高时的经验值：字号 17 时原版行距是 26。
	defaultLineGap = 9
	// messageTextColor 是深色底上的原版正文颜色：不透明白。
	// 浅色对话框（SL/SLIE 的 bg color = 0xc0ffffff）改由 textColor 选黑色，
	// 否则白字白底完全看不见。
	messageTextColor = 0xffffffff
)

// MessageWindowRenderer 画原版的对话框：底图、名字、正文、催促图标和底部菜单条。
//
// 底图和文字会被合成为一张与画布同尺寸的图层并缓存；对话文本在一句话显示期间
// 不变，逐帧重新排版整段日文（还要做描边）会明显拖慢自动播放。菜单按钮的悬停
// 状态和催促图标是逐帧变化的，所以在缓存图层上每次重画。
type MessageWindowRenderer struct {
	images *ImageCache
	layout config.MessageWindow

	fontConfig config.FontSettings
	gameMenu   config.GameMenu
	font       *textrender.Renderer

	cached     *gfx.BgraCanvas
	cacheKey   messageWindowCacheKey
	cacheValid bool
	// bgOverride 是设置画面里调出来的对话框底色（0xAARRGGBB）。
	// bgOverrideSet 区分"没调过，沿用 bsxx.cfg 的 bg color"和
	// "玩家把它调成了纯黑透明"这两种情况。
	bgOverride    uint32
	bgOverrideSet bool
	// subDay/subTime 是左侧日期小窗要显示的值，由 SceneComposer 每帧写入。
	// -1 表示不画；日期的周数/星期由渲染器换算，不需要 Session 传入字符串。
	subDay  int
	subTime int
	// windowOffset 是学園2「▲▼」按钮切换三档窗口位置时的垂直位移。
	// 只作用于对话框底图/文字/小窗/催促图标和对话框内按钮，顶部常驻工具条不动。
	windowOffset int
}

// BaseImage 返回对话框底图名。
//
// 剧本有时会把同一个底图当普通舞台图层贴出来（MK 的 `message` 层），
// 合成器据此判断"这一层就是对话框"：对话框不可见（地图/选择态）时跳过它，
// 否则地图左下角会留下一条对话框边框。
func (r *MessageWindowRenderer) BaseImage() string {
	if r == nil {
		return ""
	}
	return r.layout.Base
}

// messageWindowCacheKey 覆盖静态图层的全部输入。用结构体而不是拼字符串，
// 避免文本里恰好含分隔符时把两句不同的台词当成同一张缓存。
type messageWindowCacheKey struct {
	text         string
	speaker      string
	visible      bool
	showButtons  bool
	width        int
	height       int
	pixelSize    int
	bgColor      uint32
	multiText    string
	multiSpeaker string
	multiVisible bool
	subDay       int
	subTime      int
	windowOffset int
}

func NewMessageWindowRenderer(images *ImageCache, layout config.MessageWindow, fontConfig config.FontSettings, gameMenu config.GameMenu) *MessageWindowRenderer {
	font, err := textrender.New()
	if err != nil {
		// 字体解析失败时仍然返回渲染器：底图和按钮还能画，不至于整帧黑掉。
		core.Logf("message window: text renderer unavailable: %v", err)
	}

	return &MessageWindowRenderer{
		images:     images,
		layout:     layout,
		fontConfig: fontConfig,
		gameMenu:   gameMenu,
		font:       font,
	}
}

// SetWindowOffset 设置对话框三档位置的垂直位移（0 为底部原位）。
func (r *MessageWindowRenderer) SetWindowOffset(offset int) {
	if r == nil {
		return
	}
	r.windowOffset = offset
}

// Draw 把对话框叠到已经画好场景的画布上。
//
// 返回是否画了内容；没有可见窗口或没有台词时调用方可以不做任何额外处理。
func (r *MessageWindowRenderer) Draw(canvas *gfx.BgraCanvas, text, speaker string, visible, showButtons bool, hoverButton int, notice string) bool {
	return r.DrawReveal(canvas, text, speaker, visible, showButtons, hoverButton, notice, -1)
}

// DrawReveal 是带"逐字显示"进度的 Draw。
//
// reveal < 0 表示整段立即显示（回想/菜单预览等场景）；否则只画前 reveal 个
// 字符。原版的消息速度是按字符推进的，这里只负责截断，时间进度由
// SceneComposer 根据设置里的 message speed 计算。
func (r *MessageWindowRenderer) DrawReveal(canvas *gfx.BgraCanvas, text, speaker string, visible, showButtons bool, hoverButton int, notice string, reveal int) bool {
	return r.DrawRevealMulti(canvas, text, speaker, visible, showButtons, hoverButton, notice, reveal, "", "", false)
}

// DrawRevealMulti 在 DrawReveal 的基础上多画原版第二个消息框（multi messages）。
func (r *MessageWindowRenderer) DrawRevealMulti(canvas *gfx.BgraCanvas, text, speaker string, visible, showButtons bool, hoverButton int, notice string, reveal int, multiText, multiSpeaker string, multiVisible bool) bool {
	if r == nil || canvas == nil || !visible || (len(text) == 0 && len(multiText) == 0) {
		return false
	}
	// reveal 只截断正文；对话框底图和角色名要立刻出现，
	// 否则玩家会看到一瞬间的空白屏幕。complete 用来决定是否画催促图标。
	complete := true
	if reveal >= 0 {
		runes := []rune(text)
		complete = reveal >= len(runes)
		if reveal < len(runes) {
			text = string(runes[:max(0, reveal)])
		}
	}

	key := messageWindowCacheKey{
		text:         text,
		speaker:      speaker,
		visible:      visible,
		showButtons:  showButtons,
		width:        canvas.Width,
		height:       canvas.Height,
		pixelSize:    r.fontConfig.Size,
		bgColor:      r.effectiveBackgroundColor(),
		multiText:    multiText,
		multiSpeaker: multiSpeaker,
		multiVisible: multiVisible,
		subDay:       r.subDay,
		subTime:      r.subTime,
		windowOffset: r.windowOffset,
	}
	if !r.cacheValid || r.cacheKey != key {
		r.cached = r.compose(canvas.Width, canvas.Height, text, speaker, showButtons, multiText, multiSpeaker, multiVisible)
		r.cacheKey = key
		r.cacheValid = true
	}

	if r.cached != nil {
		// 缓存图层在窗口外是全透明的，BlitCanvas 会跳过这些像素，
		// 所以场景和立绘不会被整块清掉。
		canvas.BlitCanvas(r.cached, 255)
	}
	if complete {
		r.drawPrompt(canvas)
	}
	if showButtons {
		r.drawButtons(canvas, hoverButton)
	}
	r.drawNotice(canvas, notice)
	return true
}

func (r *MessageWindowRenderer) compose(width, height int, text, speaker string, showButtons bool, multiText, multiSpeaker string, multiVisible bool) *gfx.BgraCanvas {
	layer := gfx.NewBgraCanvas(width, height)

	// 菜单底图默认铺在对话框底下；Overlay 时（Ancient 的 msgwnd_b）盖在对话框之上，
	// 所以顺序要反过来。隐藏工具条时底图都不画。
	if showButtons && !r.gameMenu.Overlay {
		// 満淫電車2 的 gamemenu_base 只有白色描边（像素本身是透明的），
		// 原版在它下面垫了一条紫色渐变。其它作品（侵蝕/淫獄 等）的
		// gamemenu_base 自带底色，不能垫，否则会在正文上方多出一条紫条。
		if r.gameMenu.BaseFill {
			layer.FillBlend(r.gameMenu.BaseX, r.gameMenu.BaseY, 800, 44, 65, 28, 125, 225)
		}
		r.drawImage(layer, r.gameMenu.Base, r.gameMenu.BaseX, r.gameMenu.BaseY)
	}
	r.drawMessageBase(layer)
	if showButtons && r.gameMenu.Overlay {
		r.drawImage(layer, r.gameMenu.Base, r.gameMenu.BaseX, r.gameMenu.BaseY)
	}

	lineHeight := r.lineHeight()
	originX, originY := r.baseOrigin(height)
	// 名牌底图先贴，名字文字随后画在上面。只有确实有名字时才画，
	// 旁白（speaker 为空）不该留一块空白名牌。
	if len(speaker) > 0 && len(r.layout.NameBase) > 0 {
		r.drawImage(layer, r.layout.NameBase,
			originX+r.layout.NameBaseX, originY+r.layout.NameBaseY)
	}
	// 日期小窗画在正文之前：它和名字/正文不重叠，先画可以让正文盖住
	// 小窗右边框的 1px 抗锯齿边缘，避免文字压到框线上。
	r.drawSubWindow(layer, originX, originY)
	if len(speaker) > 0 && r.layout.SpeakerInline {
		// 名字和正文同一行：先量出名字位图宽度，再把正文整体右移。
		// 后续换行也落在正文起点，和 GK 手册截图一致。
		name := speaker
		if r.layout.SpeakerBrackets {
			name = "【" + name + "】"
		}
		nameImage := r.renderName(name, lineHeight)
		nameWidth := 0
		if nameImage != nil {
			nameWidth = nameImage.Bounds().Dx()
			// RenderOutline 左右各留了 edge 的描边余量；按实际墨水宽度缩进，
			// 否则名字和正文之间会多出 2~4px 的空隙。
			nameWidth -= max(0, r.fontConfig.Edge) * 2
			blitRGBA(layer, nameImage,
				originX+r.layout.NameX,
				originY+r.layout.NameY)
		}
		if len(text) > 0 {
			width := r.layout.TextW - nameWidth
			if width < 40 {
				width = r.layout.TextW
				nameWidth = 0
			}
			r.drawText(layer, text,
				originX+r.layout.TextX+nameWidth,
				originY+r.layout.TextY,
				width, r.fontConfig.Lines, r.textColor(), lineHeight)
		}
	} else {
		if len(speaker) > 0 {
			if r.layout.SpeakerBrackets {
				speaker = "【" + speaker + "】"
			}
			r.drawText(layer, speaker,
				originX+r.layout.NameX,
				originY+r.layout.NameY,
				r.layout.NameW, 1, r.layout.NameColor, lineHeight)
		}
		if len(text) > 0 {
			r.drawText(layer, text,
				originX+r.layout.TextX,
				originY+r.layout.TextY,
				r.layout.TextW, r.fontConfig.Lines, r.textColor(), lineHeight)
		}
	}
	if multiVisible && (len(multiText) > 0 || len(multiSpeaker) > 0) {
		r.drawMulti(layer, multiText, multiSpeaker, lineHeight, originX, originY)
	}
	return layer
}

// gk2Weekdays 是学園2 的星期表；原作的「第N週（X）」用月曜日到日曜日。
var gk2Weekdays = []string{"月", "火", "水", "木", "金", "土", "日"}

// drawSubWindow 画对话框左侧的日期/时间小窗。
//
// 周数/星期由 #day 换算（每天一格、七天一周），时段直接用 date_info 的
// 帧图：0=朝、4=放課後-残4…，和剧本的 #time 取值一一对应。
func (r *MessageWindowRenderer) drawSubWindow(canvas *gfx.BgraCanvas, originX, originY int) {
	sub := r.layout.SubWindow
	if !sub.Enabled {
		return
	}
	if sub.DayText && r.subDay > 0 && r.font != nil {
		week := (r.subDay-1)/7 + 1
		weekday := (r.subDay - 1) % 7
		name := gk2Weekdays[weekday]
		text := "第" + strconv.Itoa(week) + "週 （" + name + "）"
		size := sub.DayFontSize
		if size <= 0 {
			size = 15
		}
		color := sub.DayColor
		if color == 0 {
			color = 0xffffffff
		}
		width := sub.Width
		if width <= 0 {
			width = 126
		}
		rendered := r.font.RenderOutline(text, size, 1, 1, size+6, width, 1, color, outlineFor(color))
		if rendered != nil {
			x := originX + sub.X + (width-rendered.Bounds().Dx())/2
			blitRGBA(canvas, rendered, x, originY+sub.Y+sub.DayY)
		}
	}
	if sub.TimeImage != "" && r.subTime >= 0 {
		image := r.loadImage(sub.TimeImage)
		if image != nil {
			frames := sub.TimeFrames
			if frames <= 0 {
				frames = image.FrameCount
			}
			if frames > 0 {
				frameHeight := image.Height / frames
				frame := r.subTime + sub.TimeOffset
				if frame < 0 {
					return
				}
				if frame >= frames {
					frame = frames - 1
				}
				if frameHeight > 0 {
					canvas.DrawRegion(image, 0, frame*frameHeight, image.Width, frameHeight,
						originX+sub.TimeX, originY+sub.TimeY)
				}
			}
		}
	}
}

// drawMulti 画第二个发言框（[message window] mm name / mm text）。
//
// 位置和颜色来自布局；mm name align 决定名字的水平对齐。
// 第二框和主框共用同一个原点（base pos 或底部锚定），配置里的 mm 坐标是
// 相对对话框原点而不是相对主框区域。
func (r *MessageWindowRenderer) drawMulti(canvas *gfx.BgraCanvas, text, speaker string, lineHeight, originX, originY int) {
	if len(speaker) > 0 {
		r.drawTextAligned(canvas, speaker,
			originX+r.layout.MMNameX,
			originY+r.layout.MMNameY,
			r.layout.MMNameW, 1, r.layout.MMNameColor, lineHeight, r.layout.MMNameAlign)
	}
	if len(text) > 0 {
		r.drawText(canvas, text,
			originX+r.layout.MMTextX,
			originY+r.layout.MMTextY,
			r.layout.MMTextW, r.fontConfig.Lines, r.textColor(), lineHeight)
	}
}

// outlineFor 按文字颜色选描边颜色。
//
// 原引擎的描边色不是配置项：深色字配白描边、浅色字配黑描边，才能在
// SL/SLIE 的白色对话框和侵蝕/淫獄 的黑色对话框上都保持可读。
func outlineFor(color uint32) uint32 {
	red := int(color>>16) & 0xff
	green := int(color>>8) & 0xff
	blue := int(color) & 0xff
	luminance := (red*299 + green*587 + blue*114) / 1000
	if luminance >= 128 {
		return 0xff000000
	}
	return 0xffffffff
}

// drawTextAligned 按 0 左/1 中/2 右 对齐画一行。
func (r *MessageWindowRenderer) drawTextAligned(canvas *gfx.BgraCanvas, text string, x, y, maxWidth, maxLines int, color uint32, lineHeight, align int) {
	if r.font == nil || text == "" {
		return
	}
	rendered := r.font.RenderOutline(text, r.fontConfig.Size, r.fontConfig.Edge, r.fontConfig.Shadow,
		lineHeight, maxWidth, maxLines, color, outlineFor(color))
	if rendered == nil {
		return
	}
	width := rendered.Bounds().Dx()
	switch align {
	case 1:
		x += (maxWidth - width) / 2
	case 2:
		x += maxWidth - width
	}
	blitRGBA(canvas, rendered, x, y)
}

// SetSubWindowValues 写入左侧日期小窗要显示的日期/时段。
//
// 返回是否变化：值不变时不需要让 SceneComposer 重排文字。day<=0 时只画
// 时段（学園2 的序章日 #day 还是 0，原作也只显示时刻）。
func (r *MessageWindowRenderer) SetSubWindowValues(day, time int) bool {
	if r == nil {
		return false
	}
	if r.subDay == day && r.subTime == time {
		return false
	}
	r.subDay = day
	r.subTime = time
	return true
}

// SetBackgroundColor 覆盖对话框底色；返回是否真的改变了。
//
// 设置画面的四个滑杆各自是 0…255 的分量，Session 把它们拼成 ARGB 再传进来。
// 返回 bool 是为了让调用方只在变化时 Invalidate，避免拖动滑杆的每一帧
// 都把整段文字重排一遍。
func (r *MessageWindowRenderer) SetBackgroundColor(argb uint32) bool {
	if r.bgOverrideSet && r.bgOverride == argb {
		return false
	}
	r.bgOverride = argb
	r.bgOverrideSet = true
	return true
}

// effectiveBackgroundColor 返回当前该用的底色。
func (r *MessageWindowRenderer) effectiveBackgroundColor() uint32 {
	if r.bgOverrideSet {
		return r.bgOverride
	}
	return r.layout.BgColor
}

// textColor 按对话框底色选正文颜色。
//
// 原版没有单独的 `text color` 键：底色是半透明黑（侵蝕/淫獄）时用白字，
// 底色是白色（SL/SLIE 的 0xc0ffffff）时用黑字。以前固定白字，在浅色
// 对话框上等于完全看不见正文。
func (r *MessageWindowRenderer) textColor() uint32 {
	// 配置显式指定了正文颜色时优先用它。
	if r.layout.TextColorSet {
		return r.layout.TextColor
	}
	// 底图本身是浅色窗口（BaseOpaque 的 message_back 是白灰渐变）时用黑字。
	if r.layout.BaseOpaque {
		return 0xff000000
	}
	argb := r.effectiveBackgroundColor()
	if argb == 0x88ffffff {
		// 満淫電車2 的白色哨兵：底图本身是深色，正文用白字。
		return 0xffffffff
	}
	red := int(argb>>16) & 0xff
	green := int(argb>>8) & 0xff
	blue := int(argb) & 0xff
	luminance := (red*299 + green*587 + blue*114) / 1000
	if luminance >= 160 {
		return 0xff000000
	}
	return messageTextColor
}

// baseOrigin 返回对话框各元素的坐标原点。
//
// 配置给了 `base pos` 就照用（侵蝕/淫獄/SLIE 都显式写 `0,768-213`）。
// SL（BSXScript 2.0）的 bsxx.cfg 没有这一项，而原引擎的对话框本来就是底部
// 锚定的：底图贴在画布底部，名字/正文/催促图标再相对它偏移。旧实现让
// BaseX/BaseY 保持 0，整个对话框被画到屏幕顶部，而底部工具条还在原位——
// 这就是"对话框在上、按钮在下"的错位。
// 底图缺失时退回 (0,0)：没有底图就无从知道窗口高度，至少不要让坐标变成负数。
func (r *MessageWindowRenderer) baseOrigin(canvasHeight int) (int, int) {
	if r.layout.BasePosSet {
		return r.layout.BaseX, r.layout.BaseY + r.windowOffset
	}
	base := r.loadImage(r.layout.Base)
	if base == nil || base.Height <= 0 {
		return 0, r.windowOffset
	}
	return 0, canvasHeight - base.Height + r.windowOffset
}

// drawMessageBase 用 bg color 给对话框底图上色，并按颜色里的 alpha 决定不透明度。
//
// 原版把 bg color 同时当作底色和窗口透明度：默认 0x80000000 是半透明黑，
// 云纹底图被压成深色，背景场景仍然透得出来。若把底图完整不透明地贴上去，
// 场景会被整块挡住，和原版画面完全不同。未配置 bg color 时退回直接贴底图。
func (r *MessageWindowRenderer) drawMessageBase(canvas *gfx.BgraCanvas) {
	base := r.loadImage(r.layout.Base)
	if base == nil {
		return
	}
	// 原点只取决于画布高度；画布尺寸已经在 cacheKey 里，缓存不会失效错。
	originX, originY := r.baseOrigin(canvas.Height)

	// 内部衬底必须先画在边框下面：它只覆盖窗口内部，不会盖住圆角。
	// FillMask 优先：原版就是拿 message_back 的白色形状乘窗口色，
	// 圆角和渐变边缘都能保住；没有 mask 的作品仍走矩形。
	if r.layout.FillMask != "" {
		if mask := r.loadImage(r.layout.FillMask); mask != nil {
			// 设置画面的 BACKGROUND 滑杆改的是 bgOverride；有 override 时
			// 用它，否则用布局里写死的 FillColor（原版默认 0xa6000000）。
			color := r.layout.FillColor
			if r.bgOverrideSet {
				color = r.bgOverride
			}
			canvas.FillMask(mask, originX, originY, color)
		}
	} else if r.layout.FillRect[2] > 0 && r.layout.FillRect[3] > 0 {
		color := r.layout.FillColor
		fillAlpha := int(color >> 24)
		if fillAlpha > 0 {
			canvas.FillBlend(
				originX+r.layout.FillRect[0], originY+r.layout.FillRect[1],
				r.layout.FillRect[2], r.layout.FillRect[3],
				byte(color), byte(color>>8), byte(color>>16), fillAlpha)
		}
	}

	// BaseOpaque：底图是"按索引画形状"的 8 位图（调色板 alpha 全 0），
	// 必须忽略 alpha 整块铺上，否则窗口完全透明。见 config.MessageWindow.BaseOpaque。
	if r.layout.BaseOpaque {
		blitTinted(canvas, base, originX, originY, 255, 255, 255, 255, true)
		return
	}

	argb := r.effectiveBackgroundColor()
	alpha := int(argb >> 24)
	// 0x88ffffff 是"白色 = 不染色"的哨兵：満淫電車2 的 msg_back 本身
	// 已经是深色云纹面板，原版只按 alpha 叠它，不再混一层白。旧混合
	// 会把整块面板洗成灰白，和原作截图完全不像。
	if argb == 0x88ffffff && !r.layout.FillBase {
		blitAlpha(canvas, base, originX, originY, alpha)
		return
	}
	if r.layout.FillBase {
		// 底图本体透明：先用底色铺满整个底图矩形，再叠边框/标签。
		// alpha 为 0 时退到不透明，否则填了也看不见。
		fillAlpha := alpha
		if fillAlpha <= 0 {
			fillAlpha = 0xff
		}
		canvas.Fill(originX, originY, base.Width, base.Height,
			byte(argb), byte(argb>>8), byte(argb>>16), byte(fillAlpha))
	}
	if argb == 0 {
		canvas.Draw(base, originX, originY)
		return
	}
	if alpha <= 0 {
		return
	}

	blitTinted(canvas, base, originX, originY,
		int(argb>>16)&0xff, int(argb>>8)&0xff, int(argb)&0xff, alpha, false)
}

func (r *MessageWindowRenderer) drawImage(canvas *gfx.BgraCanvas, name string, x, y int) {
	if name == "" {
		return
	}
	if source := r.loadImage(name); source != nil {
		canvas.Draw(source, x, y)
	}
}

func (r *MessageWindowRenderer) loadImage(name string) *gfx.BsgImage {
	if name == "" || r.images == nil {
		return nil
	}
	return r.images.Get(name)
}

func (r *MessageWindowRenderer) lineHeight() int {
	if r.fontConfig.Height > 0 {
		return r.fontConfig.Height
	}
	return r.fontConfig.Size + defaultLineGap
}

// renderName 渲染内联名字，用于量宽和贴图。
//
// 只画一行；尺寸取 NameW（默认 240），避免超长名字按 TextW 撑开整行。
func (r *MessageWindowRenderer) renderName(name string, lineHeight int) *image.RGBA {
	if r.font == nil || name == "" {
		return nil
	}
	maxWidth := r.layout.NameW
	if maxWidth <= 0 {
		maxWidth = 240
	}
	return r.font.RenderOutline(name, r.fontConfig.Size, r.fontConfig.Edge, r.fontConfig.Shadow,
		lineHeight, maxWidth, 1, r.layout.NameColor, outlineFor(r.layout.NameColor))
}

func (r *MessageWindowRenderer) drawText(canvas *gfx.BgraCanvas, text string, x, y, maxWidth, maxLines int, color uint32, lineHeight int) {
	if r.font == nil || text == "" {
		return
	}
	rendered := r.font.RenderOutline(text, r.fontConfig.Size, r.fontConfig.Edge, r.fontConfig.Shadow,
		lineHeight, maxWidth, maxLines, color, outlineFor(color))
	blitRGBA(canvas, rendered, x, y)
}

// drawPrompt 画催促玩家点击的图标。图标按列优先切格，所以
// frame%columns 是列、frame/columns 是行；本作是 1 列 8 行的竖条。
func (r *MessageWindowRenderer) drawPrompt(canvas *gfx.BgraCanvas) {
	layout := r.layout
	source := r.loadImage(layout.Unagashi)
	if source == nil || layout.UnagashiFrames <= 0 {
		return
	}

	columns := layout.UnagashiColumns
	if columns <= 0 {
		columns = 1
	}
	cellWidth := layout.UnagashiCellW
	cellHeight := layout.UnagashiCellH
	if cellWidth <= 0 {
		cellWidth = source.Width / columns
	}
	rows := (layout.UnagashiFrames + columns - 1) / columns
	if cellHeight <= 0 && rows > 0 {
		cellHeight = source.Height / rows
	}
	if cellWidth <= 0 || cellHeight <= 0 {
		return
	}

	speed := max(1, layout.UnagashiSpeed)
	frame := int((time.Now().UnixMilli() / int64(speed)) % int64(layout.UnagashiFrames))
	column := frame % columns
	row := frame / columns

	originX, originY := r.baseOrigin(canvas.Height)
	canvas.DrawRegion(source,
		column*cellWidth, row*cellHeight, cellWidth, cellHeight,
		originX+layout.UnagashiX, originY+layout.UnagashiY)
}

// DrawMenuBar 单独画常驻工具条（游戏菜单条）。
//
// 学園2 的工具条固定在屏幕顶端，地图、存档画面和对话框收起时都可见；
// 它不能跟着对话框的缓存图层走，所以给 SceneComposer 一个独立入口。
// 只画配置了 Persistent 的作品，其它作品返回 false。
func (r *MessageWindowRenderer) DrawMenuBar(canvas *gfx.BgraCanvas, hoverButton int) bool {
	if r == nil || canvas == nil || !r.gameMenu.Persistent {
		return false
	}
	drawn := false
	if r.gameMenu.Base != "" {
		if image := r.loadImage(r.gameMenu.Base); image != nil {
			canvas.Draw(image, r.gameMenu.BaseX, r.gameMenu.BaseY)
			drawn = true
		}
	}
	for index, button := range r.gameMenu.Buttons {
		if !button.Persistent {
			continue
		}
		source := r.loadImage(button.Image)
		if source == nil {
			continue
		}
		frames := max(1, button.Frames)
		frame := 0
		if index == hoverButton {
			frame = 1
		}
		if frame >= frames {
			frame = frames - 1
		}
		height := source.Height / frames
		if height <= 0 {
			continue
		}
		canvas.DrawRegion(source, 0, frame*height, source.Width, height, button.X, button.Y)
		drawn = true
	}
	return drawn
}

// drawButtons 每帧重画菜单按钮：缓存图层只保存静态内容，悬停帧不能跟着被缓存。
func (r *MessageWindowRenderer) drawButtons(canvas *gfx.BgraCanvas, hoverButton int) {
	if r.images == nil {
		return
	}
	for index, button := range r.gameMenu.Buttons {
		source := r.loadImage(button.Image)
		if source == nil {
			continue
		}

		frames := button.Frames
		if frames < 1 {
			frames = 1
		}
		frame := 0
		if index == hoverButton {
			frame = 1
		}
		if frame >= frames {
			frame = frames - 1
		}
		height := source.Height / frames
		if height <= 0 {
			continue
		}
		y := button.Y
		// 对话框三档位置只移动窗口内的按钮（SKIP/AUTO/V.PLAY、×/▲/▼），
		// 顶部常驻工具条仍固定在 y=0。
		if r.layout.BasePosSet && button.Y >= r.layout.BaseY {
			y += r.windowOffset
		}
		canvas.DrawRegion(source, 0, frame*height, source.Width, height, button.X, y)
	}
}

// blitRGBA 把 textrender 产出的 RGBA 图层贴到 BGRA 画布上。
//
// image.RGBA 的像素是预乘 alpha 的，而 BgraCanvas 的混合函数按直通 alpha 计算；
// 直接把字节当直通色用会让文字的抗锯齿边缘发黑，所以先还原颜色再混合。
func blitRGBA(canvas *gfx.BgraCanvas, source *image.RGBA, x, y int) {
	if canvas == nil || source == nil {
		return
	}
	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(canvas.Width, x+width)
	bottom := min(canvas.Height, y+height)
	if left >= right || top >= bottom {
		return
	}

	target := canvas.Buffer
	for row := top; row < bottom; row++ {
		sourceOffset := source.PixOffset(bounds.Min.X+(left-x), bounds.Min.Y+(row-y))
		targetOffset := ((row * canvas.Width) + left) * 4

		for column := left; column < right; column++ {
			alpha := int(source.Pix[sourceOffset+3])
			if alpha != 0 {
				red := int(source.Pix[sourceOffset])
				green := int(source.Pix[sourceOffset+1])
				blue := int(source.Pix[sourceOffset+2])
				if alpha != 255 {
					red = min(255, red*255/alpha)
					green = min(255, green*255/alpha)
					blue = min(255, blue*255/alpha)
				}

				if alpha == 255 {
					target[targetOffset] = byte(blue)
					target[targetOffset+1] = byte(green)
					target[targetOffset+2] = byte(red)
					target[targetOffset+3] = 255
				} else {
					keep := int(target[targetOffset+3]) * (255 - alpha) / 255
					outAlpha := alpha + keep
					if outAlpha > 0 {
						target[targetOffset] = byte((blue*alpha + int(target[targetOffset])*keep) / outAlpha)
						target[targetOffset+1] = byte((green*alpha + int(target[targetOffset+1])*keep) / outAlpha)
						target[targetOffset+2] = byte((red*alpha + int(target[targetOffset+2])*keep) / outAlpha)
						target[targetOffset+3] = byte(outAlpha)
					}
				}
			}

			sourceOffset += 4
			targetOffset += 4
		}
	}
}

// blitTinted 把底图按颜色染色后再以指定不透明度叠到画布上。
//
// 颜色是乘算而不是覆盖：这样底图里的云纹在非黑色背景下仍然可见，
// 而默认的黑色会把纹理压成纯黑，只留下半透明窗口本身。
// opaque 为真时忽略底图自身的 alpha：底图是按"像素索引"表达形状的 8 位图
// （调色板 alpha 全 0），只有这种方式才能把窗口本体画出来。
func blitTinted(canvas *gfx.BgraCanvas, source *gfx.BsgImage, x, y, tintRed, tintGreen, tintBlue, alpha int, opaque bool) {
	if canvas == nil || source == nil || alpha <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(canvas.Width, x+source.Width)
	bottom := min(canvas.Height, y+source.Height)
	if left >= right || top >= bottom {
		return
	}

	sourceStride := source.Stride()
	target := canvas.Buffer

	for row := top; row < bottom; row++ {
		sourceOffset := ((row - y) * sourceStride) + ((left - x) * 4)
		targetOffset := ((row * canvas.Width) + left) * 4

		for column := left; column < right; column++ {
			sourceAlpha := int(source.Pixels[sourceOffset+3])
			if opaque {
				sourceAlpha = 255
			}
			effective := sourceAlpha * alpha / 255
			if effective > 0 {
				// bg color 是"叠在底图上的半透明色"，不是乘算滤镜：
				// 先按色块比例混合底图与底色，再按 alpha 叠到场景上。
				// 若直接乘 tint，默认的黑色会把整张云纹压成纯黑，只剩一块死板。
				blue := (int(source.Pixels[sourceOffset])*(255-alpha) + tintBlue*alpha) / 255
				green := (int(source.Pixels[sourceOffset+1])*(255-alpha) + tintGreen*alpha) / 255
				red := (int(source.Pixels[sourceOffset+2])*(255-alpha) + tintRed*alpha) / 255

				if effective == 255 {
					target[targetOffset] = byte(blue)
					target[targetOffset+1] = byte(green)
					target[targetOffset+2] = byte(red)
					target[targetOffset+3] = 255
				} else {
					keep := int(target[targetOffset+3]) * (255 - effective) / 255
					outAlpha := effective + keep
					if outAlpha > 0 {
						target[targetOffset] = byte((blue*effective + int(target[targetOffset])*keep) / outAlpha)
						target[targetOffset+1] = byte((green*effective + int(target[targetOffset+1])*keep) / outAlpha)
						target[targetOffset+2] = byte((red*effective + int(target[targetOffset+2])*keep) / outAlpha)
						target[targetOffset+3] = byte(outAlpha)
					}
				}
			}

			sourceOffset += 4
			targetOffset += 4
		}
	}
}

// drawNotice 画工具按钮的短暂反馈（ON/OFF）。放在对话框底图上方、
// 工具条上方一点，避免和正文重叠。
//
// 旧实现把位置写死成 (30,700)：那是 1280x720 下的经验值，而 SL 的画布是
// 1024x576，700 已经落在屏幕外，提示根本看不见。对话框原点本身又随画布高度
// 变化，所以提示必须贴着解析出来的对话框上沿走。
func (r *MessageWindowRenderer) drawNotice(canvas *gfx.BgraCanvas, notice string) {
	if r.font == nil || notice == "" {
		return
	}
	image := r.font.Render(notice, 16, 1, 1, 20, 480, 1, 0xffffffff)

	// 提示条贴着对话框上沿外侧，需要的一点余量刚好是一个正文行高。
	// 原点不能解出对话框（底图缺失，或配置把窗口贴到 0,0）时退回画布底部：
	// 提示内容本来就是底部工具条的开关状态，放在底部既不会被顶出画面，
	// 也保留了旧实现"左下角"的观感。
	noticeY := canvas.Height - 26
	_, originY := r.baseOrigin(canvas.Height)
	if originY-26 > 0 {
		noticeY = originY - 26
	}
	blitRGBA(canvas, image, 30, noticeY)
}

// blitAlpha 只按 alpha 叠底图，不改变底图颜色。
func blitAlpha(canvas *gfx.BgraCanvas, source *gfx.BsgImage, x, y, alpha int) {
	if canvas == nil || source == nil || alpha <= 0 {
		return
	}
	left := max(0, x)
	top := max(0, y)
	right := min(canvas.Width, x+source.Width)
	bottom := min(canvas.Height, y+source.Height)
	if left >= right || top >= bottom {
		return
	}
	sourceStride := source.Stride()
	target := canvas.Buffer
	for row := top; row < bottom; row++ {
		sourceOffset := ((row - y) * sourceStride) + ((left - x) * 4)
		targetOffset := ((row * canvas.Width) + left) * 4
		for column := left; column < right; column++ {
			sourceAlpha := int(source.Pixels[sourceOffset+3])
			effective := sourceAlpha * alpha / 255
			if effective > 0 {
				blue := int(source.Pixels[sourceOffset])
				green := int(source.Pixels[sourceOffset+1])
				red := int(source.Pixels[sourceOffset+2])
				if effective == 255 {
					target[targetOffset] = byte(blue)
					target[targetOffset+1] = byte(green)
					target[targetOffset+2] = byte(red)
					target[targetOffset+3] = 255
				} else {
					keep := int(target[targetOffset+3]) * (255 - effective) / 255
					outAlpha := effective + keep
					if outAlpha > 0 {
						target[targetOffset] = byte((blue*effective + int(target[targetOffset])*keep) / outAlpha)
						target[targetOffset+1] = byte((green*effective + int(target[targetOffset+1])*keep) / outAlpha)
						target[targetOffset+2] = byte((red*effective + int(target[targetOffset+2])*keep) / outAlpha)
						target[targetOffset+3] = byte(outAlpha)
					}
				}
			}
			sourceOffset += 4
			targetOffset += 4
		}
	}
}
