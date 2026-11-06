package core

import (
	"bytes"
	"strings"
	"testing"
)

func bigPayload(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		// 内容是任意的：解压路径只关心长度，不关心字节是什么。
		p[i] = byte(i*31 + i>>7)
	}
	return p
}

// TestInflateExactAllocatesOnce 钉住"按 expected 精确分配一次"这条性质：旧实现走
// io.ReadAll，ReadAll 靠 append 倍增扩容，解 4MB 的输出会额外分配（并拷贝）好几倍于
// 结果的临时缓冲——低端机上的 GC 停顿就是这么来的。这里留 1.5 倍结果的余量，只有精确
// 分配才过得去。
func TestInflateExactAllocatesOnce(t *testing.T) {
	payload := bigPayload(4 << 20)
	data := zlibBytes(t, payload)
	var out []byte
	alloc := totalAlloc(func() {
		var err error
		out, err = InflateExact(data, len(payload), "alloc")
		if err != nil {
			t.Fatalf("InflateExact: %v", err)
		}
	})
	if len(out) != len(payload) {
		t.Fatalf("InflateExact returned %d bytes, want %d", len(out), len(payload))
	}
	if !bytes.Equal(out, payload) {
		t.Fatalf("InflateExact output differs from the payload")
	}
	if limit := uint64(len(payload)) * 3 / 2; alloc > limit {
		t.Fatalf("InflateExact allocated %d bytes for a %d byte result (limit %d)", alloc, len(payload), limit)
	}
}

// TestInflateExactIntoNoAlloc 钉住 YCG 用的那条路径：缓冲由调用方给，解压过程不该再分配
// 与结果同量级的内存（否则"直接解进像素缓冲"就白做了）。
func TestInflateExactIntoNoAlloc(t *testing.T) {
	payload := bigPayload(4 << 20)
	dst := make([]byte, len(payload))
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"flate", flateBytes(t, payload)},
		{"zlib", zlibBytes(t, payload)},
	} {
		for i := range dst {
			dst[i] = 0xEE
		}
		var err error
		alloc := totalAlloc(func() {
			err = InflateExactInto(c.data, dst, c.name)
		})
		if err != nil {
			t.Fatalf("%s: InflateExactInto: %v", c.name, err)
		}
		if !bytes.Equal(dst, payload) {
			t.Fatalf("%s: InflateExactInto output differs from the payload", c.name)
		}
		// 上限只留解码器本身的固定开销（flate/zlib reader 的霍夫曼表，两次尝试各一份，
		// 加起来几十 KB），相对 4MB 的结果可以忽略；一旦有人改回"解出中间切片再拷贝"，
		// 这里立刻会超。
		if limit := uint64(256 << 10); alloc > limit {
			t.Fatalf("%s: InflateExactInto allocated %d bytes into a caller buffer (limit %d)", c.name, alloc, limit)
		}
	}
}

// TestInflateExactIntoMatchesExact 保证两条入口逐位一致：InflateExact 就是
// "make + InflateExactInto"，YCG 换成后者不改变输出，报错文案也保持带 label。
func TestInflateExactIntoMatchesExact(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		enc     func(*testing.T, []byte) []byte
	}{
		{"flate", bigPayload(1 << 20), flateBytes},
		{"zlib", bigPayload(1 << 20), zlibBytes},
		{"flate-short", []byte("kizuna"), flateBytes},
		{"zlib-short", []byte("kizuna"), zlibBytes},
	}
	for _, c := range cases {
		data := c.enc(t, c.payload)
		got, err := InflateExact(data, len(c.payload), c.name)
		if err != nil {
			t.Fatalf("%s: InflateExact: %v", c.name, err)
		}
		into := make([]byte, len(c.payload))
		if err := InflateExactInto(data, into, c.name); err != nil {
			t.Fatalf("%s: InflateExactInto: %v", c.name, err)
		}
		if !bytes.Equal(got, c.payload) || !bytes.Equal(into, got) {
			t.Fatalf("%s: InflateExact/InflateExactInto disagree with the payload", c.name)
		}
	}
	// 报错路径同样要保持一致。
	payload := bigPayload(64)
	data := flateBytes(t, payload)
	for _, label := range []string{"flate", "zlib"} {
		if _, err := InflateExact(data, len(payload)+1, label); err == nil || !strings.Contains(err.Error(), label) {
			t.Fatalf("%s: size mismatch error = %v, want a labelled error", label, err)
		}
		if err := InflateExactInto(data, make([]byte, len(payload)+1), label); err == nil || !strings.Contains(err.Error(), label) {
			t.Fatalf("%s: size mismatch error from InflateExactInto = %v", label, err)
		}
		if _, err := InflateExact([]byte{1, 2, 3}, 64, label); err == nil {
			t.Fatalf("%s: InflateExact accepted garbage", label)
		}
		if err := InflateExactInto([]byte{1, 2, 3}, make([]byte, 64), label); err == nil {
			t.Fatalf("%s: InflateExactInto accepted garbage", label)
		}
	}
}
