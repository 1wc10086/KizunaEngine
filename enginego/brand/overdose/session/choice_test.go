package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/core"
)

// 本作最独特的部分：0x1d 的 adv_select2 菜单（interrupt_select 等）带屏幕坐标，
// 不能用 Flutter 的居中列表。这个测试直接跑 interrupt_select（list_script 第 14 项），
// 断言菜单真的从 VM 解析出来并由引擎绘制/命中。
func TestAdvancedChoiceMenu(t *testing.T) {
	dir := ""
	for _, c := range []string{os.Getenv("GAME_DIR"), "/root/code/engine/游戏/YUI/優衣"} {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "script.arc")); err == nil {
			dir = c
			break
		}
	}
	if dir == "" {
		t.Skip("未找到游戏数据")
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	step, err := s.Start(14) // list_script: 14 = interrupt_select
	if err != nil {
		t.Fatalf("Start(14): %v", err)
	}
	for i := 0; i < 500 && step != core.StepAwaitSelect; i++ {
		if step == core.StepFinished || step == core.StepFaulted {
			t.Fatalf("脚本在菜单前结束: step=%v err=%s", step, s.LastError())
		}
		step, err = s.Advance()
		if err != nil {
			t.Fatalf("Advance %d: %v", i, err)
		}
	}
	if step != core.StepAwaitSelect {
		t.Fatalf("没有进入高级选择菜单，step=%v", step)
	}
	if len(s.choiceMenu) < 2 {
		t.Fatalf("菜单项 = %d: %+v", len(s.choiceMenu), s.choiceMenu)
	}
	first := s.choiceMenu[0]
	if first.image != "adv_select2" || first.x <= 0 || first.y <= 0 {
		t.Fatalf("高级菜单几何/图名错误: %+v", first)
	}
	frame := composeOK(t, s)
	if !pixelNonBlack(frame, int(first.x)+10, int(first.y)+10) {
		t.Fatal("选择肢按钮没有画进帧")
	}
	// 点击第一项中心：应离开菜单并继续脚本。
	step, err = s.Pointer(3, first.x+10, first.y+10)
	if err != nil {
		t.Fatalf("choose: %v", err)
	}
	if step == core.StepAwaitSelect && len(s.choiceMenu) > 0 && s.choiceMenu[0].x == first.x {
		t.Fatal("点击菜单项后仍停在同一菜单")
	}
}
