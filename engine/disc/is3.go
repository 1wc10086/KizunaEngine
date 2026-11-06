package disc

// is3.go 解析 InstallShield Z (IS3) 归档并解包。结构与字段顺序对照
// 项目参考/1.py 第 5 节；只读头部与目录/文件表，数据按条目按需读取。

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding/charmap"
)

const (
	is3Magic              uint32 = 0x8C655D13
	is3HeaderLengthByte          = 0x3A
	is3HeaderOffset              = 5
	is3HeaderSize                = 58
	is3EntryFixedSize            = 30
	is3AttribUncompressed        = 0x10
	is3MaxTableSize              = 64 << 20
	is3MaxPayloadSize            = 1 << 31
	is3MaxDepth                  = 3
	is3InstallDir                = "installshield"
)

type is3Entry struct {
	Path          string
	DataOffset    int64
	CompactedSize int64
	OriginalSize  int64
	Attribs       uint32
}

func (e is3Entry) payloadSize() int64 {
	if e.Attribs&is3AttribUncompressed != 0 {
		return e.CompactedSize
	}
	return e.OriginalSize
}

func isIS3Head(head []byte) bool {
	return len(head) >= 4 && binary.LittleEndian.Uint32(head) == is3Magic
}

func readFullAt(r io.ReaderAt, buf []byte, offset int64) error {
	n, err := r.ReadAt(buf, offset)
	if n == len(buf) {
		return nil
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	return err
}

func parseIS3(r io.ReaderAt, size int64) ([]is3Entry, error) {
	if size < is3HeaderOffset+is3HeaderSize {
		return nil, errors.New("is3: archive too short")
	}
	head := make([]byte, is3HeaderOffset+is3HeaderSize)
	if err := readFullAt(r, head, 0); err != nil {
		return nil, err
	}
	if !isIS3Head(head) {
		return nil, errors.New("is3: bad magic")
	}
	if head[4] != is3HeaderLengthByte {
		return nil, fmt.Errorf("is3: unexpected header length 0x%02X", head[4])
	}
	h := head[is3HeaderOffset:]
	dirsOffset := int64(binary.LittleEndian.Uint32(h[36:]))
	dirsSize := int64(binary.LittleEndian.Uint32(h[40:]))
	filesOffset := int64(binary.LittleEndian.Uint32(h[46:]))
	filesSize := int64(binary.LittleEndian.Uint32(h[50:]))

	dirTable, err := readIS3Table(r, size, dirsOffset, dirsSize)
	if err != nil {
		return nil, err
	}
	fileTable, err := readIS3Table(r, size, filesOffset, filesSize)
	if err != nil {
		return nil, err
	}
	return parseIS3Entries(parseIS3Dirs(dirTable), fileTable), nil
}

func readIS3Table(r io.ReaderAt, size, offset, length int64) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	if length > is3MaxTableSize || offset < 0 || offset+length > size {
		return nil, errors.New("is3: table out of range")
	}
	table := make([]byte, length)
	if err := readFullAt(r, table, offset); err != nil {
		return nil, err
	}
	return table, nil
}

func parseIS3Dirs(table []byte) []string {
	var dirs []string
	for pos := 0; pos+6 <= len(table); {
		entrySize := int(binary.LittleEndian.Uint16(table[pos+2:]))
		nameSize := int(binary.LittleEndian.Uint16(table[pos+4:]))
		dirs = append(dirs, decodeIS3Name(table, pos+6, nameSize))
		if entrySize <= 0 {
			break
		}
		pos += entrySize
	}
	return dirs
}

func parseIS3Entries(dirs []string, table []byte) []is3Entry {
	var entries []is3Entry
	for pos := 0; pos+is3EntryFixedSize <= len(table); {
		dirID := int(binary.LittleEndian.Uint16(table[pos+1:]))
		originalSize := int64(binary.LittleEndian.Uint32(table[pos+3:]))
		compactedSize := int64(binary.LittleEndian.Uint32(table[pos+7:]))
		dataOffset := int64(binary.LittleEndian.Uint32(table[pos+11:]))
		attribs := binary.LittleEndian.Uint32(table[pos+19:])
		entrySize := int(binary.LittleEndian.Uint16(table[pos+23:]))
		nameSize := int(table[pos+29])
		name := decodeIS3Name(table, pos+is3EntryFixedSize, nameSize)
		entryPath := name
		if dirID < len(dirs) && dirs[dirID] != "" {
			entryPath = dirs[dirID] + "\\" + name
		}
		entries = append(entries, is3Entry{
			Path:          entryPath,
			DataOffset:    dataOffset,
			CompactedSize: compactedSize,
			OriginalSize:  originalSize,
			Attribs:       attribs,
		})
		step := entrySize
		if step == 0 {
			step = 43 + nameSize
		}
		pos += step
	}
	return entries
}

func decodeIS3Name(table []byte, start, size int) string {
	end := start + size
	if end > len(table) {
		end = len(table)
	}
	if start >= end {
		return ""
	}
	return decodeCP1252(table[start:end])
}

