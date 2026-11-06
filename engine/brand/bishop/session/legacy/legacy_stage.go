package legacy

import (
	"log"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	"kizuna/engine/core"
	"kizuna/engine/core/textrender"
)

const (
	legacyBlankTolerance      = 8
	legacyBlankContentPercent = 8
)

type LegacyStage struct {
	layers               [5]*gfx.BsgImage
	// standees 用独立切片而不是 layers[1]/[2]：Seraph 的 !x 最多可同时挂 5 个
	// 立绘（例如多人场景），旧实现只有两个槽会把第 3 个开始互相覆盖。
	standees             [LegacyStandeeCapacity]*gfx.BsgImage
	windowFrame          *gfx.BsgImage
	face                 *gfx.BsgImage
	timerIcon            *gfx.BsgImage
	titleImage           *gfx.BsgImage
	endingImage          *gfx.BsgImage
	panel                *LegacyPanel
	messageText          string
	messageReveal        int
	fadeAlpha            int
	composedRevision     int
	composedMessageFrame bool
	revision             int
	backgroundName       string
	darkBackground       *gfx.BsgImage
	faceName             string
	text                 *textrender.Renderer
}

func NewLegacyStage() *LegacyStage {
	renderer, err := textrender.New()
	if err != nil {
		log.Printf("legacy stage: text renderer unavailable: %v", err)
	}
	return &LegacyStage{composedRevision: -1, text: renderer}
}

func (s *LegacyStage) Revision() int {
	return s.revision
}

func (s *LegacyStage) BackgroundName() string {
	return s.backgroundName
}

func (s *LegacyStage) DarkBackground() *gfx.BsgImage {
	return s.darkBackground
}

func (s *LegacyStage) FaceName() string {
	return s.faceName
}

func (s *LegacyStage) SetBackground(name string, image *gfx.BsgImage) {
	s.layers[3] = image
	s.backgroundName = name
	if image == nil {
		s.darkBackground = nil
	} else {
		s.darkBackground = buildLegacyDimCopy(image)
	}
	s.revision++
}

func (s *LegacyStage) SetWindowFrame(image *gfx.BsgImage) {
	s.windowFrame = WithBlackKey(image)
	s.revision++
}

func (s *LegacyStage) SetStandee(slot int, image *gfx.BsgImage) {
	if slot < 0 || slot >= len(s.standees) {
		return
	}
	s.standees[slot] = WithColorkey(image)
	s.revision++
}

func (s *LegacyStage) ClearStandees() {
	clear(s.standees[:])
	s.revision++
}

func (s *LegacyStage) SetFace(name string, image *gfx.BsgImage) {
	s.face = image
	s.faceName = name
	s.revision++
}

func (s *LegacyStage) SetTimerIcon(image *gfx.BsgImage) {
	s.timerIcon = image
	s.revision++
}

func (s *LegacyStage) SetTitle(image *gfx.BsgImage) {
	if s.titleImage == image {
		return
	}
	s.titleImage = image
	s.revision++
}

func (s *LegacyStage) SetEnding(image *gfx.BsgImage) {
	if s.endingImage == image {
		return
	}
	s.endingImage = image
	s.revision++
}

func (s *LegacyStage) SetMessage(text string, reveal int) {
	if s.messageText == text && s.messageReveal == reveal {
		return
	}
	s.messageText = text
	s.messageReveal = reveal
	s.revision++
}

// SetPanel 拷贝面板状态，避免调用方之后修改同一份数据时漏掉 revision 变化。
func (s *LegacyStage) SetPanel(panel *LegacyPanel) {
	var next *LegacyPanel
	if panel != nil {
		copied := *panel
		copied.Items = append([]string(nil), panel.Items...)
		next = &copied
	}
	if legacyPanelEqual(s.panel, next) {
		return
	}
	s.panel = next
	s.revision++
}

func (s *LegacyStage) SetFade(alpha int) {
	alpha = min(max(alpha, 0), 255)
	if s.fadeAlpha == alpha {
		return
	}
	s.fadeAlpha = alpha
	s.revision++
}

func (s *LegacyStage) ClearLayers0To3() {
	for layer := 0; layer <= 3; layer++ {
		s.layers[layer] = nil
	}
	clear(s.standees[:])
	// 背景名必须一起清：否则 !c 之后剧本再 !g 同一张图时，applyBackground
	// 会误判“背景没变”而直接 return，画面永远停在黑屏。
	s.backgroundName = ""
	s.darkBackground = nil
	s.revision++
}

func (s *LegacyStage) ClearAll() {
	for layer := range s.layers {
		s.layers[layer] = nil
	}
	clear(s.standees[:])
	s.face = nil
	s.timerIcon = nil
	s.backgroundName = ""
	s.faceName = ""
	s.darkBackground = nil
	s.revision++
}

