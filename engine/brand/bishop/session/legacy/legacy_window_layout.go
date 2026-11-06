package legacy

import "fmt"

const (
	ScreenWidth            = 640
	ScreenHeight           = 480
	FaceX                  = 12
	FaceY                  = 372
	BackgroundMarginY      = 13
	TimerIconX             = 512
	TimerIconY             = 352
	ThumbnailWidth         = 160
	ThumbnailHeight        = 120
	EndingWaitMilliseconds = 3000
	DimStep                = 0x46
)

const (
	MenuImageName   = "menu.KG"
	WindowFrameName = "frame.KG"
	TableName       = "SCNDAT.TBL"
	ScnFileName     = "SCNFILE.PFT"
)

const (
	MessageTextX      = 152
	MessageTextY      = 400
	MessageTextSize   = 16
	MessageLineHeight = 16
	MessageTextWidth  = 336
	MessageTextLines  = 4
)

func TimerIconName(oneBased int) string {
	return fmt.Sprintf("TIME%d.KG", oneBased)
}

func ScenarioName(number int) string {
	return fmt.Sprintf("SCN%03d.SCF", number)
}

func BackgroundOriginX(width int) int {
	if width >= ScreenWidth {
		return 0
	}
	return (ScreenWidth - width) / 2
}

func BackgroundOriginY(height int) int {
	if height >= ScreenHeight {
		return 0
	}
	return BackgroundMarginY
}

type LegacyPanelKind int

const (
	LegacyPanelNone LegacyPanelKind = iota
	LegacyPanelCommand
	LegacyPanelSystem
	LegacyPanelSave
	LegacyPanelLoad
)

type LegacyPanel struct {
	Kind        LegacyPanelKind
	Title       string
	Items       []string
	Highlight   int
	Scroll      int
	InstantText bool
	BgmEnabled  bool
	Status      string
}

type LegacyPanelHit struct {
	Kind  string
	Index int
}

type legacyRect struct {
	X, Y, W, H int
}

func (r legacyRect) contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

func (r legacyRect) at(dx, dy, w, h int) legacyRect {
	return legacyRect{X: r.X + dx, Y: r.Y + dy, W: w, H: h}
}

func (r legacyRect) backButton() legacyRect {
	return r.at((r.W-legacyPanelBackW)/2, r.H-34, legacyPanelBackW, legacyPanelBackH)
}

const (
	legacyPanelTitleBarHeight = 24
	legacyPanelBodyTop        = 28
	legacyPanelBackW          = 80
	legacyPanelBackH          = 24
	legacyListRows            = 10
	legacyListRowH            = 26
	legacySlotCols            = 2
	legacySlotRows            = 12
	legacySlotCount           = legacySlotCols * legacySlotRows
	legacySlotW               = 264
	legacySlotGap             = 8
	legacySlotMargin          = 12
	legacySlotRowH            = 28
)

var (
	legacyCommandPanelRect = legacyRect{X: 90, Y: 80, W: 460, H: 340}
	legacySystemPanelRect  = legacyRect{X: 170, Y: 90, W: 300, H: 300}
	legacySavePanelRect    = legacyRect{X: 40, Y: 40, W: 560, H: 400}
	legacyFrameButtonRects = [3]legacyRect{
		{X: 133, Y: 354, W: 122, H: 27},
		{X: 260, Y: 354, W: 122, H: 27},
		{X: 387, Y: 354, W: 124, H: 27},
	}
	legacyTitleButtonRects = [3]legacyRect{
		{X: 176, Y: 346, W: 292, H: 40},
		{X: 176, Y: 388, W: 292, H: 40},
		{X: 176, Y: 429, W: 292, H: 42},
	}
)

func (p *LegacyPanel) rect() legacyRect {
	switch p.Kind {
	case LegacyPanelCommand:
		return legacyCommandPanelRect
	case LegacyPanelSystem:
		return legacySystemPanelRect
	case LegacyPanelSave, LegacyPanelLoad:
		return legacySavePanelRect
	}
	return legacyRect{}
}

