package resource

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"

	"kizuna/engine/core/binutil"
)

const (
	PackPkExtension     = ".pk"
	packPkHeaderSize    = 12
	packPkEntrySize     = 0x230
	packPkNameReadSize  = 0x200
	packPkOffsetField   = 0x204
	packPkSizeField     = 0x208
	packPkMaxEntryCount = 1 << 20
)

var PackPkMagic = []byte{0x8C, 0x8E, 0x50, 0x4B}

type PackPkArchive struct {
	name    string
	length  int64
	entries []ArchiveEntry
	stream  *os.File
}

func (a *PackPkArchive) Name() string {
	return a.name
}

func (a *PackPkArchive) Version() int {
	return 1
}

func (a *PackPkArchive) Length() int64 {
	return a.length
}

func (a *PackPkArchive) Entries() []ArchiveEntry {
	return a.entries
}

func PackPkIsArchive(path string) bool {
	stream, err := os.Open(path)
	if err != nil {
		return false
	}
	defer stream.Close()

	header := make([]byte, packPkHeaderSize)
	if readAt(stream, header, 0) != len(header) {
		return false
	}

	if !bytes.Equal(header[:4], PackPkMagic) {
		return false
	}

	count := binutil.U32(header[4:])
	return count > 0 && count <= packPkMaxEntryCount
}

func PackPkOpen(path string) (*PackPkArchive, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	length := fileLength(stream)
	if length < packPkHeaderSize {
		_ = stream.Close()
		return nil, fmt.Errorf("truncated PACK.PK header: %s", path)
	}

	header := make([]byte, packPkHeaderSize)
	if readAt(stream, header, 0) != len(header) || !bytes.Equal(header[:4], PackPkMagic) {
		_ = stream.Close()
		return nil, fmt.Errorf("not a PACK.PK archive: %s", path)
	}

	count := binutil.U32(header[4:])
	if count == 0 || count > packPkMaxEntryCount || packPkHeaderSize+int64(count)*packPkEntrySize > length {
		_ = stream.Close()
		return nil, fmt.Errorf("corrupt PACK.PK index: %s", path)
	}

	entries, err := packPkReadEntries(stream, int(count), length)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}

	return &PackPkArchive{
		name:    pathFileName(path),
		length:  length,
		entries: entries,
		stream:  stream,
	}, nil
}

func (a *PackPkArchive) Read(offset int64, buffer []byte) int {
	return readAt(a.stream, buffer, offset)
}

func (a *PackPkArchive) ReadAll(entry ArchiveEntry) ([]byte, error) {
	if entry.Size < 0 {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	buffer := make([]byte, entry.Size)
	if a.Read(entry.Offset, buffer) != len(buffer) {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	return buffer, nil
}

func (a *PackPkArchive) Dispose() {
	_ = a.stream.Close()
}

func packPkReadEntries(stream *os.File, count int, length int64) ([]ArchiveEntry, error) {
	table := make([]byte, count*packPkEntrySize)
	if readAt(stream, table, packPkHeaderSize) != len(table) {
		return nil, errors.New("truncated PACK.PK index")
	}

	entries := make([]ArchiveEntry, 0, count)
	for i := 0; i < count; i++ {
		record := table[i*packPkEntrySize : (i+1)*packPkEntrySize]
		name := archiveReadName(record[:packPkNameReadSize])
		if name == "" {
			return nil, errors.New("empty PACK.PK entry name")
		}

		offset := binutil.U32(record[packPkOffsetField:])
		size := binutil.U32(record[packPkSizeField:])
		entry := ArchiveEntry{Path: name, Offset: int64(offset), Size: int32(size)}
		if size > math.MaxInt32 || entry.End() > length {
			return nil, fmt.Errorf("PACK.PK entry '%s' outside archive bounds", name)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
