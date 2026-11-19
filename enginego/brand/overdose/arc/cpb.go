package arc

import (
	"encoding/binary"
	"fmt"

	"kizuna/enginego/core/textcodec"
)

// CPB 图（overdose_spec §3）是 graphic.arc 顿然栽进来的 2059 张贴图：
//
//	+00 "TYP1"
//	+04 u16 格式（0x20）
//	+06 u16 宽
//	+08 u16 高
//	+0A u32 压缩大小
//	+0E zlib 流 → width*height*4 BGRA
//
// 宽高用 u16 存，1280*720 完全在量程内，不用疑虑超限。
// 压缩大小实际是整图长度对有效载荷的限制，就是后面 zlib 的整个 remaining；
// 显式读出，与否则 zlib 那个“边界到哪为止”拿不准，// 也用 remaining 限定读数即可。
const (
	cpbMagic    = "TYP1"
	cpbHeader   = 0x0E
	cpbFormat32 = 0x20
	cpbFormat24 = 0x18
)

func decodeCP932(b []byte) string {
	return textcodec.DecodeCP932(b)
}

// DecodeCPB 解一帧 BGRA。错错色一点去补位、颠倒 R/B 都会在立绘的皮肤上
// 先露出来——所以只认原 BGRA 约定，其他色度位都原样输出。
func DecodeCPB(b []byte) (int, int, []byte, error) {
	if len(b) < cpbHeader {
		return 0, 0, nil, fmt.Errorf("cpb: 头不足 (%d 字节)", len(b))
	}
	if string(b[0:4]) != cpbMagic {
		return 0, 0, nil, fmt.Errorf("cpb: 魔数不符 %q，期望 TYP1", b[0:4])
	}
	format := binary.LittleEndian.Uint16(b[4:6])
	// 格式字节实际是 24（0x18），像素仍是每点 8 位 B,G,R,A 四通道。
	// 文档写 32（0x20）是按位宽望文生义——以真实解出的 800x600 证据为准，
	// 这里只拦真正的未知值，不用像素数去验证。
	if format != cpbFormat32 && format != cpbFormat24 {
		return 0, 0, nil, fmt.Errorf("cpb: 格式 0x%04x 不识", format)
	}
	w := int(binary.LittleEndian.Uint16(b[6:8]))
	h := int(binary.LittleEndian.Uint16(b[8:10]))
	csz := int(binary.LittleEndian.Uint32(b[10:14]))
	if w <= 0 || h <= 0 {
		return 0, 0, nil, fmt.Errorf("cpb: 宽高无效 %dx%d", w, h)
	}
	if csz <= 0 || csz > len(b)-cpbHeader {
		return 0, 0, nil, fmt.Errorf("cpb: 压缩大小 %d 超出文件界 %d", csz, len(b)-cpbHeader)
	}
	stream := b[cpbHeader : cpbHeader+csz]
	plain, err := inflate(stream)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("cpb: zlib 解压失败 %w", err)
	}
	want := w * h * 4
	if len(plain) != want {
		return 0, 0, nil, fmt.Errorf("cpb: 解压 %d 与 %dx%dx4 不符", len(plain), w, h)
	}
	// BGRA 输出即 plain 本体：copy 防调用方持有切片与锁混进来
	out := make([]byte, len(plain))
	copy(out, plain)
	// 0x18 是“无 alpha”的整屏图（BG/标题/CG），原始第 4 字节常为 0；
	// 不补 255 的话 BgraCanvas.Blit 会把它当全透明，整帧只剩黑底。
	if format == cpbFormat24 {
		for i := 3; i < len(out); i += 4 {
			out[i] = 255
		}
	}
	return w, h, out, nil
}
