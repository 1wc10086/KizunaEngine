package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

// systemui_item.go 是 MK 的 SYSTEM → ITEM（持ち物）页。
//
// 原作这一页不是脚本画的：cfg_item_base 是一张 520x424 的面板，左侧烘着
// 五名女主角的头像和名字，右侧留白用来摆道具。道具图是 cfg_item 的 15 帧
// 64x64 竖排条（0..13 是 14 个道具，最后一帧是 X）。持有状态来自剧本维护的
// #item01..#item14 属性；未持有的格子不画图标，和原作一致。
//
// 这一页只负责展示与翻页，不直接使用道具：原作的训练道具是在剧情/训练
// 画面里消耗的，ITEM 页只是让玩家确认已经收集到哪些。

// openItemScreen 从存档/系统菜单切入 ITEM 页。
func (s *Session) openItemScreen() bool {
	if s.config == nil || s.config.Item.Empty() {
		return false
	}
	if s.ui == nil {
		// 从消息窗口的 ITEM 按钮/地图工具条进来时没有底层系统界面：
		// 先按普通系统界面打开（会抓一张剧情底图，原版 ITEM 页就是叠在
		// 当前画面上的）。
		if !s.canOpenSystemUI() {
			return false
		}
		s.beginSystemUI(uiDataItem)
	} else {
		if s.ui.kind == uiDataSave || s.ui.kind == uiDataLoad {
			s.ui.itemFromData = true
			s.ui.itemFromSave = s.ui.kind == uiDataSave
		}
		s.ui.kind = uiDataItem
	}
	s.ui.itemSlot = -1
	s.refreshItemBackdrop()
	if s.composer != nil {
		s.composer.Invalidate()
	}
	core.Logf("ui: item screen opened")
	return true
}

// drawItemScreen 画持ち物页：底层画面 + 角色/道具面板 + 模式页签 + 底部导航。
func (s *Session) drawItemScreen(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil || s.config == nil || canvas == nil {
		return
	}
	layout := s.config.Item
	// 原版把持ち物叠在当前剧情/地图画面上；drawSystemUI 先铺了黑，这里补回
	// 进入系统界面时抓到的底图。
	if ui.backdrop != nil {
		canvas.BlitCanvasRegion(ui.backdrop, 0, 0, ui.backdrop.Width, ui.backdrop.Height, 0, 0, 255)
	}
	if layout.MK2 {
		s.drawMK2ItemScreen(canvas, ui)
		return
	}
	s.drawImageAt(canvas, layout.Base, layout.BaseX, layout.BaseY)

	frames := max(1, layout.IconFrames)
	for index, slot := range layout.Slots {
		if index >= 14 {
			break
		}
		if s.sessionProperty(fmt.Sprintf("#item%02d", index+1)) == 0 {
			continue
		}
		icon := config.UIButton{
			Image: layout.Icon, Frames: frames,
			FixedFrame: true, Start: index, X: slot.X, Y: slot.Y,
		}
		s.drawFrameButton(canvas, icon, 0)
		if ui.itemSlot == index || (ui.hover.kind == hitItemSlot && ui.hover.index == index) {
			canvas.FillBlend(slot.X-3, slot.Y-3, 70, 2, 255, 255, 255, 220)
			canvas.FillBlend(slot.X-3, slot.Y+67, 70, 2, 255, 255, 255, 220)
			canvas.FillBlend(slot.X-3, slot.Y-3, 2, 70, 255, 255, 255, 220)
			canvas.FillBlend(slot.X+67, slot.Y-3, 2, 70, 255, 255, 255, 220)
		}
	}
	s.drawDataModeTabs(canvas, ui)
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitBack, layout.Back},
		{hitTitle, layout.Title},
		{hitExit, layout.Exit},
	} {
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
}

// drawDataModeTabs 画 SYSTEM 菜单的四个页签（SAVE/LOAD/CONFIG/ITEM）。
//
// 存档页和持ち物页共用这一排；当前页不额外高亮（cfg_title 只有四种文字，
// 没有选中态素材），玩家靠面板内容判断。
func (s *Session) drawDataModeTabs(canvas *gfx.BgraCanvas, ui *systemUI) {
	if s.config == nil || ui == nil {
		return
	}
	layout := s.config.DataScreen
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitSaveMode, layout.SaveMode},
		{hitLoadMode, layout.LoadMode},
		{hitDataConfig, layout.Config},
		{hitDataItem, layout.Item},
	} {
		if item.button.Empty() {
			continue
		}
		image := s.buttonImage(item.button)
		if image == nil {
			continue
		}
		height := image.Height / max(1, item.button.Frames)
		if height > 0 {
			canvas.FillBlend(item.button.X, item.button.Y, image.Width, height, 0, 0, 0, 190)
		}
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
}

// refreshItemBackdrop 重新抓一张"没有对话框"的场景底图。
//
// ITEM 页叠在场景上；从存档页切过来时 ui.backdrop 还是打开存档页前那一帧，
// 里面带着对话框和工具条，直接铺上去会把工具条浮在面板上方。
func (s *Session) refreshItemBackdrop() {
	if s.ui == nil || s.composer == nil || s.gameState == nil {
		return
	}
	s.composer.SetMessageWindowState(false, -1, false, "")
	s.composer.Compose(s.gameState)
	frame := s.composer.Frame()
	if frame == nil || frame.Width <= 0 || frame.Height <= 0 {
		return
	}
	s.ui.backdrop = gfx.NewBgraCanvas(frame.Width, frame.Height)
	s.ui.backdrop.CopyFrom(frame)
}