func decodeCP1252(raw []byte) string {
	out, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// isoFileReader 让 IS3 解析器直接在 ISO 内部读取范围，不必先解出整个归档。
type isoFileReader struct {
	volume *isoVolume
	file   *isoFile
}

func (r isoFileReader) ReadAt(p []byte, offset int64) (int, error) {
	data, err := r.volume.readRange(r.file, offset, int64(len(p)))
	if err != nil {
		return 0, err
	}
	return copy(p, data), nil
}

// readRange 读取 ISO 文件内 [offset, offset+size) 的字节，按 extent 跨扇区拼接。
func (v *isoVolume) readRange(file *isoFile, offset, size int64) ([]byte, error) {
	if file == nil || file.isDir {
		return nil, errors.New("iso: not a file")
	}
	if offset < 0 || size < 0 || offset+size > file.size {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, 0, size)
	end := offset + size
	var base int64
	for _, part := range file.extents {
		partEnd := base + part.size
		if partEnd <= offset {
			base = partEnd
			continue
		}
		if base >= end {
			break
		}
		from := offset - base
		if from < 0 {
			from = 0
		}
		to := end - base
		if to > part.size {
			to = part.size
		}
		sectorStart := from / isoSectorSize
		inner := from - sectorStart*isoSectorSize
		span := to - sectorStart*isoSectorSize
		data, err := v.image.extent(part.lba+uint32(sectorStart), span)
		if err != nil {
			return nil, err
		}
		out = append(out, data[inner:inner+(to-from)]...)
		base = partEnd
	}
	if int64(len(out)) != size {
		return nil, io.ErrUnexpectedEOF
	}
	return out, nil
}

// installShieldFiles 返回 ISO 内 IS3 归档展开后的虚拟路径，形如 _SETUP.1/Group1/SCNFILE.PFT，
// 与 Prepare 解包到 installshield/<base>/ 的布局一致。
func (v *isoVolume) installShieldFiles() []File {
	var out []File
	for _, file := range v.Files() {
		node := v.files[file.Path]
		if node == nil || file.Size < is3HeaderOffset+is3HeaderSize {
			continue
		}
		head, err := v.readRange(node, 0, 4)
		if err != nil || !isIS3Head(head) {
			continue
		}
		entries, err := parseIS3(isoFileReader{volume: v, file: node}, file.Size)
		if err != nil {
			log.Printf("disc: is3 %s: %v", file.Path, err)
			continue
		}
		base := path.Base(file.Path)
		for _, entry := range entries {
			relative, ok := sanitizeRelative(entry.Path)
			if !ok {
				continue
			}
			out = append(out, File{Path: path.Join(base, relative), Size: entry.payloadSize()})
		}
	}
	return out
}

func is3Payload(r io.ReaderAt, entry is3Entry) ([]byte, error) {
	if entry.CompactedSize < 0 || entry.CompactedSize > is3MaxPayloadSize {
		return nil, errors.New("is3: entry too large")
	}
	blob := make([]byte, entry.CompactedSize)
	if err := readFullAt(r, blob, entry.DataOffset); err != nil {
		return nil, err
	}
	if entry.Attribs&is3AttribUncompressed != 0 {
		return blob, nil
	}
	return explodePKWARE(blob, entry.OriginalSize)
}

// extractIS3 把单个 IS3 归档解到 destination。同名同大小视为已解包并跳过；
// 同名不同大小时追加 _N 后缀，避免覆盖。
func extractIS3(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	entries, err := parseIS3(file, info.Size())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		relative, ok := sanitizeRelative(entry.Path)
		if !ok {
			continue
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if existing, err := os.Stat(target); err == nil && existing.Size() == entry.payloadSize() {
			continue
		}
		target = uniqueFilePath(target)
		payload, err := is3Payload(file, entry)
		if err != nil {
			log.Printf("disc: is3 %s: %v", relative, err)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func uniqueFilePath(target string) string {
	if _, err := os.Stat(target); err != nil {
		return target
	}
	ext := filepath.Ext(target)
	stem := strings.TrimSuffix(target, ext)
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, n, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}

// unpackInstallShield 逐层解开 mount 中的 IS3 归档，输出到 mount/installshield/<base>/。
// 最多扫描 is3MaxDepth 层，嵌套归档在上一层输出目录里查找。
func unpackInstallShield(mount string) {
	installRoot := filepath.Join(mount, is3InstallDir)
	pending := []string{mount}
	for depth := 0; depth < is3MaxDepth && len(pending) > 0; depth++ {
		var next []string
		for _, dir := range pending {
			for _, archive := range findIS3Archives(dir, installRoot) {
				destination := filepath.Join(installRoot, filepath.Base(archive))
				if _, err := os.Stat(destination); err == nil {
					continue
				}
				if err := extractIS3(archive, destination); err != nil {
					log.Printf("disc: installshield %s: %v", filepath.Base(archive), err)
					continue
				}
				next = append(next, destination)
			}
		}
		pending = next
	}
}

func findIS3Archives(root, skip string) []string {
	var found []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p == skip {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < is3HeaderOffset+is3HeaderSize {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		head := make([]byte, 4)
		_, err = io.ReadFull(f, head)
		f.Close()
		if err == nil && isIS3Head(head) {
			found = append(found, p)
		}
		return nil
	})
	return found
}
