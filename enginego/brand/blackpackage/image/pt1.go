// Package image 实现 FFA SYSTEM 的 PT1 图像。
//
// PT1 是 24/32 位 BGR 图，头 0x20 字节：
//
//	type int32 (=0..3), -1, x, y, width, height, packedSize, unpackedSize
//
// 主体是四种历史 LZSS/预测编码的变体；type 3 在主体后还有一段独立的
// alpha 压缩块。这里按 GARbro 的 ImagePT1.cs 逐分支移植，输出统一 RGBA。
package image

import (
	"encoding/binary"
	"fmt"
)

const pt1Header = 0x20

// Image 是解码结果。Pixels 为 RGBA8888，行优先。
type Image struct {
	Width, Height int
	OffsetX       int
	OffsetY       int
	Pixels        []byte
	Type          int
}

// DecodePT1 解码一张 PT1。失败返回 error，调用方应跳过该图而不是让
// 整帧失败（原引擎遇到坏图也是跳层）。
func DecodePT1(data []byte) (img *Image, err error) {
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("blackpackage: PT1 解码越界: %v", r)
		}
	}()
	if len(data) < pt1Header {
		return nil, fmt.Errorf("blackpackage: PT1 头截断")
	}
	typ := int(int32(binary.LittleEndian.Uint32(data[0:4])))
	if typ < 0 || typ > 3 {
		return nil, fmt.Errorf("blackpackage: 未支持 PT1 type %d", typ)
	}
	if int32(binary.LittleEndian.Uint32(data[4:8])) != -1 {
		return nil, fmt.Errorf("blackpackage: PT1 保留字段非 -1")
	}
	ox := int(int32(binary.LittleEndian.Uint32(data[8:12])))
	oy := int(int32(binary.LittleEndian.Uint32(data[12:16])))
	w := int(binary.LittleEndian.Uint32(data[16:20]))
	h := int(binary.LittleEndian.Uint32(data[20:24]))
	packed := int(int32(binary.LittleEndian.Uint32(data[24:28])))
	unpacked := int(int32(binary.LittleEndian.Uint32(data[28:32])))
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("blackpackage: PT1 尺寸非法 %dx%d", w, h)
	}
	if unpacked != w*h*3 {
		return nil, fmt.Errorf("blackpackage: PT1 解压长度不符 %d != %d", unpacked, w*h*3)
	}
	if packed <= 0 || pt1Header+packed > len(data) {
		return nil, fmt.Errorf("blackpackage: PT1 压缩长度非法")
	}
	input := make([]byte, packed+8)
	copy(input, data[pt1Header:pt1Header+packed])
	raw := make([]byte, unpacked)
	var alpha []byte
	if typ == 3 {
		alphaAt := pt1Header + packed
		if alphaAt+4 > len(data) {
			return nil, fmt.Errorf("blackpackage: PT1 alpha 头截断")
		}
		asz := int(int32(binary.LittleEndian.Uint32(data[alphaAt : alphaAt+4])))
		if asz < 0 || alphaAt+4+asz > len(data) {
			return nil, fmt.Errorf("blackpackage: PT1 alpha 长度非法")
		}
		alpha = make([]byte, w*h)
		unpackV0(data[alphaAt+4:alphaAt+4+asz], alpha)
	}
	switch typ {
	case 0:
		unpackV0(input, raw)
	case 1:
		unpackV1(input, raw)
	case 2:
		unpackV2(input, raw, w, h)
	case 3:
		unpackV2(input, raw, w, h)
	}
	out := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		b, g, r := raw[i*3], raw[i*3+1], raw[i*3+2]
		a := byte(255)
		if alpha != nil {
			a = alpha[i]
		}
		out[i*4] = r
		out[i*4+1] = g
		out[i*4+2] = b
		out[i*4+3] = a
	}
	return &Image{Width: w, Height: h, OffsetX: ox, OffsetY: oy, Pixels: out, Type: typ}, nil
}

