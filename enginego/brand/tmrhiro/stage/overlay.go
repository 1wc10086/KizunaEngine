package stage

import (
	bishopgfx "kizuna/enginego/brand/bishop/gfx"
	"kizuna/enginego/brand/tmrhiro/grd"
)

// overlay.go 提供界面层需要的绘制原语。原引擎的对话框底图（sys_window_base）
// 不是"半透明黑块"，而是一张"白色渐变 + alpha"遮罩，用 D3D 的反相混合
// （INVSRCALPHA）画成深色：dst = dst*(1-a) + (255-dst)*a。用普通 alpha 叠加
// 只会得到一整片白，这正是修复前"对话框是白色"的原因。

// InvertImage 把一张图按反相混合叠到画布 (x, y)。
//
// 与原引擎一致：alpha=0 的像素完全不参与（背景保留）；alpha 越大越接近
// "底色的反相"，所以底图边缘的渐变会自然过渡到全黑。
func (s *Stage) InvertImage(img *grd.Image, x, y int) {
	if s == nil || img == nil || img.Width <= 0 || img.Height <= 0 {
		return
	}
	s.invertBlit(s.canvas, imageOf(img), x, y)
}

// InvertImageScaled 是 InvertImage 的缩放版本（存读档界面的缩略图底板等）。
func (s *Stage) InvertImageScaled(img *grd.Image, x, y, w, h int) {
	if s == nil || img == nil || img.Width <= 0 || img.Height <= 0 || w <= 0 || h <= 0 {
		return
	}
	if w == img.Width && h == img.Height {
		s.invertBlit(s.canvas, imageOf(img), x, y)
		return
	}
	s.invertBlitScaled(s.canvas, imageOf(img), x, y, w, h)
}

// DrawImage 贴一张整图（界面用：不参与舞台层状态，也不进存档快照）。
func (s *Stage) DrawImage(img *grd.Image, x, y int) {
	if s == nil || img == nil {
		return
	}
	s.canvas.Blit(imageOf(img), x, y, 255)
}

// DrawImageScaled 把图缩放到指定矩形（缩略图、立绘查看器）。
func (s *Stage) DrawImageScaled(img *grd.Image, x, y, w, h int, alpha int) {
	if s == nil || img == nil || w <= 0 || h <= 0 {
		return
	}
	s.canvas.BlitScaled(imageOf(img), x, y, w, h, alpha)
}

// DrawRegion 按源矩形裁剪并贴图。悬停高亮只用 hover 图里的按钮区域，
// 而不是把整张 hover 图铺满——menu_on / cgmode01_on 里每个可点项都画了
// 高亮态，整张贴上去会让所有按钮同时发亮。
func (s *Stage) DrawRegion(img *grd.Image, sx, sy, w, h, x, y int) {
	if s == nil || img == nil || w <= 0 || h <= 0 {
		return
	}
	if sx < 0 || sy < 0 || sx+w > img.Width || sy+h > img.Height {
		return
	}
	canvas := s.canvas
	source := imageOf(img)
	// 复用 BlitRegion 的裁剪语义：源矩形与目标坐标各自裁到画布内。
	canvas.BlitRegion(source, sx, sy, w, h, x, y, 255)
}

// DrawRegionScaled 是带缩放的区域贴图（鉴赏缩略图把 CG 缩进格子）。
func (s *Stage) DrawRegionScaled(img *grd.Image, sx, sy, sw, sh, x, y, w, h int, alpha int) {
	if s == nil || img == nil || sw <= 0 || sh <= 0 || w <= 0 || h <= 0 {
		return
	}
	if sx < 0 || sy < 0 || sx+sw > img.Width || sy+sh > img.Height {
		return
	}
	s.canvas.BlitScaledRegion(imageOf(img), x, y, w, h, alpha, sx, sy, sw, sh)
}

// DrawCanvas 把另一张画布整幅贴上（暂停菜单/存读档界面先画到离屏画布）。
func (s *Stage) DrawCanvas(source *bishopgfx.BgraCanvas) {
	if s == nil || source == nil {
		return
	}
	s.canvas.BlitCanvas(source, 255)
}

// Canvas 暴露内部画布，供界面层直接贴图/追加文字。
func (s *Stage) Canvas() *bishopgfx.BgraCanvas { return s.canvas }

// Snapshot 返回当前画面的一份独立拷贝（存档缩略图用）。
//
// 拷贝而不是借用：Compose 下一帧会整幅覆盖 s.canvas，存档缩略图必须保留
// 存档那一刻的画面。缩略图只在存档时生成一次，一份 3.7MB 的拷贝可以接受。
func (s *Stage) Snapshot() *grd.Image {
	if s == nil {
		return nil
	}
	cp := make([]byte, len(s.canvas.Buffer))
	copy(cp, s.canvas.Buffer)
	return &grd.Image{Width: FrameWidth, Height: FrameHeight, CanvasW: FrameWidth, CanvasH: FrameHeight, Pixels: cp}
}

// invertBlit 是 InvertImage 的实现。
func (s *Stage) invertBlit(canvas *bishopgfx.BgraCanvas, src *bishopgfx.BsgImage, x, y int) {
	if canvas == nil || src == nil {
		return
	}
	left := maxIntG(0, x)
	top := maxIntG(0, y)
	right := minIntG(canvas.Width, x+src.Width)
	bottom := minIntG(canvas.Height, y+src.Height)
	if left >= right || top >= bottom {
		return
	}
	srcStride := src.Stride()
	dst := canvas.Buffer
	for row := top; row < bottom; row++ {
		so := (row-y)*srcStride + (left-x)*4
		to := (row*canvas.Width + left) * 4
		for col := left; col < right; col++ {
			a := int(src.Pixels[so+3])
			if a > 0 {
				for k := 0; k < 3; k++ {
					v := int(dst[to+k])
					dst[to+k] = byte((v*(255-a) + (255-v)*a) / 255)
				}
				dst[to+3] = 255
			}
			so += 4
			to += 4
		}
	}
}

