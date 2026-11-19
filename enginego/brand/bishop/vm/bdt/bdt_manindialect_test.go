package bdt

import (
	"reflect"
	"testing"
)

type fakeManinRecord struct {
	caption         string
	attackTarget    int
	attackTimeLimit int
	attackCalled    bool
	organizerCalled bool
	staffRollCalled bool
}

type fakeManinHost struct {
	record *fakeManinRecord
}

func (f fakeManinHost) SetCaption(text string) {
	if f.record != nil {
		f.record.caption = text
	}
}

func (f fakeManinHost) StartManinAttack(target int, timeLimit int) {
	if f.record != nil {
		f.record.attackCalled = true
		f.record.attackTarget = target
		f.record.attackTimeLimit = timeLimit
	}
}

func (f fakeManinHost) StartManinOrganizer() {
	if f.record != nil {
		f.record.organizerCalled = true
	}
}

func (f fakeManinHost) StartManinStaffRoll() {
	if f.record != nil {
		f.record.staffRollCalled = true
	}
}

func (f fakeManinHost) SetBackground(name string, effectCode int)       {}
func (f fakeManinHost) SetSprite(slot int, name string, x, y, mode int) {}
func (f fakeManinHost) ClearSprite(slot int)                            {}
func (f fakeManinHost) ClearSprites()                                   {}
func (f fakeManinHost) SetWindow(part int, visible bool)                {}
func (f fakeManinHost) ShowMessage(speaker, text, voiceCode string)     {}
func (f fakeManinHost) ShowFace(name string)                            {}
func (f fakeManinHost) PlayVoice(code string)                           {}
func (f fakeManinHost) SetBgm(track, mode int)                          {}
func (f fakeManinHost) StopBgm()                                        {}
func (f fakeManinHost) PlaySe(name string)                              {}
func (f fakeManinHost) SetEffect(effectCode int)                        {}
func (f fakeManinHost) SetSceneId(sceneId string)                       {}
func (f fakeManinHost) SetScreenMode(mode int)                          {}
func (f fakeManinHost) Redraw()                                         {}

var (
	_ BdtHost   = fakeManinHost{}
	_ ManinHost = fakeManinHost{}
)

type fakeSpriteRecord struct {
	slot int
	name string
	x    int
	y    int
	mode int
}

type fakeSpriteHost struct {
	sprites   []fakeSpriteRecord
	clears    []int
	clearsAll int
}

func (f *fakeSpriteHost) SetBackground(name string, effectCode int) {}
func (f *fakeSpriteHost) SetSprite(slot int, name string, x, y, mode int) {
	f.sprites = append(f.sprites, fakeSpriteRecord{slot: slot, name: name, x: x, y: y, mode: mode})
}
func (f *fakeSpriteHost) ClearSprite(slot int)                        { f.clears = append(f.clears, slot) }
func (f *fakeSpriteHost) ClearSprites()                               { f.clearsAll++ }
func (f *fakeSpriteHost) SetWindow(part int, visible bool)            {}
func (f *fakeSpriteHost) ShowMessage(speaker, text, voiceCode string) {}
func (f *fakeSpriteHost) ShowFace(name string)                        {}
func (f *fakeSpriteHost) PlayVoice(code string)                       {}
func (f *fakeSpriteHost) SetBgm(track, mode int)                      {}
func (f *fakeSpriteHost) StopBgm()                                    {}
func (f *fakeSpriteHost) PlaySe(name string)                          {}
func (f *fakeSpriteHost) SetEffect(effectCode int)                    {}
func (f *fakeSpriteHost) SetSceneId(sceneId string)                   {}
func (f *fakeSpriteHost) SetScreenMode(mode int)                      {}
func (f *fakeSpriteHost) Redraw()                                     {}

type maninSpriteHost struct {
	fakeSpriteHost
	record *fakeManinRecord
}

func (m maninSpriteHost) SetCaption(text string) {
	if m.record != nil {
		m.record.caption = text
	}
}

func (m maninSpriteHost) StartManinAttack(target int, timeLimit int) {
	if m.record != nil {
		m.record.attackCalled = true
		m.record.attackTarget = target
		m.record.attackTimeLimit = timeLimit
	}
}

func (m maninSpriteHost) StartManinOrganizer() {
	if m.record != nil {
		m.record.organizerCalled = true
	}
}

func (m maninSpriteHost) StartManinStaffRoll() {
	if m.record != nil {
		m.record.staffRollCalled = true
	}
}

var (
	_ BdtHost   = (*maninSpriteHost)(nil)
	_ ManinHost = (*maninSpriteHost)(nil)
	_ BdtHost   = (*fakeSpriteHost)(nil)
)

