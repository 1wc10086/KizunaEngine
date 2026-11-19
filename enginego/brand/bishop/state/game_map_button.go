package state

type GameMapButton struct {
	Index  int
	Image  string
	X      int
	Y      int
	Width  int
	Height int
	Value  int
	Label  string
}

func NewGameMapButton(index int, image string, x, y, width, height, value int) GameMapButton {
	return GameMapButton{Index: index, Image: image, X: x, Y: y, Width: width, Height: height, Value: value}
}

func NewGameMapButtonWithLabel(index int, image string, x, y, width, height, value int, label string) GameMapButton {
	return GameMapButton{Index: index, Image: image, X: x, Y: y, Width: width, Height: height, Value: value, Label: label}
}
