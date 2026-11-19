package disc

// pkware.go 实现 PKWARE DCL implode 解码（InstallShield Z 的数据编码）。
// 码表、位流方向（LSB-first）、长度/距离编码均对照 项目参考/1.py 第 4 节，
// 不要自行改动码表或位序，否则解出的数据会错位。

import (
	"fmt"
	"io"
)

const (
	pkwareMaxBits  = 13
	pkwareEndLen   = 519
	pkwareNoSymbol = -9
)

var pkwareLitLengths = []byte{
	11, 124, 8, 7, 28, 7, 188, 13, 76, 4, 10, 8, 12, 10, 12, 10, 8, 23, 8,
	9, 7, 6, 7, 8, 7, 6, 55, 8, 23, 24, 12, 11, 7, 9, 11, 12, 6, 7, 22, 5,
	7, 24, 6, 11, 9, 6, 7, 22, 7, 11, 38, 7, 9, 8, 25, 11, 8, 11, 9, 12,
	8, 12, 5, 38, 5, 38, 5, 11, 7, 5, 6, 21, 6, 10, 53, 8, 7, 24, 10, 27,
	44, 253, 253, 253, 252, 252, 252, 13, 12, 45, 12, 45, 12, 61, 12, 45,
	44, 173,
}

var pkwareLenLengths = []byte{2, 35, 36, 53, 38, 23}

var pkwareDistLengths = []byte{2, 20, 53, 230, 247, 151, 248}

var pkwareLenBase = [16]int{3, 2, 4, 5, 6, 7, 8, 9, 10, 12, 16, 24, 40, 72, 136, 264}

var pkwareLenExtra = [16]uint{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}

var (
	pkwareLitHuff  = newPKWAREHuffman(pkwareLitLengths)
	pkwareLenHuff  = newPKWAREHuffman(pkwareLenLengths)
	pkwareDistHuff = newPKWAREHuffman(pkwareDistLengths)
)

type pkwareCode struct {
	sym    int
	length uint
}

type pkwareHuffman struct {
	count  [pkwareMaxBits + 1]int
	symbol []int
	table  []pkwareCode
}

// newPKWAREHuffman 按参考实现构建 canonical 码表，并展开成 13 位查表。
// 码字按“读入位取反，由高位向低位累积”排列，所以查表索引要做位反转。
func newPKWAREHuffman(rep []byte) *pkwareHuffman {
	var lengths []int
	for _, b := range rep {
		for i := 0; i < int(b>>4)+1; i++ {
			lengths = append(lengths, int(b&15))
		}
	}
	h := &pkwareHuffman{
		symbol: make([]int, len(lengths)),
		table:  make([]pkwareCode, 1<<pkwareMaxBits),
	}
	for _, l := range lengths {
		h.count[l]++
	}
	offs := make([]int, pkwareMaxBits+2)
	for l := 1; l < pkwareMaxBits; l++ {
		offs[l+1] = offs[l] + h.count[l]
	}
	for sym, l := range lengths {
		if l != 0 {
			h.symbol[offs[l]] = sym
			offs[l]++
		}
	}
	for i := range h.table {
		h.table[i] = pkwareCode{sym: pkwareNoSymbol, length: pkwareMaxBits}
	}
	first, index := 0, 0
	for length := 1; length <= pkwareMaxBits; length++ {
		cnt := h.count[length]
		for k := 0; k < cnt; k++ {
			code := first + k
			low := 0
			for i := 0; i < length; i++ {
				low |= (((code >> (length - 1 - i)) & 1) ^ 1) << i
			}
			sym := h.symbol[index+k]
			for slot := low; slot < 1<<pkwareMaxBits; slot += 1 << length {
				h.table[slot] = pkwareCode{sym: sym, length: uint(length)}
			}
		}
		index += cnt
		first = (first + cnt) << 1
	}
	return h
}

type pkwareBits struct {
	data []byte
	pos  int
	buf  uint64
	cnt  uint
	err  error
}

func (r *pkwareBits) fill(need uint) {
	for r.cnt < need && r.pos < len(r.data) {
		r.buf |= uint64(r.data[r.pos]) << r.cnt
		r.pos++
		r.cnt += 8
	}
}

