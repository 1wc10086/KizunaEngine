package vm

type MachineState struct {
	ProgramCounter      int
	FrameBase           int
	CallDepth           int
	Frame               []int32
	Registers           []int32
	Variables           []int32
	Properties          []int32
	Scalars             []float64
	CallReturnAddresses []int32
	CallFrameBases      []int32
	Deferred            bool
	DeferredIsCall      bool
	DeferredTarget      int
	Halted              bool
}
