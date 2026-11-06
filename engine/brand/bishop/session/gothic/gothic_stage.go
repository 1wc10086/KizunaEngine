package gothic

import (
	"math"

	"kizuna/engine/brand/bishop/gfx"
	"kizuna/engine/core"
)

const (
	gothicLayerCount = 8
	gothicPlaneCount = 9
)

const gothicEffectDurationSeconds = 0.5

type GothicStage struct {
	catalog         *GothicResourceCatalog
	font            *GothicFontFile
	layers          [gothicLayerCount]*gfx.BgraCanvas
	planes          [gothicLayerCount][gothicPlaneCount]*gfx.BsgImage
	planeX          [gothicLayerCount][gothicPlaneCount]int
	planeY          [gothicLayerCount][gothicPlaneCount]int
	planeRegion     [gothicLayerCount][gothicPlaneCount]bool
	regionX         [gothicLayerCount][gothicPlaneCount]int
	regionY         [gothicLayerCount][gothicPlaneCount]int
	regionWidth     [gothicLayerCount][gothicPlaneCount]int
	regionHeight    [gothicLayerCount][gothicPlaneCount]int
	visible         [gothicLayerCount]bool
	filled          [gothicLayerCount]bool
	fillBlue        [gothicLayerCount]byte
	fillGreen       [gothicLayerCount]byte
	fillRed         [gothicLayerCount]byte
	messageLines    []string
	menuItems       []string
	composedMessage []string
	composedMenu    []string
	messageVisible  bool
	messageReveal   int
	// messageWindow 是 Toku1 的 p_kaiwa.bmp 对话框底图。MAZE 没有窗口底图，
	// 文字直接画在画面上，所以此字段只在 Toku1 设置。
	messageWindow  *gfx.BsgImage
	messageWindowX int
	messageWindowY int
	// snapshot 是读档后恢复的整屏画面。剧本的下一个 g draw 会清掉它，
	// 在此之前作为底图显示，避免读档瞬间丢掉立绘/差分。
	snapshot         *gfx.BsgImage
	composedVisible  bool
	composedRevision int
	effectType       int
	effectParameter  int
	effectSource     int
	effectTarget     int
	effectProgress   float64
	effectDirty      bool
	Frame            *gfx.BgraCanvas
	Revision         int
	EffectActive     bool
}

func NewGothicStage(catalog *GothicResourceCatalog, font *GothicFontFile) *GothicStage {
	stage := &GothicStage{
		catalog:          catalog,
		font:             font,
		messageVisible:   true,
		messageReveal:    -1,
		composedVisible:  true,
		composedRevision: -1,
		effectProgress:   1,
		Frame:            gfx.NewBgraCanvas(GothicScreenWidth, GothicScreenHeight),
	}
	for layer := 0; layer < gothicLayerCount; layer++ {
		stage.layers[layer] = gfx.NewBgraCanvas(GothicScreenWidth, GothicScreenHeight)
		stage.visible[layer] = true
	}
	return stage
}

func (s *GothicStage) LoadBase(layer int, image string) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	// draw 是“整层换底图”，不是只换 plane0：剧本在切场景时通常只写
	// `g draw 4, "gbXX"`，不会先 layer clear；如果保留上一场的立绘
	// plane1..8，人物会残留到新背景上。
	for plane := 1; plane < gothicPlaneCount; plane++ {
		s.planes[target][plane] = nil
		s.planeX[target][plane] = 0
		s.planeY[target][plane] = 0
		s.planeRegion[target][plane] = false
	}
	s.planes[target][0] = s.resolveImage(image)
	s.planeX[target][0] = 0
	s.planeY[target][0] = 0
	s.planeRegion[target][0] = false
	s.visible[target] = true
	s.Revision++
}

