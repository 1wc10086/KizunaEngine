package scf

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kizuna/kernel/core"
)

func samplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	dsk := filepath.Join(dir, "Dsk")
	if st, err := os.Stat(dsk); err != nil || !st.IsDir() {
		t.Skipf("samples dir %s missing", dsk)
	}
	return dsk
}

type dskEntry struct {
	archive string
	name    string
	data    []byte
}

func loadDSKEntries(t *testing.T, dskDir string) []dskEntry {
	t.Helper()
	pfts, err := filepath.Glob(filepath.Join(dskDir, "*.PFT"))
	if err != nil {
		t.Fatal(err)
	}
	lower, err := filepath.Glob(filepath.Join(dskDir, "*.pft"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range pfts {
		seen[p] = true
	}
	for _, p := range lower {
		if !seen[p] {
			pfts = append(pfts, p)
		}
	}
	if len(pfts) == 0 {
		t.Skipf("no PFT files in %s", dskDir)
	}
	var out []dskEntry
	for _, pft := range pfts {
		raw, err := os.ReadFile(pft)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) < 16 {
			continue
		}
		hs := int(binary.LittleEndian.Uint16(raw))
		cs := int(binary.LittleEndian.Uint16(raw[2:]))
		count := int(int32(binary.LittleEndian.Uint32(raw[4:])))
		stem := strings.TrimSuffix(filepath.Base(pft), filepath.Ext(pft))
		dskPath := ""
		for _, cand := range []string{stem + ".DSK", stem + ".dsk", strings.ToUpper(stem) + ".DSK", strings.ToLower(stem) + ".dsk"} {
			full := filepath.Join(dskDir, cand)
			if st, err := os.Stat(full); err == nil && !st.IsDir() {
				dskPath = full
				break
			}
		}
		if dskPath == "" {
			entries, err := filepath.Glob(filepath.Join(dskDir, "*"))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				b := filepath.Base(e)
				if strings.EqualFold(strings.TrimSuffix(b, filepath.Ext(b)), stem) && strings.EqualFold(filepath.Ext(b), ".dsk") {
					dskPath = e
					break
				}
			}
		}
		if dskPath == "" {
			t.Fatalf("no DSK for %s", pft)
		}
		dskRaw, err := os.ReadFile(dskPath)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			off := hs + 16*i
			if off+16 > len(raw) {
				t.Fatalf("%s truncated at entry %d", pft, i)
			}
			name := string(raw[off : off+8])
			if k := strings.IndexByte(name, 0); k >= 0 {
				name = name[:k]
			}
			cluster := int(binary.LittleEndian.Uint32(raw[off+8:]))
			size := int(binary.LittleEndian.Uint32(raw[off+12:]))
			start := cluster * cs
			if start < 0 || start+size > len(dskRaw) {
				t.Fatalf("%s entry %s out of range", pft, name)
			}
			out = append(out, dskEntry{archive: filepath.Base(pft), name: name, data: append([]byte(nil), dskRaw[start:start+size]...)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].archive != out[j].archive {
			return out[i].archive < out[j].archive
		}
		return out[i].name < out[j].name
	})
	return out
}

func TestRoundtrip(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	scfCount := 0
	otherCount := 0
	encSeen := map[string]int{}
	unitTotal := 0
	for _, e := range entries {
		if !LooksLikeSCF(e.data) {
			otherCount++
			continue
		}
		scfCount++
		doc, err := Decode(e.data, e.name)
		if err != nil {
			t.Fatalf("%s/%s decode: %v", e.archive, e.name, err)
		}
		if doc.Feature != FeatureID || doc.Format != FormatName {
			t.Fatalf("%s/%s bad identity %s/%s", e.archive, e.name, doc.Feature, doc.Format)
		}
		encSeen[doc.Encoding]++
		unitTotal += len(doc.Units)
		back, err := Encode(doc)
		if err != nil {
			t.Fatalf("%s/%s encode: %v", e.archive, e.name, err)
		}
		if !bytes.Equal(back, e.data) {
			first := -1
			for i := 0; i < len(back) && i < len(e.data); i++ {
				if back[i] != e.data[i] {
					first = i
					break
				}
			}
			t.Fatalf("%s/%s roundtrip mismatch at byte %d (len %d vs %d)", e.archive, e.name, first, len(back), len(e.data))
		}
		units, sections, err := DecodeAt(e.data, "e99/", 5000)
		if err != nil {
			t.Fatalf("%s/%s decodeAt: %v", e.archive, e.name, err)
		}
		for _, u := range units {
			if !strings.HasPrefix(u.ID, "e99/") {
				t.Fatalf("%s/%s unit id missing prefix: %s", e.archive, e.name, u.ID)
			}
		}
		for _, s := range sections {
			if s.Offset < 5000 {
				t.Fatalf("%s/%s section offset %d below base", e.archive, e.name, s.Offset)
			}
		}
		back2, err := EncodeParts(units, sections, "")
		if err != nil {
			t.Fatalf("%s/%s encodeParts: %v", e.archive, e.name, err)
		}
		if !bytes.Equal(back2, e.data) {
			t.Fatalf("%s/%s encodeParts mismatch", e.archive, e.name)
		}
	}
	t.Logf("scf=%d other=%d units=%d encodings=%v", scfCount, otherCount, unitTotal, encSeen)
	if scfCount == 0 {
		t.Fatalf("no SCF entries found")
	}
}

