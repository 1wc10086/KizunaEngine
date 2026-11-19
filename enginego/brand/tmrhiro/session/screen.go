// 本文件把 VM 收集的"脚本定义界面"画出来并处理命中。
//
// 原版这些页面全部由脚本定义：底图来自 0x92（或 op4/0xc3 的四张图），每一格
// 的矩形与跳转标签来自 op4/0x64，页面按钮（PREVIOUS/RETURN/NEXT）来自 0x23。
// 引擎只是照这些数字贴图 + 画字；玩家点格子时，引擎把同一行的"标签"
// 交回脚本（`TitleSelect`），所以跳转逻辑仍属于原作。
//
// 分类：view（CG/场景/BGM 鉴赏）、chara（立绘查看）、select（站选择）、
// confirm（PERCENT 100/0 这类选择框）。分类依据是图形集与按钮标签的组合，
// 不写死在脚本名上——同一条 `view` 脚本里有 20 多个页面。
package session

import (
	"strings"

	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/brand/tmrhiro/vm"
)

// syncScreen 把 VM 的界面状态取回 session（在 absorb 末尾调用）。
//
// 只保留脚本真的开过界面的情况：0x82 开一张新的界面 → vm.ScreenState 非空；
// 界面定义收尾（那条 0x10）后 VM 停在 StepAwaitTitle，此时界面仍然有效，
// 玩家点格子/按钮后脚本按标签跳转。
func (s *Session) syncScreen() {
	screen := s.vm.ScreenState()
	if screen == nil {
		s.screen = nil
		return
	}
	s.screen = screen
	// 界面菜单的悬停下标由 Pointer 维护，但换界面时必须清掉，
	// 否则新页面的第一格会带着上一页的高亮。
	if s.screenHover >= len(screen.Buttons) {
		s.screenHover = -1
	}
}

// composeScreen 画脚本定义的界面（鉴赏/立绘/选择/确认）。
//
// 层序：底图（或 cgmodeNN 整屏图）→ 网格单元 → 页面按钮 → 文字。
// 网格里的缩略图由 session 现场合成（fm_ 宏 → 部件贴到一起），
// 未完成的格子用 Graphic.Incomplete 盖一层 "NO DATA"。
func (s *Session) composeScreen() {
	screen := s.screen
	if screen == nil {
		return
	}
	origin := 0
	base := screen.Base
	if base == "" {
		base = screen.Graphic.Base
	}
	if base != "" {
		if img := s.cache.Get(base); img != nil {
			// 界面的整屏图（menu/cgmode01/hsmode01…）都是 1280x720、偏移 0，直接贴。
			s.stage.DrawImage(img, img.OffsetX, img.OffsetY+origin)
		}
	}
	// 页面按钮（PREVIOUS/RETURN/NEXT、CG GALLERY 的页签等）已经画在底图上，
	// 只需要把悬停那一块换成 *_on 的高亮版本。
	// 注意不能整张铺 *_on：它把所有按钮都画成亮色，看起来像全部被选中。
	if s.screenHover >= 0 && screen.BaseOn != "" {
		if rect, ok := s.screenHoverRect(); ok && rect.X >= 0 && rect.W > 0 {
			if hover := s.cache.Get(screen.BaseOn); hover != nil {
				s.stage.DrawRegion(hover, rect.X, rect.Y, rect.W, rect.H, rect.X, rect.Y)
			}
		}
	}
	// 网格单元。
	s.drawScreenItems(screen)
	// 角色选择（op1/0xb2 + 0x43）：底图、三张立绘卡、卡上的选中高亮。
	s.drawCharaSelect(screen)
	// 达成率：原版在右上角画「達成率 NN%」。
	if screen.Percent >= 0 {
		s.drawScreenPercent(screen)
	} else if len(screen.Items) > 0 {
		// 鉴赏页（CGMODE/HSCMODE/MUSMODE）的底图上写着「達成率 __%」，
		// 数值不在脚本里，而是全局解锁位算出来的。以前 Percent 恒为 -1，
		// 于是数字位置永远空着（"达成率什么都不显示"）。
		s.drawPercentValue(s.galleryPercent())
	}
}

