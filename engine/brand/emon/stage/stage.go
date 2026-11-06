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
	return hitTestNode(s.root, x, y, isSelectTarget)
}

func (s *EmonStage) HitTest(x, y int) *EmonSprite {
	return hitTestNode(s.root, x, y, isHoverTarget)
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

func hitTestNode(node *EmonSprite, x, y int, accepts spriteAccept) *EmonSprite {
	if node.flags&EmonSpriteFlagsVisible == 0 {
		return nil
	}

	children := node.children
	for index := len(children) - 1; index >= 0; index-- {
		if child := hitTestNode(children[index], x, y, accepts); child != nil {
			return child
		}
	}

	if !accepts(node) {
		return nil
	}

	if containsPoint(node, x, y) {
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

func containsPoint(sprite *EmonSprite, x, y int) bool {
	hitRect := sprite.HitRect()
	if hitRect.Width <= 0 || hitRect.Height <= 0 {
		return false
	}

	return x >= hitRect.X && x < hitRect.X+hitRect.Width && y >= hitRect.Y && y < hitRect.Y+hitRect.Height
}
