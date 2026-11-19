package disc

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildTestIS3 构造一个只含一个未压缩条目的最小 IS3 归档：
// 头部 63 字节，随后是目录表、文件表和数据。
func buildTestIS3(payload []byte) []byte {
	const name = "SCNDAT.TBL"
	dirTable := make([]byte, 12)
	binary.LittleEndian.PutUint16(dirTable[0:], 1)
	binary.LittleEndian.PutUint16(dirTable[2:], 12)
	binary.LittleEndian.PutUint16(dirTable[4:], 6)
	copy(dirTable[6:], "Group1")

	fileTable := make([]byte, 30+len(name))
	binary.LittleEndian.PutUint32(fileTable[3:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(fileTable[7:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(fileTable[19:], is3AttribUncompressed)
	binary.LittleEndian.PutUint16(fileTable[23:], uint16(len(fileTable)))
	fileTable[29] = byte(len(name))
	copy(fileTable[30:], name)

	dirsOffset := is3HeaderOffset + is3HeaderSize
	filesOffset := dirsOffset + len(dirTable)
	dataOffset := filesOffset + len(fileTable)
	binary.LittleEndian.PutUint32(fileTable[11:], uint32(dataOffset))

	buf := make([]byte, dataOffset+len(payload))
	binary.LittleEndian.PutUint32(buf[0:], is3Magic)
	buf[4] = is3HeaderLengthByte
	h := buf[is3HeaderOffset:]
	binary.LittleEndian.PutUint32(h[36:], uint32(dirsOffset))
	binary.LittleEndian.PutUint32(h[40:], uint32(len(dirTable)))
	binary.LittleEndian.PutUint32(h[46:], uint32(filesOffset))
	binary.LittleEndian.PutUint32(h[50:], uint32(len(fileTable)))
	copy(buf[dirsOffset:], dirTable)
	copy(buf[filesOffset:], fileTable)
	copy(buf[dataOffset:], payload)
	return buf
}

func TestParseIS3EntriesAndPayload(t *testing.T) {
	archive := buildTestIS3([]byte("HELLO"))
	entries, err := parseIS3(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if got := entries[0].Path; got != `Group1\SCNDAT.TBL` {
		t.Errorf("path = %q", got)
	}
	if got := entries[0].payloadSize(); got != 5 {
		t.Errorf("payloadSize = %d, want 5", got)
	}
	payload, err := is3Payload(bytes.NewReader(archive), entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "HELLO" {
		t.Errorf("payload = %q, want HELLO", payload)
	}
}

func TestParseIS3RejectsBadMagic(t *testing.T) {
	data := make([]byte, 128)
	if _, err := parseIS3(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected error for bad magic")
	}
}

func TestExtractIS3WritesFiles(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "_SETUP.1")
	if err := os.WriteFile(archive, buildTestIS3([]byte("HELLO")), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := extractIS3(archive, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "Group1", "SCNDAT.TBL"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HELLO" {
		t.Errorf("extracted = %q, want HELLO", got)
	}
}
