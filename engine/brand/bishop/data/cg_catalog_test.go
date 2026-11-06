package data

import "testing"

// TestReadCgNameEncodings 验证 cg.dat 名字表的两种实际编码。
//
// KD 的 cg.dat 是 CP932/ASCII 单字节串；现代作品是 UTF-16LE。
// 只按 UTF-16 读会让 KD 的名字变成乱码，鉴赏室缩略图全部加载失败；
// 逐条按"后一字节是否为 0"猜编码则会被表头的日文标题带偏（KJ），
// 所以编码必须由 cgNameEncodingOf 对整个池子探测。
func TestReadCgNameEncodings(t *testing.T) {
	ansi := []byte("ev01_00_1\x00tail")
	pos := 0
	if got := readCgName(ansi, &pos, cgNameCp932); got != "ev01_00_1" || pos != len("ev01_00_1")+1 {
		t.Fatalf("ansi name = %q pos=%d", got, pos)
	}

	wide := []byte{'e', 0, 'v', 0, 0, 0, 'x'}
	pos = 0
	if got := readCgName(wide, &pos, cgNameUtf16); got != "ev" || pos != 6 {
		t.Fatalf("wide name = %q pos=%d", got, pos)
	}
}

// TestCgNameEncodingOf 验证名字池编码探测的三类输入：
//   - 单字节 ASCII/CP932 池（KD）：0 只出现在串结尾 → CP932；
//   - UTF-16LE 纯 ASCII 池（屈辱）：每个字符高字节都是 0 → UTF-16；
//   - UTF-16LE 池以日文标题开头（KJ『残酷な契り』）：ASCII 图片名仍占
//     绝对多数，整池判定 UTF-16，首名不再把逐条启发式带偏。
func TestCgNameEncodingOf(t *testing.T) {
	cp932 := []byte("ev01_00_1\x00ev02_00_2\x00残\x00\x00")
	if got := cgNameEncodingOf(cp932, 0); got != cgNameCp932 {
		t.Fatalf("cp932 pool detected as %v", got)
	}

	wide := []byte{'e', 0, 'v', 0, '0', 0, '1', 0, 0, 0}
	if got := cgNameEncodingOf(wide, 0); got != cgNameUtf16 {
		t.Fatalf("utf16 pool detected as %v", got)
	}

	// 学園（GK）的池是单字节 ASCII 图片名，860 个名字、2347 个高字节非 0
	// 的 2 字节单元。旧阈值 25% 会把它误判成 UTF-16，名字错位成韩文，
	// 鉴赏室所有 CG 都加载失败。
	gk := []byte{}
	for _, name := range []string{"g1010", "g1011", "g1020", "g1020p1", "g1030", "g1030a3", "g6032", "g7071"} {
		gk = append(gk, []byte(name)...)
		gk = append(gk, 0)
	}
	if got := cgNameEncodingOf(gk, 0); got != cgNameCp932 {
		t.Fatalf("gk-style pool detected as %v", got)
	}

	kj := []byte{}
	for _, character := range "残酷な契り" {
		kj = append(kj, byte(character), byte(character>>8))
	}
	kj = append(kj, 0, 0)
	for _, character := range "ev00_002_1p00" {
		kj = append(kj, byte(character), 0)
	}
	kj = append(kj, 0, 0)
	if got := cgNameEncodingOf(kj, 0); got != cgNameUtf16 {
		t.Fatalf("kj-style pool detected as %v", got)
	}
}

// TestParseCgCatalogLayers 验证变体的“附加图层组”结构。
//
// MK 的 cg.dat 每个变体是 [标记][主索引表][附加图层数][每层索引表]：
// 一张 CG 由底图和若干覆盖层合成（立绘 + 表情）。旧解析器只读 1 字节
// 尾部标记，遇到带图层的格子就会把后面的数据当成新变体，整张表错位，
// 最终名字表读出 0 条。这里用最小样本锁住“底图在前、图层按序在后”。
func TestParseCgCatalogLayers(t *testing.T) {
	wideName := func(text string) []byte {
		var out []byte
		for _, character := range text {
			out = append(out, byte(character), byte(character>>8))
		}
		return append(out, 0, 0)
	}
	data := []byte{}
	data = append(data, 1)          // 组数
	data = append(data, 1)          // 组内格数
	data = append(data, 1)          // 格内变体数
	data = append(data, 0)          // 变体标记
	data = append(data, 2)          // 主索引表长度
	data = append(data, 0, 0, 0, 0) // 索引 0（底图）
	data = append(data, 1, 0, 0, 0) // 索引 1（身体）
	data = append(data, 1)          // 附加图层数
	data = append(data, 1)          // 该层索引数
	data = append(data, 2, 0, 0, 0) // 索引 2（表情）
	data = append(data, 3, 0)       // 名字表条数
	data = append(data, wideName("g10110")...)
	data = append(data, wideName("g10110b")...)
	data = append(data, wideName("g10110f0")...)

	catalog, err := ParseCgCatalog(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if catalog == nil || len(catalog.Groups) != 1 {
		t.Fatalf("groups = %+v", catalog)
	}
	group := catalog.Groups[0]
	if len(group.Entries) != 1 || len(group.Entries[0].Variants) != 1 {
		t.Fatalf("entries = %+v", group.Entries)
	}
	images := group.Entries[0].Variants[0].Images
	want := []string{"g10110", "g10110b", "g10110f0"}
	if len(images) != len(want) {
		t.Fatalf("images = %v, want %v", images, want)
	}
	for index := range want {
		if images[index] != want[index] {
			t.Fatalf("images = %v, want %v", images, want)
		}
	}
}