// drawCharaSelect 画 CHARACTER SELECT（立绘选择界面）。
//
// 结构：cha_select_base 整屏底图 + select_button 图集里的三张立绘卡。
//
// select_button 是 720x2388 的 3 列 × 8 行图集（每格 240x298）：
//   - 第 0 行 常态；
//   - 第 1 行 选中（卡片发亮，原版用这一行表示"这套立绘被选中"）；
//   - 第 7 行 灰色 "complete"（剧情里已完成的立绘，原版压暗并写 complete）。
//
// 0x43 的每条记录给一张卡的位置与尺寸（见 vm.addCharaButton），
// 顺序即第 0/1/2 张。以前整个界面没有任何绘制代码，
// 玩家点 CHARACTER SELECT 只会看到底图（或者干脆什么都没画）。
func (s *Session) drawCharaSelect(screen *vm.Screen) {
	if screen.Chara.Base == "" && len(screen.Buttons) == 0 {
		return
	}
	base := screen.Chara.Base
	if base != "" {
		if img := s.cache.Get(base); img != nil {
			s.stage.DrawImage(img, img.OffsetX, img.OffsetY)
		}
	}
	atlas := s.cache.Get(screen.Chara.Button)
	if atlas == nil {
		return
	}
	// 图集是 3 列 × N 行，每格宽 = 图宽/3、高 = 图高/8。
	const cols, rows = 3, 8
	cellW := atlas.Width / cols
	cellH := atlas.Height / rows
	if cellW <= 0 || cellH <= 0 {
		return
	}
	for index, button := range screen.Buttons {
		if button.W <= 0 || button.H <= 0 {
			continue
		}
		col := index % cols
		// 第 0 行常态；悬停的那张用第 1 行（发亮）。
		row := 0
		if index == s.screenHover {
			row = 1
		}
		if row >= rows {
			row = rows - 1
		}
		s.stage.DrawRegionScaled(atlas, col*cellW, row*cellH, cellW, cellH,
			button.X, button.Y, button.W, button.H, 255)
	}
}

// drawScreenPageButtons 画页面按钮（PREVIOUS/RETURN/NEXT、CGMODE 页签），
// 直接从当前底图里裁出按钮那一块再贴上：这些按钮不是独立贴图，而是画在
// cgmodeNN/hsmodeNN/menu 底图上的文字，原版就是用同一张图重贴一次表示按下。
//
// 高亮态用 *_on 底图的同一矩形（原版把所有按钮的亮色版本画在另一张整屏图上）。
func (s *Session) drawScreenPageButtons(screen *vm.Screen) {
	if screen.Base == "" {
		return
	}
	base := s.cache.Get(screen.Base)
	hover := s.cache.Get(screen.BaseOn)
	index := 0
	for _, item := range screen.Buttons {
		rect := config.Rect{X: item.X, Y: item.Y, W: item.W, H: item.H}
		if rect.W <= 0 || rect.H <= 0 {
			index++
			continue
		}
		if hover != nil && index == s.screenHover {
			s.stage.DrawRegion(hover, rect.X, rect.Y, rect.W, rect.H, rect.X, rect.Y)
			index++
			continue
		}
		if base != nil {
			s.stage.DrawRegion(base, rect.X, rect.Y, rect.W, rect.H, rect.X, rect.Y)
		}
		index++
	}
}

// drawScreenButtons 画（并高亮）界面的按钮。
//
// 按钮没有独立贴图：高亮态来自 hover 底图（"xxx_on"），也就是原版
// "同一张整屏图、只是按钮区域换成亮色版"。所以这里按按钮矩形裁 hover 图
// 的同一块贴上去，而不是整张铺开——整张铺开会让所有按钮同时发亮。
func (s *Session) drawScreenButtons(screen *vm.Screen) {
	for index, button := range screen.Buttons {
		if index != s.screenHover {
			continue
		}
		hover := screen.BaseOn
		if hover == "" {
			hover = screen.Graphic.BaseOn
		}
		if hover == "" {
			continue
		}
		if img := s.cache.Get(hover); img != nil {
			s.stage.DrawRegion(img, button.X, button.Y, button.W, button.H, button.X, button.Y)
		}
	}
}

