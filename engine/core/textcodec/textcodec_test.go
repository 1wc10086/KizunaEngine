package textcodec

import (
	"testing"

	"kizuna/engine/core/textcodec/tables"
)

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

func TestDecodeCP932(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"konnichiha", []byte{0x82, 0xB1, 0x82, 0xF1, 0x82, 0xC9, 0x82, 0xBF, 0x82, 0xCD}, "こんにちは"},
		{"ascii", []byte{0x41, 0x42, 0x43}, "ABC"},
		{"halfwidth", []byte{0xB1, 0xB2}, "ｱｲ"},
		{"single80", []byte{0x80}, "\u0080"},
		{"singleA0", []byte{0xA0}, "\uF8F0"},
		{"singleFD", []byte{0xFD}, "\uF8F1"},
		{"singleFE", []byte{0xFE}, "\uF8F2"},
		{"singleFF", []byte{0xFF}, "\uF8F3"},
		{"truncatedLead", []byte{0x81}, "\uFFFD"},
		{"invalidTrail", []byte{0x81, 0x20, 0x41}, "\uFFFD A"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := DecodeCP932(c.in); got != c.want {
			t.Fatalf("%s: DecodeCP932 = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecodeGBK(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"euro", []byte{0x80}, "\u20AC"},
		{"zhongwen", []byte{0xD6, 0xD0, 0xCE, 0xC4}, "中文"},
		{"ascii", []byte{0x41, 0x42}, "AB"},
		{"truncatedLead", []byte{0x81}, "\uFFFD"},
		{"invalidTrail", []byte{0x81, 0x20}, "\uFFFD "},
		{"badByte", []byte{0xFF}, "\uFFFD"},
	}
	for _, c := range cases {
		if got := DecodeGBK(c.in); got != c.want {
			t.Fatalf("%s: DecodeGBK = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecodeUTF16LE(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"ascii", []byte{0x41, 0x00, 0x42, 0x00}, "AB"},
		{"surrogate", []byte{0x3D, 0xD8, 0x00, 0xDE}, "\U0001F600"},
		{"loneHigh", []byte{0x00, 0xD8}, "\uFFFD"},
		{"loneLow", []byte{0x00, 0xDC}, "\uFFFD"},
		{"oddLength", []byte{0x41, 0x00, 0x42}, "A\uFFFD"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		if got := DecodeUTF16LE(c.in); got != c.want {
			t.Fatalf("%s: DecodeUTF16LE = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecodeText(t *testing.T) {
	if got := DecodeText([]byte{0x82, 0xB1}, Cp932); got != "こ" {
		t.Fatalf("DecodeText cp932 = %q", got)
	}
	if got := DecodeText([]byte{0xD6, 0xD0}, Gbk); got != "中" {
		t.Fatalf("DecodeText gbk = %q", got)
	}
	if got := DecodeText([]byte{0x41, 0x00}, Utf16); got != "A" {
		t.Fatalf("DecodeText utf16 = %q", got)
	}
	if got := DecodeText([]byte("ok"), Utf8); got != "ok" {
		t.Fatalf("DecodeText utf8 = %q", got)
	}
}

func TestDecodeLooseUtf8(t *testing.T) {
	in := []byte{0x41, 0xE3, 0x81, 0x82, 0xFF, 0x42}
	if got := DecodeLooseUtf8(in); got != "Aあ\uFFFDB" {
		t.Fatalf("DecodeLooseUtf8 = %q", got)
	}
	if got := DecodeLooseUtf8([]byte("hello")); got != "hello" {
		t.Fatalf("DecodeLooseUtf8 ascii = %q", got)
	}
}

func TestReadStringAndTerminator(t *testing.T) {
	block := []byte{0x82, 0xB1, 0x82, 0xF1, 0x00, 0x41}
	if got := ReadString(block, 0, Cp932); got != "こん" {
		t.Fatalf("ReadString = %q", got)
	}
	if got := FindTerminator(block, 0, Cp932); got != 4 {
		t.Fatalf("FindTerminator cp932 = %d", got)
	}
	wide := []byte{0x41, 0x00, 0x42, 0x00, 0x00, 0x00, 0x43, 0x00}
	if got := FindTerminator(wide, 0, Utf16); got != 4 {
		t.Fatalf("FindTerminator utf16 = %d", got)
	}
	if got := ReadString(wide, 0, Utf16); got != "AB" {
		t.Fatalf("ReadString utf16 = %q", got)
	}
	if got := ReadString(block, 99, Cp932); got != "" {
		t.Fatalf("ReadString out of range = %q", got)
	}
	if got := ReadString(block, -1, Cp932); got != "" {
		t.Fatalf("ReadString negative = %q", got)
	}
}

func TestDetectTextEncoding(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want Encoding
	}{
		{"utf8", []byte("こんにちは"), Utf8},
		{"cp932", []byte{0x82, 0xB1, 0x82, 0xF1, 0x82, 0xC9, 0x82, 0xBF, 0x82, 0xCD}, Cp932},
		{"gbk", []byte{0xD6, 0xD0, 0xCE, 0xC4}, Gbk},
		{"ascii", []byte("abc"), Cp932},
	}
	for _, c := range cases {
		if got := DetectTextEncoding(c.in); got != c.want {
			t.Fatalf("%s: DetectTextEncoding = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTextUnitSize(t *testing.T) {
	if TextUnitSize(Utf16) != 2 {
		t.Fatal("utf16 unit size")
	}
	if TextUnitSize(Utf8) != 1 || TextUnitSize(Gbk) != 1 || TextUnitSize(Cp932) != 1 {
		t.Fatal("single byte unit size")
	}
}

func TestCanonicalName(t *testing.T) {
	cases := []struct {
		in   string
		want Encoding
	}{
		{"utf16", Utf16},
		{"UTF-16LE", Utf16},
		{"utf_16_le", Utf16},
		{"utf 16 le", Utf16},
		{"gbk", Gbk},
		{"GB2312", Gbk},
		{"gb18030", Gbk},
		{"cp932", Cp932},
		{"Shift-JIS", Cp932},
		{"SJIS", Cp932},
		{"MS932", Cp932},
		{"windows-31j", Cp932},
		{"utf8", Utf8},
		{"latin1", Utf8},
	}
	for _, c := range cases {
		if got := CanonicalName(c.in); got != c.want {
			t.Fatalf("CanonicalName(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestDecodeEncodeByName(t *testing.T) {
	if got := Decode("sjis", []byte{0x82, 0xB1}); got != "こ" {
		t.Fatalf("Decode sjis = %q", got)
	}
	if got := Decode("gb18030", []byte{0xD6, 0xD0}); got != "中" {
		t.Fatalf("Decode gb18030 = %q", got)
	}
	if got := Decode("utf-16le", []byte{0x41, 0x00}); got != "A" {
		t.Fatalf("Decode utf-16le = %q", got)
	}
	if got := Decode("unknown", []byte("hi")); got != "hi" {
		t.Fatalf("Decode unknown = %q", got)
	}
	if got := Decode("utf8", []byte("hi")); got != "hi" {
		t.Fatalf("Decode utf8 = %q", got)
	}
	enc, err := Encode("cp932", "こんにちは")
	if err != nil {
		t.Fatalf("Encode cp932 error: %v", err)
	}
	if want := []byte{0x82, 0xB1, 0x82, 0xF1, 0x82, 0xC9, 0x82, 0xBF, 0x82, 0xCD}; string(enc) != string(want) {
		t.Fatalf("Encode cp932 = % X", enc)
	}
	enc, err = Encode("gbk", "中文")
	if err != nil {
		t.Fatalf("Encode gbk error: %v", err)
	}
	if want := []byte{0xD6, 0xD0, 0xCE, 0xC4}; string(enc) != string(want) {
		t.Fatalf("Encode gbk = % X", enc)
	}
	enc, err = Encode("utf-16le", "A\U0001F600")
	if err != nil {
		t.Fatalf("Encode utf16 error: %v", err)
	}
	if want := []byte{0x41, 0x00, 0x3D, 0xD8, 0x00, 0xDE}; string(enc) != string(want) {
		t.Fatalf("Encode utf16 = % X", enc)
	}
	enc, err = Encode("utf8", "plain")
	if err != nil || string(enc) != "plain" {
		t.Fatalf("Encode utf8 = %q %v", enc, err)
	}
	if _, err := EncodeCP932("\U0001F600"); err == nil {
		t.Fatal("EncodeCP932 emoji should fail")
	}
	if _, err := EncodeGBK("\U0001F600"); err == nil {
		t.Fatal("EncodeGBK emoji should fail")
	}
}

func TestEncodeCP932Halfwidth(t *testing.T) {
	enc, err := EncodeCP932("ｱｲ")
	if err != nil {
		t.Fatalf("EncodeCP932 halfwidth error: %v", err)
	}
	if want := []byte{0xB1, 0xB2}; string(enc) != string(want) {
		t.Fatalf("EncodeCP932 halfwidth = % X", enc)
	}
}

func TestPredicates(t *testing.T) {
	if !IsCP932Lead(0x81) || !IsCP932Lead(0x9F) || !IsCP932Lead(0xE0) || !IsCP932Lead(0xFC) {
		t.Fatal("cp932 lead expected true")
	}
	if IsCP932Lead(0x80) || IsCP932Lead(0xA0) || IsCP932Lead(0xFD) {
		t.Fatal("cp932 lead expected false")
	}
	if !IsCP932Trail(0x40) || !IsCP932Trail(0x7E) || !IsCP932Trail(0x80) || !IsCP932Trail(0xFC) {
		t.Fatal("cp932 trail expected true")
	}
	if IsCP932Trail(0x3F) || IsCP932Trail(0x7F) || IsCP932Trail(0xFD) {
		t.Fatal("cp932 trail expected false")
	}
	if !IsGBKLead(0x81) || !IsGBKLead(0xFE) || IsGBKLead(0x80) || IsGBKLead(0xFF) {
		t.Fatal("gbk lead mismatch")
	}
	if !IsGBKTrail(0x40) || !IsGBKTrail(0x7E) || !IsGBKTrail(0x80) || !IsGBKTrail(0xFE) {
		t.Fatal("gbk trail expected true")
	}
	if IsGBKTrail(0x3F) || IsGBKTrail(0x7F) || IsGBKTrail(0xFF) {
		t.Fatal("gbk trail expected false")
	}
}

func TestScoreAndBlockHelpers(t *testing.T) {
	cp := []byte{0x82, 0xB1, 0x82, 0xF1}
	if ScoreCP932(cp) <= ScoreGBK(cp) {
		t.Fatal("cp932 should outscore gbk")
	}
	gb := []byte{0xD6, 0xD0, 0xCE, 0xC4}
	if ScoreGBK(gb) <= ScoreCP932(gb) {
		t.Fatal("gbk should outscore cp932")
	}
	if !IsUtf8Block([]byte("こんにちは")) {
		t.Fatal("utf8 block expected true")
	}
	if IsUtf8Block([]byte{0x82, 0xB1}) {
		t.Fatal("cp932 block expected false")
	}
	if !HasHighByte([]byte{0x41, 0x82}) || HasHighByte([]byte("abc")) {
		t.Fatal("HasHighByte mismatch")
	}
	if !IsStrictUtf8([]byte("こ")) || IsStrictUtf8([]byte{0x82, 0xB1}) {
		t.Fatal("IsStrictUtf8 mismatch")
	}
}
