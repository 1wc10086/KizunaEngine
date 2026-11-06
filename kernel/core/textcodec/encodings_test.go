package textcodec

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"kizuna/kernel/core/textcodec/tables"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCP932KnownVectors(t *testing.T) {
	vectors := []struct {
		hx string
		s  string
	}{
		{"", ""},
		{"48656c6c6f2c20576f726c6421", "Hello, World!"},
		{"82a082a282a482a682a8", "あいうえお"},
		{"83418343834583478349", "アイウエオ"},
		{"8abf8e9a207465737420313233", "漢字 test 123"},
		{"a1a2a3a4a5b1b2b3", "｡｢｣､･ｱｲｳ"},
		{"82b182f182c982bf82cd814190a28a458149", "こんにちは、世界！"},
		{"48656c6c6f2c2090a28a458149", "Hello, 世界！"},
		{"8140814181428149814881758176", "　、。！？「」"},
		{"889f88a088a188a288a388a4", "亜唖娃阿哀愛"},
		{"989f98a098a198a298a3", "弌丐丕个丱"},
		{"b6c0b6c5", "ｶﾀｶﾅ"},
		{"9286", "中"},
		{"80", "\u0080"},
		{"a0", "\uf8f0"},
		{"fd", "\uf8f1"},
		{"fe", "\uf8f2"},
		{"ff", "\uf8f3"},
	}
	for _, v := range vectors {
		b := mustHex(t, v.hx)
		if got := DecodeCP932(b); got != v.s {
			t.Errorf("DecodeCP932(%x) = %q, want %q", b, got, v.s)
		}
		enc, err := EncodeCP932(v.s)
		if err != nil {
			t.Errorf("EncodeCP932(%q) error: %v", v.s, err)
			continue
		}
		if !bytes.Equal(enc, b) {
			t.Errorf("EncodeCP932(%q) = %x, want %x", v.s, enc, b)
		}
	}
}

func TestGBKKnownVectors(t *testing.T) {
	vectors := []struct {
		hx string
		s  string
	}{
		{"", ""},
		{"48656c6c6f", "Hello"},
		{"d6d0cec4b2e2cad4", "中文测试"},
		{"bcf2cce5d6d0cec4", "简体中文"},
		{"d6d0", "中"},
		{"a4a2", "あ"},
	}
	for _, v := range vectors {
		b := mustHex(t, v.hx)
		if got := DecodeGBK(b); got != v.s {
			t.Errorf("DecodeGBK(%x) = %q, want %q", b, got, v.s)
		}
		enc, err := EncodeGBK(v.s)
		if err != nil {
			t.Errorf("EncodeGBK(%q) error: %v", v.s, err)
			continue
		}
		if !bytes.Equal(enc, b) {
			t.Errorf("EncodeGBK(%q) = %x, want %x", v.s, enc, b)
		}
	}
}

func TestCP932FullRoundTrip(t *testing.T) {
	for b := 0; b < 256; b++ {
		s := DecodeCP932([]byte{byte(b)})
		_, singleOK := tables.Cp932Single(byte(b))
		if b < 0x80 || singleOK {
			enc, err := EncodeCP932(s)
			if err != nil {
				t.Fatalf("EncodeCP932(DecodeCP932(%02X)) error: %v", b, err)
			}
			if len(enc) != 1 || enc[0] != byte(b) {
				t.Fatalf("EncodeCP932(DecodeCP932(%02X)) = %X", b, enc)
			}
		} else {
			if s != "�" {
				t.Fatalf("DecodeCP932(%02X) = %q, want replacement", b, s)
			}
			if _, err := EncodeCP932(s); err == nil {
				t.Fatalf("EncodeCP932(replacement) should fail for byte %02X", b)
			}
		}
	}
	winners := map[rune]int{}
	for lead := 0x81; lead <= 0xFC; lead++ {
		for trail := 0x40; trail <= 0xFC; trail++ {
			r, ok := tables.Cp932Lookup(byte(lead), byte(trail))
			if !ok {
				continue
			}
			key := lead*256 + trail
			if _, dup := winners[r]; !dup {
				winners[r] = key
			}
		}
	}
	if len(winners) == 0 {
		t.Fatal("no cp932 pairs")
	}
	losers := 0
	for lead := 0x81; lead <= 0xFC; lead++ {
		for trail := 0x40; trail <= 0xFC; trail++ {
			r, ok := tables.Cp932Lookup(byte(lead), byte(trail))
			if !ok {
				continue
			}
			key := lead*256 + trail
			pair := []byte{byte(lead), byte(trail)}
			s := DecodeCP932(pair)
			if s != string(r) {
				t.Fatalf("DecodeCP932(%X) = %q, want %q", pair, s, string(r))
			}
			enc, err := EncodeCP932(s)
			if err != nil {
				t.Fatalf("EncodeCP932(%q) error: %v", s, err)
			}
			want := []byte{byte(winners[r] >> 8), byte(winners[r])}
			if !bytes.Equal(enc, want) {
				t.Fatalf("EncodeCP932(%q) = %X, want %X", s, enc, want)
			}
			if key != winners[r] {
				losers++
			}
		}
	}
	if losers == 0 {
		t.Fatal("expected duplicate-encoded runes in cp932")
	}
	t.Logf("cp932 runes=%d losers=%d", len(winners), losers)
	for r := range winners {
		s := string(r)
		enc, err := EncodeCP932(s)
		if err != nil {
			t.Fatalf("EncodeCP932(%q) error: %v", s, err)
		}
		if DecodeCP932(enc) != s {
			t.Fatalf("DecodeCP932(EncodeCP932(%q)) mismatch", s)
		}
	}
	for lead := 0x81; lead <= 0xFC; lead++ {
		if isCP932Lead(byte(lead)) {
			if got := DecodeCP932([]byte{byte(lead)}); got != "�" {
				t.Fatalf("DecodeCP932(truncated %02X) = %q", lead, got)
			}
		}
	}
	if got := DecodeCP932([]byte{0x81, 0x7F, 0x41}); got != "�A" {
		t.Fatalf("DecodeCP932(81 7F 41) = %q", got)
	}
	for _, s := range []string{"测", "�", "😀", "é"} {
		if _, err := EncodeCP932(s); err == nil {
			t.Fatalf("EncodeCP932(%q) should fail", s)
		}
	}
}

