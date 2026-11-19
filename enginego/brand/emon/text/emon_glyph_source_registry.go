package text

var currentGlyphSource GlyphSource

func CurrentGlyphSource() GlyphSource {
	return currentGlyphSource
}

func SetCurrentGlyphSource(source GlyphSource) {
	currentGlyphSource = source
}

func ResolveGlyphSource() GlyphSource {
	if currentGlyphSource != nil {
		return currentGlyphSource
	}

	return EmonFallback
}
