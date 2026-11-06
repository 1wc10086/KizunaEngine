package bsmap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"kizuna/engine/brand/bishop/resource"
)

type sanshaProperties map[string]int

func (p sanshaProperties) ByName(name string) int {
	if value, ok := p[name]; ok {
		return value
	}
	return -1
}

func (p sanshaProperties) ByKey(int) int { return -1 }

func TestBsMapSanshaWritesTableIndex(t *testing.T) {
	root := t.TempDir()
	// 只需要让资源索引认识这些名字；地图 profile 的头部测量在失败时退回 64px。
	for _, name := range []string{"map_base", "map_on", "map_f100", "map_ch101", "map_ch102", "map_hit"} {
		if name == "map_f100" {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, name+".png"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "map.dat"), sanshaMapDat(), 0o644); err != nil {
		t.Fatal(err)
	}
	context := BsMapContext{
		Resources:  resource.Build(root),
		GameRoot:   root,
		Properties: sanshaProperties{"#t": 4, "#lv": 0, "#map": 0},
	}
	layout := (&BsMapProfileSansha{}).Build(context)
	if layout == nil {
		t.Fatal("layout is nil")
	}
	if layout.Background != "map_base" || layout.Overlay != "map_on" {
		t.Fatalf("background/overlay = %q/%q", layout.Background, layout.Overlay)
	}
	if !layout.TimeSlotValid || layout.TimeSlot != 0 || layout.TimeValue != 4 {
		t.Fatalf("time slot = %v %d/%d", layout.TimeSlotValid, layout.TimeSlot, layout.TimeValue)
	}
	if len(layout.Items) != 1 {
		t.Fatalf("items = %+v", layout.Items)
	}
	item := layout.Items[0]
	// 表序号 1 写回 #result；不能写地点号，也不能是打包值。
	if item.Value != 1 || item.Property != "#result" {
		t.Fatalf("item value/property = %d/%q", item.Value, item.Property)
	}
	if item.Image != "map_ch102" || item.DrawX != 15 || item.DrawY != 26 {
		t.Fatalf("item = %+v", item)
	}
	if !layout.AllowEmpty {
		t.Fatal("empty map must stay open so the next-time button works")
	}
}

func TestBsMapSanshaMatchesRejectsUK(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"map_base", "map_on", "map_ch101", "map_hit", "map_f100"} {
		if err := os.WriteFile(filepath.Join(root, name+".png"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "map.dat"), sanshaMapDat(), 0o644); err != nil {
		t.Fatal(err)
	}
	context := BsMapContext{Resources: resource.Build(root), GameRoot: root}
	if (&BsMapProfileSansha{}).Matches(context) {
		t.Fatal("UK assets (map_f100) must not match the Sansha profile")
	}
}

func sanshaMapDat() []byte {
	out := []byte("BSX MAP INFO r3\x00")
	out = append(out, '#', 't', 0)
	out = append(out, 1)
	out = appendInt32(out, 4)
	out = append(out, 2)
	out = append(out, []byte("null\x00map_ch102\x00")...)
	out = append(out, 1) // places
	out = append(out, 1) // image index（头像卡自身，不参与热区）
	out = append(out, 10, 0, 20, 0)
	out = append(out, 1) // chunks
	out = appendInt32(out, 5)
	out = appendInt32(out, 6)
	out = append(out, 1) // tables
	out = append(out, []byte("#lv\x00#map\x00")...)
	out = append(out, 1) // ranges
	out = appendInt32(out, 0)
	out = append(out, 1) // values
	out = append(out, 0x01, 0x02)
	out = append(out, 1) // results
	out = append(out, []byte("#result\x00")...)
	return out
}

func appendInt32(target []byte, value int32) []byte {
	var buffer [4]byte
	binary.LittleEndian.PutUint32(buffer[:], uint32(value))
	return append(target, buffer[:]...)
}
