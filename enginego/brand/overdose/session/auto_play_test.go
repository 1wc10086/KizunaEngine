package session

import (
	"fmt"
	"kizuna/enginego/core"
	"os"
	"path/filepath"
	"testing"
)

func TestAutoPlayNoFault(t *testing.T) {
	dir := ""
	for _, c := range []string{os.Getenv("GAME_DIR"), "/root/code/engine/游戏/YUI/優衣"} {
		if c != "" {
			if _, e := os.Stat(filepath.Join(c, "script.arc")); e == nil {
				dir = c
				break
			}
		}
	}
	if dir == "" {
		t.Skip()
	}
	s := New()
	if _, e := s.OpenExtras(dir, nil, t.TempDir()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Start(0); e != nil {
		t.Fatal(e)
	}
	step, e := s.SelectTitleItem("start")
	if e != nil {
		t.Fatal(e)
	}
	lines := 0
	choices := 0
	for i := 0; i < 20000; i++ {
		if step == core.StepAwaitChoice || step == core.StepAwaitSelect {
			choices++
			if st2, e := s.Choose(0); e != nil {
				t.Fatalf("choose %d: %v", i, e)
			} else {
				step = st2
			}
			continue
		}
		if step == core.StepFinished || step == core.StepFaulted {
			fmt.Println("end", i, "lines", lines, "choices", choices, "step", step, "err", s.LastError())
			return
		}
		st := s.Text()
		if st.Text != "" {
			lines++
		}
		if lines >= 2000 {
			fmt.Println("reached lines", lines, "choices", choices, "step", step)
			return
		}
		step, e = s.Advance()
		if e != nil {
			t.Fatalf("adv %d: %v", i, e)
		}
	}
	fmt.Println("budget end lines", lines, "choices", choices, "step", step)
}
