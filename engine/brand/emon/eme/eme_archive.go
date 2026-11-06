package eme

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kizuna/engine/core/textcodec"
)

const (
	recordSize       = 0x60
	nameSize         = 0x40
	scriptHeaderSize = 12
	imageHeaderSize  = 32
	prefixSize       = 4
)

const maxInt = int(^uint(0) >> 1)

var emeMagic = []byte{0x52, 0x52, 0x45, 0x44, 0x41, 0x54, 0x41, 0x20}

type EmeArchive struct {
	Name      string
	Directory []EmeEntry
	length    int64
	stream    *os.File
	key       []byte
	find      map[string]*EmeEntry
	disposed  bool
}

func IsArchive(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	length := info.Size()
	if length < 12 {
		return false
	}

	stream, err := os.Open(path)
	if err != nil {
		return false
	}
	defer stream.Close()

	magic := make([]byte, 8)
	if readAt(stream, magic, 0) != len(magic) || !bytes.Equal(magic, emeMagic) {
		return false
	}

	tail := make([]byte, 4)
	if readAt(stream, tail, length-4) != len(tail) {
		return false
	}

	count := binary.LittleEndian.Uint32(tail)
	return count > 0 && 4+int64(count)*recordSize+KeySize <= length
}

func OpenArchive(path string) (*EmeArchive, error) {
	stream, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	archive, err := openStream(path, stream)
	if err != nil {
		stream.Close()
		return nil, err
	}

	return archive, nil
}

func openStream(path string, stream *os.File) (*EmeArchive, error) {
	info, err := stream.Stat()
	if err != nil {
		return nil, err
	}

	length := info.Size()
	if length < 12 {
		return nil, fmt.Errorf("not an EME archive: %s", path)
	}

	magic := make([]byte, 8)
	if readAt(stream, magic, 0) != len(magic) || !bytes.Equal(magic, emeMagic) {
		return nil, fmt.Errorf("not an EME archive: %s", path)
	}

	tail := make([]byte, 4)
	if readAt(stream, tail, length-4) != len(tail) {
		return nil, fmt.Errorf("corrupt EME index: %s", path)
	}

	count := binary.LittleEndian.Uint32(tail)
	indexOffset := length - 4 - int64(count)*recordSize
	if count == 0 || indexOffset-KeySize < 0 {
		return nil, fmt.Errorf("corrupt EME index: %s", path)
	}

	total := int64(count)
	if total > int64(maxInt) {
		return nil, fmt.Errorf("corrupt EME index: %s", path)
	}

	key := make([]byte, KeySize)
	if readAt(stream, key, indexOffset-KeySize) != len(key) {
		return nil, fmt.Errorf("corrupt EME index: %s", path)
	}

	records := make([]byte, int(total)*recordSize)
	if readAt(stream, records, indexOffset) != len(records) {
		return nil, fmt.Errorf("corrupt EME index: %s", path)
	}

	directory := make([]EmeEntry, 0, int(total))
	for i := 0; i < int(total); i++ {
		block := records[i*recordSize : (i+1)*recordSize]
		if err := Decrypt(block, key); err != nil {
			return nil, err
		}

		entry, err := parseEntry(block, path)
		if err != nil {
			return nil, err
		}

		directory = append(directory, entry)
	}

	for i := range directory {
		if err := checkPlacement(&directory[i], length); err != nil {
			return nil, err
		}
	}

	sort.Slice(directory, func(i, j int) bool { return directory[i].Offset < directory[j].Offset })

	archive := &EmeArchive{
		Name:      filepath.Base(path),
		Directory: directory,
		length:    length,
		stream:    stream,
		key:       key,
		find:      make(map[string]*EmeEntry, len(directory)),
	}
	for i := range archive.Directory {
		entry := &archive.Directory[i]
		key := foldKey(entry.Name)
		if _, exists := archive.find[key]; !exists {
			archive.find[key] = entry
		}

		stem := foldKey(stripExtension(entry.Name))
		if _, exists := archive.find[stem]; !exists {
			archive.find[stem] = entry
		}
	}

	return archive, nil
}

func (a *EmeArchive) Version() int {
	return 1
}

func (a *EmeArchive) Length() int64 {
	return a.length
}

func (a *EmeArchive) IsDisposed() bool {
	return a.disposed
}

func (a *EmeArchive) Entries() []EmeEntry {
	out := make([]EmeEntry, len(a.Directory))
	copy(out, a.Directory)
	return out
}

func (a *EmeArchive) ReadEntry(entry *EmeEntry) ([]byte, error) {
	if entry == nil {
		return nil, errors.New("entry is null")
	}

	if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	switch entry.SubType {
	case 3:
		return a.readScriptData(entry)
	case 4:
		return a.readImageData(entry)
	case 5:
		return a.readPrefixedData(entry)
	default:
		return a.readStoredData(entry)
	}
}

