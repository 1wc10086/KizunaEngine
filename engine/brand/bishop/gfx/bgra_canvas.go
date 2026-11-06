package gfx

import "encoding/binary"

type BgraCanvas struct {
	Width  int
	Height int
	Buffer []byte
}

func NewBgraCanvas(width, height int) *BgraCanvas {
	if width <= 0 || height <= 0 {
		panic("canvas dimensions must be positive")
	}

	return &BgraCanvas{
		Width:  width,
		Height: height,
		Buffer: make([]byte, width*height*4),
	}
}

func (c *BgraCanvas) Pixels() []byte {
	return c.Buffer
}

func (c *BgraCanvas) Stride() int {
	return c.Width * 4
}

func (c *BgraCanvas) ByteCount() int {
	return len(c.Buffer)
}

func Pack(blue, green, red, alpha byte) uint32 {
	return uint32(blue) | uint32(green)<<8 | uint32(red)<<16 | uint32(alpha)<<24
}

func (c *BgraCanvas) Clear(blue, green, red, alpha byte) {
	pattern := Pack(blue, green, red, alpha)
	for offset := 0; offset+4 <= len(c.Buffer); offset += 4 {
		binary.LittleEndian.PutUint32(c.Buffer[offset:], pattern)
	}
}

func (c *BgraCanvas) Fill(x, y, width, height int, blue, green, red, alpha byte) {
	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+width)
	bottom := min(c.Height, y+height)

	if left >= right || top >= bottom {
		return
	}

	pattern := Pack(blue, green, red, alpha)
	for row := top; row < bottom; row++ {
		offset := ((row * c.Width) + left) * 4
		for column := left; column < right; column++ {
			binary.LittleEndian.PutUint32(c.Buffer[offset:], pattern)
			offset += 4
		}
	}
}

// FillBlend 用给定颜色以 alpha 混合覆盖一块区域。
//
// Fill 是"覆盖"语义（alpha 原样写入），适合清屏/画不透明底板；要给已画好的
// 缩略图压暗必须用混合，否则 176 的 alpha 会把整格写成纯黑——玩家在未解锁
// 的鉴赏页看到的就是一块块黑方块，而不是原版的"看得见轮廓但压暗"。
func (c *BgraCanvas) FillBlend(x, y, width, height int, blue, green, red byte, alpha int) {
	if alpha <= 0 {
		return
	}
	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+width)
	bottom := min(c.Height, y+height)
	if left >= right || top >= bottom {
		return
	}
	alpha = min(alpha, 255)
	for row := top; row < bottom; row++ {
		offset := ((row * c.Width) + left) * 4
		for column := left; column < right; column++ {
			overlayPixel(c.Buffer, offset, blue, green, red, alpha)
			offset += 4
		}
	}
}

// FillMask 用一张单色形状图（白=填充、黑/透明=不填）铺一层半透明颜色。
//
// 原版 Bishop 的窗口衬底不是矩形：message_back / msg_info_back 是 800x132、
// 88x128 的白色圆角 mask，运行时把整块 mask 乘上窗口色（放課後2 是
// 0xa6000000）再混合到画布。用矩形近似会把四个圆角画成直角，所以这里直接
// 按 mask 的 RGB 亮度当覆盖率，边缘才能和原图一致。
//
// color 是 0xAARRGGBB；mask 的 alpha 不可靠（这些素材 alpha 恒为 0），
// 覆盖率只看 RGB。
func (c *BgraCanvas) FillMask(mask *BsgImage, x, y int, color uint32) {
	if mask == nil || c == nil || mask.Width <= 0 || mask.Height <= 0 {
		return
	}
	if len(mask.Pixels) < mask.Width*mask.Height*4 {
		return
	}
	alpha := int((color >> 24) & 0xff)
	if alpha <= 0 {
		return
	}
	blue := byte(color)
	green := byte(color >> 8)
	red := byte(color >> 16)
	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+mask.Width)
	bottom := min(c.Height, y+mask.Height)
	for row := top; row < bottom; row++ {
		maskRow := (row - y) * mask.Width * 4
		offset := ((row * c.Width) + left) * 4
		for column := left; column < right; column++ {
			source := maskRow + (column-x)*4
			// BsgImage 的像素是 BGRA；白色 mask 三通道相等，取蓝通道即可。
			coverage := int(mask.Pixels[source])
			if coverage > 0 {
				if coverage > 255 {
					coverage = 255
				}
				overlayPixel(c.Buffer, offset, blue, green, red, alpha*coverage/255)
			}
			offset += 4
		}
	}
}

