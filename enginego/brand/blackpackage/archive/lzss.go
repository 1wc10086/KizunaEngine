package archive

import "fmt"

// Decompress 是 FFA SYSTEM 的 LZSS（0x1000 环形字典，初始位置 0xFEE，
// 填充 0）。与 GARbro 的 LzssReader 逐字节一致：
//
//	控制字节低位到高位依次表示 8 个 token；1=字面量，0=offset/len 对，
//	offset = lo | ((hi&0xF0)<<4)，length = 3 + (hi&0x0F)。
//
// SO4 和 WA1 都用这套编码，区别只在 8 字节头之后的负载。
func Decompress(src []byte, outLen int) ([]byte, error) {
	if outLen <= 0 {
		return nil, fmt.Errorf("blackpackage: 非法解压长度 %d", outLen)
	}
	out := make([]byte, 0, outLen)
	frame := make([]byte, 0x1000)
	pos := 0xfee
	i := 0
	for i < len(src) && len(out) < outLen {
		ctl := src[i]
		i++
		for bit := 0; bit < 8; bit++ {
			if len(out) >= outLen {
				break
			}
			if ctl&(1<<uint(bit)) != 0 {
				if i >= len(src) {
					return nil, fmt.Errorf("blackpackage: LZSS 字面量截断")
				}
				b := src[i]
				i++
				frame[pos] = b
				pos = (pos + 1) & 0xfff
				out = append(out, b)
				continue
			}
			if i+1 >= len(src) {
				return nil, fmt.Errorf("blackpackage: LZSS 匹配截断")
			}
			lo := int(src[i])
			hi := int(src[i+1])
			i += 2
			off := ((hi & 0xf0) << 4) | lo
			n := 3 + (hi & 0x0f)
			for k := 0; k < n; k++ {
				if len(out) >= outLen {
					break
				}
				v := frame[off]
				off = (off + 1) & 0xfff
				frame[pos] = v
				pos = (pos + 1) & 0xfff
				out = append(out, v)
			}
		}
	}
	if len(out) != outLen {
		return nil, fmt.Errorf("blackpackage: LZSS 解压长度 %d != %d", len(out), outLen)
	}
	return out, nil
}
