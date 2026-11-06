package bdt

import (
	"strconv"
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
)

// bdt_manin_ui.go 负责 manin 的通用界面骨架：派发、标题悬停、工具条、
// SYSTEM 設定、存读档、LOG、片尾。鉴赏室和言葉責め/地图分在各自文件，
// 这样每个文件只关心一组界面，不会长成一个巨型 switch。

func (s *Session) maninImage(name string) *gfx.BsgImage {
	if s.images == nil || name == "" {
		return nil
	}
	return s.images.Get(name)
}

func (s *Session) maninDrawImage(canvas *gfx.BgraCanvas, name string, x, y int) bool {
	image := s.maninImage(name)
	if image == nil {
		return false
	}
	canvas.Draw(image, x, y)
	return true
}

func (s *Session) maninDrawText(canvas *gfx.BgraCanvas, text string, size, x, y, maxWidth, maxLines int, color uint32) {
	if canvas == nil || strings.TrimSpace(text) == "" {
		return
	}
	font := sharedBDTFont()
	if font == nil {
		return
	}
	lineHeight := size + 4
	image := font.Render(text, size, 0, 0, lineHeight, maxWidth, maxLines, color)
	if image != nil {
		rendering.DrawTextImage(canvas, image, x, y)
	}
}

// drawManinUI 把当前 manin 界面画到帧上。调用方已经画好剧情舞台；这里
// 只负责覆盖层。
func (s *Session) drawManinUI(canvas *gfx.BgraCanvas) {
	if s.manin == nil || canvas == nil {
		return
	}
	switch s.manin.screen {
	case maninScreenSystem:
		s.drawManinSystem(canvas)
	case maninScreenSave, maninScreenLoad:
		s.drawManinSave(canvas)
	case maninScreenMuseum, maninScreenMusic, maninScreenCG, maninScreenScene, maninScreenHch, maninScreenVulgar:
		s.drawManinGallery(canvas)
	case maninScreenMap:
		s.drawManinMap(canvas)
	case maninScreenAttack:
		s.drawManinAttack(canvas)
	case maninScreenOrganize:
		s.drawManinOrganize(canvas)
	case maninScreenLog:
		s.drawManinLog(canvas)
	case maninScreenStaffRoll:
		s.drawManinStaffRoll(canvas)
	case maninScreenName:
		s.drawManinName(canvas)
	}
}

// drawManinTitle 只处理菜单悬停：s_title2 是按下/悬停帧，底图仍是 s_title1。
func (s *Session) drawManinTitle(canvas *gfx.BgraCanvas) {
	if s.manin == nil || canvas == nil {
		return
	}
	if s.manin.hover >= 0 && s.manin.hover < 6 {
		if s.maninDrawImage(canvas, "s_title2", 0, 0) {
			// s_title2 不包含第 7 项，下面仍要补 hgon。
		} else {
			s.maninDrawImage(canvas, "s_title1", 0, 0)
		}
	} else {
		s.maninDrawImage(canvas, "s_title1", 0, 0)
	}
	if s.maninVulgarAny() {
		s.maninDrawImage(canvas, "hgon", 400, 160)
	}
}

func (s *Session) pointerManinUI(x, y int) bool {
	if s.manin == nil {
		return false
	}
	switch s.manin.screen {
	case maninScreenSystem:
		return s.pointerManinSystem(x, y)
	case maninScreenSave, maninScreenLoad:
		return s.pointerManinSave(x, y)
	case maninScreenMuseum, maninScreenMusic, maninScreenCG, maninScreenScene, maninScreenHch, maninScreenVulgar:
		return s.pointerManinGallery(x, y)
	case maninScreenMap:
		return s.pointerManinMap(x, y)
	case maninScreenAttack:
		return s.pointerManinAttack(x, y)
	case maninScreenOrganize:
		return s.pointerManinOrganize(x, y)
	case maninScreenLog:
		return s.pointerManinLog(x, y)
	case maninScreenStaffRoll:
		return s.pointerManinStaffRoll(x, y)
	case maninScreenName:
		return s.pointerManinName(x, y)
	}
	return false
}