func TestSeraphAll128(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	count := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.archive, "SCNFILE") {
			continue
		}
		count++
		if !LooksLikeSCF(e.data) {
			t.Fatalf("%s/%s should be SCF", e.archive, e.name)
		}
	}
	if count != 128 {
		t.Fatalf("seraph entries = %d, want 128", count)
	}
}

func TestSeraphBranchStable(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	count := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.archive, "SCNFILE") {
			continue
		}
		count++
		c, err := parse(e.data)
		if err != nil {
			t.Fatalf("%s/%s parse: %v", e.archive, e.name, err)
		}
		w, err := walkCode(c.code, modeSeraph)
		if err != nil {
			t.Fatalf("%s/%s seraph walk: %v", e.archive, e.name, err)
		}
		if len(c.instrs) != len(w.instrs) || len(c.strs) != len(w.strs) {
			t.Fatalf("%s/%s left seraph branch (instrs %d vs %d, strs %d vs %d)",
				e.archive, e.name, len(c.instrs), len(w.instrs), len(c.strs), len(w.strs))
		}
		for i := range c.instrs {
			if c.instrs[i] != w.instrs[i] {
				t.Fatalf("%s/%s instr %d differs: %+v vs %+v", e.archive, e.name, i, c.instrs[i], w.instrs[i])
			}
		}
		for i := range c.strs {
			if c.strs[i] != w.strs[i] {
				t.Fatalf("%s/%s str %d differs: %+v vs %+v", e.archive, e.name, i, c.strs[i], w.strs[i])
			}
		}
	}
	if count != 128 {
		t.Fatalf("seraph entries = %d, want 128", count)
	}
}

func TestAcceptTJ2Dialect(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	cases := map[string]string{"SCENE.PFT": "SCN000", "scene(1).PFT": "SCN001"}
	for arch, name := range cases {
		found := false
		for _, e := range entries {
			if e.archive == arch && e.name == name {
				found = true
				if !LooksLikeSCF(e.data) {
					t.Fatalf("%s/%s should be accepted", arch, name)
				}
				doc, err := Decode(e.data, name)
				if err != nil {
					t.Fatalf("%s/%s decode: %v", arch, name, err)
				}
				back, err := Encode(doc)
				if err != nil {
					t.Fatalf("%s/%s encode: %v", arch, name, err)
				}
				if !bytes.Equal(back, e.data) {
					t.Fatalf("%s/%s roundtrip mismatch", arch, name)
				}
			}
		}
		if !found {
			t.Fatalf("sample %s/%s not found", arch, name)
		}
	}
	want := map[string]int{"SCENE.PFT": 1207, "SCNFILE(2).PFT": 128, "scene(1).PFT": 3285}
	got := map[string]int{}
	rejected := map[string][]string{}
	for _, e := range entries {
		if LooksLikeSCF(e.data) {
			if _, err := Decode(e.data, e.name); err != nil {
				t.Fatalf("%s/%s decode: %v", e.archive, e.name, err)
			}
			got[e.archive]++
			continue
		}
		rejected[e.archive] = append(rejected[e.archive], e.name)
	}
	for arch, n := range want {
		if got[arch] != n {
			t.Fatalf("%s accepted = %d, want %d (rejected: %v)", arch, got[arch], n, rejected[arch])
		}
	}
}

func hasC0Control(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' {
			return true
		}
	}
	return false
}

func hasJapanese(s string) bool {
	for _, r := range s {
		if r >= 0x3040 && r <= 0x30FF {
			return true
		}
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
		if r >= 0xFF61 && r <= 0xFF9F {
			return true
		}
	}
	return false
}

