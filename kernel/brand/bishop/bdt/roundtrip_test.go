package bdt

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func samplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	base := filepath.Join(dir, "Bdt")
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		t.Skip("missing samples: " + base)
	}
	return base
}

func collectSamples(t *testing.T, base string) []string {
	t.Helper()
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Skip("cannot read samples: " + err.Error())
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(e.Name()), ".bdt") {
			out = append(out, filepath.Join(base, e.Name()))
		}
	}
	if len(out) == 0 {
		t.Skip("no .bdt samples in " + base)
	}
	sort.Strings(out)
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
	return n
}

func TestRoundTrip(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	for _, path := range collectSamples(t, base) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read: %v", path, err)
		}
		doc, err := c.Decode(data, filepath.Base(path))
		if err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		back, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode: %v", path, err)
		}
		if !bytes.Equal(back, data) {
			t.Fatalf("%s: mismatch at byte %d (len %d -> %d)", path, firstDiff(back, data), len(data), len(back))
		}
		texts := 0
		for _, u := range doc.Units {
			if u.Kind == "text" {
				texts++
			}
		}
		t.Logf("%s: %d bytes %d units (%d text) roundtrip ok", filepath.Base(path), len(data), len(doc.Units), texts)
	}
}

func TestUtf8Target(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	for _, path := range collectSamples(t, base) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read: %v", path, err)
		}
		doc, err := c.Decode(data, filepath.Base(path))
		if err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		if doc.Encoding != "cp932" {
			t.Fatalf("%s: detected encoding = %q, want cp932", path, doc.Encoding)
		}
		doc.Encoding = "utf8"
		changed := map[string]string{}
		orig := map[string]string{}
		for i := range doc.Units {
			orig[doc.Units[i].ID] = doc.Units[i].Text
			if doc.Units[i].Kind != "text" || len(changed) >= 3 {
				continue
			}
			doc.Units[i].Text = "简体中文译文测试「译」" + doc.Units[i].ID
			changed[doc.Units[i].ID] = doc.Units[i].Text
		}
		if len(changed) == 0 {
			t.Fatalf("%s: no text units found", path)
		}
		back, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode as utf8: %v", path, err)
		}
		doc2, err := c.Decode(back, "utf8-target")
		if err != nil {
			t.Fatalf("%s: re-decode: %v", path, err)
		}
		if doc2.Encoding != "utf8" {
			t.Fatalf("%s: re-decoded encoding = %q, want utf8", path, doc2.Encoding)
		}
		for id, want := range changed {
			got, ok := c.UnitText(doc2, id)
			if !ok || got != want {
				t.Fatalf("%s: unit %s = %q, want %q", path, id, got, want)
			}
		}
		for _, u := range doc2.Units {
			if _, hit := changed[u.ID]; hit {
				continue
			}
			if u.Text != orig[u.ID] {
				t.Fatalf("%s: unit %s changed unexpectedly", path, u.ID)
			}
		}
		again, err := c.Encode(doc2)
		if err != nil {
			t.Fatalf("%s: re-encode: %v", path, err)
		}
		if !bytes.Equal(again, back) {
			t.Fatalf("%s: second roundtrip mismatch at byte %d", path, firstDiff(again, back))
		}
		t.Logf("%s: utf8 target ok (%d bytes, %d chinese units)", filepath.Base(path), len(back), len(changed))
	}
}

func TestCp932FallbackToUtf8(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	paths := collectSamples(t, base)
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("%s: read: %v", paths[0], err)
	}
	doc, err := c.Decode(data, filepath.Base(paths[0]))
	if err != nil {
		t.Fatalf("%s: decode: %v", paths[0], err)
	}
	target := ""
	for i := range doc.Units {
		if doc.Units[i].Kind == "text" {
			doc.Units[i].Text = "简体中文「译」"
			target = doc.Units[i].ID
			break
		}
	}
	if target == "" {
		t.Fatal("no text units found")
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode with unencodable char should fall back, got: %v", err)
	}
	if doc.Encoding != "utf8" {
		t.Fatalf("doc.Encoding = %q after fallback, want utf8", doc.Encoding)
	}
	if doc.Meta.Values["encoding.fallback"] == "" {
		t.Fatal("missing encoding.fallback annotation")
	}
	doc2, err := c.Decode(back, "fallback")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	got, ok := c.UnitText(doc2, target)
	if !ok || got != "简体中文「译」" {
		t.Fatalf("unit %s = %q, want chinese text", target, got)
	}
	t.Logf("%s: fallback annotation %q", filepath.Base(paths[0]), doc.Meta.Values["encoding.fallback"])
}

func TestModifyText(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	paths := collectSamples(t, base)
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("%s: read: %v", paths[0], err)
	}
	doc, err := c.Decode(data, filepath.Base(paths[0]))
	if err != nil {
		t.Fatalf("%s: decode: %v", paths[0], err)
	}
	changed := map[string]string{}
	n := 0
	for i := range doc.Units {
		if doc.Units[i].Kind != "text" || n >= 3 {
			continue
		}
		old := doc.Units[i].Text
		doc.Units[i].Text = "TEST_" + old + "_あいうえお"
		changed[doc.Units[i].ID] = doc.Units[i].Text
		n++
	}
	if n == 0 {
		t.Fatal("no text units found")
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode after modify: %v", err)
	}
	doc2, err := c.Decode(back, "modified")
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	for id, want := range changed {
		got, ok := c.UnitText(doc2, id)
		if !ok || got != want {
			t.Fatalf("unit %s: want %q got %q", id, want, got)
		}
	}
	again, err := c.Encode(doc2)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if !bytes.Equal(again, back) {
		t.Fatalf("second roundtrip mismatch at byte %d", firstDiff(again, back))
	}
	t.Logf("%s: modified %d text units, re-decode ok", filepath.Base(paths[0]), n)
}