func (c *BgraCanvas) Draw(image *BsgImage, x, y int) {
	c.DrawRegion(image, 0, 0, image.Width, image.Height, x, y)
}

func (c *BgraCanvas) DrawRegion(image *BsgImage, sourceX, sourceY, width, height, x, y int) {
	if width <= 0 || height <= 0 {
		return
	}

	sourceWidth := min(width, image.Width-sourceX)
	sourceHeight := min(height, image.Height-sourceY)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(min(c.Width, x+width), x+sourceWidth)
	bottom := min(min(c.Height, y+height), y+sourceHeight)
	if left >= right || top >= bottom {
		return
	}

	source := image.Pixels
	sourceStride := image.Stride()
	target := c.Buffer

	for row := top; row < bottom; row++ {
		sourceOffset := ((sourceY + (row - y)) * sourceStride) + ((sourceX + (left - x)) * 4)
		targetOffset := ((row * c.Width) + left) * 4

		for column := left; column < right; column++ {
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha != 0 {
				if sourceAlpha == 255 {
					target[targetOffset] = source[sourceOffset]
					target[targetOffset+1] = source[sourceOffset+1]
					target[targetOffset+2] = source[sourceOffset+2]
					target[targetOffset+3] = 255
				} else {
					keep := (int(target[targetOffset+3]) * (255 - int(sourceAlpha))) / 255
					alpha := int(sourceAlpha) + keep
					if alpha > 0 {
						target[targetOffset] = byte(((int(source[sourceOffset]) * int(sourceAlpha)) + (int(target[targetOffset]) * keep)) / alpha)
						target[targetOffset+1] = byte(((int(source[sourceOffset+1]) * int(sourceAlpha)) + (int(target[targetOffset+1]) * keep)) / alpha)
						target[targetOffset+2] = byte(((int(source[sourceOffset+2]) * int(sourceAlpha)) + (int(target[targetOffset+2]) * keep)) / alpha)
						target[targetOffset+3] = byte(alpha)
					}
				}
			}

			sourceOffset += 4
			targetOffset += 4
		}
	}
}

// BlitOpaqueRegion 忽略源像素 alpha，整块覆盖到画布。
//
// 旧式 Bishop 素材（GK 的 dm_slot 白色槽位框、message_back 等）把形状写在
// RGB 里、alpha 全为 0，原引擎按"索引值/形状"判断透明度而不是读 alpha。
// 这类图不能走 DrawRegion，否则整块透明。
func (c *BgraCanvas) BlitOpaqueRegion(image *BsgImage, sourceX, sourceY, width, height, x, y int) {
	if image == nil || width <= 0 || height <= 0 {
		return
	}
	sourceWidth := min(width, image.Width-sourceX)
	sourceHeight := min(height, image.Height-sourceY)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}
	left := max(0, x)
	top := max(0, y)
	right := min(min(c.Width, x+width), x+sourceWidth)
	bottom := min(min(c.Height, y+height), y+sourceHeight)
	if left >= right || top >= bottom {
		return
	}
	sourceStride := image.Stride()
	for row := top; row < bottom; row++ {
		sourceOffset := ((sourceY + (row - y)) * sourceStride) + ((sourceX + (left - x)) * 4)
		targetOffset := ((row * c.Width) + left) * 4
		for column := left; column < right; column++ {
			copyPixel(image.Pixels, sourceOffset, c.Buffer, targetOffset)
			c.Buffer[targetOffset+3] = 255
			sourceOffset += 4
			targetOffset += 4
		}
	}
}

func (c *BgraCanvas) Blit(image *BsgImage, x, y, alpha int) {
	c.BlitBand(image, x, y, 0, image.Height, alpha)
}

