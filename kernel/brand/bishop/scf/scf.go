package scf

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FeatureID     = "bishop.scf"
	FeatureName   = "Bishop-Scf"
	FormatName    = "SCF"
	FormatVersion = "1"
)

const (
	secHeader      = "scf.header"
	secEntries     = "scf.entries"
	secPoolOffsets = "scf.poolOffsets"
	secTransfer    = "scf.transfer"
	secJump        = "scf.jump"
	secPool        = "scf.pool"
	secCode        = "scf.code"
)

const (
	opBang  = 0x21
	opHash  = 0x23
	opLabel = 0x3B
	opWrite = 0x5B
	opLatch = 0x28
)

const (
	modeSeraph = 0
	modeByte   = 1
	modeWord   = 2
)

const maxMessageBit = 112294

type header struct {
	entries   int
	poolSize  int
	jumpCount int
	codeLen   int
	transfers int
}

type poolItem struct {
	flag byte
	text []byte
	expr []byte
}

type codeStr struct {
	letter byte
	strOff int
	rawLen int
}

type instr struct {
	start  int
	length int
	strIdx int
}

type container struct {
	h         header
	entryVals []uint16
	poolOff   []uint16
	transfer  []byte
	jump      []uint16
	pool      []byte
	code      []byte
	items     []poolItem
	instrs    []instr
	strs      []codeStr
}

func readHeader(data []byte) (header, error) {
	if len(data) < 10 {
		return header{}, errors.New("scf: too short for header")
	}
	h := header{
		entries:   int(binary.LittleEndian.Uint16(data[0:])),
		poolSize:  int(binary.LittleEndian.Uint16(data[2:])),
		jumpCount: int(binary.LittleEndian.Uint16(data[4:])),
		codeLen:   int(binary.LittleEndian.Uint16(data[6:])),
		transfers: int(binary.LittleEndian.Uint16(data[8:])),
	}
	return h, nil
}

func transferCountOK(h header) bool {
	if h.entries == 0 {
		return h.transfers == 1
	}
	return h.transfers == h.entries-1 || h.transfers == h.entries+1
}

type codeWalk struct {
	instrs   []instr
	strs     []codeStr
	prefixes []int
	bigV     bool
}

func cstringLen(code []byte, start int) (int, error) {
	if start < 0 || start > len(code) {
		return 0, fmt.Errorf("scf: string operand out of range at %d", start)
	}
	for i := start; i < len(code); i++ {
		if code[i] == 0 {
			return i + 1 - start, nil
		}
	}
	return 0, fmt.Errorf("scf: unterminated string operand at %d", start)
}

func skipItem(code []byte, idx int, litW int) (int, error) {
	c := idx + 1
	if c+4 > len(code) {
		return 0, fmt.Errorf("scf: truncated expression item at %d", idx)
	}
	c += 2
	c += 1
	op := code[c]
	c++
	for op == '+' || op == '-' {
		if c+4 > len(code) {
			return 0, fmt.Errorf("scf: truncated expression item at %d", idx)
		}
		c += 3
		op = code[c]
		c++
	}
	tail := litW
	if op == 'F' {
		tail = 5
	}
	if c+tail > len(code) {
		return 0, fmt.Errorf("scf: truncated expression item at %d", idx)
	}
	return c + tail, nil
}

func expressionLen(code []byte, start int, mode int, litW int) (int, error) {
	if mode == modeSeraph {
		i := start
		for i < len(code) {
			switch code[i] {
			case opWrite:
				if i+6 > len(code) {
					return 0, fmt.Errorf("scf: truncated write in expression at %d", i)
				}
				i += 6
				continue
			case opLabel:
				return i + 1 - start, nil
			}
			i++
		}
		return 0, fmt.Errorf("scf: unterminated expression at %d", start)
	}
	i := start
	for {
		if i >= len(code) {
			return 0, fmt.Errorf("scf: unterminated expression at %d", start)
		}
		switch code[i] {
		case opWrite:
			ni, err := skipItem(code, i, litW)
			if err != nil {
				return 0, err
			}
			i = ni
			continue
		case opLabel:
			return i + 1 - start, nil
		}
		i++
	}
}

