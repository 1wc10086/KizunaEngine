package runtime

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

type AudioDirector struct {
	machine       *vm.BsxMachine
	script        *vm.BsxScriptFile
	state         *state.GameState
	configuration *config.BsxxConfiguration
}

func NewAudioDirector(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState, configuration *config.BsxxConfiguration) *AudioDirector {
	return &AudioDirector{machine: machine, script: script, state: gameState, configuration: configuration}
}

func (d *AudioDirector) PlayBgm() state.SessionStep {
	name := d.script.GetName(int(d.machine.Frame(2)))
	if len(name) == 0 {
		return d.StopBgm()
	}
	loop := name[0] != '*'
	if loop {
		d.state.SetBgm(name)
	} else {
		d.state.SetBgm(name[1:])
	}
	d.state.SetBgmVolume(d.configuration.BgmVolume)
	d.state.SetBgmLoop(loop)
	return state.SessionContinue
}

func (d *AudioDirector) StopBgm() state.SessionStep {
	d.state.ClearBgm()
	return state.SessionContinue
}

func (d *AudioDirector) PlayEffect() state.SessionStep {
	name := d.script.GetName(int(d.machine.Frame(1)))
	if len(name) == 0 {
		return state.SessionContinue
	}
	d.state.SetSoundEffect(name)
	d.state.SetEffectSerial(d.state.EffectSerial() + 1)
	return state.SessionContinue
}

func (d *AudioDirector) PlayVoiceSequence() state.SessionStep {
	count := d.machine.Operand(1)
	if count <= 0 {
		return state.SessionContinue
	}
	names := make([]string, 0, count)
	for index := int32(0); index < count; index++ {
		name := d.script.GetName(int(d.machine.Operand(int(5 + 4*index))))
		if len(name) > 0 {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return state.SessionContinue
	}
	d.state.SetVoiceSequence(names, false)
	return state.SessionContinue
}

func (d *AudioDirector) PlayOrStopEffect() state.SessionStep {
	if d.machine.Frame(1) < 0 {
		return d.StopEffect()
	}
	return d.PlayEffect()
}

func (d *AudioDirector) StopEffect() state.SessionStep {
	d.state.ClearSoundEffect()
	d.state.SetEffectSerial(d.state.EffectSerial() + 1)
	return state.SessionContinue
}
