package binutil

import (
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf16"
)

type Reader struct {
	data []byte
	pos  int
	err  error
}

func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

func (r *Reader) Pos() int {
	return r.pos
}

func (r *Reader) SetPos(pos int) {
	r.pos = pos
}

func (r *Reader) Remaining() int {
	remaining := len(r.data) - r.pos
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (r *Reader) Err() error {
	return r.err
}

func (r *Reader) Fail(err error) {
	if r.err == nil && err != nil {
		r.err = err
	}
}

func (r *Reader) ensure(count int) bool {
	if r.err != nil {
		return false
	}
	if r.pos < 0 || count < 0 || r.pos+count > len(r.data) {
		r.Fail(fmt.Errorf("binutil: cannot read %d bytes at position %d: %w", count, r.pos, io.ErrUnexpectedEOF))
		return false
	}
	return true
}

func (r *Reader) Skip(n int) {
	if !r.ensure(n) {
		return
	}
	r.pos += n
}

func (r *Reader) U8() byte {
	if !r.ensure(1) {
		return 0
	}
	value := r.data[r.pos]
	r.pos++
	return value
}

func (r *Reader) U16() uint16 {
	if !r.ensure(2) {
		return 0
	}
	value := binary.LittleEndian.Uint16(r.data[r.pos:])
	r.pos += 2
	return value
}

func (r *Reader) U32() uint32 {
	if !r.ensure(4) {
		return 0
	}
	value := binary.LittleEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return value
}

func (r *Reader) I32() int32 {
	return int32(r.U32())
}

func (r *Reader) Bytes(n int) []byte {
	if !r.ensure(n) {
		return nil
	}
	value := make([]byte, n)
	copy(value, r.data[r.pos:r.pos+n])
	r.pos += n
	return value
}

func (r *Reader) SevenBitInt() int {
	if r.err != nil {
		return 0
	}
	result := 0
	for shift := 0; shift < 35; shift += 7 {
		if r.pos < 0 || r.pos >= len(r.data) {
			r.Fail(fmt.Errorf("binutil: truncated 7-bit integer at position %d: %w", r.pos, io.ErrUnexpectedEOF))
			return result
		}
		current := r.data[r.pos]
		r.pos++
		result |= int(current&0x7F) << shift
		if current&0x80 == 0 {
			return result
		}
	}
	r.Fail(fmt.Errorf("binutil: invalid 7-bit integer at position %d", r.pos))
	return result
}

func (r *Reader) ZeroTerminatedString() string {
	if r.err != nil {
		return ""
	}
	for i := r.pos; i < len(r.data); i++ {
		if r.data[i] == 0 {
			value := string(r.data[r.pos:i])
			r.pos = i + 1
			return value
		}
	}
	r.Fail(fmt.Errorf("binutil: unterminated string at position %d: %w", r.pos, io.ErrUnexpectedEOF))
	return ""
}

func (r *Reader) UTF16LEString(units int) string {
	if r.err != nil {
		return ""
	}
	if r.pos < 0 || units < 0 || units > (len(r.data)-r.pos)/2 {
		r.Fail(fmt.Errorf("binutil: cannot read %d utf16 units at position %d: %w", units, r.pos, io.ErrUnexpectedEOF))
		return ""
	}
	codes := make([]uint16, units)
	for i := range codes {
		codes[i] = binary.LittleEndian.Uint16(r.data[r.pos+2*i:])
	}
	r.pos += units * 2
	return string(utf16.Decode(codes))
}

func U16(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}

func U32(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}

func I32(b []byte) int32 {
	return int32(binary.LittleEndian.Uint32(b))
}

func U64(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

func PutU16(dst []byte, v uint16) {
	binary.LittleEndian.PutUint16(dst, v)
}

func PutU32(dst []byte, v uint32) {
	binary.LittleEndian.PutUint32(dst, v)
}

func PutI32(dst []byte, v int32) {
	binary.LittleEndian.PutUint32(dst, uint32(v))
}

func NegateBytes(dst []byte) {
	for i := range dst {
		dst[i] = ^dst[i]
	}
}

func SumBytes(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(^b)
	}
	return sum
}

type BitReader struct {
	data        []byte
	position    int
	accumulator byte
	available   int
}

func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data}
}

func (b *BitReader) ReadBits(count int) (int, bool) {
	if count < 0 {
		return 0, false
	}
	value := 0
	for i := 0; i < count; i++ {
		bit, ok := b.ReadBit()
		if !ok {
			return 0, false
		}
		value = (value << 1) | bit
	}
	return value, true
}

func (b *BitReader) ReadBit() (int, bool) {
	if b.available == 0 {
		if b.position < 0 || b.position >= len(b.data) {
			return 0, false
		}
		b.accumulator = b.data[b.position]
		b.position++
		b.available = 8
	}
	b.available--
	return int((b.accumulator >> uint(b.available)) & 1), true
}
