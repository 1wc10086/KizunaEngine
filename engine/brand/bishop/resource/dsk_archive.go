package resource

import (
	"fmt"
	"os"

	"kizuna/engine/core"
	"kizuna/engine/core/strutil"
)

const DskExtension = ".dsk"

type DskArchive struct {
	name    string
	length  int64
	entries []ArchiveEntry
	stream  *os.File
}

func (a *DskArchive) Name() string {
	return a.name
}

func (a *DskArchive) Version() int {
	return 1
}

func (a *DskArchive) Length() int64 {
	return a.length
}

func (a *DskArchive) Entries() []ArchiveEntry {
	return a.entries
}

func DskIsArchive(path string) bool {
	return strutil.HasSuffixFold(path, DskExtension) && core.PathExists(path) && PftTryLoad(path) != nil
}

func DskOpen(path string) (*DskArchive, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	index := PftTryLoad(path)
	if index == nil {
		_ = stream.Close()
		return nil, fmt.Errorf("missing or invalid pft for %s", path)
	}

	length := fileLength(stream)
	entries := index.ToArchiveEntries(length)
	if len(entries) == 0 {
		_ = stream.Close()
		return nil, fmt.Errorf("empty archive: %s", path)
	}

	return &DskArchive{
		name:    pathFileName(path),
		length:  length,
		entries: entries,
		stream:  stream,
	}, nil
}

func (a *DskArchive) Read(offset int64, buffer []byte) int {
	if len(buffer) == 0 || offset >= a.length {
		return 0
	}

	return readAt(a.stream, buffer, offset)
}

func (a *DskArchive) ReadAll(entry ArchiveEntry) ([]byte, error) {
	if entry.Size < 0 {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	buffer := make([]byte, entry.Size)
	if a.Read(entry.Offset, buffer) != len(buffer) {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	return buffer, nil
}

func (a *DskArchive) Dispose() {
	_ = a.stream.Close()
}