func (c *BgraCanvas) BlitRegion(image *BsgImage, sourceX, sourceY, width, height, x, y, alpha int) {
	if alpha <= 0 || width <= 0 || height <= 0 {
		return
	}

	sourceWidth := min(width, image.Width-sourceX)
	sourceHeight := min(height, image.Height-sourceY)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(min(c.Width, x+width), x+sourceWidth)
	bottom := min(min(c.Height, y+height), y+sourceHeight)
	if left >= right || top >= bottom {
		return
	}

	source := image.Pixels
	sourceStride := image.Stride()
	target := c.Buffer
	opaque := alpha >= 255

	for row := top; row < bottom; row++ {
		imageRow := sourceY + (row - y)
		sourceOffset := (imageRow * sourceStride) + ((sourceX + (left - x)) * 4)
		targetOffset := ((row * c.Width) + left) * 4

		for column := left; column < right; column++ {
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha != 0 {
				if opaque && sourceAlpha == 255 {
					copyPixel(source, sourceOffset, target, targetOffset)
				} else {
					effective := int(sourceAlpha) * alpha / 255
					if effective > 0 {
						blendPixel(source, sourceOffset, target, targetOffset, effective)
					}
				}
			}

			sourceOffset += 4
			targetOffset += 4
		}
	}
}

func (c *BgraCanvas) BlitBand(image *BsgImage, x, y, sourceY, sourceHeight, alpha int) {
	if alpha <= 0 || sourceHeight <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+image.Width)
	bottom := min(c.Height, y+sourceHeight)
	if left >= right || top >= bottom {
		return
	}

	source := image.Pixels
	sourceStride := image.Stride()
	target := c.Buffer
	opaque := alpha >= 255

	for row := top; row < bottom; row++ {
		imageRow := sourceY + (row - y)
		if imageRow < 0 || imageRow >= image.Height {
			continue
		}

		sourceOffset := (imageRow * sourceStride) + ((left - x) * 4)
		targetOffset := ((row * c.Width) + left) * 4

		for column := left; column < right; column++ {
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha != 0 {
				if opaque && sourceAlpha == 255 {
					copyPixel(source, sourceOffset, target, targetOffset)
				} else {
					effective := int(sourceAlpha) * alpha / 255
					if effective > 0 {
						blendPixel(source, sourceOffset, target, targetOffset, effective)
					}
				}
			}

			sourceOffset += 4
			targetOffset += 4
		}
	}
}

func (c *BgraCanvas) CopyFrom(source *BgraCanvas) {
	copy(c.Buffer, source.Buffer)
}

func (c *BgraCanvas) BlitCanvasRegion(source *BgraCanvas, sourceX, sourceY, width, height, x, y, alpha int) {
	if alpha <= 0 || width <= 0 || height <= 0 {
		return
	}

	sourceWidth := min(width, source.Width-sourceX)
	sourceHeight := min(height, source.Height-sourceY)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(min(c.Width, x+width), x+sourceWidth)
	bottom := min(min(c.Height, y+height), y+sourceHeight)
	if left >= right || top >= bottom {
		return
	}

	blendRows(
		source.Buffer,
		(((sourceY+(top-y))*source.Width)+(sourceX+(left-x)))*4,
		source.Width*4,
		c.Buffer,
		(((top * c.Width) + left) * 4),
		c.Width*4,
		right-left,
		bottom-top,
		alpha)
}

func (c *BgraCanvas) BlitCanvas(source *BgraCanvas, alpha int) {
	if alpha <= 0 || source.Width != c.Width || source.Height != c.Height {
		return
	}

	sourcePixels := source.Buffer
	target := c.Buffer
	opaque := alpha >= 255

	for offset := 0; offset+4 <= len(target); offset += 4 {
		sourceAlpha := sourcePixels[offset+3]
		if sourceAlpha == 0 {
			continue
		}

		if opaque && sourceAlpha == 255 {
			copyPixel(sourcePixels, offset, target, offset)
			continue
		}

		effective := int(sourceAlpha) * alpha / 255
		if effective <= 0 {
			continue
		}

		blendPixel(sourcePixels, offset, target, offset, effective)
	}
}

func (c *BgraCanvas) BlitScaled(image *BsgImage, x, y, width, height, alpha int) {
	if alpha <= 0 || width <= 0 || height <= 0 || image.Width <= 0 || image.Height <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+width)
	bottom := min(c.Height, y+height)
	if left >= right || top >= bottom {
		return
	}

	source := image.Pixels
	sourceStride := image.Stride()
	target := c.Buffer
	opaque := alpha >= 255

	for row := top; row < bottom; row++ {
		sourceRow := int(int64(row-y)*int64(image.Height)/int64(height)) * sourceStride
		targetOffset := ((row * c.Width) + left) * 4

		for column := left; column < right; column++ {
			sourceOffset := sourceRow + int(int64(column-x)*int64(image.Width)/int64(width))*4
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha != 0 {
				if opaque && sourceAlpha == 255 {
					copyPixel(source, sourceOffset, target, targetOffset)
				} else {
					effective := int(sourceAlpha) * alpha / 255
					if effective > 0 {
						blendPixel(source, sourceOffset, target, targetOffset, effective)
					}
				}
			}

			targetOffset += 4
		}
	}
}

