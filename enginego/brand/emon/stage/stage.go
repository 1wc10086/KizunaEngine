package stage

import (
	"sort"
	"strings"
)

func IsMainSprite(name string) bool {
	return strings.EqualFold(name, "メイン") || strings.EqualFold(name, "Main")
}

type EmonStage struct {
	root    *EmonSprite
	motions []*EmonSpriteMotion
}

func NewEmonStage() *EmonStage {
	root := NewEmonSprite("")
	root.SetKind(EmonSpriteKindGroup)
	root.SetVisible(true)
	return &EmonStage{root: root}
}

func (s *EmonStage) Root() *EmonSprite {
	return s.root
}

func (s *EmonStage) GetOrCreate(name string, parent *EmonSprite) *EmonSprite {
	if len(name) == 0 {
		return s.root
	}

	existing := s.Find(name)
	if existing != nil {
		if parent != nil && existing != s.root && existing.parent != parent {
			current := existing.parent
			if current != nil {
				current.RemoveChild(existing)
			} else {
				removeFromRoot(s.root, existing)
			}

			parent.AddChild(existing)
		}

		return existing
	}

	created := NewEmonSprite(name)
	host := parent
	if host == nil {
		host = s.root
	}
	host.AddChild(created)
	return created
}

func (s *EmonStage) GetOrCreateKind(name string, parent *EmonSprite, kind EmonSpriteKind) *EmonSprite {
	if len(name) == 0 {
		return s.root
	}

	existing := s.Find(name)
	if existing != nil {
		// 原引擎允许 SetSprite 重定义同一名字：父级或绘制分带改变时要真的
		// 换父/重排，否则用旧父坐标计算 AbsoluteX/Y，界面会错位。
		if existing.Kind() != kind {
			existing.SetKind(kind)
			if existing.parent != nil {
				s.Reorder(existing, kind)
			}
		}
		if parent != nil && existing.parent != parent && existing != s.root {
			if existing.parent != nil {
				existing.parent.RemoveChild(existing)
			} else {
				removeFromRoot(s.root, existing)
			}
			parent.InsertChild(existing, orderIndex(parent, kind))
		}
		return existing
	}

	created := NewEmonSprite(name)
	created.SetKind(kind)
	host := parent
	if host == nil {
		host = s.root
	}
	host.InsertChild(created, orderIndex(host, kind))
	return created
}

func (s *EmonStage) Reorder(sprite *EmonSprite, placement EmonSpriteKind) {
	parent := sprite.parent
	if parent == nil {
		return
	}

	parent.RemoveChild(sprite)
	parent.InsertChild(sprite, orderIndex(parent, placement))
}

func (s *EmonStage) Find(name string) *EmonSprite {
	return s.root.Find(name)
}

func (s *EmonStage) Remove(name string) bool {
	existing := s.Find(name)
	if existing == nil || existing == s.root {
		return false
	}

	if existing.parent == nil {
		return false
	}

	return existing.parent.RemoveChild(existing)
}

func (s *EmonStage) Clear() {
	for _, child := range append([]*EmonSprite(nil), s.root.children...) {
		s.root.RemoveChild(child)
	}

	s.motions = s.motions[:0]
}

func (s *EmonStage) All() []*EmonSprite {
	all := []*EmonSprite{s.root}
	all = append(all, s.root.Descendants()...)
	return all
}

func (s *EmonStage) Motions() []*EmonSpriteMotion {
	return s.motions
}

func (s *EmonStage) AddMotion(motion *EmonSpriteMotion) {
	s.motions = append(s.motions, motion)
}

func (s *EmonStage) RemoveMotion(motion *EmonSpriteMotion) bool {
	for index, current := range s.motions {
		if current == motion {
			s.motions = append(s.motions[:index], s.motions[index+1:]...)
			return true
		}
	}

	return false
}

func (s *EmonStage) Advance(deltaSeconds float64) {
	if len(s.motions) == 0 {
		return
	}

	for index := 0; index < len(s.motions); {
		if s.motions[index].Advance(deltaSeconds) {
			s.motions = append(s.motions[:index], s.motions[index+1:]...)
			continue
		}

		index++
	}
}

func (s *EmonStage) SelectSprites() []*EmonSprite {
	var selected []*EmonSprite
	collectSelected(s.root, &selected)
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].selectOrder < selected[j].selectOrder
	})
	return selected
}

func (s *EmonStage) HitTestSelect(x, y int) *EmonSprite {
	return hitTestNode(s.root, x, y, isSelectTarget, false)
}

func (s *EmonStage) HitTest(x, y int) *EmonSprite {
	return hitTestNode(s.root, x, y, isHoverTarget, false)
}

