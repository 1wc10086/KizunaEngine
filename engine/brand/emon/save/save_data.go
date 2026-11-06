package save

import (
	"time"

	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
)

type EmonSpriteState struct {
	Name    string
	Visible bool
	Flags   stage.EmonSpriteFlags
	X       int32
	Y       int32
	Width   int32
	Height  int32
	Kind    stage.EmonSpriteKind
	Alpha   int32
	F3D     *stage.EmonF3DSnapshot `json:"F3D,omitempty"`
}

func NewEmonSpriteState() *EmonSpriteState {
	return &EmonSpriteState{Visible: true, Alpha: 255}
}

type EmonFrameState struct {
	Program         string
	Index           int32
	Macro           string
	CallerProgram   string
	CallerStatement int32
	ReturnStatement int32
	Arguments       map[string]script.EmonValue
}

type EmonSystemStatePayload struct {
	Version         int32
	SystemVariables map[string]script.EmonValue
}

type EmonSavePayload struct {
	Slot            int32
	Comment         string
	SavedAt         time.Time
	ScriptFile      string
	StatementIndex  int32
	Variables       map[string]script.EmonValue
	SystemVariables map[string]script.EmonValue
	MacroStack      []*script.EmonMacroFrameState
	Frames          []*EmonFrameState
	AudioState      map[string]string
	SpriteStates    []*EmonSpriteState
	Preview         string
}

type EmonSaveData struct {
	Slot            int32
	Comment         string
	SavedAt         time.Time
	ScriptFile      string
	StatementIndex  int32
	Variables       map[string]script.EmonValue
	SystemVariables map[string]script.EmonValue
	MacroStack      []*script.EmonMacroFrameState
	Frames          []*EmonFrameState
	AudioState      map[string]string
	SpriteStates    []*EmonSpriteState
	Preview         string
}

func EmonSaveDataFromPayload(payload *EmonSavePayload) *EmonSaveData {
	if payload == nil {
		return nil
	}
	return &EmonSaveData{
		Slot:            payload.Slot,
		Comment:         payload.Comment,
		SavedAt:         payload.SavedAt,
		ScriptFile:      payload.ScriptFile,
		StatementIndex:  payload.StatementIndex,
		Variables:       copyEmonValues(payload.Variables),
		SystemVariables: copyEmonValues(payload.SystemVariables),
		MacroStack:      copyMacroStack(payload.MacroStack),
		Frames:          copyFrameChain(payload.Frames),
		AudioState:      copyStrings(payload.AudioState),
		SpriteStates:    filterSpriteStates(payload.SpriteStates),
		Preview:         payload.Preview,
	}
}

func (d *EmonSaveData) ToPayload() *EmonSavePayload {
	if d == nil {
		return nil
	}
	return &EmonSavePayload{
		Slot:            d.Slot,
		Comment:         d.Comment,
		SavedAt:         d.SavedAt,
		ScriptFile:      d.ScriptFile,
		StatementIndex:  d.StatementIndex,
		Variables:       copyEmonValues(d.Variables),
		SystemVariables: copyEmonValues(d.SystemVariables),
		MacroStack:      copyMacroStack(d.MacroStack),
		Frames:          copyFrameChain(d.Frames),
		AudioState:      copyStrings(d.AudioState),
		SpriteStates:    cloneSpriteStates(d.SpriteStates),
		Preview:         d.Preview,
	}
}

func copyEmonValues(entries map[string]script.EmonValue) map[string]script.EmonValue {
	out := make(map[string]script.EmonValue, len(entries))
	for name, value := range entries {
		out[name] = value
	}
	return out
}

func copyStrings(entries map[string]string) map[string]string {
	out := make(map[string]string, len(entries))
	for name, value := range entries {
		out[name] = value
	}
	return out
}

func copyMacroStack(frames []*script.EmonMacroFrameState) []*script.EmonMacroFrameState {
	out := make([]*script.EmonMacroFrameState, 0, len(frames))
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		out = append(out, &script.EmonMacroFrameState{
			Name:            frame.Name,
			ReturnFile:      frame.ReturnFile,
			CallerStatement: frame.CallerStatement,
			ReturnStatement: frame.ReturnStatement,
			Arguments:       copyEmonValues(frame.Arguments),
		})
	}
	return out
}

func copyFrameChain(frames []*EmonFrameState) []*EmonFrameState {
	out := make([]*EmonFrameState, 0, len(frames))
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		out = append(out, &EmonFrameState{
			Program:         frame.Program,
			Index:           frame.Index,
			Macro:           frame.Macro,
			CallerProgram:   frame.CallerProgram,
			CallerStatement: frame.CallerStatement,
			ReturnStatement: frame.ReturnStatement,
			Arguments:       copyEmonValues(frame.Arguments),
		})
	}
	return out
}

func filterSpriteStates(states []*EmonSpriteState) []*EmonSpriteState {
	out := make([]*EmonSpriteState, 0, len(states))
	for _, state := range states {
		if state == nil || len(state.Name) == 0 {
			continue
		}
		out = append(out, state)
	}
	return out
}

func cloneSpriteStates(states []*EmonSpriteState) []*EmonSpriteState {
	out := make([]*EmonSpriteState, 0, len(states))
	for _, state := range states {
		if state == nil {
			continue
		}
		out = append(out, &EmonSpriteState{
			Name:    state.Name,
			Visible: state.Visible,
			Flags:   state.Flags,
			X:       state.X,
			Y:       state.Y,
			Width:   state.Width,
			Height:  state.Height,
			Kind:    state.Kind,
			Alpha:   state.Alpha,
			F3D:     state.F3D,
		})
	}
	return out
}
