package tj2

import (
	"math"
	"strings"
	"time"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/brand/bishop/vm/scf"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

// 旧版（GK CD 等 SCF 作品）的正文排版。
//
// 原引擎的对话框是 640x480 下的 640x124 窗口：内侧正文区约 516x103，
// 20 号字、行高 22、一行 24 个全角字、最多 4 行。这些数值不是随手取的，
// 而是按 SP09.KG 信息框的边框内沿反推出来的；字再大或行再多都会压到
// 右侧 SAVE/LOAD/SKIP 按钮或底部雕花边上。
const (
	tj2ClassicFontSize       = 20
	tj2ClassicLineHeight     = 22
	tj2ClassicTextMaxWidth   = 500
	tj2ClassicTextMaxLines   = 4
	tj2ClassicLineRunes      = 24
	tj2ClassicMessagePageCap = 4
)

// splitTj2MessagePages 把一条 !s 文本拆成"一屏一屏"的页。
//
// 旧脚本用两种标记控制排版：
//   - _r / _n：换行；
//   - _t：等待玩家点击后翻到下一屏（不是普通换行）。
//
// 以前 CleanTj2Text 把两者都变成 '\n'，宿主再把整段话塞进一个滚动框，
// 长对白会一次性堆出十几行，和原版逐屏显示完全不同。这里保留 _t 的
// 分页语义，并按 24 字 x 4 行硬切，保证任何一句都不会溢出信息框。
func splitTj2MessagePages(raw string) []string {
	return splitTj2MessagePagesWithLimit(raw, tj2ClassicLineRunes)
}

// splitTj2MessagePagesWithLimit 允许不同信息框用不同行宽。
//
// Toku2 的 sp_mes1 内侧可用宽度约 520px，19px 步进能放 26 个全角字；
// SP09 只有 500px，24 字更稳。行宽写死会让 Toku2 比原版提前换行。
func splitTj2MessagePagesWithLimit(raw string, limit int) []string {
	if len(raw) == 0 {
		return nil
	}
	segments := make([]string, 0, 4)
	var current strings.Builder
	for index := 0; index < len(raw); index++ {
		if raw[index] == '_' && index+1 < len(raw) {
			next := raw[index+1]
			if next == 't' || next == 'T' {
				segments = append(segments, current.String())
				current.Reset()
				index++
				continue
			}
		}
		current.WriteByte(raw[index])
	}
	segments = append(segments, current.String())

	pages := make([]string, 0, len(segments))
	for _, segment := range segments {
		lines := strings.Split(CleanTj2Text(segment), "\n")
		page := make([]string, 0, tj2ClassicMessagePageCap)
		flush := func() {
			for len(page) > 0 && strings.TrimSpace(page[len(page)-1]) == "" {
				page = page[:len(page)-1]
			}
			if len(page) == 0 {
				return
			}
			pages = append(pages, strings.Join(page, "\n"))
			page = page[:0]
		}
		for _, line := range lines {
			line = strings.TrimRight(line, " \t\r")
			if line == "" {
				if len(page) > 0 {
					page = append(page, "")
				}
				continue
			}
			for _, chunk := range wrapTj2Line(line, limit) {
				if len(page) >= tj2ClassicMessagePageCap {
					flush()
				}
				page = append(page, chunk)
			}
		}
		flush()
	}
	return pages
}

// wrapTj2Line 按全角字宽硬切。日文正文没有空格可断，按 rune 切最稳；
// 半角字符会略微提前换行，但不会越出信息框。
func wrapTj2Line(line string, limit int) []string {
	if limit <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) <= limit {
		return []string{line}
	}
	result := make([]string, 0, (len(runes)+limit-1)/limit)
	for len(runes) > 0 {
		count := limit
		if len(runes) < count {
			count = len(runes)
		}
		result = append(result, string(runes[:count]))
		runes = runes[count:]
	}
	return result
}

