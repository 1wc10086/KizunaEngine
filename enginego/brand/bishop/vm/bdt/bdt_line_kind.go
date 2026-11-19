package bdt

type BdtLineKind int

const (
	BdtLineKindEmpty BdtLineKind = iota
	BdtLineKindComment
	BdtLineKindLabel
	BdtLineKindText
	BdtLineKindVoice
	BdtLineKindCommand
	BdtLineKindUnknown
	BdtLineKindDialogue
)