// maninHover 记录悬停项；各界面绘制时按 u.hover 画描边。标题用 0..5
// 表示菜单项，-1 表示没有悬停。
func (s *Session) maninHover(x, y int) {
	if s.manin == nil {
		return
	}
	u := s.manin
	u.hover = -1
	if s.titleMode && !s.maninScreenActive() {
		for index, item := range s.maninTitleItems() {
			if x >= int(item.X) && x < int(item.X)+int(item.Width) &&
				y >= int(item.Y) && y < int(item.Y)+int(item.Height) {
				u.hover = index
				return
			}
		}
		return
	}
	switch u.screen {
	case maninScreenSystem:
		u.hover = maninSystemHitIndex(x, y)
	case maninScreenSave, maninScreenLoad:
		u.hover = maninSaveRowAt(x, y, u.save.Mode, u.save.Page)
	case maninScreenMuseum:
		u.hover = maninMuseumHitIndex(x, y)
	case maninScreenMusic:
		u.hover = maninMusicRowAt(x, y)
	case maninScreenCG, maninScreenScene:
		u.hover = maninGalleryCellAt(x, y)
	case maninScreenHch:
		u.hover = maninHchCellAt(x, y)
	case maninScreenVulgar:
		u.hover = maninVulgarHitIndex(x, y)
	case maninScreenMap:
		if id := s.maninMapStationID(x, y); id > 0 {
			u.hover = id - 1
		}
	case maninScreenAttack:
		if u.attack.Phase == maninAttackSelect && x >= 24 && x < 200 && y >= 126 && y < 128+maninAttackRowsPerPage*26 {
			u.hover = (y - 128) / 26
		}
	case maninScreenOrganize:
		if x >= 20 && x < 302 && y >= 88 && y < 88+12*26 {
			u.hover = (y - 88) / 26
		}
	case maninScreenName:
		if x >= 34 && x < 34+5*32 && y >= 100 && y < 100+10*31 {
			u.hover = (y - 100) / 31
		}
	}
}

// maninBack 是右键/返回键的语义：能关的界面关掉，攻击间不能中途取消。
func (s *Session) maninBack() {
	if s.manin == nil {
		return
	}
	if s.manin.vulgarPlayback {
		s.manin.vulgarPlayback = false
		s.maninReturnToTitle()
		return
	}
	switch s.manin.screen {
	case maninScreenNone:
		return
	case maninScreenAttack:
		if s.manin.attack.Phase == maninAttackSelect && len(s.manin.attack.Selected) == 0 {
			// 还没选词时允许退出，避免选错房间卡死；已开始执行不能反悔。
			s.finishManinAttack(5)
		}
	case maninScreenOrganize:
		// 整理界面由 XR 暂停，关闭时必须把自定义 stop 清掉，否则脚本
		// 会永远停在 BdtStopOrganize 上。
		s.maninCloseOrganizer()
	case maninScreenStaffRoll:
		s.manin.staff.Index = 99
		s.maninAdvanceStaffRoll()
	default:
		s.maninExitScreen()
	}
}

// maninToolbarClick 处理消息框顶边的 SAVE/LOAD/SKIP/AUTO/LOG/SYS。
// 坐标来自 qb_win 的六个色块，窗口固定在 y=360..476。
func (s *Session) maninToolbarClick(x, y int) bool {
	if s.manin == nil || s.titleMode || s.maninScreenActive() {
		return false
	}
	if y < 360 || y >= 376 {
		return false
	}
	switch {
	case x >= 300 && x < 360:
		s.maninEnterScreen(maninScreenSave)
		return true
	case x >= 360 && x < 412:
		s.maninEnterScreen(maninScreenLoad)
		return true
	case x >= 412 && x < 464:
		s.setHostRequest("skip")
		return true
	case x >= 464 && x < 516:
		s.setHostRequest("auto")
		return true
	case x >= 516 && x < 568:
		s.manin.log = maninLogState{}
		s.maninEnterScreen(maninScreenLog)
		return true
	case x >= 568 && x < 620:
		s.maninEnterScreen(maninScreenSystem)
		return true
	}
	return false
}

