package vm

import (
	"kizuna/engine/brand/emon/save"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	emontext "kizuna/engine/brand/emon/text"
)

type EmonStep int

const (
	EmonStepContinue EmonStep = iota
	EmonStepAwaitInput
	EmonStepAwaitChoice
	EmonStepAwaitAction
	EmonStepAwaitName
	EmonStepWait
	EmonStepFinished
	EmonStepFaulted
)

type EmonFrame struct {
	Program *script.EmonProgram
	Macro   *script.EmonMacroFrame
	Index   int32
}

type EmonRunAction struct {
	Name string
	Ops  []EmonActionOp
}

type EmonActionOp struct {
	Code string
	Args []string
}

type EmonRect struct {
	X1 int
	Y1 int
	X2 int
	Y2 int
}

type EmonActionKind int

const (
	EmonActionNone EmonActionKind = iota
	EmonActionClick
	EmonActionMiddleClick
	EmonActionRightClick
	EmonActionWheelUp
	EmonActionWheelDown
	EmonActionKey
	EmonActionFlagOn
	EmonActionFlagOff
	EmonActionCondition
	EmonActionActWaitOff
	EmonActionSpriteClick
	EmonActionSpriteOver
	EmonActionSpriteOut
	EmonActionSpriteMove
	EmonActionSpriteDrag
	EmonActionSpriteOn
	EmonActionSpriteOff
	EmonActionSpriteAnmOn
	EmonActionSpriteAnmOff
	EmonActionEffectOn
	EmonActionMotionOn
	EmonActionMotionOff
	EmonActionSoundEnd
)

type EmonInputKind int

const (
	EmonInputNone EmonInputKind = iota
	EmonInputLeftClick
	EmonInputMiddleClick
	EmonInputRightClick
	EmonInputWheelUp
	EmonInputWheelDown
	EmonInputKey
)

type EmonActionTrigger struct {
	Name                    string
	Kind                    EmonActionKind
	Selector                string
	Condition               string
	Handler                 string
	KeyCode                 int
	AllowLocked             bool
	SoundVoice              bool
	SoundEffectChannel      bool
	SoundEffectChannelIndex int
	OneTime                 bool
	Latched                 bool
}

func NewEmonActionTrigger() *EmonActionTrigger {
	return &EmonActionTrigger{SoundEffectChannelIndex: -1}
}

type EmonBackLogWindow struct {
	Outer               *stage.EmonSprite
	Inner               *stage.EmonSprite
	Cursor              *stage.EmonSprite
	Setting             *emontext.EmonSpriteTextSetting
	Metrics             *emontext.EmonSpriteTextMetrics
	OuterVisible        bool
	InnerVisible        bool
	Capacity            int
	Maximum             int
	Top                 int
	Zone                int
	Rects               [4]EmonRect
	OffsetX             [4]int
	OffsetY             [4]int
	CursorHasSourceRect bool
	CursorSourceX       int
	CursorSourceY       int
	CursorSourceWidth   int
	CursorSourceHeight  int
	SavedCursorX        int
	SavedCursorY        int
	SavedCursorSprite   string

	lines         []string
	rendered      string
	renderedTop   int
	renderedCount int
}

func NewEmonBackLogWindow(outer *stage.EmonSprite, inner *stage.EmonSprite, cursor *stage.EmonSprite, setting *emontext.EmonSpriteTextSetting) *EmonBackLogWindow {
	metrics := setting.Metrics()
	capacity := metrics.MaxLines
	if capacity < 1 {
		capacity = 1
	}
	outerVisible := false
	if outer != nil {
		outerVisible = outer.Visible()
	}
	return &EmonBackLogWindow{
		Outer:         outer,
		Inner:         inner,
		Cursor:        cursor,
		Setting:       setting,
		Metrics:       metrics,
		OuterVisible:  outerVisible,
		InnerVisible:  inner.Visible(),
		Capacity:      capacity,
		Zone:          -1,
		renderedTop:   -1,
		renderedCount: -1,
	}
}

var backlogZoneOrder = [4]int{0, 3, 2, 1}

func (w *EmonBackLogWindow) ZoneArgument() int {
	if w.Zone >= 0 {
		return backlogZoneOrder[w.Zone]
	}
	return -1
}

func (w *EmonBackLogWindow) Build(history []string, glyphs emontext.GlyphSource) string {
	if w.renderedTop == w.Top && w.renderedCount == len(history) {
		return w.rendered
	}
	w.lines = w.lines[:0]
	for index := w.Top; index < len(history) && len(w.lines) < w.Capacity; index++ {
		wrapped := emontext.WrapSpriteText(w.Setting, history[index], glyphs)
		for line := 0; line < len(wrapped) && len(w.lines) < w.Capacity; line++ {
			w.lines = append(w.lines, wrapped[line])
		}
	}
	rendered := ""
	for i, line := range w.lines {
		if i != 0 {
			rendered += "\n"
		}
		rendered += line
	}
	w.rendered = rendered
	w.renderedTop = w.Top
	w.renderedCount = len(history)
	return w.rendered
}

type EmonSaveData = save.EmonSaveData