// invertBlitScaled 用最近邻缩放再反相混合。界面缩略图很小，最近邻足够，
// 双线性会让文字边缘糊成灰块。
func (s *Stage) invertBlitScaled(canvas *bishopgfx.BgraCanvas, src *bishopgfx.BsgImage, x, y, w, h int) {
	if canvas == nil || src == nil || w <= 0 || h <= 0 {
		return
	}
	left := maxIntG(0, x)
	top := maxIntG(0, y)
	right := minIntG(canvas.Width, x+w)
	bottom := minIntG(canvas.Height, y+h)
	if left >= right || top >= bottom {
		return
	}
	sourceStride := src.Stride()
	dst := canvas.Buffer
	for row := top; row < bottom; row++ {
		sy := (row - y) * src.Height / h
		if sy < 0 || sy >= src.Height {
			continue
		}
		sourceRow := sy * sourceStride
		to := (row*canvas.Width + left) * 4
		for col := left; col < right; col++ {
			so := sourceRow + ((col-x)*src.Width/w)*4
			a := int(src.Pixels[so+3])
			if a > 0 {
				for k := 0; k < 3; k++ {
					v := int(dst[to+k])
					dst[to+k] = byte((v*(255-a) + (255-v)*a) / 255)
				}
				dst[to+3] = 255
			}
			to += 4
		}
	}
}

// maxIntG / minIntG 是本文件的局部辅助：stage.go 里已有 minInt（部件合成用），
// 再定义同名会导致重复声明。
func maxIntG(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minIntG(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BlitTo 把当前画布整幅拷到宿主缓冲（界面层画完后调用）。
//
// 为什么需要第二次拷贝：Compose 先把舞台层合成并拷出去，界面层（对话框/
// 工具条/选项/菜单）随后画在同一张画布上；如果不再拷一次，玩家只能看到
// 背景，界面全部丢失。3.7MB 的额外拷贝只在有界面层时发生一次，
// 与"整帧重新合成"相比可以忽略。
func (s *Stage) BlitTo(dst []byte) bool {
	if s == nil || len(dst) < FrameWidth*FrameHeight*4 {
		return false
	}
	copy(dst, s.canvas.Buffer)
	return true
}

// ShadeImageScaled 把一张"白色渐变遮罩"当黑色遮罩用：每个像素按自身的 alpha
// 把底色压暗（dst = dst*(1-a)），不改变色相。
//
// 为什么不是反相混合：本作的 sys_window_base 是 1280x181 的白色渐变，原版
// 把它纵向拉伸到 MW_DRAW_HEIGHT(257) 再叠在场景上。若按"反相混合"
// （dst*(1-a) + (255-dst)*a）计算，白色会把场景整体洗成浅灰——手册截图里
// 对话框下半部接近纯黑，只有"按 alpha 压暗"才能得到同一结果。
func (s *Stage) ShadeImageScaled(img *grd.Image, x, y, w, h int) {
	s.ShadeImageScaledAlpha(img, x, y, w, h, 1.0)
}

// ShadeImageScaledAlpha 是 ShadeImageScaled 带整体不透明度系数的版本。
//
// 原版对话框的 D3D 材质色是 SYS_DEF_WINDOW_STAT（默认 0xB3000000，即
// A=0xB3=70%），实际混合是 dst = dst*(1 - a*0.7)。用满 alpha 会把下半屏压成
// 纯黑（比原版深得多），这就是"信息框背景太黑"的根因。
func (s *Stage) ShadeImageScaledAlpha(img *grd.Image, x, y, w, h int, scale float64) {
	if s == nil || img == nil || img.Width <= 0 || img.Height <= 0 || w <= 0 || h <= 0 {
		return
	}
	if scale < 0 {
		scale = 0
	}
	if scale > 1 {
		scale = 1
	}
	canvas := s.canvas
	src := imageOf(img)
	left := maxIntG(0, x)
	top := maxIntG(0, y)
	right := minIntG(canvas.Width, x+w)
	bottom := minIntG(canvas.Height, y+h)
	if left >= right || top >= bottom {
		return
	}
	sourceStride := src.Stride()
	dst := canvas.Buffer
	for row := top; row < bottom; row++ {
		sy := (row - y) * src.Height / h
		if sy < 0 || sy >= src.Height {
			continue
		}
		sourceRow := sy * sourceStride
		to := (row*canvas.Width + left) * 4
		for col := left; col < right; col++ {
			so := sourceRow + ((col-x)*src.Width/w)*4
			a := float64(src.Pixels[so+3]) * scale
			if a <= 0 {
				to += 4
				continue
			}
			keep := 255 - int(a)
			dst[to] = byte(int(dst[to]) * keep / 255)
			dst[to+1] = byte(int(dst[to+1]) * keep / 255)
			dst[to+2] = byte(int(dst[to+2]) * keep / 255)
			dst[to+3] = 255
			to += 4
		}
	}
}

// Clear 把整张画布擦成给定颜色（标题页每帧从底色重画）。
func (s *Stage) Clear(blue, green, red, alpha byte) {
	if s == nil {
		return
	}
	s.canvas.Clear(blue, green, red, alpha)
}
