// Package stage 是 X0 的软件绘图层。
//
// 原引擎在 24 位 BGR DIB 上做所有合成；脚本通过 op 0x2 建层、0xa3 载图、
// 0xd1 做层间 blit。这里用 RGBA 层保存同样的内容，Compose 时再转给宿主。
// 没有 DIB 的 stride 对齐问题：内部层永远 width*4。
package stage

import (
	"fmt"
	"strings"
	"sync"

	"kizuna/enginego/brand/blackpackage/archive"
	"kizuna/enginego/brand/blackpackage/image"
)

const (
	FrameWidth  = 800
	FrameHeight = 600
	// ScreenLayer 是原版真正显示给玩家的“屏幕表面”。
	//
	// X0 的画面组装是：1 号层是 800x3000 的虚拟画布（5 个 600 高的页），
	// 背景和立绘按页合成；切页时脚本用 `0x7 层1(0,y) → 层1(0,0)` 把目标页
	// 搬到第 0 页，最后用 `0x7 层1(0,0) → 层0(0,0)` 呈现到 0 号桌面层。
	// 玩家真正看到的是层 1 的第 0 页，层 0 只是原版往窗口的一次镜像；
	// 直接渲染第 0 页既等价又少一次拷贝。其余层（画廊 21、存读档窗口、
	// 菜单条 182..255）都是离屏表面，只有被 blit 到屏幕时才应可见——
	// 以前把所有层都叠上去，才会在每一屏透出画廊缩略图和工具栏。
	ScreenLayer = 1
	maxLayers   = 256
)

// Layer 是一个 800x600 上限的 RGBA 层。
type Layer struct {
	W, H    int
	X, Y    int
	Visible bool
	Pix     []byte
}

func newLayer(w, h int) *Layer {
	if w <= 0 || w > 4096 {
		w = FrameWidth
	}
	if h <= 0 || h > 4096 {
		h = FrameHeight
	}
	return &Layer{W: w, H: h, Visible: true, Pix: make([]byte, w*h*4)}
}

// Stage 实现 script.Stage。所有方法在 capi 的逻辑锁下调用，不再加锁；
// 只有图片缓存为跨帧复用加了互斥，避免未来并发合成时数据竞争。
type Stage struct {
	set    *archive.Set
	layers [maxLayers]*Layer

	mu    sync.Mutex
	cache map[string]*image.Image

	chips []chip

	// active 是脚本最近一次 SetActive 的层号，仅用于调试/诊断。
	active int32
	// windowColors 是 op 0x102 设置的颜色（BGRA 打包），0x16 建窗时使用。
	windowColors [256]int32
	created      int
	loads        int
	LoadLog      []string
	// Frame 是最后一次 Redraw 的合成结果（RGBA）。
	Frame []byte
	// titleImage 是标题画面用的独立背景图。标题不属于游戏画布，
	// 画进 1 号层会污染第 0 页，开局后仍残留（表现为标题大图盖在剧情上）。
	titleImage *image.Image
}

// SetTitleImage 设置标题背景；传 nil 表示离开标题。
func (s *Stage) SetTitleImage(name string) {
	if name == "" {
		s.titleImage = nil
		return
	}
	s.titleImage = s.image(name)
}

type chip struct {
	args [16]int32
	n    int
}

// New 建舞台。
func New(set *archive.Set) *Stage {
	return &Stage{set: set, cache: make(map[string]*image.Image), Frame: make([]byte, FrameWidth*FrameHeight*4)}
}

// SetSet 允许 Open 后再挂资源包。
func (s *Stage) SetSet(set *archive.Set) { s.set = set }

// Layer 取层，越界返回 nil。
func (s *Stage) Layer(idx int32) *Layer {
	if idx < 0 || int(idx) >= maxLayers {
		return nil
	}
	return s.layers[idx]
}

