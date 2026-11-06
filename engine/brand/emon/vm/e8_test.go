package vm

import (
	"testing"

	"kizuna/engine/brand/emon/save"
	"kizuna/engine/brand/emon/script"
)

func TestSpriteCommands(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "sprites",
		"#SetSprite(\"hero\",\"\",\"\",100,200,DISPON)\n"+
			"#SetSprPos(\"hero\",150,160)\n"+
			"#GetSprPos(\"hero\",posX,posY)\n"+
			"#SetSprRect(\"hero\",0,0,50,60)\n"+
			"#SetSprText(\"hero\",10,20,\"g\",24)\n"+
			"#SprWriteText(\"hero\",\"hi\")\n"+
			"#SetSprTrance(\"hero\",128)\n"+
			"#SetSprColor(\"hero\",COLOR,16711680,128)\n"+
			"#SetSpriteMsgSelect(\"hero\",LINE)\n"+
			"#SpriteSwitch(\"hero\",DISPOFF)\n"+
			"#SpriteSwitch(\"hero\",DISPON)\n"+
			"#ClearSprite(\"hero\")\n"+
			"#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	sprite := m.Context().Stage.Find("hero")
	if sprite == nil {
		t.Fatal("hero was not created")
	}
	off := newTestMachine()
	runBody(t, off, "off", "#SetSprite(\"hero\",\"\",\"\",0,0,DISPON)\n#SpriteSwitch(\"hero\",DISPOFF)\n#End()\n")
	offHero := off.Context().Stage.Find("hero")
	if offHero == nil {
		t.Fatal("off hero was not created")
	}
	if sprite.X() != 150 || sprite.Y() != 160 {
		t.Errorf("pos = (%d,%d), want (150,160)", sprite.X(), sprite.Y())
	}
	if got := m.Context().Variables.Lookup("posX").AsInt(); got != 150 {
		t.Errorf("posX = %d, want 150", got)
	}
	if got := m.Context().Variables.Lookup("posY").AsInt(); got != 160 {
		t.Errorf("posY = %d, want 160", got)
	}
	if sprite.Width() != 50 || sprite.Height() != 60 {
		t.Errorf("size = (%d,%d), want (50,60)", sprite.Width(), sprite.Height())
	}
	if sprite.TextSetting() == nil {
		t.Fatal("hero has no text setting")
	}
	if got := m.SpriteTexts()["hero"]; got != "hi" {
		t.Errorf("sprite text = %q, want hi", got)
	}
	if sprite.Tint() != 16711680 {
		t.Errorf("tint = %d, want 16711680", sprite.Tint())
	}
	if offHero.Visible() {
		t.Error("hero is visible, want hidden after DISPOFF")
	}
	if !sprite.Visible() {
		t.Error("hero is hidden, want visible after DISPON")
	}
	if sprite.Alpha() != 255 {
		t.Errorf("alpha = %d, want 255 after clear", sprite.Alpha())
	}
}

func TestSpriteColorAlpha(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "color",
		"#SetSprite(\"hero\",\"\",\"\",0,0,DISPON)\n"+
			"#SetSprTrance(\"hero\",128)\n"+
			"#SetSprColor(\"hero\",COLOR,16711680,64)\n"+
			"#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	sprite := m.Context().Stage.Find("hero")
	if sprite == nil {
		t.Fatal("hero was not created")
	}
	if sprite.Alpha() != 128 {
		t.Errorf("alpha = %d, want 128", sprite.Alpha())
	}
	if sprite.Tint() != 16711680 {
		t.Errorf("tint = %d, want 16711680", sprite.Tint())
	}
	if sprite.ColorAmount() != 64 {
		t.Errorf("color amount = %d, want 64", sprite.ColorAmount())
	}
}

func TestSelectOrder(t *testing.T) {
	m := newTestMachine()
	runBody(t, m, "select",
		"#SetSprite(\"a\",\"\",\"\",0,0,DISPON)\n"+
			"#SetSprite(\"b\",\"\",\"\",0,0,DISPON)\n"+
			"#SpriteSwitch(\"a\",SELECTON)\n"+
			"#SpriteSwitch(\"b\",SELECTON)\n"+
			"#End()\n")
	ordered := m.OrderedSelectSprites()
	if len(ordered) != 2 || ordered[0].Name() != "a" || ordered[1].Name() != "b" {
		names := []string{}
		for _, sprite := range ordered {
			names = append(names, sprite.Name())
		}
		t.Fatalf("ordered = %v, want [a b]", names)
	}
	if got := m.ChooseSelect(2); got != m.Step() {
		t.Fatalf("choose step = %d", got)
	}
}

