package session

import (
	"strconv"
	"time"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// 这一组函数画 bsxx.cfg 里两段"常驻小面板"：
//
//   - [extra window] / [levelmeter window]：等级 + 计量表面板
//     （GK2IEDL 的 extra_base + extra_level；UK/UKIE 的 lvmt_*/extra_*）。
//   - [free-sight screen]：自由视角的竖滑杆与「顔/胸/股間」按钮。
//
// 两者都由配置驱动，作品没写这两段时配置为空，绘制函数直接返回 false，
// 其它作品的画面完全不受影响。

// drawExtraWindow 画 [extra window] / [levelmeter window] 面板。
//
// 原版行为：鼠标进入 show area 才显示；auto hide 指定"离开后过多久收起"。
// 等级值取剧本用 `set levelmeter #point` 绑定的属性（GameState 里保存），
// 再按配置的两条帧条裁帧：等级图（Lv0…Lv6）与计量表（13 档圆环）。
//
// 关键点：UKIE 的 extra_level / extra_meter 是"整块面板大小"的竖排帧条，
// 每一帧都自带背景之外的文字/圆环，直接叠在底图原点上即可，不需要再偏移。
func (s *Session) drawExtraWindow(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || s.config == nil || s.gameState == nil {
		return false
	}
	screen := s.config.ExtraWindow
	if screen.Base == "" {
		return false
	}
	// 标题、系统界面和回想不挂 HUD：原引擎在这些画面不显示等级面板，
	// 留着会在标题右上角浮一块 Lv0。地图是否显示由下面的版式判断决定。
	if s.titleMode || s.ui != nil || s.menuVisible || s.replay {
		return false
	}
	// 引擎地图（map.dat 版式）会把整屏铺满，UK/UKIE 的等级面板留在画面上
	// 只会压住地点卡。但 CS 的欲望度面板是按 [extra window] 画的常驻 HUD，
	// 原版在地图上照样显示（手册的地图截图左下角就是它），因此只对没有
	// desire 行的作品保留"地图上不画"的旧行为。
	if s.mapShown && !screen.HasDesireMeter() {
		return false
	}
	// UKIE 的 [follow window view] mesg = game：信息窗/等级面板跟随对话框
	// 的显示状态。只对写了这一段的作品生效，其它作品维持原来的可见性，
	// 避免把 GK2IEDL 等作品的等级面板在对话框收起时也一起藏掉。
	//
	// CS 的欲望度面板在地图选择态也要显示：BeginSelect 会把消息框标记成
	// 隐藏，如果这里一并要求 MessageWindowVisible，地图上就看不到面板了。
	// 选择态/地图态属于"游戏画面"，允许显示；其它时候仍跟随对话框。
	if screen.FollowsMessageWindow && !s.gameState.Selecting() && !s.mapShown &&
		(!s.gameState.ShowMessageFrame() || !s.gameState.MessageWindowVisible()) {
		return false
	}
	if !s.extraWindowShown(screen.AutoHideMS, screen.ShowArea) {
		return false
	}
	drawn := s.drawImageAt(canvas, screen.Base, screen.BaseX, screen.BaseY)
	if screen.HasDesireMeter() {
		if s.drawDesireMeter(canvas, screen) {
			drawn = true
		}
		return drawn
	}
	value := s.extraMeterValue(screen)

	// 等级与计量表共用同一个变量。剧本里的 #point 是 0…6 的等级值
	// （7 帧等级图），计量表是 13 帧圆环：等级直接取 #point，计量表按
	// 等级范围放大到 13 档（0…6 → 0/2/4/6/8/10/12）。这样 #point=6 时
	// 正好是 Lv6 + 满格圆环，和手册截图一致。
	if screen.LevelImage != "" && screen.LevelFrames > 0 {
		if image := s.composer.Load(screen.LevelImage); image != nil {
			if frameHeight := image.Height / screen.LevelFrames; frameHeight > 0 {
				frame := core.Clamp(value, 0, screen.LevelFrames-1)
				canvas.DrawRegion(image, 0, frame*frameHeight, image.Width, frameHeight,
					screen.BaseX+screen.LevelX, screen.BaseY+screen.LevelY)
				drawn = true
			}
		}
	}
	if screen.MeterImage != "" && screen.MeterFrames > 0 {
		if image := s.composer.Load(screen.MeterImage); image != nil {
			if frameHeight := image.Height / screen.MeterFrames; frameHeight > 0 {
				frame := extraMeterFrame(value, screen.LevelFrames, screen.MeterFrames)
				canvas.DrawRegion(image, 0, frame*frameHeight, image.Width, frameHeight,
					screen.BaseX+screen.MeterX, screen.BaseY+screen.MeterY)
				drawn = true
			}
		}
	}
	return drawn
}

// drawDesireMeter 画 CS 式的多角色征服欲望度面板。
//
// 布局（全部来自 bsxx.cfg，不按作品硬编码）：
//
//	base           标题条（extra_base，198x134 的透明图，只画出顶部标题）
//	infoN          整行底图：人物头像 + 名称 + 深色轨道（exinfoN）
//	infoN bar      150x12 的绿→红渐变（extra_bar）
//	number font    0-9 竖排数字（exwindow_number）
//	unit           2 帧：% / MAX（exwindow_unit）
//
// 原引擎的行为：渐变条只画左侧 value/max 这一段（不是把整条渐变缩放），
// 数字与单位叠在条的中央；value 达到 max 时画 unit 的第 1 帧 MAX。
// 数值来自 desire 列表里的属性，和剧本的 desire_check 共用同一份数据。
func (s *Session) drawDesireMeter(canvas *gfx.BgraCanvas, screen config.ExtraWindowScreen) bool {
	if canvas == nil || s.composer == nil {
		return false
	}
	maxValue := screen.MaxValue
	if maxValue <= 0 {
		maxValue = 100
	}
	rows := min(len(screen.Infos), len(screen.DesireVariables))
	if rows <= 0 {
		return false
	}

	drawn := false
	var numberImage, unitImage *gfx.BsgImage
	if screen.NumberImage != "" && screen.NumberFrames > 0 {
		numberImage = s.composer.Load(screen.NumberImage)
	}
	if screen.UnitImage != "" && screen.UnitFrames > 0 {
		unitImage = s.composer.Load(screen.UnitImage)
	}
	for index := 0; index < rows; index++ {
		row := screen.Infos[index]
		if row.Image != "" {
			if s.drawExtraStripAt(canvas, row.Image, row.Frames, screen.BaseX+row.X, screen.BaseY+row.Y) {
				drawn = true
			}
		}
		value := s.readProperty(screen.DesireVariables[index])
		if value < 0 {
			value = 0
		}
		if value > maxValue {
			value = maxValue
		}
		barX := screen.BaseX + row.BarX
		barY := screen.BaseY + row.BarY
		if row.BarImage != "" {
			if s.drawDesireBar(canvas, row, barX, barY, value, maxValue) {
				drawn = true
			}
		}
		if s.drawDesireValue(canvas, barX, barY, row, value, maxValue, numberImage, unitImage, screen.NumberFrames, screen.UnitFrames) {
			drawn = true
		}
	}
	return drawn
}

// drawDesireBar 只画渐变条左侧的已填充部分。
//
// 关键点：原引擎是"裁左段"而不是"把整条渐变缩放成 value% 宽"。两者在
// 满值附近差别很小，但在 10% 这类低值时，缩放会把整条绿→红压成一小段、
// 末端直接变红；裁左段才和手册截图一致（低值只显示绿色）。
func (s *Session) drawDesireBar(canvas *gfx.BgraCanvas, row config.ExtraWindowInfoRow, barX, barY, value, maxValue int) bool {
	image := s.composer.Load(row.BarImage)
	if image == nil {
		return false
	}
	frames := max(1, row.BarFrames)
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return false
	}
	if value <= 0 {
		return false
	}
	width := image.Width * value / maxValue
	if width <= 0 {
		return false
	}
	width = min(width, image.Width)
	canvas.DrawRegion(image, 0, 0, width, frameHeight, barX+image.OffsetX, barY+image.OffsetY)
	return true
}