func (s *Tj2Session) ensureTextFont() *textrender.Renderer {
	if s.textFont != nil || s.textFontFailed {
		return s.textFont
	}
	font, err := textrender.New()
	if err != nil {
		s.textFontFailed = true
		core.Logf("tj2: text renderer unavailable: %v", err)
		return nil
	}
	s.textFont = font
	return font
}

// drawClassicMessageText 把当前页直接画进帧位图。
//
// 宿主的 Flutter MessageBox 是纯黑圆角框，尺寸和 SP09 的雕花框完全对不上；
// 经典分支必须由引擎自己写字，宿主只负责隐藏兜底框（State.EngineMessageWindow）。
func (s *Tj2Session) drawClassicMessageText() {
	text := s.gameState.Text()
	if len(text) == 0 {
		return
	}
	font := s.ensureTextFont()
	if font == nil {
		return
	}
	rendering.DrawText(s.frame, font, text,
		s.profile.MessageOriginX, s.profile.MessageOriginY,
		tj2ClassicFontSize, 1, 1, tj2ClassicLineHeight,
		tj2ClassicTextMaxWidth, tj2ClassicTextMaxLines,
		s.classicTextColor())
}

func (s *Tj2Session) classicTextColor() uint32 {
	return 0xFF000000 | uint32(s.textRed)<<16 | uint32(s.textGreen)<<8 | uint32(s.textBlue)
}

// drawUIText 是系统界面共用的文字入口。edge/shadow 1 像素让白字在
// SOUNDA/GALLARY 这类深色照片底上也能看清。
func (s *Tj2Session) drawUIText(text string, x, y, size, lineHeight, maxWidth, maxLines int, color uint32) {
	if len(text) == 0 {
		return
	}
	font := s.ensureTextFont()
	if font == nil {
		return
	}
	rendering.DrawText(s.frame, font, text, x, y, size, 1, 1, lineHeight, maxWidth, maxLines, color)
}

// sceneChapter 读取场景文件的第一条 !t 标题。
//
// 场景鉴赏的列表不是独立数据表，原引擎就是从各 SCNnnn.SCF 的首个
// chapter 命令取标题。文件开头偶尔有入口表，所以只在前 64 字节里找 !t；
// 找不到时退回编号，避免整个菜单因为一条坏脚本打不开。
func (s *Tj2Session) sceneChapter(number int) string {
	file, err := s.loadScenario(number)
	if err != nil || file == nil {
		return ""
	}
	code := file.Code()
	for index := 0; index+2 < len(code) && index < 64; index++ {
		if code[index] != 0x21 || code[index+1] != 0x74 {
			continue
		}
		start := index + 2
		end := start
		for end < len(code) && code[end] != 0 {
			end++
		}
		if end == start {
			return ""
		}
		text := code[start:end]
		return scf.DecodeText(text, scf.DetectTextEncoding(text))
	}
	return ""
}

// classicThumbNames 按 Thumb.DSK 的记录顺序列出 CG 缩略图。
//
// Thumb.DSK 的扩展名是 .THM，但文件内容是 KG 格式；资源索引按原扩展名
// 登记，所以这里直接用 images.Get("G1010.THM") 就能解码。顺序取归档内的
// 物理顺序（也是原作的 CG 排列顺序），不要按字典序重排。
func (s *Tj2Session) classicThumbNames() []string {
	if s.cgThumbs != nil {
		return s.cgThumbs
	}
	names := make([]string, 0, 300)
	for _, archive := range s.resources.Archives() {
		for _, entry := range archive.Entries() {
			if strings.HasSuffix(strings.ToUpper(entry.Path), ".THM") {
				names = append(names, entry.Path)
			}
		}
	}
	s.cgThumbs = names
	return names
}

// classicFullCGName 由缩略图名得到正片名。正片在 Visual.DSK 里是 .KG；
// IVENT.dsk 的 .IVD 目前没有解码器，所以不做回退，缺图时显示黑底。
func classicFullCGName(thumb string) string {
	stem := thumb
	if dot := strings.LastIndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	return stem + ".KG"
}

