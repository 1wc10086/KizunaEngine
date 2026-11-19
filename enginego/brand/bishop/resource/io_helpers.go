package resource

import (
	"io"
	"os"
)

func readAt(stream *os.File, buffer []byte, offset int64) int {
	read := 0
	for read < len(buffer) {
		chunk, err := stream.ReadAt(buffer[read:], offset+int64(read))
		read += chunk
		if err != nil {
			break
		}
	}
	return read
}

func readExactly(source io.Reader, buffer []byte) int {
	read := 0
	for read < len(buffer) {
		chunk, _ := source.Read(buffer[read:])
		if chunk <= 0 {
			break
		}
		read += chunk
	}
	return read
}

func readByte(source io.Reader) (byte, bool) {
	var one [1]byte
	if readExactly(source, one[:]) != 1 {
		return 0, false
	}
	return one[0], true
}

func fileLength(stream *os.File) int64 {
	info, err := stream.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}
