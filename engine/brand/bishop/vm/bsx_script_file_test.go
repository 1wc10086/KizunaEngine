package vm

import (
	"encoding/binary"
	"testing"
)

// buildClassicModernScript 构造一个最小的 BSXScript 2.x 文件：
// 旧头部布局（code@0x28、groups@0x34…），但指令和 UTF-16 名字表与 3.x 相同。
func buildClassicModernScript() []byte {
	data := make([]byte, 0x140)
	copy(data, "BSXScript 2.0")
	put := func(offset int, value int32) {
		binary.LittleEndian.PutUint32(data[offset:], uint32(value))
	}
	// code block：一条 HALT。
	put(0x28, 0x100)
	put(0x2c, 1)
	// 标签表：1 条（offset 0，name id 0），名字池 "boot\0"（UTF-16LE）。
	put(0x34, 0xe0)
	put(0x38, 8)
	put(0x3c, 0xe8)
	put(0x40, 10)
	put(0xe0, 0)
	put(0xe4, 0)
	for index, character := range []byte{'b', 'o', 'o', 't', 0} {
		binary.LittleEndian.PutUint16(data[0xe8+index*2:], uint16(character))
	}
	data[0x100] = OpcodeHalt
	return data
}

func TestParseBsxScript20UsesModernDialect(t *testing.T) {
	script, err := ParseBsxScriptFile(buildClassicModernScript())
	if err != nil {
		t.Fatalf("ParseBsxScriptFile: %v", err)
	}
	if script.Signature() != "BSXScript 2.0" {
		t.Fatalf("signature = %q", script.Signature())
	}
	if script.LayoutKind() != LayoutClassic {
		t.Fatalf("layout = %d, want Classic header", script.LayoutKind())
	}
	// 2.x 的头部是旧的，但操作码是现代编码；IsLegacy 必须看语义而非头部。
	if script.IsLegacy() {
		t.Fatal("BSXScript 2.x must use the modern opcode dialect")
	}
	if script.LabelCount() != 1 {
		t.Fatalf("labels = %d", script.LabelCount())
	}
	if got := script.GetLabelName(0); got != "boot" {
		t.Fatalf("label name = %q, want boot", got)
	}
	if got := script.FindLabel("boot"); got != 0 {
		t.Fatalf("FindLabel = %d, want 0", got)
	}
}

func TestDialectForSignature(t *testing.T) {
	cases := []struct {
		signature string
		layout    ScriptLayout
		want      ScriptDialectKind
	}{
		{"BSXScript 2.0", LayoutClassicValue, DialectKindModern},
		{"BSXScript 3.3", LayoutModernValue, DialectKindModern},
		{"BSScript Rev.6", LayoutClassicValue, DialectKindLegacy},
		{"BSScript", LayoutAncientValue, DialectKindAncient},
	}
	for _, item := range cases {
		if got := dialectForSignature(item.signature, item.layout).Kind(); got != item.want {
			t.Fatalf("%q -> %d, want %d", item.signature, got, item.want)
		}
	}
}
