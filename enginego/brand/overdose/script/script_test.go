package script

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 测试只跑真实抽出的 .asb；没有数据时整跳过（CI 与本地都不硬失败）。
// Why 不合成构造：本包的核心 invariant 是“解压后字节流与原引擎
// handler 的 PC/跳址/变量引用完全对齐”，合成构造只能证明解码器
// 会返回它自己写的东西，拦不住错位。

func skipIfNoData(t *testing.T) string {
	t.Helper()
	cands := []string{
		"/root/code/tmpwork/extract/script",
		"../../tmpwork/extract/script",
		"../../../tmpwork/extract/script",
		filepath.Join(os.Getenv("KIZUNA_TEST_DATA"), "script"),
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	t.Skip("未找到 /root/code/tmpwork/extract/script，跳过")
	return ""
}

func loadRaw(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("读 %s: %v", name, err)
	}
	return b
}

// decompressCopy 复刻 tmpwork 的金标准解压，独立校验 DecompressASB：
// 若实现里偷偷“回到明文直通”，这里能直接发现差异。

func decompressCopy(t *testing.T, raw []byte) []byte {
	t.Helper()
	if len(raw) < 16 || string(raw[:4]) != "ASB\x00" {
		t.Fatalf("压缩文件魔数不对")
	}
	encSize := binary.LittleEndian.Uint32(raw[4:8])
	uncompSize := binary.LittleEndian.Uint32(raw[8:12])
	if encSize != uint32(len(raw)-16) {
		t.Fatalf("头长度 %d != 文件长度 %d", encSize, len(raw)-16)
	}
	key := uncompSize ^ 0x9e370001
	body := raw[16:]
	n := len(body) / 4
	out := make([]byte, n*4)
	for i := 0; i < n; i++ {
		w := binary.LittleEndian.Uint32(body[i*4:])
		binary.LittleEndian.PutUint32(out[i*4:], w-key)
	}
	plain := append(out, body[n*4:]...)
	zr, err := zlib.NewReader(bytes.NewReader(plain))
	if err != nil {
		t.Fatalf("金标准 zlib: %v", err)
	}
	defer zr.Close()
	buf, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("金标准解压: %v", err)
	}
	if uint32(len(buf)) != uncompSize {
		t.Fatalf("金标准解压长度 %d != 头 %d", len(buf), uncompSize)
	}
	return buf
}

func TestDecompressMain(t *testing.T) {
	dir := skipIfNoData(t)
	raw := loadRaw(t, dir, "main.asb")
	got, err := DecompressASB(raw)
	if err != nil {
		t.Fatalf("DecompressASB: %v", err)
	}
	want := decompressCopy(t, raw)
	if len(got) != len(want) {
		t.Fatalf("解压长度 %d != 金标准 %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("解压字节 @%d: got %02x want %02x", i, got[i], want[i])
		}
	}
	uncomp := binary.LittleEndian.Uint32(raw[8:12])
	if uint32(len(got)) != uncomp {
		t.Fatalf("解压长度与头不一致")
	}
}

func TestParseAllScripts(t *testing.T) {
	dir := skipIfNoData(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("读目录: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".asb") && !strings.HasSuffix(e.Name(), ".raw") {
			raw := loadRaw(t, dir, e.Name())
			if _, err := DecompressASB(raw); err != nil {
				t.Errorf("%s decompress: %v", e.Name(), err)
				continue
			}
			scr, err := Unmarshal(strings.TrimSuffix(e.Name(), ".asb"), raw)
			if err != nil {
				t.Errorf("%s parse: %v", e.Name(), err)
				continue
			}
			for name, off := range scr.Labels {
				if _, ok := scr.instructionAt(off); !ok {
					t.Errorf("%s 标签 %q 偏移 %#x 不是指令起点", e.Name(), name, off)
				}
			}
			n++
		}
	}
	if n < 5 {
		t.Fatalf("解析成功文件太少 %d", n)
	}
}

func TestVMMainReachesS101(t *testing.T) {
	dir := skipIfNoData(t)
	rawMain := loadRaw(t, dir, "main.asb")
	mainScr, err := Unmarshal("main", rawMain)
	if err != nil {
		t.Fatalf("main: %v", err)
	}
	rawS101 := loadRaw(t, dir, "s101.asb")
	s101Scr, err := Unmarshal("s101", rawS101)
	if err != nil {
		t.Fatalf("s101: %v", err)
	}
	rawInit := loadRaw(t, dir, "init_eval.asb")
	initScr, err := Unmarshal("init_eval", rawInit)
	if err != nil {
		t.Fatalf("init_eval: %v", err)
	}

	loader := func(name string) (*Script, error) {
		switch name {
		case "main":
			return mainScr, nil
		case "s101":
			return s101Scr, nil
		case "init_eval":
			return initScr, nil
		default:
			// 本测试只验剧本主线；sop* 是 OP 影片脚本，真实会话交给影片层，
			// 这里当“无脚本”跳过，否则 6 段开场的循环会把预算吃光。
			return nil, errScriptNotFound
		}
	}
	// 与当前内存图一致：visited 先只记 loader 打开过的路径；
	// VM 用 Start 标入口，所以 Visited("s101") 由调用 0x0a 落入
	vm := New(loader)
	var macros []string
	vm.SetMacroHook(func(name string, args []Value) error {
		macros = append(macros, name)
		if name == "rand_int" {
			_ = vm.vars.SetInt(1, 92, 0)
		}
		return nil
	})
	if err := vm.Start("main"); err != nil {
		t.Fatalf("start: %v", err)
	}
	var lines []string
	var events []Event
	h := EventHandlerFunc(func(e Event) error {
		events = append(events, e)
		if e.Kind == EventDialogue && e.Text != "" {
			lines = append(lines, e.Text)
		}
		return nil
	})
	smallLoader := loader
	vm.loader = smallLoader
	_ = smallLoader
	steps := 0
	for steps < 2000000 && !vm.Done() {
		n, err := vm.Step(4096, h)
		steps += n
		if err != nil {
			t.Fatalf("step %d: %v", steps, err)
			break
		}
		if len(lines) > 200 {
			break
		}
		if vm.Done() && len(lines) == 0 {
			break
		}
	}
	// 恢复 vm 的 getter 习惯（测试直接用私有字段已改下方）
	_ = events
	_ = macros
	if steps >= 2000000 && len(lines) == 0 {
		t.Fatalf("跑满预算也没对白 (steps=%d)", steps)
	}
	if len(lines) == 0 {
		t.Fatalf("没拿到对白 (steps=%d)", steps)
	}
	foundKata := false
	for _, l := range lines {
		if strings.Contains(l, "――") || strings.Contains(l, "匂い") {
			foundKata = true
			break
		}
	}
	if !foundKata {
		t.Fatalf("对白中没找到 ―― 或 匂い：%q", lines[:min(3, len(lines))])
	}
	if !vm.Visited("s101") {
		t.Fatalf("2000000 步也没走到 s101 (steps=%d, events=%d)", steps, len(events))
	}
	if !contains(lines, "――匂い。") && !contains(lines, "――匂い") {
		t.Logf("首句对白: %q", lines[0])
		if !strings.Contains(lines[0], "――") {
			t.Fatalf("首句不是 ―― 开头: %q", lines[0])
		}
	}
	// 显式把 visited 的链路打开一遍：s101 在这里就是 macro 之后的本文件
	if !vm.visited["s101"] {
		t.Fatalf("visited 没有 s101")
	}
}

func contains(s []string, sub string) bool {
	for _, v := range s {
		if strings.Contains(v, sub) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
