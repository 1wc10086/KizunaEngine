// 原版存/读档（sq_save / sq_load）与确认框的引擎自绘实现。
//
// 为什么背景/按钮坐标直接取 menus.SaveLayoutFor / ConfirmLayoutFor：
// 坐标在 sq_*.nut 编译字节码的 ui.Register 实参里，Flutter 只送点击坐标，
// 若热区与画出来的图各算一套，按钮会错位；同取一张布局表可保证一致。
package session

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/overdose/menus"
	"kizuna/enginego/core"
)

// composeSaveOverlay 画存/读档：背景、10 bank、10 slot、缩略图、return。
// 按钮只用常态图（-o 是按下态，hover 状态由 host 侧指针事件驱动，这里不猜）。
func (s *Session) composeSaveOverlay(canvas *bishopgfx.BgraCanvas) {
	mode := "save"
	if s.saveMode == overlayLoad {
		mode = "load"
	}
	layout := menus.SaveLayoutFor(mode)

	s.drawImage(canvas, layout.Background, 0, 0)

	// bank：当前页用 -o 表示选中（两态图成对出现，-o 即选中态）。
	for i, r := range layout.BankRects {
		img := menus.SaveBankImage(layout.BankPrefix, i+1)
		s.drawImage(canvas, img, int(r.X), int(r.Y))
		if i == s.saveBank {
			s.drawImage(canvas, img+"-o", int(r.X), int(r.Y))
		}
	}

	// slot：253x84 常态图 + 槽内 +8,+8 起的日期/标题/摘要。
	for i, r := range layout.SlotRects {
		img := menus.SaveSlotImage(layout.SlotPrefix, i+1)
		s.drawImage(canvas, img, int(r.X), int(r.Y))
		info := s.slotInfo(slotIndex(s.saveBank, i))
		if !info.Exists {
			continue
		}
		x := int(r.X) + 8
		y := int(r.Y) + 8
		// 槽位底图是“三行横线的便签纸”（save_slot01 实测），文字必须用
		// 原版深色：save 0x304020、load 0x803040（Windows COLORREF 的
		// 0x00BBGGRR 记法，转成 textrender 的 0xAARRGGBB 就是下面两值）。
		// 画白字会在白纸上完全看不见。
		textColor := uint32(0xFF203040)
		if s.saveMode == overlayLoad {
			textColor = uint32(0xFF403080)
		}
		// 有档才画文字：空槽只留按钮底图，避免无档槽读到别的 bank 的日期。
		if info.SavedAt != "" {
			s.drawText(canvas, info.SavedAt, x, y-5, 14, 14, int(r.W)-16, 1, textColor)
		}
		// 标题一行 14 号，摘要多行 12 号；摘要列宽按水平边距截断即可。
		if info.Title != "" {
			s.drawText(canvas, info.Title, x, y+16, 14, 14, int(r.W)-16, 1, textColor)
		}
		if info.Summary != "" {
			s.drawText(canvas, info.Summary, x, y+37, 12, 12, int(r.W)-16, 2, textColor)
		}
	}

	// 缩略图：取当前 bank 第一个有档槽位（原版只预览这一张大图）。
	// 槽位无档或预览失败不回退其他 bank，避免画面与 bank 页签脱节。
	for i := 0; i < layout.Slots; i++ {
		idx := slotIndex(s.saveBank, i)
		info := s.slotInfo(idx)
		if !info.Exists {
			continue
		}
		prev, ok := s.PreviewSlot(uint32(idx))
		if !ok {
			break
		}
		tr := layout.ThumbRect
		if prev.Thumb.Width > 0 && len(prev.Thumb.Pixels) >= prev.Thumb.Width*prev.Thumb.Height*4 {
			img := bishopgfx.NewBsgImage(prev.Thumb.Width, prev.Thumb.Height, 0, 0, prev.Thumb.Pixels)
			canvas.BlitScaled(img, int(tr.X), int(tr.Y), int(tr.W), int(tr.H), 255)
		}
		break
	}

	// return 按钮（load 无独立图，复用 save_return，layout 已统一）。
	s.drawImage(canvas, layout.Return, int(layout.ReturnRect.X), int(layout.ReturnRect.Y))
}

