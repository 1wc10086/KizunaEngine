package bs

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	vmbs "kizuna/engine/brand/bishop/vm/bs"
	"kizuna/engine/core"
)

const (
	bsVoiceChannel = 1
	bsSoundChannel = 3
	bsVolumeLimit  = 10
)

type BsAudioDirector struct {
	machine       *vm.BsxMachine
	script        vm.BsMachineCore
	state         *state.GameState
	configuration *config.BsxxConfiguration
}

func NewBsAudioDirector(machine *vm.BsxMachine, script vm.BsMachineCore, state *state.GameState, configuration *config.BsxxConfiguration) *BsAudioDirector {
	return &BsAudioDirector{machine: machine, script: script, state: state, configuration: configuration}
}

func (d *BsAudioDirector) PlayBgm() state.SessionStep {
	command := d.machine.Command()
	volume, hasVolume := resolveBsVolume(int(command.Second))
	if command.First < 0 {
		if hasVolume {
			d.state.SetBgmVolume(volume)
		}
		track := "-"
		if name, ok := d.state.Bgm(); ok {
			track = name
		}
		core.Logf("audio: bgm volume %.2f track '%s'", d.state.BgmVolume(), track)
		return state.SessionContinue
	}
	name := d.script.Name(int(command.First))
	if len(name) == 0 {
		return d.stopBgm(0)
	}
	loop := name[0] != '*'
	if loop {
		d.state.SetBgm(name)
	} else {
		d.state.SetBgm(name[1:])
	}
	if hasVolume {
		d.state.SetBgmVolume(volume)
	} else {
		d.state.SetBgmVolume(d.configuration.BgmVolume)
	}
	d.state.SetBgmLoop(loop)
	bgm, _ := d.state.Bgm()
	core.Logf("audio: bgm '%s' loop=%v volume=%.2f", bgm, loop, d.state.BgmVolume())
	return state.SessionContinue
}

func (d *BsAudioDirector) StopBgm() state.SessionStep {
	return d.stopBgm(max(0, int(d.machine.Command().First)))
}

func (d *BsAudioDirector) stopBgm(fade int) state.SessionStep {
	d.state.ClearBgm()
	core.Logf("audio: bgm stopped fade=%dms", fade)
	return state.SessionContinue
}

func (d *BsAudioDirector) PlaySound() state.SessionStep {
	command := d.machine.Command()
	name := d.script.Name(int(command.First))
	if len(name) == 0 {
		core.Logf("audio: effect channel=%d ignored", int(command.Second))
		return state.SessionContinue
	}
	if d.isVoiceChannel(int(command.Second)) {
		d.state.SetVoice(name)
		d.state.SetVoiceSerial(d.state.VoiceSerial() + 1)
		core.Logf("voice: '%s' channel=%d", name, int(command.Second))
		return state.SessionContinue
	}
	d.state.SetSoundEffect(name)
	d.state.SetEffectSerial(d.state.EffectSerial() + 1)
	core.Logf("audio: effect '%s' channel=%d", name, int(command.Second))
	return state.SessionContinue
}

func (d *BsAudioDirector) StopSound() state.SessionStep {
	channel := int(d.machine.Command().First)
	switch channel {
	case bsVoiceChannel:
		d.state.ClearVoice()
		d.state.SetVoiceSerial(d.state.VoiceSerial() + 1)
		core.Logf("voice: voice stopped")
		return state.SessionContinue
	case bsSoundChannel:
		d.state.ClearSoundEffect()
		d.state.SetEffectSerial(d.state.EffectSerial() + 1)
		core.Logf("audio: effect stopped")
		return state.SessionContinue
	default:
		core.Logf("audio: stop channel=%d ignored", channel)
		return state.SessionContinue
	}
}

func resolveBsVolume(value int) (float64, bool) {
	if value < 0 {
		return 0, false
	}
	volume := float64(value) / float64(bsVolumeLimit)
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	return volume, true
}

func (d *BsAudioDirector) isVoiceChannel(channel int) bool {
	return channel == bsVoiceChannel && vmbs.RevIsRev(d.script.Dialect())
}
