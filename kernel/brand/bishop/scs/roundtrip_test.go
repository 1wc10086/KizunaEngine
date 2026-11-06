package scs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"kizuna/kernel/core"
)

func sampleDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	scsDir := filepath.Join(dir, "Scs")
	if st, err := os.Stat(scsDir); err != nil || !st.IsDir() {
		t.Skip("scs samples not found: " + scsDir)
	}
	return scsDir
}

func sampleFiles(t *testing.T) []string {
	t.Helper()
	dir := sampleDir(t)
	names := []string{"MAINCON.SCS", "MAINCON (1).SCS"}
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		t.Skip("no scs sample files present")
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

func TestDecodeEncodeRoundtrip(t *testing.T) {
	c := Codec{}
	for _, p := range sampleFiles(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: read: %v", p, err)
		}
		doc, err := c.Decode(raw, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: decode: %v", p, err)
		}
		if len(doc.Units) == 0 {
			t.Fatalf("%s: no units decoded", p)
		}
		if doc.Feature != FeatureID || doc.Format != FormatName {
			t.Fatalf("%s: bad identity %q %q", p, doc.Feature, doc.Format)
		}
		back, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode: %v", p, err)
		}
		if !bytes.Equal(back, raw) {
			t.Fatalf("%s: roundtrip mismatch at byte %d (len %d vs %d)", p, firstDiff(back, raw), len(back), len(raw))
		}
		t.Logf("%s: %d bytes %d units encoding=%s roundtrip ok", filepath.Base(p), len(raw), len(doc.Units), doc.Encoding)
	}
}

func TestTargetEncodings(t *testing.T) {
	c := Codec{}
	for _, p := range sampleFiles(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: read: %v", p, err)
		}
		orig, err := c.Decode(raw, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: decode: %v", p, err)
		}
		for _, target := range []string{"cp932", "utf8"} {
			doc, err := c.Decode(raw, filepath.Base(p))
			if err != nil {
				t.Fatalf("%s: decode: %v", p, err)
			}
			doc.Encoding = target
			for i := range doc.Sections {
				if doc.Sections[i].Kind == "lines" {
					doc.Sections[i].Encoding = target
				}
			}
			if doc.Meta.Lists != nil {
				delete(doc.Meta.Lists, "encoding")
			}
			enc, err := c.Encode(doc)
			if err != nil {
				t.Fatalf("%s: encode as %s: %v", p, target, err)
			}
			back, err := c.Decode(enc, filepath.Base(p))
			if err != nil {
				t.Fatalf("%s: re-decode %s: %v", p, target, err)
			}
			if len(back.Units) != len(orig.Units) {
				t.Fatalf("%s: %s unit count %d vs %d", p, target, len(back.Units), len(orig.Units))
			}
			for i := range orig.Units {
				if back.Units[i].Text != orig.Units[i].Text {
					t.Fatalf("%s: %s unit %s text changed", p, target, orig.Units[i].ID)
				}
			}
			again, err := c.Encode(back)
			if err != nil {
				t.Fatalf("%s: re-encode %s: %v", p, target, err)
			}
			if !bytes.Equal(again, enc) {
				t.Fatalf("%s: %s self roundtrip mismatch at byte %d", p, target, firstDiff(again, enc))
			}
			t.Logf("%s: target=%s %d bytes self-consistent", filepath.Base(p), target, len(enc))
		}
	}
}

func TestModifyTextReencode(t *testing.T) {
	c := Codec{}
	for _, p := range sampleFiles(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: read: %v", p, err)
		}
		doc, err := c.Decode(raw, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: decode: %v", p, err)
		}
		marks := map[string]string{
			"line/0": "MODIFIED_001",
			"line/3": "MODIFIED_004",
		}
		for id, text := range marks {
			if !c.SetUnitText(doc, id, text) {
				t.Fatalf("%s: missing unit %s", p, id)
			}
		}
		enc, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode: %v", p, err)
		}
		back, err := c.Decode(enc, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: re-decode: %v", p, err)
		}
		for id, text := range marks {
			got, ok := c.UnitText(back, id)
			if !ok || got != text {
				t.Fatalf("%s: unit %s = %q, want %q", p, id, got, text)
			}
		}
		if len(back.Units) != len(doc.Units) {
			t.Fatalf("%s: unit count changed %d vs %d", p, len(back.Units), len(doc.Units))
		}
		t.Logf("%s: modify+reencode ok (%d bytes)", filepath.Base(p), len(enc))
	}
}

