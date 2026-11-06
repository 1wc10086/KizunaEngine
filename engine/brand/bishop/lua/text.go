package lua

import (
	lua "github.com/yuin/gopher-lua"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
)

// font 是 DIBText.CreateFont 返回的字体句柄。
//
// 原引擎用系统 TTF（ＭＳ 明朝/ゴシック）；引擎侧统一用内嵌的
// textrender，字号保留，字形差异在可接受范围内——界面布局不依赖具体字形宽度。
type font struct {
	face string
	size int
}

func (r *Runtime) dibCreateFont(L *lua.LState) int {
	face := L.CheckString(1)
	size := L.CheckInt(2)
	if size <= 0 {
		size = 14
	}
	userdata := L.NewUserData()
	userdata.Value = &font{face: face, size: size}
	L.Push(userdata)
	return 1
}

// DrawText(target, x, y, font, text, color?, scale?)
func (r *Runtime) drawText(L *lua.LState) int {
	target := asDIB(L, 1)
	if target == nil || target.canvas == nil {
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	fontValue, _ := L.Get(4).(*lua.LUserData)
	textFont, _ := fontValue.Value.(*font)
	if textFont == nil {
		return 0
	}
	text := L.CheckString(5)
	color := uint32(L.OptNumber(6, 0))
	if color == 0 {
		// 原 Lua 用 0 表示"默认色"；底图是深色，默认取白。
		color = 0xffffffff
	}
	scale := L.OptNumber(7, 0)
	size := textFont.size
	if scale > 0 {
		size = int(float64(size) * float64(scale))
	}
	if size <= 0 || text == "" || r.host.Fonts() == nil {
		return 0
	}
	image := r.host.Fonts().Render(text, size, 1, 1, size+4, 2000, 1, color)
	if image != nil {
		rendering.DrawTextImage(target.canvas, image, x, y)
	}
	return 0
}

// DrawAsciiText(target, x, y, text, asciiFont, color?)
//
// asciiFont 是一张 32 列 × 3 行的 ASCII 图集（空格到 '~'）。有 DIBSetView
// 时把整格缩放到 view 的尺寸；没有时按原尺寸绘制。这样同一套代码既支持
// asciifont（11x22 小字），也支持 ec_day_num（16x26 大字）。
func (r *Runtime) drawAsciiText(L *lua.LState) int {
	target := asDIB(L, 1)
	source := asDIB(L, 5)
	if target == nil || target.canvas == nil || source == nil {
		return 0
	}
	image := source.ensureImage(r.host)
	if image == nil {
		return 0
	}
	x := L.CheckInt(2)
	y := L.CheckInt(3)
	text := L.CheckString(4)
	color := uint32(L.OptNumber(6, 0))

	cellWidth := image.Width / 32
	cellHeight := image.Height / 3
	if cellWidth <= 0 || cellHeight <= 0 {
		return 0
	}
	drawWidth, drawHeight, advance := cellWidth, cellHeight, cellWidth
	if source.hasView && source.view[2] > 0 && source.view[3] > 0 {
		drawWidth = source.view[2]
		drawHeight = source.view[3]
		advance = drawWidth
	}
	penX := x
	for index := 0; index < len(text); index++ {
		code := int(text[index])
		if code < 32 || code > 126 {
			penX += advance
			continue
		}
		column := (code - 32) % 32
		row := (code - 32) / 32
		sourceX := column * cellWidth
		sourceY := row * cellHeight
		if color == 0 && drawWidth == cellWidth && drawHeight == cellHeight {
			// 最常见的路径：白色 asciifont 1:1，直接走画布的 alpha 合成。
			target.canvas.DrawRegion(image, sourceX, sourceY, cellWidth, cellHeight, penX, y)
		} else {
			blitScaledTinted(target.canvas, image,
				sourceX, sourceY, cellWidth, cellHeight,
				penX, y, drawWidth, drawHeight, color)
		}
		penX += advance
	}
	return 0
}

// blitScaledTinted 把图片的一个区域最近邻缩放到画布，并按 ARGB 着色。
//
// 不能直接用 BlitScaled：它不接受源区域，而且字符图集需要按 color 着色
// （原引擎的红色负数、黄色 New 标记都靠这个）。图集的字形很小，逐像素代价可忽略。
func blitScaledTinted(canvas *gfx.BgraCanvas, image *gfx.BsgImage,
	sourceX, sourceY, sourceWidth, sourceHeight, destX, destY, destWidth, destHeight int,
	tint uint32) {
	if canvas == nil || image == nil || destWidth <= 0 || destHeight <= 0 {
		return
	}
	tintBlue := byte(255)
	tintGreen := byte(255)
	tintRed := byte(255)
	alphaScale := 255
	if tint != 0 {
		tintBlue = byte(tint)
		tintGreen = byte(tint >> 8)
		tintRed = byte(tint >> 16)
		alphaScale = int(byte(tint >> 24))
		if alphaScale == 0 {
			alphaScale = 255
		}
	}
	for row := 0; row < destHeight; row++ {
		targetY := destY + row
		if targetY < 0 || targetY >= canvas.Height {
			continue
		}
		sourceRow := sourceY + row*sourceHeight/destHeight
		if sourceRow < 0 || sourceRow >= image.Height {
			continue
		}
		for column := 0; column < destWidth; column++ {
			targetX := destX + column
			if targetX < 0 || targetX >= canvas.Width {
				continue
			}
			sourceColumn := sourceX + column*sourceWidth/destWidth
			if sourceColumn < 0 || sourceColumn >= image.Width {
				continue
			}
			sourceOffset := (sourceRow*image.Width + sourceColumn) * 4
			alpha := int(image.Pixels[sourceOffset+3]) * alphaScale / 255
			if alpha <= 0 {
				continue
			}
			blue := int(image.Pixels[sourceOffset]) * int(tintBlue) / 255
			green := int(image.Pixels[sourceOffset+1]) * int(tintGreen) / 255
			red := int(image.Pixels[sourceOffset+2]) * int(tintRed) / 255
			inverse := 255 - alpha
			targetOffset := (targetY*canvas.Width + targetX) * 4
			canvas.Buffer[targetOffset] = byte((blue*alpha + int(canvas.Buffer[targetOffset])*inverse) / 255)
			canvas.Buffer[targetOffset+1] = byte((green*alpha + int(canvas.Buffer[targetOffset+1])*inverse) / 255)
			canvas.Buffer[targetOffset+2] = byte((red*alpha + int(canvas.Buffer[targetOffset+2])*inverse) / 255)
			canvas.Buffer[targetOffset+3] = 255
		}
	}
}
