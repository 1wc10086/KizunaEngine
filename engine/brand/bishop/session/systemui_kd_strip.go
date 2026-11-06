package session

import (
	"fmt"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/gfx"
)

// systemui_kd_strip.go 是 KD 的「立ち絵鑑賞」页。
//
// 这一代作品没有 museum_buex_layout，也没有旧式 [bu viewer] 的 bu* 素材；
// 它只有 sd_nodoka / sd_hikaru / sd_shizuru / sd_kotono / sd_yukie 五张
// 150x400 的整身立绘（雪絵略宽）。原引擎的鑑賞室用这五张图做角色选择，
// 这里沿用同一批素材：左侧五个 cfg_chN 头像切换角色，中间显示选中立绘，
// 右下角「戻る」回鉴赏主菜单。

var kdStripCharacters = []string{"nodoka", "hikaru", "shizuru", "kotono", "yukie"}

// openKDStripMuseum 在通用 buex/legacy 都不可用时打开 KD 的立绘页。
func (s *Session) openKDStripMuseum() bool {
	if s.ui == nil || s.composer == nil || !s.resourceExists("sd_"+kdStripCharacters[0]) {
		return false
	}
	s.ui.kind = uiMuseumBU
	s.ui.buex = nil
	s.ui.kdStrip = true
	s.ui.kdStripIndex = 0
	s.ui.hover = uiHit{kind: hitNone, index: -1}
	s.composer.Invalidate()
	return true
}

func (s *Session) kdStripRect(index int) (x, y, w, h int) {
	return 10, 96 + index*78, 64, 64
}

func (s *Session) drawKDStripMuseum(canvas *gfx.BgraCanvas) {
	ui := s.ui
	if ui == nil {
		return
	}
	s.drawImageAt(canvas, s.config.Museum.Base, 0, 0)
	index := clampInt(ui.kdStripIndex, 0, len(kdStripCharacters)-1)
	image := s.composer.Load("sd_" + kdStripCharacters[index])
	if image != nil {
		// sd_* 是 2 帧竖排图（地图上用来切待机/动作）；鉴赏只画第 0 帧，
		// 整张画会同时出现两个人物。放大 2 倍方便看清，仍保持原比例。
		// sd_* 图头写的是单帧（FrameCount=1），但素材实际是上下两张
		// （地图用 Height/2 切待机/动作）；鉴赏页也按 2 帧切。
		frames := 2
		frameHeight := image.Height / frames
		if frameHeight <= 0 {
			frameHeight = image.Height
		}
		offset := 0
		if offset+image.Width*frameHeight*4 > len(image.Pixels) {
			frameHeight = len(image.Pixels) / max(1, image.Width*4)
		}
		pixels := image.Pixels[offset : offset+image.Width*frameHeight*4]
		frame := gfx.NewBsgImage(image.Width, frameHeight, 0, 0, pixels)
		width := image.Width * 2
		height := frameHeight * 2
		x := (canvas.Width - width) / 2
		y := max(40, (canvas.Height-height)/2-20)
		canvas.BlitScaled(frame, x, y, width, height, 255)
	}
	for i := range kdStripCharacters {
		x, y, _, _ := s.kdStripRect(i)
		// cfg_chN 第 1 帧是“语音关闭”的灰头像+红叉，不能拿来当选中态；
		// 当前角色用外框表示，头像本身始终画彩色的第 0 帧。
		s.drawFrameButton(canvas, config.UIButton{
			Image: fmt.Sprintf("cfg_ch%d", i+1), Frames: 2, X: x, Y: y,
		}, 0)
		if i == index {
			canvas.Fill(x-2, y-2, 68, 2, 0, 200, 255, 255)
			canvas.Fill(x-2, y+64, 68, 2, 0, 200, 255, 255)
			canvas.Fill(x-2, y-2, 2, 68, 0, 200, 255, 255)
			canvas.Fill(x+64, y-2, 2, 68, 0, 200, 255, 255)
		}
	}
	// 复用 museum_menu 的第 5 个部件（戻る）。
	s.drawFrameButton(canvas, s.config.Museum.Return, 0)
}

func (s *Session) hitKDStrip(x, y int) uiHit {
	for i := range kdStripCharacters {
		bx, by, bw, bh := s.kdStripRect(i)
		if x >= bx && y >= by && x < bx+bw && y < by+bh {
			return uiHit{kind: hitBUCharacter, index: i}
		}
	}
	if hitButton(s, s.config.Museum.Return, x, y) {
		return uiHit{kind: hitBUClose}
	}
	return uiHit{kind: hitNone, index: -1}
}

func (s *Session) activateKDStrip(hit uiHit) {
	ui := s.ui
	if ui == nil {
		return
	}
	switch hit.kind {
	case hitBUCharacter:
		ui.kdStripIndex = clampInt(hit.index, 0, len(kdStripCharacters)-1)
	case hitBUClose:
		// 回到鉴赏主菜单，而不是关掉整个播放页。
		ui.kind = uiMuseum
		ui.kdStrip = false
		ui.hover = uiHit{kind: hitNone, index: -1}
	}
	s.composer.Invalidate()
}
