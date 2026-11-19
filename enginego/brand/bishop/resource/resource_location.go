package resource

import (
	"io"
	"math"
	"os"
	"strconv"
)

type ResourceLocation struct {
	name      string
	category  string
	archive   Archive
	entry     ArchiveEntry
	hasEntry  bool
	looseFile string
}

func ArchivedResourceLocation(name, category string, archive Archive, entry ArchiveEntry) *ResourceLocation {
	return &ResourceLocation{name: name, category: category, archive: archive, entry: entry, hasEntry: true}
}

func LooseResourceLocation(name, category, path string) *ResourceLocation {
	return &ResourceLocation{name: name, category: category, looseFile: path}
}

func (l *ResourceLocation) Name() string {
	return l.name
}

func (l *ResourceLocation) Category() string {
	return l.category
}

func (l *ResourceLocation) Archive() Archive {
	return l.archive
}

func (l *ResourceLocation) Entry() (ArchiveEntry, bool) {
	return l.entry, l.hasEntry
}

func (l *ResourceLocation) LooseFile() string {
	return l.looseFile
}

func (l *ResourceLocation) IsLoose() bool {
	return l.looseFile != ""
}

func (l *ResourceLocation) Key() string {
	if l.looseFile != "" {
		return l.looseFile
	}

	name := ""
	if l.archive != nil {
		name = l.archive.Name()
	}
	if !l.hasEntry {
		return name + "#"
	}

	return name + "#" + strconv.FormatInt(l.entry.Offset, 10)
}

func (l *ResourceLocation) Size() int64 {
	if l.hasEntry {
		return int64(l.entry.Size)
	}
	if l.looseFile == "" {
		return 0
	}

	info, err := os.Stat(l.looseFile)
	if err != nil {
		return 0
	}

	return info.Size()
}

func (l *ResourceLocation) Read() []byte {
	if l.looseFile != "" {
		data, err := os.ReadFile(l.looseFile)
		if err != nil {
			return nil
		}

		return data
	}

	if l.archive == nil || !l.hasEntry {
		return nil
	}

	data, err := l.archive.ReadAll(l.entry)
	if err != nil {
		return nil
	}

	return data
}

func (l *ResourceLocation) ReadPrefix(length int) []byte {
	total := l.Size()
	if total > math.MaxInt32 {
		total = math.MaxInt32
	}

	size := int64(length)
	if size > total {
		size = total
	}
	if size <= 0 {
		return []byte{}
	}

	if l.looseFile != "" {
		stream, err := os.Open(l.looseFile)
		if err != nil {
			return []byte{}
		}
		defer stream.Close()

		buffer := make([]byte, size)
		read := 0
		for read < len(buffer) {
			chunk, err := stream.Read(buffer[read:])
			read += chunk
			if err != nil {
				break
			}
		}

		return buffer[:read]
	}

	if l.archive == nil || !l.hasEntry {
		return []byte{}
	}

	buffer := make([]byte, size)
	available := l.archive.Read(l.entry.Offset, buffer)
	if available < 0 {
		available = 0
	}
	if available > len(buffer) {
		available = len(buffer)
	}

	return buffer[:available]
}

// ReadRange 读取资源内部的一段字节。
//
// モーションプラス 的分层动画是 100MB+ 的容器，里面按块存着上百帧；
// 播放时只能按帧取，不能整份读进内存（手机上直接 OOM）。
func (l *ResourceLocation) ReadRange(offset, length int64) []byte {
	if length <= 0 || offset < 0 {
		return []byte{}
	}
	total := l.Size()
	if offset >= total {
		return []byte{}
	}
	if offset+length > total {
		length = total - offset
	}
	if length <= 0 || length > math.MaxInt32 {
		return []byte{}
	}
	size := int(length)

	if l.looseFile != "" {
		stream, err := os.Open(l.looseFile)
		if err == nil {
			defer stream.Close()
			if _, err := stream.Seek(offset, 0); err == nil {
				buffer := make([]byte, size)
				read, _ := io.ReadFull(stream, buffer)
				return buffer[:read]
			}
		}
		return []byte{}
	}

	if l.archive == nil || !l.hasEntry {
		return []byte{}
	}

	buffer := make([]byte, size)
	available := l.archive.Read(l.entry.Offset+offset, buffer)
	if available < 0 {
		available = 0
	}
	if available > len(buffer) {
		available = len(buffer)
	}
	return buffer[:available]
}
