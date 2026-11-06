package resource

type ZbmEntry struct {
	Name         string
	DataOffset   int64
	PackedSize   int
	UnpackedSize int
	Attributes   byte
	Timestamp    []byte
}

func (e ZbmEntry) DataEnd() int64 {
	return e.DataOffset + int64(e.PackedSize)
}
