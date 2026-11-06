package runtime

import (
	"testing"

	"kizuna/engine/brand/bishop/state"
)

// TestBestChangeTarget 验证 CHANGE_IMAGE 在多层合成图层里只换目标部件。
//
// 地图前景层同时有日期卡、周数卡和各地点条；切换某个地点的状态时，
// 不能把同层其它部件一起清掉。
func TestBestChangeTarget(t *testing.T) {
	pieces := []state.LayerArtwork{
		{Image: "place1_event_bar1", X: 244, Y: 264},
		{Image: "place5_event_bar1", X: 772, Y: 517},
		{Image: "map_date_base", X: 8, Y: 8},
		{Image: "map_week1", X: 8, Y: 8},
	}
	if index := bestChangeTarget(pieces, "place1_event_bar3"); index != 0 {
		t.Fatalf("place1 target = %d, want 0", index)
	}
	if index := bestChangeTarget(pieces, "place5_event_bar2"); index != 1 {
		t.Fatalf("place5 target = %d, want 1", index)
	}
	// 和任何部件都只共享很短前缀时不能乱换。
	if index := bestChangeTarget(pieces, "place2_event_bar1"); index != -1 {
		t.Fatalf("unrelated target = %d, want -1", index)
	}
}

func TestCommonPrefixLength(t *testing.T) {
	if got := commonPrefixLength("place1_event_bar1", "place1_event_bar3"); got != 16 {
		t.Fatalf("prefix = %d", got)
	}
	if got := commonPrefixLength("abc", "xyz"); got != 0 {
		t.Fatalf("prefix = %d", got)
	}
}

// 前景叠图按顺序占层，但不能覆盖角色层：KZK 的地图把角色状态条
// 放在 2..8，随后的 IMAGE type=2 已完成标记必须落到角色层之后。
func TestNextForegroundPlaneSkipsCharacterLayers(t *testing.T) {
	gameState := state.NewGameState()
	gameState.SetLayerAt(1, "screen", state.StageLayerScreen, []state.LayerArtwork{{Image: "bg"}})
	gameState.SetLayerAt(2, "bar", state.StageLayerCharacter, []state.LayerArtwork{{Image: "map_base_ch1"}})
	director := &StageDirector{state: gameState}
	if plane := director.nextForegroundPlane(); plane != 1 {
		t.Fatalf("first foreground plane = %d, want 1", plane)
	}
	if plane := director.nextForegroundPlane(); plane != 3 {
		t.Fatalf("second foreground plane = %d, want 3 (skip character plane 2)", plane)
	}
	if plane := director.nextForegroundPlane(); plane != 4 {
		t.Fatalf("third foreground plane = %d, want 4", plane)
	}
}
