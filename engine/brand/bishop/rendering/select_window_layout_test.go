package rendering

import (
	"testing"

	"kizuna/engine/brand/bishop/config"
)

func TestBuildSelectWindowHeaderFooter(t *testing.T) {
	design := config.SelectDesign{
		Item:         "item0",
		Interval:     0,
		TextX:        341,
		TextY:        9,
		Alignment:    config.SelectDesignAlignCenter,
		HeaderFooter: true,
	}
	items := BuildSelectWindow(design, 1024, 768, 682, 40, 35, 35, 230, []string{"A", "B"})
	if len(items) != 2 {
		t.Fatalf("%d items", len(items))
	}
	// 每块 = 35(头) + 40(中) + 35(脚) = 110；两块总高 220，中心 230 → 顶 120。
	if items[0].Y != 120+35 {
		t.Fatalf("first middle y = %d, want 155", items[0].Y)
	}
	if items[1].Y != 120+110+35 {
		t.Fatalf("second middle y = %d, want 265", items[1].Y)
	}
	if items[0].Head != "item0_head" || items[0].Foot != "item0_foot" {
		t.Fatalf("decorations = %q/%q", items[0].Head, items[0].Foot)
	}
	if items[0].Height != 40 {
		t.Fatalf("hit height = %d, want 40", items[0].Height)
	}
}

func TestBuildSelectWindowWithoutDecorations(t *testing.T) {
	design := config.SelectDesign{Item: "item0", Alignment: config.SelectDesignAlignCenter}
	items := BuildSelectWindow(design, 1024, 768, 462, 42, 0, 0, 300, []string{"A", "B", "C"})
	if len(items) != 3 {
		t.Fatalf("%d items", len(items))
	}
	// 无装饰时行高 42、间距 0：总高 126，顶 237。
	if items[0].Y != 300-63 {
		t.Fatalf("first y = %d, want 237", items[0].Y)
	}
	if items[1].Y-items[0].Y != 42 {
		t.Fatalf("row gap = %d, want 42", items[1].Y-items[0].Y)
	}
}
