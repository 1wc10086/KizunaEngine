package state

type StageOverlayAnchor int

const (
	StageOverlayTopLeft StageOverlayAnchor = iota
	StageOverlayTopRight
	StageOverlayBottomBand
)

type StageOverlay struct {
	Image  string
	Anchor StageOverlayAnchor
	X      int
	Y      int
}
