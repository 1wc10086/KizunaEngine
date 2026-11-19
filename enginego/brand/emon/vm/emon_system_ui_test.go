package vm

import (
	"testing"

	"kizuna/enginego/brand/emon/script"
	"kizuna/enginego/brand/emon/stage"
)

// memorySaves 是只保留内存副本的存档后端，避免单测写用户游戏目录。
type memorySaves struct {
	data map[int]*EmonSaveData
}

func newMemorySaves() *memorySaves {
	return &memorySaves{data: map[int]*EmonSaveData{}}
}

func (s *memorySaves) SystemState() EmonSystemState { return &fakeSystemState{} }

func (s *memorySaves) Write(data *EmonSaveData) bool {
	if data == nil {
		return false
	}
	s.data[int(data.Slot)] = data
	return true
}

func (s *memorySaves) Read(slot int) *EmonSaveData { return s.data[slot] }
func (s *memorySaves) Exists(slot int) bool        { _, ok := s.data[slot]; return ok }
func (s *memorySaves) Delete(slot int) bool        { delete(s.data, slot); return true }

// TestNotereaAndRectRecording 覆盖两个渲染根因：
//   - NOTEREA 必须记成“不画位图”（滑杆轨道不能把 atlas 的 ON/OFF 画出来）；
//   - 源图未加载时声明的裁剪矩形也必须写入 spriteRects，否则 ReLoadSprite 后
//     会被 refreshLinkedFollowers 重置成整图尺寸。
func TestNotereaAndRectRecording(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "sprite-ui",
		"#SetSprite(knob,Main,0,0,0,NULL,10,20,30,40,DISPON,NOTEREA)\n"+
			"#End()\n")
	sprite := m.Context().Stage.Find("knob")
	if sprite == nil {
		t.Fatal("knob sprite was not created")
	}
	if !sprite.NoTextureArea() {
		t.Error("NOTEREA sprite must not draw its bitmap")
	}
	recorded, ok := m.spriteRects.Get("knob")
	if !ok || recorded != "10,20,30,40" {
		t.Fatalf("spriteRects = %q, %v; want 10,20,30,40", recorded, ok)
	}
}

// TestSpriteAreaClamps 验证 SPREREA 是移动范围而不是点击热区：
// 旋钮被拖到范围外时必须夹回 [x1, x2-width] / [y1, y2-height]。
func TestSpriteAreaClamps(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "sprite-area",
		"#SetSprite(knob,Main,0,0,0,NULL,0,0,10,10,DISPON,NOTEREA)\n"+
			"#End()\n")
	knob := m.Context().Stage.Find("knob")
	if knob == nil {
		t.Fatal("knob sprite was not created")
	}

	m.applySpriteArea([]string{"knob", "100", "200", "300", "220", "MACRO", "missing"})
	if knob.X() != 100 || knob.Y() != 200 {
		t.Fatalf("clamped position = (%d,%d), want (100,200)", knob.X(), knob.Y())
	}

	knob.SetX(999)
	knob.SetY(999)
	m.applySpriteArea([]string{"knob", "100", "200", "300", "220", "MACRO", "missing"})
	if knob.X() != 290 || knob.Y() != 210 {
		t.Fatalf("clamped position = (%d,%d), want (290,210)", knob.X(), knob.Y())
	}

	// SPRMOUSEMOVE 的持续跟随也必须夹在范围内。
	m.pointerX = 999
	m.pointerY = 999
	m.pointerDown = true
	m.applyMouseFollow(&MouseFollow{Sprite: "knob", RawDeltaX: "0", RawDeltaY: "0"}, true)
	if knob.X() != 290 || knob.Y() != 210 {
		t.Fatalf("follow position = (%d,%d), want (290,210)", knob.X(), knob.Y())
	}
}

// TestMakeSaveKeepsPrefetchedHud 复刻原引擎的存档宏顺序：
// MakeSave 先取景，随后宏隐藏独占/会話 HUD，最后 Save(slot)。
// 读档快照必须保留隐藏前的 HUD，否则用户读档后看不到游玩系统。
func TestMakeSaveKeepsPrefetchedHud(t *testing.T) {
	m := newTestMachine()
	store := newMemorySaves()
	m.Context().Saves = store

	hud := m.Context().Stage.GetOrCreate("独占モードスプライト", m.Context().Stage.Root())
	hud.SetKind(stage.EmonSpriteKindGroup)
	hud.SetVisible(true)
	knob := m.Context().Stage.GetOrCreate("knob", hud)
	knob.SetFlags(stage.EmonSpriteFlagsVisible | stage.EmonSpriteFlagsSelect)
	knob.SetSize(10, 10)
	knob.SetSelectOrder(42)
	m.frames = nil
	runBody(t, m, "save-flow",
		"#MakeSave\n"+
			"#SpriteSwitch(独占モードスプライト,DISPOFF)\n"+
			"#Save(1,cmt)\n"+
			"#End()\n")

	saved := store.Read(1)
	if saved == nil {
		t.Fatal("Save(1) did not write a slot")
	}
	visible := false
	selectOrder := 0
	for _, state := range saved.SpriteStates {
		if state.Name == "独占モードスプライト" {
			visible = state.Visible
		}
		if state.Name == "knob" {
			selectOrder = state.SelectOrder
		}
	}
	if selectOrder != 42 {
		t.Fatalf("saved select order = %d, want 42", selectOrder)
	}
	if !visible {
		t.Fatal("saved HUD must keep the visibility captured by MakeSave")
	}
	if !saved.ScreenRoots["独占モードスプライト"] {
		t.Fatal("ScreenRoots must record the HUD root")
	}
}

