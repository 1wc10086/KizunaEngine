package runtime

import (
	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

// HostAction 是剧本把宿主命令写在 SELECT/SELECT_TAIL/TRANSLATE 参数里时
// 的回调。返回值 handled=false 表示"不是宿主命令"，路由继续走原有语义
// （选择肢、地图、影片），因此普通 AVG 的行为完全不变。
type HostAction func(command vm.ScriptCommandId) (state.SessionStep, bool)

type ScriptRouter struct {
	hostAction HostAction
	messages   *MessageDirector
	stage      *StageDirector
	audio      *AudioDirector
	flow       *FlowDirector
	movies     *MovieDirector
}

func NewScriptRouter(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState, configuration *config.BsxxConfiguration, resources *resource.GameResourceIndex) *ScriptRouter {
	var layout config.MessageWindow
	if configuration != nil {
		layout = configuration.Message
	}
	return &ScriptRouter{
		messages: NewMessageDirector(machine, script, gameState, layout),
		stage:    NewStageDirector(machine, script, gameState, configuration, resources),
		audio:    NewAudioDirector(machine, script, gameState, configuration),
		flow:     NewFlowDirector(machine, script, gameState),
		movies:   NewMovieDirector(machine, script, gameState),
	}
}

func (r *ScriptRouter) Choose(position int) *vm.SelectOption {
	return r.flow.Take(position)
}

func (r *ScriptRouter) SetHostAction(action HostAction) {
	r.hostAction = action
}

// SetBustupOverride 让 DRESS 选择实时影响之后画的立绘。
func (r *ScriptRouter) SetBustupOverride(override func(string) string) {
	r.stage.SetBustupOverride(override)
}

func (r *ScriptRouter) Dispatch(command vm.ScriptCommandId) state.SessionStep {
	// 宿主命令必须先于选择肢/地图处理：SLG 的 SELECT 参数是
	// `InitBeginTimeParameters()` 这类 Lua 调用，没有 FETCH 选项；
	// 先走 PresentChoices 会把它们当"空选择"静默吞掉。
	if r.hostAction != nil {
		if step, handled := r.hostAction(command); handled {
			return step
		}
	}
	return r.DispatchModern(command)
}

func (r *ScriptRouter) DispatchModern(command vm.ScriptCommandId) state.SessionStep {
	switch command {
	case vm.DisplayMessage:
		return r.messages.Show()
	case vm.Image:
		return r.stage.ShowImage()
	case vm.ImageExtended:
		return r.stage.ShowImageExtended()
	case vm.Attach:
		// モーションプラス：把 *_anim_sc 播放脚本挂到刚设置的动态 CG 上。
		return r.stage.AttachAnimation()
	case vm.ReleaseLayer:
		return r.stage.ReleaseLayer()
	case vm.ShowLayerImage:
		// SHOW_LAYER：地图靠它按当前可用的地点隐藏/显示前景层。
		return r.stage.ShowLayerImage()
	case vm.ChangeImage:
		return r.stage.ChangeImage()
	case vm.LayerParameter:
		return r.stage.ApplyLayerParameter()
	case vm.WindowVisibility:
		return r.messages.ApplyWindowVisibility()
	case vm.Mask:
		return r.stage.ApplyMask()
	case vm.Transition:
		return r.stage.ApplyTransition()
	case vm.Fill:
		return r.stage.ApplyFill()
	case vm.Character:
		return r.stage.ShowCharacter()
	case vm.PlayBgm:
		return r.audio.PlayBgm()
	case vm.StopBgm:
		return r.audio.StopBgm()
	case vm.PlayEffect:
		return r.audio.PlayEffect()
	case vm.PlayAmbient:
		return r.audio.PlayEffect()
	case vm.AmbientCommand:
		return r.audio.PlayOrStopEffect()
	case vm.PlayVoiceTrack:
		return r.audio.PlayEffect()
	case vm.VoiceTrackCommand:
		return r.audio.PlayOrStopEffect()
	case vm.Invoke:
		return r.audio.PlayVoiceSequence()
	case vm.Select:
		return r.flow.PresentChoices()
	case vm.SelectTail:
		// BSXScript 3.x 把"影片 / 片尾"也编码成 SELECT_TAIL：前面用
		// MOVE frame[1] 填影片名（op / staffroll），再发 0x37。
		// 原实现无脑走地图选择，结果 op 影片被当成空地图跳过，玩家点
		// "オープニングから始める"后只看到黑屏/直接进 day 1。
		// 先尝试影片；名字不是影片时再按地图/选择处理，两者的指令形状
		// 相同，靠资源是否存在来区分，不需要给剧本加特殊标记。
		if step, ok := r.movies.RequestIfMovie(); ok {
			return step
		}
		return r.stage.ApplySelect()
	case vm.SelectNext:
		return r.flow.WaitSelect()
	case vm.SelectEntry:
		// SELECT_ENTRY 的 4 字节参数决定语义（见 session.run 的处理）：
		//   arg >= 0：剧情里的场景结尾，登记鉴赏条目，继续执行；
		//   arg == -1：鉴赏回放的公共收尾，结束回放。
		// 这里保持 Continue，参数判定放在 session（需要访问全局存档）。
		return state.SessionContinue
	case vm.Evaluate:
		return r.movies.Request()
	case vm.Halt, vm.ReturnOne, vm.ReturnTwo:
		return state.SessionFinished
	default:
		return state.SessionContinue
	}
}