// drawMK2ItemScreen 画牝教師2 老版的 ITEM 页。
//
// cfg_item_base 的 4 行（早苗/沙耶/直美/梓）× 3 组空槽已经烘焙好；
// cfg_item 的三个部件是三个槽位的 12 帧图标条，帧号 = 角色行*3 + 组号。
// 每组的三个槽位由 #X_itemN 的位掩码控制：0x1 左上、0x2 左下、0x4 右侧。
// 布局坐标与 MK2IE 的 define.bsa:status_layout 完全一致。
func (s *Session) drawMK2ItemScreen(canvas *gfx.BgraCanvas, ui *systemUI) {
	s.drawLayerAt(canvas, "cfg_item_base", 0, 0)
	icons := s.composer.Load("cfg_item")
	if icons != nil && len(icons.Parts) >= 3 {
		characters := []string{"yuk", "tkm", "kzm", "skm"}
		for row, character := range characters {
			for group := 0; group < 3; group++ {
				value := s.sessionProperty("#" + character + "_item" + string(rune('1'+group)))
				if value == 0 {
					continue
				}
				frame := row*3 + group
				for slot := 0; slot < 3; slot++ {
					if value&(1<<slot) == 0 {
						continue
					}
					part := icons.Parts[slot]
					if part == nil || part.Height <= 0 {
						continue
					}
					cell := part.Height / 12
					if cell <= 0 || frame*cell+cell > part.Height {
						continue
					}
					x := 292 + group*168
					y := 116 + row*96
					if slot == 1 {
						y = 160 + row*96
					}
					if slot == 2 {
						x = 360 + group*168
						y = 128 + row*96
					}
					canvas.DrawRegion(part, 0, frame*cell, part.Width, cell, x, y)
				}
			}
		}
	}
	s.drawDataModeTabs(canvas, ui)
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitBack, s.config.DataScreen.Back},
		{hitTitle, s.config.DataScreen.Title},
		{hitExit, s.config.DataScreen.Exit},
	} {
		frame := 0
		if ui.hover.kind == item.kind {
			frame = 1
		}
		s.drawFrameButton(canvas, item.button, frame)
	}
}

// hitItemScreen 处理持ち物页的页签、道具格和导航键。
func (s *Session) hitItemScreen(x, y int) uiHit {
	ui := s.ui
	if ui == nil || s.config == nil {
		return uiHit{kind: hitNone, index: -1}
	}
	layout := s.config.DataScreen
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitSaveMode, layout.SaveMode},
		{hitLoadMode, layout.LoadMode},
		{hitDataConfig, layout.Config},
		{hitDataItem, layout.Item},
	} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	itemLayout := s.config.Item
	if itemLayout.MK2 {
		// MK2 的道具格只展示不可点；页签与 BACK/TITLE/QUIT 用存档页那套按钮。
		for _, item := range []struct {
			kind   uiHitKind
			button config.UIButton
		}{
			{hitBack, s.config.DataScreen.Back},
			{hitTitle, s.config.DataScreen.Title},
			{hitExit, s.config.DataScreen.Exit},
		} {
			if hitButton(s, item.button, x, y) {
				return uiHit{kind: item.kind}
			}
		}
		return uiHit{kind: hitNone, index: -1}
	}
	for _, item := range []struct {
		kind   uiHitKind
		button config.UIButton
	}{
		{hitBack, itemLayout.Back},
		{hitTitle, itemLayout.Title},
		{hitExit, itemLayout.Exit},
	} {
		if hitButton(s, item.button, x, y) {
			return uiHit{kind: item.kind}
		}
	}
	for index, slot := range itemLayout.Slots {
		if index >= 14 {
			break
		}
		if s.sessionProperty(fmt.Sprintf("#item%02d", index+1)) == 0 {
			continue
		}
		if x >= slot.X && y >= slot.Y && x < slot.X+64 && y < slot.Y+64 {
			return uiHit{kind: hitItemSlot, index: index}
		}
	}
	return uiHit{kind: hitNone, index: -1}
}

// activateItemScreen 处理页签切换、返回标题/退出和道具选中。
func (s *Session) activateItemScreen(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	switch hit.kind {
	case hitSaveMode:
		ui.kind = uiDataSave
		ui.deleteMode = false
	case hitLoadMode:
		ui.kind = uiDataLoad
		ui.deleteMode = false
	case hitDataConfig:
		if !s.openConfigScreen() {
			s.setHostRequest("settings")
		}
		return
	case hitDataItem:
		return
	case hitBack:
		// 从存档页切进来的：BACK 回存档页（原来在 SAVE 就回 SAVE）。
		if ui.itemFromData {
			save := ui.itemFromSave
			ui.itemFromData = false
			if save {
				ui.kind = uiDataSave
			} else {
				ui.kind = uiDataLoad
			}
			if s.composer != nil {
				s.composer.Invalidate()
			}
			return
		}
		s.closeSystemUI()
		return
	case hitTitle:
		s.closeSystemUI()
		s.enterTitle()
		return
	case hitExit:
		s.setHostRequest("exit")
		s.closeSystemUI()
		return
	case hitItemSlot:
		ui.itemSlot = hit.index
	}
	if s.composer != nil {
		s.composer.Invalidate()
	}
}
