package script

import (
	"testing"

	"kizuna/enginego/core/textcodec"
)

// 汉化剧本把字符串节点写成 UTF-8；原版仍是 CP932。解码必须同时吃两种，
// 且不能把 CP932 双字节误判成 UTF-8。
func TestDecodeScriptStringEncodings(t *testing.T) {
	utf8Text := "你好，世界"
	if got := decodeScriptString([]byte(utf8Text + "\x00")); got != utf8Text {
		t.Fatalf("UTF-8 = %q, want %q", got, utf8Text)
	}
	cp932, err := textcodec.EncodeCP932("こんにちは")
	if err != nil {
		t.Fatal(err)
	}
	payload := append(append([]byte(nil), cp932...), 0)
	if got := decodeScriptString(payload); got != "こんにちは" {
		t.Fatalf("CP932 = %q", got)
	}
	// BOM 不应出现在正文里。
	if got := decodeScriptString(append([]byte{0xEF, 0xBB, 0xBF}, append([]byte("你好"), 0)...)); got != "你好" {
		t.Fatalf("BOM = %q", got)
	}
}
