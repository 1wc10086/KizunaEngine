package stage

import (
	"math"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/core"
)

const (
	backBand        = 0
	middleBackBand  = 1
	normalBand      = 2
	middleFrontBand = 3
	bandCount       = 4
)

type EmonStageComposer struct {
	current      *bishopgfx.BgraCanvas
	snapshot     *bishopgfx.BgraCanvas
	scratch      *bishopgfx.BgraCanvas
	backgroundB  uint8
	backgroundG  uint8
	backgroundR  uint8
	painted      map[*EmonSprite]struct{}
	revision     int
	effect       *EmonEffect
	glyphSource  emontext.GlyphSource
	maskPatterns MaskPatternSource
	textStyle    *emontext.EmonTextStyle
	frameDelta   float64
	hasSnapshot  bool
}

func NewEmonStageComposer(width, height int) *EmonStageComposer {
	current := bishopgfx.NewBgraCanvas(width, height)
	current.Clear(0, 0, 0, 255)
	return &EmonStageComposer{
		current:    current,
		painted:    make(map[*EmonSprite]struct{}),
		textStyle:  emontext.NewEmonTextStyle(),
		frameDelta: 1.0 / 60.0,
	}
}

func (c *EmonStageComposer) Width() int {
	return c.current.Width
}

func (c *EmonStageComposer) Height() int {
	return c.current.Height
}

func (c *EmonStageComposer) Current() *bishopgfx.BgraCanvas {
	return c.current
}

func (c *EmonStageComposer) SnapshotCanvas() *bishopgfx.BgraCanvas {
	if c.snapshot == nil {
		c.snapshot = c.createBlack()
	}
	return c.snapshot
}

func (c *EmonStageComposer) HasSnapshot() bool {
	return c.hasSnapshot
}

func (c *EmonStageComposer) Revision() int {
	return c.revision
}

func (c *EmonStageComposer) Effect() *EmonEffect {
	return c.effect
}

func (c *EmonStageComposer) GlyphSource() emontext.GlyphSource {
	return c.glyphSource
}

func (c *EmonStageComposer) SetGlyphSource(source emontext.GlyphSource) {
	c.glyphSource = source
}

func (c *EmonStageComposer) MaskPatterns() MaskPatternSource {
	return c.maskPatterns
}

func (c *EmonStageComposer) SetMaskPatterns(patterns MaskPatternSource) {
	c.maskPatterns = patterns
}

func (c *EmonStageComposer) TextStyle() *emontext.EmonTextStyle {
	return c.textStyle
}

func (c *EmonStageComposer) FrameDelta() float64 {
	return c.frameDelta
}

func (c *EmonStageComposer) SetFrameDelta(value float64) {
	c.frameDelta = value
}

func (c *EmonStageComposer) SetEffect(effect *EmonEffect) {
	if c.effect == effect {
		return
	}

	c.effect = effect
	c.revision++
}

func (c *EmonStageComposer) SetBackground(b, g, r int) {
	c.backgroundB = toChannel(b)
	c.backgroundG = toChannel(g)
	c.backgroundR = toChannel(r)
	c.current.Clear(c.backgroundB, c.backgroundG, c.backgroundR, 255)
	c.revision++
}

func (c *EmonStageComposer) Compose(stage *EmonStage, target *bishopgfx.BgraCanvas) {
	c.current.Clear(c.backgroundB, c.backgroundG, c.backgroundR, 255)
	clear(c.painted)
	rootTint := 0xFFFFFF
	rootAmount := 0
	if stage.root.colorAmount > 0 {
		rootTint = stage.root.tint
		rootAmount = stage.root.colorAmount
	}
	c.paintChildren(stage.root, 0, 0, 255, rootTint, rootAmount, 0, 0, 0, 0, false, true)

	if c.effect != nil {
		c.effect.Advance(c.frameDelta)
		c.effect.ApplyWith(c.SnapshotCanvas(), c.current, c.current, c.scratchCanvas(), c.maskPatterns)
		if !c.effect.Active() {
			c.effect = nil
		}
	}

	copyComposerCanvas(c.current, target)
}

func (c *EmonStageComposer) Snapshot() {
	c.SnapshotCanvas().CopyFrom(c.current)
	c.hasSnapshot = true
	c.revision++
}

func (c *EmonStageComposer) BeginEffect(kind EmonEffectKind, frames, param2, param3 int) *EmonEffect {
	if kind == EmonEffectKindNone || frames <= 0 {
		return nil
	}

	return NewEmonEffect(kind, frames, param2, param3)
}

