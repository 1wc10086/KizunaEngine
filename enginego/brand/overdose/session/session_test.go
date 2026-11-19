package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kizuna/enginego/core"
)

// 真数据端到端：Open -> 标题 -> Start -> 多句对白 -> 合成 -> 存/读。
// 没有游戏目录时跳过；CI 的普通单测不会因此变红。
func TestPlaythroughSmoke(t *testing.T) {
	cands := []string{os.Getenv("GAME_DIR"), "/root/code/engine/游戏/YUI/優衣"}
	dir := ""
	for _, c := range cands {
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
		t.Fatalf("Open: %v", err)
	}
	if step, err := s.Start(0); err != nil || step != core.StepAwaitTitle {
		t.Fatalf("Start title = %v, %v", step, err)
	}
	if items := s.TitleMenu(); len(items) == 0 {
		t.Fatal("标题菜单为空")
	}
	frame := make([]byte, 800*600*4)
	if ok, _ := s.Compose(unsafePointer(&frame[0]), len(frame)); !ok {
		t.Fatal("标题合成失败")
	}
	step, err := s.SelectTitleItem("start")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Logf("after start step=%v text=%q", step, s.Text().Text)
	lines := []string{}
	frames := 0
	audioSeen := false
	for i := 0; i < 400; i++ {
		st := s.Text()
		if st.Text != "" {
			lines = append(lines, st.Text)
		}
		if len(lines) >= 60 {
			break
		}
		step, err = s.Advance()
		if err != nil {
			t.Fatalf("Advance %d: %v", i, err)
		}
		if step == core.StepFinished || step == core.StepFaulted {
			t.Logf("stopped at %v after %d lines", step, len(lines))
			break
		}
		if frames < 3 && step == core.StepAwaitInput {
			if ok, _ := s.Compose(unsafePointer(&frame[0]), len(frame)); !ok {
				t.Fatal("剧本合成失败")
			}
			nonBlack := false
			for i := 0; i < len(frame); i += 4 {
				if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
					nonBlack = true
					break
				}
			}
			if !nonBlack {
				t.Fatal("剧本帧全黑（背景/立绘没合成）")
			}
			frames++
		}
		for _, ev := range s.Audio() {
			if ev.Path == "" {
				continue
			}
			if _, err := os.Stat(ev.Path); err != nil {
				t.Fatalf("音频物化文件不存在: %s", ev.Path)
			}
			audioSeen = true
		}
	}
	if !audioSeen {
		t.Fatal("前几段没有产生可用的音频事件")
	}
	if len(lines) == 0 {
		t.Fatalf("没有对白；step=%v err=%v text=%q", step, err, s.Text().Text)
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "――") || strings.Contains(l, "匂い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("前几段对白不含预期文本: %q", lines[:min(3, len(lines))])
	}
	// 存档/读档：把当前状态写进槽 1，再读回并确认文本一致。
	before := s.Text()
	if err := s.Save(1, "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if slots := s.Slots(); len(slots) == 0 || !slots[0].Exists {
		t.Fatalf("Slots 未列出存档: %+v", slots)
	}
	if _, err := s.Load(1); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s.Text(); got.Speaker != before.Speaker || got.Text != before.Text {
		t.Fatalf("读档文本不一致: %q/%q vs %q/%q", got.Speaker, got.Text, before.Speaker, before.Text)
	}
	if scenes := s.Scenes(); len(scenes) == 0 {
		t.Fatal("鉴赏场景为空")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