func (a *EmeArchive) ReadScript(entry *EmeEntry) ([]byte, error) {
	if entry == nil {
		return nil, errors.New("entry is null")
	}

	if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	if !entry.IsScript() {
		return nil, fmt.Errorf("EME entry '%s' is not a script", entry.Name)
	}

	return a.readScriptData(entry)
}

func (a *EmeArchive) Find(name string) *EmeEntry {
	if name == "" {
		return nil
	}

	if entry, ok := a.find[foldKey(name)]; ok {
		return entry
	}

	for i := range a.Directory {
		if strings.EqualFold(a.Directory[i].Name, name) {
			return &a.Directory[i]
		}
	}

	return nil
}

func (a *EmeArchive) Read(offset int64, buffer []byte) int {
	stream := a.stream
	if a.disposed || stream == nil || offset < 0 || offset >= a.length || len(buffer) == 0 {
		return 0
	}

	available := len(buffer)
	if remaining := a.length - offset; remaining < int64(available) {
		available = int(remaining)
	}

	return readAt(stream, buffer[:available], offset)
}

func (a *EmeArchive) Dispose() {
	if a.disposed {
		return
	}

	a.disposed = true
	if a.stream != nil {
		a.stream.Close()
		a.stream = nil
	}

	a.key = nil
}

func (a *EmeArchive) readScriptData(entry *EmeEntry) ([]byte, error) {
	if err := a.checkSpan(entry, scriptHeaderSize); err != nil {
		return nil, err
	}

	header := make([]byte, scriptHeaderSize)
	if err := a.readSpan(entry, entry.Offset, header); err != nil {
		return nil, err
	}

	if err := DecryptRange(header, 0, scriptHeaderSize, a.key); err != nil {
		return nil, err
	}

	packedSize := binary.LittleEndian.Uint32(header[0:4])
	part2Size := binary.LittleEndian.Uint32(header[4:8])

	if entry.FrameSize == 0 {
		if err := a.checkSpan(entry, scriptHeaderSize+int64(entry.Size)); err != nil {
			return nil, err
		}

		plain := make([]byte, entry.Size)
		if err := a.readSpan(entry, entry.Offset+scriptHeaderSize, plain); err != nil {
			return nil, err
		}

		return plain, nil
	}

	if part2Size != 0 && int64(part2Size) < int64(entry.UnpackedSize) {
		if packedSize > uint32(entry.Size) {
			return nil, fmt.Errorf("EME script '%s' has a corrupt header", entry.Name)
		}

		prefix := int(packedSize)
		if err := a.checkSpan(entry, scriptHeaderSize+int64(prefix)+int64(entry.Size)); err != nil {
			return nil, err
		}

		region := make([]byte, prefix+int(entry.Size))
		if err := a.readSpan(entry, entry.Offset+scriptHeaderSize, region); err != nil {
			return nil, err
		}

		part1Size := int(entry.UnpackedSize) - int(part2Size)
		first, err := LzssDecompress(region[prefix:], part1Size, int(entry.FrameSize), int(entry.FrameInitPos))
		if err != nil {
			return nil, err
		}

		second, err := LzssDecompress(region[:prefix], int(part2Size), int(entry.FrameSize), int(entry.FrameInitPos))
		if err != nil {
			return nil, err
		}

		if len(first) != part1Size || len(second) != int(part2Size) {
			return nil, fmt.Errorf(
				"EME script '%s' decompressed to %d of %d bytes",
				entry.Name, len(first)+len(second), int(entry.UnpackedSize))
		}

		stitched := make([]byte, entry.UnpackedSize)
		copy(stitched[:part1Size], first)
		copy(stitched[part1Size:], second)
		return stitched, nil
	}

	if err := a.checkSpan(entry, scriptHeaderSize+int64(entry.Size)); err != nil {
		return nil, err
	}

	payload := make([]byte, entry.Size)
	if err := a.readSpan(entry, entry.Offset+scriptHeaderSize, payload); err != nil {
		return nil, err
	}

	decompressed, err := LzssDecompress(payload, int(entry.UnpackedSize), int(entry.FrameSize), int(entry.FrameInitPos))
	if err != nil {
		return nil, err
	}

	if len(decompressed) != int(entry.UnpackedSize) {
		return nil, fmt.Errorf(
			"EME script '%s' decompressed to %d of %d bytes",
			entry.Name, len(decompressed), int(entry.UnpackedSize))
	}

	return decompressed, nil
}

func (a *EmeArchive) readImageData(entry *EmeEntry) ([]byte, error) {
	if err := a.checkSpan(entry, imageHeaderSize); err != nil {
		return nil, err
	}

	header := make([]byte, imageHeaderSize)
	if err := a.readSpan(entry, entry.Offset, header); err != nil {
		return nil, err
	}

	if err := DecryptRange(header, 0, imageHeaderSize, a.key); err != nil {
		return nil, err
	}

	total := int64(imageHeaderSize) + int64(paletteBytes(header)) + int64(entry.Size)
	if err := a.checkSpan(entry, total); err != nil {
		return nil, err
	}

	if total < 0 || total > int64(maxInt) {
		return nil, fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
	}

	result := make([]byte, int(total))
	if err := a.readSpan(entry, entry.Offset, result); err != nil {
		return nil, err
	}

	if err := DecryptRange(result, 0, imageHeaderSize, a.key); err != nil {
		return nil, err
	}

	return result, nil
}

