package session

import (
	"kizuna/engine/brand/bishop/gfx"
)

// systemui_cgviewer_config.go 处理 KD CG 查看器底栏的 CFG 按钮。
//
// 为什么需要快照：设置画面和 CG 查看器共用 Session.ui 这一块状态，打开设置
// 会销毁查看器。原作里 CFG 是"从查看器进设置、返回后还在原差分"，如果直接
// closeSystemUI 会退回标题（查看器是从标题进的），玩家得重新进鉴赏室。
// 这里在打开设置前保存查看器的分组/差分/CG 画布，设置关闭时恢复。

type cgViewerResume struct {
	group, entry, variant int
	canvas                *gfx.BgraCanvas
	bgVolumeShown         bool
	panelShown            bool
}

// openCGViewerConfig 保存查看器状态并打开设置画面。
func (s *Session) openCGViewerConfig() {
	ui := s.ui
	if ui == nil || ui.kind != uiCGViewer {
		return
	}
	resume := &cgViewerResume{
		group:         ui.cgGroup,
		entry:         ui.cgEntry,
		variant:       ui.cgVariant,
		canvas:        ui.cgCanvas,
		bgVolumeShown: ui.cgBGVolumeShown,
		panelShown:    ui.cgPanelShown,
	}
	s.cgResume = resume
	// openConfigScreen 会优先用扩展布局，没有时自动退回旧式 [config screen]；
	// KD 是后者。两种都打不开时保持查看器不动，不能留下一个永远恢复不了的快照。
	if s.openConfigScreen() {
		return
	}
	s.cgResume = nil
}

// resumeCGViewerIfNeeded 在设置画面关闭后恢复 CG 查看器。
//
// 只由"关闭设置"的路径调用（戻る/右键）。选择タイトル或終了时必须先清掉
// cgResume，否则会在标题画面上又叠一层查看器。
func (s *Session) resumeCGViewerIfNeeded() {
	resume := s.cgResume
	if resume == nil {
		return
	}
	s.cgResume = nil
	if s.composer == nil || resume.canvas == nil {
		return
	}
	s.beginSystemUI(uiCGViewer)
	ui := s.ui
	if ui == nil {
		return
	}
	ui.cgGroup = resume.group
	ui.cgEntry = resume.entry
	ui.cgVariant = resume.variant
	ui.cgCanvas = resume.canvas
	ui.cgBGVolumeShown = resume.bgVolumeShown
	ui.cgPanelShown = resume.panelShown
	s.composer.Invalidate()
}