// HasLayer 报告某层是否已经建立。session 用它避免在 INIT 建好层之后再
// 建一次空层（会把 INIT 画好的窗口/背景清掉）。
func (s *Stage) HasLayer(idx int32) bool {
	return idx >= 0 && int(idx) < maxLayers && s.layers[idx] != nil
}

// CreateLayer 按脚本参数建层。0x2 的实参（idx 已单独取出）是
// flag, ?, ?, width, height, ?, ? —— 宽高在 args[3]/args[4]。
//
// 尺寸必须照抄脚本：原版 1 号层是 800x3000 的“虚拟画布”（INIT@0x10c0
// 传 800 × 600*5），背景/立绘先载入 y=600*k 的离屏页，再用 0x7 把当前页
// 拷到 0 号桌面层。层高度不足时离屏页不存在，立绘会全部落在同一位置
// 互相叠加、换表情也覆盖不掉旧图。
func (s *Stage) CreateLayer(idx int32, args []int32) {
	if idx < 0 || int(idx) >= maxLayers {
		return
	}
	w, h := FrameWidth, FrameHeight
	if len(args) >= 5 {
		cw, ch := args[3], args[4]
		if cw >= 32 && cw <= 4096 && ch >= 32 && ch <= 4096 {
			w, h = int(cw), int(ch)
		}
	}
	s.layers[idx] = newLayer(w, h)
	s.created++
}

// FreeLayer 释放层。保留 nil 而不是清空像素，合成时自然跳过。
func (s *Stage) FreeLayer(idx int32) {
	if idx >= 0 && int(idx) < maxLayers {
		s.layers[idx] = nil
	}
}

// LayerRect 对应 op 0x0a（0x41b570）：设置窗口的**文字绘制/裁剪矩形**，
// 不改变层表面的大小和位置。
//
// 以前把它当成“移动并 resize 层”，脚本对 0 号桌面层设置文本区
// (32,452,750,136) 时，整个屏幕层被缩成 750x136 并移到 (32,452)，
// 之后所有呈现拷贝都画到错误坐标，画面全黑。
// 引擎的文字由自绘 UI 负责，这里只需保留接口，不修改层。
func (s *Stage) LayerRect(idx int32, x, y, w, h int32) {}

// LoadImage 把 PT1 合成到目标层。函数名允许省略扩展名；找不到时保留旧层，
// 与原版“跳层”一致。
//
// 这里的关键是“合成”而不是“替换”。X0 的画面由脚本按顺序叠进 1 号层的
// 各个页里：背景页、立绘页（底图和同名遮罩分处两页）、消息窗页……最后由
// 脚本把当前页拷到第 0 页呈现。以前每次载入都 resize 整层并覆盖像素，
// 背景会被立绘清掉、遮罩会被当成图片盖上去（角色变成黑剪影）。
//
// 现在的规则：
//   1. 目标坐标用脚本给的 args[3]/args[4]；PT1 头里的偏移不能叠加，
//      否则遮罩（由 0xd1 sel 24 按同一对坐标合成）会和底图错位；
//   2. 图层页在换人/换表情时要能清空，靠 args[2]==1 的“重建”标志实现；
//   3. 立绘遮罩不在载入时套用——它自己也是要画进某页的图片，
//      真正的合成发生在 Stage.maskBlend（0xd1 sel 24）。
func (s *Stage) LoadImage(layer int32, name string, args []int32) {
	if len(s.LoadLog) < 400 {
		s.LoadLog = append(s.LoadLog, name)
	}
	trimmed := strings.TrimSpace(name)
	img := s.image(trimmed)
	if img == nil {
		return
	}
	created := false
	l := s.Layer(layer)
	if l == nil {
		s.CreateLayer(layer, []int32{0, 0, 0, int32(img.Width), int32(img.Height), 0, 0})
		l = s.Layer(layer)
		if l == nil {
			return
		}
		created = true
	}
	// args[3]/args[4] 是图片左上角在层内的落点（含页原点）。
	ox, oy := img.OffsetX, img.OffsetY
	if len(args) >= 5 {
		ox, oy = int(args[3]), int(args[4])
	}
	if created {
		// 自动建出的离屏层本身就以图片偏移为位置，图片在层内从 0,0 开始。
		l.X, l.Y = ox, oy
		ox, oy = 0, 0
	}
	// args[2]==1 表示“重建这一页”。立绘页（y=1200/1800 等）在换人、
	// 换表情时会被反复写入，不先清空就会留下上一张图的残余，表现为多个
	// 角色重叠、或表情变了但旧图不消失。
	if len(args) >= 3 && args[2] == 1 && l.H > FrameHeight {
		band := (oy / FrameHeight) * FrameHeight
		if band >= 0 && band+FrameHeight <= l.H {
			for i := band * l.W * 4; i < (band+FrameHeight)*l.W*4; i++ {
				l.Pix[i] = 0
			}
		}
	}
	s.loads++
	s.blendImage(l, img, nil, ox, oy)
	l.Visible = true
}