func (c *EmonStageComposer) paintChildren(parent *EmonSprite, parentX, parentY, parentAlpha, parentTint, parentAmount, clipLeft, clipTop, clipRight, clipBottom int, hasClip, chainVisible bool) {
	visible := chainVisible && parent.Visible()
	if !visible {
		return
	}

	children := parent.children
	count := len(children)
	if count == 0 {
		return
	}

	for band := 0; band < bandCount; band++ {
		for index := 0; index < count; index++ {
			child := children[index]
			if !child.Visible() || paintBand(child) != band {
				continue
			}

			c.paintNode(child, parentX, parentY, parentAlpha, parentTint, parentAmount, clipLeft, clipTop, clipRight, clipBottom, hasClip, true)
		}
	}
}

func (c *EmonStageComposer) paintNode(node *EmonSprite, parentX, parentY, parentAlpha, parentTint, parentAmount, clipLeft, clipTop, clipRight, clipBottom int, hasClip, chainVisible bool) {
	alpha := node.alpha * parentAlpha / 255
	if alpha <= 0 {
		return
	}

	if !chainVisible || !node.Visible() {
		return
	}

	if _, painted := c.painted[node]; painted {
		return
	}
	c.painted[node] = struct{}{}

	tint := parentTint
	amount := parentAmount
	if node.colorAmount > 0 {
		tint = node.tint
		amount = node.colorAmount
	}

	x := parentX + node.x
	y := parentY + node.y

	if image := node.ResolveImage(); image != nil {
		c.drawSprite(node, image, x, y, alpha, tint, amount, clipLeft, clipTop, clipRight, clipBottom, hasClip)
	}

	if setting := node.textSetting; setting != nil {
		if text := node.text; text != nil && !text.IsEmpty() {
			glyphs := c.glyphSource
			if glyphs == nil {
				glyphs = emontext.ResolveGlyphSource()
			}
			emontext.DrawSpriteText(c.current, setting, text.Content(), x+setting.X(), y+setting.Y(), alpha, glyphs, clipLeft, clipTop, clipRight, clipBottom, hasClip)
		}
	}

	c.paintChildren(node, x, y, alpha, tint, amount, clipLeft, clipTop, clipRight, clipBottom, hasClip, true)
}

func (c *EmonStageComposer) drawSprite(sprite *EmonSprite, image *bishopgfx.BsgImage, x, y, alpha, tint, amount, clipLeft, clipTop, clipRight, clipBottom int, hasClip bool) {
	if image.Width <= 0 || image.Height <= 0 || alpha <= 0 {
		return
	}

	tiled := sprite.flags.HasFlag(EmonSpriteFlagsTile)
	sourceX := 0
	sourceY := 0
	sourceWidth := image.Width
	sourceHeight := image.Height
	if sprite.hasSourceRect {
		sourceX = core.Clamp(sprite.sourceX, 0, image.Width)
		sourceY = core.Clamp(sprite.sourceY, 0, image.Height)
		sourceWidth = min(sprite.sourceWidth, image.Width-sourceX)
		sourceHeight = min(sprite.sourceHeight, image.Height-sourceY)
		if sourceWidth <= 0 || sourceHeight <= 0 {
			return
		}
	}

	displayWidth := sprite.width
	if displayWidth <= 0 {
		displayWidth = sourceWidth
	}
	displayHeight := sprite.height
	if displayHeight <= 0 {
		displayHeight = sourceHeight
	}

	var width int
	var height int
	if tiled {
		width = max(sourceWidth, displayWidth)
		height = max(sourceHeight, displayHeight)
	} else {
		width = min(displayWidth, sourceWidth)
		height = min(displayHeight, sourceHeight)
	}

	mask := sprite.ResolveMask()
	if f3d := sprite.f3d; f3d != nil && mask == nil && amount == 0 && !tiled {
		if c.tryDrawF3DSprite(image, f3d, x, y, width, height, sourceX, sourceY, sourceWidth, sourceHeight, alpha, clipLeft, clipTop, clipRight, clipBottom, hasClip) {
			return
		}
	}

	left := max(x, 0)
	top := max(y, 0)
	right := min(x+width, c.current.Width)
	bottom := min(y+height, c.current.Height)

	if hasClip {
		left = max(left, clipLeft)
		top = max(top, clipTop)
		right = min(right, clipRight)
		bottom = min(bottom, clipBottom)
	}

	if left >= right || top >= bottom {
		return
	}

	if mask == nil && amount == 0 && !tiled {
		c.current.BlitRegion(image, sourceX+left-x, sourceY+top-y, right-left, bottom-top, left, top, alpha)
		return
	}

	if mask == nil && amount == 0 {
		c.drawTiled(image, sourceX, sourceY, sourceWidth, sourceHeight, x, y, left, top, right, bottom, alpha)
		return
	}

	c.stageProcessed(image, mask, sourceX, sourceY, sourceWidth, sourceHeight, x, y, tint, amount, left, top, right, bottom, alpha)
}