func (s *GothicStage) SetBaseImage(layer int, image *gfx.BsgImage) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	s.planes[target][0] = image
	s.planeX[target][0] = 0
	s.planeY[target][0] = 0
	s.planeRegion[target][0] = false
	s.visible[target] = true
	s.Revision++
}

func (s *GothicStage) SetPlane(layer, plane int, image string, x, y int) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	if plane < 1 || plane > 8 {
		return
	}
	s.planes[target][plane] = s.resolveImage(image)
	s.planeX[target][plane] = x
	s.planeY[target][plane] = y
	s.planeRegion[target][plane] = false
	s.visible[target] = true
	s.Revision++
}

func (s *GothicStage) SetPlaneRegion(layer, plane int, image string, sourceX, sourceY, width, height, x, y int) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	if plane < 1 || plane > 8 {
		return
	}
	s.planes[target][plane] = s.resolveImage(image)
	s.planeX[target][plane] = x
	s.planeY[target][plane] = y
	s.planeRegion[target][plane] = true
	s.regionX[target][plane] = sourceX
	s.regionY[target][plane] = sourceY
	s.regionWidth[target][plane] = width
	s.regionHeight[target][plane] = height
	s.visible[target] = true
	s.Revision++
}

func (s *GothicStage) ClearPlanes(layer int) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	for plane := 1; plane < gothicPlaneCount; plane++ {
		s.planes[target][plane] = nil
		s.planeX[target][plane] = 0
		s.planeY[target][plane] = 0
		s.planeRegion[target][plane] = false
	}
	s.Revision++
}

func (s *GothicStage) ClearLayer(layer int) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	for plane := 0; plane < gothicPlaneCount; plane++ {
		s.planes[target][plane] = nil
		s.planeX[target][plane] = 0
		s.planeY[target][plane] = 0
		s.planeRegion[target][plane] = false
	}
	s.filled[target] = false
	s.visible[target] = false
	s.Revision++
}

func (s *GothicStage) FillLayer(layer int, blue, green, red byte) {
	s.snapshot = nil
	target := clampGothicLayer(layer)
	s.filled[target] = true
	s.fillBlue[target] = blue
	s.fillGreen[target] = green
	s.fillRed[target] = red
	s.visible[target] = true
	s.Revision++
}

func (s *GothicStage) CopyLayer(source, target int) {
	s.snapshot = nil
	from := clampGothicLayer(source)
	to := clampGothicLayer(target)
	for plane := 0; plane < gothicPlaneCount; plane++ {
		s.planes[to][plane] = s.planes[from][plane]
		s.planeX[to][plane] = s.planeX[from][plane]
		s.planeY[to][plane] = s.planeY[from][plane]
		s.planeRegion[to][plane] = s.planeRegion[from][plane]
		s.regionX[to][plane] = s.regionX[from][plane]
		s.regionY[to][plane] = s.regionY[from][plane]
		s.regionWidth[to][plane] = s.regionWidth[from][plane]
		s.regionHeight[to][plane] = s.regionHeight[from][plane]
	}
	s.filled[to] = s.filled[from]
	s.fillBlue[to] = s.fillBlue[from]
	s.fillGreen[to] = s.fillGreen[from]
	s.fillRed[to] = s.fillRed[from]
	s.visible[to] = s.visible[from]
	s.Revision++
}

func (s *GothicStage) SetMessageLines(lines []string) {
	s.messageLines = s.messageLines[:0]
	if lines != nil {
		for _, line := range lines {
			s.messageLines = append(s.messageLines, line)
		}
	}
	s.Revision++
}

func (s *GothicStage) SetMenuItems(items []string) {
	s.menuItems = s.menuItems[:0]
	if items != nil {
		for _, item := range items {
			s.menuItems = append(s.menuItems, item)
		}
	}
	s.Revision++
}

func (s *GothicStage) ClearMenuItems() {
	s.menuItems = s.menuItems[:0]
	s.Revision++
}

func (s *GothicStage) SetMessageReveal(limit int) {
	s.messageReveal = limit
	s.Revision++
}

