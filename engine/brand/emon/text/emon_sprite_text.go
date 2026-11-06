package text

import "strings"

const (
	EmonSpriteShadow            = 0x1
	EmonSpriteBlock             = 0x2
	EmonSpriteProportion        = 0x10
	EmonSpriteAntiArea          = 0x40000020
	EmonSpriteAntiAreaQuality   = 0x20
	EmonSpriteMessageSelectMask = 0xC000
	EmonSpriteMessageSelectLine = 0x4000
	EmonSpriteMessageSelectChar = 0x8000
	EmonSpriteMessageSelectWord = 0xC000
	EmonSpriteNoAutoHeight      = 0x10000
	EmonSpriteFontBold          = 0x20000
	EmonSpriteWeightNormal      = 400
	EmonSpriteWeightBold        = 700
	EmonSpriteQualityPlain      = 3
	EmonSpriteQualityAntiArea   = 4
	EmonSpriteMessageFlagMask   = 0x33
	EmonSpriteSystemFontFlag    = "FontFlag"
)

func InheritMessageFlags(own int, registered int) int {
	return own ^ ((own ^ registered) & EmonSpriteMessageFlagMask)
}

func MergeSystemFontFlag(own int, fontFlag int) int {
	return fontFlag | (own & ^EmonSpriteMessageFlagMask)
}

func ParseSpriteTextFlags(flags string) int {
	if len(flags) == 0 {
		return 0
	}

	value := 0
	if containsFold(flags, "PROPOTION") {
		value |= EmonSpriteProportion
	}

	if containsFold(flags, "ANTIEREA") {
		value |= EmonSpriteAntiArea
	}

	if containsFold(flags, "SHADOW") {
		value |= EmonSpriteShadow
	}

	if containsFold(flags, "BLOCK") {
		value |= EmonSpriteBlock
	}

	if containsFold(flags, "NOTAUTOHEIGHT") {
		value |= EmonSpriteNoAutoHeight
	}

	if containsFold(flags, "FONTBOLD") {
		value |= EmonSpriteFontBold
	}

	return value
}

func TryParseMessageSelect(keyword string, mode *int) bool {
	*mode = 0
	if len(keyword) == 0 {
		return false
	}

	if strings.EqualFold(keyword, "NULL") {
		return true
	}

	if strings.EqualFold(keyword, "LINE") {
		*mode = EmonSpriteMessageSelectLine
		return true
	}

	if strings.EqualFold(keyword, "CHAR") {
		*mode = EmonSpriteMessageSelectChar
		return true
	}

	if strings.EqualFold(keyword, "WORD") {
		*mode = EmonSpriteMessageSelectWord
		return true
	}

	return false
}

type EmonSpriteKind int

const (
	EmonSpriteKindNone        EmonSpriteKind = 0
	EmonSpriteKindGroup       EmonSpriteKind = 0x20
	EmonSpriteKindBack        EmonSpriteKind = 0x40000000
	EmonSpriteKindMiddleFront EmonSpriteKind = 0x10000000
	EmonSpriteKindMiddleBack  EmonSpriteKind = 0x20000000
)

type EmonSpriteFlags int

const (
	EmonSpriteFlagNone       EmonSpriteFlags = 0
	EmonSpriteFlagVisible    EmonSpriteFlags = 0x10000
	EmonSpriteFlagClip       EmonSpriteFlags = 4
	EmonSpriteFlagSelect     EmonSpriteFlags = 0x10
	EmonSpriteFlagEffectOnly EmonSpriteFlags = 8
	EmonSpriteFlagDrawMe     EmonSpriteFlags = 2
	EmonSpriteFlagClearMe    EmonSpriteFlags = 0x200000
	EmonSpriteFlagBack       EmonSpriteFlags = 0x40
	EmonSpriteFlagTile       EmonSpriteFlags = 0x100000
	EmonSpriteFlagMotion     EmonSpriteFlags = 0x800000
	EmonSpriteFlagAnimation  EmonSpriteFlags = 1
)

type EmonSpriteTextMetrics struct {
	LineHeight int
	Columns    int
	MaxLines   int
	WrapWidth  int
	Weight     int
	Quality    int
}

func NewEmonSpriteTextMetrics(boxWidth int, boxHeight int, size int, flags int, lineHeightOverride int) *EmonSpriteTextMetrics {
	unit := size
	if unit < 1 {
		unit = 1
	}

	lineHeight := lineHeightOverride
	if lineHeight == 0 {
		lineHeight = int(float64(unit) * 1.1)
	}

	if lineHeight < 1 {
		lineHeight = 1
	}

	normalizedWidth := boxWidth
	if normalizedWidth < 0 {
		normalizedWidth = 0
	}

	normalizedHeight := boxHeight
	if normalizedHeight < 0 {
		normalizedHeight = 0
	}

	columns := normalizedWidth / unit
	lines := normalizedHeight / lineHeight
	if lines == 0 {
		lines = 1
	}

	if (flags&EmonSpriteNoAutoHeight) == 0 && lineHeight > 1 {
		adjusted := normalizedHeight / lines
		if adjusted < 1 {
			adjusted = 1
		}
		lineHeight = adjusted
	}

	wrapWidth := boxWidth - ((3 * unit) / 2)
	if wrapWidth < 1 {
		wrapWidth = 1
	}

	weight := EmonSpriteWeightNormal
	if (flags & EmonSpriteFontBold) != 0 {
		weight = EmonSpriteWeightBold
	}

	quality := EmonSpriteQualityPlain
	if (flags & EmonSpriteAntiAreaQuality) != 0 {
		quality = EmonSpriteQualityAntiArea
	}

	return &EmonSpriteTextMetrics{
		LineHeight: lineHeight,
		Columns:    columns,
		MaxLines:   lines,
		WrapWidth:  wrapWidth,
		Weight:     weight,
		Quality:    quality,
	}
}