func (c *EmonStageComposer) tryDrawF3DSprite(image *bishopgfx.BsgImage, f3d *EmonF3DState, x, y, width, height, sourceX, sourceY, sourceWidth, sourceHeight, alpha, clipLeft, clipTop, clipRight, clipBottom int, hasClip bool) bool {
	f3d.UpdateBounds(float64(x), float64(y), float64(width), float64(height))
	if !f3d.bounds.HasValue() {
		return false
	}

	p00X, p00Y, ok := projectF3DCorner(f3d, float64(x), float64(y))
	if !ok {
		return false
	}

	p10X, p10Y, ok := projectF3DCorner(f3d, float64(x+width), float64(y))
	if !ok {
		return false
	}

	p01X, p01Y, ok := projectF3DCorner(f3d, float64(x), float64(y+height))
	if !ok {
		return false
	}

	p11X, p11Y, ok := projectF3DCorner(f3d, float64(x+width), float64(y+height))
	if !ok {
		return false
	}

	return c.drawF3DQuad(image, sourceX, sourceY, sourceWidth, sourceHeight, p00X, p00Y, p10X, p10Y, p01X, p01Y, p11X, p11Y, alpha, clipLeft, clipTop, clipRight, clipBottom, hasClip)
}

func projectF3DCorner(f3d *EmonF3DState, localX, localY float64) (float64, float64, bool) {
	worldX, worldY, worldZ := f3d.ToWorld(localX, localY)
	return F3DProjectorProjectToScreen(f3d.cameraX, f3d.cameraY, f3d.cameraZ, worldX, worldY, worldZ)
}

func (c *EmonStageComposer) drawF3DQuad(image *bishopgfx.BsgImage, sourceX, sourceY, sourceWidth, sourceHeight int, p00X, p00Y, p10X, p10Y, p01X, p01Y, p11X, p11Y float64, alpha, clipLeft, clipTop, clipRight, clipBottom int, hasClip bool) bool {
	minX := math.Min(math.Min(p00X, p10X), math.Min(p01X, p11X))
	minY := math.Min(math.Min(p00Y, p10Y), math.Min(p01Y, p11Y))
	maxX := math.Max(math.Max(p00X, p10X), math.Max(p01X, p11X))
	maxY := math.Max(math.Max(p00Y, p10Y), math.Max(p01Y, p11Y))

	left := int(math.Floor(minX))
	top := int(math.Floor(minY))
	right := int(math.Ceil(maxX))
	bottom := int(math.Ceil(maxY))

	left = max(left, 0)
	top = max(top, 0)
	right = min(right, c.current.Width)
	bottom = min(bottom, c.current.Height)

	if hasClip {
		left = max(left, clipLeft)
		top = max(top, clipTop)
		right = min(right, clipRight)
		bottom = min(bottom, clipBottom)
	}

	if left >= right || top >= bottom {
		return false
	}

	ax := p10X - p00X
	ay := p10Y - p00Y
	bx := p01X - p00X
	by := p01Y - p00Y
	det := ax*by - ay*bx
	if det == 0.0 {
		return false
	}

	invDet := 1.0 / det
	imageWidth := image.Width
	imageHeight := image.Height

	for row := top; row < bottom; row++ {
		dy := float64(row) + 0.5 - p00Y

		for column := left; column < right; column++ {
			dx := float64(column) + 0.5 - p00X
			u := (dx*by - dy*bx) * invDet
			v := (dy*ax - dx*ay) * invDet

			if u < 0.0 || u > 1.0 || v < 0.0 || v > 1.0 {
				continue
			}

			sxInt := sourceX + int(u*float64(sourceWidth))
			syInt := sourceY + int(v*float64(sourceHeight))
			if uint(sxInt) >= uint(imageWidth) || uint(syInt) >= uint(imageHeight) {
				continue
			}

			c.current.BlitRegion(image, sxInt, syInt, 1, 1, column, row, alpha)
		}
	}

	return true
}

func (c *EmonStageComposer) drawTiled(image *bishopgfx.BsgImage, sourceX, sourceY, sourceWidth, sourceHeight, x, y, left, top, right, bottom, alpha int) {
	for tileTop := top - mod(top-y, sourceHeight); tileTop < bottom; tileTop += sourceHeight {
		destY := max(tileTop, top)
		rows := min(tileTop+sourceHeight, bottom) - destY
		if rows <= 0 {
			continue
		}

		for tileLeft := left - mod(left-x, sourceWidth); tileLeft < right; tileLeft += sourceWidth {
			destX := max(tileLeft, left)
			columns := min(tileLeft+sourceWidth, right) - destX
			if columns <= 0 {
				continue
			}

			c.current.BlitRegion(image, sourceX+destX-tileLeft, sourceY+destY-tileTop, columns, rows, destX, destY, alpha)
		}
	}
}