func (r *pkwareBits) bits(n uint) int {
	if n == 0 {
		return 0
	}
	if r.cnt < n {
		r.fill(n + 8)
		if r.cnt < n {
			r.err = io.ErrUnexpectedEOF
			return 0
		}
	}
	val := int(r.buf & (uint64(1)<<n - 1))
	r.buf >>= n
	r.cnt -= n
	return val
}

func (r *pkwareBits) decode(h *pkwareHuffman) int {
	if r.cnt < pkwareMaxBits {
		r.fill(pkwareMaxBits + 8)
	}
	if r.cnt >= pkwareMaxBits {
		code := h.table[r.buf&(1<<pkwareMaxBits-1)]
		if code.sym >= 0 {
			r.buf >>= code.length
			r.cnt -= code.length
			return code.sym
		}
	}
	return r.decodeSlow(h)
}

// decodeSlow 处理位流尾部不足 13 位的情况，逐位回退。
func (r *pkwareBits) decodeSlow(h *pkwareHuffman) int {
	code, first, index := 0, 0, 0
	for length := 1; length <= pkwareMaxBits; length++ {
		if r.cnt == 0 {
			r.fill(64)
			if r.cnt == 0 {
				return pkwareNoSymbol
			}
		}
		code |= (int(r.buf&1) ^ 1)
		r.buf >>= 1
		r.cnt--
		cnt := h.count[length]
		if code < first+cnt {
			return h.symbol[index+code-first]
		}
		index += cnt
		first = (first + cnt) << 1
		code <<= 1
	}
	return pkwareNoSymbol
}

// explodePKWARE 解出一段 DCL implode 数据；expectedSize>=0 时要求输出长度严格相等。
func explodePKWARE(data []byte, expectedSize int64) ([]byte, error) {
	r := &pkwareBits{data: data}
	literalsCoded := r.bits(8)
	if r.err != nil {
		return nil, r.err
	}
	if literalsCoded > 1 {
		return nil, fmt.Errorf("pkware: invalid literal flag %d", literalsCoded)
	}
	dictBits := uint(r.bits(8))
	if r.err != nil {
		return nil, r.err
	}
	if dictBits < 4 || dictBits > 6 {
		return nil, fmt.Errorf("pkware: invalid dictionary bits %d", dictBits)
	}
	capacity := 0
	if expectedSize > 0 && expectedSize < 1<<31 {
		capacity = int(expectedSize)
	}
	out := make([]byte, 0, capacity)
	for {
		if r.err != nil {
			return nil, r.err
		}
		if r.bits(1) == 1 {
			sym := r.decode(pkwareLenHuff)
			if sym < 0 || sym >= len(pkwareLenBase) {
				return nil, fmt.Errorf("pkware: length code decode failed")
			}
			length := pkwareLenBase[sym] + r.bits(pkwareLenExtra[sym])
			if r.err != nil {
				return nil, r.err
			}
			if length == pkwareEndLen {
				break
			}
			extra := dictBits
			if length == 2 {
				extra = 2
			}
			dist := r.decode(pkwareDistHuff)
			if dist < 0 {
				return nil, fmt.Errorf("pkware: distance code decode failed")
			}
			dist = (dist << extra) + r.bits(extra) + 1
			if r.err != nil {
				return nil, r.err
			}
			have := len(out)
			if dist > have {
				return nil, fmt.Errorf("pkware: distance %d exceeds output %d", dist, have)
			}
			// 逐字节回填：距离小于长度时会读到刚写入的字节，正是 DCL 的重叠拷贝语义。
			for i := 0; i < length; i++ {
				out = append(out, out[have-dist+i])
			}
		} else {
			var sym int
			if literalsCoded == 1 {
				sym = r.decode(pkwareLitHuff)
			} else {
				sym = r.bits(8)
			}
			if r.err != nil {
				return nil, r.err
			}
			if sym < 0 || sym > 0xff {
				return nil, fmt.Errorf("pkware: literal decode failed")
			}
			out = append(out, byte(sym))
		}
	}
	if expectedSize >= 0 && int64(len(out)) != expectedSize {
		return nil, fmt.Errorf("pkware: output %d bytes, want %d", len(out), expectedSize)
	}
	return out, nil
}
