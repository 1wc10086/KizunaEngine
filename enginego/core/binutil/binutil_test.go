package binutil

import (
	"errors"
	"io"
	"testing"
)

func TestReaderEmpty(t *testing.T) {
	r := NewReader(nil)
	if got := r.U8(); got != 0 {
		t.Fatalf("U8 = %d, want 0", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}
	if r.Remaining() != 0 {
		t.Fatalf("Remaining = %d, want 0", r.Remaining())
	}
	if r.Pos() != 0 {
		t.Fatalf("Pos = %d, want 0", r.Pos())
	}
}

func TestReaderExactSize(t *testing.T) {
	r := NewReader([]byte{0x34, 0x12})
	if got := r.U16(); got != 0x1234 {
		t.Fatalf("U16 = %#x, want 0x1234", got)
	}
	if r.Err() != nil {
		t.Fatalf("Err = %v, want nil", r.Err())
	}
	if r.Remaining() != 0 {
		t.Fatalf("Remaining = %d, want 0", r.Remaining())
	}

	r = NewReader([]byte{0x78, 0x56, 0x34, 0x12})
	if got := r.U32(); got != 0x12345678 {
		t.Fatalf("U32 = %#x, want 0x12345678", got)
	}
	if got := r.I32(); got != 0 {
		t.Fatalf("I32 = %d, want 0", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}

	r = NewReader([]byte{0x01, 0x7F, 0x00, 0x00})
	if got := r.I32(); got != 0x7F01 {
		t.Fatalf("I32 = %#x, want 0x7F01", got)
	}
	if r.Err() != nil {
		t.Fatalf("Err = %v, want nil", r.Err())
	}

	r = NewReader([]byte{0x00, 0x00, 0x00, 0x80})
	if got := r.I32(); got != -2147483648 {
		t.Fatalf("I32 = %d, want -2147483648", got)
	}
}

func TestReaderOneByteShort(t *testing.T) {
	r := NewReader([]byte{0x01})
	if got := r.U16(); got != 0 {
		t.Fatalf("U16 = %d, want 0", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}
	if r.Pos() != 0 {
		t.Fatalf("Pos = %d, want 0", r.Pos())
	}

	r = NewReader([]byte{0x01, 0x02, 0x03})
	if got := r.Bytes(4); got != nil {
		t.Fatalf("Bytes = %v, want nil", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}
	if r.Remaining() != 3 {
		t.Fatalf("Remaining = %d, want 3", r.Remaining())
	}

	r = NewReader([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	r.SetPos(4)
	if got := r.Bytes(4); got != nil {
		t.Fatalf("Bytes = %v, want nil", got)
	}
}

func TestReaderBytes(t *testing.T) {
	data := []byte{0x0A, 0x0B, 0x0C, 0x0D}
	r := NewReader(data)
	got := r.Bytes(2)
	if len(got) != 2 || got[0] != 0x0A || got[1] != 0x0B {
		t.Fatalf("Bytes = %v", got)
	}
	got[0] = 0xFF
	if data[0] != 0x0A {
		t.Fatalf("Bytes did not copy: %v", data)
	}
	if r.Pos() != 2 {
		t.Fatalf("Pos = %d, want 2", r.Pos())
	}
	if empty := r.Bytes(0); empty == nil || len(empty) != 0 {
		t.Fatalf("Bytes(0) = %v", empty)
	}
}

func TestReaderSkip(t *testing.T) {
	r := NewReader([]byte{0x01, 0x02, 0x03})
	r.Skip(2)
	if r.Pos() != 2 {
		t.Fatalf("Pos = %d, want 2", r.Pos())
	}
	if got := r.U8(); got != 0x03 {
		t.Fatalf("U8 = %#x, want 0x03", got)
	}
	r.Skip(1)
	if r.Err() == nil {
		t.Fatalf("Err = nil, want error")
	}
}

func TestReaderPositionOverflow(t *testing.T) {
	r := NewReader([]byte{0x01, 0x02, 0x03})
	r.SetPos(10)
	if r.Remaining() != 0 {
		t.Fatalf("Remaining = %d, want 0", r.Remaining())
	}
	if got := r.U8(); got != 0 {
		t.Fatalf("U8 = %d, want 0", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}
	if r.Pos() != 10 {
		t.Fatalf("Pos = %d, want 10", r.Pos())
	}
}

func TestReaderLatchedError(t *testing.T) {
	r := NewReader([]byte{0x01})
	first := errors.New("boom")
	r.Fail(first)
	r.Fail(errors.New("second"))
	if !errors.Is(r.Err(), first) {
		t.Fatalf("Err = %v, want first", r.Err())
	}
	if got := r.U8(); got != 0 {
		t.Fatalf("U8 = %d, want 0", got)
	}
	if got := r.U32(); got != 0 {
		t.Fatalf("U32 = %d, want 0", got)
	}
	if r.Pos() != 0 {
		t.Fatalf("Pos = %d, want 0", r.Pos())
	}
}

func TestSevenBitInt(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want int
	}{
		{"zero", []byte{0x00}, 0},
		{"127", []byte{0x7F}, 127},
		{"128", []byte{0x80, 0x01}, 128},
		{"0x3FFF", []byte{0xFF, 0x7F}, 0x3FFF},
		{"0x40000000", []byte{0x80, 0x80, 0x80, 0x80, 0x04}, 0x40000000},
		{"0xFFFFFFFF", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, 0xFFFFFFFF},
		{"five byte maximum", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x7F}, 0x7FFFFFFFF},
	}
	for _, test := range cases {
		r := NewReader(test.data)
		if got := r.SevenBitInt(); got != test.want {
			t.Fatalf("%s: SevenBitInt = %d, want %d", test.name, got, test.want)
		}
		if r.Pos() != len(test.data) {
			t.Fatalf("%s: Pos = %d, want %d", test.name, r.Pos(), len(test.data))
		}
		if r.Err() != nil {
			t.Fatalf("%s: Err = %v, want nil", test.name, r.Err())
		}
	}
}

func TestSevenBitIntTruncated(t *testing.T) {
	cases := []struct {
		data []byte
		want int
	}{
		{[]byte{0x80}, 0},
		{[]byte{0x80, 0x80}, 0},
		{[]byte{0xFF, 0xFF}, 0x3FFF},
		{nil, 0},
	}
	for _, test := range cases {
		r := NewReader(test.data)
		if got := r.SevenBitInt(); got != test.want {
			t.Fatalf("%v: SevenBitInt = %d, want %d", test.data, got, test.want)
		}
		if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
			t.Fatalf("%v: Err = %v, want ErrUnexpectedEOF", test.data, r.Err())
		}
	}
}

func TestSevenBitIntOverlong(t *testing.T) {
	r := NewReader([]byte{0x80, 0x80, 0x80, 0x80, 0x80})
	if got := r.SevenBitInt(); got != 0 {
		t.Fatalf("SevenBitInt = %d, want 0", got)
	}
	if r.Err() == nil {
		t.Fatalf("Err = nil, want error")
	}
	if r.Pos() != 5 {
		t.Fatalf("Pos = %d, want 5", r.Pos())
	}
}

func TestZeroTerminatedString(t *testing.T) {
	r := NewReader([]byte{0x68, 0x69, 0x00, 0x21})
	if got := r.ZeroTerminatedString(); got != "hi" {
		t.Fatalf("ZeroTerminatedString = %q, want %q", got, "hi")
	}
	if got := r.U8(); got != 0x21 {
		t.Fatalf("U8 = %#x, want 0x21", got)
	}
	if r.Err() != nil {
		t.Fatalf("Err = %v, want nil", r.Err())
	}

	r = NewReader([]byte{0x61, 0x62})
	if got := r.ZeroTerminatedString(); got != "" {
		t.Fatalf("ZeroTerminatedString = %q, want empty", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}

	r = NewReader([]byte{0x00})
	if got := r.ZeroTerminatedString(); got != "" {
		t.Fatalf("ZeroTerminatedString = %q, want empty", got)
	}
	if r.Pos() != 1 {
		t.Fatalf("Pos = %d, want 1", r.Pos())
	}
}

func TestUTF16LEString(t *testing.T) {
	r := NewReader([]byte{0x48, 0x00, 0x69, 0x00})
	if got := r.UTF16LEString(2); got != "Hi" {
		t.Fatalf("UTF16LEString = %q, want %q", got, "Hi")
	}
	if r.Err() != nil {
		t.Fatalf("Err = %v, want nil", r.Err())
	}
	if r.Remaining() != 0 {
		t.Fatalf("Remaining = %d, want 0", r.Remaining())
	}

	r = NewReader([]byte{0x48, 0x00, 0x69})
	if got := r.UTF16LEString(2); got != "" {
		t.Fatalf("UTF16LEString = %q, want empty", got)
	}
	if !errors.Is(r.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("Err = %v, want ErrUnexpectedEOF", r.Err())
	}

	r = NewReader([]byte{0x48, 0x00})
	if got := r.UTF16LEString(0); got != "" {
		t.Fatalf("UTF16LEString = %q, want empty", got)
	}
	if r.Err() != nil {
		t.Fatalf("Err = %v, want nil", r.Err())
	}

	r = NewReader([]byte{0x00})
	if got := r.UTF16LEString(-1); got != "" {
		t.Fatalf("UTF16LEString = %q, want empty", got)
	}
	if r.Err() == nil {
		t.Fatalf("Err = nil, want error")
	}
}

func TestUTF16LEStringSurrogatePair(t *testing.T) {
	r := NewReader([]byte{0x3D, 0xD8, 0x00, 0xDE})
	if got := r.UTF16LEString(2); got != "\U0001F600" {
		t.Fatalf("UTF16LEString = %q, want emoji", got)
	}
}

func TestNegateBytes(t *testing.T) {
	data := []byte{0x00, 0xFF, 0x0F}
	NegateBytes(data)
	want := []byte{0xFF, 0x00, 0xF0}
	for i := range want {
		if data[i] != want[i] {
			t.Fatalf("NegateBytes = % v, want % v", data, want)
		}
	}
	NegateBytes(data)
	if data[0] != 0x00 || data[1] != 0xFF || data[2] != 0x0F {
		t.Fatalf("NegateBytes not involutive: % v", data)
	}
	empty := []byte{}
	NegateBytes(empty)
	if len(empty) != 0 {
		t.Fatalf("NegateBytes empty = % v", empty)
	}
}

func TestSumBytes(t *testing.T) {
	if got := SumBytes(nil); got != 0 {
		t.Fatalf("SumBytes(nil) = %d, want 0", got)
	}
	if got := SumBytes([]byte{0x00, 0xFF}); got != 255 {
		t.Fatalf("SumBytes = %d, want 255", got)
	}
	if got := SumBytes([]byte{0xFF, 0xFF}); got != 0 {
		t.Fatalf("SumBytes = %d, want 0", got)
	}
}

func TestScalarHelpers(t *testing.T) {
	if got := U16([]byte{0x34, 0x12}); got != 0x1234 {
		t.Fatalf("U16 = %#x, want 0x1234", got)
	}
	if got := U32([]byte{0x78, 0x56, 0x34, 0x12}); got != 0x12345678 {
		t.Fatalf("U32 = %#x, want 0x12345678", got)
	}
	if got := I32([]byte{0xFE, 0xFF, 0xFF, 0xFF}); got != -2 {
		t.Fatalf("I32 = %d, want -2", got)
	}
	if got := U64([]byte{0xEF, 0xCD, 0xAB, 0x89, 0x67, 0x45, 0x23, 0x01}); got != 0x0123456789ABCDEF {
		t.Fatalf("U64 = %#x, want 0x0123456789ABCDEF", got)
	}
}

func TestPutRoundTrips(t *testing.T) {
	buf := make([]byte, 8)
	PutU16(buf, 0xABCD)
	if U16(buf) != 0xABCD {
		t.Fatalf("U16 = %#x, want 0xABCD", U16(buf))
	}
	PutU32(buf, 0xDEADBEEF)
	if U32(buf) != 0xDEADBEEF {
		t.Fatalf("U32 = %#x, want 0xDEADBEEF", U32(buf))
	}
	PutI32(buf, -1234567)
	if I32(buf) != -1234567 {
		t.Fatalf("I32 = %d, want -1234567", I32(buf))
	}

	PutI32(buf, -1)
	r := NewReader(buf)
	if got := r.I32(); got != -1 {
		t.Fatalf("Reader.I32 = %d, want -1", got)
	}
	PutU32(buf, 0x01020304)
	r.SetPos(0)
	if got := r.U32(); got != 0x01020304 {
		t.Fatalf("Reader.U32 = %#x, want 0x01020304", got)
	}
	PutU16(buf, 0x0203)
	r.SetPos(0)
	if got := r.U16(); got != 0x0203 {
		t.Fatalf("Reader.U16 = %#x, want 0x0203", got)
	}
}

func TestBitReaderOrder(t *testing.T) {
	want := []int{1, 0, 1, 1, 0, 1, 0, 1, 0, 0, 1, 1, 1, 0, 1, 0}
	b := NewBitReader([]byte{0xB5, 0x3A})
	for i, expected := range want {
		got, ok := b.ReadBit()
		if !ok {
			t.Fatalf("bit %d: ok = false", i)
		}
		if got != expected {
			t.Fatalf("bit %d = %d, want %d", i, got, expected)
		}
	}
	if _, ok := b.ReadBit(); ok {
		t.Fatalf("exhausted ReadBit ok = true")
	}

	b = NewBitReader([]byte{0xB5, 0x3A})
	got, ok := b.ReadBits(16)
	if !ok || got != 0xB53A {
		t.Fatalf("ReadBits(16) = %#x, %v, want 0xB53A", got, ok)
	}
	if _, ok := b.ReadBits(1); ok {
		t.Fatalf("exhausted ReadBits ok = true")
	}
}

func TestBitReaderPartial(t *testing.T) {
	b := NewBitReader([]byte{0x01})
	if got, ok := b.ReadBits(4); !ok || got != 0 {
		t.Fatalf("ReadBits(4) = %d, %v, want 0, true", got, ok)
	}
	if got, ok := b.ReadBits(5); ok || got != 0 {
		t.Fatalf("ReadBits(5) = %d, %v, want 0, false", got, ok)
	}
	if got, ok := b.ReadBits(0); !ok || got != 0 {
		t.Fatalf("ReadBits(0) = %d, %v, want 0, true", got, ok)
	}
}

func TestBitReaderEmpty(t *testing.T) {
	b := NewBitReader(nil)
	if got, ok := b.ReadBit(); ok || got != 0 {
		t.Fatalf("ReadBit = %d, %v, want 0, false", got, ok)
	}
	if got, ok := b.ReadBits(1); ok || got != 0 {
		t.Fatalf("ReadBits(1) = %d, %v, want 0, false", got, ok)
	}
	b = NewBitReader([]byte{0xFF})
	if got, ok := b.ReadBits(-1); ok || got != 0 {
		t.Fatalf("ReadBits(-1) = %d, %v, want 0, false", got, ok)
	}
}