func lzssFrame() []byte {
	frame := make([]byte, 0x1000)
	fill := 0
	for al := 0; al < 0x100; al++ {
		for ecx := 0x0d; ecx > 0; ecx-- {
			frame[fill] = byte(al)
			fill++
		}
	}
	for al := 0; al < 0x100; al++ {
		frame[fill] = byte(al)
		fill++
	}
	for al := 0xff; al >= 0; al-- {
		frame[fill] = byte(al)
		fill++
	}
	for ecx := 0x80; ecx > 0; ecx-- {
		frame[fill] = 0
		fill++
	}
	for ecx := 0x6e; ecx > 0; ecx-- {
		frame[fill] = 0x20
		fill++
	}
	for ecx := 0x12; ecx > 0; ecx-- {
		frame[fill] = 0
		fill++
	}
	return frame
}

func unpackV0(input, output []byte) {
	frame := lzssFrame()
	ebp := 0xfee
	dst, src := 0, 0
	for src < len(input) {
		ah := input[src]
		src++
		for mask := 1; mask != 0x100; mask <<= 1 {
			if dst >= len(output) {
				return
			}
			if ah&byte(mask) != 0 {
				if src >= len(input) {
					return
				}
				al := input[src]
				src++
				frame[ebp] = al
				ebp = (ebp + 1) & 0xfff
				output[dst] = al
				dst++
			} else {
				if src+1 >= len(input) {
					return
				}
				offset := int(input[src])
				count := int(input[src+1])
				src += 2
				offset |= (count & 0xf0) << 4
				count = (count & 0x0f) + 3
				for i := 0; i < count; i++ {
					if dst >= len(output) {
						return
					}
					al := frame[offset]
					offset = (offset + 1) & 0xfff
					frame[ebp] = al
					ebp = (ebp + 1) & 0xfff
					output[dst] = al
					dst++
				}
			}
		}
	}
}

func unpackV1(input, output []byte) {
	frame := lzssFrame()
	ebp := 0xfee
	dst, src := 0, 0
	for src < len(input) {
		ah := input[src]
		src++
		for mask := 1; mask != 0x100; mask <<= 1 {
			if dst >= len(output) {
				return
			}
			if ah&byte(mask) != 0 {
				if src >= len(input) {
					return
				}
				al := input[src]
				src++
				frame[ebp] = al
				ebp = (ebp + 1) & 0xfff
				for i := 0; i < 3; i++ {
					if dst >= len(output) {
						return
					}
					output[dst] = al
					dst++
				}
			} else {
				if src+1 >= len(input) {
					return
				}
				offset := int(input[src])
				count := int(input[src+1])
				src += 2
				offset |= (count & 0xf0) << 4
				count = (count & 0x0f) + 3
				for i := 0; i < count; i++ {
					al := frame[offset]
					offset = (offset + 1) & 0xfff
					frame[ebp] = al
					ebp = (ebp + 1) & 0xfff
					for k := 0; k < 3; k++ {
						if dst >= len(output) {
							return
						}
						output[dst] = al
						dst++
					}
				}
			}
		}
	}
}

// v2Reader 复刻 GARbro UnpackV2 的位流：一个 32 位 edx 寄存器、ch 位计数，
// ReadNext 从输入补位。注意字节序和“cl>>3”步进与 C# 完全一致。
type v2Reader struct {
	in     []byte
	out    []byte
	src    int
	dst    int
	edx    uint32
	ch     byte
	width  int
	height int
	stride int
}

func unpackV2(input, output []byte, w, h int) {
	r := &v2Reader{in: input, out: output, width: w, height: h, stride: w * 3}
	r.unpack()
}

