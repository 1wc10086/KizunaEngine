package bs

import (
	"testing"

	"kizuna/engine/brand/bishop/state"
)

type stubMapCatalog struct {
	empty bool
}

func (s stubMapCatalog) IsEmpty() bool {
	return s.empty
}

func TestBsSpeakerStripsEscape(t *testing.T) {
	got := Speaker("　　・テスト\\X")
	want := "テスト"
	if got != want {
		t.Fatalf("Speaker = %q, want %q", got, want)
	}
}

func TestBsMessageCollapsesSpaces(t *testing.T) {
	got := Message("あ　い う。 え")
	want := "あ\u3000い\u3000う。\u3000え"
	if got != want {
		t.Fatalf("Message = %q, want %q", got, want)
	}
}

func TestBsResolveVolume(t *testing.T) {
	if _, ok := resolveBsVolume(-1); ok {
		t.Fatalf("negative volume must be absent")
	}
	got, ok := resolveBsVolume(5)
	if !ok || got != 0.5 {
		t.Fatalf("volume 5 = %v,%v", got, ok)
	}
	got, _ = resolveBsVolume(99)
	if got != 1 {
		t.Fatalf("volume clamp = %v", got)
	}
}

func TestBsTextSpeed(t *testing.T) {
	if bsTextSpeed("xt") != 420 || bsTextSpeed("xtabc") != 420 || bsTextSpeed("xt999") != 420 {
		t.Fatalf("bad text speed default")
	}
	if bsTextSpeed("xt300") != 300 {
		t.Fatalf("text speed parse")
	}
}

func TestBsScreenDetect(t *testing.T) {
	if !isBsScreen("staffroll") || !isBsScreen("eyecatch01") || isBsScreen("map") {
		t.Fatalf("screen detect")
	}
	if !isBsEngineUpdate("love3") || isBsEngineUpdate("del") {
		t.Fatalf("engine update detect")
	}
}

func TestBsTryOpenMapWithoutCatalog(t *testing.T) {
	InstalledMapCatalog = nil
	d := NewBsSystemDirector(nil, nil, state.NewGameState())
	if d.TryOpenMap() != state.SessionContinue {
		t.Fatalf("map without catalog must continue")
	}
	InstalledMapCatalog = stubMapCatalog{empty: true}
	if d.TryOpenMap() != state.SessionContinue {
		t.Fatalf("map with empty catalog must continue")
	}
	InstalledMapCatalog = stubMapCatalog{empty: false}
	if d.TryOpenMap() != state.SessionAwaitMap {
		t.Fatalf("map with catalog must await map")
	}
	InstalledMapCatalog = nil
}
