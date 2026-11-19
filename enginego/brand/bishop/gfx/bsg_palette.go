package gfx

const (
	BsgPaletteColorCount = 256
	BsgPaletteEntrySize  = 4
)

type BsgPalette struct {
	entries []byte
}

func ReadBsgPalette(data []byte, offset int) *BsgPalette {
	length := BsgPaletteColorCount * BsgPaletteEntrySize
	if offset <= 0 || len(data) < offset+length {
		return nil
	}

	entries := make([]byte, length)
	copy(entries, data[offset:offset+length])
	return &BsgPalette{entries: entries}
}

func (p *BsgPalette) Write(index int, destination []byte) {
	source := (index & 0xFF) * BsgPaletteEntrySize
	destination[0] = p.entries[source]
	destination[1] = p.entries[source+1]
	destination[2] = p.entries[source+2]
	destination[3] = p.entries[source+3]
}
