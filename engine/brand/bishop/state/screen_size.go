package state

type ScreenSize struct {
	Width  int
	Height int
}

func (s ScreenSize) Area() int64 {
	return int64(s.Width) * int64(s.Height)
}

func (s ScreenSize) Aspect() float64 {
	if s.Height <= 0 {
		return 0
	}
	return float64(s.Width) / float64(s.Height)
}
