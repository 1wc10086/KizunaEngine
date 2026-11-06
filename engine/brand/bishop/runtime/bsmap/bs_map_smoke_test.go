package bsmap

import (
	"encoding/binary"
	"testing"

	"kizuna/engine/brand/bishop/gfx"
)

func TestBsMapMaskDataItemAt(t *testing.T) {
	data := &BsMapMaskData{
		Width:        3,
		Height:       2,
		Values:       []byte{0, 1, 2, 5, 0, 7},
		ItemForValue: []int{0, 1, -1, -1, -1, 2, -1, -1},
	}
	if got := data.ValueAt(1, 0); got != 1 {
		t.Fatalf("ValueAt(1,0)=%d", got)
	}
	if got := data.ItemAt(1, 0); got != 1 {
		t.Fatalf("ItemAt(1,0)=%d", got)
	}
	if got := data.ItemAt(2, 0); got != -1 {
		t.Fatalf("ItemAt unmapped=%d", got)
	}
	if got := data.ItemAt(0, 1); got != 2 {
		t.Fatalf("ItemAt(0,1)=%d", got)
	}
	if got := data.ValueAt(3, 0); got != -1 {
		t.Fatalf("ValueAt oob=%d", got)
	}
	if got := data.ItemAt(-1, 0); got != -1 {
		t.Fatalf("ItemAt negative=%d", got)
	}
	if got := data.ItemAt(0, 2); got != -1 {
		t.Fatalf("ItemAt oob row=%d", got)
	}
}

func TestBsMapMaskRegions(t *testing.T) {
	pixels := []byte{
		9, 0, 0, 255, 9, 0, 0, 255, 0, 0, 0, 255, 5, 5, 5, 0,
		3, 0, 0, 255, 9, 0, 0, 255, 3, 0, 0, 255, 0, 0, 0, 1,
	}
	image := gfx.NewBsgImage(4, 2, 0, 0, pixels)
	regions := BsMapMaskRegions(image, BsMapMaskBlueChannel)
	if len(regions) != 2 {
		t.Fatalf("regions=%d", len(regions))
	}
	if regions[0].Value != 3 || regions[0].X != 0 || regions[0].Y != 1 || regions[0].Width != 3 || regions[0].Height != 1 {
		t.Fatalf("region0=%+v", regions[0])
	}
	if regions[1].Value != 9 || regions[1].X != 0 || regions[1].Y != 0 || regions[1].Width != 2 || regions[1].Height != 2 {
		t.Fatalf("region1=%+v", regions[1])
	}
	if regions[1].CenterX() != 1 || regions[1].CenterY() != 1 {
		t.Fatalf("center=%d,%d", regions[1].CenterX(), regions[1].CenterY())
	}
}

func bsMapTestMapDat() []byte {
	out := []byte("BSX MAP INFO r3\x00")
	out = append(out, 't', 0)
	out = append(out, 1)
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, 7)
	out = append(out, value...)
	out = append(out, 1)
	out = append(out, 'i', 'm', 'g', 0)
	out = append(out, 1)
	out = append(out, 2)
	out = append(out, 0x10, 0)
	out = append(out, 0x20, 0)
	out = append(out, 1)
	binary.LittleEndian.PutUint32(value, 100)
	out = append(out, value...)
	binary.LittleEndian.PutUint32(value, 200)
	out = append(out, value...)
	out = append(out, 0)
	out = append(out, 1)
	out = append(out, 'r', 'e', 's', 0)
	return out
}

func TestTryParseBsMapData(t *testing.T) {
	var data BsMapData
	if !TryParseBsMapData(bsMapTestMapDat(), &data) {
		t.Fatal("valid map.dat rejected")
	}
	if data.TimeProperty != "t" || len(data.TimeValues) != 1 || data.TimeValues[0] != 7 {
		t.Fatalf("time=%+v", data)
	}
	if len(data.Images) != 1 || data.Images[0] != "img" {
		t.Fatalf("images=%v", data.Images)
	}
	if len(data.Places) != 1 || data.Places[0].ImageIndex != 2 || data.Places[0].BaseX != 16 || data.Places[0].BaseY != 32 {
		t.Fatalf("places=%+v", data.Places)
	}
	if len(data.Places[0].Chunks) != 1 || data.Places[0].Chunks[0].X != 100 || data.Places[0].Chunks[0].Y != 200 {
		t.Fatalf("chunks=%+v", data.Places[0].Chunks)
	}
	if data.ResultProperty != "res" {
		t.Fatalf("result=%q", data.ResultProperty)
	}
	if data.TimeSlot(7) != 0 || data.TimeSlot(8) != -1 {
		t.Fatal("TimeSlot wrong")
	}
	bad := bsMapTestMapDat()
	bad[0] = 'X'
	var data2 BsMapData
	if TryParseBsMapData(bad, &data2) {
		t.Fatal("bad signature accepted")
	}
	trailing := append(bsMapTestMapDat(), 0)
	var data3 BsMapData
	if TryParseBsMapData(trailing, &data3) {
		t.Fatal("trailing bytes accepted")
	}
	var data4 BsMapData
	if TryParseBsMapData([]byte{1, 2, 3}, &data4) {
		t.Fatal("short data accepted")
	}
}

