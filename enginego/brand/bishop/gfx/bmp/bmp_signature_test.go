package bmp

import (
	"encoding/binary"
	"testing"
)

func TestIsBmpSignatureTolerance(t *testing.T) {
	cases := []struct {
		name   string
		first  byte
		second byte
		want   bool
	}{
		{"normal", 'B', 'M', true},
		{"zero first byte", 0, 'M', true},
		{"wrong second byte", 'B', 'X', false},
		{"zero second byte", 'B', 0, false},
		{"both zero", 0, 0, false},
		{"reversed", 'M', 'B', false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, BmpFileHeaderSize)
			data[0] = tc.first
			data[1] = tc.second
			if got := IsBmp(data); got != tc.want {
				t.Fatalf("IsBmp(%q,%q) = %v, want %v", tc.first, tc.second, got, tc.want)
			}
		})
	}
}

func TestIsBmpRejectsShortData(t *testing.T) {
	data := make([]byte, BmpFileHeaderSize-1)
	data[0], data[1] = 'B', 'M'
	if IsBmp(data) {
		t.Fatal("data shorter than BmpFileHeaderSize must not match")
	}
}

func buildBmp24x1() []byte {
	data := make([]byte, 58)
	copy(data[0:2], "BM")
	binary.LittleEndian.PutUint32(data[2:], 58)
	binary.LittleEndian.PutUint32(data[10:], 54)
	binary.LittleEndian.PutUint32(data[14:], 40)
	binary.LittleEndian.PutUint32(data[18:], 1)
	binary.LittleEndian.PutUint32(data[22:], 1)
	binary.LittleEndian.PutUint16(data[26:], 1)
	binary.LittleEndian.PutUint16(data[28:], 24)
	return data
}

func TestParseBmpHeaderAcceptsZeroFirstByte(t *testing.T) {
	data := buildBmp24x1()
	if _, err := ParseBmpHeader(data); err != nil {
		t.Fatalf("valid BMP rejected: %v", err)
	}

	data[0] = 0
	h, err := ParseBmpHeader(data)
	if err != nil {
		t.Fatalf("BMP with zero first byte rejected: %v", err)
	}
	if h.Width != 1 || h.Height != 1 {
		t.Fatalf("size = %dx%d, want 1x1", h.Width, h.Height)
	}
}

func TestParseBmpHeaderRejectsWrongSecondByte(t *testing.T) {
	data := buildBmp24x1()
	data[1] = 'X'
	if _, err := ParseBmpHeader(data); err == nil {
		t.Fatal("BMP with wrong second byte must be rejected")
	}
}