// BlitScaledRegion 把一个源矩形缩放后贴到画布。
//
// 与 BlitScaled 的区别是多了源矩形：CS 的 exwindow_number 是 0-9 竖排
// 一条图，必须从指定的帧（源 Y）取数字再缩放；只调用 BlitScaled 会把
// 整条 120px 高的数字条一起压扁。源矩形越界时按图片边界裁剪。
func (c *BgraCanvas) BlitScaledRegion(image *BsgImage, x, y, width, height, alpha, sourceX, sourceY, sourceWidth, sourceHeight int) {
	if image == nil || alpha <= 0 || width <= 0 || height <= 0 || sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}
	if image.Width <= 0 || image.Height <= 0 {
		return
	}
	sourceX = max(0, sourceX)
	sourceY = max(0, sourceY)
	if sourceX >= image.Width || sourceY >= image.Height {
		return
	}
	sourceWidth = min(sourceWidth, image.Width-sourceX)
	sourceHeight = min(sourceHeight, image.Height-sourceY)
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return
	}

	left := max(0, x)
	top := max(0, y)
	right := min(c.Width, x+width)
	bottom := min(c.Height, y+height)
	if left >= right || top >= bottom {
		return
	}

	source := image.Pixels
	sourceStride := image.Stride()
	target := c.Buffer
	opaque := alpha >= 255

	for row := top; row < bottom; row++ {
		sourceRow := (sourceY + int(int64(row-y)*int64(sourceHeight)/int64(height))) * sourceStride
		targetOffset := ((row * c.Width) + left) * 4
		for column := left; column < right; column++ {
			sourceOffset := sourceRow + (sourceX+int(int64(column-x)*int64(sourceWidth)/int64(width)))*4
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha != 0 {
				if opaque && sourceAlpha == 255 {
					copyPixel(source, sourceOffset, target, targetOffset)
				} else {
					effective := int(sourceAlpha) * alpha / 255
					if effective > 0 {
						blendPixel(source, sourceOffset, target, targetOffset, effective)
					}
				}
			}
			targetOffset += 4
		}
	}
}

func (c *BgraCanvas) Overlay(blue, green, red byte, alpha int) {
	if alpha <= 0 {
		return
	}

	overlayRows(c.Buffer, 0, c.Width*4, c.Width, c.Height, blue, green, red, alpha)
}

func BgraCanvasFromImage(image *BsgImage) *BgraCanvas {
	canvas := NewBgraCanvas(image.Width, image.Height)
	copy(canvas.Buffer, image.Pixels)
	return canvas
}

func (c *BgraCanvas) ScaleTo(width, height int) *BgraCanvas {
	width2 := max(1, width)
	height2 := max(1, height)
	target := NewBgraCanvas(width2, height2)
	result := target.Buffer

	for y := 0; y < height2; y++ {
		sourceY0 := y * c.Height / height2
		sourceY1 := max(sourceY0+1, (y+1)*c.Height/height2)
		for x := 0; x < width2; x++ {
			sourceX0 := x * c.Width / width2
			sourceX1 := max(sourceX0+1, (x+1)*c.Width/width2)

			var blue int64
			var green int64
			var red int64
			alpha := 0
			count := 0

			for row := sourceY0; row < sourceY1; row++ {
				offset := ((row * c.Width) + sourceX0) * 4
				for column := sourceX0; column < sourceX1; column++ {
					weight := int(c.Buffer[offset+3])
					blue += int64(c.Buffer[offset]) * int64(weight)
					green += int64(c.Buffer[offset+1]) * int64(weight)
					red += int64(c.Buffer[offset+2]) * int64(weight)
					alpha += weight
					count++
					offset += 4
				}
			}

			if count == 0 {
				count = 1
			}

			destination := ((y * width2) + x) * 4
			if alpha > 0 {
				result[destination] = byte(blue / int64(alpha))
				result[destination+1] = byte(green / int64(alpha))
				result[destination+2] = byte(red / int64(alpha))
			}

			result[destination+3] = byte(alpha / count)
		}
	}

	return target
}

func (c *BgraCanvas) ToImage(offsetX, offsetY int) *BsgImage {
	return NewBsgImage(c.Width, c.Height, offsetX, offsetY, c.Buffer)
}
