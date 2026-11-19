// Package stage 是 TMR-Hiro 的软件合成层。
//
// 原引擎在 1280x720 的 32 位画布上分层绘制：背景 → 立绘（最多 4 个槽，
// 每个槽可由多个 GRD 部件合成）→ 全屏 CG → 对话框 → 菜单。这里用同样的层序，
// 像素格式固定 BGRA（与 core.Engine.Compose 的约定一致）。
//
// 坐标约定：
//   - GRD 自带“虚拟屏幕”坐标（OffsetX/OffsetY），整屏背景通常都是 0；
//   - 立绘的锚点是脚底中点：脚本里 x 是屏幕中心偏移，y 是脚底；
//   - 缩放以图片中心为原点（等价 lune2renpy 的 convert_pos_x/y）。
package stage

import (
	"fmt"
	"strings"
	"sync"

	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/tmrhiro/grd"
	"kizuna/enginego/core"
)

// 画面尺寸：原引擎默认 1280x720（_system 的 DEF_WINDOW_WIDTH/HEIGHT）。
const (
	FrameWidth  = 1280
	FrameHeight = 720
	// CharSlots 是原版 CHR_MAX=4。
	CharSlots = 4
)

// CharSlot 是一个立绘槽。
type CharSlot struct {
	Name  string  // fm_ 宏名或直接 GRD 名
	X     int     // 屏幕中心偏移
	Y     int     // 脚底 y
	Scale float64 // 等比缩放
	Alpha int     // 0..255
}

// Stage 是分层画布。
type Stage struct {
	cache *grd.Cache

	background string
	chars      [CharSlots]CharSlot
	cg         string
	fadeColor  string
	fadeAlpha  int
	filter     string

	canvas *bishopgfx.BgraCanvas
	runs   []core.TextRun

	mu    sync.Mutex
	text  []core.TextRun
	macro map[string][]string
	dirty bool

	// composed 是“宏名 → 合成图”的缓存。
	//
	// 一个立绘宏有 4~8 个部件、虚拟屏幕 1000x2048x4 = 8MB；不缓存的话
	// 每帧合成都要重解所有部件并重新混合，手机上会掉到个位数帧率。
	// 合成图只在换装/换表情时变，按名字缓存后稳态只是一次贴图。
	composed       map[string]*grd.Image
	order          []string
	composedBudget int
}

// NewStage 建舞台。
func NewStage(cache *grd.Cache) *Stage {
	return &Stage{
		cache:          cache,
		canvas:         bishopgfx.NewBgraCanvas(FrameWidth, FrameHeight),
		dirty:          true,
		composed:       make(map[string]*grd.Image, 8),
		composedBudget: 6,
	}
}

// SetBackground 切背景；空串表示黑屏。
func (s *Stage) SetBackground(name string) {
	s.background = name
	s.dirty = true
}

// Background 返回当前背景名。
func (s *Stage) Background() string { return s.background }

// SetCG 设置/清除全屏 CG。
func (s *Stage) SetCG(name string) {
	s.cg = name
	s.dirty = true
}

// CG 返回当前全屏 CG 名。
func (s *Stage) CG() string { return s.cg }

// ClearChars 清空所有立绘槽。
func (s *Stage) ClearChars() {
	for i := range s.chars {
		s.chars[i] = CharSlot{}
	}
	s.dirty = true
}

