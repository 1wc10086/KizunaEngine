package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/core"
)

// sop4 是用户反馈"HCG/场景无限重复"的现场：0x0a 的动态 label 是
// "talk" + var(1,50)，0x0b 是跳转而不是压帧。回归要求：选两次"話す"后
// 进入 end 场景并结束，菜单最多出现 2 次。
func TestSop4DynamicLabelEnds(t *testing.T) {
	dir := "/root/code/engine/游戏/YUI/優衣"
	if _, err := os.Stat(filepath.Join(dir, "script.arc")); err != nil {
		t.Skip()
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.settings.TextSpeed = 0
	step, err := s.Start(110) // 110 = list_script.tbl 里的 sop4
	if err != nil {
		t.Fatal(err)
	}
	menus := 0
	for i := 0; i < 20000; i++ {
		if step == core.StepFinished {
			t.Logf("finished after %d steps, menus=%d", i, menus)
			return
		}
		if step == core.StepFaulted {
			t.Fatalf("fault: %s", s.LastError())
		}
		if step == core.StepAwaitSelect {
			menus++
			var texts []string
			for _, it := range s.choiceMenu {
				texts = append(texts, it.text)
			}
			t.Logf("menu#%d %v", menus, texts)
			if menus > 6 {
				t.Fatalf("sop4 菜单未收敛")
			}
			pick := uint32(0) // 优先"話す"
			for i, it := range s.choiceMenu {
				if it.text == "話す" {
					pick = uint32(i)
					break
				}
			}
			step, err = s.Choose(pick)
		} else {
			step, err = s.Advance()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("sop4 未结束 step=%v", step)
}