func (r *v2Reader) unpack() {
	r.dst = 0
	if len(r.in) < 3 {
		return
	}
	copy(r.out[0:3], r.in[0:3])
	r.src = 3
	r.dst = 3
	if r.src+4 <= len(r.in) {
		r.edx = binary.LittleEndian.Uint32(r.in[r.src : r.src+4])
	}
	r.src += 3
	r.ch = 0x18
	for i := 1; i < r.width; i++ {
		r.readNext()
		if r.edx&1 != 0 {
			r.edx >>= 1
			r.ch--
			r.copyFrom(r.dst-3, 3)
		} else {
			r.edx >>= 1
			r.ch -= 2
			if r.edx&1 != 0 {
				r.edx >>= 1
				// 三个通道各自预测。注意第一通道前没有 ReadNext：
				// 0x4200ff 分支直接 call 预测子，循环头已经补过位。
				ah := r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
				r.dst++
			} else {
				r.edx >>= 1
				r.readNext()
				r.out[r.dst] = byte(r.edx)
				r.out[r.dst+1] = byte(r.edx >> 8)
				r.out[r.dst+2] = byte(r.edx >> 16)
				r.dst += 3
				r.edx >>= 24
				r.ch -= 24
			}
		}
	}
	for i := 1; i < r.height; i++ {
		r.readNext()
		if r.edx&1 != 0 {
			r.edx >>= 1
			r.ch--
			r.copyFrom(r.dst-r.stride, 3)
		} else {
			r.edx >>= 1
			r.ch -= 2
			if r.edx&1 != 0 {
				r.edx >>= 1
				ah := r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-r.stride]))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-r.stride]))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-r.stride]))
				r.dst++
			} else {
				r.edx >>= 1
				r.readNext()
				r.out[r.dst] = byte(r.edx)
				r.out[r.dst+1] = byte(r.edx >> 8)
				r.out[r.dst+2] = byte(r.edx >> 16)
				r.dst += 3
				r.edx >>= 24
				r.ch -= 24
			}
		}
		for j := 1; j < r.width; j++ {
			r.readNext()
			if r.edx&1 != 0 {
				r.edx >>= 1
				r.ch--
				ah := r.sub4225EA()
				up := r.dst - r.stride
				r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-3]) + int(r.out[up]) + int(ah))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-2]) + int(r.out[up+1]) + int(ah))
				r.dst++
				r.readNext()
				ah = r.sub4225EA()
				r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-1]) + int(r.out[up+2]) + int(ah))
				r.dst++
			} else {
				r.edx >>= 1
				if r.edx&1 != 0 {
					r.edx >>= 1
					r.ch -= 2
					up := r.dst - r.stride
					r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-3]) + int(r.out[up]))
					r.dst++
					r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-2]) + int(r.out[up+1]))
					r.dst++
					r.out[r.dst] = byte(int(r.out[r.dst-3]) - int(r.out[up-1]) + int(r.out[up+2]))
					r.dst++
				} else {
					r.edx >>= 1
					r.ch -= 2
					ebx := int(r.edx & 3)
					// 低 2 位只是探测：3/2/1 才消耗它们；为 0 时原版
					// (0x4204da) 直接从同一位置取 4 位选择子，ch -= 6。
					if ebx != 0 {
						r.edx >>= 2
						r.ch -= 2
					}
					switch ebx {
					case 3:
						r.copyFrom(r.dst-3, 3)
					case 2:
						r.readNext()
						r.out[r.dst] = byte(r.edx)
						r.out[r.dst+1] = byte(r.edx >> 8)
						r.out[r.dst+2] = byte(r.edx >> 16)
						r.dst += 3
						r.edx >>= 24
						r.ch -= 24
					case 1:
						ah := r.sub4225EA()
						r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
						r.dst++
						r.readNext()
						ah = r.sub4225EA()
						r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
						r.dst++
						r.readNext()
						ah = r.sub4225EA()
						r.out[r.dst] = byte(int(ah) + int(r.out[r.dst-3]))
						r.dst++
					default:
						ebx = int(r.edx & 0xf)
						r.edx >>= 4
						r.ch -= 4
						up := r.dst - r.stride
						switch ebx {
						case 0:
							r.copyFrom(r.dst-r.stride-3, 3)
						case 8:
							r.copyFrom(r.dst-r.stride, 3)
						default:
							off := up
							if ebx == 4 {
								off -= 3
							}
							ah := r.sub4225EA()
							r.out[r.dst] = byte(int(ah) + int(r.out[off]))
							off++
							r.dst++
							r.readNext()
							ah = r.sub4225EA()
							r.out[r.dst] = byte(int(ah) + int(r.out[off]))
							off++
							r.dst++
							r.readNext()
							ah = r.sub4225EA()
							r.out[r.dst] = byte(int(ah) + int(r.out[off]))
							off++
							r.dst++
						}
					}
				}
			}
		}
	}
}