func TestTJ2Stats(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	type stats struct {
		total        int
		structured   int
		translatable int
		units        int
	}
	byArch := map[string]*stats{}
	for _, e := range entries {
		st := byArch[e.archive]
		if st == nil {
			st = &stats{}
			byArch[e.archive] = st
		}
		st.total++
		doc, err := Decode(e.data, e.name)
		if err != nil {
			t.Fatalf("%s/%s decode: %v", e.archive, e.name, err)
		}
		st.structured++
		tr := false
		for _, u := range doc.Units {
			if u.Kind == "raw" || u.Text == "" {
				continue
			}
			if hasC0Control(u.Text) {
				t.Fatalf("%s/%s unit %s has control bytes: %q", e.archive, e.name, u.ID, u.Text)
			}
			st.units++
			if hasJapanese(u.Text) {
				tr = true
			}
		}
		if tr {
			st.translatable++
		}
	}
	wantTotal := map[string]int{"SCENE.PFT": 1207, "SCNFILE(2).PFT": 128, "scene(1).PFT": 3285}
	wantTranslatable := map[string]int{"SCENE.PFT": 1205, "SCNFILE(2).PFT": 115, "scene(1).PFT": 3098}
	for arch, n := range wantTotal {
		st := byArch[arch]
		if st == nil {
			t.Fatalf("archive %s not found", arch)
		}
		if st.total != n || st.structured != n {
			t.Fatalf("%s total=%d structured=%d, want %d", arch, st.total, st.structured, n)
		}
		if st.translatable != wantTranslatable[arch] {
			t.Fatalf("%s translatable=%d, want %d", arch, st.translatable, wantTranslatable[arch])
		}
		t.Logf("%s: entries=%d structured=%d translatable=%d units=%d", arch, st.total, st.structured, st.translatable, st.units)
	}
}