func TestBackLog(t *testing.T) {
	m := newTestMachine()
	for _, line := range []string{"one", "two", "three", "four", "five"} {
		m.AddHistory(line)
	}
	step := runBody(t, m, "backlog",
		"#SetSprite(\"inner\",\"\",\"\",0,0)\n"+
			"#SetSprText(\"inner\",0,0,\"g\",24,400,300)\n"+
			"#BackLog(\"\",inner,\"\",0,0,100,20,0,0,0,0,100,20,0,0,0,0,100,20,0,0,0,0,100,20,0,0)\n"+
			"#End()\n")
	if step != EmonStepWait {
		t.Fatalf("step = %d, want wait", step)
	}
	if !m.BackLogActive() {
		t.Fatal("backlog is not active")
	}
	revision := m.BackLogRevision()
	m.backLogPointer(10, 50, false)
	if m.BackLogRevision() == revision {
		t.Error("page-up scroll did not refresh the backlog")
	}
	if m.backLog.Top != 0 {
		t.Errorf("top = %d, want 0 after page up", m.backLog.Top)
	}
	m.backLogPointer(10, 400, false)
	if m.backLog.Top != 1 {
		t.Errorf("top = %d, want 1 after line down", m.backLog.Top)
	}
	m.backLogPointer(10, 550, false)
	if m.backLog.Top != m.backLog.Maximum {
		t.Fatalf("top = %d, want maximum %d", m.backLog.Top, m.backLog.Maximum)
	}
	if !m.BackLogActive() {
		t.Fatal("backlog closed too early at maximum")
	}
	m.backLogPointer(10, 550, false)
	if m.BackLogActive() {
		t.Fatal("backlog is still active after scrolling past the end")
	}
}

func TestBackLogMessage(t *testing.T) {
	m := newTestMachine()
	for _, line := range []string{"alpha", "beta", "gamma"} {
		m.AddHistory(line)
	}
	m.mutableAudioState().Set("voice", "v01")
	m.AddHistory("delta")
	runBody(t, m, "backlogmsg",
		"#BackLogMessage(GETTEXT,1,gotText)\n"+
			"#BackLogMessage(GETSOUND,3,gotVoice)\n"+
			"#BackLogMessage(GETTEXT,99,gotEmpty)\n"+
			"#SetNumber(cur=2)\n"+
			"#BackLogMessage(NULL,cur)\n"+
			"#SetSprite(\"box\",\"\",\"\",0,0)\n"+
			"#SetSprText(\"box\",0,0,\"g\",24)\n"+
			"#SprWriteText(\"box\",\"boxed\")\n"+
			"#BackLogMessage(box,curBox)\n"+
			"#End()\n")
	if got := m.Context().Variables.Lookup("gotText").AsString(); got != "beta" {
		t.Errorf("gotText = %q, want beta", got)
	}
	if got := m.Context().Variables.Lookup("gotVoice").AsString(); got != "v01" {
		t.Errorf("gotVoice = %q, want v01", got)
	}
	if got := m.Context().Variables.Lookup("gotEmpty").AsInt(); got != 0 {
		t.Errorf("gotEmpty = %d, want 0", got)
	}
	if got := m.Context().Variables.Lookup("cur").AsInt(); got != 2 {
		t.Errorf("cur = %d, want 2", got)
	}
	if got := m.Context().Variables.Lookup("curBox").AsInt(); got != int32(len(m.BackLog())-1) {
		t.Errorf("curBox = %d, want %d", got, len(m.BackLog())-1)
	}
	if last := m.BackLog()[len(m.BackLog())-1]; last != "boxed" {
		t.Errorf("last history = %q, want boxed", last)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "game",
		"#Text(\"hi\")\n"+
			"#SetNumber(score=42)\n"+
			"#SetSprite(\"hero\",\"\",\"\",10,20)\n"+
			"#Wait(500)\n"+
			"#SetNumber(after=1)\n"+
			"#End()\n")
	if step != EmonStepWait {
		t.Fatalf("step = %d, want wait", step)
	}
	data := m.CaptureSessionState(3, "cmt")
	if data == nil {
		t.Fatal("capture returned nil")
	}
	if data.ScriptFile != "game" {
		t.Errorf("script = %q, want game", data.ScriptFile)
	}
	if data.Comment != "cmt" {
		t.Errorf("comment = %q, want cmt", data.Comment)
	}
	if data.Preview != "hi" {
		t.Errorf("preview = %q, want hi", data.Preview)
	}
	if got := data.Variables["score"].AsInt(); got != 42 {
		t.Errorf("score = %d, want 42", got)
	}
	if len(data.Frames) == 0 {
		t.Fatal("frame chain is empty")
	}
	found := false
	for _, state := range data.SpriteStates {
		if state.Name == "hero" && state.X == 10 && state.Y == 20 {
			found = true
		}
	}
	if !found {
		t.Error("hero sprite state was not captured")
	}
	m.SetVariable("score", script.EmonValueFromInt(0))
	m.Context().Stage.Find("hero").SetX(999)
	store := save.NewEmonSaveStore(t.TempDir())
	if !store.Write(data) {
		t.Fatal("store write failed")
	}
	read := store.Read(3)
	if read == nil {
		t.Fatal("store read returned nil")
	}
	if read.Comment != "cmt" || read.ScriptFile != "game" || read.Preview != "hi" {
		t.Errorf("reread = %+v, want comment cmt game hi", read)
	}
	if got := read.Variables["score"].AsInt(); got != 42 {
		t.Errorf("reread score = %d, want 42", got)
	}
	if !m.RestoreState(read) {
		t.Fatal("restore failed")
	}
	if m.Replaying() {
		t.Error("chained restore should not enter replay")
	}
	if got := m.Context().Variables.Lookup("score").AsInt(); got != 42 {
		t.Errorf("score = %d, want 42 after restore", got)
	}
	if x := m.Context().Stage.Find("hero").X(); x != 10 {
		t.Errorf("hero x = %d, want 10 after restore", x)
	}
	step = m.Run(0)
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if got := m.Context().Variables.Lookup("after").AsInt(); got != 1 {
		t.Errorf("after = %d, want 1", got)
	}
}