func TestGBKFullRoundTrip(t *testing.T) {
	for b := 0; b < 256; b++ {
		s := DecodeGBK([]byte{byte(b)})
		if b < 0x80 {
			enc, err := EncodeGBK(s)
			if err != nil {
				t.Fatalf("EncodeGBK(DecodeGBK(%02X)) error: %v", b, err)
			}
			if len(enc) != 1 || enc[0] != byte(b) {
				t.Fatalf("EncodeGBK(DecodeGBK(%02X)) = %X", b, enc)
			}
			continue
		}
		if s != "�" {
			t.Fatalf("DecodeGBK(%02X) = %q, want replacement", b, s)
		}
		if _, err := EncodeGBK(s); err == nil {
			t.Fatalf("EncodeGBK(replacement) should fail for byte %02X", b)
		}
	}
	winners := map[rune]int{}
	for lead := 0x81; lead <= 0xFE; lead++ {
		for trail := 0x40; trail <= 0xFE; trail++ {
			r, ok := tables.GbkLookup(byte(lead), byte(trail))
			if !ok {
				continue
			}
			key := lead*256 + trail
			if _, dup := winners[r]; !dup {
				winners[r] = key
			}
		}
	}
	if len(winners) == 0 {
		t.Fatal("no gbk pairs")
	}
	for lead := 0x81; lead <= 0xFE; lead++ {
		for trail := 0x40; trail <= 0xFE; trail++ {
			r, ok := tables.GbkLookup(byte(lead), byte(trail))
			if !ok {
				continue
			}
			pair := []byte{byte(lead), byte(trail)}
			if got := DecodeGBK(pair); got != string(r) {
				t.Fatalf("DecodeGBK(%X) = %q, want %q", pair, got, string(r))
			}
			enc, err := EncodeGBK(string(r))
			if err != nil {
				t.Fatalf("EncodeGBK(%q) error: %v", string(r), err)
			}
			want := []byte{byte(winners[r] >> 8), byte(winners[r])}
			if !bytes.Equal(enc, want) {
				t.Fatalf("EncodeGBK(%q) = %X, want %X", string(r), enc, want)
			}
		}
	}
	for r := range winners {
		s := string(r)
		enc, err := EncodeGBK(s)
		if err != nil {
			t.Fatalf("EncodeGBK(%q) error: %v", s, err)
		}
		if DecodeGBK(enc) != s {
			t.Fatalf("DecodeGBK(EncodeGBK(%q)) mismatch", s)
		}
	}
	for lead := 0x81; lead <= 0xFE; lead++ {
		if got := DecodeGBK([]byte{byte(lead)}); got != "�" {
			t.Fatalf("DecodeGBK(truncated %02X) = %q", lead, got)
		}
	}
	for _, s := range []string{"�", "😀"} {
		if _, err := EncodeGBK(s); err == nil {
			t.Fatalf("EncodeGBK(%q) should fail", s)
		}
	}
}

