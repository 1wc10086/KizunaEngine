package rendering

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// InformationWindowValues 是信息窗要显示的值。
//
// 引擎把"读变量"和"画窗口"分开：读值由 Session（它才知道脚本变量表）完成，
// 渲染器只拿到已经解析好的数字，因此这个文件不依赖 vm。
type InformationWindowValues struct {
	// Day 是日期；负数表示当前不显示日期（例如标题画面、回想场景）。
	Day int
	// Time 是时段；负数表示不显示。
	Time int
	// Week / WeekDay 是脚本维护的周数与曜日（#week_no / #week_day）。
	// 0 是合法的周数，不能靠数值判断是否提供，所以只有 WeekKnown 为真时才使用。
	Week      int
	WeekDay   int
	WeekKnown bool
}

// InformationWindowRenderer 画「行動開始から X日目」行动信息窗。
//
// 窗口在屏幕上的位置来自 `base pos`，并不固定在左上角：淫獄の放課後2
// 把它放在右上角 (862,0)。配置里的其余坐标（day no / day unit / day sp /
// time zone）都相对这个原点，所以下面统一加上 origin 偏移。
//
// 原引擎把数字做成竖排图（info_dayno），按值取帧贴上去，而不是用字体渲染。
// 这里沿用同一套素材与坐标，保证和原版视觉一致。
type InformationWindowRenderer struct {
	layout config.InformationWindow
	images *ImageCache
	// last 记录上次画的值，用于避免重复日志（值不变时每帧都会重画，
	// 但只有真正变化时才值得记一行日志）。
	last  InformationWindowValues
	drawn bool
}

func NewInformationWindowRenderer(images *ImageCache, layout config.InformationWindow) *InformationWindowRenderer {
	return &InformationWindowRenderer{layout: layout, images: images}
}

// Visible 判断这一帧是否应该显示信息窗。
//
// 标题画面、回想、系统界面里 Day 和 Time 都是 -1：此时连底板也不能画。
// 底板是一条带圆角的深色胶囊，单独画出来会在标题左上角留下一个空白标签，
// 玩家会以为日期加载失败；原引擎在这些画面完全不显示信息窗。
func (v InformationWindowValues) Visible() bool {
	return v.Day >= 0 || v.Time >= 0 || v.WeekKnown
}

// Empty 表示这部作品没有配信息窗，调用方可以直接跳过。
func (r *InformationWindowRenderer) Empty() bool {
	return r == nil || r.layout.Empty()
}

// origin 返回信息窗的坐标原点。
//
// 底板图 info_base 就是整窗大小（162x126），数字和单位图都是叠在它内部的，
// 因此配置坐标必须在底板原点上加偏移。
//
// 注意：`base pos` 并不只服务于底板。特別授業3SLG 旧版（tj3slg）没有
// `base = info_base`，但仍有 `base pos = 100,410`，日期数字和单位图都是
// 相对它定位的。旧实现只在有底板时才使用 BaseX/BaseY，导致原点回落到
// (0,0)，整块信息窗被画到画面左上角。只要配置给了非零的 base pos 就必须
// 采用它，哪怕没有底板图。
func (r *InformationWindowRenderer) origin() (int, int) {
	if len(r.layout.Base) > 0 || r.layout.BaseX != 0 || r.layout.BaseY != 0 {
		return r.layout.BaseX, r.layout.BaseY
	}
	return 0, 0
}

