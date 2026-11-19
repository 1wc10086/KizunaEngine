package config

import "testing"

func TestMapButtonValueAcceptsHexAndSplitValue(t *testing.T) {
	// SMIE：value, frames 写在同一行，值用 0x 十六进制。
	hexDocument := &BsxxConfiguration{document: ParseIniDocumentText(`
[button object]
command_combi2_ryoujoku = 0xb0, 2
command_noselect = 0xe0, 2
`)}
	if got := hexDocument.MapButtonValue("command_combi2_ryoujoku"); got != 0xb0 {
		t.Fatalf("hex value = 0x%x, want 0xb0", got)
	}
	if got := hexDocument.MapButtonFrames("command_combi2_ryoujoku"); got != 2 {
		t.Fatalf("hex frames = %d, want 2", got)
	}

	// SM：帧数写在主键，真正的值写在 "<name> value"。
	splitDocument := &BsxxConfiguration{document: ParseIniDocumentText(`
[button object]
command_combi2_ryoujoku = 2
command_combi2_ryoujoku value = 0xb0
command_noselect = 2
command_noselect value = 0xe0
`)}
	if got := splitDocument.MapButtonValue("command_combi2_ryoujoku"); got != 0xb0 {
		t.Fatalf("split value = 0x%x, want 0xb0", got)
	}
	if got := splitDocument.MapButtonValue("command_noselect"); got != 0xe0 {
		t.Fatalf("split noselect = 0x%x, want 0xe0", got)
	}
}

// TestMapButtonValueKeepsHighBit 锁住学園3 行动卡的值语义。
//
// 行动卡把"已选择 + 角色号 + 行动号"打包进 bit31..0，值形如 0x80001002；
// 旧实现对 0x 用 ParseInt(...,32)，超过 MaxInt32 直接报错并返回 0。
// 玩家点卡后剧本收到 0，`#select_value & 0x80000000` 永远为假，行动槽
// 一张也填不进去。这里同时覆盖两种 [button object] 写法与 3 帧卡面。
func TestMapButtonValueKeepsHighBit(t *testing.T) {
	inline := &BsxxConfiguration{document: ParseIniDocumentText(`
[button object]
card16 = 0x80001002, 3
card10 = 0x00001000, 3
`)}
	if got := uint32(inline.MapButtonValue("card16")); got != 0x80001002 {
		t.Fatalf("inline card16 = 0x%08x, want 0x80001002", got)
	}
	if got := inline.MapButtonFrames("card16"); got != 3 {
		t.Fatalf("inline card16 frames = %d, want 3", got)
	}

	split := &BsxxConfiguration{document: ParseIniDocumentText(`
[button object]
card16 = 3
card16 value = 0x80001002
card10 = 3
card10 value = 0x00001000
`)}
	if got := uint32(split.MapButtonValue("card16")); got != 0x80001002 {
		t.Fatalf("split card16 = 0x%08x, want 0x80001002", got)
	}
	// 单列主键是帧数，不是值；旧实现会回退到 2 帧，把 128x576 的卡面
	// 切成两半。
	if got := split.MapButtonFrames("card16"); got != 3 {
		t.Fatalf("split card16 frames = %d, want 3", got)
	}
	if got := uint32(split.MapButtonValue("card10")); got != 0x00001000 {
		t.Fatalf("split card10 = 0x%08x, want 0x00001000", got)
	}
}
