package gfx

import "testing"

func signatureBuffer(size int, sig string) []byte {
	data := make([]byte, size)
	copy(data, sig)
	return data
}

func TestIsBsgGraphicsSignatureTolerance(t *testing.T) {
	cases := []struct {
		name string
		sig  string
		want bool
	}{
		{"normal", "BSS-Graphics", true},
		{"first byte zero", "\x00SS-Graphics", true},
		{"ninth byte zero", "BSS-Grap\x00ics", true},
		{"two zeros", "\x00SS-Grap\x00ics", true},
		{"three zeros", "\x00\x00S-Grap\x00ics", false},
		{"wrong non-zero byte", "BSS-Graphicz", false},
		{"wrong byte with zero", "\x00SS-Graphicz", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBsgGraphics(signatureBuffer(BsgHeaderSize, tc.sig), 0); got != tc.want {
				t.Fatalf("IsBsgGraphics(%q) = %v, want %v", tc.sig, got, tc.want)
			}
		})
	}
}

func TestIsBsgGraphicsNonZeroOffset(t *testing.T) {
	data := append(make([]byte, 4), signatureBuffer(BsgHeaderSize, "\x00SS-Graphics")...)
	if !IsBsgGraphics(data, 4) {
		t.Fatal("signature at offset 4 with zero first byte should match")
	}
	if IsBsgGraphics(data, -1) {
		t.Fatal("negative offset must not match")
	}
}

func TestIsBsgGraphicsRejectsShortData(t *testing.T) {
	if IsBsgGraphics(signatureBuffer(BsgHeaderSize-1, "BSS-Graphics"), 0) {
		t.Fatal("data shorter than BsgHeaderSize must not match")
	}
}

func TestIsBsgCompositionSignatureTolerance(t *testing.T) {
	cases := []struct {
		name string
		sig  string
		want bool
	}{
		{"normal", "BSS-Composition", true},
		{"first byte zero", "\x00SS-Composition", true},
		{"two zeros", "\x00SS-Com\x00osition", true},
		{"three zeros", "\x00\x00S-Com\x00osition", false},
		{"wrong non-zero byte", "BSS-Compositiom", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBsgComposition(signatureBuffer(BsgCompositionHeaderSize, tc.sig)); got != tc.want {
				t.Fatalf("IsBsgComposition(%q) = %v, want %v", tc.sig, got, tc.want)
			}
		})
	}
}

func TestIsBsgCompositionRejectsShortData(t *testing.T) {
	if IsBsgComposition(signatureBuffer(BsgCompositionHeaderSize-1, "BSS-Composition")) {
		t.Fatal("data shorter than BsgCompositionHeaderSize must not match")
	}
}

func TestMatchesTolerantSignatureEdges(t *testing.T) {
	data := []byte("BSS-Graphics")
	if matchesTolerantSignature(data, 0, nil) {
		t.Fatal("empty signature must not match")
	}
	if matchesTolerantSignature(data, 1, BsgGraphicsSignature) {
		t.Fatal("signature running past end of data must not match")
	}
}
