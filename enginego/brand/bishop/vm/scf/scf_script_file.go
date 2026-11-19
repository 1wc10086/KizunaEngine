package scf

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type ScfScriptFile struct {
	data           []byte
	header         ScfHeader
	entries        []ScfEntry
	poolOffsetBase int
	transferBase   int
	jumpBase       int
	poolBase       int
	codeBase       int
}

func newScfScriptFile(data []byte, header ScfHeader) *ScfScriptFile {
	file := &ScfScriptFile{data: data, header: header, entries: make([]ScfEntry, header.EntryCount)}
	for index := range file.entries {
		offset := ScfHeaderSize + (6 * index)
		file.entries[index] = ScfEntry{
			Head:     int(binary.LittleEndian.Uint16(data[offset:])),
			Next:     int(binary.LittleEndian.Uint16(data[offset+2:])),
			Variable: int(binary.LittleEndian.Uint16(data[offset+4:])),
		}
	}
	file.poolOffsetBase = ScfHeaderSize + (6 * header.EntryCount)
	file.transferBase = file.poolOffsetBase + (2 * header.EntryCount)
	file.jumpBase = file.transferBase + (4 * header.TransferCount)
	file.poolBase = file.jumpBase + (2 * header.JumpTargetCount)
	file.codeBase = file.poolBase + header.PoolSize
	return file
}

func LoadScfScriptFile(data []byte) (*ScfScriptFile, error) {
	header, ok := ReadScfHeader(data)
	if !ok {
		return nil, fmt.Errorf("Data is too short for an SCF header.")
	}
	if !header.IsTransferCountValid() {
		return nil, fmt.Errorf("Invalid SCF transfer count %d for %d entries.", header.TransferCount, header.EntryCount)
	}
	if header.TotalLength() != len(data) {
		return nil, fmt.Errorf("SCF length mismatch: header describes %d bytes but data has %d bytes.", header.TotalLength(), len(data))
	}
	return newScfScriptFile(data, header), nil
}

func (f *ScfScriptFile) Name() string {
	return ""
}

func (f *ScfScriptFile) Signature() string {
	return ScfSignatureValue
}

func (f *ScfScriptFile) CodeLength() int {
	return f.header.CodeLength
}

func (f *ScfScriptFile) Code() []byte {
	return f.data[f.codeBase : f.codeBase+f.header.CodeLength]
}

func (f *ScfScriptFile) EntryCount() int {
	return f.header.EntryCount
}

func (f *ScfScriptFile) Entries() []ScfEntry {
	return f.entries
}

func (f *ScfScriptFile) JumpTargetCount() int {
	return f.header.JumpTargetCount
}

func (f *ScfScriptFile) JumpTarget(index int) int {
	if uint(index) >= uint(f.header.JumpTargetCount) {
		return 0
	}
	return int(binary.LittleEndian.Uint16(f.data[f.jumpBase+(2*index):]))
}

func (f *ScfScriptFile) TransferCount() int {
	return f.header.TransferCount
}

func (f *ScfScriptFile) Match(transfer int) byte {
	offset, ok := f.transferOffset(transfer)
	if !ok {
		return 0
	}
	return f.data[offset]
}

func (f *ScfScriptFile) Set(transfer int) byte {
	offset, ok := f.transferOffset(transfer)
	if !ok {
		return 0
	}
	return f.data[offset+1]
}

func (f *ScfScriptFile) TransferBase(transfer int) int {
	offset, ok := f.transferOffset(transfer)
	if !ok {
		return 0
	}
	return int(binary.LittleEndian.Uint16(f.data[offset+2:]))
}

func (f *ScfScriptFile) PoolOffset(index int) int {
	if uint(index) >= uint(f.header.EntryCount) {
		return 0
	}
	return int(binary.LittleEndian.Uint16(f.data[f.poolOffsetBase+(2*index):]))
}

func (f *ScfScriptFile) PoolFlag(poolOffset int) (byte, error) {
	tail, err := f.poolTail(poolOffset)
	if err != nil {
		return 0, err
	}
	return tail[0], nil
}

func (f *ScfScriptFile) PoolText(poolOffset int) (string, error) {
	tail, err := f.poolTail(poolOffset)
	if err != nil {
		return "", err
	}
	end := bytes.IndexByte(tail[1:], 0)
	if end < 0 {
		return "", fmt.Errorf("Unterminated SCF pool text at pool offset %d.", poolOffset)
	}
	slice := tail[1 : 1+end]
	return DecodeText(slice, DetectTextEncoding(slice)), nil
}

func (f *ScfScriptFile) PoolExpression(poolOffset int) (string, error) {
	tail, err := f.poolTail(poolOffset)
	if err != nil {
		return "", err
	}
	if tail[0] != 0 {
		return "", nil
	}
	textEnd := bytes.IndexByte(tail[1:], 0)
	if textEnd < 0 {
		return "", fmt.Errorf("Unterminated SCF pool text at pool offset %d.", poolOffset)
	}
	length, err := ScfExpressionLength(tail, textEnd+2)
	if err != nil {
		return "", err
	}
	slice := tail[textEnd+2 : textEnd+2+length]
	return DecodeText(slice, DetectTextEncoding(slice)), nil
}

func (f *ScfScriptFile) Data() []byte {
	return f.data
}

func (f *ScfScriptFile) transferOffset(transfer int) (int, bool) {
	if uint(transfer) >= uint(f.header.TransferCount) {
		return 0, false
	}
	return f.transferBase + (4 * transfer), true
}

func (f *ScfScriptFile) poolTail(poolOffset int) ([]byte, error) {
	if uint(poolOffset) >= uint(f.header.PoolSize) {
		return nil, fmt.Errorf("SCF pool offset out of range: %d.", poolOffset)
	}
	return f.data[f.poolBase+poolOffset : f.poolBase+f.header.PoolSize], nil
}