// drawDesireValue 在渐变条中央画百分比或 MAX。
//
// 位置：数字 + 单位整体水平居中到 bar 的中央，垂直居中到 bar 的中央。
// 这比"固定坐标"更稳：不同作品的 bar 宽度可以不同，而原引擎也是相对
// 进度条定位的。
func (s *Session) drawDesireValue(canvas *gfx.BgraCanvas, barX, barY int, row config.ExtraWindowInfoRow, value, maxValue int, numberImage, unitImage *gfx.BsgImage, numberFrames, unitFrames int) bool {
	if numberImage == nil || unitImage == nil {
		return false
	}
	numberFrames = max(1, numberFrames)
	unitFrames = max(1, unitFrames)
	digitWidth := numberImage.Width
	digitHeight := numberImage.Height / numberFrames
	unitWidth := unitImage.Width
	unitHeight := unitImage.Height / unitFrames
	if digitWidth <= 0 || digitHeight <= 0 || unitWidth <= 0 || unitHeight <= 0 {
		return false
	}
	barWidth := 0
	barHeight := 12
	if row.BarImage != "" {
		if image := s.composer.Load(row.BarImage); image != nil {
			barWidth = image.Width
			if frames := max(1, row.BarFrames); image.Height/frames > 0 {
				barHeight = image.Height / frames
			}
		}
	}
	if barWidth <= 0 {
		return false
	}

	full := value >= maxValue
	digits := ""
	unitFrame := 0
	if !full {
		percent := value * 100 / maxValue
		digits = strconv.Itoa(percent)
	} else {
		unitFrame = min(1, unitFrames-1)
	}

	// unit 每帧是 34x12，但 "%" 只占帧内右侧的一小块，左边是透明留白。
	// 原引擎画 "12%" 时没有那段空隙，所以这里取该帧的 alpha 包围盒，
	// 只贴出可见字形；MAX 帧的包围盒本来就是整帧 34x12。
	glyphX, glyphY, glyphWidth, glyphHeight, ok := extraFrameGlyphBounds(unitImage, unitFrame, unitFrames)
	if !ok {
		return false
	}

	// 原引擎把数字按约 2/3 的宽度横向压缩（高度不变）再贴到条上：
	// 手册截图里 "12%" 只占约 25px，而 12x12 的三帧原尺寸要 33px。
	// 只压横向是关键——等比缩小会让字变得又小又低，和原版高度对不上。
	digitDrawWidth := max(1, digitWidth*2/3)
	totalWidth := len(digits)*digitDrawWidth + glyphWidth
	x := barX + (barWidth-totalWidth)/2
	y := barY + (barHeight-glyphHeight)/2
	if y < barY {
		y = barY
	}

	if len(digits) > 0 {
		for index := 0; index < len(digits); index++ {
			digit := int(digits[index] - '0')
			if digit < 0 || digit >= numberFrames {
				continue
			}
			canvas.BlitScaledRegion(numberImage,
				x+index*digitDrawWidth, y, digitDrawWidth, digitHeight, 255,
				0, digit*digitHeight, digitWidth, digitHeight)
		}
	}
	canvas.DrawRegion(unitImage, glyphX, glyphY, glyphWidth, glyphHeight,
		x+len(digits)*digitDrawWidth, barY+(barHeight-glyphHeight)/2)
	return true
}

