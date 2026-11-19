// Package grd 解码 TMR-Hiro ADV System 的 .GRD 图像。
//
// 头 0x20 字节（小端）：
//
//	u16 format      低字节 1/2 是像素格式（2 = 带偏移的“容器”图），
//	                高字节是压缩类型：1 = 纯 RLE，0xA1 = Huffman+RLE，0xA2 = Huffman+LZ77
//	u16 screenW, screenH   虚拟屏幕尺寸（立绘合并用，不是显示尺寸）
//	u16 bpp         24 或 32
//	u16 left, right, top, bottom   本图在虚拟屏幕里的矩形
//	u32 alphaSize, rSize, gSize, bSize   四个通道各自的压缩块长度
//
// 通道块的顺序是 Alpha（仅 32bpp 且有 alpha 时）、R、G、B，紧跟在头之后。
// 解压后每通道是 width*height 字节，按行从上到下；但显示时原引擎把第 0 行
// 画在画布底部（DirectX DIB 的 bottom-up 约定），所以这里直接翻转成
// 从上到下的顺序，交给上层按普通坐标绘制。
//
// 参考实现：GARbro Legacy/Lune/ArcPACK.cs 的 PackImageDecoder（同一套 Lune/
// TMR-Hiro 引擎），以及 lune2renpy 的 archive.py。两者对 RLE/Huffman/LZ77
// 的位序描述一致，这里是它们的 Go 移植。
package grd

import (
	"encoding/binary"
	"fmt"
)

const headerSize = 0x20

// Huffman 常量：叶子节点 0x00..0xFF，内部节点 0x100..0x1FE，根固定是 0x1FE。
const (
	leafCount  = 0x100
	rootNode   = 0x1FE
	nodeCount  = 0x200
	maxBitRead = 1 << 24
)

// Image 是解码后的 BGRA 图（行从上到下）。
//
// 几何有两层：Width/Height 是本图的矩形尺寸，CanvasW/CanvasH 是它所属的
// 虚拟屏幕（GRD 头里的 screenW/screenH）。立绘的定位是以“虚拟屏幕中心”
// 为原点的（见 stage.drawChar 的推导），所以两个尺寸都必须保留。
type Image struct {
	Width   int
	Height  int
	OffsetX int // 矩形左上角在虚拟屏幕里的坐标
	OffsetY int // 已换算成“距虚拟屏幕底部”的 y（与头里的 bottom 对齐）
	CanvasW int
	CanvasH int
	// Top/Bottom 是 header 原样的矩形上/下沿。界面条带（比虚拟屏幕窄的
	// sys_* 图）在原版里按 Top 直接贴，调用方需要它时用这两个字段。
	Top    int
	Bottom int
	Pixels []byte // BGRA
}

// Meta 是图像头里的几何信息。
type Meta struct {
	Format   int
	ScreenW  int
	ScreenH  int
	BPP      int
	Left     int
	Right    int
	Top      int
	Bottom   int
	AlphaLen int
	RLen     int
	GSize    int
	BSize    int
}

// IsGRD 判断字节是否是 .GRD 头（供归档识别脚本/图像时使用）。
func IsGRD(data []byte) bool {
	if len(data) < headerSize {
		return false
	}
	format := binary.LittleEndian.Uint16(data[0:2])
	if format&0xFF != 1 && format&0xFF != 2 {
		return false
	}
	pack := format >> 8
	if pack != 1 && pack != 0xA1 && pack != 0xA2 {
		return false
	}
	bpp := binary.LittleEndian.Uint16(data[6:8])
	return bpp == 24 || bpp == 32
}