func (s *GothicStage) SetMessageVisible(visible bool) {
	s.messageVisible = visible
	s.Revision++
}

// SetMessageWindow 设置 Toku1 的信息框底图。
//
// 底图必须在文字之前画：Compose 里先铺窗口再调用文字合成器，
// 否则 p_kaiwa 的紫色内框会把正文整块盖住。
func (s *GothicStage) SetMessageWindow(image *gfx.BsgImage, x, y int) {
	s.messageWindow = image
	s.messageWindowX = x
	s.messageWindowY = y
	s.Revision++
}

func (s *GothicStage) BeginEffect(effectType, parameter, sourceLayer, targetLayer int) {
	s.effectType = effectType
	s.effectParameter = parameter
	s.effectSource = clampGothicLayer(sourceLayer)
	s.effectTarget = clampGothicLayer(targetLayer)
	s.effectProgress = 0
	if effectType == 0 {
		s.EffectActive = false
		s.effectProgress = 1
	} else if effectType == 2 || effectType == 3 {
		s.EffectActive = true
	} else {
		core.Logf("gothic: effect type %d treated as instant", effectType)
		s.EffectActive = false
		s.effectProgress = 1
	}
	s.effectDirty = true
	s.Revision++
}

func (s *GothicStage) AdvanceEffect(elapsedSeconds float64) bool {
	if !s.EffectActive {
		return false
	}
	if math.IsNaN(elapsedSeconds) || math.IsInf(elapsedSeconds, 0) || elapsedSeconds < 0 {
		elapsedSeconds = 0
	}
	if s.effectType != 2 && s.effectType != 3 {
		s.EffectActive = false
		s.effectProgress = 1
		s.effectDirty = true
		s.Revision++
		return false
	}
	s.effectProgress += elapsedSeconds / gothicEffectDurationSeconds
	if s.effectProgress >= 1 {
		s.effectProgress = 1
		s.EffectActive = false
	}
	s.effectDirty = true
	s.Revision++
	return s.EffectActive
}

func (s *GothicStage) Compose() bool {
	if s.Revision == s.composedRevision && !s.textChanged() && !s.effectDirty {
		return false
	}
	if s.snapshot != nil {
		// 读档后的整屏快照已经包含背景和所有立绘；再画图层会让
		// 不透明背景把快照里的差分盖掉。下一次 g draw/layer 会清掉
		// snapshot，届时恢复正常图层合成。
		s.Frame.Draw(s.snapshot, 0, 0)
	} else {
		s.Frame.Clear(0, 0, 0, 255)
		fading := s.EffectActive && (s.effectType == 2 || s.effectType == 3)
		for layer := 0; layer < gothicLayerCount; layer++ {
			if fading && layer == s.effectTarget {
				continue
			}
			s.drawLayer(layer, 255)
		}
		if fading {
			s.rebuildLayer(s.effectTarget)
			if s.hasContent(s.effectTarget) {
				alpha := int(s.effectProgress * 255)
				if alpha < 0 {
					alpha = 0
				}
				if alpha > 255 {
					alpha = 255
				}
				s.Frame.BlitCanvas(s.layers[s.effectTarget], alpha)
			}
		}
	}
	if s.messageWindow != nil && s.messageVisible && len(s.messageLines) > 0 {
		s.Frame.Draw(s.messageWindow, s.messageWindowX, s.messageWindowY)
	}
	DrawGothicTextComposer(s.Frame, s.font, s.messageLines, s.messageVisible, s.menuItems, s.messageReveal)
	s.composedRevision = s.Revision
	s.snapshotText()
	s.effectDirty = false
	return true
}

