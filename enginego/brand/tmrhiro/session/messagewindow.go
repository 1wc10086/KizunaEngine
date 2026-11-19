// 本文件是"引擎自绘的消息框"：底图、角色名、正文、催促图标和底部工具条。
//
// 为什么必须由引擎画：原版的对话框底图 sys_window_base 是一张 1280x181 的
// 白色渐变遮罩，靠 D3D 的反相混合画成深色（dst = dst*(1-a) + (255-dst)*a）。
// 普通 alpha 叠加会把它画成一整片白；而 Flutter 的对话框又完全不是原版排版
// （字体、行距、名字位置、工具条全不一样）。所以这里按 _system 的
// [MESSAGEWINDOW] 逐像素复刻。
//
// 坐标关系：MW_DRAW_POS_Y 是底图在屏幕上的 y；正文/名字/催促图标的配置坐标
// 都相对这个原点。工具条（TWB_* 种类 0）的 Y 也相对它，X 是屏幕绝对坐标。
package session

import (
	"strings"
	"time"

	"kizuna/enginego/brand/tmrhiro/config"
	"kizuna/enginego/core/textrender"
)

// MessageAction 是工具条/菜单按钮的动作。
type MessageAction uint8

const (
	ActionNone MessageAction = iota
	ActionQSave
	ActionQLoad
	ActionBacklog
	ActionAuto
	ActionSkip
	ActionVoice
	ActionSystem
	ActionClose
	ActionSave
	ActionLoad
	ActionTitle
	ActionExit
	ActionMenu
	ActionConfig
	ActionScene
)

// toolbarAction 把原版按钮键名映射成动作。
//
// 键名来自 _system 的 TWB_*（底部工具条）与 MMB_*（暂停菜单）。两套名字有
// 重叠（TWB_SAVE / MMB_SAVE），动作也相同，所以一张表即可。
func toolbarAction(key string) MessageAction {
	switch key {
	case "QSAVE":
		return ActionQSave
	case "QLOAD":
		return ActionQLoad
	case "BACKLOG":
		return ActionBacklog
	case "AUTO":
		return ActionAuto
	case "SKIP":
		return ActionSkip
	case "VOICE":
		return ActionVoice
	case "SYSTEM", "SYSTEM2":
		return ActionSystem
	case "CLOSE":
		return ActionClose
	case "SAVE":
		return ActionSave
	case "LOAD":
		return ActionLoad
	case "TITLE":
		return ActionTitle
	case "EXIT":
		return ActionExit
	case "MENU":
		return ActionMenu
	case "SCENE_BACK":
		return ActionScene
	}
	return ActionNone
}

