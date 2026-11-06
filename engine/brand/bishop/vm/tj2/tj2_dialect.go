package tj2

type Tj2Variant int

const (
	Tj2VariantModern Tj2Variant = iota
	Tj2VariantClassic
)

type Tj2Dialect struct {
	Variant       Tj2Variant
	EngineVersion int
	FlagUnitWidth int
	FlagMaximum   int
}

func ModernDialect() Tj2Dialect {
	return Tj2Dialect{Variant: Tj2VariantModern, EngineVersion: 220, FlagUnitWidth: 1, FlagMaximum: 255}
}

func ClassicDialect() Tj2Dialect {
	return Tj2Dialect{Variant: Tj2VariantClassic, EngineVersion: 217, FlagUnitWidth: 2, FlagMaximum: 65535}
}

func CreateDialect(engineVersion, flagUnitWidth int) Tj2Dialect {
	if flagUnitWidth <= 1 {
		return Tj2Dialect{Variant: Tj2VariantModern, EngineVersion: engineVersion, FlagUnitWidth: 1, FlagMaximum: 255}
	}
	return Tj2Dialect{Variant: Tj2VariantClassic, EngineVersion: engineVersion, FlagUnitWidth: flagUnitWidth, FlagMaximum: 65535}
}

func (d Tj2Dialect) LiteralWidth() int {
	return d.FlagUnitWidth
}

func (d Tj2Dialect) WideFlags() bool {
	return d.FlagUnitWidth > 1
}

func (d Tj2Dialect) UsesFlagAliases() bool {
	return d.EngineVersion >= 0xD5
}

func (d Tj2Dialect) FlagBufferSize(flagCount int) int {
	return d.FlagUnitWidth*flagCount + 16
}
