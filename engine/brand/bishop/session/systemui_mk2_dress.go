package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// systemui_mk2_dress.go 是「牝教師2～淫悦の学舎～」的ぬぎぬぎ設定画面。
//
// 原作的 DRESS 按钮打开一个换装界面：先选角色，再按「着衣 / 下着+ストッキング /
// 下着 / 全裸」切换该角色的立绘状态，服装用左右按钮翻。MK2 的发行版没有
// strip_base / strip_ch* / costume_* 这些界面素材（它们编译在 mk2.exe 里），
// 但立绘本身在：bu<角色2><服装2><阶段2><表情2>，400x600。
//
// 这里按"素材能讲的布局"还原：museum_chs 的 7 张角色卡（与鉴赏室同一组）、
// 底部一排阶段按钮、服装左右切换。所有坐标/命中共用一份常量。
// MK2IE 有完整的 [dress screen] 配置，走原来的配置驱动渲染，不进入这里。

type mk2DressState struct {
	// selecting 为真时停在角色选择页。
	selecting bool
	character int
	wear      int
	stage     int
	// choices 记录玩家真正改过的角色 → 服装/阶段；没改过的角色保持剧本
	// 原本的立绘，不会被默认值误改。
	choices map[int]dressLiveChoice
}

type mk2DressButton struct {
	kind  uiHitKind
	x     int
	y     int
	width int
	label string
}

var mk2DressControls = []mk2DressButton{
	{kind: hitMK2Back, x: 16, y: 548, width: 80, label: "戻る"},
	{kind: hitMK2DressStage, x: 106, y: 548, width: 90, label: "着衣"},
	{kind: hitMK2DressStage, x: 202, y: 548, width: 128, label: "下着+スト"},
	{kind: hitMK2DressStage, x: 336, y: 548, width: 84, label: "下着"},
	{kind: hitMK2DressStage, x: 426, y: 548, width: 84, label: "全裸"},
	{kind: hitMK2DressPrev, x: 520, y: 548, width: 70, label: "< 服装"},
	{kind: hitMK2DressNext, x: 596, y: 548, width: 70, label: "服装 >"},
}

// openMK2DressScreen 打开这一代的ぬぎぬぎ設定。
func (s *Session) openMK2DressScreen() bool {
	if s.composer == nil || s.config == nil || !s.config.Dress.MK2 {
		return false
	}
	if s.ui != nil && s.ui.kind == uiDress && s.ui.mk2Dress != nil {
		s.ui.mk2Dress.selecting = true
		s.composer.Invalidate()
		return true
	}
	if s.ui == nil || s.ui.kind == uiMuseum {
		if !s.canOpenSystemUI() {
			return false
		}
		s.beginSystemUI(uiDress)
	} else if s.ui.kind != uiDress {
		return false
	}
	state := &mk2DressState{selecting: true, choices: map[int]dressLiveChoice{}}
	state.character = 0
	state.wear = 1
	state.stage = 0
	s.ui.mk2Dress = state
	s.ui.kind = uiDress
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	core.Logf("ui: mk2 dress screen opened")
	return true
}

func (s *Session) mk2DressName() string {
	state := s.ui.mk2Dress
	if state == nil || state.character < 0 || state.character >= len(mk2StripOrder) {
		return ""
	}
	character := mk2StripOrder[state.character]
	wear := clampInt(state.wear, 0, 13)
	stage := clampInt(state.stage, 0, 3) * 10
	for _, tryWear := range []int{wear, 0} {
		for _, tryStage := range []int{stage, 0} {
			name := fmt.Sprintf("bu%02d%02d%02d11", character, tryWear, tryStage)
			if s.resourceExists(name) {
				return name
			}
		}
	}
	return ""
}

