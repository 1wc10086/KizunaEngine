package eme

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"kizuna/kernel/core/textcodec"
)

const (
	magicSize  = 8
	recordSize = 0x60
	nameSize   = 0x40
)

var emeMagic = []byte("RREDATA ")

type entry struct {
	Name         string
	RawName      []byte
	FrameSize    int
	RawPos       int
	Flags        uint32
	Tail         [8]byte
	SubType      int
	Size         int
	UnpackedSize int
	Offset       int
}

func (e entry) FrameInitPos() int {
	if e.FrameSize == 0 {
		return 0
	}
	return (e.FrameSize - e.RawPos) % e.FrameSize
}

func parseArchive(data []byte) ([]byte, []entry, error) {
	if len(data) < magicSize+4 {
		return nil, nil, errors.New("eme: archive too short")
	}
	if !bytes.Equal(data[:magicSize], emeMagic) {
		return nil, nil, errors.New("eme: missing RREDATA magic")
	}
	tail := binary.LittleEndian.Uint32(data[len(data)-4:])
	count := int(tail)
	idxOff := len(data) - 4 - count*recordSize
	if count <= 0 || idxOff-keySize < 0 {
		return nil, nil, fmt.Errorf("eme: corrupt index (count=%d, idxOff=%d)", count, idxOff)
	}
	key := make([]byte, keySize)
	copy(key, data[idxOff-keySize:idxOff])
	rawIdx := make([]byte, count*recordSize)
	copy(rawIdx, data[idxOff:idxOff+count*recordSize])
	entries := make([]entry, count)
	for i := 0; i < count; i++ {
		block := make([]byte, recordSize)
		copy(block, rawIdx[i*recordSize:(i+1)*recordSize])
		decrypt(block, key)
		nameEnd := bytes.IndexByte(block[:nameSize], 0)
		if nameEnd < 0 {
			nameEnd = nameSize
		}
		if nameEnd == 0 {
			return nil, nil, fmt.Errorf("eme: empty entry name at index %d", i)
		}
		rawName := make([]byte, nameEnd)
		copy(rawName, block[:nameEnd])
		fs := int(binary.LittleEndian.Uint16(block[0x40:0x42]))
		rp := int(binary.LittleEndian.Uint16(block[0x42:0x44]))
		flags := binary.LittleEndian.Uint32(block[0x44:0x48])
		sub := int(int32(binary.LittleEndian.Uint32(block[0x48:0x4C])))
		size := int(binary.LittleEndian.Uint32(block[0x4C:0x50]))
		usize := int(binary.LittleEndian.Uint32(block[0x50:0x54]))
		off := int(binary.LittleEndian.Uint32(block[0x54:0x58]))
		var tail [8]byte
		copy(tail[:], block[0x58:0x60])
		entries[i] = entry{
			Name:         textcodec.DecodeCP932(rawName),
			RawName:      rawName,
			FrameSize:    fs,
			RawPos:       rp,
			Flags:        flags,
			Tail:         tail,
			SubType:      sub,
			Size:         size,
			UnpackedSize: usize,
			Offset:       off,
		}
	}
	return key, entries, nil
}

func looksLikeArchive(data []byte) bool {
	if len(data) < magicSize+4 {
		return false
	}
	if !bytes.Equal(data[:magicSize], emeMagic) {
		return false
	}
	count := binary.LittleEndian.Uint32(data[len(data)-4:])
	return count > 0 && int(count)*recordSize+keySize+4 <= len(data)
}

func decryptScriptHeaderBytes(data []byte, off int, key []byte) (packed, part2 uint32) {
	hdr := make([]byte, 12)
	copy(hdr, data[off:off+12])
	decrypt(hdr, key)
	packed = binary.LittleEndian.Uint32(hdr[0:4])
	part2 = binary.LittleEndian.Uint32(hdr[4:8])
	return
}