// ------------------------------------------------------------------ SYSTEM

type maninRect struct{ x, y, w, h int }

var maninSystemRects = struct {
	window, fullscreen               maninRect
	effectOn, effectOff              maninRect
	bgmOn, bgmOff                    maninRect
	seOn, seOff                      maninRect
	voiceOn, voiceOff                maninRect
	voiceRows                        [6]maninRect
	msgNormal, msgFast               maninRect
	skipNormal, skipFast, skipChoice maninRect
	unreadOn, unreadOff              maninRect
	back, title, quit                maninRect
}{
	window:     maninRect{36, 107, 166, 24},
	fullscreen: maninRect{36, 133, 166, 24},
	effectOn:   maninRect{36, 203, 78, 24},
	effectOff:  maninRect{122, 203, 80, 24},
	bgmOn:      maninRect{36, 273, 78, 24},
	bgmOff:     maninRect{122, 273, 80, 24},
	seOn:       maninRect{36, 343, 78, 24},
	seOff:      maninRect{122, 343, 80, 24},
	voiceOn:    maninRect{236, 107, 80, 24},
	voiceOff:   maninRect{324, 107, 80, 24},
	voiceRows: [6]maninRect{
		{236, 199, 168, 24}, {236, 225, 168, 24}, {236, 251, 168, 24},
		{236, 277, 168, 24}, {236, 303, 168, 24}, {236, 329, 168, 24},
	},
	msgNormal:  maninRect{438, 107, 78, 24},
	msgFast:    maninRect{524, 107, 80, 24},
	skipNormal: maninRect{438, 216, 78, 24},
	skipFast:   maninRect{524, 216, 80, 24},
	skipChoice: maninRect{438, 242, 166, 24},
	unreadOn:   maninRect{438, 306, 78, 24},
	unreadOff:  maninRect{524, 306, 80, 24},
	back:       maninRect{32, 432, 174, 32},
	title:      maninRect{232, 432, 176, 32},
	quit:       maninRect{434, 432, 174, 32},
}

func maninRectHit(rect maninRect, x, y int) bool {
	return x >= rect.x && x < rect.x+rect.w && y >= rect.y && y < rect.y+rect.h
}

// maninSystemHitIndex 把 SYSTEM 控件编号成 0..N，供 hover 描边；-1 无。
func maninSystemHitIndex(x, y int) int {
	r := maninSystemRects
	switch {
	case maninRectHit(r.window, x, y), maninRectHit(r.fullscreen, x, y):
		return 0
	case maninRectHit(r.effectOn, x, y), maninRectHit(r.effectOff, x, y):
		return 1
	case maninRectHit(r.bgmOn, x, y), maninRectHit(r.bgmOff, x, y):
		return 2
	case maninRectHit(r.seOn, x, y), maninRectHit(r.seOff, x, y):
		return 3
	case maninRectHit(r.voiceOn, x, y), maninRectHit(r.voiceOff, x, y):
		return 4
	case maninRectHit(r.msgNormal, x, y), maninRectHit(r.msgFast, x, y):
		return 5
	case maninRectHit(r.skipNormal, x, y), maninRectHit(r.skipFast, x, y), maninRectHit(r.skipChoice, x, y):
		return 6
	case maninRectHit(r.unreadOn, x, y), maninRectHit(r.unreadOff, x, y):
		return 7
	case x >= 240 && x < 620 && y >= 372 && y < 398:
		return 8
	case maninRectHit(r.back, x, y), maninRectHit(r.title, x, y), maninRectHit(r.quit, x, y):
		return 9
	}
	for index, row := range r.voiceRows {
		if maninRectHit(row, x, y) {
			return 10 + index
		}
	}
	return -1
}

