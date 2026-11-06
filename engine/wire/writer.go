package wire

import (
	"encoding/binary"
	"math"

	"kizuna/engine/core/binutil"
)

const (
	typeU32   byte = 1
	typeI32   byte = 2
	typeU64   byte = 3
	typeStr   byte = 4
	typeBytes byte = 5
	typeList  byte = 6
	typeBool  byte = 7
	typeF64   byte = 8
)

type Writer struct {
	buf []byte
}

func NewWriter() *Writer {
	return &Writer{buf: make([]byte, 0, 256)}
}

func (w *Writer) Reset() {
	w.buf = w.buf[:0]
}

func (w *Writer) Bytes() []byte {
	return w.buf
}

func (w *Writer) head(id uint16, typ byte, n int) {
	var b [8]byte
	binutil.PutU16(b[0:2], id)
	b[2] = typ
	binutil.PutU32(b[4:8], uint32(n))
	w.buf = append(w.buf, b[:]...)
}

func (w *Writer) Str(id uint16, v string) {
	w.head(id, typeStr, len(v))
	w.buf = append(w.buf, v...)
}

func (w *Writer) U32(id uint16, v uint32) {
	w.head(id, typeU32, 4)
	var b [4]byte
	binutil.PutU32(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

func (w *Writer) I32(id uint16, v int32) {
	w.head(id, typeI32, 4)
	var b [4]byte
	binutil.PutU32(b[:], uint32(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *Writer) U64(id uint16, v uint64) {
	w.head(id, typeU64, 8)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

func (w *Writer) F64(id uint16, v float64) {
	w.head(id, typeF64, 8)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *Writer) Bool(id uint16, v bool) {
	w.head(id, typeBool, 1)
	if v {
		w.buf = append(w.buf, 1)
		return
	}
	w.buf = append(w.buf, 0)
}

func (w *Writer) Raw(id uint16, v []byte) {
	w.head(id, typeBytes, len(v))
	w.buf = append(w.buf, v...)
}

func (w *Writer) StrList(id uint16, vals []string) {
	w.List(id, func(sub *Writer) {
		for _, v := range vals {
			sub.Str(0, v)
		}
	})
}

func (w *Writer) List(id uint16, fn func(*Writer)) {
	base := len(w.buf)
	w.head(id, typeList, 0)
	if fn != nil {
		fn(w)
	}
	binutil.PutU32(w.buf[base+4:], uint32(len(w.buf)-base-8))
}
