package vm

type ScriptCommandId int

const (
	Halt              ScriptCommandId = 0
	ReturnOne         ScriptCommandId = 1
	ReturnTwo         ScriptCommandId = 2
	DisplayMessage    ScriptCommandId = 29
	Image             ScriptCommandId = 30
	ImageExtended     ScriptCommandId = 31
	ReleaseLayer      ScriptCommandId = 32
	ChangeImage       ScriptCommandId = 33
	ShowLayerImage    ScriptCommandId = 34
	LayerParameter    ScriptCommandId = 35
	LayerCurve        ScriptCommandId = 36
	RegisterCg        ScriptCommandId = 37
	WindowVisibility  ScriptCommandId = 38
	Transition        ScriptCommandId = 39
	Mask              ScriptCommandId = 40
	ObjectCommand     ScriptCommandId = 41
	ScreenEffect      ScriptCommandId = 42
	Fill              ScriptCommandId = 43
	PlayBgm           ScriptCommandId = 44
	StopBgm           ScriptCommandId = 45
	PlayAmbient       ScriptCommandId = 46
	AmbientCommand    ScriptCommandId = 47
	PlayVoiceTrack    ScriptCommandId = 48
	VoiceTrackCommand ScriptCommandId = 49
	PlayEffect        ScriptCommandId = 50
	Select            ScriptCommandId = 52
	SelectNext        ScriptCommandId = 53
	SelectEntry       ScriptCommandId = 54
	SelectTail        ScriptCommandId = 55
	DefineLabel       ScriptCommandId = 56
	Character         ScriptCommandId = 57
	Invoke            ScriptCommandId = 62
	Attach            ScriptCommandId = 63
	Evaluate          ScriptCommandId = 64
	Translate         ScriptCommandId = 65
	Restart           ScriptCommandId = 0x100
	Wait              ScriptCommandId = 0x101
	SystemCall        ScriptCommandId = 0x102
	Dialog            ScriptCommandId = 0x103
	ClearScreen       ScriptCommandId = 0x104
	None              ScriptCommandId = 0x105
	BsLayerControl    ScriptCommandId = 0x106
	BsLayerImage      ScriptCommandId = 0x107
	BsLayerRename     ScriptCommandId = 0x108
	BsStopSound       ScriptCommandId = 0x109
)
