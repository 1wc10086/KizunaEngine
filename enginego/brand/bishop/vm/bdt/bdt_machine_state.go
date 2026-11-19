package bdt

type BdtStop int

const (
	BdtStopNone BdtStop = iota
	BdtStopHalt
	BdtStopWait
	BdtStopText
	BdtStopChoice
	BdtStopInput
	BdtStopMap
	BdtStopScreen
	BdtStopTitle
	BdtStopAttack
	BdtStopOrganize
	BdtStopStaffRoll
)

type BdtMachineState struct {
	ProgramCounter int
	X              []int32
	Z              []int32
	Flags          []int32
	SystemFlags    []int32
	CallStack      []int
	Stop           BdtStop
}

func (s *BdtMachineState) Clone() *BdtMachineState {
	if s == nil {
		return nil
	}
	out := &BdtMachineState{
		ProgramCounter: s.ProgramCounter,
		Stop:           s.Stop,
	}
	if s.X != nil {
		out.X = make([]int32, len(s.X))
		copy(out.X, s.X)
	}
	if s.Z != nil {
		out.Z = make([]int32, len(s.Z))
		copy(out.Z, s.Z)
	}
	if s.Flags != nil {
		out.Flags = make([]int32, len(s.Flags))
		copy(out.Flags, s.Flags)
	}
	if s.SystemFlags != nil {
		out.SystemFlags = make([]int32, len(s.SystemFlags))
		copy(out.SystemFlags, s.SystemFlags)
	}
	if s.CallStack != nil {
		out.CallStack = make([]int, len(s.CallStack))
		copy(out.CallStack, s.CallStack)
	}
	return out
}
