package session

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
	"kizuna/engine/brand/bishop/state"
)

// SL 的剧本不画 START，引擎必须在进入自定义地图选择态时补上它；
// 否则选择态没有任何按钮，玩家会永远停在工作画面上。
func TestBeginCustomMapAddsConfiguredStart(t *testing.T) {
	s := &Session{
		config: &config.BsxxConfiguration{CustomMap: config.CustomMapPhase{
			Start: config.UIButton{Image: "work_start", Frames: 4, X: 128, Y: 528},
		}},
		gameState: state.NewGameState(),
	}
	s.beginCustomMap()
	buttons := s.gameState.Buttons()
	if len(buttons) != 1 {
		t.Fatalf("buttons = %d, want 1", len(buttons))
	}
	if buttons[0].Image != "work_start" || buttons[0].X != 128 || buttons[0].Y != 528 ||
		buttons[0].Frames != 4 {
		t.Fatalf("start button = %+v", buttons[0])
	}
}

// 剧本自己画了 START（SLIE）时不能重复补一颗。
func TestBeginCustomMapKeepsScriptStart(t *testing.T) {
	s := &Session{
		config: &config.BsxxConfiguration{CustomMap: config.CustomMapPhase{
			Start: config.UIButton{Image: "work_start", Frames: 4, X: 128, Y: 528},
		}},
		gameState: state.NewGameState(),
	}
	s.gameState.AddButton(state.NewStageButtonWithFrames("work_start", 170, 704, 0, 4))
	s.beginCustomMap()
	if got := len(s.gameState.Buttons()); got != 1 {
		t.Fatalf("buttons = %d, want 1 (script button only)", got)
	}
}

// 地图卡有 _hit 掩码时，命中只认掩码像素；掩码外的点不能退回
// 按钮的矩形（全屏卡图会让所有卡共享同一个矩形）。
func TestHitButtonPrefersMask(t *testing.T) {
	session := &Session{gameState: state.NewGameState()}
	masked := state.NewStageButtonWithFrames("card", 0, 0, 10, 2)
	masked.HitMask = &state.StageHitMask{
		Width:  4,
		Height: 4,
		Values: []uint8{
			0, 0, 0, 0,
			0, 9, 0, 0,
			0, 0, 0, 0,
			0, 0, 0, 0,
		},
	}
	session.gameState.AddButton(masked)
	if got := session.hitButton(1, 1); got != 0 {
		t.Fatalf("mask pixel hit = %d, want 0", got)
	}
	if got := session.hitButton(2, 2); got != -1 {
		t.Fatalf("mask miss = %d, want -1 (no rectangle fallback)", got)
	}

	// 无掩码的普通按钮继续走矩形命中。
	plain := state.NewStageButtonWithFrames("plain", 50, 50, 20, 2)
	plain.Width = 10
	plain.Height = 10
	session.gameState.AddButton(plain)
	if got := session.hitButton(55, 55); got != 1 {
		t.Fatalf("plain rectangle hit = %d, want 1", got)
	}
}

// 掩码按钮导出给宿主的热区必须是掩码外接矩形，而不是整张全屏卡图；
// 否则宿主画的指示框和按下标点击的中心点都会落在卡外。
func TestButtonRectUsesMaskBounds(t *testing.T) {
	session := &Session{gameState: state.NewGameState()}
	button := state.NewStageButtonWithFrames("card", 100, 200, 10, 2)
	button.HitMask = &state.StageHitMask{Width: 2, Height: 2, Values: []uint8{1, 0, 0, 0}}
	button.HitX, button.HitY, button.HitWidth, button.HitHeight = 150, 260, 8, 9
	x, y, width, height := session.buttonRect(button)
	if x != 150 || y != 260 || width != 8 || height != 9 {
		t.Fatalf("buttonRect = (%d,%d,%d,%d), want (150,260,8,9)", x, y, width, height)
	}
}
