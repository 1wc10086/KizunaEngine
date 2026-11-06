package session

import (
	"testing"

	"kizuna/engine/brand/bishop/runtime/bsmap"
	"kizuna/engine/brand/bishop/state"
)

// 地图上的系统按钮（学園天堂的 config/log/...）点击后必须保持选择态：
// 不结束选择、不清空角色卡。工具键认不出时同样停在地图上，
// 不能落到写回地点结果的路径。
func TestMapSystemButtonKeepsSelection(t *testing.T) {
	s := &Session{gameState: state.NewGameState()}
	s.mapItems = []bsmap.BsMapItem{bsmap.NewBsMapItem("map_sakura", 0, 0, 0, 0, 0, 10, 10, "")}
	s.gameState.AddButton(state.NewStageButtonWithFrames("map_sakura", 0, 0, 0, 1))
	s.mapSystemCount = 1
	s.gameState.AddButton(state.NewStageButtonWithSize("map_unknown", 0, 0, 0, 3, 48, 48))
	s.gameState.BeginSelect()

	if step := s.chooseButton(1); step != state.SessionContinue {
		t.Fatalf("step = %v, want SessionContinue", step)
	}
	if !s.gameState.Selecting() {
		t.Fatal("system button ended the map selection")
	}
	if got := len(s.gameState.Buttons()); got != 2 {
		t.Fatalf("buttons = %d, want 2 (map cards must stay)", got)
	}
	if len(s.mapItems) != 1 {
		t.Fatalf("mapItems = %d, want 1", len(s.mapItems))
	}
}