// drawMessageWindow 把对话框画到舞台画布。
//
// 层序与原版一致：底图（反相混合）→ 边框 → 名字 → 正文 → 催促图标 → 工具条。
// 工具条在消息框之上，因为按钮图里已经画好了文字；正文只到工具条上沿为止。
func (s *Session) composeMessageWindow() {
	if s.stage == nil || s.system == nil || !s.messageVisible {
		return
	}
	// 模态界面开着时不画对话框：原版的设置/存读档/鉴赏/回想都是整屏界面，
	// 对话框（含底部工具条）不会留在上面。以前只靠 layer 顺序去盖，
	// 但工具条压在界面下方，玩家会看到"System 界面 + 一条对话框文字"。
	if s.overlay != overlayNone || s.charaViewActive() {
		return
	}
	if s.screen != nil && s.pointerScreenOwns() {
		return
	}
	layout := s.ui.Message
	originX, originY := layout.X, layout.Y
	baseName := messageBaseName(layout)
	if baseName != "" {
		if img := s.cache.Get(baseName); img != nil {
			// 底图按"窗口区域"整块拉伸：素材是 1280x181，而 MW_DRAW_HEIGHT=257。
			// 原版把这张 181 行高的白色渐变纵向拉满 257 行再按 alpha 压暗场景，
			// 所以窗口从顶部近乎透明渐变到底部纯黑；按原尺寸贴只涂到 644，
			// 玩家会在底部看到一条没盖住的场景（手册截图的对话框一直铺到屏幕底）。
			width := layout.Width
			if width <= 0 {
				width = frameWidth
			}
			height := layout.Height
			if height <= 0 {
				height = img.Height
			}
			s.stage.ShadeImageScaledAlpha(img, originX, originY, width, height, s.windowAlphaScale())
		}
	}
	// 边框装饰（MW_FRAM_DRAW）：本作的 sys_window_frame 落在窗口区域底部
	// (463+227)，正好被工具条盖住，原版也是"只画底图渐变"。所以这里不再
	// 贴 frame 图，避免在工具条上方多出一条 631x22 的白框。
	textColor := layout.TextColor
	if s.lastMessageWasRead {
		textColor = layout.ReadColor
	}
	lineHeight := layout.TextNext
	if lineHeight <= 0 {
		lineHeight = layout.FontSize + 6
	}
	// 名字。NP_DRAW_FLAG 关闭时原版不画名字区域（个别作品把名字写进正文），
	// 这里仍然画：本作 start 的名字表是独立的（c01a_name），缺了玩家分不清谁在说话。
	if s.speaker != "" {
		nameX := originX + layout.NameX + layout.NamePlusX
		nameY := originY + layout.NameY + layout.NamePlusY
		s.drawUIRun(s.speaker, nameX, nameY, layout.FontSize, textColor, layout.NameWidth)
	}
	// 正文：逐字显示。reveal < 0 表示整段。
	body := s.visibleText()
	if body != "" {
		maxWidth := layout.Width - layout.TextX - 40
		maxWidth = s.ui.ScreenW - layout.TextX - 40
		s.drawUIRun(body, originX+layout.TextX, originY+layout.TextY,
			layout.FontSize, textColor, maxWidth)
		// 文字条目直接追加在标题画布/输入画布上时，也可能被下一次 Compose 的
		// Clear 清掉；这里统一由 Compose 末尾 TakeRuns 收口，不在这里处理。
	}
	// 催促图标：原版是 1 列 N 帧的竖条，按 CW_DRAW_TIME 循环。
	if s.showPrompt {
		s.drawWaitIcon(originX+layout.WaitX, originY+layout.WaitY, layout)
	}
	s.drawToolbar(originX, originY)
	s.drawNotice(originX, originY)
}

// drawNotice 画短暂的操作反馈（"クイックセーブしました"等）。
//
// 原版没有这条；但工具条里的 QSAVE/QLOAD 在成功时除了缩略图没有任何可见反馈，
// 触屏上玩家会以为按钮没响应。提示画在对话框上沿外侧，几秒后由 tickNotice 清掉。
func (s *Session) drawNotice(originX, originY int) {
	if s.notice == "" {
		return
	}
	s.drawUIRun(s.notice, originX+30, originY-34, 18, 0xffffffff, 600)
}

// tickNotice 到期清掉提示（由 Compose 每帧调用）。
func (s *Session) tickNotice(now time.Time) {
	if s.notice == "" {
		return
	}
	if s.noticeAt.IsZero() {
		s.noticeAt = now
		return
	}
	if now.Sub(s.noticeAt) > 3*time.Second {
		s.notice = ""
		s.noticeAt = time.Time{}
	}
}

// messageBaseName 选出当前该用的对话框底图。
//
// MW_GRAPHIC_COUNT 给出底图变体张数：本作是 2，sys_window_base 与
// sys_window_base01 只差一像素描边；多出来的 02 用作"窗口位置切换"的另一档。
// 原版按 window stat 选，这里固定用第一张（与手册截图一致）。
func messageBaseName(layout config.MessageWindow) string {
	return "sys_window_base"
}