// Decode 解析一张 .GRD。
func Decode(data []byte) (*Image, error) {
	m, err := readMeta(data)
	if err != nil {
		return nil, err
	}
	width := m.Right - m.Left
	height := m.Bottom - m.Top
	if width < 0 {
		width = -width
	}
	if height < 0 {
		height = -height
	}
	if width == 0 || height == 0 {
		return &Image{Width: 0, Height: 0}, nil
	}
	channelLen := width * height
	// 输出固定 BGRA（4 字节/像素）：上层是 bishop 的 BgraCanvas，它只认 BGRA；
	// 24bpp 的图在这里补上 alpha=255，而不是让每个绘制点去分支判断像素宽度。
	pixels := make([]byte, 4*channelLen)

	pos := headerSize
	channels := make([][]byte, 4)
	unpack := func(index, size int) error {
		if size <= 0 {
			channels[index] = make([]byte, channelLen)
			return nil
		}
		if pos+size > len(data) {
			return fmt.Errorf("grd: 通道 %d 越界 (pos=%d size=%d len=%d)", index, pos, size, len(data))
		}
		blk := data[pos : pos+size]
		pos += size
		plane, err := unpackChannel(blk, m.Format>>8, channelLen)
		if err != nil {
			return err
		}
		channels[index] = plane
		return nil
	}

	if m.BPP == 32 && m.AlphaLen > 0 {
		if err := unpack(3, m.AlphaLen); err != nil {
			return nil, err
		}
	}
	if err := unpack(2, m.RLen); err != nil {
		return nil, err
	}
	if err := unpack(1, m.GSize); err != nil {
		return nil, err
	}
	if err := unpack(0, m.BSize); err != nil {
		return nil, err
	}

	// 通道逐个拷进最终像素，同时把 bottom-up 翻成 top-down：源行 h-1-y
	// 对应目标行 y。原引擎与 GARbro 都是这个方向（否则标题图上下颠倒）。
	for y := 0; y < height; y++ {
		srcRow := (height - 1 - y) * width
		dstRow := y * width
		for x := 0; x < width; x++ {
			src := srcRow + x
			dst := (dstRow + x) * 4
			if channels[0] != nil {
				pixels[dst] = channels[0][src]
			}
			if channels[1] != nil {
				pixels[dst+1] = channels[1][src]
			}
			if channels[2] != nil {
				pixels[dst+2] = channels[2][src]
			}
			// 有 alpha 块用它；否则不透明（全透明会让整张图消失）。
			if channels[3] != nil && m.AlphaLen > 0 {
				pixels[dst+3] = channels[3][src]
			} else {
				pixels[dst+3] = 0xFF
			}
		}
	}

	// OffsetY 采用 GARbro 的 `screen_height - bottom`（bottom-up 约定），
	// 立绘部件（1000x2048 的虚拟屏幕）与对话框底图都靠它定位：
	// 眼睛部件 (rect 413,1770) 换算成 159 才落在头部，用 top 会落到腿上。
	//
	// 界面条带（比虚拟屏幕窄的 sys_* 图）例外：原版按 header 的 top 直接贴，
	// 这种位置由调用方用 Image.Top 自行决定（见 session/overlay.go）。
	offsetY := m.ScreenH - m.Bottom
	if offsetY < 0 {
		offsetY = 0
	}
	return &Image{
		Width:   width,
		Height:  height,
		OffsetX: m.Left,
		OffsetY: offsetY,
		CanvasW: m.ScreenW,
		CanvasH: m.ScreenH,
		Pixels:  pixels,
	}, nil
}

// readMeta 解析头部并校验通道长度之和等于文件长度（与原引擎一致），
// 不做整图缓冲，方便上层在解码前先看尺寸。
func readMeta(data []byte) (Meta, error) {
	if len(data) < headerSize {
		return Meta{}, fmt.Errorf("grd: 数据太短 (%d)", len(data))
	}
	m := Meta{
		Format:   int(binary.LittleEndian.Uint16(data[0:2])),
		ScreenW:  int(binary.LittleEndian.Uint16(data[2:4])),
		ScreenH:  int(binary.LittleEndian.Uint16(data[4:6])),
		BPP:      int(binary.LittleEndian.Uint16(data[6:8])),
		Left:     int(binary.LittleEndian.Uint16(data[8:10])),
		Right:    int(binary.LittleEndian.Uint16(data[0x0A:0x0C])),
		Top:      int(binary.LittleEndian.Uint16(data[0x0C:0x0E])),
		Bottom:   int(binary.LittleEndian.Uint16(data[0x0E:0x10])),
		AlphaLen: int(int32(binary.LittleEndian.Uint32(data[0x10:0x14]))),
		RLen:     int(int32(binary.LittleEndian.Uint32(data[0x14:0x18]))),
		GSize:    int(int32(binary.LittleEndian.Uint32(data[0x18:0x1C]))),
		BSize:    int(int32(binary.LittleEndian.Uint32(data[0x1C:0x20]))),
	}
	if m.Format&0xFF != 1 && m.Format&0xFF != 2 {
		return Meta{}, fmt.Errorf("grd: 未知格式 %#x", m.Format)
	}
	pack := m.Format >> 8
	if pack != 1 && pack != 0xA1 && pack != 0xA2 {
		return Meta{}, fmt.Errorf("grd: 未知压缩 %#x", pack)
	}
	if m.BPP != 24 && m.BPP != 32 {
		return Meta{}, fmt.Errorf("grd: 不支持的 bpp %d", m.BPP)
	}
	if m.Left > m.Right {
		m.Left, m.Right = m.Right, m.Left
	}
	if m.Top > m.Bottom {
		m.Top, m.Bottom = m.Bottom, m.Top
	}
	if m.AlphaLen < 0 || m.RLen < 0 || m.GSize < 0 || m.BSize < 0 {
		return Meta{}, fmt.Errorf("grd: 通道长度为负")
	}
	if headerSize+m.AlphaLen+m.RLen+m.GSize+m.BSize != len(data) {
		// 通道长度之和必须精确等于文件长度；打包时按 4 字节对齐的旧版补丁
		// 会出现少量差异，放宽到“不超出”以避免整部作品打不开。
		if headerSize+m.AlphaLen+m.RLen+m.GSize+m.BSize > len(data) {
			return Meta{}, fmt.Errorf("grd: 通道长度之和超出文件 (%d)", len(data))
		}
	}
	return m, nil
}

