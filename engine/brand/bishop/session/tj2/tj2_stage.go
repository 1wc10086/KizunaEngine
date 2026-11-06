package tj2

import (
	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/brand/bishop/rendering"
	vmtj2 "kizuna/engine/brand/bishop/vm/tj2"
	"kizuna/engine/core"
)

type Tj2BackgroundLayer struct {
	Name    string
	OffsetX int
	OffsetY int
}

type Tj2Stage struct {
	images                  *rendering.ImageCache
	backgroundHistory       []Tj2BackgroundLayer
	standees                []*gfx.BsgImage
	standeeOffsetX          []int
	standeeOffsetY          []int
	background              *gfx.BsgImage
	accumulateBackground    bool
	backgroundSurfaceCanvas *gfx.BgraCanvas
	backgroundDrawn         bool
	backgroundOffsetX       int
	backgroundOffsetY       int
	messageWindow           *gfx.BsgImage
	messageFallbackImage    string
	messageHasSource        bool
	// messageKey 表示信息框底图需要用纯绿键色去透明。Toku2/Classic 为真，
	// 纯现代 profile 为假（保持原来的 alpha 行为）。
	messageKey           bool
	messageSourceX       int
	messageSourceY       int
	messageSourceWidth   int
	messageSourceHeight  int
	messageDestX         int
	messageDestY         int
	composedRevision     int
	composedMessageFrame bool
	revision             int
	standeeSlotCount     int
	backgroundName       string
	classic              bool
}

const tj2StageDefaultSlots = 5

const tj2StageMaxBackgroundHistory = 32

func NewTj2Stage(images *rendering.ImageCache, standeeSlots int) *Tj2Stage {
	count := standeeSlots
	if count < 1 {
		count = 1
	}
	if count > tj2StageDefaultSlots {
		count = tj2StageDefaultSlots
	}
	return &Tj2Stage{
		images:               images,
		standees:             make([]*gfx.BsgImage, count),
		standeeOffsetX:       make([]int, count),
		standeeOffsetY:       make([]int, count),
		messageFallbackImage: MessageWindowImage,
		messageDestX:         MessageOriginX,
		messageDestY:         MessageOriginY,
		composedRevision:     -1,
		standeeSlotCount:     count,
	}
}

func (s *Tj2Stage) Revision() int {
	return s.revision
}

func (s *Tj2Stage) StandeeSlotCount() int {
	return s.standeeSlotCount
}

func (s *Tj2Stage) BackgroundName() string {
	return s.backgroundName
}

func (s *Tj2Stage) BackgroundOffsetX() int {
	return s.backgroundOffsetX
}

func (s *Tj2Stage) BackgroundOffsetY() int {
	return s.backgroundOffsetY
}

func (s *Tj2Stage) Invalidate() {
	s.revision++
}

func (s *Tj2Stage) ConfigureProfile(profile Tj2GameProfile) {
	s.messageFallbackImage = profile.MessageWindowImage
	s.messageHasSource = profile.MessageWindowIsPartsSheet
	s.messageKey = profile.UIFamily != Tj2UIFamilyModern
	s.messageSourceX = profile.MessageWindowSourceX
	s.messageSourceY = profile.MessageWindowSourceY
	s.messageSourceWidth = profile.MessageWindowSourceWidth
	s.messageSourceHeight = profile.MessageWindowSourceHeight
	s.messageDestX = profile.MessageWindowDestX
	s.messageDestY = profile.MessageWindowDestY
	s.accumulateBackground = profile.Variant == vmtj2.Tj2VariantClassic
	s.classic = profile.Variant == vmtj2.Tj2VariantClassic
	s.revision++
}

func (s *Tj2Stage) SetMessageWindow(image *gfx.BsgImage) {
	if image != nil && s.messageKey {
		// sp_mes1/sp09 的 KG 都是索引色，透明区是纯绿而不是 alpha。
		// 只有自绘界面的 profile 才做色键；纯现代分支的窗口可能用别的
		// 透明方式，不能在这里误键。
		image = WithGreenKey(image)
	}
	s.messageWindow = image
	s.revision++
}

func (s *Tj2Stage) SetBackground(name string, image *gfx.BsgImage, offsetX, offsetY int) {
	s.background = image
	s.backgroundName = name
	s.backgroundOffsetX = offsetX
	s.backgroundOffsetY = offsetY
	if s.accumulateBackground {
		if s.backgroundSurfaceCanvas == nil {
			s.backgroundSurfaceCanvas = gfx.NewBgraCanvas(ScreenWidth, ScreenHeight)
		}
		s.backgroundSurfaceCanvas.Draw(image, backgroundOriginX(image.Width)+offsetX, backgroundOriginY(image.Height)+offsetY)
		s.backgroundDrawn = true
		if len(s.backgroundHistory) >= tj2StageMaxBackgroundHistory {
			core.Logf("tj2: background history full, dropping oldest '%s'", s.backgroundHistory[0].Name)
			s.backgroundHistory = append(s.backgroundHistory[:0], s.backgroundHistory[1:]...)
		}
		s.backgroundHistory = append(s.backgroundHistory, Tj2BackgroundLayer{Name: name, OffsetX: offsetX, OffsetY: offsetY})
	}
	s.revision++
}

func (s *Tj2Stage) BackgroundHistory() []Tj2BackgroundLayer {
	out := make([]Tj2BackgroundLayer, len(s.backgroundHistory))
	copy(out, s.backgroundHistory)
	return out
}