// Draw 把信息窗画到画布上。
//
// 返回是否真的画了东西：调用方据此决定要不要把这一帧标成"有内容"。
func (r *InformationWindowRenderer) Draw(canvas *gfx.BgraCanvas, values InformationWindowValues) bool {
	// 没有任何要显示的值时整块跳过：包括底板。
	// 只判断 Empty 不够——标题画面也有配置，但 Day/Time 都是 -1。
	if r.Empty() || canvas == nil || !values.Visible() {
		return false
	}
	drawn := false
	ox, oy := r.origin()

	// 内部衬底先画，边框和字牌随后覆盖。坐标是绝对屏幕坐标。
	// FillMask 用 msg_info_back 的形状铺色，圆角才和原版一致。
	if r.layout.FillMask != "" {
		if mask := r.images.Get(r.layout.FillMask); mask != nil {
			canvas.FillMask(mask, r.layout.BaseX+mask.OffsetX, r.layout.BaseY+mask.OffsetY, r.layout.FillColor)
		}
	} else if r.layout.FillRect[2] > 0 && r.layout.FillRect[3] > 0 {
		color := r.layout.FillColor
		if alpha := int(color >> 24); alpha > 0 {
			canvas.FillBlend(
				r.layout.FillRect[0], r.layout.FillRect[1],
				r.layout.FillRect[2], r.layout.FillRect[3],
				byte(color), byte(color>>8), byte(color>>16), alpha)
		}
	}
	// 底板先画，数字与单位叠在上面。
	if len(r.layout.Base) > 0 {
		if image := r.images.Get(r.layout.Base); image != nil {
			canvas.Draw(image, r.layout.BaseX+image.OffsetX, r.layout.BaseY+image.OffsetY)
			drawn = true
		}
	}
	// 固定字牌（第/週/曜/日）属于窗口装饰，和底板一起画；数字随后覆盖在
	// 它们预留的空位里。坐标是绝对屏幕坐标，不能再叠加 base pos。
	weekend := r.isWeekend(values)
	if r.drawLabels(canvas, weekend) {
		drawn = true
	}

	if (values.Day >= 0 || values.WeekKnown) && len(r.layout.DayVariable) > 0 {
		if r.drawDay(canvas, values, ox, oy, weekend) {
			drawn = true
		}
	}

	if values.Time >= 0 && r.inTimeZone(values.Time) {
		// 时段变量是 1 基的（#time 走 1..5），而时段图只有 5 帧（0..4）。
		// 直接用变量值当帧号会让第 0 帧永远显示不到、最后一帧被 clamp 掉，
		// 所以按"区间下界"对齐：value - TimeZoneLow 才是帧号。
		if r.drawStrip(canvas, r.layout.TimeStrip, values.Time-r.layout.TimeZoneLow, ox, oy) {
			drawn = true
		}
	}

	if drawn {
		r.logChange(values)
	}
	return drawn
}

func (r *InformationWindowRenderer) inTimeZone(value int) bool {
	return value >= r.layout.TimeZoneLow && value <= r.layout.TimeZoneHigh
}

// isWeekend 判断当前是否处于一周的最后一天（放課後2 的「週末」）。
//
// 只有配置了 WeekendFirst 的作品才可能为真；一周天数沿用 WeekDays
// （放課後2 是 6：月～週末）。脚本给了 #week_day 时直接用，否则按
// (#day-WeekStart)%WeekDays 换算，与 drawDay 的算法保持一致。
func (r *InformationWindowRenderer) isWeekend(values InformationWindowValues) bool {
	if len(r.layout.WeekendFirst.Image) == 0 {
		return false
	}
	days := r.layout.WeekDays
	if days <= 0 {
		days = 7
	}
	weekday := values.WeekDay
	if !values.WeekKnown {
		offset := values.Day - r.layout.WeekStart
		if offset < 0 {
			return false
		}
		weekday = offset % days
	}
	if weekday < 0 {
		weekday += days
	}
	return weekday == days-1
}

// drawLabels 画信息窗上的固定字牌。
//
// 每块字牌是一张多帧图的一帧（放課後2 的 msg_info_str1 是 22x88 的四帧：
// 第/週/曜/日）。UIButton.Start 指定帧号，FixedFrame 为真时不受悬停影响——
// 信息窗本来也没有交互，这里统一按固定帧画。
//
// weekend 为真且配置要求替换时，跳过最后两块（曜/日）：那两个字的位置
// 会被「週末」两帧占用，否则会叠成「週曜日」。
func (r *InformationWindowRenderer) drawLabels(canvas *gfx.BgraCanvas, weekend bool) bool {
	if r.images == nil {
		return false
	}
	labels := r.layout.Labels
	if weekend && r.layout.WeekendReplacesLabels && len(labels) >= 2 {
		labels = labels[:len(labels)-2]
	}
	drawn := false
	for _, label := range labels {
		if label.Empty() {
			continue
		}
		image := r.images.Get(label.Image)
		if image == nil || image.Height <= 0 {
			continue
		}
		frames := max(1, label.Frames)
		cell := image.Height / frames
		if cell <= 0 {
			continue
		}
		frame := core.Clamp(label.Start, 0, frames-1)
		canvas.DrawRegion(image, 0, frame*cell, image.Width, cell,
			label.X+image.OffsetX, label.Y+image.OffsetY)
		drawn = true
	}
	return drawn
}

