// 引擎自绘 UI 的公共骨架：覆盖层状态机、文字/图片绘制、对话框与打字机。
//
// 为什么这些界面必须由引擎画：标题、存读档、设置、鉴赏、回想的坐标和贴图
// 都来自原版 sq_*.nut（见 tmpwork/ui_exact_spec.md）。交给 Flutter 重画会
// 出现“按钮图是原版、热区是另一套”的错位，玩家点不到看得见的按钮。
//
// 本文件只放共享原语与对话框；各覆盖层的合成/命中在 overlay_*.go 里，
// 这样四个界面可以独立修改而不互相覆盖。
package session

import (
	"strings"
	"sync"
	"time"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	overaudio "kizuna/enginego/brand/overdose/audio"
	"kizuna/enginego/brand/overdose/menus"
	"kizuna/enginego/core"
	"kizuna/enginego/core/textrender"
)

// overlayKind 是引擎自绘的模态界面。互斥：同一帧至多一个。
type overlayKind uint8

const (
	overlayNone overlayKind = iota
	overlaySave
	overlayLoad
	overlayConfig
	overlayBacklog
	overlayExtra
	overlayCGView
	overlayConfirm
)

// ---------------- 绘制原语 ----------------

var (
	uiRendererOnce sync.Once
	uiRenderer     *textrender.Renderer
)

func textRenderer() *textrender.Renderer {
	uiRendererOnce.Do(func() {
		uiRenderer, _ = textrender.New()
	})
	return uiRenderer
}

// drawImage 贴一张图；缺图返回 false，调用方据此跳过而不是画黑块。
func (s *Session) drawImage(canvas *bishopgfx.BgraCanvas, name string, x, y int) bool {
	if s.cache == nil {
		return false
	}
	img := s.cache.Get(name)
	if img == nil {
		return false
	}
	canvas.Blit(img, x, y, 255)
	return true
}

// drawButton 贴按钮：pressed 时用原版的 "-o" 选中图（BUTTON_SELECT）。
// 所有按钮图都是“常态 + -o”成对出现，找不到 -o 时退回常态。
func (s *Session) drawButton(canvas *bishopgfx.BgraCanvas, image string, r menus.Rect, pressed bool) {
	if pressed && s.drawImage(canvas, image+"-o", int(r.X), int(r.Y)) {
		return
	}
	s.drawImage(canvas, image, int(r.X), int(r.Y))
}

// drawImageAt 把图缩放到指定矩形（缩略图/全屏 CG 用）。
func (s *Session) drawImageAt(canvas *bishopgfx.BgraCanvas, name string, x, y, w, h int, alpha int) bool {
	if s.cache == nil {
		return false
	}
	img := s.cache.Get(name)
	if img == nil {
		return false
	}
	canvas.BlitScaled(img, x, y, w, h, alpha)
	return true
}

// drawText 把一段文字排到画布。color 是 0xAARRGGBB；maxWidth<=0 不按宽度折行。
// 引擎只做排版，文字条目追加到 canvas.Runs，由宿主用平台字体绘制。
func (s *Session) drawText(canvas *bishopgfx.BgraCanvas, text string, x, y, size, lineHeight, maxWidth, maxLines int, color uint32) {
	if text == "" || canvas == nil {
		return
	}
	r := textRenderer()
	if r == nil {
		return
	}
	// 原版 UIs 无描边/阴影：edge/shadow 都是 0，这里保持同一观感。
	rendered := r.RenderOutline(text, size, 0, 0, lineHeight, maxWidth, maxLines, color, 0)
	if rendered == nil {
		return
	}
	canvas.AppendRuns(rendered.RunsAt(x, y))
}

// drawTextCentered 在矩形内水平居中画一行。
func (s *Session) drawTextCentered(canvas *bishopgfx.BgraCanvas, text string, r menus.Rect, size int, color uint32) {
	rw := textWidth(text, size)
	x := int(r.X) + (int(r.W)-rw)/2
	if x < int(r.X) {
		x = int(r.X)
	}
	s.drawText(canvas, text, x, int(r.Y), size, size, 0, 1, color)
}

// textWidth 近似估算一行文字宽度：半角 0.5em，全角 1em。
// 只用于居中/裁剪，不参与最终排版，误差不会累积。
func textWidth(text string, size int) int {
	units := 0
	for _, r := range text {
		if isHalfWidth(r) {
			units++
		} else {
			units += 2
		}
	}
	return units * size / 2
}

