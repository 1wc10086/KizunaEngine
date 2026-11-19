package save

import (
	"time"

	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/brand/emon/stage"
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
	// SelectOrder 记录原引擎的 SELECT 顺序。读档后如果不恢复，脚本按
	// 选择顺序编号的选项会重新排号，键盘/手柄选中的目标可能和存档前不同。
	SelectOrder int
	F3D         *stage.EmonF3DSnapshot `json:"F3D,omitempty"`
	// ImageFile 是这个精灵当前位图的来源文件名（脚本 SetSprite/ReLoadSprite 的
	// 参数）。原引擎的存档保存的是整张精灵表（含像素），读档后画面仍在；
	// 我们只存状态不存像素，读档后 sprite 有坐标/可见性但没有图像，画面就是
	// 全黑。记住文件名并在读档时重新加载即可复原（美术资源本就在 archives 里）。
	ImageFile string
	// ImageSourceSprite 非空表示该精灵是链到另一个精灵的图像（SetSprite 传
	// 精灵名而不是文件名），读档时重建链接。
	ImageSourceSprite string
	// MaskFile 是 SetSprMsk 的遮罩文件名。
	MaskFile string
	// SourceRect 记录 sprite 的裁剪矩形（SetSprRect），nil 表示整图。
	SourceRect *SpriteRectState `json:"SourceRect,omitempty"`
	// Tint/ColorAmount 是 SetSprColor 的效果，丢失会让读档后的立绘变色。
	Tint        int
	ColorAmount int
	// Text/Typing 是 SprWriteText 写进精灵的文字（选项、槽位备注等）。
	Text string
}

type SpriteRectState struct {
	X      int32
	Y      int32
	Width  int32
	Height int32
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
	Fields          []script.EmonValue
	MacroStack      []*script.EmonMacroFrameState
	Frames          []*EmonFrameState
	AudioState      map[string]string
	SpriteStates    []*EmonSpriteState
	// ScreenRoots 记录存档前各「画面根」精灵的可见性（背景/CG/调教/标题）。
	// 读档必须回到当时的画面：单靠 画面モード 判断会受存档界面宏把根节点
	// 关掉的影响，而且不同作品的名字/取值也不同，记录实际状态最稳妥。
	ScreenRoots map[string]bool
	Preview     string
}

type EmonSaveData struct {
	Slot            int32
	Comment         string
	SavedAt         time.Time
	ScriptFile      string
	StatementIndex  int32
	Variables       map[string]script.EmonValue
	SystemVariables map[string]script.EmonValue
	Fields          []script.EmonValue
	MacroStack      []*script.EmonMacroFrameState
	Frames          []*EmonFrameState
	AudioState      map[string]string
	SpriteStates    []*EmonSpriteState
	ScreenRoots     map[string]bool
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
		Fields:          copyEmonValueList(payload.Fields),
		MacroStack:      copyMacroStack(payload.MacroStack),
		Frames:          copyFrameChain(payload.Frames),
		AudioState:      copyStrings(payload.AudioState),
		SpriteStates:    filterSpriteStates(payload.SpriteStates),
		ScreenRoots:     cloneVisibility(payload.ScreenRoots),
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
		Fields:          copyEmonValueList(d.Fields),
		MacroStack:      copyMacroStack(d.MacroStack),
		Frames:          copyFrameChain(d.Frames),
		AudioState:      copyStrings(d.AudioState),
		SpriteStates:    cloneSpriteStates(d.SpriteStates),
		ScreenRoots:     cloneVisibility(d.ScreenRoots),
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

// copyEmonValueList 复制 Save/CheckSaveData 的同序字段，保持整数/字符串类型，
// 避免仅靠字符串往返导致数值字段在使用侧被重新解析。
func copyEmonValueList(entries []script.EmonValue) []script.EmonValue {
	if entries == nil {
		return nil
	}
	out := make([]script.EmonValue, len(entries))
	copy(out, entries)
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

func cloneVisibility(source map[string]bool) map[string]bool {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]bool, len(source))
	for name, visible := range source {
		out[name] = visible
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
			Name:        state.Name,
			Visible:     state.Visible,
			Flags:       state.Flags,
			X:           state.X,
			Y:           state.Y,
			Width:       state.Width,
			Height:      state.Height,
			Kind:        state.Kind,
			Alpha:       state.Alpha,
			SelectOrder: state.SelectOrder,
			F3D:         state.F3D,

			ImageFile:         state.ImageFile,
			ImageSourceSprite: state.ImageSourceSprite,
			MaskFile:          state.MaskFile,
			SourceRect:        state.SourceRect,
			Tint:              state.Tint,
			ColorAmount:       state.ColorAmount,
			Text:              state.Text,
		})
	}
	return out
}