func (c *EmonStageComposer) stageProcessed(image, mask *bishopgfx.BsgImage, sourceX, sourceY, sourceWidth, sourceHeight, x, y, color, amount, left, top, right, bottom, alpha int) {
	spanWidth := right - left
	spanHeight := bottom - top
	scratch := c.scratchCanvas()
	scratch.Fill(left, top, spanWidth, spanHeight, 0, 0, 0, 0)
	staged := scratch.Buffer
	stagedWidth := scratch.Width
	source := image.Pixels
	sourceStride := image.Width * 4

	var maskPixels []byte
	maskWidth := 1
	maskHeight := 1
	if mask != nil {
		maskPixels = mask.Pixels
		maskWidth = mask.Width
		maskHeight = mask.Height
	}

	tintBlue := color & 0xFF
	tintGreen := (color >> 8) & 0xFF
	tintRed := (color >> 16) & 0xFF

	for row := top; row < bottom; row++ {
		sourceRow := sourceY + mod(row-y, sourceHeight)
		maskRow := mod(sourceY+(row-y), maskHeight)

		for column := left; column < right; column++ {
			sourceColumn := sourceX + mod(column-x, sourceWidth)
			sourceOffset := sourceRow*sourceStride + sourceColumn*4
			sourceAlpha := source[sourceOffset+3]
			if sourceAlpha == 0 {
				continue
			}

			maskAlpha := 255
			if maskPixels != nil {
				maskColumn := mod(sourceX+(column-x), maskWidth)
				maskOffset := maskRow*maskWidth*4 + maskColumn*4
				maskAlpha = Luminance(maskPixels, maskOffset)
				if maskAlpha == 0 {
					continue
				}
			}

			blended := int(sourceAlpha) * maskAlpha * alpha / 65025
			if blended <= 0 {
				continue
			}

			stagedOffset := (row*stagedWidth + column) * 4
			staged[stagedOffset] = byte(int(source[sourceOffset]) + ((tintBlue-int(source[sourceOffset]))*amount)/256)
			staged[stagedOffset+1] = byte(int(source[sourceOffset+1]) + ((tintGreen-int(source[sourceOffset+1]))*amount)/256)
			staged[stagedOffset+2] = byte(int(source[sourceOffset+2]) + ((tintRed-int(source[sourceOffset+2]))*amount)/256)
			staged[stagedOffset+3] = byte(blended)
		}
	}

	c.current.BlitCanvasRegion(scratch, left, top, spanWidth, spanHeight, left, top, 255)
}

func (c *EmonStageComposer) scratchCanvas() *bishopgfx.BgraCanvas {
	if c.scratch == nil {
		c.scratch = c.createBlack()
	}
	return c.scratch
}

func (c *EmonStageComposer) createBlack() *bishopgfx.BgraCanvas {
	canvas := bishopgfx.NewBgraCanvas(c.current.Width, c.current.Height)
	canvas.Clear(0, 0, 0, 255)
	return canvas
}

func copyComposerCanvas(source, target *bishopgfx.BgraCanvas) {
	if source == target {
		return
	}

	if source.Width == target.Width && source.Height == target.Height {
		target.CopyFrom(source)
		return
	}

	width := min(source.Width, target.Width)
	height := min(source.Height, target.Height)
	sourcePixels := source.Buffer
	targetPixels := target.Buffer
	target.Clear(0, 0, 0, 255)

	for y := 0; y < height; y++ {
		copy(targetPixels[y*target.Width*4:], sourcePixels[y*source.Width*4:y*source.Width*4+width*4])
	}
}

func paintBand(sprite *EmonSprite) int {
	if sprite.kind.HasFlag(EmonSpriteKindMiddleBack) {
		return middleBackBand
	}

	if sprite.kind.HasFlag(EmonSpriteKindMiddleFront) {
		return middleFrontBand
	}

	if sprite.kind.HasFlag(EmonSpriteKindBack) || sprite.flags.HasFlag(EmonSpriteFlagsBack) {
		return backBand
	}

	return normalBand
}

func mod(value, extent int) int {
	if extent <= 1 {
		return 0
	}

	remainder := value % extent
	if remainder < 0 {
		remainder += extent
	}
	return remainder
}

func toChannel(value int) uint8 {
	return uint8(core.Clamp(value, 0, 255))
}