func (s *Session) drawManinSystem(canvas *gfx.BgraCanvas) {
	u := s.manin
	canvas.Clear(0, 0, 0, 255)
	if !s.maninDrawImage(canvas, "s_sys01", 0, 0) {
		s.maninDrawText(canvas, "SYSTEM", 28, 24, 16, 600, 1, 0xffffffff)
	}
	mark := func(rect maninRect, on bool) {
		if !on {
			return
		}
		canvas.FillBlend(rect.x, rect.y, rect.w, rect.h, 60, 40, 20, 90)
		canvas.Fill(rect.x, rect.y+rect.h-1, rect.w, 2, 80, 60, 20, 220)
	}
	mark(maninSystemRects.window, u.sys.Fullscreen)
	mark(maninSystemRects.fullscreen, !u.sys.Fullscreen)
	mark(maninSystemRects.effectOn, u.sys.EffectOn)
	mark(maninSystemRects.effectOff, !u.sys.EffectOn)
	mark(maninSystemRects.bgmOn, u.sys.BgmOn)
	mark(maninSystemRects.bgmOff, !u.sys.BgmOn)
	mark(maninSystemRects.seOn, u.sys.SeOn)
	mark(maninSystemRects.seOff, !u.sys.SeOn)
	mark(maninSystemRects.voiceOn, u.sys.VoiceOn)
	mark(maninSystemRects.voiceOff, !u.sys.VoiceOn)
	mark(maninSystemRects.msgNormal, !u.sys.MessageFast)
	mark(maninSystemRects.msgFast, u.sys.MessageFast)
	mark(maninSystemRects.skipNormal, u.sys.SkipFast == 0)
	mark(maninSystemRects.skipFast, u.sys.SkipFast == 1)
	mark(maninSystemRects.skipChoice, u.sys.SkipFast == 2)
	mark(maninSystemRects.unreadOn, u.sys.SkipUnread)
	mark(maninSystemRects.unreadOff, !u.sys.SkipUnread)
	for index, row := range maninSystemRects.voiceRows {
		if u.sys.VoiceByGirl[index] {
			canvas.Fill(row.x+row.w-16, row.y+7, 10, 10, 60, 220, 60, 255)
		} else {
			canvas.Fill(row.x+row.w-16, row.y+7, 10, 10, 60, 60, 220, 255)
		}
	}
	// オート速度滑杆：底图只画了轨道，这里补一个红色滑块。
	knob := 240 + int(float64(u.sys.AutoSpeed)/10.0*370)
	canvas.Fill(knob, 374, 8, 20, 40, 40, 220, 255)
}

func (s *Session) pointerManinSystem(x, y int) bool {
	u := s.manin
	r := maninSystemRects
	switch {
	case maninRectHit(r.window, x, y):
		u.sys.Fullscreen = true
	case maninRectHit(r.fullscreen, x, y):
		u.sys.Fullscreen = false
	case maninRectHit(r.effectOn, x, y):
		u.sys.EffectOn = true
	case maninRectHit(r.effectOff, x, y):
		u.sys.EffectOn = false
	case maninRectHit(r.bgmOn, x, y):
		u.sys.BgmOn = true
	case maninRectHit(r.bgmOff, x, y):
		u.sys.BgmOn = false
	case maninRectHit(r.seOn, x, y):
		u.sys.SeOn = true
	case maninRectHit(r.seOff, x, y):
		u.sys.SeOn = false
	case maninRectHit(r.voiceOn, x, y):
		u.sys.VoiceOn = true
	case maninRectHit(r.voiceOff, x, y):
		u.sys.VoiceOn = false
	case maninRectHit(r.msgNormal, x, y):
		u.sys.MessageFast = false
	case maninRectHit(r.msgFast, x, y):
		u.sys.MessageFast = true
	case maninRectHit(r.skipNormal, x, y):
		u.sys.SkipFast = 0
	case maninRectHit(r.skipFast, x, y):
		u.sys.SkipFast = 1
	case maninRectHit(r.skipChoice, x, y):
		u.sys.SkipFast = 2
	case maninRectHit(r.unreadOn, x, y):
		u.sys.SkipUnread = true
	case maninRectHit(r.unreadOff, x, y):
		u.sys.SkipUnread = false
	case x >= 240 && x < 620 && y >= 372 && y < 398:
		u.sys.AutoSpeed = (x - 240) * 10 / 380
		if u.sys.AutoSpeed < 0 {
			u.sys.AutoSpeed = 0
		}
		if u.sys.AutoSpeed > 10 {
			u.sys.AutoSpeed = 10
		}
	case maninRectHit(r.back, x, y):
		s.maninApplySettings()
		s.maninExitScreen()
	case maninRectHit(r.title, x, y):
		s.maninApplySettings()
		s.maninReturnToTitle()
	case maninRectHit(r.quit, x, y):
		s.maninApplySettings()
		s.setHostRequest("exit")
	default:
		for index, row := range r.voiceRows {
			if maninRectHit(row, x, y) {
				u.sys.VoiceByGirl[index] = !u.sys.VoiceByGirl[index]
				break
			}
		}
	}
	s.maninApplySettings()
	s.composerInvalidate()
	return true
}

