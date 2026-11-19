package session

import (
	"strings"

	"kizuna/enginego/brand/emon/stage"
	"kizuna/enginego/brand/emon/vm"
	"kizuna/enginego/core"
)

// MapButtons 对 Emon 返回空：ASENURE 的标题、系统菜单、存读档槽位和调教
// 选项全部是脚本 SELECT sprite，由引擎的 Pointer 触发器绘制并命中。若把
// 这些 sprite 交给宿主 MapLayer，Flutter 会判定 overlayActive、用
// chooseMapPixel 抢走点击，引擎的 SPRCLICK 永远收不到（表现为主界面点
// 任何按钮都没反应）。其他品牌各自的 session 仍按需实现。
func (s *Session) MapButtons() []core.MapButton {
	return nil
}

func (s *Session) MapMask() (bool, int, int) {
	return false, 0, 0
}

func (s *Session) visibleSelectSprites() []*stage.EmonSprite {
	if s.stage == nil {
		return nil
	}
	if s.step != vm.EmonStepAwaitChoice && s.step != vm.EmonStepAwaitAction {
		return nil
	}
	if s.machine != nil && s.machine.BackLogActive() {
		return nil
	}
	var visible []*stage.EmonSprite
	for _, sprite := range s.stage.SelectSprites() {
		if !sprite.Visible() || !isChainVisible(sprite) || isMessageWindowSelect(sprite) {
			continue
		}
		visible = append(visible, sprite)
	}
	return visible
}

func (s *Session) publishSelect() {
	visible := s.visibleSelectSprites()
	if len(visible) == 0 {
		if s.selectShown {
			s.selectNames = nil
			s.selectSig = 0
			s.selectShown = false
		}
		return
	}
	signature := selectSignature(visible)
	if s.selectShown && signature == s.selectSig {
		return
	}
	s.selectNames = s.selectNames[:0]
	for _, sprite := range visible {
		s.selectNames = append(s.selectNames, sprite.Name())
	}
	s.selectSig = signature
	s.selectShown = true
	core.Logf("emon: select window %d item(s)", len(visible))
}

// isMessageWindowSelect 判断 sprite 是否属于对白窗口的工具条。工具条按钮也带
// SELECT（用于 hover/点击），但它们不是选项；把它们放进 MapButtons/选择窗会让
// 宿主把 セーブ/ロード/ログ 当成剧情选项，点第一个永远停在工具条上。
func isMessageWindowSelect(sprite *stage.EmonSprite) bool {
	for current := sprite.Parent(); current != nil; current = current.Parent() {
		if strings.EqualFold(current.Name(), "メッセージウィンドウ") {
			return true
		}
	}
	return false
}

func selectSignature(sprites []*stage.EmonSprite) int {
	hash := 17
	hash = hash*31 + len(sprites)
	for _, sprite := range sprites {
		for _, c := range sprite.Name() {
			hash = hash*31 + int(c)
		}
		hash = hash*31 + sprite.AbsoluteX()
		hash = hash*31 + sprite.AbsoluteY()
	}
	return hash
}

func (s *Session) resolveChoicePosition(index int) int {
	if len(s.selectNames) == 0 {
		return -1
	}
	if index >= 0 && index < len(s.selectNames) {
		return index
	}
	if index >= 1 && index <= len(s.selectNames) {
		return index - 1
	}
	return -1
}

func (s *Session) selectPosition(position int) vm.EmonStep {
	if s.machine == nil || position < 0 || position >= len(s.selectNames) {
		return s.step
	}
	name := s.selectNames[position]
	if name != "" {
		return s.machine.ChooseSelectSprite(name)
	}
	return s.machine.ChooseSelect(position + 1)
}

func (s *Session) hitButton(x, y int) int {
	// prompt 模态窗优先：它盖在存档/读档页上层，click 区与 SL 槽位重叠时先让模态赢。
	if id, hit := s.hitPromptButton(x, y); hit {
		_ = id
		return -2
	}
	visible := s.visibleSelectSprites()
	for index, sprite := range visible {
		rect := sprite.HitRect()
		width := rect.Width
		height := rect.Height
		if width <= 0 || height <= 0 {
			width = selectItemMinimum
			height = selectItemMinimum
		}
		left := sprite.AbsoluteX()
		top := sprite.AbsoluteY()
		if x >= left && y >= top && x < left+width && y < top+height {
			return index
		}
	}
	return -1
}