func newManinTestScript(lines []string) *BdtScriptFile {
	out := make([]BdtLine, 0, len(lines)+1)
	out = append(out, BdtLine{Index: 0, Offset: 0, Kind: BdtLineKindLabel, Text: "$999-98"})
	for i, line := range lines {
		out = append(out, BdtLine{Index: i + 1, Offset: i + 1, Kind: ClassifyBdtLine(line), Text: line})
	}
	return &BdtScriptFile{Name: "test.bdt", Lines: out, labelIndex: map[string]int{"999-98": 0}}
}

type fakeManinLoader struct{}

func (fakeManinLoader) Load(scriptId string) *BdtScriptFile { return nil }

func TestManinLXCaption(t *testing.T) {
	script := newManinTestScript([]string{"LXfoo"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.SetXValue(30, 123)
	if m.Dialect() != BdtDialectManin {
		t.Fatalf("dialect = %v, want Manin", m.Dialect())
	}
	m.Step()
	if rec.caption != "foo" {
		t.Fatalf("caption = %q, want foo", rec.caption)
	}
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop = %v, want Halt (script ends)", m.Stop())
	}
	if m.x[30] != 123 {
		t.Fatalf("x[30] = %d, want 123 (LX must not touch X30)", m.x[30])
	}
	// LX is not a stop: a fresh machine that only runs LX should not be stopped
	// mid-script. Verify by checking pc advanced past LX.
	script2 := newManinTestScript([]string{"LXfoo", "E"})
	rec2 := &fakeManinRecord{}
	m2 := NewBdtMachine(script2, fakeManinHost{record: rec2}, fakeManinLoader{})
	m2.SetDialect(BdtDialectManin)
	m2.Start("999-98")
	m2.Step()
	if rec2.caption != "foo" {
		t.Fatalf("caption2 = %q, want foo", rec2.caption)
	}
	if m2.Stop() != BdtStopHalt {
		t.Fatalf("stop2 = %v, want Halt after E", m2.Stop())
	}
}

func TestManinXBXTXA(t *testing.T) {
	script := newManinTestScript([]string{"XB1,f01", "XT420", "XA", "E"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.Step()
	if m.ManinTimeLimit() != 420 {
		t.Fatalf("maninTimeLimit = %d, want 420", m.ManinTimeLimit())
	}
	targets := m.ManinTargets()
	if len(targets) != 6 {
		t.Fatalf("len(targets) = %d, want 6", len(targets))
	}
	if targets[0] != "f01" {
		t.Fatalf("targets[0] = %q, want f01", targets[0])
	}
	if !rec.attackCalled {
		t.Fatalf("attack not called")
	}
	// X29 defaults to 0, clamped to 1.
	if rec.attackTarget != 1 {
		t.Fatalf("attack target = %d, want 1 (clamped)", rec.attackTarget)
	}
	if rec.attackTimeLimit != 420 {
		t.Fatalf("attack timeLimit = %d, want 420", rec.attackTimeLimit)
	}
	if m.Stop() != BdtStopAttack {
		t.Fatalf("stop = %v, want Attack", m.Stop())
	}
	m.CloseManinStop()
	if m.Stop() != BdtStopNone {
		t.Fatalf("stop after CloseManinStop = %v, want None", m.Stop())
	}
	m.Step()
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop after E = %v, want Halt", m.Stop())
	}
}

func TestManinXROrganizer(t *testing.T) {
	script := newManinTestScript([]string{"XR", "E"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.Step()
	if !rec.organizerCalled {
		t.Fatalf("organizer not called")
	}
	if m.Stop() != BdtStopOrganize {
		t.Fatalf("stop = %v, want Organize", m.Stop())
	}
	m.CloseManinStop()
	m.Step()
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop after E = %v, want Halt", m.Stop())
	}
}

func TestManinSROLStaffRoll(t *testing.T) {
	script := newManinTestScript([]string{"SROL1", "E"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.Step()
	if !rec.staffRollCalled {
		t.Fatalf("staff roll not called")
	}
	if m.Stop() != BdtStopStaffRoll {
		t.Fatalf("stop = %v, want StaffRoll", m.Stop())
	}
	m.CloseManinStop()
	m.Step()
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop after E = %v, want Halt", m.Stop())
	}
}

func TestManinXM_XE_NoOp(t *testing.T) {
	script := newManinTestScript([]string{"XM", "XE", "LXbar", "E"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.Step()
	if rec.caption != "bar" {
		t.Fatalf("caption = %q, want bar", rec.caption)
	}
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop = %v, want Halt", m.Stop())
	}
}

func TestManinCA_CK(t *testing.T) {
	script := newManinTestScript([]string{"CA3,gc101", "CK1", "E"})
	rec := &fakeManinRecord{}
	host := maninSpriteHost{record: rec}
	m := NewBdtMachine(script, &host, fakeManinLoader{})
	m.SetDialect(BdtDialectManin)
	m.Start("999-98")
	m.Step()
	if len(host.sprites) != 1 {
		t.Fatalf("sprites = %+v, want 1 call", host.sprites)
	}
	got := host.sprites[0]
	if got.slot != 0 || got.name != "gc101" || got.x != 160 || got.y != 0 || got.mode != 0 {
		t.Fatalf("sprite = %+v, want {0 gc101 160 0 0}", got)
	}
	if len(host.clears) != 1 || host.clears[0] != 0 {
		t.Fatalf("clears = %v, want [0]", host.clears)
	}
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop = %v, want Halt", m.Stop())
	}
	// Also assert the machine cleared sprite tracking.
	if m.spriteUsed[0] {
		t.Fatalf("spriteUsed[0] still true after CK")
	}
}

func TestContainerDoesNotInvokeManinHostAndPreservesOldBehavior(t *testing.T) {
	script := newManinTestScript([]string{"LXfoo", "XB1,f01", "XT420", "XA", "SROL1", "CA3,gc101", "CK1", "E"})
	rec := &fakeManinRecord{}
	host := maninSpriteHost{record: rec}
	m := NewBdtMachine(script, &host, fakeManinLoader{})
	// Default dialect is Container; explicitly set it too.
	if m.Dialect() != BdtDialectContainer {
		t.Fatalf("default dialect = %v, want Container", m.Dialect())
	}
	m.SetDialect(BdtDialectContainer)
	m.Start("999-98")
	m.SetXValue(30, 777)
	m.Step()

	if rec.caption != "" || rec.attackCalled || rec.organizerCalled || rec.staffRollCalled {
		t.Fatalf("ManinHost invoked under Container: %+v", rec)
	}
	if m.ManinTimeLimit() != 0 {
		t.Fatalf("maninTimeLimit = %d, want 0 under Container", m.ManinTimeLimit())
	}
	targets := m.ManinTargets()
	if !reflect.DeepEqual(targets, make([]string, 6)) {
		t.Fatalf("targets = %v, want empty", targets)
	}
	if m.x[30] != 777 {
		t.Fatalf("x[30] = %d, want 777 (X30 must not be clobbered)", m.x[30])
	}
	// Container CA: x-coordinate sprite add (3, gc101). CK1: slot-based removal.
	if len(host.sprites) != 1 {
		t.Fatalf("sprites = %+v, want 1 container sprite", host.sprites)
	}
	got := host.sprites[0]
	if got.slot != 0 || got.name != "gc101" || got.x != 3 {
		t.Fatalf("container sprite = %+v, want {0 gc101 3 ...}", got)
	}
	// CK1 removed slot 1 -> in Container terms index 0; after CA slot0 used, CK1 should clear it
	// via the shift logic (no ClearSprite(0) from manin path; Container CK goes through executeSpriteRemove).
	// We just assert the script ran to completion without manin stops.
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop = %v, want Halt", m.Stop())
	}
}

func TestContainerSROL_Ignored(t *testing.T) {
	script := newManinTestScript([]string{"SROL1", "LXx", "E"})
	rec := &fakeManinRecord{}
	m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
	m.SetDialect(BdtDialectContainer)
	m.Start("999-98")
	m.Step()
	if rec.staffRollCalled {
		t.Fatalf("staff roll called under Container")
	}
	if rec.caption != "" {
		t.Fatalf("caption = %q, want empty under Container", rec.caption)
	}
	if m.Stop() != BdtStopHalt {
		t.Fatalf("stop = %v, want Halt", m.Stop())
	}
}

func TestManinX29Clamp(t *testing.T) {
	for _, tc := range []struct {
		x29  int32
		want int
	}{
		{0, 1},
		{2, 2},
		{6, 6},
		{99, 6},
	} {
		script := newManinTestScript([]string{"XA"})
		rec := &fakeManinRecord{}
		m := NewBdtMachine(script, fakeManinHost{record: rec}, fakeManinLoader{})
		m.SetDialect(BdtDialectManin)
		m.Start("999-98")
		m.SetXValue(29, tc.x29)
		m.Step()
		if rec.attackTarget != tc.want {
			t.Fatalf("x29=%d target=%d want %d", tc.x29, rec.attackTarget, tc.want)
		}
	}
}

func TestManinSetXValueSetZValue(t *testing.T) {
	script := newManinTestScript([]string{"E"})
	m := NewBdtMachine(script, fakeManinHost{}, fakeManinLoader{})
	m.SetXValue(30, 123)
	m.SetZValue(5, 42)
	if m.x[30] != 123 {
		t.Fatalf("x[30] = %d, want 123", m.x[30])
	}
	if m.z[5] != 42 {
		t.Fatalf("z[5] = %d, want 42", m.z[5])
	}
	// X29 set via SetXValue after Start drives XA target.
	script2 := newManinTestScript([]string{"XA"})
	rec := &fakeManinRecord{}
	m2 := NewBdtMachine(script2, fakeManinHost{record: rec}, fakeManinLoader{})
	m2.SetDialect(BdtDialectManin)
	m2.Start("999-98")
	m2.SetXValue(29, 4)
	m2.Step()
	if rec.attackTarget != 4 {
		t.Fatalf("attack target = %d, want 4", rec.attackTarget)
	}
}
