package gothic

import "testing"

func TestToFullWidth(t *testing.T) {
	if got := toFullWidth("A1 /"); got != "Ａ１　／" {
		t.Fatalf("toFullWidth = %q", got)
	}
}

func TestGothicMapColorValue(t *testing.T) {
	cases := map[int]int32{
		0x00FFFF: 1,
		0x0000FF: 2,
		0x00FF00: 3,
		0xFF0000: 4,
		0x123456: 0,
	}
	for color, want := range cases {
		if got := gothicMapColorValue(color); got != want {
			t.Fatalf("color %06X = %d want %d", color, got, want)
		}
	}
}

func TestGothicVisibleLines(t *testing.T) {
	lines := []string{"あいう", "えお"}
	if got := gothicVisibleLines(lines, 2); len(got) != 1 || got[0] != "あい" {
		t.Fatalf("limit 2 = %#v", got)
	}
	if got := gothicVisibleLines(lines, 4); len(got) != 2 || got[1] != "え" {
		t.Fatalf("limit 4 = %#v", got)
	}
	if got := gothicVisibleLines(lines, -1); len(got) != 2 {
		t.Fatalf("limit -1 = %#v", got)
	}
}

func TestAlbumCellGrid(t *testing.T) {
	first := albumCellRect(0)
	if first != [4]int{20, 76, 126, 99} {
		t.Fatalf("cell 0 = %v", first)
	}
	last := albumCellRect(11)
	if last[0] != 479 || last[1] != 322 {
		t.Fatalf("cell 11 = %v", last)
	}
}

func TestHSceneKey(t *testing.T) {
	if got := hSceneKey(1); got != "h01" {
		t.Fatalf("h1 = %q", got)
	}
	if got := hSceneKey(21); got != "h21" {
		t.Fatalf("h21 = %q", got)
	}
}
