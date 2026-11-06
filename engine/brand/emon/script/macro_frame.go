package script

type EmonMacroFrame struct {
	Name            string
	Arguments       *EmonVariableStore
	ReturnStatement int32
	CallerProgram   string
	CallerStatement int32
}

func NewEmonMacroFrame(name string) *EmonMacroFrame {
	return &EmonMacroFrame{
		Name:      name,
		Arguments: NewEmonVariableStore(),
	}
}

func NewEmonMacroFrameWithCaller(name string, callerProgram string, callerStatement int32, returnStatement int32) *EmonMacroFrame {
	frame := NewEmonMacroFrame(name)
	frame.CallerProgram = callerProgram
	frame.CallerStatement = callerStatement
	frame.ReturnStatement = returnStatement
	return frame
}

func (f *EmonMacroFrame) ToState() *EmonMacroFrameState {
	return &EmonMacroFrameState{
		Name:            f.Name,
		ReturnFile:      f.CallerProgram,
		CallerStatement: f.CallerStatement,
		ReturnStatement: f.ReturnStatement,
		Arguments:       f.Arguments.Snapshot(),
	}
}

func EmonMacroFrameFromState(state *EmonMacroFrameState) *EmonMacroFrame {
	frame := NewEmonMacroFrameWithCaller(state.Name, state.ReturnFile, state.CallerStatement, state.ReturnStatement)
	frame.Arguments.LoadFrom(state.Arguments)
	return frame
}

type EmonMacroFrameState struct {
	Name            string
	ReturnFile      string
	CallerStatement int32
	ReturnStatement int32
	Arguments       map[string]EmonValue
}

func NewEmonMacroFrameState() *EmonMacroFrameState {
	return &EmonMacroFrameState{Arguments: make(map[string]EmonValue)}
}