// maninApplySettings 把界面开关写回全局并立即生效（BGM/SE/语音音量）。
func (s *Session) maninApplySettings() {
	if s.manin == nil {
		return
	}
	u := s.manin
	u.global.Settings = u.sys
	if u.sys.BgmOn {
		s.gameState.SetBgmVolume(1)
	} else {
		s.gameState.SetBgmVolume(0)
	}
	if u.sys.SeOn {
		s.sfxVolume = 1
	} else {
		s.sfxVolume = 0
	}
	if u.sys.VoiceOn {
		s.voiceVolume = 1
	} else {
		s.voiceVolume = 0
	}
	saveManinGlobal(s.maninSaveDir(), u.global)
}

func (s *Session) maninReturnToTitle() {
	if s.manin == nil || s.machine == nil {
		return
	}
	if s.titleMode {
		s.maninExitScreen()
		return
	}
	s.maninCloseAll()
	s.resetManinRun()
	// 回标题等于放弃本周目；再点「始めから」时从 100-01 新开，不重播
	// 999-98 的警告/Logo 过场（原作也是直接进正片）。舞台/音乐也一起清掉，
	// 避免旧场景在 100-01 铺底前闪一帧。
	s.backgroundName = ""
	s.sprites = nil
	s.bgmTrack = 0
	s.gameState.ClearBgm()
	s.stage.ClearSprites()
	s.stage.SetFace("")
	s.stage.ShowMessage("", "")
	s.machine.Start("100-01")
	s.SetBgm(4, 0)
	s.beginTitle()
}

// ------------------------------------------------------------------ 存读档

func maninSaveRowAt(x, y int, saveMode bool, page int) int {
	if y < 31 || y >= 31+19*21 {
		return -1
	}
	left, right := 63, 437
	if saveMode {
		left, right = 246, 620
	}
	if x < left || x >= right {
		if x >= 464 && x < 608 && y >= 386 && y < 450 {
			return 20 // BACK
		}
		if saveMode && x >= 32 && x < 176 && y >= 384 && y < 448 {
			return 20
		}
		return -1
	}
	row := (y - 31) / 21
	if row < 0 || row >= 19 {
		return -1
	}
	return row
}

func (s *Session) drawManinSave(canvas *gfx.BgraCanvas) {
	u := s.manin
	canvas.Clear(0, 0, 0, 255)
	name := "s_load1"
	if u.save.Page >= 2 {
		name = "s_load2"
	}
	if u.save.Mode {
		name = "s_save1"
		if u.save.Page >= 2 {
			name = "s_save2"
		}
	}
	image := s.maninImage(name)
	if image != nil {
		// s_load1/s_save1 是 1280x480 的双页图；page%2 决定取左半还是右半，
		// 和原作翻页时画面横向滑动的观感一致。
		sourceX := 0
		if u.save.Page%2 == 1 {
			sourceX = 640
		}
		if sourceX+640 <= image.Width {
			canvas.DrawRegion(image, sourceX, 0, 640, 480, 0, 0)
		} else {
			canvas.Draw(image, 0, 0)
		}
	}
	if u.save.Page < 0 {
		u.save.Page = 0
	}
	slots := u.save.Slots
	if len(slots) == 0 {
		slots = s.Slots()
	}
	left := 63
	if u.save.Mode {
		left = 246
	}
	for row := 0; row < 19; row++ {
		index := u.save.Page*19 + row
		y := 31 + row*21
		if u.hover == row {
			canvas.FillBlend(left, y, 374, 20, 40, 40, 120, 80)
		}
		if index < 0 || index >= len(slots) || !slots[index].Exists {
			s.maninDrawText(canvas, "-- No Data --", 10, left+4, y+3, 360, 1, 0xffb0b0b0)
			continue
		}
		summary := strings.TrimSpace(slots[index].Summary)
		if summary == "" {
			summary = strings.TrimSpace(slots[index].Title)
		}
		if summary == "" {
			summary = "No Title"
		}
		stamp := maninSlotStamp(slots[index])
		line := summary
		if stamp != "" {
			line = stamp + "  " + summary
		}
		s.maninDrawText(canvas, line, 10, left+4, y+3, 366, 1, 0xff202020)
	}
	s.maninDrawText(canvas, "PAGE "+strconv.Itoa(u.save.Page+1)+"/3", 12, 560, 6, 70, 1, 0xffffffff)
}