func TestPlainBranch(t *testing.T) {
	c := Codec{}
	raw := []byte("#init\x0d\x0a#t wait 1\x0a\x0d\x0a#end")
	doc, err := c.Decode(raw, "plain.scs")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Units) != 4 {
		t.Fatalf("units = %d, want 4", len(doc.Units))
	}
	if doc.Meta.Values["encrypted"] != "false" {
		t.Fatalf("encrypted flag = %q", doc.Meta.Values["encrypted"])
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(back, raw) {
		t.Fatalf("plain roundtrip mismatch at byte %d: %q vs %q", firstDiff(back, raw), back, raw)
	}
}

func TestRunCodecFiles(t *testing.T) {
	c := Codec{}
	paths := sampleFiles(t)
	p := paths[0]
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%s: read: %v", p, err)
	}
	tmp := t.TempDir()
	jsonPath := filepath.Join(tmp, "out.scs.json")
	res, err := c.Run("decode", p, jsonPath, core.Options{})
	if err != nil {
		t.Fatalf("run decode: %v", err)
	}
	if res.BytesIn != int64(len(raw)) {
		t.Fatalf("bytesIn = %d, want %d", res.BytesIn, len(raw))
	}
	binPath := filepath.Join(tmp, "out.scs")
	if _, err := c.Run("encode", jsonPath, binPath, core.Options{}); err != nil {
		t.Fatalf("run encode: %v", err)
	}
	back, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(back, raw) {
		t.Fatalf("run roundtrip mismatch at byte %d", firstDiff(back, raw))
	}
	plainJSON := filepath.Join(tmp, "u.scs.json")
	if _, err := c.Run("decode", p, plainJSON, core.Options{TextEncoding: "utf8"}); err != nil {
		t.Fatalf("run decode utf8: %v", err)
	}
	utf8Bin := filepath.Join(tmp, "u.scs")
	if _, err := c.Run("encode", plainJSON, utf8Bin, core.Options{}); err != nil {
		t.Fatalf("run encode utf8: %v", err)
	}
	encRaw, err := os.ReadFile(utf8Bin)
	if err != nil {
		t.Fatalf("read utf8 back: %v", err)
	}
	first, err := c.Decode(raw, filepath.Base(p))
	if err != nil {
		t.Fatalf("decode orig: %v", err)
	}
	second, err := c.Decode(encRaw, "u.scs")
	if err != nil {
		t.Fatalf("decode utf8 back: %v", err)
	}
	if len(second.Units) != len(first.Units) {
		t.Fatalf("utf8 unit count %d vs %d", len(second.Units), len(first.Units))
	}
	for i := range first.Units {
		if second.Units[i].Text != first.Units[i].Text {
			t.Fatalf("utf8 unit %s text changed", first.Units[i].ID)
		}
	}
}

func TestEncodeRejectsForeignFeature(t *testing.T) {
	c := Codec{}
	doc := &core.Document{Feature: "bishop.bdt", Format: "bdt"}
	if _, err := c.Encode(doc); err == nil {
		t.Fatal("expected ErrNotMine, got nil")
	}
}

func TestUtf8TargetChinese(t *testing.T) {
	c := Codec{}
	for _, p := range sampleFiles(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: read: %v", p, err)
		}
		doc, err := c.Decode(raw, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: decode: %v", p, err)
		}
		if doc.Encoding != "cp932" {
			t.Fatalf("%s: doc.Encoding = %q, want cp932", p, doc.Encoding)
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
			if doc.Units[i].Kind != "line" || doc.Units[i].Text == "" {
				continue
			}
			doc.Units[i].Text = "简体中文译文「译」" + doc.Units[i].ID
			changed[doc.Units[i].ID] = doc.Units[i].Text
		}
		if len(changed) == 0 {
			t.Fatalf("%s: no line units found", p)
		}
		enc, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode with utf8 target: %v", p, err)
		}
		back, err := c.Decode(enc, filepath.Base(p))
		if err != nil {
			t.Fatalf("%s: re-decode: %v", p, err)
		}
		if len(back.Units) != len(doc.Units) {
			t.Fatalf("%s: unit count %d vs %d", p, len(back.Units), len(doc.Units))
		}
		for id, want := range changed {
			got, ok := c.UnitText(back, id)
			if !ok || got != want {
				t.Fatalf("%s: unit %s = %q, want %q", p, id, got, want)
			}
		}
		for _, u := range back.Units {
			if _, hit := changed[u.ID]; hit {
				continue
			}
			if u.Text != orig[u.ID] {
				t.Fatalf("%s: unit %s changed unexpectedly", p, u.ID)
			}
		}
		again, err := c.Encode(back)
		if err != nil {
			t.Fatalf("%s: re-encode: %v", p, err)
		}
		if !bytes.Equal(again, enc) {
			t.Fatalf("%s: second roundtrip mismatch at byte %d", p, firstDiff(again, enc))
		}
		t.Logf("%s: utf8 chinese target ok (%d -> %d bytes)", filepath.Base(p), len(raw), len(enc))
	}
}
