package runtime

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/resource"
	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/brand/bishop/vm"
)

// newParkedButtonScript 造一份只含一个名字（card16）的最小现代剧本。
//
// ShowImageExtended 通过 frame[1] 取名字表下标、frame[2..4] 取类型与坐标。
// 这里只需要名字表与一段合法代码，不跑指令，所以代码区放一条 HALT。
func newParkedButtonScript(t *testing.T) *vm.BsxScriptFile {
	t.Helper()
	data := make([]byte, 0x160)
	copy(data, "BSXScript 3.00")
	putInt32 := func(offset, value int) {
		data[offset] = byte(value)
		data[offset+1] = byte(value >> 8)
		data[offset+2] = byte(value >> 16)
		data[offset+3] = byte(value >> 24)
	}
	putInt32(0x2C, 0x150) // code offset
	putInt32(0x30, 1)     // code size
	// name group：一条表项，名字块从 0x98 开始。
	putInt32(0x48, 0x94) // list offset
	putInt32(0x4C, 4)    // list size
	putInt32(0x50, 0x98) // block offset
	putInt32(0x54, 16)   // block size
	putInt32(0x94, 0)    // first name at block offset 0
	for index, character := range []byte("card16") {
		data[0x98+index*2] = character
	}
	data[0x150] = vm.OpcodeHalt

	script, err := vm.ParseBsxScriptFile(data)
	if err != nil {
		t.Fatalf("parse parked-button script: %v", err)
	}
	return script
}

// TestParkedMapButtonGetsAutoLayer 覆盖学園3/学園3IE 的行动卡摆位。
//
// 剧本用 `IMAGE_EX type=160, x=-800` 把卡停在屏幕外，再用
// `LAYER_PARAM type=0` 按 #layercount（6、7…）把卡摆到卡位。按钮不占
// 自动层号时 MoveButtonByLayer 找不到它，五张行动卡会永远留在 -800，
// 画面上只剩 "Action slot" 底板，玩家无法选择行动。
func TestParkedMapButtonGetsAutoLayer(t *testing.T) {
	script := newParkedButtonScript(t)
	machine := vm.NewBsxMachine(script)
	gameState := state.NewGameState()
	configuration, err := config.Load("/nonexistent-parked-button.ini")
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	director := NewStageDirector(machine, script, gameState, configuration, &resource.GameResourceIndex{})

	machine.SetFrame(1, 0)    // name index: card16
	machine.SetFrame(2, 160)  // IMAGE_EX type: map button
	machine.SetFrame(3, -800) // 屏幕外哨兵
	machine.SetFrame(4, 0)
	director.ShowImageExtended()

	buttons := gameState.Buttons()
	if len(buttons) != 1 {
		t.Fatalf("buttons = %d, want 1", len(buttons))
	}
	if buttons[0].Layer != 1 {
		t.Fatalf("parked button layer = %d, want 1", buttons[0].Layer)
	}
	director.applyLayerPlacement(1, 144, 54)
	buttons = gameState.Buttons()
	if buttons[0].X != 144 || buttons[0].Y != 54 {
		t.Fatalf("card at (%d,%d), want (144,54)", buttons[0].X, buttons[0].Y)
	}
}

// TestAbsoluteMapButtonKeepsNoLayer 保证普通地图卡不受影响。
//
// 只有负坐标（停在屏幕外等待 LAYER_PARAM）的按钮才申请自动层号。
// KZK / 放課後3 等直接给屏幕坐标的卡保持 layer=-1，不会被同号
// LAYER_PARAM 误移动。
func TestAbsoluteMapButtonKeepsNoLayer(t *testing.T) {
	script := newParkedButtonScript(t)
	machine := vm.NewBsxMachine(script)
	gameState := state.NewGameState()
	configuration, err := config.Load("/nonexistent-absolute-button.ini")
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	director := NewStageDirector(machine, script, gameState, configuration, &resource.GameResourceIndex{})

	machine.SetFrame(1, 0)
	machine.SetFrame(2, 160)
	machine.SetFrame(3, 348)
	machine.SetFrame(4, 490)
	director.ShowImageExtended()

	buttons := gameState.Buttons()
	if len(buttons) != 1 {
		t.Fatalf("buttons = %d, want 1", len(buttons))
	}
	if buttons[0].Layer != -1 {
		t.Fatalf("absolute button layer = %d, want -1", buttons[0].Layer)
	}
}
