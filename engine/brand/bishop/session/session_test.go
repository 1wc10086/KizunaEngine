package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/state"
	"kizuna/engine/core"
)

func TestMapSessionStep(t *testing.T) {
	cases := map[state.SessionStep]core.Step{
		state.SessionContinue:    core.StepContinue,
		state.SessionAwaitInput:  core.StepAwaitInput,
		state.SessionAwaitChoice: core.StepAwaitChoice,
		state.SessionAwaitSelect: core.StepAwaitSelect,
		state.SessionAwaitMap:    core.StepFaulted,
		state.SessionWait:        core.StepWait,
		state.SessionFinished:    core.StepFinished,
		state.SessionFaulted:     core.StepFaulted,
	}
	for in, want := range cases {
		if got := mapSessionStep(in); got != want {
			t.Fatalf("mapSessionStep(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestRoundHalfEven(t *testing.T) {
	cases := map[float64]int{2.5: 2, 3.5: 4, 2.4: 2, 2.6: 3, 128.0: 128, 96.0: 96}
	for in, want := range cases {
		if got := roundHalfEven(in); got != want {
			t.Fatalf("roundHalfEven(%v) = %d, want %d", in, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	s := &Session{gameState: state.NewGameState()}
	s.gameState.SetSpeaker("Alice")
	s.gameState.SetText("hello\nworld   ")
	if got := s.summarize(); got != "Alice: hello world" {
		t.Fatalf("summarize = %q", got)
	}
	s.gameState.SetSpeaker("")
	s.gameState.SetText("01234567890123456789012345678901234567890123456789")
	if got := s.summarize(); len([]rune(got)) != 42 {
		t.Fatalf("summarize runes = %d", len([]rune(got)))
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "data", "bsxx.dat"))
	writeFile(t, filepath.Join(root, "data", "game.cfg"))
	dataDir, err := resolveDataDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dataDir) != "data" {
		t.Fatalf("dataDir = %q", dataDir)
	}
	script, err := findScript(dataDir)
	if err != nil || filepath.Base(script) != "bsxx.dat" {
		t.Fatalf("script = %q err = %v", script, err)
	}
	cfg := findConfiguration(dataDir)
	if filepath.Base(cfg) != "game.cfg" {
		t.Fatalf("config = %q", cfg)
	}
	if got := findFile(dataDir, "cg.dat"); got != "" {
		t.Fatalf("findFile = %q", got)
	}
	if _, err := resolveDataDir(t.TempDir()); err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestFreshSession(t *testing.T) {
	var s Session
	if got := s.Audio(); len(got) != 0 {
		t.Fatalf("audio = %d", len(got))
	}
	if got := len(s.Scenes()); got != 0 {
		t.Fatalf("scenes = %d", got)
	}
	if got := s.LastError(); got != "" {
		t.Fatalf("lastError = %q", got)
	}
	if got := s.TextList(); got != nil {
		t.Fatalf("textList = %v", got)
	}
	if _, err := s.Start(0); err == nil {
		t.Fatal("expected not-open error")
	}
	if _, err := s.Load(0); err == nil {
		t.Fatal("expected not-open error")
	}
	if err := s.Save(0, ""); err == nil {
		t.Fatal("expected not-open error")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
