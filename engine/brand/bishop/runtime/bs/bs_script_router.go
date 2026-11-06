package bs

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

type BsScriptRouter struct {
	messages *BsMessageDirector
	stage    *BsStageDirector
	audio    *BsAudioDirector
	flow     *BsFlowDirector
	system   *BsSystemDirector
}

func NewBsScriptRouter(machine *vm.BsxMachine, script vm.BsMachineCore, st *state.GameState, configuration *config.BsxxConfiguration, resources *resource.GameResourceIndex) *BsScriptRouter {
	stage := NewBsStageDirector(machine, script, st, configuration, resources)
	return &BsScriptRouter{
		messages: NewBsMessageDirector(machine, script, st, stage),
		stage:    stage,
		audio:    NewBsAudioDirector(machine, script, st, configuration),
		flow:     NewBsFlowDirector(machine, script, st),
		system:   NewBsSystemDirector(machine, script, st),
	}
}

// StageDirector 暴露舞台导演给 Session：DRESS/VIEW 按钮要直接改 Rev5
// 合成立绘的服装/视点，不能等下一次 IMAGE 指令才生效。
func (r *BsScriptRouter) StageDirector() *BsStageDirector {
	return r.stage
}

// SetScreenHandler 把脚本请求的系统界面（sceneroom 等）转交给 Session。
func (r *BsScriptRouter) SetScreenHandler(handler func(keyword string) state.SessionStep) {
	r.system.SetScreenHandler(handler)
}

// SetSystemCallHandler 把作品专属的 system call 交给 Session 处理。
func (r *BsScriptRouter) SetSystemCallHandler(handler func(name string) (state.SessionStep, bool)) {
	r.system.SetSystemCallHandler(handler)
}

func (r *BsScriptRouter) Choose(position int) *vm.SelectOption {
	return r.flow.Take(position)
}

func (r *BsScriptRouter) Dispatch(command vm.ScriptCommandId) state.SessionStep {
	switch command {
	case vm.DisplayMessage:
		return r.messages.Show()
	case vm.Image:
		return r.stage.ShowImage()
	case vm.WindowVisibility:
		return r.stage.ApplyWindow()
	case vm.ClearScreen:
		return r.stage.ApplyClearScreen()
	case vm.Transition:
		return r.stage.ApplyTransition()
	case vm.PlayBgm:
		return r.audio.PlayBgm()
	case vm.StopBgm:
		return r.audio.StopBgm()
	case vm.PlayEffect:
		return r.audio.PlaySound()
	case vm.Select:
		return r.flow.PresentChoices()
	case vm.Wait:
		return r.system.Wait()
	case vm.SystemCall:
		return r.system.SystemCall()
	case vm.Dialog:
		return r.system.Dialog()
	case vm.Restart:
		return r.system.Restart()
	case vm.BsLayerControl:
		return r.stage.ApplyLayerControl()
	case vm.BsLayerImage:
		return r.stage.ApplyLayerImage()
	case vm.BsLayerRename:
		return r.stage.ApplyLayerRename()
	case vm.BsStopSound:
		return r.audio.StopSound()
	case vm.Halt:
		return r.system.Halt()
	default:
		return state.SessionContinue
	}
}
