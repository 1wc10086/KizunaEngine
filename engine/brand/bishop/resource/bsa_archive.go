package resource

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"kizuna/engine/core/binutil"
)

const (
	bsaMaxEntryCount  = 1 << 20
	bsaReadBufferSize = 1 << 16
)

type BsaArchive struct {
	name    string
	version int
	length  int64
	entries []ArchiveEntry
	stream  *os.File
}

func (a *BsaArchive) Name() string {
	return a.name
}

func (a *BsaArchive) Version() int {
	return a.version
}

func (a *BsaArchive) Length() int64 {
	return a.length
}

func (a *BsaArchive) Entries() []ArchiveEntry {
	return a.entries
}

func BsaOpen(path string) (*BsaArchive, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	length := fileLength(stream)
	header := make([]byte, ArchiveSignatureHeaderSize)
	if readAt(stream, header, 0) != len(header) || !ArchiveSignatureIsBsaHeader(header) {
		_ = stream.Close()
		return nil, fmt.Errorf("not a BSArc archive: %s", path)
	}

	if binutil.U16(header[4:]) != 'c' {
		_ = stream.Close()
		return nil, fmt.Errorf("unexpected BSArc marker: %s", path)
	}

	version := int(binutil.U16(header[8:]))
	if version < 1 || version > 3 {
		_ = stream.Close()
		return nil, fmt.Errorf("unsupported BSArc version %d: %s", version, path)
	}

	count := int(binutil.U16(header[10:]))
	indexOffset := binutil.U32(header[12:])
	if bsaIndexInsane(count, length, indexOffset) {
		_ = stream.Close()
		return nil, fmt.Errorf("corrupt BSArc index: %s", path)
	}

	var entries []ArchiveEntry
	switch version {
	case 1:
		entries, err = bsaReadVersion1(stream, count, indexOffset, length)
	case 2:
		entries, err = bsaReadVersion2(stream, count, indexOffset, length)
	default:
		entries, err = bsaReadVersion3(stream, count, indexOffset, length)
	}
	if err != nil {
		_ = stream.Close()
		return nil, err
	}

	return &BsaArchive{
		name:    pathFileName(path),
		version: version,
		length:  length,
		entries: entries,
		stream:  stream,
	}, nil
}

func BsaIsArchive(path string) bool {
	stream, err := os.Open(path)
	if err != nil {
		return false
	}
	defer stream.Close()

	header := make([]byte, ArchiveSignatureHeaderSize)
	return readAt(stream, header, 0) == len(header) && ArchiveSignatureIsBsaHeader(header)
}

func (a *BsaArchive) Read(offset int64, buffer []byte) int {
	return readAt(a.stream, buffer, offset)
}

