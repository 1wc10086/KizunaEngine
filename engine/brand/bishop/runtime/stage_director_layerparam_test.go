package runtime

import (
	"testing"

	"kizuna/engine/brand/bishop/state"
)

// TestApplyLayerPlacementMovesMapCardButton 覆盖放課後3 的地图卡摆位。
//
// 剧本用 CHAR type=160 把角色卡登记成按钮（坐标 0,0），随后用
// LAYER_PARAM type=0 按层号把卡移到 #X.mapXa/mapYa。只移动舞台层而不
// 移动按钮时，卡图虽然动了，热区仍全部叠在 (0,0)，玩家点哪都只会进
// 第一张卡的剧情。
func TestApplyLayerPlacementMovesMapCardButton(t *testing.T) {
	gameState := state.NewGameState()
	card := state.NewStageButtonWithFrames("01_talk01", 0, 0, 11, 3)
	card.Layer = 6
	gameState.AddButton(card)
	home := state.NewStageButtonWithFrames("map_home", 10, 670, 95, 2)
	gameState.AddButton(home)

	director := &StageDirector{state: gameState, mapLayerPlacements: map[int]mapLayerPlacement{}}
	director.applyLayerPlacement(6, 530, 585)

	buttons := gameState.Buttons()
	if buttons[0].X != 530 || buttons[0].Y != 585 {
		t.Fatalf("card moved to (%d,%d), want (530,585)", buttons[0].X, buttons[0].Y)
	}
	// 没有层号的工具栏按钮不能被同号 LAYER_PARAM 误移动。
	if buttons[1].X != 10 || buttons[1].Y != 670 {
		t.Fatalf("home button moved to (%d,%d), want (10,670)", buttons[1].X, buttons[1].Y)
	}
}

// TestApplyLayerPlacementStoresPositionForLaterCards 保证"先摆位后登记"也不丢坐标。
func TestApplyLayerPlacementStoresPositionForLaterCards(t *testing.T) {
	director := &StageDirector{state: state.NewGameState(), mapLayerPlacements: map[int]mapLayerPlacement{}}
	director.applyLayerPlacement(9, 295, 205)
	placement, ok := director.mapLayerPlacements[9]
	if !ok || placement.x != 295 || placement.y != 205 {
		t.Fatalf("placement = %+v, ok=%t", placement, ok)
	}
}

// TestMoveLayerToKeepsRelativeParts 验证多层合成层按整层平移，而不是把所有部件吸到同一点。
func TestMoveLayerToKeepsRelativeParts(t *testing.T) {
	gameState := state.NewGameState()
	gameState.SetLayerAt(2, "date", state.StageLayerScreen, []state.LayerArtwork{
		{Image: "map_date_base", X: 210, Y: 678},
		{Image: "map_week01", X: 231, Y: 691},
	})
	if !gameState.MoveLayerTo(2, 310, 700) {
		t.Fatal("MoveLayerTo = false, want true")
	}
	pieces := gameState.LayerAt(2).Artworks()
	if pieces[0].X != 310 || pieces[0].Y != 700 {
		t.Fatalf("first piece = (%d,%d), want (310,700)", pieces[0].X, pieces[0].Y)
	}
	if pieces[1].X != 331 || pieces[1].Y != 713 {
		t.Fatalf("second piece = (%d,%d), want (331,713)", pieces[1].X, pieces[1].Y)
	}
}
