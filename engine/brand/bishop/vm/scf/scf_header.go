package scf

import "encoding/binary"

const ScfHeaderSize = 10

type ScfHeader struct {
	EntryCount      int
	PoolSize        int
	JumpTargetCount int
	CodeLength      int
	TransferCount   int
}

func ReadScfHeader(data []byte) (ScfHeader, bool) {
	if len(data) < ScfHeaderSize {
		return ScfHeader{}, false
	}
	return ScfHeader{
		EntryCount:      int(binary.LittleEndian.Uint16(data[0:])),
		PoolSize:        int(binary.LittleEndian.Uint16(data[2:])),
		JumpTargetCount: int(binary.LittleEndian.Uint16(data[4:])),
		CodeLength:      int(binary.LittleEndian.Uint16(data[6:])),
		TransferCount:   int(binary.LittleEndian.Uint16(data[8:])),
	}, true
}

func (h ScfHeader) IsTransferCountValid() bool {
	plus := 1
	minus := 1
	if h.EntryCount > 0 {
		plus = h.EntryCount + 1
		minus = h.EntryCount - 1
	}
	return h.TransferCount == plus || h.TransferCount == minus
}

func (h ScfHeader) TotalLength() int {
	return ScfHeaderSize + (6 * h.EntryCount) + (2 * h.EntryCount) + (4 * h.TransferCount) + (2 * h.JumpTargetCount) + h.PoolSize + h.CodeLength
}
