package session

import "kizuna/engine/core"

// wheel.go 只负责把滚轮事件分派给当前界面。
//
// 滚轮事件按协议只带 delta，没有坐标；各界面自己知道"一次滚多少、滚哪一栏"。
// 把具体滚动逻辑放回各自的文件（systemui_bgv.go / systemui_dress.go /
// systemui_buex.go），是为了让布局、行高和滚动条几何只有一份来源——
// 以前 BGV 的滚动逻辑写在 wheel.go 里，行高和列宽却在 bgv_layout 里，
// 两边任何一边改动都会让"能滚但看起来没动"。
//
// 注意：滚轮要借助最后一次指针位置（Session.lastPointerX/Y）判断当前栏。
// 平台层如果只发滚轮不发 hover，会退化成"滚动最后一栏"；宿主在收到
// PointerScrollEvent 时会先补一次 move，见 player_chrome.dart。
//
// 触屏拖动不走这里：宿主按协议发 Pointer op 6（带坐标），由
// systemui.go 的 dragSystemUI 处理，两者共用各界面自己的行高/滚动逻辑。
func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.slgState != nil {
		if s.slgVM != nil {
			key := 0x3c // KEY_WU
			if delta > 0 {
				key = 0x3e // KEY_WD
			}
			if s.slgVM.Key(key) {
				return s.finishRun(s.finishSLGState()), nil
			}
		}
		s.composer.Invalidate()
		return s.lastStep, nil
	}
	if s.ui == nil || delta == 0 {
		return s.lastStep, nil
	}
	switch s.ui.kind {
	case uiCGViewer:
		s.wheelCGViewer(delta)
	case uiScenePlaylist:
		s.wheelScenePlaylist(delta)
	case uiMuseumCG:
		// 缩放只在 CG 查看器内有效，不污染缩略图页的滚轮。
	case uiMuseumBGV:
		s.wheelBGV(delta)
	case uiDress:
		if s.ui != nil && s.ui.mk2Dress != nil {
			s.wheelMK2Dress(delta)
		} else {
			s.wheelDress(delta)
		}
	case uiLog:
		s.wheelLog(delta)
	case uiStatus, uiStatusSLG:
		if s.ui.kind == uiStatusSLG {
			s.wheelSLGStatus(delta)
		} else if s.config != nil {
			s.wheelStatus(delta)
		}
	case uiMuseumBU:
		s.wheelBuex(delta)
	case uiMuseumStrip:
		if s.ui != nil && s.ui.mkStrip != nil {
			s.wheelMKStrip(delta)
		} else {
			s.wheelMK2Strip(delta)
		}
	}
	return s.lastStep, nil
}

func (s *Session) wheelStatus(delta int32) {
	if s.ui == nil || s.config == nil {
		return
	}
	if s.ui.chart != nil {
		s.wheelChart(delta)
		return
	}
	layout := s.statusLayout
	if layout == nil || len(layout.Pages) == 0 || layout.Pages[0].Achievement == nil {
		return
	}
	achievement := layout.Pages[0].Achievement
	rows := achievement.ScrollBar.MaxNo
	if rows <= 0 {
		rows = 21
	}
	total := 0
	if table := s.ensureAchievements(); table != nil {
		total = table.Len()
	}
	maxScroll := max(0, total-rows)
	s.statusScroll = clampInt(s.statusScroll+int(delta), 0, maxScroll)
	s.composer.Invalidate()
}

func (s *Session) wheelLog(delta int32) {
	if s.ui == nil || s.ui.log == nil || s.config == nil {
		return
	}
	lines := max(1, s.config.Log.Lines)
	maxScroll := max(0, len(s.ui.log.entries)-lines)
	s.ui.log.scroll = clampInt(s.ui.log.scroll+int(delta), 0, maxScroll)
	s.composer.Invalidate()
}

// wheelCGViewer 处理 CG 查看器上的滚轮缩放（MD2 手册：向上放大到 2 倍）。
//
// 滚轮方向（player_chrome.dart）：Flutter 的 scrollDelta.dy 向下为正，
// 向上滚动（手册的“滚轮向上=放大”）传到引擎是 delta=-1。方向键同理：
// ArrowUp 发 wheel(-1)。所以这里用 -delta 作为放大步进，向下滚（+1）
// 才会缩小。缩放后以画面中心为基准重算 pan 限幅，保证图像始终盖满画面。
func (s *Session) wheelCGViewer(delta int32) {
	ui := s.ui
	if ui == nil || delta == 0 {
		return
	}
	if ui.cgZoom == 0 {
		ui.cgZoom = 1
	}
	steps := -int(delta)
	// delta 的绝对值通常是 1（宿主归一化），直接用；若宿主发出多于 1
	// 的增量，按增量步进，但不超出 1..2 的范围。
	ui.cgZoom = clampFloat(ui.cgZoom+float64(steps)*cgZoomStep, 1, cgZoomMax)
	if ui.cgZoom == 1 {
		ui.cgPanX = 0
		ui.cgPanY = 0
	} else {
		// 缩放变化后立刻把 pan 钳到新限幅，防止缩小时 pan 越界留黑边。
		ui.cgPanX = clampInt(ui.cgPanX, -s.cgPanLimitX(), s.cgPanLimitX())
		ui.cgPanY = clampInt(ui.cgPanY, -s.cgPanLimitY(), s.cgPanLimitY())
	}
	s.composer.Invalidate()
}