// HitTestName 对应原引擎 GetOnSpriteName 的 sub_42A3F0/sub_42A4D0：
// 除了矩形，还采样位图 alpha（透明像素不算命中）。点击/悬停走 HitTest，
// 原引擎的选择命中只看矩形，标题菜单按钮的图形在背景图里、按钮图本身
// 是全透明的，不能采样 alpha。
func (s *EmonStage) HitTestName(x, y int) *EmonSprite {
	return hitTestNode(s.root, x, y, isHoverTarget, true)
}

func removeFromRoot(root, sprite *EmonSprite) {
	root.RemoveChild(sprite)
	for _, child := range root.children {
		removeFromRoot(child, sprite)
	}
}

func orderIndex(parent *EmonSprite, kind EmonSpriteKind) int {
	const bands = EmonSpriteKindBack | EmonSpriteKindMiddleFront | EmonSpriteKindMiddleBack
	key := kind & bands
	if key == EmonSpriteKindBack {
		return 0
	}

	children := parent.children
	index := len(children)
	if key == EmonSpriteKindMiddleBack {
		for index > 0 && children[index-1].kind&EmonSpriteKindBack == 0 {
			index--
		}
	} else if key == EmonSpriteKindMiddleFront {
		for index > 0 && children[index-1].kind&bands == 0 {
			index--
		}
	}

	return index
}

type spriteAccept func(*EmonSprite) bool

func hitTestNode(node *EmonSprite, x, y int, accepts spriteAccept, sampleAlpha bool) *EmonSprite {
	if node.flags&EmonSpriteFlagsVisible == 0 {
		return nil
	}

	children := node.children
	// 命中顺序必须与绘制顺序一致：先测最前面的 MiddleFront/普通/ MiddleBack/
	// Back 分带，同一分带内后插入的在上。否则 Back 精灵可能抢走本应属于
	// 前面按钮的点击（原引擎按绘制层级做命中）。
	for band := bandCount - 1; band >= 0; band-- {
		for index := len(children) - 1; index >= 0; index-- {
			child := children[index]
			if paintBand(child) != band {
				continue
			}
			if found := hitTestNode(child, x, y, accepts, sampleAlpha); found != nil {
				return found
			}
		}
	}

	if !accepts(node) {
		return nil
	}

	if containsPoint(node, x, y, sampleAlpha) {
		return node
	}

	return nil
}

func isSelectTarget(sprite *EmonSprite) bool {
	return !sprite.IsGroup() && sprite.flags&EmonSpriteFlagsSelect != 0 && isChainVisible(sprite)
}

func isHoverTarget(sprite *EmonSprite) bool {
	return !sprite.IsGroup() && sprite.flags&EmonSpriteFlagsSelect != 0 && isChainVisible(sprite)
}

func collectSelected(node *EmonSprite, selected *[]*EmonSprite) {
	if node.flags&EmonSpriteFlagsSelect != 0 && isChainVisible(node) {
		*selected = append(*selected, node)
	}

	for _, child := range node.children {
		collectSelected(child, selected)
	}
}

func isChainVisible(sprite *EmonSprite) bool {
	for current := sprite; current != nil; current = current.parent {
		if current.flags&EmonSpriteFlagsVisible == 0 {
			return false
		}
	}

	return true
}

func containsPoint(sprite *EmonSprite, x, y int, sampleAlpha bool) bool {
	hitRect := sprite.HitRect()
	if hitRect.Width <= 0 || hitRect.Height <= 0 {
		return false
	}
	if x < hitRect.X || x >= hitRect.X+hitRect.Width || y < hitRect.Y || y >= hitRect.Y+hitRect.Height {
		return false
	}
	if !sampleAlpha {
		return true
	}
	image := sprite.ResolveImage()
	if image == nil || image.Width <= 0 || image.Height <= 0 || len(image.Pixels) < image.Width*image.Height*4 {
		return true
	}
	// 原引擎点击命中会采样位图 alpha（sub_42A4D0 → sub_42A1C0）：
	// 透明像素不算命中，避免点按钮的空白外框也触发。
	sourceX, sourceY := 0, 0
	sourceWidth, sourceHeight := image.Width, image.Height
	if sprite.HasSourceRect() {
		sourceX = sprite.SourceX()
		sourceY = sprite.SourceY()
		sourceWidth = sprite.SourceWidth()
		sourceHeight = sprite.SourceHeight()
	}
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return true
	}
	localX := x - hitRect.X
	localY := y - hitRect.Y
	var sampleX, sampleY int
	if sprite.Flags()&EmonSpriteFlagsTile != 0 {
		sampleX = sourceX + mod(localX, sourceWidth)
		sampleY = sourceY + mod(localY, sourceHeight)
	} else {
		sampleX = sourceX + localX*sourceWidth/hitRect.Width
		sampleY = sourceY + localY*sourceHeight/hitRect.Height
	}
	if sampleX < 0 || sampleX >= image.Width || sampleY < 0 || sampleY >= image.Height {
		return true
	}
	alpha := image.Pixels[(sampleY*image.Width+sampleX)*4+3]
	return alpha != 0
}
