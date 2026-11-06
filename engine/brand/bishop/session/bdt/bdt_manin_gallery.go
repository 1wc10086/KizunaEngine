package bdt

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core/textcodec"
)

// bdt_manin_gallery.go 还原「鑑賞室」三层：
//   1. s_omk1/2/3 的入口菜单（音楽/CG/シーン/戻る）；
//   2. s_mu01 的音乐 20 行与 s_mu04 命中；
//   3. s_cgNNb / s_scNNb 的 6x4 缩略图页与 s_cgNNh / s_scNNh 命中。
// 缩略图本身不烘焙在底图里（底图只有黑色格子），所以必须由引擎按 gtbl
// 的名字从 cg/*.zbm 缩放绘制；未解锁的格子保持黑色。

func maninMuseumHitIndex(x, y int) int {
	switch {
	case x >= 168 && x < 472 && y >= 126 && y < 190:
		return 0
	case x >= 168 && x < 472 && y >= 208 && y < 272:
		return 1
	case x >= 168 && x < 472 && y >= 292 && y < 356:
		return 2
	case x >= 228 && x < 412 && y >= 376 && y < 440:
		return 3
	}
	return -1
}

func maninMusicRowAt(x, y int) int {
	if y >= 96 && y < 96+10*32 {
		row := (y - 96) / 32
		switch {
		case x >= 32 && x < 312:
			return row
		case x >= 328 && x < 608:
			return row + 10
		}
	}
	if x >= 272 && x < 364 && y >= 422 && y < 454 {
		return 20
	}
	return -1
}

func maninGalleryCellAt(x, y int) int {
	for row := 0; row < 4; row++ {
		for col := 0; col < 6; col++ {
			cellX := 30 + col*100
			cellY := 88 + row*80
			if x >= cellX && x < cellX+80 && y >= cellY && y < cellY+60 {
				return row*6 + col
			}
		}
	}
	return -1
}

func maninHchCellAt(x, y int) int {
	columns := []int{312, 448}
	rows := []int{132, 224, 320}
	for row, cellY := range rows {
		for col, cellX := range columns {
			if x >= cellX && x < cellX+120 && y >= cellY && y < cellY+80 {
				return row*2 + col
			}
		}
	}
	return -1
}

func maninVulgarHitIndex(x, y int) int {
	if y >= 400 && y < 464 {
		switch {
		case x >= 0 && x < 424:
			return 6
		case x >= 424:
			return 7
		}
	}
	if y >= 0 && y < 400 {
		switch {
		case x >= 0 && x < 106:
			return 0
		case x >= 106 && x < 212:
			return 1
		case x >= 212 && x < 318:
			return 2
		case x >= 318 && x < 424:
			return 3
		case x >= 424 && x < 530:
			return 4
		case x >= 530:
			return 5
		}
	}
	return -1
}

// maninCgNames 解析 gtbl.rbd（index,name,char），返回图片名顺序。
// 文件同样 0xFF 异或；解析失败返回空，鉴赏室会退化成黑格而不是崩溃。
func (s *Session) maninCgNames() []string {
	if s.manin == nil {
		return nil
	}
	if len(s.manin.cgNames) > 0 {
		return s.manin.cgNames
	}
	if s.resources == nil {
		return nil
	}
	location, ok := s.resources.TryGet("gtbl")
	if !ok {
		location, ok = s.resources.TryGet("gtbl.rbd")
	}
	if !ok || location == nil {
		return nil
	}
	data := location.Read()
	if len(data) == 0 {
		return nil
	}
	decrypted := make([]byte, len(data))
	for i, value := range data {
		decrypted[i] = ^value
	}
	text := textcodec.DecodeCP932(decrypted)
	// gtbl 第三列是组号：g101/g101p1/g101p2 同组。原作鉴赏室按组显示
	// 一张缩略图，104 组正好 5 页 ×24 格。
	order := make([]int, 0, 128)
	groups := map[int][]string{}
	for lineIndex, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		group := lineIndex + 1
		if len(parts) >= 3 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil && parsed > 0 {
				group = parsed
			}
		}
		if _, seen := groups[group]; !seen {
			order = append(order, group)
		}
		groups[group] = append(groups[group], name)
	}
	s.manin.cgVariants = map[string][]string{}
	for _, group := range order {
		names := groups[group]
		if len(names) == 0 {
			continue
		}
		representative := names[0]
		s.manin.cgNames = append(s.manin.cgNames, representative)
		s.manin.cgVariants[representative] = names
	}
	return s.manin.cgNames
}

