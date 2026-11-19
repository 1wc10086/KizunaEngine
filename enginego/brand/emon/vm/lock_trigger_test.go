package vm

import "testing"

// TestLockDoesNotBlockSpriteClicks 回归 NOZOMI 标题按钮“全死”的问题。
//
// #Lock 在原引擎里只冻结对白送り（sub_422510 只改消息标志），标题菜单正是
// 在 #Lock 后的轮询循环里等 SPRCLICK 把 SelectNumber 设好。早期实现把
// #Lock 当成全局输入锁，triggerEnabled/Tap 会丢弃锁定期间的所有精灵触发器，
// 结果标题按钮、工具条全部点不动。这里直接验证锁定状态下 SPRCLICK 仍然执行。
func TestLockDoesNotBlockSpriteClicks(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "locked-click",
		"#SetSprite(btn,Main,0,0,0,NULL,10,10,20,20,DISPON@SELECT,NOTEREA)\n"+
			"#SetRunAction(hit,FLAGON,clicked)\n"+
			"#SetAction(選択,SPRCLICK,btn,hit)\n"+
			"#Lock()\n"+
			"#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("setup step = %d, want finished", int(step))
	}
	sprite := m.Context().Stage.Find("btn")
	if sprite == nil {
		t.Fatal("btn sprite was not created")
	}
	if !m.Locked() {
		t.Fatal("setup should leave the machine locked")
	}
	m.fireTriggers(sprite, EmonInputLeftClick, 15, 15, 0)
	if got := m.Lookup("clicked").AsInt(); got != 1 {
		t.Fatalf("SPRCLICK handler did not run while locked (clicked=%d)", got)
	}
}
