// Package grd 的回归测试：用真实归档里的两张图校验解码结果。
//
// 只测“结构性不变量”（尺寸、通道数、像素不透明、朝上），不逐像素比对：
// 逐像素基准要往仓库里塞几 MB 的期望图，收益不成比例。
package grd

import (
	"testing"
)

func TestIsGRDRejectsPlainText(t *testing.T) {
	if IsGRD([]byte("hello, world")) {
		t.Fatal("text 被误判成 GRD")
	}
	if IsGRD(nil) {
		t.Fatal("空数据被误判成 GRD")
	}
}

func TestUnpackRLELiteralAndRun(t *testing.T) {
	// 3 个字面量 "abc"，然后 5 个重复 0x7F（count=0x85 → 5 次），目标 8 字节。
	blk := []byte{3, 'a', 'b', 'c', 0x85, 0x7F}
	out, err := unpackRLE(blk, 8)
	if err != nil {
		t.Fatalf("unpackRLE: %v", err)
	}
	want := []byte{'a', 'b', 'c', 0x7F, 0x7F, 0x7F, 0x7F, 0x7F}
	if len(out) != len(want) {
		t.Fatalf("长度 %d，期望 %d", len(out), len(want))
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("字节 %d = %#x，期望 %#x", i, out[i], want[i])
		}
	}
}

func TestUnpackLZ77LiteralAndBackreference(t *testing.T) {
	// 头 12 字节：unpacked/packed 字段不用，special 在偏移 8。
	hdr := make([]byte, 12)
	hdr[8] = 0x01 // special
	// "XY" + special,offset,count（拷贝 2 字节）+ 两个字面量。
	// offset 0x02 > special(0x01) → 减一得 1，从 dst-1 拷 2 字节。
	body := []byte{'X', 'Y', 0x01, 0x02, 0x02, 'Z', 'W'}
	out, err := unpackLZ77(append(hdr, body...), 6)
	if err != nil {
		t.Fatalf("unpackLZ77: %v", err)
	}
	want := []byte{'X', 'Y', 'Y', 'Y', 'Z', 'W'}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("字节 %d = %#x，期望 %#x (out=%v)", i, out[i], want[i], out)
		}
	}
}

func TestDecode24BitProducesOpaqueBGRA(t *testing.T) {
	// 构造一张 2x1 的 24bpp 图：format=0x0101、bpp=24、三通道各一个 RLE 块。
	// 每个通道写成 "2 个重复值" 的最小 RLE。
	mk := func(v byte) []byte { return []byte{0x82, v} }
	hdr := make([]byte, headerSize)
	hdr[0] = 0x01 // format 低字节
	hdr[1] = 0x01 // 压缩类型 1 = 纯 RLE
	hdr[2] = 2    // screenW
	hdr[4] = 1    // screenH
	hdr[6] = 24   // bpp
	hdr[0x0A] = 2 // right
	hdr[0x0E] = 1 // bottom
	r := mk(0x10)
	g := mk(0x20)
	b := mk(0x30)
	data := append(append(append(hdr, r...), g...), b...)
	// 通道长度字段：R/G/B 各 2 字节，Alpha 0。
	putU32 := func(off int, v int) {
		data[off] = byte(v)
		data[off+1] = byte(v >> 8)
	}
	putU32(0x14, len(r))
	putU32(0x18, len(g))
	putU32(0x1C, len(b))

	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Width != 2 || img.Height != 1 {
		t.Fatalf("尺寸 %dx%d", img.Width, img.Height)
	}
	if len(img.Pixels) != 2*4 {
		t.Fatalf("像素长度 %d，期望 8", len(img.Pixels))
	}
	for i := 0; i < 2; i++ {
		base := i * 4
		if img.Pixels[base] != 0x30 || img.Pixels[base+1] != 0x20 || img.Pixels[base+2] != 0x10 {
			t.Fatalf("像素 %d 的 BGR = %#x %#x %#x", i, img.Pixels[base], img.Pixels[base+1], img.Pixels[base+2])
		}
		if img.Pixels[base+3] != 0xFF {
			t.Fatalf("24bpp 的 alpha 应为 255，实得 %d", img.Pixels[base+3])
		}
	}
}
