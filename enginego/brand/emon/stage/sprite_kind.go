package stage

type EmonSpriteKind int

const (
	EmonSpriteKindNone        EmonSpriteKind = 0
	EmonSpriteKindGroup       EmonSpriteKind = 0x20
	EmonSpriteKindBack        EmonSpriteKind = 0x40000000
	EmonSpriteKindMiddleFront EmonSpriteKind = 0x10000000
	EmonSpriteKindMiddleBack  EmonSpriteKind = 0x20000000
)

func (k EmonSpriteKind) HasFlag(flag EmonSpriteKind) bool {
	return k&flag != 0
}
