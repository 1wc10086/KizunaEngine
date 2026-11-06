package vm

import (
	"testing"

	"kizuna/engine/brand/emon/eme"
	"kizuna/engine/brand/emon/script"
	"kizuna/engine/brand/emon/stage"
	emontext "kizuna/engine/brand/emon/text"
)

type fakeConfig struct {
	start   string
	macros  []script.EmonMacroDefinition
	symbols map[string]string
	width   int
	height  int
}

func (c *fakeConfig) StartScript() string                     { return c.start }
func (c *fakeConfig) MacroList() []script.EmonMacroDefinition { return c.macros }
func (c *fakeConfig) Symbols() map[string]string              { return c.symbols }
func (c *fakeConfig) WindowWidth() int                        { return c.width }
func (c *fakeConfig) WindowHeight() int                       { return c.height }

type fakeSystemState struct{}

func (s *fakeSystemState) Load(variables *script.EmonVariableStore) bool { return false }
func (s *fakeSystemState) Pump(variables *script.EmonVariableStore)      {}
func (s *fakeSystemState) Write(variables map[string]script.EmonValue) bool {
	return true
}

type fakeSaves struct{}

func (s *fakeSaves) SystemState() EmonSystemState  { return &fakeSystemState{} }
func (s *fakeSaves) Write(data *EmonSaveData) bool { return true }
func (s *fakeSaves) Read(slot int) *EmonSaveData   { return nil }
func (s *fakeSaves) Exists(slot int) bool          { return false }
func (s *fakeSaves) Delete(slot int) bool          { return false }

func newTestMachine() *Machine {
	archives := eme.OpenDirectory("")
	audio := NewEmonAudioEngine("", nil)
	context := NewRuntimeContext(
		&fakeConfig{symbols: map[string]string{}, width: 800, height: 600},
		archives,
		script.NewEmonVariableStore(),
		script.NewEmonVariableStore(),
		script.NewEmonVariableStore(),
		script.NewEmonMacroRegistry(),
		stage.NewEmonStage(),
		stage.NewEmonStageComposer(800, 600),
		emontext.NewEmonMessageBox(),
		audio,
		&fakeSaves{},
		nil,
		"",
	)
	return NewMachine(context)
}

func runBody(t *testing.T, m *Machine, name, body string) EmonStep {
	t.Helper()
	program := m.Context().Scripts.ParseBody(name, body)
	m.pushFileFrame(program)
	return m.Run(0)
}

func TestAssignments(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "assign", "#SetNumber(a=1+2*3,b=10%3,c=7&3|8,d=1<<3,e=16>>2,f=(2+3)*4)\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	want := map[string]int32{"a": 7, "b": 1, "c": 11, "d": 8, "e": 4, "f": 20}
	for name, value := range want {
		got := m.Context().Variables.Lookup(name).AsInt()
		if got != value {
			t.Errorf("%s = %d, want %d", name, got, value)
		}
	}
}

func TestCompoundAssignments(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "compound", "#SetNumber(a=10,a+=5,a-=3,a*=2,a/=4,a%=5,a|=8,a&=6,a^=3)\n#End()\n")
	if got := m.Context().Variables.Lookup("a").AsInt(); got != 3 {
		t.Errorf("a = %d, want 3", got)
	}
}

func TestLookupOrder(t *testing.T) {
	m := newTestMachine()
	m.Context().Symbols.Set("s", script.EmonValueFromString("sym"))
	m.Context().Variables.Set("s", script.EmonValueFromString("local"))
	m.Context().SystemVariables.Set("s", script.EmonValueFromString("sys"))
	if got := m.Lookup("s").AsString(); got != "sys" {
		t.Errorf("lookup = %q, want sys", got)
	}
	m.Context().SystemVariables.Remove("s")
	if got := m.Lookup("s").AsString(); got != "local" {
		t.Errorf("lookup = %q, want local", got)
	}
	m.Context().Variables.Remove("s")
	if got := m.Lookup("s").AsString(); got != "sym" {
		t.Errorf("lookup = %q, want sym", got)
	}
	m.Context().Symbols.Set("AB", script.EmonValueFromString("ab"))
	if got := m.LookupArgument("ABcd").AsString(); got != "abcd" {
		t.Errorf("fused = %q, want abcd", got)
	}
	if got := m.Lookup("missing-name").AsString(); got != "missing-name" {
		t.Errorf("miss = %q, want echo", got)
	}
}

func TestJumpToLabel(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "jump", "#SetNumber(a=1)\n#Jump(SKIP)\n#SetNumber(a=99)\n#SceneName(SKIP)\n#SetNumber(b=2)\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if got := m.Context().Variables.Lookup("a").AsInt(); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
	if got := m.Context().Variables.Lookup("b").AsInt(); got != 2 {
		t.Errorf("b = %d, want 2", got)
	}
}