func (s *Session) pointerManinSave(x, y int) bool {
	u := s.manin
	row := maninSaveRowAt(x, y, u.save.Mode, u.save.Page)
	if row == 20 {
		s.maninExitScreen()
		return true
	}
	if row < 0 {
		// ページ送り：左右边缘没有色块，用它们做上一页/下一页，滚轮也支持。
		if x >= 620 {
			if u.save.Page < 2 {
				u.save.Page++
			}
			s.composerInvalidate()
			return true
		}
		if x < 30 && u.save.Page > 0 {
			u.save.Page--
			s.composerInvalidate()
			return true
		}
		return true
	}
	slot := u.save.Page*19 + row
	if slot == 0 && u.save.Page == 0 && u.save.Mode {
		// 第一页首行是オートセーブ専用，原作不允许手动覆盖。
		return true
	}
	if u.save.Mode {
		if err := s.Save(uint32(slot), ""); err != nil {
			core.Logf("bdt manin: save slot %d failed: %v", slot, err)
			return true
		}
		u.save.Slots = s.Slots()
		return true
	}
	slots := u.save.Slots
	if len(slots) == 0 {
		slots = s.Slots()
	}
	if slot < 0 || slot >= len(slots) || !slots[slot].Exists {
		return true
	}
	if _, err := s.Load(uint32(slot)); err != nil {
		core.Logf("bdt manin: load slot %d failed: %v", slot, err)
		return true
	}
	// 存档停在 MAP 时 Load 已经重开了地图；此时不能 maninCloseAll，
	// 否则会把地图关掉，玩家看到一帧黑屏且无法点站。
	if !s.maninScreenActive() {
		s.maninCloseAll()
	}
	s.composerInvalidate()
	return true
}

// ------------------------------------------------------------------ LOG

func (s *Session) drawManinLog(canvas *gfx.BgraCanvas) {
	u := s.manin
	canvas.Clear(4, 4, 12, 255)
	s.maninDrawText(canvas, "LOG", 18, 24, 10, 200, 1, 0xffffffff)
	lines := u.logLines
	visible := 18
	total := len(lines)
	start := total - visible - u.log.Scroll
	if start < 0 {
		start = 0
	}
	if start > total-1 && total > 0 {
		start = total - 1
	}
	y := 40
	for index := start; index < total && index < start+visible; index++ {
		line := lines[index]
		text := line.Text
		if line.Speaker != "" {
			text = line.Speaker + "「" + text + "」"
		}
		s.maninDrawText(canvas, text, 12, 24, y, 570, 1, 0xffe8e8e8)
		y += 23
	}
	// 滚动条：顶部=最新，底部=最早。
	if total > visible {
		ratio := float64(u.log.Scroll) / float64(total-visible)
		barY := 40 + int(ratio*380)
		canvas.Fill(612, barY, 6, 60, 120, 120, 120, 255)
	}
	canvas.Fill(520, 430, 100, 30, 80, 80, 140, 220)
	s.maninDrawText(canvas, "閉じる", 14, 550, 437, 80, 1, 0xffffffff)
}