func TestLegacyRestoreReplays(t *testing.T) {
	m := newTestMachine()
	m.Context().Scripts.ParseBody("legacy.txt", "#SetNumber(a=1)\n#SetNumber(b=2)\n#End()\n")
	data := &EmonSaveData{
		Slot:           1,
		ScriptFile:     "legacy.txt",
		StatementIndex: 1,
		Variables:      map[string]script.EmonValue{"a": script.EmonValueFromInt(7)},
		AudioState:     map[string]string{},
	}
	if !m.RestoreState(data) {
		t.Fatal("legacy restore failed")
	}
	if !m.Replaying() {
		t.Fatal("legacy restore should enter replay")
	}
	if step := m.Run(0); step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	if m.Replaying() {
		t.Error("replay did not finish")
	}
	if got := m.Context().Variables.Lookup("a").AsInt(); got != 7 {
		t.Errorf("a = %d, want saved 7", got)
	}
}

func TestSoundEndTrigger(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "triggers",
		"#SetRunAction(ACT,FLAGON,FLAGX)\n"+
			"#SetAction(TRIG,SOUNDEND@VOICE,ACT)\n"+
			"#SetAction(ONCE,SOUNDEND@SECH@0@ONETIME,ACT)\n"+
			"#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	m.NotifySoundEnded("v01", true)
	m.pollSoundEndTriggers()
	if got := m.Context().Variables.Lookup("FLAGX").AsInt(); got != 1 {
		t.Errorf("FLAGX = %d, want 1 after voice end", got)
	}
	m.SetVariable("FLAGX", script.EmonValueFromInt(0))
	m.NotifySoundEndedChannel("se01", false, 0)
	m.pollSoundEndTriggers()
	if got := m.Context().Variables.Lookup("FLAGX").AsInt(); got != 1 {
		t.Errorf("FLAGX = %d, want 1 after channel end", got)
	}
	for _, trigger := range m.triggers {
		if trigger.Name == "ONCE" {
			t.Fatal("one-time trigger was not removed")
		}
	}
}

func TestRunActionSpriteOps(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "runops",
		"#SetSprite(\"s\",\"\",\"\",5,5)\n"+
			"#SetRunAction(HIDE,SPROFF,s)\n"+
			"#SetRunAction(MOVE,SETSPRITEPOS,s,30,40)\n"+
			"#RunAction(HIDE,MOVE)\n"+
			"#End()\n")
	if step != EmonStepFinished {
		t.Fatalf("step = %d, want finished", step)
	}
	sprite := m.Context().Stage.Find("s")
	if sprite == nil {
		t.Fatal("s was not created")
	}
	if sprite.Visible() {
		t.Error("s is visible, want hidden")
	}
	if sprite.X() != 30 || sprite.Y() != 40 {
		t.Errorf("pos = (%d,%d), want (30,40)", sprite.X(), sprite.Y())
	}
}

func TestMessageBoxAndPrompts(t *testing.T) {
	m := newTestMachine()
	step := runBody(t, m, "prompt",
		"#MessageBox(\"line1\",\"line2\")\n"+
			"#End()\n")
	if step != EmonStepAwaitInput {
		t.Fatalf("step = %d, want await input", step)
	}
	if message, ok := m.PendingMessage(); !ok || message != "line1\nline2" {
		t.Errorf("pending = %q %v, want line1\\nline2", message, ok)
	}
	m.DismissMessage()
	step = runBody(t, m, "ask", "#GetStringMessage(\"name?\",who)\n#End()\n")
	if step != EmonStepAwaitName {
		t.Fatalf("step = %d, want await name", step)
	}
	m.SubmitText("taro")
	if got := m.Context().Variables.Lookup("who").AsString(); got != "taro" {
		t.Errorf("who = %q, want taro", got)
	}
	step = runBody(t, m, "askyn", "#GetYesNoMessage(\"sure?\",ok)\n#End()\n")
	if step != EmonStepAwaitName {
		t.Fatalf("step = %d, want await name", step)
	}
	m.SubmitYesNo(true)
	if got := m.Context().Variables.Lookup("ok").AsInt(); got != 1 {
		t.Errorf("ok = %d, want 1", got)
	}
}