func TestUTF16LERoundTrip(t *testing.T) {
	if got := DecodeUTF16LE(mustHex(t, "42304100")); got != "あA" {
		t.Fatalf("DecodeUTF16LE vector = %q", got)
	}
	cases := []string{
		"",
		"A",
		"Hello, World!",
		"あA",
		"こんにちは、世界！",
		"漢字 test 123 ｱｲｳ",
		"𠀋😀A",
		"a\x00b",
		"中文测试",
	}
	for _, s := range cases {
		enc, err := EncodeUTF16LE(s)
		if err != nil {
			t.Fatalf("EncodeUTF16LE(%q) error: %v", s, err)
		}
		if len(enc)%2 != 0 {
			t.Fatalf("EncodeUTF16LE(%q) odd length", s)
		}
		if got := DecodeUTF16LE(enc); got != s {
			t.Fatalf("DecodeUTF16LE(EncodeUTF16LE(%q)) = %q", s, got)
		}
	}
	if got := DecodeUTF16LE([]byte{0x41}); got != "�" {
		t.Fatalf("DecodeUTF16LE(41) = %q", got)
	}
	if got := DecodeUTF16LE([]byte{0x42, 0x30, 0xFF}); got != "あ�" {
		t.Fatalf("DecodeUTF16LE(4230 FF) = %q", got)
	}
	if got := DecodeUTF16LE([]byte{0x00, 0xD8, 0x41, 0x00}); got != "�A" {
		t.Fatalf("DecodeUTF16LE(lone surrogate) = %q", got)
	}
	if got := DecodeUTF16LE(nil); got != "" {
		t.Fatalf("DecodeUTF16LE(nil) = %q", got)
	}
}

func TestDetectSynthetic(t *testing.T) {
	cp932Block, err := EncodeCP932("こんにちは、世界！\x00おはようございます\x00")
	if err != nil {
		t.Fatal(err)
	}
	utf16Block, err := EncodeUTF16LE("こんにちは\x00おはよう\x00")
	if err != nil {
		t.Fatal(err)
	}
	gbkBlock, err := EncodeGBK("中文测试，简体中文。\x00还有更多内容\x00")
	if err != nil {
		t.Fatal(err)
	}
	kanaOnly, err := EncodeUTF16LE("こんにちは")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		b    []byte
		want string
	}{
		{"cp932", cp932Block, "cp932"},
		{"utf8zh", []byte("中文测试，简体中文。\x00还有更多\x00"), "utf8"},
		{"utf8ja", []byte("こんにちは、世界！\x00おはよう\x00"), "utf8"},
		{"utf16le", utf16Block, "utf16le"},
		{"utf16kana", kanaOnly, "utf16le"},
		{"gbk", gbkBlock, "gbk"},
		{"ascii", []byte("Hello"), "cp932"},
		{"empty", []byte{}, "cp932"},
	}
	for _, c := range cases {
		if got := DetectTextEncoding(c.b); got != c.want {
			t.Errorf("DetectTextEncoding(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func samplesDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("KZ_SAMPLES")
	if dir == "" {
		dir = "/root/code/Bishop/示例"
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Skip("samples dir missing")
	}
	return dir
}

func TestDetectRealSamples(t *testing.T) {
	dir := samplesDir(t)
	raw, err := os.ReadFile(filepath.Join(dir, "Bdt", "scene00.bdt"))
	if err != nil {
		t.Skip("bdt sample missing")
	}
	xored := make([]byte, len(raw))
	for i, c := range raw {
		xored[i] = c ^ 0xFF
	}
	if got := DetectTextEncoding(xored); got != "cp932" {
		t.Fatalf("DetectTextEncoding(bdt) = %q, want cp932", got)
	}
	text := DecodeCP932(xored)
	if !utf8.ValidString(text) {
		t.Fatal("bdt decoded text is not valid utf8")
	}
	hira := 0
	for _, r := range text {
		if r == 0xFFFD {
			t.Fatal("bdt decoded text contains replacement char")
		}
		if r >= 0x3040 && r <= 0x309F {
			hira++
		}
	}
	if hira == 0 {
		t.Fatal("bdt decoded text has no hiragana")
	}
	enc, err := EncodeCP932(text)
	if err != nil {
		t.Fatalf("EncodeCP932(bdt text) error: %v", err)
	}
	if !bytes.Equal(enc, xored) {
		t.Fatal("bdt text does not byte-round-trip through cp932")
	}
}

func TestDetectRealUTF16Samples(t *testing.T) {
	dir := samplesDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "BSXScript", "*", "*.dat"))
	if err != nil || len(files) == 0 {
		t.Skip("bsxscript samples missing")
	}
	hits := 0
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for off := 0; off+64 <= len(raw); off += 2 {
			if looksUTF16LE(raw[off : off+64]) {
				got := DecodeUTF16LE(raw[off : off+64])
				kana := 0
				for _, r := range got {
					if (r >= 0x3040 && r <= 0x30FF) || (r >= 0x4E00 && r <= 0x9FFF) {
						kana++
					}
				}
				if kana > 4 {
					hits++
					break
				}
			}
		}
	}
	if hits == 0 {
		t.Fatal("no utf16le window found in bsxscript samples")
	}
	t.Logf("utf16le windows in %d files", hits)
}