// SetChar 设置第 idx 个立绘槽；name 为空表示清空。
func (s *Stage) SetChar(idx int, name string, x, y int, scale float64) {
	if idx < 0 || idx >= CharSlots {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	if name == "" || name == "-" {
		s.chars[idx] = CharSlot{}
	} else {
		s.chars[idx] = CharSlot{Name: name, X: x, Y: y, Scale: scale, Alpha: 255}
	}
	s.dirty = true
}

// Chars 返回槽位快照。
func (s *Stage) Chars() [CharSlots]CharSlot {
	return s.chars
}

// HideChar 清空名字匹配的槽（立绘按“角色组”替换，不是按槽号）。
func (s *Stage) HideChar(name string) {
	for i := range s.chars {
		if s.chars[i].Name == name {
			s.chars[i] = CharSlot{}
		}
	}
	s.dirty = true
}

// Fade 记录纯色覆盖（0..255）。
func (s *Stage) Fade(color string, alpha int) {
	s.fadeColor = color
	s.fadeAlpha = clampInt(alpha, 0, 255)
	s.dirty = true
}

// FadeState 返回当前的覆盖色与透明度。
func (s *Stage) FadeState() (string, int) { return s.fadeColor, s.fadeAlpha }

// SetFilter 记录滤镜名。
func (s *Stage) SetFilter(name string) { s.filter = name; s.dirty = true }

// Filter 返回滤镜名。
func (s *Stage) Filter() string { return s.filter }

// SetMacroTable 注入部件宏（fm_XXX → 部件名列表）。
func (s *Stage) SetMacroTable(table map[string][]string) {
	s.mu.Lock()
	s.macro = table
	s.mu.Unlock()
}

// BeginText 清空本轮文字收集（每帧 Compose 前调用）。
func (s *Stage) BeginText() { s.text = s.text[:0] }

// AppendText 追加一段文字（引擎坐标系，Y 是基线）。
//
// 文字不进像素缓冲：汉字字形由宿主用平台字体栅格，引擎只给坐标与样式。
// 这也让“逐字显示/高亮”只改文字，不必重画位图。
func (s *Stage) AppendText(run core.TextRun) { s.text = append(s.text, run) }

// TakeTextRuns 取走本轮收集到的文字（取走即清空）。
//
// 取走而不是读取：同一次合成里对话框与菜单可能各自追加文字，
// 但宿主每帧只该收到一份完整列表。
func (s *Stage) TakeTextRuns() []core.TextRun {
	if len(s.text) == 0 {
		return nil
	}
	out := make([]core.TextRun, len(s.text))
	copy(out, s.text)
	s.text = s.text[:0]
	return out
}

// Compose 合成一帧到 dst。
//
// 每次都从背景重画整帧：GRD 命中缓存后单层贴图是纯内存拷贝，一帧四层合成
// 只有几毫秒，比维护脏矩形更不容易出现“上一帧残留立绘”。
func (s *Stage) Compose(dst []byte) bool {
	if len(dst) < FrameWidth*FrameHeight*4 {
		return false
	}
	c := s.canvas
	c.Clear(0, 0, 0, 0xFF)

	if s.background != "" {
		if img := s.resolveImage(s.background); img != nil {
			c.Blit(imageOf(img), img.OffsetX, img.OffsetY, 255)
		}
	}
	for i := range s.chars {
		if s.chars[i].Name == "" {
			continue
		}
		s.drawChar(c, &s.chars[i])
	}
	// 全屏 CG 必须画在立绘之上。
	//
	// 原版的层序是 背景 → 立绘 → 全屏 CG：事件 CG（fm_A01…/fm_B01…）是
	// 1280x720 的整屏图，出现时整幅盖住立绘。这里早期把 CG 画在立绘之前，
	// 于是 "已经在显示 HCG，角色立绘却还压在上面"——立绘永远不消失。
	// 注意这只是"绘制顺序"，不是清空槽位：CG 结束后的 `bl` + 背景记录会让
	// 场景复原，立绘槽仍然保留（原版同样不会因为 CG 而丢失立绘状态）。
	if s.cg != "" {
		if img := s.resolveImage(s.cg); img != nil {
			x, y := 0, 0
			if img.Width < FrameWidth {
				x = (FrameWidth - img.Width) / 2
			}
			if img.Height < FrameHeight {
				y = (FrameHeight - img.Height) / 2
			}
			c.Blit(imageOf(img), x, y, 255)
		}
	}
	if s.fadeAlpha > 0 {
		b, g, r := parseColor(s.fadeColor)
		c.Fill(0, 0, FrameWidth, FrameHeight, b, g, r, byte(s.fadeAlpha))
	}
	copy(dst, c.Buffer)
	s.dirty = false
	return true
}

// Dirty 报告舞台自上次合成后是否变化。
func (s *Stage) Dirty() bool { return s.dirty }

// MarkDirty 由 session 在改动层数据后调用。
func (s *Stage) MarkDirty() { s.dirty = true }

// TextRuns 返回最近一次 Compose 收集到的文字（core.TextRunReporter 用）。
func (s *Stage) TextRuns() []core.TextRun {
	if len(s.runs) == 0 {
		return nil
	}
	out := make([]core.TextRun, len(s.runs))
	copy(out, s.runs)
	return out
}

// SetRuns 保存本次要交给宿主的文字（由 session 在 Compose 末尾调用）。
func (s *Stage) SetRuns(runs []core.TextRun) { s.runs = runs }

// ResolveImage 是 resolveImage 的公开版本（鉴赏合成要用）。
func (s *Stage) ResolveImage(name string) *grd.Image { return s.resolveImage(name) }

// MergeImages 把 top 按 alpha 叠到 base（鉴赏合成用）。
//
// 与部件合成不同，这里的两张图坐标系一致（都是“屏幕左上角”），
// 不需要按 Offset 平移；直接逐像素混合即可。
func MergeImages(base, top *grd.Image) *grd.Image {
	if base == nil {
		return top
	}
	if top == nil || top.Width <= 0 || top.Height <= 0 {
		return base
	}
	out := &grd.Image{
		Width: base.Width, Height: base.Height,
		CanvasW: base.CanvasW, CanvasH: base.CanvasH,
		Pixels: make([]byte, len(base.Pixels)),
	}
	copy(out.Pixels, base.Pixels)
	blitOnto(out.Pixels, out.Width, out.Height, top.Pixels, top.Width, top.Height, 0, 0)
	return out
}

// imageOf 把 grd.Image 转成 bishop 的 BsgImage（同一种 BGRA 布局）。
func imageOf(img *grd.Image) *bishopgfx.BsgImage {
	return bishopgfx.NewBsgImage(img.Width, img.Height, img.OffsetX, img.OffsetY, img.Pixels)
}

// resolveImage 按名取图；名字可能是部件宏，需要拼合。
func (s *Stage) resolveImage(name string) *grd.Image {
	if s.cache == nil || name == "" {
		return nil
	}
	if img := s.cache.Get(name); img != nil {
		return img
	}
	parts := s.macroParts(name)
	if len(parts) == 0 {
		return nil
	}
	first := s.cache.Get(parts[0])
	if first == nil {
		return nil
	}
	if len(parts) == 1 {
		return first
	}
	s.mu.Lock()
	if img, ok := s.composed[name]; ok {
		s.mu.Unlock()
		return img
	}
	s.mu.Unlock()
	// 部件合成必须发生在“整张虚拟屏幕”空间里，而不是以第一张部件为底：
	// 表情差分部件通常只有几百像素、位置在脸上，用第一张的尺寸做画布会把
	// 它裁掉。这里取所有部件的虚拟屏幕（同组部件一致），按各自的矩形贴。
	composite := &grd.Image{
		Width:   first.CanvasW,
		Height:  first.CanvasH,
		CanvasW: first.CanvasW,
		CanvasH: first.CanvasH,
		Pixels:  make([]byte, first.CanvasW*first.CanvasH*4),
	}
	for _, part := range parts {
		img := s.cache.Get(part)
		if img == nil || img.Width <= 0 || img.Height <= 0 {
			continue
		}
		blitOnto(composite.Pixels, composite.Width, composite.Height,
			img.Pixels, img.Width, img.Height, img.OffsetX, img.OffsetY)
	}
	s.mu.Lock()
	s.composed[name] = composite
	s.order = append(s.order, name)
	for len(s.order) > s.composedBudget {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.composed, oldest)
	}
	s.mu.Unlock()
	return composite
}

