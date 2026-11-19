package gothic

type GothicActionKind int

const (
	GothicActionText GothicActionKind = iota
	GothicActionCommand
	GothicActionPause
	GothicActionWait
	GothicActionPageBreak
	GothicActionMenu
	GothicActionFinish
	GothicActionEmpty
	GothicActionWaveOpen
	GothicActionWaveWait
	GothicActionWavePlay
	GothicActionWaveChain
	GothicActionTilt
	GothicActionAniToggle
	GothicActionRmap
	GothicActionSmap
	GothicActionDmap
	GothicActionSetControl
	GothicActionInputName
)