// windowAlphaScale 返回对话框遮罩的整体不透明度系数（0..1）。
//
// 原版把 SYS_DEF_WINDOW_STAT（0xB3000000）当作 D3D 材质色，A=0xB3=0.7，
// 底图的每像素 alpha 乘以它才是实际混合强度。以前直接用满 alpha，把画面
// 压成纯黑；这里按配置还原（缺省 0.7 与本作默认值一致）。
func (s *Session) windowAlphaScale() float64 {
	const fallback = 0.7
	if s.system == nil {
		return fallback
	}
	value := strings.TrimSpace(s.system.Get("SYSTEM", "SYS_DEF_WINDOW_STAT"))
	// 值写成 "B3000000" 这样的 32 位十六进制 ARGB。
	if len(value) != 8 {
		return fallback
	}
	var parsed uint32
	for i := 0; i < 8; i++ {
		ch := value[i]
		var digit uint32
		switch {
		case ch >= '0' && ch <= '9':
			digit = uint32(ch - '0')
		case ch >= 'a' && ch <= 'f':
			digit = uint32(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			digit = uint32(ch-'A') + 10
		default:
			return fallback
		}
		parsed = parsed<<4 | digit
	}
	alpha := float64((parsed>>24)&0xFF) / 255.0
	if alpha <= 0 || alpha > 1 {
		return fallback
	}
	return alpha
}

// visibleText 返回"按逐字进度截断"的正文。
//
// 原版把一句话按正文速度一个字一个字地送进消息窗；整段一次贴出会让
// 玩家的第一次点击变成"瞬间全显"，观感与原作完全不同。进度由 session
// 的时钟推进（见 tickReveal），这里只负责截断。
//
// 返回空串是刻意的：句子的第一帧必须是空的。以前这里在 revealAt 为零值时
// 返回整句，结果对话框里从不出现"逐字"过程（第一帧就是满字，之后每帧
// 只会更长），玩家看到的永远是整句。
func (s *Session) visibleText() string {
	if s.text == "" {
		return ""
	}
	if s.revealDone {
		return s.text
	}
	return s.revealedText
}

// tickReveal 推进逐字显示。返回 true 表示本帧需要重绘。
func (s *Session) tickReveal(now time.Time) bool {
	if s.text == "" {
		s.revealedText = ""
		return false
	}
	if s.revealDone {
		if s.revealedText != s.text {
			s.revealedText = s.text
			return true
		}
		return false
	}
	if s.revealAt.IsZero() {
		// 第一次显示这句：从头开始。
		//
		// 注意：这里只记起点，不把前缀写空——revealAt 为 0 时 visibleText()
		// 会退回整句（见 visibleText 的注释），否则"第一帧还没排版"的那一帧
		// 会显示成空对话框。
		runes := []rune(s.text)
		s.revealRunes = 0
		s.revealAt = now
		s.revealDone = len(runes) == 0
	}
	runes := []rune(s.text)
	// 正文速度来自 SYS_DEF_TEXTSPEED（1..10，越大越快）：原版每字 30ms 起，
	// 每档减 3ms，最快 3ms/字。
	speed := s.textSpeed
	if speed <= 0 {
		speed = 1
	}
	if speed > 10 {
		speed = 10
	}
	per := time.Duration(30-(speed-1)*3) * time.Millisecond
	if s.skipUnread || s.autoActive {
		// AUTO 与 SKIP 走固定快节奏（原版 AUTO 也加速正文）。
		per = 12 * time.Millisecond
	}
	if per < 3*time.Millisecond {
		per = 3 * time.Millisecond
	}
	limit := int(now.Sub(s.revealAt)/per) + 1
	if limit < 1 {
		limit = 1
	}
	if limit > len(runes) {
		limit = len(runes)
	}
	if limit <= s.revealRunes {
		return false
	}
	s.revealRunes = limit
	s.revealedText = string(runes[:limit])
	if limit >= len(runes) {
		s.revealDone = true
	}
	return true
}

// textHasRevealed 报告正文是否已经显示完（催促图标只在这时出现）。
//
// 新建/读档/切过场时 revealAt 为零值，此时不能要求 revealedText == text：
// 逐字显示要到下一次 tickReveal 才真正开始，这期间如果拿"前缀等于全文"
// 判定，正文会卡成空字符串（玩家看到的是只有底图的对话框）。
func (s *Session) textHasRevealed() bool {
	if s.text == "" {
		return true
	}
	if s.revealAt.IsZero() {
		return false
	}
	return s.revealDone || s.revealedText == s.text
}

// drawWaitIcon 画催促图标。
func (s *Session) drawWaitIcon(x, y int, layout config.MessageWindow) {
	frames := layout.WaitFrames
	if frames <= 0 {
		return
	}
	img := s.cache.Get("sys_click_wait")
	if img == nil {
		return
	}
	frameH := layout.WaitH
	if frameH <= 0 {
		frameH = img.Height
	}
	rows := img.Height / frameH
	if rows <= 0 {
		return
	}
	speed := layout.WaitTimeMs
	if speed <= 0 {
		speed = 50
	}
	frame := int((time.Now().UnixMilli() / int64(speed)) % int64(frames))
	if frame >= rows {
		frame = rows - 1
	}
	s.stage.DrawRegion(img, 0, frame*frameH, layout.WaitW, frameH, x, y)
}

// drawToolbar 画底部工具条。
//
// 按钮图 sys_window_button 是一张 577x165 的横排大图，按配置里的源矩形裁出
// 每一格（Q-SAVE / Q-LOAD / BACKLOG / AUTO / SKIP / VOICE / SYSTEM / CLEAR）。
// 悬停用 sys_window_button02（同一张图的高亮版本）。
func (s *Session) drawToolbar(originX, originY int) {
	if s.ui == nil || s.ui.Message.Width <= 0 {
		return
	}
	buttonImage := "sys_window_button"
	if s.hoveredToolbar >= 0 {
		buttonImage = "sys_window_button02"
	}
	img := s.cache.Get(buttonImage)
	if img == nil {
		return
	}
	for index, button := range s.ui.Toolbar.Buttons {
		if !button.Enabled {
			continue
		}
		srcX, srcY := button.SrcX, button.SrcY
		if index == s.hoveredToolbar {
			// 高亮图与常态图尺寸一致，源矩形相同。
			srcX, srcY = button.SrcX, button.SrcY
		}
		w, h := button.W, button.H
		if w <= 0 || h <= 0 {
			continue
		}
		s.stage.DrawRegion(img, srcX, srcY, w, h, button.X, originY+button.Y)
	}
}

// drawUIRun 把一行文字排到舞台画布（引擎只排版，字形由宿主画）。
func (s *Session) drawUIRun(text string, x, y, size int, color uint32, maxWidth int) {
	if text == "" || s.stage == nil {
		return
	}
	renderer := uiTextRenderer()
	if renderer == nil {
		return
	}
	lineHeight := size + 6
	lines := 2
	if s.ui != nil && s.ui.Message.LineMax > 0 {
		lines = s.ui.Message.LineMax
	}
	// 原版正文没有描边/阴影以外的修饰；SYS_DEF_TEXT_DRAW_SHADOW=2 是"带阴影"，
	// 这里按 1px 白阴影近似（原版阴影就是描边色）。
	rendered := renderer.RenderOutline(text, size, 0, 0, lineHeight, maxWidth, lines, color, 0)
	if rendered == nil {
		return
	}
	s.stage.Canvas().AppendRuns(rendered.RunsAt(x, y))
}

var uiRenderer = func() *textrender.Renderer {
	r, _ := textrender.New()
	return r
}()

func uiTextRenderer() *textrender.Renderer { return uiRenderer }

// hitToolbar 返回命中的工具条按钮下标（-1 表示没命中）。
func (s *Session) hitToolbar(x, y int) int {
	if s.ui == nil {
		return -1
	}
	originY := s.ui.Message.Y
	for index, button := range s.ui.Toolbar.Buttons {
		if !button.Enabled {
			continue
		}
		rect := config.Rect{X: button.X, Y: originY + button.Y, W: button.W, H: button.H}
		if rect.Contains(x, y) {
			return index
		}
	}
	return -1
}
