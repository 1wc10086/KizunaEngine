package wire

import (
	"math"

	"kizuna/engine/core/binutil"
)

type field struct {
	typ byte
	val []byte
}

type Reader struct {
	fields map[uint16]field
}

func NewReader(b []byte) *Reader {
	r := &Reader{fields: make(map[uint16]field, 8)}
	for len(b) >= 8 {
		id := binutil.U16(b[0:2])
		typ := b[2]
		n := binutil.U32(b[4:8])
		b = b[8:]
		if uint64(n) > uint64(len(b)) {
			break
		}
		r.fields[id] = field{typ: typ, val: b[:n]}
		b = b[n:]
	}
	return r
}

func (r *Reader) value(id uint16) []byte {
	if r == nil || r.fields == nil {
		return nil
	}
	f, ok := r.fields[id]
	if !ok {
		return nil
	}
	return f.val
}

func (r *Reader) U32(id uint16) uint32 {
	v := r.value(id)
	if len(v) < 4 {
		return 0
	}
	return binutil.U32(v)
}

func (r *Reader) I32(id uint16) int32 {
	return int32(r.U32(id))
}

func (r *Reader) U64(id uint16) uint64 {
	v := r.value(id)
	if len(v) < 8 {
		return 0
	}
	return binutil.U64(v)
}

func (r *Reader) F64(id uint16) float64 {
	v := r.value(id)
	if len(v) < 8 {
		return 0
	}
	return math.Float64frombits(binutil.U64(v))
}

func (r *Reader) Bool(id uint16) bool {
	v := r.value(id)
	if len(v) < 1 {
		return false
	}
	return v[0] != 0
}

func (r *Reader) Str(id uint16) string {
	return string(r.value(id))
}

func (r *Reader) Raw(id uint16) []byte {
	return r.value(id)
}

func (r *Reader) StrList(id uint16) []string {
	if r == nil || r.fields == nil {
		return nil
	}
	f, ok := r.fields[id]
	if !ok || f.typ != typeList {
		return nil
	}
	var out []string
	sub := f.val
	for len(sub) >= 8 {
		typ := sub[2]
		n := binutil.U32(sub[4:8])
		v := sub[8:]
		if uint64(n) > uint64(len(v)) {
			return out
		}
		if typ == typeStr {
			out = append(out, string(v[:n]))
		}
		sub = v[n:]
	}
	return out
}

func (r *Reader) Each(id uint16, fn func(*Reader)) {
	if fn == nil {
		return
	}
	if r == nil || r.fields == nil {
		return
	}
	f, ok := r.fields[id]
	if !ok || f.typ != typeList {
		return
	}
	sub := f.val
	for len(sub) >= 8 {
		typ := sub[2]
		n := binutil.U32(sub[4:8])
		v := sub[8:]
		if uint64(n) > uint64(len(v)) {
			return
		}
		if typ == typeList {
			fn(NewReader(v[:n]))
		}
		sub = v[n:]
	}
}