func (s *Session) drawMK2DressScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || ui.mk2Dress == nil {
		return
	}
	state := ui.mk2Dress
	layout := s.config.Museum
	s.drawImageAt(canvas, s.config.Dress.Base, 0, 0)
	if state.selecting {
		if !layout.StripBackdrop.Empty() {
			s.drawFrameButton(canvas, layout.StripBackdrop, 0)
		}
		for index, card := range layout.StripPortraits {
			s.drawFrameButton(canvas, card, 0)
			if s.ui.hover.kind == hitMK2Char && s.ui.hover.index == index {
				if image := s.buttonImage(card); image != nil {
					canvas.Fill(card.X-2, card.Y-2, image.Width+4, 2, 255, 255, 255, 220)
					canvas.Fill(card.X-2, card.Y+image.Height, image.Width+4, 2, 255, 255, 255, 220)
				}
			}
		}
		if s.textFont != nil {
			rendering.DrawText(canvas, s.textFont, "DRESS", 24, 24, 28, 2, 2, 32, 300, 1, 0xFFFFFFFF)
		}
		return
	}
	if name := s.mk2DressName(); len(name) > 0 {
		if image := s.composer.Load(name); image != nil {
			canvas.Draw(image, (canvas.Width-image.Width)/2, max(0, canvas.Height-image.Height))
		}
	}
	canvas.FillBlend(0, 536, canvas.Width, 64, 0, 0, 0, 170)
	for _, control := range mk2DressControls {
		drawMK2DressButton(canvas, s, control, s.ui.hover.kind == control.kind)
	}
	if s.textFont != nil {
		label := fmt.Sprintf("服装 %02d  段階 %d", state.wear, state.stage)
		rendering.DrawText(canvas, s.textFont, label, 600, 8, 16, 1, 1, 20, 180, 1, 0xFFFFFFFF)
	}
}

func drawMK2DressButton(canvas *gfx.BgraCanvas, s *Session, control mk2DressButton, hover bool) {
	if s.textFont == nil {
		return
	}
	color := uint32(0xFFFFFFFF)
	if hover {
		color = 0xFFFFFF66
	}
	rendering.DrawText(canvas, s.textFont, control.label, control.x, control.y, 18, 1, 1, 22, control.width, 1, color)
}

func (s *Session) hitMK2Dress(x, y int) uiHit {
	ui := s.ui
	if ui == nil || ui.mk2Dress == nil || s.config == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	state := ui.mk2Dress
	if state.selecting {
		for index, card := range s.config.Museum.StripPortraits {
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
	for index, control := range mk2DressControls {
		if x >= control.x && y >= control.y && x < control.x+control.width && y < control.y+mk2DressControlHeight {
			hit := uiHit{kind: control.kind, index: -1}
			if control.kind == hitMK2DressStage {
				hit.index = index - 1 // 0..3 = 着衣/下着+スト/下着/全裸
			}
			return hit
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

const mk2DressControlHeight = 34

func (s *Session) activateMK2Dress(hit uiHit) {
	ui := s.ui
	if ui == nil || ui.mk2Dress == nil {
		return
	}
	state := ui.mk2Dress
	switch hit.kind {
	case hitMK2Char:
		if hit.index < 0 || hit.index >= len(mk2StripOrder) {
			return
		}
		state.character = hit.index
		state.selecting = false
	case hitMK2Back:
		state.selecting = true
	case hitMK2DressStage:
		state.stage = clampInt(hit.index, 0, 3)
		s.recordMK2DressChoice(state)
	case hitMK2DressPrev:
		state.wear = mk2StripStep(state.wear, -1, 14)
		s.recordMK2DressChoice(state)
	case hitMK2DressNext:
		state.wear = mk2StripStep(state.wear, 1, 14)
		s.recordMK2DressChoice(state)
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}

// recordMK2DressChoice 记下当前角色的换装选择，并立刻应用到剧情立绘。
func (s *Session) recordMK2DressChoice(state *mk2DressState) {
	if state == nil || state.character < 0 || state.character >= len(mk2StripOrder) {
		return
	}
	if state.choices == nil {
		state.choices = map[int]dressLiveChoice{}
	}
	character := mk2StripOrder[state.character]
	state.choices[character] = dressLiveChoice{wear: state.wear, stage: state.stage}
	s.dressLive = make(map[int]dressLiveChoice, len(state.choices))
	for key, value := range state.choices {
		s.dressLive[key] = value
	}
	s.applyDressLiveToStage()
}

// backMK2DressViewer 处理右键：换装查看页先回角色选择页。
func (s *Session) backMK2DressViewer() bool {
	if s.ui == nil || s.ui.kind != uiDress || s.ui.mk2Dress == nil {
		return false
	}
	if s.ui.mk2Dress.selecting {
		return false
	}
	s.ui.mk2Dress.selecting = true
	s.composer.Invalidate()
	return true
}

func (s *Session) wheelMK2Dress(delta int32) bool {
	if s.ui == nil || s.ui.kind != uiDress || s.ui.mk2Dress == nil || s.ui.mk2Dress.selecting {
		return false
	}
	s.ui.mk2Dress.wear = mk2StripStep(s.ui.mk2Dress.wear, int(delta), 14)
	s.composer.Invalidate()
	return true
}
