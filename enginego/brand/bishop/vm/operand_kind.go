package vm

type OperandKind uint16

const (
	OperandImmediate    OperandKind = 0x100
	OperandImmediateAlt OperandKind = 0x101
	OperandVariable     OperandKind = 0x102
	OperandProperty     OperandKind = 0x103
	OperandFrame        OperandKind = 0x104
	OperandRegister     OperandKind = 0x105
	OperandScalar       OperandKind = 0x106
)