func (s *Tj2Stage) ReplayBackgroundHistory(history []Tj2BackgroundLayer) {
	s.background = nil
	s.backgroundSurfaceCanvas = nil
	s.backgroundDrawn = false
	s.backgroundOffsetX = 0
	s.backgroundOffsetY = 0
	s.backgroundName = ""
	s.backgroundHistory = s.backgroundHistory[:0]
	for _, layer := range history {
		if len(layer.Name) == 0 {
			continue
		}
		image := s.images.Get(layer.Name)
		if image == nil {
			core.Logf("tj2: save background '%s' missing", layer.Name)
			continue
		}
		s.SetBackground(layer.Name, image, layer.OffsetX, layer.OffsetY)
	}
	s.revision++
}

func (s *Tj2Stage) SetStandee(layer int, image *gfx.BsgImage, offsetX, offsetY int) {
	if layer < 0 || layer >= s.standeeSlotCount {
		core.Logf("tj2: standee layer %d outside 0..%d", layer, s.standeeSlotCount-1)
		return
	}
	s.standees[layer] = WithColorkey(image)
	s.standeeOffsetX[layer] = offsetX
	s.standeeOffsetY[layer] = offsetY
	s.revision++
}

func (s *Tj2Stage) ClearStandees() {
	for layer := range s.standees {
		s.standees[layer] = nil
		s.standeeOffsetX[layer] = 0
		s.standeeOffsetY[layer] = 0
	}
	s.revision++
}

func (s *Tj2Stage) ClearAll() {
	s.background = nil
	s.backgroundSurfaceCanvas = nil
	s.backgroundDrawn = false
	s.backgroundHistory = s.backgroundHistory[:0]
	s.messageWindow = nil
	for layer := range s.standees {
		s.standees[layer] = nil
		s.standeeOffsetX[layer] = 0
		s.standeeOffsetY[layer] = 0
	}
	s.backgroundOffsetX = 0
	s.backgroundOffsetY = 0
	s.backgroundName = ""
	s.revision++
}

func (s *Tj2Stage) ComposeTo(frame *gfx.BgraCanvas, showMessageFrame bool) bool {
	if s.composedRevision == s.revision && s.composedMessageFrame == showMessageFrame {
		return false
	}
	s.composedRevision = s.revision
	s.composedMessageFrame = showMessageFrame
	frame.Clear(0, 0, 0, 255)
	if s.accumulateBackground {
		if s.backgroundDrawn && s.backgroundSurfaceCanvas != nil {
			frame.BlitCanvas(s.backgroundSurfaceCanvas, 255)
		}
	} else if s.background != nil {
		frame.Blit(s.background, backgroundOriginX(s.background.Width)+s.backgroundOffsetX, backgroundOriginY(s.background.Height)+s.backgroundOffsetY, 255)
	}
	for layer := range s.standees {
		if s.standees[layer] != nil {
			frame.Blit(s.standees[layer], 8*s.standeeOffsetX[layer], 8*s.standeeOffsetY[layer], 255)
		}
	}
	if showMessageFrame {
		window := s.messageWindow
		if window == nil {
			window = s.images.Get(s.messageFallbackImage)
		}
		if window != nil {
			if s.messageHasSource {
				frame.BlitRegion(window, s.messageSourceX, s.messageSourceY, s.messageSourceWidth, s.messageSourceHeight, s.messageDestX, s.messageDestY, 255)
			} else {
				frame.Blit(window, s.messageDestX, s.messageDestY, 255)
			}
		}
	}
	return true
}

func WithColorkey(image *gfx.BsgImage) *gfx.BsgImage {
	if image == nil || len(image.Pixels) < 4 {
		return image
	}
	keyBlue := image.Pixels[0]
	keyGreen := image.Pixels[1]
	keyRed := image.Pixels[2]
	return withColorKey(image, keyBlue, keyGreen, keyRed)
}

// WithGreenKey 专用于 TJ2 的界面覆盖层。
//
// 这些图（sp_mes1、title02/04、music2、cgmode4…）统一用纯绿 (0,255,0)
// 表示透明，但左上角是米色边框，不能拿首像素当键色。立绘仍用 WithColorkey，
// 因为它们的键色不一定是绿。
func WithGreenKey(image *gfx.BsgImage) *gfx.BsgImage {
	if image == nil || len(image.Pixels) < 4 {
		return image
	}
	return withColorKey(image, 0, 255, 0)
}

func withColorKey(image *gfx.BsgImage, keyBlue, keyGreen, keyRed byte) *gfx.BsgImage {
	for offset := 3; offset < len(image.Pixels); offset += 4 {
		if image.Pixels[offset] != 255 {
			return image
		}
	}
	cp := make([]byte, len(image.Pixels))
	copy(cp, image.Pixels)
	for offset := 0; offset+4 <= len(cp); offset += 4 {
		if cp[offset] == keyBlue && cp[offset+1] == keyGreen && cp[offset+2] == keyRed {
			cp[offset+3] = 0
			// 透明像素的 RGB 也要清零：存档缩略图用 ScaleTo 做区域平均，
			// 它不按 alpha 预乘，保留绿色会让缩略图底部出现一条绿带。
			cp[offset] = 0
			cp[offset+1] = 0
			cp[offset+2] = 0
		}
	}
	return gfx.NewBsgImage(image.Width, image.Height, image.OffsetX, image.OffsetY, cp)
}

func backgroundOriginX(width int) int {
	if width >= ScreenWidth {
		return 0
	}
	return (ScreenWidth - width) / 2
}

func backgroundOriginY(height int) int {
	if height >= ScreenHeight {
		return 0
	}
	return (ScreenHeight - height) / 2
}