func isHalfWidth(r rune) bool {
	// 正文排版只关心“占几格”：ASCII 与半角片假名算 1 格，其余（汉字/假名/
	// 全角标点）算 2 格。更精确的 East Asian Width 表对这里没有收益。
	if r <= 0x7F {
		return true
	}
	return r >= 0xFF61 && r <= 0xFF9F
}

// hit 是半开区间命中判定，和 TitleLayer 的热区换算保持同一约定。
func hit(r menus.Rect, x, y int32) bool {
	return x >= int32(r.X) && x < int32(r.X+r.W) && y >= int32(r.Y) && y < int32(r.Y+r.H)
}

// ---------------- 覆盖层状态 ----------------

// openOverlay 打开一个覆盖层；ret 是关闭后要回到的 step。
func (s *Session) openOverlay(kind overlayKind, ret core.Step) core.Step {
	s.overlay = kind
	s.overlayReturn = ret
	s.configDrag = 0
	s.step = ret
	return ret
}

// closeOverlay 关闭当前覆盖层并回到打开前的位置。
// 确认框关闭时回 confirmBack，而不是直接回剧情。
func (s *Session) closeOverlay() core.Step {
	if s.overlay == overlayConfirm {
		s.overlay = s.confirmBack
		s.confirmKind = ""
		s.step = s.overlayReturn
		return s.step
	}
	s.overlay = overlayNone
	s.configDrag = 0
	// 从标题打开的覆盖层关掉后必须回标题（TitleMenu 才会重新出现）。
	if s.atTitle {
		s.playTitleBGM()
		s.step = core.StepAwaitTitle
		s.overlayReturn = core.StepAwaitTitle
		return s.step
	}
	if s.overlayReturn == core.StepAwaitTitle {
		s.overlayReturn = core.StepAwaitInput
	}
	s.step = s.overlayReturn
	return s.step
}

// openConfirm 打开确认框。kind 取 save/overwrite/load/quit。
func (s *Session) openConfirm(kind string, slot int, back overlayKind) core.Step {
	s.confirmKind = kind
	s.confirmSlot = slot
	s.confirmBack = back
	s.overlay = overlayConfirm
	s.step = s.overlayReturn
	return s.step
}

// slotIndex 把界面上的 bank(0..9)/slot(0..9) 换成 1..100 的存档下标。
func slotIndex(bank, slot int) int { return bank*10 + slot + 1 }

// slotInfo 取一个槽位的展示信息；无档返回 Exists=false。
func (s *Session) slotInfo(index int) core.Slot {
	slot := core.Slot{Index: uint32(index)}
	if s.saves == nil || index < 1 || index > saveSlotCount {
		return slot
	}
	data, err := s.saves.Load(index)
	if err != nil || data == nil {
		return slot
	}
	slot.Exists = true
	slot.Title = data.Title
	slot.Summary = data.Summary
	if !data.SavedAt.IsZero() {
		slot.SavedAt = data.SavedAt.Format("2006-01-02 15:04")
	}
	return slot
}

// openSaveOverlay 打开存/读档界面。mode 只能是 overlaySave 或 overlayLoad。
func (s *Session) openSaveOverlay(mode overlayKind) core.Step {
	if mode != overlaySave && mode != overlayLoad {
		mode = overlaySave
	}
	s.saveMode = mode
	s.saveBank = 0
	// 选择肢状态（choiceOptions）没有进存档：在菜单上存档再读档会丢选项、
	// 让脚本卡在等选择。存档入口在选择肢上直接拒绝；读档仍允许（它替换整局）。
	if mode == overlaySave && !s.atTitle && (s.step == core.StepAwaitSelect || s.step == core.StepAwaitChoice) {
		return s.step
	}
	ret := core.StepAwaitInput
	if s.atTitle {
		ret = core.StepAwaitTitle
	} else if s.step == core.StepAwaitSelect || s.step == core.StepAwaitChoice || s.step == core.StepWait {
		ret = s.step
	}
	return s.openOverlay(mode, ret)
}

