package bdt

import "kizuna/engine/brand/bishop/gfx"

// bdt_manin_map.go 还原 MAP 移动画面。
//
// 原作不是 AFDVD 的 system.prm 地图：脚本先 CFS_MAP1 铺底，再 MAP 停住，
// 玩家点 s_map3 上的 8 个彩色站点之一，引擎把站点号写进 X31，脚本随后
// 用 J31,X,... 分派到 110-01..117-01。地图下方的 SAVE/LOAD/SYSTEM 是
// 三个固定按钮，和消息框工具条共用同一套存读档/设定界面。

type maninStationHit struct {
	id int
	x  int
	y  int
	w  int
	h  int
}

// s_map3 的 8 个纯色站点。位置表来自 mden.exe 的 0x48b560（8 对坐标，
// 与 s_map3 色块中心一一对应）；顺序就是脚本 110-01..117-01 的站点号：
// 1 鎌多、2 新重、3 樟南、4 聖城、5 桜木町、6 新横山、7 一ヶ谷、8 町屋。
var maninStationHits = []maninStationHit{
	{1, 404, 136, 32, 32}, // 红
	{2, 302, 186, 44, 44}, // 黄
	{3, 314, 94, 30, 30},  // 绿
	{4, 296, 280, 48, 48}, // 青
	{5, 456, 215, 40, 40}, // 蓝
	{6, 528, 168, 48, 48}, // 品红
	{7, 116, 212, 48, 48}, // 暗红
	{8, 248, 396, 48, 48}, // 暗黄
}

func maninStationHitByID(id int) (maninStationHit, bool) {
	for _, hit := range maninStationHits {
		if hit.id == id {
			return hit, true
		}
	}
	return maninStationHit{}, false
}

// maninStationIDFromColor 把 s_map3 的纯色块映射回站点号。
func maninStationIDFromColor(blue, green, red byte) int {
	switch {
	case red == 255 && green == 0 && blue == 0:
		return 1
	case red == 255 && green == 255 && blue == 0:
		return 2
	case red == 0 && green == 255 && blue == 0:
		return 3
	case red == 0 && green == 255 && blue == 255:
		return 4
	case red == 0 && green == 0 && blue == 255:
		return 5
	case red == 255 && green == 0 && blue == 255:
		return 6
	case red == 131 && green == 0 && blue == 0:
		return 7
	case red == 131 && green == 131 && blue == 0:
		return 8
	}
	return 0
}

// maninMapStationID 先读 s_map3 的像素，再退回连通块矩形。用像素判定
// 是原作的做法，能完全消除「站点序号猜错」的问题。
func (s *Session) maninMapStationID(x, y int) int {
	if s.manin == nil {
		return 0
	}
	image := s.maninImage("s_map3")
	if image != nil && x >= 0 && y >= 0 && x < image.Width && y < image.Height {
		offset := y*image.Stride() + x*4
		if offset+3 < len(image.Pixels) {
			if id := maninStationIDFromColor(image.Pixels[offset], image.Pixels[offset+1], image.Pixels[offset+2]); id > 0 {
				return id
			}
		}
	}
	for _, hit := range maninStationHits {
		if x >= hit.x && x < hit.x+hit.w && y >= hit.y && y < hit.y+hit.h {
			return hit.id
		}
	}
	return 0
}

func (s *Session) drawManinMap(canvas *gfx.BgraCanvas) {
	u := s.manin
	canvas.Clear(0, 0, 0, 255)
	base := "s_map1"
	if u.hover >= 0 && u.hover < len(maninStationHits) {
		if s.maninImage("s_map2") != nil {
			base = "s_map2"
		}
	}
	s.maninDrawImage(canvas, base, 0, 0)
	s.drawManinMapFaces(canvas)
	if u.hover >= 0 && u.hover < len(maninStationHits) {
		hit := maninStationHits[u.hover]
		canvas.FillBlend(hit.x-3, hit.y-3, hit.w+6, 3, 255, 255, 255, 200)
		canvas.FillBlend(hit.x-3, hit.y+hit.h, hit.w+6, 3, 255, 255, 255, 200)
		canvas.FillBlend(hit.x-3, hit.y, 3, hit.h, 255, 255, 255, 200)
		canvas.FillBlend(hit.x+hit.w, hit.y, 3, hit.h, 255, 255, 255, 200)
		// 悬停时显示站名（ms1..ms8），对应原作地图上的车站标签。
		if name := s.maninImage("ms" + string(rune('1'+u.hover))); name != nil {
			canvas.Draw(name, hit.x+hit.w/2-name.Width/2, hit.y-58)
		}
	}
	if u.caption != "" {
		s.maninDrawText(canvas, u.caption, 12, 300, 450, 320, 1, 0xffffffff)
	}
}

// drawManinMapFaces 按 ctbl.rbd 的进行值/时间带把 mfN 表情贴到对应车站。
// 同一站有多人时横向错开 8px，避免完全重叠。
func (s *Session) drawManinMapFaces(canvas *gfx.BgraCanvas) {
	if s.machine == nil {
		return
	}
	x := s.machine.X()
	if len(x) < 2 {
		return
	}
	slot := int(x[1]) - 2
	if slot < 0 || slot > 5 {
		return
	}
	used := map[int]int{}
	for index := 0; index < 6; index++ {
		station := s.maninCharacterStation(index, slot)
		if station == 0 {
			continue
		}
		hit, ok := maninStationHitByID(station)
		if !ok {
			continue
		}
		image := s.maninImage("mf" + string(rune('1'+index)))
		if image == nil {
			continue
		}
		offset := used[station] * 8
		used[station]++
		faceX := hit.x + hit.w/2 - 24 + offset
		faceY := hit.y - 44
		if faceX < 0 {
			faceX = 0
		}
		if faceY < 0 {
			faceY = 0
		}
		canvas.Draw(image, faceX, faceY)
	}
}

func (s *Session) pointerManinMap(x, y int) bool {
	u := s.manin
	switch {
	case x >= 0 && x < 147 && y >= 368 && y < 396:
		s.maninEnterScreen(maninScreenSave)
		return true
	case x >= 0 && x < 147 && y >= 400 && y < 428:
		s.maninEnterScreen(maninScreenLoad)
		return true
	case x >= 0 && x < 147 && y >= 432 && y < 460:
		s.maninEnterScreen(maninScreenSystem)
		return true
	}
	id := s.maninMapStationID(x, y)
	if id == 0 || s.machine == nil {
		return true
	}
	s.machine.SetXValue(31, int32(id))
	// MAP 之后脚本先看 Z31=1 来收掉地图画面（CB/CD9），再用 X31 分派车站；
	// 两个寄存器都要写，否则会停在黑屏/地图上。
	s.machine.SetZValue(31, 1)
	s.machine.CloseMap()
	u.station = id
	u.screen = maninScreenNone
	u.prevScreen = maninScreenNone
	u.hover = 0
	s.gameState.EndSelect()
	s.composerInvalidate()
	s.runSteps(defaultRunBudget)
	return true
}