// maninCgUnlocked 按组判定：看过任意一个变体就算这组已解锁。
func (s *Session) maninCgUnlocked(name string) bool {
	if s.manin == nil {
		return false
	}
	if variants, ok := s.manin.cgVariants[name]; ok {
		for _, variant := range variants {
			if s.manin.cgUnlocked(variant) {
				return true
			}
		}
		return false
	}
	return s.manin.cgUnlocked(name)
}

// maninSceneNames 原作シーン鑑賞是 2 页 ×24，共 48 个事件；这里取 CG 组
// 前 48 个作为可回放的事件缩略图（gtbl 没有独立 scene 表）。
func (s *Session) maninSceneNames() []string {
	names := s.maninCgNames()
	if len(names) > 48 {
		return names[:48]
	}
	return names
}

func (s *Session) drawManinGallery(canvas *gfx.BgraCanvas) {
	u := s.manin
	if u.gallery.Preview != "" {
		canvas.Clear(0, 0, 0, 255)
		if !s.maninDrawImage(canvas, u.gallery.Preview, 0, 0) {
			s.maninDrawText(canvas, u.gallery.Preview, 18, 40, 40, 560, 2, 0xffcccccc)
		}
		s.maninDrawText(canvas, "クリックで戻る", 12, 480, 452, 150, 1, 0xffffffff)
		return
	}
	switch u.screen {
	case maninScreenMuseum:
		canvas.Clear(0, 0, 0, 255)
		if u.hover >= 0 && u.hover < 3 && s.maninDrawImage(canvas, "s_omk2", 0, 0) {
			return
		}
		s.maninDrawImage(canvas, "s_omk1", 0, 0)
	case maninScreenMusic:
		canvas.Clear(0, 0, 0, 255)
		s.maninDrawImage(canvas, "s_mu01", 0, 0)
		if u.hover >= 0 && u.hover < 20 {
			row := u.hover
			x := 32
			if row >= 10 {
				x = 328
			}
			canvas.FillBlend(x, 96+(row%10)*32, 280, 28, 20, 20, 120, 90)
		}
	case maninScreenCG:
		s.drawManinGalleryGrid(canvas, false)
	case maninScreenScene:
		s.drawManinGalleryGrid(canvas, true)
	case maninScreenHch:
		s.drawManinHchGrid(canvas)
	case maninScreenVulgar:
		canvas.Clear(0, 0, 0, 255)
		if u.hover >= 0 && u.hover < 6 && s.maninDrawImage(canvas, "s_vlgn", 0, 0) {
			return
		}
		s.maninDrawImage(canvas, "s_vlgb", 0, 0)
		for index := 0; index < 6; index++ {
			if !u.global.VulgarGirls[index] {
				canvas.FillBlend(index*106, 0, 106, 400, 0, 0, 0, 140)
			}
		}
	}
}

func (s *Session) drawManinGalleryGrid(canvas *gfx.BgraCanvas, scene bool) {
	u := s.manin
	prefix := "s_cg"
	if scene {
		prefix = "s_sc"
	}
	page := u.gallery.Page
	if page < 0 {
		page = 0
	}
	imageName := prefix + twoDigits(page) + "b"
	canvas.Clear(0, 0, 0, 255)
	if !s.maninDrawImage(canvas, imageName, 0, 0) {
		s.maninDrawImage(canvas, prefix+"00b", 0, 0)
	}
	names := s.maninCgNames()
	if scene {
		names = s.maninSceneNames()
	}
	totalPages := (len(names) + 23) / 24
	if totalPages < 1 {
		totalPages = 1
	}
	if page >= totalPages {
		page = totalPages - 1
		u.gallery.Page = page
	}
	title := "CG鑑賞"
	if scene {
		title = "シーン鑑賞"
	}
	// 底图的页数文字是烘焙的（1/5），与动态总页数可能不一致；用一条
	// 半透明条盖住右上角再写真实页数，避免玩家看到错误进度。
	canvas.FillBlend(500, 22, 120, 40, 20, 20, 40, 220)
	s.maninDrawText(canvas, title, 14, 24, 26, 240, 1, 0xffffffff)
	s.maninDrawText(canvas, strconv.Itoa(page+1)+"/"+strconv.Itoa(totalPages), 14, 540, 30, 80, 1, 0xffffffff)
	for cell := 0; cell < 24; cell++ {
		x := 30 + (cell%6)*100
		y := 88 + (cell/6)*80
		index := page*24 + cell
		if index >= len(names) {
			continue
		}
		name := names[index]
		if !s.maninCgUnlocked(name) {
			continue
		}
		image := s.maninImage(name)
		if image == nil {
			continue
		}
		canvas.BlitScaled(image, x, y, 80, 60, 255)
		if u.hover == cell {
			canvas.FillBlend(x, y, 80, 3, 255, 255, 255, 160)
			canvas.FillBlend(x, y+57, 80, 3, 255, 255, 255, 160)
		}
	}
}

