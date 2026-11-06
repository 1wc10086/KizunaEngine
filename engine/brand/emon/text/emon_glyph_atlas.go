package text

import (
	"container/list"
	"sync"
)

type GlyphSource interface {
	Render(character rune, pixelSize int) *EmonGlyphBitmap
}

type StyledGlyphSource interface {
	RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *EmonGlyphBitmap
}

type EmonGlyphBitmap struct {
	Width  int
	Height int
	Alpha  []byte
}

func EmonGlyphRequestRender(source GlyphSource, character rune, pixelSize int, family string, weight int, quality int) *EmonGlyphBitmap {
	if source == nil {
		return nil
	}

	if pixelSize <= 0 {
		return nil
	}

	if styled, ok := source.(StyledGlyphSource); ok {
		return styled.RenderStyled(character, pixelSize, family, weight, quality)
	}

	return source.Render(character, pixelSize)
}

const DefaultGlyphAtlasCapacity = 4096

type glyphKey struct {
	character rune
	pixelSize int
	family    string
	weight    int
	quality   int
}

type EmonGlyphAtlas struct {
	source   GlyphSource
	gate     sync.Mutex
	cache    map[glyphKey]*EmonGlyphBitmap
	nodes    map[glyphKey]*list.Element
	order    *list.List
	capacity int
}

func NewEmonGlyphAtlas(source GlyphSource, capacity int) *EmonGlyphAtlas {
	if capacity < 1 {
		capacity = 1
	}

	return &EmonGlyphAtlas{
		source:   source,
		cache:    map[glyphKey]*EmonGlyphBitmap{},
		nodes:    map[glyphKey]*list.Element{},
		order:    list.New(),
		capacity: capacity,
	}
}

func (a *EmonGlyphAtlas) Capacity() int {
	return a.capacity
}

func (a *EmonGlyphAtlas) Render(character rune, pixelSize int) *EmonGlyphBitmap {
	return a.resolve(glyphKey{character: character, pixelSize: pixelSize})
}

func (a *EmonGlyphAtlas) RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *EmonGlyphBitmap {
	return a.resolve(glyphKey{character: character, pixelSize: pixelSize, family: family, weight: weight, quality: quality})
}

func (a *EmonGlyphAtlas) Clear() {
	a.gate.Lock()
	defer a.gate.Unlock()
	a.cache = map[glyphKey]*EmonGlyphBitmap{}
	a.nodes = map[glyphKey]*list.Element{}
	a.order.Init()
}

func (a *EmonGlyphAtlas) Count() int {
	a.gate.Lock()
	defer a.gate.Unlock()
	return len(a.cache)
}

func (a *EmonGlyphAtlas) resolve(key glyphKey) *EmonGlyphBitmap {
	if key.pixelSize <= 0 {
		return nil
	}

	a.gate.Lock()
	defer a.gate.Unlock()

	if cached, ok := a.cache[key]; ok {
		a.touch(key)
		return cached
	}

	bitmap := EmonGlyphRequestRender(a.source, key.character, key.pixelSize, key.family, key.weight, key.quality)
	a.cache[key] = bitmap
	a.nodes[key] = a.order.PushBack(key)
	for a.order.Len() > a.capacity {
		oldest := a.order.Front()
		if oldest == nil {
			break
		}

		a.order.Remove(oldest)
		oldKey := oldest.Value.(glyphKey)
		delete(a.cache, oldKey)
		delete(a.nodes, oldKey)
	}

	return bitmap
}

func (a *EmonGlyphAtlas) touch(key glyphKey) {
	node, ok := a.nodes[key]
	if !ok || node == nil {
		return
	}

	a.order.Remove(node)
	a.order.PushBack(key)
}

type EmonGlyphFallback struct{}

var EmonFallback = &EmonGlyphFallback{}

func (f *EmonGlyphFallback) Render(character rune, pixelSize int) *EmonGlyphBitmap {
	return f.RenderStyled(character, pixelSize, "", 0, 0)
}

func (f *EmonGlyphFallback) RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *EmonGlyphBitmap {
	if pixelSize <= 0 {
		return nil
	}

	if character == ' ' || character == '\t' {
		spaceWidth := pixelSize / 2
		if spaceWidth < 1 {
			spaceWidth = 1
		}
		return &EmonGlyphBitmap{Width: spaceWidth, Height: pixelSize, Alpha: make([]byte, spaceWidth*pixelSize)}
	}

	if character == '\u3000' {
		return &EmonGlyphBitmap{Width: pixelSize, Height: pixelSize, Alpha: make([]byte, pixelSize*pixelSize)}
	}

	if character >= 0x20 && character <= 0x7E {
		return emonRenderAscii(character, pixelSize)
	}

	return emonRenderBox(pixelSize)
}

func emonRenderAscii(character rune, pixelSize int) *EmonGlyphBitmap {
	width := pixelSize / 2
	if width < 1 {
		width = 1
	}

	alpha := make([]byte, width*pixelSize)
	offset := int(character-0x20) * 16
	for y := 0; y < pixelSize; y++ {
		row := emonGlyphRows[offset+((y*16)/pixelSize)]
		for x := 0; x < width; x++ {
			if row[(x*8)/width] == '#' {
				alpha[(y*width)+x] = 255
			}
		}
	}

	return &EmonGlyphBitmap{Width: width, Height: pixelSize, Alpha: alpha}
}

func emonRenderBox(pixelSize int) *EmonGlyphBitmap {
	alpha := make([]byte, pixelSize*pixelSize)
	edge := 1
	if pixelSize >= 32 {
		edge = 2
	}

	for y := 0; y < pixelSize; y++ {
		for x := 0; x < pixelSize; x++ {
			if x < edge || y < edge || x >= pixelSize-edge || y >= pixelSize-edge {
				alpha[(y*pixelSize)+x] = 255
			}
		}
	}

	return &EmonGlyphBitmap{Width: pixelSize, Height: pixelSize, Alpha: alpha}
}
