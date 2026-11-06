package tj2

type Tj2Command struct {
	Kind    Tj2CommandKind
	Index   int
	Flag    int
	Value   int
	Text    string
	Options []Tj2Choice
	OffsetX int
	OffsetY int
}
