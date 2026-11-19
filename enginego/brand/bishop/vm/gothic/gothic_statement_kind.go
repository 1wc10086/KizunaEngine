package gothic

type GothicStatementKind int

const (
	GothicStatementEmpty GothicStatementKind = iota
	GothicStatementText
	GothicStatementWaitBreak
	GothicStatementPageBreak
	GothicStatementLabel
	GothicStatementCommand
	GothicStatementAssignment
	GothicStatementFinish
)
