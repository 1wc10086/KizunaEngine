package gfx

import "kizuna/engine/core/binutil"

// ReadBsgIndexedPixels 解出 8 位索引图的调色板下标（屏幕坐标，已上下翻转）。
//
// map_hit 这类掩码图的值不是颜色，而是"地点号 - 1"。按 BGRA 解码后调色板
// 里三个通道可能重复（alpha 甚至全是 0），无法可靠还原下标；所以判定地图
// 掩码时必须直接读原始索引，而不是从像素颜色反推。
//
// 只服务索引图：签名/尺寸/调色板偏移同样走 ParseBsgHeader，压缩格式与
// DecodeBsgImage 保持一致。
func ReadBsgIndexedPixels(data []byte) (width, height int, indices []byte, ok bool) {
	header, err := ParseBsgHeader(data, 0)
	if err != nil || header.ColorMode != BsgColorModeIndexed8 {
		return 0, 0, nil, false
	}
	if header.Width <= 0 || header.Height <= 0 || header.PixelCount() <= 0 {
		return 0, 0, nil, false
	}
	source := data[header.DataOffset : header.DataOffset+header.DataSize]
	indices = make([]byte, header.PixelCount())
	switch header.Compression {
	case BsgCompressionNone:
		copy(indices, source)
	case BsgCompressionLz:
		UnpackBsgLz(source, indices, 0, 1)
	default:
		UnpackBsgRle(source, indices, 0, 1)
	}
	// flipVertical 是 RGBA 像素专用的；索引图按字节翻转同样的行。
	flipIndexedVertical(indices, header.Width, header.Height)
	return header.Width, header.Height, indices, true
}

func flipIndexedVertical(pixels []byte, width, height int) {
	if width <= 0 || height <= 0 || len(pixels) < width*height {
		return
	}
	scratch := make([]byte, width)
	for row := 0; row < height/2; row++ {
		top := row * width
		bottom := (height - 1 - row) * width
		copy(scratch, pixels[top:top+width])
		copy(pixels[top:top+width], pixels[bottom:bottom+width])
		copy(pixels[bottom:bottom+width], scratch)
	}
}

// BsgCompositionPartSizes 只读 BSS-Composition 各部件的宽高，不解码像素。
//
// 立绘的眼部帧数只能从"眼睛条"的实际高度推断（bsxx.cfg 缺 eye pattern 时）。
// ParseBsgComposition 会把每个部件（包括 800x600 的整张脸）完整解码，启动时
// 采样十几张图代价太高；这里只走 BSG 头（0x40 字节）与块长字段。
func BsgCompositionPartSizes(data []byte) ([][2]int, bool) {
	if !IsBsgComposition(data) {
		return nil, false
	}
	count := int(data[BsgPartCountOffset])
	if count <= 0 {
		return nil, false
	}
	offset := BsgCompositionHeaderSize
	result := make([][2]int, 0, count)
	for index := 0; index < count; index++ {
		if !IsBsgGraphics(data, offset) || offset+BsgHeaderSize > len(data) {
			break
		}
		width := int(binutil.U16(data[offset+0x16:]))
		height := int(binutil.U16(data[offset+0x18:]))
		result = append(result, [2]int{width, height})
		block := ReadBsgBlockSize(data, offset)
		if block <= 0 {
			break
		}
		offset += block
	}
	return result, len(result) > 0
}
