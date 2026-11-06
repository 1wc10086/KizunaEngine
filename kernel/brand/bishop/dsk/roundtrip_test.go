package dsk

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kizuna/kernel/core"
)

var samplePairs = []struct{ dsk, pft string }{
	{"SCENE.DSK", "SCENE.PFT"},
	{"SCNFILE(2).DSK", "SCNFILE(2).PFT"},
	{"scene(1).DSK", "scene(1).PFT"},
}

func samplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	sub := filepath.Join(dir, "Dsk")
	if st, err := os.Stat(sub); err != nil || !st.IsDir() {
		t.Skipf("samples dir %s missing", sub)
	}
	return sub
}

func copyPair(t *testing.T, dir, dskName, pftName string) (string, string, []byte, []byte) {
	t.Helper()
	dskData, err := os.ReadFile(filepath.Join(samplesDir(t), dskName))
	if err != nil {
		t.Skipf("sample %s missing: %v", dskName, err)
	}
	pftData, err := os.ReadFile(filepath.Join(samplesDir(t), pftName))
	if err != nil {
		t.Skipf("sample %s missing: %v", pftName, err)
	}
	dskPath := filepath.Join(dir, dskName)
	pftPath := filepath.Join(dir, pftName)
	if err := os.WriteFile(dskPath, dskData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pftPath, pftData, 0o644); err != nil {
		t.Fatal(err)
	}
	return dskPath, pftPath, dskData, pftData
}

