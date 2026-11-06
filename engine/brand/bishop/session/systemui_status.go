package session

import (
	"strings"

	"kizuna/engine/brand/bishop/rendering"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_status.go 是原版"状態"画面（[status screen] → define.bsa:status_layout）。
//
// 工具条上的 status 按钮在多数 Bishop 作品里是注释掉的，只有部分作品
// （キョウセイ支配）启用。画面内容完全由布局文件驱动：
//
//	base: "status_base"
//	"mode button": [ { name:"inventory_back", max_state:2, pos:[901,0] } ]
//	layout: { inventory: { base:"inventory_base", slot:[
//	    { name:"status_ch01_01", pos:[0,63], condition:"#status_ch01 == 1" }
//	] } }
//
// 引擎只负责：铺底图、按条件贴角色状态/事件图、画返回按钮并处理点击。
// 条件直接读脚本属性，所以剧本改数值，画面立刻跟着变，不需要逐部作品硬编码。

// openStatusScreen 打开"状態"画面。布局缺失或解析失败时返回 false，
// 调用方保留工具条原状，不会把玩家卡在一个空白界面上。
func (s *Session) openStatusScreen() bool {
	if s.ui != nil || s.menuVisible {
		return false
	}
	// OH/OHIE 的工具条 `status` 键实际绑定的是 btn_karte：原版打开
	// [status2 screen] 的「ケダモノカルテ」。OHIE 同时带 [status screen]
	// layout（发行版里没有入口），如果先走 status_layout，Karte 按钮
	// 会打开一张与原作无关的界面；所以有 status2 时优先它。
	if s.config != nil && s.config.Status2.HasLayout() {
		return s.openKarteScreen()
	}
	// 牝教師2 老版的 ITEM 按钮：cfg_item_base + cfg_item 的自绘持ち物页。
	if s.config != nil && s.config.Item.MK2 {
		return s.openItemScreen()
	}
	// [status screen] script name 的作品（特別授業3SLG）没有 layout，
	// 状态画面由 Lua 对象定义；这里改用原生实现的四页版本，而不是
	// 直接判定"没有状态画面"。
	if s.config != nil && s.config.Status.ScriptName != "" {
		return s.openSLGStatusScreen()
	}
	// Tbxdl 世代的状态画面没有 layout 资源，底图与面板都由引擎内置。
	if s.config != nil && s.config.Status.Ancient {
		return s.openAncientStatusScreen()
	}
	// 教育指導（KS）的旧式 ITEM 页：item111…item533 直接叠在 status_base 上。
	// 只在这部作品没有 status_layout / script name 时启用，避免抢走其它
	// 同时带 base/item 键但实际有布局资源的作品。
	if s.config != nil && s.config.Status.Layout == "" &&
		s.config.Status.ScriptName == "" && !s.config.Status.Ancient &&
		!s.config.LegacyStatus.Empty() {
		return s.openLegacyStatusScreen()
	}
	layout := s.ensureStatusScreen()
	if layout == nil {
		return false
	}
	s.beginSystemUI(uiStatus)
	if s.ui != nil {
		s.ui.hover = uiHit{kind: hitNone, index: -1}
		// status_layout 有两种页：按条件贴图的"状態"页，和 CHART 流程图页。
		// 后者需要额外的 chart.db，初始化失败时保持普通页，不会画出半张图。
		s.initStatusChart(layout)
	}
	core.Logf("ui: status screen opened (%d page(s))", len(layout.Pages))
	return true
}

// ensureStatusScreen 惰性读取并解析布局资源：defines.bsa 里的 status_layout
// 只有几百行，但没必要每次打开都重新解析一遍。
func (s *Session) ensureStatusScreen() *config.StatusScreenLayout {
	if s.statusDone {
		return s.statusLayout
	}
	s.statusDone = true
	if s.config == nil || len(s.config.Status.Layout) == 0 {
		return nil
	}
	location, ok := s.res.TryGet(s.config.Status.Layout)
	if !ok || location == nil {
		core.Logf("ui: status layout '%s' not found", s.config.Status.Layout)
		return nil
	}
	layout, err := config.ParseStatusScreen(location.Read())
	if err != nil {
		core.Logf("ui: status layout parse failed: %v", err)
		return nil
	}
	s.statusLayout = layout
	return layout
}

func (s *Session) drawStatusScreen(canvas *gfx.BgraCanvas) {
	// CHART 是 status_layout 的另一种页：底图/按钮在布局里，格子/连线在
	// chart.db 里。两者的绘制入口分开，避免把流程图画成 inventory 的空底图。
	if s.ui != nil && s.ui.chart != nil {
		s.drawChartScreen(canvas, s.ui.chart)
		return
	}
	layout := s.statusLayout
	if layout == nil || canvas == nil {
		return
	}
	s.drawLayerAt(canvas, layout.Base, 0, 0)
	// 布局目前只有一页（inventory）。若以后出现多页，页签信息也在布局里，
	// 但在确认格式前只画第一页，避免把几页底图叠在一起。
	if len(layout.Pages) > 0 {
		page := layout.Pages[0]
		if page.Achievement != nil {
			// 牝教師3 的 achievement 页没有 slot：底图 + 两列文字 + 滚动条。
			// 如果仍按 slot 画，玩家只会看到一张空底图，"状態"按钮看起来
			// 像是没反应。
			s.drawStatusAchievements(canvas, page, layout)
		} else {
			s.drawLayerAt(canvas, page.Base, 0, 0)
			for _, slot := range page.Slots {
				if s.statusConditionMet(slot.Condition) {
					s.drawLayerAt(canvas, slot.Image, slot.X, slot.Y)
				}
			}
		}
	}
	for index, button := range layout.ModeButtons {
		frame := 0
		if s.ui != nil && s.ui.hover.kind == hitStatusButton && s.ui.hover.index == index {
			frame = 1
		}
		s.drawFrameButton(canvas, statusButton(button), frame)
	}
}

// drawStatusAchievements 画成就列表。
//
// 行高来自布局的 interval.y（本作 30），可见行数由 scroll bar 的 max_no
// 给出；没有 max_no 时按屏幕高度推算。未解锁且 secret 的条目按原作用
// "？？？" 隐藏，未解锁的非 secret 条目仍显示名字但用 lock 色。
func (s *Session) drawStatusAchievements(canvas *gfx.BgraCanvas, page config.StatusPage, layout *config.StatusScreenLayout) {
	achievement := page.Achievement
	if achievement == nil || axisNil(canvas) {
		return
	}
	table := s.ensureAchievements()
	if table == nil {
		return
	}
	entries := table.Entries()
	gap := max(1, achievement.RowGap)
	rows := achievement.ScrollBar.MaxNo
	if rows <= 0 {
		rows = max(1, (canvas.Height-achievement.NameY)/gap)
	}
	maxScroll := max(0, len(entries)-rows)
	scroll := clampInt(s.statusScroll, 0, maxScroll)
	s.statusScroll = scroll
	size := max(8, achievement.FontSize)
	nameWidth := max(40, achievement.NotesX-achievement.NameX-8)
	notesWidth := max(40, canvas.Width-achievement.NotesX-achievement.ScrollBar.Width-24)
	for row := 0; row < rows; row++ {
		index := scroll + row
		if index >= len(entries) {
			break
		}
		entry := entries[index]
		unlocked := s.achievementUnlocked(entry.ID)
		name, notes := entry.Name, entry.Notes
		if !unlocked && entry.Secret {
			name, notes = "？？？", "？？？？？"
		}
		color := achievement.UnlockColor
		if !unlocked {
			color = achievement.LockColor
		}
		y := achievement.NameY + row*gap
		if nameImage := s.textFont.Render(name, size, 1, 1, gap-4, nameWidth, 1, color); nameImage != nil {
			rendering.DrawTextImage(canvas, nameImage, achievement.NameX, y)
		}
		if notesImage := s.textFont.Render(notes, size, 1, 1, gap-4, notesWidth, 1, color); notesImage != nil {
			rendering.DrawTextImage(canvas, notesImage, achievement.NotesX, achievement.NotesY+row*gap)
		}
	}
	s.drawStatusScrollBar(canvas, achievement.ScrollBar, scroll, maxScroll, rows, len(entries))
}

func axisNil(canvas *gfx.BgraCanvas) bool {
	return canvas == nil
}

// drawStatusScrollBar 画成就页右侧滚动条。原图 status_bar 是竖条，
// 引擎只画滑块位置（没有单独 thumb 素材），用半透明亮条表示。
func (s *Session) drawStatusScrollBar(canvas *gfx.BgraCanvas, bar config.StatusScrollBar, scroll, maxScroll, rows, total int) {
	if bar.Name == "" {
		return
	}
	image := s.composer.Load(bar.Name)
	if image == nil {
		return
	}
	canvas.Draw(image, bar.X, bar.Y)
	if maxScroll <= 0 || total <= 0 {
		return
	}
	width := image.Width
	if bar.Width > 0 {
		width = min(width, bar.Width)
	}
	thumbHeight := max(20, image.Height*rows/total)
	fraction := float64(scroll) / float64(maxScroll)
	thumbY := bar.Y + int(fraction*float64(max(image.Height-thumbHeight, 0)))
	canvas.Fill(bar.X, thumbY, max(4, width-4), thumbHeight, 255, 255, 255, 170)
}

func statusButton(button config.StatusModeButton) config.UIButton {
	return config.UIButton{
		Image:  button.Name,
		Frames: max(1, button.MaxState),
		X:      button.X,
		Y:      button.Y,
	}
}

func (s *Session) hitStatusScreen(x, y int) uiHit {
	if s.ui != nil && s.ui.chart != nil {
		return s.hitChartScreen(x, y)
	}
	layout := s.statusLayout
	if layout == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	for index, button := range layout.ModeButtons {
		image := s.composer.Load(button.Name)
		if image == nil {
			continue
		}
		frames := max(1, button.MaxState)
		height := image.Height / frames
		if height < 1 {
			height = 1
		}
		if x >= button.X && y >= button.Y && x < button.X+image.Width && y < button.Y+height {
			return uiHit{kind: hitStatusButton, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateStatusScreen(hit uiHit) {
	if s.ui != nil && s.ui.chart != nil {
		s.activateChartScreen(hit)
		return
	}
	switch hit.kind {
	case hitStatusButton:
		// 本作只有 inventory_back：按返回即关闭，回到剧情。
		// 若以后布局带页签，可按 button.Name 分派，这里保持最小语义。
		s.closeSystemUI()
	case hitNone:
		// 布局没有 mode button（Immoral 的 inventory_back 被注释掉）时，玩家只能用
		// 右键/ESC 返回；手机没有这些键，所以点击空白处也关闭，避免卡在画面里。
		if s.statusLayout != nil && len(s.statusLayout.ModeButtons) == 0 {
			s.closeSystemUI()
		}
	}
}

// statusConditionMet 求值布局里的 condition。
//
// 语法：`#属性 运算符 整数`，可用 && / || 组合。未知属性按 0 处理：
// 条件写 `!= 0` 的隐藏图不会显示，写 `== 1` 的也不会误显示，符合"没解锁"。
func (s *Session) statusConditionMet(condition string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	for _, alternative := range strings.Split(condition, "||") {
		if s.statusConditionAnd(alternative) {
			return true
		}
	}
	return false
}

func (s *Session) statusConditionAnd(condition string) bool {
	for _, term := range strings.Split(condition, "&&") {
		if !s.statusConditionTerm(term) {
			return false
		}
	}
	return true
}

func (s *Session) statusConditionTerm(term string) bool {
	term = strings.TrimSpace(term)
	if term == "" {
		return true
	}
	for _, operator := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		index := strings.Index(term, operator)
		if index < 0 {
			continue
		}
		name := strings.TrimSpace(term[:index])
		right := strings.TrimSpace(term[index+len(operator):])
		value, ok := parseStatusInt(right)
		if !ok {
			return false
		}
		// 布局用 `@名字` 引用变量、`#名字` 引用属性，两者分开存放，缺失时按 0 处理。
		current, _ := s.titleScriptValue(name)
		switch operator {
		case "==":
			return current == value
		case "!=":
			return current != value
		case ">=":
			return current >= value
		case "<=":
			return current <= value
		case ">":
			return current > value
		case "<":
			return current < value
		}
	}
	return false
}

// parseStatusInt 解析条件右侧的十进制整数（允许负号）。
func parseStatusInt(text string) (int, bool) {
	if text == "" {
		return 0, false
	}
	negative := false
	if text[0] == '+' || text[0] == '-' {
		negative = text[0] == '-'
		text = text[1:]
	}
	if text == "" {
		return 0, false
	}
	value := 0
	for index := 0; index < len(text); index++ {
		if text[index] < '0' || text[index] > '9' {
			return 0, false
		}
		value = value*10 + int(text[index]-'0')
	}
	if negative {
		value = -value
	}
	return value, true
}
