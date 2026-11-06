package session

import (
	"strconv"

	"kizuna/engine/brand/bishop/gfx"
)

// systemui_status_ancient.go 实现特別病棟（Tbxdl 世代）的内置“状態”画面。
//
// 这一代没有 status_layout 资源：status_base 800x600 已经把四位角色的资料烘焙在整屏里，
// 引擎只在每个面板下方叠 status_comment1..4（“現在の状況について”文本图）。
// 面板热区是按 800x600 的 2x2 象限近似的，真机对位后需要校正。

const (
	ancientStatusPanels   = 4
	ancientStatusComments = 4
	ancientStatusCommentW = 340
	ancientStatusCommentH = 250
)

// ancientStatusRects 是四个面板的 {x, y, w, h}，顺序为左上、右上、左下、右下。
var ancientStatusRects = [ancientStatusPanels][4]int{
	{0, 0, 400, 300},
	{400, 0, 400, 300},
	{0, 300, 400, 300},
	{400, 300, 400, 300},
}

func (s *Session) openAncientStatusScreen() bool {
	// 内置画面不依赖 layout 资源，因此总是能打开；底图缺失时只是画黑底，不会把按钮变成无反应。
	s.beginSystemUI(uiStatusAncient)
	if s.ui != nil {
		s.ui.ancientStatusSelected = -1
	}
	return true
}

// ancientStatusComment 返回第 panel 个面板当前显示的 status_commentN 名字。
// 初始时面板 i 显示第 i+1 张评论，点击后在四张评论之间循环，所以 step 每次加 1 取模。
func ancientStatusComment(panel, step int) string {
	return "status_comment" + strconv.Itoa((panel+step)%ancientStatusComments+1)
}

func (s *Session) drawAncientStatusScreen(canvas *gfx.BgraCanvas) {
	if canvas == nil || s.ui == nil {
		return
	}
	base := "status_base"
	if s.config != nil && s.config.Status.Base != "" {
		base = s.config.Status.Base
	}
	s.drawLayerAt(canvas, base, 0, 0)
	// 默认只显示整屏 status_base（资料已经烘焙在底图里）。点击某个面板后
	// 才把该角色的 status_commentN 以居中弹窗展开：评论图 340x250，
	// 直接铺在面板底部会盖住资料，和原版的“先选人再看评论”不符。
	panel := s.ui.ancientStatusSelected
	if panel < 0 || panel >= ancientStatusPanels {
		return
	}
	x := (canvas.Width - ancientStatusCommentW) / 2
	y := (canvas.Height - ancientStatusCommentH) / 2
	canvas.FillBlend(x-6, y-6, ancientStatusCommentW+12, ancientStatusCommentH+12, 0, 0, 0, 220)
	s.drawLayerAt(canvas, ancientStatusComment(panel, 0), x, y)
}

func (s *Session) hitAncientStatusScreen(x, y int) uiHit {
	for index, rect := range ancientStatusRects {
		if x >= rect[0] && y >= rect[1] && x < rect[0]+rect[2] && y < rect[1]+rect[3] {
			return uiHit{kind: hitStatusButton, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateAncientStatusScreen 点击面板后切换该面板的评论。
// ESC 与右键由通用的 closeSystemUI 路径处理，这里不再重复实现返回。
func (s *Session) activateAncientStatusScreen(hit uiHit) {
	if s.ui == nil || hit.kind != hitStatusButton || hit.index < 0 || hit.index >= ancientStatusPanels {
		return
	}
	if s.ui.ancientStatusSelected == hit.index {
		s.ui.ancientStatusSelected = -1
	} else {
		s.ui.ancientStatusSelected = hit.index
	}
	s.composer.Invalidate()
}
