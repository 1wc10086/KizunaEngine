package session

import (
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/brand/emon/vm"
	"kizuna/enginego/core"
)

func TestMapEmonStep(t *testing.T) {
	cases := []struct {
		name     string
		step     vm.EmonStep
		backlog  bool
		selectOn bool
		want     core.Step
	}{
		{"continue", vm.EmonStepContinue, false, false, core.StepContinue},
		{"await input", vm.EmonStepAwaitInput, false, false, core.StepAwaitInput},
		{"await choice plain", vm.EmonStepAwaitChoice, false, false, core.StepAwaitChoice},
		{"await choice select", vm.EmonStepAwaitChoice, false, true, core.StepAwaitSelect},
		{"await action plain", vm.EmonStepAwaitAction, false, false, core.StepAwaitSelect},
		{"await action select", vm.EmonStepAwaitAction, false, true, core.StepAwaitSelect},
		{"await name", vm.EmonStepAwaitName, false, false, core.StepAwaitName},
		{"wait", vm.EmonStepWait, false, false, core.StepWait},
		{"finished", vm.EmonStepFinished, false, false, core.StepFinished},
		{"faulted", vm.EmonStepFaulted, false, false, core.StepFaulted},
		{"backlog continue", vm.EmonStepContinue, true, false, core.StepWait},
		{"backlog choice select", vm.EmonStepAwaitChoice, true, true, core.StepWait},
		{"backlog name", vm.EmonStepAwaitName, true, false, core.StepWait},
	}
	for _, tc := range cases {
		if got := mapEmonStep(tc.step, tc.backlog, tc.selectOn); got != tc.want {
			t.Errorf("%s: got %d want %d", tc.name, int(got), int(tc.want))
		}
	}
}

func TestPlaybackMapping(t *testing.T) {
	s := New()
	play := func(kind vm.EmonPlaybackKind, file string, group, channel int, loop bool, volume, frames int) core.AudioEvent {
		before := len(s.Audio())
		s.onAudioCommand(vm.EmonPlaybackCommand{Kind: kind, File: file, Group: group, Channel: channel, Loop: loop, Volume: volume, Frames: frames})
		events := s.Audio()
		if len(events) != before+1 && len(events) != 1 {
			t.Fatalf("kind %d: got %d events", int(kind), len(events))
		}
		return events[len(events)-1]
	}
	music := play(vm.EmonPlaybackPlay, "bgm01", -1, -1, true, 200, 0)
	if music.Kind != 0 || music.Path != "bgm01" || !music.Loop || music.Channel != noTarget || music.Group != noTarget {
		t.Errorf("bgm play mapped to %+v", music)
	}
	voice := play(vm.EmonPlaybackPlay, "v01", -1, vm.VoiceChannel, false, 255, 0)
	if voice.Kind != 2 || voice.Path != "v01" {
		t.Errorf("voice play mapped to %+v", voice)
	}
	se := play(vm.EmonPlaybackPlay, "se01", -1, 3, false, 128, 0)
	if se.Kind != 1 || se.Path != "se01" || se.Channel != 3 {
		t.Errorf("se play mapped to %+v", se)
	}
	group := play(vm.EmonPlaybackPlay, "g01", 2, -1, true, 255, 0)
	if group.Kind != 1 || group.Group != 2 {
		t.Errorf("group play mapped to %+v", group)
	}
	stopMusic := play(vm.EmonPlaybackStop, "bgm01", -1, -1, false, 0, 0)
	if stopMusic.Kind != 3 {
		t.Errorf("music stop mapped to %+v", stopMusic)
	}
	if state := s.State(); state.Bgm != "" {
		t.Errorf("bgm not cleared after stop: %q", state.Bgm)
	}
	stopVoice := play(vm.EmonPlaybackStop, "v01", -1, vm.VoiceChannel, false, 0, 0)
	if stopVoice.Kind != 4 {
		t.Errorf("voice stop mapped to %+v", stopVoice)
	}
	stopSe := play(vm.EmonPlaybackStop, "", 0, 1, false, 0, 0)
	if stopSe.Kind != 5 {
		t.Errorf("se stop mapped to %+v", stopSe)
	}
	setVolume := play(vm.EmonPlaybackSetVolume, "bgm01", -1, -1, false, 100, 0)
	if setVolume.Kind != 7 {
		t.Errorf("set volume mapped to %+v", setVolume)
	}
	fade := play(vm.EmonPlaybackFadeOut, "bgm01", -1, -1, false, 200, 30)
	if fade.Kind != 8 || fade.Frames != 30 {
		t.Errorf("fade out mapped to %+v", fade)
	}
	stopAll := play(vm.EmonPlaybackStopAll, "", -1, -1, false, 0, 0)
	if stopAll.Kind != 9 {
		t.Errorf("stop all mapped to %+v", stopAll)
	}
	if rest := s.Audio(); len(rest) != 0 {
		t.Errorf("drain left %d events", len(rest))
	}
}