// openExtraOverlay 打开鉴赏页（CG/音乐/场景）。
func (s *Session) openExtraOverlay(tab galleryKind) core.Step {
	if tab == galleryNone {
		tab = galleryCG
	}
	s.extraTab = tab
	s.extraPage = 0
	ret := core.StepAwaitInput
	if s.atTitle {
		ret = core.StepAwaitTitle
	}
	return s.openOverlay(overlayExtra, ret)
}

// voiceAllowed 按角色别ボイス設定过滤语音。文件名前缀是原版三人的固定
// 命名（syu=優衣, sky=恭子, smi=美雪），UseChara 顺序与 sq_setting 的
// chkChara01/02/03（3/1/2）一致。其它前缀（路人/系统语音）不受开关影响。
func (s *Session) voiceAllowed(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "syu"):
		return s.settings.UseChara[0]
	case strings.HasPrefix(lower, "sky"):
		return s.settings.UseChara[1]
	case strings.HasPrefix(lower, "smi"):
		return s.settings.UseChara[2]
	}
	return true
}

// scaleVolume 把脚本给的通道音量（0..100）乘上系统设置的主音量/分路音量。
// 原版 opt.SetVolMaster/Music/Sound/Voice 在引擎音频层生效；只在设置界面推
// SetVolume 不够，下一句脚本 Play 又会用 100 盖回去，所以所有事件入口都要过这里。
func (s *Session) scaleVolume(kind overaudio.Category, volume int32) int32 {
	if volume < 0 {
		volume = 0
	}
	channel := s.settings.BgmVolume
	switch kind {
	case overaudio.CategorySound:
		channel = s.settings.SeVolume
	case overaudio.CategoryVoice:
		channel = s.settings.VoiceVolume
	}
	if channel < 0 {
		channel = 0
	}
	master := s.settings.MasterVolume
	if master < 0 {
		master = 0
	}
	return volume * channel / 100 * master / 100
}

// playTitleBGM 进入标题时播菜单 BGM（sq_menu OnCreate 的 bgm01）。
// 已经在播就不重发：从设置/读档界面返回标题时不能让曲子从头开始。
func (s *Session) playTitleBGM() {
	if s.audio == nil {
		return
	}
	if snap := s.audio.Snapshot(); snap.BgmFile == menus.TitleBgm {
		return
	}
	s.audio.Play(overaudio.PlayRequest{
		Channel: overaudio.Channel{Kind: overaudio.CategoryMusic, Idx: 0},
		Name:    menus.TitleBgm,
		Loop:    true,
		Volume:  s.scaleVolume(overaudio.CategoryMusic, 100),
	})
	s.bgmFile = menus.TitleBgm
}

// ---------------- 对话框与打字机 ----------------

// 原版 adv_msg_wnd 的几何（sq_adv.nut）：
//
//	名字 x=46 y=428 尺寸 180x42 字号 26
//	正文 x=78 y=458 每行 56 半角格（28 全角）3 行 字号 23 行距 25
const (
	msgNameX, msgNameY = 46, 428
	msgTextX, msgTextY = 78, 458
	msgCellWidth       = 56 // 半角格
	msgLinesPerPage    = 3
	msgPixelSize       = 23
	msgLineHeight      = 25
)

// setMessage 换一句新对白并按原版 3 行窗口分页，重置打字机。
func (s *Session) setMessage(text string) {
	s.text = text
	s.msgVisible = true
	s.msgFull = text
	s.msgPages = paginateMessage(text)
	s.msgPage = 0
	s.msgRevealAt = time.Now()
}

// setMessageImmediate 用于读档：整句直接显示，不重播打字机。
func (s *Session) setMessageImmediate(text string) {
	s.text = text
	s.msgFull = text
	s.msgPages = paginateMessage(text)
	s.msgPage = 0
	s.msgRevealAt = time.Time{}
}

// textSpeedMs 返回每个字的显示间隔。原版三档：slow=100, normal=30, fast=0。
func (s *Session) textSpeedMs() int {
	switch s.settings.TextSpeed {
	case 1:
		return 100
	case 3:
		return 0
	default:
		return 30
	}
}

// messagePageDone 报告当前页是否已经完整显示。
func (s *Session) messagePageDone() bool {
	if s.msgRevealAt.IsZero() {
		return true
	}
	speed := s.textSpeedMs()
	if speed <= 0 {
		return true
	}
	total := pageRuneCount(s.currentPageLines())
	shown := int(time.Since(s.msgRevealAt).Milliseconds())/speed + 1
	return shown >= total
}

