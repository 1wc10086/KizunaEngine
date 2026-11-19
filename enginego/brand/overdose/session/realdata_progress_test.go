package session

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/core"
)

// 真数据回归：锁住 Rand_int / 动态 label / gosub 三个语义修复后，
// 剧情必须能离开序盤A 的日常循环（bool74 由 sc03 置位），并持续出现新对白。
// CI 没有游戏目录时 Skip；本地有数据时约 20s。
func TestRealDataStoryProgresses(t *testing.T) {
	dir := os.Getenv("GAME_DIR")
	if dir == "" {
		dir = "/root/code/engine/游戏/YUI/優衣"
	}
	if _, err := os.Stat(filepath.Join(dir, "script.arc")); err != nil {
		t.Skip("未找到游戏数据")
	}
	s := New()
	if _, err := s.OpenExtras(dir, nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	step, err := s.Start(1)
	if err != nil {
		t.Fatal(err)
	}
	s.settings.TextSpeed = 0
	if step == core.StepAwaitTitle {
		if step, err = s.SelectTitleItem("start"); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]bool{}
	progressed := false
	for i := 0; i < 50000 && step != core.StepFinished && step != core.StepFaulted; i++ {
		if b, _ := s.vm.Variables().GetBool(0, 74); b {
			progressed = true
		}
		if step == core.StepAwaitSelect {
			pick := uint32(len(s.choiceMenu) - 1)
			if len(s.choiceMenu) == 7 {
				// 兴奋度刷到 10 再"襲う"；其余情况最后一项是推进项。
				if v, _ := s.vm.Variables().GetInt(1, 62); v < 10 {
					pick = 5
				} else {
					pick = 6
				}
			}
			if len(s.choiceMenu) == 0 {
				step, err = s.Advance()
			} else {
				step, err = s.Choose(pick)
			}
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if txt := s.Text().Text; txt != "" {
			seen[fmt.Sprintf("%s#%d", s.currentScript, s.currentPC())] = true
		}
		step, err = s.Advance()
		if err != nil {
			t.Fatal(err)
		}
	}
	if step == core.StepFaulted {
		t.Fatalf("FAULT: %s", s.LastError())
	}
	if !progressed {
		t.Fatal("bool74 未置位：Rand_int 未生效，剧情卡在序盤A 循环")
	}
	if len(seen) < 1500 {
		t.Fatalf("只见到 %d 个对白位置，剧情没有真正推进", len(seen))
	}
	t.Logf("对白位置=%d，最终 step=%v", len(seen), step)
}
