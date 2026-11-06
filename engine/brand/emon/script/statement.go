package script

type EmonStatement struct {
	Command   string
	Raw       string
	Line      int32
	IsText    bool
	Text      string
	Arguments []EmonArgument
}

func (s *EmonStatement) Argument(index int32) EmonArgument {
	if index >= 0 && index < int32(len(s.Arguments)) {
		return s.Arguments[index]
	}
	return EmonArgument{}
}

func (s *EmonStatement) ArgumentCount() int32 {
	return int32(len(s.Arguments))
}
