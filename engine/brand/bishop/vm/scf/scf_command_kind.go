package scf

type ScfCommandKind int

const (
	ScfCommandKindNone ScfCommandKind = iota
	ScfCommandKindRefresh
	ScfCommandKindBackground
	ScfCommandKindStandee
	ScfCommandKindMessage
	ScfCommandKindFunction
	ScfCommandKindPlayBgm
	ScfCommandKindPlaySound
	ScfCommandKindLoadScenario
	ScfCommandKindLoadGameDialog
	ScfCommandKindSaveGameDialog
	ScfCommandKindOptions
	ScfCommandKindHalt
	ScfCommandKindFaulted
	ScfCommandKindClear
)
