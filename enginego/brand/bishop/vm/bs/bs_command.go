package bs

type BsCommandKind int

const (
	BsCommandKindNone BsCommandKind = iota
	BsCommandKindHalt
	BsCommandKindRestart
	BsCommandKindQuit
	BsCommandKindMessage
	BsCommandKindImage
	BsCommandKindWindow
	BsCommandKindClearScreen
	BsCommandKindTransition
	BsCommandKindPlayBgm
	BsCommandKindStopBgm
	BsCommandKindPlaySound
	BsCommandKindSelect
	BsCommandKindWait
	BsCommandKindSystemCall
	BsCommandKindDialog
	BsCommandKindLayerControl
	BsCommandKindLayerImage
	BsCommandKindLayerRename
	BsCommandKindStopSound
)

type BsCommand struct {
	Kind   BsCommandKind
	First  int32
	Second int32
	Third  int32
	Fourth int32
}

type BsChoice struct {
	Index       int
	MessageId   int32
	LabelOffset int
}

type BsLayerOperation int

const (
	BsLayerSwap       BsLayerOperation = 0
	BsLayerVisibility BsLayerOperation = 1
	BsLayerPosition   BsLayerOperation = 2
	BsLayerFilter     BsLayerOperation = 3
)
