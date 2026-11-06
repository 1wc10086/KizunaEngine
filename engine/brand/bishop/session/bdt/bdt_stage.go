package bdt

import (
	"strings"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

const (
	bdtWindowBlue  = 10
	bdtWindowGreen = 10
	bdtWindowRed   = 10
	bdtWindowAlpha = 196
	bdtBorderBlue  = 212
	bdtBorderGreen = 212
	bdtBorderRed   = 212
	bdtBorderAlpha = 255
)

var bdtWindowRects = [][4]int{
	{0, 240, 640, 240},
	{0, 0, 320, 240},
	{320, 0, 320, 240},
}

type BdtStage struct {
	resolveImage func(string) *gfx.BsgImage
	// font 是引擎自绘对话框用的字体。BDT 没有 message.bsg 素材，原引擎的
	// 对话框也是纯色矩形 + 系统字体；这里用随二进制发布的内嵌字体，
	// 保证安卓端不依赖系统是否装了日文字体。
	font     *textrender.Renderer
	sprites  []*BdtSprite
	windows  [3]bool
	frame    *gfx.BgraCanvas
	revision int

	backgroundName    string
	effectCode        int
	screenMode        int
	speaker           string
	text              string
	faceName          string
	variant           BdtVariant
	background        *gfx.BsgImage
	backgroundFigure  *gfx.BsgImage
	backgroundMissing bool
	faceImage         *gfx.BsgImage
	faceImageKey      string
	// messageColor 是正文颜色；manin 的責め言葉台词用粉色，普通台词白色。
	messageColor uint32
	// windowKeyed 是做过颜色键处理的对话框素材（sb_win 的纯蓝背景）。
	// 每个 BdtStage 只处理一次：640x120 的逐像素扫描挂在每次重绘上会拖慢打字机。
	windowKeyed     *gfx.BsgImage
	windowKeyedName string
	paintedRevision int
}

func NewBdtStage(resolveImage func(string) *gfx.BsgImage) *BdtStage {
	sprites := make([]*BdtSprite, SpriteSlots)
	for slot := range sprites {
		sprites[slot] = NewBdtSprite(slot)
	}
	font, err := textrender.New()
	if err != nil {
		core.Logf("bdt: text renderer unavailable: %v", err)
		font = nil
	}
	return &BdtStage{
		resolveImage:    resolveImage,
		font:            font,
		sprites:         sprites,
		frame:           gfx.NewBgraCanvas(ScreenWidth, ScreenHeight),
		variant:         ContainerVariant,
		paintedRevision: -1,
	}
}

func (s *BdtStage) Frame() *gfx.BgraCanvas {
	return s.frame
}

func (s *BdtStage) Windows() [3]bool {
	return s.windows
}

func (s *BdtStage) Revision() int {
	return s.revision
}

func (s *BdtStage) BackgroundName() string {
	return s.backgroundName
}

func (s *BdtStage) EffectCode() int {
	return s.effectCode
}

func (s *BdtStage) ScreenMode() int {
	return s.screenMode
}

func (s *BdtStage) Speaker() string {
	return s.speaker
}

func (s *BdtStage) Text() string {
	return s.text
}

func (s *BdtStage) FaceName() string {
	return s.faceName
}

func (s *BdtStage) Variant() BdtVariant {
	return s.variant
}

func (s *BdtStage) SetVariant(variant BdtVariant) {
	if s.variant != variant {
		s.variant = variant
		s.revision++
	}
}

func (s *BdtStage) SetFace(name string) {
	s.faceName = name
	s.revision++
}

func (s *BdtStage) SetBackground(name string, effectCode int) {
	s.backgroundName = name
	s.effectCode = effectCode
	image, masked := s.resolveArt(s.backgroundName)
	s.backgroundMissing = image == nil && len(s.backgroundName) > 0
	if image == nil {
		if s.backgroundMissing {
			core.Logf("bdt: background '%s' not found", s.backgroundName)
		}
		s.background = nil
		s.backgroundFigure = nil
	} else if masked {
		s.backgroundFigure = image
	} else {
		s.background = image
		s.backgroundFigure = nil
	}
	s.revision++
}

func (s *BdtStage) SetSprite(slot int, name string, x, y, mode int) {
	if slot < 0 || slot >= len(s.sprites) {
		core.Logf("bdt: sprite slot out of range: %d", slot)
		return
	}
	sprite := s.sprites[slot]
	if len(strings.TrimSpace(name)) == 0 {
		sprite.Clear()
		s.revision++
		return
	}
	clean := strings.TrimSpace(name)
	image, _ := s.resolveArt(clean)
	if image == nil {
		core.Logf("bdt: sprite '%s' not found", clean)
		sprite.Clear()
		s.revision++
		return
	}
	if s.variant.UsesContainerArchive {
		sprite.Set(clean, x, y, mode, image)
	} else {
		sprite.SetOpaque(clean, x, y, mode, image)
	}
	s.revision++
}

func (s *BdtStage) ClearSprite(slot int) {
	if slot < 0 || slot >= len(s.sprites) {
		core.Logf("bdt: sprite slot out of range: %d", slot)
		return
	}
	s.sprites[slot].Clear()
	s.revision++
}

func (s *BdtStage) ClearSprites() {
	for _, sprite := range s.sprites {
		sprite.Clear()
	}
	s.revision++
}

func (s *BdtStage) SetWindow(part int, visible bool) {
	if part < 0 || part >= len(s.windows) {
		core.Logf("bdt: window part out of range: %d", part)
		return
	}
	s.windows[part] = visible
	s.revision++
}

func (s *BdtStage) ShowMessage(speaker, text string) {
	s.speaker = speaker
	s.text = text
	s.revision++
}

// SetMessageColor 设置正文颜色。0 表示回到默认白色，避免旧状态漏色。
func (s *BdtStage) SetMessageColor(color uint32) {
	if color == 0 {
		color = 0xffffffff
	}
	if s.messageColor == color {
		return
	}
	s.messageColor = color
	s.revision++
}

func (s *BdtStage) SetScreenMode(mode int) {
	s.screenMode = mode
	s.revision++
}

func (s *BdtStage) Redraw() {
	s.paintedRevision = -1
	s.refresh()
}

func (s *BdtStage) ComposeTo(target *gfx.BgraCanvas, showMessageFrame bool) {
	if target == nil {
		return
	}
	s.refresh()
	if target.Width == s.frame.Width && target.Height == s.frame.Height {
		target.BlitCanvas(s.frame, 255)
	} else {
		s.paintScene(target)
	}
	if showMessageFrame {
		s.paintWindows(target)
	}
}

func (s *BdtStage) refresh() {
	if s.paintedRevision == s.revision {
		return
	}
	s.paintScene(s.frame)
	s.paintedRevision = s.revision
}

func (s *BdtStage) paintScene(canvas *gfx.BgraCanvas) {
	if s.background != nil {
		canvas.Blit(s.background, 0, 0, 255)
	} else if s.backgroundMissing {
		canvas.Fill(0, 0, canvas.Width, canvas.Height, 255, 255, 255, 255)
	} else {
		canvas.Fill(0, 0, canvas.Width, canvas.Height, 0, 0, 0, 255)
	}
	if s.backgroundFigure != nil {
		canvas.Blit(s.backgroundFigure, 0, 0, 255)
	}
	for _, sprite := range s.sprites {
		if image := sprite.Image(); image != nil {
			canvas.Blit(image, sprite.X()+image.OffsetX, sprite.Y()+image.OffsetY, 255)
		}
	}
	if !s.variant.UsesContainerArchive {
		s.paintManinFace(canvas)
	}
}

func (s *BdtStage) paintManinFace(canvas *gfx.BgraCanvas) {
	if len(s.faceName) == 0 {
		return
	}
	if len(s.variant.NoFacePlaceholder) > 0 && strings.EqualFold(s.faceName, s.variant.NoFacePlaceholder) {
		return
	}
	if !strings.EqualFold(s.faceImageKey, s.faceName) {
		s.faceImage, _ = s.resolveArt(s.faceName)
		s.faceImageKey = s.faceName
		if s.faceImage == nil {
			core.Logf("bdt: face '%s' not found", s.faceName)
		}
	}
	if s.faceImage != nil {
		x, y, _ := FaceRect(s.variant)
		canvas.Blit(s.faceImage, x, y, 255)
	}
}

func (s *BdtStage) paintWindows(canvas *gfx.BgraCanvas) {
	// 第 0 框是主消息框。BDT 脚本不像现代 BSXScript 那样用 IMAGE 铺对话框，
	// 引擎默认就要显示它；即使脚本用 BW0,0 暂时隐藏，只要当前有对白，
	// 仍然要画出来，否则玩家会看到 CG 却读不到文字。
	if s.windows[0] || len(s.text) > 0 || len(s.speaker) > 0 {
		// 有 sb_win 素材时画原版对话框；没有才退回纯色矩形。
		if !s.paintWindowImage(canvas) {
			paintBdtWindow(canvas, 0)
		}
		s.paintMessageText(canvas)
	}
	// part 1/2 是剧本用的半屏窗口，保持原样：脚本显式打开才画。
	for part := 1; part < len(s.windows); part++ {
		if s.windows[part] {
			paintBdtWindow(canvas, part)
		}
	}
}

// paintMessageText 把当前说话人和正文画进主消息框。
//
// BDT 的正文是 CP932 解码后的纯文本，没有换行符；这里按像素宽度自动折行。
// 名字和正文分开画，名字用暖色，正文用白色 + 黑描边，和 Bishop 同一代的
// 对话框观感一致。
func (s *BdtStage) paintMessageText(canvas *gfx.BgraCanvas) {
	if canvas == nil || s.font == nil {
		return
	}
	left, top, width, height := WindowRect(s.variant)
	x := left + s.variant.TextLeft
	nameY := s.variant.TextTop - 20
	textY := s.variant.TextTop
	maxWidth := s.variant.TextWidth
	if maxWidth <= 0 {
		maxWidth = width - s.variant.TextLeft - 40
	}
	if maxWidth <= 0 {
		return
	}
	lineHeight := s.variant.TextLineHeight
	if lineHeight <= 0 {
		lineHeight = 20
	}
	maxLines := (top + height - textY - 4) / lineHeight
	if maxLines < 1 {
		maxLines = 1
	}
	if speaker := strings.TrimSpace(s.speaker); speaker != "" {
		if image := s.font.RenderOutline(speaker, 18, 1, 1, lineHeight, maxWidth, 1, 0xffffcc66, 0xff000000); image != nil {
			rendering.DrawTextImage(canvas, image, x, nameY)
		}
	}
	if text := strings.TrimSpace(s.text); text != "" {
		color := s.messageColor
		if color == 0 {
			color = 0xffffffff
		}
		if image := s.font.RenderOutline(text, 18, 1, 1, lineHeight, maxWidth, maxLines, color, 0xff000000); image != nil {
			rendering.DrawTextImage(canvas, image, x, textY)
		}
	}
}

// paintWindowImage 画原版对话框素材并做颜色键。
//
// sb_win 是 1280x120 的双联图：左半是绿框（蓝底透明键）、右半是灰框。
// 游戏使用左半 640x120；蓝色 (0,0,255) 是透明色，直接 Blit 会在屏幕上留下
// 一大块蓝色。键控结果缓存到 windowKeyed，避免每帧扫 7.6 万像素。
func (s *BdtStage) paintWindowImage(canvas *gfx.BgraCanvas) bool {
	if canvas == nil || s.variant.WindowImage == "" {
		return false
	}
	if s.windowKeyed == nil || s.windowKeyedName != s.variant.WindowImage {
		raw := s.resolveImage(s.variant.WindowImage)
		if raw == nil {
			return false
		}
		// 只取左半：素材是"正常/灰色"两套并排，右半不是本作使用的版本。
		width := min(640, raw.Width)
		height := raw.Height
		pixels := make([]byte, width*height*4)
		for y := 0; y < height; y++ {
			copy(pixels[y*width*4:(y+1)*width*4], raw.Pixels[y*raw.Stride():y*raw.Stride()+width*4])
		}
		key := s.variant.WindowImageColorKey
		keyB := byte(key)
		keyG := byte(key >> 8)
		keyR := byte(key >> 16)
		for i := 0; i+3 < len(pixels); i += 4 {
			if pixels[i] == keyB && pixels[i+1] == keyG && pixels[i+2] == keyR {
				pixels[i+3] = 0
			}
		}
		s.windowKeyed = gfx.NewBsgImage(width, height, 0, 0, pixels)
		s.windowKeyedName = s.variant.WindowImage
	}
	if !s.variant.UsesContainerArchive {
		// manin 的 sb_win 用黄色抠出左右两块内部区域：左块放 96x96 表情，
		// 右块放正文。只给正文块铺深色底；左块有表情时保持透明让
		// paintScene 画好的表情透出来，没有表情时补深色，避免背景从
		// 窗口里穿出来。
		if len(s.faceName) == 0 || (len(s.variant.NoFacePlaceholder) > 0 && strings.EqualFold(s.faceName, s.variant.NoFacePlaceholder)) {
			canvas.Fill(s.variant.FaceLeft, s.variant.FaceTop, s.variant.FaceSize, s.variant.FaceSize, 36, 18, 10, 225)
		}
		canvas.Fill(s.variant.TextLeft-10, s.variant.TextTop-18,
			s.variant.TextWidth+20, s.variant.WindowHeight-22, 36, 18, 10, 225)
	}
	canvas.Draw(s.windowKeyed, s.variant.WindowLeft, s.variant.WindowTop)
	return true
}

func paintBdtWindow(canvas *gfx.BgraCanvas, part int) {
	rect := bdtWindowRects[part]
	canvas.Fill(rect[0], rect[1], rect[2], rect[3], bdtWindowBlue, bdtWindowGreen, bdtWindowRed, bdtWindowAlpha)
	canvas.Fill(rect[0], rect[1], rect[2], 1, bdtBorderBlue, bdtBorderGreen, bdtBorderRed, bdtBorderAlpha)
	canvas.Fill(rect[0], rect[1]+rect[3]-1, rect[2], 1, bdtBorderBlue, bdtBorderGreen, bdtBorderRed, bdtBorderAlpha)
	canvas.Fill(rect[0], rect[1], 1, rect[3], bdtBorderBlue, bdtBorderGreen, bdtBorderRed, bdtBorderAlpha)
	canvas.Fill(rect[0]+rect[2]-1, rect[1], 1, rect[3], bdtBorderBlue, bdtBorderGreen, bdtBorderRed, bdtBorderAlpha)
}

func (s *BdtStage) resolveArt(name string) (*gfx.BsgImage, bool) {
	if len(strings.TrimSpace(name)) == 0 {
		return nil, false
	}
	raw := s.resolveImage(strings.TrimSpace(name))
	if raw == nil {
		return nil, false
	}
	if s.variant.UsesContainerArchive {
		return DecodeBdtArt(raw)
	}
	return DecodeBdtArtWithVariant(raw, s.variant)
}
