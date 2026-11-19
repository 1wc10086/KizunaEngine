package config

const (
	SelectDesignAlignLeft   = 0
	SelectDesignAlignCenter = 1
	SelectDesignAlignRight  = 2

	SelectWindowDefaultDatumLine = 363
)

type SelectDesign struct {
	Item         string
	Interval     int
	TextX        int
	TextY        int
	Alignment    int
	DefaultColor uint32
	AlreadyColor uint32
	SelectColor  uint32
	// HeaderFooter 表示选项条自带上下装饰图（<item>_head / <item>_foot）。
	// 这些图不参与命中，但决定选项块的实际高度，排布时要算进去。
	HeaderFooter bool
	// HeaderFooterOnce 表示头/脚只画一次（整组选项的顶部和底部），
	// 而不是每一项都画一对。ヒメドレイ 的 item0_head/foot 是整组窗口的
	// 装饰盖，逐项画会在每两行之间叠出两层花纹。
	HeaderFooterOnce bool
	// Frames 是选项条图片的竖排帧数：0 沿用旧约定（2 帧：常态/悬停）；
	// 学園（GK）的 item0 是单帧 520x48 的整条，不能按 2 帧切成两半。
	Frames int
	// SelectedItem 是悬停时替换选项条用的图片名。零值/空字符串保持旧行为：
	// 悬停只切到 Frame 的第 1 帧（或 2 帧约定的悬停帧），不换图。
	// 有值时渲染器改画 SelectedItem 的同帧位切片，适配常态/悬停为两张图的作品。
	SelectedItem string
}

func DefaultSelectDesign() SelectDesign {
	return SelectDesign{
		Item:         "item0",
		Interval:     10,
		TextX:        0,
		TextY:        6,
		Alignment:    SelectDesignAlignCenter,
		DefaultColor: 0xffffffff,
		AlreadyColor: 0x88ffffff,
		SelectColor:  0xffff4488,
	}
}

var selectDesignFallback = []SelectDesign{DefaultSelectDesign()}

type SelectWindowConfiguration struct {
	DatumLine       int
	Option2AutoType int
	Designs         []SelectDesign
}

func NewSelectWindowConfiguration(datumLine, option2AutoType int, designs []SelectDesign) SelectWindowConfiguration {
	configuration := SelectWindowConfiguration{
		DatumLine:       datumLine,
		Option2AutoType: option2AutoType,
		Designs:         designs,
	}

	if configuration.DatumLine <= 0 {
		configuration.DatumLine = SelectWindowDefaultDatumLine
	}

	if len(configuration.Designs) == 0 {
		configuration.Designs = selectDesignFallback
	}

	return configuration
}

func DefaultSelectWindowConfiguration() SelectWindowConfiguration {
	return SelectWindowConfiguration{
		DatumLine:       SelectWindowDefaultDatumLine,
		Option2AutoType: 0,
		Designs:         selectDesignFallback,
	}
}

func (c SelectWindowConfiguration) Design(index int) SelectDesign {
	if index >= 0 && index < len(c.Designs) {
		return c.Designs[index]
	}

	return c.Designs[0]
}