// blendImage 按 alpha 把 img 混合到层上的 (ox,oy)。mask 目前恒为 nil
// （遮罩改由 maskBlend 处理），保留参数是为了未来直接贴带 alpha 的图。
func (s *Stage) blendImage(l *Layer, img, mask *image.Image, ox, oy int) {
	useMask := mask != nil && mask.Width == img.Width && mask.Height == img.Height
	for y := 0; y < img.Height; y++ {
		yy := oy + y
		if yy < 0 || yy >= l.H {
			continue
		}
		for x := 0; x < img.Width; x++ {
			xx := ox + x
			if xx < 0 || xx >= l.W {
				continue
			}
			si := (y*img.Width + x) * 4
			a := img.Pixels[si+3]
			if useMask {
				mi := si
				lum := (int(mask.Pixels[mi])*299 + int(mask.Pixels[mi+1])*587 + int(mask.Pixels[mi+2])*114) / 1000
				a = byte(255 - lum)
			}
			if a == 0 {
				continue
			}
			di := (yy*l.W + xx) * 4
			if a == 255 {
				copy(l.Pix[di:di+4], img.Pixels[si:si+4])
				continue
			}
			inv := 255 - int(a)
			l.Pix[di] = byte((int(img.Pixels[si])*int(a) + int(l.Pix[di])*inv) / 255)
			l.Pix[di+1] = byte((int(img.Pixels[si+1])*int(a) + int(l.Pix[di+1])*inv) / 255)
			l.Pix[di+2] = byte((int(img.Pixels[si+2])*int(a) + int(l.Pix[di+2])*inv) / 255)
			l.Pix[di+3] = 255
		}
	}
}

func (s *Stage) image(name string) *image.Image {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || s.set == nil {
		return nil
	}
	s.mu.Lock()
	if img, ok := s.cache[name]; ok {
		s.mu.Unlock()
		return img
	}
	s.mu.Unlock()
	raw, err := s.set.Entry(name)
	if err != nil && !strings.HasSuffix(name, ".pt1") {
		raw, err = s.set.Entry(name + ".pt1")
	}
	if err != nil {
		return nil
	}
	img, err := image.DecodePT1(raw)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	if len(s.cache) > 64 {
		for k := range s.cache {
			delete(s.cache, k)
			break
		}
	}
	s.cache[name] = img
	s.mu.Unlock()
	return img
}

// Fill 清一块矩形为透明黑（原版 0x98 是清屏/填充）。
func (s *Stage) Fill(layer int32, x, y, w, h, color int32) {
	l := s.Layer(layer)
	if l == nil {
		return
	}
	if w <= 0 || h <= 0 {
		w, h = int32(l.W), int32(l.H)
	}
	for yy := int(y); yy < int(y+h) && yy < l.H; yy++ {
		if yy < 0 {
			continue
		}
		for xx := int(x); xx < int(x+w) && xx < l.W; xx++ {
			if xx < 0 {
				continue
			}
			i := (yy*l.W + xx) * 4
			l.Pix[i], l.Pix[i+1], l.Pix[i+2], l.Pix[i+3] = 0, 0, 0, 255
		}
	}
}

