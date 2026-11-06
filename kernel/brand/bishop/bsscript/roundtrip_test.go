package bsscript

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplesRoot(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("samples dir %s missing: %v", dir, err)
	}
	return dir
}

func collectDat(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".dat") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

func TestRoundtrip(t *testing.T) {
	root := filepath.Join(samplesRoot(t), "BSScript")
	files := collectDat(t, root)
	if len(files) == 0 {
		t.Skipf("no .dat under %s", root)
	}
	c := codec{}
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("%s: read: %v", f, err)
			continue
		}
		if _, ok := c.DetectFormat(f, data); !ok {
			t.Errorf("%s: DetectFormat rejected", f)
			continue
		}
		doc, err := c.Decode(data, filepath.Base(f))
		if err != nil {
			t.Errorf("%s: decode: %v", f, err)
			continue
		}
		if doc.Feature != FeatureID {
			t.Errorf("%s: feature = %q", f, doc.Feature)
		}
		if len(doc.Units) == 0 {
			t.Errorf("%s: no units", f)
		}
		back, err := c.Encode(doc)
		if err != nil {
			t.Errorf("%s: encode: %v", f, err)
			continue
		}
		if !bytes.Equal(back, data) {
			t.Errorf("%s: roundtrip differs: len %d -> %d, first diff at %d", f, len(data), len(back), firstDiff(data, back))
			continue
		}
		count++
	}
	t.Logf("bsscript roundtrip ok: %d/%d", count, len(files))
	if count != len(files) {
		t.Fatalf("only %d of %d samples roundtripped", count, len(files))
	}
}

func TestEditText(t *testing.T) {
	root := filepath.Join(samplesRoot(t), "BSScript")
	files := collectDat(t, root)
	if len(files) == 0 {
		t.Skipf("no .dat under %s", root)
	}
	c := codec{}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	doc, err := c.Decode(data, filepath.Base(files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Units) < 3 {
		t.Fatalf("too few units: %d", len(doc.Units))
	}
	changed := map[string]string{}
	for i, u := range doc.Units {
		if i%11 == 0 {
			changed[u.ID] = u.Text + "改編テスト中文XYZ"
		}
	}
	for id, text := range changed {
		if !c.SetUnitText(doc, id, text) {
			t.Fatalf("SetUnitText %s failed", id)
		}
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode after edit: %v", err)
	}
	doc2, err := c.Decode(back, "edited")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	for _, u := range doc2.Units {
		want, ok := changed[u.ID]
		if ok {
			if u.Text != want {
				t.Errorf("unit %s = %q, want %q", u.ID, u.Text, want)
			}
			continue
		}
		got, found := c.UnitText(doc, u.ID)
		if !found {
			t.Errorf("unit %s lost", u.ID)
			continue
		}
		if u.Text != got {
			t.Errorf("unit %s changed unexpectedly", u.ID)
		}
	}
}

func TestTargetEncodingUtf8(t *testing.T) {
	root := filepath.Join(samplesRoot(t), "BSScript")
	files := collectDat(t, root)
	if len(files) == 0 {
		t.Skipf("no .dat under %s", root)
	}
	c := codec{}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	doc, err := c.Decode(data, filepath.Base(files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Encoding != "cp932" {
		t.Fatalf("doc.Encoding = %q, want cp932", doc.Encoding)
	}
	doc.Encoding = "utf8"
	changed := map[string]string{}
	orig := map[string]string{}
	for i, u := range doc.Units {
		orig[u.ID] = u.Text
		if i%7 == 0 {
			changed[u.ID] = u.Text + "简体中文「译」"
		}
	}
	for id, text := range changed {
		if !c.SetUnitText(doc, id, text) {
			t.Fatalf("SetUnitText %s failed", id)
		}
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode with utf8 target: %v", err)
	}
	doc2, err := c.Decode(back, "utf8-target")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	for id, want := range changed {
		got, ok := c.UnitText(doc2, id)
		if !ok || got != want {
			t.Fatalf("unit %s = %q, want %q", id, got, want)
		}
	}
	for _, u := range doc2.Units {
		if _, hit := changed[u.ID]; hit {
			continue
		}
		if u.Text != orig[u.ID] {
			t.Fatalf("unit %s changed unexpectedly", u.ID)
		}
	}
	again, err := c.Encode(doc2)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(again, back) {
		t.Fatalf("second roundtrip mismatch at byte %d", firstDiff(again, back))
	}
	t.Logf("%s: utf8 target flow ok (%d -> %d bytes)", filepath.Base(files[0]), len(data), len(back))
}

func TestUnencodableCharFallback(t *testing.T) {
	root := filepath.Join(samplesRoot(t), "BSScript")
	files := collectDat(t, root)
	if len(files) == 0 {
		t.Skipf("no .dat under %s", root)
	}
	c := codec{}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	doc, err := c.Decode(data, filepath.Base(files[0]))
	if err != nil {
		t.Fatal(err)
	}
	target := ""
	for i := range doc.Units {
		if doc.Units[i].Kind == "message" {
			doc.Units[i].Text = "简体中文「译」汉语文"
			target = doc.Units[i].ID
			break
		}
	}
	if target == "" {
		t.Fatal("no message units found")
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode with simplified-only chars should fall back, got: %v", err)
	}
	doc2, err := c.Decode(back, "fallback")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	got, ok := c.UnitText(doc2, target)
	if !ok || got != "简体中文「译」汉语文" {
		t.Fatalf("unit %s = %q, want chinese text", target, got)
	}
	again, err := c.Encode(doc2)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(again, back) {
		t.Fatalf("second roundtrip mismatch at byte %d", firstDiff(again, back))
	}
	t.Logf("%s: per-string utf8 fallback ok (%d -> %d bytes)", filepath.Base(files[0]), len(data), len(back))
}
