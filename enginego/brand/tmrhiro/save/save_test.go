package save

import (
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	slot := Slot{
		Title: "chapter", SavedAt: "No.003", Summary: "s",
		Script: "00_01", Index: 12,
		CallStack: []string{"start:47"},
		Vars:      map[string]int32{"$glo100": 1},
	}
	if err := store.Save(3, slot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load(3)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Script != "00_01" || got.Index != 12 {
		t.Fatalf("读回 %+v", got)
	}
	if got.Vars["$glo100"] != 1 {
		t.Fatalf("变量 = %v", got.Vars)
	}
}

func TestSaveRejectsSlotZero(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(0, Slot{}); err == nil {
		t.Fatal("槽位 0 应被拒绝")
	}
}

func TestListReturnsEmptyForMissingSlots(t *testing.T) {
	store := New(t.TempDir())
	list := store.List(3)
	if len(list) != 3 {
		t.Fatalf("长度 %d", len(list))
	}
	for i, item := range list {
		if item.Script != "" || item.Title != "" {
			t.Fatalf("槽 %d 不该有内容: %+v", i+1, item)
		}
	}
}

func TestExistingAndSlotNumber(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(7, Slot{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := store.Existing(); len(got) != 1 || got[0] != 7 {
		t.Fatalf("Existing = %v", got)
	}
	if n, ok := SlotNumber("slot007.json"); !ok || n != 7 {
		t.Fatalf("SlotNumber = %d, %v", n, ok)
	}
	if _, ok := SlotNumber("other.json"); ok {
		t.Fatal("非槽位文件名被解析")
	}
}

func TestFormatClock(t *testing.T) {
	at := time.Date(2026, 11, 16, 18, 41, 0, 0, time.UTC)
	got := FormatClock(2, at)
	if got != "No.002 : 2026/11/16 18:41" {
		t.Fatalf("FormatClock = %q", got)
	}
}