// Meta 是给上层判断图片尺寸用的公开入口。
func ReadMeta(data []byte) (Meta, error) { return readMeta(data) }

// unpackChannel 按压缩类型解出一个通道。
func unpackChannel(blk []byte, pack int, total int) ([]byte, error) {
	if pack == 1 {
		return unpackRLE(blk, total)
	}
	// 0xA1/0xA2 的块以 0x08 字节的 Huffman 头开头；只有压缩类型是它们时
	// 才走 Huffman，不能“解不开就当 RLE”——那样会把位流当计数读，解出乱码
	// 却没有任何错误可查。
	huf, err := unpackHuffman(blk)
	if err != nil {
		return nil, err
	}
	if pack == 0xA2 {
		return unpackLZ77(huf, total)
	}
	return unpackRLE(huf, total)
}

// unpackRLE 是 GrdReader.UnpackRLE 的移植：
// count > 0x7F → 重复 (count&0x7F) 次同一个字节；count > 0 → 复制 count 个字面量。
func unpackRLE(data []byte, total int) ([]byte, error) {
	out := make([]byte, 0, total)
	i := 0
	for i < len(data) && len(out) < total {
		count := data[i]
		i++
		switch {
		case count > 0x7F:
			count &= 0x7F
			if i >= len(data) {
				return finish(out, total), nil
			}
			v := data[i]
			i++
			repeat := int(count)
			for k := 0; k < repeat && len(out) < total; k++ {
				out = append(out, v)
			}
		case count > 0:
			need := int(count)
			if i+need > len(data) {
				need = len(data) - i
			}
			for k := 0; k < need && len(out) < total; k++ {
				out = append(out, data[i+k])
			}
			i += need
		}
	}
	return finish(out, total), nil
}

func finish(out []byte, total int) []byte {
	if len(out) >= total {
		return out[:total]
	}
	padded := make([]byte, total)
	copy(padded, out)
	return padded
}

// unpackHuffman 解 Huffman 层。
//
// 结构与 GARbro 的 CreateHuffmanTree 完全一致：
//
//	i32 unpackedSize, i32 packedSize,
//	0x100 × u32 频率（按值 0..255）,
//	LSB 位流（每个字节从最低位先读）
//
// 建树用的是“按频率插入有序表”的原始算法，而不是堆：树形必须与压缩端
// 逐位一致，换一种建树顺序会得到另一套码表、解出来全是乱码。
func unpackHuffman(data []byte) ([]byte, error) {
	if len(data) < 8+4*leafCount {
		return nil, fmt.Errorf("grd: Huffman 头太短")
	}
	unpackedSize := int(int32(binary.LittleEndian.Uint32(data[0:4])))
	if unpackedSize < 0 || unpackedSize > maxBitRead {
		return nil, fmt.Errorf("grd: Huffman 解压长度非法 %d", unpackedSize)
	}
	freq := make([]uint32, nodeCount)
	left := make([]int, nodeCount)
	right := make([]int, nodeCount)
	for i := 0; i < leafCount; i++ {
		freq[i] = binary.LittleEndian.Uint32(data[8+4*i : 12+4*i])
		left[i] = -1
		right[i] = -1
	}

	// 有序表（按频率升序，同频按插入顺序靠后），与 C# 的 List + AddNode 等价。
	tree := make([]int, 0, leafCount)
	insert := func(index int) {
		f := freq[index]
		pos := len(tree)
		for i := 0; i < len(tree); i++ {
			if freq[tree[i]] > f {
				pos = i
				break
			}
		}
		tree = append(tree, 0)
		copy(tree[pos+1:], tree[pos:])
		tree[pos] = index
	}
	for i := 0; i < leafCount; i++ {
		insert(i)
	}
	last := leafCount
	for len(tree) > 1 {
		l := tree[0]
		tree = tree[1:]
		r := tree[0]
		tree = tree[1:]
		freq[last] = freq[l] + freq[r]
		left[last] = l
		right[last] = r
		insert(last)
		last++
	}

	bits := newLSBReader(data, 8+4*leafCount)
	out := make([]byte, unpackedSize)
	for i := 0; i < unpackedSize; i++ {
		node := rootNode
		for node > 0xFF {
			bit, ok := bits.next()
			if !ok {
				// 位流提前耗尽：保留已解出的部分，别让一张图把整帧拖死。
				return out, nil
			}
			if bit != 0 {
				node = right[node]
			} else {
				node = left[node]
			}
			if node < 0 || node >= nodeCount {
				return out, nil
			}
		}
		out[i] = byte(node)
	}
	return out, nil
}

