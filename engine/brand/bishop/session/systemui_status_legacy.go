package session

import (
	"strconv"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_status_legacy.go 是旧式 [status screen] 的 ITEM 页。
//
// 教育指導（KS / KSIE）没有 define.bsa 的 status_layout：底图 status_base
// 已经把五名角色、每人三组道具空槽与箭头烘焙在整屏里，item111…item533
// 只是取得道具后叠到空槽上的图片。道具状态来自剧本属性：
//   0 = 未取得，1 = 取得第一件，2 = 取得第二件，3 = 完成。
// 因此第 part 张图在属性 >= part 时显示。
//
// 工具条的 ITEM 按钮和地图上的 item 按钮都走 openStatusScreen；这条分支
// 只在其前面拦截"有旧式 ITEM 布局"的作品，其它状态画面不受影响。

// legacyStatusCharacters 是 itemXYZ 的 X（1..5）对应的属性前缀。
// KS/KSIE 的角色表固定，item111 就是 #kiyoka_item1。
var legacyStatusCharacters = []string{"kiyoka", "yuzuki", "chinatsu", "kaede", "eriko"}

func (s *Session) openLegacyStatusScreen() bool {
	if s.config == nil || s.config.LegacyStatus.Empty() {
		return false
	}
	if !s.canOpenSystemUI() {
		return false
	}
	s.beginSystemUI(uiStatusLegacy)
	core.Logf("ui: legacy item screen opened (%d item image(s))", len(s.config.LegacyStatus.Items))
	return s.ui != nil
}

func (s *Session) drawLegacyStatusScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || canvas == nil || s.config == nil {
		return
	}
	layout := s.config.LegacyStatus
	s.drawLayerAt(canvas, layout.Base, 0, 0)
	for _, item := range layout.Items {
		if s.legacyStatusItemState(item) < item.Part {
			continue
		}
		frame := 0
		if item.Button.Frames > 1 {
			frame = 0
		}
		s.drawFrameButton(canvas, item.Button, frame)
	}
	s.drawLegacyStatusButton(canvas, layout.Back, hitBack)
	s.drawLegacyStatusButton(canvas, layout.Title, hitTitle)
	s.drawLegacyStatusButton(canvas, layout.Exit, hitExit)
}

func (s *Session) drawLegacyStatusButton(canvas *gfx.BgraCanvas, button config.UIButton, kind uiHitKind) {
	if button.Empty() {
		return
	}
	frame := 0
	if s.ui != nil && s.ui.hover.kind == kind {
		frame = 1
		if frame >= button.Frames && button.Frames > 0 {
			frame = button.Frames - 1
		}
	}
	s.drawFrameButton(canvas, button, frame)
}

func (s *Session) hitLegacyStatusScreen(x, y int) uiHit {
	if s.config == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	layout := s.config.LegacyStatus
	if hitButton(s, layout.Back, x, y) {
		return uiHit{kind: hitBack}
	}
	if hitButton(s, layout.Title, x, y) {
		return uiHit{kind: hitTitle}
	}
	if hitButton(s, layout.Exit, x, y) {
		return uiHit{kind: hitExit}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateLegacyStatusScreen(hit uiHit) {
	switch hit.kind {
	case hitBack:
		s.closeSystemUI()
	case hitTitle:
		s.closeSystemUI()
		s.enterTitle()
	case hitExit:
		s.setHostRequest("exit")
	}
}

func (s *Session) legacyStatusItemState(item config.LegacyStatusItem) int {
	if item.Character < 1 || item.Character > len(legacyStatusCharacters) || item.Item < 1 {
		return 0
	}
	name := "#" + legacyStatusCharacters[item.Character-1] + "_item" + strconv.Itoa(item.Item)
	return s.sessionProperty(name)
}