func (s *LegacyStage) ComposeTo(frame *gfx.BgraCanvas, showMessageFrame bool) bool {
	if s.composedRevision == s.revision && s.composedMessageFrame == showMessageFrame {
		return false
	}
	s.composedRevision = s.revision
	s.composedMessageFrame = showMessageFrame
	frame.Clear(0, 0, 0, 255)
	if s.titleImage != nil {
		frame.Blit(s.titleImage, 0, 0, 255)
		// 标题上的读档面板必须画在标题图之后；否则标题分支提前返回，
		// 「途中から始める」打开的面板永远不可见。
		if s.panel != nil {
			drawLegacyPanel(frame, s.text, s.panel)
		}
		return true
	}
	if background := s.layers[3]; background != nil {
		if background.Width >= ScreenWidth || background.Height >= ScreenHeight {
			frame.Blit(background, 0, 0, 255)
		} else {
			frame.Blit(background, BackgroundOriginX(background.Width), BackgroundOriginY(background.Height), 255)
		}
	}
	for _, standee := range s.standees {
		if standee != nil {
			frame.Blit(standee, standee.OffsetX, standee.OffsetY, 255)
		}
	}
	if s.face != nil {
		frame.Blit(s.face, FaceX, FaceY, 255)
	}
	if showMessageFrame && s.windowFrame != nil {
		frame.Blit(s.windowFrame, 0, 0, 255)
	}
	if s.timerIcon != nil {
		frame.Blit(s.timerIcon, TimerIconX, TimerIconY, 255)
	}
	if showMessageFrame && s.messageText != "" {
		drawLegacyMessage(frame, s.text, visibleLegacyMessage(s.messageText, s.messageReveal))
	}
	if s.panel != nil {
		drawLegacyPanel(frame, s.text, s.panel)
	}
	if s.endingImage != nil {
		frame.Blit(s.endingImage, 0, 0, 255)
	}
	if s.fadeAlpha > 0 {
		frame.Overlay(0, 0, 0, s.fadeAlpha)
	}
	return true
}

func drawLegacyMessage(frame *gfx.BgraCanvas, renderer *textrender.Renderer, text string) {
	rendering.DrawText(frame, renderer, text, MessageTextX, MessageTextY, MessageTextSize, 1, 1, MessageLineHeight, MessageTextWidth, MessageTextLines, 0xffffffff)
}

func visibleLegacyMessage(text string, reveal int) string {
	if reveal < 0 {
		return text
	}
	runes := []rune(text)
	if reveal >= len(runes) {
		return text
	}
	return string(runes[:reveal])
}

func legacyPanelEqual(a, b *LegacyPanel) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind || a.Title != b.Title || a.Highlight != b.Highlight || a.Scroll != b.Scroll ||
		a.InstantText != b.InstantText || a.BgmEnabled != b.BgmEnabled || a.Status != b.Status ||
		len(a.Items) != len(b.Items) {
		return false
	}
	for index := range a.Items {
		if a.Items[index] != b.Items[index] {
			return false
		}
	}
	return true
}

func IsBlankCard(image *gfx.BsgImage) bool {
	pixels := image.Pixels
	count := len(pixels) / 4
	if count <= 0 {
		return true
	}
	keyBlue := int(pixels[0])
	keyGreen := int(pixels[1])
	keyRed := int(pixels[2])
	content := 0
	for offset := 0; offset+4 <= len(pixels); offset += 4 {
		if core.Abs(int(pixels[offset])-keyBlue) > legacyBlankTolerance ||
			core.Abs(int(pixels[offset+1])-keyGreen) > legacyBlankTolerance ||
			core.Abs(int(pixels[offset+2])-keyRed) > legacyBlankTolerance {
			content++
		}
	}
	return content*100 <= count*legacyBlankContentPercent
}

func WithColorkey(image *gfx.BsgImage) *gfx.BsgImage {
	pixels := image.Pixels
	if len(pixels) < 4 {
		return image
	}
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 255 {
			return image
		}
	}
	keyBlue := pixels[0]
	keyGreen := pixels[1]
	keyRed := pixels[2]
	copied := make([]byte, len(pixels))
	copy(copied, pixels)
	for offset := 0; offset+4 <= len(copied); offset += 4 {
		if copied[offset] == keyBlue && copied[offset+1] == keyGreen && copied[offset+2] == keyRed {
			copied[offset+3] = 0
		}
	}
	return gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, copied)
}

// WithBlackKey 把 frame.KG 里的纯黑 (0,0,0) 作为透明键。已经带有透明像素的图视为
// 处理过，直接返回，避免重复扣键。
func WithBlackKey(image *gfx.BsgImage) *gfx.BsgImage {
	if image == nil || len(image.Pixels) < 4 {
		return image
	}
	pixels := image.Pixels
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 255 {
			return image
		}
	}
	copied := make([]byte, len(pixels))
	copy(copied, pixels)
	for offset := 0; offset+4 <= len(copied); offset += 4 {
		if copied[offset] == 0 && copied[offset+1] == 0 && copied[offset+2] == 0 {
			copied[offset+3] = 0
		}
	}
	return gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, copied)
}

func buildLegacyDimCopy(image *gfx.BsgImage) *gfx.BsgImage {
	copied := make([]byte, len(image.Pixels))
	copy(copied, image.Pixels)
	for offset := 0; offset+4 <= len(copied); offset += 4 {
		copied[offset] = dimByte(copied[offset])
		copied[offset+1] = dimByte(copied[offset+1])
		copied[offset+2] = dimByte(copied[offset+2])
	}
	return gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, copied)
}

func dimByte(value byte) byte {
	if value <= DimStep {
		return 0
	}
	return value - DimStep
}