func TestTJ2EditText(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	targets := map[string]*dskEntry{}
	for i, e := range entries {
		if len(targets) == 2 {
			break
		}
		if e.archive != "scene(1).PFT" && e.archive != "SCENE.PFT" {
			continue
		}
		if _, done := targets[e.archive]; done {
			continue
		}
		doc, err := Decode(e.data, e.name)
		if err != nil {
			t.Fatal(err)
		}
		pool, code := false, false
		for _, u := range doc.Units {
			if strings.HasPrefix(u.ID, "pool/") && u.Text != "" {
				pool = true
			}
			if strings.HasPrefix(u.ID, "code/") && u.Kind == "message" && hasJapanese(u.Text) {
				code = true
			}
		}
		if pool && code {
			targets[e.archive] = &entries[i]
		}
	}
	if len(targets) != 2 {
		t.Fatalf("tj2 edit targets = %d, want 2", len(targets))
	}
	for arch, target := range targets {
		doc, err := Decode(target.data, target.name)
		if err != nil {
			t.Fatal(err)
		}
		edited := map[string]string{}
		for i := range doc.Units {
			if doc.Units[i].Kind == "raw" || doc.Units[i].Text == "" {
				continue
			}
			edited[doc.Units[i].ID] = doc.Units[i].Text + "X"
			doc.Units[i].Text += "X"
		}
		if len(edited) == 0 {
			t.Fatalf("%s: nothing to edit", arch)
		}
		enc1, err := Encode(doc)
		if err != nil {
			t.Fatalf("%s encode edited: %v", arch, err)
		}
		if bytes.Equal(enc1, target.data) {
			t.Fatalf("%s edited encode identical to original", arch)
		}
		doc2, err := Decode(enc1, target.name)
		if err != nil {
			t.Fatalf("%s decode edited: %v", arch, err)
		}
		for _, u := range doc2.Units {
			want, ok := edited[u.ID]
			if !ok {
				continue
			}
			if u.Text != want {
				t.Fatalf("%s unit %s = %q, want %q", arch, u.ID, u.Text, want)
			}
		}
		enc2, err := Encode(doc2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(enc1, enc2) {
			t.Fatalf("%s re-encode unstable", arch)
		}
		t.Logf("%s/%s: edited units=%d", arch, target.name, len(edited))
	}
}

func TestEditText(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	var target *dskEntry
	for i, e := range entries {
		if !strings.HasPrefix(e.archive, "SCNFILE") {
			continue
		}
		doc, err := Decode(e.data, e.name)
		if err != nil {
			t.Fatal(err)
		}
		hasPool, hasCode := false, false
		for _, u := range doc.Units {
			if strings.HasPrefix(u.ID, "pool/") {
				hasPool = true
			}
			if strings.HasPrefix(u.ID, "code/") && u.Kind == "message" {
				hasCode = true
			}
		}
		if hasPool && hasCode {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("no seraph entry with pool and code strings")
	}
	doc, err := Decode(target.data, target.name)
	if err != nil {
		t.Fatal(err)
	}
	edited := map[string]string{}
	for i := range doc.Units {
		if doc.Units[i].Kind == "raw" {
			continue
		}
		edited[doc.Units[i].ID] = doc.Units[i].Text + "X"
		doc.Units[i].Text += "X"
	}
	if len(edited) == 0 {
		t.Fatalf("nothing to edit")
	}
	enc1, err := Encode(doc)
	if err != nil {
		t.Fatalf("encode edited: %v", err)
	}
	if bytes.Equal(enc1, target.data) {
		t.Fatalf("edited encode identical to original")
	}
	doc2, err := Decode(enc1, target.name)
	if err != nil {
		t.Fatalf("decode edited: %v", err)
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
	enc2, err := Encode(doc2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(enc1, enc2) {
		t.Fatalf("re-encode unstable")
	}
}

func TestFeatureFiles(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	var raw []byte
	var name string
	for _, e := range entries {
		if strings.HasPrefix(e.archive, "SCNFILE") && LooksLikeSCF(e.data) {
			c, err := parse(e.data)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.items) > 0 && len(c.strs) > 0 {
				raw = e.data
				name = e.name + ".scf"
				break
			}
		}
	}
	if raw == nil {
		t.Fatalf("no suitable sample")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, name)
	if err := os.WriteFile(in, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	mid := in + ".json"
	out := filepath.Join(dir, "out.scf")
	f := Feature{}
	if _, err := f.Run("decode", in, mid, core.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Run("encode", mid, out, core.Options{}); err != nil {
		t.Fatal(err)
	}
	back, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, raw) {
		t.Fatalf("file roundtrip mismatch")
	}
	format, ok := f.DetectFormat(in, raw[:4])
	if !ok || format != FormatName {
		t.Fatalf("detect decode = %q,%v", format, ok)
	}
	format, ok = f.DetectFormat(mid, nil)
	if !ok || format != FormatName {
		t.Fatalf("detect encode = %q,%v", format, ok)
	}
}

func TestTargetEncodingUtf8(t *testing.T) {
	entries := loadDSKEntries(t, samplesDir(t))
	var target *dskEntry
	for i, e := range entries {
		if !strings.HasPrefix(e.archive, "SCNFILE") {
			continue
		}
		doc, err := Decode(e.data, e.name)
		if err != nil {
			t.Fatal(err)
		}
		hasPool, hasCode := false, false
		for _, u := range doc.Units {
			if strings.HasPrefix(u.ID, "pool/") {
				hasPool = true
			}
			if strings.HasPrefix(u.ID, "code/") && u.Kind == "message" {
				hasCode = true
			}
		}
		if hasPool && hasCode {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("no seraph entry with pool and code strings")
	}
	doc, err := Decode(target.data, target.name)
	if err != nil {
		t.Fatal(err)
	}
	doc.Encoding = "utf8"
	orig := map[string]string{}
	for _, u := range doc.Units {
		orig[u.ID] = u.Text
	}
	changed := map[string]string{}
	for i := range doc.Units {
		if len(changed) >= 3 {
			break
		}
		if doc.Units[i].Kind == "raw" {
			continue
		}
		doc.Units[i].Text = "简体中文「译」" + doc.Units[i].ID
		changed[doc.Units[i].ID] = doc.Units[i].Text
	}
	if len(changed) == 0 {
		t.Fatalf("nothing to change")
	}
	enc1, err := Encode(doc)
	if err != nil {
		t.Fatalf("encode with utf8 target: %v", err)
	}
	if bytes.Equal(enc1, target.data) {
		t.Fatalf("encoded output identical to original")
	}
	doc2, err := Decode(enc1, target.name)
	if err != nil {
		t.Fatalf("decode encoded: %v", err)
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
	enc2, err := Encode(doc2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(enc1, enc2) {
		t.Fatalf("re-encode unstable")
	}
	t.Logf("%s/%s: utf8 target ok (%d -> %d bytes)", target.archive, target.name, len(target.data), len(enc1))
}