// extraFrameGlyphBounds 返回竖排图某一帧里非透明像素的外接矩形。
//
// 只用于 exwindow_unit 这种"帧内留白很大"的字形图：不裁掉留白，
// "%" 和前面的数字之间会出现 20px 的空洞，看起来像两个独立文本。
func extraFrameGlyphBounds(image *gfx.BsgImage, frame, frames int) (int, int, int, int, bool) {
	if image == nil || frames <= 0 || frame < 0 || frame >= frames {
		return 0, 0, 0, 0, false
	}
	frameHeight := image.Height / frames
	if frameHeight <= 0 {
		return 0, 0, 0, 0, false
	}
	top := frame * frameHeight
	stride := image.Stride()
	left, right := image.Width, -1
	upper, lower := frameHeight, -1
	for y := 0; y < frameHeight; y++ {
		row := (top + y) * stride
		for x := 0; x < image.Width; x++ {
			if image.Pixels[row+x*4+3] == 0 {
				continue
			}
			left = min(left, x)
			right = max(right, x)
			upper = min(upper, y)
			lower = max(lower, y)
		}
	}
	if right < left || lower < upper {
		return 0, 0, 0, 0, false
	}
	return left, top + upper, right - left + 1, lower - upper + 1, true
}

// drawExtraStripAt 画一条竖排图的第一帧（infoN 通常是 1 帧）。
func (s *Session) drawExtraStripAt(canvas *gfx.BgraCanvas, name string, frames, x, y int) bool {
	if canvas == nil || s.composer == nil {
		return false
	}
	image := s.composer.Load(name)
	if image == nil {
		return false
	}
	count := max(1, frames)
	height := image.Height / count
	if height <= 0 {
		return false
	}
	canvas.DrawRegion(image, 0, 0, image.Width, height, x+image.OffsetX, y+image.OffsetY)
	return true
}

