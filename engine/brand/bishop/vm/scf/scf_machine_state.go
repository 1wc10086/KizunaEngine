package scf

type ScfMachineState struct {
	ProgramCounter int
	ScenarioNumber int
	Flags          []byte
	Source         []byte
	NameSlots      []string
	CallDepth      int
	Frames         []ScfMachineState
	Halted         bool
	Faulted        bool
	CondDepth      int
	CondStack      []int
	LoopSlots      []int
	ElseSlots      []int
	Latch          int
	MessageOffset  int
}