// Chip 记录 0x9a 的 16 字段描述。原版把每个 chip 存进 0x4d9300 的
// 0x40 字节表；这里只保留非零参数，Redraw 时按索引顺序绘制。
func (s *Stage) Chip(args []int32) {
	var c chip
	c.n = len(args)
	if c.n > len(c.args) {
		c.n = len(c.args)
	}
	copy(c.args[:], args[:c.n])
	s.chips = append(s.chips, c)
}

// Blit 处理 0xd1 的层间拷贝。选择器 0/1 的参数顺序已由原版反汇编确认：
// blend, src, sx, sy, w, h, dst, dx, dy。其余选择器先按同一套坐标处理，
// 保证背景/立绘至少能出现；混合模式差异在后续版本补齐。
func (s *Stage) Blit(sel int32, args []int32) {
	if len(args) < 6 {
		return
	}
	var src, dst int32
	var sx, sy, w, h, dx, dy int32
	switch sel {
	case 0, 1, 18, 35:
		if len(args) < 9 {
			return
		}
		src, sx, sy, w, h, dst, dx, dy = args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8]
	case 24:
		// 立绘“底图 + 遮罩”合成（原版 0x4066b0 的四表面混合）。
		//
		// 本作把立绘拆成两张图分页装载，再用本选择子合成：
		//   x0bw123.pt1  → 层1 (0,1200)   ← 底图
		//   x0bw123m.pt1 → 层1 (0,1800)   ← 灰度遮罩（白=透明、黑=不透明）
		// 合成结果写到 层1 (0,0)，也就是真正呈现给玩家的第 0 页。
		//
		// 实测 args 的语义是三组 (层, x, y) + 宽高：
		//   [0] 模式  [1..3] 底图(层,x,y)  [4..6] 目标(层,x,y)
		//   [7..9] 遮罩(层,x,y)  [10] w  [11] h  [12..14] 调色(层,x,y)
		if len(args) < 12 {
			return
		}
		src, sx, sy := args[1], args[2], args[3]
		dst, dx, dy := args[4], args[5], args[6]
		mask, mx, my := args[7], args[8], args[9]
		s.maskBlend(dst, dx, dy, src, sx, sy, mask, mx, my, args[10], args[11])
		return
	case 22, 23, 28:
		if len(args) < 12 {
			return
		}
		src, sx, sy, w, h = args[1], args[2], args[3], args[4], args[5]
		dst, dx, dy = args[7], args[8], args[9]
	case 30:
		if len(args) < 9 {
			return
		}
		src, sx, sy, w, h, dst, dx, dy = args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7]
	case 38:
		if len(args) < 6 {
			return
		}
		dst, dx, dy, w, h = args[0], args[1], args[2], args[3], args[4]
		src, sx, sy = dst, dx, dy
	case 40:
		if len(args) < 9 {
			return
		}
		src, sx, sy, w, h, dst, dx, dy = args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7]
	case 6, 7:
		// op 0x6/0x7：src, sx, sy, w, h, dst, dx, dy 的近似。
		if len(args) < 8 {
			return
		}
		src, sx, sy, w, h, dst, dx, dy = args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7]
	default:
		return
	}
	srcL := s.Layer(src)
	dstL := s.Layer(dst)
	if srcL == nil || dstL == nil {
		return
	}
	s.blitRect(srcL, dstL, int(sx), int(sy), int(w), int(h), int(dx), int(dy))
}

