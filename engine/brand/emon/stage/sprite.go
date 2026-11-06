package stage

import (
	"strings"
	"sync/atomic"

	bishopgfx "kizuna/engine/brand/bishop/gfx"
	emontext "kizuna/engine/brand/emon/text"
	"kizuna/engine/core"
)

var (
	selectSequence atomic.Int32
	spriteRevision atomic.Int64
)

func EmonSpriteRevision() int64 {
	return spriteRevision.Load()
}

type SpriteRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

type EmonSprite struct {
	name string

	parent   *EmonSprite
	children []*EmonSprite

	flags       EmonSpriteFlags
	kind        EmonSpriteKind
	selectOrder int
	alpha       int
	tint        int
	colorAmount int

	image       *bishopgfx.BsgImage
	mask        *bishopgfx.BsgImage
	imageSource *EmonSprite
	f3d         *EmonF3DState

	textSetting  *emontext.EmonSpriteTextSetting
	text         *emontext.EmonSpriteText
	textRevision int

	x      int
	y      int
	width  int
	height int

	hasSourceRect bool
	sourceX       int
	sourceY       int
	sourceWidth   int
	sourceHeight  int
}

func NewEmonSprite(name string) *EmonSprite {
	return &EmonSprite{name: name, alpha: 255, tint: 0xFFFFFF}
}

func NewEmonSpriteSize(name string, width, height int) *EmonSprite {
	return &EmonSprite{name: name, alpha: 255, tint: 0xFFFFFF, width: width, height: height}
}

func (s *EmonSprite) Name() string {
	return s.name
}

func (s *EmonSprite) Parent() *EmonSprite {
	return s.parent
}

func (s *EmonSprite) Children() []*EmonSprite {
	return s.children
}

func (s *EmonSprite) Kind() EmonSpriteKind {
	return s.kind
}

func (s *EmonSprite) SetKind(value EmonSpriteKind) {
	s.kind = value
	s.touch()
}

func (s *EmonSprite) Flags() EmonSpriteFlags {
	return s.flags
}

func (s *EmonSprite) SetFlags(value EmonSpriteFlags) {
	if value&EmonSpriteFlagsSelect != 0 && s.selectOrder == 0 {
		s.selectOrder = int(selectSequence.Add(1))
	}

	s.flags = value
	s.touch()
}

func (s *EmonSprite) X() int {
	return s.x
}

func (s *EmonSprite) SetX(value int) {
	s.x = value
	s.touch()
}

func (s *EmonSprite) Y() int {
	return s.y
}

func (s *EmonSprite) SetY(value int) {
	s.y = value
	s.touch()
}

func (s *EmonSprite) Width() int {
	return s.width
}

func (s *EmonSprite) SetWidth(value int) {
	s.width = value
	s.touch()
}

func (s *EmonSprite) Height() int {
	return s.height
}

func (s *EmonSprite) SetHeight(value int) {
	s.height = value
	s.touch()
}

func (s *EmonSprite) HasSourceRect() bool {
	return s.hasSourceRect
}

func (s *EmonSprite) SourceX() int {
	return s.sourceX
}

func (s *EmonSprite) SourceY() int {
	return s.sourceY
}

func (s *EmonSprite) SourceWidth() int {
	return s.sourceWidth
}

func (s *EmonSprite) SourceHeight() int {
	return s.sourceHeight
}

func (s *EmonSprite) SetSourceRect(x, y, width, height int) {
	s.sourceX = x
	s.sourceY = y
	s.sourceWidth = width
	s.sourceHeight = height
	s.hasSourceRect = true
	s.width = width
	s.height = height
	s.touch()
}

func (s *EmonSprite) ClearSourceRect() {
	s.hasSourceRect = false
	s.sourceX = 0
	s.sourceY = 0
	s.sourceWidth = 0
	s.sourceHeight = 0
	s.touch()
}

func (s *EmonSprite) Image() *bishopgfx.BsgImage {
	return s.image
}

func (s *EmonSprite) SetImage(value *bishopgfx.BsgImage) {
	s.image = value
	s.touch()
}

func (s *EmonSprite) Mask() *bishopgfx.BsgImage {
	return s.mask
}

func (s *EmonSprite) SetMask(value *bishopgfx.BsgImage) {
	s.mask = value
	s.touch()
}

func (s *EmonSprite) ImageSource() *EmonSprite {
	return s.imageSource
}

func (s *EmonSprite) SetImageSource(value *EmonSprite) {
	s.imageSource = value
	s.touch()
}

func (s *EmonSprite) F3D() *EmonF3DState {
	return s.f3d
}

func (s *EmonSprite) SetF3D(value *EmonF3DState) {
	s.f3d = value
	s.touch()
}

func (s *EmonSprite) TextSetting() *emontext.EmonSpriteTextSetting {
	return s.textSetting
}

func (s *EmonSprite) SetTextSetting(value *emontext.EmonSpriteTextSetting) {
	if s.textSetting == value {
		return
	}

	s.textSetting = value
	s.textRevision++
	s.touch()
}

func (s *EmonSprite) Text() *emontext.EmonSpriteText {
	return s.text
}

