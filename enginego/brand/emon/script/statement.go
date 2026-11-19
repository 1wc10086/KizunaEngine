package script

type EmonStatement struct {
	Command string
	Raw     string
	Line    int32
	IsText  bool
	// Inline 表示这条正文是"同一脚本行里命令之后的后半句"（ZUBUNURE
	// 的 `閉めた。#立ち消去#…ご丁寧に鍵をかける音…`）。原引擎里它和前半句
	// 属于同一段正文，追加时不能插入换行。
	Inline    bool
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
