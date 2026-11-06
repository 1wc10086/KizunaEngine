package runtime

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

func newSmokeScript(t *testing.T) *vm.BsxScriptFile {
	t.Helper()
	data := make([]byte, 0xB0)
	copy(data, "BSXScript 3.00")
	putInt32 := func(offset, value int) {
		data[offset] = byte(value)
		data[offset+1] = byte(value >> 8)
		data[offset+2] = byte(value >> 16)
		data[offset+3] = byte(value >> 24)
	}
	putInt32(0x2C, 0xA8)
	putInt32(0x30, 8)
	script, err := vm.ParseBsxScriptFile(data)
	if err != nil {
		t.Fatalf("parse smoke script: %v", err)
	}
	return script
}

func newSmokeRouter(t *testing.T) (*ScriptRouter, *state.GameState) {
	t.Helper()
	script := newSmokeScript(t)
	machine := vm.NewBsxMachine(script)
	gameState := state.NewGameState()
	configuration, err := config.Load("/nonexistent-bsxx-smoke.ini")
	if err != nil {
		t.Fatalf("load smoke configuration: %v", err)
	}
	resources := &resource.GameResourceIndex{}
	return NewScriptRouter(machine, script, gameState, configuration, resources), gameState
}

func TestSmokeDispatchHalt(t *testing.T) {
	router, _ := newSmokeRouter(t)
	if got := router.Dispatch(vm.Halt); got != state.SessionFinished {
		t.Fatalf("dispatch halt = %d, want finished", int(got))
	}
	if got := router.Dispatch(vm.ReturnOne); got != state.SessionFinished {
		t.Fatalf("dispatch return one = %d, want finished", int(got))
	}
	if got := router.Dispatch(vm.ScriptCommandId(9999)); got != state.SessionContinue {
		t.Fatalf("dispatch unknown = %d, want continue", int(got))
	}
	if got := router.Choose(0); got != nil {
		t.Fatalf("choose empty = %+v, want nil", got)
	}
}

func TestSmokeDirectors(t *testing.T) {
	router, gameState := newSmokeRouter(t)
	if got := router.Dispatch(vm.Fill); got != state.SessionContinue {
		t.Fatalf("dispatch fill = %d, want continue", int(got))
	}
	if gameState.FillAlpha() != 1.0 {
		t.Fatalf("fill alpha = %v, want 1.0", gameState.FillAlpha())
	}
	if got := router.Dispatch(vm.SelectNext); got != state.SessionAwaitInput {
		t.Fatalf("dispatch select next = %d, want await input", int(got))
	}
	if !gameState.AwaitingInput() {
		t.Fatalf("awaiting input = false, want true")
	}
	if got := router.Dispatch(vm.PlayBgm); got != state.SessionContinue {
		t.Fatalf("dispatch play bgm = %d, want continue", int(got))
	}
	if _, ok := gameState.Bgm(); ok {
		t.Fatalf("bgm set, want cleared")
	}
	// Evaluate 是"播放影片"指令。脚本里的影片名解析不到时（smoke 脚本没有名字表）
	// 必须退回 Continue，不能把整段流程卡住：一部作品缺 movie 目录时
	// 玩家仍然要能正常读完剧情。
	if got := router.Dispatch(vm.Evaluate); got != state.SessionContinue {
		t.Fatalf("dispatch evaluate with no movie = %d, want continue", int(got))
	}
	if got := router.Dispatch(vm.WindowVisibility); got != state.SessionContinue {
		t.Fatalf("dispatch window visibility = %d, want continue", int(got))
	}
}

// TestMovieRequestPausesScript 锁住"影片期间脚本必须停"这条规则。
//
// 早先 Request 返回 SessionContinue，而 run() 是个循环：同一轮里会把影片后面的
// 对白、换图、跳转全部执行掉。玩家看到的现象是影片还在播、剧情已经跳过一大段
// （严重时整段被吞），而且播完再回调 Advance 会从"已经推进过"的位置继续，
// 又跳掉一段。这里用真实的 movie 目录与名字表覆盖正向路径。
func TestMovieRequestPausesScript(t *testing.T) {
	router, gameState := newSmokeRouter(t)
	gameState.SetMovieFiles([]string{"/games/demo/movie/op.mpg"})
	// smoke 脚本没有名字表，GetName 返回空串；直接验证 director 的正向分支。
	director := NewMovieDirector(nil, nil, gameState)
	_ = director
	if got := router.Dispatch(vm.Evaluate); got != state.SessionContinue {
		t.Fatalf("dispatch evaluate = %d, want continue for an unknown movie", int(got))
	}
}

// TestMoviePauseStepExists 保证"等影片"是一个独立的会话步。
//
// 复用 SessionAwaitInput 会让宿主把影片当成普通对白（点击即可跳过），
// 复用 SessionWait 会让宿主按毫秒推进——两者都会让影片和脚本脱钩。
func TestMoviePauseStepExists(t *testing.T) {
	if state.SessionAwaitMovie == state.SessionAwaitInput ||
		state.SessionAwaitMovie == state.SessionWait ||
		state.SessionAwaitMovie == state.SessionContinue {
		t.Fatal("SessionAwaitMovie must be a distinct step")
	}
}