// ---- 转场效果 ----
//
// 旧脚本的 func1 是"画面闪白/闪黑"，参数是颜色 + 两段毫秒数；func0 是
// 震动。它们的共同点是短时间效果，不是永久状态，所以用墙钟计时并在
// Compose 里逐帧绘制，State().Animating 让宿主在这段时间保持合成。

func (s *Tj2Session) startShake(x, y, durationMs int) {
	if durationMs <= 0 {
		durationMs = 220
	}
	s.shakeX = x
	s.shakeY = y
	s.shakeUntil = time.Now().Add(time.Duration(durationMs) * time.Millisecond)
}

func (s *Tj2Session) startScreenFill(red, green, blue, durationMs int) {
	if durationMs <= 0 {
		durationMs = 400
	}
	if durationMs > 2000 {
		durationMs = 2000
	}
	s.fillRed = clampTj2Byte(red)
	s.fillGreen = clampTj2Byte(green)
	s.fillBlue = clampTj2Byte(blue)
	s.fillStart = time.Now()
	s.fillDuration = time.Duration(durationMs) * time.Millisecond
}

// fxAnimating 在效果刚结束的过渡帧也返回 true，让宿主多合成一帧，
// 把震动/残色清掉。State().Animating 用这个而不是 effectsActive。
func (s *Tj2Session) fxAnimating() bool {
	return s.effectsActive() || s.fxWasActive
}

func (s *Tj2Session) effectsActive() bool {
	if time.Now().Before(s.shakeUntil) {
		return true
	}
	if s.fillDuration > 0 && time.Since(s.fillStart) < s.fillDuration {
		return true
	}
	return false
}

func (s *Tj2Session) applyScreenEffects(dirty bool) bool {
	active := false
	if time.Now().Before(s.shakeUntil) {
		s.applyShake()
		active = true
	}
	if alpha, ok := s.screenFillAlpha(); ok && alpha > 0 {
		s.frame.FillBlend(0, 0, ScreenWidth, ScreenHeight, s.fillBlue, s.fillGreen, s.fillRed, int(alpha))
		active = true
	}
	return dirty || active
}

func (s *Tj2Session) screenFillAlpha() (byte, bool) {
	if s.fillDuration <= 0 {
		return 0, false
	}
	elapsed := time.Since(s.fillStart)
	if elapsed < 0 || elapsed >= s.fillDuration {
		return 0, false
	}
	// 三角波：0 -> 峰值 -> 0。闪白/闪黑都只在一瞬间达到最亮/最暗，
	// 而不是整整一秒钟全黑挡住玩家操作。
	progress := float64(elapsed) / float64(s.fillDuration)
	alpha := 1 - math.Abs(2*progress-1)
	return byte(alpha * 220), true
}

func (s *Tj2Session) applyShake() {
	if s.fxCanvas == nil {
		s.fxCanvas = gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
	}
	s.fxCanvas.CopyFrom(s.frame)
	dx := s.shakeX / 2
	dy := s.shakeY / 2
	if (time.Now().UnixMilli()/60)%2 == 0 {
		dx = -dx
		dy = -dy
	}
	if dx == 0 && dy == 0 {
		return
	}
	s.frame.Clear(0, 0, 0, 255)
	sourceX := 0
	sourceY := 0
	destX := dx
	destY := dy
	width := ScreenWidth
	height := ScreenHeight
	if destX < 0 {
		sourceX = -destX
		width += destX
		destX = 0
	}
	if destY < 0 {
		sourceY = -destY
		height += destY
		destY = 0
	}
	if width <= 0 || height <= 0 {
		return
	}
	if sourceX+width > ScreenWidth {
		width = ScreenWidth - sourceX
	}
	if sourceY+height > ScreenHeight {
		height = ScreenHeight - sourceY
	}
	s.frame.BlitCanvasRegion(s.fxCanvas, sourceX, sourceY, width, height, destX, destY, 255)
}