// maskBlend 是 0x4066b0 的“底图 + 遮罩”路径：颜色取自 src，alpha 取自
// mask 的灰度反相（白=透明），写到 dst 的对应位置。
//
// 三张表面通常同层不同页（本作都在 1 号层），所以这里按“页内坐标”复制：
// 目标位置 = (dstX + (i - srcX), dstY + (i - srcY))。
func (s *Stage) maskBlend(dst, dx, dy, src, sx, sy, mask, mx, my, w, h int32) {
	dl := s.Layer(dst)
	sl := s.Layer(src)
	ml := s.Layer(mask)
	if dl == nil || sl == nil || ml == nil || w <= 0 || h <= 0 {
		return
	}
	for y := int32(0); y < h; y++ {
		for x := int32(0); x < w; x++ {
			sxx, syy := int(sx)+int(x), int(sy)+int(y)
			mxx, myy := int(mx)+int(x), int(my)+int(y)
			dxx, dyy := int(dx)+int(x), int(dy)+int(y)
			if sxx < 0 || sxx >= sl.W || syy < 0 || syy >= sl.H {
				continue
			}
			if mxx < 0 || mxx >= ml.W || myy < 0 || myy >= ml.H {
				continue
			}
			if dxx < 0 || dxx >= dl.W || dyy < 0 || dyy >= dl.H {
				continue
			}
			si := (syy*sl.W + sxx) * 4
			mi := (myy*ml.W + mxx) * 4
			di := (dyy*dl.W + dxx) * 4
			lum := (int(ml.Pix[mi])*299 + int(ml.Pix[mi+1])*587 + int(ml.Pix[mi+2])*114) / 1000
			a := 255 - lum
			if ml.Pix[mi+3] == 0 {
				a = 0
			}
			if a <= 0 {
				continue
			}
			if a >= 255 {
				copy(dl.Pix[di:di+4], sl.Pix[si:si+4])
				dl.Pix[di+3] = 255
				continue
			}
			inv := 255 - a
			dl.Pix[di] = byte((int(sl.Pix[si])*a + int(dl.Pix[di])*inv) / 255)
			dl.Pix[di+1] = byte((int(sl.Pix[si+1])*a + int(dl.Pix[di+1])*inv) / 255)
			dl.Pix[di+2] = byte((int(sl.Pix[si+2])*a + int(dl.Pix[di+2])*inv) / 255)
			dl.Pix[di+3] = 255
		}
	}
}

func (s *Stage) blitRect(src, dst *Layer, sx, sy, w, h, dx, dy int) {
	if w <= 0 || h <= 0 {
		w, h = src.W-sx, src.H-sy
	}
	for y := 0; y < h; y++ {
		syy := sy + y
		dyy := dy + y
		if syy < 0 || syy >= src.H || dyy < 0 || dyy >= dst.H {
			continue
		}
		for x := 0; x < w; x++ {
			sxx := sx + x
			dxx := dx + x
			if sxx < 0 || sxx >= src.W || dxx < 0 || dxx >= dst.W {
				continue
			}
			si := (syy*src.W + sxx) * 4
			di := (dyy*dst.W + dxx) * 4
			a := src.Pix[si+3]
			if a == 0 {
				continue
			}
			if a == 255 {
				copy(dst.Pix[di:di+4], src.Pix[si:si+4])
				continue
			}
			// 原版是 24-bit 无 alpha，只有 color key。这里做普通 alpha over。
			ia := int(a)
			inv := 255 - ia
			dst.Pix[di] = byte((int(src.Pix[si])*ia + int(dst.Pix[di])*inv) / 255)
			dst.Pix[di+1] = byte((int(src.Pix[si+1])*ia + int(dst.Pix[di+1])*inv) / 255)
			dst.Pix[di+2] = byte((int(src.Pix[si+2])*ia + int(dst.Pix[di+2])*inv) / 255)
			dst.Pix[di+3] = 255
		}
	}
}

// SetActive 记录当前活动层。
func (s *Stage) SetActive(layer int32) { s.active = layer }

// SetWindowColor 记录 op 0x102 的窗口颜色。
func (s *Stage) SetWindowColor(idx, color int32) {
	if idx >= 0 && int(idx) < len(s.windowColors) {
		s.windowColors[idx] = color
	}
}