func bangOperand(code []byte, pos int, mode int) (int, int, error) {
	if pos+1 >= len(code) {
		return 0, -1, fmt.Errorf("scf: truncated bang opcode at %d", pos)
	}
	if mode == modeSeraph {
		switch code[pos+1] {
		case 'c', 'r', 'w':
			return 0, -1, nil
		case 'b', 'e', 'g', 'j', 'n':
			if pos+4 > len(code) {
				return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
			}
			return 2, -1, nil
		case 'x':
			if pos+5 > len(code) {
				return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
			}
			return 3, -1, nil
		case 'a':
			if pos+6 > len(code) {
				return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
			}
			return 4, -1, nil
		case 'i', 'm', 's', 't':
			n, err := cstringLen(code, pos+2)
			if err != nil {
				return 0, -1, err
			}
			return n, pos + 2, nil
		case 'p':
			if pos+4 > len(code) {
				return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
			}
			n, err := cstringLen(code, pos+4)
			if err != nil {
				return 0, -1, err
			}
			return 2 + n, pos + 4, nil
		}
		return 0, -1, fmt.Errorf("scf: unknown bang opcode %q at %d", code[pos+1], pos)
	}
	switch code[pos+1] {
	case 'c', 'r', 'w':
		return 0, -1, nil
	case 'b', 'g', 'j', 'n':
		if pos+4 > len(code) {
			return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
		}
		return 2, -1, nil
	case 'e', 'v', 'x':
		if pos+5 > len(code) {
			return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
		}
		return 3, -1, nil
	case 'a':
		if pos+6 > len(code) {
			return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
		}
		return 4, -1, nil
	case 'i', 'm', 't':
		n, err := cstringLen(code, pos+2)
		if err != nil {
			return 0, -1, err
		}
		return n, pos + 2, nil
	case 's':
		if pos+5 > len(code) {
			return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
		}
		n, err := cstringLen(code, pos+5)
		if err != nil {
			return 0, -1, err
		}
		return 3 + n, pos + 5, nil
	case 'p':
		if pos+4 > len(code) {
			return 0, -1, fmt.Errorf("scf: truncated bang operand at %d", pos)
		}
		n, err := cstringLen(code, pos+4)
		if err != nil {
			return 0, -1, err
		}
		return 2 + n, pos + 4, nil
	}
	return 0, -1, fmt.Errorf("scf: unknown bang opcode %q at %d", code[pos+1], pos)
}

func hashFixed(code []byte, start int, count int) error {
	if start < 0 || start+count > len(code) {
		return fmt.Errorf("scf: truncated hash operand at %d", start)
	}
	return nil
}