// TestRestoreModeHud 覆盖旧存档（ScreenRoots 里没有 HUD）的兜底：
// 读档后按 画面モード / 会話モード会話中 把系统 HUD 补开。
func TestRestoreModeHud(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "mode-hud",
		"#SetSprite(独占モードスプライト,Main,GROUP@MIDDLE_F,0,0)\n"+
			"#SetSprite(会話モードスプライト,Main,GROUP@MIDDLE_F,0,0)\n"+
			"#SetSprite(ミニ選択スプライト,Main,GROUP@MIDDLE_F,0,0)\n"+
			"#SpriteSwitch(独占モードスプライト,DISPOFF,会話モードスプライト,DISPOFF,ミニ選択スプライト,DISPOFF)\n"+
			"#End()\n")

	m.SetVariable("画面モード", script.EmonValueFromString("調教選択背景"))
	m.restoreModeHud()
	if !m.Context().Stage.Find("独占モードスプライト").Visible() {
		t.Fatal("training mode must restore the exclusive HUD")
	}

	m.SetVariable("会話モード会話中", script.EmonValueFromInt(1))
	m.restoreModeHud()
	if !m.Context().Stage.Find("会話モードスプライト").Visible() {
		t.Fatal("conversation mode must restore the conversation HUD")
	}
	if !m.Context().Stage.Find("ミニ選択スプライト").Visible() {
		t.Fatal("conversation mode must restore the mini select layer")
	}
}

// TestReparentOnSpriteRedefinition 验证同一名字的 SetSprite 换父级后
// AbsoluteX/Y 要按新父级计算（原引擎会重新挂到新父节点）。
func TestReparentOnSpriteRedefinition(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "reparent",
		"#SetSprite(A,Main,GROUP,0,0)\n"+
			"#SetSprite(B,Main,GROUP,100,0)\n"+
			"#SetSprite(C,A,0,10,10,NULL,0,0,10,10,DISPON,NOTEREA)\n"+
			"#SetSprite(C,B,0,10,10,NULL,0,0,10,10,DISPON,NOTEREA)\n"+
			"#End()\n")
	sprite := m.Context().Stage.Find("C")
	if sprite == nil {
		t.Fatal("C was not created")
	}
	if sprite.AbsoluteX() != 110 {
		t.Fatalf("C absolute x = %d, want 110 after reparenting", sprite.AbsoluteX())
	}
}

// TestGroupSoundLoopTracking 验证组音停止条件不再靠 groupFiles 的 key
// 推断循环属性：非循环组音也要能被 StopSound(SEGROUPNOLOOP,N) 停掉，
// 同时不能误杀循环组音。
func TestGroupSoundLoopTracking(t *testing.T) {
	m := newTestMachine()
	m.groupFiles[2] = "se.ogg"
	m.groupLoops[2] = false
	m.groupFiles[3] = "loop.ogg"
	m.groupLoops[3] = true

	m.stopGroupSoundFiltered(2, true, false)
	if _, ok := m.groupFiles[2]; ok {
		t.Fatal("non-loop group sound must be stopped by NOLOOP filter")
	}
	m.stopGroupSoundFiltered(3, true, false)
	if _, ok := m.groupFiles[3]; !ok {
		t.Fatal("loop group sound must survive NOLOOP filter")
	}
	m.stopGroupSoundFiltered(3, false, true)
	if _, ok := m.groupFiles[3]; ok {
		t.Fatal("loop group sound must be stopped by LOOP filter")
	}
}

// TestMainSpritePositionAndColor 覆盖 メイン 的两个特殊操作：
//   - SetSprPos/GetSprPos 移动的是舞台根节点（横ゆれ/縦ゆれ/震动）；
//   - SetSprColor 是整屏闪光（CCG白点滅/赤点滅/画面フラッシュ）。
func TestMainSpritePositionAndColor(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "main-sprite",
		"#SetSprPos(メイン,12,34)\n"+
			"#GetSprPos(メイン,rootX,rootY)\n"+
			"#SetSprColor(メイン,COLOR,0xff0000,128)\n"+
			"#End()\n")
	root := m.Context().Stage.Root()
	if root.X() != 12 || root.Y() != 34 {
		t.Fatalf("root pos = (%d,%d), want (12,34)", root.X(), root.Y())
	}
	if got := m.Context().Variables.Lookup("rootX").AsInt(); got != 12 {
		t.Fatalf("rootX = %d, want 12", got)
	}
	if root.Tint() != 0xff0000 || root.ColorAmount() != 128 {
		t.Fatalf("root color = %#x/%d, want 0xff0000/128", root.Tint(), root.ColorAmount())
	}
}
