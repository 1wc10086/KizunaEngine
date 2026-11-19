package tj2

import (
	"strings"
	"testing"
)

func TestSplitTj2MessagePagesByMarker(t *testing.T) {
	pages := splitTj2MessagePages("一行目_r二行目_t三行目")
	if len(pages) != 2 {
		t.Fatalf("pages=%d want 2 (%q)", len(pages), pages)
	}
	if pages[0] != "一行目\n二行目" {
		t.Fatalf("page0=%q", pages[0])
	}
	if pages[1] != "三行目" {
		t.Fatalf("page1=%q", pages[1])
	}
}

func TestSplitTj2MessagePagesCapsLines(t *testing.T) {
	raw := strings.Join([]string{"a", "b", "c", "d", "e"}, "_r")
	pages := splitTj2MessagePages(raw)
	if len(pages) != 2 {
		t.Fatalf("pages=%d want 2", len(pages))
	}
	if strings.Count(pages[1], "\n") != 0 {
		t.Fatalf("page1=%q want single line", pages[1])
	}
}

func TestSplitTj2MessagePagesWrapsLongLine(t *testing.T) {
	raw := strings.Repeat("あ", tj2ClassicLineRunes*2)
	pages := splitTj2MessagePages(raw)
	if len(pages) != 1 {
		t.Fatalf("pages=%d want 1", len(pages))
	}
	if lines := strings.Count(pages[0], "\n") + 1; lines != 2 {
		t.Fatalf("lines=%d want 2", lines)
	}
}

func TestSplitTj2MessagePagesDropsBlankPages(t *testing.T) {
	pages := splitTj2MessagePages("_t_t_r")
	if len(pages) != 0 {
		t.Fatalf("pages=%q want empty", pages)
	}
}