// extraMeterValue 读出等级面板当前应显示的值。
//
// 剧本 `set levelmeter #point` 绑定的是变量名（UK/UKIE 都是 #point），
// 它优先于配置里的默认值；没有绑定或变量不存在时返回 0。
func (s *Session) extraMeterValue(screen config.ExtraWindowScreen) int {
	name := ""
	if s.gameState != nil {
		name = s.gameState.ExtraMeterProperty()
	}
	if len(name) == 0 {
		name = screen.LevelProperty
	}
	if len(name) == 0 || s.script == nil || s.machine == nil {
		return 0
	}
	index := s.script.GetPropertyIndex(name)
	if index < 0 {
		return 0
	}
	value := int(s.machine.Property(index))
	if value < 0 {
		return 0
	}
	return value
}

// extraMeterFrame 把等级值换算成计量表帧号。
//
// 剧本里的 #point 是 0…6 的等级值（7 帧等级图），圆环计量表是 13 帧；
// 按等级范围线性放大：0→0、1→2、…、6→12，满级时圆环正好全亮。
// 没有等级图时退回按计量表帧数直接夹取，保持旧作品的行为。
func extraMeterFrame(value, levelFrames, meterFrames int) int {
	if meterFrames <= 1 {
		return 0
	}
	if levelFrames <= 1 {
		return core.Clamp(value, 0, meterFrames-1)
	}
	frame := value * (meterFrames - 1) / (levelFrames - 1)
	return core.Clamp(frame, 0, meterFrames-1)
}

// extraWindowShown 判断面板当前是否可见。
//
// 配置语义：auto hide = 0 表示"永不自动收起"，面板常驻；auto hide > 0 表示
// 指针离开显示区后超过该毫秒数才收起，避免鼠标一抖面板就闪掉。
func (s *Session) extraWindowShown(autoHideMS int, area [4]int) bool {
	if area[2] <= 0 || area[3] <= 0 {
		return true
	}
	inside := s.lastPointerX >= area[0] && s.lastPointerX < area[0]+area[2] &&
		s.lastPointerY >= area[1] && s.lastPointerY < area[1]+area[3]
	if inside {
		s.extraWindowSeenAt = time.Now()
	}
	if autoHideMS <= 0 {
		return true
	}
	return time.Since(s.extraWindowSeenAt) < time.Duration(autoHideMS)*time.Millisecond
}

// drawFreeSight 画自由视角的滑杆和三个定位按钮。
//
// 只有 [free-sight screen] mode=1 且玩家这次对话里按过「顔/胸/股間」才显示：
// 原版是靠工具条的三个视点按钮进入的，关掉对话框就回到普通画面。用
// freeSightActive 这个会话级开关表达"玩家正在看立绘"，而不是拿 ViewMode
// 判断——ViewMode 的默认值 0 和"看脸"是同一个值，分不开。
func (s *Session) drawFreeSight(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.composer == nil || s.config == nil || s.gameState == nil {
		return false
	}
	view := s.config.FreeSight
	if !view.Mode || !s.freeSightActive {
		return false
	}
	left, top, width, height := view.Meter[0], view.Meter[1], view.Meter[2], view.Meter[3]
	if width <= 0 || height <= 0 {
		return false
	}
	// 轨道在原版是底图上的一条细线；这里铺一层半透明黑底表示可拖动区域，
	// 没有额外素材也不会盖住立绘。
	canvas.FillBlend(left, top, width, height, 0, 0, 0, 90)
	mode := s.gameState.ViewMode()
	if !view.Knob.Empty() {
		if image := s.composer.Load(view.Knob.Image); image != nil {
			knobWidth, knobHeight := image.Width, image.Height
			if frames := max(1, view.Knob.Frames); frames > 1 && knobHeight%frames == 0 {
				knobHeight /= frames
			}
			x := left + (width-knobWidth)/2
			y := top + (height-knobHeight)*mode/2
			if y < top {
				y = top
			}
			if y > top+height-knobHeight {
				y = top + height - knobHeight
			}
			canvas.DrawRegion(image, 0, 0, knobWidth, knobHeight, x, y)
		}
	}
	for _, button := range []config.UIButton{view.Face, view.Bust, view.Hip} {
		if !button.Empty() {
			s.drawFrameButton(canvas, button, 0)
		}
	}
	return true
}

// freeSightToggle 处理工具条的「顔/胸/股間」点击：进入自由视角并记录视点。
//
// 再次点同一个按钮就退出自由视角，和原版"按钮点亮/熄灭"的观感一致。
func (s *Session) freeSightToggle(mode int) bool {
	if s.config == nil || !s.config.FreeSight.Mode {
		return false
	}
	if s.freeSightActive && s.gameState.ViewMode() == mode {
		s.freeSightActive = false
		s.composer.Invalidate()
		return true
	}
	s.freeSightActive = true
	s.gameState.SetViewMode(mode)
	s.composer.Invalidate()
	return true
}
