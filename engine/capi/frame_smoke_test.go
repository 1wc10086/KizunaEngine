package capi

import (
	"testing"

	"kizuna/engine/core"
	"kizuna/engine/wire"
)

// frameEngine 只实现 OpFrame 会用到的那几个方法，其余 core.Engine 方法靠嵌入的 nil
// 接口满足签名：这个测试因此不需要任何真实游戏资源。字段号是 Dart 侧的读取契约，
// 这里逐个钉死，避免以后有人改字段号而宿主静默读错。
type frameEngine struct {
	core.Engine
	step core.Step
	text core.TextState
	st   core.State
	ev   []core.AudioEvent
}

func (f *frameEngine) CurrentStep() core.Step   { return f.step }
func (f *frameEngine) Text() core.TextState     { return f.text }
func (f *frameEngine) State() core.State        { return f.st }
func (f *frameEngine) Audio() []core.AudioEvent { return f.ev }

// frameEngineNoStep 故意不实现 core.StepReporter，用来验证其他品牌走 StepContinue。
type frameEngineNoStep struct {
	core.Engine
}

func (f frameEngineNoStep) Text() core.TextState     { return core.TextState{} }
func (f frameEngineNoStep) State() core.State        { return core.State{} }
func (f frameEngineNoStep) Audio() []core.AudioEvent { return nil }

func TestOpFrameFields(t *testing.T) {
	e := &frameEngine{
		step: core.StepAwaitInput,
		text: core.TextState{Speaker: "speaker", Text: "text", Awaiting: 7},
		st: core.State{
			Bgm:                  "bgm",
			PC:                   11,
			Scene:                22,
			Animating:            true,
			WaitMs:               33,
			MessageWindowVisible: true,
			AwaitingName:         true,
			BackLogActive:        true,
			ReplayActive:         true,
			InstructionStart:     44,
			NamePrompt:           "name",
		},
		ev: []core.AudioEvent{{
			Kind: 2, Path: "voice/1.ogg", Volume: 0.5, Loop: true,
			Channel: 3, Group: 4, Names: []string{"a"}, Frames: 5,
		}},
	}
	status, r := invokeSmoke(t, &handle{engine: e}, wire.OpFrame, 9, nil)
	if status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if got := r.U32(10); got != uint32(core.StepAwaitInput) {
		t.Fatalf("f10 step=%d", got)
	}
	if r.Str(30) != "speaker" || r.Str(31) != "text" || r.U32(32) != 7 {
		t.Fatalf("text=%q/%q/%d", r.Str(30), r.Str(31), r.U32(32))
	}
	if r.Str(35) != "bgm" || r.U32(36) != 11 || r.U32(37) != 22 {
		t.Fatalf("state bgm/pc/scene=%q/%d/%d", r.Str(35), r.U32(36), r.U32(37))
	}
	if !r.Bool(38) || r.U32(45) != 33 || !r.Bool(46) || !r.Bool(47) ||
		!r.Bool(48) || !r.Bool(49) || r.U32(60) != 44 || r.Str(79) != "name" {
		t.Fatal("state flags mismatch")
	}
	count := 0
	r.Each(26, func(item *wire.Reader) {
		count++
		if item.U32(27) != 2 || item.Str(1) != "voice/1.ogg" || item.F64(28) != 0.5 ||
			!item.Bool(29) || item.U32(3) != 3 || item.U32(69) != 4 ||
			item.U32(71) != 5 || len(item.StrList(70)) != 1 {
			t.Fatalf("audio item mismatch: kind=%d path=%q", item.U32(27), item.Str(1))
		}
	})
	if count != 1 {
		t.Fatalf("audio count=%d", count)
	}
}

func TestOpFrameStepFallback(t *testing.T) {
	status, r := invokeSmoke(t, &handle{engine: frameEngineNoStep{}}, wire.OpFrame, 10, nil)
	if status != wire.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if got := r.U32(10); got != uint32(core.StepContinue) {
		t.Fatalf("f10 step=%d want Continue", got)
	}
}
