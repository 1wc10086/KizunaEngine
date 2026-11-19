package resource

import "bytes"

const (
	ArchiveSignatureHeaderSize          = 16
	ArchiveSignatureV1EntrySize         = 0x28
	ArchiveSignatureV2EntrySize         = 0xC
	ArchiveSignaturePackedV2EntrySize   = 0x28
	ArchiveSignaturePackedV2NameSize    = 0x18
	ArchiveSignaturePackedV2OffsetField = 0x20
	ArchiveSignaturePackedV2SizeField   = 0x24
	ArchiveSignaturePftEntrySize        = 16
	ArchiveSignaturePftNameSize         = 8
)

var ArchiveSignatureBsa = []byte("BSArc")

func ArchiveSignatureIsBsaHeader(header []byte) bool {
	return len(header) >= ArchiveSignatureHeaderSize && bytes.Equal(header[:5], ArchiveSignatureBsa)
}
