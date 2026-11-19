package disc

// iso9660.go 是只读 ISO9660/Joliet 目录解析。
//
// 游戏光盘需要的是“路径 -> 文件内容”，不是挂载点，所以这里不做系统挂载，
// 也不一次性把镜像读进内存：目录记录按需读取，文件按 extent 逐扇区流式读取。
// 名称优先 Joliet（Bishop 的日文盘都带 Joliet，能拿到 Graphics2.Snn 这样的长名，
// 避免 PROGRA~1 / GRAPHI~1 短名导致 .Inx 与 .Snn 配对失败），其次 Rock Ridge，
// 最后回退 ISO9660 名称并按 CP932 解码。

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/japanese"
)

const (
	isoSectorSize      = 2048
	isoMaxFiles        = 200000
	isoMaxDirectoryLen = 64 << 20
	isoMaxDepth        = 32
)

// File 是光盘里的一个文件。Path 使用 '/' 分隔、不含前导斜杠。
type File struct {
	Path string
	Size int64
}

type extent struct {
	lba  uint32
	size int64
}

type isoFile struct {
	path     string
	size     int64
	extents  []extent
	isDir    bool
	children []*isoFile
}

type isoVolume struct {
	image  *sectorImage
	joliet bool
	root   *isoFile
	files  map[string]*isoFile
}

func openISO(image *sectorImage) (*isoVolume, error) {
	primary, err := image.extent(16, isoSectorSize)
	if err != nil || len(primary) < isoSectorSize {
		return nil, errors.New("iso: cannot read primary volume descriptor")
	}
	if string(primary[1:6]) != isoMagic {
		return nil, errors.New("iso: missing CD001 primary volume descriptor")
	}
	volume := &isoVolume{image: image, files: map[string]*isoFile{}}
	descriptor := primary
	joliet := false
	// SVD 从 LBA17 开始连续排列，遇到 type=255 结束。
	for lba := uint32(17); lba < 32; lba++ {
		block, err := image.extent(lba, isoSectorSize)
		if err != nil || len(block) < isoSectorSize || string(block[1:6]) != isoMagic {
			break
		}
		if block[0] == 0xff {
			break
		}
		if block[0] == 2 && isJolietEscape(block[88:91]) {
			descriptor = block
			joliet = true
			break
		}
	}
	volume.joliet = joliet
	rootLBA, rootSize, ok := rootRecord(descriptor)
	if !ok {
		return nil, errors.New("iso: invalid root directory record")
	}
	root := &isoFile{path: "", isDir: true}
	if err := volume.readDirectory(root, rootLBA, rootSize, 0); err != nil {
		return nil, err
	}
	volume.root = root
	volume.collect(root, "")
	return volume, nil
}

func isJolietEscape(value []byte) bool {
	return len(value) == 3 && value[0] == '%' && value[1] == '/' &&
		(value[2] == '@' || value[2] == 'C' || value[2] == 'E')
}

func rootRecord(descriptor []byte) (uint32, int64, bool) {
	record := descriptor[156:190]
	if len(record) < 34 || record[0] < 34 || record[25]&0x02 == 0 {
		return 0, 0, false
	}
	lba := binary.LittleEndian.Uint32(record[2:])
	size := int64(binary.LittleEndian.Uint32(record[10:]))
	return lba, size, size > 0
}

func (v *isoVolume) readDirectory(directory *isoFile, lba uint32, size int64, depth int) error {
	if depth > isoMaxDepth || size <= 0 || size > isoMaxDirectoryLen {
		return nil
	}
	data, err := v.image.extent(lba, size)
	if err != nil {
		return err
	}
	position := 0
	for position < len(data) {
		length := int(data[position])
		if length == 0 {
			// 记录不会跨扇区：0 表示剩余部分是填充，跳到下一扇区。
			position = (position/isoSectorSize + 1) * isoSectorSize
			continue
		}
		if position+length > len(data) || length < 34 {
			break
		}
		record := data[position : position+length]
		position += length
		nameLength := int(record[32])
		if 33+nameLength > len(record) {
			continue
		}
		rawName := record[33 : 33+nameLength]
		if len(rawName) == 1 && (rawName[0] == 0 || rawName[0] == 1) {
			continue
		}
		flags := record[25]
		entryLBA := binary.LittleEndian.Uint32(record[2:])
		entrySize := int64(binary.LittleEndian.Uint32(record[10:]))
		userArea := record[33+nameLength:]
		if nameLength%2 == 0 && len(userArea) > 0 {
			userArea = userArea[1:]
		}
		name := v.decodeName(rawName, userArea)
		if name == "" {
			continue
		}
		childPath := name
		if directory.path != "" {
			childPath = directory.path + "/" + name
		}
		child := &isoFile{path: childPath, size: entrySize, isDir: flags&0x02 != 0}
		if child.isDir {
			if err := v.readDirectory(child, entryLBA, entrySize, depth+1); err != nil {
				continue
			}
			directory.children = append(directory.children, child)
			v.files[childPath] = child
			continue
		}
		child.extents = []extent{{lba: entryLBA, size: entrySize}}
		// multi-extent：紧接着的同名记录是它的后续片段。
		for flags&0x80 != 0 {
			nextPos := position
			if nextPos >= len(data) {
				break
			}
			nextLength := int(data[nextPos])
			if nextLength == 0 || nextPos+nextLength > len(data) || nextLength < 34 {
				break
			}
			next := data[nextPos : nextPos+nextLength]
			nextNameLen := int(next[32])
			nextName := v.decodeName(next[33:33+nextNameLen], nil)
			if nextName != name {
				break
			}
			position += nextLength
			flags = next[25]
			nextLBA := binary.LittleEndian.Uint32(next[2:])
			nextSize := int64(binary.LittleEndian.Uint32(next[10:]))
			child.extents = append(child.extents, extent{lba: nextLBA, size: nextSize})
			child.size += nextSize
		}
		directory.children = append(directory.children, child)
		v.files[childPath] = child
		if len(v.files) > isoMaxFiles {
			return errors.New("iso: too many files")
		}
	}
	return nil
}