func (s *GothicStage) Reset() {
	for layer := 0; layer < gothicLayerCount; layer++ {
		for plane := 0; plane < gothicPlaneCount; plane++ {
			s.planes[layer][plane] = nil
			s.planeX[layer][plane] = 0
			s.planeY[layer][plane] = 0
			s.planeRegion[layer][plane] = false
		}
		s.filled[layer] = false
		s.visible[layer] = true
		s.layers[layer].Clear(0, 0, 0, 0)
	}
	s.messageLines = s.messageLines[:0]
	s.menuItems = s.menuItems[:0]
	s.snapshot = nil
	s.messageVisible = true
	s.messageReveal = -1
	s.EffectActive = false
	s.effectType = 0
	s.effectProgress = 1
	s.effectDirty = true
	s.Frame.Clear(0, 0, 0, 255)
	s.Revision++
}

func (s *GothicStage) resolveImage(image string) *gfx.BsgImage {
	if len(image) == 0 {
		return nil
	}
	if s.catalog == nil {
		return nil
	}
	return s.catalog.Resolve(image)
}

func (s *GothicStage) drawLayer(layer, alpha int) {
	if !s.visible[layer] || !s.hasContent(layer) {
		return
	}
	s.rebuildLayer(layer)
	s.Frame.BlitCanvas(s.layers[layer], alpha)
}

func (s *GothicStage) rebuildLayer(layer int) {
	canvas := s.layers[layer]
	canvas.Clear(0, 0, 0, 0)
	if s.filled[layer] {
		canvas.Fill(0, 0, GothicScreenWidth, GothicScreenHeight, s.fillBlue[layer], s.fillGreen[layer], s.fillRed[layer], 255)
	}
	for plane := 0; plane < gothicPlaneCount; plane++ {
		image := s.planes[layer][plane]
		if image == nil {
			continue
		}
		if s.planeRegion[layer][plane] {
			canvas.DrawRegion(image, s.regionX[layer][plane], s.regionY[layer][plane], s.regionWidth[layer][plane], s.regionHeight[layer][plane], s.planeX[layer][plane]+image.OffsetX, s.planeY[layer][plane]+image.OffsetY)
		} else {
			// GSA 头部带原图在 640x480 画布上的绝对偏移。立绘差分
			// gc*.gsa 的 OffsetX 通常是 180~230，必须加上；否则角色会
			// 被贴到左上角，和原版构图完全不同。整屏背景的偏移是 0。
			canvas.Draw(image, s.planeX[layer][plane]+image.OffsetX, s.planeY[layer][plane]+image.OffsetY)
		}
	}
}

func (s *GothicStage) hasContent(layer int) bool {
	if s.filled[layer] {
		return true
	}
	for plane := 0; plane < gothicPlaneCount; plane++ {
		if s.planes[layer][plane] != nil {
			return true
		}
	}
	return false
}

func (s *GothicStage) textChanged() bool {
	if s.messageVisible != s.composedVisible {
		return true
	}
	if !equalGothicStrings(s.messageLines, s.composedMessage) {
		return true
	}
	return !equalGothicStrings(s.menuItems, s.composedMenu)
}

func (s *GothicStage) snapshotText() {
	s.composedMessage = append(s.composedMessage[:0], s.messageLines...)
	s.composedMenu = append(s.composedMenu[:0], s.menuItems...)
	s.composedVisible = s.messageVisible
}

func equalGothicStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func clampGothicLayer(layer int) int {
	if layer < 0 || layer > 7 {
		return 0
	}
	return layer
}

// Invalidate 强制下一次 Compose 重画整个舞台。
//
// 引擎自绘的系统界面（标题高亮、设置页、鉴赏网格）是直接叠在
// stage.Frame 上的覆盖层，不属于图层状态。界面切页/悬停时必须
// 让底图先重画一遍，否则上一帧的文字会和新文字叠在一起。
func (s *GothicStage) SetSnapshot(image *gfx.BsgImage) {
	s.snapshot = image
	s.Revision++
}

func (s *GothicStage) Invalidate() {
	s.Revision++
}
