package text

type EmonTextStyle struct {
	Size                 int
	FontFamily           string
	X                    int
	Y                    int
	SpeakerX             int
	SpeakerY             int
	LineHeight           int
	MaxCharactersPerLine int
	MaxLines             int
}

func NewEmonTextStyle() *EmonTextStyle {
	return &EmonTextStyle{
		Size:                 24,
		FontFamily:           "MS Gothic",
		X:                    40,
		Y:                    57,
		SpeakerX:             40,
		SpeakerY:             26,
		LineHeight:           32,
		MaxCharactersPerLine: 34,
		MaxLines:             4,
	}
}