func TestDispatch(t *testing.T) {
	if UnitSize("utf16le") != 2 {
		t.Fatal("UnitSize utf16le")
	}
	if UnitSize("UTF-16LE") != 2 {
		t.Fatal("UnitSize UTF-16LE")
	}
	for _, e := range []string{"utf8", "UTF-8", "cp932", "gbk", "GBK", "nope", ""} {
		if UnitSize(e) != 1 && e != "utf16le" && e != "UTF-16LE" {
			t.Fatalf("UnitSize(%q)", e)
		}
	}
	if got := Decode("utf16le", mustHex(t, "42304100")); got != "あA" {
		t.Fatalf("Decode utf16le = %q", got)
	}
	if got := Decode("CP932", mustHex(t, "82a0")); got != "あ" {
		t.Fatalf("Decode CP932 = %q", got)
	}
	if got := Decode("gbk", mustHex(t, "d6d0")); got != "中" {
		t.Fatalf("Decode gbk = %q", got)
	}
	if got := Decode("utf8", []byte("中")); got != "中" {
		t.Fatalf("Decode utf8 = %q", got)
	}
	if got := Decode("unknown-encoding", []byte("ABC")); got != "ABC" {
		t.Fatalf("Decode unknown = %q", got)
	}
	for _, e := range []string{"utf8", "utf16le", "cp932", "gbk"} {
		s := "あA"
		if e == "gbk" {
			s = "中A"
		}
		enc, err := Encode(e, s)
		if err != nil {
			t.Fatalf("Encode(%s) error: %v", e, err)
		}
		if got := Decode(e, enc); got != s {
			t.Fatalf("Decode(%s, Encode(%s)) = %q", e, e, got)
		}
	}
	if enc, err := Encode("", "abc"); err != nil || string(enc) != "abc" {
		t.Fatalf("Encode empty = %q %v", enc, err)
	}
	if _, err := Encode("unknown-encoding", "abc"); err == nil {
		t.Fatal("Encode unknown should fail")
	}
	if _, err := Encode("cp932", ""); err != nil {
		t.Fatalf("Encode cp932 empty error: %v", err)
	}
}

func TestTableChecksums(t *testing.T) {
	const offset uint64 = 14695981039346656037
	const prime uint64 = 1099511628211
	hash := func(h uint64, b byte) uint64 {
		h ^= uint64(b)
		return h * prime
	}
	feed := func(h uint64, r rune) uint64 {
		u := uint32(r)
		for k := 0; k < 4; k++ {
			h = hash(h, byte(u>>(8*k)))
		}
		return h
	}
	h := offset
	for lead := 0x81; lead <= 0xFC; lead++ {
		if !((lead <= 0x9F) || (lead >= 0xE0)) {
			continue
		}
		for trail := 0x40; trail <= 0xFC; trail++ {
			if !((trail <= 0x7E) || (trail >= 0x80)) {
				continue
			}
			r, ok := tables.Cp932Lookup(byte(lead), byte(trail))
			if !ok {
				h = feed(h, -1)
				continue
			}
			h = feed(h, r)
		}
	}
	for b := 0; b < 256; b++ {
		if r, ok := tables.Cp932Single(byte(b)); ok {
			h = hash(h, byte(b))
			h = feed(h, r)
		}
	}
	if h != tables.Cp932Checksum {
		t.Fatalf("cp932 checksum %016X != %016X", h, tables.Cp932Checksum)
	}
	h = offset
	for lead := 0x81; lead <= 0xFE; lead++ {
		for trail := 0x40; trail <= 0xFE; trail++ {
			if !((trail <= 0x7E) || (trail >= 0x80)) {
				continue
			}
			r, ok := tables.GbkLookup(byte(lead), byte(trail))
			if !ok {
				h = feed(h, -1)
				continue
			}
			h = feed(h, r)
		}
	}
	for b := 0; b < 256; b++ {
		if r, ok := tables.GbkSingle(byte(b)); ok {
			h = hash(h, byte(b))
			h = feed(h, r)
		}
	}
	if h != tables.GbkChecksum {
		t.Fatalf("gbk checksum %016X != %016X", h, tables.GbkChecksum)
	}
}