// Redraw 对应 op 0x9c/0x14。原版把各层合成到屏幕层；我们的 Compose 在
// 宿主取帧时按层顺序直接合成，所以脚本触发的重绘不需要再做一遍整帧
// 合成——演出循环里 0x9c 调用很频繁，重复合成会把播放拖慢一个数量级。
func (s *Stage) Redraw() {}

// drawFullImage 把整张图按原尺寸贴到帧上（标题等全屏画面）。
func (s *Stage) drawFullImage(frame []byte, img *image.Image) {
	w, h := img.Width, img.Height
	if w > FrameWidth {
		w = FrameWidth
	}
	if h > FrameHeight {
		h = FrameHeight
	}
	for y := 0; y < h; y++ {
		copy(frame[y*FrameWidth*4:y*FrameWidth*4+w*4], img.Pixels[y*img.Width*4:y*img.Width*4+w*4])
	}
}

// Compose 只把屏幕层（1 号层）贴到帧上。
//
// 这是原版的真实模型：所有离屏层都要由脚本用 0xd1 显式 blit 到屏幕层，
// 引擎本身不会替脚本“把所有层都画一遍”。以前遍历全部 256 层直接叠加，
// 导致画廊（21）、存读档窗口、菜单条（182..255）等离屏表面常驻在每一屏，
// 玩家会看到大量不该出现的 UI。若屏幕层尚未建立（脚本刚开始初始化），
// 退回 0 号层，避免整帧全黑。
func (s *Stage) Compose() {
	frame := s.Frame
	for i := range frame {
		frame[i] = 0
	}
	idx := int32(ScreenLayer)
	if s.layers[idx] == nil {
		idx = 0
	}
	if s.titleImage != nil {
		s.drawFullImage(frame, s.titleImage)
		return
	}
	l := s.layers[idx]
	if l == nil || !l.Visible {
		return
	}
	// 画布层高 3000（5 页），只有第 0 页是当前显示页，其余页是离屏缓冲。
	if l.H > FrameHeight {
		l = &Layer{W: l.W, H: FrameHeight, X: l.X, Y: l.Y, Visible: l.Visible, Pix: l.Pix[:l.W*FrameHeight*4]}
	}
	for y := 0; y < l.H; y++ {
		yy := l.Y + y
		if yy < 0 || yy >= FrameHeight {
			continue
		}
		for x := 0; x < l.W; x++ {
			xx := l.X + x
			if xx < 0 || xx >= FrameWidth {
				continue
			}
			si := (y*l.W + x) * 4
			a := l.Pix[si+3]
			if a == 0 {
				continue
			}
			di := (yy*FrameWidth + xx) * 4
			if a == 255 {
				copy(frame[di:di+4], l.Pix[si:si+4])
				continue
			}
			inv := 255 - int(a)
			frame[di] = byte((int(l.Pix[si])*int(a) + int(frame[di])*inv) / 255)
			frame[di+1] = byte((int(l.Pix[si+1])*int(a) + int(frame[di+1])*inv) / 255)
			frame[di+2] = byte((int(l.Pix[si+2])*int(a) + int(frame[di+2])*inv) / 255)
			frame[di+3] = 255
		}
	}
}

// Clear 把整帧清黑（session 在打开/切场景时用）。
func (s *Stage) Clear() {
	for i := range s.Frame {
		s.Frame[i] = 0
	}
}

// DebugLayers 返回非空层数量，日志用。
func (s *Stage) DebugLayers() string {
	n := 0
	var b strings.Builder
	for i, l := range s.layers {
		if l == nil {
			continue
		}
		n++
		fmt.Fprintf(&b, " #%d %dx%d@%d,%d vis=%v;", i, l.W, l.H, l.X, l.Y, l.Visible)
	}
	return fmt.Sprintf("layers=%d created=%d loads=%d%s", n, s.created, s.loads, b.String())
}

