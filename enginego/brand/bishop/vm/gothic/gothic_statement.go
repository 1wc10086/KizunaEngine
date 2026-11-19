package gothic

type GothicStatement struct {
	Kind   GothicStatementKind
	Index  int32
	Raw    string
	Name   string
	Sub    string
	Args   []string
	Groups []string
	Offset int64
}

func NewGothicStatement(kind GothicStatementKind, index int32, raw, name, sub string, args, groups []string, offset int64) *GothicStatement {
	return &GothicStatement{
		Kind:   kind,
		Index:  index,
		Raw:    raw,
		Name:   name,
		Sub:    sub,
		Args:   args,
		Groups: groups,
		Offset: offset,
	}
}
