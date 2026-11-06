package resource

import (
	"fmt"
	"math"
	"os"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
	"kizuna/engine/core/strutil"
)

const (
	SnnExtension      = ".snn"
	snnIndexExtension = ".inx"
	snnEntrySize      = 0x48
	snnNameSize       = 0x40
	snnMaxEntryCount  = 1 << 20
	snnOffsetField    = 0x40
	snnSizeField      = 0x44
)

type SnnArchive struct {
	name    string
	version int
	length  int64
	entries []ArchiveEntry
	stream  *os.File
}

func (a *SnnArchive) Name() string {
	return a.name
}

func (a *SnnArchive) Version() int {
	return a.version
}

func (a *SnnArchive) Length() int64 {
	return a.length
}

func (a *SnnArchive) Entries() []ArchiveEntry {
	return a.entries
}

func SnnIsArchive(path string) bool {
	return SnnIsArchiveWithIndex(path, nil)
}

func SnnIsArchiveWithIndex(path string, indexes map[string]string) bool {
	if !strutil.HasSuffixFold(path, SnnExtension) {
		return false
	}

	indexPath, ok := findSnnIndexWithGlobal(path, indexes)
	return ok && core.PathExists(indexPath)
}

func SnnOpen(path string) (*SnnArchive, error) {
	return SnnOpenWithIndex(path, nil)
}

func SnnOpenWithIndex(path string, indexes map[string]string) (*SnnArchive, error) {
	indexPath, ok := findSnnIndexWithGlobal(path, indexes)
	if !ok {
		return nil, fmt.Errorf("missing index for %s", path)
	}

	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	length := fileLength(stream)
	entries, err := readSnnIndex(indexPath, length)
	if err != nil {
		_ = stream.Close()
		return nil, err
	}
	if len(entries) == 0 {
		_ = stream.Close()
		return nil, fmt.Errorf("empty index: %s", indexPath)
	}

	return &SnnArchive{
		name:    pathFileName(path),
		version: 1,
		length:  length,
		entries: entries,
		stream:  stream,
	}, nil
}

func (a *SnnArchive) Read(offset int64, buffer []byte) int {
	return readAt(a.stream, buffer, offset)
}

func (a *SnnArchive) ReadAll(entry ArchiveEntry) ([]byte, error) {
	if entry.Size < 0 {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	buffer := make([]byte, entry.Size)
	if a.Read(entry.Offset, buffer) != len(buffer) {
		return nil, fmt.Errorf("truncated entry '%s'", entry.Path)
	}

	return buffer, nil
}

func (a *SnnArchive) Dispose() {
	_ = a.stream.Close()
}

func findSnnIndex(path string) (string, bool) {
	return findSnnIndexWithGlobal(path, nil)
}

// findSnnIndexWithGlobal 先按同目录配对；失败时查全局索引。
// 只按 stem 小写完全相等配对，不做模糊匹配，避免把 Graphics1.Inx 错配给
// Graphics2.Snn（两者只差一个字符，猜错会让所有图像解码成花屏）。
func findSnnIndexWithGlobal(path string, indexes map[string]string) (string, bool) {
	if local, ok := findLocalSnnIndex(path); ok {
		return local, true
	}
	if indexes == nil {
		return "", false
	}
	stem := strings.ToLower(pathFileNameWithoutExtension(path))
	if candidate, ok := indexes[stem]; ok && core.PathExists(candidate) {
		core.Logf("index: paired %s with cross-root index %s", pathFileName(path), pathFileName(candidate))
		return candidate, true
	}
	// 8.3 短名（GRAPHI~1.SNN）在“先解包再丢进目录”的安装方式里会丢掉
	// Joliet 长名，无法按 stem 精确配对。此时只在候选唯一且该索引确实没有
	// 同目录 .Snn 时才回退，避免把 Graphics1.Inx 错配给 Graphics2.Snn。
	if strings.Contains(stem, "~") {
		prefix := stem
		if tilde := strings.Index(prefix, "~"); tilde >= 0 {
			prefix = prefix[:tilde]
		}
		var unpaired []string
		for candidateStem, candidate := range indexes {
			if !strings.HasPrefix(candidateStem, prefix) {
				continue
			}
			if hasAdjacentArchive(candidate) {
				continue
			}
			unpaired = append(unpaired, candidate)
		}
		if len(unpaired) == 1 && core.PathExists(unpaired[0]) {
			core.Logf("index: short name %s paired with unpaired index %s", pathFileName(path), pathFileName(unpaired[0]))
			return unpaired[0], true
		}
	}
	return "", false
}

// hasAdjacentArchive 判断索引旁边是否已经有数据文件；有就说明它不是
// “等着跨目录配对”的那一个。
func hasAdjacentArchive(indexPath string) bool {
	directory := pathDirectoryName(indexPath)
	if directory == "" {
		return false
	}
	stem := pathFileNameWithoutExtension(indexPath)
	for _, extension := range []string{".Snn", ".snn", ".SNN"} {
		if core.PathExists(core.Join(directory, stem+extension)) {
			return true
		}
	}
	return false
}

func findLocalSnnIndex(path string) (string, bool) {
	directory := pathDirectoryName(path)
	if directory == "" || !core.DirExists(directory) {
		return "", false
	}

	stem := pathFileNameWithoutExtension(path)
	direct := core.Join(directory, stem+snnIndexExtension)
	if core.PathExists(direct) {
		return direct, true
	}

	for _, candidate := range listFilesTop(directory) {
		if strutil.HasSuffixFold(candidate, snnIndexExtension) &&
			strings.EqualFold(pathFileNameWithoutExtension(candidate), stem) {
			return candidate, true
		}
	}

	return "", false
}

// collectSnnIndexes 收集所有根的 .Inx，主根优先。只记 stem，不记目录：
// 配对发生在后面按 .Snn 的 stem 查询时。
func collectSnnIndexes(roots []string) map[string]string {
	indexes := make(map[string]string)
	for _, root := range roots {
		for _, extension := range []string{"*.Inx", "*.inx", "*.INX"} {
			for _, path := range enumerateFiles(root, extension) {
				stem := strings.ToLower(pathFileNameWithoutExtension(path))
				if _, taken := indexes[stem]; !taken {
					indexes[stem] = path
				}
			}
		}
	}
	return indexes
}

func readSnnIndex(indexPath string, length int64) ([]ArchiveEntry, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("truncated index: %s", indexPath)
	}

	count := int(binutil.I32(data))
	if count <= 0 || count > snnMaxEntryCount || 4+int64(count)*snnEntrySize > int64(len(data)) {
		return nil, fmt.Errorf("invalid index count %d: %s", binutil.I32(data), indexPath)
	}

	entries := make([]ArchiveEntry, 0, count)
	position := 4
	for index := 0; index < count; index++ {
		record := data[position : position+snnEntrySize]
		position += snnEntrySize

		name := archiveReadName(record[:snnNameSize])
		if name == "" {
			continue
		}

		offset := binutil.U32(record[snnOffsetField:])
		size := binutil.U32(record[snnSizeField:])
		entry := ArchiveEntry{Path: name, Offset: int64(offset), Size: int32(size)}
		if size > math.MaxInt32 || entry.End() > length {
			core.Logf("index: snn entry '%s' outside archive, skipped", name)
			continue
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