// drawDay 画日期数字 + 「日目」单位图。
//
// 关键细节：原引擎从个位往高位画，每进一位横移 Offset（负数，往左）。
// 但"日目"单位图的位置是固定的，所以一位数和两位数必须占同样宽度——
// 否则个位数时数字会紧贴单位图、两位数时又会重叠。
// 做法是：先算出总位数，把个位起点整体左移 (位数-1)*|Offset|，
// 这样无论几位数，最高位都对齐在同一处，个位随位数变化。
func (r *InformationWindowRenderer) drawDay(canvas *gfx.BgraCanvas, values InformationWindowValues, ox, oy int, weekend bool) bool {
	day := values.Day
	// 日期越过门槛时，整块换成固定图案（例如"行動開始から しばらく"）。
	// 这是原引擎表达"日期还没有意义"的方式：变量被置成大数当哨兵，
	// 此时画数字会得到一个荒唐的天数，所以必须整块替换而不是只换某几位。
	if day >= r.layout.DayBorder {
		if len(r.layout.DaySpecial) == 0 {
			// 日期未知（#day 被脚本置成 255/128 这类哨兵）时不画数字；
			// 底图和时段仍然显示，和原引擎的"第 週 曜日"空档一致。
			return false
		}
		if image := r.images.Get(r.layout.DaySpecial); image != nil {
			canvas.Draw(image, r.layout.DaySpecialX+ox+image.OffsetX, r.layout.DaySpecialY+oy+image.OffsetY)
			return true
		}
		return false
	}
	// week no / week day 版式：脚本只维护 #day 这个连续计数，周数和星期由
	// 引擎换算。素材给了 9 帧"N週"和 6 帧"X曜日"，说明是每周 6 天（月～土），
	// 因此第 day 天属于第 (day-1)/6+1 周、第 (day-1)%6 个工作日。
	// 注意这里用未加 DayAdjustment 的原始计数：调整量是给"第几日"显示用的，
	// 套到周数上会让每周第一天算错。
	if len(r.layout.WeekStrip.Image) > 0 {
		// WeekDays/WeekStart 决定"一周几天、从哪天起算"：
		//   - 黒の教室：6 天、从 1 起算（月～土，周日休息）；
		//   - KD：10 天、从 0 起算（#day 的十位是周、个位是曜日，
		//     30 = 第3週日曜日；60 … 110 直接是路线名）。
		var week, weekday int
		if values.WeekKnown {
			week, weekday = values.Week, values.WeekDay
		} else {
			days := r.layout.WeekDays
			if days <= 0 {
				days = 6
			}
			offset := day - r.layout.WeekStart
			if offset < 0 {
				return false
			}
			week = offset / days
			weekday = offset % days
		}
		if week < 0 {
			return false
		}
		drawnWeek := r.drawStrip(canvas, r.layout.WeekStrip, week, ox, oy)
		// KD 的 オープニング（#day < 10）只用一条宽标签；再叠曜日会压字。
		// 日期未知（Day < 0）时不据此抑制曜日。
		if values.Day >= 0 && day < r.layout.WeekDayFrom {
			return drawnWeek
		}
		// 周末：原版画「週」「末」两帧，不再画 WeekDayStrip 的月..日。
		if weekend && len(r.layout.WeekendFirst.Image) > 0 {
			drawnFirst := r.drawStrip(canvas, r.layout.WeekendFirst, 0, ox, oy)
			drawnSecond := r.drawStrip(canvas, r.layout.WeekendSecond, 0, ox, oy)
			return drawnWeek || drawnFirst || drawnSecond
		}
		drawnDay := r.drawStrip(canvas, r.layout.WeekDayStrip, weekday, ox, oy)
		return drawnWeek || drawnDay
	}
	// 显示值 = 变量值 + 调整量；原引擎用它把"第 0 天"显示成"1 日目"之类。
	day += r.layout.DayAdjustment
	if day < 0 {
		return false
	}

	strip := r.layout.DayStrip
	if len(strip.Image) == 0 {
		return false
	}
	image := r.images.Get(strip.Image)
	if image == nil {
		return false
	}
	frames := strip.Frames
	if frames <= 0 {
		frames = image.Height
	}
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return false
	}

	digits := digitCount(day)
	// 单位图与数字的基准位置由配置给（相对信息窗原点）；单位图固定在 DayUnitX/Y。
	// 数字的最高位对齐到"单位图左侧再留出位数宽度"的位置。
	offset := strip.Offset
	if offset == 0 {
		offset = -frameHeight // 没配时按图高退一位，保证两位数不重叠
	}
	// 从最高位画到个位：个位在 X，十位在 X+Offset（更左）。
	// 为了让"1日"和"10日"的单位图始终贴在数字右边，这里以个位为锚点，
	// 个位位置固定，位数增加时向左扩展——这与原引擎一致。
	topX := strip.X + ox + (digits-1)*offset
	for i := 0; i < digits; i++ {
		value := digitAt(day, digits-1-i)
		x := topX - i*offset
		canvas.DrawRegion(image, 0, value*frameHeight, image.Width, frameHeight,
			x+image.OffsetX, strip.Y+oy+image.OffsetY)
	}

	// 单位图（日目）贴在数字右侧。
	if len(r.layout.DayUnit) > 0 {
		if unit := r.images.Get(r.layout.DayUnit); unit != nil {
			canvas.Draw(unit, r.layout.DayUnitX+ox+unit.OffsetX, r.layout.DayUnitY+oy+unit.OffsetY)
		}
	}
	return true
}

