package core

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"runtime"
	"strings"
	"testing"
)

// totalAlloc 返回 f 期间累计分配的字节数（TotalAlloc 单调递增，不受回收影响）。
func totalAlloc(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func zlibBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

func flateBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}
	return buf.Bytes()
}

func TestInflateExactValidStreams(t *testing.T) {
	payload := []byte("kizuna engine inflate sample sample sample sample")
	cases := []struct {
		name string
		data []byte
	}{
		{"flate", flateBytes(t, payload)},
		{"zlib", zlibBytes(t, payload)},
	}
	for _, c := range cases {
		got, err := InflateExact(c.data, len(payload), c.name)
		if err != nil {
			t.Fatalf("%s: InflateExact: %v", c.name, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("%s: InflateExact = %q, want %q", c.name, got, payload)
		}
	}
}

func TestInflateExactWrongSize(t *testing.T) {
	payload := []byte("kizuna engine inflate sample sample sample sample")
	data := flateBytes(t, payload)
	_, err := InflateExact(data, len(payload)+1, "flab")
	if err == nil {
		t.Fatalf("InflateExact accepted a mismatched expected size")
	}
	if !strings.Contains(err.Error(), "flab") {
		t.Fatalf("InflateExact error %q does not contain the label", err.Error())
	}
}

func TestInflateExactInvalidInput(t *testing.T) {
	for _, data := range [][]byte{nil, {}, {0x00}} {
		if _, err := InflateExact(data, 4, "bad"); err == nil {
			t.Fatalf("InflateExact(%v, 4) returned no error", data)
		}
	}
	if _, err := InflateExact(nil, 0, "nil0"); err == nil {
		t.Fatalf("InflateExact(nil, 0) returned no error")
	}
}

func TestInflateExactEmptyStream(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"flate", flateBytes(t, nil)},
		{"zlib", zlibBytes(t, nil)},
	}
	for _, c := range cases {
		got, err := InflateExact(c.data, 0, c.name)
		if err != nil {
			t.Fatalf("%s: InflateExact empty stream: %v", c.name, err)
		}
		if len(got) != 0 {
			t.Fatalf("%s: InflateExact empty stream = %q, want empty", c.name, got)
		}
	}
}
