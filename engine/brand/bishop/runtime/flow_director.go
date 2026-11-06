package runtime

import (
	"sort"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

type FlowDirector struct {
	machine *vm.BsxMachine
	script  *vm.BsxScriptFile
	state   *state.GameState
	pending []vm.SelectOption
}

func NewFlowDirector(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState) *FlowDirector {
	return &FlowDirector{machine: machine, script: script, state: gameState}
}

func (d *FlowDirector) PresentChoices() state.SessionStep {
	options := d.machine.SelectOptions()
	if len(options) == 0 {
		return state.SessionContinue
	}
	d.pending = append(d.pending[:0], options...)
	sort.Slice(d.pending, func(a, b int) bool { return d.pending[a].Index < d.pending[b].Index })
	texts := make([]string, 0, len(d.pending))
	for _, option := range d.pending {
		texts = append(texts, d.state.ResolveNameCodes(d.choiceText(option)))
	}
	d.state.SetChoices(texts)
	core.Logf("choice: %d options", len(texts))
	return state.SessionAwaitChoice
}

// choiceText 取选项显示文本。
//
// BSXScript 的 FETCH 有三个操作数：MessageId / LabelId / Value。正常作品里
// MessageId 与 Value 相同；部分汉化补丁只改写了其中一个（黒の教室 的 Value
// 才是译文，MessageId 指向正文），所以两边都试，优先用能取到文本的 Value。
func (d *FlowDirector) choiceText(option vm.SelectOption) string {
	if option.Value != option.MessageId {
		if text := d.script.GetMessage(option.Value); text != "" {
			return text
		}
	}
	return d.script.GetMessage(option.MessageId)
}

func (d *FlowDirector) Take(position int) *vm.SelectOption {
	if position < 0 || position >= len(d.pending) {
		return nil
	}
	option := d.pending[position]
	d.pending = d.pending[:0]
	d.state.ClearChoices()
	core.Logf("choice: picked %d value=%d type=%d kind=%d", position, option.Value, option.Index, option.Kind)
	return &option
}

func (d *FlowDirector) AwaitInput() state.SessionStep {
	d.state.SetAwaitingInput(true)
	d.machine.ApplyDeferredJump()
	return state.SessionAwaitInput
}

func (d *FlowDirector) WaitSelect() state.SessionStep {
	mode := d.machine.Frame(1)
	delay := d.machine.Frame(2)
	if mode != 2 || delay <= 0 {
		return d.AwaitInput()
	}
	wait := delay * 100
	if wait < 0 {
		wait = 0
	}
	if wait > 3000 {
		wait = 3000
	}
	d.state.SetWaitMilliseconds(int(wait))
	return state.SessionWait
}
