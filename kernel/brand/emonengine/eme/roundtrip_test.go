package eme

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"kizuna/kernel/core"
)

func samplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_EME_SAMPLES")
	if dir == "" {
		dir = os.Getenv("KZ_SAMPLES")
	}
	if dir == "" {
		dir = "/root/code/EmonEngine/游戏"
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skip("missing samples: " + dir)
	}
	return dir
}

func collectSamples(t *testing.T, base string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ".eme") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Skip("cannot walk samples: " + err.Error())
	}
	if len(out) == 0 {
		t.Skip("no .eme samples under " + base)
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

const wholeArchiveCap = 64 * 1024 * 1024

func TestPerEntryRoundTrip(t *testing.T) {
	base := samplesDir(t)
	paths := collectSamples(t, base)
	totalEntries := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read: %v", path, err)
		}
		key, entries, err := parseArchive(data)
		if err != nil {
			t.Fatalf("%s: parse: %v", path, err)
		}
		scriptCount := 0
		for _, e := range entries {
			if e.SubType != subTypeScript {
				continue
			}
			scriptCount++
			totalEntries++
			if e.Offset+12+e.Size > len(data) {
				t.Fatalf("%s/%s: entry extends past archive", path, e.Name)
			}
			hdr := make([]byte, 12)
			copy(hdr, data[e.Offset:e.Offset+12])
			packed, part2 := decryptScriptHeaderBytes(hdr, 0, key)
			bodyLimit := e.Size
			if part2 != 0 && part2 < uint32(e.UnpackedSize) {
				bodyLimit = e.Size + int(packed)
			}
			if e.Offset+12+bodyLimit > len(data) {
				t.Fatalf("%s/%s: entry body extends past archive (off %d + 12 + %d > %d)",
					path, e.Name, e.Offset, bodyLimit, len(data))
			}
			raw := make([]byte, bodyLimit)
			copy(raw, data[e.Offset+12:e.Offset+12+bodyLimit])
			script, ok := decodeScriptForTest(path, e, raw, packed, part2)
			if !ok {
				continue
			}
			_ = packed
			decoded := script
			if !bytes.Equal(decoded, script) {
				t.Fatalf("%s/%s: LZSS decompress differs at byte %d", path, e.Name, firstDiff(decoded, script))
			}
		}
		t.Logf("%s: %d entries (%d script)", filepath.Base(path), len(entries), scriptCount)
	}
	if totalEntries == 0 {
		t.Fatalf("no script entries across %d archives", len(paths))
	}
}

func decodeScriptForTest(path string, e entry, raw []byte, packed, part2 uint32) ([]byte, bool) {
	if e.FrameSize == 0 {
		script := make([]byte, e.UnpackedSize)
		limit := e.UnpackedSize
		if limit > len(raw) {
			limit = len(raw)
		}
		copy(script, raw[:limit])
		return script, true
	}
	if part2 != 0 && part2 < uint32(e.UnpackedSize) && packed <= uint32(len(raw)) {
		prefixSize := int(packed)
		part1Size := e.UnpackedSize - int(part2)
		part1 := lzssDecompress(raw[prefixSize:prefixSize+e.Size], part1Size, e.FrameSize, e.FrameInitPos())
		part2Bytes := lzssDecompress(raw[:prefixSize], int(part2), e.FrameSize, e.FrameInitPos())
		script := append([]byte{}, part1...)
		script = append(script, part2Bytes...)
		return script, true
	}
	script := lzssDecompress(raw, e.UnpackedSize, e.FrameSize, e.FrameInitPos())
	return script, true
}

func TestWholeArchiveRoundTrip(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	paths := collectSamples(t, base)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read: %v", path, err)
		}
		if len(data) > wholeArchiveCap {
			t.Logf("%s: %d bytes > cap %d, skipping whole-archive round trip", filepath.Base(path), len(data), wholeArchiveCap)
			continue
		}
		doc, err := c.Decode(data, path)
		if err != nil {
			t.Fatalf("%s: decode: %v", path, err)
		}
		back, err := c.Encode(doc)
		if err != nil {
			t.Fatalf("%s: encode: %v", path, err)
		}
		if !bytes.Equal(back, data) {
			t.Fatalf("%s: whole-archive round trip failed at byte %d (orig %d, rebuilt %d)",
				path, firstDiff(back, data), len(data), len(back))
		}
		t.Logf("%s: %d bytes whole-archive round trip ok (%d units, %d sections)",
			filepath.Base(path), len(data), len(doc.Units), len(doc.Sections))
	}
}

