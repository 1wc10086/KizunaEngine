package text

import "strings"

const (
	EmonFontDefaultFamily = "MS Gothic"
	EmonFontDefaultSize   = 24
	EmonFontNormalWeight  = EmonSpriteWeightNormal
	EmonFontBoldWeight    = EmonSpriteWeightBold
)

type EmonFontFace struct {
	family   string
	size     int
	weight   int
	quality  int
	provider GlyphSource
}

func NewEmonFontFace(family string, size int, weight int, quality int) *EmonFontFace {
	face := &EmonFontFace{provider: EmonFallback}
	if strings.TrimSpace(family) == "" {
		face.family = EmonFontDefaultFamily
	} else {
		face.family = family
	}

	if size > 0 {
		face.size = size
	} else {
		face.size = EmonFontDefaultSize
	}

	if weight > 0 {
		face.weight = weight
	} else {
		face.weight = EmonFontNormalWeight
	}

	face.quality = quality
	return face
}

var DefaultEmonFontFace = NewEmonFontFace(EmonFontDefaultFamily, EmonFontDefaultSize, EmonFontNormalWeight, 0)

func (f *EmonFontFace) Family() string {
	return f.family
}

func (f *EmonFontFace) Size() int {
	return f.size
}

func (f *EmonFontFace) Weight() int {
	return f.weight
}

func (f *EmonFontFace) Quality() int {
	return f.quality
}

func (f *EmonFontFace) Bold() bool {
	return f.weight >= EmonFontBoldWeight
}

func (f *EmonFontFace) Provider() GlyphSource {
	return f.provider
}

func (f *EmonFontFace) SetProvider(source GlyphSource) {
	if source == nil {
		f.provider = EmonFallback
		return
	}

	f.provider = source
}

func EmonFontFaceFromStyle(style *EmonTextStyle) *EmonFontFace {
	if style == nil {
		return NewEmonFontFace(EmonFontDefaultFamily, EmonFontDefaultSize, EmonFontNormalWeight, 0)
	}

	return NewEmonFontFace(style.FontFamily, style.Size, EmonFontNormalWeight, 0)
}

func EmonFontFaceFromSprite(setting *EmonSpriteTextSetting) *EmonFontFace {
	if setting == nil {
		return NewEmonFontFace(EmonFontDefaultFamily, EmonFontDefaultSize, EmonFontNormalWeight, 0)
	}

	metrics := setting.Metrics()
	return NewEmonFontFace(setting.Family(), setting.Size(), metrics.Weight, metrics.Quality)
}

func (f *EmonFontFace) Render(character rune, pixelSize int) *EmonGlyphBitmap {
	return f.RenderStyled(character, pixelSize, f.family, f.weight, f.quality)
}

func (f *EmonFontFace) RenderStyled(character rune, pixelSize int, family string, weight int, quality int) *EmonGlyphBitmap {
	return EmonGlyphRequestRender(f.provider, character, pixelSize, family, weight, quality)
}