func (s *Stage) macroParts(name string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.macro[name]
}

// drawChar 画一个立绘槽。
//
// 定位公式取自原引擎的坐标换算（lune2renpy 的 convert_pos_x/y 是同一套）：
//
//	left = 1280/2 + x - w/2
//	top  = y + h*(1-scale)/2
//
// 即 x 是“水平方向以屏幕中心为原点的偏移”，y 是“图片顶部在 scale=1 时的
// 位置”，缩放以图片中心为锚点（lune2renpy 的 convert_pos_x/y 就是这个
// 含义，Ren'Py 的 ypos 定位的是图片 top）。
//
// 立绘的虚拟屏幕是 1000x2048（GRD 头里的 screenW/H），合成图按这个尺寸
// 生成，部件之间的相对位置直接用各自的 OffsetX/OffsetY。0.68 的全身立绘
// 高 1392px，从 y≈0 起画，玩家看到的是“头顶到腰”的特写，与实机一致。
func (s *Stage) drawChar(c *bishopgfx.BgraCanvas, slot *CharSlot) {
	img := s.resolveImage(slot.Name)
	if img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}
	scale := slot.Scale
	if scale <= 0 {
		scale = 1
	}
	w := int(float64(img.Width) * scale)
	h := int(float64(img.Height) * scale)
	// 缩放以“未缩放图片”的中心为锚点：补偿项用 img.Width/img.Height，
	// 不是缩放后的 w/h。用缩放后尺寸会让立绘整体上移 (1-scale)/2*h。
	left := FrameWidth/2 + slot.X - int(float64(img.Width)*scale/2)
	top := slot.Y + int(float64(img.Height)*(1-scale)/2)
	if w <= 0 || h <= 0 {
		return
	}
	alpha := slot.Alpha
	if alpha <= 0 {
		alpha = 255
	}
	if w == img.Width && h == img.Height {
		c.Blit(imageOf(img), left, top, alpha)
		return
	}
	c.BlitScaled(imageOf(img), left, top, w, h, alpha)
}

