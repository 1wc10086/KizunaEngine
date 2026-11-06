package bs

import (
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

type BsFlowDirector struct {
	machine *vm.BsxMachine
	script  vm.BsMachineCore
	state   *state.GameState
}

func NewBsFlowDirector(machine *vm.BsxMachine, script vm.BsMachineCore, state *state.GameState) *BsFlowDirector {
	return &BsFlowDirector{machine: machine, script: script, state: state}
}

func (d *BsFlowDirector) PresentChoices() state.SessionStep {
	choices := d.script.Choices()
	if len(choices) == 0 {
		return state.SessionContinue
	}
	texts := make([]string, 0, len(choices))
	for _, choice := range choices {
		// 选择支文本也是消息表里的正文，同样可能带 \A/\B 姓名占位符和
		// $0 爱心码；不在这里清洗的话，玩家会直接看到反斜杠转义和 "$0"。
		texts = append(texts, LegacyMessage(d.state.ResolveNameCodes(d.script.Message(int(choice.MessageId)))))
	}
	d.state.SetChoices(texts)
	core.Logf("choice: %d options", len(texts))
	return state.SessionAwaitChoice
}

func (d *BsFlowDirector) Take(position int) *vm.SelectOption {
	if position < 0 {
		return nil
	}
	options := d.machine.SelectOptions()
	if position < len(options) {
		option := options[position]
		d.apply(position)
		core.Logf("choice: picked %d message=%d", position, option.MessageId)
		result := option
		return &result
	}
	choices := d.script.Choices()
	if position >= len(choices) {
		return nil
	}
	choice := choices[position]
	d.apply(position)
	core.Logf("choice: picked %d message=%d", position, int(choice.MessageId))
	return &vm.SelectOption{Index: choice.Index, MessageId: int(choice.MessageId), LabelId: -1, Value: choice.Index, Kind: vm.BsChoiceKind}
}

func (d *BsFlowDirector) apply(position int) {
	d.state.ClearChoices()
	if d.machine.ChooseOption(position) {
		return
	}
	core.Logf("choice: picked %d has no matching machine entry", position)
}