// currentPageLines 返回当前页的原始行（未截断）。
func (s *Session) currentPageLines() []string {
	if s.msgPage < 0 || s.msgPage >= len(s.msgPages) {
		return nil
	}
	return s.msgPages[s.msgPage]
}

// revealedPageText 按已过时间截断当前页，返回可绘制的多行文本。
// 新行本身不消耗“字”，否则每翻一行都会慢一拍。
func (s *Session) revealedPageText() string {
	lines := s.currentPageLines()
	if len(lines) == 0 {
		return ""
	}
	limit := -1
	if !s.msgRevealAt.IsZero() {
		if speed := s.textSpeedMs(); speed > 0 {
			limit = int(time.Since(s.msgRevealAt).Milliseconds())/speed + 1
		}
	}
	if limit < 0 {
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	remaining := limit
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if remaining <= 0 {
			continue
		}
		runes := []rune(line)
		if remaining >= len(runes) {
			b.WriteString(line)
			remaining -= len(runes)
			continue
		}
		b.WriteString(string(runes[:remaining]))
		remaining = 0
	}
	return b.String()
}

func pageRuneCount(lines []string) int {
	n := 0
	for _, line := range lines {
		n += len([]rune(line))
	}
	return n
}

// paginateMessage 先把正文按 56 半角格硬换行，再每 3 行分一页。
// 硬换行是为了让打字机逐字显示时不会因重新折行而跳行；渲染时不再按像素
// 宽度折行（maxWidth=0），只认这里插入的 \n。
func paginateMessage(text string) [][]string {
	lines := wrapMessage(text)
	if len(lines) == 0 {
		return nil
	}
	var pages [][]string
	for i := 0; i < len(lines); i += msgLinesPerPage {
		end := i + msgLinesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	return pages
}

// wrapMessage 按原版 56 半角格/行换行；显式 \n 也保留。
func wrapMessage(text string) []string {
	var out []string
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		cur := make([]rune, 0, msgCellWidth)
		cells := 0
		flush := func() {
			out = append(out, string(cur))
			cur = cur[:0]
			cells = 0
		}
		for _, r := range paragraph {
			w := 1
			if !isHalfWidth(r) {
				w = 2
			}
			if cells+w > msgCellWidth && len(cur) > 0 {
				flush()
			}
			cur = append(cur, r)
			cells += w
		}
		flush()
	}
	return out
}

// composeMessageWindow 画对话框底图、说话人、正文和等待图标。
// 只有 msgVisible 时才画；msgoff 的脚本段落保持全屏演出。
func (s *Session) composeMessageWindow(canvas *bishopgfx.BgraCanvas) {
	if !s.msgVisible {
		return
	}
	// 设置里的「ウィンドウ透過率」直接作用于对话框底图；文字/按钮保持
	// 不透明。公式与 config_mw 预览一致：255-(100-opacity)*255/100。
	if s.cache != nil {
		if img := s.cache.Get(menus.MessageWindowImage); img != nil {
			op := int(s.settings.WindowOpacity)
			if op < 0 {
				op = 0
			}
			if op > 100 {
				op = 100
			}
			canvas.Blit(img, 0, 420, 255-(100-op)*255/100)
		}
	}
	if s.speaker != "" {
		s.drawText(canvas, s.speaker, msgNameX, msgNameY, 26, 26, 180, 1, 0xFFFFFFFF)
	}
	if s.text != "" || s.msgFull != "" {
		body := s.revealedPageText()
		if body == "" {
			body = s.msgFull
		}
		s.drawText(canvas, body, msgTextX, msgTextY, msgPixelSize, msgLineHeight, 0, msgLinesPerPage, 0xFFFFFFFF)
	}
	// 等待点击图标 adv_key_wait 是 30x180 的 6 帧条，间隔 200ms。
	if s.step == core.StepAwaitInput && s.messagePageDone() {
		if s.msgPage == len(s.msgPages)-1 {
			frame := int(time.Now().UnixMilli()/200) % 6
			if img := s.cache.Get("adv_key_wait"); img != nil {
				canvas.BlitRegion(img, 0, frame*30, 30, 30, 697, 540, 255)
			}
		}
	}
	s.composeAdvBar(canvas)
}

