package bs

import (
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

type BsMessageDirector struct {
	machine *vm.BsxMachine
	script  vm.BsMachineCore
	state   *state.GameState
	stage   *BsStageDirector
}

func NewBsMessageDirector(machine *vm.BsxMachine, script vm.BsMachineCore, state *state.GameState, stage *BsStageDirector) *BsMessageDirector {
	return &BsMessageDirector{machine: machine, script: script, state: state, stage: stage}
}

func (d *BsMessageDirector) Show() state.SessionStep {
	command := d.machine.Command()
	messageId := int(command.First)
	d.state.SetText(LegacyMessage(d.script.Message(messageId)))
	character := int(command.Second)
	if character >= 0 {
		d.state.SetSpeaker(Speaker(d.script.Character(character)))
	} else {
		d.state.SetSpeaker("")
	}
	voice := int(command.Third)
	if voice >= 0 {
		track := d.script.Name(voice)
		if len(track) > 0 {
			d.state.SetVoice(track)
			d.state.SetVoiceSerial(d.state.VoiceSerial() + 1)
			core.Logf("voice: '%s'", track)
		}
	}
	d.stage.ApplyFace(int(command.Fourth))
	d.stage.ShowMessageWindow()
	d.state.SetAwaitingInput(true)
	core.Logf("text: msg#%d char='%s' '%s'", messageId, d.state.Speaker(), trimBsLog(d.state.Text()))
	return state.SessionAwaitInput
}

func trimBsLog(value string) string {
	runes := []rune(value)
	if len(runes) <= 32 {
		return value
	}
	return string(runes[:32])
}
