package resource

import (
	"fmt"
	"os"
	"strings"

	"kizuna/engine/core/strutil"
)

const ZbmExtension = ".zbm"

type ZbmArchive struct {
	name      string
	stream    *os.File
	container *ZbmContainer
	entries   []ArchiveEntry
	length    int64
}

func (a *ZbmArchive) Name() string {
	return a.name
}

func (a *ZbmArchive) Version() int {
	return a.container.Header.Version
}

func (a *ZbmArchive) Length() int64 {
	return a.length
}

func (a *ZbmArchive) Entries() []ArchiveEntry {
	return a.entries
}

func ZbmIsArchive(path string) bool {
	if !strutil.HasSuffixFold(path, ZbmExtension) {
		return false
	}

	stream, err := os.Open(path)
	if err != nil {
		return false
	}
	defer stream.Close()

	header := make([]byte, ZbmHeaderSize)
	if readAt(stream, header, 0) != len(header) {
		return false
	}

	_, err = ZbmParseHeader(header)
	return err == nil
}

func ZbmOpen(path string) (*ZbmArchive, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	length := fileLength(stream)
	container, err := ZbmParseContainer(stream, length)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}

	entries := make([]ArchiveEntry, 0, len(container.Entries))
	for _, item := range container.Entries {
		entries = append(entries, ArchiveEntry{Path: item.Name, Offset: item.DataOffset, Size: int32(item.UnpackedSize)})
	}

	return &ZbmArchive{
		name:      pathFileName(path),
		stream:    stream,
		container: container,
		entries:   entries,
		length:    length,
	}, nil
}

func (a *ZbmArchive) TryFindEntry(name string) (ArchiveEntry, bool) {
	found, ok := a.container.TryFindEntry(name)
	if !ok {
		return ArchiveEntry{}, false
	}

	for _, item := range a.entries {
		if strings.EqualFold(item.Path, found.Name) {
			return item, true
		}
	}

	return ArchiveEntry{}, false
}

func (a *ZbmArchive) Read(offset int64, buffer []byte) int {
	return readAt(a.stream, buffer, offset)
}

func (a *ZbmArchive) ReadAll(entry ArchiveEntry) ([]byte, error) {
	found, ok := a.container.TryFindEntry(entry.Path)
	if !ok {
		return nil, fmt.Errorf("unknown ZBM entry '%s'", entry.Path)
	}

	packed := make([]byte, found.PackedSize)
	if readAt(a.stream, packed, found.DataOffset) != len(packed) {
		return nil, fmt.Errorf("truncated ZBM entry '%s'", entry.Path)
	}

	return ZbmUnpack(packed, found.UnpackedSize)
}

func (a *ZbmArchive) Dispose() {
	_ = a.stream.Close()
}
