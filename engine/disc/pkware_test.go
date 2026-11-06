package disc

import "testing"

// pkwareTestWriter 按 LSB-first 写位流，与 pkwareBits 读取方向一致。
type pkwareTestWriter struct {
	out []byte
	n   int
}

func (w *pkwareTestWriter) bits(value, count int) {
	for i := 0; i < count; i++ {
		if w.n%8 == 0 {
			w.out = append(w.out, 0)
		}
		if (value>>i)&1 == 1 {
			w.out[len(w.out)-1] |= 1 << (w.n % 8)
		}
		w.n++
	}
}

// code 写入霍夫曼码字：解码端按 MSB 到 LSB 的顺序读取并取反。
func (w *pkwareTestWriter) code(rep []byte, sym int) {
	code, length := pkwareTestCanonical(rep, sym)
	for i := length - 1; i >= 0; i-- {
		w.bits(((code>>i)&1)^1, 1)
	}
}

// pkwareTestCanonical 与 newPKWAREHuffman 使用同一套 canonical 分配规则。
func pkwareTestCanonical(rep []byte, want int) (int, int) {
	var lengths []int
	for _, b := range rep {
		for i := 0; i < int(b>>4)+1; i++ {
			lengths = append(lengths, int(b&15))
		}
	}
	var count [pkwareMaxBits + 1]int
	for _, l := range lengths {
		count[l]++
	}
	first := 0
	for length := 1; length <= pkwareMaxBits; length++ {
		k := 0
		for sym, l := range lengths {
			if l == length {
				if sym == want {
					return first + k, length
				}
				k++
			}
		}
		first = (first + count[length]) << 1
	}
	return -1, -1
}

func pkwareTestHeader(w *pkwareTestWriter) {
	w.bits(0, 8)
	w.bits(4, 8)
}

func pkwareTestEnd(w *pkwareTestWriter) {
	w.bits(1, 1)
	w.code(pkwareLenLengths, 15)
	w.bits(255, 8)
}

func TestExplodePKWARELiterals(t *testing.T) {
	var w pkwareTestWriter
	pkwareTestHeader(&w)
	for _, c := range []byte("ABC") {
		w.bits(0, 1)
		w.bits(int(c), 8)
	}
	pkwareTestEnd(&w)
	got, err := explodePKWARE(w.out, 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ABC" {
		t.Errorf("got %q, want ABC", got)
	}
}

func TestExplodePKWAREOverlappingCopy(t *testing.T) {
	var w pkwareTestWriter
	pkwareTestHeader(&w)
	for _, c := range []byte("AB") {
		w.bits(0, 1)
		w.bits(int(c), 8)
	}
	// 长度 4、距离 2：解码端要读到刚写入的字节，得到 ABABAB。
	w.bits(1, 1)
	w.code(pkwareLenLengths, 2)
	w.code(pkwareDistLengths, 0)
	w.bits(1, 4)
	pkwareTestEnd(&w)
	got, err := explodePKWARE(w.out, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ABABAB" {
		t.Errorf("got %q, want ABABAB", got)
	}
}

func TestExplodePKWAREBackReference(t *testing.T) {
	var w pkwareTestWriter
	pkwareTestHeader(&w)
	for _, c := range []byte("ABC") {
		w.bits(0, 1)
		w.bits(int(c), 8)
	}
	// 长度 3、距离 3
	w.bits(1, 1)
	w.code(pkwareLenLengths, 0)
	w.code(pkwareDistLengths, 0)
	w.bits(2, 4)
	pkwareTestEnd(&w)
	got, err := explodePKWARE(w.out, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ABCABC" {
		t.Errorf("got %q, want ABCABC", got)
	}
}

func TestExplodePKWAREErrors(t *testing.T) {
	if _, err := explodePKWARE([]byte{2, 4, 0}, -1); err == nil {
		t.Error("literal flag 2 should fail")
	}
	if _, err := explodePKWARE([]byte{0, 7, 0}, -1); err == nil {
		t.Error("dictionary bits 7 should fail")
	}
	var w pkwareTestWriter
	pkwareTestHeader(&w)
	w.bits(0, 1)
	w.bits('A', 8)
	pkwareTestEnd(&w)
	if _, err := explodePKWARE(w.out, 2); err == nil {
		t.Error("size mismatch should fail")
	}
}
