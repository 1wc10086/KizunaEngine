package resource

type ArchiveEntry struct {
	Path   string
	Offset int64
	Size   int32
}

func (e ArchiveEntry) End() int64 {
	return e.Offset + int64(e.Size)
}