// drawScreenItems 画网格单元。
func (s *Session) drawScreenItems(screen *vm.Screen) {
	for index, item := range screen.Items {
		switch item.Kind {
		case vm.ScreenItemCG:
			s.drawCgCell(screen, item, index)
		case vm.ScreenItemScene:
			s.drawSceneCell(screen, item, index)
		case vm.ScreenItemBGM:
			s.drawBGMRow(screen, item, index)
		}
	}
}

// drawCgCell 画一个 CG 格子：缩略图 + （未解锁时）NO DATA 覆盖。
func (s *Session) drawCgCell(screen *vm.Screen, item vm.ScreenItem, index int) {
	if len(item.Names) == 0 {
		s.drawMissingCell(screen, item)
		return
	}
	name := item.Names[0]
	unlocked := s.CgUnlockedByName(name)
	if !unlocked {
		s.drawMissingCell(screen, item)
		return
	}
	frame := s.stage.ResolveImage(name)
	if frame == nil || frame.Width <= 0 || frame.Height <= 0 {
		s.drawMissingCell(screen, item)
		return
	}
	// 原版把整屏 1280x720 的 CG 等比缩进 180x104 的格子（两者都是 16:9，
	// 所以缩放会铺满整格，不需要裁切）。
	s.stage.DrawImageScaled(frame, item.X, item.Y, item.W, item.H, 255)
}

// drawSceneCell 画场景单元：封面取该脚本里的第一张 CG。
func (s *Session) drawSceneCell(screen *vm.Screen, item vm.ScreenItem, index int) {
	if len(item.Names) == 0 {
		s.drawMissingCell(screen, item)
		return
	}
	// 解锁位按"脚本名"查，而不是按网格下标：HSCMODE 的每一页是按顺序列出
	// 若干脚本名（02_04、02_05…），下标与 _var 的 [HSC] 全局顺序不一致，
	// 用 index 查会把"看过的第一页"画成 NO DATA、把没看过的画成缩略图。
	if !s.sceneUnlockedByName(item.Names[0]) {
		s.drawMissingCell(screen, item)
		return
	}
	cover := s.sceneCover(item.Names[0])
	if cover == nil {
		s.drawMissingCell(screen, item)
		return
	}
	s.stage.DrawImageScaled(cover, item.X, item.Y, item.W, item.H, 255)
	s.drawUIRun(item.Names[0], item.X+8, item.Y+item.H-24, 16, 0xffffffff, item.W-16)
}

// drawBGMRow 画一行 BGM：只有文字（原版是"标题 + 曲名"，没有缩略图）。
func (s *Session) drawBGMRow(screen *vm.Screen, item vm.ScreenItem, index int) {
	if len(item.Names) == 0 {
		return
	}
	s.drawUIRun(item.Names[0], item.X+8, item.Y+6, 20, 0xffffffff, item.W-16)
	if index == s.screenHover {
		if img := s.cache.Get(screen.BaseOn); img != nil {
			s.stage.DrawRegion(img, item.X, item.Y, item.W, item.H, item.X, item.Y)
		}
	}
}

// drawMissingCell 画"空位/未完成"格子：原版的 nodata / incomplete 贴图。
func (s *Session) drawMissingCell(screen *vm.Screen, item vm.ScreenItem) {
	image := screen.Graphic.Empty
	if image == "" {
		image = "nodata"
	}
	if img := s.cache.Get(image); img != nil {
		// nodata 是 180x104，比格子小时居中；比格子大时缩到格子内。
		if img.Width > item.W || img.Height > item.H {
			s.stage.DrawImageScaled(img, item.X, item.Y, item.W, item.H, 255)
			return
		}
		x := item.X + (item.W-img.Width)/2
		y := item.Y + (item.H-img.Height)/2
		s.stage.DrawImage(img, x, y)
	}
}

// drawScreenPercent 画达成率（右上角「達成率 NN%」）。
//
// 原版底图 cgmodeNN 上已经画好「達成率」和「%」两个字样，中间的数字用
// sys_number 图集（197x26 的 10 个数字，每个 20x26）逐位贴上去。
// 以前这里用 drawUIRun 把整句文字画一遍：底图上的「%」还在，中间却又多出
// 一段文字，字号与位置都对不上，看上去就是"达成率没正常显示"。
// 现在按原版做法只贴数字，x 从 VIEW_CG_PERCENT 的坐标起逐位推进。
func (s *Session) drawScreenPercent(screen *vm.Screen) {
	s.drawPercentValue(screen.Percent)
}

