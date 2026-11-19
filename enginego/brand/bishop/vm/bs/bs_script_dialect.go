package bs

type BsScriptDialect int

const (
	BsScriptAncientWide BsScriptDialect = iota
	BsScriptAncientCompact
	BsScriptRev2
	BsScriptRev3
	BsScriptRev4
	BsScriptRev5
	BsScriptRev6
)