func (s *Session) pointerManinLog(x, y int) bool {
	if x >= 520 && x < 620 && y >= 430 && y < 460 {
		s.maninExitScreen()
		return true
	}
	return true
}

// ------------------------------------------------------------------ 片尾

var maninStaffImages = []string{
	"e_logo", "e_cast", "e_ch0", "e_ch1", "e_ch2", "e_ch3", "e_ch4", "e_ch5",
	"e_staff00", "e_staff01", "e_staff02",
}

func (s *Session) drawManinStaffRoll(canvas *gfx.BgraCanvas) {
	canvas.Clear(0, 0, 0, 255)
	index := s.manin.staff.Index
	if index >= 0 && index < len(maninStaffImages) {
		s.maninDrawImage(canvas, maninStaffImages[index], 0, 0)
	}
	s.maninDrawText(canvas, "クリックでスキップ", 10, 480, 456, 150, 1, 0xff808080)
}

func (s *Session) pointerManinStaffRoll(x, y int) bool {
	if s.manin != nil {
		s.manin.staff.Index = 99
		s.maninAdvanceStaffRoll()
	}
	return true
}

// drawManinSubwindow 画原作的左上角小窗：当前车站 + 目标脸 + 事件标题。
// 原作还显示日期/时间，但日期在脚本里没有稳定的寄存器映射，硬猜会把
// 第 1 天写成第 N 天；这里只显示确定的信息，避免给玩家错误情报。
func (s *Session) drawManinSubwindow(canvas *gfx.BgraCanvas) {
	if s.manin == nil || s.machine == nil || canvas == nil {
		return
	}
	x := s.machine.X()
	if len(x) < 2 {
		return
	}
	// 反汇编 subwindow：X0=日、X1=时间带。X1=2..7 对应
	// 朝1/朝2/昼1/昼2/夜1/夜2；图标用 (X1-2)/2 选 mtime 的 3 帧。
	day := int(x[0])
	slot := int(x[1])
	if day < 1 || day > 99 || slot < 1 {
		return
	}
	s.maninDrawImage(canvas, "minfo", 10, 17)
	if image := s.maninImage("mdate10"); image != nil {
		canvas.DrawRegion(image, (day/10)*24, 0, 24, 32, 26, 20)
	}
	if image := s.maninImage("mdate_1"); image != nil {
		canvas.DrawRegion(image, (day%10)*24, 0, 24, 32, 54, 20)
	}
	if slot >= 2 && slot <= 7 {
		icon := (slot - 2) / 2
		if image := s.maninImage("mtime"); image != nil {
			canvas.DrawRegion(image, icon*32, 0, 32, 32, 160, 28)
		}
	}
}

// Wheel 只接管 manin 的滚动界面；其他情况返回当前步，保持 AFDVD 行为。
func (s *Session) Wheel(delta int32) (core.Step, error) {
	if s.disposed {
		return core.StepFinished, nil
	}
	if s.manin == nil {
		return s.currentStep(), nil
	}
	u := s.manin
	switch u.screen {
	case maninScreenLog:
		u.log.Scroll -= int(delta) * 3
		if u.log.Scroll < 0 {
			u.log.Scroll = 0
		}
		if u.log.Scroll > len(u.logLines) {
			u.log.Scroll = len(u.logLines)
		}
		s.composerInvalidate()
	case maninScreenSave, maninScreenLoad:
		if delta > 0 && u.save.Page < 2 {
			u.save.Page++
		} else if delta < 0 && u.save.Page > 0 {
			u.save.Page--
		}
		s.composerInvalidate()
	case maninScreenMusic:
		if delta > 0 && u.gallery.Page < 0 {
			u.gallery.Page = 0
		}
	case maninScreenAttack:
		if u.attack.Phase == maninAttackSelect {
			pool := u.attackPool()
			if len(pool) > 0 {
				u.attack.Cursor -= int(delta)
				if u.attack.Cursor < 0 {
					u.attack.Cursor = len(pool) - 1
				}
				if u.attack.Cursor >= len(pool) {
					u.attack.Cursor = 0
				}
				s.composerInvalidate()
			}
		}
	}
	return s.currentStep(), nil
}