func bsMapTestPrms() []byte {
	header := make([]byte, 60)
	put := func(word, value int) {
		binary.LittleEndian.PutUint32(header[word*4:], uint32(value))
	}
	strings := []byte("ab\x00cd\x00")
	blockA := []byte{1, 2, 3, 4}
	blockB := []byte{0x85, 0x01, 0, 0, 0, 0x00, 0x85, 0x00, 0, 0, 0}
	put(0, 2)
	put(7, 60)
	put(8, len(blockA))
	put(9, 60+len(blockA))
	put(10, len(blockB))
	put(11, 60+len(blockA)+len(blockB))
	put(12, 0)
	put(13, 60+len(blockA)+len(blockB))
	put(14, len(strings))
	out := append(header, blockA...)
	out = append(out, blockB...)
	out = append(out, strings...)
	return out
}

func TestParseBsMapParameters(t *testing.T) {
	params := ParseBsMapParameters(bsMapTestPrms())
	if params == nil {
		t.Fatal("valid MapPrms rejected")
	}
	if params.PlaceCount != 2 || len(params.Names) != 2 || params.Names[0] != "ab" || params.Names[1] != "cd" {
		t.Fatalf("params=%+v", params)
	}
	if len(params.Jumps) != 2 || params.Jumps[0] != 1 || params.Jumps[1] != 0 {
		t.Fatalf("jumps=%v", params.Jumps)
	}
	if params.PlaceLabelName(0) != "cd" || params.PlaceLabelName(5) != "" || params.Name(9) != "" {
		t.Fatal("label lookup wrong")
	}
	if ParseBsMapParameters([]byte{1, 2}) != nil {
		t.Fatal("short data accepted")
	}
	empty := &BsMapParameters{}
	if !empty.IsEmpty() {
		t.Fatal("IsEmpty wrong")
	}
}

func TestSelectBsMapTbxdlBranch(t *testing.T) {
	plain := NewBsMapItem("", 0, -1, -1, 0, 0, 1, 1, "fallback")
	if got := SelectBsMapTbxdlBranch(plain, func(int) int { return 0 }); got != "fallback" {
		t.Fatalf("fallback=%q", got)
	}
	item := plain
	item.Branches = []BsTbxMapBranch{
		{Conditions: []BsTbxMapTriple{{Property: 3, Value: 5, Operation: 0x0A}}, Label: "first"},
		{Conditions: []BsTbxMapTriple{{Property: 3, Value: 5, Operation: 0x0B, Invert: true}}, Label: "second"},
	}
	if got := SelectBsMapTbxdlBranch(item, func(key int) int { return 5 }); got != "first" {
		t.Fatalf("equal=%q", got)
	}
	if got := SelectBsMapTbxdlBranch(item, func(key int) int { return 6 }); got != "fallback" {
		t.Fatalf("nomatch=%q", got)
	}
	if got := SelectBsMapTbxdlBranch(item, func(key int) int { return 5 }); got != "first" {
		t.Fatalf("invert edge=%q", got)
	}
}

func TestScanBsMapTbxdlSections(t *testing.T) {
	pool := []byte{
		0x2E,
		0x02, 0x40, 0x00, 0x00, 0x01, 0x00, 0x0A, 0x30, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF,
		0x2E,
		0xFB, 'A', 'B', 0x00, 0xFF, 0xFF,
		0x2E,
	}
	sections := scanBsMapTbxdlSections(pool)
	if len(sections) != 1 {
		t.Fatalf("sections=%d", len(sections))
	}
	if sections[0].HighLight != 1 || sections[0].FirstLabel != "AB" {
		t.Fatalf("section=%+v", sections[0])
	}
	if len(sections[0].Branches) != 1 || sections[0].Branches[0].Label != "AB" {
		t.Fatalf("branches=%+v", sections[0].Branches)
	}
	if scanBsMapTbxdlSections([]byte{0x2E, 0x02}) != nil && len(scanBsMapTbxdlSections([]byte{0x2E})) != 0 {
		t.Fatal("empty pool should yield no sections")
	}
}

func TestBsMapLayoutEmpty(t *testing.T) {
	if !BsMapLayoutEmpty.IsEmpty() {
		t.Fatal("empty layout not empty")
	}
	with := BsMapLayout{Items: []BsMapItem{{}}}
	if with.IsEmpty() || with.HasMask() {
		t.Fatal("layout predicates wrong")
	}
	masked := BsMapLayout{Items: []BsMapItem{{}}, Mask: &BsMapMaskData{Width: 2, Height: 2, Values: []byte{0, 0, 0, 0}, ItemForValue: []int{0}}}
	if !masked.HasMask() {
		t.Fatal("HasMask wrong")
	}
	if BsMapCatalogEmpty.IsEmpty() != true {
		t.Fatal("catalog empty wrong")
	}
}