func TestIdentity(t *testing.T) {
	for _, p := range samplePairs {
		t.Run(p.dsk, func(t *testing.T) {
			dir := t.TempDir()
			_, _, dskData, pftData := copyPair(t, dir, p.dsk, p.pft)
			doc, err := DecodeWithPFT(dskData, pftData, p.dsk)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Feature != FeatureID || doc.Format != FormatName {
				t.Fatalf("bad identity %s/%s", doc.Feature, doc.Format)
			}
			tab, err := parseTable(doc.Meta.Lists[listEntries], 2048)
			if err != nil {
				t.Fatal(err)
			}
			scfCount := 0
			for _, e := range tab {
				if e.scf {
					scfCount++
				}
			}
			backDSK, backPFT, err := EncodeWithPFT(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(backDSK, dskData) {
				first := -1
				for i := 0; i < len(backDSK) && i < len(dskData); i++ {
					if backDSK[i] != dskData[i] {
						first = i
						break
					}
				}
				t.Fatalf("dsk mismatch at byte %d (len %d vs %d)", first, len(backDSK), len(dskData))
			}
			if !bytes.Equal(backPFT, pftData) {
				t.Fatalf("pft mismatch (len %d vs %d)", len(backPFT), len(pftData))
			}
			t.Logf("entries=%d scf=%d raw=%d units=%d sections=%d", len(tab), scfCount, len(tab)-scfCount, len(doc.Units), len(doc.Sections))
			if len(tab) == 0 {
				t.Fatalf("no entries decoded")
			}
		})
	}
}

func TestEditText(t *testing.T) {
	samplesDir(t)
	dir := t.TempDir()
	_, _, dskData, pftData := copyPair(t, dir, "SCNFILE(2).DSK", "SCNFILE(2).PFT")
	doc, err := DecodeWithPFT(dskData, pftData, "SCNFILE(2).DSK")
	if err != nil {
		t.Fatal(err)
	}
	edited := map[string]string{}
	for i := range doc.Units {
		if doc.Units[i].Kind == "raw" {
			continue
		}
		doc.Units[i].Text += "X"
		edited[doc.Units[i].ID] = doc.Units[i].Text
	}
	if len(edited) == 0 {
		t.Fatalf("no editable units")
	}
	newDSK, newPFT, err := EncodeWithPFT(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(newDSK, dskData) {
		t.Fatalf("edited encode identical to original")
	}
	if len(newPFT) != len(pftData) {
		t.Fatalf("pft length changed %d -> %d", len(pftData), len(newPFT))
	}
	doc2, err := DecodeWithPFT(newDSK, newPFT, "edited.DSK")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	for _, u := range doc2.Units {
		want, ok := edited[u.ID]
		if !ok {
			continue
		}
		if u.Text != want {
			t.Fatalf("unit %s = %q, want %q", u.ID, u.Text, want)
		}
	}
	hs, cs, names, clusters, sizes, err := parsePFT(newPFT)
	if err != nil {
		t.Fatal(err)
	}
	oldHS, _, oldNames, _, _, err := parsePFT(pftData)
	if err != nil {
		t.Fatal(err)
	}
	if hs != oldHS || !equalStrings(names, oldNames) {
		t.Fatalf("pft order/names changed")
	}
	tab2, err := parseTable(doc2.Meta.Lists[listEntries], cs)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _, err := rebuildEntries(doc2, tab2, mustSlots(t, doc2))
	if err != nil {
		t.Fatal(err)
	}
	first := (hs + cs - 1) / cs
	if first != 1 {
		t.Fatalf("first cluster = %d, want 1", first)
	}
	for _, b := range newDSK[:first*cs] {
		if b != 0 {
			t.Fatalf("leading reserved cluster not zero")
		}
	}
	cursor := first
	for k := range tab2 {
		if int64(clusters[k])*int64(cs) != int64(cursor)*int64(cs) {
			t.Fatalf("entry %d cluster %d breaks contiguity", k, clusters[k])
		}
		if uint64(sizes[k]) != uint64(len(rebuilt[k])) {
			t.Fatalf("entry %d pft size %d != rebuilt %d", k, sizes[k], len(rebuilt[k]))
		}
		off := int(clusters[k]) * cs
		if !bytes.Equal(newDSK[off:off+len(rebuilt[k])], rebuilt[k]) {
			t.Fatalf("entry %d bytes misplaced", k)
		}
		if len(rebuilt[k]) > 0 {
			cursor += (len(rebuilt[k]) + cs - 1) / cs
		}
	}
	if len(newDSK) != cursor*cs {
		t.Fatalf("dsk length %d != packed end %d", len(newDSK), cursor*cs)
	}
	t.Logf("edited units=%d newLen=%d oldLen=%d", len(edited), len(newDSK), len(dskData))
}

func mustSlots(t *testing.T, doc *core.Document) []slot {
	t.Helper()
	slots, err := parseSlots(doc.Meta.Lists[listSlots])
	if err != nil {
		t.Fatal(err)
	}
	return slots
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFeatureFiles(t *testing.T) {
	samplesDir(t)
	dir := t.TempDir()
	dskPath, pftPath, dskData, pftData := copyPair(t, dir, "SCNFILE(2).DSK", "SCNFILE(2).PFT")
	f := Feature{}
	jsonPath := dskPath + ".json"
	res, err := f.Run("decode", dskPath, jsonPath, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Details["pft"] != "ok" {
		t.Fatalf("decode pft status = %q", res.Details["pft"])
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outDSK := filepath.Join(outDir, "SCNFILE(2).DSK")
	outPFT := filepath.Join(outDir, "SCNFILE(2).PFT")
	if err := os.WriteFile(outPFT, pftData, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = f.Run("encode", jsonPath, outDSK, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Details["pft"] != "updated" {
		t.Fatalf("encode pft status = %q", res.Details["pft"])
	}
	backDSK, err := os.ReadFile(outDSK)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backDSK, dskData) {
		t.Fatalf("file roundtrip dsk mismatch")
	}
	backPFT, err := os.ReadFile(outPFT)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backPFT, pftData) {
		t.Fatalf("file roundtrip pft mismatch")
	}
	if _, err := os.Stat(pftPath); err != nil {
		t.Fatal(err)
	}
	format, ok := f.DetectFormat(dskPath, dskData[:64])
	if !ok || format != FormatName {
		t.Fatalf("detect decode = %q,%v", format, ok)
	}
	format, ok = f.DetectFormat(jsonPath, nil)
	if !ok || format != FormatName {
		t.Fatalf("detect encode = %q,%v", format, ok)
	}
	foreign := filepath.Join(dir, "foreign.dat.json")
	if err := os.WriteFile(foreign, []byte("{\"feature\":\"bishop.scs\"}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.DetectFormat(foreign, nil); ok {
		t.Fatalf("foreign json detected as mine")
	}
	var foreignDoc core.Document
	foreignDoc.Feature = "bishop.scs"
	if _, err := f.Encode(&foreignDoc); err == nil {
		t.Fatalf("expected ErrNotMine, got nil")
	} else if !strings.Contains(err.Error(), "not mine") {
		t.Fatalf("expected ErrNotMine, got %v", err)
	}
}

func TestMissingPFT(t *testing.T) {
	samplesDir(t)
	dir := t.TempDir()
	loneDir := filepath.Join(dir, "lone")
	if err := os.MkdirAll(loneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dskData, err := os.ReadFile(filepath.Join(samplesDir(t), "SCNFILE(2).DSK"))
	if err != nil {
		t.Skipf("sample missing: %v", err)
	}
	loneDSK := filepath.Join(loneDir, "ONLY.DSK")
	if err := os.WriteFile(loneDSK, dskData, 0o644); err != nil {
		t.Fatal(err)
	}
	f := Feature{}
	jsonPath := loneDSK + ".json"
	res, err := f.Run("decode", loneDSK, jsonPath, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Details["pft"] != "missing" {
		t.Fatalf("decode pft status = %q", res.Details["pft"])
	}
	var doc core.Document
	if err := core.ReadJSON(jsonPath, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Meta.Values[metaMissing] != "true" {
		t.Fatalf("missing pft not marked in meta")
	}
	outDSK := filepath.Join(loneDir, "OUT.DSK")
	res, err = f.Run("encode", jsonPath, outDSK, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Details["pft"] != "missing" {
		t.Fatalf("encode pft status = %q", res.Details["pft"])
	}
	back, err := os.ReadFile(outDSK)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, dskData) {
		t.Fatalf("missing-pft roundtrip mismatch")
	}
	if _, err := os.Stat(filepath.Join(loneDir, "OUT.pft")); !os.IsNotExist(err) {
		t.Fatalf("pft should not have been created")
	}
}

func TestFindPFT(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Aa.DSK"), []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aa.PFT"), []byte{4, 5}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := FindPFT(filepath.Join(dir, "Aa.DSK"))
	if !ok {
		t.Fatalf("pft not found")
	}
	if filepath.Base(got) != "aa.PFT" {
		t.Fatalf("case not preserved: %s", got)
	}
	if _, ok := FindPFT(filepath.Join(dir, "NOPE.DSK")); ok {
		t.Fatalf("phantom pft found")
	}
}

func TestPFTLayout(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(samplesDir(t), "scene(1).PFT"))
	if err != nil {
		t.Skipf("sample missing: %v", err)
	}
	hs, cs, names, clusters, sizes, err := parsePFT(raw)
	if err != nil {
		t.Fatal(err)
	}
	if hs != 16 || cs != 2048 || len(names) != 3285 {
		t.Fatalf("hs=%d cs=%d count=%d", hs, cs, len(names))
	}
	if int64(hs)+int64(len(names))*16 != int64(len(raw)) {
		t.Fatalf("pft has trailing bytes")
	}
	_ = binary.LittleEndian.Uint16(raw)
	for i, n := range names {
		if n != scnName(i) {
			t.Fatalf("entry %d name = %q", i, n)
		}
		_ = clusters[i]
		_ = sizes[i]
	}
}

func TestTargetEncodingUtf8(t *testing.T) {
	samplesDir(t)
	dir := t.TempDir()
	_, _, dskData, pftData := copyPair(t, dir, "SCNFILE(2).DSK", "SCNFILE(2).PFT")
	plain, err := DecodeWithPFT(dskData, pftData, "SCNFILE(2).DSK")
	if err != nil {
		t.Fatal(err)
	}
	backDSK, backPFT, err := EncodeWithPFT(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backDSK, dskData) {
		t.Fatalf("identity dsk mismatch (%d vs %d bytes)", len(backDSK), len(dskData))
	}
	if !bytes.Equal(backPFT, pftData) {
		t.Fatalf("identity pft mismatch (%d vs %d bytes)", len(backPFT), len(pftData))
	}
	doc, err := DecodeWithPFT(dskData, pftData, "SCNFILE(2).DSK")
	if err != nil {
		t.Fatal(err)
	}
	doc.Encoding = "utf8"
	orig := make(map[string]string, len(doc.Units))
	for _, u := range doc.Units {
		orig[u.ID] = u.Text
	}
	changed := map[string]string{}
	for i := range doc.Units {
		if len(changed) >= 5 {
			break
		}
		if doc.Units[i].Kind == "raw" || doc.Units[i].Text == "" {
			continue
		}
		doc.Units[i].Text = "翻译测试" + doc.Units[i].ID
		changed[doc.Units[i].ID] = doc.Units[i].Text
	}
	if len(changed) == 0 {
		t.Fatalf("no editable units")
	}
	newDSK, newPFT, err := EncodeWithPFT(doc)
	if err != nil {
		t.Fatalf("encode with utf8 target: %v", err)
	}
	if bytes.Equal(newDSK, dskData) {
		t.Fatalf("encoded output identical to original")
	}
	doc2, err := DecodeWithPFT(newDSK, newPFT, "utf8.DSK")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	for id, want := range changed {
		got, ok := core.DefaultUnitText(doc2, id)
		if !ok || got != want {
			t.Fatalf("unit %s = %q, want %q", id, got, want)
		}
	}
	for _, u := range doc2.Units {
		if _, hit := changed[u.ID]; hit {
			continue
		}
		if u.Text != orig[u.ID] {
			t.Fatalf("unit %s changed unexpectedly: %q vs %q", u.ID, u.Text, orig[u.ID])
		}
	}
	again, _, err := EncodeWithPFT(doc2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, newDSK) {
		first := -1
		for i := 0; i < len(again) && i < len(newDSK); i++ {
			if again[i] != newDSK[i] {
				first = i
				break
			}
		}
		t.Fatalf("re-encode unstable at byte %d (%d vs %d bytes)", first, len(again), len(newDSK))
	}
	t.Logf("utf8 target ok (changed=%d %d -> %d bytes)", len(changed), len(dskData), len(newDSK))
}

func scnName(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return "SCN" + s
}
