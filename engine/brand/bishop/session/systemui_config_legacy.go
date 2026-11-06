package session

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_config_legacy.go 是 bsxx.cfg 旧式 [config screen] 的引擎实现。
//
// 为什么不在 Flutter 里做：这一屏的底图、cfg_* 按钮、勾选/单选图标和每一条
// 滑杆的坐标都由作品自己的配置给出；宿主重画一套会与原作错位，而且作品之间
// 的角色数（4/7）与音量通道数不同。引擎按坐标画，换作品只换配置。
//
// 绘制与命中共用 legacyControls：控件图有多帧、滑杆的命中区与底图不完全一致，
// 两处各算一次几何必然漂移。

type legacyConfigPage int

const (
	legacyPageMain legacyConfigPage = iota
	legacyPageEffect
	legacyPageSound
	legacyPageLiquid
)

type legacyControlKind int

const (
	legacyControlScreenMode legacyControlKind = iota
	legacyControlSquareMode
	legacyControlAllEffect
	legacyControlCeaseSkip
	legacyControlSkipUnread
	legacyControlVoiceLoop
	legacyControlSkipMode
	legacyControlEffectDetail
	legacyControlLiquidDetail
	legacyControlLiquidType
	legacyControlAutoSelect
	// legacyControlBgmSelect 是旧式设置里的 BGM曲調（ノーマル/ハード）。
	legacyControlBgmSelect
	// legacyControlRadioGroup 是配置里新增的通用互斥组（画面効果/計量/射精位置）。
	legacyControlRadioGroup
	legacyControlSpeed
	legacyControlVoiceMode
	legacyControlColor
	legacyControlVolume
	legacyControlCharacter
	legacyControlCharacterDetail
	legacyControlReset
	legacyControlBack
	legacyControlTitle
	legacyControlExit
	legacyControlEffectToggle
	legacyControlSoundVolume
	legacyControlVoicePlay
	// 旧式设置画面底部的记录/读入/环境设定页签。
	legacyControlModeSave
	legacyControlModeLoad
	legacyControlModeConfig
	// legacyControlValueGroup 是取值型选项（GK2 设置）：每个按钮代表一个设置值，
	// 当前值的按钮画第 2 帧。它的 groupKey/option 决定写回哪个设置字段。
	legacyControlValueGroup
	// legacyControlExtraToggle 是 ExtraToggles 里的作品专属复选框，
	// groupKey 是设置键名（anti alias / cutin …）。
	legacyControlExtraToggle
)

type legacyControl struct {
	kind legacyControlKind
	// index 的含义随 kind 变化：滑杆通道、颜色分量、角色号、开关序号。
	index int
	// gauge 为 true 时用 value 画滑杆，x/y/w/h 是整条滑杆的命中区。
	gauge bool
	x, y  int
	w, h  int
	// button 是可绘制/可命中的多帧按钮；toggle 用 check/radio 图标。
	button config.UIButton
	// vertical/knob/trackHeight 是竖向滑杆：底图画好轨道，只画滑块。
	vertical    bool
	knob        config.UIButton
	trackHeight int
	// valueFrame 表示 button 是多帧"值图"（Tbxdl cfg_item4/6），按当前值取帧。
	valueFrame bool
	// toggle 表示这个控件画勾选/单选图标。
	toggle bool
	radio  bool
	// explicit 为 true 时，这是 ON/OFF 两点之一：value 是这一点代表的设置值，
	// 勾选状态 = 设置值 == value，而不是对设置取反。KD 把开关画成两个独立点。
	// markFrame 选择 cfg_mark 的颜色帧；零值即绿色，与旧作品保持一致。
	explicit  bool
	value     bool
	markFrame int
	// delta 非 0 表示这是滑杆两端的"减少/增加"按钮。
	delta int
	// dots 是离散刻度；非空时按点击/当前值选点。
	dots    []config.UIPoint
	dotMark config.UIButton
	// markerOnly 表示底图已画好横条，只画当前位置的竖线。
	markerOnly bool
	trackWidth int
	// groupKey/option 用于通用单选组：key 决定设置字段，option 是本项的值。
	groupKey string
	option   int
	// hitWidth 是勾选/单选控件的横向命中宽度；<=28 时只命中图标。
	hitWidth int
	// on 返回当前是否勾选/单选中；gauge 时由 value 返回 0..max。
	label string
	// fill 为 true 时 Bar 按值裁剪宽度（GK2 音量条的底槽已由底图画好）。
	fill bool
	// horizontalKnob 为 true 时滑块沿 trackWidth 横向移动，knobW 是滑块宽度。
	horizontalKnob bool
	knobW          int
	// steps 大于 0 时把取值量化到 steps 档（速度条为 12 档）。
	steps int
}

type legacyConfigState struct {
	page legacyConfigPage
	// character 是"詳細音声"页当前编辑的角色。
	character int
	// controls 是当前页面按绘制顺序排列的控件。
	controls []legacyControl
	// dragging 是正在拖动的滑杆控件下标；-1 表示没有。
	dragging int
	hover    int
}

// openLegacyConfigScreen 打开旧式设置画面。
func (s *Session) openLegacyConfigScreen() bool {
	layout := s.config.LegacyConfig
	if layout.Empty() {
		return false
	}
	if s.gameState == nil || s.composer == nil || !s.canOpenSystemUI() {
		return false
	}
	s.beginSystemUI(uiLegacyConfig)
	state := &legacyConfigState{dragging: -1, hover: -1}
	s.ui.legacy = state
	s.ui.page = 0
	s.buildLegacyControls()
	core.Logf("ui: legacy config screen opened (%d control(s))", len(state.controls))
	return true
}

func (s *Session) legacyState() *legacyConfigState {
	if s.ui == nil {
		return nil
	}
	if s.ui.legacy == nil {
		s.ui.legacy = &legacyConfigState{dragging: -1, hover: -1}
	}
	return s.ui.legacy
}