type EmonSpriteTextSetting struct {
	name               string
	x                  int
	y                  int
	family             string
	size               int
	boxWidth           int
	boxHeight          int
	flags              int
	lineHeightOverride int
	color              int
	shadowColor        int
	blockColor         int
	messageSelectMode  int
	metrics            *EmonSpriteTextMetrics
	revision           int
}

func NewEmonSpriteTextSetting(name string, x int, y int, family string, size int, boxWidth int, boxHeight int) *EmonSpriteTextSetting {
	return &EmonSpriteTextSetting{
		name:      name,
		x:         x,
		y:         y,
		family:    family,
		size:      size,
		boxWidth:  boxWidth,
		boxHeight: boxHeight,
		flags:     EmonSpriteProportion,
		color:     0xFFFFFF,
	}
}

func (s *EmonSpriteTextSetting) Name() string {
	return s.name
}

func (s *EmonSpriteTextSetting) X() int {
	return s.x
}

func (s *EmonSpriteTextSetting) Y() int {
	return s.y
}

func (s *EmonSpriteTextSetting) Family() string {
	return s.family
}

func (s *EmonSpriteTextSetting) Size() int {
	return s.size
}

func (s *EmonSpriteTextSetting) BoxWidth() int {
	return s.boxWidth
}

func (s *EmonSpriteTextSetting) BoxHeight() int {
	return s.boxHeight
}

func (s *EmonSpriteTextSetting) Flags() int {
	return s.flags
}

func (s *EmonSpriteTextSetting) SetFlags(value int) {
	if s.flags == value {
		return
	}

	s.flags = value
	s.metrics = nil
	s.revision++
}

func (s *EmonSpriteTextSetting) LineHeightOverride() int {
	return s.lineHeightOverride
}

func (s *EmonSpriteTextSetting) SetLineHeightOverride(value int) {
	if s.lineHeightOverride == value {
		return
	}

	s.lineHeightOverride = value
	s.metrics = nil
	s.revision++
}

func (s *EmonSpriteTextSetting) Color() int {
	return s.color
}

func (s *EmonSpriteTextSetting) SetColor(value int) {
	if s.color == value {
		return
	}

	s.color = value & 0xFFFFFF
	s.revision++
}

func (s *EmonSpriteTextSetting) ShadowColor() int {
	return s.shadowColor
}

func (s *EmonSpriteTextSetting) SetShadowColor(value int) {
	next := value & 0xFFFFFF
	if s.shadowColor == next {
		return
	}

	s.shadowColor = next
	s.revision++
}

func (s *EmonSpriteTextSetting) BlockColor() int {
	return s.blockColor
}

func (s *EmonSpriteTextSetting) SetBlockColor(value int) {
	next := value & 0xFFFFFF
	if s.blockColor == next {
		return
	}

	s.blockColor = next
	s.revision++
}

func (s *EmonSpriteTextSetting) MessageSelectMode() int {
	return s.messageSelectMode
}

func (s *EmonSpriteTextSetting) SetMessageSelectMode(value int) {
	next := value & EmonSpriteMessageSelectMask
	if s.messageSelectMode == next {
		return
	}

	s.messageSelectMode = next
	s.revision++
}

func (s *EmonSpriteTextSetting) Revision() int {
	return s.revision
}

func (s *EmonSpriteTextSetting) Metrics() *EmonSpriteTextMetrics {
	if s.metrics == nil {
		s.metrics = NewEmonSpriteTextMetrics(s.boxWidth, s.boxHeight, s.size, s.flags, s.lineHeightOverride)
	}

	return s.metrics
}

type EmonSpriteText struct {
	content  string
	revision int
}

func NewEmonSpriteText() *EmonSpriteText {
	return &EmonSpriteText{}
}

func (t *EmonSpriteText) Content() string {
	return t.content
}

func (t *EmonSpriteText) SetContent(value string) {
	if t.content == value {
		return
	}

	t.content = value
	t.revision++
}

func (t *EmonSpriteText) Revision() int {
	return t.revision
}

func (t *EmonSpriteText) IsEmpty() bool {
	return len(t.content) == 0
}

func (t *EmonSpriteText) Clear() {
	t.SetContent("")
}

func (t *EmonSpriteText) Append(text string) {
	if len(text) == 0 {
		return
	}

	if len(t.content) == 0 {
		t.SetContent(text)
		return
	}

	t.SetContent(t.content + "\n" + text)
}
