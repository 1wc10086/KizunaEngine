package scf

type ScfCommand struct {
	Kind    ScfCommandKind
	Index   int
	Value   int
	Flag    int
	Text    string
	Options []ScfChoice
}

var ScfCommandNone = ScfCommand{Kind: ScfCommandKindNone}

var ScfCommandHalt = ScfCommand{Kind: ScfCommandKindHalt}

func ScfCommandFault(text string) ScfCommand {
	return ScfCommand{Kind: ScfCommandKindFaulted, Text: text}
}