func (a *BsaArchive) ReadAll(entry ArchiveEntry) ([]byte, error) {
	if entry.Size < 0 {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	buffer := make([]byte, entry.Size)
	if a.Read(entry.Offset, buffer) != len(buffer) {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	return buffer, nil
}

func (a *BsaArchive) Dispose() {
	_ = a.stream.Close()
}

func bsaIndexInsane(count int, length int64, indexOffset uint32) bool {
	if count <= 0 || count > bsaMaxEntryCount || int64(indexOffset) >= length {
		return true
	}

	return int64(indexOffset)+int64(count)*ArchiveSignatureV2EntrySize > length
}

func bsaReadVersion1(stream *os.File, count int, indexOffset uint32, length int64) ([]ArchiveEntry, error) {
	buffer := make([]byte, count*ArchiveSignatureV1EntrySize)
	if readAt(stream, buffer, int64(indexOffset)) != len(buffer) {
		return nil, errors.New("truncated BSArc V1 index")
	}

	entries := make([]ArchiveEntry, 0, count)
	var directories []string

	for i := 0; i < count; i++ {
		position := i * ArchiveSignatureV1EntrySize
		name := archiveReadName(buffer[position : position+0x20])
		if name == "" {
			return nil, errors.New("empty BSArc V1 entry name")
		}

		if name[0] == '>' {
			directories = append(directories, name[1:])
			continue
		}

		if name[0] == '<' {
			if len(directories) > 0 {
				directories = directories[:len(directories)-1]
			}
			continue
		}

		offset := binutil.U32(buffer[position+0x20:])
		size := binutil.U32(buffer[position+0x24:])
		entry, err := bsaCreateEntry(directories, name, offset, size, length)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func bsaReadVersion2(stream *os.File, count int, indexOffset uint32, length int64) ([]ArchiveEntry, error) {
	if packed, ok := bsaTryReadPackedVersion2(stream, count, indexOffset, length); ok {
		return packed, nil
	}

	return bsaReadVersion3(stream, count, indexOffset, length)
}

func bsaTryReadPackedVersion2(stream *os.File, count int, indexOffset uint32, length int64) ([]ArchiveEntry, bool) {
	tableSize := int64(count) * ArchiveSignaturePackedV2EntrySize
	if int64(indexOffset)+tableSize != length || tableSize <= 0 || tableSize > math.MaxInt32 {
		return nil, false
	}

	table := make([]byte, tableSize)
	if readAt(stream, table, int64(indexOffset)) != len(table) {
		return nil, false
	}

	entries := make([]ArchiveEntry, 0, count)
	var directories []string

	for i := 0; i < count; i++ {
		position := i * ArchiveSignaturePackedV2EntrySize
		record := table[position : position+ArchiveSignaturePackedV2EntrySize]
		name := archiveReadName(record[:ArchiveSignaturePackedV2NameSize])
		if name == "" {
			return nil, false
		}

		if name[0] == '>' {
			directories = append(directories, name[1:])
			continue
		}

		if name[0] == '<' {
			if len(directories) > 0 {
				directories = directories[:len(directories)-1]
			}
			continue
		}

		offset := binutil.U32(record[ArchiveSignaturePackedV2OffsetField:])
		size := binutil.U32(record[ArchiveSignaturePackedV2SizeField:])
		entry := ArchiveEntry{
			Path:   buildArchivePath(directories, name),
			Offset: int64(offset),
			Size:   int32(size),
		}
		if size > math.MaxInt32 || entry.Offset < 0 || entry.End() > length {
			return nil, false
		}

		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil, false
	}

	return entries, true
}

func bsaReadVersion3(stream *os.File, count int, indexOffset uint32, length int64) ([]ArchiveEntry, error) {
	tableSize := count * ArchiveSignatureV2EntrySize
	poolOffset := indexOffset + uint32(tableSize)
	poolSize := int(length - int64(poolOffset))
	if poolSize <= 0 {
		return nil, errors.New("BSArc name pool is empty")
	}

	table := make([]byte, tableSize)
	if readAt(stream, table, int64(indexOffset)) != len(table) {
		return nil, errors.New("truncated BSArc V2 index")
	}

	pool := make([]byte, poolSize)
	if readAt(stream, pool, int64(poolOffset)) != len(pool) {
		return nil, errors.New("truncated BSArc name pool")
	}

	entries := make([]ArchiveEntry, 0, count)
	var directories []string

	for i := 0; i < count; i++ {
		position := i * ArchiveSignatureV2EntrySize
		nameOffset := int(binutil.U32(table[position:]))
		if uint32(nameOffset) >= uint32(len(pool)) {
			return nil, fmt.Errorf("BSArc name offset %d outside pool", nameOffset)
		}

		name := archiveReadName(pool[nameOffset:])
		if name == "" {
			return nil, errors.New("empty BSArc entry name")
		}

		if name[0] == '>' {
			directories = append(directories, name[1:])
			continue
		}

		if name[0] == '<' {
			if len(directories) > 0 {
				directories = directories[:len(directories)-1]
			}
			continue
		}

		offset := binutil.U32(table[position+4:])
		size := binutil.U32(table[position+8:])
		entry, err := bsaCreateEntry(directories, name, offset, size, length)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func bsaCreateEntry(directories []string, name string, offset uint32, size uint32, length int64) (ArchiveEntry, error) {
	entry := ArchiveEntry{
		Path:   buildArchivePath(directories, name),
		Offset: int64(offset),
		Size:   int32(size),
	}
	if entry.Offset < 0 || entry.End() > length {
		return ArchiveEntry{}, fmt.Errorf("BSArc entry '%s' outside archive bounds", entry.Path)
	}

	return entry, nil
}

func buildArchivePath(directories []string, name string) string {
	if len(directories) == 0 {
		return name
	}

	var builder strings.Builder
	for _, directory := range directories {
		builder.WriteString(directory)
		builder.WriteByte('/')
	}
	builder.WriteString(name)
	return builder.String()
}