// drawStrip 画一条竖排数字/图案条，按 value 取帧。坐标同样相对信息窗原点。
func (r *InformationWindowRenderer) drawStrip(canvas *gfx.BgraCanvas, strip config.DigitStrip, value, ox, oy int) bool {
	if len(strip.Image) == 0 {
		return false
	}
	image := r.images.Get(strip.Image)
	if image == nil {
		return false
	}
	frames := strip.Frames
	if strip.Horizontal {
		// 横向图：info_font 是变宽字形条，0-9 每格 10px，月..日每格 20px；
		// 其它横向图仍按等宽切帧。
		index := value + strip.Start
		if strip.AtlasDigits > 0 && strip.AtlasDigitWidth > 0 && strip.AtlasWideWidth > 0 {
			sourceX, width := 0, 0
			if index < strip.AtlasDigits {
				sourceX = index * strip.AtlasDigitWidth
				width = strip.AtlasDigitWidth
			} else {
				sourceX = strip.AtlasWideStart + (index-strip.AtlasDigits)*strip.AtlasWideWidth
				width = strip.AtlasWideWidth
			}
			if sourceX < 0 || width <= 0 || sourceX+width > image.Width {
				return false
			}
			canvas.DrawRegion(image, sourceX, 0, width, image.Height,
				strip.X+ox+image.OffsetX, strip.Y+oy+image.OffsetY)
			return true
		}
		if frames <= 0 {
			frames = image.Width
		}
		frameWidth := image.Width / max(frames, 1)
		if frameWidth <= 0 {
			return false
		}
		// Start 把曜日等偏移到图中的后半段，必须在 Clamp 之前加上。
		index = core.Clamp(index, 0, frames-1)
		canvas.DrawRegion(image, index*frameWidth, 0, frameWidth, image.Height,
			strip.X+ox+image.OffsetX, strip.Y+oy+image.OffsetY)
		return true
	}
	if frames <= 0 {
		frames = image.Height
	}
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return false
	}
	// 竖排零值时 Start 为 0，结果与原来一致。
	index := core.Clamp(value+strip.Start, 0, frames-1)
	canvas.DrawRegion(image, 0, index*frameHeight, image.Width, frameHeight,
		strip.X+ox+image.OffsetX, strip.Y+oy+image.OffsetY)
	return true
}

func (r *InformationWindowRenderer) logChange(values InformationWindowValues) {
	if r.drawn && r.last == values {
		return
	}
	r.drawn = true
	r.last = values
	core.Logf("hud: information window day=%d time=%d week=%d weekday=%d week_known=%t", values.Day, values.Time, values.Week, values.WeekDay, values.WeekKnown)
}

// digitCount 返回十进制位数（0 算 1 位）。
func digitCount(value int) int {
	if value <= 0 {
		return 1
	}
	count := 0
	for value > 0 {
		value /= 10
		count++
	}
	return count
}

// digitAt 取从高位起的第 index 位数字（0 是最高位）。
func digitAt(value, index int) int {
	digits := digitCount(value)
	if index < 0 || index >= digits {
		return 0
	}
	for i := 0; i < digits-index-1; i++ {
		value /= 10
	}
	return value % 10
}
