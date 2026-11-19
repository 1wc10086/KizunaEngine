package session

import (
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

// exprReaderForText 是 TextList 用的轻量表达式跳过器：只需要知道
// 0x01/0x02 各带 4 字节，其它 token 1 字节，0x00 结束。它不执行表达式，
// 因此不会因为变量/跳转破坏当前 VM。
type exprReaderForText struct {
	pay []byte
	pos int
}

func (r *exprReaderForText) skipExpr() {
	for r.pos < len(r.pay) {
		b := r.pay[r.pos]
		r.pos++
		switch b {
		case 0x00:
			return
		case 0x01, 0x02:
			r.pos += 4
		}
	}
}

func (r *exprReaderForText) cstr() string {
	start := r.pos
	for r.pos < len(r.pay) && r.pay[r.pos] != 0 {
		r.pos++
	}
	s := decodeCP932Local(r.pay[start:r.pos])
	if r.pos < len(r.pay) {
		r.pos++
	}
	return s
}

func decodeCP932Local(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	out, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}