func legacyCommandRowRect(panel legacyRect, row int) legacyRect {
	return panel.at(10, legacyPanelBodyTop+row*legacyListRowH, panel.W-20, legacyListRowH-2)
}

func legacySlotRect(panel legacyRect, slot int) legacyRect {
	row := slot / legacySlotCols
	col := slot % legacySlotCols
	return panel.at(legacySlotMargin+col*(legacySlotW+legacySlotGap), legacyPanelBodyTop+row*legacySlotRowH, legacySlotW, legacySlotRowH-2)
}

type legacyControl struct {
	Kind  string
	Label string
	Rect  legacyRect
}

func legacySystemControls(panel legacyRect) []legacyControl {
	return []legacyControl{
		{Kind: "instant", Label: "一気に表示する", Rect: panel.at(20, 48, 260, 22)},
		{Kind: "typewriter", Label: "１文字ずつ表示", Rect: panel.at(20, 74, 260, 22)},
		{Kind: "save", Label: "ＤＡＴＡ　ＳＡＶＥ", Rect: panel.at(20, 112, 260, 24)},
		{Kind: "load", Label: "ＤＡＴＡ　ＬＯＡＤ", Rect: panel.at(20, 144, 260, 24)},
		{Kind: "bgm", Label: "ＢＧＭの演奏を行う", Rect: panel.at(20, 184, 260, 22)},
	}
}

func (p *LegacyPanel) HitTest(x, y int) (LegacyPanelHit, bool) {
	if p == nil || p.Kind == LegacyPanelNone {
		return LegacyPanelHit{}, false
	}
	panel := p.rect()
	if !panel.contains(x, y) {
		return LegacyPanelHit{}, false
	}
	if panel.backButton().contains(x, y) {
		return LegacyPanelHit{Kind: "back"}, true
	}
	switch p.Kind {
	case LegacyPanelCommand:
		return p.hitCommand(panel, x, y)
	case LegacyPanelSystem:
		return hitSystemControls(panel, x, y)
	case LegacyPanelSave:
		return hitSlots(panel, x, y, "save")
	case LegacyPanelLoad:
		return hitSlots(panel, x, y, "load")
	}
	return LegacyPanelHit{}, false
}

func (p *LegacyPanel) hitCommand(panel legacyRect, x, y int) (LegacyPanelHit, bool) {
	for row := 0; row < legacyListRows; row++ {
		if !legacyCommandRowRect(panel, row).contains(x, y) {
			continue
		}
		index := p.Scroll + row
		if index < 0 || index >= len(p.Items) {
			return LegacyPanelHit{}, false
		}
		return LegacyPanelHit{Kind: "item", Index: index}, true
	}
	return LegacyPanelHit{}, false
}

func hitSystemControls(panel legacyRect, x, y int) (LegacyPanelHit, bool) {
	for _, control := range legacySystemControls(panel) {
		if control.Rect.contains(x, y) {
			return LegacyPanelHit{Kind: control.Kind}, true
		}
	}
	return LegacyPanelHit{}, false
}

func hitSlots(panel legacyRect, x, y int, kind string) (LegacyPanelHit, bool) {
	for slot := 0; slot < legacySlotCount; slot++ {
		if legacySlotRect(panel, slot).contains(x, y) {
			return LegacyPanelHit{Kind: kind, Index: slot}, true
		}
	}
	return LegacyPanelHit{}, false
}

func FrameButtonAt(x, y int) (int, bool) {
	for index, rect := range legacyFrameButtonRects {
		if rect.contains(x, y) {
			return index, true
		}
	}
	return 0, false
}

func TitleItemAt(x, y int) (int, bool) {
	for index, rect := range legacyTitleButtonRects {
		if rect.contains(x, y) {
			return index, true
		}
	}
	return 0, false
}