// blitOnto 把一个 BGRA 图层按 alpha 叠到目标缓冲（部件合成用）。
func blitOnto(dst []byte, dw, dh int, src []byte, sw, sh, dx, dy int) {
	for y := 0; y < sh; y++ {
		ty := dy + y
		if ty < 0 || ty >= dh {
			continue
		}
		for x := 0; x < sw; x++ {
			tx := dx + x
			if tx < 0 || tx >= dw {
				continue
			}
			si := (y*sw + x) * 4
			sa := int(src[si+3])
			if sa == 0 {
				continue
			}
			di := (ty*dw + tx) * 4
			if sa >= 255 {
				copy(dst[di:di+4], src[si:si+4])
				continue
			}
			for k := 0; k < 3; k++ {
				dst[di+k] = byte((int(src[si+k])*sa + int(dst[di+k])*(255-sa)) / 255)
			}
			dst[di+3] = byte(minInt(255, sa+int(dst[di+3])*(255-sa)/255))
		}
	}
}

// parseColor 解析 "RRGGBB"/"#RRGGBB" 字符串为 BGRA 分量（脚本里的颜色都是 RGB）。
func parseColor(value string) (byte, byte, byte) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) >= 6 {
		if v, ok := hexValue(value[len(value)-6:]); ok {
			// 输入是 RGB（"FF0000" 是纯红），输出是 BGRA 分量 → 低字节是 B。
			return byte(v & 0xFF), byte(v >> 8 & 0xFF), byte(v >> 16 & 0xFF)
		}
	}
	return 0, 0, 0
}

func hexValue(s string) (int, bool) {
	if len(s) != 6 {
		return 0, false
	}
	value := 0
	for i := 0; i < 6; i++ {
		value <<= 4
		switch {
		case s[i] >= '0' && s[i] <= '9':
			value |= int(s[i] - '0')
		case s[i] >= 'a' && s[i] <= 'f':
			value |= int(s[i]-'a') + 10
		case s[i] >= 'A' && s[i] <= 'F':
			value |= int(s[i]-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Describe 给日志用的一句话状态。
func (s *Stage) Describe() string {
	names := make([]string, 0, CharSlots)
	for i := range s.chars {
		if s.chars[i].Name != "" {
			names = append(names, fmt.Sprintf("%d:%s", i, s.chars[i].Name))
		}
	}
	return fmt.Sprintf("bg=%s cg=%s chars=[%s]", s.background, s.cg, strings.Join(names, ","))
}
