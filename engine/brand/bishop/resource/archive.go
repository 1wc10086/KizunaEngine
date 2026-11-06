package resource

type Archive interface {
	Name() string
	Version() int
	Length() int64
	Entries() []ArchiveEntry
	Read(offset int64, buffer []byte) int
	ReadAll(entry ArchiveEntry) ([]byte, error)
	Dispose()
}
