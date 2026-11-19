package vm

const (
	OpcodeHalt              byte = 0x00
	OpcodeReturnOne         byte = 0x01
	OpcodeReturnTwo         byte = 0x02
	OpcodeGotoLabel         byte = 0x03
	OpcodeJump              byte = 0x04
	OpcodeJumpWhenTrue      byte = 0x05
	OpcodeJumpWhenFalse     byte = 0x06
	OpcodeDeferLabel        byte = 0x07
	OpcodeDeferLabelCall    byte = 0x08
	OpcodeCall              byte = 0x09
	OpcodeReturn            byte = 0x0A
	OpcodeResetFrame        byte = 0x0B
	OpcodeGreater           byte = 0x0C
	OpcodeLess              byte = 0x0D
	OpcodeGreaterOrEqual    byte = 0x0E
	OpcodeLessOrEqual       byte = 0x0F
	OpcodeNotEqual          byte = 0x10
	OpcodeEqual             byte = 0x11
	OpcodeMove              byte = 0x12
	OpcodeOr                byte = 0x13
	OpcodeAnd               byte = 0x14
	OpcodeAdd               byte = 0x15
	OpcodeSubtract          byte = 0x16
	OpcodeMultiply          byte = 0x17
	OpcodeDivide            byte = 0x18
	OpcodeModulo            byte = 0x19
	OpcodeNegate            byte = 0x1A
	OpcodeIncrement         byte = 0x1B
	OpcodeDecrement         byte = 0x1C
	OpcodeDisplayMessage    byte = 0x1D
	OpcodeFirstCommand      byte = 0x1E
	OpcodeLastCommand       byte = 0x32
	OpcodeFetch             byte = 0x33
	OpcodeSelectArgument    byte = 0x34
	OpcodeSelectCommand     byte = 0x35
	OpcodeSelectOption      byte = 0x36
	OpcodeSelectCommandTail byte = 0x37
	OpcodeDefineLabel       byte = 0x38
	OpcodeLabelCommand      byte = 0x39
	OpcodeExclusiveOr       byte = 0x3A
	OpcodeShiftLeft         byte = 0x3B
	OpcodeShiftRight        byte = 0x3C
	OpcodeComplement        byte = 0x3D
	OpcodeInvoke            byte = 0x3E
	OpcodeAttach            byte = 0x3F
	OpcodeEvaluate          byte = 0x40
	OpcodeTranslate         byte = 0x41
)

const (
	OpcodeBinarySize        = 18
	OpcodeUnarySize         = 12
	OpcodeFetchSize         = 13
	OpcodeFixedArgumentSize = 5
)
