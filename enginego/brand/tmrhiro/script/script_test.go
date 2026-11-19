package script

import "testing"

func TestRot4IsInvolution(t *testing.T) {
	for b := 0; b < 256; b++ {
		if got := rot4(rot4(byte(b))); int(got) != b {
			t.Fatalf("rot4 不是对合: %#x", b)
		}
	}
}

func TestIsScriptRejectsBrokenContainer(t *testing.T) {
	// 记录数 2，但只有一条记录、且长度越界。
	bad := []byte{2, 0, 0, 0, 6, 0, 1, 2, 3, 4}
	if IsScript(bad) {
		t.Fatal("长度不符的容器被判成 .sct")
	}
}

func TestParseReadsOpArgAndPayload(t *testing.T) {
	// 文件里整条 chunk（含 op/arg）都是 nibble 交换过的形态；Parse 再交换回来。
	// 这里先拼逻辑值（op=0 文本，arg=0x20 有人名，正文 "A,B"），再统一交换。
	logical := []byte{0x00, 0x00, 0x20, 0x00, 'A', ',', 'B'}
	disk := make([]byte, len(logical))
	for i, b := range logical {
		disk[i] = rot4(b)
	}
	// 记录长度字段不交换：u16 len = 4 字节头 + 3 字节正文 = 7。
	rec := append([]byte{7, 0}, disk...)
	data := append([]byte{1, 0, 0, 0}, rec...)
	s, err := Parse("t", data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(s.Records) != 1 {
		t.Fatalf("记录数 %d", len(s.Records))
	}
	r := s.Records[0]
	if r.Op != OpText || r.Arg != 0x20 {
		t.Fatalf("op/arg = %d/%#x", r.Op, r.Arg)
	}
	fields := SplitText(r.Raw, 2)
	if len(fields) != 2 || fields[0] != "A" || fields[1] != "B" {
		t.Fatalf("字段 = %v", fields)
	}
}

func TestSplitTextKeepsSurplusCommasInLastField(t *testing.T) {
	fields := SplitText([]byte("name,body,with,commas"), 2)
	if len(fields) != 2 || fields[1] != "body,with,commas" {
		t.Fatalf("字段 = %v", fields)
	}
}

func TestAudioChannelFromHighNibble(t *testing.T) {
	if got := AudioChannel(OpAudio, 0x008c); got != 0 {
		t.Fatalf("0x008c 通道 = %d", got)
	}
	if got := AudioChannel(OpAudio, 0x028c); got != 2 {
		t.Fatalf("0x028c 通道 = %d", got)
	}
	if got := AudioChannel(OpAudio, 0x10c2); got != 0 {
		t.Fatalf("0x10c2 通道 = %d", got)
	}
	if got := AudioChannel(OpAudio, 0x12c2); got != 2 {
		t.Fatalf("0x12c2 通道 = %d", got)
	}
}
