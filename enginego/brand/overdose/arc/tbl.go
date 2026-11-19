package arc

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TBL 索引表（overdose_spec §4）是 system.arc 的台账来源。
// layout：
//
//	+00 "TBL\0"
//	+04 压缩后载荷长度
//	+08 解压后载荷长度
//	+0c 版本/标志（实测 1）
//	+10 记录条数
//	+14 zlib 载荷
//
// 记录宽：列脚本表与 CG 表都是 160 字节只用前 80 字节是哈希+名字；
// 场景页表是 40960 一条子表数 256，只认 +10 的前若干条子表。勿按“整文件÷160”
// 算条数——页表会把无效子表也看进去，gallery 会出现大量空名字。
const (
	tblMagic     = "TBL\x00"
	tblHeader    = 0x14
	recSize      = 160
	pageRecSize  = 40960
	pageSubSize  = 160
	pageSlots    = 256
	nameFieldLen = 76
)

// Record 是索引表一行的精简投影：哈希（用于校验）与名字（用于定位）。
// Cells 只在页表（40960 字节/条）里填：一行的 256 个子格里非空的单元格，
// 场景鉴赏要靠第 1/2 列取回放脚本与 label，CG 行则有多张同组变体。
type Record struct {
	Hash  uint32
	Name  string
	Cells []string
}

// ParseTable 解一个 TBL 流回记录列（页表与普通表按记录宽自动区分）。
// 压缩/未压缩长度只作一致性校验，不影响按数解出的记录数；
// 若实际载荷和头所报对不上，zlib 读出的长短可能不匹配，同样报错。
func ParseTable(b []byte) ([]Record, error) {
	if len(b) < tblHeader {
		return nil, fmt.Errorf("tbl: 头不足 (%d 字节)", len(b))
	}
	if string(b[0:4]) != tblMagic {
		return nil, fmt.Errorf("tbl: 魔数不符 %q，期望 TBL\\0", b[0:4])
	}
	// compressSize := binary.LittleEndian.Uint32(b[4:8]) // 当前不校验，此处仅留数
	// decSize := binary.LittleEndian.Uint32(b[8:12])
	// flags := binary.LittleEndian.Uint32(b[12:16])
	cnt := binary.LittleEndian.Uint32(b[16:20])
	body := b[tblHeader:]
	// 载荷期：tbl 在 system.arc 里大都已 zlib 解过一层（Entry 已处理），
	// 直接落成解压后；若刚好是 zlib 魔数 0x78 就再解一次。
	var plain []byte
	if len(body) > 0 && body[0] == 0x78 {
		if z, err := inflate(body); err == nil {
			plain = z
		} else {
			plain = body
		}
	} else {
		plain = body
	}
	if cnt == 0 {
		return nil, fmt.Errorf("tbl: 记录数 0")
	}
	perRec := 0
	if len(plain) > 0 {
		perRec = len(plain) / int(cnt)
	}
	if perRec == 0 {
		return nil, fmt.Errorf("tbl: 载荷太短，放不下 %d 条记录", cnt)
	}
	out := make([]Record, 0, int(cnt))
	visitor := 0
	if perRec == pageRecSize {
		// 页表：每条 40960 里只有前 cnt 条子表有效（见注），按 sub 160 解析
		if int(cnt) > pageSlots {
			return nil, fmt.Errorf("tbl: 页表条目数 %d 超过子表数 %d", cnt, pageSlots)
		}
		for i := 0; i < int(cnt); i++ {
			rec := plain[i*pageSubSize : (i+1)*pageSubSize]
			h := binary.LittleEndian.Uint32(rec[0:4])
			nameBytes := rec[4 : 4+nameFieldLen]
			if j := bytes.IndexByte(nameBytes, 0); j >= 0 {
				nameBytes = nameBytes[:j]
			}
			name := decodeCP932(nameBytes)
			out = append(out, Record{Hash: h, Name: name, Cells: pageCells(plain, i)})
			visitor++
			_ = visitor
		}
		return out, nil
	}
	if perRec < recSize {
		return nil, fmt.Errorf("tbl: 记录宽 %d 小于 %d", perRec, recSize)
	}
	for i := 0; i < int(cnt); i++ {
		rec := plain[i*recSize : (i+1)*recSize]
		h := binary.LittleEndian.Uint32(rec[0:4])
		nameBytes := rec[4 : 4+nameFieldLen]
		if j := bytes.IndexByte(nameBytes, 0); j >= 0 {
			nameBytes = nameBytes[:j]
		}
		name := decodeCP932(nameBytes)
		out = append(out, Record{Hash: h, Name: name})
	}
	return out, nil
}

// pageCells 解页表第 row 行的 256 个子格（160 字节/格，名字在 +4）。
// 越界或全空返回 nil；只保留到最后一个非空格，避免把 256 个空串塞给调用方。
func pageCells(plain []byte, row int) []string {
	base := row * pageRecSize
	if base < 0 || base+pageRecSize > len(plain) {
		return nil
	}
	cells := make([]string, 0, pageSlots)
	last := -1
	for col := 0; col < pageSlots; col++ {
		rec := plain[base+col*pageSubSize : base+(col+1)*pageSubSize]
		nameBytes := rec[4 : 4+nameFieldLen]
		if j := bytes.IndexByte(nameBytes, 0); j >= 0 {
			nameBytes = nameBytes[:j]
		}
		name := decodeCP932(nameBytes)
		cells = append(cells, name)
		if name != "" {
			last = col
		}
	}
	if last < 0 {
		return nil
	}
	return cells[:last+1]
}
