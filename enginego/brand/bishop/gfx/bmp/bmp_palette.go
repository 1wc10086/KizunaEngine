package bmp

const (
	BmpPaletteColorCount = 256
	BmpPaletteEntrySize  = 4
)

func ReadBmpPalette(header *BmpHeader, data []byte) []byte {
	if header.PaletteCount <= 0 {
		return nil
	}

	entries := make([]byte, BmpPaletteColorCount*BmpPaletteEntrySize)
	for offset := 0; offset < len(entries); offset += BmpPaletteEntrySize {
		entries[offset+3] = 255
	}

	count := min(header.PaletteCount, BmpPaletteColorCount)
	for index := 0; index < count; index++ {
		origin := header.PaletteOffset + index*header.PaletteEntrySize
		target := index * BmpPaletteEntrySize
		entries[target] = data[origin]
		entries[target+1] = data[origin+1]
		entries[target+2] = data[origin+2]
		entries[target+3] = 255
	}

	return entries
}
