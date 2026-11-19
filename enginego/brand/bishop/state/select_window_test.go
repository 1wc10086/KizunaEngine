package state

import "testing"

// TestBeginSelectHidesMessageWindow 是"地图上还挂着上一句台词"这个 bug 的回归测试。
//
// 进地图前玩家刚读完一句对白，对话框是可见的；原引擎会把消息窗收掉再铺地图，
// 我们的引擎之前没有，于是地图全屏铺开后下方仍然悬着那句台词。
func TestBeginSelectHidesMessageWindow(t *testing.T) {
	state := NewGameState()
	state.SetMessageWindowVisible(true)

	state.BeginSelect()
	if state.MessageWindowVisible() {
		t.Fatal("message window should be hidden while selecting")
	}
	if !state.Selecting() {
		t.Fatal("state should be selecting")
	}

	state.EndSelect()
	if !state.MessageWindowVisible() {
		t.Fatal("message window should be restored after the select")
	}
}

// TestEndSelectKeepsHiddenWindow 验证"进选择前对话框本来就是关的"时不会被强行打开。
//
// 纯演出段（黑屏过渡、只有立绘）常常先关掉消息窗再进地图；
// 如果退出时无条件显示，会凭空多出一个空对话框。
func TestEndSelectKeepsHiddenWindow(t *testing.T) {
	state := NewGameState()
	state.SetMessageWindowVisible(false)

	state.BeginSelect()
	state.EndSelect()
	if state.MessageWindowVisible() {
		t.Fatal("a window that was hidden before the select must stay hidden")
	}
}

// TestEndSelectWithoutBegin 保证 Reset/ClearStage 路径不会篡改可见性。
//
// Reset 会调 EndSelect，但那时并没有"选择中"；若无条件恢复可见性，
// 标题画面（明确设成不可见）会被这里改回可见，凭空画出空框。
func TestEndSelectWithoutBegin(t *testing.T) {
	state := NewGameState()
	state.SetMessageWindowVisible(false)
	state.EndSelect()
	if state.MessageWindowVisible() {
		t.Fatal("EndSelect outside a select must not change visibility")
	}
}

// TestBeginSelectIsIdempotent 防止重复 BeginSelect 把"原始状态"覆盖成已隐藏。
//
// 地图目录与选择窗口两条路径都可能调 BeginSelect；若第二次把
// selectWindowHidden 记成 true，退出时就再也不会恢复对话框。
func TestBeginSelectIsIdempotent(t *testing.T) {
	state := NewGameState()
	state.SetMessageWindowVisible(true)
	state.BeginSelect()
	state.BeginSelect()
	state.EndSelect()
	if !state.MessageWindowVisible() {
		t.Fatal("repeated BeginSelect corrupted the saved visibility")
	}
}