func (s *EmonSprite) SetText(value *emontext.EmonSpriteText) {
	if s.text == value {
		return
	}

	s.text = value
	s.textRevision++
	s.touch()
}

func (s *EmonSprite) TextRevision() int {
	revision := s.textRevision
	if s.text != nil {
		revision += s.text.Revision()
	}
	if s.textSetting != nil {
		revision += s.textSetting.Revision()
	}
	return revision
}

func (s *EmonSprite) InvalidateText() {
	s.textRevision++
	s.touch()
}

func (s *EmonSprite) ResolveImage() *bishopgfx.BsgImage {
	current := s
	for depth := 0; depth < 16; depth++ {
		if current.image != nil {
			return current.image
		}

		current = current.imageSource
		if current == nil {
			return nil
		}
	}

	return nil
}

func (s *EmonSprite) ResolveMask() *bishopgfx.BsgImage {
	current := s
	for depth := 0; depth < 16; depth++ {
		if current.mask != nil {
			return current.mask
		}

		current = current.imageSource
		if current == nil {
			return nil
		}
	}

	return nil
}

func (s *EmonSprite) Tint() int {
	return s.tint
}

func (s *EmonSprite) SetTint(value int) {
	s.tint = value
	s.touch()
}

func (s *EmonSprite) ColorAmount() int {
	return s.colorAmount
}

func (s *EmonSprite) SetColorAmount(value int) {
	clamped := core.Clamp(value, 0, 256)
	if s.colorAmount == clamped {
		return
	}

	s.colorAmount = clamped
	s.touch()
}

func (s *EmonSprite) Alpha() int {
	return s.alpha
}

func (s *EmonSprite) SetAlpha(value int) {
	s.alpha = core.Clamp(value, 0, 255)
	s.touch()
}

func (s *EmonSprite) SelectOrder() int {
	return s.selectOrder
}

func (s *EmonSprite) SetSelectOrder(value int) {
	s.selectOrder = value
	s.touch()
}

func (s *EmonSprite) Visible() bool {
	return s.flags&EmonSpriteFlagsVisible != 0
}

func (s *EmonSprite) SetVisible(value bool) {
	if value {
		s.SetFlags(s.flags | EmonSpriteFlagsVisible)
		return
	}

	s.SetFlags(s.flags &^ EmonSpriteFlagsVisible)
}

func (s *EmonSprite) IsGroup() bool {
	return s.kind&EmonSpriteKindGroup != 0
}

func (s *EmonSprite) AbsoluteX() int {
	x := 0
	for current := s; current != nil; current = current.parent {
		x += current.x
	}

	return x
}

func (s *EmonSprite) AbsoluteY() int {
	y := 0
	for current := s; current != nil; current = current.parent {
		y += current.y
	}

	return y
}

func (s *EmonSprite) HitRect() SpriteRect {
	width := s.width
	if width <= 0 {
		if image := s.ResolveImage(); image != nil {
			width = image.Width
		}
	}

	height := s.height
	if height <= 0 {
		if image := s.ResolveImage(); image != nil {
			height = image.Height
		}
	}

	return SpriteRect{X: s.AbsoluteX(), Y: s.AbsoluteY(), Width: width, Height: height}
}

func (s *EmonSprite) SetSize(width, height int) {
	s.width = width
	s.height = height
	s.touch()
}

func (s *EmonSprite) AddChild(child *EmonSprite) {
	s.InsertChild(child, len(s.children))
}

func (s *EmonSprite) InsertChild(child *EmonSprite, index int) {
	if child == s || s.IsDescendantOf(child) {
		panic("stage: cannot parent '" + child.name + "' under '" + s.name + "'")
	}

	if child.parent != nil {
		child.parent.RemoveChild(child)
	}

	s.removeChildRef(child)
	child.parent = s
	index = core.Clamp(index, 0, len(s.children))
	s.children = append(s.children, nil)
	copy(s.children[index+1:], s.children[index:])
	s.children[index] = child
	s.touch()
}

func (s *EmonSprite) RemoveChild(child *EmonSprite) bool {
	if !s.removeChildRef(child) {
		return false
	}

	child.parent = nil
	s.touch()
	return true
}

func (s *EmonSprite) Find(name string) *EmonSprite {
	if strings.EqualFold(s.name, name) {
		return s
	}

	for _, child := range s.children {
		if match := child.Find(name); match != nil {
			return match
		}
	}

	return nil
}

func (s *EmonSprite) Descendants() []*EmonSprite {
	var descendants []*EmonSprite
	for _, child := range s.children {
		descendants = append(descendants, child)
		descendants = append(descendants, child.Descendants()...)
	}

	return descendants
}

func (s *EmonSprite) touch() {
	spriteRevision.Add(1)
}

func (s *EmonSprite) IsDescendantOf(candidate *EmonSprite) bool {
	for ancestor := s.parent; ancestor != nil; ancestor = ancestor.parent {
		if ancestor == candidate {
			return true
		}
	}

	return false
}

func (s *EmonSprite) removeChildRef(child *EmonSprite) bool {
	for index, current := range s.children {
		if current == child {
			s.children = append(s.children[:index], s.children[index+1:]...)
			return true
		}
	}

	return false
}
