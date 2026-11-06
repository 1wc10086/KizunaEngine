package glyph

import (
	"container/list"
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"kizuna/engine/brand/emon/text"
	"kizuna/engine/core"
)

const (
	glyphBaselineRatio  = 0.86
	glyphCacheCapacity  = 4096
	glyphRasterDPI      = 72.0
	glyphFixedPointOne  = 64
	glyphFullWidthLimit = 0x80
	glyphHalfKanaFirst  = 0xFF61
	glyphHalfKanaLast   = 0xFF9F
	glyphIdeographicSpc = '\u3000'
)

type glyphSourceKey struct {
	character rune
	pixelSize int
	family    string
	weight    int
	quality   int
}

type typefaceKey struct {
	canonical string
	weight    int
	pixelSize int
}

type EmonGlyphSource struct {
	gate     sync.Mutex
	cache    map[glyphSourceKey]*text.EmonGlyphBitmap
	nodes    map[glyphSourceKey]*list.Element
	recent   *list.List
	faces    map[typefaceKey]font.Face
	reported bool
}

func NewEmonGlyphSource() *EmonGlyphSource {
	return &EmonGlyphSource{
		cache:  map[glyphSourceKey]*text.EmonGlyphBitmap{},
		nodes:  map[glyphSourceKey]*list.Element{},
		recent: list.New(),
		faces:  map[typefaceKey]font.Face{},
	}
}

var Instance = NewEmonGlyphSource()

func init() {
	text.SetCurrentGlyphSource(Instance)
}

func Install() {
	text.SetCurrentGlyphSource(Instance)
}

func (s *EmonGlyphSource) Render(character rune, pixelSize int) *text.EmonGlyphBitmap {
	return s.resolve(glyphSourceKey{character: character, pixelSize: pixelSize})
}

func (s *EmonGlyphSource) RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *text.EmonGlyphBitmap {
	return s.resolve(glyphSourceKey{character: character, pixelSize: pixelSize, family: family, weight: weight, quality: quality})
}

func (s *EmonGlyphSource) ClearCache() {
	s.gate.Lock()
	defer s.gate.Unlock()
	s.cache = map[glyphSourceKey]*text.EmonGlyphBitmap{}
	s.nodes = map[glyphSourceKey]*list.Element{}
	s.recent.Init()
	s.faces = map[typefaceKey]font.Face{}
}

func (s *EmonGlyphSource) resolve(key glyphSourceKey) *text.EmonGlyphBitmap {
	if key.pixelSize <= 0 {
		return nil
	}

	s.gate.Lock()
	if cached, ok := s.cache[key]; ok {
		s.touchLocked(key)
		s.gate.Unlock()
		return cached
	}
	s.gate.Unlock()

	bitmap := s.rasterize(key.character, key.pixelSize, key.family, key.weight)

	s.gate.Lock()
	s.cache[key] = bitmap
	s.touchLocked(key)
	s.evictLocked()
	s.gate.Unlock()

	return bitmap
}

func (s *EmonGlyphSource) rasterize(character rune, pixelSize int, family string, weight int) *text.EmonGlyphBitmap {
	if character < ' ' || character == glyphIdeographicSpc {
		blank := glyphCellWidth(character, pixelSize)
		return &text.EmonGlyphBitmap{Width: blank, Height: pixelSize, Alpha: make([]byte, blank*pixelSize)}
	}

	bitmap, err := s.draw(character, pixelSize, family, weight)
	if err == nil && bitmap != nil {
		return bitmap
	}

	if err != nil {
		s.report(err)
	}

	return text.EmonFallback.Render(character, pixelSize)
}

func (s *EmonGlyphSource) draw(character rune, pixelSize int, family string, weight int) (bitmap *text.EmonGlyphBitmap, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			bitmap = nil
			err = fmt.Errorf("%v", recovered)
		}
	}()

	face := s.resolveFace(family, weight, pixelSize)
	if face == nil {
		return nil, errors.New("emon glyph font face unavailable")
	}

	advance, ok := face.GlyphAdvance(character)
	if !ok {
		return nil, nil
	}

	advancePixels := float64(advance) / float64(glyphFixedPointOne)
	if math.IsNaN(advancePixels) || math.IsInf(advancePixels, 0) {
		return nil, nil
	}

	minimum := glyphCellWidth(character, pixelSize)
	width := core.Clamp(int(math.Ceil(advancePixels)), minimum, pixelSize*2)
	height := pixelSize

	target := image.NewAlpha(image.Rect(0, 0, width, height))
	baseline := int(math.Round(float64(pixelSize) * glyphBaselineRatio))
	if baseline < 0 {
		baseline = 0
	}

	drawer := &font.Drawer{
		Dst:  target,
		Src:  image.White,
		Face: face,
		Dot:  fixed.Point26_6{X: 0, Y: fixed.I(baseline)},
	}
	drawer.DrawString(string(character))

	alpha := make([]byte, width*height)
	for index := range alpha {
		alpha[index] = target.Pix[index]
	}

	if glyphIsBlank(alpha) {
		return nil, nil
	}

	return &text.EmonGlyphBitmap{Width: width, Height: height, Alpha: alpha}, nil
}

func (s *EmonGlyphSource) resolveFace(family string, weight int, pixelSize int) font.Face {
	canonical := CanonicalKey(family)
	key := typefaceKey{canonical: canonical, weight: weight, pixelSize: pixelSize}

	s.gate.Lock()
	if cached, ok := s.faces[key]; ok {
		s.gate.Unlock()
		return cached
	}
	s.gate.Unlock()

	_ = ResolveFamilyList(canonical, family)

	parsed, err := embeddedFont()
	if err != nil {
		return nil
	}

	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    float64(pixelSize),
		DPI:     glyphRasterDPI,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil
	}

	s.gate.Lock()
	s.faces[key] = face
	s.gate.Unlock()

	return face
}

func (s *EmonGlyphSource) touchLocked(key glyphSourceKey) {
	if node, ok := s.nodes[key]; ok {
		s.recent.Remove(node)
	}

	s.nodes[key] = s.recent.PushBack(key)
}

func (s *EmonGlyphSource) evictLocked() {
	for len(s.cache) > glyphCacheCapacity {
		oldest := s.recent.Front()
		if oldest == nil {
			return
		}

		s.recent.Remove(oldest)
		oldKey := oldest.Value.(glyphSourceKey)
		delete(s.nodes, oldKey)
		delete(s.cache, oldKey)
	}
}

func (s *EmonGlyphSource) report(err error) {
	s.gate.Lock()
	if s.reported {
		s.gate.Unlock()
		return
	}

	s.reported = true
	s.gate.Unlock()

	core.Logf("emon-glyph: glyph rasterisation failed, built-in table in use: %T %v", err, err)
}

func glyphCellWidth(character rune, pixelSize int) int {
	if glyphIsHalfWidth(character) {
		width := pixelSize / 2
		if width < 1 {
			width = 1
		}

		return width
	}

	return pixelSize
}

func glyphIsHalfWidth(character rune) bool {
	return character < glyphFullWidthLimit || (character >= glyphHalfKanaFirst && character <= glyphHalfKanaLast)
}

func glyphIsBlank(alpha []byte) bool {
	for _, value := range alpha {
		if value != 0 {
			return false
		}
	}

	return true
}
