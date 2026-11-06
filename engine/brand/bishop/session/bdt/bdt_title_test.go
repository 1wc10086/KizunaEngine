package bdt

import "testing"

func TestTitleMenuHitRects(t *testing.T) {
	s := &Session{titleMode: true}
	items := s.TitleMenu()
	if len(items) != 7 {
		t.Fatalf("want 7 title items, got %d", len(items))
	}
	if items[0].Key != "start" || items[6].Key != "quit" {
		t.Fatalf("unexpected order: first=%s last=%s", items[0].Key, items[6].Key)
	}
	if got := s.titleKeyAt(50, 182); got != "start" {
		t.Fatalf("top-left of START: got %q", got)
	}
	if got := s.titleKeyAt(249, 457); got != "quit" {
		t.Fatalf("bottom-right of QUIT: got %q", got)
	}
	if got := s.titleKeyAt(250, 182); got != "" {
		t.Fatalf("right of menu must miss, got %q", got)
	}
	if got := s.titleKeyAt(49, 182); got != "" {
		t.Fatalf("left of menu must miss, got %q", got)
	}
	s.titleMode = false
	if n := len(s.TitleMenu()); n != 0 {
		t.Fatalf("outside title want no items, got %d", n)
	}
}
