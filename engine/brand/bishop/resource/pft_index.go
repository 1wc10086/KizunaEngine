package resource

import (
	"math"
	"os"
	"strconv"
	"strings"

	"kizuna/engine/core"
	"kizuna/engine/core/binutil"
)

const (
	pftHeaderLength   = 8
	pftMaxEntryCount  = 1000000
	pftNameSize       = 8
	pftClusterField   = 8
	pftSizeField      = 12
	pftLowerExtension = ".pft"
	pftUpperExtension = ".PFT"
)

type PftEntry struct {
	Name         string
	ClusterIndex uint32
	Size         uint32
}

type PftIndex struct {
	dskPath     string
	headerSize  uint16
	clusterSize uint16
	entries     []PftEntry
	extension   string
}

func (p *PftIndex) DskPath() string {
	return p.dskPath
}

func (p *PftIndex) HeaderSize() uint16 {
	return p.headerSize
}

func (p *PftIndex) ClusterSize() uint16 {
	return p.clusterSize
}

func (p *PftIndex) Entries() []PftEntry {
	return p.entries
}

func (p *PftIndex) Extension() string {
	return p.extension
}

func PftTryLoad(dskPath string) *PftIndex {
	pftPath, ok := findPftPair(dskPath)
	if !ok {
		return nil
	}

	data, err := os.ReadFile(pftPath)
	if err != nil {
		return nil
	}
	if len(data) < pftHeaderLength {
		return nil
	}

	headerSize := binutil.U16(data)
	clusterSize := binutil.U16(data[2:])
	count := int(binutil.I32(data[4:]))
	if count <= 0 || count > pftMaxEntryCount {
		return nil
	}
	if headerSize < 6 || clusterSize == 0 {
		return nil
	}
	if int64(headerSize)+int64(count)*ArchiveSignaturePftEntrySize > int64(len(data)) {
		return nil
	}

	entries := make([]PftEntry, 0, count)
	position := int(headerSize)
	for index := 0; index < count; index++ {
		record := data[position : position+ArchiveSignaturePftEntrySize]
		position += ArchiveSignaturePftEntrySize

		name := archiveReadName(record[:ArchiveSignaturePftNameSize])
		cluster := binutil.U32(record[pftClusterField:])
		size := binutil.U32(record[pftSizeField:])
		entries = append(entries, PftEntry{Name: name, ClusterIndex: cluster, Size: size})
	}

	return &PftIndex{
		dskPath:     dskPath,
		headerSize:  headerSize,
		clusterSize: clusterSize,
		entries:     entries,
		extension:   LegacyArchiveExtensionForDsk(dskPath),
	}
}

func (p *PftIndex) ToArchiveEntries(dskLength int64) []ArchiveEntry {
	result := make([]ArchiveEntry, 0, len(p.entries))
	used := make(map[string]struct{}, len(p.entries))

	for _, entry := range p.entries {
		if entry.Name == "" {
			continue
		}
		if entry.Size > math.MaxInt32 {
			core.Logf("archive: dsk entry '%s' outside archive, skipped", entry.Name)
			continue
		}

		offset := int64(entry.ClusterIndex) * int64(p.clusterSize)
		if offset+int64(entry.Size) > dskLength {
			core.Logf("archive: dsk entry '%s' outside archive, skipped", entry.Name)
			continue
		}

		path := entry.Name + p.extension
		suffix := 0
		for {
			if _, taken := used[strings.ToLower(path)]; !taken {
				break
			}
			suffix++
			path = entry.Name + "_" + strconv.Itoa(suffix) + p.extension
		}
		used[strings.ToLower(path)] = struct{}{}

		result = append(result, ArchiveEntry{Path: path, Offset: offset, Size: int32(entry.Size)})
	}

	return result
}

func findPftPair(dskPath string) (string, bool) {
	directory := pathDirectoryName(dskPath)
	if directory == "" || !core.DirExists(directory) {
		return "", false
	}

	stem := pathFileNameWithoutExtension(dskPath)
	lower := core.Join(directory, stem+pftLowerExtension)
	if core.PathExists(lower) {
		return lower, true
	}

	upper := core.Join(directory, stem+pftUpperExtension)
	if core.PathExists(upper) {
		return upper, true
	}

	wanted := stem + pftLowerExtension
	for _, candidate := range listFilesTop(directory) {
		if strings.EqualFold(pathFileName(candidate), wanted) {
			return candidate, true
		}
	}

	return "", false
}
