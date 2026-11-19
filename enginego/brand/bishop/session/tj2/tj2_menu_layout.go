package tj2

const (
	MenuScreenWidth  = ScreenWidth
	MenuScreenHeight = ScreenHeight

	MenuMessageX      = MessageOriginX
	MenuMessageY      = MessageOriginY
	MenuMessageWidth  = 608
	MenuMessageHeight = 112

	MenuTextRows    = 4
	MenuTextColumns = 52
	MenuLineHeight  = 22
	MenuAdvance     = 19
	MenuGlyphWidth  = 20
	MenuGlyphHeight = 22

	MenuMessageButtonX      = 540
	MenuMessageButtonWidth  = 67
	MenuMessageButtonHeight = 24

	MenuSkipButtonRow = 0
	MenuSaveButtonRow = 1
	MenuLoadButtonRow = 2
	MenuLogButtonRow  = 3

	MenuChoiceRowX      = 13
	MenuChoiceRowY      = 169
	MenuChoiceRowWidth  = 496
	MenuChoiceRowHeight = 42
	MenuChoiceRowStep   = 50

	MenuChoiceIconX      = 16
	MenuChoiceIconWidth  = 32
	MenuChoiceIconHeight = 42

	MenuClickCursorSize         = 24
	MenuClickCursorFramesTop    = 7
	MenuClickCursorFramesBottom = 4
	MenuClickCursorIntervalMs   = 100

	MenuTitleButtonX      = 464
	MenuTitleButtonWidth  = 160
	MenuTitleButtonHeight = 40

	MenuOmakeButtonX     = 160
	MenuOmakeButtonWidth = 320

	MenuGalleryCropWidth  = 64
	MenuGalleryCropHeight = 48

	MenuSceneThumbnailWidth  = 96
	MenuSceneThumbnailHeight = 72

	MenuSavePanelMaxSlots = 12
)

var MenuMessageButtonY = []int{8, 32, 56, 80}

var MenuTitleButtonY = []int{256, 304, 352, 400}

var MenuOmakeButtonY = []int{144, 224, 304, 416}
