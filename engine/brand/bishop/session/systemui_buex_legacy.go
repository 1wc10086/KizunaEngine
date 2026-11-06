package session

import (
	"strings"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_buex_legacy.go 是旧式立绘鉴赏（[museum phase - bu] + [bu viewer]）。
//
// 新版作品把立绘鉴赏写进 define.bsa 的 museum_buex_layout；旧版（特別授業3SLG
// 旧版 tj3slg）没有这个资源，而是：
//
//	[museum phase - bu]  先显示一张角色选择底图，target0..3 是四个可点角色
//	[bu viewer]          点角色后进入换装查看器（同一套 dress_view_* 素材）
//
// 两段配置的数据形状和 buex 很接近，所以这里把旧配置"翻译"成一份内存里的
// BuexLayout，复用已经调好的立绘绘制/换装/开关逻辑，而不是复制第二套渲染器。
// 差异只有三点：没有背景/留言/预设页、退出回角色选择页、开关数量来自旧表。

// openLegacyBustupMuseum 打开旧式立绘鉴赏入口。
//
// 有角色目标时先停在选择页；没有目标（发行版把选择页省掉）就直接开查看器。
func (s *Session) openLegacyBustupMuseum() bool {
	if s.config == nil || s.composer == nil {
		return false
	}
	viewer := s.config.Museum.BuViewer
	if viewer.Empty() {
		return false
	}
	ui := s.ui
	if ui == nil {
		return false
	}
	if len(s.config.Museum.Bu.Targets) > 0 && !s.config.Museum.MultiBu {
		// 有些发行版配置里保留了两段旧式界面，却没有打包 museum_bu /
		// museum_bu_ch* 素材（tj3slg 的 Graphics.bsa 就没有）。这时停在一个
		// 只有页签的黑页没有意义：直接进查看器，让玩家用角色图标切人。
		//
		// [museum phase] multi bu = 1 的作品（牝教師3）永远不进这一页：
		// 查看器本身就是完整的立ち絵鉴赏界面，六张角色卡和各自的 ON/OFF
		// 都在 viewer 里。若把 `museum_bu` 当成可选的角色选择页打开，
		// 发行版里任何一个同名散文件/说明图都会让玩家停在
		// "标题背景 + 两列头像 + 鉴赏室页签"的残页上，查看器根本进不去。
		if s.resourceExists(s.config.Museum.Bu.Base) && legacyTargetsPlaced(s.config.Museum.Bu) {
			ui.kind = uiMuseumBU
			ui.buex = nil
			ui.hover = uiHit{kind: hitNone, index: -1}
			s.composer.Invalidate()
			core.Logf("ui: legacy bust-up select opened (%d targets)", len(s.config.Museum.Bu.Targets))
			return true
		}
		core.Logf("ui: legacy bust-up select unusable (base='%s'), opening viewer", s.config.Museum.Bu.Base)
	}
	if s.config.Museum.MultiBu {
		core.Logf("ui: multi bu viewer opened directly (selection page skipped)")
	}
	character := 0
	if len(viewer.DefaultOrder) > 0 {
		character = viewer.DefaultOrder[0] - 1
	}
	return s.openLegacyBustupViewer(character)
}

// drawLegacyBustupSelect 画角色选择页：底图 + 目标按钮 + 鉴赏室页签。
func (s *Session) drawLegacyBustupSelect(canvas *gfx.BgraCanvas) {
	if s.ui == nil {
		return
	}
	page := s.config.Museum.Bu
	if page.Empty() {
		return
	}
	if page.Atlas {
		// 精灵图世代：Base 是素材表，只画角色展示区和标题/底板。
		if !page.CharactersBase.Empty() {
			s.drawFrameButton(canvas, page.CharactersBase, 0)
		}
		if !page.Title.Empty() {
			s.drawFrameButton(canvas, page.Title, 0)
		}
		if !page.Menu.Empty() {
			s.drawFrameButton(canvas, page.Menu, 0)
		}
	} else {
		s.drawLayerAt(canvas, page.Base, 0, 0)
	}
	// 当前/悬停角色的高亮框：selectedtarget 是 7 帧横向图，
	// 一帧对应一个目标位（target0..6 的顺序）。
	if !page.SelectedTarget.Empty() && s.ui.hover.kind == hitBUTarget && s.ui.hover.index >= 0 {
		// 只在悬停角色时画高亮框；常驻 frame0 会盖住第一个角色。
		s.drawFrameButton(canvas, page.SelectedTarget, s.ui.hover.index)
	}
	for index, target := range page.Targets {
		frame := 0
		if s.ui.hover.kind == hitBUTarget && s.ui.hover.index == index {
			frame = 1
		}
		// 精灵图世代的角色卡没有悬停帧（patterns 缺省 1 帧），drawFrameButton
		// 会把 frame 夹到 0，不会画出越界区域。
		s.drawFrameButton(canvas, target.Button, frame)
	}
	s.drawMuseumTabs(canvas)
}

func (s *Session) hitLegacyBustupSelect(x, y int) uiHit {
	page := s.config.Museum.Bu
	for index, target := range page.Targets {
		if hitButton(s, target.Button, x, y) {
			return uiHit{kind: hitBUTarget, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateLegacyBustupSelect(hit uiHit, x, y int) {
	if hit.kind != hitBUTarget {
		return
	}
	page := s.config.Museum.Bu
	if hit.index < 0 || hit.index >= len(page.Targets) {
		return
	}
	s.openLegacyBustupViewer(page.Targets[hit.index].Character)
}

// openLegacyBustupViewer 打开（或重建）旧式换装查看器。
func (s *Session) openLegacyBustupViewer(character int) bool {
	if s.config == nil || s.composer == nil {
		return false
	}
	viewer := s.config.Museum.BuViewer
	if viewer.Atlas {
		return s.openMD2BuViewer(character)
	}
	if viewer.Empty() || s.ui == nil {
		return false
	}
	layout := legacyBuexLayout(viewer, s.config.Museum.MultiBu)
	state := s.buexNewState(layout)
	state.legacy = true
	// 只有"确实有角色选择页、且不是 multi bu 作品"时才允许退出回选择页。
	// 牝教師3 的选择页素材即使存在也不用：查看器就是完整界面，
	// 退出应直接回鉴赏室主菜单，而不是停在一张绘制不完整的残页上。
	state.legacySelect = !s.config.Museum.MultiBu &&
		len(s.config.Museum.Bu.Targets) > 0 &&
		s.resourceExists(s.config.Museum.Bu.Base)
	// 旧式查看器没有背景选择页，但 SL 的 setting6 是独立的"背景"开关：
	// 底图 dress_view_base 是一张透明覆盖层，真正的背景必须由引擎铺。
	// 没有这一层时玩家看到的是黑屏（立绘鉴赏"背景显示错误"的另一个来源）。
	state.bgNames = fallbackBackgroundNames(s.resourceImageNames())
	state.evPages = nil
	state.pages = nil
	state.background = ""
	if len(state.bgNames) > 0 {
		state.background = state.bgNames[0]
	}
	state.page = buexPageStandee

	// default order 是 1 基角色编号；排在前面的先显示，但 selected 仍然是
	// 玩家在角色选择页点的那个人。
	if len(viewer.DefaultOrder) > 0 {
		order := make([]int, 0, len(state.order))
		seen := make([]bool, len(state.order))
		for _, number := range viewer.DefaultOrder {
			index := number - 1
			if index < 0 || index >= len(seen) || seen[index] {
				continue
			}
			seen[index] = true
			order = append(order, index)
		}
		for index := range state.order {
			if !seen[index] {
				order = append(order, index)
			}
		}
		state.order = order
	}
	if character < 0 || character >= len(state.chars) {
		character = 0
	}
	state.selected = character
	// 旧式角色选择页点的是"要看谁"：多角色同屏模式下让这个人一开始就是
	// ON 状态，别的角色保持 OFF，和玩家点选的角色对得上。
	if state.multi {
		for index := range state.chars {
			state.chars[index].visible = index == character
		}
	}

	s.ui.kind = uiMuseumBU
	s.ui.buex = state
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	core.Logf("ui: legacy bust-up viewer opened (%d characters)", len(state.chars))
	return true
}

// legacyBuexLayout 把 [bu viewer] 翻译成 BuexLayout。
//
// 坐标和编号表原样搬运：旧配置的 settingN 顺序是
// ポーズ / 視点 / ぬぎぬぎ / 服装 / 下着 / 表情(两段)，
// 与 buex 的六组开关一一对应；表情的 setting5 + setting6 是同一组的
// 上下两排，合并时用 setting6 的 Y 减 setting5 的 Y 作为行距。
//
// multi 来自 [museum phase] multi bu：牝教師3 的查看器是"多名角色同屏"，
// 角色图标旁的 ON/OFF 各自独立；其它旧作的查看器仍然一次只看一个角色。
func legacyBuexLayout(viewer config.BuViewer, multi bool) *config.BuexLayout {
	layout := &config.BuexLayout{
		MultiStandee:      multi,
		EnableViewControl: viewer.EnableViewControl,
	}
	if !viewer.Exit.Empty() {
		layout.Menu = []config.BuexMenuEntry{{
			Key:    "back_to_thumbs",
			Button: legacyConfigButton(viewer.Exit),
		}}
	}
	buttons := map[string]config.ConfigButton{}
	for key, button := range map[string]config.UIButton{
		"order_up":      viewer.OrderUp,
		"order_down":    viewer.OrderDown,
		"on_off_button": viewer.OnOff,
		"pos_pointer":   viewer.PosPointer,
		"switch":        viewer.Switch,
	} {
		if !button.Empty() {
			buttons[key] = legacyConfigButton(button)
		}
	}

	setting := config.BuexSetting{
		Base:          viewer.Base,
		Buttons:       buttons,
		PosRegion:     viewer.PosRegion,
		PoseSignature: viewer.PoseSignature,
		ViewSignature: viewer.ViewSignature,
		StripConvert:  viewer.NgngConvert,
	}
	setting.Switches = legacySwitchGroups(viewer)
	for index, character := range viewer.Characters {
		entry := config.BuexCharacter{
			Wear:      character.Wear,
			UnderWear: character.UnderWear,
			Faces:     character.Faces,
		}
		if index < len(viewer.Icons) {
			entry.Icon = legacyConfigButton(viewer.Icons[index])
		}
		setting.Characters = append(setting.Characters, entry)
	}
	layout.Setting = setting
	return layout
}

// legacySwitchGroups 生成六组（或七组）开关。
//
// setting4（下着）在原配置里是 -16,-16，表示"这一组不显示"；仍然保留
// 占位，否则后面的组会整体前移，命中种类和角色数据错位。
//
// setting6 有两种含义，靠坐标区分：
//   - tj3slg：它是表情组的第二排，和 setting5 只隔一个行距；
//   - SL：它是独立的"背景"开关组（dress_view_base 上写着 背景），
//     与表情组隔着很远的距离。
//
// 旧实现无条件把它并进表情组，SL 的第 7 组就永远画不出来。
func legacySwitchGroups(viewer config.BuViewer) []config.BuexSwitchGroup {
	settings := viewer.Settings
	group := func(index int) config.BuexSwitchGroup {
		if index < 0 || index >= len(settings) {
			// 缺项用负坐标标记禁用：零值会被当成"画在 (0,0)"。
			return config.BuexSwitchGroup{X: -1, Y: -1}
		}
		item := settings[index]
		spec := config.BuexSwitchGroup{
			X:       item.X,
			Y:       item.Y,
			SpaceX:  item.SpaceX,
			SpaceY:  item.SpaceY,
			Columns: item.Columns,
		}
		if item.Disabled {
			// 负坐标是旧配置的"禁用"哨兵；渲染/命中都会跳过。
			spec.X, spec.Y = -1, -1
		}
		return spec
	}
	groups := []config.BuexSwitchGroup{
		group(0), // ポーズ
		group(1), // 視点
		group(2), // ぬぎぬぎ
		group(3), // 服装
		group(4), // 下着（旧版禁用）
	}
	face := group(5)
	background := group(6)
	if len(settings) > 6 && !settings[6].Disabled {
		other := settings[6]
		gap := other.Y - face.Y
		if face.SpaceY > 0 && gap > face.SpaceY*2 {
			// SL：setting6 是独立的背景组。
			groups = append(groups, face, background)
			return groups
		}
		// tj3slg：setting5 + setting6 是表情组的上下两排。
		face.Count = face.Columns + other.Columns
		if other.Y > face.Y {
			face.SpaceY = other.Y - face.Y
		}
	}
	return append(groups, face)
}

func legacyConfigButton(button config.UIButton) config.ConfigButton {
	frames := button.Frames
	if frames < 1 {
		frames = 1
	}
	return config.ConfigButton{Name: button.Image, MaxState: frames, X: button.X, Y: button.Y}
}

// legacyTargetsPlaced 判断 [museum phase - bu] 的目标按钮有没有真实坐标。
//
// 牝教師3 的 target0..5 全部写成 `..., 0, 0`：按钮图本身没有屏幕坐标，
// 引擎按按钮矩形绘制时六张卡会叠在左上角，底图与命中区也全部错位。
// 这种选择页即使有 museum_bu 素材也不能用，应直接进查看器。
func legacyTargetsPlaced(page config.MuseumBUPage) bool {
	for _, target := range page.Targets {
		if target.Button.X != 0 || target.Button.Y != 0 {
			return true
		}
	}
	return false
}

// resourceExists 只查索引，不解码图片。
//
// 布局探测阶段只需要知道"有没有这张图"；用 composer.Load 会触发一次
// 解码并在缺图时打错误日志，探测一个可选页面不该污染日志。
func (s *Session) resourceExists(name string) bool {
	if s.res == nil || name == "" {
		return false
	}
	location, ok := s.res.TryGet(name)
	if !ok || location == nil {
		return false
	}
	// 发行版根目录的 manual/imgs 里有同名说明截图（museum_bu.png 等），
	// 资源索引会把它当普通散文件收录。它不是界面素材：拿它当底图会得到
	// 一张带红框标注的小截图，于是立绘鉴赏停在"黑屏 + 页签"上。
	if location.IsLoose() && strings.Contains(strings.ToLower(location.LooseFile()), "manual") {
		return false
	}
	return true
}