func (r *v2Reader) copyFrom(off, n int) {
	if off < 0 {
		return
	}
	for i := 0; i < n && r.dst < len(r.out); i++ {
		r.out[r.dst] = r.out[off+i]
		r.dst++
	}
}

func (r *v2Reader) readNext() {
	cl := byte(32 - r.ch)
	r.edx &= 0xFFFFFFFF >> cl
	if r.src+4 <= len(r.in) {
		r.edx += binary.LittleEndian.Uint32(r.in[r.src:r.src+4]) << (r.ch & 31)
	}
	r.src += int(cl) >> 3
	r.ch += cl & 0xf8
}

func (r *v2Reader) sub4225EA() int8 {
	if r.edx&1 != 0 {
		r.edx >>= 1
		r.ch--
		return 0
	}
	r.edx >>= 1
	// 原版先只看低 2 位：只有 1/2 时才把它消耗掉；为 0 时这 2 位
	// 属于后面更大的码字（见 0x4206c3 之后的 switch），不能再 >>2。
	bits := r.edx & 3
	if bits == 2 {
		r.edx >>= 2
		r.ch -= 3
		return -1
	}
	if bits == 1 {
		r.edx >>= 2
		r.ch -= 3
		return 1
	}
	switch r.edx & 7 {
	case 7:
		r.edx >>= 3
		r.ch -= 4
		return -2
	case 3:
		r.edx >>= 3
		r.ch -= 4
		return 2
	case 4:
		r.edx >>= 3
		r.ch -= 4
		return -3
	}
	switch r.edx & 0x3f {
	case 0x38:
		r.edx >>= 6
		r.ch -= 7
		return 3
	case 0x18:
		r.edx >>= 6
		r.ch -= 7
		return -4
	case 0x28:
		r.edx >>= 6
		r.ch -= 7
		return 4
	case 0x08:
		r.edx >>= 6
		r.ch -= 7
		return -5
	case 0x30:
		r.edx >>= 6
		r.ch -= 7
		return 5
	case 0x10:
		r.edx >>= 6
		r.ch -= 7
		return -6
	case 0x20:
		r.edx >>= 6
		r.ch -= 7
		return 6
	}
	switch r.edx & 0xff {
	case 0xc0:
		r.edx >>= 8
		r.ch -= 9
		return -7
	case 0x40:
		r.edx >>= 8
		r.ch -= 9
		return 7
	case 0x80:
		r.edx >>= 8
		r.ch -= 9
		return -8
	}
	switch r.edx & 0x3ff {
	case 0x300:
		r.edx >>= 10
		r.ch -= 11
		return 8
	case 0x100:
		r.edx >>= 10
		r.ch -= 11
		return -9
	case 0x200:
		r.edx >>= 10
		r.ch -= 11
		return 9
	}
	switch r.edx & 0xfff {
	case 0xc00:
		r.edx >>= 12
		r.ch -= 13
		return -10
	case 0x400:
		r.edx >>= 12
		r.ch -= 13
		return 10
	case 0x800:
		r.edx >>= 12
		r.ch -= 13
		return -11
	}
	switch r.edx & 0x3fff {
	case 0x3000:
		r.edx >>= 14
		r.ch -= 15
		return 11
	case 0x1000:
		r.edx >>= 14
		r.ch -= 15
		return -12
	case 0x2000:
		r.edx >>= 14
		r.ch -= 15
		return 12
	}
	r.edx >>= 14
	r.ch -= 15
	return -13
}