// drawPercentValue 按数值画达成率数字（没有界面上下文时也能用）。
func (s *Session) drawPercentValue(percent int) {
	v := s.ui.View
	x, y := v.PercentX, v.PercentY
	if x <= 0 {
		x = 1031
	}
	if y <= 0 {
		y = 86
	}
	digits := itoaPercent(percent)
	img := s.cache.Get("sys_number")
	if img == nil {
		// 没有数字图集时退回文字（其它世代的作品仍能显示）。
		s.drawUIRun("達成率 "+digits+"%", x, y, 20, 0xffffffff, 200)
		return
	}
	// 图集是 10 个等宽数字：第 i 个的源 x = i*字数宽，宽度 = 图宽/10。
	digitW := img.Width / 10
	if digitW <= 0 {
		return
	}
	unitW := v.NumberW
	if unitW <= 0 {
		unitW = digitW
	}
	unitH := v.NumberH
	if unitH <= 0 {
		unitH = img.Height
	}
	cx := x
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			continue
		}
		src := int(ch-'0') * digitW
		s.stage.DrawRegionScaled(img, src, 0, digitW, img.Height, cx, y, unitW, unitH, 255)
		cx += unitW
	}
}

func itoaPercent(value int) string {
	if value <= 0 {
		return "0"
	}
	if value > 100 {
		value = 100
	}
	return itoa(value)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	out := make([]byte, 0, 4)
	for value > 0 {
		out = append([]byte{byte('0' + value%10)}, out...)
		value /= 10
	}
	return string(out)
}

// hitScreenButton 返回命中的按钮下标（含网格单元；-1 表示没命中）。
//
// 网格单元也是"按钮"：点 CG 格子等于选那个宏对应的页面。单元没有单独的
// 标签时，用它的名字（原版就是把宏名当标签传给 0x10 跳转）。
func (s *Session) hitScreenButton(x, y int) int {
	screen := s.screen
	if screen == nil {
		return -1
	}
	for index, item := range screen.Items {
		if item.W <= 0 || item.H <= 0 {
			continue
		}
		rect := config.Rect{X: item.X, Y: item.Y, W: item.W, H: item.H}
		if rect.Contains(x, y) {
			return len(screen.Buttons) + index
		}
	}
	for index, button := range screen.Buttons {
		rect := config.Rect{X: button.X, Y: button.Y, W: button.W, H: button.H}
		if rect.Contains(x, y) {
			return index
		}
	}
	return -1
}

// screenLabel 返回某个命中下标对应的跳转标签。
func (s *Session) screenLabel(index int) string {
	screen := s.screen
	if screen == nil || index < 0 {
		return ""
	}
	if index < len(screen.Buttons) {
		return screen.Buttons[index].Label
	}
	itemIndex := index - len(screen.Buttons)
	if itemIndex < len(screen.Items) {
		item := screen.Items[itemIndex]
		if item.Label != "" {
			return item.Label
		}
		if len(item.Names) > 0 {
			return item.Names[0]
		}
	}
	return ""
}

// screenHoverLabel 用于绘制：网格单元悬停时返回它的矩形。
func (s *Session) screenHoverRect() (config.Rect, bool) {
	screen := s.screen
	if screen == nil || s.screenHover < 0 {
		return config.Rect{}, false
	}
	if s.screenHover < len(screen.Buttons) {
		b := screen.Buttons[s.screenHover]
		return config.Rect{X: b.X, Y: b.Y, W: b.W, H: b.H}, true
	}
	index := s.screenHover - len(screen.Buttons)
	if index >= 0 && index < len(screen.Items) {
		item := screen.Items[index]
		return config.Rect{X: item.X, Y: item.Y, W: item.W, H: item.H}, true
	}
	return config.Rect{}, false
}

// screenDefaultOr 返回界面的默认标签；没有时用给定的后备名。
func (s *Session) screenDefaultOr(fallback string) string {
	if s.screen != nil && s.screen.Default != "" {
		return s.screen.Default
	}
	return fallback
}

// displayName 把脚本名整理成可读标题（只用于日志/无障碍文本）。
func displayName(name string) string {
	return strings.TrimSpace(name)
}