func (v *isoVolume) decodeName(raw []byte, userArea []byte) string {
	var name string
	if v.joliet {
		if len(raw)%2 != 0 {
			raw = raw[:len(raw)-1]
		}
		units := make([]uint16, 0, len(raw)/2)
		for index := 0; index+1 < len(raw); index += 2 {
			units = append(units, binary.BigEndian.Uint16(raw[index:]))
		}
		name = string(utf16.Decode(units))
	} else if rock := parseRockRidgeName(userArea); rock != "" {
		name = rock
	} else {
		decoded, err := japanese.ShiftJIS.NewDecoder().Bytes(raw)
		if err == nil {
			name = string(decoded)
		} else {
			name = string(raw)
		}
	}
	return cleanISOName(name)
}

func parseRockRidgeName(userArea []byte) string {
	var builder strings.Builder
	for position := 0; position+4 <= len(userArea); {
		signature := userArea[position : position+2]
		length := int(userArea[position+2])
		if length < 4 || position+length > len(userArea) {
			break
		}
		if string(signature) == "NM" && length >= 5 {
			flags := userArea[position+4]
			if flags&0x01 == 0 {
				builder.Write(userArea[position+5 : position+length])
			}
		}
		position += length
	}
	return builder.String()
}

func cleanISOName(name string) string {
	name = strings.TrimSpace(name)
	if index := strings.LastIndex(name, ";"); index >= 0 {
		name = name[:index]
	}
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\r', '\n', 0:
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSuffix(name, ".")
	if name == "." || name == ".." {
		return ""
	}
	return name
}

func (v *isoVolume) collect(directory *isoFile, prefix string) {
	sort.Slice(directory.children, func(i, j int) bool {
		return directory.children[i].path < directory.children[j].path
	})
}

// Files 返回排序后的文件列表（不含目录）。
func (v *isoVolume) Files() []File {
	out := make([]File, 0, len(v.files))
	for _, file := range v.files {
		if file.isDir {
			continue
		}
		out = append(out, File{Path: file.path, Size: file.size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (v *isoVolume) readFile(file *isoFile) ([]byte, error) {
	if file == nil || file.isDir {
		return nil, errors.New("iso: not a file")
	}
	if file.size > int64(^uint(0)>>1) {
		return nil, errors.New("iso: file is too large")
	}
	out := make([]byte, 0, file.size)
	for _, part := range file.extents {
		if part.size <= 0 {
			continue
		}
		data, err := v.image.extent(part.lba, part.size)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		out = append(out, data...)
	}
	if int64(len(out)) > file.size {
		out = out[:file.size]
	}
	return out, nil
}

// ReadFile 按光盘内路径读取完整文件。路径使用 '/'，大小写敏感（ISO 名称本身
// 保留原始大小写；调用方应使用 Files 返回的路径）。
func (v *isoVolume) ReadFile(name string) ([]byte, error) {
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/")
	file := v.files[name]
	if file == nil {
		return nil, fmt.Errorf("iso: file not found: %s", name)
	}
	return v.readFile(file)
}

func (v *isoVolume) streamFile(file *isoFile, writer io.Writer) error {
	if file == nil || file.isDir {
		return errors.New("iso: not a file")
	}
	buffer := make([]byte, 1<<20)
	remaining := file.size
	for _, part := range file.extents {
		if part.size <= 0 || remaining <= 0 {
			continue
		}
		sectors := (part.size + 2047) / 2048
		for sector := int64(0); sector < sectors && remaining > 0; sector++ {
			want := int64(2048)
			if remaining < want {
				want = remaining
			}
			n, err := v.image.sector(part.lba+uint32(sector), buffer[:2048])
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if n <= 0 {
				return io.ErrUnexpectedEOF
			}
			if int64(n) > want {
				n = int(want)
			}
			if _, err := writer.Write(buffer[:n]); err != nil {
				return err
			}
			remaining -= int64(n)
		}
	}
	return nil
}

// sanitizeRelative 把光盘路径变成安全的本地相对路径：拒绝 ..、绝对路径，
// 清理 Windows 不允许的字符，避免解包时写到缓存目录之外。
func sanitizeRelative(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	parts := strings.Split(name, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		part = cleanISOName(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "", false
	}
	return path.Join(cleaned...), true
}