// buildLegacyControls 依据当前页和设置值重建控件。值本身不缓存：设置存在
// s.settings，读的时候现取，滑杆拖动/重置后不需要同步第二份数据。
func (s *Session) buildLegacyControls() {
	state := s.legacyState()
	if state == nil {
		return
	}
	layout := s.config.LegacyConfig
	state.controls = state.controls[:0]
	// cfg_check / cfg_radio 是 12x12、14x14 的**单帧**图标：底图已经画好
	// 空方框/空心圆，引擎只在"选中"时把勾/圆点叠上去。旧实现把它们当
	// 两帧竖排图切，结果只画到图标的上/下半块（显示成一团红块）。
	checkFrames := max(1, layout.CheckMarkFrames)
	radioFrames := max(1, layout.RadioFrames)
	checkbox := config.UIButton{Image: layout.CheckMark, Frames: checkFrames}
	radio := config.UIButton{Image: layout.RadioButton, Frames: radioFrames}
	// 精灵图世代：勾选/单选是 config_base 里的一个矩形，Image 也是
	// config_base；直接沿用整张图会画出整张设置页。
	if !layout.CheckMarkAtlas.Empty() {
		checkbox = layout.CheckMarkAtlas
		checkbox.Frames = checkFrames
	}
	if !layout.RadioButtonAtlas.Empty() {
		radio = layout.RadioButtonAtlas
		radio.Frames = radioFrames
	}

	// addMarkedToggle 是所有勾选/单选的底层构造。explicit 为 true 时，这个点只代表
	// ON 或 OFF 其中之一（value），勾选状态按设置值比对，而不是取反；markFrame
	// 选择 cfg_mark 的颜色帧。
	// 返回是否真的追加了控件。调用方常紧接着写 index/groupKey；如果点
	// 是空的 (0,0)，不能去改上一条控件的字段（首条时会直接越界 panic）。
	addMarkedToggle := func(kind legacyControlKind, point config.UIPoint, isRadio, explicit, value bool, markFrame int) bool {
		if point.X == 0 && point.Y == 0 {
			return false
		}
		button := checkbox
		if isRadio {
			button = radio
		}
		// drawFrameButton/hitButton 都从 button.X/Y 取绘制原点；
		// 勾选图标只有配置坐标，不写回按钮会全部画到 (0,0)。
		button.X = point.X
		button.Y = point.Y
		state.controls = append(state.controls, legacyControl{
			kind: kind, toggle: true, radio: isRadio,
			x: point.X, y: point.Y, button: button,
			explicit: explicit, value: value, markFrame: markFrame,
			hitWidth: layout.ToggleHitWidth,
		})
		return true
	}
	// 单点开关：点的位置即开启态，保持原有作品的行为。
	addToggle := func(kind legacyControlKind, point config.UIPoint, isRadio bool) {
		addMarkedToggle(kind, point, isRadio, false, false, 0)
	}
	// ON/OFF 分两点画：这里的 point 代表 value 所对应的那一态。
	addExplicitToggle := func(kind legacyControlKind, point config.UIPoint, value bool, markFrame int) {
		addMarkedToggle(kind, point, false, true, value, markFrame)
	}
	addButton := func(kind legacyControlKind, index int, button config.UIButton) {
		// 纯热区按钮（SPMM 的 SAVE/LOAD/CONFIG 页签与页脚）没有独立
		// 图片：文字/边框已经烘焙在底图里，配置只给出 Hit 矩形。
		// 旧实现把 Empty 直接当成"没有这个按钮"，这些页签会全部点不动。
		if button.Empty() {
			if button.Hit[2] <= 0 || button.Hit[3] <= 0 {
				return
			}
		}
		state.controls = append(state.controls, legacyControl{
			kind: kind, index: index, button: button,
			x: button.X, y: button.Y,
		})
	}
	addGauge := func(kind legacyControlKind, index int, gauge config.LegacyGauge) {
		if gauge.HorizontalKnob {
			// GK2 速度条：底图已画好轨道，滑块沿 TrackWidth 横向移动。命中区覆盖整条轨道，
			// 拖动时按指针 x 取比例，所以滑块中心要减去半个滑块宽度才能与指针对齐。
			knob := gauge.Knob
			if knob.Empty() || gauge.TrackWidth <= 0 {
				return
			}
			knobW, knobH := 20, 22
			if image := s.composer.Load(knob.Image); image != nil {
				knobW = image.Width
				knobH = image.Height / max(1, knob.Frames)
			}
			if gauge.TrackWidth <= knobW {
				return
			}
			state.controls = append(state.controls, legacyControl{
				kind: kind, index: index, gauge: true, horizontalKnob: true,
				x: knob.X, y: knob.Y - 4, w: gauge.TrackWidth, h: knobH + 8,
				button: knob, knob: knob, knobW: knobW, trackWidth: gauge.TrackWidth,
				steps: gauge.Steps,
			})
			return
		}
		if gauge.Vertical {
			// 竖向滑杆：底图已经画好轨道，只画 cfg_knob 滑块。
			// knob 的 X/Y 在配置里表示轨道左上角；TrackHeight 是轨道高度。
			knob := gauge.Knob
			if knob.Empty() {
				return
			}
			knobW := 20
			if image := s.composer.Load(knob.Image); image != nil {
				knobW = image.Width
			}
			trackHeight := gauge.TrackHeight
			if trackHeight <= 0 {
				trackHeight = 180
			}
			trackX, trackY := knob.X, knob.Y
			if !gauge.Bar.Empty() {
				trackX, trackY = gauge.Bar.X, gauge.Bar.Y
			}
			state.controls = append(state.controls, legacyControl{
				kind: kind, index: index, gauge: true, vertical: true,
				x: trackX, y: trackY, w: max(24, knobW), h: trackHeight,
				button: knob, knob: knob, trackHeight: trackHeight,
				valueFrame: gauge.ValueFrame,
			})
			step := 50
			switch kind {
			case legacyControlColor:
				step = 5
			case legacyControlSpeed:
				step = 5
			}
			if !gauge.Min.Empty() {
				addButton(kind, index, gauge.Min)
				if len(state.controls) > 0 {
					state.controls[len(state.controls)-1].delta = -step
				}
			}
			if !gauge.Max.Empty() {
				addButton(kind, index, gauge.Max)
				if len(state.controls) > 0 {
					state.controls[len(state.controls)-1].delta = step
				}
			}
			return
		}
		if len(gauge.Dots) > 0 {
			// 离散刻度：命中区覆盖所有点，点击时按最近的点取整。
			left, top := gauge.Dots[0].X, gauge.Dots[0].Y
			right, bottom := left, top
			for _, point := range gauge.Dots {
				left = min(left, point.X)
				top = min(top, point.Y)
				right = max(right, point.X)
				bottom = max(bottom, point.Y)
			}
			// 点图是 16x16 的 cfg_mark，向外各扩 4px；不能扩得太多，
			// 否则会自动速度那行会盖住下方"未読もスキップ"复选框。
			state.controls = append(state.controls, legacyControl{
				kind: kind, index: index, gauge: true,
				x: left - 4, y: top - 4, w: right - left + 24, h: bottom - top + 24,
				dots: append([]config.UIPoint(nil), gauge.Dots...), dotMark: gauge.DotMark,
			})
			return
		}
		if gauge.MarkerOnly {
			// 底图已经画好横条（AFDVD 的文字背景/音量），只画当前位置竖线。
			w := gauge.TrackWidth
			h := gauge.TrackHeight
			if w <= 0 {
				w = 200
			}
			if h <= 0 {
				h = 16
			}
			state.controls = append(state.controls, legacyControl{
				kind: kind, index: index, gauge: true,
				x: gauge.Knob.X, y: gauge.Knob.Y - 8, w: w, h: h + 16,
				markerOnly: true, trackWidth: w,
			})
			step := 50
			switch kind {
			case legacyControlColor:
				step = 5
			case legacyControlSpeed:
				step = 5
			}
			if !gauge.Min.Empty() {
				addButton(kind, index, gauge.Min)
				if len(state.controls) > 0 {
					state.controls[len(state.controls)-1].delta = -step
				}
			}
			if !gauge.Max.Empty() {
				addButton(kind, index, gauge.Max)
				if len(state.controls) > 0 {
					state.controls[len(state.controls)-1].delta = step
				}
			}
			return
		}
		if gauge.Bar.Empty() {
			return
		}
		image := s.composer.Load(gauge.Bar.Image)
		w, h := 120, 16
		if gauge.Bar.Atlas && gauge.Bar.Clip[2] > 0 && gauge.Bar.Clip[3] > 0 {
			w, h = gauge.Bar.Clip[2], gauge.Bar.Clip[3]
		} else if image != nil {
			w = image.Width
			h = image.Height / max(1, gauge.Bar.Frames)
		}
		// 命中区向上下各扩 6px：滑杆图很薄，手指直接点细线很难。
		state.controls = append(state.controls, legacyControl{
			kind: kind, index: index, gauge: true,
			x: gauge.Bar.X, y: gauge.Bar.Y - 6, w: w, h: h + 12, button: gauge.Bar, fill: gauge.Fill,
			valueFrame: gauge.ValueFrame,
		})
		step := 50
		switch kind {
		case legacyControlColor:
			step = 5
		case legacyControlSpeed:
			step = 5
		}
		if !gauge.Min.Empty() {
			addButton(kind, index, gauge.Min)
			if len(state.controls) > 0 {
				state.controls[len(state.controls)-1].delta = -step
			}
		}
		if !gauge.Max.Empty() {
			addButton(kind, index, gauge.Max)
			if len(state.controls) > 0 {
				state.controls[len(state.controls)-1].delta = step
			}
		}
	}

	switch state.page {
	case legacyPageEffect:
		page := layout.Effect
		if page == nil {
			state.page = legacyPageMain
			s.buildLegacyControls()
			return
		}
		for index, toggle := range page.Toggles {
			point := config.UIPoint{X: toggle.X, Y: toggle.Y}
			// 每个键是一个布尔设置，绘制时按 page.Toggles 的顺序读 state。
			_ = toggle.Key
			if addMarkedToggle(legacyControlEffectToggle, point, false, false, false, 0) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		addButton(legacyControlBack, 0, page.Back)
	case legacyPageSound:
		page := layout.Sound
		if page == nil {
			state.page = legacyPageMain
			s.buildLegacyControls()
			return
		}
		addToggle(legacyControlVoicePlay, page.VoicePlay, false)
		for index, gauge := range page.Volumes {
			addGauge(legacyControlSoundVolume, index, gauge)
		}
		addButton(legacyControlBack, 0, page.Back)
	case legacyPageLiquid:
		page := layout.Liquid
		if page == nil {
			state.page = legacyPageMain
			s.buildLegacyControls()
			return
		}
		// 两个单选点代表“なし/通常”，用 cfg_radio（或 check mark）叠在底图上。
		for index, point := range page.Options {
			if addMarkedToggle(legacyControlLiquidType, point, true, true, index != 0, 0) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		addButton(legacyControlBack, 0, page.Back)
	default:
		addToggle(legacyControlScreenMode, layout.ScreenMode, false)
		addToggle(legacyControlSquareMode, layout.SquareMode, false)
		addToggle(legacyControlAllEffect, layout.AllEffect, false)
		addToggle(legacyControlCeaseSkip, layout.CeaseSkip, false)
		// VoiceLoop/SkipUnread 配了 OFF 点时画成两个独立点，各自按设置值决定是否画标记；
		// 否则退回单点勾选，保持其它作品的行为。
		if layout.SkipUnreadOff.X != 0 || layout.SkipUnreadOff.Y != 0 {
			addExplicitToggle(legacyControlSkipUnread, layout.SkipUnread, true, 0)
			addExplicitToggle(legacyControlSkipUnread, layout.SkipUnreadOff, false, 0)
		} else {
			addToggle(legacyControlSkipUnread, layout.SkipUnread, false)
		}
		if layout.VoiceLoopOff.X != 0 || layout.VoiceLoopOff.Y != 0 {
			addExplicitToggle(legacyControlVoiceLoop, layout.VoiceLoop, true, 0)
			addExplicitToggle(legacyControlVoiceLoop, layout.VoiceLoopOff, false, 0)
		} else {
			addToggle(legacyControlVoiceLoop, layout.VoiceLoop, false)
		}
		for _, toggle := range layout.ExtraToggles {
			point := config.UIPoint{X: toggle.X, Y: toggle.Y}
			if addMarkedToggle(legacyControlExtraToggle, point, false, false, false, 0) {
				state.controls[len(state.controls)-1].groupKey = toggle.Key
			}
		}
		for index, point := range layout.SkipModes {
			// スキップ処理的标记帧由配置指定；AFDVD 用第 1 帧的圆点。
			if addMarkedToggle(legacyControlSkipMode, point, true, false, false, layout.SkipModeMarkFrame) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		for index, point := range layout.AutoSelect {
			if addMarkedToggle(legacyControlAutoSelect, point, true, false, false, layout.AutoSelectMarkFrame) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		for index, point := range layout.BgmSelectors {
			if addMarkedToggle(legacyControlBgmSelect, point, true, false, false, 0) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		for index, point := range layout.VoiceMode {
			if addMarkedToggle(legacyControlVoiceMode, point, true, false, false, 0) {
				state.controls[len(state.controls)-1].index = index
			}
		}
		for index, gauge := range layout.Speeds {
			addGauge(legacyControlSpeed, index, gauge)
		}
		for index, gauge := range layout.Colors {
			addGauge(legacyControlColor, index, gauge)
		}
		for index, gauge := range layout.Volumes {
			addGauge(legacyControlVolume, index, gauge)
		}
		for index, character := range layout.Characters {
			addButton(legacyControlCharacter, index, character.Button)
			addButton(legacyControlCharacterDetail, index, character.Detail)
		}
		for groupIndex, group := range layout.RadioGroups {
			for option, point := range group.Points {
				if !addMarkedToggle(legacyControlRadioGroup, point, true, false, false, group.MarkFrame) {
					continue
				}
				control := &state.controls[len(state.controls)-1]
				control.index = groupIndex
				control.groupKey = group.Key
				control.option = option
				control.hitWidth = group.HitWidth
			}
		}
		// 取值组每个按钮独立成控件；option 是它代表的设置值，命中后写回对应字段。
		for _, group := range layout.ValueGroups {
			for _, option := range group.Options {
				if option.Button.Empty() {
					continue
				}
				state.controls = append(state.controls, legacyControl{
					kind: legacyControlValueGroup, button: option.Button,
					x: option.Button.X, y: option.Button.Y,
					groupKey: group.Key, option: option.Value,
				})
			}
		}
		// 纯热区页签没有图片，不能沿用 Empty 判断，否则 SPMM 的
		// SAVE/LOAD/CONFIG 三个入口全部不会进入控件表，点击无效。
		addButton(legacyControlModeSave, 0, layout.ModeSave)
		addButton(legacyControlModeLoad, 0, layout.ModeLoad)
		addButton(legacyControlModeConfig, 0, layout.ModeConfig)
		addButton(legacyControlEffectDetail, 0, layout.EffectDetail)
		addButton(legacyControlLiquidDetail, 0, layout.LiquidDetail)
		addButton(legacyControlReset, 0, layout.Reset)
		addButton(legacyControlBack, 0, layout.Back)
		addButton(legacyControlTitle, 0, layout.Title)
		addButton(legacyControlExit, 0, layout.Exit)
	}
}

// ---------------------------------------------------------------- 绘制

func (s *Session) drawLegacyConfigScreen(canvas *gfx.BgraCanvas) {
	state := s.legacyState()
	if state == nil {
		return
	}
	layout := s.config.LegacyConfig
	// 整屏背景先铺：MK 的设置内容 cfg_ex 只有 600x404，白色装饰底图
	// config_base 必须垫在下面，否则面板周围会直接露出游戏画面。
	if len(layout.Backdrop) > 0 {
		s.drawImageAt(canvas, layout.Backdrop, 0, 0)
	}
	// 左上角 plaque 的大号模式名；设置页固定 CONFIG 帧。
	if !layout.TitleLabel.Empty() {
		s.drawFrameButton(canvas, layout.TitleLabel, 0)
	}
	switch state.page {
	case legacyPageEffect:
		if layout.Effect != nil {
			s.drawLayerAt(canvas, layout.Effect.Base, 0, 0)
		}
	case legacyPageSound:
		if layout.Sound != nil {
			s.drawLayerAt(canvas, layout.Sound.Base, 0, 0)
		}
	case legacyPageLiquid:
		if layout.Liquid != nil {
			s.drawLayerAt(canvas, layout.Liquid.Base, 0, 0)
		}
	default:
		// 主设置页的 Base 允许偏移（MK 的 cfg_ex 是 600x404，原版放在
		// 182,98 以避开 config_base 左上角的装饰）；零值仍是 (0,0)。
		s.drawLayerAt(canvas, layout.Base, layout.BaseX, layout.BaseY)
	}
	// 左侧菜单覆盖图（201x600）必须画在 Base 之后：SPMM 的 config_frame
	// 自带整屏底与页脚，菜单图单独叠在左栏；先画会被 Base 整块盖住。
	// 零值保持旧行为：不叠菜单。
	s.drawImageAt(canvas, layout.Menu, 0, 0)

	for index, control := range state.controls {
		hover := s.ui != nil && s.ui.hover.kind == hitLegacyConfigControl && s.ui.hover.index == index
		switch {
		case control.gauge:
			s.drawLegacyGauge(canvas, control)
		case control.kind == legacyControlValueGroup:
			// 当前值画第 2 帧（选中），悬停画第 1 帧，其余为常态。
			// Exclusive 组（MDDVD 的画/未読/スキップ速度）的选项共用同一个
			// 显示位置：只能画当前值和悬停项，否则 ON/OFF 两张图会叠在一起。
			frame := 0
			selected := s.legacyValueGroupValue(control.groupKey) == control.option
			if selected {
				frame = 2
			} else if hover {
				frame = 1
			}
			if group, ok := s.legacyValueGroup(control.groupKey); ok && group.Exclusive && !selected && !hover {
				break
			}
			s.drawFrameButton(canvas, control.button, frame)
		case control.toggle:
			on := s.legacyToggleOn(control)
			button := control.button
			if hover {
				// 勾选图标很小，悬停时给一个浅色底提示可点。
				canvas.Fill(control.x-4, control.y-4, max(24, s.buttonWidth(button)+8), 24, 80, 80, 80, 90)
			}
			// 未选中不画：底图上已经有空方框/空心圆，再叠一个同样的图标
			// 只会让"选中"和"未选中"看起来一样。
			if on {
				s.drawFrameButton(canvas, button, control.markFrame)
			}
		default:
			frame := 0
			if hover {
				frame = 1
			}
			if control.kind == legacyControlCharacter && layout.CharacterOffFrame >= 0 {
				// KD：第 0 帧彩色头像、第 1 帧灰头像+红叉。按语音开关画，
				// 不能把"当前选中角色"画成红叉。
				frame = 0
				if s.legacyToggleOn(control) && layout.CharacterSelectedFrame > 0 && s.legacyCharacterSelected(control.index) {
					frame = layout.CharacterSelectedFrame
				} else if !s.legacyToggleOn(control) {
					frame = clampInt(layout.CharacterOffFrame, 0, max(1, control.button.Frames)-1)
				}
			} else if control.kind == legacyControlCharacter && s.legacyCharacterSelected(control.index) {
				// 其它作品的第 1 帧是选中态。
				frame = 1
			}
			s.drawFrameButton(canvas, control.button, frame)
		}
	}
	if state.page == legacyPageMain {
		s.drawLegacyOptions(canvas)
		s.drawLegacyRadioLabels(canvas)
	}
}

// drawLegacyRadioLabels 画配置里没有烤进底图的选项文字。
//
// AFDVD 的"射精位置"面板底图只有两个黑框，10 个选项名完全由引擎绘制；
// 其它作品不配 Labels，这段不会执行。
func (s *Session) drawLegacyRadioLabels(canvas *gfx.BgraCanvas) {
	if s.textFont == nil || s.config == nil {
		return
	}
	for _, group := range s.config.LegacyConfig.RadioGroups {
		if len(group.Labels) == 0 {
			continue
		}
		for index, label := range group.Labels {
			if index >= len(group.Points) {
				break
			}
			point := group.Points[index]
			image := s.textFont.Render(label, 14, 1, 1, 18, 140, 1, 0xffffffff)
			if image != nil {
				rendering.DrawTextImage(canvas, image, point.X+20, point.Y+1)
			}
		}
	}
}

// drawLegacyOptions 画 Tbxdl 设置画面的"值图"选项。
//
// 这些图本身就是一组状态（OFF/ON、通常/高速/選択肢まで、Window/全画面），
// 底图只画标签；引擎按当前设置取对应帧，玩家一眼就能看到当前值。
func (s *Session) drawLegacyOptions(canvas *gfx.BgraCanvas) {
	layout := s.config.LegacyConfig
	settings := s.settings
	draw := func(button config.UIButton, frame, frames int) {
		if button.Empty() {
			return
		}
		s.drawFrameButton(canvas, button, clampInt(frame, 0, max(0, frames-1)))
	}
	boolFrame := func(value bool) int {
		if value {
			return 1
		}
		return 0
	}
	draw(layout.SkipUnreadButton, boolFrame(settings.SkipUnread), 3)
	draw(layout.SkipSpeedButton, settings.SkipMode, 3)
	draw(layout.VoiceRepeatButton, boolFrame(settings.VoiceLoop), 2)
	draw(layout.ScreenModeButton, boolFrame(settings.ScreenMode == "full"), 2)
	draw(layout.ScreenEffectButton, boolFrame(settings.AllEffect), 2)
	draw(layout.JuiceButton, boolFrame(settings.Steam), 2)
	// 11 帧值条：0..10 对应 0..100，四舍五入到最近一帧。
	drawBar := func(button config.UIButton, value int) {
		if button.Empty() {
			return
		}
		frames := max(2, button.Frames)
		frame := int(float64(core.Clamp(value, 0, 100))/100*float64(frames-1) + 0.5)
		s.drawFrameButton(canvas, button, clampInt(frame, 0, frames-1))
	}
	drawBar(layout.TextSpeedBar, settings.MessageSpeed)
	drawBar(layout.AutoSpeedBar, settings.ClickWait)
}

// hitLegacyOptionButton 返回命中的 Tbxdl 值图按钮下标；-1 表示没有命中。
// 顺序与 activateLegacyOption 的 switch 一致。
func (s *Session) hitLegacyOptionButton(x, y int) int {
	layout := s.config.LegacyConfig
	buttons := [...]config.UIButton{
		layout.SkipUnreadButton,
		layout.SkipSpeedButton,
		layout.VoiceRepeatButton,
		layout.ScreenModeButton,
		layout.ScreenEffectButton,
		layout.JuiceButton,
	}
	for index, button := range buttons {
		if hitButton(s, button, x, y) {
			return index
		}
	}
	return -1
}

// activateLegacyOption 循环切换一个值图按钮对应的设置。
func (s *Session) activateLegacyOption(index int) {
	switch index {
	case 0:
		s.settings.SkipUnread = !s.settings.SkipUnread
	case 1:
		s.settings.SkipMode = (s.settings.SkipMode + 1) % 3
	case 2:
		s.settings.VoiceLoop = !s.settings.VoiceLoop
	case 3:
		if s.settings.ScreenMode == "full" {
			s.settings.ScreenMode = "window"
		} else {
			s.settings.ScreenMode = "full"
		}
	case 4:
		s.settings.AllEffect = !s.settings.AllEffect
	case 5:
		s.settings.Steam = !s.settings.Steam
	default:
		return
	}
	s.saveGameSettings()
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

func (s *Session) buttonWidth(button config.UIButton) int {
	if button.Empty() {
		return 0
	}
	if image := s.composer.Load(button.Image); image != nil {
		return image.Width
	}
	return 28
}

// drawLegacyGauge 画一条滑杆：底条 + 当前位置的滑块标记。
//
// 原作没有单独的滑块素材，只有 bar/min/max 三张图；用一条亮色竖线表示当前
// 值，既不会盖住底条的刻度，也能一眼看出音量/速度的位置。
func (s *Session) drawLegacyGauge(canvas *gfx.BgraCanvas, control legacyControl) {
	if control.horizontalKnob {
		// 滑块位置只由当前值决定；value 已在写入时量化到 steps 档，这里直接按比例取位置。
		value, maximum := s.legacyGaugeValue(control)
		if maximum <= 0 {
			return
		}
		fraction := max(0, min(1, float64(value)/float64(maximum)))
		knob := control.button
		knob.X = control.x + int(fraction*float64(max(0, control.trackWidth-control.knobW))+0.5)
		s.drawFrameButton(canvas, knob, 0)
		return
	}
	if control.markerOnly {
		// 底图已经画好横条和刻度，只画当前位置的红色竖线。
		value, maximum := s.legacyGaugeValue(control)
		if maximum <= 0 || control.trackWidth <= 0 {
			return
		}
		fraction := max(0, min(1, float64(value)/float64(maximum)))
		x := control.x + 2 + int(fraction*float64(control.trackWidth-4))
		height := max(1, control.h-16)
		canvas.Fill(x, control.y+8, 3, height, 40, 40, 255, 255)
		return
	}
	if len(control.dots) > 0 {
		// 离散刻度：在当前值对应的点上画 cfg_mark 的菱形帧。
		value, maximum := s.legacyGaugeValue(control)
		if maximum <= 0 || len(control.dots) <= 1 {
			return
		}
		index := int(float64(value)/float64(maximum)*float64(len(control.dots)-1) + 0.5)
		index = clampInt(index, 0, len(control.dots)-1)
		if control.dotMark.Empty() {
			return
		}
		mark := control.dotMark
		mark.X = control.dots[index].X - 2
		mark.Y = control.dots[index].Y - 2
		s.drawFrameButton(canvas, mark, 2)
		return
	}
	if control.vertical {
		image := s.buttonImage(control.knob)
		if image == nil {
			return
		}
		knobH := image.Height / max(1, control.knob.Frames)
		if knobH <= 0 {
			return
		}
		value, maximum := s.legacyGaugeValue(control)
		if maximum <= 0 {
			return
		}
		fraction := max(0, min(1, float64(value)/float64(maximum)))
		travel := max(0, control.h-knobH)
		x := control.x + (control.w-image.Width)/2
		y := control.y + int(fraction*float64(travel))
		canvas.DrawRegion(image, 0, 0, image.Width, knobH, x, y)
		return
	}
	image := s.composer.Load(control.button.Image)
	if image == nil {
		return
	}
	if control.fill {
		// 底槽由底图画好，这里只把填充段按值裁到对应宽度；宽度为 0 时不调用 DrawRegion。
		value, maximum := s.legacyGaugeValue(control)
		if maximum <= 0 {
			return
		}
		// 多帧填充条取哪一帧由 FixedFrame/Start 决定：MK 的 cfg_meter 五帧是
		// 黄/红/绿/蓝/灰五条色条（BGM/音量黄、赤绿青濃度各一帧），不能永远取
		// 第 0 帧。其它作品的填充条没有 FixedFrame，仍取第 0 帧。
		frames := max(1, control.button.Frames)
		frame := 0
		if control.button.FixedFrame {
			frame = clampInt(control.button.Start, 0, frames-1)
		}
		srcX, srcY, fullWidth, height := 0, frame*(image.Height/frames), image.Width, image.Height/frames
		if control.button.Atlas && control.button.Clip[2] > 0 && control.button.Clip[3] > 0 {
			srcX, srcY = control.button.Clip[0], control.button.Clip[1]
			fullWidth, height = control.button.Clip[2], control.button.Clip[3]
			if control.button.AtlasAxis == 1 {
				srcX += frame * fullWidth
			} else {
				srcY += frame * height
			}
		}
		fraction := max(0, min(1, float64(value)/float64(maximum)))
		width := int(fraction*float64(fullWidth) + 0.5)
		if width > 0 && height > 0 {
			canvas.DrawRegion(image, srcX, srcY, width, height, control.button.X, control.button.Y)
		}
		return
	}
	frames := max(1, control.button.Frames)
	height := image.Height / frames
	value, maximum := s.legacyGaugeValue(control)
	if maximum <= 0 {
		return
	}
	fraction := float64(value) / float64(maximum)
	fraction = max(0, min(1, fraction))
	frame := 0
	if frames > 1 && control.valueFrame {
		// cfg_item4/6 这类 11 帧值条本身就是"当前值"的视觉；不要只画第 0 帧。
		frame = clampInt(int(fraction*float64(frames-1)+0.5), 0, frames-1)
		s.drawFrameButton(canvas, control.button, frame)
		return
	}
	s.drawFrameButton(canvas, control.button, 0)
	track := control.w - 6
	if track <= 0 {
		return
	}
	markerX := control.x + 3 + int(fraction*float64(track))
	canvas.Fill(markerX, control.y+4, 3, max(1, height-8), 60, 200, 255, 255)
}

// ---------------------------------------------------------------- 值

// legacyExtraToggleOn 读取 ExtraToggles 的当前状态。
// 键名与 session/game_settings.go 的字段一一对应；未知键只显示为开，
// 这样汉化补丁多写一个开关也不会让整行消失。
func (s *Session) legacyExtraToggleOn(key string) bool {
	switch key {
	case "anti alias":
		return s.settings.AntiAlias
	case "cutin":
		return s.settings.Cutin
	case "wipe":
		return s.settings.Wipe
	case "hold":
		return s.settings.Hold
	case "blink":
		return s.settings.Blink
	case "x count":
		return s.settings.XCountdown
	case "steam":
		return s.settings.Steam
	case "hair mode":
		return s.settings.HairMode
	case "character voice 0":
		return s.settings.CharacterVoiceOn[0]
	case "character voice 1":
		return s.settings.CharacterVoiceOn[1]
	case "character voice 2":
		return s.settings.CharacterVoiceOn[2]
	}
	return true
}

func (s *Session) toggleLegacyExtra(key string) {
	switch key {
	case "anti alias":
		s.settings.AntiAlias = !s.settings.AntiAlias
	case "cutin":
		s.settings.Cutin = !s.settings.Cutin
	case "wipe":
		s.settings.Wipe = !s.settings.Wipe
	case "hold":
		s.settings.Hold = !s.settings.Hold
	case "blink":
		s.settings.Blink = !s.settings.Blink
	case "x count":
		s.settings.XCountdown = !s.settings.XCountdown
	case "steam":
		s.settings.Steam = !s.settings.Steam
	case "hair mode":
		s.settings.HairMode = !s.settings.HairMode
	case "character voice 0":
		s.settings.CharacterVoiceOn[0] = !s.settings.CharacterVoiceOn[0]
	case "character voice 1":
		s.settings.CharacterVoiceOn[1] = !s.settings.CharacterVoiceOn[1]
	case "character voice 2":
		s.settings.CharacterVoiceOn[2] = !s.settings.CharacterVoiceOn[2]
	default:
		return
	}
	s.saveGameSettings()
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// legacyToggleOn 返回一个开关控件的当前状态。
func (s *Session) legacyToggleOn(control legacyControl) bool {
	settings := s.settings
	switch control.kind {
	case legacyControlScreenMode:
		return settings.ScreenMode == "full"
	case legacyControlSquareMode:
		return settings.SquareMode
	case legacyControlAllEffect:
		return settings.AllEffect
	case legacyControlCeaseSkip:
		return settings.CeaseSkip
	case legacyControlSkipUnread:
		if control.explicit {
			return settings.SkipUnread == control.value
		}
		return settings.SkipUnread
	case legacyControlVoiceLoop:
		if control.explicit {
			return settings.VoiceLoop == control.value
		}
		return settings.VoiceLoop
	case legacyControlSkipMode:
		return settings.SkipMode == control.index
	case legacyControlRadioGroup:
		return s.legacyRadioValue(control.groupKey) == control.option
	case legacyControlLiquidType:
		// 0 = なし、1.. = 通常/絶倫；当前版本只区分“显示/不显示”。
		return control.explicit && settings.Steam == control.value
	case legacyControlAutoSelect:
		return settings.AutoSelect == control.index
	case legacyControlBgmSelect:
		return settings.BgmSelect == control.index
	case legacyControlVoiceMode:
		return settings.VoiceMode == control.index
	case legacyControlCharacter:
		return control.index >= 0 && control.index < len(settings.CharacterVoiceOn) &&
			settings.CharacterVoiceOn[control.index]
	case legacyControlEffectToggle:
		return s.legacyEffectToggleOn(control.index)
	case legacyControlExtraToggle:
		return s.legacyExtraToggleOn(control.groupKey)
	case legacyControlVoicePlay:
		// 詳細音声页的这个勾选是"当前角色语音 ON/OFF"，不是试听按钮；
		// 原配置把它标在 voice play 坐标上。
		character := s.selectedLegacyCharacter()
		return settings.CharacterVoiceOn[character]
	}
	return false
}

// setLegacyDotGauge 按点击位置选最近的刻度点。
func (s *Session) setLegacyDotGauge(control legacyControl, x, y int) {
	if len(control.dots) == 0 {
		return
	}
	best, bestDistance := 0, 1<<30
	for index, point := range control.dots {
		dx, dy := x-point.X-8, y-point.Y-8
		distance := dx*dx + dy*dy
		if distance < bestDistance {
			best, bestDistance = index, distance
		}
	}
	_, maximum := s.legacyGaugeValue(control)
	if maximum <= 0 || len(control.dots) <= 1 {
		return
	}
	value := int(float64(best) / float64(len(control.dots)-1) * float64(maximum))
	s.applyLegacyGauge(control, value)
	s.saveGameSettings()
	s.applyDisplaySettings()
	s.composer.Invalidate()
}

// legacyValueGroup 按 key 找配置里的取值组；找不到时返回 false。
func (s *Session) legacyValueGroup(key string) (config.LegacyValueGroup, bool) {
	if s.config == nil || len(key) == 0 {
		return config.LegacyValueGroup{}, false
	}
	for _, group := range s.config.LegacyConfig.ValueGroups {
		if group.Key == key {
			return group, true
		}
	}
	return config.LegacyValueGroup{}, false
}

// legacyValueGroupValue 返回取值组当前对应的设置值，与 setLegacyValueGroup 互逆。
// 未知键返回 -1，保证任何按钮都不会被误判为当前值。
func (s *Session) legacyValueGroupValue(key string) int {
	settings := s.settings
	switch key {
	case "screen mode":
		if settings.ScreenMode == "full" {
			return 1
		}
		return 0
	case "screen effect":
		// 画面効果以 AllEffect 为总开关：关闭时统一显示「なし」，与写回时的规则一致。
		if !settings.AllEffect {
			return 0
		}
		return settings.EffectMode
	case "liquid":
		if settings.Steam {
			return 1
		}
		return 0
	case "skip unread":
		if settings.SkipUnread {
			return 1
		}
		return 0
	case "skip speed":
		return settings.SkipMode
	case "voice loop":
		if settings.VoiceLoop {
			return 1
		}
		return 0
	}
	return -1
}

// setLegacyValueGroup 把取值组选中的值写回 GameSettings。
// 画面効果选「なし」(0) 时必须同时关掉 AllEffect，否则脚本读到的仍是特效开启。
func (s *Session) setLegacyValueGroup(key string, value int) {
	settings := &s.settings
	switch key {
	case "screen mode":
		if value == 1 {
			settings.ScreenMode = "full"
		} else {
			settings.ScreenMode = "window"
		}
	case "screen effect":
		settings.EffectMode = value
		settings.AllEffect = value != 0
	case "liquid":
		settings.Steam = value != 0
	case "skip unread":
		settings.SkipUnread = value != 0
	case "skip speed":
		settings.SkipMode = clampInt(value, 0, 2)
	case "voice loop":
		settings.VoiceLoop = value != 0
	}
}

// legacyRadioValue / setLegacyRadioValue 把通用单选组的键映射到 GameSettings。
// 只认识配置里实际使用的三个键；未知键返回 0，写回时忽略。
func (s *Session) legacyRadioValue(key string) int {
	switch key {
	case "screen effect":
		return s.settings.EffectMode
	case "measure":
		return s.settings.MeasureMode
	case "auto select pos":
		return s.settings.AutoSelectPosition
	case "male liquid":
		return s.settings.MaleLiquid
	case "female liquid":
		return s.settings.FemaleLiquid
	case "milk liquid":
		return s.settings.MilkLiquid
	case "skip mode":
		return s.settings.SkipMode
	}
	return 0
}

func (s *Session) setLegacyRadioValue(key string, value int) {
	switch key {
	case "screen effect":
		s.settings.EffectMode = value
		// AllEffect 是旧演出开关；画面効果选"なし"时必须一起关掉，
		// 否则其它作品/脚本读到的仍是"特效开"。
		s.settings.AllEffect = value != 0
	case "measure":
		s.settings.MeasureMode = value
	case "auto select pos":
		s.settings.AutoSelectPosition = value
	case "male liquid":
		s.settings.MaleLiquid = value
	case "female liquid":
		s.settings.FemaleLiquid = value
	case "milk liquid":
		s.settings.MilkLiquid = value
	case "skip mode":
		s.settings.SkipMode = clampInt(value, 0, 2)
	}
}

// legacyEffectToggleOn 按 [detailed effect setting] 的键顺序读取开关。
// 键与 GameSettings 的映射集中在这里：布局里没有独立字段的键（ahe 等）
// 返回 true，只显示勾选，不改变游戏行为。
func (s *Session) legacyEffectToggleOn(index int) bool {
	layout := s.config.LegacyConfig
	if layout.Effect == nil || index < 0 || index >= len(layout.Effect.Toggles) {
		return false
	}
	switch layout.Effect.Toggles[index].Key {
	case "effect mode":
		return s.settings.AllEffect
	case "layer action":
		return s.settings.LayerAction
	case "forced action0":
		return s.settings.ForcedAction
	case "unlock notice":
		return s.settings.UnlockNotice
	case "eye anime":
		return s.settings.Blink
	case "cutin":
		return s.settings.Cutin
	case "x count":
		return s.settings.XCountdown
	case "hair mode":
		return s.settings.UnderHair
	case "steam mode":
		return s.settings.Steam
	case "ahe mode":
		return s.settings.AheOff
	}
	return true
}

func (s *Session) legacyGaugeValue(control legacyControl) (int, int) {
	settings := s.settings
	switch control.kind {
	case legacyControlVolume:
		switch control.index {
		case 0:
			return int(settings.MasterVolume * 1000), 1000
		case 1:
			return int(settings.BgmVolume * 1000), 1000
		case 2:
			return int(settings.SeVolume * 1000), 1000
		case 3:
			return int(settings.BgsVolume * 1000), 1000
		case 4:
			return int(settings.VoiceVolume * 1000), 1000
		case 5:
			return int(settings.BgvVolume * 1000), 1000
		case 6:
			return int(settings.BgmAttenuation * 1000), 1000
		}
	case legacyControlColor:
		switch control.index {
		case 0:
			return settings.BgRed, 255
		case 1:
			return settings.BgGreen, 255
		case 2:
			return settings.BgBlue, 255
		case 3:
			return settings.BgAlpha, 255
		}
	case legacyControlSpeed:
		if control.index == 0 {
			return settings.MessageSpeed, 100
		}
		return settings.ClickWait, 100
	case legacyControlSoundVolume:
		character := s.selectedLegacyCharacter()
		if control.index == 0 {
			return int(settings.CharacterVoice[character] * 1000), 1000
		}
		return int(settings.CharacterBgv[character] * 1000), 1000
	}
	return 0, 0
}

func (s *Session) selectedLegacyCharacter() int {
	state := s.legacyState()
	if state == nil {
		return 0
	}
	if state.character < 0 || state.character >= len(s.settings.CharacterVoice) {
		return 0
	}
	return state.character
}

func (s *Session) legacyCharacterSelected(index int) bool {
	state := s.legacyState()
	return state != nil && state.character == index
}

// ---------------------------------------------------------------- 命中/交互

func (s *Session) hitLegacyConfigScreen(x, y int) uiHit {
	state := s.legacyState()
	if state == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	// Tbxdl 的值图按钮不在 controls 里（它们是按设置值取帧的显示项），
	// 先判它们，避免被底下的滑杆/开关抢走。
	if state.page == legacyPageMain {
		if index := s.hitLegacyOptionButton(x, y); index >= 0 {
			return uiHit{kind: hitLegacyOption, index: index}
		}
	}
	// 后画的控件在上层，倒序命中。
	for index := len(state.controls) - 1; index >= 0; index-- {
		control := state.controls[index]
		if control.gauge {
			if x >= control.x && y >= control.y && x < control.x+control.w && y < control.y+control.h {
				return uiHit{kind: hitLegacyConfigControl, index: index}
			}
			continue
		}
		if hitButton(s, control.button, x, y) {
			return uiHit{kind: hitLegacyConfigControl, index: index}
		}
		// 勾选图标很小，按 28x24 扩大命中区；坐标是图标左上角。
		// 配了 hitWidth 的作品（AFDVD）把整行标签也纳入命中。
		hitWidth := 28
		if control.hitWidth > hitWidth {
			hitWidth = control.hitWidth
		}
		if control.toggle &&
			x >= control.x-4 && y >= control.y-4 && x < control.x-4+hitWidth && y < control.y+24 {
			return uiHit{kind: hitLegacyConfigControl, index: index}
		}
		if control.kind == legacyControlCharacterDetail && !control.button.Empty() {
			if hitButton(s, control.button, x, y) {
				return uiHit{kind: hitLegacyConfigControl, index: index}
			}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateLegacyConfigScreen(hit uiHit, x, y int) {
	state := s.legacyState()
	if state == nil {
		return
	}
	if hit.kind == hitLegacyOption {
		s.activateLegacyOption(hit.index)
		return
	}
	if hit.kind != hitLegacyConfigControl {
		return
	}
	if hit.index < 0 || hit.index >= len(state.controls) {
		return
	}
	control := state.controls[hit.index]
	if control.gauge {
		if len(control.dots) > 0 {
			s.setLegacyDotGauge(control, x, y)
			return
		}
		state.dragging = hit.index
		s.setLegacyGaugeAt(control, x, y)
		return
	}
	state.dragging = -1
	if control.delta != 0 {
		s.adjustLegacyGauge(control)
		return
	}
	s.toggleLegacyControl(control)
}

// legacyPointerDrag 处理滑杆拖动（Pointer op 6）。只有按下过滑杆才开始，
// 否则聊天/剧情区的拖动会被误当成调音量。
func (s *Session) dragLegacyConfig(x, y int) bool {
	state := s.legacyState()
	if state == nil || state.dragging < 0 || state.dragging >= len(state.controls) {
		return false
	}
	control := state.controls[state.dragging]
	if !control.gauge {
		state.dragging = -1
		return false
	}
	s.setLegacyGaugeAt(control, x, y)
	return true
}

// setLegacyGaugeAt 按指针位置设置滑杆值；竖向滑杆用 y，横向用 x。
func (s *Session) setLegacyGaugeAt(control legacyControl, x, y int) {
	_, maximum := s.legacyGaugeValue(control)
	if maximum <= 0 {
		return
	}
	fraction := 0.0
	if control.vertical {
		knobH := 8
		if image := s.buttonImage(control.knob); image != nil {
			knobH = image.Height / max(1, control.knob.Frames)
		}
		track := control.h - knobH
		if track <= 0 {
			return
		}
		fraction = float64(y-control.y-knobH/2) / float64(track)
	} else {
		// 与 drawLegacyGauge 的 markerOnly 映射保持一致：
		// 标记中心从 x+2 走到 x+w-2。
		if control.horizontalKnob {
			// 滑块中心跟随指针：减去半个滑块宽度后，取值范围是 trackWidth-knobW。
			track := control.trackWidth - control.knobW
			if track <= 0 {
				return
			}
			fraction = float64(x-control.x-control.knobW/2) / float64(track)
		} else {
			track := control.w - 4
			if track <= 0 {
				return
			}
			fraction = float64(x-control.x-2) / float64(track)
		}
	}
	fraction = max(0, min(1, fraction))
	value := int(fraction*float64(maximum) + 0.5)
	if control.steps > 0 {
		// 按档位取整：拖动落在哪一档就写哪一档的值，与绘制时反推的位置一致。
		value = int(fraction*float64(control.steps)+0.5) * maximum / control.steps
	}
	s.applyLegacyGauge(control, value)
	s.saveGameSettings()
	s.applyDisplaySettings()
}

func (s *Session) applyLegacyGauge(control legacyControl, value int) {
	switch control.kind {
	case legacyControlVolume:
		scaled := clampVolumeSetting(float64(value) / 1000)
		switch control.index {
		case 0:
			s.settings.MasterVolume = scaled
		case 1:
			s.settings.BgmVolume = scaled
		case 2:
			s.settings.SeVolume = scaled
		case 3:
			s.settings.BgsVolume = scaled
		case 4:
			s.settings.VoiceVolume = scaled
		case 5:
			s.settings.BgvVolume = scaled
		case 6:
			s.settings.BgmAttenuation = scaled
		}
	case legacyControlColor:
		value = clampInt(value, 0, 255)
		switch control.index {
		case 0:
			s.settings.BgRed = value
		case 1:
			s.settings.BgGreen = value
		case 2:
			s.settings.BgBlue = value
		case 3:
			s.settings.BgAlpha = value
		}
	case legacyControlSpeed:
		value = clampInt(value, 0, 100)
		if control.index == 0 {
			s.settings.MessageSpeed = value
		} else {
			s.settings.ClickWait = value
		}
	case legacyControlSoundVolume:
		character := s.selectedLegacyCharacter()
		scaled := clampVolumeSetting(float64(value) / 1000)
		if control.index == 0 {
			s.settings.CharacterVoice[character] = scaled
		} else {
			s.settings.CharacterBgv[character] = scaled
		}
	}
}

// adjustLegacyGauge 处理滑杆两端按钮：读当前值、按步长夹紧后写回。
// 步长与拖动共用 applyLegacyGauge，避免两套夹紧/持久化逻辑。
func (s *Session) adjustLegacyGauge(control legacyControl) {
	value, maximum := s.legacyGaugeValue(control)
	if maximum <= 0 {
		return
	}
	value = clampInt(value+control.delta, 0, maximum)
	s.applyLegacyGauge(control, value)
	s.saveGameSettings()
	s.applyDisplaySettings()
	s.composer.Invalidate()
}

func (s *Session) toggleLegacyControl(control legacyControl) {
	settings := &s.settings
	switch control.kind {
	case legacyControlScreenMode:
		if settings.ScreenMode == "full" {
			settings.ScreenMode = "window"
		} else {
			settings.ScreenMode = "full"
		}
	case legacyControlSquareMode:
		settings.SquareMode = !settings.SquareMode
	case legacyControlAllEffect:
		settings.AllEffect = !settings.AllEffect
	case legacyControlCeaseSkip:
		settings.CeaseSkip = !settings.CeaseSkip
	case legacyControlSkipUnread:
		if control.explicit {
			settings.SkipUnread = control.value
		} else {
			settings.SkipUnread = !settings.SkipUnread
		}
	case legacyControlVoiceLoop:
		if control.explicit {
			settings.VoiceLoop = control.value
		} else {
			settings.VoiceLoop = !settings.VoiceLoop
		}
	case legacyControlSkipMode:
		settings.SkipMode = control.index
	case legacyControlValueGroup:
		if group, ok := s.legacyValueGroup(control.groupKey); ok && group.ExclusiveToggle && len(group.Options) > 0 {
			// 共用显示位置的 ON/OFF（MDDVD 的「画面表示」）：点击只能命中
			// 最上层的那张图，直接切到下一个值，而不是按命中的图取值。
			current := s.legacyValueGroupValue(control.groupKey)
			next := group.Options[0].Value
			for index, option := range group.Options {
				if option.Value == current {
					next = group.Options[(index+1)%len(group.Options)].Value
					break
				}
			}
			s.setLegacyValueGroup(control.groupKey, next)
			break
		}
		s.setLegacyValueGroup(control.groupKey, control.option)
	case legacyControlAutoSelect:
		settings.AutoSelect = control.index
	case legacyControlBgmSelect:
		settings.BgmSelect = clampSetting(control.index, 0, 1)
	case legacyControlRadioGroup:
		s.setLegacyRadioValue(control.groupKey, control.option)
	case legacyControlExtraToggle:
		s.toggleLegacyExtra(control.groupKey)
		return
	case legacyControlVoiceMode:
		settings.VoiceMode = control.index
	case legacyControlCharacter:
		if control.index >= 0 && control.index < len(settings.CharacterVoiceOn) {
			settings.CharacterVoiceOn[control.index] = !settings.CharacterVoiceOn[control.index]
			if state := s.legacyState(); state != nil && s.config.LegacyConfig.CharacterSelectedFrame > 0 {
				state.character = control.index
			}
		}
	case legacyControlCharacterDetail:
		state := s.legacyState()
		if state != nil && s.config.LegacyConfig.Sound != nil {
			state.character = control.index
			state.page = legacyPageSound
			s.buildLegacyControls()
		}
	case legacyControlEffectDetail:
		state := s.legacyState()
		if state != nil && s.config.LegacyConfig.Effect != nil {
			state.page = legacyPageEffect
			s.buildLegacyControls()
		}
	case legacyControlLiquidDetail:
		state := s.legacyState()
		if state != nil && s.config.LegacyConfig.Liquid != nil {
			state.page = legacyPageLiquid
			s.buildLegacyControls()
		}
	case legacyControlLiquidType:
		settings.Steam = control.value
	case legacyControlEffectToggle:
		s.toggleLegacyEffect(control.index)
	case legacyControlVoicePlay:
		character := s.selectedLegacyCharacter()
		if character < 0 || character >= len(settings.CharacterVoiceOn) {
			break
		}
		settings.CharacterVoiceOn[character] = !settings.CharacterVoiceOn[character]
		core.Logf("ui: legacy config character %d voice=%v", character, settings.CharacterVoiceOn[character])
	case legacyControlModeSave:
		s.openDataScreen(true)
		return
	case legacyControlModeLoad:
		s.openDataScreen(false)
		return
	case legacyControlModeConfig:
		// 已经在环境设定页，点自己不做任何事。
		return
	case legacyControlReset:
		s.settings = defaultGameSettings(s.config)
		s.applyDisplaySettings()
		s.buildLegacyControls()
	case legacyControlBack:
		state := s.legacyState()
		if state != nil && state.page != legacyPageMain {
			state.page = legacyPageMain
			s.buildLegacyControls()
			break
		}
		s.closeSystemUI()
		s.resumeCGViewerIfNeeded()
		return
	case legacyControlTitle:
		s.cgResume = nil
		s.closeSystemUI()
		s.enterTitle()
		return
	case legacyControlExit:
		s.cgResume = nil
		s.setHostRequest("exit")
		return
	}
	s.saveGameSettings()
	s.applyDisplaySettings()
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

func (s *Session) toggleLegacyEffect(index int) {
	layout := s.config.LegacyConfig
	if layout.Effect == nil || index < 0 || index >= len(layout.Effect.Toggles) {
		return
	}
	settings := &s.settings
	switch layout.Effect.Toggles[index].Key {
	case "effect mode":
		settings.AllEffect = !settings.AllEffect
	case "layer action":
		settings.LayerAction = !settings.LayerAction
	case "forced action0":
		settings.ForcedAction = !settings.ForcedAction
	case "unlock notice":
		settings.UnlockNotice = !settings.UnlockNotice
	case "eye anime":
		settings.Blink = !settings.Blink
	case "cutin":
		settings.Cutin = !settings.Cutin
	case "x count":
		settings.XCountdown = !settings.XCountdown
	case "hair mode":
		settings.UnderHair = !settings.UnderHair
	case "steam mode":
		settings.Steam = !settings.Steam
	case "ahe mode":
		settings.AheOff = !settings.AheOff
	}
}
