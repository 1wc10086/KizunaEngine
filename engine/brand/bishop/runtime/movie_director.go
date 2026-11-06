package runtime

import (
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
	"kizuna/engine/core"
)

type MovieDirector struct {
	machine *vm.BsxMachine
	script  *vm.BsxScriptFile
	state   *state.GameState
}

func NewMovieDirector(machine *vm.BsxMachine, script *vm.BsxScriptFile, gameState *state.GameState) *MovieDirector {
	return &MovieDirector{machine: machine, script: script, state: gameState}
}

// RequestIfMovie 在 SELECT_TAIL / EVALUATE 共用的一种语义里尝试请求影片。
//
// BSXScript 3.x 的 SELECT_TAIL 被同时用于地图选择和影片播放：两者都先把
// 名字放进 frame[1]，再发 0x37。仅凭操作码区分不开，所以这里以"名字在
// movie 目录里是否存在"为准。返回 ok=false 表示不是影片，调用方应继续走
// 地图/选择逻辑；ok=true 时 step 一定是 AwaitMovie（或 Continue，当名字
// 空/影片不存在时不会 ok）。
func (d *MovieDirector) RequestIfMovie() (state.SessionStep, bool) {
	if d == nil || d.machine == nil || d.script == nil || d.state == nil {
		return state.SessionContinue, false
	}
	name := d.script.GetName(int(d.machine.Frame(1)))
	if len(name) == 0 {
		return state.SessionContinue, false
	}
	file, ok := d.state.MoviePath(name)
	if !ok {
		return state.SessionContinue, false
	}
	d.state.SetMovie(name)
	d.state.SetMovieSerial(d.state.MovieSerial() + 1)
	core.Logf("movie: '%s' -> %s", name, file)
	return state.SessionAwaitMovie, true
}

func (d *MovieDirector) Request() state.SessionStep {
	if step, ok := d.RequestIfMovie(); ok {
		return step
	}
	return state.SessionContinue
}