func hashOperand(code []byte, pos int, mode int, litW int) (int, error) {
	if pos+1 >= len(code) {
		return 0, fmt.Errorf("scf: truncated hash opcode at %d", pos)
	}
	if mode == modeSeraph {
		switch code[pos+1] {
		case 'e', 'l', 'n', 'r', 'w':
			return 0, nil
		case 'g':
			if pos+4 > len(code) {
				return 0, fmt.Errorf("scf: truncated hash operand at %d", pos)
			}
			return 2, nil
		case 'i':
			n, err := expressionLen(code, pos+2, mode, litW)
			if err != nil {
				return 0, err
			}
			return n, nil
		}
		return 0, fmt.Errorf("scf: unknown hash opcode %q at %d", code[pos+1], pos)
	}
	peek := func(i int) byte {
		if i < 0 || i >= len(code) {
			return 0
		}
		return code[i]
	}
	switch code[pos+1] {
	case 'e', 'n':
		return 0, nil
	case 'W':
		if err := hashFixed(code, pos+2, 1); err != nil {
			return 0, err
		}
		return 1, nil
	case 'g':
		if err := hashFixed(code, pos+2, 2); err != nil {
			return 0, err
		}
		return 2, nil
	case 'i':
		n, err := expressionLen(code, pos+2, mode, litW)
		if err != nil {
			return 0, err
		}
		return n, nil
	case 'b':
		switch peek(pos + 2) {
		case 'd':
			if err := hashFixed(code, pos+2, 1); err != nil {
				return 0, err
			}
			return 1, nil
		case 'i', 'o':
			if err := hashFixed(code, pos+2, 2); err != nil {
				return 0, err
			}
			return 2, nil
		default:
			if err := hashFixed(code, pos+2, 1); err != nil {
				return 0, err
			}
			return 1, nil
		}
	case 's':
		if err := hashFixed(code, pos+2, 1); err != nil {
			return 0, err
		}
		return 1, nil
	case 'l':
		if peek(pos+2) == 'i' {
			n, err := expressionLen(code, pos+3, mode, litW)
			if err != nil {
				return 0, err
			}
			return 1 + n, nil
		}
		return 0, nil
	case 'r':
		if pos+6 <= len(code) && code[pos+2] == 'e' && code[pos+3] == 's' && code[pos+4] == 'e' && code[pos+5] == 't' {
			return 4, nil
		}
		return 0, nil
	case 't':
		switch peek(pos + 2) {
		case 'w':
			if err := hashFixed(code, pos+3, 2); err != nil {
				return 0, err
			}
			return 3, nil
		case 't':
			if err := hashFixed(code, pos+3, 1); err != nil {
				return 0, err
			}
			return 2, nil
		default:
			if err := hashFixed(code, pos+2, 1); err != nil {
				return 0, err
			}
			return 1, nil
		}
	case 'w':
		if peek(pos+2) == 'b' {
			return 1, nil
		}
		return 0, nil
	case 'x':
		if peek(pos+2) == 'l' {
			if err := hashFixed(code, pos+3, 2); err != nil {
				return 0, err
			}
			return 3, nil
		}
		if err := hashFixed(code, pos+2, 1); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return 0, fmt.Errorf("scf: unknown hash opcode %q at %d", code[pos+1], pos)
}

func writeLength(code []byte, pos int, litW int) (int, []int, error) {
	var vals []int
	c := pos + 1
	if c+4 > len(code) {
		return 0, nil, fmt.Errorf("scf: truncated write at %d", pos)
	}
	c += 2
	c += 1
	c += 1
	for {
		for c < len(code) && code[c] == '(' {
			c++
		}
		if c >= len(code) {
			return 0, nil, fmt.Errorf("scf: truncated write at %d", pos)
		}
		switch code[c] {
		case 'F':
			c += 3
		case 'V':
			c++
			if c+litW > len(code) {
				return 0, nil, fmt.Errorf("scf: truncated write at %d", pos)
			}
			val := 0
			for k := 0; k < litW; k++ {
				val = val<<8 | int(code[c])
				c++
			}
			vals = append(vals, val)
		default:
			c++
		}
		if c >= len(code) {
			return 0, nil, fmt.Errorf("scf: truncated write at %d", pos)
		}
		op := code[c]
		c++
		for c < len(code) && code[c] == ')' {
			c++
		}
		if op == 0 {
			return c - pos, vals, nil
		}
		if c >= len(code) {
			return 0, nil, fmt.Errorf("scf: truncated write at %d", pos)
		}
	}
}

func walkCode(code []byte, mode int) (codeWalk, error) {
	var w codeWalk
	litW := 1
	if mode == modeWord {
		litW = 2
	}
	pos := 0
	for pos < len(code) {
		switch code[pos] {
		case opBang:
			n, s, err := bangOperand(code, pos, mode)
			if err != nil {
				return codeWalk{}, err
			}
			idx := -1
			if s >= 0 {
				idx = len(w.strs)
				w.strs = append(w.strs, codeStr{letter: code[pos+1], strOff: s, rawLen: n - (s - pos - 2)})
				if mode != modeSeraph && code[pos+1] == 's' {
					w.prefixes = append(w.prefixes, int(code[s-3])|int(code[s-2])<<8|int(code[s-1])<<16)
				}
			}
			w.instrs = append(w.instrs, instr{start: pos, length: 2 + n, strIdx: idx})
			pos += 2 + n
		case opHash:
			n, err := hashOperand(code, pos, mode, litW)
			if err != nil {
				return codeWalk{}, err
			}
			w.instrs = append(w.instrs, instr{start: pos, length: 2 + n, strIdx: -1})
			pos += 2 + n
		case opLabel:
			w.instrs = append(w.instrs, instr{start: pos, length: 1, strIdx: -1})
			pos++
		case opWrite:
			if mode == modeSeraph {
				if pos+6 > len(code) {
					return codeWalk{}, fmt.Errorf("scf: truncated write at %d", pos)
				}
				w.instrs = append(w.instrs, instr{start: pos, length: 6, strIdx: -1})
				pos += 6
				continue
			}
			n, vals, err := writeLength(code, pos, litW)
			if err != nil {
				return codeWalk{}, err
			}
			for _, v := range vals {
				if v > 0xFF {
					w.bigV = true
				}
			}
			w.instrs = append(w.instrs, instr{start: pos, length: n, strIdx: -1})
			pos += n
		case opLatch:
			if pos+3 > len(code) {
				return codeWalk{}, fmt.Errorf("scf: truncated latch at %d", pos)
			}
			w.instrs = append(w.instrs, instr{start: pos, length: 3, strIdx: -1})
			pos += 3
		default:
			w.instrs = append(w.instrs, instr{start: pos, length: 1, strIdx: -1})
			pos++
		}
	}
	return w, nil
}

func validPrefixes(p []int) bool {
	if len(p) == 0 {
		return false
	}
	seen := make(map[int]bool, len(p))
	for _, v := range p {
		if v < 0 || v > maxMessageBit {
			return false
		}
		if seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}

func pickWidth(bW codeWalk, bErr error, wW codeWalk, wErr error) ([]instr, []codeStr, error) {
	if bErr == nil && wErr == nil {
		if wW.bigV {
			return bW.instrs, bW.strs, nil
		}
		return wW.instrs, wW.strs, nil
	}
	if bErr == nil {
		return bW.instrs, bW.strs, nil
	}
	if wErr == nil {
		return wW.instrs, wW.strs, nil
	}
	return nil, nil, bErr
}

func detectCode(code []byte) ([]instr, []codeStr, error) {
	sW, sErr := walkCode(code, modeSeraph)
	bW, bErr := walkCode(code, modeByte)
	wW, wErr := walkCode(code, modeWord)
	if sErr == nil {
		if bErr == nil && validPrefixes(bW.prefixes) {
			return pickWidth(bW, bErr, wW, wErr)
		}
		if wErr == nil && validPrefixes(wW.prefixes) {
			return wW.instrs, wW.strs, nil
		}
		return sW.instrs, sW.strs, nil
	}
	if bErr == nil || wErr == nil {
		return pickWidth(bW, bErr, wW, wErr)
	}
	if sErr != nil {
		return nil, nil, sErr
	}
	if bErr != nil {
		return nil, nil, bErr
	}
	return nil, nil, wErr
}

func splitPool(pool []byte, offsets []uint16) ([]poolItem, error) {
	n := len(offsets)
	items := make([]poolItem, n)
	seen := make(map[int]int, n)
	for i, o := range offsets {
		off := int(o)
		if off < 0 || off >= len(pool) {
			return nil, fmt.Errorf("scf: pool offset %d out of range", off)
		}
		if prev, dup := seen[off]; dup {
			return nil, fmt.Errorf("scf: duplicate pool offset %d at entries %d and %d", off, prev, i)
		}
		seen[off] = i
		if i > 0 && off < int(offsets[i-1]) {
			return nil, fmt.Errorf("scf: pool offsets out of order at entry %d", i)
		}
	}
	for i, o := range offsets {
		off := int(o)
		end := len(pool)
		if i+1 < n {
			end = int(offsets[i+1])
		}
		seg := pool[off:end]
		if len(seg) < 2 {
			return nil, fmt.Errorf("scf: pool entry %d too short", i)
		}
		flag := seg[0]
		rest := seg[1:]
		nul := -1
		for k, c := range rest {
			if c == 0 {
				nul = k
				break
			}
		}
		if nul < 0 {
			return nil, fmt.Errorf("scf: pool entry %d unterminated", i)
		}
		after := rest[nul+1:]
		var expr []byte
		if flag == 0 {
			k := 0
			done := false
			for k < len(after) {
				switch after[k] {
				case opWrite:
					if k+6 > len(after) {
						return nil, fmt.Errorf("scf: pool entry %d truncated expression", i)
					}
					k += 6
					continue
				case opLabel:
					expr = after[:k]
					after = after[k+1:]
					done = true
				}
				if done {
					break
				}
				k++
			}
			if !done {
				return nil, fmt.Errorf("scf: pool entry %d unterminated expression", i)
			}
		}
		if len(after) != 0 {
			return nil, fmt.Errorf("scf: pool entry %d has trailing bytes", i)
		}
		items[i] = poolItem{flag: flag, text: rest[:nul], expr: expr}
	}
	return items, nil
}

func parse(data []byte) (*container, error) {
	h, err := readHeader(data)
	if err != nil {
		return nil, err
	}
	if !transferCountOK(h) {
		return nil, fmt.Errorf("scf: invalid transfer count %d for %d entries", h.transfers, h.entries)
	}
	if h.codeLen <= 0 {
		return nil, errors.New("scf: empty code area")
	}
	total := 10 + 6*h.entries + 2*h.entries + 4*h.transfers + 2*h.jumpCount + h.poolSize + h.codeLen
	if total != len(data) {
		return nil, fmt.Errorf("scf: length mismatch, header describes %d but data has %d", total, len(data))
	}
	c := &container{h: h}
	pos := 10
	c.entryVals = make([]uint16, 3*h.entries)
	for i := range c.entryVals {
		c.entryVals[i] = binary.LittleEndian.Uint16(data[pos:])
		pos += 2
	}
	c.poolOff = make([]uint16, h.entries)
	for i := range c.poolOff {
		c.poolOff[i] = binary.LittleEndian.Uint16(data[pos:])
		pos += 2
	}
	c.transfer = append([]byte(nil), data[pos:pos+4*h.transfers]...)
	pos += 4 * h.transfers
	c.jump = make([]uint16, h.jumpCount)
	for i := range c.jump {
		v := int(binary.LittleEndian.Uint16(data[pos:]))
		if v < 0 || v > h.codeLen {
			return nil, fmt.Errorf("scf: jump target %d out of range", v)
		}
		c.jump[i] = uint16(v)
		pos += 2
	}
	if h.transfers > 0 && h.jumpCount > 0 {
		for t := 0; t < h.transfers; t++ {
			base := int(binary.LittleEndian.Uint16(c.transfer[4*t+2:]))
			if base < 0 || base >= h.jumpCount {
				return nil, fmt.Errorf("scf: transfer base %d out of range", base)
			}
		}
	}
	c.pool = append([]byte(nil), data[pos:pos+h.poolSize]...)
	pos += h.poolSize
	c.code = append([]byte(nil), data[pos:pos+h.codeLen]...)
	instrs, strs, err := detectCode(c.code)
	if err != nil {
		return nil, err
	}
	c.instrs = instrs
	c.strs = strs
	items, err := splitPool(c.pool, c.poolOff)
	if err != nil {
		return nil, err
	}
	c.items = items
	return c, nil
}