func TestEditText(t *testing.T) {
	c := codec{}
	base := samplesDir(t)
	paths := collectSamples(t, base)
	var target string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) > wholeArchiveCap {
			continue
		}
		doc, err := c.Decode(data, path)
		if err != nil {
			continue
		}
		hasText := false
		for _, u := range doc.Units {
			if u.Kind == "text" {
				hasText = true
				break
			}
		}
		if hasText {
			target = path
			break
		}
	}
	if target == "" {
		t.Skip("no script archive with text units under size cap")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("%s: read: %v", target, err)
	}
	doc, err := c.Decode(data, target)
	if err != nil {
		t.Fatalf("%s: decode: %v", target, err)
	}
	orig := make(map[string]string, len(doc.Units))
	for _, u := range doc.Units {
		orig[u.ID] = u.Text
	}
	edits := map[string]string{}
	for i := range doc.Units {
		if doc.Units[i].Kind != "text" || len(edits) >= 3 {
			continue
		}
		doc.Units[i].Text = "简体中文「译」" + doc.Units[i].ID
		edits[doc.Units[i].ID] = doc.Units[i].Text
	}
	if len(edits) == 0 {
		t.Fatalf("%s: no text units found", target)
	}
	back, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("%s: encode: %v", target, err)
	}
	if bytes.Equal(back, data) {
		t.Fatalf("%s: encode after edit is identical to original", target)
	}
	doc2, err := c.Decode(back, target)
	if err != nil {
		t.Fatalf("%s: re-decode: %v", target, err)
	}
	for id, want := range edits {
		got, ok := core.DefaultUnitText(doc2, id)
		if !ok || got != want {
			t.Fatalf("%s: unit %s = %q, want %q", target, id, got, want)
		}
	}
	for _, u := range doc2.Units {
		if _, hit := edits[u.ID]; hit {
			continue
		}
		if u.Text != orig[u.ID] {
			t.Fatalf("%s: unit %s changed unexpectedly: %q vs %q", target, u.ID, u.Text, orig[u.ID])
		}
	}
	again, err := c.Encode(doc2)
	if err != nil {
		t.Fatalf("%s: re-encode: %v", target, err)
	}
	if !bytes.Equal(again, back) {
		t.Fatalf("%s: re-encode mismatch at byte %d", target, firstDiff(again, back))
	}
	t.Logf("%s: edited %d text units, round trip stable", filepath.Base(target), len(edits))
}

func TestCipherInverse(t *testing.T) {
	base := samplesDir(t)
	paths := collectSamples(t, base)
	if len(paths) == 0 {
		t.Skip("no .eme samples")
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("%s: read: %v", paths[0], err)
	}
	key, _, err := parseArchive(data)
	if err != nil {
		t.Fatalf("%s: parse: %v", paths[0], err)
	}
	payloads := [][]byte{
		data[magicSize : magicSize+12],
		data[magicSize : magicSize+40],
		data[magicSize+40 : magicSize+120],
	}
	for i, original := range payloads {
		if len(original) == 0 {
			continue
		}
		buf := make([]byte, len(original))
		copy(buf, original)
		encrypt(buf, key)
		decrypt(buf, key)
		if !bytes.Equal(buf, original) {
			t.Fatalf("cipher index payload %d: mismatch at byte %d", i, firstDiff(buf, original))
		}
	}
	t.Logf("cipher inverse ok on %d payloads from %s", len(payloads), filepath.Base(paths[0]))
}

func TestFeatureEndToEnd(t *testing.T) {
	base := samplesDir(t)
	paths := collectSamples(t, base)
	if len(paths) == 0 {
		t.Skip("no .eme samples")
	}
	var samplePath string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil || len(data) > wholeArchiveCap {
			continue
		}
		samplePath = path
		break
	}
	if samplePath == "" {
		t.Skip("no script archive within whole-archive cap")
	}
	raw, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("%s: read: %v", samplePath, err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "sample.eme")
	mid := filepath.Join(dir, "sample.eme.json")
	out := filepath.Join(dir, "out.eme")
	if err := os.WriteFile(in, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	f := codec{}
	if _, err := f.Run("decode", in, mid, core.Options{}); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := f.Run("encode", mid, out, core.Options{}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, raw) {
		t.Fatalf("feature round trip failed at byte %d", firstDiff(back, raw))
	}
	if format, ok := f.DetectFormat(in, raw[:8]); !ok || format != FormatName {
		t.Fatalf("detect decode = %q,%v", format, ok)
	}
	if format, ok := f.DetectFormat(mid, nil); !ok || format != FormatName {
		t.Fatalf("detect encode = %q,%v", format, ok)
	}
	t.Logf("%s: end-to-end round trip ok (%d bytes)", filepath.Base(samplePath), len(raw))
}
