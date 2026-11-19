package gfx

// 缩放模式（本包两种尺度语义）。
// 画布固定 800x600（横屏 4:3 整图），背景与 CG 正好同尺寸，不缩放；
// 立绘 600 高、宽度按图而定（tmi_d05 为 334x600），只按 x 坐标摆位，
// 也几乎不缩放。缩放只发生在“目标尺寸 != 源尺寸”的显式调用上，
// 默认走 Bilinear（柔边），点阵/UI 图可显式切 Nearest（保边）。
type ScaleMode int

const (
	// ScaleNearest 最近邻：放大不糊，但锯齿在立绘的发梢先看出来；适合 UI 贴图。
	ScaleNearest ScaleMode = iota
	// ScaleBilinear 双线性：默认。照片类背景/CG 放大更顺，代价是慢一点。
	ScaleBilinear
)

// Clamp 把目标宽高合法化：原引擎的 +0x15 scale 小数来自 INT/1000，
// 若脚本给 0 会把图压成 0 尺寸——这里直接回退到源尺寸而非画空图。
func clampScale(dstW, dstH, srcW, srcH int) (int, int) {
	if dstW <= 0 {
		dstW = srcW
	}
	if dstH <= 0 {
		dstH = srcH
	}
	if dstW <= 0 || dstH <= 0 {
		return srcW, srcH
	}
	return dstW, dstH
}

// ScaleImage 按模式把 img 缩放到 (dstW, dstH) 返回新图；
// dst 与 src 一致时返回原图（不复制，省 1.9MB）。
func ScaleImage(img *Image, dstW, dstH int, mode ScaleMode) *Image {
	if img == nil || img.Width <= 0 || img.Height <= 0 || len(img.Pixels) < img.Width*img.Height*4 {
		return nil
	}
	dw, dh := clampScale(dstW, dstH, img.Width, img.Height)
	if dw == img.Width && dh == img.Height {
		return img
	}
	if mode == ScaleBilinear {
		return scaleBilinear(img, dw, dh)
	}
	return scaleNearest(img, dw, dh)
}

func scaleNearest(img *Image, dw, dh int) *Image {
	src := img.Pixels
	sw, sh := img.Width, img.Height
	out := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		sy := y * sh / dh
		if sy >= sh {
			sy = sh - 1
		}
		row := sy * sw * 4
		dstRow := y * dw * 4
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			if sx >= sw {
				sx = sw - 1
			}
			so := row + sx*4
			do := dstRow + x*4
			copy(out[do:do+4], src[so:so+4])
		}
	}
	return NewImage(dw, dh, out)
}

// 双线性采样按“对齐中心”算源坐标（(x+0.5)*src/dst-0.5），
// 否则缩小图会整体偏右半像素在原引擎里不出现。
func scaleBilinear(img *Image, dw, dh int) *Image {
	src := img.Pixels
	sw, sh := img.Width, img.Height
	out := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(fy)
		if y0 < 0 {
			y0 = 0
		}
		if y0 >= sh-1 {
			y0 = sh - 1
		}
		ty := int((fy - float64(y0)) * 256)
		if ty < 0 {
			ty = 0
		}
		if ty > 256 {
			ty = 256
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
			ty = 0
		}
		for x := 0; x < dw; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(fx)
			if x0 < 0 {
				x0 = 0
			}
			if x0 >= sw-1 {
				x0 = sw - 1
			}
			tx := int((fx - float64(x0)) * 256)
			if tx < 0 {
				tx = 0
			}
			if tx > 256 {
				tx = 256
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
				tx = 0
			}
			do := (y*dw + x) * 4
			for ch := 0; ch < 4; ch++ {
				c00 := int(src[(y0*sw+x0)*4+ch])
				c10 := int(src[(y0*sw+x1)*4+ch])
				c01 := int(src[(y1*sw+x0)*4+ch])
				c11 := int(src[(y1*sw+x1)*4+ch])
				top := c00*(256-tx) + c10*tx
				bot := c01*(256-tx) + c11*tx
				out[do+ch] = byte((top*(256-ty) + bot*ty + 32768) >> 16)
			}
		}
	}
	return NewImage(dw, dh, out)
}

// AspectFit 计算“保持宽高比地装进 (boxW, boxH)”的目标尺寸。
// 背景/CG 本就是 800x600 精确对应，此函数只供非 4:3 的图（立绘缩放到 600 高时用不到，
// 但图集缩略图会用到，避免压扁）。
func AspectFit(srcW, srcH, boxW, boxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 || boxW <= 0 || boxH <= 0 {
		return srcW, srcH
	}
	// 用交叉乘避免浮点：srcW/boxW vs srcH/boxH
	if int64(srcW)*int64(boxH) > int64(srcH)*int64(boxW) {
		// 视频宽高比更宽 → 按宽铺满
		return boxW, boxW * srcH / srcW
	}
	return boxH * srcW / srcH, boxH
}
