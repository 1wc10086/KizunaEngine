package tj2

type Tj2CommandKind int

const (
	Tj2CommandKindNone Tj2CommandKind = iota
	Tj2CommandKindBackground
	Tj2CommandKindStandee
	Tj2CommandKindMessage
	Tj2CommandKindChapter
	Tj2CommandKindFunction
	Tj2CommandKindPlayBgm
	Tj2CommandKindPlaySound
	Tj2CommandKindPlayVoice
	Tj2CommandKindLoadScenario
	Tj2CommandKindWait
	Tj2CommandKindWipe
	Tj2CommandKindClearFlags
	Tj2CommandKindName
	Tj2CommandKindHalt
	Tj2CommandKindFaulted
	// 旧版 SCF 的 !r / !w 是"读档/存档对话框"而不是立即完成的空指令。
	// 单独给两个 kind，让 session 能把剧本停在对话框上等玩家操作；
	// 追加在枚举末尾，不影响既有 kind 的数值。
	Tj2CommandKindLoadDialog
	Tj2CommandKindSaveDialog
)
