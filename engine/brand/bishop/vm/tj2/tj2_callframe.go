package tj2

type Tj2CallFrameState struct {
	ScenarioNumber int
	ProgramCounter int
	CondDepth      int
	CondStack      []int
	LoopSlots      []int
	Latch          int
	MessageOffset  int
	Source         []byte
}

type tj2CallFrame struct {
	scenario      int
	pc            int
	condDepth     int
	condStack     []int
	latch         int
	messageOffset int
	source        []byte
}

func newTj2CallFrame() *tj2CallFrame {
	return &tj2CallFrame{messageOffset: -1}
}