func (a *EmeArchive) readPrefixedData(entry *EmeEntry) ([]byte, error) {
	if err := a.checkSpan(entry, int64(entry.Size)); err != nil {
		return nil, err
	}

	if int(entry.Size) <= prefixSize {
		return a.readStoredData(entry)
	}

	result := make([]byte, entry.Size)
	if err := a.readSpan(entry, entry.Offset, result); err != nil {
		return nil, err
	}

	if err := DecryptRange(result, 0, prefixSize, a.key); err != nil {
		return nil, err
	}

	return result, nil
}

func (a *EmeArchive) readStoredData(entry *EmeEntry) ([]byte, error) {
	if err := a.checkSpan(entry, int64(entry.Size)); err != nil {
		return nil, err
	}

	result := make([]byte, entry.Size)
	if err := a.readSpan(entry, entry.Offset, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (a *EmeArchive) readSpan(entry *EmeEntry, offset int64, buffer []byte) error {
	if offset < 0 || offset+int64(len(buffer)) > a.length {
		return fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
	}

	stream := a.stream
	if stream == nil {
		return fmt.Errorf("EME archive '%s' is disposed", a.Name)
	}

	if readAt(stream, buffer, offset) != len(buffer) {
		return fmt.Errorf("EME entry '%s' is truncated", entry.Name)
	}

	return nil
}

func (a *EmeArchive) checkSpan(entry *EmeEntry, span int64) error {
	if entry.Offset < 0 || entry.Size < 0 || span < 0 || entry.Offset+span > a.length {
		return fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
	}

	return nil
}

func (a *EmeArchive) ensureUsable() error {
	if a.disposed {
		return fmt.Errorf("EME archive '%s' is disposed", a.Name)
	}

	return nil
}

func parseEntry(block []byte, path string) (EmeEntry, error) {
	end := bytes.IndexByte(block[:nameSize], 0)
	if end < 0 {
		end = nameSize
	}

	name := textcodec.DecodeCP932(block[:end])
	if len(name) == 0 {
		return EmeEntry{}, fmt.Errorf("corrupt EME entry name: %s", path)
	}

	frameSize := int32(binary.LittleEndian.Uint16(block[0x40:0x42]))
	rawPos := int32(binary.LittleEndian.Uint16(block[0x42:0x44]))
	size := binary.LittleEndian.Uint32(block[0x4C:0x50])
	unpackedSize := binary.LittleEndian.Uint32(block[0x50:0x54])
	offset := binary.LittleEndian.Uint32(block[0x54:0x58])
	if size > 0x7FFFFFFF || unpackedSize > 0x7FFFFFFF {
		return EmeEntry{}, fmt.Errorf("corrupt EME entry '%s': %s", name, path)
	}

	frameInitPos := int32(0)
	if frameSize != 0 {
		frameInitPos = (frameSize - rawPos) % frameSize
	}

	return EmeEntry{
		Name:         name,
		SubType:      int32(binary.LittleEndian.Uint32(block[0x48:0x4C])),
		Size:         int32(size),
		UnpackedSize: int32(unpackedSize),
		Offset:       int64(offset),
		FrameSize:    frameSize,
		FrameInitPos: frameInitPos,
	}, nil
}

func checkPlacement(entry *EmeEntry, length int64) error {
	if entry.Offset < 0 || entry.Size < 0 {
		return fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
	}

	if entry.SubType == 3 {
		if entry.Offset+scriptHeaderSize > length {
			return fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
		}

		return nil
	}

	header := int64(0)
	if entry.SubType == 4 {
		header = imageHeaderSize
	}

	if entry.Offset+header+int64(entry.Size) > length {
		return fmt.Errorf("EME entry '%s' is outside archive bounds", entry.Name)
	}

	return nil
}

func paletteBytes(header []byte) int {
	bpp := int(binary.LittleEndian.Uint16(header[0:2]) & 0xFF)
	colors := int(binary.LittleEndian.Uint16(header[6:8]))
	if colors == 0 || bpp == 7 {
		return 0
	}

	if colors < 3 {
		colors = 3
	}

	return colors * 4
}

func stripExtension(name string) string {
	dot := strings.LastIndexByte(name, '.')
	if dot > 0 {
		return name[:dot]
	}

	return name
}

func foldKey(name string) string {
	return strings.ToLower(name)
}

func readAt(stream *os.File, buffer []byte, offset int64) int {
	if len(buffer) == 0 {
		return 0
	}

	read, err := stream.ReadAt(buffer, offset)
	if err != nil && err != io.EOF {
		return read
	}

	return read
}
