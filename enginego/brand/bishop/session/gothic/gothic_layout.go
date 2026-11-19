package gothic

const (
	GothicScreenWidth  = 640
	GothicScreenHeight = 480
)

const (
	GothicTextOriginX = 21
	GothicTextOriginY = 10
	GothicTextColumns = 26
	GothicTextRows    = 16
)

const (
	GothicToku1TextOriginX = 96
	GothicToku1TextOriginY = 340
	GothicToku1TextColumns = 26
	GothicToku1TextRows    = 4
)

const (
	GothicThumbnailWidth  = 160
	GothicThumbnailHeight = 120
)

func GothicTextOriginXFor(toku1 bool) int {
	if toku1 {
		return GothicToku1TextOriginX
	}
	return GothicTextOriginX
}

func GothicTextOriginYFor(toku1 bool) int {
	if toku1 {
		return GothicToku1TextOriginY
	}
	return GothicTextOriginY
}

func GothicTextColumnsFor(toku1 bool) int {
	if toku1 {
		return GothicToku1TextColumns
	}
	return GothicTextColumns
}

func GothicTextRowsFor(toku1 bool) int {
	if toku1 {
		return GothicToku1TextRows
	}
	return GothicTextRows
}

const (
	GothicDefaultBlue  = byte(128)
	GothicDefaultGreen = byte(64)
	GothicDefaultRed   = byte(64)
	GothicDefaultAlpha = byte(255)

	GothicSepiaBlue  = byte(9)
	GothicSepiaGreen = byte(54)
	GothicSepiaRed   = byte(126)
	GothicSepiaAlpha = byte(255)

	GothicDarkBlue  = byte(86)
	GothicDarkGreen = byte(86)
	GothicDarkRed   = byte(86)
	GothicDarkAlpha = byte(255)

	GothicRedBlue  = byte(56)
	GothicRedGreen = byte(56)
	GothicRedRed   = byte(112)
	GothicRedAlpha = byte(255)

	GothicBlueBlue  = byte(112)
	GothicBlueGreen = byte(56)
	GothicBlueRed   = byte(56)
	GothicBlueAlpha = byte(255)
)
