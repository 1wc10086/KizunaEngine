package tj2

type Tj2MachineState struct {
	ProgramCounter int
	ScenarioNumber int
	Flags          []byte
	Source         []byte
	NameSlots      []string
	CondDepth      int
	CondStack      []int
	LoopSlots      []int
	LoopHead       int
	Latch          int
	MessageOffset  int
	Halted         bool
	Faulted        bool
	Frames         []Tj2CallFrameState
}
