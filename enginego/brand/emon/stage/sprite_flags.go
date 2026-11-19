package stage

type EmonSpriteFlags int

const (
	EmonSpriteFlagsNone       EmonSpriteFlags = 0
	EmonSpriteFlagsVisible    EmonSpriteFlags = 0x10000
	EmonSpriteFlagsClip       EmonSpriteFlags = 4
	EmonSpriteFlagsSelect     EmonSpriteFlags = 0x10
	EmonSpriteFlagsEffectOnly EmonSpriteFlags = 8
	EmonSpriteFlagsDrawMe     EmonSpriteFlags = 2
	EmonSpriteFlagsClearMe    EmonSpriteFlags = 0x200000
	EmonSpriteFlagsBack       EmonSpriteFlags = 0x40
	EmonSpriteFlagsTile       EmonSpriteFlags = 0x100000
	EmonSpriteFlagsMotion     EmonSpriteFlags = 0x800000
	EmonSpriteFlagsAnimation  EmonSpriteFlags = 1
)

func (f EmonSpriteFlags) HasFlag(flag EmonSpriteFlags) bool {
	return f&flag != 0
}