func (s *Session) drawManinHchGrid(canvas *gfx.BgraCanvas) {
	u := s.manin
	canvas.Clear(0, 0, 0, 255)
	page := u.gallery.Page
	if page < 0 {
		page = 0
	}
	if page > 6 {
		page = 6
	}
	name := "s_hch" + twoDigits(page) + "b"
	if !s.maninDrawImage(canvas, name, 0, 0) {
		s.maninDrawImage(canvas, "s_hch00b", 0, 0)
	}
	names := s.maninCgNames()
	for cell := 0; cell < 6; cell++ {
		x := 312
		if cell%2 == 1 {
			x = 448
		}
		y := 132 + (cell/2)*96
		index := page*6 + cell
		if index >= len(names) || !s.maninCgUnlocked(names[index]) {
			continue
		}
		image := s.maninImage(names[index])
		if image == nil {
			continue
		}
		canvas.BlitScaled(image, x, y, 120, 80, 255)
	}
}

func (s *Session) pointerManinGallery(x, y int) bool {
	u := s.manin
	if u.gallery.Preview != "" {
		u.gallery.Preview = ""
		s.composerInvalidate()
		return true
	}
	switch u.screen {
	case maninScreenMuseum:
		switch maninMuseumHitIndex(x, y) {
		case 0:
			s.maninEnterScreen(maninScreenMusic)
		case 1:
			s.maninEnterScreen(maninScreenCG)
		case 2:
			s.maninEnterScreen(maninScreenScene)
		case 3:
			s.maninExitScreen()
		}
	case maninScreenMusic:
		row := maninMusicRowAt(x, y)
		if row == 20 {
			s.maninExitScreen()
		} else if row >= 0 && row < 20 {
			u.gallery.Thumb = row
			s.SetBgm(row+2, 0)
		}
	case maninScreenCG, maninScreenScene:
		page := u.gallery.Page
		names := s.maninCgNames()
		if u.screen == maninScreenScene {
			names = s.maninSceneNames()
		}
		totalPages := (len(names) + 23) / 24
		if totalPages < 1 {
			totalPages = 1
		}
		if x >= 280 && x < 360 && y >= 404 && y < 442 {
			s.maninExitScreen()
			return true
		}
		if x >= 430 && x < 510 && y >= 404 && y < 442 {
			if page+1 < totalPages {
				u.gallery.Page = page + 1
				s.composerInvalidate()
			}
			return true
		}
		cell := maninGalleryCellAt(x, y)
		if cell >= 0 {
			index := page*24 + cell
			if index >= 0 && index < len(names) && s.maninCgUnlocked(names[index]) {
				u.gallery.Preview = names[index]
				s.composerInvalidate()
			}
		}
	case maninScreenHch:
		if x >= 369 && x < 512 && y >= 432 && y < 464 {
			s.maninExitScreen()
			return true
		}
		cell := maninHchCellAt(x, y)
		if cell >= 0 {
			names := s.maninCgNames()
			index := u.gallery.Page*6 + cell
			if index >= 0 && index < len(names) && s.maninCgUnlocked(names[index]) {
				u.gallery.Preview = names[index]
				s.composerInvalidate()
			}
		}
	case maninScreenVulgar:
		switch index := maninVulgarHitIndex(x, y); {
		case index >= 0 && index < 6:
			s.maninStartVulgarScene(index)
		case index == 6:
			// その他のシーン：优先播放第一位已解锁角色；没有独立 607 脚本。
			for i := 0; i < 6; i++ {
				if s.manin.global.VulgarGirls[i] {
					s.maninStartVulgarScene(i)
					break
				}
			}
		case index == 7:
			s.maninExitScreen()
		}
	}
	return true
}

// maninStartVulgarScene 从标题的卑語モード选择画面进入 60x-01。
// 这些场景没有 E 结束标记，右键/返回由 vulgarPlayback 负责回标题。
func (s *Session) maninStartVulgarScene(character int) {
	if s.manin == nil || s.machine == nil {
		return
	}
	if character < 0 || character >= 6 || !s.manin.global.VulgarGirls[character] {
		return
	}
	s.manin.vulgarPlayback = true
	s.maninCloseAll()
	s.leaveTitle()
	s.titleMode = false
	s.machine.Start("60" + string(rune('1'+character)) + "-01")
	// 60x 场景本身不带 CF，所有台词共用进 H 场景时的 CG。从标题进入时
	// 原画面已经丢失，这里用该角色第一张 H CG 作为底图（g101/g201/...）。
	s.ClearSprites()
	s.stage.SetFace("")
	s.SetBackground("g"+string(rune('1'+character))+"01", 0)
	s.gameState.SetMessageWindowVisible(true)
	s.runSteps(defaultRunBudget)
}
