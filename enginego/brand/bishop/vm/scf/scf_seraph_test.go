package scf

import "testing"

func scfSeraphTable() scfTestTable {
	return scfTestTable{
		flags:   4,
		source:  2,
		graphic: []string{"bg", "aki"},
		tracks:  []string{"7"},
		sounds:  []string{"snd"},
		slots: []ScfFunctionSlot{
			{Number: "48", Name: "wait"},
			{Number: "49", Name: "fadeout"},
			{Number: "50", Name: "timeset"},
			{Number: "51", Name: "timeget"},
			{Number: "52", Name: "ending"},
			{Number: "53", Name: "roll"},
		},
		names: []string{"a"},
	}
}

func scfSeraphBangS(text string) []byte {
	out := []byte{ScfOpcodeBang, 's'}
	out = append(out, text...)
	return append(out, 0)
}

func scfSeraphMachine(t *testing.T, code []byte, pageBreaks bool) *ScfMachine {
	t.Helper()
	data := buildScfData(code, 0, 0, 0, 1, nil)
	machine := NewScfMachine(scfSeraphTable(), scfTestLoader(data))
	machine.SetPageBreaks(pageBreaks)
	machine.Start(0)
	return machine
}

func TestScfFunctionUsesSlotIndex(t *testing.T) {
	code := []byte{ScfOpcodeBang, 'p', 0x00, 0x02, 'X', 0x00}
	machine := scfSeraphMachine(t, code, false)
	command := machine.Fetch()
	if command.Kind != ScfCommandKindFunction || command.Index != 2 || command.Value != 2 || command.Text != "X" {
		t.Fatalf("function command = %+v", command)
	}
}

func TestScfClearReturnsClearKind(t *testing.T) {
	code := []byte{ScfOpcodeBang, 'c'}
	machine := scfSeraphMachine(t, code, false)
	machine.Flags()[1] = 9
	command := machine.Fetch()
	if command.Kind != ScfCommandKindClear {
		t.Fatalf("clear command = %+v", command)
	}
	if machine.Flags()[1] != 0 {
		t.Fatalf("flag after clear = %d", machine.Flags()[1])
	}
}

func TestScfMessageWithoutPageBreaksIsSingleMessage(t *testing.T) {
	code := scfSeraphBangS("_fAKI:HELLO_rWORLD")
	machine := scfSeraphMachine(t, code, false)
	if command := machine.Fetch(); command.Kind != ScfCommandKindStandee || command.Text != "AKI" {
		t.Fatalf("standee command = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindMessage || command.Text != "HELLO\nWORLD" {
		t.Fatalf("message command = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("tail command = %+v", command)
	}
}

func TestScfPagedMessageSplitsOnRead(t *testing.T) {
	code := scfSeraphBangS("_fAKI:HELLO_rWORLD")
	machine := scfSeraphMachine(t, code, true)
	if command := machine.Fetch(); command.Kind != ScfCommandKindStandee || command.Text != "AKI" || command.Flag != 2 || command.Index != -1 {
		t.Fatalf("standee command = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindMessage || command.Text != "HELLO" {
		t.Fatalf("first page = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindMessage || command.Text != "WORLD" {
		t.Fatalf("second page = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("tail command = %+v", command)
	}
}

func TestScfPagedMessageBreaksAfterFourLines(t *testing.T) {
	code := scfSeraphBangS("A_nB_nC_nD_nE")
	machine := scfSeraphMachine(t, code, true)
	if command := machine.Fetch(); command.Kind != ScfCommandKindMessage || command.Text != "A\nB\nC\nD" {
		t.Fatalf("first page = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindMessage || command.Text != "E" {
		t.Fatalf("second page = %+v", command)
	}
	if command := machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("tail command = %+v", command)
	}
}

func TestScfPagedMessageControlOnlyIsSafe(t *testing.T) {
	code := scfSeraphBangS("_r")
	machine := scfSeraphMachine(t, code, true)
	if command := machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("control-only command = %+v", command)
	}
	if machine.Faulted() {
		t.Fatal("machine faulted on control-only message")
	}
}

func TestScfRootOptionsWithoutScriptIsNone(t *testing.T) {
	machine := NewScfMachine(scfSeraphTable(), nil)
	if command := machine.RootOptions(); command.Kind != ScfCommandKindNone {
		t.Fatalf("root options without script = %+v", command)
	}
}
