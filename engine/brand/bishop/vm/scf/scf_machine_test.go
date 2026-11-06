package scf

import (
	"encoding/binary"
	"fmt"
	"testing"

	"kizuna/engine/brand/bishop/vm/tj2"
)

type scfTestTable struct {
	flags   int
	source  int
	graphic []string
	tracks  []string
	sounds  []string
	slots   []ScfFunctionSlot
	names   []string
}

func (t scfTestTable) FlagCount() int                   { return t.flags }
func (t scfTestTable) SourceCount() int                 { return t.source }
func (t scfTestTable) GraphicNames() []string           { return t.graphic }
func (t scfTestTable) TrackNumbers() []string           { return t.tracks }
func (t scfTestTable) SoundNames() []string             { return t.sounds }
func (t scfTestTable) FunctionSlots() []ScfFunctionSlot { return t.slots }
func (t scfTestTable) SlotNames() []string              { return t.names }

func newScfTestTable() scfTestTable {
	return scfTestTable{
		flags:   4,
		source:  2,
		graphic: []string{"bg"},
		tracks:  []string{"7"},
		sounds:  []string{"snd"},
		slots:   []ScfFunctionSlot{{Number: "3", Name: "f"}},
		names:   []string{"one", "two"},
	}
}

func scfTestLoader(data []byte) ScfScenarioLoader {
	return func(scenario int) (*ScfScriptFile, error) {
		if scenario != 0 {
			return nil, fmt.Errorf("scenario %d not available", scenario)
		}
		return LoadScfScriptFile(data)
	}
}

func buildScfData(code []byte, entryCount, poolSize, jumpTargetCount, transferCount int, fill func([]byte)) []byte {
	total := ScfHeaderSize + (6 * entryCount) + (2 * entryCount) + (4 * transferCount) + (2 * jumpTargetCount) + poolSize + len(code)
	data := make([]byte, total)
	binary.LittleEndian.PutUint16(data[0:], uint16(entryCount))
	binary.LittleEndian.PutUint16(data[2:], uint16(poolSize))
	binary.LittleEndian.PutUint16(data[4:], uint16(jumpTargetCount))
	binary.LittleEndian.PutUint16(data[6:], uint16(len(code)))
	binary.LittleEndian.PutUint16(data[8:], uint16(transferCount))
	copy(data[total-len(code):], code)
	if fill != nil {
		fill(data)
	}
	return data
}

func TestScfMachineFetchSequence(t *testing.T) {
	code := []byte{
		ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00,
		ScfOpcodeBang, 's', 'H', 'I', 0x00,
		ScfOpcodeBang, 'g', 0x00, 0x00,
		ScfOpcodeWrite, 0x00, 0x01, 0x00, '+', 0x05,
	}
	data := buildScfData(code, 0, 0, 0, 1, nil)
	if !IsScf(data) {
		t.Fatal("IsScf rejected a valid script")
	}
	if _, err := LoadScfScriptFile(data); err != nil {
		t.Fatalf("LoadScfScriptFile: %v", err)
	}

	machine := NewScfMachine(newScfTestTable(), scfTestLoader(data))
	machine.Start(0)

	if command := machine.Fetch(); command.Kind != ScfCommandKindRefresh {
		t.Fatalf("command 1 kind = %d", command.Kind)
	}
	command := machine.Fetch()
	if command.Kind != ScfCommandKindMessage || command.Text != "HI" {
		t.Fatalf("command 2 = %+v", command)
	}
	command = machine.Fetch()
	if command.Kind != ScfCommandKindBackground || command.Index != 0 || command.Text != "BG.KG" {
		t.Fatalf("command 3 = %+v", command)
	}
	if command = machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("command 4 = %+v", command)
	}
	if machine.Faulted() {
		t.Fatal("machine faulted")
	}
	if !machine.Halted() {
		t.Fatal("machine not halted")
	}
	if machine.Flags()[1] != 5 {
		t.Fatalf("flag 1 = %d", machine.Flags()[1])
	}
	if machine.ProgramCounter() != len(code) {
		t.Fatalf("pc = %d", machine.ProgramCounter())
	}
	if command = machine.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("command 5 = %+v", command)
	}

	state := machine.Capture()
	if state.ProgramCounter != len(code) || state.Flags[1] != 5 {
		t.Fatalf("captured state = %+v", state)
	}
	restored := NewScfMachine(newScfTestTable(), scfTestLoader(data))
	restored.Restore(state)
	if restored.ProgramCounter() != len(code) || !restored.Halted() {
		t.Fatalf("restored pc = %d halted = %v", restored.ProgramCounter(), restored.Halted())
	}
	if restored.Flags()[1] != 5 {
		t.Fatalf("restored flag 1 = %d", restored.Flags()[1])
	}
}

