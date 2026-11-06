package bs

type BsMachineState struct {
	Dialect          BsScriptDialect
	ProgramCounter   int
	InstructionStart int
	StackTop         int
	Stack            []int32
	Accumulator      int32
	Flag             int32
	Registers        []int32
	Properties       []int32
	Variables        []int32
	Choices          []BsChoice
	Halted           bool
	Faulted          bool
}

type RevMachineState struct {
	Dialect          BsScriptDialect
	ProgramCounter   int
	InstructionStart int
	StackTop         int
	Stack            []int32
	Accumulator      int32
	Watch            int
	Flags            []int32
	Variables        []int32
	Properties       []int32
	CallbackTop      int
	Callbacks        []int32
	ChoiceCounters   []byte
	Choices          []BsChoice
	Halted           bool
	Faulted          bool
}

func RevMachineStateFromBs(state BsMachineState) RevMachineState {
	return RevMachineState{
		Dialect:          state.Dialect,
		ProgramCounter:   state.ProgramCounter,
		InstructionStart: state.InstructionStart,
		StackTop:         state.StackTop,
		Stack:            state.Stack,
		Accumulator:      state.Accumulator,
		Watch:            -1,
		Flags:            state.Registers,
		Variables:        state.Variables,
		Properties:       state.Properties,
		CallbackTop:      0,
		Callbacks:        []int32{},
		ChoiceCounters:   []byte{},
		Choices:          state.Choices,
		Halted:           state.Halted,
		Faulted:          state.Faulted,
	}
}

func (s RevMachineState) ToBsMachineState() BsMachineState {
	var flag int32
	if len(s.Flags) > 0 {
		flag = s.Flags[0]
	}
	return BsMachineState{
		Dialect:          s.Dialect,
		ProgramCounter:   s.ProgramCounter,
		InstructionStart: s.InstructionStart,
		StackTop:         s.StackTop,
		Stack:            s.Stack,
		Accumulator:      s.Accumulator,
		Flag:             flag,
		Registers:        s.Flags,
		Properties:       s.Properties,
		Variables:        s.Variables,
		Choices:          s.Choices,
		Halted:           s.Halted,
		Faulted:          s.Faulted,
	}
}
