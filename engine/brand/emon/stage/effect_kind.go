package stage

type EmonEffectKind int

const (
	EmonEffectKindNone EmonEffectKind = iota
	EmonEffectKindMixBlend
	EmonEffectKindMaskFade
	EmonEffectKindLeftRightScroll
	EmonEffectKindTopBottomScroll
)
