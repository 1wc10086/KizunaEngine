package state

type BsLayerEntry struct {
	Name     string
	X        int
	Y        int
	Kind     StageLayerKind
	Artworks []LayerArtwork
	Visible  bool
}

func NewBsLayerEntry(name string, x, y int, kind StageLayerKind, artworks []LayerArtwork) BsLayerEntry {
	return BsLayerEntry{Name: name, X: x, Y: y, Kind: kind, Artworks: artworks, Visible: true}
}

func NewBsLayerEntryWithVisible(name string, x, y int, kind StageLayerKind, artworks []LayerArtwork, visible bool) BsLayerEntry {
	return BsLayerEntry{Name: name, X: x, Y: y, Kind: kind, Artworks: artworks, Visible: visible}
}

func (e BsLayerEntry) Moved(x, y int) BsLayerEntry {
	if x == e.X && y == e.Y {
		return e
	}
	return BsLayerEntry{Name: e.Name, X: x, Y: y, Kind: e.Kind, Artworks: e.shifted(x-e.X, y-e.Y), Visible: e.Visible}
}

func (e BsLayerEntry) WithArtworks(name string, artworks []LayerArtwork) BsLayerEntry {
	return BsLayerEntry{Name: name, X: e.X, Y: e.Y, Kind: e.Kind, Artworks: artworks, Visible: e.Visible}
}

func (e BsLayerEntry) shifted(dx, dy int) []LayerArtwork {
	if dx == 0 && dy == 0 {
		return e.Artworks
	}
	shifted := make([]LayerArtwork, len(e.Artworks))
	for index, artwork := range e.Artworks {
		artwork.X += dx
		artwork.Y += dy
		shifted[index] = artwork
	}
	return shifted
}