func TestScfMachineOptionsAndChoice(t *testing.T) {
	code := []byte{
		ScfOpcodeLabel,
		ScfOpcodeLabel,
		ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00,
	}
	data := buildScfData(code, 1, 8, 1, 2, func(data []byte) {
		data[18] = 1
		data[19] = 2
		binary.LittleEndian.PutUint16(data[26:], 1)
		data[28] = 1
		copy(data[29:], "SCF")
	})

	machine := NewScfMachine(newScfTestTable(), scfTestLoader(data))
	machine.Start(0)
	command := machine.Fetch()
	if command.Kind != ScfCommandKindOptions {
		t.Fatalf("options kind = %d text = %q", command.Kind, command.Text)
	}
	if len(command.Options) != 1 {
		t.Fatalf("option count = %d", len(command.Options))
	}
	option := command.Options[0]
	if option.Index != 0 || option.Entry != 0 || option.Text != "SCF" {
		t.Fatalf("option = %+v", option)
	}
	if !machine.Choose(0) {
		t.Fatal("choose rejected")
	}
	if machine.Faulted() {
		t.Fatal("machine faulted after choose")
	}
	if machine.ProgramCounter() != 2 {
		t.Fatalf("pc after choose = %d", machine.ProgramCounter())
	}
	if command = machine.Fetch(); command.Kind != ScfCommandKindRefresh {
		t.Fatalf("post choose command = %+v", command)
	}
}

func TestScfFileRejectsMalformedData(t *testing.T) {
	code := []byte{ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00}
	data := buildScfData(code, 0, 0, 0, 1, nil)
	if _, err := LoadScfScriptFile(data[:4]); err == nil {
		t.Fatal("short header accepted")
	}
	if _, err := LoadScfScriptFile(data[:len(data)-1]); err == nil {
		t.Fatal("length mismatch accepted")
	}
	bad := append([]byte(nil), data...)
	binary.LittleEndian.PutUint16(bad[8:], 5)
	if _, err := LoadScfScriptFile(bad); err == nil {
		t.Fatal("invalid transfer count accepted")
	}
}

func TestScfOpcodeLengths(t *testing.T) {
	code := []byte{
		ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00,
		ScfOpcodeBang, 's', 'H', 'I', 0x00,
		ScfOpcodeWrite, 0x00, 0x01, 0x00, '+', 0x05,
	}
	length, err := ScfInstructionLength(code, 0)
	if err != nil || length != 6 {
		t.Fatalf("!a length = %d err = %v", length, err)
	}
	length, err = ScfInstructionLength(code, 6)
	if err != nil || length != 5 {
		t.Fatalf("!s length = %d err = %v", length, err)
	}
	length, err = ScfInstructionLength(code, 11)
	if err != nil || length != 6 {
		t.Fatalf("write length = %d err = %v", length, err)
	}
	if _, err = ScfInstructionLength(code, 99); err == nil {
		t.Fatal("out of range instruction accepted")
	}
	if _, err = ScfExpressionLength(code, 0); err == nil {
		t.Fatal("unterminated expression accepted")
	}
}