// LayerCount 返回非空层数量（测试/日志）。
func (s *Stage) LayerCount() int {
	n := 0
	for _, l := range s.layers {
		if l != nil {
			n++
		}
	}
	return n
}

// FillRect 对应 op 0x6/0xa2：用 0xRRGGBB 颜色填充矩形。原版层是 24 位
// BGR，D3D 的颜色字是 0x00RRGGBB；alpha 字节按不透明处理。
func (s *Stage) FillRect(layer, x, y, w, h, color int32) {
	l := s.Layer(layer)
	if l == nil {
		return
	}
	if w <= 0 || h <= 0 {
		return
	}
	r := byte((color >> 16) & 0xff)
	g := byte((color >> 8) & 0xff)
	b := byte(color & 0xff)
	x0, y0 := int(x), int(y)
	for yy := y0; yy < y0+int(h); yy++ {
		if yy < 0 || yy >= l.H {
			continue
		}
		for xx := x0; xx < x0+int(w); xx++ {
			if xx < 0 || xx >= l.W {
				continue
			}
			i := (yy*l.W + xx) * 4
			l.Pix[i], l.Pix[i+1], l.Pix[i+2], l.Pix[i+3] = r, g, b, 255
		}
	}
}

// CopyRect 对应 op 0x7/0xa6：把 src 的矩形复制到 dst。原版 0x404300
// 还支持混合色和 8/24 位表面；这里层统一 RGBA，直接按 alpha 合成。
func (s *Stage) CopyRect(src, dst, sx, sy, w, h, dx, dy int32) {
	sl := s.Layer(src)
	dl := s.Layer(dst)
	if sl == nil || dl == nil {
		return
	}
	if w <= 0 || h <= 0 {
		w, h = int32(sl.W)-sx, int32(sl.H)-sy
	}
	s.blitRect(sl, dl, int(sx), int(sy), int(w), int(h), int(dx), int(dy))
}

// ScreenSnapshot 返回当前合成后的 800x600 RGBA 帧（用于存档缩略画面）。
// 只取画布第 0 页，不包含引擎自绘的消息窗，读档时由宿主重新绘制。
func (s *Stage) ScreenSnapshot() []byte {
	s.Compose()
	out := make([]byte, FrameWidth*FrameHeight*4)
	copy(out, s.Frame)
	return out
}

// RestoreScreen 把存档里的画面写回画布第 0 页。
//
// 为什么需要：原版的画面是脚本逐条绘制累积出来的，而读档只是把变量/IP
// 拨回存档点，脚本不会重画已经画过的东西。从标题画面读档时画布上仍是
// 标题图，就会出现“背景还是主界面”的现象。存档时一并保存画面即可。
func (s *Stage) RestoreScreen(pix []byte) bool {
	if len(pix) < FrameWidth*FrameHeight*4 {
		return false
	}
	l := s.layers[ScreenLayer]
	if l == nil {
		s.CreateLayer(ScreenLayer, []int32{0, 0, 0, FrameWidth, FrameHeight * 5, 0, 0})
		l = s.layers[ScreenLayer]
	}
	if l == nil || l.W < FrameWidth || l.H < FrameHeight {
		return false
	}
	copy(l.Pix[:FrameWidth*FrameHeight*4], pix[:FrameWidth*FrameHeight*4])
	s.titleImage = nil
	return true
}

// LookupImage 暴露解码后的图片给 session 的引擎自绘 UI（缩略图/鉴赏）。
func (s *Stage) LookupImage(name string) *image.Image { return s.image(name) }

// ImageSize 返回图片尺寸。0xa3 需要把载入后占用的矩形回写给脚本
// （脚本随后用 0x7 从这个矩形拷贝到屏幕），所以 VM 必须能拿到 w/h。
func (s *Stage) ImageSize(name string) (int, int) {
	img := s.image(name)
	if img == nil {
		return 0, 0
	}
	return img.Width, img.Height
}