func TestJumpToFile(t *testing.T) {
	m := newTestMachine()
	m.Context().Scripts.ParseBody("other.txt", "#SetNumber(filed=1)\n#End()\n")
	step := runBody(t, m, "main", "#Jump(other.txt)\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if got := m.Context().Variables.Lookup("filed").AsInt(); got != 1 {
		t.Errorf("filed = %d, want 1", got)
	}
}

func TestPrintf(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "printf", "#SetSymbol(msg,\"%04d-%s\",7,\"hi\")\n#End()\n")
	if got := m.Context().Symbols.Lookup("msg").AsString(); got != "0007-hi" {
		t.Errorf("msg = %q, want 0007-hi", got)
	}
}

func TestMacroArguments(t *testing.T) {
	m := newTestMachine()
	m.Context().Macros.Add("M", script.EmonMacroDefinition{Name: "M", Body: "#SetNumber(r=引数１)\n"})
	step := runBody(t, m, "macrocall", "#M(42)\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if got := m.Context().Variables.Lookup("r").AsInt(); got != 42 {
		t.Errorf("r = %d, want 42", got)
	}
}

func TestFaultTolerance(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "fault", "#SetNumber(a=1)\n#BogusCommand(zzz)\n#SetNumber(b=2)\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if got := m.Context().Variables.Lookup("a").AsInt(); got != 1 {
		t.Errorf("a = %d, want 1", got)
	}
	if got := m.Context().Variables.Lookup("b").AsInt(); got != 2 {
		t.Errorf("b = %d, want 2", got)
	}
}

func TestFloatVolume(t *testing.T) {
	m := newTestMachine()
	var events []EmonPlaybackCommand
	audio := m.Context().Audio.(*EmonAudioEngine)
	audio.CommandIssued = func(command EmonPlaybackCommand) {
		events = append(events, command)
	}
	runBody(t, m, "vol", "#Music(bgm03,\"0.5\")\n#End()\n")
	if len(events) != 1 || events[0].Volume != 127 {
		t.Fatalf("events = %+v, want single play at 127", events)
	}
}

func TestAudioSequence(t *testing.T) {
	m := newTestMachine()
	var events []EmonPlaybackCommand
	audio, ok := m.Context().Audio.(*EmonAudioEngine)
	if !ok {
		t.Fatal("audio is not the vm engine")
	}
	audio.CommandIssued = func(command EmonPlaybackCommand) {
		events = append(events, command)
	}
	step := runBody(t, m, "audio", "#Music(bgm01,200)\n#Sound(se01)\n#SetSndVolCh(0,128)\n#FadeOutMusic(30)\n#StopMusic()\n#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	kinds := []EmonPlaybackKind{}
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	want := []EmonPlaybackKind{EmonPlaybackPlay, EmonPlaybackPlay, EmonPlaybackSetVolume, EmonPlaybackFadeOut, EmonPlaybackStop}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if events[0].File != "bgm01" || events[0].Volume != 200 || !events[0].Loop {
		t.Errorf("music event = %+v", events[0])
	}
	if events[1].Channel != 0 {
		t.Errorf("se channel = %d, want 0", events[1].Channel)
	}
	if state, exists := m.AudioState()["bgm"]; exists {
		t.Errorf("bgm state = %q, want cleared", state)
	}
}

func TestNotifyPlaybackCompleted(t *testing.T) {
	audio := NewEmonAudioEngine("", nil)
	audio.PlaySound(EmonSoundRequest{File: "voice01", Flags: EmonSoundVoice | EmonSoundNoLoop, Channel: VoiceChannel})
	audio.NotifyPlaybackCompleted("voice01", true)
	if !audio.HasCompletedSounds() {
		t.Fatal("want a completion")
	}
	completion, ok := audio.DequeueCompletion()
	if !ok || completion.File != "voice01" || !completion.Voice {
		t.Fatalf("completion = %+v %v", completion, ok)
	}
	audio.NotifyPlaybackCompleted("voice01", true)
	if audio.HasCompletedSounds() {
		t.Fatal("duplicate completion was not deduplicated")
	}
}

func TestFadeOutUpdate(t *testing.T) {
	audio := NewEmonAudioEngine("", nil)
	var volumes []int
	audio.CommandIssued = func(command EmonPlaybackCommand) {
		if command.Kind == EmonPlaybackSetVolume {
			volumes = append(volumes, command.Volume)
		}
	}
	audio.PlayMusic("bgm02", 255)
	audio.FadeOutMusic(60)
	audio.Update(0.5)
	if len(volumes) == 0 {
		t.Fatal("want volume updates during fade")
	}
	last := volumes[len(volumes)-1]
	if last >= 255 {
		t.Errorf("last volume = %d, want < 255", last)
	}
}