// unpackLZ77 是 GARbro 的 UnpackLZ77：
// special = data[8]，数据从偏移 12 开始；字面量= 原文，special+special=字面 special，
// special+offset+count = 回拷贝（offset > special 时减一）。
func unpackLZ77(data []byte, total int) ([]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("grd: LZ77 头太短")
	}
	special := data[8]
	src := 12
	out := make([]byte, 0, total)
	for len(out) < total {
		if src >= len(data) {
			break
		}
		b := data[src]
		src++
		if b != special {
			out = append(out, b)
			continue
		}
		if src >= len(data) {
			break
		}
		offset := data[src]
		src++
		if offset == special {
			out = append(out, offset)
			continue
		}
		if src >= len(data) {
			break
		}
		count := int(data[src])
		src++
		if offset > special {
			offset--
		}
		start := len(out) - int(offset)
		if start < 0 {
			start = 0
		}
		for k := 0; k < count && len(out) < total; k++ {
			// 重叠拷贝：源与目标在同一缓冲区，逐字节复制即可（等价 CopyOverlapped）。
			out = append(out, out[start+k])
		}
	}
	return finish(out, total), nil
}

// lsbReader 是从最低位开始的位流读取器（GARbro 的 LsbBitStream）。
type lsbReader struct {
	data []byte
	pos  int
	cur  byte
	left int
}

func newLSBReader(data []byte, pos int) *lsbReader {
	return &lsbReader{data: data, pos: pos}
}

func (r *lsbReader) next() (int, bool) {
	if r.left == 0 {
		if r.pos >= len(r.data) {
			return 0, false
		}
		r.cur = r.data[r.pos]
		r.pos++
		r.left = 8
	}
	bit := r.cur & 1
	r.cur >>= 1
	r.left--
	return int(bit), true
}

// Cache 是有界 LRU：GRD 一张 1280x720x4 就是 3.5MB，无界会把 1.7G 的
// 归档全摊进内存。立绘由部件合成，命中率靠“同名复用”。
type Cache struct {
	maxBytes int64
	maxItems int
	items    map[string]*Image
	order    []string
	bytes    int64
	loader   Loader
}

// Loader 按名取原始字节（宿主/归档注入）。
type Loader func(name string) ([]byte, error)

// NewCache 建缓存；默认 64MB / 64 张，与 overdose 的 gfx 量级一致。
func NewCache(loader Loader) *Cache {
	return NewCacheBounded(loader, 64<<20, 64)
}

// NewCacheBounded 建带预算的缓存。
func NewCacheBounded(loader Loader, maxBytes int64, maxItems int) *Cache {
	return &Cache{maxBytes: maxBytes, maxItems: maxItems, items: make(map[string]*Image), loader: loader}
}

// Get 取图；取不到返回 nil。
func (c *Cache) Get(name string) *Image {
	if c == nil || name == "" {
		return nil
	}
	if img, ok := c.items[name]; ok {
		c.touch(name)
		return img
	}
	if c.loader == nil {
		return nil
	}
	raw, err := c.loader(name)
	if err != nil || len(raw) == 0 {
		return nil
	}
	img, err := Decode(raw)
	if err != nil || img.Width <= 0 || img.Height <= 0 {
		// 解码失败也要记住，避免每帧重试同一张坏图。
		c.items[name] = nil
		c.order = append(c.order, name)
		return nil
	}
	c.put(name, img)
	return img
}

func (c *Cache) put(name string, img *Image) {
	c.items[name] = img
	c.order = append(c.order, name)
	c.bytes += int64(len(img.Pixels))
	c.evict()
}

// touch 把命中项挪到队尾（LRU 更新）。
func (c *Cache) touch(name string) {
	for i, key := range c.order {
		if key != name {
			continue
		}
		copy(c.order[i:], c.order[i+1:])
		c.order[len(c.order)-1] = name
		return
	}
}

func (c *Cache) evict() {
	for len(c.order) > c.maxItems || c.bytes > c.maxBytes {
		if len(c.order) == 0 {
			return
		}
		oldest := c.order[0]
		c.order = c.order[1:]
		if img, ok := c.items[oldest]; ok {
			if img != nil {
				c.bytes -= int64(len(img.Pixels))
			}
			delete(c.items, oldest)
		}
	}
}

// Purge 清空缓存（切标题/退出时调用）。
func (c *Cache) Purge() {
	c.items = make(map[string]*Image)
	c.order = nil
	c.bytes = 0
}