// pointerSaveOverlay 命中顺序：bank 页签 → slot → return → 点空关闭。
// return/点空关闭用 hit=true：不能退化成“点画面推进”。
func (s *Session) pointerSaveOverlay(x, y int32) (core.Step, bool) {
	mode := "save"
	if s.saveMode == overlayLoad {
		mode = "load"
	}
	layout := menus.SaveLayoutFor(mode)

	// bank 切换：同一点击不穿透去激活 slot。
	for i, r := range layout.BankRects {
		if hit(r, x, y) {
			s.saveBank = i
			return s.step, true
		}
	}

	// slot：save 有档走 overwrite 确认、无档直接写；load 有档走 load 确认。
	for i, r := range layout.SlotRects {
		if !hit(r, x, y) {
			continue
		}
		idx := slotIndex(s.saveBank, i)
		info := s.slotInfo(idx)
		if s.saveMode == overlayLoad {
			if info.Exists {
				return s.openConfirm("load", idx, overlayLoad), true
			}
			// 读档空槽无动作，界面保持打开。
			return s.step, true
		}
		// save 模式。
		if info.Exists {
			return s.openConfirm("overwrite", idx, overlaySave), true
		}
		// 无档直接保存并保持界面（原版不关闭，让玩家继续连存）。
		_ = s.Save(uint32(idx), s.summarize())
		return s.step, true
	}

	// return 关闭界面；点到背景/槽间隙不做任何事（原版只在 return 或右键退出）。
	if hit(layout.ReturnRect, x, y) {
		return s.closeOverlay(), true
	}
	return s.step, true
}

// composeConfirmOverlay 画确认框：背景 + 本 kind 消息图 + yes/no。
// 不清屏：框叠在当前画面（剧情或存/读档）上，原底图必须保留。
func (s *Session) composeConfirmOverlay(canvas *bishopgfx.BgraCanvas) {
	layout := menus.ConfirmLayoutFor()

	s.drawImage(canvas, layout.Background, int(layout.BgRect.X), int(layout.BgRect.Y))
	// 消息图与背景同矩形：背景纯底色，文字靠 msg 图叠出。
	if msg := menus.ConfirmMsgFor(s.confirmKind); msg != "" {
		s.drawImage(canvas, msg, int(layout.MsgRect.X), int(layout.MsgRect.Y))
	}
	s.drawImage(canvas, layout.Yes, int(layout.YesRect.X), int(layout.YesRect.Y))
	s.drawImage(canvas, layout.No, int(layout.NoRect.X), int(layout.NoRect.Y))
}

// pointerConfirmOverlay 确认框命中：yes 按 kind 分派，no 回 confirmBack。
// 点框外不关层：确认框是模态的，误点不能直接回剧情。
func (s *Session) pointerConfirmOverlay(x, y int32) (core.Step, bool) {
	layout := menus.ConfirmLayoutFor()

	if hit(layout.YesRect, x, y) {
		switch s.confirmKind {
		case "save", "overwrite":
			_ = s.Save(uint32(s.confirmSlot), s.summarize())
			// closeOverlay(confirm) 回 confirmBack（存档界面），保持界面不关。
			return s.closeOverlay(), true
		case "load":
			if _, err := s.Load(uint32(s.confirmSlot)); err == nil {
				return s.closeOverlay(), true
			}
			// 读档失败（槽位损坏/删档）留在确认框，让玩家改选或按 no。
			return s.step, true
		case "quit":
			s.requestHost("exit")
			return s.closeOverlay(), true
		default:
			// 未知 kind 不执行任何动作，避免误存/误读。
			return s.closeOverlay(), true
		}
	}
	if hit(layout.NoRect, x, y) {
		return s.closeOverlay(), true
	}
	// 框外：右键等同 no 已由 rightClickAt 伪造 NoRect 坐标走这里；
	// 其余点击吞掉，不穿透剧情推进。
	return s.step, false
}
