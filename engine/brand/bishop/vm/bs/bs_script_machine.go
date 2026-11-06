package bs

type BsScriptMachine interface {
	Dialect() BsScriptDialect
	Halted() bool
	Faulted() bool
	ProgramCounter() int
	Choices() []BsChoice
	Fetch() BsCommand
	Choose(index int) bool
	ClearStack()
	Reset()
	Name(index int) string
	Message(index int) string
	Character(index int) string
}

type ScriptFile interface {
	GetLabelOffset(index int) int
	FindLabel(name string) int
	GetName(index int) string
	GetMessage(index int) string
	GetCharacterName(index int) string
	BsStackCapacity() int
	BsRegisterCapacity() int
	BsVariableCapacity() int
	BsPropertyCapacity() int
	BsGlobalCapacity() int
	BsFlagCapacity() int
	BsVariableArrayCapacity() int
	BsPropertyArrayCapacity() int
	BsChoiceSlotCount() int
	PropertyNameCount() int
}

func loadInt32Slice(target, source []int32) {
	for i := range target {
		target[i] = 0
	}
	copy(target, source)
}
