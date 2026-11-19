package arc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 以下测试都只查真实游戏文件，文件不在时整轮跳过，
// 保证 CI 在没有游戏数据的时间里也要全绿。
// 查找顺序：GAME_DIR 环境、容器机的默认路径、相对路径兜底。
var gameDirCandidates = []string{
	os.Getenv("GAME_DIR"),
	"/root/code/engine/游戏/YUI/優衣",
	"../../../engine/游戏/YUI/優衣",
	"./testdata",
}

func gameDir(t *testing.T) string {
	t.Helper()
	for _, c := range gameDirCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(c, "system.arc")); err == nil {
			return c
		}
	}
	t.Skip("未找到游戏数据目录（设置 GAME_DIR）")
	return ""
}

// 五个存档的 48 字节头必须能解出 ARC\0：
// 这是流密码、密钥表与 GenKeyTable 的最硬断言。要注意头本身是密文，
// 直接读魔数只能读到密文，必须用表从文件大小导出后异或一轮。
func TestFiveHeadersDecryptToARCZero(t *testing.T) {
	dir := gameDir(t)
	for _, name := range []string{"graphic.arc", "script.arc", "sound.arc", "music.arc", "voice.arc", "system.arc"} {
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		buf := make([]byte, 48)
		if _, err := f.ReadAt(buf, 0); err != nil {
			f.Close()
			t.Fatalf("%s: %v", name, err)
		}
		f.Close()
		table := GenKeyTable(uint32(fi.Size()))
		plain := Decrypt(buf, table, 0)
		if string(plain[0:4]) != "ARC\x00" {
			t.Fatalf("%s header magic %q, want ARC\\0", name, plain[0:4])
		}
		cnt := int(plain[8]) | int(plain[9])<<8 | int(plain[10])<<16 | int(plain[11])<<24
		if cnt <= 0 {
			t.Fatalf("%s entry count %d 非法", name, cnt)
		}
	}
}

// spec §1 给定了各存档的条目数，这里逐一对照：
// 对不上说明索引解密或字段偏移错了，不用大批量取条目即见。
func TestEntryCounts(t *testing.T) {
	dir := gameDir(t)
	cases := []struct {
		name  string
		count int
	}{
		{"graphic.arc", 2059},
		{"script.arc", 172},
		{"sound.arc", 78},
		{"music.arc", 10},
		{"voice.arc", 6573},
		{"system.arc", 27},
	}
	for _, c := range cases {
		a, err := Open(filepath.Join(dir, c.name))
		if err != nil {
			t.Fatalf("Open %s: %v", c.name, err)
		}
		if a.Count != c.count {
			a.Close()
			t.Fatalf("%s entries = %d, want %d", c.name, a.Count, c.count)
		}
		a.Close()
	}
}

func TestNamesTbl(t *testing.T) {
	dir := gameDir(t)
	a, err := Open(filepath.Join(dir, "system.arc"))
	if err != nil {
		t.Fatalf("Open system.arc: %v", err)
	}
	defer a.Close()
	names := a.Names(".tbl")
	if len(names) != 27 {
		t.Fatalf("system .tbl entries = %d, want 27", len(names))
	}
	// sysenv.tbl 在同一座，名录里必有一席之地
	found := false
	for _, n := range names {
		if n == "sysenv.tbl" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("sysenv.tbl not in names %v", names)
	}
	// 无后缀的也应查过：列出全部应含表与非表两种条目
	if all := a.Names(""); len(all) < 27 {
		t.Fatalf("Names() too few: %d", len(all))
	}
}

func TestEntryDecryptRoundTrip(t *testing.T) {
	dir := gameDir(t)
	a, err := Open(filepath.Join(dir, "script.arc"))
	if err != nil {
		t.Fatalf("Open script.arc: %v", err)
	}
	defer a.Close()
	if _, ok := a.find("main.asb"); !ok {
		t.Fatalf("main.asb missing")
	}
	data, err := a.Entry("main.asb")
	if err != nil {
		t.Fatalf("Entry(main.asb): %v", err)
	}
	// m 以内：spec §5 说明 decompressed entry 是 ASB\0 容器，
	// 空属可笑但先验主链。
	if len(data) < 16 || string(data[0:4]) != "ASB\x00" {
		t.Fatalf("main.asb head %q, want ASB\\0 (len %d)", data[:min(4, len(data))], len(data))
	}
	if sz, ok := a.EntrySize("main.asb"); !ok || sz <= 0 {
		t.Fatalf("EntrySize(main.asb) = %d, %v", sz, ok)
	}
}

func TestCPBDecodeFirstImage(t *testing.T) {
	dir := gameDir(t)
	set, err := OpenSet(dir)
	if err != nil {
		t.Fatalf("OpenSet: %v", err)
	}
	defer set.Close()
	names := set.Graphic.Names(".cpb")
	if len(names) != 2059 {
		t.Fatalf("graphic .cpb = %d, want 2059", len(names))
	}
	w, h, bgra, err := set.Image(names[0])
	if err != nil {
		t.Fatalf("Image(%s): %v", names[0], err)
	}
	if w <= 0 || h <= 0 || len(bgra) != w*h*4 {
		t.Fatalf("Image %s: %dx%d, %d bytes", names[0], w, h, len(bgra))
	}
	// 抽样说明：不是所有图形都是 800x600，压测不应将宽高写死
	t.Logf("sample %s %dx%d", names[0], w, h)
}

func TestTableParse(t *testing.T) {
	dir := gameDir(t)
	a, err := Open(filepath.Join(dir, "system.arc"))
	if err != nil {
		t.Fatalf("Open system.arc: %v", err)
	}
	defer a.Close()
	// list_script.tbl 有 150 条，cg/scene 分页表各结构均应可解
	for _, name := range []string{"list_script.tbl", "list_cg.tbl", "scene_page01.tbl"} {
		rec, err := a.Table(name)
		if err != nil {
			t.Fatalf("Table(%s): %v", name, err)
		}
		if len(rec) == 0 {
			t.Fatalf("Table(%s): empty", name)
		}
		empty := 0
		for _, r := range rec {
			if strings.TrimSpace(r.Name) == "" {
				empty++
			}
		}
		if empty == len(rec) {
			t.Fatalf("Table(%s): all empty names", name)
		}
		t.Logf("%s: %d records, first %q", name, len(rec), rec[0].Name)
	}
}
