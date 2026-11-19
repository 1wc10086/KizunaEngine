package asb

import (
	"encoding/binary"
	"strings"
	"testing"

	"kizuna/kernel/core/textcodec"
)

func strNode(t *testing.T, text string) []byte {
	t.Helper()
	payload, err := textcodec.EncodeCP932(text)
	if err != nil {
		t.Fatal(err)
	}
	out := append([]byte{byte(len(payload) + 3), 7}, payload...)
	return append(out, 0)
}

func instr(op uint16, field uint32, argc int, args []byte) []byte {
	out := make([]byte, 12+len(args))
	binary.LittleEndian.PutUint16(out[0:], uint16(len(out)))
	binary.LittleEndian.PutUint16(out[2:], op)
	binary.LittleEndian.PutUint32(out[4:], field)
	binary.LittleEndian.PutUint16(out[8:], uint16(argc))
	copy(out[12:], args)
	return out
}

// 真实剧本的容器/文本/跳转三层一起测：解码出说话人与正文，把正文改成
// UTF-8 中文，重编后容器仍能解、文本正确、0x05 的跳转偏移被重算。
func TestCodecRoundTripUTF8(t *testing.T) {
	speaker := strNode(t, "ユウ")
	body := strNode(t, "こんにちは")
	dialogue := instr(0x1f, 0, 2, append(speaker, body...))

	// 0x05 跳到最后一条 return；field 是“旧流里的字节偏移”。
	jump := instr(0x05, uint32(len(dialogue)+12), 0, nil)
	ret := instr(0x08, 0, 0, nil)
	plain := append(append(dialogue, jump...), ret...)
	raw, err := compress(plain, 3)
	if err != nil {
		t.Fatal(err)
	}

	c := codec{}
	doc, err := c.Decode(raw, "main.asb")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(doc.Units) != 2 {
		t.Fatalf("units = %d: %+v", len(doc.Units), doc.Units)
	}
	if doc.Units[0].Text != "ユウ" || doc.Units[1].Text != "こんにちは" {
		t.Fatalf("decoded = %#v", doc.Units)
	}
	c.SetUnitText(doc, "asb:0:1", "你好，世界")
	out, err := c.Encode(doc)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	doc2, err := c.Decode(out, "main.asb")
	if err != nil {
		t.Fatalf("Decode roundtrip: %v", err)
	}
	got := ""
	for _, u := range doc2.Units {
		if u.ID == "asb:0:1" {
			got = u.Text
		}
	}
	if got != "你好，世界" {
		t.Fatalf("roundtrip body = %q", got)
	}

	// 跳转必须落到新的 return 偏移，而不是旧偏移。
	newPlain, err := decompress(out)
	if err != nil {
		t.Fatal(err)
	}
	ins, err := parseStream(newPlain, "main.asb")
	if err != nil {
		t.Fatal(err)
	}
	if len(ins) != 3 {
		t.Fatalf("instructions = %d", len(ins))
	}
	if int(ins[1].field) != ins[2].offset {
		t.Fatalf("jump field = %d, want %d", ins[1].field, ins[2].offset)
	}
	if !strings.Contains(string(newPlain), "你好，世界") {
		t.Fatal("UTF-8 正文没有写进指令流")
	}
}

func TestDetectFormat(t *testing.T) {
	c := codec{}
	raw, _ := compress(instr(0x08, 0, 0, nil), 1)
	if format, ok := c.DetectFormat("s101.asb", raw[:4]); !ok || format != "ASB" {
		t.Fatalf("detect = %q,%v", format, ok)
	}
	if _, ok := c.DetectFormat("s101.asb", []byte("NOPE")); ok {
		t.Fatal("非 ASB 不应命中")
	}
}