func TestScfFileBridgesToTj2(t *testing.T) {
	code := []byte{
		ScfOpcodeLabel,
		ScfOpcodeLabel,
		ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00,
	}
	data := buildScfData(code, 1, 8, 1, 2, func(data []byte) {
		binary.LittleEndian.PutUint16(data[10:], 0)
		binary.LittleEndian.PutUint16(data[12:], 0)
		binary.LittleEndian.PutUint16(data[14:], 0)
		binary.LittleEndian.PutUint16(data[26:], 1)
		data[28] = 1
		copy(data[29:], "SCF")
	})
	file, err := LoadScfScriptFile(data)
	if err != nil {
		t.Fatalf("LoadScfScriptFile: %v", err)
	}
	var bridged tj2.ScfFile = file.AsTj2File()
	if bridged.EntryCount() != 1 || len(bridged.Entries()) != 1 {
		t.Fatalf("entries = %d/%d", bridged.EntryCount(), len(bridged.Entries()))
	}
	text, err := bridged.PoolText(0)
	if err != nil || text != "SCF" {
		t.Fatalf("pool text = %q err = %v", text, err)
	}
	if bridged.TransferCount() != 2 || bridged.JumpTargetCount() != 1 {
		t.Fatalf("transfer/jump = %d/%d", bridged.TransferCount(), bridged.JumpTargetCount())
	}
	if bridged.JumpTarget(0) != 1 {
		t.Fatalf("jump target = %d", bridged.JumpTarget(0))
	}
	if len(bridged.Code()) != len(code) || len(bridged.Data()) != len(data) {
		t.Fatal("code/data length mismatch")
	}
}

func scfConditionalCode() []byte {
	return []byte{
		ScfOpcodeHash, 'i', ScfOpcodeWrite, 0x00, 0x01, 0x00, '=', 0x05, ScfOpcodeLabel,
		ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00,
		ScfOpcodeHash, 'n',
	}
}

func TestScfMachineConditionalSkip(t *testing.T) {
	code := scfConditionalCode()
	data := buildScfData(code, 0, 0, 0, 1, nil)

	taken := NewScfMachine(newScfTestTable(), scfTestLoader(data))
	taken.Start(0)
	taken.Flags()[1] = 5
	if command := taken.Fetch(); command.Kind != ScfCommandKindRefresh {
		t.Fatalf("taken command = %+v", command)
	}
	if command := taken.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("taken tail command = %+v", command)
	}

	skipped := NewScfMachine(newScfTestTable(), scfTestLoader(data))
	skipped.Start(0)
	if command := skipped.Fetch(); command.Kind != ScfCommandKindHalt {
		t.Fatalf("skipped command = %+v", command)
	}
	if skipped.Faulted() {
		t.Fatal("skipped machine faulted")
	}
}

func TestScfExpressionEvaluate(t *testing.T) {
	code := scfConditionalCode()
	flags := make([]byte, 4)
	flags[1] = 5
	value, used := EvaluateExpression(code, 2, flags)
	if !value || used != 7 {
		t.Fatalf("value = %v used = %d", value, used)
	}
	flags[1] = 4
	if value, used = EvaluateExpression(code, 2, flags); value || used != 7 {
		t.Fatalf("value = %v used = %d", value, used)
	}
	if _, used = EvaluateExpression(code, len(code)+1, flags); used != -1 {
		t.Fatalf("out of range used = %d", used)
	}
}

func TestScfMachineLoadScenario(t *testing.T) {
	first := buildScfData([]byte{ScfOpcodeBang, 'j', 0x00, 0x01}, 0, 0, 0, 1, nil)
	second := buildScfData([]byte{ScfOpcodeBang, 'a', 0x00, 0x00, 0x00, 0x00}, 0, 0, 0, 1, nil)
	loader := func(scenario int) (*ScfScriptFile, error) {
		switch scenario {
		case 0:
			return LoadScfScriptFile(first)
		case 1:
			return LoadScfScriptFile(second)
		}
		return nil, fmt.Errorf("scenario %d not available", scenario)
	}
	machine := NewScfMachine(newScfTestTable(), loader)
	machine.Start(0)
	command := machine.Fetch()
	if command.Kind != ScfCommandKindLoadScenario || command.Value != 1 {
		t.Fatalf("jump command = %+v", command)
	}
	if machine.ScenarioNumber() != 1 {
		t.Fatalf("scenario = %d", machine.ScenarioNumber())
	}
	if command = machine.Fetch(); command.Kind != ScfCommandKindRefresh {
		t.Fatalf("second scenario command = %+v", command)
	}
}
