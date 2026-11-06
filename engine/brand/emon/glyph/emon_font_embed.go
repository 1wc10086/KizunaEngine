package glyph

import (
	_ "embed"
	"sync"

	"golang.org/x/image/font/opentype"
)

//go:embed NotoSansSC-Subset.ttf
var notoSansSCSubset []byte

//go:embed LICENSE-OFL.txt
var licenseOFL []byte

var (
	embeddedFontOnce sync.Once
	embeddedFontData *opentype.Font
	embeddedFontErr  error
)

func embeddedFont() (*opentype.Font, error) {
	embeddedFontOnce.Do(func() {
		embeddedFontData, embeddedFontErr = opentype.Parse(notoSansSCSubset)
	})

	return embeddedFontData, embeddedFontErr
}
