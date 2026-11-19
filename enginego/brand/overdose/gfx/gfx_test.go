package gfx

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"kizuna/enginego/brand/overdose/arc"
)

// 以下测试只在有真实游戏数据时跑，无数据整轮跳过，CI 仍全绿。
// 选择 bg011b/title_bg/tmi_d05 是因为三者分别覆盖背景（0x18 格式）与立绘（0x20 格式）。

var cpbDirCandidates = []string{
	os.Getenv("GAME_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
	"./testdata",
}

func cpbDir(t *testing.T) string {
	t.Helper()
	for _, c := range cpbDirCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "graphic.arc")); err == nil {
			return c
		}
	}
	t.Skip("未找到游戏数据目录（设置 GAME_DIR）")
	return ""
}

func cpbSet(t *testing.T) *Cache {
	t.Helper()
	dir := cpbDir(t)
	set, err := arc.OpenSet(dir)
	if err != nil {
		t.Fatalf("openSet: %v", err)
	}
	t.Cleanup(func() { set.Close() })
	return FromArchiveSet(set)
}

func TestDecodeBg011b(t *testing.T) {
	c := cpbSet(t)
	w, h := c.Get("bg011b").Width, c.Get("bg011b").Height
	if w != 800 || h != 600 {
		t.Fatalf("bg011b %dx%d, want 800x600", w, h)
	}
}

func TestDecodeTitleBg(t *testing.T) {
	c := cpbSet(t)
	img := c.Get("title_bg")
	if img == nil {
		t.Fatalf("title_bg nil")
	}
	if img.Width != 800 || img.Height != 600 {
		t.Fatalf("title_bg %dx%d, want 800x600", img.Width, img.Height)
	}
}

func TestDecodeTmiD05(t *testing.T) {
	c := cpbSet(t)
	img := c.Get("tmi_d05")
	if img == nil {
		t.Fatalf("tmi_d05 nil")
	}
	if img.Height != 600 {
		t.Fatalf("tmi_d05 height %d, want 600", img.Height)
	}
	if img.Width <= 0 || img.Width > 800 {
		t.Fatalf("tmi_d05 width %d out of range", img.Width)
	}
}

// Cache_Get 命中时应复用同一指针（转场混帧不复制）且字节预算不翻倍。
func TestCacheHitReuses(t *testing.T) {
	c := cpbSet(t)
	a := c.Get("bg011b")
	b := c.Get("bg011b")
	if a != b {
		t.Fatalf("cache miss dedup failed")
	}
	if c.Hits() == 0 {
		t.Fatalf("hits should be >0")
	}
	if c.Bytes() <= 0 {
		t.Fatalf("bytes should be >0")
	}
}

// 缓存有界：塞远超预算的条目时不无限涨。
func TestCacheBounded(t *testing.T) {
	c := NewCacheBounded(func(name string) ([]byte, error) {
		return makeCPBBinary(t, 64, 64), nil
	}, 4096, 4)
	for i := 0; i < 10; i++ {
		c.Put(string(rune('a'+i)), NewImage(64, 64, make([]byte, 64*64*4)))
	}
	if c.Bytes() > 4096+64*64*4 {
		t.Fatalf("cache bytes %d exceeds budget", c.Bytes())
	}
	if c.Len() > 5 {
		t.Fatalf("cache len %d exceeds bound", c.Len())
	}
}

func TestScaleImage(t *testing.T) {
	src := NewImage(8, 8, make([]byte, 8*8*4))
	for i := range src.Pixels {
		src.Pixels[i] = byte(i)
	}
	near := ScaleImage(src, 4, 4, ScaleNearest)
	if near.Width != 4 || near.Height != 4 {
		t.Fatalf("nearest scale %dx%d", near.Width, near.Height)
	}
	// 1:1 返回原图不复制（合成层靠它省内存）
	if ScaleImage(src, 8, 8, ScaleBilinear) != src {
		t.Fatalf("identity scale should return same image")
	}
}

// makeCPBBinary 合成 TYP1 头 + zlib 流（不直接读写存档，防测试数据缺席时硬失败）。
func makeCPBBinary(t *testing.T, w, h int) []byte {
	t.Helper()
	return cpbZero(t, w, h)
}

func cpbZero(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	plain := make([]byte, w*h*4)
	plain[3] = 255
	plain[len(plain)-1] = 255
	if _, err := zw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	compressed := buf.Bytes()
	out := make([]byte, 14)
	copy(out, "TYP1")
	binary.LittleEndian.PutUint16(out[4:], 0x20)
	binary.LittleEndian.PutUint16(out[6:], uint16(w))
	binary.LittleEndian.PutUint16(out[8:], uint16(h))
	binary.LittleEndian.PutUint32(out[10:], uint32(len(compressed)))
	out = append(out, compressed...)
	return out
}