// composeAdvBar 画对话框内工具栏按钮（sq_adv 的 y=574 一排）。
// 原版底部 240..753 之间有 4 个按钮宽的空档，save/load/qsave/qload 由
// 引擎侧滑出面板注册（脚本里的 _ShowMenuPanel 是空实现，无法从字节码取得
// 面板坐标）。这里把 save/load 放回该空档，保证触屏玩家在剧情中也能存档；
// 其余四个（qsave/qload）继续走键盘/宿主入口，避免凭空发明界面。
func (s *Session) composeAdvBar(canvas *bishopgfx.BgraCanvas) {
	for _, b := range menus.AdvBar() {
		s.drawImage(canvas, b.Image, int(b.Rect.X), int(b.Rect.Y))
	}
	s.drawImage(canvas, "adv_save", 477, 574)
	s.drawImage(canvas, "adv_load", 556, 574)
}

// hitAdvBar 命中对话框工具栏，返回逻辑 key。
func (s *Session) hitAdvBar(x, y int32) (string, bool) {
	for _, b := range menus.AdvBar() {
		if hit(b.Rect, x, y) {
			return b.Key, true
		}
	}
	if hit(menus.Rect{X: 477, Y: 574, W: 76, H: 20}, x, y) {
		return "save", true
	}
	if hit(menus.Rect{X: 556, Y: 574, W: 76, H: 20}, x, y) {
		return "load", true
	}
	return "", false
}

// selectAdvBar 处理工具栏点击。auto/skip 交给宿主切自己的开关，
// 这样 Flutter 的自动推进/跳过逻辑与引擎状态保持同一份。
func (s *Session) selectAdvBar(key string) (core.Step, error) {
	switch key {
	case "auto":
		s.requestHost("auto")
		return s.step, nil
	case "skip":
		s.requestHost("skip")
		return s.step, nil
	case "log":
		return s.openOverlay(overlayBacklog, core.StepAwaitInput), nil
	case "config":
		return s.openOverlay(overlayConfig, core.StepAwaitInput), nil
	case "voice":
		if s.lastVoice != "" && s.audio != nil {
			s.audio.Play(overaudio.PlayRequest{
				Channel: overaudio.Channel{Kind: overaudio.CategoryVoice, Idx: 0},
				Name:    s.lastVoice,
			})
		}
		return s.step, nil
	case "close":
		// 原版 close 是“截图后关闭消息窗”，点一下只是把文字收起来；
		// 下一句对白到来时 setMessage 会重新打开。
		s.msgVisible = false
		return s.step, nil
	case "save":
		return s.openSaveOverlay(overlaySave), nil
	case "load":
		return s.openSaveOverlay(overlayLoad), nil
	}
	return s.step, nil
}

// requestHost 发一个宿主动作（串行号递增，宿主按序号去重）。
func (s *Session) requestHost(action string) {
	s.hostRequest = action
	s.hostRequestSerial++
}

// PressKey 处理桌面键盘。ESC 与原版右键同义；S/L/B/C 是给桌面端的
// 快捷键（Flutter 工具栏的“菜单”按钮就是发 ESC），触屏仍用工具栏。
func (s *Session) PressKey(code uint32) (core.Step, error) {
	switch code {
	case 27:
		return s.rightClickAt()
	case 'S':
		if !s.atTitle && s.overlay == overlayNone {
			return s.openSaveOverlay(overlaySave), nil
		}
	case 'L':
		if s.overlay == overlayNone {
			return s.openSaveOverlay(overlayLoad), nil
		}
	case 'C':
		if s.overlay == overlayNone {
			ret := s.step
			if s.atTitle {
				ret = core.StepAwaitTitle
			} else if ret == core.StepContinue || ret == core.StepFinished || ret == core.StepFaulted {
				ret = core.StepAwaitInput
			}
			return s.openOverlay(overlayConfig, ret), nil
		}
	case 'B':
		if !s.atTitle && s.overlay == overlayNone {
			ret := s.step
			if ret == core.StepContinue || ret == core.StepFinished || ret == core.StepFaulted {
				ret = core.StepAwaitInput
			}
			return s.openOverlay(overlayBacklog, ret), nil
		}
	}
	return s.step, nil
}