func TestDefaultName(t *testing.T) {
	if got := defaultName(nil); got != "主人公" {
		t.Errorf("nil lookup got %q", got)
	}
	if got := defaultName(func(string) string { return "" }); got != "主人公" {
		t.Errorf("empty lookup got %q", got)
	}
	tables := map[string]string{"主人公苗字": "山田", "主人公名前": "太郎"}
	got := defaultName(func(name string) string { return tables[name] })
	if got != "太郎" {
		t.Errorf("priority order got %q want 太郎", got)
	}
	self := defaultName(func(name string) string { return name })
	if self != "主人公" {
		t.Errorf("self reference got %q", self)
	}
	initial := defaultName(func(name string) string {
		if name == "主人公初期名前" {
			return "初期名"
		}
		return ""
	})
	if initial != "初期名" {
		t.Errorf("initial name got %q", initial)
	}
}

func TestDetectFindsEmecfg(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "emecfg.ecf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !detect(root) {
		t.Error("detect missed nested emecfg.ecf")
	}
	empty := t.TempDir()
	if detect(empty) {
		t.Error("detect hit empty directory")
	}
	deep := t.TempDir()
	far := filepath.Join(deep, "1", "2", "3", "4", "5")
	if err := os.MkdirAll(far, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(far, "emecfg.ecf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if detect(deep) {
		t.Error("detect exceeded depth 4")
	}
}

func TestSanitizeName(t *testing.T) {
	if got := sanitizeName("bgm/01:test?.ogg"); got == "bgm/01:test?.ogg" || got == "" {
		t.Errorf("sanitize kept invalid characters: %q", got)
	}
	if got := sanitizeName(""); got != "game" {
		t.Errorf("empty sanitize got %q", got)
	}
}

// 回归：宿主传入的是父目录（.../ASENURE），真正的游戏根是它的直接子目录
// （.../ASENURE/ASENURE）。resolveGameRoot 必须在父目录场景下返回子目录根，
// 且 dir 本身含 emecfg 时不要被更深层子目录"抢走"根。
func TestResolveGameRoot(t *testing.T) {
	writeEcf := func(dir string) {
		if err := os.WriteFile(filepath.Join(dir, "emecfg.ecf"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	parent := t.TempDir()
	game := filepath.Join(parent, "ASENURE")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEcf(game)
	if got := resolveGameRoot(parent); got != game {
		t.Errorf("parent dir: got %q want %q", got, game)
	}
	// dir 本身就是游戏根时根即 dir。
	if got := resolveGameRoot(game); got != game {
		t.Errorf("game dir itself: got %q want %q", got, game)
	}
	// dir 本身含 emecfg 时，不应被子目录抢走根。
	nested := filepath.Join(game, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEcf(nested)
	if got := resolveGameRoot(game); got != game {
		t.Errorf("game dir with nested ecf: got %q want %q", got, game)
	}
	// 浅层 BFS：emecfg 在深度 2 子目录。
	outer := t.TempDir()
	deep := filepath.Join(outer, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeEcf(deep)
	if got := resolveGameRoot(outer); got != deep {
		t.Errorf("depth-2: got %q want %q", got, deep)
	}
	// 空目录：原样返回，不静默改根。
	empty := t.TempDir()
	if got := resolveGameRoot(empty); got != empty {
		t.Errorf("empty: got %q want %q", got, empty)
	}
	if got := resolveGameRoot(""); got != "" {
		t.Errorf("empty string: got %q", got)
	}
}

// 回归：父目录场景下，OpenExtras 解析出的 s.dir / archiveDir / 存档目录
// 必须落在子目录游戏根上（配置仍用同一份 emecfg），而不是父目录。
// emecfg 内容为占位字节（非法格式）时 config 为 nil、machine 为 nil，
// 这里只断言根路径选择，不驱动 Run（不 fault 避开黑盒脚本问题）。
func TestOpenExtrasParentResolvesGameRoot(t *testing.T) {
	parent := t.TempDir()
	game := filepath.Join(parent, "ASENURE")
	if err := os.MkdirAll(filepath.Join(game, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "emecfg.ecf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(game, "data", "ASscene.eme"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	if _, err := s.OpenExtras(parent, nil, filepath.Join(t.TempDir(), "cache")); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.dir != game {
		t.Errorf("s.dir: got %q want %q", s.dir, game)
	}
	if s.archiveDir != filepath.Join(game, "data") {
		t.Errorf("archiveDir: got %q want %q", s.archiveDir, filepath.Join(game, "data"))
	}
	wantSave := filepath.Join(game, "SAVEDATA")
	if s.saves == nil || s.saves.DirectoryPath() != wantSave {
		got := ""
		if s.saves != nil {
			got = s.saves.DirectoryPath()
		}
		t.Errorf("saves dir: got %q want %q", got, wantSave)
	}
}
