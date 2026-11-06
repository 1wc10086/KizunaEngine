package session

import (
	"fmt"
	"sort"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_mk2_strip.go 是「牝教師2～淫悦の学舎～」（MK2，BSScript Rev.6）的
// 立ち絵鉴赏页。
//
// 为什么需要单独实现：这一代的鉴赏室全部编译在 mk2.exe 里，既没有
// museum_buex_layout（MK2IE 才有），也没有 bsxx.cfg 的 [museum phase - bu]。
// 素材能说明的结构是：
//   - museum_chs 的第 1 个部件是 572x574 的装饰底板（219,20），
//     第 2..8 个部件是 7 张 170x180 的角色卡（各自带屏幕坐标）；
//   - 立绘本体是 bu + 角色2位 + 6位编号（bu01000011，400x600）；
//   - 背景是 gb + 5位编号 + a/b（gb00001a，800x600），
//     对应 [main window] bg signature = gb[0-9]{5}[a-z]；
//   - 7 张卡片与 bu 的角色顺序不是 1..7：对比头像后是
//     bu01, bu02, bu03, bu04, bu06, bu07, bu05。
//
// 原引擎的精确按钮坐标没有可核对的常量表，这里按手册的玩法还原：
// 选择页点角色卡，查看页左右换差分、点「背景」换背景、右键/「戻る」回选择页。
// 所有绘制/命中都用同一套常量，避免两边几何漂移。

// mk2StripOrder 把卡片下标映射成 bu 的角色号（1 基）。
var mk2StripOrder = []int{1, 2, 3, 4, 6, 7, 5}

// mk2StripControl 是查看页底部的一颗文字按钮。
type mk2StripControl struct {
	kind  uiHitKind
	x     int
	y     int
	width int
	label string
}

// mk2StripControls 是查看页的控制条。按钮全部画在底部黑条上，命中矩形
// 与绘制位置共用同一份数据。
var mk2StripControls = []mk2StripControl{
	{kind: hitMK2Back, x: 16, y: 548, width: 90, label: "戻る"},
	{kind: hitMK2Prev, x: 118, y: 548, width: 130, label: "< 差分"},
	{kind: hitMK2BG, x: 260, y: 548, width: 120, label: "背景"},
	{kind: hitMK2Next, x: 392, y: 548, width: 130, label: "差分 >"},
}

const (
	mk2StripControlHeight = 34
	mk2StripBarTop        = 536
	mk2StripBarHeight     = 64
)

// mk2StripState 是立ち絵鉴赏的运行状态。
type mk2StripState struct {
	// character 是卡片下标；-1 表示停在角色选择页。
	character int
	variant   int
	bg        int
	// variants 是当前角色可浏览的 bu 立绘名（按名字排序）。
	variants []string
	// backgrounds 是 gb 背景候选（按名字排序）。
	backgrounds []string
}

// openMK2StripMuseum 打开这一代的立ち絵页。
//
// 已经停在该页时只回到角色选择页，不重建状态：玩家调整过的差分/背景
// 在切走再切回来时还能保留（与 buex 页的处理一致）。
func (s *Session) openMK2StripMuseum() bool {
	if s.ui == nil || s.composer == nil || s.config == nil || !s.config.Museum.MK2Strip {
		return false
	}
	if s.ui.kind == uiMuseumStrip && s.ui.mk2Strip != nil {
		s.ui.mk2Strip.character = -1
		s.composer.Invalidate()
		return true
	}
	state := &mk2StripState{character: -1}
	state.backgrounds = fallbackBackgroundNames(s.resourceImageNames())
	s.ui.kind = uiMuseumStrip
	s.ui.mk2Strip = state
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	core.Logf("ui: mk2 standing gallery opened (%d background(s))", len(state.backgrounds))
	return true
}

// mk2StripVariants 收集一个角色的 bu 立绘名。
//
// 只接受 bu + 2 位角色号 + 6 位数字的 10 字符名字：更长的
// bu0100001101 是"带表情/差分"的合成层，*eye 是眼睛层，单独显示会缺身体。
func (s *Session) mk2StripVariants(character int) []string {
	prefix := fmt.Sprintf("bu%02d", character)
	result := make([]string, 0, 256)
	for _, name := range s.resourceImageNames() {
		if len(name) != 10 || !strings.HasPrefix(name, prefix) {
			continue
		}
		if !allDigits(name[4:]) {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// drawMK2StripMuseum 画选择页或查看页。
func (s *Session) drawMK2StripMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.mk2Strip == nil || s.config == nil {
		return
	}
	layout := s.config.Museum
	state := ui.mk2Strip
	s.drawImageAt(canvas, layout.Base, 0, 0)
	if state.character < 0 {
		if !layout.StripBackdrop.Empty() {
			s.drawFrameButton(canvas, layout.StripBackdrop, 0)
		}
		for index, card := range layout.StripPortraits {
			s.drawFrameButton(canvas, card, 0)
			if s.ui.hover.kind == hitMK2Char && s.ui.hover.index == index {
				if image := s.buttonImage(card); image != nil {
					// 悬停描一圈白框：卡片本身是单帧，无法靠换帧表示 hover。
					canvas.Fill(card.X-2, card.Y-2, image.Width+4, 2, 255, 255, 255, 220)
					canvas.Fill(card.X-2, card.Y+image.Height, image.Width+4, 2, 255, 255, 255, 220)
					canvas.Fill(card.X-2, card.Y-2, 2, image.Height+4, 255, 255, 255, 220)
					canvas.Fill(card.X+image.Width, card.Y-2, 2, image.Height+4, 255, 255, 255, 220)
				}
			}
		}
		s.drawMuseumTabs(canvas)
		return
	}
	// 查看页：背景铺满整屏，立绘居中（400x600），底部控制条压在立绘下缘。
	if len(state.backgrounds) > 0 {
		s.drawImageAt(canvas, state.backgrounds[state.bg], 0, 0)
	}
	if len(state.variants) > 0 {
		if image := s.composer.Load(state.variants[state.variant]); image != nil {
			canvas.Draw(image, (canvas.Width-image.Width)/2, max(0, canvas.Height-image.Height))
		}
	}
	canvas.FillBlend(0, mk2StripBarTop, canvas.Width, mk2StripBarHeight, 0, 0, 0, 170)
	for _, control := range mk2StripControls {
		s.drawMK2StripButton(canvas, control)
	}
	// 右上角状态：差分序号与背景名，方便玩家确认当前组合。
	label := ""
	if len(state.variants) > 0 {
		label = fmt.Sprintf("%d/%d", state.variant+1, len(state.variants))
	}
	if len(state.backgrounds) > 0 {
		label += "  " + state.backgrounds[state.bg]
	}
	if label != "" && s.textFont != nil {
		rendering.DrawText(canvas, s.textFont, label, 616, 8, 16, 1, 1, 20, 180, 1, 0xFFFFFFFF)
	}
	s.drawMuseumTabs(canvas)
}

func (s *Session) drawMK2StripButton(canvas *gfx.BgraCanvas, control mk2StripControl) {
	if s.textFont == nil {
		return
	}
	color := uint32(0xFFFFFFFF)
	if s.ui != nil && s.ui.hover.kind == control.kind {
		color = 0xFFFFFF66
	}
	rendering.DrawText(canvas, s.textFont, control.label, control.x, control.y, 18, 1, 1, 22, control.width, 1, color)
}

// hitMK2Strip 命中选择页卡片与查看页控制条。
func (s *Session) hitMK2Strip(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.mk2Strip == nil || s.config == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	layout := s.config.Museum
	state := ui.mk2Strip
	if state.character < 0 {
		for index, card := range layout.StripPortraits {
			image := s.buttonImage(card)
			if image == nil {
				continue
			}
			if x >= card.X && y >= card.Y && x < card.X+image.Width && y < card.Y+image.Height {
				return uiHit{kind: hitMK2Char, index: index}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	for _, control := range mk2StripControls {
		if x >= control.x && y >= control.y &&
			x < control.x+control.width && y < control.y+mk2StripControlHeight {
			return uiHit{kind: control.kind, index: -1}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateMK2Strip 处理选择页/查看页的点击。
func (s *Session) activateMK2Strip(hit uiHit) {
	ui := s.ui
	if ui == nil || ui.mk2Strip == nil {
		return
	}
	state := ui.mk2Strip
	switch hit.kind {
	case hitMK2Char:
		if hit.index < 0 || hit.index >= len(mk2StripOrder) {
			return
		}
		state.character = hit.index
		state.variants = s.mk2StripVariants(mk2StripOrder[hit.index])
		state.variant = 0
		if len(state.backgrounds) == 0 {
			state.backgrounds = fallbackBackgroundNames(s.resourceImageNames())
		}
		state.bg = clampInt(state.bg, 0, max(0, len(state.backgrounds)-1))
		core.Logf("ui: mk2 standing viewer character=%d variants=%d", mk2StripOrder[hit.index], len(state.variants))
	case hitMK2Back:
		state.character = -1
	case hitMK2Prev:
		state.variant = mk2StripStep(state.variant, -1, len(state.variants))
	case hitMK2Next:
		state.variant = mk2StripStep(state.variant, 1, len(state.variants))
	case hitMK2BG:
		if len(state.backgrounds) > 0 {
			state.bg = (state.bg + 1) % len(state.backgrounds)
		}
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// backMK2StripViewer 处理右键：查看页先回选择页，选择页交给默认关闭逻辑。
// 返回 true 表示已经消费这次右键。
func (s *Session) backMK2StripViewer() bool {
	if s.ui == nil || s.ui.kind != uiMuseumStrip || s.ui.mk2Strip == nil {
		return false
	}
	if s.ui.mk2Strip.character < 0 {
		return false
	}
	s.ui.mk2Strip.character = -1
	s.composer.Invalidate()
	return true
}

// wheelMK2Strip 用滚轮换背景：查看页最常调整的是背景组合。
func (s *Session) wheelMK2Strip(delta int32) bool {
	if s.ui == nil || s.ui.kind != uiMuseumStrip || s.ui.mk2Strip == nil {
		return false
	}
	state := s.ui.mk2Strip
	if state.character < 0 || len(state.backgrounds) == 0 {
		return false
	}
	state.bg = mk2StripStep(state.bg, int(delta), len(state.backgrounds))
	s.composer.Invalidate()
	return true
}

func mk2StripStep(value, delta, count int) int {
	if count <= 0 {
		return 0
	}
	return ((value+delta)%count + count) % count
}
