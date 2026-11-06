package runtime

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

// mapButtonHitSuffix 是地图卡命中掩码的命名约定：<图片名>_hit。
//
// KZK、屈辱2、侵蝕、淫獄の放課後2 的卡图都是 1024x768 全屏透明叠层，
// 卡片本体只占其中一小块；没有掩码就只能用整张图的矩形做热区，多张卡
// 会完全重叠。掩码是 8 位索引图，0 表示不可点，非零表示可点。
const mapButtonHitSuffix = "_hit"

// loadMapButtonHitMask 读取按钮图的同名命中掩码。
//
// 只接受 8 位索引图：这些掩码的调色板所有项 alpha 都是 0，
// 按普通 RGBA 解码会得到一张全透明图，索引值才是真正的命中信息
// （与 bsmap 里 Gakuen2 的掩码处理同因）。
// 原始索引行是自下而上的（与原引擎屏幕坐标相反），这里翻成行 0 = 顶部，
// 之后掩码坐标就可以直接和按钮绘制原点做相对运算。
func loadMapButtonHitMask(resources state.ResourceIndex, name string) *state.StageHitMask {
	if resources == nil || len(name) == 0 {
		return nil
	}
	location, ok := resources.TryGet(name + mapButtonHitSuffix)
	if !ok || location == nil {
		return nil
	}
	data := location.Read()
	if len(data) == 0 || !gfx.IsBsgGraphics(data, 0) {
		return nil
	}
	header, err := gfx.ParseBsgHeader(data, 0)
	if err != nil || header.Width <= 0 || header.Height <= 0 {
		return nil
	}
	if header.ColorMode != gfx.BsgColorModeIndexed8 {
		return nil
	}
	buffer := make([]byte, header.PixelCount())
	source := data[header.DataOffset : header.DataOffset+header.DataSize]
	switch header.Compression {
	case gfx.BsgCompressionNone:
		copy(buffer, source[:min(len(source), len(buffer))])
	case gfx.BsgCompressionLz:
		gfx.UnpackBsgLz(source, buffer, 0, 1)
	default:
		gfx.UnpackBsgRle(source, buffer, 0, 1)
	}
	width, height := header.Width, header.Height
	values := make([]byte, width*height)
	for row := 0; row < height; row++ {
		sourceRow := row * width
		targetRow := ((height - 1) - row) * width
		copy(values[targetRow:targetRow+width], buffer[sourceRow:sourceRow+width])
	}
	mask := &state.StageHitMask{Width: width, Height: height, Values: values}
	if _, _, _, _, ok := mask.Bounds(); !ok {
		core.Logf("gfx: map button '%s' hit mask is empty, treated as unclickable", name)
	}
	return mask
}

// applyMapButtonHitMask 把掩码和它的外接矩形挂到按钮上。
//
// 掩码像素坐标是“相对按钮绘制原点”的，所以外接矩形要加上 X/Y 才是
// 屏幕坐标；点击判定用相对坐标（见 Session.hitButton）。
func applyMapButtonHitMask(button *state.StageButton, mask *state.StageHitMask) {
	if button == nil || mask == nil {
		return
	}
	button.HitMask = mask
	if x, y, width, height, ok := mask.Bounds(); ok {
		button.HitX = button.X + x
		button.HitY = button.Y + y
		button.HitWidth = width
		button.HitHeight = height
		return
	}
	button.HitX = button.X
	button.HitY = button.Y
	button.HitWidth = 0
	button.HitHeight = 0
}
