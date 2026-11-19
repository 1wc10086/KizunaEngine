package gothic

type GothicAction struct {
	Kind      GothicActionKind
	Statement *GothicStatement
	Value     int32
	Text      string
	Menu      *GothicMenu
}

func NewGothicAction(kind GothicActionKind, statement *GothicStatement, value int32, text string, menu *GothicMenu) *GothicAction {
	return &GothicAction{
		Kind:      kind,
		Statement: statement,
		Value:     value,
		Text:      text,
		Menu:      menu,
	}
}
